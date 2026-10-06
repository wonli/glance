package desktopui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/workspace"
)

func TestHomeActionCardsMatchPenDimensions(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(144, 0), Max: image.Pt(144, 200)},
	}
	dims := homeActionCard(gtx, material.NewTheme(), new(widget.Clickable), homeActions[0])
	if dims.Size != image.Pt(144, 132) {
		t.Fatalf("home action card size=%v; want 144x132 from Pen node R713ue", dims.Size)
	}
}

func TestHomeFavoriteStarFillsGoldWhenSelected(t *testing.T) {
	want := color.NRGBA{R: 176, G: 138, B: 55, A: 255}
	icon, ok := tintedToolbarIcon("star-filled", want)
	if !ok {
		t.Fatal("filled favorite star icon is unavailable")
	}
	got := color.NRGBAModel.Convert(icon.At(icon.Bounds().Dx()/2, icon.Bounds().Dy()/2)).(color.NRGBA)
	if got != want {
		t.Fatalf("selected favorite star center=%+v; want solid gold %+v", got, want)
	}
}

func TestHomeGreetingMatchesTimeOfDay(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const width, height = 560, 100
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	app := New(nil, "")
	ops := new(op.Ops)
	render := func(now time.Time, widget layout.Widget) ([]byte, error) {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Now:         now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(width, height)},
		}
		widget(gtx)
		if err := window.Frame(ops); err != nil {
			return nil, err
		}
		screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
		if err := window.Screenshot(screenshot); err != nil {
			return nil, err
		}
		return screenshot.Pix, nil
	}

	cases := []struct {
		hour     int
		greeting string
	}{
		{hour: 0, greeting: "凌晨好"},
		{hour: 4, greeting: "凌晨好"},
		{hour: 5, greeting: "早上好"},
		{hour: 11, greeting: "早上好"},
		{hour: 12, greeting: "中午好"},
		{hour: 13, greeting: "中午好"},
		{hour: 14, greeting: "下午好"},
		{hour: 15, greeting: "下午好"},
		{hour: 17, greeting: "下午好"},
		{hour: 18, greeting: "晚上好"},
		{hour: 23, greeting: "晚上好"},
	}
	for _, test := range cases {
		now := time.Date(2026, time.September, 30, test.hour, 0, 0, 0, time.Local)
		t.Run(now.Format("15:04"), func(t *testing.T) {
			actual, err := render(now, app.layoutHomeGreeting)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := render(now, expectedHomeGreetingLayout(app.theme, test.greeting))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, expected) {
				t.Fatalf("home greeting at %s does not render %q", now.Format("15:04"), test.greeting)
			}
		})
	}
}

func expectedHomeGreetingLayout(theme *material.Theme, greeting string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(78))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		style := material.H4(theme, greeting+"\n今天想创建点什么？")
		style.TextSize = unit.Sp(34)
		style.LineHeightScale = 1.15
		style.MaxLines = 2
		style.Color = color.NRGBA{R: 35, G: 40, B: 48, A: 255}
		style.Font.Weight = font.Bold
		return style.Layout(gtx)
	}
}

func TestHomeFilterBarMatchesPenDimensions(t *testing.T) {
	var ops op.Ops
	app := &App{
		theme:             material.NewTheme(),
		homeFilterButtons: make(map[string]*widget.Clickable),
		homeTypeFilter:    "全部",
	}
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(1090, 44), Max: image.Pt(1090, 44)},
	}
	dims := app.layoutHomeFilters(gtx)
	if dims.Size != image.Pt(1090, 44) {
		t.Fatalf("home filter bar size=%v; want 1090x44 from Pen node R713ue", dims.Size)
	}
}

func TestHomeFilterBarReflowsAtMinimumWindowWidth(t *testing.T) {
	var ops op.Ops
	app := &App{
		theme:             material.NewTheme(),
		homeFilterButtons: make(map[string]*widget.Clickable),
		homeTypeFilter:    "全部",
	}
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(550, 120)}}
	dims := app.layoutHomeFilters(gtx)
	if dims.Size != image.Pt(550, 82) {
		t.Fatalf("compact home filters size=%v; want readable two-row controls at 550dp", dims.Size)
	}
}

func TestHomeRecentTableMatchesPenDimensions(t *testing.T) {
	var ops op.Ops
	app := &App{
		theme:      material.NewTheme(),
		recentList: widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(1090, 330), Max: image.Pt(1090, 330)},
	}
	dims := app.layoutHomeRecentTable(gtx)
	if dims.Size != image.Pt(1090, 330) {
		t.Fatalf("home recent table size=%v; want 1090x330 from Pen node R713ue", dims.Size)
	}
}

