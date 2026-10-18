package desktopui

import (
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetCellRichTextSelectionFormatting(t *testing.T) {
	wb := xlsx.NewBlank()
	sheet := wb.SheetNames()[0]
	cellRef := "A1"
	text := "你好世界测试文字"
	if err := wb.SetCellText(sheet, cellRef, text); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheet = sheet
	state.selectedCell = cellRef
	state.selectionSet = true

	app := New(nil, "")
	app.documentSessionState = state

	// 1. When not in editing mode, toolbarActionEnabled(4) is enabled for the whole cell.
	if !app.toolbarActionEnabled(4) {
		t.Fatal("slot 4 (bold) should be enabled when whole cell is selected")
	}

	// 2. Start editing cell A1
	state.cellEdit.selectCell(cellRef, text)
	state.cellEdit.begin()
	app.cellEditor.SetText(text)

	// In edit mode with NO text selection, partial format toolbar actions should be disabled.
	app.cellEditor.SetCaret(0, 0)
	if app.toolbarActionEnabled(4) || app.toolbarActionEnabled(5) || app.toolbarActionEnabled(6) {
		t.Fatal("slots 4, 5, 6 should be disabled in edit mode when no text is selected")
	}

	// 3. Select "世界" (rune offset 2 to 4)
	app.cellEditor.SetCaret(2, 4)
	if !app.toolbarActionEnabled(4) || !app.toolbarActionEnabled(5) || !app.toolbarActionEnabled(6) {
		t.Fatal("slots 4, 5, 6 should be enabled in edit mode when text is selected")
	}

	// 4. Apply bold via applySpreadsheetFormatAction(4)
	app.applySpreadsheetFormatAction(4)

	runs, err := wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 rich text runs after bold, got %d: %#v", len(runs), runs)
	}
	if runs[0].Text != "你好" || runs[0].Bold {
		t.Errorf("run 0 = %#v; want '你好' (not bold)", runs[0])
	}
	if runs[1].Text != "世界" || !runs[1].Bold {
		t.Errorf("run 1 = %#v; want '世界' (bold)", runs[1])
	}
	if runs[2].Text != "测试文字" || runs[2].Bold {
		t.Errorf("run 2 = %#v; want '测试文字' (not bold)", runs[2])
	}

	// 5. Select "测试" (rune offset 4 to 6) and apply underline (slot 6)
	app.cellEditor.SetCaret(4, 6)
	app.applySpreadsheetFormatAction(6)

	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	// Expected: "你好" (normal), "世界" (bold), "测试" (underline), "文字" (normal)
	if len(runs) != 4 {
		t.Fatalf("expected 4 rich text runs after underline, got %d: %#v", len(runs), runs)
	}
	if runs[1].Text != "世界" || !runs[1].Bold || runs[1].Underline {
		t.Errorf("run 1 = %#v; want '世界' (bold, not underline)", runs[1])
	}
	if runs[2].Text != "测试" || runs[2].Bold || !runs[2].Underline {
		t.Errorf("run 2 = %#v; want '测试' (not bold, underline)", runs[2])
	}

	// 6. Select "世界" again and toggle bold off
	app.cellEditor.SetCaret(2, 4)
	app.applySpreadsheetFormatAction(4)

	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	// "你好" (normal) and "世界" (now unbolded) should merge into "你好世界"
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs after unbolding, got %d: %#v", len(runs), runs)
	}
	if runs[0].Text != "你好世界" || runs[0].Bold {
		t.Errorf("run 0 = %#v; want '你好世界' unbolded", runs[0])
	}

	// 7. Select "你好" (offset 0 to 2) and apply italic (slot 5)
	app.cellEditor.SetCaret(0, 2)
	app.applySpreadsheetFormatAction(5)

	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) < 2 || runs[0].Text != "你好" || !runs[0].Italic {
		t.Fatalf("expected run 0 to be '你好' (italic), got %#v", runs)
	}
}

