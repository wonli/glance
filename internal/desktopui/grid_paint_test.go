package desktopui

import (
	"glance/internal/document"
	"image"
	"image/color"
	"testing"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func TestGridPaintStyleUsesCellStyle(t *testing.T) {
	s := paintStyle(document.CellStyle{FillColor: "#aabbcc", FontColor: "#102030", FontFamily: "宋体", Bold: true, Italic: true, Underline: true, WrapText: true, HorizontalAlignment: "center", VerticalAlignment: "center"})
	if s.Fill != (color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 255}) || s.Font != (color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 255}) || s.FontFamily != "宋体" || !s.Bold || !s.Italic || !s.Underline || !s.Wrap || s.Align != "center" || s.Vertical != "center" {
		t.Fatalf("style=%+v", s)
	}
}

func TestCachedCellPaintReusesLayoutAfterScroll(t *testing.T) {
	v := newGridViewport()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	theme := material.NewTheme()
	cell := document.Cell{Value: "centered", Style: document.CellStyle{HorizontalAlignment: "center"}}
	style := paintStyle(cell.Style)
	full := image.Rect(0, 0, 120, 24)
	paintCachedCell(gtx, v, theme, cell, full, full.Intersect(image.Rect(0, 0, 80, 24)), style)
	if got := len(v.CellPaintCache); got != 1 {
		t.Fatalf("cache entries after initial paint=%d; want 1", got)
	}
	entry := firstCellPaintEntry(v)
	ops.Reset()
	gtx.Ops = &ops
	scrolled := image.Rect(0, -7, 120, 17)
	paintCachedCell(gtx, v, theme, cell, scrolled, scrolled.Intersect(image.Rect(0, 0, 80, 24)), style)
	if got := len(v.CellPaintCache); got != 1 {
		t.Fatalf("scroll added a text layout; cache entries=%d", got)
	}
	if firstCellPaintEntry(v) != entry {
		t.Fatal("scroll replaced the recorded cell layout")
	}
}

func TestCellPaintCacheRecordsOnlyWhenContentGeometryChanges(t *testing.T) {
	v := newGridViewport()
	key := cellPaintKey{Value: "stable", Size: image.Pt(90, 24), PxPerDp: 2, PxPerSp: 2}
	records := 0
	record := func(*cellPaintEntry) { records++ }
	v.cachedCellPaint(key, record)
	v.cachedCellPaint(key, record)
	if records != 1 {
		t.Fatalf("stable cell was recorded %d times; want once", records)
	}
	key.Value = "changed"
	v.cachedCellPaint(key, record)
	if records != 2 {
		t.Fatalf("content change produced %d recordings; want 2", records)
	}
	key.Value = "changed"
	key.Size.X++
	v.cachedCellPaint(key, record)
	if records != 3 {
		t.Fatalf("geometry change produced %d recordings; want 3", records)
	}
}

func firstCellPaintEntry(v *gridViewport) *cellPaintEntry {
	for _, entry := range v.CellPaintCache {
		return entry
	}
	return nil
}

func TestCellEditorUsesRenderedStyleAndKeepsSingleCellGeometry(t *testing.T) {
	theme := material.NewTheme()
	editor := &widget.Editor{Submit: true}
	cell := document.Cell{Style: document.CellStyle{FontColor: "#123456", FontFamily: "宋体", Bold: true, Italic: true, WrapText: true, HorizontalAlignment: "right"}}
	style := paintStyle(cell.Style)
	got := cellEditorStyle(theme, editor, cell.Style, style)
	if got.Color != style.Font || got.Font.Typeface != cellTypeface(style) || got.Font.Weight != font.Bold || got.Font.Style != font.Italic {
		t.Fatalf("editor font style=%+v", got)
	}
	if editor.Alignment != text.End || editor.SingleLine || !editor.Submit {
		t.Fatalf("editor alignment/singleline/submit=%v/%v/%v", editor.Alignment, editor.SingleLine, editor.Submit)
	}
	if got.TextSize != theme.TextSize*14.0/16.0 {
		t.Fatalf("editor text size=%v; want the rendered cell text size", got.TextSize)
	}
}

func TestGridFontWeightsDistinguishBoldFromRegular(t *testing.T) {
	regular := cellFontWeight(false)
	bold := cellFontWeight(true)
	if regular != font.Normal || bold != font.Bold || bold <= regular {
		t.Fatalf("font weights regular=%d bold=%d; want %d < %d", regular, bold, font.Normal, font.Bold)
	}
}

func TestCellTypefaceKeepsWorkbookFamilyAndAddsCJKFallbacks(t *testing.T) {
	got := cellTypeface(gridPaintStyle{FontFamily: "微软雅黑"})
	want := font.Typeface("微软雅黑, " + string(preferredSystemTypeface))
	if got != want {
		t.Fatalf("cell typeface=%q; want workbook family followed by CJK fallbacks %q", got, want)
	}
	if got := cellTypeface(gridPaintStyle{}); got != preferredSystemTypeface {
		t.Fatalf("default cell typeface=%q; want platform CJK fallback list %q", got, preferredSystemTypeface)
	}
}

func TestAppThemePrefersSystemCJKFonts(t *testing.T) {
	if got := New(nil, "").theme.Face; got != preferredSystemTypeface {
		t.Fatalf("theme typeface=%q; want preferred CJK fallback list %q", got, preferredSystemTypeface)
	}
}

func TestCellLayoutAlignmentCombinesAxes(t *testing.T) {
	for _, tc := range []struct {
		horizontal, vertical string
		want                 layout.Direction
	}{{"", "", layout.W}, {"left", "center", layout.W}, {"center", "center", layout.Center}, {"right", "top", layout.NE}, {"right", "bottom", layout.SE}} {
		if got := cellLayoutAlignment(tc.horizontal, tc.vertical); got != tc.want {
			t.Errorf("cellLayoutAlignment(%q,%q)=%v; want %v", tc.horizontal, tc.vertical, got, tc.want)
		}
	}
}