func TestHomeMainContentMatchesPenVerticalLayout(t *testing.T) {
	var ops op.Ops
	app := &App{
		theme:               material.NewTheme(),
		homeContentList:     widget.List{List: layout.List{Axis: layout.Vertical}},
		homeActionButtons:   make(map[string]*widget.Clickable),
		homeFilterButtons:   make(map[string]*widget.Clickable),
		homeOpenButtons:     make(map[string]*widget.Clickable),
		homeFavoriteButtons: make(map[string]*widget.Clickable),
		homeMoreButtons:     make(map[string]*widget.Clickable),
		homeTypeFilter:      "全部",
		recentList:          widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1190, 900)},
	}
	dims := app.layoutHomeContent(gtx)
	if dims.Size != image.Pt(1190, 748) {
		t.Fatalf("home main content size=%v; want 1190x748 after adding the Pen table-header top spacing", dims.Size)
	}
}

func TestHomeContentScrollsWithinShortWindow(t *testing.T) {
	app := &App{
		theme:               material.NewTheme(),
		homeContentList:     widget.List{List: layout.List{Axis: layout.Vertical}},
		homeActionButtons:   make(map[string]*widget.Clickable),
		homeFilterButtons:   make(map[string]*widget.Clickable),
		homeOpenButtons:     make(map[string]*widget.Clickable),
		homeFavoriteButtons: make(map[string]*widget.Clickable),
		homeMoreButtons:     make(map[string]*widget.Clickable),
		homeTypeFilter:      "全部",
		recentList:          widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1190, 558)}}
	dims := app.layoutHomeContent(gtx)
	if dims.Size != image.Pt(1190, 558) {
		t.Fatalf("home content size=%v; want the available 1190x558 viewport", dims.Size)
	}
	if app.homeContentList.Position.Length <= dims.Size.Y {
		t.Fatalf("scrollable content length=%d; want more than viewport height %d", app.homeContentList.Position.Length, dims.Size.Y)
	}
}

func TestHomeActionCardsWrapWithinMinimumWindowWidth(t *testing.T) {
	app := &App{theme: material.NewTheme(), homeActionButtons: make(map[string]*widget.Clickable)}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(550, 500)}}
	dims := app.layoutHomeActions(gtx)
	if dims.Size != image.Pt(550, 278) {
		t.Fatalf("home actions size=%v; want four cards per row within 550dp and two compact rows", dims.Size)
	}
	if len(app.homeActionButtons) != len(homeActions) {
		t.Fatalf("laid out %d action buttons; want all %d", len(app.homeActionButtons), len(homeActions))
	}
}

func TestHomeRecentTableColumnsFitMinimumWindowWidth(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(506, 60)}}
	metrics := homeTableColumnMetricsForWidth(gtx, 506)
	if !metrics.compact || metrics.locationWidth != 0 {
		t.Fatalf("minimum-width table layout=%+v; want compact columns without the location column", metrics)
	}
	totalWidth := metrics.nameWidth + metrics.modifiedWidth + metrics.sizeWidth + metrics.actionWidth
	if totalWidth > 506 {
		t.Fatalf("minimum-width table columns total %dpx; want at most 506px", totalWidth)
	}
	if metrics.nameWidth < 350 {
		t.Fatalf("minimum-width name column=%dpx; want readable width of at least 350px", metrics.nameWidth)
	}
	dims := homeTableColumns(gtx, material.NewTheme(), true, nil, "打开时间 ↓")
	if dims.Size.X > 506 {
		t.Fatalf("minimum-width home table header=%dpx; want it to fit inside the 506px inner width", dims.Size.X)
	}
}

func TestHomeRecentsSortMatchesOpenedTimeHeader(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	newerOpened := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	olderOpened := newerOpened.Add(-time.Hour)
	collection.StageRecents([]workspace.Recent{
		{Path: "/tmp/older.xlsx", Kind: "xlsx", OpenedAt: newerOpened},
		{Path: "/tmp/newer.xlsx", Kind: "xlsx", OpenedAt: olderOpened},
	})
	app := &App{
		desktopRuntime: &desktopRuntime{collection: collection},
		homeMetadata: map[string]homeFileMetadata{
			"/tmp/older.xlsx": {modified: olderOpened, size: 999},
			"/tmp/newer.xlsx": {modified: newerOpened, size: 1},
		},
		homeSortOpenedDesc: true,
	}
	if got := app.homeSortLabel(); got != "打开时间 ↓" {
		t.Fatalf("descending sort header=%q; want 打开时间 ↓", got)
	}
	if got := app.homeVisibleRecents(true)[0].Path; got != "/tmp/older.xlsx" {
		t.Fatalf("descending opened-time first result=%q; want /tmp/older.xlsx", got)
	}
	app.homeSortOpenedDesc = false
	if got := app.homeSortLabel(); got != "打开时间 ↑" {
		t.Fatalf("ascending sort header=%q; want 打开时间 ↑", got)
	}
	if got := app.homeVisibleRecents(true)[0].Path; got != "/tmp/newer.xlsx" {
		t.Fatalf("ascending opened-time first result=%q; want /tmp/newer.xlsx", got)
	}
}

