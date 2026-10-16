package desktopui

import (
	"image"
	"testing"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

type gridTextHarness struct {
	router             input.Router
	ops                op.Ops
	editor             *widget.Editor
	draw               func(layout.Context)
	now                time.Duration
	app                *App
	paragraphSelection *textEditorParagraphSelection
}

func newGridTextHarness(t *testing.T, formula bool, value string) *gridTextHarness {
	t.Helper()
	h := &gridTextHarness{editor: &widget.Editor{}}
	h.editor.SetText(value)
	theme := material.NewTheme()
	var selection textEditorParagraphSelection
	h.paragraphSelection = &selection
	h.draw = func(gtx layout.Context) {
		selection.layout(gtx, h.editor, func(gtx layout.Context) layout.Dimensions {
			if formula {
				return layoutFormulaEditorField(gtx, theme, h.editor, "", 0)
			} else {
				style := document.CellStyle{WrapText: true}
				paintCellEditor(gtx, theme, h.editor, image.Rect(0, 0, 400, 90), image.Rect(0, 0, 400, 90), style, paintStyle(style), LightColorTokens.Grid.SelectionBorder)
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
		})
	}
	h.frame()
	return h
}
func (h *gridTextHarness) frame() {
	h.ops.Reset()
	h.draw(layout.Context{Ops: &h.ops, Source: h.router.Source(), Now: time.Unix(0, int64(h.now)), Constraints: layout.Exact(image.Pt(400, 90)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
	h.router.Frame(&h.ops)
}
func (h *gridTextHarness) point(kind pointer.Kind, x, y float32) {
	h.now += 30 * time.Millisecond
	buttons := pointer.ButtonPrimary
	if kind == pointer.Release {
		buttons = 0
	}
	h.router.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(x, y), Buttons: buttons, Time: h.now})
	h.frame()
}
func TestGridTextDragSelectionDelete(t *testing.T) {
	for _, formula := range []bool{false, true} {
		name := "cell"
		if formula {
			name = "formula"
		}
		t.Run(name, func(t *testing.T) {
			h := newGridTextHarness(t, formula, "alpha beta gamma")
			y := float32(40)
			if formula {
				y = 14
			}
			h.point(pointer.Press, 25, y)
			h.point(pointer.Move, 95, y)
			h.point(pointer.Release, 95, y)
			selected := h.editor.SelectedText()
			if selected == "" {
				t.Fatal("drag did not select text")
			}
			start, end := h.editor.Selection()
			if start > end {
				start, end = end, start
			}
			r := []rune(h.editor.Text())
			want := string(r[:start]) + string(r[end:])
			h.router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
			h.frame()
			if got := h.editor.Text(); got != want {
				t.Fatalf("Delete after selecting %q = %q; want %q", selected, got, want)
			}
		})
	}
}
func TestGridTextDoubleClickSelectsParagraph(t *testing.T) {
	for _, formula := range []bool{false, true} {
		name := "cell"
		if formula {
			name = "formula"
		}
		t.Run(name, func(t *testing.T) {
			value := "alpha beta gamma"
			h := newGridTextHarness(t, formula, value)
			y := float32(40)
			if formula {
				y = 14
			}
			h.point(pointer.Press, 60, y)
			if h.paragraphSelection.paragraphSelected {
				t.Fatal("single click reported paragraph selection")
			}
			h.point(pointer.Release, 60, y)
			h.point(pointer.Press, 60, y)
			if !h.paragraphSelection.paragraphSelected {
				t.Fatal("double click did not report paragraph selection")
			}
			h.point(pointer.Release, 60, y)
			if got := h.editor.SelectedText(); got != value {
				t.Fatalf("double click selected %q; want paragraph %q", got, value)
			}
			if h.paragraphSelection.paragraphSelected {
				t.Fatal("paragraph selection notification survived another frame")
			}
		})
	}
}

func newFullGridTextHarness(t *testing.T, formula bool, value string) *gridTextHarness {
	t.Helper()
	book := xlsx.NewBlank()
	t.Cleanup(func() { _ = book.Close() })
	state := newDocumentSessionState("")
	state.book = book
	state.sheet = book.SheetNames()[0]
	state.selectedCell = "A1"
	state.cellEdit.selectCell("A1", value)
	state.cellEdit.begin()
	state.cellEditor.SetText(value)
	state.formulaEditor.SetText(value)
	state.selectionSet = true
	v := newGridViewport()
	state.virtualGrid = v
	v.WindowSheet = state.sheet
	v.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1000, EndColumn: 1000}
	v.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{{Value: value, Style: document.CellStyle{WrapText: true}}}}}
	_ = v.Rows.SetSize(0, 60)
	_ = v.Columns.SetSize(0, 350)
	a := &App{documentSessionState: state, theme: material.NewTheme(), window: new(gioapp.Window)}
	h := &gridTextHarness{editor: &state.cellEditor, app: a}
	h.draw = func(gtx layout.Context) { a.layoutVirtualGrid(gtx) }
	if formula {
		h.editor = &state.formulaEditor
		h.draw = func(gtx layout.Context) { a.layoutGridControls(gtx) }
	}
	h.frame()
	return h
}

