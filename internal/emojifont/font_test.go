package emojifont

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	fontapi "github.com/go-text/typesetting/font"
	"image/png"
	"testing"
)

func TestBundledEmojiHasBitmapGlyphs(t *testing.T) {
	collection, err := Collection()
	if err != nil {
		t.Fatal(err)
	}
	if len(collection) != 1 {
		t.Fatalf("collection count=%d", len(collection))
	}
	face := collection[0].Face.Face()
	for _, r := range "🚀📑🎨📊🧮📈🎮🔖✨✅" {
		g, ok := face.NominalGlyph(r)
		if !ok || g == 0 {
			t.Fatalf("missing %c", r)
		}
		b, ok := face.GlyphData(g).(fontapi.GlyphBitmap)
		if !ok || b.Format != fontapi.PNG || len(b.Data) == 0 {
			t.Fatalf("%c has no PNG bitmap", r)
		}
	}
}

func TestBitmapPNGForTextPreservesEmojiSequences(t *testing.T) {
	for _, value := range []string{"🚀", "👩🏽‍💻", "👨‍👩‍👧‍👦", "🇨🇳", "1️⃣", "⚠️"} {
		data, width, height, ok := BitmapPNGForText(value)
		if !ok || width <= 0 || height <= 0 {
			t.Fatalf("%q did not shape to a color bitmap", value)
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != width || config.Height != height {
			t.Fatalf("%q bitmap size=%dx%d decoded=%dx%d err=%v", value, width, height, config.Width, config.Height, err)
		}
	}
}
func TestBundledEmojiUsesPrivateFamily(t *testing.T) {
	c, err := Collection()
	if err != nil {
		t.Fatal(err)
	}
	if string(c[0].Font.Typeface) != "Glance Color Emoji" {
		t.Fatalf("family=%s", c[0].Font.Typeface)
	}
}
func TestBundledEmojiSharesFontButNotFaces(t *testing.T) {
	a, e := Collection()
	if e != nil {
		t.Fatal(e)
	}
	b, e := Collection()
	if e != nil {
		t.Fatal(e)
	}
	x, y := a[0].Face.Face(), b[0].Face.Face()
	if x == y || x.Font != y.Font {
		t.Fatal("font data must be shared with independent faces")
	}
}

func TestBundledEmojiProvenance(t *testing.T) {
	raw, err := assets.ReadFile("assets/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Files map[string]struct {
			SHA256 string `json:"sha256"`
		}
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"NotoColorEmoji.ttf", "OFL.txt"} {
		data, err := assets.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		if sum != m.Files[name].SHA256 {
			t.Fatalf("digest mismatch for %s", name)
		}
	}
}
