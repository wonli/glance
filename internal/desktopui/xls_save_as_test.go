package desktopui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func newImportedXLSApp(t *testing.T) (*App, *desktopRuntime, string, []byte, *xlsx.Workbook) {
	t.Helper()
	path := legacyXLSFixture(t)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("load legacy XLS: %v", err)
	}
	book, ok := loaded.book.(*xlsx.Workbook)
	if !ok {
		closeLoadedDocument(loaded)
		t.Fatalf("loaded workbook is %T; want XLSX workbook", loaded.book)
	}
	desktop := newDesktopRuntime()
	app := newAppWithRuntime(nil, desktop)
	if err := app.installDocument(loaded); err != nil {
		closeLoadedDocument(loaded)
		close(app.shutdown)
		desktop.shutdown()
		t.Fatalf("install imported XLS: %v", err)
	}
	t.Cleanup(func() {
		close(app.shutdown)
		desktop.shutdown()
	})
	return app, desktop, path, original, book
}

func waitForSaveResult(t *testing.T, app *App) saveResult {
	t.Helper()
	select {
	case result := <-app.saveResults:
		app.saveResults <- result
		app.processAsyncResults()
		return result
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for Save As result")
		return saveResult{}
	}
}

func TestImportedXLSSaveStartsXLSXSaveAs(t *testing.T) {
	app, _, path, original, book := newImportedXLSApp(t)
	if err := book.SetCellText("总览", "A2", "保存前修改"); err != nil {
		t.Fatal(err)
	}
	type pickerRequest struct {
		kind   fileCommandKind
		format string
	}
	called := make(chan pickerRequest, 1)
	app.fileCommandRunner = func(kind fileCommandKind, format string) fileCommandResult {
		called <- pickerRequest{kind: kind, format: format}
		return fileCommandResult{}
	}
	app.startSave()
	select {
	case request := <-called:
		if request.kind != commandSaveAs || request.format != "xlsx" {
			t.Fatalf("picker request = %+v; want Save As XLSX", request)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("ordinary Save did not start the XLSX Save As picker")
	}
	select {
	case result := <-app.fileCommandResults:
		app.fileCommandResults <- result
		app.processAsyncResults()
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for canceled Save As picker")
	}
	if app.saving || app.path != path || !book.Dirty() || !app.documentSessionState.saveAsRequired {
		t.Fatalf("cancel changed session: saving=%v path=%q dirty=%v saveAsRequired=%v", app.saving, app.path, book.Dirty(), app.documentSessionState.saveAsRequired)
	}
	if app.status != "已取消另存为" {
		t.Fatalf("cancel status = %q", app.status)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("canceling Save As changed the original XLS bytes")
	}
}

func TestImportedXLSSaveAsRejectsXLSAndKeepsSource(t *testing.T) {
	app, _, path, original, book := newImportedXLSApp(t)
	if err := book.SetCellText("总览", "A2", "保留脏修改"); err != nil {
		t.Fatal(err)
	}
	app.startSaveAs(filepath.Join(t.TempDir(), "wrong-format.xls"))
	if !strings.Contains(app.status, ".xlsx") {
		t.Fatalf("rejected Save As status = %q; want .xlsx guidance", app.status)
	}
	if app.saving || app.path != path || !book.Dirty() || !app.documentSessionState.saveAsRequired {
		t.Fatalf("rejected Save As changed session: saving=%v path=%q dirty=%v saveAsRequired=%v", app.saving, app.path, book.Dirty(), app.documentSessionState.saveAsRequired)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("rejecting an XLS destination changed the original XLS bytes")
	}
}

func TestImportedXLSSaveAsAdoptsXLSXPathAndClearsRequirement(t *testing.T) {
	app, _, sourcePath, original, book := newImportedXLSApp(t)
	if err := book.SetCellText("总览", "A2", "另存后的编辑值"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "converted.xlsx")
	app.startSaveAs(output)
	result := waitForSaveResult(t, app)
	if result.err != nil {
		t.Fatalf("Save As: %v", result.err)
	}
	if app.path != filepath.Clean(output) || app.documentSessionState.path != filepath.Clean(output) || app.documentSessionState.sourceFormat != application.FormatXLSX || app.documentSessionState.saveAsRequired {
		t.Fatalf("post-Save-As app=%q sessionPath=%q want=%q source=%q saveAsRequired=%v", app.path, app.documentSessionState.path, filepath.Clean(output), app.documentSessionState.sourceFormat, app.documentSessionState.saveAsRequired)
	}
	if book.Dirty() {
		t.Fatal("successful Save As left the workbook dirty")
	}
	if _, err := book.Window("总览", 1, 1, 4, 6); err != nil {
		t.Fatalf("read current workbook after save: %v", err)
	}
	reopened, err := xlsx.Open(output)
	if err != nil {
		t.Fatalf("reopen saved XLSX: %v", err)
	}
	defer reopened.Close()
	window, err := reopened.Window("总览", 1, 1, 4, 6)
	if err != nil {
		t.Fatalf("read reopened XLSX: %v", err)
	}
	if got := window.Cells[1][0].Value; got != "另存后的编辑值" {
		t.Fatalf("A2 after reopen = %q", got)
	}
	if got := window.Cells[0][3]; got.Formula != "(B1*2)" || got.Value != "85.00" {
		t.Fatalf("untouched D1 after reopen = %+v; want formula and cached value", got)
	}
	if got := window.Cells[0][0]; got.StyleID == 0 || got.Style.FillColor == "" || got.Style.NumberFormat == "" {
		t.Fatalf("untouched styled A1 after reopen = %+v; want style preserved", got)
	}
	if got := window.Cells[3][5]; got.Value != "" || got.StyleID == 0 || got.Style.FillColor == "" {
		t.Fatalf("untouched styled blank F4 after reopen = %+v; want style preserved", got)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("Save As changed the original XLS bytes")
	}
}

func TestImportedXLSSaveAsFailureKeepsSourceAndDirtyState(t *testing.T) {
	app, _, sourcePath, original, book := newImportedXLSApp(t)
	if err := book.SetCellText("总览", "A2", "失败后仍保留"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "missing-directory", "converted.xlsx")
	app.startSaveAs(output)
	result := waitForSaveResult(t, app)
	if result.err == nil {
		t.Fatal("Save As unexpectedly succeeded in a missing directory")
	}
	if app.path != sourcePath || !book.Dirty() || !app.documentSessionState.saveAsRequired {
		t.Fatalf("failed Save As changed session: path=%q dirty=%v saveAsRequired=%v", app.path, book.Dirty(), app.documentSessionState.saveAsRequired)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("failed Save As changed the original XLS bytes")
	}
}

func TestImportedXLSGridReaderRemainsEditableAfterSaveAs(t *testing.T) {
	app, desktop, _, _, book := newImportedXLSApp(t)
	if _, ok := any(book).(document.GridReader); !ok {
		t.Fatal("imported workbook lost GridReader")
	}
	output := filepath.Join(t.TempDir(), "converted.xlsx")
	if _, err := desktop.applicationRuntime.SaveDocumentAs(context.Background(), app.documentID, output); err != nil {
		t.Fatalf("runtime Save As: %v", err)
	}
	if _, ok := any(app.book).(document.GridReader); !ok {
		t.Fatal("Save As replaced the active editor with a non-grid XLS reader")
	}
}