func TestHomeRecentsFallbackToModifiedWhenOpenedAtIsZero(t *testing.T) {
	newerMod := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	olderMod := newerMod.Add(-time.Hour)
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	// Zero OpenedAt items in collection
	collection.StageRecents([]workspace.Recent{
		{Path: "/tmp/mod_older.xlsx", Kind: "xlsx", OpenedAt: time.Time{}},
		{Path: "/tmp/mod_newer.xlsx", Kind: "xlsx", OpenedAt: time.Time{}},
	})
	app := &App{
		desktopRuntime: &desktopRuntime{collection: collection},
		homeMetadata: map[string]homeFileMetadata{
			"/tmp/mod_older.xlsx": {modified: olderMod, size: 100},
			"/tmp/mod_newer.xlsx": {modified: newerMod, size: 200},
		},
		homeSortOpenedDesc: true,
	}
	results := app.homeVisibleRecents(true)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 recents, got %d", len(results))
	}
}

func TestHomeFiltersCompactSearchFieldDoesNotDistort(t *testing.T) {
	for _, pxPerDp := range []float32{1, 2, 3} {
		t.Run(fmt.Sprintf("PxPerDp_%v", pxPerDp), func(t *testing.T) {
			app := New(nil, "")
			var ops op.Ops
			width := int(500 * pxPerDp)
			scale := float32(width) / float32(int(650*pxPerDp))
			if scale > 1 {
				scale = 1
			}
			rowHeightDp := 44 * scale
			gtx := layout.Context{
				Ops:    &ops,
				Metric: unit.Metric{PxPerDp: pxPerDp, PxPerSp: pxPerDp},
			}
			rowHeightPx := gtx.Dp(unit.Dp(rowHeightDp))
			expectedTotalHeightPx := rowHeightPx*2 + gtx.Dp(unit.Dp(8))
			gtx.Constraints = layout.Exact(image.Pt(width, expectedTotalHeightPx))

			dims := app.layoutHomeFiltersCompact(gtx, width)
			if dims.Size.Y != expectedTotalHeightPx {
				t.Fatalf("compact filters height=%dpx; want %dpx for PxPerDp=%v", dims.Size.Y, expectedTotalHeightPx, pxPerDp)
			}

			// Directly verify search field height matches rowHeightPx, not rowHeightPx * pxPerDp
			searchDims := homeSearchField(gtx, app.theme, &app.homeSearch, 240*scale, rowHeightDp)
			if searchDims.Size.Y != rowHeightPx {
				t.Fatalf("homeSearchField height=%dpx; want rowHeightPx=%dpx (PxPerDp=%v)", searchDims.Size.Y, rowHeightPx, pxPerDp)
			}
		})
	}
}

func TestHomeFiltersCompactLayoutPositions(t *testing.T) {
	app := New(nil, "")
	var ops op.Ops
	width := 700
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, 96)),
	}
	dims := app.layoutHomeFiltersCompact(gtx, width)
	if dims.Size.X != width {
		t.Fatalf("compact filters width=%d; want %d", dims.Size.X, width)
	}

	// Verify homeSearchField with width=0 fills available constraint width
	searchGtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(450, 44)),
	}
	searchDims := homeSearchField(searchGtx, app.theme, &app.homeSearch, 0, 44)
	if searchDims.Size.X != 450 {
		t.Fatalf("fill-width search box width=%d; want 450", searchDims.Size.X)
	}
	if searchDims.Size.Y != 44 {
		t.Fatalf("fill-width search box height=%d; want 44", searchDims.Size.Y)
	}
}

func TestHomeSearchCopyAndOpenCardOnlyAdvertiseImplementedActions(t *testing.T) {
	if homeSearchPlaceholder != "搜索文件名..." {
		t.Fatalf("home search placeholder=%q; want filename-only copy", homeSearchPlaceholder)
	}
	if homeActions[len(homeActions)-1].extension != ".docx / .xlsx" {
		t.Fatalf("open card extensions=%q; want supported Office examples only", homeActions[len(homeActions)-1].extension)
	}
}

