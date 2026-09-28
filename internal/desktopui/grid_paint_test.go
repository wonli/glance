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
	s := paintStyle(document.CellStyle{FillColor: "#aabbcc", FontColor: "#102030", Bold: true, Italic: true, WrapText: true, HorizontalAlignment: "center", VerticalAlignment: "center"})
	if s.Fill != (color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 255}) || s.Font != (color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 255}) || !s.Bold || !s.Italic || !s.Wrap || s.Align != "center" || s.Vertical != "center" {
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
	cell := document.Cell{Style: document.CellStyle{FontColor: "#123456", Bold: true, Italic: true, WrapText: true, HorizontalAlignment: "right"}}
	style := paintStyle(cell.Style)
	got := cellEditorStyle(theme, editor, cell.Style, style)
	if got.Color != style.Font || got.Font.Weight != 700 || got.Font.Style != font.Italic {
		t.Fatalf("editor font style=%+v", got)
	}
	if editor.Alignment != text.End || editor.SingleLine || !editor.Submit {
		t.Fatalf("editor alignment/singleline/submit=%v/%v/%v", editor.Alignment, editor.SingleLine, editor.Submit)
	}
	if got.TextSize != theme.TextSize*14.0/16.0 {
		t.Fatalf("editor text size=%v; want the rendered cell text size", got.TextSize)
	}
}
