package desktopui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
)

var (
	pdfMergeSplitPale = color.NRGBA{R: 255, G: 246, B: 239, A: 255}
	pdfMergeSplitLine = color.NRGBA{R: 229, G: 232, B: 236, A: 255}
	pdfMergeSplitInk  = color.NRGBA{R: 37, G: 43, B: 51, A: 255}
	pdfMergeSplitMute = color.NRGBA{R: 126, G: 136, B: 148, A: 255}
)

func (a *App) layoutPDFMergeSplitTool(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	return state.pageList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(26), Bottom: unit.Dp(32), Left: unit.Dp(32), Right: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutPDFMergeSplitHeader(gtx) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitModeTabs(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if state.mode == pdfMergeSplitModeSplit {
							return a.layoutPDFMergeSplitSplitDraft(gtx)
						}
						return a.layoutPDFMergeSplitMergeDraft(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, a.layoutPDFMergeSplitStatus)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "info", unit.Dp(16), pdfMergeSplitMute)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									style := material.Body2(a.theme, a.textID("pdf_merge_split.help.content_boundary"))
									style.TextSize = unit.Sp(12)
									style.Color = pdfMergeSplitMute
									style.MaxLines = 2
									return style.Layout(gtx)
								})
							}),
						)
					})
				}),
			)
		})
	})
}

func (a *App) layoutPDFMergeSplitHeader(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitTextButton(gtx, a.theme, &state.backButton, "arrow-left", a.textID("pdf_merge_split.back"), false, state.busy)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.title"), 27, pdfMergeSplitInk, true)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.subtitle"), 14, pdfMergeSplitMute, false)
						})
					}),
				)
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitModeTabs(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitTextButton(gtx, a.theme, &state.mergeModeButton, "file-text", a.textID("pdf_merge_split.mode.merge"), state.mode == pdfMergeSplitModeMerge, state.busy)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitTextButton(gtx, a.theme, &state.splitModeButton, "scan-pages", a.textID("pdf_merge_split.mode.split"), state.mode == pdfMergeSplitModeSplit, state.busy)
		}),
	)
}

func (a *App) layoutPDFMergeSplitMergeDraft(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool.mergeDraft
	return pdfMergeSplitPanel(gtx, 438, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(22), Bottom: unit.Dp(20), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.merge.files"), 16, pdfMergeSplitInk, true)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return pdfMergeSplitTextButton(gtx, a.theme, &state.clearButton, "trash-2", a.textID("pdf_merge_split.merge.clear"), false, a.pdfMergeSplitTool.busy)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitMergeFiles(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitOutputFields(gtx, &state.outputName, state.outputFolder, &state.outputButton, "pdf_merge_split.merge.output_name")
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitMergeSummary(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return pdfMergeSplitTextButton(gtx, a.theme, &state.mergeButton, "file-text", a.textID("pdf_merge_split.run.merge"), true, a.pdfMergeSplitTool.busy)
					})
				}),
			)
		})
	})
}

func (a *App) layoutPDFMergeSplitMergeFiles(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool.mergeDraft
	locale := a.localeSnapshot().Resolved
	if len(state.files) == 0 {
		return a.layoutPDFMergeSplitEmptyFiles(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return state.fileList.Layout(gtx, len(state.files), func(gtx layout.Context, index int) layout.Dimensions {
				file := state.files[index]
				return layoutPDFMergeSplitFileRow(gtx, a.theme, locale, file, state.fileButtons[file.identity], true, a.pdfMergeSplitTool.busy)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, a.layoutPDFMergeSplitAddDropZone)
		}),
	)
}

func (a *App) layoutPDFMergeSplitEmptyFiles(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool.mergeDraft
	return a.pdfMergeSplitDropZone(gtx, &state.addButton, "file", "pdf_merge_split.merge.empty", "pdf_merge_split.merge.add", a.pdfMergeSplitTool.busy)
}

func (a *App) layoutPDFMergeSplitAddDropZone(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool.mergeDraft
	return a.pdfMergeSplitDropZone(gtx, &state.addButton, "plus", "pdf_merge_split.merge.empty", "pdf_merge_split.merge.add", a.pdfMergeSplitTool.busy)
}

