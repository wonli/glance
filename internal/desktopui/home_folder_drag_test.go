package desktopui

import (
	"image"
	"image/color"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/workspace"
)

func TestHomeFolderDropTargetAtUsesRootCoordinates(t *testing.T) {
	targets := []homeFolderDropTarget{{FolderID: "folder", Bounds: image.Rect(18, 240, 238, 276)}}
	if got := homeFolderDropTargetAt(image.Pt(24, 250), targets); got != "folder" {
		t.Fatalf("drop target at root point (24,250)=%q, want folder", got)
	}
	if got := homeFolderDropTargetAt(image.Pt(250, 250), targets); got != "" {
		t.Fatalf("drop target outside root bounds=%q, want empty", got)
	}
}

func TestHomeFolderDropTargetsMatchRenderedSidebarCoordinates(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "target", Name: "目标"}}})
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(250, 700))}
		app.layoutHomeSidebar(gtx)
		router.Frame(&ops)
	}
	draw()
	if len(app.homeFolderDropTargets) != 1 {
		t.Fatalf("rendered sidebar has %d drop targets, want one: %+v", len(app.homeFolderDropTargets), app.homeFolderDropTargets)
	}
	target := app.homeFolderDropTargets[0]
	if target.Bounds.Min != app.homeFolderTreeRootOrigin || target.Bounds.Dx() != 214 || target.Bounds.Dy() != 36 {
		t.Fatalf("drop target bounds=%v, tree origin=%v; want a 214x36 target at the rendered tree origin", target.Bounds, app.homeFolderTreeRootOrigin)
	}
	center := f32.Pt(float32(target.Bounds.Min.X+target.Bounds.Dx()/2), float32(target.Bounds.Min.Y+target.Bounds.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 3, Position: center})
	draw()
	if row := app.homeFolderRows["target"]; row == nil || !row.selectButton.Hovered() {
		t.Fatal("computed root-coordinate drop target does not overlap the rendered folder row")
	}
}

func TestHomeRecentRowOriginMatchesRenderedTableCoordinates(t *testing.T) {
	app := newHomeFolderTestApp(t)
	path := filepath.Join(t.TempDir(), "recent.docx")
	app.collection.StageRecents([]workspace.Recent{{Path: path, Kind: "docx", OpenedAt: time.Now()}})
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1440, 900))}
		app.layoutRecent(gtx)
		router.Frame(&ops)
	}
	draw()
	rowOrigin := app.homeRecentRowOrigin(0, layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
	if rowOrigin != app.homeRecentListRootOrigin {
		t.Fatalf("recent row origin=%v, rendered list origin=%v; first row should start at the list origin", rowOrigin, app.homeRecentListRootOrigin)
	}
	point := f32.Pt(float32(app.homeRecentListRootOrigin.X+100), float32(app.homeRecentListRootOrigin.Y+20))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 5, Position: point})
	draw()
	if button := app.homeOpenButtons[path]; button == nil || !button.Hovered() {
		t.Fatal("computed recent-row root coordinates do not overlap the rendered document row")
	}
}

func TestHomeFolderDropTargetsExcludeCollapsedAndOffscreenRows(t *testing.T) {
	snapshot := workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{
		{ID: "parent", Name: "父级"},
		{ID: "child", ParentID: "parent", Name: "子级"},
		{ID: "other", Name: "其他"},
		{ID: "last", Name: "末项"},
	}}
	collapsedRows := homeFolderTreeRows(snapshot, map[string]bool{})
	collapsedTargets := homeFolderDropTargetsForVisibleRows(collapsedRows, 0, 0, 2, image.Pt(18, 10), image.Rect(18, 10, 238, 100), "", 36, 102)
	for _, target := range collapsedTargets {
		if target.FolderID == "child" {
			t.Fatal("collapsed child remained a drop target")
		}
	}

	visibleRows := homeFolderTreeRows(snapshot, map[string]bool{"parent": true})
	viewport := image.Rect(18, 20, 238, 56)
	targets := homeFolderDropTargetsForVisibleRows(visibleRows, 1, 8, 2, image.Pt(18, 20), viewport, "", 36, 102)
	if len(targets) != 2 || targets[0].FolderID != visibleRows[1].folder.ID || targets[1].FolderID != visibleRows[2].folder.ID {
		t.Fatalf("visible targets=%+v, want the two rows starting at list index 1", targets)
	}
	if targets[0].Bounds != image.Rect(18, 20, 238, 48) || targets[1].Bounds != image.Rect(18, 48, 238, 56) {
		t.Fatalf("targets were not clipped to the tree viewport: %+v", targets)
	}
	for _, target := range targets {
		if target.FolderID == "last" {
			t.Fatal("offscreen row remained a drop target")
		}
	}
}

