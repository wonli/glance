package desktopui

import (
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/workspace"
)

func TestHomeFolderVisibleRecentsFiltersDirectMembershipAndExistingFilters(t *testing.T) {
	app := newHomeFolderTestApp(t)
	root := workspace.HomeFolder{ID: "root", Name: "项目"}
	child := workspace.HomeFolder{ID: "child", ParentID: root.ID, Name: "子项"}
	other := workspace.HomeFolder{ID: "other", Name: "其他"}
	snapshot := workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{root, child, other}}
	var recents []workspace.Recent
	for i := range 7 {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("项目文档%d.docx", i))
		recents = append(recents, workspace.Recent{Path: path, Kind: "docx", OpenedAt: time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)})
		var err error
		snapshot, err = snapshot.AssignDocument(canonicalDocumentPath(path), root.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	childPath := filepath.Join(t.TempDir(), "子文件.xlsx")
	childRecent := workspace.Recent{Path: childPath, Kind: "xlsx", OpenedAt: time.Date(2026, 1, 1, 0, 8, 0, 0, time.UTC)}
	recents = append(recents, childRecent)
	snapshot, _ = snapshot.AssignDocument(canonicalDocumentPath(childPath), child.ID)
	otherPath := filepath.Join(t.TempDir(), "其他.docx")
	recents = append(recents, workspace.Recent{Path: otherPath, Kind: "docx"})
	snapshot, _ = snapshot.AssignDocument(canonicalDocumentPath(otherPath), other.ID)
	app.desktopRuntime.setHomeFolderTestSnapshot(snapshot)
	app.collection.StageRecents(recents)

	if got := len(app.homeVisibleRecents(true)); got != 5 {
		t.Fatalf("recent view count=%d, want existing five-row cap", got)
	}
	app.selectHomeFolder(root.ID)
	if got := len(app.homeFolderVisibleRecents(root.ID)); got != 7 {
		t.Fatalf("folder view count=%d, want all 7 direct documents", got)
	}
	app.homeTypeFilter = "文档"
	app.homeSearch.SetText("文档3")
	got := app.homeFolderVisibleRecents(root.ID)
	if len(got) != 1 || filepath.Base(got[0].Path) != "项目文档3.docx" {
		t.Fatalf("folder filters returned %+v, want the matching direct document", got)
	}
	app.homeSearch.SetText("")
	app.homeTypeFilter = "全部"
	if got := app.homeFolderVisibleRecents(root.ID); len(got) != 7 {
		t.Fatalf("direct membership included nested/other-folder documents: %d", len(got))
	}
	app.homeSection = homeSectionRecent
	if got := len(app.homeVisibleRecents(true)); got != 5 {
		t.Fatalf("recent view hid assigned documents or changed its cap: %d", got)
	}
}

func TestHomeFolderMembershipMatchesRelativeRecentPaths(t *testing.T) {
	app := newHomeFolderTestApp(t)
	relative := filepath.Join("relative-test", "relative.docx")
	snapshot, err := (workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "folder", Name: "资料"}}}).AssignDocument(relative, "folder")
	if err != nil {
		t.Fatal(err)
	}
	app.desktopRuntime.setHomeFolderTestSnapshot(snapshot)
	app.collection.StageRecents([]workspace.Recent{{Path: relative, Kind: "docx"}})
	if got := app.homeFolderVisibleRecents("folder"); len(got) != 1 || got[0].Path != relative {
		t.Fatalf("relative path assignment did not match the Recent row: %+v", got)
	}
}

func TestHomeFolderMembershipCanonicalizesSymlinkAliases(t *testing.T) {
	app := newHomeFolderTestApp(t)
	directory := t.TempDir()
	canonicalPath := filepath.Join(directory, "canonical.docx")
	aliasPath := filepath.Join(directory, "alias.docx")
	if err := osWriteFile(canonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonicalPath, aliasPath); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	snapshot, err := (workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{
		{ID: "folder", Name: "资料"},
		{ID: "other", Name: "其他"},
	}}).AssignDocument(canonicalDocumentPath(canonicalPath), "folder")
	if err != nil {
		t.Fatal(err)
	}
	app.desktopRuntime.setHomeFolderTestSnapshot(snapshot)
	app.collection.StageRecents([]workspace.Recent{{Path: aliasPath, Kind: "docx"}})
	app.homeMetadataResults = make(chan homeMetadataResult, 1)
	app.refreshHomeMetadata()
	select {
	case result := <-app.homeMetadataResults:
		if result.canonicalPaths[cleanHomeDocumentPath(aliasPath)] != canonicalDocumentPath(canonicalPath) {
			t.Fatalf("background metadata did not canonicalize the alias: %+v", result.canonicalPaths)
		}
		app.homeCanonicalPaths = result.canonicalPaths
	case <-time.After(time.Second):
		t.Fatal("timed out canonicalizing recent paths in the background")
	}
	got := app.homeFolderVisibleRecents("folder")
	if len(got) != 1 || got[0].Path != aliasPath {
		t.Fatalf("symlink recent did not match the canonical assignment: %+v", got)
	}
	if err := app.assignHomeDocumentToFolder(aliasPath, "other"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = app.homeFolderSnapshot()
	if len(snapshot.Assignments) != 1 || snapshot.Assignments[0].Path != canonicalDocumentPath(canonicalPath) || snapshot.Assignments[0].FolderID != "other" {
		t.Fatalf("symlink drag did not keep one canonical assignment: %+v", snapshot.Assignments)
	}
}

