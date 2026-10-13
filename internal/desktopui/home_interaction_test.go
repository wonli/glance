package desktopui

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/workspace"
)

func TestFavoriteClickOnlyTogglesFavorite(t *testing.T) {
	app, item := clickRecentRow(t, f32.Pt(835, 28))
	if !app.homeFavorites[item.Path] {
		t.Fatal("favorite click did not toggle the star")
	}
	if app.opening {
		t.Fatal("favorite click also started opening the document")
	}
}

func TestRecentRowClickStillOpensDocument(t *testing.T) {
	app, _ := clickRecentRow(t, f32.Pt(100, 28))
	if !app.opening {
		t.Fatal("clicking the file row did not start opening the document")
	}
}

func clickRecentRow(t *testing.T, position f32.Point) (*App, workspace.Recent) {
	t.Helper()
	app := New(nil, "")
	t.Cleanup(func() { close(app.shutdown) })
	app.openResults = nil
	item := workspace.Recent{Path: filepath.Join(t.TempDir(), "recent.xlsx")}
	app.homeOpenButtons[item.Path] = new(widget.Clickable)
	app.homeFavoriteButtons[item.Path] = new(widget.Clickable)

	var router input.Router
	var ops op.Ops
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(900, 56)),
		}
		app.layoutHomeRecentRow(gtx, item, 0)
		router.Frame(&ops)
	}
	draw()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: position, Buttons: pointer.ButtonPrimary})
	draw()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	draw()
	return app, item
}
