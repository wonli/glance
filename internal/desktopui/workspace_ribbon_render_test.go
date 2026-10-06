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
		toolbarY    = 102
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

func TestWordDocumentTabStripMatchesPenClpgmGeometry(t *testing.T) {
	app := New(nil, "")
	tab, close := app.documentTabWidgets("doc-a")
	app.activeTabID = "doc-a"
	app.tabMeta["doc-a"] = workspace.Tab{ID: "doc-a", Title: "项目策划书.docx", Position: 0}
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:    ops,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(600, 42),
		},
	}
	dims := app.paintDocumentTab(gtx, tab, close, "项目策划书.docx", true)
	if dims.Size != image.Pt(190, 37) {
		t.Fatalf("Pen Clpgm document tab size=%v, want 190×37 dp", dims.Size)
	}
	itemGtx := gtx
	itemGtx.Ops = new(op.Ops)
	itemDims := app.layoutWorkspaceTabListItem(itemGtx, []string{"doc-a"}, 1)
	if itemDims.Size != image.Pt(196, 37) {
		t.Fatalf("Word document tab slot size=%v, want 196×37 dp (6 dp leading gap plus 190 dp tab)", itemDims.Size)
	}
}
