package desktopui

import (
	"image"
	"image/color"
	"strconv"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
)

var (
	pdfMergeSplitCanvas           = color.NRGBA{R: 251, G: 251, B: 252, A: 255}
	pdfMergeSplitPale             = color.NRGBA{R: 255, G: 240, B: 235, A: 255}
	pdfMergeSplitLine             = color.NRGBA{R: 226, G: 230, B: 235, A: 255}
	pdfMergeSplitInk              = color.NRGBA{R: 35, G: 40, B: 48, A: 255}
	pdfMergeSplitMute             = color.NRGBA{R: 119, G: 129, B: 142, A: 255}
	pdfMergeSplitGreen            = color.NRGBA{R: 66, G: 131, B: 114, A: 255}
	pdfMergeSplitEntryBorder      = color.NRGBA{R: 227, G: 230, B: 235, A: 255}
	pdfMergeSplitEntryDescription = color.NRGBA{R: 105, G: 117, B: 131, A: 255}
	pdfMergeSplitEntryArrow       = color.NRGBA{R: 166, G: 174, B: 184, A: 255}
	pdfMergeSplitEntryHint        = color.NRGBA{R: 113, G: 123, B: 136, A: 255}
)

const (
	pdfMergeSplitWorkbenchMaxWidth        unit.Dp = 1120
	pdfMergeSplitWorkbenchPageMinHeight   unit.Dp = 858 // 900dp design canvas minus the 42dp workspace tab strip.
	pdfMergeSplitReorderAnimationDuration         = 160 * time.Millisecond
)

func (a *App) layoutPDFMergeSplitTool(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	pageHeight := max(gtx.Constraints.Max.Y, gtx.Dp(pdfMergeSplitWorkbenchPageMinHeight))
	return state.pageList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		// A vertical List gives each item an effectively infinite main-axis
		// maximum. Bound this single workbench page so scrolling can't move it
		// hundreds of pixels out of view.
		gtx.Constraints.Min.Y = 0
		gtx.Constraints.Max.Y = pageHeight
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, pdfMergeSplitCanvas, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				width := min(gtx.Constraints.Max.X, gtx.Dp(pdfMergeSplitWorkbenchMaxWidth))
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(52)}.Layout(gtx, a.layoutPDFMergeSplitTitle)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return a.layoutPDFMergeSplitModeTabs(gtx)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
								return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, a.layoutPDFMergeSplitContentBoundaryNote)
							}),
						)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
				)
			},
		)
	})
}

func (a *App) layoutPDFMergeSplitTitle(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.title"), 28, pdfMergeSplitInk, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.subtitle"), 14, pdfMergeSplitMute, false)
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitModeTabs(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	width := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(352)))
	height := gtx.Dp(unit.Dp(44))
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(10)))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 237, G: 240, B: 243, A: 255}, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitModeSegment(gtx, &state.mergeModeButton, "files", a.textID("pdf_merge_split.mode.merge"), state.mode == pdfMergeSplitModeMerge, state.busy)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitModeSegment(gtx, &state.splitModeButton, "scissors", a.textID("pdf_merge_split.mode.split"), state.mode == pdfMergeSplitModeSplit, state.busy)
					}),
				)
			})
		},
	)
}

func (a *App) layoutPDFMergeSplitModeSegment(gtx layout.Context, button *widget.Clickable, icon, label string, selected, disabled bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	background, fg := color.NRGBA{A: 0}, color.NRGBA{R: 101, G: 113, B: 126, A: 255}
	if selected {
		background, fg = pdfMergeSplitPale, color.NRGBA{R: 185, G: 85, B: 59, A: 255}
	}
	if disabled {
		fg = pdfMergeSplitMute
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, background, background, unit.Dp(7), func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, icon, unit.Dp(16), fg) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, label, 13, fg, selected)
							})
						})
					}),
				)
			})
		})
	})
}