func TestHomeFolderSelectionOpensOneBranchPerLevel(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{
		{ID: "first-root", Name: "项目一"},
		{ID: "first-child", ParentID: "first-root", Name: "文档"},
		{ID: "first-grandchild", ParentID: "first-child", Name: "草稿"},
		{ID: "second-child", ParentID: "first-root", Name: "归档"},
		{ID: "second-root", Name: "项目二"},
		{ID: "second-root-child", ParentID: "second-root", Name: "工作簿"},
	}})

	app.selectHomeFolder("first-grandchild")
	if app.homeSelectedFolderID != "first-grandchild" || !app.homeFolderExpanded["first-root"] || !app.homeFolderExpanded["first-child"] {
		t.Fatalf("selecting nested folder did not open its path: selected=%q expanded=%v", app.homeSelectedFolderID, app.homeFolderExpanded)
	}
	if app.homeFolderExpanded["second-child"] || app.homeFolderExpanded["second-root"] {
		t.Fatalf("selecting nested folder left a same-level branch open: %v", app.homeFolderExpanded)
	}

	app.selectHomeFolder("second-child")
	if app.homeSelectedFolderID != "second-child" || !app.homeFolderExpanded["first-root"] || app.homeFolderExpanded["first-child"] {
		t.Fatalf("switching siblings did not move the open branch: selected=%q expanded=%v", app.homeSelectedFolderID, app.homeFolderExpanded)
	}

	app.selectHomeFolder("second-root-child")
	if app.homeSelectedFolderID != "second-root-child" || app.homeFolderExpanded["first-root"] || !app.homeFolderExpanded["second-root"] {
		t.Fatalf("switching roots did not close the previous branch: selected=%q expanded=%v", app.homeSelectedFolderID, app.homeFolderExpanded)
	}

	app.selectHomeFolder("")
	if app.homeSelectedFolderID != "" || len(app.homeFolderExpanded) != 0 {
		t.Fatalf("clearing the selected folder left the tree open: selected=%q expanded=%v", app.homeSelectedFolderID, app.homeFolderExpanded)
	}
}

func TestHomeFolderDisclosureAndSelectionStaySynchronized(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{
		{ID: "parent", Name: "项目"},
		{ID: "child", ParentID: "parent", Name: "文档"},
	}})
	app.toggleHomeFolderExpanded("parent")
	if app.homeSelectedFolderID != "parent" || !app.homeFolderExpanded["parent"] {
		t.Fatalf("opening a folder did not select it: selected=%q expanded=%v", app.homeSelectedFolderID, app.homeFolderExpanded)
	}
	app.toggleHomeFolderExpanded("parent")
	if app.homeSelectedFolderID != "" || len(app.homeFolderExpanded) != 0 {
		t.Fatalf("closing the active root folder did not clear its open selection: selected=%q expanded=%v", app.homeSelectedFolderID, app.homeFolderExpanded)
	}
}

func TestHomeFolderSwitchReusesDirectoryScanResults(t *testing.T) {
	app := newHomeFolderTestApp(t)
	firstDir, secondDir := t.TempDir(), t.TempDir()
	if err := osWriteFile(filepath.Join(firstDir, "first.docx")); err != nil {
		t.Fatal(err)
	}
	if err := osWriteFile(filepath.Join(secondDir, "second.xlsx")); err != nil {
		t.Fatal(err)
	}
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{
		{ID: "first", Name: "项目一", SourcePath: firstDir},
		{ID: "second", Name: "项目二", SourcePath: secondDir},
	}})
	app.selectHomeFolder("first")
	app.selectHomeFolder("second")
	results := make([]homeFolderContentsResult, 0, 2)
	for range 2 {
		select {
		case result := <-app.homeFolderContentsResults:
			results = append(results, result)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out scanning both selected folders")
		}
	}
	for _, result := range results {
		app.applyHomeFolderContentsResult(result)
	}
	if len(app.homeFolderContents["first"]) != 1 || len(app.homeFolderContents["second"]) != 1 {
		t.Fatalf("switching folders failed to cache both scans: first=%+v second=%+v", app.homeFolderContents["first"], app.homeFolderContents["second"])
	}

	firstGeneration := app.homeFolderContentsGeneration
	if err := osWriteFile(filepath.Join(firstDir, "new.docx")); err != nil {
		t.Fatal(err)
	}
	app.selectHomeFolder("first")
	if app.homeFolderContentsGeneration != firstGeneration {
		t.Fatalf("immediate folder switch rescanned the directory: generation=%d want %d", app.homeFolderContentsGeneration, firstGeneration)
	}
	if len(app.homeFolderContents["first"]) != 1 {
		t.Fatalf("cached folder contents were not immediately reusable: %+v", app.homeFolderContents["first"])
	}
}

func TestHomeFolderCreateUsesSelectedParentAndSelectsResult(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "parent", Name: "项目"}}})
	app.selectHomeFolder("parent")
	if err := app.createHomeFolder(app.homeSelectedFolderID, "  文档  "); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := app.homeFolderSnapshot()
	if len(snapshot.Folders) != 2 {
		t.Fatalf("created snapshot has %d folders, want 2", len(snapshot.Folders))
	}
	child := snapshot.Folders[1]
	if child.ParentID != "parent" || child.Name != "文档" {
		t.Fatalf("created folder=%+v, want normalized child of selected parent", child)
	}
	if app.homeSelectedFolderID != child.ID || !app.homeFolderExpanded["parent"] {
		t.Fatal("successful create did not select the new folder and expand its parent")
	}
}

