package desktopui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func TestSheetTabsUseOnlyTheirIntrinsicWidth(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Max: image.Pt(800, 34)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	dims := layoutSheetTab(gtx, material.NewTheme(), defaultShellTokens.Colors, new(widget.Clickable), "Sheet1", true)
	if dims.Size.X <= 0 || dims.Size.X >= 300 {
		t.Fatalf("short sheet tab uses width %d; want intrinsic width under 300px", dims.Size.X)
	}
}

func TestSheetTabRowUsesAvailableWidthAndKeepsTabsAtStart(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Max: image.Pt(800, 34)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	children := []layout.FlexChild{
		layout.Rigid(func(layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(70, 34)} }),
		layout.Rigid(func(layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(80, 34)} }),
	}
	dims := layoutSheetTabRow(gtx, children...)
	if dims.Size.X != 800 {
		t.Fatalf("sheet tab row width=%d; want available width 800 so tabs start at left", dims.Size.X)
	}
}
