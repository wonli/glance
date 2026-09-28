package desktopui

import (
	"glance/internal/document"
	"image/color"
	"testing"
)

func TestGridPaintStyleUsesCellStyle(t *testing.T) {
	s := paintStyle(document.CellStyle{FillColor: "#aabbcc", FontColor: "#102030", Bold: true, Italic: true, WrapText: true, HorizontalAlignment: "center", VerticalAlignment: "center"})
	if s.Fill != (color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 255}) || s.Font != (color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 255}) || !s.Bold || !s.Italic || !s.Wrap || s.Align != "center" || s.Vertical != "center" {
		t.Fatalf("style=%+v", s)
	}
}
