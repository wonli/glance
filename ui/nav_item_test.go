package ui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestLayoutNavLabelUsesNaturalHeightInsideFixedRow(t *testing.T) {
	gtx := layout.Context{
		Ops:    new(op.Ops),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Min: image.Pt(0, 42),
			Max: image.Pt(260, 42),
		},
	}
	label := material.Body2(material.NewTheme(), "最近")
	label.TextSize = 15
	label.MaxLines = 1

	dims := layoutNavLabel(gtx, label, 3)
	if dims.Size.Y <= 0 || dims.Size.Y >= 42 {
		t.Fatalf("navigation label height=%d; want natural text height below fixed 42px row", dims.Size.Y)
	}
}
