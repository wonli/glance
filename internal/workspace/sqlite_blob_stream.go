package workspace

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
)

const sqliteBlobChunkSize = 1 << 20

// StreamDocumentPayload writes a document payload to dst. If the payload is
// stored in the sessions directory, it streams in bounded 1 MiB chunks from
// disk. If the database still holds a legacy BLOB in the documents table, it
// streams from SQLite.
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

	var sqlitePayload []byte
	err = s.db.QueryRow(`SELECT payload FROM documents WHERE id=?`, id).Scan(&sqlitePayload)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, sql.ErrNoRows
	}
	if err != nil {
		return 0, err
	}

	// Legacy BLOB fallback
	if len(sqlitePayload) > 0 {
		for offset := 0; offset < len(sqlitePayload); {
			end := offset + sqliteBlobChunkSize
			if end > len(sqlitePayload) {
				end = len(sqlitePayload)
			}
			chunk := sqlitePayload[offset:end]
			n, writeErr := dst.Write(chunk)
			if n < 0 || n > len(chunk) {
				return written, fmt.Errorf("payload writer returned invalid count %d for %d-byte chunk", n, len(chunk))
			}
			written += int64(n)
			if writeErr != nil {
				return written, writeErr
			}
			if n != len(chunk) {
				return written, io.ErrShortWrite
			}
			offset = end
		}
		return written, nil
	}

	target := s.payloadPath(id)
	if target == "" {
		return 0, nil
	}
	f, err := os.Open(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	buffer := make([]byte, sqliteBlobChunkSize)
	for {
		nr, rerr := f.Read(buffer)
		if nr > 0 {
			nw, werr := dst.Write(buffer[:nr])
			if nw < 0 || nw > nr {
				return written, fmt.Errorf("payload writer returned invalid count %d for %d-byte chunk", nw, nr)
			}
			written += int64(nw)
			if werr != nil {
				return written, werr
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return written, rerr
		}
	}
	return written, nil
}
