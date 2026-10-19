package desktopui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	"glance/internal/i18n"
	"glance/internal/workspace"
)

func TestHomeRecentFloatingMenuBoundsAlignFlipAndClamp(t *testing.T) {
	viewport := image.Rect(0, 0, 800, 600)
	size := image.Pt(218, 213)
	for _, test := range []struct {
		name   string
		anchor image.Rectangle
		want   image.Rectangle
	}{
		{
			name:   "below and right aligned",
			anchor: image.Rect(560, 100, 584, 124),
			want:   image.Rect(366, 132, 584, 345),
		},
		{
			name:   "flips above near bottom",
			anchor: image.Rect(560, 540, 584, 564),
			want:   image.Rect(366, 319, 584, 532),
		},
		{
			name:   "clamps to left viewport edge",
			anchor: image.Rect(5, 100, 29, 124),
			want:   image.Rect(0, 132, 218, 345),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := homeRecentFloatingMenuBounds(test.anchor, viewport, size, 8); got != test.want {
				t.Fatalf("menu bounds=%v; want %v", got, test.want)
			}
		})
	}
}

func TestHomeRecentMenuSelectionWraps(t *testing.T) {
	if got := homeRecentMenuNextSelection(-1, -1); got != 4 {
		t.Fatalf("Up before selecting an item moved to %d; want last item 4", got)
	}
	if got := homeRecentMenuNextSelection(-1, 1); got != 0 {
		t.Fatalf("Down before selecting an item moved to %d; want first item 0", got)
	}
	if got := homeRecentMenuNextSelection(0, -1); got != 4 {
		t.Fatalf("Up from first menu item selected %d; want last item 4", got)
	}
	if got := homeRecentMenuNextSelection(4, 1); got != 0 {
		t.Fatalf("Down from last menu item selected %d; want first item 0", got)
	}
	if got := homeRecentMenuNextSelection(2, 1); got != 3 {
		t.Fatalf("Down from item 2 selected %d; want item 3", got)
	}
}

func TestHomeRecentMenuOnlyKeepsOneRecentMenuOpen(t *testing.T) {
	app := New(nil, "")
	t.Cleanup(func() { close(app.shutdown) })
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	app.homeMoreButtons = map[string]*widget.Clickable{"first": new(widget.Clickable), "second": new(widget.Clickable)}
	app.toggleHomeRecentMenu("first", gtx)
	app.toggleHomeRecentMenu("second", gtx)
	if app.homeRecentMenuPath != "second" {
		t.Fatalf("switching row menus left %q open; want second", app.homeRecentMenuPath)
	}
	app.toggleHomeRecentMenu("second", gtx)
	if app.homeRecentMenuPath != "" {
		t.Fatalf("clicking the active ellipsis did not close its menu: %q", app.homeRecentMenuPath)
	}
}

func TestHomeRecentMenuActionsMatchPen(t *testing.T) {
	want := []struct{ id, label, icon string }{
		{"open", "打开", "folder-open"},
		{"open-folder", "打开文件夹", "folder"},
		{"copy-path", "复制路径", "link"},
		{"copy-name", "复制文件名", "copy"},
		{"remove", "移除", "list-minus"},
	}
	if len(homeRecentMenuActions) != len(want) {
		t.Fatalf("menu has %d actions; want %d", len(homeRecentMenuActions), len(want))
	}
	for index, expected := range want {
		got := homeRecentMenuActions[index]
		if got.id != expected.id || got.label != expected.label || got.icon != expected.icon {
			t.Errorf("action %d = (%q, %q, %q); want (%q, %q, %q)", index, got.id, got.label, got.icon, expected.id, expected.label, expected.icon)
		}
		if _, ok := svgIconTintAtSize("home", got.icon, 32, color.NRGBA{R: 55, G: 63, B: 72, A: 255}); !ok {
			t.Errorf("action %q has no renderable home SVG icon %q", got.id, got.icon)
		}
	}
}

