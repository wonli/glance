package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/fontcatalog"
	"golang.org/x/image/font/gofont/goregular"
)

func TestFontPickerToggleKeepsRenderingAndDoesNotPreloadPrivateFonts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	faces, err := fontcatalog.Inspect(context.Background(), path, fontcatalog.Private)
	if err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	a.initFontPicker()
	defer a.stopAndWaitManagedBackground()
	a.fontPicker.snapshot = fontcatalog.NewSnapshot(1, faces)
	a.prepareFontRendering()
	select {
	case result := <-a.fontPicker.renderResults:
		a.fontPicker.renderPending = ""
		a.fontPicker.renderAccepted = result.key
		a.theme.Shaper = result.shaper
	case <-time.After(5 * time.Second):
		t.Fatal("initial render timed out")
	}
	original := a.theme.Shaper
	for _, open := range []bool{true, false, true, false} {
		a.fontPicker.open = open
		a.prepareFontRendering()
		if a.fontPicker.renderPending != "" {
			t.Fatalf("picker open=%v triggered font rebuild", open)
		}
	}
	if a.theme.Shaper != original {
		t.Fatal("picker replaced document shaper")
	}
	if len(a.fontPicker.renderCache.parsedItems) != 0 {
		t.Fatal("picker parsed unused private fonts")
	}
}

func TestFontRenderingDropsUnusedDocumentFontCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	faces, err := fontcatalog.Inspect(context.Background(), path, fontcatalog.Private)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fontcatalog.NewSnapshot(1, faces)
	cache := newFontCollectionCache()
	collection := fontCollectionForRendering(snapshot, []string{faces[0].Family}, cache)
	if len(collection) == 0 || cache.parsedBytes == 0 {
		t.Fatal("document font was not loaded")
	}
	// Returning to the shell has no document font demand.
	fontCollectionForRendering(snapshot, nil, cache)
	if len(cache.parsedItems) != 0 || cache.parsedBytes != 0 {
		t.Fatalf("unused document font retained: entries=%d bytes=%d", len(cache.parsedItems), cache.parsedBytes)
	}
	// Existing renderers remain valid while their frame finishes.
	if _, ok := collection[0].Face.Face().NominalGlyph('A'); !ok {
		t.Fatal("eviction invalidated existing font")
	}
}