func TestHomeRecentRowDragDropAssignsWithoutOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.docx")
	app, frame, queue, openCount := newHomeFolderDragHarness(t, path, workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "target", Name: "目标"}}})
	frame()
	dragHomeDocument(queue, frame, f32.Pt(550, 420), f32.Pt(120, 148), 11)

	snapshot, _ := app.homeFolderSnapshot()
	if len(snapshot.Assignments) != 1 || snapshot.Assignments[0] != (workspace.HomeFolderAssignment{Path: path, FolderID: "target"}) {
		t.Fatalf("drag assignment=%+v, want %q in target", snapshot.Assignments, path)
	}
	if *openCount != 0 {
		t.Fatal("dragging a document invoked the row's open action")
	}
}

func TestHomeRecentRowDragCancelPreservesAssignment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.docx")
	origin := workspace.HomeFolder{ID: "origin", Name: "原文件夹"}
	target := workspace.HomeFolder{ID: "target", Name: "目标"}
	snapshot, _ := (workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{origin, target}}).AssignDocument(path, origin.ID)
	app, frame, queue, _ := newHomeFolderDragHarness(t, path, snapshot)
	frame()
	queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 11, Buttons: pointer.ButtonPrimary, Position: f32.Pt(550, 420)})
	frame()
	queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 11, Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, 148)})
	frame()
	queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 11, Position: f32.Pt(800, 650)})
	frame()
	got, _ := app.homeFolderSnapshot()
	if len(got.Assignments) != 1 || got.Assignments[0].FolderID != origin.ID {
		t.Fatalf("cancel changed document assignment: %+v", got.Assignments)
	}
	if app.homeActiveDropFolderID != "" {
		t.Fatalf("cancel left folder %q highlighted", app.homeActiveDropFolderID)
	}
}

func TestHomeRecentRowDropOnSameFolderIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.docx")
	snapshot, _ := (workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "target", Name: "目标"}}}).AssignDocument(path, "target")
	app, frame, queue, _ := newHomeFolderDragHarness(t, path, snapshot)
	_, revision := app.homeFolderSnapshot()
	frame()
	dragHomeDocument(queue, frame, f32.Pt(550, 420), f32.Pt(120, 148), 11)
	_, gotRevision := app.homeFolderSnapshot()
	if gotRevision != revision {
		t.Fatalf("dropping into the same folder changed revision %d to %d", revision, gotRevision)
	}
}

func TestHomeRecentRowClickStillOpensAfterDragSupport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.docx")
	app, frame, queue, openCount := newHomeFolderDragHarness(t, path, workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "target", Name: "目标"}}})
	frame()
	position := f32.Pt(550, 420)
	queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 11, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 11, Position: position})
	frame()
	if *openCount != 1 {
		t.Fatalf("ordinary click opened %d times, want 1", *openCount)
	}
	if snapshot, _ := app.homeFolderSnapshot(); len(snapshot.Assignments) != 0 {
		t.Fatalf("ordinary click unexpectedly changed folder assignment: %+v", snapshot.Assignments)
	}
}

func TestHomeFolderDragVisualStatesAtOneAndTwoScale(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(map[float32]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			app := newHomeFolderTestApp(t)
			row := homeFolderTreeRow{folder: workspace.HomeFolder{ID: "target", Name: "目标"}}
			app.homeActiveDropFolderID = row.folder.ID
			app.homeFolderRows[row.folder.ID] = &homeFolderRowWidgets{menuItems: make(map[string]*widget.Clickable)}
			width, height := int(240*scale), int(36*scale)
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(width, height))}
			if got := app.layoutHomeFolderTreeRowContents(gtx, row, app.homeFolderRows[row.folder.ID]); got.Size != image.Pt(width, height) {
				t.Fatalf("drop highlight size=%v, want %dx%d", got.Size, width, height)
			}
			window, err := headless.NewWindow(width, height)
			if err != nil {
				t.Skipf("headless GPU is unavailable: %v", err)
			}
			defer window.Release()
			if err := window.Frame(&ops); err != nil {
				t.Fatalf("render %v drag target: %v", scale, err)
			}
			screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
			if err := window.Screenshot(screenshot); err != nil {
				t.Fatalf("capture %v drag target: %v", scale, err)
			}
			pixel := screenshot.At(width-4, height/2)
			if got := color.NRGBAModel.Convert(pixel); got != homeDropTargetFill {
				t.Fatalf("drop target background at %v=%v, want %v", image.Pt(width-4, height/2), got, homeDropTargetFill)
			}
		})
	}
}

