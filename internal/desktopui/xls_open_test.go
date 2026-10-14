package desktopui

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

type nonGridSpreadsheet struct{}

func (nonGridSpreadsheet) SheetNames() []string { return []string{"Sheet1"} }
func (nonGridSpreadsheet) Range(string, int, int) ([][]document.Cell, error) {
	return nil, nil
}
func (nonGridSpreadsheet) SetCellText(string, string, string) error { return nil }
func (nonGridSpreadsheet) Save() error                              { return nil }
func (nonGridSpreadsheet) Dirty() bool                              { return false }
func (nonGridSpreadsheet) Editable() bool                           { return true }
func (nonGridSpreadsheet) Close() error                             { return nil }

func legacyXLSFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "format", "xls", "testdata", "formula-style-multisheet.xls"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadLegacyXLSReturnsEditableGridReaderForAllSheets(t *testing.T) {
	path := legacyXLSFixture(t)
	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatalf("load legacy XLS: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })
	if _, ok := loaded.book.(document.GridReader); !ok {
		t.Fatalf("legacy XLS book %T does not implement GridReader", loaded.book)
	}
	if got := loaded.sheets; len(got) != 2 || got[0] != "总览" || got[1] != "明细" {
		t.Fatalf("sheets = %#v; want [总览 明细]", got)
	}
	if loaded.path != path || loaded.sourceFormat != application.FormatXLS || !loaded.saveAsRequired {
		t.Fatalf("source metadata = path %q format %q saveAsRequired %v", loaded.path, loaded.sourceFormat, loaded.saveAsRequired)
	}
	if err := loaded.book.Save(); err == nil {
		t.Fatal("imported XLS workbook unexpectedly has a save path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("original XLS should remain present: %v", err)
	}
}

func TestImportedXLSUsesVirtualGridAndTabsBelowHeaders(t *testing.T) {
	loaded, err := loadDocument(legacyXLSFixture(t))
	if err != nil {
		t.Fatalf("load legacy XLS: %v", err)
	}
	t.Cleanup(func() { closeLoadedDocument(loaded) })
	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() { app.ensureDesktopRuntime().shutdown(); close(app.shutdown) })
	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("install imported workbook: %v", err)
	}
	if app.virtualGrid == nil {
		t.Fatal("imported XLS did not initialize the virtual grid")
	}
	if _, ok := app.book.(document.GridReader); !ok {
		t.Fatalf("installed book %T does not use the virtual grid", app.book)
	}
	if app.documentSessionState.sourceFormat != application.FormatXLS || !app.documentSessionState.saveAsRequired {
		t.Fatalf("installed session lost XLS source metadata: format=%q saveAsRequired=%v", app.documentSessionState.sourceFormat, app.documentSessionState.saveAsRequired)
	}
	if app.sheet != "总览" || len(app.sheets) != 2 {
		t.Fatalf("installed sheet state = active %q, sheets %#v", app.sheet, app.sheets)
	}
	viewport := app.virtualGrid
	viewport.WindowSheet = app.sheet
	viewport.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 100, EndColumn: 16}
	viewport.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{{Value: "单元格"}}}}
	viewport.PendingSelectionLoad = false
	size := image.Pt(900, 700)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	if dims := app.layoutWorkbook(gtx); dims.Size != size {
		t.Fatalf("workbook layout size = %v; want %v", dims.Size, size)
	}
	win, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skip("headless window unavailable:", err)
	}
	defer win.Release()
	if err := win.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	screenshot := image.NewRGBA(image.Rectangle{Max: size})
	if err := win.Screenshot(screenshot); err != nil {
		t.Fatal(err)
	}
	minTabY := size.Y
	for y := 250; y < size.Y; y++ {
		for x := 8; x < 150; x++ {
			pixel := screenshot.RGBAAt(x, y)
			if pixel.R < 130 && pixel.G > 70 && pixel.G < 160 && pixel.B > 120 {
				if y < minTabY {
					minTabY = y
				}
			}
		}
	}
	if minTabY == size.Y {
		t.Fatal("selected imported-XLS sheet tab was not rendered in the lower grid strip")
	}
	if minTabY < 400 {
		t.Fatalf("sheet tab blue ink begins at y=%d; expected below the visible grid and column header", minTabY)
	}
}