func TestHomeFolderRenameAndRemoveKeepLogicalSemantics(t *testing.T) {
	app := newHomeFolderTestApp(t)
	root := workspace.HomeFolder{ID: "root", Name: "项目"}
	child := workspace.HomeFolder{ID: "child", ParentID: root.ID, Name: "文档"}
	path := canonicalDocumentPath(filepath.Join(t.TempDir(), "keep.docx"))
	snapshot, _ := (workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{root, child}}).AssignDocument(path, root.ID)
	app.desktopRuntime.setHomeFolderTestSnapshot(snapshot)
	app.selectHomeFolder(root.ID)
	if err := app.renameHomeFolder(child.ID, "资料"); err != nil {
		t.Fatal(err)
	}
	app.desktopRuntime.drainWorkspaceWriter()
	renameResult := awaitHomeFolderWrite(t, app.homeFolderWriteResults)
	app.homeFolderWriteResults <- renameResult
	app.processAsyncResults()
	_, beforeNoopRevision := app.homeFolderSnapshot()
	if err := app.renameHomeFolder(child.ID, "资料"); err != nil {
		t.Fatal(err)
	}
	_, afterNoopRevision := app.homeFolderSnapshot()
	if afterNoopRevision != beforeNoopRevision {
		t.Fatal("no-op rename submitted a folder snapshot")
	}
	if err := app.removeHomeFolder(root.ID); err != nil {
		t.Fatal(err)
	}
	removeResult := awaitHomeFolderWrite(t, app.homeFolderWriteResults)
	app.homeFolderWriteResults <- removeResult
	app.processAsyncResults()
	snapshot, _ = app.homeFolderSnapshot()
	if len(snapshot.Folders) != 1 || snapshot.Folders[0].ID != child.ID || snapshot.Folders[0].ParentID != "" {
		t.Fatalf("removed parent did not promote its child: %+v", snapshot.Folders)
	}
	if len(snapshot.Assignments) != 0 {
		t.Fatalf("removed folder retained direct document assignments: %+v", snapshot.Assignments)
	}
	if app.homeSection != homeSectionRecent || app.homeSelectedFolderID != "" {
		t.Fatal("removing the selected folder did not return to Recent")
	}
}

func TestHomeFolderMenuDismissesOnBlankClickAndEscape(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.homeFolderMenuFolderID = "folder"
	app.dismissHomeFolderMenu()
	if app.homeFolderMenuFolderID != "" {
		t.Fatal("blank-click menu dismissal left the folder menu open")
	}
	app.homeFolderMenuFolderID = "folder"
	app.dismissHomeFolderMenu()
	if app.homeFolderMenuFolderID != "" {
		t.Fatal("Escape menu dismissal left the folder menu open")
	}
}

func TestHomeFolderMenuSupportsArrowNavigationAndEnter(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "folder", Name: "项目资料"}}})
	app.toggleHomeFolderMenu("folder")
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(360, 260))}
		app.layoutHomeFolderTree(gtx)
		app.layoutHomeFolderMenuDismissInput(gtx)
		router.Frame(&ops)
	}
	draw()
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	draw()
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	draw()
	router.Queue(key.Event{Name: key.NameEnter, State: key.Press})
	draw()
	if !app.modal.Visible || app.modal.Title != "重命名文件夹" {
		t.Fatalf("Down, Down, Enter opened modal=%v title=%q; want the rename action", app.modal.Visible, app.modal.Title)
	}
	if app.homeFolderMenuFolderID != "" {
		t.Fatalf("Enter left folder menu open for %q", app.homeFolderMenuFolderID)
	}
}

func TestHomeFolderMenuUpSelectsLastItem(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "folder", Name: "项目资料"}}})
	app.toggleHomeFolderMenu("folder")
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(360, 260))}
		app.layoutHomeFolderTree(gtx)
		app.layoutHomeFolderMenuDismissInput(gtx)
		router.Frame(&ops)
	}
	draw()
	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	draw()
	router.Queue(key.Event{Name: key.NameEnter, State: key.Press})
	draw()
	if !app.modal.Visible || app.modal.Title != "移除文件夹" {
		t.Fatalf("Up, Enter opened modal=%v title=%q; want the remove action", app.modal.Visible, app.modal.Title)
	}
}

func TestHomeFolderMenuEscapeReturnsFocusToAnchor(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "folder", Name: "项目资料"}}})
	app.toggleHomeFolderMenu("folder")
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(360, 260))}
		app.layoutHomeFolderTree(gtx)
		app.layoutHomeFolderMenuDismissInput(gtx)
		router.Frame(&ops)
	}
	draw()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	draw()
	if app.homeFolderMenuFolderID != "" {
		t.Fatalf("Escape left folder menu open for %q", app.homeFolderMenuFolderID)
	}
	anchor := app.homeFolderRows["folder"]
	if anchor == nil || !router.Source().Focused(&anchor.menuButton) {
		t.Fatal("Escape did not return focus to the folder menu anchor")
	}
}

func TestHomeFolderMenuDismissesOnContentPanePress(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.homeFolderMenuFolderID = "folder"
	app.homeFolderMenuBounds = image.Rect(16, 30, 180, 128)
	app.homeFolderMenuAnchorBounds = image.Rect(12, 4, 200, 40)
	var ops op.Ops
	var router input.Router
	contentButton := new(widget.Clickable)
	draw := func() bool {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Point{}, Max: image.Pt(300, 200)},
		}
		clicked := contentButton.Clicked(gtx)
		transform := op.Offset(image.Pt(210, 90)).Push(gtx.Ops)
		buttonGtx := gtx
		buttonGtx.Constraints = layout.Exact(image.Pt(80, 40))
		contentButton.Layout(buttonGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(80, 40)}
		})
		transform.Pop()
		app.layoutHomeFolderMenuDismissInput(gtx)
		return clicked
	}
	clicked := draw()
	router.Frame(&ops)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(230, 110), Buttons: pointer.ButtonPrimary})
	clicked = draw() || clicked
	if app.homeFolderMenuFolderID == "" {
		t.Fatal("folder menu closed before the outside click could reach its target")
	}
	router.Frame(&ops)
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(230, 110)})
	clicked = draw() || clicked
	router.Frame(&ops)
	clicked = draw() || clicked
	if app.homeFolderMenuFolderID != "" {
		t.Fatal("releasing on the content pane did not dismiss the folder menu")
	}
	if !clicked {
		t.Fatalf("outside-click dismissal prevented the content pane button from receiving its click; history=%+v", contentButton.History())
	}
}

