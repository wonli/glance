package desktopui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/workspace"

	"github.com/xuri/excelize/v2"
)

func csvFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadCSVReturnsEditableGridReader(t *testing.T) {
	raw := "Name,Age,City\nAlice,30,Beijing\nBob,25,Shanghai\n"
	path := csvFixture(t, raw)

	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("loadDocument(csv): %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })

	reader, ok := loaded.book.(document.GridReader)
	if !ok {
		t.Fatalf("loaded.book %T does not implement GridReader", loaded.book)
	}
	if loaded.sourceFormat != application.FormatCSV {
		t.Fatalf("sourceFormat = %q; want %q", loaded.sourceFormat, application.FormatCSV)
	}
	if loaded.saveAsRequired {
		t.Fatal("saveAsRequired should be false for CSV documents")
	}
	if len(loaded.sheets) != 1 || loaded.sheets[0] != "Sheet1" {
		t.Fatalf("sheets = %#v; want [Sheet1]", loaded.sheets)
	}

	window, err := reader.Window("Sheet1", 1, 1, 3, 3)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if len(window.Cells) != 3 || len(window.Cells[0]) != 3 {
		t.Fatalf("expected 3x3 window, got %dx%d", len(window.Cells), len(window.Cells[0]))
	}
	if window.Cells[0][0].Value != "Name" || window.Cells[1][0].Value != "Alice" || window.Cells[2][1].Value != "25" {
		t.Fatalf("unexpected window values: %#v", window.Cells)
	}
}

func TestInstallCSVSharesSpreadsheetVirtualGrid(t *testing.T) {
	raw := "ID,Code,Note\n1,0123,Test\n"
	path := csvFixture(t, raw)

	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	if app.virtualGrid == nil {
		t.Fatal("CSV document did not initialize the virtual grid")
	}
	if _, ok := app.book.(document.GridReader); !ok {
		t.Fatalf("installed book %T does not implement GridReader", app.book)
	}
	if app.documentSessionState.sourceFormat != application.FormatCSV {
		t.Fatalf("installed session format = %q; want %q", app.documentSessionState.sourceFormat, application.FormatCSV)
	}
	if app.documentSessionState.saveAsRequired {
		t.Fatal("CSV session should not require SaveAs")
	}
	if !strings.Contains(app.status, "CSV 虚拟网格就绪") {
		t.Fatalf("unexpected app.status: %q", app.status)
	}
}

func TestCSVCellEditAndSaveRoundTrip(t *testing.T) {
	raw := "\xef\xbb\xbf姓名,年龄\n张三,28\n"
	path := csvFixture(t, raw)

	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	// Edit a cell
	if err := app.book.SetCellText(app.sheet, "B2", "35"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}
	if !app.isDirty() {
		t.Fatal("expected document to be dirty after cell edit")
	}

	// Save back to disk
	if err := app.save(); err != nil {
		t.Fatalf("app.save(): %v", err)
	}

	savedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	savedStr := string(savedBytes)

	// Verify UTF-8 BOM was preserved
	if !strings.HasPrefix(savedStr, "\xef\xbb\xbf") {
		t.Fatal("UTF-8 BOM was lost on save")
	}
	if !strings.Contains(savedStr, "35") {
		t.Fatalf("saved CSV does not contain edited value 35: %q", savedStr)
	}
	if strings.Contains(savedStr, "28") {
		t.Fatalf("saved CSV still contains old value 28: %q", savedStr)
	}
}

func TestCSVSaveAsXLSXAndCSV(t *testing.T) {
	raw := "ColA,ColB\nHello,World\n"
	path := csvFixture(t, raw)

	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	// Test SaveAs to XLSX
	xlsxTarget := filepath.Join(t.TempDir(), "export.xlsx")
	saver, ok := app.book.(interface{ SaveAsDocument(string) error })
	if !ok {
		t.Fatalf("app.book %T does not implement SaveAsDocument", app.book)
	}
	if err := saver.SaveAsDocument(xlsxTarget); err != nil {
		t.Fatalf("SaveAsDocument(xlsx): %v", err)
	}

	// Verify excelize can open the exported XLSX
	f, err := excelize.OpenFile(xlsxTarget)
	if err != nil {
		t.Fatalf("excelize.OpenFile: %v", err)
	}
	defer f.Close()
	val, err := f.GetCellValue("Sheet1", "B2")
	if err != nil || val != "World" {
		t.Fatalf("exported XLSX B2 = %q, err = %v; want 'World'", val, err)
	}

	// Test SaveAs to another CSV
	csvTarget := filepath.Join(t.TempDir(), "export.csv")
	if err := saver.SaveAsDocument(csvTarget); err != nil {
		t.Fatalf("SaveAsDocument(csv): %v", err)
	}
	csvContent, err := os.ReadFile(csvTarget)
	if err != nil {
		t.Fatalf("read export.csv: %v", err)
	}
	if !strings.Contains(string(csvContent), "Hello,World") {
		t.Fatalf("exported CSV content mismatch: %q", string(csvContent))
	}
}

func TestCSVSnapshotAndWorkspaceRestore(t *testing.T) {
	raw := "X,Y\n10,20\n"
	path := csvFixture(t, raw)

	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	snapBytes, err := snapshotState(app.documentSessionState)
	if err != nil {
		t.Fatalf("snapshotState: %v", err)
	}
	if len(snapBytes) == 0 {
		t.Fatal("empty snapshot bytes")
	}

	// Restore session from tab snapshot
	session := workspace.Session{
		Tab: workspace.Tab{
			ID:         "csv-tab-1",
			Kind:       "document",
			SourcePath: path,
			Title:      "test.csv",
		},
		Format:  "csv",
		Payload: snapBytes,
		Dirty:   false,
	}
	restored, err := app.restoreSessionState(session)
	if err != nil {
		t.Fatalf("restoreSessionState: %v", err)
	}
	if restored == nil || restored.book == nil {
		t.Fatal("restored document or book is nil")
	}
	if restored.sourceFormat != application.FormatCSV {
		t.Fatalf("restored sourceFormat = %q; want %q", restored.sourceFormat, application.FormatCSV)
	}

	reader, ok := restored.book.(document.GridReader)
	if !ok {
		t.Fatalf("restored book %T does not implement GridReader", restored.book)
	}
	window, err := reader.Window("Sheet1", 1, 1, 2, 2)
	if err != nil {
		t.Fatalf("Window on restored book: %v", err)
	}
	if window.Cells[1][1].Value != "20" {
		t.Fatalf("restored cell [2,2] = %q; want '20'", window.Cells[1][1].Value)
	}
}

func TestCSVApplicationRuntimeWorkflow(t *testing.T) {
	ctx := context.Background()
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)

	path := csvFixture(t, "Item,Qty\nApple,5\nBanana,10\n")
	handle, err := runtime.applicationRuntime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	if handle.Format != application.FormatCSV {
		t.Fatalf("handle.Format = %q; want %q", handle.Format, application.FormatCSV)
	}

	// Set cell B2 to 15
	args, _ := json.Marshal(map[string]string{"sheet": "Sheet1", "cell": "B2", "value": "15"})
	res, err := runtime.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "req-1", DocumentID: handle.DocumentID, CommandID: "spreadsheet.cell.set",
		ExpectedRevision: handle.Revision, Operator: application.Operator{ID: "test", Kind: application.OperatorHuman},
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("spreadsheet.cell.set: %v", err)
	}

	// Save
	_, _, err = runtime.applicationRuntime.SaveDocumentWithSnapshot(ctx, handle.DocumentID)
	if err != nil {
		t.Fatalf("SaveDocumentWithSnapshot: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !strings.Contains(string(saved), "Apple,15") {
		t.Fatalf("saved content mismatch: %q", string(saved))
	}

	// Undo back to 5
	undoArgs, _ := json.Marshal(map[string]string{"action": "undo"})
	_, err = runtime.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "req-2", DocumentID: handle.DocumentID, CommandID: "spreadsheet.history.apply",
		ExpectedRevision: res.Revision, Operator: application.Operator{ID: "test", Kind: application.OperatorHuman},
		Arguments: undoArgs,
	})
	if err != nil {
		t.Fatalf("spreadsheet.history.apply undo: %v", err)
	}

	// Save after undo
	_, _, err = runtime.applicationRuntime.SaveDocumentWithSnapshot(ctx, handle.DocumentID)
	if err != nil {
		t.Fatalf("SaveDocumentWithSnapshot after undo: %v", err)
	}

	savedAfterUndo, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file after undo: %v", err)
	}
	if !strings.Contains(string(savedAfterUndo), "Apple,5") {
		t.Fatalf("saved content after undo mismatch: %q", string(savedAfterUndo))
	}
}