func TestHomeRecentEllipsisOpensMenuWithoutOpeningDocument(t *testing.T) {
	app := New(nil, "")
	t.Cleanup(func() { close(app.shutdown) })
	app.openResults = nil
	item := workspace.Recent{Path: filepath.Join(t.TempDir(), "recent.xlsx"), Kind: "xlsx", OpenedAt: time.Now()}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(900, 56))}
	app.layoutHomeRecentRow(gtx, item, 0)
	app.homeMoreButtons[item.Path].Click()
	app.layoutHomeRecentRow(gtx, item, 0)
	if app.homeRecentMenuPath != item.Path {
		t.Fatalf("ellipsis click opened menu for %q; want %q", app.homeRecentMenuPath, item.Path)
	}
	if app.opening {
		t.Fatal("ellipsis click also opened the document row")
	}
}

func TestHomeRecentEllipsisPointerOpensAtTrailingAnchor(t *testing.T) {
	app, item := clickRecentRow(t, f32.Pt(869, 28))
	if app.homeRecentMenuPath != item.Path {
		t.Fatalf("pointer click at the trailing ellipsis did not open its menu: %q", app.homeRecentMenuPath)
	}
	if app.opening {
		t.Fatal("pointer click on the ellipsis opened the document")
	}
	if got, want := app.homeRecentMenuAnchorBounds, image.Rect(860, 19, 878, 37); got != want {
		t.Fatalf("ellipsis anchor=%v; want %v", got, want)
	}
}

func TestHomeRecentMenuCopiesAbsolutePathAndFilenameWithExtension(t *testing.T) {
	path := filepath.Join("relative folder", "报告 final.docx")
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := homeRecentMenuCopyValue("copy-path", path); err != nil || got != absolute {
		t.Fatalf("copy path=(%q,%v); want (%q,nil)", got, err, absolute)
	}
	if got, err := homeRecentMenuCopyValue("copy-name", path); err != nil || got != "报告 final.docx" {
		t.Fatalf("copy filename=(%q,%v); want (报告 final.docx,nil)", got, err)
	}
}

func TestHomeRecentMenuEscapeAndEnterKeyboardActions(t *testing.T) {
	app := New(nil, "")
	t.Cleanup(func() { close(app.shutdown) })
	app.applyLocaleSelection(i18n.SelectionChinese)
	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 400))}
		app.layoutHomeRecentMenuDismissInput(gtx)
		app.layoutHomeRecentFloatingMenu(gtx)
		router.Frame(&ops)
	}
	path := filepath.Join(t.TempDir(), "报告 final.docx")
	app.homeRecentMenuPath = path
	app.homeRecentMenuAnchorBounds = image.Rect(300, 100, 324, 124)
	router.Source().Execute(key.FocusCmd{Tag: &app.homeRecentMenuDismiss})
	draw()
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	draw()
	if app.homeRecentMenuIndex != 1 {
		t.Fatalf("Down moved selection to %d; want 1", app.homeRecentMenuIndex)
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	draw()
	if app.homeRecentMenuPath != "" {
		t.Fatal("Escape did not close the file menu")
	}

	app.homeRecentMenuPath = path
	app.homeRecentMenuAnchorBounds = image.Rect(300, 100, 324, 124)
	app.homeRecentMenuIndex = 3
	router.Source().Execute(key.FocusCmd{Tag: &app.homeRecentMenuDismiss})
	draw()
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	draw()
	if app.statusText() != "文件名已复制" || app.homeRecentMenuPath != "" {
		t.Fatalf("Enter action status=%q menu=%q; want copied filename and a closed menu", app.statusText(), app.homeRecentMenuPath)
	}
}

