package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"gioui.org/widget/material"
)

var fontPickerInk = color.NRGBA{R: 52, G: 59, B: 69, A: 255}
var fontPickerMuted = color.NRGBA{R: 122, G: 130, B: 141, A: 255}

const (
	fontPickerSearchTextOpticalTopInset  unit.Dp = 0
	fontPickerRefreshTextOpticalTopInset unit.Dp = 0
)

func (a *App) fontPickerLabel(gtx layout.Context, value string, size unit.Sp, fg color.NRGBA) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	l := material.Label(a.theme, size, value)
	l.Color = fg
	l.MaxLines = 1
	return l.Layout(gtx)
}
func (a *App) layoutFontPicker(gtx layout.Context) layout.Dimensions {
	p := &a.fontPicker
	if !p.open || !a.fontPickerTargetValid() {
		return layout.Dimensions{}
	}
	event.Op(gtx.Ops, &p.keyTag)
	if p.focusRequested {
		gtx.Execute(key.FocusCmd{Tag: &p.search})
		p.focusRequested = false
	}
	rows := a.fontPickerRowsForCurrentState()
	width := min(gtx.Dp(388), gtx.Constraints.Max.X)
	desired := gtx.Dp(38 + 48 + 34 + 2)
	content := p.sectionRowCount*gtx.Dp(24) + p.familyRowCount*gtx.Dp(44)
	if content == 0 {
		content = gtx.Dp(220)
	}
	desired += min(content+gtx.Dp(16), gtx.Dp(352))
	x := gtx.Dp(12)
	if p.target.word {
		x = wordPenCommandX(gtx, p.target.state, "font-family")
	} else {
		state := p.target.state
		actions := toolbarActionsForTab(state.toolbarTab)
		if state.toolbarTab == toolbarTabHome {
			groups := spreadsheetHomeRibbonGroups(actions)
			first := min(max(0, state.toolbarGroupList.Position.First), len(groups))
			offset := 0
			for i := 0; i < first; i++ {
				offset += groups[i].widthDP + 4
			}
			x = gtx.Dp(unit.Dp(80+spreadsheetHomeActionAnchorOffset(groups, toolbarActionFontFamily)-offset)) - state.toolbarGroupList.Position.Offset
		} else {
			x = spreadsheetPenCommandX(gtx, state, toolbarActionFontFamily)
		}
	}
	anchorX := x
	x = min(max(0, x), max(0, gtx.Constraints.Max.X-width))
	y := gtx.Dp(unit.Dp(wordRibbonHeightDP + 2))
	height := min(desired, max(0, gtx.Constraints.Max.Y-gtx.Dp(12)))
	if y+height > gtx.Constraints.Max.Y {
		y = max(0, gtx.Constraints.Max.Y-height)
	}
	if width == 0 || height == 0 {
		return layout.Dimensions{}
	}
	p.bounds = image.Rect(x, y, x+width, y+height)
	p.anchor = image.Rect(anchorX, gtx.Dp(unit.Dp(wordRibbonCategoryHeightDP)), anchorX+gtx.Dp(108), gtx.Dp(unit.Dp(wordRibbonHeightDP)))
	return layout.Stack{}.Layout(gtx, layout.Expanded(func(gtx layout.Context) layout.Dimensions {
		pass := pointer.PassOp{}.Push(gtx.Ops)
		defer pass.Pop()
		return p.dismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
	}), layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		defer off.Pop()
		gtx.Constraints = layout.Exact(image.Pt(width, height))
		rr := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(5))
		paint.FillShape(gtx.Ops, color.NRGBA{R: 216, G: 222, B: 230, A: 255}, rr.Op(gtx.Ops))
		return p.panelInput.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(1).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(a.layoutFontPickerHeader), layout.Rigid(a.layoutFontPickerSearch), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						p.visibleRowKeys = p.visibleRowKeys[:0]
						if len(rows) == 0 {
							return a.layoutFontPickerEmpty(gtx)
						}
						return p.list.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
							p.visibleRowKeys = append(p.visibleRowKeys, fontRowKey(rows[i]))
							return a.layoutFontPickerRow(gtx, rows[i], i)
						})
					})
				}), layout.Rigid(a.layoutFontPickerFooter))
			})
		})
	}))
}
func (a *App) layoutFontPickerHeader(gtx layout.Context) layout.Dimensions {
	p := &a.fontPicker
	gtx.Constraints.Min.Y = gtx.Dp(38)
	gtx.Constraints.Max.Y = gtx.Dp(38)
	return layout.Inset{Left: 16, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.fontPickerLabel(gtx, a.textID("font.picker.title"), 15, fontPickerInk)
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.close.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(28), gtx.Dp(28)))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, "x", 14, fontPickerMuted) })
			})
		}))
	})
}
func (a *App) layoutFontPickerSearch(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: 12, Right: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(36)
		gtx.Constraints.Max.Y = gtx.Dp(36)
		settingsFieldFocus(gtx, &a.fontPicker.search)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 246, G: 247, B: 249, A: 255}, clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(36))}, gtx.Dp(4)).Op(gtx.Ops))
		return layout.Inset{Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				return homeIcon(gtx, "search", 14, fontPickerMuted)
			}), layout.Rigid(layout.Spacer{Width: 8}.Layout), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				e := material.Editor(a.theme, &a.fontPicker.search, a.textID("font.search.placeholder"))
				e.TextSize = unit.Sp(12)
				e.Color = fontPickerInk
				return layout.Inset{Top: fontPickerSearchTextOpticalTopInset}.Layout(gtx, e.Layout)
			}))
		})
	})
}
func (a *App) layoutFontPickerFooter(gtx layout.Context) layout.Dimensions {
	p := &a.fontPicker
	gtx.Constraints.Min.Y = gtx.Dp(34)
	gtx.Constraints.Max.Y = gtx.Dp(34)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 231, G: 234, B: 240, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	return layout.Inset{Left: 14, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			hintID := "font.picker.status.cached"
			if p.snapshot.Version == 0 {
				hintID = "font.picker.status.loading"
			}
			if p.event.CacheErr != nil {
				hintID = "font.picker.status.cache_write_failed"
			}
			if p.event.Err != nil {
				hintID = "font.picker.status.refresh_failed"
			}
			return a.fontPickerLabel(gtx, a.textID(hintID), 10, fontPickerMuted)
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.event.Refreshing {
				gtx = gtx.Disabled()
			}
			return p.refresh.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Point{}
					return homeIcon(gtx, "refresh-cw", 14, fontPickerMuted)
				}), layout.Rigid(layout.Spacer{Width: 5}.Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					labelID := "font.picker.action.refresh"
					if p.event.Refreshing {
						labelID = "font.picker.action.refreshing"
					} else if p.event.Err != nil {
						labelID = "font.picker.action.retry_refresh"
					}
					return layout.Inset{Top: fontPickerRefreshTextOpticalTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.fontPickerLabel(gtx, a.textID(labelID), 11, fontPickerInk)
					})
				}))
			})
		}))
	})
}
func (a *App) layoutFontPickerEmpty(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			textID := "font.picker.empty.no_match"
			if a.fontPicker.snapshot.Version == 0 {
				textID = "font.picker.status.loading"
			}
			return a.fontPickerLabel(gtx, a.textID(textID), 14, fontPickerInk)
		}), layout.Rigid(layout.Spacer{Height: 12}.Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.fontPicker.clear.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.fontPickerLabel(gtx, a.textID("font.picker.action.clear_search"), 12, fontPickerMuted)
			})
		}))
	})
}
func (a *App) layoutFontPickerRow(gtx layout.Context, row fontPickerRow, index int) layout.Dimensions {
	p := &a.fontPicker
	if row.title != "" {
		gtx.Constraints.Min.Y = gtx.Dp(24)
		return layout.Inset{Left: 8, Top: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := row.title
			if row.titleMessageID != "" {
				label = a.textID(row.titleMessageID)
			}
			return a.fontPickerLabel(gtx, label, 10, fontPickerMuted)
		})
	}
	gtx.Constraints.Min.Y = gtx.Dp(44)
	gtx.Constraints.Max.Y = gtx.Dp(44)
	key := fontRowKey(row)
	button := p.buttons[key]
	if button == nil {
		button = &fontPickerButton{}
		p.buttons[key] = button
	}
	displayFamily := row.displayFamily
	if displayFamily == "" {
		displayFamily = row.family
	}
	selected := strings.EqualFold(row.family, p.target.current) || strings.EqualFold(displayFamily, p.target.current)
	if !selected {
		for _, alias := range row.aliases {
			if strings.EqualFold(alias, p.target.current) {
				selected = true
				break
			}
		}
	}
	if selected && row.faceID != "" {
		selected = a.fontPickerResolvedFaceID() == row.faceID
	}
	if selected || button.choose.Hovered() || (gtx.Focused(&p.keyTag) && index == p.active) {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 238, G: 240, B: 243, A: 255}, clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(44))}, gtx.Dp(4)).Op(gtx.Ops))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		if row.missing || row.online != nil {
			gtx = gtx.Disabled()
		}
		return button.choose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.fontPickerLabel(gtx, displayFamily, 13, fontPickerInk)
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					fg := fontPickerMuted
					if row.missing {
						fg = color.NRGBA{R: 165, G: 103, B: 34, A: 255}
					}
					source := row.source
					if row.sourceMessageID != "" {
						source = a.textID(row.sourceMessageID)
					}
					return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.fontPickerLabel(gtx, source, 10, fg) })
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if row.online != nil {
						return layout.Dimensions{}
					}
					if row.missing {
						return a.fontPickerLabel(gtx, a.textID("font.picker.preview.substitute"), 10, fontPickerMuted)
					}
					return a.fontPickerLabel(gtx, a.textID("font.picker.preview.sample"), 11, fontPickerMuted)
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !selected {
						return layout.Dimensions{}
					}
					gtx.Constraints.Min = image.Point{}
					return homeIcon(gtx, "check", 14, fontPickerInk)
				}))
			})
		})
	}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if row.online == nil {
			return layout.Dimensions{}
		}
		return button.download.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(gtx.Dp(64), gtx.Dp(44))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				status := p.downloads[row.online.ID]
				if status.running {
					value := a.textID("common.action.cancel")
					if status.total > 0 {
						value = fmt.Sprintf("%d%%", status.received*100/status.total)
					}
					return a.fontPickerLabel(gtx, value, 11, fontPickerMuted)
				}
				if status.err != "" {
					return a.fontPickerLabel(gtx, a.textID("common.action.retry"), 11, fontPickerInk)
				}
				return homeIcon(gtx, "download", 16, color.NRGBA{R: 37, G: 99, B: 169, A: 255})
			})
		})
	}))
}