func TestSpreadsheetCellRichTextShortcuts(t *testing.T) {
	wb := xlsx.NewBlank()
	sheet := wb.SheetNames()[0]
	cellRef := "B2"
	text := "快捷键测试内容"
	if err := wb.SetCellText(sheet, cellRef, text); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("shortcut-test.xlsx")
	state.book = wb
	state.sheet = sheet
	state.selectedCell = cellRef
	state.selectionSet = true

	app := New(nil, "")
	app.documentSessionState = state

	state.cellEdit.selectCell(cellRef, text)
	state.cellEdit.begin()
	state.cellEditor.SetText(text)

	// Select "测试" (rune offset 3 to 5)
	state.cellEditor.SetCaret(3, 5)

	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "B", key.ModCtrl)

	runs, err := wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs with Ctrl+B, got %d: %#v", len(runs), runs)
	}
	if runs[1].Text != "测试" || !runs[1].Bold {
		t.Fatalf("expected '测试' to be bold with Ctrl+B, got %#v", runs[1])
	}

	// Also test ModShortcut (Cmd on macOS) to toggle bold off
	state.cellEditor.SetCaret(3, 5)
	pressOfficeShortcut(t, h, "B", key.ModShortcut)
	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Bold {
		t.Fatalf("expected 1 unbolded run after toggling bold off with Cmd+B, got %#v", runs)
	}

	// Test italic with Ctrl+I
	state.cellEditor.SetCaret(0, 3) // "快捷键"
	pressOfficeShortcut(t, h, "I", key.ModCtrl)
	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) < 2 || runs[0].Text != "快捷键" || !runs[0].Italic {
		t.Fatalf("expected '快捷键' to be italic with Ctrl+I, got %#v", runs)
	}

	// Test underline with Ctrl+U
	state.cellEditor.SetCaret(5, 7) // "内容"
	pressOfficeShortcut(t, h, "U", key.ModCtrl)
	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	lastRun := runs[len(runs)-1]
	if lastRun.Text != "内容" || !lastRun.Underline {
		t.Fatalf("expected '内容' to be underline with Ctrl+U, got %#v", lastRun)
	}
}

func TestSpreadsheetFormulaBarRichTextFormatting(t *testing.T) {
	wb := xlsx.NewBlank()
	sheet := wb.SheetNames()[0]
	cellRef := "A1"
	text := "公式栏富文本测试"
	if err := wb.SetCellText(sheet, cellRef, text); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("formula-bar-test.xlsx")
	state.book = wb
	state.sheet = sheet
	state.selectedCell = cellRef
	state.selectionSet = true

	app := New(nil, "")
	app.documentSessionState = state

	// In-cell edit is NOT active (user is editing via formula bar)
	state.cellEdit.setEditing(false)
	app.formulaEditor.SetText(text)

	// 1. Without selection, text selection formatting should not be active
	app.formulaEditor.SetCaret(0, 0)
	if app.spreadsheetCellTextSelectionActive() {
		t.Fatal("selection should not be active when caret is at 0,0")
	}

	// 2. Select "富文本" (runes 3 to 6) in formula bar
	app.formulaEditor.SetCaret(3, 6)
	if !app.spreadsheetCellTextSelectionActive() {
		t.Fatal("spreadsheetCellTextSelectionActive should be true when formula bar has text selected")
	}
	if !app.toolbarActionEnabled(4) {
		t.Fatal("toolbar slot 4 (bold) should be enabled for formula bar text selection")
	}

	// 3. Apply bold via applySpreadsheetFormatAction (Ribbon button click)
	app.applySpreadsheetFormatAction(4)

	runs, err := wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs after bolding in formula bar, got %d: %#v", len(runs), runs)
	}
	if runs[1].Text != "富文本" || !runs[1].Bold {
		t.Fatalf("expected '富文本' to be bold, got %#v", runs[1])
	}

	// 4. Test shortcut (Ctrl+I) while editing in formula bar
	app.formulaEditor.SetCaret(0, 3) // "公式栏"
	h := newOfficeShortcutHarness(app)
	pressOfficeShortcut(t, h, "I", key.ModCtrl)

	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) < 2 || runs[0].Text != "公式栏" || !runs[0].Italic {
		t.Fatalf("expected '公式栏' to be italic with Ctrl+I from formula bar, got %#v", runs)
	}
}

func TestSpreadsheetCellRichTextPreservesCellStyleAndViewport(t *testing.T) {
	wb := xlsx.NewBlank()
	sheet := wb.SheetNames()[0]
	cellRef := "C3"
	text := "居中对齐蓝色大号文字"
	if err := wb.SetCellText(sheet, cellRef, text); err != nil {
		t.Fatal(err)
	}

	size := 16.0
	color := "#0000FF"
	align := "center"
	if err := wb.SetCellFormats(sheet, document.CellRange{StartRow: 3, StartColumn: 3, EndRow: 3, EndColumn: 3}, document.CellFormatPatch{
		FontSize:            &size,
		FontColor:           &color,
		HorizontalAlignment: &align,
	}); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("style-preserve.xlsx")
	state.book = wb
	state.sheet = sheet
	state.selectedCell = cellRef
	state.selectionSet = true

	grid := &gridViewport{
		WindowSheet: sheet,
		WindowRange: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 5, EndColumn: 5},
	}
	gridWindow, err := wb.Window(sheet, 1, 1, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	grid.Window = gridWindow
	state.virtualGrid = grid

	app := New(nil, "")
	app.documentSessionState = state

	// Begin cell edit
	state.cellEdit.selectCell(cellRef, text)
	state.cellEdit.begin()
	state.cellEditor.SetText(text)

	// Select "蓝色" (runes 4 to 6)
	state.cellEditor.SetCaret(4, 6)

	// Format selection with bold
	app.applySpreadsheetCellTextFormat(4)

	// Check that viewport WindowRange was NOT wiped
	if state.virtualGrid.WindowRange.StartRow == 0 {
		t.Fatal("virtualGrid.WindowRange should not be wiped when formatting cell rich text")
	}

	// Check that in-memory cell style is preserved
	cell := state.virtualGrid.cell(3, 3)
	if cell.Style.FontSize != 16.0 {
		t.Fatalf("expected in-memory cell font size 16, got %v", cell.Style.FontSize)
	}
	if cell.Style.FontColor != "#0000FF" {
		t.Fatalf("expected in-memory cell font color #0000FF, got %v", cell.Style.FontColor)
	}
	if len(cell.RichText) != 3 {
		t.Fatalf("expected 3 rich text runs in viewport cell, got %d: %#v", len(cell.RichText), cell.RichText)
	}
	if cell.RichText[1].Text != "蓝色" || !cell.RichText[1].Bold {
		t.Fatalf("expected '蓝色' to be bold, got %#v", cell.RichText[1])
	}
	if cell.RichText[1].FontSize != 16.0 {
		t.Fatalf("expected '蓝色' to keep font size 16, got %v", cell.RichText[1].FontSize)
	}
	if cell.RichText[1].FontColor != "#0000FF" {
		t.Fatalf("expected '蓝色' to keep color #0000FF, got %v", cell.RichText[1].FontColor)
	}
}