func TestHomeFolderFloatingMenuEscapesTreeRowHeightConstraints(t *testing.T) {
	app := newHomeFolderTestApp(t)
	widgets := &homeFolderRowWidgets{menuItems: make(map[string]*widget.Clickable)}
	var ops op.Ops
	gtx := layout.Context{
		Ops:    &ops,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		// layout.List gives row callbacks a large scrollable max height. The
		// popover must use its own height instead of painting to that maximum.
		Constraints: layout.Constraints{Min: image.Pt(240, 0), Max: image.Pt(240, 1_000_000)},
	}
	dims := app.layoutHomeFolderMenu(gtx, widgets)
	if dims.Size.Y != homeFolderMenuHeight {
		t.Fatalf("floating menu height=%d, want %dpx for three 42px actions, spacing, and padding", dims.Size.Y, homeFolderMenuHeight)
	}
	tightWidgets := &homeFolderRowWidgets{menuItems: make(map[string]*widget.Clickable)}
	tightGtx := layout.Context{Ops: new(op.Ops), Metric: gtx.Metric, Constraints: layout.Constraints{Min: image.Pt(240, 36), Max: image.Pt(240, 36)}}
	if tightDims := app.layoutHomeFolderMenu(tightGtx, tightWidgets); tightDims.Size.Y != homeFolderMenuHeight {
		t.Fatalf("menu inherited tight row height=%d, want %dpx", tightDims.Size.Y, homeFolderMenuHeight)
	}
	window, err := headless.NewWindow(240, 160)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(&ops); err != nil {
		t.Fatalf("render folder menu: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, 240, 160))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture folder menu: %v", err)
	}
	for index, label := range []string{"新建子文件夹", "重命名", "从列表移除"} {
		visiblePixels := 0
		rowTop := 6 + index*(homeFolderMenuItemHeight+4)
		for y := rowTop; y < rowTop+homeFolderMenuItemHeight; y++ {
			for x := 8; x < 232; x++ {
				pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
				if pixel != homeSurface {
					visiblePixels++
				}
			}
		}
		if visiblePixels == 0 {
			t.Errorf("folder menu row %q has no visible pixels", label)
		}
	}
	for id, button := range widgets.menuItems {
		if len(button.History()) != 0 {
			t.Fatalf("unexpected click history before interaction for %s", id)
		}
	}
}

func TestHomeFolderMenuIconsAndLabelsShareCenterline(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := newHomeFolderTestApp(t)
	actions := []struct {
		id    string
		label string
		icon  string
		color color.NRGBA
	}{
		{id: "create-child", label: "新建子文件夹", icon: "folder-plus", color: homeText},
		{id: "rename", label: "重命名", icon: "pencil", color: homeText},
		{id: "remove", label: "从列表移除", icon: "folder-minus", color: color.NRGBA{R: 196, G: 73, B: 73, A: 255}},
	}
	for _, scale := range []float32{1, 2} {
		t.Run(map[float32]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			width := int(250 * scale)
			height := int(float32(homeFolderMenuHeight) * scale)
			metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
			for _, action := range actions {
				if got := homeFolderMenuIconName(action.id); got != action.icon {
					t.Fatalf("menu item %q icon=%q; want Pen Lucide icon %q", action.label, got, action.icon)
				}
				if _, ok := svgIconTintAtSize("home", action.icon, int(18*scale), action.color); !ok {
					t.Fatalf("Pen Lucide icon %q has no matching home SVG asset", action.icon)
				}
				var measureOps op.Ops
				measureGtx := layout.Context{Ops: &measureOps, Metric: metric, Constraints: layout.Constraints{Max: image.Pt(width-60*int(scale), 1000*int(scale))}}
				naturalText := homeTextLabel(measureGtx, app.theme, action.label, unit.Sp(14), action.color, false)
				availableTextHeight := int(float32(homeFolderMenuItemHeight-6-6-7) * scale)
				if naturalText.Size.Y > availableTextHeight {
					t.Fatalf("menu item %q leaves %dpx for text, below its natural %dpx line height", action.label, availableTextHeight, naturalText.Size.Y)
				}
			}
			window, err := headless.NewWindow(width, height)
			if err != nil {
				t.Skipf("headless GPU is unavailable: %v", err)
			}
			defer window.Release()
			widgets := &homeFolderRowWidgets{menuItems: make(map[string]*widget.Clickable)}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(image.Pt(width, height))}
			if dims := app.layoutHomeFolderMenu(gtx, widgets); dims.Size != image.Pt(width, height) {
				t.Fatalf("menu size=%v; want %dx%d", dims.Size, width, height)
			}
			if err := window.Frame(&ops); err != nil {
				t.Fatalf("render menu text: %v", err)
			}
			screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
			if err := window.Screenshot(screenshot); err != nil {
				t.Fatalf("capture menu text: %v", err)
			}
			for index, action := range actions {
				rowTop := int(float32(6+index*(homeFolderMenuItemHeight+4)) * scale)
				iconFirst, iconLast := height, -1
				textFirst, textLast := height, -1
				for y := rowTop; y < rowTop+int(float32(homeFolderMenuItemHeight)*scale); y++ {
					for x := 18 * int(scale); x < width-8*int(scale); x++ {
						pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
						if pixel.A < 200 {
							continue
						}
						isInk := pixel.R < 170 && pixel.G < 180 && pixel.B < 200
						if action.id == "remove" {
							isInk = pixel.R > 130 && pixel.G < 150 && pixel.B < 150
						}
						if !isInk {
							continue
						}
						if x < 36*int(scale) {
							iconFirst = min(iconFirst, y)
							iconLast = max(iconLast, y)
						} else if x >= 46*int(scale) {
							textFirst = min(textFirst, y)
							textLast = max(textLast, y)
						}
					}
				}
				if iconLast < iconFirst || textLast < textFirst {
					t.Fatalf("menu item %q has no visible icon or text ink: icon=%d..%d text=%d..%d", action.label, iconFirst, iconLast, textFirst, textLast)
				}
				iconCenter := float32(iconFirst+iconLast) / 2
				textCenter := float32(textFirst+textLast) / 2
				delta := iconCenter - textCenter
				if delta < 0 {
					delta = -delta
				}
				if delta > scale {
					t.Fatalf("menu item %q icon/text ink centers are %.1fpx and %.1fpx (difference %.1fpx at %.1fx)", action.label, iconCenter, textCenter, delta, scale)
				}
			}
		})
	}
}