func (a *App) layoutPDFMergeSplitContentBoundaryNote(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, "info", unit.Dp(14), pdfMergeSplitMute)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(a.theme, a.textID("pdf_merge_split.help.content_boundary"))
					style.TextSize = unit.Sp(11)
					style.Color = pdfMergeSplitMute
					style.MaxLines = 2
					return style.Layout(gtx)
				})
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitMergeDraft(gtx layout.Context) layout.Dimensions {
	return a.layoutPDFMergeSplitWorkbenchPanel(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(650, a.layoutPDFMergeSplitMergeLeft),
			layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return pdfMergeSplitDivider(gtx, unit.Dp(506)) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(23)}.Layout),
			layout.Flexed(374, a.layoutPDFMergeSplitOutputSettingsMerge),
		)
	})
}

func (a *App) layoutPDFMergeSplitOutputSettingsMerge(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	draft := &state.mergeDraft
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.merge.settings"), 15, pdfMergeSplitInk, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitTextField(gtx, &draft.outputName, "pdf_merge_split.merge.output_name", "")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.merge.output_name.hint"), 10, pdfMergeSplitMute, false)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitOutputFolder(gtx, draft.outputFolder, &draft.outputButton)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitMergeSummaryCard(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitInfoBar(gtx, "info", a.textID("pdf_merge_split.merge.output_hint"))
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitPrimaryButton(gtx, a.theme, &draft.mergeButton, "files", a.textID("pdf_merge_split.run.merge"), len(draft.files) < 2 || draft.outputFolder == "" || state.busy)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.output.local_save"), 10, pdfMergeSplitMute, false)
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitOutputSettingsSplit(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool
	draft := &state.splitDraft
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.split.settings"), 15, pdfMergeSplitInk, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitTextField(gtx, &draft.prefix, "pdf_merge_split.split.prefix", "")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.split.prefix.hint"), 10, pdfMergeSplitMute, false)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitOutputFolder(gtx, draft.outputFolder, &draft.outputButton)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitSplitSummaryCard(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitInfoBar(gtx, "info", a.textID("pdf_merge_split.split.order_hint"))
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitPrimaryButton(gtx, a.theme, &draft.splitButton, "scissors", a.textID("pdf_merge_split.run.split"), draft.source == nil || draft.rangeError != "" || draft.outputFolder == "" || state.busy)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.output.local_save"), 10, pdfMergeSplitMute, false)
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitWorkbenchPanel(gtx layout.Context, child layout.Widget) layout.Dimensions {
	width, height := gtx.Constraints.Max.X, gtx.Dp(unit.Dp(554))
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	return homeRoundedPanel(gtx, homeSurface, pdfMergeSplitLine, unit.Dp(14), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(24), Bottom: unit.Dp(24), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, child)
	})
}

func pdfMergeSplitDivider(gtx layout.Context, height unit.Dp) layout.Dimensions {
	widthPx, heightPx := gtx.Dp(unit.Dp(1)), gtx.Dp(height)
	size := image.Pt(widthPx, heightPx)
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 233, G: 236, B: 240, A: 255}, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

func (a *App) layoutPDFMergeSplitMergeLeft(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool.mergeDraft
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPDFMergeSplitFilesHeading(gtx, a.textID("pdf_merge_split.merge.files"), len(state.files), &state.clearButton, a.textID("pdf_merge_split.merge.clear"))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.pdfMergeSplitDropZone(gtx, &state.addButton, "files", "pdf_merge_split.merge.drop.title", "pdf_merge_split.merge.drop.subtitle", "pdf_merge_split.merge.drop.action", a.pdfMergeSplitTool.busy, false)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, a.layoutPDFMergeSplitMergeFiles)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitInfoBar(gtx, "info", a.textID("pdf_merge_split.merge.order_hint"))
			})
		}),
	)
}

func pdfMergeSplitNaturalHeight(gtx layout.Context, child layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	return child(gtx)
}

