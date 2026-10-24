package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// The incremental SQLite BLOB API limit retained for signature validation.
const maxSQLiteBlobAPIInt = int64(1<<31 - 1)

// SaveDocumentStream atomically replaces a document payload from a reader.
// The payload is streamed in bounded chunks directly to the sessions filesystem directory,
// keeping Go allocations and SQLite database file size bounded.
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

	target := s.payloadPath(doc.ID)
	if target == "" {
		return errors.New("cannot determine document payload path")
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-session-*")
	if err != nil {
		return fmt.Errorf("create temporary session file: %w", err)
	}
	tmpPath := tmpFile.Name()
	var committed bool
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	var written int64
	if payloadSize > 0 {
		buf := make([]byte, 64*1024)
		lr := io.LimitReader(payload, payloadSize)
		n, copyErr := io.CopyBuffer(tmpFile, lr, buf)
		written = n
		if copyErr != nil {
			_ = tmpFile.Close()
			return copyErr
		}
		if written != payloadSize {
			_ = tmpFile.Close()
			return fmt.Errorf("unexpected EOF writing document payload: wrote %d, want %d", written, payloadSize)
		}
	}
	if err := requireStreamEOF(payload); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync session file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close session file: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("commit session file: %w", err)
	}
	committed = true

	dirty := 0
	if doc.Dirty {
		dirty = 1
	}
	_, err = s.db.Exec(`INSERT INTO documents(id,format,payload,dirty,updated_at,view_state)
		VALUES(?,?,X'',?,?,?)
		ON CONFLICT(id) DO UPDATE SET format=excluded.format,payload=X'',
			dirty=excluded.dirty,updated_at=excluded.updated_at,view_state=excluded.view_state`,
		doc.ID, doc.Format, dirty, doc.UpdatedAt.UTC().UnixNano(), doc.ViewState)
	return err
}

func requireStreamEOF(source io.Reader) error {
	var probe [1]byte
	n, err := source.Read(probe[:])
	if errors.Is(err, io.EOF) || (err == nil && n == 0) {
		return nil
	}
	if err == nil {
		return errors.New("document payload reader has more data than declared size")
	}
	return err
}
