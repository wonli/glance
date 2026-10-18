package desktopui

import (
	"bytes"
	"image"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
)

func TestSpreadsheetRichEditorLiveFormattingPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []int{1, 2} {
		size := image.Pt(300*scale, 44*scale)
		renderer, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("GPU unavailable: %v", err)
		}
		theme := material.NewTheme()
		render := func(run document.CellRichTextRun) ([]byte, []widget.Region) {
			var ops op.Ops
			var editor widget.Editor
			editor.SetText("AB中文CD")
			editor.SetCaret(2, 4)
			editor.PaintSelectionWhenUnfocused = true
			runs := []document.CellRichTextRun{{Text: "AB"}, run, {Text: "CD"}}
			gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}}
			style := document.CellStyle{}
			paintCellRichEditor(gtx, theme, &editor, nil, image.Rectangle{Max: size}, image.Rectangle{Max: size}, style, paintStyle(style), LightColorTokens.Grid.SelectionBorder, runs, 1)
			if err := renderer.Frame(&ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			if err := renderer.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			return shot.Pix, editor.Regions(2, 4, nil)
		}
		plain, geometry := render(document.CellRichTextRun{Text: "中文"})
		for _, run := range []document.CellRichTextRun{{Text: "中文", Bold: true}, {Text: "中文", Italic: true}, {Text: "中文", Underline: true}} {
			styled, regions := render(run)
			if bytes.Equal(plain, styled) {
				t.Errorf("%dx live formatting has no visible effect: %+v", scale, run)
			}
			if len(regions) != len(geometry) {
				t.Fatal("formatting changed editor geometry")
			}
			for i := range regions {
				if regions[i] != geometry[i] {
					t.Fatal("formatting moved selection")
				}
			}
		}
		renderer.Release()
	}
}

// Exercise the real toolbar event -> workbook -> viewport -> editing grid path.
func TestSpreadsheetRichEditorToolbarToGridPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, slot := range []int{4, 5, 6} {
		t.Run(map[int]string{4: "bold", 5: "italic", 6: "underline"}[slot], func(t *testing.T) {
			h := newFullGridTextHarness(t, false, "AB中文CD")
			h.app.window = nil
			if err := h.app.book.SetCellText(h.app.sheet, "A1", "AB中文CD"); err != nil {
				t.Fatal(err)
			}
			h.editor.SetCaret(2, 4)
			renderer, err := headless.NewWindow(400, 90)
			if err != nil {
				t.Skipf("GPU unavailable: %v", err)
			}
			defer renderer.Release()
			capture := func() []byte {
				h.frame()
				if err := renderer.Frame(&h.ops); err != nil {
					t.Fatal(err)
				}
				shot := image.NewRGBA(image.Rect(0, 0, 400, 90))
				if err := renderer.Screenshot(shot); err != nil {
					t.Fatal(err)
				}
				return shot.Pix
			}
			before := capture()
			h.app.toolbarActionButtons[slot].Click()
			h.app.handleSpreadsheetToolbarEvents(layout.Context{})
			after := capture()
			if bytes.Equal(before, after) {
				t.Fatal("toolbar formatting did not reach active grid editor")
			}
			style, mixed, ok := h.app.spreadsheetTextSelectionStyle(slot)
			if !ok || mixed || !formatStyleActive(slot, style) {
				t.Fatal("toolbar selection not active after click")
			}
			if !h.app.cellEdit.Editing || h.editor.SelectedText() != "中文" {
				t.Fatal("toolbar click lost cell edit or selection")
			}
		})
	}
}

func TestSpreadsheetRichWrappedCellPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []int{1, 2} {
		size := image.Pt(80*scale, 160*scale)
		renderer, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("GPU unavailable: %v", err)
		}
		var ops op.Ops
		style := document.CellStyle{WrapText: true}
		cell := document.Cell{Value: "自动换行加粗之后继续自动换行", Style: style, RichText: []document.CellRichTextRun{{Text: "自动换行"}, {Text: "加粗之后", Bold: true}, {Text: "继续自动换行"}}}
		gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}}
		paintStyledCell(gtx, material.NewTheme(), cell, image.Rectangle{Max: size}, paintStyle(style), false)
		if err := renderer.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		shot := image.NewRGBA(image.Rectangle{Max: size})
		if err := renderer.Screenshot(shot); err != nil {
			t.Fatal(err)
		}
		ink := 0
		for y := 30 * scale; y < 100*scale; y++ {
			for x := 6 * scale; x < 75*scale; x++ {
				c := shot.RGBAAt(x, y)
				if c.R < 150 && c.G < 150 && c.B < 150 && c.A > 0 {
					ink++
				}
			}
		}
		renderer.Release()
		if ink == 0 {
			t.Fatalf("%dx wrapped rich text has no glyphs below first line", scale)
		}
	}
}

func TestSpreadsheetRichItalicSurvivesBrowseRendering(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []int{1, 2} {
		for _, wrap := range []bool{false, true} {
			h := newFullGridTextHarness(t, false, "测试中文斜体内容")
			h.app.window = nil
			h.size = image.Pt(400*scale, 90*scale)
			draw := h.draw
			h.draw = func(gtx layout.Context) {
				gtx.Metric = unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}
				draw(gtx)
			}
			if err := h.app.book.SetCellText(h.app.sheet, "A1", "测试中文斜体内容"); err != nil {
				t.Fatal(err)
			}
			h.app.virtualGrid.Window.Cells[0][0].Style.WrapText = wrap
			h.editor.SetCaret(2, 6)
			h.app.toolbarActionButtons[5].Click()
			h.app.handleSpreadsheetToolbarEvents(layout.Context{})
			if err := h.app.commitGridCell(h.editor.Text()); err != nil {
				t.Fatal(err)
			}
			if h.app.cellEdit.Editing {
				t.Fatal("still editing after commit")
			}
			renderer, err := headless.NewWindow(h.size.X, h.size.Y)
			if err != nil {
				t.Skipf("GPU unavailable: %v", err)
			}
			capture := func() []byte {
				h.frame()
				if err := renderer.Frame(&h.ops); err != nil {
					t.Fatal(err)
				}
				shot := image.NewRGBA(image.Rectangle{Max: h.size})
				if err := renderer.Screenshot(shot); err != nil {
					t.Fatal(err)
				}
				return shot.Pix
			}
			italic := capture()
			cell := &h.app.virtualGrid.Window.Cells[0][0]
			found := false
			for i := range cell.RichText {
				if cell.RichText[i].Italic {
					found = true
				}
				cell.RichText[i].Italic = false
			}
			if !found {
				t.Fatal("commit lost italic run")
			}
			h.app.virtualGrid.CellPaintCache = nil
			plain := capture()
			renderer.Release()
			if bytes.Equal(italic, plain) {
				t.Errorf("%dx wrap=%v: browsing italic looks identical to regular text", scale, wrap)
			}
		}
	}
}
