package fontcatalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
)

type cachedFaceFile struct {
	data    []byte
	size    int64
	modTime int64
	digest  string
	unmap   func()
}

var (
	faceFileCacheMu sync.RWMutex
	faceFileCache   = make(map[string]*cachedFaceFile)
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
		size := info.Size()
		if size > MaxSystemFontBytes {
			file.Close()
			return nil, errors.New("font resource exceeds size limit")
		}
		modTime := info.ModTime().UnixNano()

		faceFileCacheMu.RLock()
		cached := faceFileCache[f.Path]
		cachedValid := cached != nil && cached.size == size && cached.modTime == modTime && (f.Digest == "" || cached.digest == f.Digest)
		faceFileCacheMu.RUnlock()

		if !cachedValid && f.Digest != "" {
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
			if hex.EncodeToString(h.Sum(nil)) != f.Digest {
				file.Close()
				return nil, errors.New("font resource changed; refresh fonts")
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				file.Close()
				return nil, err
			}
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

		info, err := os.Stat(f.Path)
		if err != nil {
			return nil, err
		}
		size := info.Size()
		if size > MaxSystemFontBytes {
			return nil, errors.New("font resource exceeds size limit")
		}
		modTime := info.ModTime().UnixNano()

		faceFileCacheMu.RLock()
		cached := faceFileCache[f.Path]
		if cached != nil && cached.size == size && cached.modTime == modTime {
			if f.Digest == "" || cached.digest == f.Digest {
				faceFileCacheMu.RUnlock()
				return cached.data, nil
			}
		}
		faceFileCacheMu.RUnlock()

		file, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}

		data, unmap, mapErr := mapFaceFile(file, size)
		if mapErr != nil {
			data, err = io.ReadAll(io.LimitReader(file, MaxSystemFontBytes+1))
			file.Close()
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > MaxSystemFontBytes {
				return nil, errors.New("font resource exceeds size limit")
			}
		} else {
			file.Close()
		}

		sum := sha256.Sum256(data)
		sumHex := hex.EncodeToString(sum[:])
		if f.Digest != "" && sumHex != f.Digest {
			if unmap != nil {
				unmap()
			}
			return nil, errors.New("font resource changed; refresh fonts")
		}

		entry := &cachedFaceFile{
			data:    data,
			size:    size,
			modTime: modTime,
			digest:  sumHex,
			unmap:   unmap,
		}
		faceFileCacheMu.Lock()
		if old := faceFileCache[f.Path]; old != nil && old.unmap != nil && old != entry {
			old.unmap()
		}
		faceFileCache[f.Path] = entry
		faceFileCacheMu.Unlock()

		return data, nil
	}
	return nil, os.ErrNotExist
}

// PrivateResource is published only after successful download validation.
type PrivateResource struct{ Path string }
