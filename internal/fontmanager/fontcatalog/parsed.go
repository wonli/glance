package fontcatalog

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
)

var (
	parsedFacesMu sync.RWMutex
	parsedFaces   = make(map[ResourceID]*font.Font)
)

// OpenParsedFace returns a parsed, immutable *font.Font for the face identified by id.
// Font tables are parsed with zero-copy sub-slicing directly from mmap virtual memory.
// Parsed instances are deduplicated globally across all consumers (UI, Word, PDF).
func (s Snapshot) OpenParsedFace(id ResourceID) (*font.Font, error) {
	parsedFacesMu.RLock()
	cached := parsedFaces[id]
	parsedFacesMu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	for _, f := range s.faces {
		if f.ID != id {
			continue
		}
		if f.Unavailable != "" {
			return nil, fmt.Errorf("font face unavailable: %s", f.Unavailable)
		}

		data, err := s.ReadFace(id)
		if err != nil {
			return nil, err
		}

		loaders, err := opentype.NewLoaders(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if uint64(f.Index) >= uint64(len(loaders)) {
			return nil, fmt.Errorf("font collection index %d out of range", f.Index)
		}

		parsed, err := font.NewFont(loaders[f.Index])
		if err != nil {
			return nil, err
		}

		parsedFacesMu.Lock()
		parsedFaces[id] = parsed
		parsedFacesMu.Unlock()

		return parsed, nil
	}
	return nil, os.ErrNotExist
}
