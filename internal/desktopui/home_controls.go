package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func homeTextLabel(gtx layout.Context, theme *material.Theme, text string, size unit.Sp, fg color.NRGBA, bold bool) layout.Dimensions {
	weight := font.Normal
	if bold {
		weight = font.SemiBold
	}
	return homeLabelStyle(theme, text, size, fg, weight).Layout(gtx)
}

func homeLabelStyle(theme *material.Theme, text string, size unit.Sp, fg color.NRGBA, weight font.Weight) material.LabelStyle {
	style := material.Body2(theme, text)
	style.TextSize = size
	style.Color = fg
	style.Font.Weight = weight
	style.MaxLines = 1
	return style
}

func homeRecentCountStyle(theme *material.Theme, text string, scale float32) material.LabelStyle {
	return homeLabelStyle(theme, text, unit.Sp(14*scale), homeMuted, font.SemiBold)
}

func homeIcon(gtx layout.Context, name string, size unit.Dp, tint color.NRGBA) layout.Dimensions {
	pixels := gtx.Dp(size)
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(pixels, pixels), image.Pt(pixels, pixels)
	icon, ok := tintedToolbarIcon(name, tint)
	if !ok {
		return layout.Dimensions{Size: image.Pt(pixels, pixels)}
	}
	return widget.Image{
		Src:      paint.NewImageOp(icon),
		Fit:      widget.Contain,
		Position: layout.Center,
		Scale:    float32(size) / 96,
	}.Layout(gtx)
}

func homeActionCard(gtx layout.Context, theme *material.Theme, button *widget.Clickable, action homeAction) layout.Dimensions {
	height := gtx.Dp(unit.Dp(132))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	leftInset := unit.Dp(22)
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(136)) {
		leftInset = unit.Dp(14)
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(14), func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18), Left: leftInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return homeActionBadge(gtx, action) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeActionTitle(gtx, theme, action)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Caption(theme, action.extension)
							style.TextSize = unit.Sp(13)
							if action.id == "open" {
								style.TextSize = unit.Sp(11)
							}
							style.Color = homeMuted
							style.MaxLines = 1
							return style.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func homeActionTitle(gtx layout.Context, theme *material.Theme, action homeAction) layout.Dimensions {
	scale := float32(gtx.Constraints.Max.X) / float32(gtx.Dp(unit.Dp(122)))
	if scale > 1 {
		scale = 1
	}
	style := homeLabelStyle(theme, action.label, unit.Sp(float32(action.titleSize)*scale), color.NRGBA{R: 31, G: 37, B: 45, A: 255}, font.SemiBold)
	if !action.ai {
		return style.Layout(gtx)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(style.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				ai := homeLabelStyle(theme, "AI", unit.Sp(12*scale), color.NRGBA{R: 52, G: 120, B: 209, A: 255}, font.SemiBold)
				return ai.Layout(gtx)
			})
		}),
	)
}

func homeActionBadge(gtx layout.Context, action homeAction) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(38)), gtx.Dp(unit.Dp(38))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	shape := clip.UniformRRect(image.Rect(0, 0, width, height), gtx.Dp(unit.Dp(7)))
	paint.FillShape(gtx.Ops, action.iconColor, shape.Op(gtx.Ops))
	iconTint := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeIcon(gtx, action.icon, 21, iconTint)
	})
}

func homeFilterGroup(gtx layout.Context, theme *material.Theme, buttons map[string]*widget.Clickable, active *string, scale float32, onChange func(string)) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(650*scale)), gtx.Dp(unit.Dp(44*scale))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			shape := clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(11*scale)))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 240, G: 241, B: 243, A: 255}, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: image.Pt(width, height)}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4 * scale), Left: unit.Dp(6 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(homeFilters))
				for index, label := range homeFilters {
					filter := label
					button := buttons[filter]
					if button == nil {
						button = new(widget.Clickable)
						buttons[filter] = button
					}
					if button.Clicked(gtx) {
						onChange(filter)
					}
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						right := unit.Dp(0)
						if index < len(homeFilters)-1 {
							right = unit.Dp(4 * scale)
						}
						return layout.Inset{Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeFilterChip(gtx, theme, button, filter, *active == filter, scale)
						})
					}))
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
		},
	)
}

