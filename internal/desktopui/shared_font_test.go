package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/fontcatalog"
	"golang.org/x/image/font/gofont/goregular"
)

func TestFontRenderingSharesDataButNotMutableFaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	faces, err := fontcatalog.Inspect(context.Background(), path, fontcatalog.System)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fontcatalog.NewSnapshot(1, faces)
	first, _, ok := parseFontCollection(snapshot, faces[0])
	if !ok {
		t.Fatal("first parse failed")
	}
	second, _, ok := parseFontCollection(snapshot, faces[0])
	if !ok {
		t.Fatal("second parse failed")
	}
	a, b := first[0].Face.Face(), second[0].Face.Face()
	if a.Font != b.Font {
		t.Fatal("independent renderers duplicated immutable font tables")
	}
	if a == b {
		t.Fatal("independent renderers share mutable Face caches")
	}
}
