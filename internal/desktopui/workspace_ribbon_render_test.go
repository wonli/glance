package desktopui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/workspace"
)

func TestSpreadsheetRibbonMeetsSelectedDocumentTabWithoutWideningTheSeam(t *testing.T) {
	const (
		width       = 720
		stripY      = 42
		toolbarY    = 90
		selectedX   = 140
		unselectedX = 20
	)

	app := New(nil, "")
	app.clientTitlebar = false
	app.activeTabID = "doc-a"
	app.tabMeta[app.activeTabID] = workspace.Tab{ID: app.activeTabID, Title: "report.xlsx", Position: 0}
	book := &toolbarTestBook{editable: true}
	state := newDocumentSessionState("")
	state.book = book
	app.documentSessionState = state
	app.book = book

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:    ops,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(width, stripY+toolbarY),
		},
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(app.layoutWorkspaceTabs),
		layout.Rigid(app.layoutSpreadsheetToolbar),
	)
	if dims.Size != image.Pt(width, stripY+toolbarY) {
		t.Fatalf("chrome size=%v, want %v", dims.Size, image.Pt(width, stripY+toolbarY))
	}

	window, err := headless.NewWindow(width, stripY+toolbarY)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(ops); err != nil {
		t.Fatalf("render workspace and ribbon chrome: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, width, stripY+toolbarY))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture workspace and ribbon chrome: %v", err)
	}

	assertPixel := func(x, y int, want color.NRGBA, description string) {
		t.Helper()
		got := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
		if got != want {
			t.Errorf("%s pixel (%d,%d)=%v, want %v", description, x, y, got, want)
		}
	}
	assertPixel(unselectedX, stripY-1, workspaceTabStripSurface, "unselected tab strip below the active tab")
	assertPixel(selectedX, stripY-1, contentSurface, "active document tab meets the ribbon")
	assertPixel(unselectedX, stripY, contentSurface, "ribbon starts at the tab strip edge")
}
