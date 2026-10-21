package workspace

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"runtime"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	sqlite3 "modernc.org/sqlite/lib"
)

const sqliteBlobChunkSize = 1 << 20

// The incremental SQLite BLOB API uses signed 32-bit sizes and offsets.
const maxSQLiteBlobAPIInt = int64(1<<31 - 1)

// sqliteBlob owns a low-level SQLite connection and the handles bound to its
// transaction. The connection is private to one payload stream.
type sqliteBlob struct {
	tls         *libc.TLS
	db          uintptr
	stmt        uintptr
	blob        uintptr
	transaction bool
	closed      bool
}

func (s *Store) StreamDocumentPayload(id string, dst io.Writer) (written int64, err error) {
	if s == nil || s.db == nil {
		return 0, errors.New("workspace store is not open")
	}
	if s.path == "" {
		return 0, errors.New("streaming document payloads requires a filesystem workspace database")
	}
	if dst == nil {
		return 0, errors.New("payload destination writer is required")
	}
	reader, size, err := openSQLiteBlob(s.path, id)
	if err != nil {
		return 0, err
	}
	commit := false
	defer func() {
		if closeErr := reader.close(commit); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if size == 0 {
		commit = true
		return 0, nil
	}

	bufferSize := int64(sqliteBlobChunkSize)
	if size < bufferSize {
		bufferSize = size
	}
	buffer := make([]byte, int(bufferSize))
	for offset := int64(0); offset < size; {
		chunkSize := int64(len(buffer))
		if remaining := size - offset; remaining < chunkSize {
			chunkSize = remaining
		}
		rc := sqlite3.Xsqlite3_blob_read(reader.tls, reader.blob, uintptr(unsafe.Pointer(&buffer[0])), int32(chunkSize), int32(offset))
		runtime.KeepAlive(buffer)
		if rc != sqlite3.SQLITE_OK {
			return written, reader.sqliteError("read document payload chunk", rc)
		}
		n, writeErr := dst.Write(buffer[:int(chunkSize)])
		runtime.KeepAlive(buffer)
		if n < 0 || n > int(chunkSize) {
			return written, fmt.Errorf("payload writer returned invalid count %d for %d-byte chunk", n, chunkSize)
		}
		written += int64(n)
		if writeErr != nil {
			return written, writeErr
		}
		if int64(n) != chunkSize {
			return written, io.ErrShortWrite
		}
		offset += chunkSize
	}
	commit = true
	return written, nil
}

func openSQLiteBlob(path, id string) (reader *sqliteBlob, size int64, err error) {
	r := &sqliteBlob{tls: libc.NewTLS()}
	defer func() {
		if err != nil {
			err = errors.Join(err, r.close(false))
			r = nil
		}
	}()

	dbPath, err := libc.CString(path)
	if err != nil {
		return nil, 0, fmt.Errorf("allocate SQLite database path: %w", err)
	}
	defer libc.Xfree(r.tls, dbPath)
	dbOut := libc.Xmalloc(r.tls, types.Size_t(unsafe.Sizeof(uintptr(0))))
	if dbOut == 0 {
		return nil, 0, errors.New("allocate SQLite database handle failed")
	}
	*(*uintptr)(unsafe.Pointer(dbOut)) = 0
	rc := sqlite3.Xsqlite3_open_v2(r.tls, dbPath, dbOut, sqlite3.SQLITE_OPEN_READONLY|sqlite3.SQLITE_OPEN_FULLMUTEX, 0)
	r.db = *(*uintptr)(unsafe.Pointer(dbOut))
	libc.Xfree(r.tls, dbOut)
	if rc != sqlite3.SQLITE_OK {
		return nil, 0, r.sqliteError("open read-only workspace database", rc)
	}
	if r.db == 0 {
		return nil, 0, errors.New("SQLite opened the workspace without a database handle")
	}
	if rc := sqlite3.Xsqlite3_busy_timeout(r.tls, r.db, 5000); rc != sqlite3.SQLITE_OK {
		return nil, 0, r.sqliteError("configure workspace database busy timeout", rc)
	}
	if err := r.exec("BEGIN"); err != nil {
		return nil, 0, fmt.Errorf("begin workspace payload read transaction: %w", err)
	}
	r.transaction = true
	rowID, size, err := r.documentRowID(id)
	if err != nil {
		return nil, 0, err
	}
	if size < 0 {
		return nil, 0, errors.New("SQLite returned a negative document payload size")
	}
	if size > maxSQLiteBlobAPIInt {
		return nil, 0, fmt.Errorf("document payload size %d exceeds SQLite incremental BLOB API limit %d", size, maxSQLiteBlobAPIInt)
	}
	if err := r.openPayloadBlob(rowID, false); err != nil {
		return nil, 0, err
	}
	blobSize := int64(sqlite3.Xsqlite3_blob_bytes(r.tls, r.blob))
	if blobSize < 0 || blobSize != size {
		return nil, 0, fmt.Errorf("SQLite document payload size changed inside read transaction: metadata=%d BLOB=%d", size, blobSize)
	}
	reader = r
	return reader, size, nil
}

func (r *sqliteBlob) documentRowID(id string) (rowID, size int64, err error) {
	query, err := libc.CString(`SELECT rowid,length(payload) FROM documents WHERE id=?`)
	if err != nil {
		return 0, 0, fmt.Errorf("allocate document rowid query: %w", err)
	}
	defer libc.Xfree(r.tls, query)
	stmtOut := libc.Xmalloc(r.tls, types.Size_t(unsafe.Sizeof(uintptr(0))))
	if stmtOut == 0 {
		return 0, 0, errors.New("allocate SQLite statement handle failed")
	}
	defer libc.Xfree(r.tls, stmtOut)
	*(*uintptr)(unsafe.Pointer(stmtOut)) = 0
	rc := sqlite3.Xsqlite3_prepare_v2(r.tls, r.db, query, -1, stmtOut, 0)
	r.stmt = *(*uintptr)(unsafe.Pointer(stmtOut))
	if rc != sqlite3.SQLITE_OK {
		return 0, 0, r.sqliteError("prepare document rowid query", rc)
	}
	if r.stmt == 0 {
		return 0, 0, errors.New("SQLite prepared an empty document rowid query")
	}
	var idPointer uintptr
	defer func() {
		if r.stmt != 0 {
			finalizeRC := sqlite3.Xsqlite3_finalize(r.tls, r.stmt)
			r.stmt = 0
			if finalizeRC != sqlite3.SQLITE_OK {
				err = errors.Join(err, r.sqliteError("finalize document rowid query", finalizeRC))
			}
		}
		if idPointer != 0 {
			libc.Xfree(r.tls, idPointer)
		}
	}()
	if len(id) > int(^uint32(0)>>1) {
		return 0, 0, errors.New("workspace document id exceeds SQLite's bind length limit")
	}
	idPointer, err = libc.CString(id)
	if err != nil {
		return 0, 0, fmt.Errorf("allocate document id binding: %w", err)
	}
	if rc := sqlite3.Xsqlite3_bind_text(r.tls, r.stmt, 1, idPointer, int32(len(id)), 0); rc != sqlite3.SQLITE_OK {
		return 0, 0, r.sqliteError("bind document id", rc)
	}
	switch rc := sqlite3.Xsqlite3_step(r.tls, r.stmt); rc {
	case sqlite3.SQLITE_ROW:
		return int64(sqlite3.Xsqlite3_column_int64(r.tls, r.stmt, 0)), int64(sqlite3.Xsqlite3_column_int64(r.tls, r.stmt, 1)), nil
	case sqlite3.SQLITE_DONE:
		return 0, 0, sql.ErrNoRows
	default:
		return 0, 0, r.sqliteError("query document rowid", rc)
	}
}

func (r *sqliteBlob) openPayloadBlob(rowID int64, writable bool) error {
	databaseName, err := libc.CString("main")
	if err != nil {
		return fmt.Errorf("allocate SQLite schema name: %w", err)
	}
	defer libc.Xfree(r.tls, databaseName)
	tableName, err := libc.CString("documents")
	if err != nil {
		return fmt.Errorf("allocate SQLite table name: %w", err)
	}
	defer libc.Xfree(r.tls, tableName)
	columnName, err := libc.CString("payload")
	if err != nil {
		return fmt.Errorf("allocate SQLite column name: %w", err)
	}
	defer libc.Xfree(r.tls, columnName)
	blobOut := libc.Xmalloc(r.tls, types.Size_t(unsafe.Sizeof(uintptr(0))))
	if blobOut == 0 {
		return errors.New("allocate SQLite BLOB handle failed")
	}
	defer libc.Xfree(r.tls, blobOut)
	*(*uintptr)(unsafe.Pointer(blobOut)) = 0
	readWrite := int32(0)
	if writable {
		readWrite = 1
	}
	rc := sqlite3.Xsqlite3_blob_open(r.tls, r.db, databaseName, tableName, columnName, rowID, readWrite, blobOut)
	r.blob = *(*uintptr)(unsafe.Pointer(blobOut))
	if rc != sqlite3.SQLITE_OK {
		return r.sqliteError("open document payload BLOB", rc)
	}
	if r.blob == 0 {
		return errors.New("SQLite opened the document payload without a BLOB handle")
	}
	return nil
}

func (r *sqliteBlob) exec(query string) error {
	queryPointer, err := libc.CString(query)
	if err != nil {
		return fmt.Errorf("allocate SQLite command: %w", err)
	}
	defer libc.Xfree(r.tls, queryPointer)
	if rc := sqlite3.Xsqlite3_exec(r.tls, r.db, queryPointer, 0, 0, 0); rc != sqlite3.SQLITE_OK {
		return r.sqliteError(query, rc)
	}
	return nil
}

func (r *sqliteBlob) sqliteError(operation string, rc int32) error {
	message := ""
	if r != nil && r.tls != nil && r.db != 0 {
		if pointer := sqlite3.Xsqlite3_errmsg(r.tls, r.db); pointer != 0 {
			message = libc.GoString(pointer)
		}
	}
	if message == "" {
		return fmt.Errorf("%s: SQLite result code %d", operation, rc)
	}
	return fmt.Errorf("%s: SQLite result code %d: %s", operation, rc, message)
}

func (r *sqliteBlob) close(commit bool) (err error) {
	if r == nil || r.closed {
		return nil
	}
	r.closed = true
	if r.blob != 0 {
		if rc := sqlite3.Xsqlite3_blob_close(r.tls, r.blob); rc != sqlite3.SQLITE_OK {
			err = errors.Join(err, r.sqliteError("close document payload BLOB", rc))
		}
		r.blob = 0
	}
	if r.stmt != 0 {
		if rc := sqlite3.Xsqlite3_finalize(r.tls, r.stmt); rc != sqlite3.SQLITE_OK {
			err = errors.Join(err, r.sqliteError("finalize document rowid query", rc))
		}
		r.stmt = 0
	}
	if r.transaction {
		if commit && err == nil {
			if commitErr := r.exec("COMMIT"); commitErr != nil {
				err = errors.Join(err, commitErr, r.exec("ROLLBACK"))
			} else {
				r.transaction = false
			}
		} else {
			err = errors.Join(err, r.exec("ROLLBACK"))
			r.transaction = false
		}
	}
	if r.db != 0 {
		if rc := sqlite3.Xsqlite3_close_v2(r.tls, r.db); rc != sqlite3.SQLITE_OK {
			err = errors.Join(err, r.sqliteError("close workspace database connection", rc))
		}
		r.db = 0
	}
	if r.tls != nil {
		r.tls.Close()
		r.tls = nil
	}
	return err
}