func TestHomeRecentMenuOutsideClickPassesThrough(t *testing.T) {
	app := New(nil, "")
	t.Cleanup(func() { close(app.shutdown) })
	var router input.Router
	var ops op.Ops
	behind := new(widget.Clickable)
	draw := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 400))}
		transform := op.Offset(image.Pt(400, 80)).Push(gtx.Ops)
		behind.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(80, 40)} })
		transform.Pop()
		app.layoutHomeRecentMenuDismissInput(gtx)
		app.layoutHomeRecentFloatingMenu(gtx)
		router.Frame(&ops)
	}
	app.homeRecentMenuPath = "/tmp/report.xlsx"
	app.homeRecentMenuAnchorBounds = image.Rect(280, 100, 304, 124)
	draw()
	point := f32.Pt(430, 100)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 2, Position: point, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 2, Position: point})
	draw()
	draw()
	if app.homeRecentMenuPath != "" {
		t.Fatal("outside click did not dismiss the file menu")
	}
	if len(behind.History()) == 0 {
		t.Fatal("outside menu dismissal blocked the control underneath")
	}
}

func TestHomeRecentFolderActionUsesDirectRevealCommandAndRetriesAfterFailure(t *testing.T) {
	app := newHomeFolderTestApp(t)
	path := filepath.Join(t.TempDir(), "report.xlsx")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.homeRecentMenuPath = path
	app.fileCommandRunner = func(kind fileCommandKind, target string) fileCommandResult {
		if kind != commandRevealFile || target != path {
			t.Fatalf("runner got command=%d target=%q; want direct reveal of %q", kind, target, path)
		}
		return fileCommandResult{path: target, err: os.ErrPermission}
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 400))}
	app.executeHomeRecentMenuAction(gtx, 1)
	select {
	case result := <-app.fileCommandResults:
		if result.kind != commandRevealFile || result.path != path || result.err == nil {
			t.Fatalf("reveal result=%+v; want direct reveal and a surfaced failure", result)
		}
		app.fileCommandResults <- result
	case <-time.After(5 * time.Second):
		t.Fatal("reveal command did not complete")
	}
	app.processAsyncResults()
	if app.homeRecentMenuPath != path || app.statusRef.ID != "file.command.reveal_failed" || !strings.Contains(app.feedback.detail, "permission") {
		t.Fatalf("failed reveal closed menu or hid reason: menu=%q status=%q detail=%q", app.homeRecentMenuPath, app.statusText(), app.feedback.detail)
	}
}

func TestHomeRecentRemovalCanUndoWithoutDeletingTheFile(t *testing.T) {
	app := newHomeFolderTestApp(t)
	path := filepath.Join(t.TempDir(), "keep.docx")
	if err := os.WriteFile(path, []byte("still here"), 0o600); err != nil {
		t.Fatal(err)
	}
	opened := time.Date(2026, 10, 13, 11, 10, 0, 0, time.UTC)
	original := []workspace.Recent{
		{Path: path, Kind: "docx", OpenedAt: opened},
		{Path: "/tmp/other.xlsx", Kind: "xlsx", OpenedAt: opened.Add(time.Minute)},
	}
	app.collection.StageRecentsSnapshot(original)
	app.homeRecentMenuPath = path
	app.removeHomeRecent(path)
	if got := app.collection.Recents(); len(got) != 1 || got[0].Path != original[1].Path {
		t.Fatalf("remove staged recents=%+v; want only the other file", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("remove deleted source file: %v", err)
	}
	result := awaitHomeRecentWrite(t, app.homeRecentWriteResults)
	app.handleHomeRecentWriteResult(result)
	if app.feedback.actionLabel != "撤销" {
		t.Fatalf("successful remove action label=%q; want 撤销", app.feedback.actionLabel)
	}
	app.feedback.action.Click()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 200)), Now: time.Now()}
	app.layoutFeedback(gtx)
	if got := app.collection.Recents(); len(got) != 2 || got[0] != original[0] || got[1] != original[1] {
		t.Fatalf("Undo did not restore exact recent order and timestamps: %+v", got)
	}
	result = awaitHomeRecentWrite(t, app.homeRecentWriteResults)
	app.handleHomeRecentWriteResult(result)
	if got := app.collection.Recents(); len(got) != 2 || app.feedback.actionLabel != "" {
		t.Fatalf("successful Undo left wrong state: recents=%+v action=%q", got, app.feedback.actionLabel)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Undo changed source file: %v", err)
	}
}