func TestHomeDocumentDragPreviewTracksPointerOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dragged.docx")
	app, frame, queue, _ := newHomeFolderDragHarness(t, path, workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "target", Name: "目标"}}})
	start := f32.Pt(550, 420)
	target := f32.Pt(120, 148)
	frame() // Register the persistent drag region before delivering pointer input.
	dragHomeDocument(queue, frame, start, target, 11)
	state := app.homeDocumentDragState(path)
	wantPointer := image.Pt(int(target.X), int(target.Y))
	if state.pointer != wantPointer {
		t.Fatalf("preview pointer=%v; want cursor position %v (press=%v offset=%v started=%v)", state.pointer, wantPointer, state.press, state.pressOffset, state.started)
	}
	if state.pressOffset != image.Pt(50, 20) {
		t.Fatalf("grab offset=%v; want the pointer's original position inside the row (50,20)", state.pressOffset)
	}
	wantOrigin := wantPointer.Sub(image.Pt(50, 20))
	if got := state.pointer.Sub(state.pressOffset); got != wantOrigin {
		t.Fatalf("preview origin=%v; want it to preserve the pointer's grab offset %v", got, wantOrigin)
	}
}

func TestHomeDocumentDragOverlayRendersFloatingRow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := newHomeFolderTestApp(t)
	path := filepath.Join(t.TempDir(), "dragged.docx")
	app.homeActiveDragPath = path
	app.homeDocumentDrags = map[string]*homeDocumentDragState{
		path: {started: true, pointer: image.Pt(250, 180), pressOffset: image.Pt(30, 14), rowSize: image.Pt(220, 56)},
	}
	window, err := headless.NewWindow(420, 280)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(420, 280))}
	app.layoutHomeDocumentDragOverlay(gtx)
	if err := window.Frame(&ops); err != nil {
		t.Fatalf("render floating document row: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, 420, 280))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture floating document row: %v", err)
	}
	inside := color.NRGBAModel.Convert(screenshot.At(400, 180)).(color.NRGBA)
	outside := color.NRGBAModel.Convert(screenshot.At(160, 130)).(color.NRGBA)
	if inside.A < 240 || outside.A != 0 {
		t.Fatalf("floating preview coverage inside=%+v outside=%+v; want a solid row at cursor offset and clear old position", inside, outside)
	}
}

func newHomeFolderDragHarness(t *testing.T, path string, snapshot workspace.HomeFolderSnapshot) (*App, func(), func(...event.Event), *int) {
	t.Helper()
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(snapshot)
	app.homeFolderDropTargets = []homeFolderDropTarget{{FolderID: "target", Bounds: image.Rect(18, 130, 238, 166)}}
	app.homeFolderTreeRootOrigin = image.Pt(18, 100)
	app.homeRecentListRootOrigin = image.Pt(500, 400)

	var router input.Router
	var ops op.Ops
	var openButton widget.Clickable
	openCount := new(int)
	origin := image.Pt(500, 400)
	frame := func() {
		ops.Reset()
		root := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1000, 700)),
		}
		app.updateHomeDocumentDrag(root, path, origin)
		if openButton.Clicked(root) {
			*openCount++
		}
		local := root
		local.Constraints = layout.Exact(image.Pt(200, 56))
		offset := op.Offset(origin).Push(&ops)
		area := clip.Rect{Max: local.Constraints.Max}.Push(&ops)
		openButton.Layout(local, func(gtx layout.Context) layout.Dimensions {
			app.homeDocumentDragState(path).drag.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		area.Pop()
		offset.Pop()
		app.layoutHomeDocumentDragOverlay(root)
		router.Frame(&ops)
	}
	queue := func(events ...event.Event) { router.Queue(events...) }
	return app, frame, queue, openCount
}

func dragHomeDocument(queue func(...event.Event), frame func(), start, target f32.Point, pointerID pointer.ID) {
	queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: pointerID, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: pointerID, Buttons: pointer.ButtonPrimary, Position: target})
	frame()
	queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: pointerID, Position: target})
	frame()
}
