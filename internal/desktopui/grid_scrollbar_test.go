package desktopui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/xlsx"
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

func TestSpreadsheetVerticalScrollbarUnifiedWithXLSX(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app.ensureDesktopRuntime().shutdown()
		close(app.shutdown)
	})
	app.theme = material.NewTheme()
	book := xlsx.NewBlank()
	loaded := loadedDocument{book: book, sheets: []string{"Sheet1"}}
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
		t.Fatal("XLSX grid vertical scrollbar has no accessible bounds")
	}
	if scrollbarBounds.Max.X != gridWidth {
		t.Fatalf("scrollbarBounds.Max.X = %d; want %d", scrollbarBounds.Max.X, gridWidth)
	}
}

func TestSpreadsheetVerticalScrollbarHeightBasedOnActualContent(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Content smaller than viewport (5 rows): thumb should be full height (start=0, end=1)
	csv5Path := filepath.Join(tempDir, "five_rows.csv")
	var b5 strings.Builder
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&b5, "row%d,data%d\n", i, i)
	}
	if err := os.WriteFile(csv5Path, []byte(b5.String()), 0644); err != nil {
		t.Fatalf("WriteFile five_rows: %v", err)
	}

	app5 := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app5.ensureDesktopRuntime().shutdown()
		close(app5.shutdown)
	})
	app5.theme = material.NewTheme()
	loaded5, err := loadDocument(csv5Path)
	if err != nil {
		t.Fatalf("loadDocument five_rows: %v", err)
	}
	defer closeLoadedDocument(loaded5)
	if err := app5.installDocument(loaded5); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	var ops op.Ops
	gridWidth, gridHeight := 1000, 600
	gtx5 := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(gridWidth, gridHeight)},
	}
	app5.layoutVirtualGrid(gtx5)
	if app5.virtualGrid.ScrollbarStart != 0 || app5.virtualGrid.ScrollbarEnd != 1 {
		t.Fatalf("expected full scrollbar thumb for 5 rows in 600px viewport, got start=%v end=%v",
			app5.virtualGrid.ScrollbarStart, app5.virtualGrid.ScrollbarEnd)
	}

	// 2. Content larger than viewport (100 rows): thumb height should reflect viewport / actualContentHeight
	csv100Path := filepath.Join(tempDir, "hundred_rows.csv")
	var b100 strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b100, "row%d,val%d\n", i, i)
	}
	if err := os.WriteFile(csv100Path, []byte(b100.String()), 0644); err != nil {
		t.Fatalf("WriteFile hundred_rows: %v", err)
	}

	app100 := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		app100.ensureDesktopRuntime().shutdown()
		close(app100.shutdown)
	})
	app100.theme = material.NewTheme()
	loaded100, err := loadDocument(csv100Path)
	if err != nil {
		t.Fatalf("loadDocument hundred_rows: %v", err)
	}
	defer closeLoadedDocument(loaded100)
	if err := app100.installDocument(loaded100); err != nil {
		t.Fatalf("installDocument: %v", err)
	}

	ops.Reset()
	gtx100 := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(gridWidth, gridHeight)},
	}
	app100.layoutVirtualGrid(gtx100)

	thumbFraction := app100.virtualGrid.ScrollbarEnd - app100.virtualGrid.ScrollbarStart
	// For 100 rows (~2000-2400px) in ~572px body height, thumbFraction should be around 0.20 - 0.35.
	// Previously it was ~0.028 (16px clamped minimum out of 1 million rows).
	if thumbFraction < 0.15 || thumbFraction > 0.40 {
		t.Fatalf("expected thumb fraction to reflect 100 rows (~0.20-0.35), got %v (start=%v, end=%v)",
			thumbFraction, app100.virtualGrid.ScrollbarStart, app100.virtualGrid.ScrollbarEnd)
	}

	// 3. XLSX document with 80 rows: thumb height should reflect 80 rows
	xlsxBook := xlsx.NewBlank()
	if err := xlsxBook.SetCellText("Sheet1", "A80", "bottom_data"); err != nil {
		t.Fatalf("SetCellText: %v", err)
	}
	appXLSX := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(func() {
		appXLSX.ensureDesktopRuntime().shutdown()
		close(appXLSX.shutdown)
	})
	appXLSX.theme = material.NewTheme()
	loadedXLSX := loadedDocument{book: xlsxBook, sheets: []string{"Sheet1"}}
	if err := appXLSX.installDocument(loadedXLSX); err != nil {
		t.Fatalf("installDocument: %v", err)
	}
	ops.Reset()
	gtxXLSX := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(gridWidth, gridHeight)},
	}
	appXLSX.layoutVirtualGrid(gtxXLSX)
	xlsxThumbFraction := appXLSX.virtualGrid.ScrollbarEnd - appXLSX.virtualGrid.ScrollbarStart
	// For 80 rows (~1600-1920px) in 572px body height, thumb fraction should be around 0.20 - 0.50
	if xlsxThumbFraction < 0.20 || xlsxThumbFraction > 0.50 {
		t.Fatalf("expected XLSX thumb fraction to reflect 80 rows (~0.25-0.45), got %v (start=%v, end=%v)",
			xlsxThumbFraction, appXLSX.virtualGrid.ScrollbarStart, appXLSX.virtualGrid.ScrollbarEnd)
	}
}

func TestSpreadsheetVerticalScrollbarFlushToWorkspaceEdge(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "sample.csv")
	csvContent := "A,B,C\n1,2,3\n"
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
	workspaceWidth, workspaceHeight := 1200, 700
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(workspaceWidth, workspaceHeight)),
	}

	// Layout the entire workbook (which includes toolbar, controls, virtual grid, and sheet tabs)
	app.layoutWorkbook(gtx)
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

	// Scrollbar MUST stick to the right edge of the workspace (Max.X == workspaceWidth)
	if scrollbarBounds.Max.X != workspaceWidth {
		t.Fatalf("scrollbarBounds.Max.X = %d; want %d (scrollbar should stick to workspace edge)",
			scrollbarBounds.Max.X, workspaceWidth)
	}
}
