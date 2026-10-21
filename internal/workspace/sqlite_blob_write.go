package workspace

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
	sqlite3 "modernc.org/sqlite/lib"
)

// SaveDocumentStream atomically replaces a document payload from a reader.
// The payload is written in bounded chunks to a pre-sized SQLite BLOB, so large
// snapshots do not need a full-size Go or SQLite binding copy.
func (s *Store) SaveDocumentStream(doc Document, payload io.Reader, payloadSize int64) (err error) {
	if s == nil || s.db == nil {
		return errors.New("workspace store is not open")
	}
	if s.path == "" {
		return errors.New("streaming document payload writes requires a filesystem workspace database")
	}
	if doc.ID == "" || doc.Format == "" {
		return errors.New("document id and format are required")
	}
	if payload == nil {
		return errors.New("document payload reader is required")
	}
	if payloadSize < 0 || payloadSize > maxSQLiteBlobAPIInt {
		return fmt.Errorf("document payload size %d exceeds SQLite incremental BLOB API limit %d", payloadSize, maxSQLiteBlobAPIInt)
	}
	if len(doc.ID) > int(^uint32(0)>>1) || len(doc.Format) > int(^uint32(0)>>1) || int64(len(doc.ViewState)) > maxSQLiteBlobAPIInt {
		return errors.New("document metadata exceeds SQLite binding limits")
	}
	if doc.ViewState == nil {
		doc.ViewState = []byte{}
	}
	return s.saveDocumentStreamPlatform(doc, payload, payloadSize)
}

