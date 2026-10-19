package desktopui

import (
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Dropdown indicators are graphics, not text. Shaping U+25BE can load an entire
// symbol fallback font just for a tiny triangle (about 46 MiB in a macOS probe).
// Existing command labels keep their marker for semantic/display metadata; only
// the text sent to the shaper excludes it. The enclosing button owns hit tests.
func layoutDropdownLabel(gtx layout.Context, style material.LabelStyle) layout.Dimensions {
	if !strings.HasSuffix(style.Text, " ▾") {
		return style.Layout(gtx)
	}
	style.Text = strings.TrimSuffix(style.Text, " ▾")
	gtx.Constraints.Min = image.Point{}
	width, height := min(gtx.Dp(unit.Dp(6)), gtx.Constraints.Max.X), min(gtx.Dp(unit.Dp(4)), gtx.Constraints.Max.Y)
	gap := min(gtx.Dp(unit.Dp(4)), max(0, gtx.Constraints.Max.X-width))
	// Align beside the final text line when a caption wraps. The small lower
	// inset follows the font size, rather than centering over the whole paragraph.
	bottom := gtx.Sp(style.TextSize) * 3 / 4
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = max(0, gtx.Constraints.Max.X-gap-width)
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(gap, 0)} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(f32.Pt(0, 0))
			path.LineTo(f32.Pt(float32(width), 0))
			path.LineTo(f32.Pt(float32(width)/2, float32(height)))
			path.Close()
			paint.FillShape(gtx.Ops, style.Color, clip.Outline{Path: path.End()}.Op())
			return layout.Dimensions{Size: image.Pt(width, height+bottom)}
		}),
	)
}