func TestOpenRealCustomerCSVInApp(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})
	dbPath, err := workspace.DefaultDatabasePath()
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	app.collection = col
	for _, tab := range col.Tabs() {
		app.tabMeta[tab.Tab.ID] = tab.Tab
		if strings.Contains(tab.Tab.Title, "customers") {
			t.Logf("Found tab: %+v", tab)
			app.activateWorkspaceTab(tab.Tab.ID)
			t.Logf("Status after activate: %q, activeTabID: %q, book: %v", app.status, app.activeTabID, app.book != nil)
			if app.book == nil {
				select {
				case res := <-app.openResults:
					t.Logf("openResult: err=%v, loaded.sheets=%v", res.err, res.loaded.sheets)
					if res.err == nil {
						if err := app.installDocument(res.loaded); err != nil {
							t.Fatalf("installDocument err=%v", err)
						}
					}
				case <-time.After(5 * time.Second):
					t.Fatal("timeout waiting for openResults")
				}
			}
			if app.book == nil {
				t.Fatal("app.book is nil after activating tab")
			}
			if app.virtualGrid == nil {
				t.Fatal("app.virtualGrid is nil after activating tab")
			}
			if app.virtualGrid.Rows.Count() != 2000001 {
				t.Fatalf("virtualGrid.Rows.Count() = %d; want 2000001", app.virtualGrid.Rows.Count())
			}
			t.Logf("Success! activeTabID=%s, virtualGrid rows=%d, cols=%d", app.activeTabID, app.virtualGrid.Rows.Count(), app.virtualGrid.Columns.Count())
		}
	}
}