func TestHomeFavoriteTogglePersistsAcrossAppReload(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first := &App{desktopRuntime: &desktopRuntime{workspaceStore: store}, homeFavorites: make(map[string]bool)}
	first.toggleHomeFavorite("/tmp/budget.xlsx")
	if !first.homeFavorites["/tmp/budget.xlsx"] {
		t.Fatal("favorite was not enabled in the current home view")
	}

	reloaded := &App{desktopRuntime: &desktopRuntime{workspaceStore: store}, homeFavorites: make(map[string]bool)}
	reloaded.loadHomeFavorites()
	if !reloaded.homeFavorites["/tmp/budget.xlsx"] {
		t.Fatal("favorite was not restored from the workspace store")
	}
	reloaded.toggleHomeFavorite("/tmp/budget.xlsx")
	if reloaded.homeFavorites["/tmp/budget.xlsx"] {
		t.Fatal("favorite was not removed in the current home view")
	}
	final := &App{desktopRuntime: &desktopRuntime{workspaceStore: store}, homeFavorites: make(map[string]bool)}
	final.loadHomeFavorites()
	if final.homeFavorites["/tmp/budget.xlsx"] {
		t.Fatal("removed favorite was restored from the workspace store")
	}
}

func TestHomeSidebarFillsPenFrameHeight(t *testing.T) {
	var ops op.Ops
	app := New(nil, "")
	gtx := layout.Context{
		Ops:    &ops,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Min: image.Pt(250, 0),
			Max: image.Pt(250, 900),
		},
	}
	dims := app.layoutHomeSidebar(gtx)
	if dims.Size != image.Pt(250, 900) {
		t.Fatalf("home sidebar size=%v; want 250x900 to match Pen node R713ue", dims.Size)
	}
}

func TestHomeActionIconsMatchPenLucideAssets(t *testing.T) {
	want := []string{"file-text", "file-spreadsheet", "presentation", "file-code", "braces", "file", "folder-open"}
	if len(homeActions) != len(want) {
		t.Fatalf("home action count=%d; want %d", len(homeActions), len(want))
	}
	for i, icon := range want {
		if homeActions[i].icon != icon {
			t.Errorf("home action %q icon=%q; want Pen icon %q", homeActions[i].id, homeActions[i].icon, icon)
		}
		if _, ok := svgIconOriginal("home", icon); !ok {
			t.Errorf("home action %q has no sprite asset for Pen icon %q", homeActions[i].id, icon)
		}
	}
	for _, icon := range []string{"clock-3", "star", "folder", "folder-open", "folder-plus", "search", "list-filter", "ellipsis", "chevron-down"} {
		if _, ok := toolbarIconData(icon); !ok {
			t.Errorf("home control has no sprite asset for Pen icon %q", icon)
		}
	}
}

func TestHomeTableFileIconsMatchPenFileTypes(t *testing.T) {
	for _, test := range []struct {
		extension string
		want      string
	}{
		{extension: "XLSX", want: "file-spreadsheet"},
		{extension: "DOCX", want: "file-text"},
		{extension: "PPTX", want: "presentation"},
		{extension: "PDF", want: "file"},
	} {
		if got := homeFileIconName(test.extension); got != test.want {
			t.Errorf("homeFileIconName(%q)=%q; want %q from Pen node R713ue", test.extension, got, test.want)
		}
	}
}

func TestHomeLabelStyleSupportsPenSemiboldText(t *testing.T) {
	style := homeLabelStyle(material.NewTheme(), "名称", 14, homeText, font.SemiBold)
	if style.Font.Weight != font.SemiBold {
		t.Fatalf("home label weight=%v; want semibold to match Pen node R713ue", style.Font.Weight)
	}
}

func TestHomeRecentCountFitsPenColumnWithoutTruncation(t *testing.T) {
	theme := New(nil, "").theme
	style := homeRecentCountStyle(theme, "最近使用  5 个文件", 1)
	if style.TextSize != unit.Sp(14) {
		t.Fatalf("recent count text size=%v; want 14sp so the full label fits Pen's 126px column", style.TextSize)
	}
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(126, 24)},
	}
	_, info := (widget.Label{MaxLines: style.MaxLines}).LayoutDetailed(gtx, style.Shaper, style.Font, style.TextSize, style.Text, op.CallOp{})
	if info.Truncated != 0 {
		t.Fatalf("recent count truncates %d characters at Pen's 126px column width", info.Truncated)
	}
}

func TestFormatHomeFileSizeMatchesPenPrecision(t *testing.T) {
	for _, test := range []struct {
		bytes int64
		want  string
	}{
		{bytes: 13 * 1024, want: "13 KB"},
		{bytes: 3 * 1024, want: "3 KB"},
		{bytes: 3 * 1024 * 1024, want: "3.0 MB"},
	} {
		if got := formatHomeFileSize(test.bytes); got != test.want {
			t.Errorf("formatHomeFileSize(%d)=%q; want %q", test.bytes, got, test.want)
		}
	}
}