func TestHomeFolderMenuPositionFlipsAndClampsToShortViewport(t *testing.T) {
	rows := []homeFolderTreeRow{
		{folder: workspace.HomeFolder{ID: "first"}},
		{folder: workspace.HomeFolder{ID: "last"}},
	}
	index, top, visible := homeFolderMenuPosition(rows, "last", 0, 0, 2, 36, homeFolderMenuHeight, 72)
	if !visible || index != 1 || top != 0 {
		t.Fatalf("short viewport menu position=(%d,%d,%v); want visible last row and top=0", index, top, visible)
	}
	index, top, visible = homeFolderMenuPosition(rows, "first", 0, 0, 2, 36, homeFolderMenuHeight, 240)
	if !visible || index != 0 || top != 36 {
		t.Fatalf("roomy viewport menu position=(%d,%d,%v); want menu below first row", index, top, visible)
	}
}

func TestHomeFolderFloatingMenuEscapesShortListViewport(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{
		{ID: "first", Name: "第一个"},
		{ID: "last", Name: "最后一个"},
	}})
	app.homeFolderMenuFolderID = "last"
	app.homeFolderTreeRootOrigin = image.Point{}
	const width, treeHeight, windowHeight = 250, 72, 160
	window, err := headless.NewWindow(width, windowHeight)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Point{}, Max: image.Pt(width, treeHeight)},
	}
	if dims := app.layoutHomeFolderTree(gtx); dims.Size.Y != treeHeight {
		t.Fatalf("tree height=%d; want short %dpx viewport", dims.Size.Y, treeHeight)
	}
	if err := window.Frame(&ops); err != nil {
		t.Fatalf("render folder tree menu: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, width, windowHeight))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture folder tree menu: %v", err)
	}
	visiblePanelPixels := 0
	for y := treeHeight; y < homeFolderMenuHeight; y++ {
		for x := 16; x < width-4; x++ {
			pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
			if pixel.R > 245 && pixel.G > 245 && pixel.B > 245 && pixel.A > 245 {
				visiblePanelPixels++
			}
		}
	}
	if visiblePanelPixels < 100 {
		t.Fatalf("floating menu has only %d white panel pixels below the 72px list viewport; it was clipped", visiblePanelPixels)
	}
}

func TestHomeFolderMenuMatchesFolderWidthAndHighlightsHoveredItem(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "folder", Name: "项目资料"}}})
	app.homeFolderMenuFolderID = "folder"
	const width, height = 250, 220
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, height))}
		app.layoutHomeFolderTree(gtx)
		router.Frame(&ops)
	}
	draw()
	if got := app.homeFolderMenuBounds.Dx(); got != width {
		t.Fatalf("menu width=%d; want it to match the %dpx folder row", got, width)
	}
	point := f32.Pt(200, float32(app.homeFolderMenuBounds.Min.Y+6+homeFolderMenuItemHeight+4+homeFolderMenuItemHeight/2))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 4, Position: point})
	draw()
	draw()
	if row := app.homeFolderRows["folder"]; row == nil || !row.menuItems["rename"].Hovered() {
		t.Fatal("hovering the rename menu row did not select it")
	}
	if err := window.Frame(&ops); err != nil {
		t.Fatalf("render hovered folder menu: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture hovered folder menu: %v", err)
	}
	pointPixel := color.NRGBAModel.Convert(screenshot.At(200, int(point.Y))).(color.NRGBA)
	if pointPixel != homeHoverFill {
		t.Fatalf("hovered menu background=%v; want selected fill %v", pointPixel, homeHoverFill)
	}
}

func TestHomeFolderIconOpensAndSelectsFolder(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{
		{ID: "parent", Name: "项目资料"},
		{ID: "child", ParentID: "parent", Name: "子目录"},
	}})
	const width, height = 250, 200
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, height))}
		app.layoutHomeFolderTree(gtx)
		router.Frame(&ops)
	}
	draw()
	point := f32.Pt(22, 18)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 5, Buttons: pointer.ButtonPrimary, Position: point})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 5, Position: point})
	draw()
	if !app.homeFolderExpanded["parent"] || app.homeSelectedFolderID != "parent" {
		t.Fatalf("folder-icon click expanded=%v selected=%q; want the opened folder selected", app.homeFolderExpanded["parent"], app.homeSelectedFolderID)
	}
}

func TestHomeFolderSelectionReconcilesAfterExternalRemoval(t *testing.T) {
	first := newHomeFolderTestApp(t)
	first.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "selected", Name: "当前目录"}}})
	second := &App{
		desktopRuntime:         first.desktopRuntime,
		homeFolderWriteResults: make(chan homeFolderWriteResult, 1),
	}
	first.selectHomeFolder("selected")
	if err := second.removeHomeFolder("selected"); err != nil {
		t.Fatal(err)
	}
	if result := awaitHomeFolderWrite(t, second.homeFolderWriteResults); result.err != nil {
		t.Fatal(result.err)
	}
	first.reconcileHomeFolderSelection()
	if first.homeSelectedFolderID != "" || first.homeSection != homeSectionRecent {
		t.Fatalf("removed selection was not reconciled: folder=%q section=%d", first.homeSelectedFolderID, first.homeSection)
	}
}