func TestCSV2MillionVirtualGridNavigationAndEditing(t *testing.T) {
	path := "/Users/mima1234/Downloads/customers-2000000.csv"
	if _, err := os.Stat(path); err != nil {
		t.Skip("customers-2000000.csv not found")
	}

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})

	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	defer closeLoadedDocument(loaded)

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	if app.virtualGrid.Rows.Count() != 2000001 {
		t.Fatalf("virtualGrid.Rows.Count() = %d; want 2000001", app.virtualGrid.Rows.Count())
	}

	// 1. Navigate to row 2,000,001 column 2
	app.navigateToCell(2000001, 2)
	wantCell := "B2000001"
	if app.selectedCell != wantCell {
		t.Fatalf("selectedCell = %q; want %q", app.selectedCell, wantCell)
	}
	if app.virtualGrid.SelectedRow != 2000001 || app.virtualGrid.SelectedColumn != 2 {
		t.Fatalf("grid selection = (%d, %d); want (2000001, 2)", app.virtualGrid.SelectedRow, app.virtualGrid.SelectedColumn)
	}

	// 2. Read cell value
	gridReader := app.book.(document.GridReader)
	w, err := gridReader.Window("Sheet1", 2000001, 2, 1, 1)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	origVal := w.Cells[0][0].Value

	// 3. Edit cell at row 2,000,001
	if err := app.book.SetCellText("Sheet1", wantCell, "MODIFIED_2M_VAL"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}

	wAfter, err := gridReader.Window("Sheet1", 2000001, 2, 1, 1)
	if err != nil {
		t.Fatalf("Window after edit: %v", err)
	}
	if wAfter.Cells[0][0].Value != "MODIFIED_2M_VAL" {
		t.Fatalf("got %q, want MODIFIED_2M_VAL", wAfter.Cells[0][0].Value)
	}

	// 4. Undo
	history := app.book.(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo: %v, undone=%v", err, undone)
	}
	wUndone, err := gridReader.Window("Sheet1", 2000001, 2, 1, 1)
	if err != nil {
		t.Fatalf("Window after undo: %v", err)
	}
	if wUndone.Cells[0][0].Value != origVal {
		t.Fatalf("got %q, want %q", wUndone.Cells[0][0].Value, origVal)
	}

	// 5. Navigate to row 1,048,576
	app.navigateToCell(1048576, 1)
	if app.selectedCell != "A1048576" {
		t.Fatalf("selectedCell = %q; want A1048576", app.selectedCell)
	}

	// 6. Test compact snapshot
	snap, err := app.snapshotActive()
	if err != nil {
		t.Fatalf("snapshotActive: %v", err)
	}
	if len(snap) > 1024*1024 {
		t.Fatalf("snapshot payload size %d is too large for fast workspace persistence", len(snap))
	}
	t.Logf("Snapshot active size: %d bytes", len(snap))
}

