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

// OpenFaceResource opens a validated font resource without retaining its
// contents in memory. The digest is checked by streaming the same open file,
// which is then rewound for callers that can parse through io.ReaderAt.
func (s Snapshot) OpenFaceResource(id ResourceID) (*os.File, error) {
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
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, err
		}
		if info.Size() > MaxSystemFontBytes {
			file.Close()
			return nil, errors.New("font resource exceeds size limit")
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(file, MaxSystemFontBytes+1))
		if err != nil {
			file.Close()
			return nil, err
		}
		if n > MaxSystemFontBytes {
			file.Close()
			return nil, errors.New("font resource exceeds size limit")
		}
		if f.Digest != "" && hex.EncodeToString(h.Sum(nil)) != f.Digest {
			file.Close()
			return nil, errors.New("font resource changed; refresh fonts")
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return nil, err
		}
		return file, nil
	}
	return nil, os.ErrNotExist
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