func (a *App) pdfMergeSplitDropZone(gtx layout.Context, button *widget.Clickable, icon, descriptionKey, actionKey string, disabled bool) layout.Dimensions {
	width := gtx.Constraints.Max.X
	height := gtx.Dp(unit.Dp(112))
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		background, border, foreground := pdfMergeSplitPale, color.NRGBA{R: 244, G: 209, B: 183, A: 255}, pdfMergeSplitAccent
		if disabled {
			background, border, foreground = color.NRGBA{R: 245, G: 246, B: 247, A: 255}, pdfMergeSplitLine, pdfMergeSplitMute
		}
		shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(10)))
		paint.FillShape(gtx.Ops, background, shape.Op(gtx.Ops))
		paintRoundedBorder(gtx.Ops, shape, float32(max(1, gtx.Dp(unit.Dp(1)))), border)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, icon, unit.Dp(23), foreground)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, a.textID(descriptionKey), 13, pdfMergeSplitMute, false)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, a.textID(actionKey), 13, foreground, true)
					})
				}),
			)
		})
	})
}

func layoutPDFMergeSplitFileRow(gtx layout.Context, theme *material.Theme, locale i18n.Locale, file pdfMergeSplitFile, buttons *pdfMergeSplitFileButtons, reorder bool, disabled bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(54))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return homeRoundedPanel(gtx, color.NRGBA{R: 250, G: 251, B: 252, A: 255}, pdfMergeSplitLine, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, "file", unit.Dp(20), pdfMergeSplitAccent)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, theme, file.name, 13, pdfMergeSplitInk, true)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, theme, formatPDFUtilityFileSummary(file.pageCount, file.sizeBytes, locale), 11, pdfMergeSplitMute, false)
							}),
						)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if buttons == nil {
						return layout.Dimensions{}
					}
					if reorder {
						return layoutPDFMergeSplitDragHandle(gtx, buttons)
					}
					return homeIconButton(gtx, &buttons.remove, "x", unit.Dp(32), pdfMergeSplitMute, disabled)
				}),
			)
		})
	})
}

func layoutPDFMergeSplitDragHandle(gtx layout.Context, buttons *pdfMergeSplitFileButtons) layout.Dimensions {
	size := gtx.Dp(unit.Dp(32))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	return buttons.grab.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		buttons.drag.Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, "grip-vertical", unit.Dp(18), pdfMergeSplitMute)
		})
	})
}

func (a *App) layoutPDFMergeSplitOutputFields(gtx layout.Context, editor *widget.Editor, folder string, folderButton *widget.Clickable, nameKey string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPDFMergeSplitTextField(gtx, editor, nameKey, "")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.output_folder"), 12, pdfMergeSplitMute, false)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := a.textID("pdf_merge_split.output_folder.unselected")
								if folder != "" {
									label = folder
								}
								return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, label, 13, pdfMergeSplitInk, false)
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return pdfMergeSplitTextButton(gtx, a.theme, folderButton, "folder-open", a.textID("pdf_merge_split.output_folder.choose"), false, a.pdfMergeSplitTool.busy)
					}),
				)
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitTextField(gtx layout.Context, editor *widget.Editor, labelKey, hint string) layout.Dimensions {
	editor.ReadOnly = a.pdfMergeSplitTool.busy
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID(labelKey), 12, pdfMergeSplitMute, false)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(42)))
				gtx.Constraints = layout.Exact(size)
				settingsFieldFocus(gtx, editor)
				shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(7)))
				paint.FillShape(gtx.Ops, homeSurface, shape.Op(gtx.Ops))
				paintRoundedBorder(gtx.Ops, shape, float32(max(1, gtx.Dp(unit.Dp(1)))), pdfMergeSplitLine)
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						style := material.Editor(a.theme, editor, hint)
						style.TextSize = unit.Sp(13)
						style.Color = pdfMergeSplitInk
						style.HintColor = pdfMergeSplitMute
						return style.Layout(gtx)
					}))
				})
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitMergeSummary(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.mergeDraft
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.merge.summary_pages"), formatPDFUtilityNumber(draft.totalPageCount))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.merge.summary_size"), formatPDFUtilityBytes(draft.totalSizeBytes))
		}),
	)
}

