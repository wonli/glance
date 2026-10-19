package desktopui

import (
	"fmt"
	giofont "gioui.org/font"
	"gioui.org/font/opentype"
	"glance/internal/fontcatalog"
	"glance/internal/fontfallback"
	"golang.org/x/image/font/gofont/goregular"
	"testing"
)

func TestUIFontFacesSelectOnlyRegularAndSemibold(t *testing.T) {
	var faces []fontcatalog.Face
	for _, weight := range []int{100, 200, 300, 400, 500, 600, 700, 800, 900} {
		faces = append(faces, fontcatalog.Face{ID: fontcatalog.ResourceID(fmt.Sprint(weight)), Family: "Inter", Weight: weight, Source: fontcatalog.System})
	}
	faces = append(faces, fontcatalog.Face{ID: "italic", Family: "Inter", Weight: 400, Italic: true, Source: fontcatalog.System})
	selected := uiFontFaces(fontcatalog.NewSnapshot(1, faces), newFontCollectionCache())
	if len(selected) != 2 {
		t.Fatalf("preloaded %d faces; want only the two UI weights", len(selected))
	}
	if selected[0].Weight != 400 || selected[1].Weight != 600 || selected[0].Italic || selected[1].Italic {
		t.Fatalf("wrong UI faces: %+v", selected)
	}
}

func TestUIFontFacesKeepFallbackAndPreferredSource(t *testing.T) {
	family := fontfallback.CJKFamilies()[0]
	snapshot := fontcatalog.NewSnapshot(1, []fontcatalog.Face{
		{ID: "regular", Family: family, Weight: 400, Source: fontcatalog.System},
		{ID: "private", Family: family, Weight: 400, Source: fontcatalog.Private, Preferred: true},
		{ID: "bold", Family: family, Weight: 700, Source: fontcatalog.System},
	})
	selected := uiFontFaces(snapshot, newFontCollectionCache())
	if len(selected) != 2 || selected[0].ID != "private" || selected[1].ID != "bold" {
		t.Fatalf("wrong fallback/source: %+v", selected)
	}
}

func TestUIFontFacesMatchGioMissingWeightRules(t *testing.T) {
	snapshot := fontcatalog.NewSnapshot(1, []fontcatalog.Face{
		{ID: "light", Family: "Inter", Weight: 300, Source: fontcatalog.System},
		{ID: "medium", Family: "Inter", Weight: 500, Source: fontcatalog.System},
		{ID: "bold", Family: "Inter", Weight: 700, Source: fontcatalog.System},
	})
	selected := uiFontFaces(snapshot, newFontCollectionCache())
	if len(selected) != 2 || selected[0].ID != "medium" || selected[1].ID != "bold" {
		t.Fatalf("missing UI weights differ from Gio's CSS matching: %+v", selected)
	}
}

func TestUIAndDocumentFontsShareCollectionBudget(t *testing.T) {
	// Use pre-parsed small fixtures with modeled source sizes, avoiding large files.
	uiFace := fontcatalog.Face{ID: "ui", Path: "ui.ttf", Family: "Inter", Weight: 400, Size: 70 << 20}
	docFace := fontcatalog.Face{ID: "doc", Path: "doc.ttf", Family: "Document", Weight: 400, Size: 70 << 20}
	snapshot := fontcatalog.NewSnapshot(1, []fontcatalog.Face{uiFace, docFace})
	cache := newFontCollectionCache()
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	for _, face := range []fontcatalog.Face{uiFace, docFace} {
		key := fontParseCacheKey(face)
		value := parsedFontEntry{key: key, faces: []giofont.FontFace{{Font: parsed.Font(), Face: parsed}}, bytes: face.Size}
		cache.parsedItems[key] = cache.parsedLRU.PushFront(value)
	}
	collection := fontCollectionForRendering(snapshot, []string{"Document"}, cache)
	for _, f := range collection {
		if f.Font.Typeface == "Inter" {
			t.Fatal("UI preload displaced a requested document font or used a second budget")
		}
	}
	if len(collection) != 1 {
		t.Fatalf("want only the requested document face within the shared budget, got %d", len(collection))
	}
}