func homeFilterChip(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, selected bool, scale float32) layout.Dimensions {
	width := homeFilterWidth(label)
	height := unit.Dp(36 * scale)
	width = unit.Dp(float32(width) * scale)
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(gtx.Dp(width), gtx.Dp(height)), image.Pt(gtx.Dp(width), gtx.Dp(height))
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if selected {
					bg = homeSurface
				} else if button.Hovered() {
					bg = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(8*scale)))
				paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(theme, label)
				style.TextSize = unit.Sp(15 * scale)
				style.Color = color.NRGBA{R: 104, G: 113, B: 124, A: 255}
				style.Font.Weight = font.Normal
				if selected {
					style.Color = color.NRGBA{R: 37, G: 43, B: 51, A: 255}
					style.Font.Weight = font.SemiBold
				}
				return layout.Center.Layout(gtx, style.Layout)
			},
		)
	})
}

func homeFilterWidth(label string) unit.Dp {
	switch label {
	case "全部", "文档", "表格", "演示":
		return 65
	case "PDF":
		return 64
	case "Markdown":
		return 109
	case "HTML":
		return 77
	default:
		return 0
	}
}

func homeSearchField(gtx layout.Context, theme *material.Theme, editor *widget.Editor, width, height float32) layout.Dimensions {
	searchWidth, searchHeight := gtx.Dp(unit.Dp(width)), gtx.Dp(unit.Dp(height))
	scale := width / 240
	if scale > 1 {
		scale = 1
	}
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(searchWidth, searchHeight), image.Pt(searchWidth, searchHeight)
	return homeRoundedPanel(gtx, homeSurface, color.NRGBA{R: 224, G: 228, B: 233, A: 255}, unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return layout.Inset{Left: unit.Dp(14 * scale), Right: unit.Dp(10 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, "search", unit.Dp(19*scale), homeMuted)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				style := material.Editor(theme, editor, homeSearchPlaceholder)
				style.TextSize = unit.Sp(15 * scale)
				style.Color = homeText
				style.HintColor = color.NRGBA{R: 150, G: 157, B: 166, A: 255}
				return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, style.Layout)
			}),
		)
	})
}

func homeSortButton(gtx layout.Context, button *widget.Clickable, width, height float32) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(width)), gtx.Dp(unit.Dp(height)))
	gtx.Constraints.Min, gtx.Constraints.Max = size, size
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, "list-filter", unit.Dp(width), color.NRGBA{R: 110, G: 119, B: 130, A: 255})
		})
	})
}

func homeSmallIconButton(gtx layout.Context, button *widget.Clickable, icon string, size unit.Dp, fg color.NRGBA, onClick func()) layout.Dimensions {
	if button.Clicked(gtx) {
		onClick()
	}
	hit := gtx.Dp(size)
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(hit, hit), image.Pt(hit, hit)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, icon, size, fg) })
	})
}

func homeIconButton(gtx layout.Context, button *widget.Clickable, icon string, size unit.Dp, fg color.NRGBA, selected bool) layout.Dimensions {
	buttonSize := gtx.Dp(size)
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(buttonSize, buttonSize), image.Pt(buttonSize, buttonSize)
	if selected {
		if icon == "star" {
			icon = "star-filled"
			fg = homeFavoriteGold
		} else {
			fg = color.NRGBA{R: 110, G: 119, B: 130, A: 255}
		}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, icon, size, fg) })
	})
}

func homeFileBadge(gtx layout.Context, extension string, fg color.NRGBA, size unit.Dp) layout.Dimensions {
	w, h := gtx.Dp(size), gtx.Dp(size)
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(w, h), image.Pt(w, h)
	shape := clip.UniformRRect(image.Rect(0, 0, w, h), gtx.Dp(unit.Dp(5)))
	paint.FillShape(gtx.Ops, fg, shape.Op(gtx.Ops))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeIcon(gtx, homeFileIconName(extension), 20, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	})
}

func homeFileIconName(extension string) string {
	switch extension {
	case "XLS", "XLSX", "CSV":
		return "file-spreadsheet"
	case "PPT", "PPTX":
		return "presentation"
	case "PDF", "FILE":
		return "file"
	case "MD", "MARK":
		return "file-code"
	case "HTML", "HTM":
		return "code"
	default:
		return "file-text"
	}
}

func homeRoundedPanel(gtx layout.Context, fill, border color.NRGBA, radius unit.Dp, child layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(radius))
			paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: size}
		}, child,
	)
}
