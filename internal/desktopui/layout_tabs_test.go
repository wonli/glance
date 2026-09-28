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
	dims := layoutSheetTab(gtx, material.NewTheme(), new(widget.Clickable), "Sheet1", true)
	if dims.Size.X <= 0 || dims.Size.X >= 300 {
		t.Fatalf("short sheet tab uses width %d; want intrinsic width under 300px", dims.Size.X)
	}
}
