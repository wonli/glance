package desktopui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/format/xlsx"
)

func TestSpreadsheetShareActionOpensXLSXCopyPickerAndCancelPreservesSession(t *testing.T) {
	book := xlsx.NewBlank()
	defer book.Close()
	book.SetDirty(true)
	state := newDocumentSessionState("source.xlsx")
	state.book = book
	state.sheets = []string{"Sheet1"}
	state.sheet = "Sheet1"
	app := &App{
		documentSessionState: state,
		activeTabID:          "tab-current",
		fileCommandResults:   make(chan fileCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
	pickerCalls := make(chan struct {
		kind   fileCommandKind
		format string
	}, 1)
	app.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls <- struct {
			kind   fileCommandKind
			format string
		}{kind: kind, format: format}
		return fileCommandResult{kind: kind}
	})

	if !app.toolbarActionEnabled(toolbarActionShare) {
		t.Fatal("Spreadsheet File > Share should be enabled for an open workbook")
	}
	app.requestSpreadsheetShareCopy()
	select {
	case call := <-pickerCalls:
		if call.kind != commandShareCopy || call.format != "xlsx" {
			t.Fatalf("share picker request = %#v; want XLSX copy picker", call)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Spreadsheet File > Share did not open the XLSX copy picker")
	}
	select {
	case result := <-app.fileCommandResults:
		app.fileCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("share picker cancellation was not delivered")
	}

	if app.status != "已取消共享副本导出" {
		t.Fatalf("share cancellation status = %q", app.status)
	}
	if app.book != book || app.path != "source.xlsx" || state.path != "source.xlsx" || !book.Dirty() {
		t.Fatalf("cancel changed source session: active=%p path=%q statePath=%q dirty=%v", app.book, app.path, state.path, book.Dirty())
	}
}

func TestSpreadsheetShareExportsAtomicXLSXCopyWithoutChangingSourceSession(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.xlsx")
	destinationPath := filepath.Join(t.TempDir(), "shared-copy.xlsx")
	book := xlsx.NewBlank()
	defer book.Close()
	if err := book.SetCellText("Sheet1", "A1", "原始版本"); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAsDocument(sourcePath); err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText("Sheet1", "A1", "共享版本 updated"); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState(sourcePath)
	state.book = book
	state.sheets = []string{"Sheet1"}
	state.sheet = "Sheet1"
	app := &App{
		documentSessionState:        state,
		activeTabID:                 "tab-current",
		spreadsheetShareCopyResults: make(chan spreadsheetShareCopyResult, 1),
		shutdown:                    make(chan struct{}),
	}
	app.spreadsheetShareCopyRequest = &spreadsheetShareCopyPickerRequest{state: state, historyState: book.HistoryState()}
	app.startSpreadsheetShareCopyFile(app.spreadsheetShareCopyRequest, destinationPath)

	select {
	case result := <-app.spreadsheetShareCopyResults:
		app.applySpreadsheetShareCopyResult(result)
	case <-time.After(3 * time.Second):
		t.Fatal("export timed out")
	}

	if state.path != sourcePath || !book.Dirty() {
		t.Fatalf("export changed active source session: path=%q dirty=%v", state.path, book.Dirty())
	}
	if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, sourceBefore) {
		t.Fatalf("source XLSX changed: read error=%v equal=%v", err, bytes.Equal(got, sourceBefore))
	}

	copyBook, err := xlsx.Open(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	defer copyBook.Close()
	if val, err := copyBook.Value("Sheet1", "A1"); err != nil || val != "共享版本 updated" {
		t.Fatalf("shared copy cell A1 = %q, %v", val, err)
	}
}

func TestSpreadsheetShareRejectsSourceOverwriteAndStalePickerChoice(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.xlsx")
	book := xlsx.NewBlank()
	defer book.Close()
	if err := book.SaveAsDocument(sourcePath); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState(sourcePath)
	state.book = book
	app := &App{
		documentSessionState:        state,
		activeTabID:                 "tab-current",
		spreadsheetShareCopyResults: make(chan spreadsheetShareCopyResult, 1),
		shutdown:                    make(chan struct{}),
	}

	// Reject overwriting source
	app.startSpreadsheetShareCopyFile(&spreadsheetShareCopyPickerRequest{state: state, historyState: book.HistoryState()}, sourcePath)
	if app.status != "共享副本不能覆盖当前 XLSX" {
		t.Fatalf("source overwrite status = %q", app.status)
	}

	// Reject stale history state
	stalePath := filepath.Join(t.TempDir(), "stale.xlsx")
	app.startSpreadsheetShareCopyFile(&spreadsheetShareCopyPickerRequest{state: state, historyState: book.HistoryState() + 99}, stalePath)
	if app.status != "工作簿在选择位置期间已更改，未导出共享副本；请重试" {
		t.Fatalf("stale history state status = %q", app.status)
	}
}