func (a *App) layoutPDFMergeSplitFilesHeading(gtx layout.Context, title string, count int, clearButton *widget.Clickable, clearLabel string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(26))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, title, 15, color.NRGBA{R: 48, G: 57, B: 68, A: 255}, true)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return pdfMergeSplitCountPill(gtx, a.theme, a.textArgs("pdf_merge_split.files.count", i18n.Args{"Count": formatPDFUtilityNumber(count)}))
					})
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(24))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				return pdfMergeSplitPlainButton(gtx, a.theme, clearButton, clearLabel, false)
			})
		}),
	)
}

func pdfMergeSplitCountPill(gtx layout.Context, theme *material.Theme, label string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(22))
	gtx.Constraints.Min.X = 0
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	style := material.Body2(theme, label)
	style.TextSize = unit.Sp(11)
	style.Color = color.NRGBA{R: 107, G: 117, B: 129, A: 255}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			width := gtx.Constraints.Min.X
			gtx.Constraints = layout.Exact(image.Pt(width, height))
			shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, height/2)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 242, G: 244, B: 246, A: 255}, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(9), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return pdfMergeSplitNaturalHeight(gtx, style.Layout)
					})
				}),
			)
		},
	)
}

func pdfMergeSplitPlainButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, disabled bool) layout.Dimensions {
	if button == nil {
		return layout.Dimensions{}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		fg := color.NRGBA{R: 138, G: 148, B: 159, A: 255}
		if disabled {
			fg = pdfMergeSplitMute
		}
		return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := material.Body2(theme, label)
			style.TextSize = unit.Sp(12)
			style.Color = fg
			style.Font.Weight = font.Medium
			return style.Layout(gtx)
		})
	})
}

func (a *App) layoutPDFMergeSplitInfoBar(gtx layout.Context, icon, message string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(30))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return homeRoundedPanel(gtx, color.NRGBA{R: 248, G: 249, B: 250, A: 255}, color.NRGBA{A: 0}, unit.Dp(7), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, icon, unit.Dp(14), color.NRGBA{R: 138, G: 148, B: 159, A: 255})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, message, 11, color.NRGBA{R: 121, G: 131, B: 143, A: 255}, false)
						})
					})
				}),
			)
		})
	})
}

func (a *App) layoutPDFMergeSplitMergeFiles(gtx layout.Context) layout.Dimensions {
	state := &a.pdfMergeSplitTool.mergeDraft
	if len(state.files) == 0 {
		return layout.Dimensions{}
	}
	files := state.files
	if active := pdfMergeSplitActiveDrag(state); active != nil {
		files = pdfMergeSplitMoveFileSlice(files, active.dragStartIndex, active.dragTargetIndex)
	}
	return state.fileList.Layout(gtx, len(files), func(gtx layout.Context, index int) layout.Dimensions {
		file := files[index]
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.layoutPDFMergeSplitFileRow(gtx, file, state.fileButtons[file.identity], true, a.pdfMergeSplitTool.busy)
		})
	})
}

func (a *App) pdfMergeSplitDropZone(gtx layout.Context, button *widget.Clickable, icon, titleKey, subtitleKey, actionKey string, disabled, greenAction bool) layout.Dimensions {
	width := gtx.Constraints.Max.X
	height := gtx.Dp(unit.Dp(108))
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		background, border, foreground := color.NRGBA{R: 251, G: 252, B: 253, A: 255}, color.NRGBA{R: 226, G: 231, B: 236, A: 255}, pdfMergeSplitAccent
		if disabled {
			background, border, foreground = color.NRGBA{R: 245, G: 246, B: 247, A: 255}, pdfMergeSplitLine, pdfMergeSplitMute
		}
		shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(10)))
		paint.FillShape(gtx.Ops, background, shape.Op(gtx.Ops))
		paintRoundedBorder(gtx.Ops, shape, float32(max(1, gtx.Dp(unit.Dp(1)))), border)
		return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					iconSize := gtx.Dp(unit.Dp(42))
					gtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
					return homeRoundedPanel(gtx, pdfMergeSplitPale, pdfMergeSplitPale, unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, icon, unit.Dp(19), foreground) })
					})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.textID(titleKey), 13, color.NRGBA{R: 52, G: 61, B: 72, A: 255}, true)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, a.textID(subtitleKey), 11, color.NRGBA{R: 138, G: 148, B: 159, A: 255}, false)
								})
							}),
						)
					})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitChooseFileButton(gtx, a.theme, a.textID(actionKey), greenAction, disabled)
				}),
			)
		})
	})
}

