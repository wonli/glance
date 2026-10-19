package desktopui

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"
)

type adaptiveChoiceOption struct{ ID, Label string }

// adaptiveChoice belongs to persistent view state. Update consumes intents before
// layout; LayoutOverlay is called at the page root using a page-local anchor.
type adaptiveChoice struct {
	trigger, dismiss              widget.Clickable
	buttons                       map[string]*widget.Clickable
	list                          widget.List
	compact, open, focusRequested bool
	active                        int
	anchor, bounds                image.Rectangle
}

type adaptiveChoiceMetrics struct {
	width, titleWidth int
	optionWidths      []int
}

func choiceTextWidth(gtx layout.Context, theme *material.Theme, value string, weight font.Weight) int {
	label := homeLabelStyle(theme, value, 11, mutedText, weight)
	theme.Shaper.LayoutString(text.Parameters{Font: label.Font, PxPerEm: fixed.I(gtx.Sp(label.TextSize)), MaxWidth: 1 << 20, Locale: gtx.Locale}, value)
	var width fixed.Int26_6
	for glyph, ok := theme.Shaper.NextGlyph(); ok; glyph, ok = theme.Shaper.NextGlyph() {
		width = max(width, glyph.X+glyph.Advance)
	}
	return width.Ceil()
}

func measureAdaptiveChoice(gtx layout.Context, theme *material.Theme, title string, options []adaptiveChoiceOption) adaptiveChoiceMetrics {
	m := adaptiveChoiceMetrics{titleWidth: choiceTextWidth(gtx, theme, title, font.Normal)}
	m.width = gtx.Dp(20)
	if title != "" {
		m.width += m.titleWidth + gtx.Dp(8)
	}
	for i, option := range options {
		// Reserve both weights so changing selection cannot move the control.
		w := max(choiceTextWidth(gtx, theme, option.Label, font.Normal), choiceTextWidth(gtx, theme, option.Label, font.SemiBold))
		w = max(gtx.Dp(22), w+gtx.Dp(16))
		m.optionWidths = append(m.optionWidths, w)
		m.width += w
		if i > 0 {
			m.width += gtx.Dp(5)
		}
	}
	return m
}

func (c *adaptiveChoice) button(id string) *widget.Clickable {
	if c.buttons == nil {
		c.buttons = make(map[string]*widget.Clickable)
	}
	if c.buttons[id] == nil {
		c.buttons[id] = new(widget.Clickable)
	}
	return c.buttons[id]
}

func (c *adaptiveChoice) Layout(gtx layout.Context, theme *material.Theme, title string, options []adaptiveChoiceOption, selected string) layout.Dimensions {
	m := measureAdaptiveChoice(gtx, theme, title, options)
	c.compact = m.width > gtx.Constraints.Max.X
	w := min(m.width, gtx.Constraints.Max.X)
	gtx.Constraints = layout.Exact(image.Pt(w, min(gtx.Dp(34), gtx.Constraints.Max.Y)))
	if !c.compact {
		if c.open {
			c.open = false
			if selected != "" {
				gtx.Execute(key.FocusCmd{Tag: c.button(selected)})
			}
		}
	}
	return homeRoundedPanel(gtx, jsonSidebarColor, jsonBorderColor, 6, func(gtx layout.Context) layout.Dimensions {
		if c.compact {
			label := ""
			for _, option := range options {
				if option.ID == selected {
					label = option.Label
					break
				}
			}
			return c.trigger.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				jsonButtonSemantics(gtx, title+": "+label, true)
				return layout.Inset{Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return homeLabelStyle(theme, title, 11, mutedText, font.Normal).Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeLabelStyle(theme, label+" ▾", 11, jsonBlue, font.SemiBold).Layout(gtx)
							})
						}),
					)
				})
			})
		}
		return layout.Inset{Left: 10, Right: 10, Top: 5, Bottom: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(options)+1)
			if title != "" {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return homeLabelStyle(theme, title, 11, mutedText, font.Normal).Layout(gtx)
					})
				}))
			}
			for i, option := range options {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gap := unit.Dp(5)
					if i == len(options)-1 {
						gap = 0
					}
					return layout.Inset{Right: gap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(m.optionWidths[i], gtx.Dp(24)))
						return c.layoutOption(gtx, theme, option, option.ID == selected, false)
					})
				}))
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

func (c *adaptiveChoice) layoutOption(gtx layout.Context, theme *material.Theme, option adaptiveChoiceOption, selected, active bool) layout.Dimensions {
	button := c.button(option.ID)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		jsonButtonSemantics(gtx, option.Label, true)
		bg, fg, weight := jsonSidebarColor, mutedText, font.Normal
		if selected {
			bg, fg, weight = contentSurface, jsonBlue, font.SemiBold
		} else if active || button.Hovered() {
			bg = homeHoverFill
		}
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			label := homeLabelStyle(theme, option.Label, 11, fg, weight)
			if c.open {
				label.MaxLines = 0
			}
			return label.Layout(gtx)
		})
	})
}
