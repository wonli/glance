package desktopui

import (
	"strings"

	giofont "gioui.org/font"
	"glance/internal/fontmanager/fontcatalog"
)

// A single builder accounts for both shell and document fonts. TTC weights share
// a source-file charge, while parsed faces retain distinct collection indices.
type fontCollectionBuilder struct {
	snapshot    fontcatalog.Snapshot
	cache       *fontCollectionCache
	collection  []giofont.FontFace
	loaded      map[string][]giofont.FontFace
	files       map[string]bool
	descriptors map[giofont.Font]bool
	bytes       int64
}

func newFontCollectionBuilder(snapshot fontcatalog.Snapshot, cache *fontCollectionCache) *fontCollectionBuilder {
	return &fontCollectionBuilder{snapshot: snapshot, cache: cache, loaded: make(map[string][]giofont.FontFace), files: make(map[string]bool), descriptors: make(map[giofont.Font]bool)}
}

func (b *fontCollectionBuilder) add(face fontcatalog.Face, aliases []string) {
	if face.Unavailable != "" {
		return
	}
	fileKey := face.Path + "\x00" + face.Digest
	if !b.files[fileKey] && face.Size > fontCollectionCacheLimit-b.bytes {
		return
	}
	parseKey := fontParseCacheKey(face)
	fonts, ok := b.loaded[parseKey]
	if !ok {
		var size int64
		fonts, size, ok = b.cache.parsedFaces(b.snapshot, face)
		if !ok || len(fonts) == 0 {
			return
		}
		if !b.files[fileKey] && size > fontCollectionCacheLimit-b.bytes {
			return
		}
		b.loaded[parseKey] = fonts
		if !b.files[fileKey] {
			b.bytes += size
			b.files[fileKey] = true
		}
	}
	if len(fonts) == 0 {
		return
	}
	for _, alias := range aliases {
		if strings.TrimSpace(alias) == "" {
			continue
		}
		f := fonts[0]
		f.Font.Typeface = giofont.Typeface(alias)
		if !b.descriptors[f.Font] {
			b.collection = append(b.collection, f)
			b.descriptors[f.Font] = true
		}
	}
}

func (b *fontCollectionBuilder) addFamilies(names []string) {
	for _, name := range names {
		for _, family := range b.cache.familiesFor(b.snapshot, name) {
			aliases := append([]string{family.Name}, family.Aliases...)
			for _, face := range family.Faces {
				b.add(face, aliases)
			}
		}
	}
}