func pdfMergeSplitChooseFileButton(gtx layout.Context, theme *material.Theme, label string, green, disabled bool) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(96)), gtx.Dp(unit.Dp(34))
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	background, border, foreground := homeSurface, color.NRGBA{R: 221, G: 226, B: 232, A: 255}, color.NRGBA{R: 79, G: 90, B: 103, A: 255}
	if green {
		background, border, foreground = color.NRGBA{R: 234, G: 244, B: 241, A: 255}, color.NRGBA{R: 200, G: 229, B: 219, A: 255}, pdfMergeSplitGreen
	}
	if disabled {
		background, border, foreground = color.NRGBA{R: 245, G: 246, B: 247, A: 255}, pdfMergeSplitLine, pdfMergeSplitMute
	}
	return homeRoundedPanel(gtx, background, border, unit.Dp(7), func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := material.Body2(theme, label)
			style.TextSize = unit.Sp(12)
			style.Color = foreground
			style.Font.Weight = font.SemiBold
			return style.Layout(gtx)
		})
	})
}

func (a *App) layoutPDFMergeSplitFileRow(gtx layout.Context, file pdfMergeSplitFile, buttons *pdfMergeSplitFileButtons, reorder bool, disabled bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(72))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	rowOffset, liftProgress := float32(0), float32(0)
	animating := false
	if buttons != nil {
		if buttons.dragStarted {
			rowOffset = buttons.dragOffsetY
		} else if buttons.drag.Pressed() {
			rowOffset = buttons.dragPressVisualOffsetY
		} else {
			rowOffset, animating = pdfMergeSplitReorderOffsetAt(buttons, gtx.Now)
		}
		var liftAnimating bool
		liftProgress, liftAnimating = pdfMergeSplitDragLiftAt(buttons, gtx.Now)
		animating = animating || liftAnimating
	}
	translation := op.Offset(image.Pt(0, int(rowOffset))).Push(gtx.Ops)
	defer translation.Pop()
	if animating {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 60)})
	}
	if liftProgress > 0 {
		pdfMergeSplitFileRowShadow(gtx, liftProgress)
	}
	return homeRoundedPanel(gtx, homeSurface, color.NRGBA{R: 230, G: 233, B: 238, A: 255}, unit.Dp(9), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(13), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, 10)
			if reorder {
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layoutPDFMergeSplitDragHandle(gtx, buttons) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				)
			}
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(unit.Dp(40))
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(36)), size))
					return homeRoundedPanel(gtx, color.NRGBA{R: 255, G: 242, B: 238, A: 255}, color.NRGBA{A: 0}, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeIcon(gtx, "file-text", unit.Dp(18), pdfMergeSplitAccent)
						})
					})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, file.name, 13, color.NRGBA{R: 48, G: 57, B: 68, A: 255}, true)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, a.textArgs("pdf_merge_split.file_detail", i18n.Args{"Pages": formatPDFUtilityNumber(file.pageCount)}), 11, color.NRGBA{R: 135, G: 145, B: 157, A: 255}, false)
								})
							}),
						)
					})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, formatPDFUtilityBytes(file.sizeBytes), 12, color.NRGBA{R: 86, G: 97, B: 109, A: 255}, true)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, a.textArgs("pdf_merge_split.file.pages", i18n.Args{"Pages": formatPDFUtilityNumber(file.pageCount)}), 10, color.NRGBA{R: 144, G: 154, B: 165, A: 255}, false)
								})
							}),
						)
					})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			)
			if buttons != nil {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return pdfMergeSplitRemoveButton(gtx, &buttons.remove, disabled)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

func pdfMergeSplitRemoveButton(gtx layout.Context, button *widget.Clickable, disabled bool) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(26)), gtx.Dp(unit.Dp(30)))
	gtx.Constraints = layout.Exact(size)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, "x", unit.Dp(17), pdfMergeSplitMute)
		})
	})
}