func pdfMergeSplitSummaryItem(gtx layout.Context, theme *material.Theme, title, value string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, theme, title, 11, pdfMergeSplitMute, false)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, theme, value, 13, pdfMergeSplitInk, true)
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitSplitDraft(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	return pdfMergeSplitPanel(gtx, 484, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(22), Bottom: unit.Dp(20), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.split.source"), 16, pdfMergeSplitInk, true)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitSplitSource(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return pdfMergeSplitTextButton(gtx, a.theme, &draft.rangesButton, "list-minus", a.textID("pdf_merge_split.split.by_ranges"), draft.rangeMode == pdfMergeSplitSplitModeRanges, a.pdfMergeSplitTool.busy)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return pdfMergeSplitTextButton(gtx, a.theme, &draft.byPageButton, "scan-pages", a.textID("pdf_merge_split.split.by_page"), draft.rangeMode == pdfMergeSplitSplitModeByPage, a.pdfMergeSplitTool.busy)
							}),
						)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if draft.rangeMode == pdfMergeSplitSplitModeByPage {
						return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							count := "0"
							if draft.source != nil {
								count = formatPDFUtilityNumber(draft.source.pageCount)
							}
							return homeTextLabel(gtx, a.theme, a.textArgs("pdf_merge_split.split.range_preview", i18n.Args{"Count": count}), 13, pdfMergeSplitMute, false)
						})
					}
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitTextField(gtx, &draft.rangeInput, "pdf_merge_split.split.ranges", a.textID("pdf_merge_split.split.ranges.placeholder"))
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if draft.rangeMode == pdfMergeSplitSplitModeByPage {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if draft.rangeError != "" {
							return homeTextLabel(gtx, a.theme, draft.rangeError, 12, color.NRGBA{R: 173, G: 54, B: 48, A: 255}, false)
						}
						return homeTextLabel(gtx, a.theme, a.textArgs("pdf_merge_split.split.range_preview", i18n.Args{"Count": formatPDFUtilityNumber(draft.rangeCount)}), 12, pdfMergeSplitMute, false)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitTextField(gtx, &draft.prefix, "pdf_merge_split.split.prefix", "")
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitOutputFolder(gtx, draft.outputFolder, &draft.outputButton)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return pdfMergeSplitTextButton(gtx, a.theme, &draft.splitButton, "scan-pages", a.textID("pdf_merge_split.run.split"), true, a.pdfMergeSplitTool.busy)
					})
				}),
			)
		})
	})
}

func (a *App) layoutPDFMergeSplitSplitSource(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	if draft.source == nil {
		return a.pdfMergeSplitDropZone(gtx, &draft.selectButton, "file", "pdf_merge_split.split.empty", "pdf_merge_split.split.select", a.pdfMergeSplitTool.busy)
	}
	return layoutPDFMergeSplitFileRow(gtx, a.theme, a.localeSnapshot().Resolved, *draft.source, nil, false, true)
}

func (a *App) layoutPDFMergeSplitStatus(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	children := make([]layout.FlexChild, 0, len(state.lastOutputs)+3)
	if state.busy {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.status.busy"), 13, pdfMergeSplitMute, false)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitTextButton(gtx, a.theme, &state.cancelButton, "x", a.textID("pdf_merge_split.status.cancel"), false, false)
				}),
			)
		}))
	}
	if state.lastError != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, state.lastError, 13, color.NRGBA{R: 173, G: 54, B: 48, A: 255}, false)
		}))
	} else if state.cancelled {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.status.cancelled"), 13, pdfMergeSplitMute, false)
		}))
	} else if len(state.lastOutputs) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.status.success"), 13, color.NRGBA{R: 49, G: 121, B: 78, A: 255}, false)
		}))
	}
	if (state.lastError != "" || state.cancelled) && len(state.lastOutputs) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.status.partial"), 12, pdfMergeSplitMute, false)
		}))
	}
	if len(state.lastOutputs) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			rowHeight := gtx.Dp(unit.Dp(24))
			height := min(len(state.lastOutputs)*rowHeight, gtx.Dp(unit.Dp(144)))
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			return state.outputList.Layout(gtx, len(state.lastOutputs), func(gtx layout.Context, index int) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, state.lastOutputs[index], 12, pdfMergeSplitInk, false)
			})
		}))
	}
	if len(children) == 0 {
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
}

