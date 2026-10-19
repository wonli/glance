package desktopui

import (
	"bytes"
	"image"
	"testing"

	giofont "gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	fontapi "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/font/gofont/goregular"
)

type observedDropdownCmap struct {
	fontapi.Cmap
	lookups map[rune]int
}

func (c observedDropdownCmap) Lookup(r rune) (fontapi.GID, bool) {
	c.lookups[r]++
	return c.Cmap.Lookup(r)
}

func TestRibbonDropdownDoesNotShapeIndicatorGlyph(t *testing.T) {
	loaders, err := opentype.NewLoaders(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	ft, err := fontapi.NewFont(loaders[0])
	if err != nil {
		t.Fatal(err)
	}
	observed := observedDropdownCmap{ft.Cmap, make(map[rune]int)}
	ft.Cmap = observed
	theme := material.NewTheme()
	theme.Face = "probe"
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection([]giofont.FontFace{{Font: giofont.Font{Typeface: "probe"}, Face: selectedCollectionFace{ft}}}))
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(80, 68))}
	layoutOfficeRibbonCommandContents(gtx, theme, nil, "Paste ▾")
	if observed.lookups['P'] == 0 {
		t.Fatal("label was not shaped")
	}
	if observed.lookups['▾'] != 0 {
		t.Fatal("dropdown indicator reached font shaping and can load a large fallback font")
	}
}