func layoutPDFMergeSplitDragHandle(gtx layout.Context, buttons *pdfMergeSplitFileButtons) layout.Dimensions {
	hitSize := image.Pt(gtx.Dp(unit.Dp(16)), gtx.Dp(unit.Dp(32)))
	gtx.Constraints = layout.Exact(hitSize)
	return buttons.grab.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if buttons.dragStarted {
			pointer.CursorGrabbing.Add(gtx.Ops)
		} else {
			pointer.CursorGrab.Add(gtx.Ops)
		}
		buttons.drag.Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Dp(unit.Dp(16)), gtx.Dp(unit.Dp(24)))
			gtx.Constraints = layout.Exact(size)
			dotSize := max(1, gtx.Dp(unit.Dp(3)))
			columnGap := gtx.Dp(unit.Dp(3))
			columnHeight := gtx.Dp(unit.Dp(18))
			left := (size.X - 2*dotSize - columnGap) / 2
			top := (size.Y - columnHeight) / 2
			grip := color.NRGBA{R: 167, G: 176, B: 186, A: 255}
			for column := range 2 {
				for row := range 3 {
					x := left + column*(dotSize+columnGap)
					y := top + row*(columnHeight-dotSize)/2
					bounds := image.Rect(x, y, x+dotSize, y+dotSize)
					paint.FillShape(gtx.Ops, grip, clip.Ellipse(bounds).Op(gtx.Ops))
				}
			}
			return layout.Dimensions{Size: size}
		})
	})
}

func pdfMergeSplitActiveDrag(state *pdfMergeSplitMergeDraft) *pdfMergeSplitFileButtons {
	if state == nil {
		return nil
	}
	for _, file := range state.files {
		buttons := state.fileButtons[file.identity]
		if buttons != nil && buttons.dragStarted {
			return buttons
		}
	}
	return nil
}

func pdfMergeSplitReorderOffsetAt(buttons *pdfMergeSplitFileButtons, now time.Time) (float32, bool) {
	if buttons == nil || buttons.reorderAnimationStart.IsZero() {
		return 0, false
	}
	progress := float32(now.Sub(buttons.reorderAnimationStart)) / float32(pdfMergeSplitReorderAnimationDuration)
	if progress >= 1 {
		return 0, false
	}
	progress = max(0, progress)
	eased := pdfMergeSplitEaseOut(progress)
	return buttons.reorderOffsetY * (1 - eased), true
}

func pdfMergeSplitDragLiftAt(buttons *pdfMergeSplitFileButtons, now time.Time) (float32, bool) {
	if buttons == nil {
		return 0, false
	}
	if buttons.dragStarted {
		if buttons.dragLiftStartedAt.IsZero() {
			return 1, false
		}
		progress := float32(now.Sub(buttons.dragLiftStartedAt)) / float32(pdfMergeSplitReorderAnimationDuration)
		if progress >= 1 {
			return 1, false
		}
		return pdfMergeSplitEaseOut(max(0, progress)), true
	}
	if buttons.dragDropStartedAt.IsZero() {
		return 0, false
	}
	progress := float32(now.Sub(buttons.dragDropStartedAt)) / float32(pdfMergeSplitReorderAnimationDuration)
	if progress >= 1 {
		return 0, false
	}
	return 1 - pdfMergeSplitEaseOut(max(0, progress)), true
}

