package emojifont

import (
	"bytes"
	fontapi "github.com/go-text/typesetting/font"
	"image/png"
	"reflect"
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

func TestSplitTextKeepsEmojiPresentationClustersIntact(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []TextRun
	}{
		{text: "plain ⚠ text", want: []TextRun{{Text: "plain ⚠ text"}}},
		{text: "before ⚠️ after", want: []TextRun{{Text: "before "}, {Text: "⚠️", ColorEmoji: true}, {Text: " after"}}},
		{text: "x👩🏽‍💻y", want: []TextRun{{Text: "x"}, {Text: "👩🏽‍💻", ColorEmoji: true}, {Text: "y"}}},
	} {
		if got := SplitText(tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SplitText(%q)=%+v, want %+v", tc.text, got, tc.want)
		}
	}
}