func (a *App) layoutPDFMergeSplitOutputFolder(gtx layout.Context, folder string, button *widget.Clickable) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.output_folder"), 12, pdfMergeSplitMute, false)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := a.textID("pdf_merge_split.output_folder.unselected")
					if folder != "" {
						label = folder
					}
					return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, label, 13, pdfMergeSplitInk, false)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitTextButton(gtx, a.theme, button, "folder-open", a.textID("pdf_merge_split.output_folder.choose"), false, a.pdfMergeSplitTool.busy)
		}),
	)
}

func pdfMergeSplitPanel(gtx layout.Context, height unit.Dp, child layout.Widget) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(height))
	gtx.Constraints = layout.Exact(size)
	return homeRoundedPanel(gtx, homeSurface, pdfMergeSplitLine, unit.Dp(12), child)
}

func pdfMergeSplitTextButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, icon, label string, selected, disabled bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(42))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	if button == nil {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
	}
	fg := pdfMergeSplitInk
	background := color.NRGBA{R: 249, G: 250, B: 251, A: 255}
	border := pdfMergeSplitLine
	if selected {
		fg = pdfMergeSplitAccent
		background = pdfMergeSplitPale
		border = color.NRGBA{R: 239, G: 186, B: 151, A: 255}
	}
	if disabled {
		fg = pdfMergeSplitMute
		background = color.NRGBA{R: 245, G: 246, B: 247, A: 255}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, background, border, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, icon, unit.Dp(17), fg) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						style := material.Body2(theme, label)
						style.TextSize = unit.Sp(13)
						style.Color = fg
						style.Font.Weight = font.Medium
						style.MaxLines = 1
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, style.Layout)
					}),
				)
			})
		})
	})
}

func formatPDFUtilityFileSummary(pages int, size int64, locale i18n.Locale) string {
	pageLabel := "pages"
	if locale == i18n.LocaleChinese {
		pageLabel = "页"
	}
	return formatPDFUtilityNumber(pages) + " " + pageLabel + " · " + formatPDFUtilityBytes(size)
}

func formatPDFUtilityNumber(value int) string {
	if value < 0 {
		value = 0
	}
	return strconv.Itoa(value)
}

func formatPDFUtilityBytes(size int64) string {
	if size < 0 {
		size = 0
	}
	if size < 1024 {
		return strconv.FormatInt(size, 10) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(size)
	index := -1
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + units[index]
}

func (a *App) layoutHomeToolsPDFMergeSplitCard(gtx layout.Context) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(720)))
	if width <= 0 {
		return layout.Dimensions{}
	}
	size := image.Pt(width, gtx.Dp(unit.Dp(200)))
	gtx.Constraints = layout.Exact(size)
	button := a.homeNavButton("tools-pdf-merge-split")
	if button.Clicked(gtx) {
		a.pdfMergeSplitToolPage = true
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(14), func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutPDFMergeSplitEntryIcon(gtx)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "chevron-right", unit.Dp(18), homeMuted)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(13)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, a.textID("home.tools.pdf_merge_split.title"), 20, homeToolsTextTitle, true)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Body2(a.theme, a.textID("home.tools.pdf_merge_split.description"))
							style.TextSize = unit.Sp(14)
							style.Color = homeMuted
							style.MaxLines = 2
							return style.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, a.textID("home.tools.pdf_merge_split.action"), 13, pdfMergeSplitAccent, true)
						})
					}),
				)
			})
		})
	})
}

func layoutPDFMergeSplitEntryIcon(gtx layout.Context) layout.Dimensions {
	size := gtx.Dp(unit.Dp(38))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	shape := clip.UniformRRect(image.Rect(0, 0, size, size), gtx.Dp(unit.Dp(8)))
	paint.FillShape(gtx.Ops, pdfMergeSplitPale, shape.Op(gtx.Ops))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeIcon(gtx, "file", unit.Dp(21), pdfMergeSplitAccent)
	})
}
