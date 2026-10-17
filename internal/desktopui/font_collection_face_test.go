package desktopui

import (
	"encoding/binary"
	giofont "gioui.org/font"
	"gioui.org/font/opentype"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/fontcatalog"
	"golang.org/x/image/font/gofont/goregular"
)

// An unrelated broken face must not prevent loading the requested TTC face.
// Parsing every face also multiplies the table memory of large CJK collections.
func TestFontRenderingParsesOnlyRequestedCollectionFace(t *testing.T) {
	data := make([]byte, 20)
	copy(data, "ttcf")
	binary.BigEndian.PutUint32(data[4:], 0x10000)
	binary.BigEndian.PutUint32(data[8:], 2)
	binary.BigEndian.PutUint32(data[12:], 20)
	fontData := append([]byte(nil), goregular.TTF...)
	tables := int(binary.BigEndian.Uint16(fontData[4:]))
	for i := 0; i < tables; i++ {
		offset := fontData[12+16*i+8:]
		binary.BigEndian.PutUint32(offset, binary.BigEndian.Uint32(offset)+20)
	}
	data = append(data, fontData...)
	binary.BigEndian.PutUint32(data[16:], uint32(len(data)))
	data = append(data, make([]byte, 12)...)
	binary.BigEndian.PutUint32(data[len(data)-12:], 0x10000)
	path := filepath.Join(t.TempDir(), "collection.ttc")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	face := fontcatalog.Face{ID: "selected", Path: path, Index: 0, Size: int64(len(data)), Family: "Go"}
	snapshot := fontcatalog.NewSnapshot(1, []fontcatalog.Face{face})
	fonts, _, ok := parseFontCollection(snapshot, face)
	if !ok || len(fonts) != 1 {
		t.Fatalf("requested face unavailable: ok=%v count=%d", ok, len(fonts))
	}
	if _, ok := fonts[0].Face.Face().NominalGlyph('A'); !ok {
		t.Fatal("requested face has no A glyph")
	}
}

func TestFontCollectionBudgetCountsCollectionFileOnce(t *testing.T) {
	// Model two already parsed faces of a 70 MiB TTC without allocating padding.
	// The resource budget limits unique source files, not the number of weights.
	face0 := fontcatalog.Face{ID: "regular", Path: "large.ttc", Index: 0, Size: 70 << 20, Family: "Regular"}
	face1 := face0
	face1.ID, face1.Index, face1.Family = "bold", 1, "Bold"
	snapshot := fontcatalog.NewSnapshot(1, []fontcatalog.Face{face0, face1})
	cache := newFontCollectionCache()
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []fontcatalog.Face{face0, face1} {
		key := fontParseCacheKey(f)
		entry := parsedFontEntry{key: key, faces: []giofont.FontFace{{Font: parsed.Font(), Face: parsed}}, bytes: 70 << 20}
		cache.parsedItems[key] = cache.parsedLRU.PushFront(entry)
	}
	collection := fontCollectionWithCache(snapshot, []string{"Regular", "Bold"}, cache)
	if len(collection) != 2 {
		t.Fatalf("collection budget dropped a requested weight: got %d faces, want 2", len(collection))
	}
}
