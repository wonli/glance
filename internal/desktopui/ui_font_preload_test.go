package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/fontmanager/fontfallback"
	"golang.org/x/image/font/gofont/goregular"
)

func TestShellDoesNotPreparseSystemUIFonts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	faces, err := fontcatalog.Inspect(context.Background(), path, fontcatalog.System)
	if err != nil {
		t.Fatal(err)
	}
	// Use real font bytes behind the platform's preferred UI family.
	faces[0].Family = fontfallback.CJKFamilies()[0]
	faces[0].Aliases = []string{faces[0].Family}
	snapshot := fontcatalog.NewSnapshot(1, faces)
	cache := newFontCollectionCache()
	collection := fontCollectionForRendering(snapshot, nil, cache)
	if len(collection) != 0 || cache.parsedBytes != 0 {
		t.Fatalf("shell redundantly parsed system font: descriptors=%d bytes=%d", len(collection), cache.parsedBytes)
	}
	// A document explicitly using that family must still get its selected font.
	collection = fontCollectionForRendering(snapshot, []string{faces[0].Family}, cache)
	if len(collection) == 0 {
		t.Fatal("requested document font was dropped")
	}
	if _, ok := collection[0].Face.Face().NominalGlyph('G'); !ok {
		t.Fatal("requested document font is unusable")
	}
	fontCollectionForRendering(snapshot, nil, cache)
	if cache.parsedBytes != 0 {
		t.Fatal("closing document retained the redundant UI parse")
	}
	// Private UI sources are not discoverable through Gio's system scanner.
	privateFaces, err := fontcatalog.Inspect(context.Background(), path, fontcatalog.Private)
	if err != nil {
		t.Fatal(err)
	}
	privateFaces[0].Family = faces[0].Family
	privateFaces[0].Aliases = []string{faces[0].Family}
	privateSnapshot := fontcatalog.NewSnapshot(2, privateFaces)
	privateCache := newFontCollectionCache()
	if got := fontCollectionForRendering(privateSnapshot, nil, privateCache); len(got) == 0 {
		t.Fatal("private UI font registration was removed with system preloads")
	}
}
