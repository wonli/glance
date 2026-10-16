package desktopui

import (
	"fmt"
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/workspace"
)

func TestSpreadsheetSheetTabsScrollHorizontallyWithWheelAndTouchpad(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scroll f32.Point
	}{
		{name: "mouse wheel over strip", scroll: f32.Pt(0, 120)},
		{name: "horizontal touchpad over strip", scroll: f32.Pt(120, 0)},
		{name: "diagonal touchpad with opposing vertical component", scroll: f32.Pt(120, -50)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := New(nil, "")
			defer close(a.shutdown)
			names := make([]string, 24)
			for i := range names {
				names[i] = fmt.Sprintf("Sheet%d", i+1)
			}
			a.sheets = names
			a.sheet = names[0]
			a.documentSessionState.sheets = names
			a.documentSessionState.sheet = names[0]

			var router input.Router
			var ops op.Ops
			frame := func() {
				t.Helper()
				ops.Reset()
				gtx := layout.Context{
					Ops: &ops, Source: router.Source(), Now: time.Now(),
					Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(460, 32)),
				}
				a.layoutSheetTabs(gtx)
				router.Frame(&ops)
			}
			frame()
			frame()
			before := a.documentSessionState.sheetTabList.Position
			router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 16), Scroll: tc.scroll})
			frame()
			after := a.documentSessionState.sheetTabList.Position
			if before.First == after.First && before.Offset == after.Offset {
				t.Fatalf("sheet strip did not scroll: before=%+v after=%+v", before, after)
			}
			frame()
			settled := a.documentSessionState.sheetTabList.Position
			if settled.First != after.First || settled.Offset != after.Offset {
				t.Fatalf("sheet tab scroll did not remain settled: after=%+v settled=%+v", after, settled)
			}
		})
	}
}

func TestSpreadsheetSheetSelectionRevealsActiveTabAfterManySheets(t *testing.T) {
	a := New(nil, "")
	defer close(a.shutdown)
	names := make([]string, 24)
	for i := range names {
		names[i] = fmt.Sprintf("Sheet%d", i+1)
	}
	a.sheets = names
	a.sheet = names[len(names)-1]
	a.documentSessionState.sheets = names
	a.documentSessionState.sheet = names[len(names)-1]

	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(460, 32)),
		}
		a.layoutSheetTabs(gtx)
	}
	frame()
	frame()
	position := a.documentSessionState.sheetTabList.Position
	selectedIndex := len(names) - 1
	if selectedIndex < position.First || selectedIndex >= position.First+position.Count {
		t.Fatalf("active worksheet tab index %d is outside visible items [%d,%d)", selectedIndex, position.First, position.First+position.Count)
	}
}

func TestWorkspaceTabsWheelScrollHorizontally(t *testing.T) {
	a := New(nil, "")
	defer close(a.shutdown)
	a.clientTitlebar = true
	a.activeTabID = workspace.RecentTabID
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("doc-%02d", i)
		a.tabMeta[id] = workspace.Tab{ID: id, Title: id, Position: i}
	}

	var router input.Router
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 42)),
		}
		a.layoutWorkspaceTabs(gtx)
		router.Frame(&ops)
	}
	frame()
	frame()
	before := a.workspaceTabsList.Position
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(220, 20), Scroll: f32.Pt(0, 120)})
	frame()
	after := a.workspaceTabsList.Position
	if before.First == after.First && before.Offset == after.Offset {
		t.Fatalf("workspace tab strip did not scroll: before=%+v after=%+v", before, after)
	}
	frame()
	settled := a.workspaceTabsList.Position
	if settled.First != after.First || settled.Offset != after.Offset {
		t.Fatalf("workspace tab scroll did not remain settled: after=%+v settled=%+v", after, settled)
	}
}

func TestWorkspaceTabSelectionRevealsActiveTabAfterManyTabs(t *testing.T) {
	a := New(nil, "")
	defer close(a.shutdown)
	a.clientTitlebar = true
	ids := make([]string, 14)
	for i := range ids {
		id := fmt.Sprintf("doc-%02d", i)
		ids[i] = id
		a.tabMeta[id] = workspace.Tab{ID: id, Title: id, Position: i}
	}
	a.activeTabID = ids[len(ids)-1]

	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(500, 42)),
		}
		a.layoutWorkspaceTabs(gtx)
	}
	frame()
	frame()
	selectedIndex := len(ids) // Recent is item 0; doc i is item i+1.
	position := a.workspaceTabsList.Position
	if selectedIndex < position.First || selectedIndex >= position.First+position.Count {
		t.Fatalf("active workspace tab index %d is outside visible items [%d,%d)", selectedIndex, position.First, position.First+position.Count)
	}
}

func TestSpreadsheetSheetTabsClickable(t *testing.T) {
	a := New(nil, "")
	defer close(a.shutdown)
	names := []string{"Sheet1", "Sheet2", "Sheet3"}
	a.sheets = names
	a.sheet = names[0]
	a.documentSessionState.sheets = names
	a.documentSessionState.sheet = names[0]

	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(460, 32)),
		}
		a.layoutSheetTabs(gtx)
		router.Frame(&ops)
	}
	draw()
	// Click second sheet tab. Sheet1 is around 10..80px, Sheet2 starts around 84px.
	pt := f32.Pt(110, 16)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: pt, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pt})
	draw()

	if a.documentSessionState.sheet != "Sheet2" {
		t.Fatalf("clicking sheet tab activated %q; want Sheet2", a.documentSessionState.sheet)
	}
	if a.sheet != "Sheet2" {
		t.Fatalf("app.sheet = %q; want Sheet2", a.sheet)
	}
}

func TestSpreadsheetSheetTabsUseHorizontalGioListWithoutScrollAnyAxis(t *testing.T) {
	a := New(nil, "")
	defer close(a.shutdown)
	names := []string{"Sheet1", "Sheet2"}
	a.sheets = names
	a.sheet = names[0]
	a.documentSessionState.sheets = names
	a.documentSessionState.sheet = names[0]

	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(460, 32)),
	}
	a.layoutSheetTabs(gtx)

	if a.documentSessionState.sheetTabList.Axis != layout.Horizontal {
		t.Fatalf("sheet tab list axis=%v, want horizontal", a.documentSessionState.sheetTabList.Axis)
	}
	if a.documentSessionState.sheetTabList.ScrollAnyAxis {
		t.Fatal("sheet tab list should keep horizontal scrolling separate from vertical wheel/touchpad noise")
	}
}
