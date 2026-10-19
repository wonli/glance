package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/xuri/excelize/v2"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
)

func TestSpreadsheetStatusBarLoadsSelectionStatsAsynchronously(t *testing.T) {
	app := New(nil, "")
	file := excelize.NewFile()
	for cell, value := range map[string]any{"A1": 2, "B1": 4, "A2": "text"} {
		if err := file.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	book := xlsx.NewFromExcelize(file, "")
	state := newDocumentSessionState("summary.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{Row: 1, Column: 1}}
	state.selectionSet = true
	app.documentSessionState = state
	app.documentGeneration = 7
	app.sheet = "Sheet1"
	app.statusRef = i18n.Message{ID: "desktop.status.inline.ready_b796f2"}
	defer book.Close()
	defer func() {
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		app.stopAndWaitManagedBackground()
	}()

	for attempt := 0; attempt < 100; attempt++ {
		var ops op.Ops
		gtx := layout.Context{
			Ops:         &ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(900, 50)),
		}
		app.layoutSpreadsheetStatusBar(gtx)
		app.processAsyncResults()
		if state.spreadsheetSelectionStatsSet {
			got := state.spreadsheetSelectionStats
			if got.Count != 4 || got.NumericCount != 2 || got.Sum != 6 {
				t.Fatalf("selection stats = %+v; want Count=4 NumericCount=2 Sum=6", got)
			}
			if app.statusText() != "就绪" {
				t.Fatalf("selection summary replaced the document status with %q", app.statusText())
			}
			if gotText, wantText := app.spreadsheetStatusSelectionStatsText(), "平均值 3  ·  计数 4  ·  求和 6"; gotText != wantText {
				t.Fatalf("selection summary text = %q; want %q", gotText, wantText)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("status bar did not load stats for a multi-cell selection")
}

func TestSpreadsheetStatusBarShowsCountForBlankMultiCellSelection(t *testing.T) {
	app := New(nil, "")
	book := xlsx.NewBlank()
	state := newDocumentSessionState("blank-selection.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{Row: 1, Column: 2}}
	state.selectionSet = true
	app.documentSessionState = state
	app.sheet = "Sheet1"
	defer book.Close()
	defer func() {
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		app.stopAndWaitManagedBackground()
	}()

	for attempt := 0; attempt < 100; attempt++ {
		_ = app.spreadsheetStatusSelectionStatsText()
		app.processSpreadsheetSelectionStatsResults()
		if state.spreadsheetSelectionStatsSet {
			if got := app.spreadsheetStatusSelectionStatsText(); got != "计数 6" {
				t.Fatalf("blank selection summary = %q; want %q", got, "计数 6")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("status bar did not load stats for a blank multi-cell selection")
}

func TestSpreadsheetStatusBarHidesStatsForSingleCellSelection(t *testing.T) {
	app := New(nil, "")
	book := xlsx.NewBlank()
	state := newDocumentSessionState("single.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
	state.selectionSet = true
	app.documentSessionState = state
	defer book.Close()
	defer func() {
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		app.stopAndWaitManagedBackground()
	}()

	if got := app.spreadsheetStatusSelectionStatsText(); got != "" {
		t.Fatalf("single-cell selection summary = %q; want empty", got)
	}
	if state.spreadsheetSelectionStatsPending {
		t.Fatal("single-cell selection started a statistics scan")
	}
}

func TestSpreadsheetStatusBarRejectsStaleSelectionStats(t *testing.T) {
	app := New(nil, "")
	state := newDocumentSessionState("stale.xlsx")
	state.sheet = "Sheet1"
	state.wordRevisionAtomic.Store(4)
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{Column: 1}}
	state.selectionSet = true
	state.spreadsheetSelectionStatsPending = true
	state.spreadsheetSelectionStatsKeySet = true
	state.spreadsheetSelectionStatsResults = make(chan spreadsheetSelectionStatsResult, 1)
	state.spreadsheetSelectionStatsKey = spreadsheetSelectionStatsRequestKey{
		Generation: 8, Revision: 4, Sheet: "Sheet1", Bounds: state.selection.Bounds(), Sequence: 2,
	}
	state.spreadsheetSelectionStatsResults <- spreadsheetSelectionStatsResult{
		Key: spreadsheetSelectionStatsRequestKey{
			Generation: 8, Revision: 3, Sheet: "Sheet1", Bounds: state.selection.Bounds(), Sequence: 1,
		},
		Stats: document.SpreadsheetSelectionStats{Count: 2, NumericCount: 2, Sum: 10},
	}
	app.documentSessionState = state
	app.documentGeneration = 8
	app.processSpreadsheetSelectionStatsResults()
	if state.spreadsheetSelectionStatsSet || state.spreadsheetSelectionStats.Count != 0 {
		t.Fatalf("stale summary was applied: %+v", state.spreadsheetSelectionStats)
	}
	app.shutdownOnce.Do(func() { close(app.shutdown) })
	app.stopAndWaitManagedBackground()
}