func TestHomeFolderRemovalPendingSurvivesQueuedWriteFailuresAcrossWindows(t *testing.T) {
	first := newHomeFolderTestApp(t)
	first.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "selected", Name: "当前目录"}}})
	second := &App{desktopRuntime: first.desktopRuntime}
	first.selectHomeFolder("selected")
	second.selectHomeFolder("selected")

	runtime := first.desktopRuntime
	runtime.beginHomeFolderRemoval("selected")
	runtime.mu.Lock()
	runtime.homeFolderRevision = 1
	runtime.homeFolderWorking = workspace.HomeFolderSnapshot{}
	runtime.mu.Unlock()
	runtime.setHomeFolderRemovalRevision("selected", 1)
	runtime.mu.Lock()
	runtime.homeFolderRevision = 2
	runtime.homeFolderWorking = workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "later", Name: "后续变更"}}}
	runtime.mu.Unlock()
	selectionSnapshot, removalPending := runtime.homeFolderSelectionState("selected")
	if !removalPending || hasHomeFolder(selectionSnapshot, "selected") {
		t.Fatalf("atomic selection state snapshot=%+v pending=%v; want the optimistic removal and pending marker together", selectionSnapshot, removalPending)
	}
	first.reconcileHomeFolderSelection()
	if first.homeSelectedFolderID != "selected" {
		t.Fatal("another window's pending removal cleared the selected folder before persistence completed")
	}

	if runtime.finishHomeFolderWrite(homeFolderWriteResult{revision: 1, err: fmt.Errorf("forced removal failure")}) {
		t.Fatal("failed older removal unexpectedly rolled back the newer optimistic snapshot")
	}
	first.reconcileHomeFolderSelection()
	second.reconcileHomeFolderSelection()
	if first.homeSelectedFolderID != "selected" || second.homeSelectedFolderID != "selected" {
		t.Fatalf("intermediate failure cleared selection first=%q second=%q; want pending selections retained in both windows", first.homeSelectedFolderID, second.homeSelectedFolderID)
	}
	if !runtime.homeFolderRemovalPending("selected") {
		t.Fatal("older removal failure cleared its marker while a newer write was pending")
	}
	if !runtime.finishHomeFolderWrite(homeFolderWriteResult{revision: 2, err: fmt.Errorf("forced later write failure")}) {
		t.Fatal("failed latest folder write did not roll back to the committed snapshot")
	}
	first.reconcileHomeFolderSelection()
	second.reconcileHomeFolderSelection()
	if first.homeSelectedFolderID != "selected" || second.homeSelectedFolderID != "selected" {
		t.Fatalf("final rollback did not preserve selection first=%q second=%q", first.homeSelectedFolderID, second.homeSelectedFolderID)
	}
	if runtime.homeFolderRemovalPending("selected") {
		t.Fatal("settled removal failure left a shared pending marker")
	}
}

func TestHomeFolderSelectionReconcilesAfterWriteRollback(t *testing.T) {
	runtime, storePath := newHomeFolderRuntimeForTest(t)
	db, err := sql.Open("sqlite", storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_home_folder_selection BEFORE INSERT ON home_folders WHEN NEW.name='Fail selection' BEGIN SELECT RAISE(ABORT, 'forced folder insert failure'); END`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	app := &App{
		desktopRuntime:         runtime,
		homeFolderWriteResults: make(chan homeFolderWriteResult, 1),
		homeFolderExpanded:     make(map[string]bool),
	}
	if err := app.createHomeFolder("", "Fail selection"); err != nil {
		t.Fatal(err)
	}
	if app.homeSelectedFolderID == "" {
		t.Fatal("create did not optimistically select its pending folder")
	}
	if result := awaitHomeFolderWrite(t, app.homeFolderWriteResults); result.err == nil {
		t.Fatal("test folder write unexpectedly succeeded")
	}
	app.reconcileHomeFolderSelection()
	if app.homeSelectedFolderID != "" {
		t.Fatalf("failed create left a dangling selected folder %q", app.homeSelectedFolderID)
	}
}

func TestFailedHomeFolderRemovalKeepsSelectedFolder(t *testing.T) {
	runtime, storePath := newHomeFolderRuntimeForTest(t)
	db, err := sql.Open("sqlite", storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_home_folder_remove BEFORE DELETE ON home_folders WHEN OLD.name='selected folder' BEGIN SELECT RAISE(ABORT, 'forced folder delete failure'); END`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	app := &App{
		desktopRuntime:         runtime,
		homeFolderWriteResults: make(chan homeFolderWriteResult, 1),
		homeFolderRows:         make(map[string]*homeFolderRowWidgets),
		homeFolderExpanded:     make(map[string]bool),
	}
	if err := app.createHomeFolder("", "selected folder"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := app.homeFolderSnapshot()
	folderID := snapshot.Folders[0].ID
	if result := awaitHomeFolderWrite(t, app.homeFolderWriteResults); result.err != nil {
		t.Fatal(result.err)
	}
	app.selectHomeFolder(folderID)
	if err := app.removeHomeFolder(folderID); err != nil {
		t.Fatal(err)
	}
	if app.homeSelectedFolderID != folderID {
		t.Fatalf("selection cleared before persistence completed: %q", app.homeSelectedFolderID)
	}
	result := awaitHomeFolderWrite(t, app.homeFolderWriteResults)
	if result.err == nil {
		t.Fatal("test folder removal unexpectedly succeeded")
	}
	app.homeFolderWriteResults <- result
	app.processAsyncResults()
	snapshot, _ = app.homeFolderSnapshot()
	if app.homeSelectedFolderID != folderID || !hasHomeFolder(snapshot, app.homeSelectedFolderID) {
		t.Fatalf("failed removal did not preserve the restored selected folder: %q", app.homeSelectedFolderID)
	}
}

func TestHomeFolderEmptyStateAndRemoveCancel(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "empty", Name: "空文件夹"}}})
	app.collection.StageRecents([]workspace.Recent{{Path: filepath.Join(t.TempDir(), "outside.docx"), Kind: "docx"}})
	app.selectHomeFolder("empty")
	if got := app.homeFolderVisibleRecents("empty"); len(got) != 0 {
		t.Fatalf("empty folder produced %d documents", len(got))
	}
	app.showRemoveHomeFolderConfirm("empty")
	if !app.modal.Visible {
		t.Fatal("remove confirmation did not open")
	}
	app.modal.OnAction("cancel")
	snapshot, _ := app.homeFolderSnapshot()
	if len(snapshot.Folders) != 1 || app.homeSelectedFolderID != "empty" {
		t.Fatal("canceling remove changed the folder snapshot or selection")
	}
}