func TestHomeRecentRemovalFailureRestoresRowAndReopensMenu(t *testing.T) {
	app := newHomeFolderTestApp(t)
	path := filepath.Join(t.TempDir(), "keep.xlsx")
	if err := os.WriteFile(path, []byte("still here"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := []workspace.Recent{{Path: path, Kind: "xlsx", OpenedAt: time.Now()}}
	app.collection.StageRecentsSnapshot(original)
	if err := app.workspaceStore.Close(); err != nil {
		t.Fatal(err)
	}
	app.homeRecentMenuPath = path
	app.removeHomeRecent(path)
	result := awaitHomeRecentWriteFailure(t, app.homeRecentWriteResults)
	app.handleHomeRecentWriteResult(result)
	if got := app.collection.Recents(); len(got) != 1 || got[0] != original[0] {
		t.Fatalf("failed removal did not restore recent row: %+v", got)
	}
	if app.homeRecentMenuPath != path || !strings.Contains(app.statusText(), "从最近列表移除失败") {
		t.Fatalf("failed removal did not leave a retry path: menu=%q status=%q", app.homeRecentMenuPath, app.statusText())
	}
}

func TestHomeRecentUndoFailureKeepsRemovedStateAndAllowsRetry(t *testing.T) {
	app := newHomeFolderTestApp(t)
	path := filepath.Join(t.TempDir(), "keep-for-undo.xlsx")
	if err := os.WriteFile(path, []byte("still here"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := []workspace.Recent{{Path: path, Kind: "xlsx", OpenedAt: time.Now()}}
	app.collection.StageRecentsSnapshot(original)
	app.removeHomeRecent(path)
	result := awaitHomeRecentWrite(t, app.homeRecentWriteResults)
	app.handleHomeRecentWriteResult(result)
	if app.feedback.actionLabel != "撤销" {
		t.Fatalf("remove success action=%q; want 撤销", app.feedback.actionLabel)
	}
	if err := app.workspaceStore.Close(); err != nil {
		t.Fatal(err)
	}
	app.undoHomeRecentRemoval()
	result = awaitHomeRecentWriteFailure(t, app.homeRecentWriteResults)
	app.handleHomeRecentWriteResult(result)
	if got := app.collection.Recents(); len(got) != 0 {
		t.Fatalf("failed Undo should restore the removed-list state, got %+v", got)
	}
	if app.feedback.actionLabel != "重试" {
		t.Fatalf("failed Undo action=%q; want retry", app.feedback.actionLabel)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("failed Undo changed source file: %v", err)
	}
}

func awaitHomeRecentWrite(t *testing.T, results <-chan homeRecentWriteResult) homeRecentWriteResult {
	t.Helper()
	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("save recent list: %v", result.err)
		}
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for recent-list persistence")
		return homeRecentWriteResult{}
	}
}

func awaitHomeRecentWriteFailure(t *testing.T, results <-chan homeRecentWriteResult) homeRecentWriteResult {
	t.Helper()
	select {
	case result := <-results:
		if result.err == nil {
			t.Fatal("closed workspace store unexpectedly saved recents")
		}
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for failed recent-list persistence")
		return homeRecentWriteResult{}
	}
}

func TestHomeRecentFloatingMenuRendersAllPenActionsAtOneAndTwoX(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		t.Run(map[float32]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			width, height := int(homeRecentMenuWidth*scale), int(homeRecentMenuHeight*scale)
			window, err := headless.NewWindow(width, height)
			if err != nil {
				t.Skipf("headless GPU is unavailable: %v", err)
			}
			defer window.Release()
			app := &App{theme: material.NewTheme(), homeRecentMenuPath: "/tmp/long final report.docx", homeRecentMenuIndex: -1, homeRecentMenuAnchorBounds: image.Rect(width-36, 24, width-12, 48)}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(width, height))}
			app.layoutHomeRecentMenuDismissInput(gtx)
			if dims := app.layoutHomeRecentFloatingMenu(gtx); dims.Size != image.Pt(width, height) {
				t.Fatalf("menu size=%v; want %dx%d", dims.Size, width, height)
			}
			if err := window.Frame(&ops); err != nil {
				t.Fatalf("render recent menu: %v", err)
			}
			screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
			if err := window.Screenshot(screenshot); err != nil {
				t.Fatalf("capture recent menu: %v", err)
			}
			for index, action := range homeRecentMenuActions {
				rowTop := homeRecentMenuRowTop(index, scale)
				iconInk, textInk := 0, 0
				iconFirst, iconLast := height, -1
				textFirst, textLast := height, -1
				for y := rowTop; y < rowTop+int(float32(homeRecentMenuItemHeight)*scale); y++ {
					for x := int(8 * scale); x < width-int(8*scale); x++ {
						pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
						if pixel.A < 180 {
							continue
						}
						ink := pixel.R < 175 && pixel.G < 185 && pixel.B < 200
						if action.remove {
							ink = pixel.R > 120 && pixel.G < 230 && pixel.B < 230
						}
						if !ink {
							continue
						}
						if x < int(40*scale) {
							iconInk++
							iconFirst = min(iconFirst, y)
							iconLast = max(iconLast, y)
						} else {
							textInk++
							textFirst = min(textFirst, y)
							textLast = max(textLast, y)
						}
					}
				}
				if iconInk == 0 || textInk == 0 {
					t.Errorf("menu action %q has no visible icon/text pixels: icon=%d text=%d", action.label, iconInk, textInk)
					continue
				}
				iconCenter := float32(iconFirst+iconLast) / 2
				textCenter := float32(textFirst+textLast) / 2
				delta := iconCenter - textCenter
				if delta < 0 {
					delta = -delta
				}
				if delta > 2*scale {
					t.Errorf("menu action %q icon/text ink centers differ by %.1fpx at %.1fx", action.label, iconCenter-textCenter, scale)
				}
			}
		})
	}
}

