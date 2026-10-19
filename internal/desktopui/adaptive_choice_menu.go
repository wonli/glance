package desktopui

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (c *adaptiveChoice) close(gtx layout.Context) {
	c.open = false
	gtx.Execute(key.FocusCmd{Tag: &c.trigger})
}

// Update returns a stable option ID, independent of its translated label.
func (c *adaptiveChoice) Update(gtx layout.Context, options []adaptiveChoiceOption, selected string) string {
	if c.trigger.Clicked(gtx) && c.compact {
		if c.open {
			c.close(gtx)
		} else {
			c.open = true
		}
		c.focusRequested = c.open
		c.active = 0
		for i, option := range options {
			if option.ID == selected {
				c.active = i
			}
		}
		if c.open {
			c.list.List.ScrollTo(c.active)
		}
	}
	if c.open && c.dismiss.Clicked(gtx) {
		history := c.dismiss.History()
		if len(history) > 0 {
			point := history[len(history)-1].Position
			if !point.In(c.anchor) && !point.In(c.bounds) {
				c.close(gtx)
			}
		}
	}
	for _, option := range options {
		if c.button(option.ID).Clicked(gtx) {
			if c.open {
				c.close(gtx)
			}
			return option.ID
		}
	}
	if !c.open {
		return ""
	}
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &c.dismiss, Name: key.NameEscape},
			key.Filter{Focus: &c.dismiss, Name: key.NameUpArrow},
			key.Filter{Focus: &c.dismiss, Name: key.NameDownArrow},
			key.Filter{Focus: &c.dismiss, Name: key.NameEnter},
			key.Filter{Focus: &c.dismiss, Name: key.NameReturn},
		)
		if !ok {
			break
		}
		pressed, ok := e.(key.Event)
		if !ok || pressed.State != key.Press {
			continue
		}
		switch pressed.Name {
		case key.NameEscape:
			c.close(gtx)
		case key.NameUpArrow:
			if len(options) > 0 {
				c.active = (c.active + len(options) - 1) % len(options)
				c.list.List.ScrollTo(c.active)
			}
		case key.NameDownArrow:
			if len(options) > 0 {
				c.active = (c.active + 1) % len(options)
				c.list.List.ScrollTo(c.active)
			}
		case key.NameEnter, key.NameReturn:
			if c.active >= 0 && c.active < len(options) {
				c.close(gtx)
				return options[c.active].ID
			}
		}
	}
	return ""
}

// LayoutOverlay must run at the same page root used for anchor coordinates.
// The menu has its own bounded list, so it never inherits a toolbar row clip.
func (c *adaptiveChoice) LayoutOverlay(gtx layout.Context, theme *material.Theme, options []adaptiveChoiceOption, selected string) layout.Dimensions {
	if !c.open || !c.compact || len(options) == 0 {
		return layout.Dimensions{}
	}
	if c.focusRequested {
		c.focusRequested = false
		gtx.Execute(key.FocusCmd{Tag: &c.dismiss})
	}
	viewport := gtx.Constraints.Max
	w := c.anchor.Dx()
	for _, option := range options {
		w = max(w, choiceTextWidth(gtx, theme, option.Label, font.SemiBold)+gtx.Dp(32))
	}
	w = min(w, viewport.X)
	rowHeights := make([]int, len(options))
	height := gtx.Dp(8)
	for i, option := range options {
		measure := gtx
		measure.Constraints = layout.Constraints{Max: image.Pt(max(0, w-gtx.Dp(24)), 1<<20)}
		macro := op.Record(gtx.Ops)
		label := homeLabelStyle(theme, option.Label, 11, mutedText, font.SemiBold)
		label.MaxLines = 0
		dims := label.Layout(measure)
		macro.Stop()
		rowHeights[i] = max(gtx.Dp(32), dims.Size.Y+gtx.Dp(12))
		height += rowHeights[i]
	}
	height = min(height, viewport.Y)
	x := min(max(0, c.anchor.Max.X-w), max(0, viewport.X-w))
	y := c.anchor.Max.Y + gtx.Dp(4)
	if y+height > viewport.Y {
		y = c.anchor.Min.Y - gtx.Dp(4) - height
	}
	y = min(max(0, y), max(0, viewport.Y-height))
	c.bounds = image.Rect(x, y, x+w, y+height)
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			pass := pointer.PassOp{}.Push(gtx.Ops)
			defer pass.Pop()
			return c.dismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			offset := op.Offset(c.bounds.Min).Push(gtx.Ops)
			defer offset.Pop()
			gtx.Constraints = layout.Exact(c.bounds.Size())
			return homeRoundedPanel(gtx, jsonSidebarColor, jsonBorderColor, 6, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					c.list.List.Axis = layout.Vertical
					return c.list.Layout(gtx, len(options), func(gtx layout.Context, i int) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, rowHeights[i]))
						return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return c.layoutOption(gtx, theme, options[i], options[i].ID == selected, i == c.active)
						})
					})
				})
			})
		}),
	)
}