func TestHomeFolderEmptyStateDistinguishesEmptyAndFilteredResults(t *testing.T) {
	if got := homeFolderEmptyStateMessage(false); got != "暂无文件" {
		t.Fatalf("empty folder message=%q; want no-files message", got)
	}
	if got := homeFolderEmptyStateMessage(true); got != "没有符合条件的文件" {
		t.Fatalf("filtered folder message=%q; want no-matching-files message", got)
	}
}

func TestHomeContentScrollbarRangeTracksNaturalFolderPage(t *testing.T) {
	cases := []struct {
		name   string
		pos    layout.Position
		view   int
		start  float32
		end    float32
		active bool
	}{
		{name: "top", pos: layout.Position{Length: 2400}, view: 600, start: 0, end: .25, active: true},
		{name: "middle", pos: layout.Position{Length: 2400, Offset: 1500}, view: 600, start: .625, end: .875, active: true},
		{name: "bottom", pos: layout.Position{Length: 2400, Offset: 1800}, view: 600, start: .75, end: 1, active: true},
		{name: "fits", pos: layout.Position{Length: 600}, view: 600, start: 0, end: 1, active: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			start, end, active := homeContentScrollbarRange(test.pos, test.view)
			if start != test.start || end != test.end || active != test.active {
				t.Fatalf("range=(%v,%v) active=%v; want (%v,%v) active=%v", start, end, active, test.start, test.end, test.active)
			}
		})
	}
}

func TestHomeContentScrollbarLaysOutAtViewportEdge(t *testing.T) {
	app := &App{
		theme:                     material.NewTheme(),
		homeContentList:           widget.List{List: layout.List{Axis: layout.Vertical}},
		homeContentViewportHeight: 600,
	}
	app.homeContentList.Position.Length = 2400
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(10, 600))}
	dimensions := app.layoutHomeContentScrollbar(gtx)
	if dimensions.Size != image.Pt(10, 600) {
		t.Fatalf("scrollbar dimensions=%v; want a 10x600 track aligned to the viewport edge", dimensions.Size)
	}
}

func TestHomeFolderContentPageRendersRowsAtNaturalHeight(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{
		Folders: []workspace.HomeFolder{{ID: "folder", Name: "项目文件"}},
	})
	app.homeSelectedFolderID = "folder"
	for i := 0; i < 12; i++ {
		app.homeFolderContents["folder"] = append(app.homeFolderContents["folder"], workspace.Recent{
			Path: filepath.Join(t.TempDir(), fmt.Sprintf("document-%02d.docx", i)),
			Kind: "docx",
		})
	}
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1090, 900)},
	}
	dims := app.layoutHomeFolderContentPage(gtx)
	wantRowsHeight := 12 * gtx.Dp(unit.Dp(56))
	if dims.Size.Y < wantRowsHeight+gtx.Dp(unit.Dp(48)) {
		t.Fatalf("folder page height=%d; want room for all 12 rows and the table header", dims.Size.Y)
	}
}

func TestHomeFolderPageLoadsThirtyMoreRowsNearBottom(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.homeFolderPageID = "folder"
	app.homeFolderPageSize = homeFolderPageSize
	app.homeContentViewportHeight = 600
	app.homeContentList.Position.Length = 2400
	app.homeContentList.Position.Offset = 0
	app.loadMoreHomeFolderItems("folder", 65)
	if app.homeFolderPageSize != 30 {
		t.Fatalf("page size at top=%d; want 30", app.homeFolderPageSize)
	}
	app.homeContentList.Position.Offset = 1850
	app.loadMoreHomeFolderItems("folder", 65)
	if app.homeFolderPageSize != 60 {
		t.Fatalf("page size at bottom=%d; want 60", app.homeFolderPageSize)
	}
	app.homeContentList.Position.Length = 4080
	app.homeContentList.Position.Offset = 3480
	app.loadMoreHomeFolderItems("folder", 65)
	if app.homeFolderPageSize != 65 {
		t.Fatalf("last page size=%d; want all 65 remaining items", app.homeFolderPageSize)
	}
}

func TestHomeFolderNaturalScrollLoadsNextPageAtBottom(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "folder", Name: "项目"}}})
	app.homeSelectedFolderID = "folder"
	for index := 0; index < 65; index++ {
		app.homeFolderContents["folder"] = append(app.homeFolderContents["folder"], workspace.Recent{
			Path: filepath.Join(t.TempDir(), fmt.Sprintf("item-%02d.docx", index)),
			Kind: "docx",
		})
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1190, 900))}
	app.layoutHomeContent(gtx)
	if app.homeFolderPageSize != homeFolderPageSize || app.homeContentViewportHeight != 832 {
		t.Fatalf("initial folder page size=%d viewport=%d; want 30 rows in an 832px natural-scroll viewport", app.homeFolderPageSize, app.homeContentViewportHeight)
	}
	if app.homeContentList.Position.Length < 48+homeFolderPageSize*56 {
		t.Fatalf("natural folder list length=%d; want at least the header plus 30 rows", app.homeContentList.Position.Length)
	}
	app.homeContentList.Position.Offset = max(0, app.homeContentList.Position.Length-app.homeContentViewportHeight)
	ops.Reset()
	app.layoutHomeContent(gtx)
	if app.homeFolderPageSize != 60 {
		t.Fatalf("page size after reaching list bottom=%d; want 60", app.homeFolderPageSize)
	}
}

func TestFailedHomeFolderDirectoryRescanClearsStaleEntries(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.desktopRuntime.setHomeFolderTestSnapshot(workspace.HomeFolderSnapshot{Folders: []workspace.HomeFolder{{ID: "folder", Name: "项目", SourcePath: t.TempDir()}}})
	app.homeSelectedFolderID = "folder"
	app.homeFolderContents["folder"] = []workspace.Recent{{Path: filepath.Join(t.TempDir(), "stale.docx"), Kind: "docx"}}
	app.homeFolderContentsGeneration = 7
	app.homeFolderContentsLatest["folder"] = 7
	app.homeFolderContentsLoading["folder"] = true
	app.applyHomeFolderContentsResult(homeFolderContentsResult{folderID: "folder", generation: 7, err: fmt.Errorf("directory unavailable")})
	if len(app.homeFolderContents["folder"]) != 0 {
		t.Fatalf("failed directory rescan left stale files visible: %+v", app.homeFolderContents["folder"])
	}
}