func (s *Store) saveDocumentStreamTranslated(doc Document, payload io.Reader, payloadSize int64) (err error) {
	writer, err := openSQLiteBlobWriter(s.path)
	if err != nil {
		return err
	}
	commit := false
	defer func() {
		if closeErr := writer.close(commit); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	if err := writer.replaceDocumentPayload(doc, payloadSize); err != nil {
		return err
	}
	if payloadSize > 0 {
		rowID, storedSize, err := writer.documentRowID(doc.ID)
		if err != nil {
			return err
		}
		if storedSize != payloadSize {
			return fmt.Errorf("SQLite document payload size %d does not match requested size %d", storedSize, payloadSize)
		}
		if err := writer.openPayloadBlob(rowID, true); err != nil {
			return err
		}
		if got := int64(sqlite3.Xsqlite3_blob_bytes(writer.tls, writer.blob)); got != payloadSize {
			return fmt.Errorf("SQLite writable BLOB size %d does not match requested size %d", got, payloadSize)
		}
		if err := writer.writePayload(payload, payloadSize); err != nil {
			return err
		}
	}
	if err := requireStreamEOF(payload); err != nil {
		return err
	}
	commit = true
	return nil
}

func openSQLiteBlobWriter(path string) (writer *sqliteBlob, err error) {
	r := &sqliteBlob{tls: libc.NewTLS()}
	defer func() {
		if err != nil {
			err = errors.Join(err, r.close(false))
			r = nil
		}
	}()

	dbPath, err := libc.CString(path)
	if err != nil {
		return nil, fmt.Errorf("allocate SQLite database path: %w", err)
	}
	defer libc.Xfree(r.tls, dbPath)
	dbOut := libc.Xmalloc(r.tls, types.Size_t(unsafe.Sizeof(uintptr(0))))
	if dbOut == 0 {
		return nil, errors.New("allocate SQLite database handle failed")
	}
	*(*uintptr)(unsafe.Pointer(dbOut)) = 0
	rc := sqlite3.Xsqlite3_open_v2(r.tls, dbPath, dbOut, sqlite3.SQLITE_OPEN_READWRITE|sqlite3.SQLITE_OPEN_FULLMUTEX, 0)
	r.db = *(*uintptr)(unsafe.Pointer(dbOut))
	libc.Xfree(r.tls, dbOut)
	if rc != sqlite3.SQLITE_OK {
		return nil, r.sqliteError("open workspace database for document payload write", rc)
	}
	if r.db == 0 {
		return nil, errors.New("SQLite opened the workspace without a database handle")
	}
	if rc := sqlite3.Xsqlite3_busy_timeout(r.tls, r.db, 5000); rc != sqlite3.SQLITE_OK {
		return nil, r.sqliteError("configure workspace database busy timeout", rc)
	}
	if err := r.exec("BEGIN IMMEDIATE"); err != nil {
		return nil, fmt.Errorf("begin workspace payload write transaction: %w", err)
	}
	r.transaction = true
	return r, nil
}

func (r *sqliteBlob) replaceDocumentPayload(doc Document, payloadSize int64) (err error) {
	const query = `INSERT INTO documents(id,format,payload,dirty,updated_at,view_state)
		VALUES(?,?,zeroblob(?),?,?,?)
		ON CONFLICT(id) DO UPDATE SET format=excluded.format,payload=excluded.payload,
			dirty=excluded.dirty,updated_at=excluded.updated_at,view_state=excluded.view_state`
	queryPointer, err := libc.CString(query)
	if err != nil {
		return fmt.Errorf("allocate document payload upsert query: %w", err)
	}
	stmtOut := libc.Xmalloc(r.tls, types.Size_t(unsafe.Sizeof(uintptr(0))))
	if stmtOut == 0 {
		libc.Xfree(r.tls, queryPointer)
		return errors.New("allocate SQLite statement handle failed")
	}
	*(*uintptr)(unsafe.Pointer(stmtOut)) = 0
	rc := sqlite3.Xsqlite3_prepare_v2(r.tls, r.db, queryPointer, -1, stmtOut, 0)
	r.stmt = *(*uintptr)(unsafe.Pointer(stmtOut))
	libc.Xfree(r.tls, stmtOut)
	if rc != sqlite3.SQLITE_OK {
		libc.Xfree(r.tls, queryPointer)
		return r.sqliteError("prepare document payload upsert", rc)
	}
	if r.stmt == 0 {
		libc.Xfree(r.tls, queryPointer)
		return errors.New("SQLite prepared an empty document payload upsert")
	}
	var idPointer, formatPointer, viewStatePointer uintptr
	defer func() {
		if r.stmt != 0 {
			finalizeRC := sqlite3.Xsqlite3_finalize(r.tls, r.stmt)
			r.stmt = 0
			if finalizeRC != sqlite3.SQLITE_OK {
				err = errors.Join(err, r.sqliteError("finalize document payload upsert", finalizeRC))
			}
		}
		if idPointer != 0 {
			libc.Xfree(r.tls, idPointer)
		}
		if formatPointer != 0 {
			libc.Xfree(r.tls, formatPointer)
		}
		if viewStatePointer != 0 {
			libc.Xfree(r.tls, viewStatePointer)
		}
		libc.Xfree(r.tls, queryPointer)
	}()

	idPointer, err = libc.CString(doc.ID)
	if err != nil {
		return fmt.Errorf("allocate document id binding: %w", err)
	}
	formatPointer, err = libc.CString(doc.Format)
	if err != nil {
		return fmt.Errorf("allocate document format binding: %w", err)
	}
	if rc := sqlite3.Xsqlite3_bind_text(r.tls, r.stmt, 1, idPointer, int32(len(doc.ID)), 0); rc != sqlite3.SQLITE_OK {
		return r.sqliteError("bind document id", rc)
	}
	if rc := sqlite3.Xsqlite3_bind_text(r.tls, r.stmt, 2, formatPointer, int32(len(doc.Format)), 0); rc != sqlite3.SQLITE_OK {
		return r.sqliteError("bind document format", rc)
	}
	if rc := sqlite3.Xsqlite3_bind_int64(r.tls, r.stmt, 3, payloadSize); rc != sqlite3.SQLITE_OK {
		return r.sqliteError("bind document payload size", rc)
	}
	dirty := int32(0)
	if doc.Dirty {
		dirty = 1
	}
	if rc := sqlite3.Xsqlite3_bind_int(r.tls, r.stmt, 4, dirty); rc != sqlite3.SQLITE_OK {
		return r.sqliteError("bind document dirty state", rc)
	}
	if rc := sqlite3.Xsqlite3_bind_int64(r.tls, r.stmt, 5, doc.UpdatedAt.UTC().UnixNano()); rc != sqlite3.SQLITE_OK {
		return r.sqliteError("bind document update time", rc)
	}
	if len(doc.ViewState) == 0 {
		if rc := sqlite3.Xsqlite3_bind_zeroblob(r.tls, r.stmt, 6, 0); rc != sqlite3.SQLITE_OK {
			return r.sqliteError("bind empty document view state", rc)
		}
	} else {
		viewStatePointer = libc.Xmalloc(r.tls, types.Size_t(len(doc.ViewState)))
		if viewStatePointer == 0 {
			return errors.New("allocate document view state binding failed")
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(viewStatePointer)), len(doc.ViewState)), doc.ViewState)
		if rc := sqlite3.Xsqlite3_bind_blob(r.tls, r.stmt, 6, viewStatePointer, int32(len(doc.ViewState)), 0); rc != sqlite3.SQLITE_OK {
			return r.sqliteError("bind document view state", rc)
		}
	}
	if rc := sqlite3.Xsqlite3_step(r.tls, r.stmt); rc != sqlite3.SQLITE_DONE {
		return r.sqliteError("write document payload metadata", rc)
	}
	return nil
}

func (r *sqliteBlob) writePayload(source io.Reader, size int64) error {
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
		if _, err := io.ReadFull(source, buffer[:int(chunkSize)]); err != nil {
			return fmt.Errorf("read document payload chunk at offset %d: %w", offset, err)
		}
		rc := sqlite3.Xsqlite3_blob_write(r.tls, r.blob, uintptr(unsafe.Pointer(&buffer[0])), int32(chunkSize), int32(offset))
		runtime.KeepAlive(buffer)
		if rc != sqlite3.SQLITE_OK {
			return r.sqliteError("write document payload chunk", rc)
		}
		offset += chunkSize
	}
	return nil
}

func requireStreamEOF(source io.Reader) error {
	var extra [1]byte
	n, err := io.ReadFull(source, extra[:])
	if n != 0 {
		return errors.New("document payload reader contains more bytes than declared")
	}
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("verify document payload length: %w", err)
	}
	return errors.New("document payload reader contains more bytes than declared")
}
