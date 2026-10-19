package wordfont

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"fmt"
	"sync"
	"weak"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
)

const parsedFontIndexLimit = 256

type parsedFontKey struct {
	digest [sha256.Size]byte
	index  uint32
}

type parsedFontValue struct {
	key  parsedFontKey
	font weak.Pointer[font.Font]
}

var sharedParsedFonts = struct {
	sync.Mutex
	items map[parsedFontKey]*list.Element
	lru   list.List
}{items: make(map[parsedFontKey]*list.Element)}

// ParseFont reuses immutable tables for identical contents and collection index.
// Each consumer must create its own font.Face: its glyph caches are mutable.
// The bounded weak index does not keep unused document fonts alive.
// Callers remain responsible for validating catalog resource identity before use.
func ParseFont(data []byte, index uint32) (*font.Font, error) {
	key := parsedFontKey{digest: sha256.Sum256(data), index: index}
	sharedParsedFonts.Lock()
	defer sharedParsedFonts.Unlock()
	item := sharedParsedFonts.items[key]
	if item != nil {
		sharedParsedFonts.lru.MoveToFront(item)
		if parsed := item.Value.(parsedFontValue).font.Value(); parsed != nil {
			return parsed, nil
		}
	}
	loaders, err := opentype.NewLoaders(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if uint64(index) >= uint64(len(loaders)) {
		return nil, fmt.Errorf("font collection index %d is out of range", index)
	}
	parsed, err := font.NewFont(loaders[index])
	if err != nil {
		return nil, err
	}
	value := parsedFontValue{key: key, font: weak.Make(parsed)}
	if item != nil {
		item.Value = value
	} else {
		sharedParsedFonts.items[key] = sharedParsedFonts.lru.PushFront(value)
		if sharedParsedFonts.lru.Len() > parsedFontIndexLimit {
			oldest := sharedParsedFonts.lru.Back()
			delete(sharedParsedFonts.items, oldest.Value.(parsedFontValue).key)
			sharedParsedFonts.lru.Remove(oldest)
		}
	}
	return parsed, nil
}
