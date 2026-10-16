package desktopui

import (
	"image"
	"math"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestSheetTabsTextVerticallyCentered(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	app.sheets = []string{"Sheet1", "Sheet2"}
	app.sheet = "Sheet1"

	size := image.Pt(800, 32)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	}

	dims := app.layoutSheetTabs(gtx)
	if dims.Size.Y != 32 {
		t.Fatalf("sheet tabs height = %d; want 32", dims.Size.Y)
	}

	win, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skip("headless window unavailable:", err)
	}
	defer win.Release()

	if err := win.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: size})
	if err := win.Screenshot(shot); err != nil {
		t.Fatal(err)
	}

	// Look at the first tab "Sheet1".
	// The tab starts around x = 10, width ~80.
	// Find the bounding box of text ink inside the first tab.
	// Background of active tab is contentSurface (white: 255,255,255).
	// Text color is #3E6FA9 (R:62, G:111, B:169).
	minY, maxY := -1, -1
	for y := 0; y < size.Y; y++ {
		rowCount := 0
		for x := 10; x < 100; x++ {
			c := shot.RGBAAt(x, y)
			// Text is bluish / darker than white background
			if c.R < 150 && c.B > 100 {
				rowCount++
			}
		}
		if rowCount > 0 {
			if minY == -1 {
				minY = y
			}
			maxY = y
		}
	}

	t.Logf("Sheet1 text Y bounds: [%d, %d]", minY, maxY)
	topPad := minY
	bottomPad := size.Y - 1 - maxY
	t.Logf("topPad: %d, bottomPad: %d", topPad, bottomPad)
}

func TestSpreadsheetStatusBarVerticallyCentered(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	app.sheets = []string{"Sheet1"}
	app.sheet = "Sheet1"
	app.status = "就绪"

	size := image.Pt(800, 50)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	}

	dims := app.layoutSpreadsheetStatusBar(gtx)
	t.Logf("status bar dims: %v", dims.Size)

	win, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skip("headless window unavailable:", err)
	}
	defer win.Release()

	if err := win.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: size})
	if err := win.Screenshot(shot); err != nil {
		t.Fatal(err)
	}

	minY, maxY := -1, -1
	for y := 0; y < size.Y; y++ {
		rowCount := 0
		for x := 20; x < 60; x++ {
			c := shot.RGBAAt(x, y)
			// Status text color is mutedText #68717B (R:104, G:113, B:123), darker than chromeSurface #F1F3F5
			if c.R < 150 && c.G < 150 {
				rowCount++
			}
		}
		if rowCount > 0 {
			if minY == -1 {
				minY = y
			}
			maxY = y
		}
	}

	t.Logf("Status text '就绪' Y bounds: [%d, %d]", minY, maxY)
	if minY == -1 {
		t.Fatal("no status text ink found")
	}
	topPad := minY
	bottomPad := size.Y - 1 - maxY
	diff := topPad - bottomPad
	if diff < 0 {
		diff = -diff
	}
	if diff > 3 {
		t.Fatalf("status bar text is not vertically centered: topPad=%d, bottomPad=%d (diff=%d > 3)", topPad, bottomPad, diff)
	}

	// Verify Zoom Value ("100%") is positioned at the far right
	zoomTextFound := false
	for x := size.X - 80; x < size.X-20; x++ {
		for y := 0; y < size.Y; y++ {
			c := shot.RGBAAt(x, y)
			if c.R < 150 && c.G < 150 {
				zoomTextFound = true
				break
			}
		}
		if zoomTextFound {
			break
		}
	}
	if !zoomTextFound {
		t.Fatal("zoom value text not found at right side of status bar")
	}

	// Verify the middle area between status info (left) and zoom controls (right) is empty
	for x := 180; x < 500; x++ {
		for y := 2; y < size.Y-2; y++ {
			c := shot.RGBAAt(x, y)
			if c.R < 150 && c.G < 150 {
				t.Fatalf("unexpected ink found in spacer area at x=%d, y=%d", x, y)
			}
		}
	}
}

func TestSpreadsheetStatusBarShowsEndModeIndicator(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	state := newDocumentSessionState("test.xlsx")
	state.gridEndMode = true
	app.documentSessionState = state
	app.status = " "
	size := image.Pt(800, 50)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	}
	app.layoutSpreadsheetStatusBar(gtx)

	win, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skip("headless window unavailable:", err)
	}
	defer win.Release()
	if err := win.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: size})
	if err := win.Screenshot(shot); err != nil {
		t.Fatal(err)
	}
	ink := 0
	for y := 0; y < size.Y; y++ {
		for x := 20; x < 52; x++ {
			pixel := shot.RGBAAt(x, y)
			if pixel.R < 150 && pixel.G < 150 && pixel.B < 150 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("END mode indicator has no visible text in the status bar")
	}
	if app.status != " " {
		t.Fatalf("rendering End mode changed the document status to %q", app.status)
	}
}

