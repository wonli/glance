package desktopui

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/workspace"
)

func TestHomeVisualSnapshot(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	root := t.TempDir()
	store, err := workspace.Open(filepath.Join(root, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	fixtures := []struct {
		path     string
		kind     string
		modified time.Time
		size     int64
	}{
		{"office/人事表格-员工绩效表-1.xlsx", "xlsx", now.AddDate(0, 0, -1), 13 * 1024},
		{"office/人事表格-员工绩效表.xlsx", "xlsx", now.AddDate(0, 0, -1), 12 * 1024},
		{"office/大猫OA软件使用说明.docx", "docx", time.Date(now.Year(), time.September, 27, 12, 0, 0, 0, now.Location()), 3 * 1024 * 1024},
		{"GenOffice/未命名文档.docx", "docx", time.Date(now.Year(), time.September, 28, 12, 0, 0, 0, now.Location()), 3 * 1024},
		{"Downloads/百度文心助手 – 办公学习一站解决.pdf", "pdf", time.Date(now.Year(), time.September, 28, 12, 0, 0, 0, now.Location()), 480 * 1024},
	}
	recents := make([]workspace.Recent, 0, len(fixtures))
	app := New(nil, "")
	app.homeMetadata = make(map[string]homeFileMetadata, len(fixtures))
	for _, fixture := range fixtures {
		path := filepath.Join(root, fixture.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(fixture.size); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		recents = append(recents, workspace.Recent{Path: path, Kind: fixture.kind, OpenedAt: fixture.modified})
		app.homeMetadata[path] = homeFileMetadata{modified: fixture.modified, size: fixture.size}
	}
	collection.StageRecents(recents)
	app.collection = collection
	app.homeSection = homeSectionRecent
	app.homeTypeFilter = homeFilterAll

	const width, height = 1440, 900
	ops := new(op.Ops)
	gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(width, height)}}
	dims := app.layoutRecent(gtx)
	if dims.Size != image.Pt(width, height) {
		t.Fatalf("home screenshot layout=%v, want %dx%d", dims.Size, width, height)
	}
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatal(err)
	}
	t.Run("first recent row text is vertically centered", func(t *testing.T) {
		const rowTop, rowBottom = 466, 522
		rowCenter := float64(rowTop+rowBottom-1) / 2
		cells := []struct {
			name string
			area image.Rectangle
		}{
			{name: "filename", area: image.Rect(370, rowTop+1, 800, rowBottom-1)},
			{name: "location", area: image.Rect(822, rowTop+1, 932, rowBottom-1)},
			{name: "modified", area: image.Rect(932, rowTop+1, 1022, rowBottom-1)},
			{name: "size", area: image.Rect(1022, rowTop+1, 1082, rowBottom-1)},
		}
		for _, cell := range cells {
			t.Run(cell.name, func(t *testing.T) {
				inkTop, inkBottom, ok := darkInkVerticalBounds(screenshot, cell.area)
				if !ok {
					t.Fatalf("first recent row %s text has no visible ink", cell.name)
				}
				inkCenter := float64(inkTop+inkBottom) / 2
				if math.Abs(inkCenter-rowCenter) > 2 {
					t.Fatalf("first recent row %s ink center=%.1f, row center=%.1f (ink y=%d..%d)", cell.name, inkCenter, rowCenter, inkTop, inkBottom)
				}
			})
		}
	})
	darkPixelsNearSidebarTop := 0
	for y := 0; y < 100; y++ {
		for x := 0; x < 250; x++ {
			r, g, b, _ := screenshot.At(x, y).RGBA()
			if r < 180*257 && g < 180*257 && b < 180*257 {
				darkPixelsNearSidebarTop++
			}
		}
	}
	if darkPixelsNearSidebarTop == 0 {
		t.Fatal("Pen sidebar brand and navigation should start at the top of its frame")
	}
	path := os.Getenv("GLANCE_HOME_SCREENSHOT")
	if path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, screenshot); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote home screenshot to %s", path)
	}

	const narrowWidth, narrowHeight = 900, 600
	narrowOps := new(op.Ops)
	narrowGtx := layout.Context{Ops: narrowOps, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(narrowWidth, narrowHeight)}}
	narrowDims := app.layoutRecent(narrowGtx)
	if narrowDims.Size != image.Pt(narrowWidth, narrowHeight) {
		t.Fatalf("narrow home layout=%v, want %dx%d", narrowDims.Size, narrowWidth, narrowHeight)
	}
	narrowWindow, err := headless.NewWindow(narrowWidth, narrowHeight)
	if err != nil {
		t.Skipf("headless GPU is unavailable for the minimum-width home: %v", err)
	}
	defer narrowWindow.Release()
	if err := narrowWindow.Frame(narrowOps); err != nil {
		t.Fatal(err)
	}
	narrowScreenshot := image.NewRGBA(image.Rect(0, 0, narrowWidth, narrowHeight))
	if err := narrowWindow.Screenshot(narrowScreenshot); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("GLANCE_HOME_NARROW_SCREENSHOT"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, narrowScreenshot); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote narrow home screenshot to %s", path)
	}

	app.clientTitlebar = true
	app.showNewMenu = true
	const appHeight = height + int(workspaceTabStripSize)
	appOps := new(op.Ops)
	appGtx := layout.Context{Ops: appOps, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(width, appHeight)}}
	appDims := app.layout(appGtx)
	if appDims.Size != image.Pt(width, appHeight) {
		t.Fatalf("home with workspace chrome layout=%v, want %dx%d", appDims.Size, width, appHeight)
	}
	appWindow, err := headless.NewWindow(width, appHeight)
	if err != nil {
		t.Skipf("headless GPU is unavailable for workspace chrome: %v", err)
	}
	defer appWindow.Release()
	if err := appWindow.Frame(appOps); err != nil {
		t.Fatal(err)
	}
	appScreenshot := image.NewRGBA(image.Rect(0, 0, width, appHeight))
	if err := appWindow.Screenshot(appScreenshot); err != nil {
		t.Fatal(err)
	}
	menuPath := os.Getenv("GLANCE_HOME_MENU_SCREENSHOT")
	if menuPath != "" {
		menuFile, err := os.Create(menuPath)
		if err != nil {
			t.Fatal(err)
		}
		defer menuFile.Close()
		if err := png.Encode(menuFile, appScreenshot); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote home and plus-menu screenshot to %s", menuPath)
	}
}