func TestDesktopRuntimeOpensLegacyXLSAsEditableXLSXSession(t *testing.T) {
	ctx := context.Background()
	path := legacyXLSFixture(t)
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	shared, openedFromDisk, err := desktop.openLiveDocument(path)
	if err != nil {
		t.Fatalf("open legacy XLS through application runtime: %v", err)
	}
	if !openedFromDisk || shared.path != path || shared.sourceFormat != application.FormatXLS || !shared.saveAsRequired {
		t.Fatalf("shared session metadata: openedFromDisk=%v path=%q sourceFormat=%q saveAsRequired=%v", openedFromDisk, shared.path, shared.sourceFormat, shared.saveAsRequired)
	}
	if _, ok := shared.book.(document.GridReader); !ok {
		t.Fatalf("shared workbook %T is not a virtual GridReader", shared.book)
	}
	adapter, err := newDesktopSessionAdapter(desktop, shared)
	if err != nil {
		t.Fatalf("create desktop adapter: %v", err)
	}
	if adapter.Format() != application.FormatXLSX || adapter.SourceFormat() != application.FormatXLS || adapter.ReadOnly() {
		t.Fatalf("desktop adapter format=%q source=%q readOnly=%v", adapter.Format(), adapter.SourceFormat(), adapter.ReadOnly())
	}
	if err := adapter.Save(ctx); err == nil || !strings.Contains(err.Error(), "另存为 .xlsx") {
		t.Fatalf("Save error = %v; want instruction to Save As XLSX", err)
	}
	book, ok := shared.book.(*xlsx.Workbook)
	if !ok {
		t.Fatalf("imported workbook type = %T; want XLSX workbook", shared.book)
	}
	if err := book.SetCellText("总览", "A1", "已编辑"); err != nil {
		t.Fatalf("edit imported XLSX workbook: %v", err)
	}
	app := newAppWithRuntime(nil, desktop)
	t.Cleanup(func() { close(app.shutdown) })
	if err := app.installDocument(loadedDocumentFromSharedSession(shared)); err != nil {
		t.Fatalf("install imported session: %v", err)
	}
	output := filepath.Join(t.TempDir(), "edited.xlsx")
	if _, err := desktop.applicationRuntime.SaveDocumentAs(ctx, shared.documentID, output); err != nil {
		t.Fatalf("save imported workbook as XLSX: %v", err)
	}
	if shared.path != canonicalSessionPath(output) || shared.sourceFormat != application.FormatXLSX || shared.saveAsRequired {
		t.Fatalf("post-Save-As session path=%q source=%q saveAsRequired=%v", shared.path, shared.sourceFormat, shared.saveAsRequired)
	}
	if err := book.Save(); err != nil {
		t.Fatalf("save after successful Save As: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("XLSX output missing: %v", err)
	}
}

func TestLoadLegacyXLSRejectsBrokenConversionWithoutLeakingTempFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.xls")
	if err := os.WriteFile(path, []byte("not a BIFF workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDocument(path); err == nil {
		t.Fatal("loadDocument accepted malformed XLS")
	}
	if _, err := os.Stat(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect source after failed conversion: %v", err)
	}
}

func TestUnsupportedXLSOpenShowsActionableFeedback(t *testing.T) {
	path := filepath.Join("..", "format", "xls", "testdata", "unsupported-chart.xls")
	desktop := newDesktopRuntime()
	app := newAppWithRuntime(nil, desktop)
	t.Cleanup(func() {
		app.stopAndWaitManagedBackground()
		close(app.shutdown)
		desktop.shutdown()
	})

	app.startOpen(path)
	select {
	case result := <-app.openResults:
		app.openResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("opening an unsupported XLS did not finish in the background")
	}
	app.feedback.observe(app.status, time.Now())
	if !app.feedback.visible(time.Now()) {
		t.Fatalf("XLS import failure was not surfaced as visible feedback: %q", app.status)
	}
	if !strings.Contains(app.feedback.message, "无法安全导入") || strings.Contains(app.feedback.message, "WPS") {
		t.Fatalf("XLS import feedback should explain the safe-import limitation without redirecting to WPS: %q", app.feedback.message)
	}
}

func TestInstallDocumentRejectsNonGridSpreadsheetReader(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	book := nonGridSpreadsheet{}
	err := app.installDocument(loadedDocument{path: "legacy.xls", book: book, sheets: book.SheetNames()})
	if err == nil {
		t.Fatal("installDocument accepted a spreadsheet without GridReader")
	}
	if !strings.Contains(err.Error(), "网格读取") {
		t.Fatalf("non-grid rejection = %v; want a grid-reader diagnostic", err)
	}
}