func pdfMergeSplitEaseOut(progress float32) float32 {
	remaining := 1 - max(0, min(1, progress))
	return 1 - remaining*remaining*remaining
}

func pdfMergeSplitFileRowShadow(gtx layout.Context, progress float32) {
	if gtx.Constraints.Max.X <= 0 {
		return
	}
	height := gtx.Dp(unit.Dp(72))
	distance := gtx.Dp(unit.Dp(2 + 3*progress))
	shape := clip.UniformRRect(image.Rect(0, distance, gtx.Constraints.Max.X, height+distance), gtx.Dp(unit.Dp(9)))
	alpha := uint8(10 + 24*progress)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 48, G: 57, B: 68, A: alpha}, shape.Op(gtx.Ops))
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

func (a *App) layoutPDFMergeSplitMergeSummaryCard(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.mergeDraft
	return pdfMergeSplitSummaryCard(gtx, a.theme, a.textID("pdf_merge_split.merge.summary.title"), [3]layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.merge.summary_files"), formatPDFUtilityNumber(len(draft.files)))
		},
		func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.merge.summary_pages"), formatPDFUtilityNumber(draft.totalPageCount))
		},
		func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.merge.summary_size"), formatPDFUtilityBytes(draft.totalSizeBytes))
		},
	})
}

func (a *App) layoutPDFMergeSplitSplitSummaryCard(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	count, size := 0, int64(0)
	if draft.source != nil {
		size = draft.source.sizeBytes
		if draft.rangeMode == pdfMergeSplitSplitModeByPage {
			count = draft.source.pageCount
		} else {
			count = draft.rangeCount
		}
	}
	return pdfMergeSplitSummaryCard(gtx, a.theme, a.textID("pdf_merge_split.split.summary.title"), [3]layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.split.summary_files"), formatPDFUtilityNumber(count))
		},
		func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.split.summary_pages"), formatPDFUtilityNumber(draft.rangeCount))
		},
		func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitSummaryItem(gtx, a.theme, a.textID("pdf_merge_split.split.summary_size"), formatPDFUtilityBytes(size))
		},
	})
}

func pdfMergeSplitSummaryCard(gtx layout.Context, theme *material.Theme, title string, items [3]layout.Widget) layout.Dimensions {
	height := gtx.Dp(unit.Dp(112))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return homeRoundedPanel(gtx, color.NRGBA{R: 249, G: 250, B: 251, A: 255}, pdfMergeSplitLine, unit.Dp(9), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(12), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeTextLabel(gtx, theme, title, 12, pdfMergeSplitInk, true)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, items[0]),
						layout.Flexed(1, items[1]),
						layout.Flexed(1, items[2]),
					)
				}),
			)
		})
	})
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

func pdfMergeSplitPrimaryButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, icon, label string, disabled bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(48))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	if button == nil {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
	}
	background, foreground := pdfMergeSplitAccent, color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if disabled {
		background, foreground = color.NRGBA{R: 239, G: 194, B: 178, A: 255}, color.NRGBA{R: 255, G: 247, B: 244, A: 255}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, background, background, unit.Dp(9), func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, icon, unit.Dp(17), foreground) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Body2(theme, label)
							style.TextSize, style.Color, style.Font.Weight = unit.Sp(13), foreground, font.SemiBold
							return pdfMergeSplitNaturalHeight(gtx, style.Layout)
						})
					}),
				)
			})
		})
	})
}

func (a *App) layoutPDFMergeSplitSplitDraft(gtx layout.Context) layout.Dimensions {
	return a.layoutPDFMergeSplitWorkbenchPanel(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(650, a.layoutPDFMergeSplitSplitLeft),
			layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return pdfMergeSplitDivider(gtx, unit.Dp(506)) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(23)}.Layout),
			layout.Flexed(374, a.layoutPDFMergeSplitOutputSettingsSplit),
		)
	})
}

