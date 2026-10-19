package main

import (
	"image"
	"image/color"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/ui"
)

type galleryPane struct {
	nav         widget.Clickable
	button      widget.Clickable
	plainButton widget.Clickable
	field       widget.Editor
	placeholder widget.Editor
	masked      widget.Editor
	suffix      widget.Clickable
	selected    bool
	showSecret  bool
}

type gallery struct {
	zeroOffsets    widget.Clickable
	light          galleryPane
	dark           galleryPane
	theme          *material.Theme
	offsetsAreZero bool
}

func run(window *app.Window) error {
	var ops op.Ops
	g := gallery{theme: material.NewTheme()}
	g.light.field.SingleLine = true
	g.light.field.SetText("model-name-1")
	g.light.placeholder.SingleLine = true
	g.light.masked.SingleLine = true
	g.light.masked.SetText("synthetic-gallery-key-8f3a-2d71-long-value-for-horizontal-scroll-preview")
	g.light.masked.Mask = '*'
	g.dark.field.SingleLine = true
	g.dark.field.SetText("模型名称示例")
	g.dark.placeholder.SingleLine = true
	g.dark.masked.SingleLine = true
	g.dark.masked.SetText("仅用于展示的合成长文本凭据-不要用于真实账号-横向滚动演示")
	g.dark.masked.Mask = '*'
	for {
		event := window.Event()
		switch e := event.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			g.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (g *gallery) layout(gtx layout.Context) {
	if g.zeroOffsets.Clicked(gtx) {
		g.offsetsAreZero = !g.offsetsAreZero
	}
	paint.Fill(gtx.Ops, color.NRGBA{R: 0xF4, G: 0xF6, B: 0xF8, A: 0xFF})
	layout.Inset{Top: 24, Bottom: 24, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						style := material.H6(g.theme, "Reusable Gio controls")
						style.Color = color.NRGBA{R: 0x24, G: 0x2B, B: 0x35, A: 0xFF}
						return style.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := "Use zero offsets"
						if g.offsetsAreZero {
							label = "Use optical offsets"
						}
						return (ui.IconTextButton{Button: &g.zeroOffsets, Label: label, Style: ui.IconTextButtonStyle{
							Height: 40, Gap: 8, Radius: 8, Text: ui.TextStyle{Size: 13, Weight: font.Medium},
							Colors: ui.StateColors{Background: color.NRGBA{R: 0xE5, G: 0xEB, B: 0xF2, A: 0xFF}, Hover: color.NRGBA{R: 0xD8, G: 0xE1, B: 0xEC, A: 0xFF}, Border: color.NRGBA{R: 0xC8, G: 0xD2, B: 0xDF, A: 0xFF}, Foreground: color.NRGBA{R: 0x35, G: 0x41, B: 0x4F, A: 0xFF}},
						}}).Layout(gtx, g.theme)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 18}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return g.layoutPane(gtx, &g.light, false)
					}),
					layout.Rigid(layout.Spacer{Width: 18}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return g.layoutPane(gtx, &g.dark, true)
					}),
				)
			}),
		)
	})
}

