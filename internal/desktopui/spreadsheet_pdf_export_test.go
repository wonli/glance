package desktopui

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/format/xlsx"
)

func TestSpreadsheetPDFExportToolbarActionEnabled(t *testing.T) {
	book := xlsx.NewBlank()
	defer book.Close()

	state := newDocumentSessionState("test.xlsx")
	state.book = book
	state.sheets = book.SheetNames()
	state.sheet = state.sheets[0]
	state.documentID = "sheet-test"

	app := &App{
		documentSessionState: state,
		activeTabID:          "sheet-test",
		fileCommandResults:   make(chan fileCommandResult, 1),
		shutdown:             make(chan struct{}),
	}

	if !app.toolbarActionEnabled(toolbarActionExport) {
		t.Fatal("toolbarActionExport should be enabled when workbook is loaded")
	}

	pickerCalls := make(chan fileCommandKind, 1)
	app.fileCommandRunner = func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls <- kind
		return fileCommandResult{kind: kind, path: ""}
	}

	app.requestSpreadsheetPDFExport()
	waitForSpreadsheetPDFPreview(t, app)
	if !app.modal.Visible || app.modal.Title != "导出 PDF 预览" {
		t.Fatalf("expected PDF preview before picker, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")

	select {
	case kind := <-pickerCalls:
		if kind != commandExportPDF {
			t.Fatalf("expected commandExportPDF, got %v", kind)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for PDF picker call")
	}

	// Process cancel result
	app.processAsyncResults()
	if app.status != "已取消导出 PDF" {
		t.Fatalf("expected status '已取消导出 PDF', got %q", app.status)
	}
}

func TestSpreadsheetPDFExportWritesValidatedPDFWithoutChangingWorkbook(t *testing.T) {
	book := xlsx.NewBlank()
	defer book.Close()

	sheet := book.SheetNames()[0]
	if err := book.SetCellText(sheet, "A1", "导出测试"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText(sheet, "B1", "123.45"); err != nil {
		t.Fatal(err)
	}
	book.SetDirty(true)

	before, err := book.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("test.xlsx")
	state.book = book
	state.sheets = book.SheetNames()
	state.sheet = sheet
	state.documentID = "sheet-export-test"

	outPath := filepath.Join(t.TempDir(), "export.pdf")

	app := &App{
		documentSessionState:        state,
		activeTabID:                 "sheet-export-test",
		spreadsheetPDFExportResults: make(chan spreadsheetPDFExportResult, 1),
		shutdown:                    make(chan struct{}),
	}

	app.startSpreadsheetPDFFileExport(state, sheet, outPath, "portrait")

	var result spreadsheetPDFExportResult
	select {
	case result = <-app.spreadsheetPDFExportResults:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for spreadsheet PDF export")
	}

	app.applySpreadsheetPDFExportResult(result)
	if result.err != nil {
		t.Fatalf("spreadsheet PDF export error: %v", result.err)
	}

	if state.spreadsheetPDFExportRunning {
		t.Fatal("export completion left state running")
	}
	if !book.Dirty() {
		t.Fatal("export changed workbook dirty flag")
	}

	after, err := book.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("export changed source workbook snapshot")
	}

	reader, err := gopdf.Open(outPath)
	if err != nil {
		t.Fatalf("open exported PDF: %v", err)
	}
	if reader.NumPages() < 1 {
		t.Fatalf("exported PDF pages = %d, want >= 1", reader.NumPages())
	}
	if app.status != "PDF 已导出：export.pdf" {
		t.Fatalf("unexpected export status: %q", app.status)
	}
}

func TestSpreadsheetPDFExportFlushesPendingCellDraft(t *testing.T) {
	book := xlsx.NewBlank()
	defer book.Close()

	sheet := book.SheetNames()[0]
	state := newDocumentSessionState("test.xlsx")
	state.book = book
	state.sheets = book.SheetNames()
	state.sheet = sheet
	state.documentID = "sheet-draft-test"

	app := &App{
		documentSessionState: state,
		activeTabID:          "sheet-draft-test",
		fileCommandResults:   make(chan fileCommandResult, 1),
		shutdown:             make(chan struct{}),
	}

	app.cellEdit.selectCell("A1", "old value")
	app.cellEdit.begin()
	app.cellEdit.setDraft("new draft value")

	pickerCalls := make(chan fileCommandKind, 1)
	app.fileCommandRunner = func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls <- kind
		return fileCommandResult{kind: kind, path: ""}
	}

	app.requestSpreadsheetPDFExport()
	waitForSpreadsheetPDFPreview(t, app)
	if !app.modal.Visible || app.modal.Title != "导出 PDF 预览" {
		t.Fatalf("expected PDF preview after draft flush, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")

	select {
	case kind := <-pickerCalls:
		if kind != commandExportPDF {
			t.Fatalf("expected commandExportPDF, got %v", kind)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected PDF picker to be called")
	}
	if app.cellEdit.Editing {
		t.Fatal("cellEdit should no longer be in editing state after flush")
	}
	val, err := book.Value(sheet, "A1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "new draft value" {
		t.Fatalf("cell A1 value = %q, want 'new draft value'", val)
	}
}

func waitForSpreadsheetPDFPreview(t *testing.T, app *App) {
	t.Helper()
	select {
	case result := <-app.spreadsheetPDFPreviewResults:
		app.applySpreadsheetPDFPreviewResult(result)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for spreadsheet PDF preview")
	}
}
