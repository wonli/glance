package desktopui

import (
	"image"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetAltEnterPreviewsMultilineRowAndEscapeRestoresIt(t *testing.T) {
	h, book := newGridRowAutofitInteractionHarness(t, "豆")
	a := h.app
	state := a.documentSessionState
	v := state.virtualGrid

	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModAlt})
	h.frame()

	if got := a.cellEditor.Text(); got != "豆\n" {
		t.Fatalf("Alt+Enter cell text = %q; want a trailing line break", got)
	}
	if state.cellEdit.Draft != "豆\n" || state.cellEdit.Original != "豆" {
		t.Fatalf("Alt+Enter draft/original = %q/%q; want %q/%q", state.cellEdit.Draft, state.cellEdit.Original, "豆\n", "豆")
	}
	if v.CellEditRowPreview == nil || v.CellEditRowPreview.PreviewPx <= v.CellEditRowPreview.OriginalPx {
		t.Fatalf("multiline row preview = %#v; want height greater than the original row", v.CellEditRowPreview)
	}
	if got := v.Rows.sizeAt(0); got <= 20 {
		t.Fatalf("row preview size = %vpx; want more than the 20px base", got)
	}

	h.router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	h.frame()
	if v.CellEditRowPreview != nil || v.Rows.sizeAt(0) != 20 {
		t.Fatalf("Escape left row preview %#v at %vpx; want preview cleared and row restored to 20px", v.CellEditRowPreview, v.Rows.sizeAt(0))
	}
	if got := a.cellEditor.Text(); got != "豆" {
		t.Fatalf("Escape editor text = %q; want original cell value", got)
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "豆" {
		t.Fatalf("Escape workbook value = %q, %v; want unchanged %q", got, err, "豆")
	}
}

func TestSpreadsheetMultilineEditHeightAndValueSurviveSaveAndReopen(t *testing.T) {
	h, book := newGridRowAutofitInteractionHarness(t, "标题")
	a := h.app
	state := a.documentSessionState
	v := state.virtualGrid
	value := "第一行内容\n第二行内容\n第三行内容\n第四行内容"
	a.cellEditor.SetText(value)
	a.syncSpreadsheetGridTextDraft()
	updateGridCellRowPreview(gridRowAutofitContext(h), a, v)
	if v.CellEditRowPreview == nil || v.CellEditRowPreview.PreviewPx <= v.CellEditRowPreview.OriginalPx {
		t.Fatalf("multiline row preview = %#v; want automatic expansion", v.CellEditRowPreview)
	}

	if err := a.commitGridCell(value); err != nil {
		t.Fatalf("commit multiline cell: %v", err)
	}
	if v.CellEditRowPreview != nil {
		t.Fatalf("row preview remained active after commit: %#v", v.CellEditRowPreview)
	}
	path := filepath.Join(t.TempDir(), "multiline.xlsx")
	if err := book.SaveAsDocument(path); err != nil {
		t.Fatalf("save multiline workbook: %v", err)
	}
	reopened, err := xlsx.Open(path)
	if err != nil {
		t.Fatalf("reopen multiline workbook: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got, err := reopened.Value(state.sheet, "A1"); err != nil || got != value {
		t.Fatalf("reopened cell value = %q, %v; want %q", got, err, value)
	}
	window, err := reopened.Window(state.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatalf("read reopened row height: %v", err)
	}
	if len(window.RowHeightsPoints) != 1 || window.RowHeightsPoints[0] <= 15 || window.RowHeightsPoints[0] > 409.5 {
		t.Fatalf("reopened row heights = %#v; want expanded height within XLSX limits", window.RowHeightsPoints)
	}
}

func TestSpreadsheetAutomaticallyExpandedRowShrinksWhenCellTextShortens(t *testing.T) {
	h, book := newGridRowAutofitInteractionHarness(t, "标题")
	a := h.app
	state := a.documentSessionState
	v := state.virtualGrid
	longValue := "第一行内容\n第二行内容\n第三行内容\n第四行内容"
	a.cellEditor.SetText(longValue)
	a.syncSpreadsheetGridTextDraft()
	updateGridCellRowPreview(gridRowAutofitContext(h), a, v)
	if err := a.commitGridCell(longValue); err != nil {
		t.Fatalf("commit long cell value: %v", err)
	}
	tall := v.Rows.sizeAt(0)
	if tall <= 20 {
		t.Fatalf("expanded row height = %vpx; want greater than 20px", tall)
	}
	key := gridAutoFitRow{Sheet: state.sheet, Row: 0}
	if _, ok := state.spreadsheetAutoFitRowBasePoints[key]; !ok {
		t.Fatal("automatic row height did not retain its base size for a later shrink")
	}
	window, err := book.Window(state.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatalf("reload A1 after first commit: %v", err)
	}
	v.Window, v.WindowSheet = window, state.sheet
	v.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1000, EndColumn: 1000}
	if err := v.applyWindowGeometry(window); err != nil {
		t.Fatalf("apply reloaded row dimensions: %v", err)
	}

	state.cellEdit.begin()
	a.cellEditor.SingleLine = false
	a.cellEditor.SetText("短")
	a.cellEdit.setDraft("短")
	updateGridCellRowPreview(gridRowAutofitContext(h), a, v)
	if v.CellEditRowPreview == nil || v.CellEditRowPreview.PreviewPx >= tall {
		t.Fatalf("short draft row preview = %#v; want a height below %vpx", v.CellEditRowPreview, tall)
	}
	if err := a.commitGridCell("短"); err != nil {
		t.Fatalf("commit shortened cell value: %v", err)
	}
	if got := v.Rows.sizeAt(0); got >= tall || got > 30 {
		t.Fatalf("row after shortening = %vpx; want it to shrink from %vpx to fit one line", got, tall)
	}
}

func newGridRowAutofitInteractionHarness(t *testing.T, initial string) (*gridTextHarness, *xlsx.Workbook) {
	t.Helper()
	h := newFullGridTextHarness(t, false, initial)
	a := h.app
	state := a.documentSessionState
	book := state.book.(*xlsx.Workbook)
	sheet := state.sheet
	a.sheet = sheet
	if err := book.SetCellText(sheet, "A1", initial); err != nil {
		t.Fatalf("seed A1: %v", err)
	}
	wrap := true
	if err := book.SetCellFormats(sheet, document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 1}, document.CellFormatPatch{WrapText: &wrap}); err != nil {
		t.Fatalf("wrap A1: %v", err)
	}
	window, err := book.Window(sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatalf("read A1: %v", err)
	}
	v := state.virtualGrid
	v.WindowSheet = sheet
	v.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1000, EndColumn: 1000}
	v.Window = window
	if err := v.Rows.SetSize(0, 20); err != nil {
		t.Fatalf("set test row height: %v", err)
	}
	v.cancelCellEditRowPreview()
	if err := v.Rows.SetSize(0, 20); err != nil {
		t.Fatalf("reset test row height after initial layout: %v", err)
	}
	state.cellEdit.selectCell("A1", initial)
	state.cellEdit.begin()
	a.cellEditor.SingleLine = false
	a.cellEditor.SetText(initial)
	a.cellEditor.SetCaret(len([]rune(initial)), len([]rune(initial)))
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	v.cancelCellEditRowPreview()
	if err := v.Rows.SetSize(0, 20); err != nil {
		t.Fatalf("restore default test row height: %v", err)
	}
	return h, book
}

func gridRowAutofitContext(h *gridTextHarness) layout.Context {
	return layout.Context{
		Ops:         &h.ops,
		Source:      h.router.Source(),
		Constraints: layout.Exact(image.Pt(400, 90)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
}
