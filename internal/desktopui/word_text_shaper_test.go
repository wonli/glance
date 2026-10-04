package desktopui

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
	"glance/internal/document"
)

func TestWordTextShaperMixedCJKLatinMissingGlyphUsesReportedFallback(t *testing.T) {
	shaper, err := newWordTextShaperWithCacheDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shaped, err := shaper.Shape("A中B", document.WordTextStyle{FontFamily: "font-family-that-does-not-exist", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(shaped.Glyphs) < 3 {
		t.Fatalf("shaped %q into %d glyphs; want fallback output for Latin and Chinese", "A中B", len(shaped.Glyphs))
	}
	foundChinese := false
	foundChineseOutline := false
	for _, glyph := range shaped.Glyphs {
		data, err := shaper.FontData(glyph.FontKey)
		if err != nil || len(data) == 0 {
			t.Fatalf("reported font key %q has no embeddable font bytes: size=%d err=%v", glyph.FontKey, len(data), err)
		}
		faces, err := font.ParseTTC(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("reported font key %q cannot be parsed for PDF embedding: %v", glyph.FontKey, err)
		}
		marker := strings.LastIndex(glyph.FontKey, "#face=")
		if marker < 0 {
			t.Fatalf("reported font key %q does not identify its collection face", glyph.FontKey)
		}
		faceIndexText := strings.SplitN(glyph.FontKey[marker+len("#face="):], "#", 2)[0]
		faceIndex, err := strconv.Atoi(faceIndexText)
		if err != nil || faceIndex < 0 || faceIndex >= len(faces) {
			t.Fatalf("reported font key %q has invalid TTC face index %q (faces=%d, err=%v)", glyph.FontKey, faceIndexText, len(faces), err)
		}
		for index := glyph.RuneStart; index < glyph.RuneEnd; index++ {
			if []rune("A中B")[index] != '中' {
				continue
			}
			foundChinese = true
			glyphID, covered := faces[faceIndex].NominalGlyph('中')
			if !covered {
				t.Errorf("reported fallback face %d in font %q does not contain the CJK rune 中", faceIndex, glyph.FontKey)
			} else if uint32(glyphID) != glyph.GlyphID {
				t.Errorf("reported glyph ID %d for 中 does not match face %d cmap ID %d in %q", glyph.GlyphID, faceIndex, glyphID, glyph.FontKey)
			} else {
				outline, err := shaper.GlyphOutline(glyph.FontKey, glyph.GlyphID)
				if err != nil || outline.UnitsPerEm <= 0 || len(outline.Commands) == 0 {
					t.Errorf("reported CJK glyph %d has no vector outline: commands=%d unitsPerEm=%g err=%v", glyph.GlyphID, len(outline.Commands), outline.UnitsPerEm, err)
				} else {
					foundChineseOutline = true
				}
			}
		}
	}
	if !foundChinese {
		t.Fatal("shaper returned no glyph cluster covering the Chinese rune")
	}
	if !foundChineseOutline {
		t.Fatal("CJK fallback glyph did not resolve to a vector outline")
	}
}