func TestHomeFolderDirectoryRegistrationDeduplicatesAndShowsContentsWithoutAddingRecents(t *testing.T) {
	app := newHomeFolderTestApp(t)
	directory := t.TempDir()
	if err := osWriteFile(filepath.Join(directory, "document.docx")); err != nil {
		t.Fatal(err)
	}
	if err := osWriteFile(filepath.Join(directory, "workbook.xlsx")); err != nil {
		t.Fatal(err)
	}
	if err := osWriteFile(filepath.Join(directory, "slides.pptx")); err != nil {
		t.Fatal(err)
	}
	if err := osWriteFile(filepath.Join(directory, "ignore.txt")); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(directory, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := osWriteFile(filepath.Join(nested, "nested.docx")); err != nil {
		t.Fatal(err)
	}
	app.collection.StageRecents([]workspace.Recent{{Path: filepath.Join(t.TempDir(), "existing.docx"), Kind: "docx"}})
	if err := app.registerHomeFolderPath(directory); err != nil {
		t.Fatal(err)
	}
	if err := app.registerHomeFolderPath(filepath.Join(directory, ".")); err == nil {
		t.Fatal("registering an already registered path should return a duplicate notice")
	}
	snapshot, _ := app.homeFolderSnapshot()
	if len(snapshot.Folders) != 1 || snapshot.Folders[0].SourcePath != canonicalDocumentPath(directory) {
		t.Fatalf("registered folders=%+v, want one canonical root path", snapshot.Folders)
	}
	if len(app.collection.Recents()) != 1 {
		t.Fatal("folder listing changed the user's Recent document history")
	}
	select {
	case result := <-app.homeFolderContentsResults:
		app.applyHomeFolderContentsResult(result)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out scanning registered folder contents")
	}
	visible := app.homeFolderVisibleRecents(snapshot.Folders[0].ID)
	if len(visible) != 3 {
		t.Fatalf("registered directory shows %d supported direct files, want 3: %+v", len(visible), visible)
	}
	for _, item := range visible {
		if filepath.Base(item.Path) == "ignore.txt" || filepath.Base(item.Path) == "nested.docx" {
			t.Fatalf("registered directory showed unsupported or nested file %q", item.Path)
		}
	}
}

func TestHomeFolderDirectoryPickerCancelLeavesSnapshotUnchanged(t *testing.T) {
	app := newHomeFolderTestApp(t)
	app.setWordFileCommandRunner(func(kind fileCommandKind, _ string) fileCommandResult {
		if kind != commandRegisterHomeFolder {
			t.Fatalf("folder picker requested command %v", kind)
		}
		return fileCommandResult{kind: kind}
	})
	before, _ := app.homeFolderSnapshot()
	app.startHomeFolderDirectoryPicker()
	result := <-app.fileCommandResults
	app.handleHomeFolderPickerResult(result)
	after, _ := app.homeFolderSnapshot()
	if len(after.Folders) != len(before.Folders) || app.status != "已取消打开文件夹" {
		t.Fatalf("cancel changed folder state or status: folders=%+v status=%q", after.Folders, app.status)
	}
}

func newHomeFolderTestApp(t *testing.T) *App {
	t.Helper()
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	runtime.collection = collection
	if err := runtime.loadHomeFolderState(); err != nil {
		t.Fatal(err)
	}
	runtime.startWorkspaceWriter()
	app := &App{
		desktopRuntime:             runtime,
		theme:                      material.NewTheme(),
		homeSection:                homeSectionRecent,
		homeTypeFilter:             "全部",
		homeSortOpenedDesc:         true,
		homeActionButtons:          make(map[string]*widget.Clickable),
		homeFolderWriteResults:     make(chan homeFolderWriteResult, 1),
		homeFolderRows:             make(map[string]*homeFolderRowWidgets),
		homeFolderExpanded:         make(map[string]bool),
		homeFavorites:              make(map[string]bool),
		homeMetadata:               make(map[string]homeFileMetadata),
		homeFilterButtons:          make(map[string]*widget.Clickable),
		homeFolderContents:         make(map[string][]workspace.Recent),
		homeFolderContentsResults:  make(chan homeFolderContentsResult, 8),
		homeFolderContentsLatest:   make(map[string]uint64),
		homeFolderContentsLoading:  make(map[string]bool),
		homeFolderContentsLastScan: make(map[string]time.Time),
		homeOpenButtons:            make(map[string]*widget.Clickable),
		homeFavoriteButtons:        make(map[string]*widget.Clickable),
		homeMoreButtons:            make(map[string]*widget.Clickable),
		recentList:                 widget.List{List: layout.List{Axis: layout.Vertical}},
		homeFolderTreeList:         widget.List{List: layout.List{Axis: layout.Vertical}},
		homeContentList:            widget.List{List: layout.List{Axis: layout.Vertical}},
		fileCommandResults:         make(chan fileCommandResult, 1),
		shutdown:                   make(chan struct{}),
	}
	t.Cleanup(runtime.shutdown)
	return app
}

func (r *desktopRuntime) setHomeFolderTestSnapshot(snapshot workspace.HomeFolderSnapshot) {
	r.mu.Lock()
	r.homeFolderWorking = cloneHomeFolderRuntimeSnapshot(snapshot)
	r.homeFolderCommitted = cloneHomeFolderRuntimeSnapshot(snapshot)
	r.homeFolderRevision = 0
	r.homeFolderCommittedRev = 0
	r.homeFolderCompletedRev = 0
	r.homeFolderPendingRemove = nil
	r.homeFolderLoaded = true
	r.homeFolderLoadErr = nil
	r.mu.Unlock()
}

func osWriteFile(path string) error {
	return os.WriteFile(path, []byte("test"), 0o600)
}