func TestHomeFavoriteNavigationSelectsOnFirstClick(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	app := New(nil, "")
	app.homeNavButtons["favorites"] = new(widget.Clickable)
	app.homeNavButtons["favorites"].Click()
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(900, 600)},
	}
	app.layoutRecent(gtx)
	window, err := headless.NewWindow(900, 600)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, 900, 600))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatal(err)
	}
	if app.homeSection != homeSectionFavorites {
		t.Fatalf("home section after first click=%v; want favorites", app.homeSection)
	}
	assertBackground := func(label string, point image.Point, want color.NRGBA) {
		t.Helper()
		got := color.NRGBAModel.Convert(screenshot.At(point.X, point.Y)).(color.NRGBA)
		if got != want {
			t.Errorf("%s background after first click=%+v; want %+v", label, got, want)
		}
	}
	assertBackground("recent navigation", image.Pt(100, 109), homeSidebarFill)
	assertBackground("favorites navigation", image.Pt(100, 157), homeSelectedFill)
	if path := os.Getenv("GLANCE_HOME_NAV_CLICK_SCREENSHOT"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, screenshot); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote first-click navigation screenshot to %s", path)
	}
}

func darkInkVerticalBounds(src image.Image, area image.Rectangle) (top, bottom int, ok bool) {
	top, bottom = area.Max.Y, area.Min.Y
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			if r >= 190*257 || g >= 190*257 || b >= 190*257 {
				continue
			}
			if y < top {
				top = y
			}
			if y > bottom {
				bottom = y
			}
			ok = true
		}
	}
	return top, bottom, ok
}
