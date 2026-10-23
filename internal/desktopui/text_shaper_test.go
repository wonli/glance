package desktopui

import (
	"gioui.org/font"
	"gioui.org/text"
	"glance/internal/emojifont"
	"golang.org/x/image/math/fixed"
	"testing"
)

func TestWindowsEmojiQueriesUseBundledFace(t *testing.T) {
	for _, weight := range []font.Weight{font.Normal, font.SemiBold} {
		s := newTextShaperForOS("windows", nil, text.NoSystemFonts())
		s.LayoutString(text.Parameters{Font: font.Font{Typeface: emojifont.Family, Weight: weight}, PxPerEm: fixed.I(32), MaxWidth: 1000}, "🚀")
		g, ok := s.NextGlyph()
		if !ok || g.ID == 0 || g.Advance == 0 {
			t.Fatalf("weight %v: missing glyph %+v", weight, g)
		}
	}
}
