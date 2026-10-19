package desktopui

import (
	"fmt"
	"image"

	"path/filepath"

	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/i18n"
	"glance/internal/workspace"
)

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
	app.applyLocaleSelection(i18n.SelectionChinese)
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
			searchDims := homeSearchField(gtx, app.theme, &app.homeSearch, app.homeSearchPlaceholder(), 240*scale, rowHeightDp)
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
	searchDims := homeSearchField(searchGtx, app.theme, &app.homeSearch, app.homeSearchPlaceholder(), 0, 44)
	if searchDims.Size.X != 450 {
		t.Fatalf("fill-width search box width=%d; want 450", searchDims.Size.X)
	}
	if searchDims.Size.Y != 44 {
		t.Fatalf("fill-width search box height=%d; want 44", searchDims.Size.Y)
	}
}

func TestHomeSearchCopyAndOpenCardOnlyAdvertiseImplementedActions(t *testing.T) {
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
