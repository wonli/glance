//go:build darwin && cgo

package workspace

/*
#cgo LDFLAGS: -lsqlite3
#include <sqlite3.h>
#include <stdlib.h>

static int bindText(sqlite3_stmt *stmt, int index, const char *value, int length) {
    return sqlite3_bind_text(stmt, index, value, length, SQLITE_TRANSIENT);
}
static int bindBytes(sqlite3_stmt *stmt, int index, const void *value, int length) {
    return sqlite3_bind_blob(stmt, index, value, length, SQLITE_TRANSIENT);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"unsafe"
)

// The translated SQLite allocator retains two PDF-sized anonymous mappings
// after an incremental BLOB write. The system SQLite implementation performs
// the same transaction while keeping its temporary storage bounded.
func (s *Store) saveDocumentStreamPlatform(doc Document, payload io.Reader, payloadSize int64) (err error) {
	path := C.CString(s.path)
	defer C.free(unsafe.Pointer(path))
	var db *C.sqlite3
	if rc := C.sqlite3_open_v2(path, &db, C.SQLITE_OPEN_READWRITE|C.SQLITE_OPEN_FULLMUTEX, nil); rc != C.SQLITE_OK {
		message := "open workspace database"
		if db != nil {
			message = C.GoString(C.sqlite3_errmsg(db))
			C.sqlite3_close_v2(db)
		}
		return fmt.Errorf("open workspace database: %s (SQLite %d)", message, int(rc))
	}
	defer func() {
		if rc := C.sqlite3_close_v2(db); rc != C.SQLITE_OK {
			err = errors.Join(err, sqliteNativeError(db, "close workspace database", rc))
		}
	}()
	if rc := C.sqlite3_busy_timeout(db, 5000); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "configure workspace busy timeout", rc)
	}
	if err := sqliteNativeExec(db, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, sqliteNativeExec(db, "ROLLBACK"))
		}
	}()
	if err := sqliteNativeInsertSnapshot(db, doc, payloadSize); err != nil {
		return err
	}
	if payloadSize > 0 {
		if err := sqliteNativeWriteBlob(db, doc.ID, payload, payloadSize); err != nil {
			return err
		}
	}
	if err := requireStreamEOF(payload); err != nil {
		return err
	}
	if err := sqliteNativeExec(db, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func sqliteNativeInsertSnapshot(db *C.sqlite3, doc Document, payloadSize int64) (err error) {
	exists, err := sqliteNativeDocumentExists(db, doc.ID)
	if err != nil {
		return err
	}
	query := `INSERT INTO documents(id,format,payload,dirty,updated_at,view_state)
		VALUES(?,?,zeroblob(?),?,?,?)`
	if exists {
		query = `UPDATE documents SET format=?,payload=zeroblob(?),dirty=?,updated_at=?,view_state=? WHERE id=?`
	}
	statement := C.CString(query)
	defer C.free(unsafe.Pointer(statement))
	var stmt *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(db, statement, -1, &stmt, nil); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "prepare document snapshot", rc)
	}
	defer func() {
		if rc := C.sqlite3_finalize(stmt); rc != C.SQLITE_OK {
			err = errors.Join(err, sqliteNativeError(db, "finalize document snapshot", rc))
		}
	}()
	id, format := C.CString(doc.ID), C.CString(doc.Format)
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(format))
	idIndex, formatIndex := C.int(1), C.int(2)
	sizeIndex, dirtyIndex, timeIndex, stateIndex := C.int(3), C.int(4), C.int(5), C.int(6)
	if exists {
		idIndex, formatIndex = 6, 1
		sizeIndex, dirtyIndex, timeIndex, stateIndex = 2, 3, 4, 5
	}
	if rc := C.bindText(stmt, idIndex, id, C.int(len(doc.ID))); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "bind document id", rc)
	}
	if rc := C.bindText(stmt, formatIndex, format, C.int(len(doc.Format))); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "bind document format", rc)
	}
	if rc := C.sqlite3_bind_int64(stmt, sizeIndex, C.sqlite3_int64(payloadSize)); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "bind document payload size", rc)
	}
	dirty := C.int(0)
	if doc.Dirty {
		dirty = 1
	}
	if rc := C.sqlite3_bind_int(stmt, dirtyIndex, dirty); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "bind document dirty state", rc)
	}
	if rc := C.sqlite3_bind_int64(stmt, timeIndex, C.sqlite3_int64(doc.UpdatedAt.UTC().UnixNano())); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "bind document update time", rc)
	}
	if len(doc.ViewState) == 0 {
		if rc := C.sqlite3_bind_zeroblob(stmt, stateIndex, 0); rc != C.SQLITE_OK {
			return sqliteNativeError(db, "bind empty document view state", rc)
		}
	} else if rc := C.bindBytes(stmt, stateIndex, unsafe.Pointer(&doc.ViewState[0]), C.int(len(doc.ViewState))); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "bind document view state", rc)
	}
	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return sqliteNativeError(db, "write document snapshot", rc)
	}
	return nil
}

func sqliteNativeDocumentExists(db *C.sqlite3, id string) (bool, error) {
	query := C.CString("SELECT 1 FROM documents WHERE id=?")
	defer C.free(unsafe.Pointer(query))
	var stmt *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(db, query, -1, &stmt, nil); rc != C.SQLITE_OK {
		return false, sqliteNativeError(db, "prepare document lookup", rc)
	}
	identity := C.CString(id)
	rc := C.bindText(stmt, 1, identity, C.int(len(id)))
	C.free(unsafe.Pointer(identity))
	if rc != C.SQLITE_OK {
		result := sqliteNativeError(db, "bind document lookup", rc)
		C.sqlite3_finalize(stmt)
		return false, result
	}
	rc = C.sqlite3_step(stmt)
	if rc != C.SQLITE_ROW && rc != C.SQLITE_DONE {
		result := sqliteNativeError(db, "query document lookup", rc)
		C.sqlite3_finalize(stmt)
		return false, result
	}
	exists := rc == C.SQLITE_ROW
	if rc := C.sqlite3_finalize(stmt); rc != C.SQLITE_OK {
		return false, sqliteNativeError(db, "finalize document lookup", rc)
	}
	return exists, nil
}

func sqliteNativeWriteBlob(db *C.sqlite3, id string, source io.Reader, size int64) (err error) {
	rowQuery := C.CString("SELECT rowid,length(payload) FROM documents WHERE id=?")
	defer C.free(unsafe.Pointer(rowQuery))
	var stmt *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(db, rowQuery, -1, &stmt, nil); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "prepare document rowid", rc)
	}
	identity := C.CString(id)
	if rc := C.bindText(stmt, 1, identity, C.int(len(id))); rc != C.SQLITE_OK {
		C.free(unsafe.Pointer(identity))
		C.sqlite3_finalize(stmt)
		return sqliteNativeError(db, "bind document rowid", rc)
	}
	C.free(unsafe.Pointer(identity))
	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_ROW {
		C.sqlite3_finalize(stmt)
		return sqliteNativeError(db, "query document rowid", rc)
	}
	rowID := C.sqlite3_column_int64(stmt, 0)
	storedSize := int64(C.sqlite3_column_int64(stmt, 1))
	if rc := C.sqlite3_finalize(stmt); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "finalize document rowid", rc)
	}
	if storedSize != size {
		return fmt.Errorf("SQLite document payload size %d does not match requested size %d", storedSize, size)
	}
	database, table, column := C.CString("main"), C.CString("documents"), C.CString("payload")
	defer C.free(unsafe.Pointer(database))
	defer C.free(unsafe.Pointer(table))
	defer C.free(unsafe.Pointer(column))
	var blob *C.sqlite3_blob
	if rc := C.sqlite3_blob_open(db, database, table, column, rowID, 1, &blob); rc != C.SQLITE_OK {
		return sqliteNativeError(db, "open document payload BLOB", rc)
	}
	defer func() {
		if rc := C.sqlite3_blob_close(blob); rc != C.SQLITE_OK {
			err = errors.Join(err, sqliteNativeError(db, "close document payload BLOB", rc))
		}
	}()
	if got := int64(C.sqlite3_blob_bytes(blob)); got != size {
		return fmt.Errorf("SQLite writable BLOB size %d does not match requested size %d", got, size)
	}
	buffer := make([]byte, min(int64(sqliteBlobChunkSize), size))
	for offset := int64(0); offset < size; {
		chunk := int(min(int64(len(buffer)), size-offset))
		if _, err := io.ReadFull(source, buffer[:chunk]); err != nil {
			return fmt.Errorf("read document payload chunk at offset %d: %w", offset, err)
		}
		if rc := C.sqlite3_blob_write(blob, unsafe.Pointer(&buffer[0]), C.int(chunk), C.int(offset)); rc != C.SQLITE_OK {
			return sqliteNativeError(db, "write document payload chunk", rc)
		}
		runtime.KeepAlive(buffer)
		offset += int64(chunk)
	}
	return nil
}

func sqliteNativeExec(db *C.sqlite3, query string) error {
	statement := C.CString(query)
	defer C.free(unsafe.Pointer(statement))
	if rc := C.sqlite3_exec(db, statement, nil, nil, nil); rc != C.SQLITE_OK {
		return sqliteNativeError(db, query, rc)
	}
	return nil
}

func sqliteNativeError(db *C.sqlite3, operation string, code C.int) error {
	return fmt.Errorf("%s: SQLite result code %d: %s", operation, int(code), C.GoString(C.sqlite3_errmsg(db)))
}