func TestSpreadsheetCellRichTextRibbonEvents(t *testing.T) {
	wb := xlsx.NewBlank()
	sheet := wb.SheetNames()[0]
	cellRef := "A1"
	text := "工具栏按钮点击测试"
	if err := wb.SetCellText(sheet, cellRef, text); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("toolbar-click-test.xlsx")
	state.book = wb
	state.sheet = sheet
	state.selectedCell = cellRef
	state.selectionSet = true

	grid := &gridViewport{
		WindowSheet: sheet,
		WindowRange: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 5, EndColumn: 5},
	}
	gridWindow, err := wb.Window(sheet, 1, 1, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	grid.Window = gridWindow
	state.virtualGrid = grid

	app := New(nil, "")
	app.documentSessionState = state

	// Begin edit and select "按钮点击" (runes 3 to 7)
	state.cellEdit.selectCell(cellRef, text)
	state.cellEdit.begin()
	app.cellEditor.SetText(text)
	app.cellEditor.SetCaret(3, 7)

	// Simulate ribbon button 4 (bold) click
	state.toolbarActionButtons[4].Click()
	app.handleSpreadsheetToolbarEvents(layout.Context{})

	runs, err := wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs after ribbon bold click, got %d: %#v", len(runs), runs)
	}
	if runs[1].Text != "按钮点击" || !runs[1].Bold {
		t.Fatalf("expected '按钮点击' to be bold, got %#v", runs[1])
	}

	// Commit text entry (e.g. user presses Enter)
	app.commitSpreadsheetGridTextEntry(layout.Context{}, state.virtualGrid, key.NameDownArrow)

	// Verify cell rich text in workbook survived commit
	runs, err = wb.GetCellRichText(sheet, cellRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs after commit, got %d: %#v", len(runs), runs)
	}
	if runs[1].Text != "按钮点击" || !runs[1].Bold {
		t.Fatalf("expected '按钮点击' to still be bold after commit, got %#v", runs[1])
	}

	// Verify in-memory virtual grid cell survived commit
	vCell := state.virtualGrid.cell(1, 1)
	if len(vCell.RichText) != 3 || !vCell.RichText[1].Bold {
		t.Fatalf("expected virtual grid cell to preserve rich text after commit, got %#v", vCell.RichText)
	}
}

func TestSpreadsheetRichSelectionToolbarState(t *testing.T) {
	wb := xlsx.NewBlank()
	sheet := wb.SheetNames()[0]
	if err := wb.SetCellText(sheet, "A1", "中文测试"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("test.xlsx")
	state.book, state.sheet, state.selectedCell, state.selectionSet = wb, sheet, "A1", true
	state.cellEdit.selectCell("A1", "中文测试")
	state.cellEdit.begin()
	state.cellEditor.SetText("中文测试")
	state.cellEditor.SetCaret(1, 3)
	app := New(nil, "")
	app.documentSessionState = state
	for _, slot := range []int{4, 5, 6} {
		app.applySpreadsheetFormatAction(slot)
		style, mixed, ok := app.spreadsheetTextSelectionStyle(slot)
		if !ok || mixed || !formatStyleActive(slot, style) {
			t.Fatalf("slot %d selection state = %+v mixed=%v ok=%v", slot, style, mixed, ok)
		}
	}
	state.cellEditor.SetCaret(0, 4)
	_, mixed, _ := app.spreadsheetTextSelectionStyle(4)
	if !mixed {
		t.Fatal("mixed selection must be reported")
	}
}