func TestHomeRecentFloatingMenuLayoutUsesExactSizeAtOneAndTwoX(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		width, height := int(homeRecentMenuWidth*scale), int(homeRecentMenuHeight*scale)
		app := &App{theme: material.NewTheme(), homeRecentMenuPath: "/tmp/report.xlsx", homeRecentMenuIndex: -1, homeRecentMenuAnchorBounds: image.Rect(width-36, 24, width-12, 48)}
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(width, height))}
		dims := app.layoutHomeRecentFloatingMenu(gtx)
		if dims.Size != image.Pt(width, height) {
			t.Errorf("at %.1fx menu size=%v; want %dx%d", scale, dims.Size, width, height)
		}
		if got := app.homeRecentMenuBounds; got != image.Rect(0, 0, width, height) {
			t.Errorf("at %.1fx menu bounds=%v; want viewport bounds", scale, got)
		}
	}
}

func TestHomeRecentMenuIconsHaveVisiblePixelsAtOneAndTwoX(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, action := range homeRecentMenuActions {
			pixels := 0
			icon, ok := svgIconTintAtSize("home", action.icon, int(16*scale), homeRecentMenuActionColor(action))
			if !ok {
				t.Fatalf("missing %q icon at %.1fx", action.icon, scale)
			}
			for y := 0; y < icon.Bounds().Dy(); y++ {
				for x := 0; x < icon.Bounds().Dx(); x++ {
					if _, _, _, alpha := icon.At(x, y).RGBA(); alpha != 0 {
						pixels++
					}
				}
			}
			if pixels == 0 {
				t.Errorf("%q icon has no visible raster pixels at %.1fx", action.icon, scale)
			}
		}
	}
}

func homeRecentMenuRowTop(index int, scale float32) int {
	top := 6
	if index < 4 {
		top += index * 40
	} else {
		top += 4*40 + 5
	}
	return int(float32(top) * scale)
}
