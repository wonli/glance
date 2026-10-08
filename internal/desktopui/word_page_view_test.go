package desktopui

import (
	"image"
	"reflect"
	"testing"

	"glance/internal/wordlayout"
)

func TestDrawWordPageCellBorderHonorsSceneEdges(t *testing.T) {
	cell := wordlayout.TableCellPlacement{
		Bounds:         wordlayout.Rect{XPT: 10, YPT: 20, WidthPT: 30, HeightPT: 40},
		DrawLeftBorder: true, DrawRightBorder: true,
	}
	want := []image.Rectangle{image.Rect(10, 20, 11, 60), image.Rect(39, 20, 40, 60)}
	if got := wordPageCellBorderRects(cell, image.Point{}, 1); !reflect.DeepEqual(got, want) {
		t.Fatalf("internal merge border rectangles = %+v; want only the outside vertical edges %+v", got, want)
	}

	cell.DrawTopBorder = true
	cell.DrawBottomBorder = true
	want = []image.Rectangle{
		image.Rect(10, 20, 40, 21), image.Rect(10, 59, 40, 60),
		image.Rect(10, 20, 11, 60), image.Rect(39, 20, 40, 60),
	}
	if got := wordPageCellBorderRects(cell, image.Point{}, 1); !reflect.DeepEqual(got, want) {
		t.Fatalf("page-segment border rectangles = %+v; want visible segment boundaries %+v", got, want)
	}
}