func (a *App) layoutPDFMergeSplitSplitLeft(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	var clearButton *widget.Clickable
	if draft.source != nil {
		clearButton = &draft.clearButton
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPDFMergeSplitFilesHeading(gtx, a.textID("pdf_merge_split.split.source"), boolPDFMergeSplitSourceCount(draft.source), clearButton, a.textID("pdf_merge_split.split.clear"))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, a.layoutPDFMergeSplitSplitSource)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPDFMergeSplitSplitSettingsCard(gtx)
			})
		}),
	)
}

func boolPDFMergeSplitSourceCount(source *pdfMergeSplitFile) int {
	if source == nil {
		return 0
	}
	return 1
}

func (a *App) layoutPDFMergeSplitSplitSettingsCard(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	height := gtx.Dp(unit.Dp(204))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return homeRoundedPanel(gtx, color.NRGBA{R: 249, G: 250, B: 251, A: 255}, pdfMergeSplitLine, unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.split.method"), 12, pdfMergeSplitMute, true)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(9)}.Layout(gtx, a.layoutPDFMergeSplitRangeModeTabs)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if draft.rangeMode == pdfMergeSplitSplitModeRanges {
						return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutPDFMergeSplitRangeEditor(gtx)
						})
					}
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						count := 0
						if draft.source != nil {
							count = draft.source.pageCount
						}
						return homeTextLabel(gtx, a.theme, a.textArgs("pdf_merge_split.split.by_page_hint", i18n.Args{"Count": formatPDFUtilityNumber(count)}), 11, pdfMergeSplitMute, false)
					})
				}),
			)
		})
	})
}

func (a *App) layoutPDFMergeSplitRangeModeTabs(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, height)
			shape := clip.UniformRRect(image.Rectangle{Max: size}, height/2)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 238, G: 241, B: 244, A: 255}, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(3), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitModeSegment(gtx, &draft.rangesButton, "list-minus", a.textID("pdf_merge_split.split.mode_ranges_short"), draft.rangeMode == pdfMergeSplitSplitModeRanges, a.pdfMergeSplitTool.busy)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutPDFMergeSplitModeSegment(gtx, &draft.byPageButton, "scan-pages", a.textID("pdf_merge_split.split.mode_page_short"), draft.rangeMode == pdfMergeSplitSplitModeByPage, a.pdfMergeSplitTool.busy)
					}),
				)
			})
		},
	)
}

func (a *App) layoutPDFMergeSplitRangeEditor(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.split.ranges"), 11, pdfMergeSplitMute, false)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(36))
				size := image.Pt(gtx.Constraints.Max.X, height)
				gtx.Constraints = layout.Exact(size)
				settingsFieldFocus(gtx, &draft.rangeInput)
				shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(6)))
				paint.FillShape(gtx.Ops, homeSurface, shape.Op(gtx.Ops))
				paintRoundedBorder(gtx.Ops, shape, float32(max(1, gtx.Dp(unit.Dp(1)))), pdfMergeSplitLine)
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						style := material.Editor(a.theme, &draft.rangeInput, a.textID("pdf_merge_split.split.ranges.placeholder"))
						style.TextSize, style.Color, style.HintColor = unit.Sp(12), pdfMergeSplitInk, pdfMergeSplitMute
						return style.Layout(gtx)
					}))
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			message := a.textArgs("pdf_merge_split.split.range_preview", i18n.Args{"Count": formatPDFUtilityNumber(draft.rangeCount)})
			fg := pdfMergeSplitMute
			if draft.rangeError != "" {
				message, fg = draft.rangeError, color.NRGBA{R: 173, G: 54, B: 48, A: 255}
			}
			return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, message, 10, fg, false)
			})
		}),
	)
}