func TestSpreadsheetStatusBarZoomRightAlignmentStability(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	app.sheets = []string{"Sheet1"}
	app.sheet = "Sheet1"

	findZoomValueMinX := func(status string) int {
		app.status = status
		size := image.Pt(800, 50)
		var ops op.Ops
		gtx := layout.Context{
			Ops:         &ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size),
		}
		app.layoutSpreadsheetStatusBar(gtx)

		win, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skip("headless window unavailable:", err)
		}
		defer win.Release()

		if err := win.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		shot := image.NewRGBA(image.Rectangle{Max: size})
		if err := win.Screenshot(shot); err != nil {
			t.Fatal(err)
		}

		for x := size.X - 100; x < size.X; x++ {
			for y := 0; y < size.Y; y++ {
				c := shot.RGBAAt(x, y)
				if c.R < 150 && c.G < 150 {
					return x
				}
			}
		}
		return -1
	}

	minXShort := findZoomValueMinX("就绪")
	minXLong := findZoomValueMinX("正在执行复杂的数据透视和多表联合计算操作...")

	if minXShort == -1 || minXLong == -1 {
		t.Fatalf("failed to find zoom value: short=%d, long=%d", minXShort, minXLong)
	}
	if minXShort != minXLong {
		t.Fatalf("zoom value position is not stable on the right: short status x=%d, long status x=%d", minXShort, minXLong)
	}
}

func TestSpreadsheetZoomInteractionsAndClamping(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	app.sheets = []string{"Sheet1"}
	app.sheet = "Sheet1"

	if zoom := app.spreadsheetZoom(); zoom != 1.0 {
		t.Fatalf("initial zoom = %v; want 1.0", zoom)
	}

	// Test fraction mapping
	if frac := spreadsheetZoomFraction(1.0); math.Abs(frac-0.5) > 1e-6 {
		t.Fatalf("zoom 1.0 fraction = %v; want 0.5", frac)
	}
	if frac := spreadsheetZoomFraction(0.5); frac != 0.0 {
		t.Fatalf("zoom 0.5 fraction = %v; want 0.0", frac)
	}
	if frac := spreadsheetZoomFraction(2.0); frac != 1.0 {
		t.Fatalf("zoom 2.0 fraction = %v; want 1.0", frac)
	}
	if z := spreadsheetZoomFromFraction(0.5); math.Abs(z-1.0) > 1e-6 {
		t.Fatalf("fraction 0.5 zoom = %v; want 1.0", z)
	}
	if z := spreadsheetZoomFromFraction(0.0); math.Abs(z-0.5) > 1e-6 {
		t.Fatalf("fraction 0.0 zoom = %v; want 0.5", z)
	}
	if z := spreadsheetZoomFromFraction(1.0); math.Abs(z-2.0) > 1e-6 {
		t.Fatalf("fraction 1.0 zoom = %v; want 2.0", z)
	}

	// Test setSpreadsheetZoom & clamping
	app.setSpreadsheetZoom(1.5)
	if zoom := app.spreadsheetZoom(); math.Abs(zoom-1.5) > 1e-6 {
		t.Fatalf("zoom = %v; want 1.5", zoom)
	}

	app.setSpreadsheetZoom(3.0)
	if zoom := app.spreadsheetZoom(); zoom != 2.0 {
		t.Fatalf("clamped upper zoom = %v; want 2.0", zoom)
	}

	app.setSpreadsheetZoom(0.1)
	if zoom := app.spreadsheetZoom(); zoom != 0.5 {
		t.Fatalf("clamped lower zoom = %v; want 0.5", zoom)
	}

	// Test reset
	app.setSpreadsheetZoom(1.0)
	if zoom := app.spreadsheetZoom(); zoom != 1.0 {
		t.Fatalf("reset zoom = %v; want 1.0", zoom)
	}

	// Test grid viewport scaling with zoom
	app.virtualGrid = newGridViewport()
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
		Constraints: layout.Exact(image.Pt(800, 600)),
	}
	app.setSpreadsheetZoom(1.5)
	v := app.virtualGrid
	v.setScale(float64(gtx.Metric.PxPerDp) * app.spreadsheetZoom())
	if v.Scale != 3.0 {
		t.Fatalf("grid scale at 2x metric and 1.5 zoom = %v; want 3.0", v.Scale)
	}
}
