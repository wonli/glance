package ui

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TextField renders a single editor inside a framed input area.
// The caller owns the Editor and any Clickable used by Suffix.
type TextField struct {
	Editor   *widget.Editor
	Hint     string
	Suffix   layout.Widget
	Style    TextFieldStyle
	Hovered  bool
	Focused  bool
	Disabled bool
}

// Layout draws the field. The text viewport may be shorter than the frame;
// clicks across the input portion still focus the same stable editor.
func (f TextField) Layout(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	if f.Editor == nil {
		return layout.Dimensions{}
	}
	if f.Disabled {
		gtx = gtx.Disabled()
	}
	width := gtx.Constraints.Max.X
	if f.Style.Width > 0 {
		width = gtx.Dp(f.Style.Width)
	}
	if width < gtx.Constraints.Min.X {
		width = gtx.Constraints.Min.X
	}
	if gtx.Constraints.Max.X > 0 && width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	height := gtx.Dp(f.Style.Height)
	if f.Style.Height <= 0 {
		height = gtx.Constraints.Min.Y
	}
	if height < gtx.Constraints.Min.Y {
		height = gtx.Constraints.Min.Y
	}
	if gtx.Constraints.Max.Y > 0 && height > gtx.Constraints.Max.Y {
		height = gtx.Constraints.Max.Y
	}
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)

	fill, border := f.Style.Fill, f.Style.Border
	foreground, hintColor := f.Style.Foreground, f.Style.Hint
	if f.Disabled {
		fill, border = f.Style.DisabledFill, f.Style.DisabledBorder
		foreground, hintColor = f.Style.DisabledForeground, f.Style.DisabledHint
	} else if f.Focused && f.Style.FocusBorder.A > 0 {
		border = f.Style.FocusBorder
	} else if f.Hovered && f.Style.HoverBorder.A > 0 {
		border = f.Style.HoverBorder
	}
	shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(f.Style.Radius))
	paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
	if border.A > 0 {
		paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	}

	suffixWidth := 0
	if f.Suffix != nil {
		suffixWidth = gtx.Dp(f.Style.SuffixWidth)
		if suffixWidth < 0 {
			suffixWidth = 0
		}
		if suffixWidth > width {
			suffixWidth = width
		}
	}
	inputWidth := width - suffixWidth
	focusArea := clip.Rect(image.Rect(0, 0, inputWidth, height)).Push(gtx.Ops)
	pointer.CursorText.Add(gtx.Ops)
	event.Op(gtx.Ops, f.Editor)
	focusArea.Pop()
	for _, ok := gtx.Event(pointer.Filter{Target: f.Editor, Kinds: pointer.Press}); ok; _, ok = gtx.Event(pointer.Filter{Target: f.Editor, Kinds: pointer.Press}) {
		gtx.Execute(key.FocusCmd{Tag: f.Editor})
	}

	layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: f.Style.LeftInset, Right: f.Style.RightInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				label := material.Editor(theme, f.Editor, f.Hint)
				label.TextSize = f.Style.Text.Size
				label.Font.Weight = f.Style.Text.Weight
				label.Color = foreground
				label.HintColor = hintColor
				return (OpticalPlacement{OffsetY: f.Style.TextOffsetY}).Layout(gtx, label.Layout)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if f.Suffix == nil {
				return layout.Dimensions{}
			}
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = suffixWidth, suffixWidth
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			return f.Suffix(gtx)
		}),
	)
	return layout.Dimensions{Size: size}
}