func (g *gallery) layoutPane(gtx layout.Context, pane *galleryPane, dark bool) layout.Dimensions {
	if pane.nav.Clicked(gtx) {
		pane.selected = !pane.selected
	}
	if pane.suffix.Clicked(gtx) {
		pane.showSecret = !pane.showSecret
		if pane.showSecret {
			pane.masked.Mask = 0
		} else {
			pane.masked.Mask = '*'
		}
	}
	colors := palette(dark)
	fill := colors.surface
	shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(12))
	paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, colors.border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	offset := unit.Dp(3)
	if g.offsetsAreZero {
		offset = 0
	}
	return layout.Inset{Top: 18, Bottom: 18, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				style := material.H6(g.theme, map[bool]string{false: "Light · English", true: "Dark · 中文"}[dark])
				style.Color = colors.foreground
				return style.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: 18}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.NavItem{Button: &pane.nav, Label: map[bool]string{false: "Recent files", true: "最近文件"}[dark], Trailing: "12",
					Icon:     glyph(g.theme, "◷", colors.foreground),
					Selected: pane.selected,
					Style: ui.NavItemStyle{Height: 44, IconSize: 20, LeftInset: 12, RightInset: 12, IconGap: 12, TrailingGap: 12, Radius: 9,
						Text: ui.TextStyle{Size: 14, Weight: font.Normal}, SelectedText: ui.TextStyle{Size: 14, Weight: font.SemiBold}, TrailingText: ui.TextStyle{Size: 12},
						TextOffsetY: offset, TrailingOffsetY: offset, TrailingColor: colors.muted,
						Colors: ui.StateColors{Background: colors.surface, Hover: colors.hover, Selected: colors.selected, Border: colors.surface, Foreground: colors.foreground}},
				}).Layout(gtx, g.theme)
			}),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.IconTextButton{Button: &pane.button, Label: map[bool]string{false: "Open recent item", true: "打开最近项目"}[dark], Icon: glyph(g.theme, "✦", colors.foreground),
					Style: ui.IconTextButtonStyle{Height: 42, IconSize: 16, Gap: 8, Radius: 8, Text: ui.TextStyle{Size: 13, Weight: font.Medium}, TextOffsetY: offset,
						Colors: ui.StateColors{Background: colors.button, Hover: colors.hover, Border: colors.border, Foreground: colors.foreground}}}).Layout(gtx, g.theme)
			}),
			layout.Rigid(layout.Spacer{Height: 16}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.IconTextButton{Button: &pane.plainButton, Label: map[bool]string{false: "Save changes", true: "保存更改"}[dark],
					Style: ui.IconTextButtonStyle{Height: 38, Radius: 8, Text: ui.TextStyle{Size: 13, Weight: font.Medium}, TextOffsetY: offset,
						Colors: ui.StateColors{Background: colors.selected, Hover: colors.hover, Border: colors.border, Foreground: colors.foreground}}}).Layout(gtx, g.theme)
			}),
			layout.Rigid(layout.Spacer{Height: 16}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return label(gtx, g.theme, "Single-line editor · 单行输入", colors.muted, 12)
			}),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.TextField{Editor: &pane.field, Hint: "Placeholder · 输入模型名称", Style: ui.TextFieldStyle{Height: 42, LeftInset: 12, RightInset: 12, Radius: 7, Text: ui.TextStyle{Size: 13}, Fill: colors.surface, Border: colors.border, Foreground: colors.foreground, Hint: colors.muted, TextOffsetY: offset}}).Layout(gtx, g.theme)
			}),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.TextField{Editor: &pane.placeholder, Hint: "Placeholder · 输入模型名称", Style: ui.TextFieldStyle{Height: 42, LeftInset: 12, RightInset: 12, Radius: 7, Text: ui.TextStyle{Size: 13}, Fill: colors.surface, Border: colors.border, Foreground: colors.foreground, Hint: colors.muted, TextOffsetY: offset}}).Layout(gtx, g.theme)
			}),
			layout.Rigid(layout.Spacer{Height: 12}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.TextField{Editor: &pane.masked, Hint: "Synthetic key", Suffix: func(gtx layout.Context) layout.Dimensions {
					return pane.suffix.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						labelText := "Show"
						if pane.showSecret {
							labelText = "Hide"
						}
						return label(gtx, g.theme, labelText, colors.foreground, 11)
					})
				}, Style: ui.TextFieldStyle{Height: 42, LeftInset: 12, SuffixWidth: 58, Radius: 7, Text: ui.TextStyle{Size: 13}, Fill: colors.surface, Border: colors.border, Foreground: colors.foreground, Hint: colors.muted, TextOffsetY: offset}}).Layout(gtx, g.theme)
			}),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return label(gtx, g.theme, "Long synthetic value stays editable and horizontally scrollable.", colors.muted, 11)
			}),
		)
	})
}

type paletteColors struct {
	surface, border, foreground, muted, hover, selected, button color.NRGBA
}

func palette(dark bool) paletteColors {
	if dark {
		return paletteColors{
			surface: color.NRGBA{R: 0x20, G: 0x25, B: 0x2D, A: 0xFF}, border: color.NRGBA{R: 0x3C, G: 0x44, B: 0x50, A: 0xFF},
			foreground: color.NRGBA{R: 0xE8, G: 0xEC, B: 0xF1, A: 0xFF}, muted: color.NRGBA{R: 0xA6, G: 0xB0, B: 0xBE, A: 0xFF},
			hover: color.NRGBA{R: 0x32, G: 0x3B, B: 0x48, A: 0xFF}, selected: color.NRGBA{R: 0x36, G: 0x48, B: 0x5C, A: 0xFF}, button: color.NRGBA{R: 0x2D, G: 0x3B, B: 0x4C, A: 0xFF},
		}
	}
	return paletteColors{
		surface: color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, border: color.NRGBA{R: 0xD9, G: 0xDE, B: 0xE5, A: 0xFF},
		foreground: color.NRGBA{R: 0x2E, G: 0x35, B: 0x3E, A: 0xFF}, muted: color.NRGBA{R: 0x7C, G: 0x86, B: 0x94, A: 0xFF},
		hover: color.NRGBA{R: 0xF1, G: 0xF4, B: 0xF7, A: 0xFF}, selected: color.NRGBA{R: 0xE5, G: 0xED, B: 0xF8, A: 0xFF}, button: color.NRGBA{R: 0xF5, G: 0xF7, B: 0xFA, A: 0xFF},
	}
}

func glyph(theme *material.Theme, value string, tint color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		style := material.Body2(theme, value)
		style.TextSize = unit.Sp(16)
		style.Color = tint
		return layout.Center.Layout(gtx, style.Layout)
	}
}

func label(gtx layout.Context, theme *material.Theme, value string, tint color.NRGBA, size unit.Sp) layout.Dimensions {
	style := material.Body2(theme, value)
	style.TextSize = size
	style.Color = tint
	return style.Layout(gtx)
}
