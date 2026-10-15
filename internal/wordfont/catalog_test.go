package wordfont

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/fontcatalog"
	"golang.org/x/image/font/gofont/goregular"
)

func TestCatalogShaperUsesPrivateFace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.ttf")
	os.WriteFile(path, goregular.TTF, 0600)
	faces, err := fontcatalog.Inspect(context.Background(), path, fontcatalog.Private)
	if err != nil {
		t.Fatal(err)
	}
	shaper, err := NewWithSnapshot(fontcatalog.NewSnapshot(1, faces))
	if err != nil {
		t.Fatal(err)
	}
	result, err := shaper.Shape("Hello", document.WordTextStyle{FontFamily: "Go", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	for _, g := range result.Glyphs {
		if !strings.Contains(g.FontKey, path) {
			t.Fatalf("wrong font: %+v", g)
		}
	}
}

func TestCatalogFontDataRejectsReplacedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "font.ttf")
	os.WriteFile(path, goregular.TTF, 0600)
	faces, _ := fontcatalog.Inspect(context.Background(), path, fontcatalog.Private)
	s, _ := NewWithSnapshot(fontcatalog.NewSnapshot(1, faces))
	key, err := s.ResolveSpreadsheetFont(document.CellStyle{FontFamily: "Go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("changed"), 0600)
	if _, err = s.FontData(key); err == nil {
		t.Fatal("FontData bypassed resource identity check")
	}
}

func TestCatalogShapeRejectsChangedResourceRatherThanSubstituting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "font.ttf")
	os.WriteFile(path, goregular.TTF, 0600)
	faces, _ := fontcatalog.Inspect(context.Background(), path, fontcatalog.Private)
	shaper, _ := NewWithSnapshot(fontcatalog.NewSnapshot(1, faces))
	os.WriteFile(path, []byte("replaced font"), 0600)
	if _, err := shaper.Shape("Hello", document.WordTextStyle{FontFamily: "Go", FontSize: 12}); err == nil {
		t.Fatal("changed resource silently substituted")
	}
}
