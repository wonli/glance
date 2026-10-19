package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// OpticalPlacement adjusts a child's vertical text or icon placement.
// OffsetY translates drawing without changing the reported geometry or hit
// bounds. TopInset adds layout space before the child and affects geometry.
// Positive values move content downward, in device-independent pixels.
type OpticalPlacement struct {
	OffsetY  unit.Dp
	TopInset unit.Dp
}

// Layout applies the requested optical placement to child.
func (p OpticalPlacement) Layout(gtx layout.Context, child layout.Widget) layout.Dimensions {
	if child == nil {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: p.TopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		shift := op.Offset(image.Pt(0, gtx.Dp(p.OffsetY))).Push(gtx.Ops)
		dims := child(gtx)
		shift.Pop()
		return dims
	})
}
