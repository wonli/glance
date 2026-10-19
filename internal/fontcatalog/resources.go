package fontcatalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

func (s Snapshot) OpenFace(id ResourceID) (io.ReadCloser, error) {
	data, err := s.ReadFace(id)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// ReadFace returns validated resource bytes without another whole-file copy.
func (s Snapshot) ReadFace(id ResourceID) ([]byte, error) {
	for _, f := range s.faces {
		if f.ID != id {
			continue
		}
		if f.Unavailable != "" {
			return nil, errors.New(f.Unavailable)
		}
		file, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, MaxSystemFontBytes+1))
		file.Close()
		if int64(len(data)) > MaxSystemFontBytes {
			return nil, errors.New("font resource exceeds size limit")
		}
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if f.Digest != "" && hex.EncodeToString(sum[:]) != f.Digest {
			return nil, errors.New("font resource changed; refresh fonts")
		}
		return data, nil
	}
	return nil, os.ErrNotExist
}

// PrivateResource is published only after successful download validation.
type PrivateResource struct{ Path string }
