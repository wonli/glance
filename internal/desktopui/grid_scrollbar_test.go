package desktopui

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestSpreadsheetVerticalScrollbarSemanticsAndPosition(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "sample.csv")
	csvContent := "A,B,C\n1,2,3\n4,5,6\n7,8,9\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})
	app.theme = material.NewTheme()
	loaded, err := loadDocument(csvPath)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	defer closeLoadedDocument(loaded)

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	router := new(input.Router)
	var ops op.Ops
	gridWidth, gridHeight := 1000, 600
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(gridWidth, gridHeight)},
	}

	app.layoutVirtualGrid(gtx)
	router.Frame(&ops)

	var scrollbarBounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "表格垂直滚动条" {
				scrollbarBounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))

	if scrollbarBounds.Empty() {
		t.Fatal("表格垂直滚动条 has no accessible bounds in semantic tree")
	}

	// Scrollbar must align to the right edge of the grid
	if scrollbarBounds.Max.X != gridWidth {
		t.Fatalf("scrollbarBounds.Max.X = %d; want %d", scrollbarBounds.Max.X, gridWidth)
	}
	columnHeaderPx := gtx.Dp(unit.Dp(gridColumnHeader))
	if scrollbarBounds.Min.Y != columnHeaderPx {
		t.Fatalf("scrollbarBounds.Min.Y = %d; want columnHeaderPx (%d)", scrollbarBounds.Min.Y, columnHeaderPx)
	}
	if scrollbarBounds.Max.Y != gridHeight {
		t.Fatalf("scrollbarBounds.Max.Y = %d; want %d", scrollbarBounds.Max.Y, gridHeight)
	}
	if app.virtualGrid.ScrollbarWidthPx <= 0 {
		t.Fatalf("expected positive ScrollbarWidthPx, got %d", app.virtualGrid.ScrollbarWidthPx)
	}
}

func TestSpreadsheetVerticalScrollbarDoesNotSelectCell(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "sample.csv")
	csvContent := "A,B,C\n1,2,3\n4,5,6\n7,8,9\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})
	app.theme = material.NewTheme()
	loaded, err := loadDocument(csvPath)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	defer closeLoadedDocument(loaded)

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	// Set initial selection to A1
	app.navigateToCell(1, 1)
	if app.selectedCell != "A1" {
		t.Fatalf("initial cell = %q; want A1", app.selectedCell)
	}

	router := new(input.Router)
	var ops op.Ops
	gridWidth, gridHeight := 1000, 600
	renderFrame := func() {
		ops.Reset()
		gtx := layout.Context{
			Source:      router.Source(),
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(gridWidth, gridHeight)},
		}
		app.layoutVirtualGrid(gtx)
		router.Frame(&ops)
	}

	// Frame 1: render grid and setup scrollbar
	renderFrame()

	// Click on the rightmost scrollbar area (e.g. X = gridWidth - 4, Y = 200)
	scrollbarX := float32(gridWidth - 4)
	router.Queue(
		pointer.Event{
			Kind:     pointer.Press,
			Source:   pointer.Mouse,
			Buttons:  pointer.ButtonPrimary,
			Position: f32.Pt(scrollbarX, 200),
		},
		pointer.Event{
			Kind:     pointer.Release,
			Source:   pointer.Mouse,
			Position: f32.Pt(scrollbarX, 200),
		},
	)

	// Frame 2: process events
	renderFrame()

	// Selection should remain A1, NOT jump to whatever cell happened to be under the scrollbar
	if app.selectedCell != "A1" {
		t.Fatalf("selectedCell changed to %q after clicking scrollbar; want A1 preserved", app.selectedCell)
	}
	if app.cellEdit.Editing {
		t.Fatal("cellEdit became active after clicking scrollbar; want editing inactive")
	}
}

func TestSpreadsheetVerticalScrollbarTracksGridScroll(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "sample.csv")
	csvContent := "A,B,C\n1,2,3\n4,5,6\n7,8,9\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})
	app.theme = material.NewTheme()
	loaded, err := loadDocument(csvPath)
	if err != nil {
		t.Fatalf("loadDocument: %v", err)
	}
	defer closeLoadedDocument(loaded)

	if err := app.installDocument(loaded); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	router := new(input.Router)
	var ops op.Ops
	gridWidth, gridHeight := 1000, 600
	renderFrame := func() {
		ops.Reset()
		gtx := layout.Context{
			Source:      router.Source(),
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(gridWidth, gridHeight)},
		}
		app.layoutVirtualGrid(gtx)
		router.Frame(&ops)
	}

	renderFrame()
	v := app.virtualGrid
	initialY := v.Y

	// Scroll down via mouse wheel
	router.Queue(
		pointer.Event{
			Kind:     pointer.Scroll,
			Source:   pointer.Mouse,
			Position: f32.Pt(300, 300),
			Scroll:   f32.Pt(0, 150),
		},
	)

	renderFrame()

	if v.Y <= initialY {
		t.Fatalf("v.Y did not increase after downward scroll: before=%v after=%v", initialY, v.Y)
	}

	// Verify navigating to row 10,000 updates Y and keeps scrollbar valid
	app.navigateToCell(10000, 1)
	renderFrame()

	if v.Y <= initialY+1000 {
		t.Fatalf("v.Y did not advance to row 10000: v.Y=%v", v.Y)
	}
}