func (a *App) layoutPDFMergeSplitSplitSource(gtx layout.Context) layout.Dimensions {
	draft := &a.pdfMergeSplitTool.splitDraft
	height := gtx.Dp(unit.Dp(72))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return draft.selectButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, homeSurface, pdfMergeSplitLine, unit.Dp(9), func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Dp(unit.Dp(40))
						gtx.Constraints = layout.Exact(image.Pt(size, size))
						return homeRoundedPanel(gtx, color.NRGBA{R: 255, G: 242, B: 238, A: 255}, color.NRGBA{A: 0}, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "file-text", unit.Dp(18), pdfMergeSplitAccent)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
							if draft.source == nil {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.split.empty"), 13, pdfMergeSplitInk, true)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeTextLabel(gtx, a.theme, a.textID("pdf_merge_split.split.select"), 11, pdfMergeSplitMute, false)
										})
									}),
								)
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, draft.source.name, 13, pdfMergeSplitInk, true)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return homeTextLabel(gtx, a.theme, a.textArgs("pdf_merge_split.file_detail", i18n.Args{"Pages": formatPDFUtilityNumber(draft.source.pageCount)}), 11, pdfMergeSplitMute, false)
									})
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := a.textID("pdf_merge_split.split.select")
						if draft.source != nil {
							label = a.textID("pdf_merge_split.split.replace")
						}
						return pdfMergeSplitChooseFileButton(gtx, a.theme, label, false, a.pdfMergeSplitTool.busy)
					}),
				)
			})
		})
	})
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
					return pdfMergeSplitCompactTextButton(gtx, a.theme, &state.cancelButton, "x", a.textID("pdf_merge_split.status.cancel"), false, false, unit.Dp(112))
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
			return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
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
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return pdfMergeSplitCompactTextButton(gtx, a.theme, button, "folder-open", a.textID("pdf_merge_split.output_folder.choose"), false, a.pdfMergeSplitTool.busy, unit.Dp(144))
		}),
	)
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
						return pdfMergeSplitNaturalHeight(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, style.Layout)
						})
					}),
				)
			})
		})
	})
}

func pdfMergeSplitCompactTextButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, icon, label string, selected, disabled bool, width unit.Dp) layout.Dimensions {
	widthPx := gtx.Dp(width)
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx
	return pdfMergeSplitTextButton(gtx, theme, button, icon, label, selected, disabled)
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
	width := gtx.Constraints.Max.X
	if width <= 0 {
		return layout.Dimensions{}
	}
	size := image.Pt(width, gtx.Dp(unit.Dp(218)))
	gtx.Constraints = layout.Exact(size)
	button := a.homeNavButton("tools-pdf-merge-split")
	if button.Clicked(gtx) {
		a.openPDFMergeSplitToolTab()
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, homeSurface, pdfMergeSplitEntryBorder, unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(22), Bottom: unit.Dp(22), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutPDFMergeSplitEntryIcon(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "arrow-up-right", unit.Dp(17), pdfMergeSplitEntryArrow)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, a.textID("home.tools.pdf_merge_split.title"), 17, homeToolsTextTitle, true)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Body2(a.theme, a.textID("home.tools.pdf_merge_split.description"))
							style.TextSize = unit.Sp(12)
							style.Color = pdfMergeSplitEntryDescription
							style.MaxLines = 2
							style.LineHeightScale = 1.65
							return style.Layout(gtx)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Stack{Alignment: layout.SW}.Layout(gtx,
							layout.Expanded(func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: gtx.Constraints.Max}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.textID("home.tools.pdf_merge_split.features"), 11, pdfMergeSplitEntryHint, false)
							}),
						)
					}),
				)
			})
		})
	})
}

func layoutPDFMergeSplitEntryIcon(gtx layout.Context) layout.Dimensions {
	size := gtx.Dp(unit.Dp(40))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	shape := clip.UniformRRect(image.Rect(0, 0, size, size), gtx.Dp(unit.Dp(10)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 241, B: 236, A: 255}, shape.Op(gtx.Ops))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeIcon(gtx, "files", unit.Dp(22), color.NRGBA{R: 222, G: 112, B: 83, A: 255})
	})
}