func TestGridTextFullLayoutDragSelectionDelete(t *testing.T) {
	for _, formula := range []bool{false, true} {
		name := "cell"
		if formula {
			name = "formula"
		}
		t.Run(name, func(t *testing.T) {
			h := newFullGridTextHarness(t, formula, "alpha beta gamma")
			x, y := float32(65), float32(62)
			if formula {
				x = 205
				y = 33
			}
			h.point(pointer.Press, x, y)
			h.point(pointer.Move, x+70, y)
			h.point(pointer.Release, x+70, y)
			selected := h.editor.SelectedText()
			if selected == "" {
				t.Fatal("drag did not select text")
			}
			start, end := h.editor.Selection()
			if start > end {
				start, end = end, start
			}
			r := []rune(h.editor.Text())
			want := string(r[:start]) + string(r[end:])
			h.router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
			h.frame()
			if got := h.editor.Text(); got != want {
				t.Fatalf("Delete after full-layout drag selecting %q = %q; want %q", selected, got, want)
			}
		})
	}
}
func TestGridTextFullLayoutDoubleClickSelectsParagraph(t *testing.T) {
	for _, formula := range []bool{false, true} {
		name := "cell"
		if formula {
			name = "formula"
		}
		t.Run(name, func(t *testing.T) {
			value := "alpha beta gamma"
			h := newFullGridTextHarness(t, formula, value)
			x, y := float32(90), float32(62)
			if formula {
				x = 240
				y = 33
			}
			h.point(pointer.Press, x, y)
			h.point(pointer.Release, x, y)
			h.point(pointer.Press, x, y)
			h.point(pointer.Release, x, y)
			if got := h.editor.SelectedText(); got != value {
				t.Fatalf("double click selected %q; want paragraph %q", got, value)
			}
		})
	}
}

func TestGridTextDoubleClickSelectsOnlyCurrentNewlineParagraph(t *testing.T) {
	h := newGridTextHarness(t, false, "alpha beta gamma\n中文后续段落")
	h.point(pointer.Press, 60, 33)
	h.point(pointer.Release, 60, 33)
	h.point(pointer.Press, 60, 33)
	h.point(pointer.Release, 60, 33)
	if got := h.editor.SelectedText(); got != "alpha beta gamma" {
		t.Fatalf("double click selected %q; want first paragraph only", got)
	}
	// A later double click in the second paragraph must replace the old selection.
	h.now += time.Second
	h.point(pointer.Press, 35, 50)
	h.point(pointer.Release, 35, 50)
	h.point(pointer.Press, 35, 50)
	h.point(pointer.Release, 35, 50)
	if got := h.editor.SelectedText(); got != "中文后续段落" {
		t.Fatalf("double click selected %q; want Chinese paragraph only", got)
	}
}

func TestGridTextDragSelectionDeleteAfterDoubleClickEntersEditing(t *testing.T) {
	for _, delay := range []time.Duration{0, time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			h := newFullGridTextHarness(t, false, "alpha 中文 beta gamma")
			h.app.cellEdit.Editing = false
			h.frame()
			h.point(pointer.Press, 90, 62)
			h.point(pointer.Release, 90, 62)
			h.point(pointer.Press, 90, 62)
			h.point(pointer.Release, 90, 62)
			if !h.app.cellEdit.Editing {
				t.Fatal("double click did not enter editing")
			}
			h.now += delay
			h.point(pointer.Press, 160, 62)
			h.point(pointer.Move, 60, 62)
			h.point(pointer.Release, 60, 62)
			selected := h.editor.SelectedText()
			if selected == "" {
				start, end := h.editor.Selection()
				t.Fatalf("drag did not select text after double click; caret=(%d,%d), selectionDragging=%v", start, end, h.app.selectionDragging)
			}
			start, end := h.editor.Selection()
			if start > end {
				start, end = end, start
			}
			r := []rune(h.editor.Text())
			want := string(r[:start]) + string(r[end:])
			h.router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
			h.frame()
			if got := h.editor.Text(); got != want {
				t.Fatalf("Delete after selecting %q = %q; want %q", selected, got, want)
			}
		})
	}
}
