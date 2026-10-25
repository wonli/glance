package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	proofreadSourceCanvas = color.NRGBA{R: 233, G: 237, B: 241, A: 255}
	proofreadXCanvas      = color.NRGBA{R: 243, G: 245, B: 248, A: 255}
	proofreadInk          = color.NRGBA{R: 36, G: 43, B: 52, A: 255}
	proofreadMuted        = color.NRGBA{R: 105, G: 115, B: 126, A: 255}
	proofreadLine         = color.NRGBA{R: 213, G: 219, B: 226, A: 255}
	proofreadBlue         = color.NRGBA{R: 39, G: 103, B: 180, A: 255}
)

func (a *App) layoutProofreadingWorkspace(gtx layout.Context) layout.Dimensions {
	defer a.queueProofreadingViewPersistence(gtx)
	a.acceptProofreadingConfirmation()
	state := a.proofreading
	if state == nil {
		return layout.Dimensions{}
	}
	a.processProofreadingOCRScan(gtx)
	w := a.ensureProofreadingWorkbench()
	if w.historyOpen {
		return a.layoutProofreadingHistoryPage(gtx)
	}
	if state.controller != nil {
		a.processProofreadingHistoryKeys(gtx)
		a.processProofreadingEditorEvents(gtx)
		a.acceptProofreadingModelLayout()
		a.processProofreadingLayoutRequests()
		a.processProofreadingMenuEvents(gtx)
	}
	if a.clientTitlebar {
		a.processProofreadingWindowControlClicks(gtx)
	}
	if a.pdfDoc != nil {
		a.handlePDFEvents(gtx)
	}
	var content layout.Widget
	if state.bDocument == nil {
		content = func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutProofreadingTabChrome),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Flexed(1, a.layoutProofreadingComparePanes),
						layout.Rigid(a.layoutProofreadingEmptyFilmstrip),
					)
				}),
				layout.Rigid(a.layoutProofreadingStatus),
			)
		}
	} else {
		content = func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutProofreadingTabChrome),
				layout.Rigid(a.layoutProofreadingToolbar),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Flexed(1, a.layoutProofreadingComparePanes),
						layout.Rigid(a.layoutProofreadingPagination),
					)
				}),
				layout.Rigid(a.layoutProofreadingStatus),
			)
		}
	}
	return content(gtx)
}

func (a *App) processProofreadingWindowControlClicks(gtx layout.Context) {
	if a.window == nil {
		return
	}
	if a.windowCloseButton.Clicked(gtx) {
		a.window.Perform(system.ActionClose)
	}
	if a.windowMinimizeButton.Clicked(gtx) {
		a.window.Perform(system.ActionMinimize)
	}
	if a.windowMaximizeButton.Clicked(gtx) {
		a.window.Perform(windowModeToggleAction(a.windowMode))
	}
}

func (a *App) layoutProofreadingComparePanes(gtx layout.Context) layout.Dimensions {
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutProofreadingPane(gtx, proofreadingRoleA)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutProofreadingPane(gtx, proofreadingRoleX)
				}),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			dividerX := gtx.Constraints.Max.X / 2
			paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Min: image.Pt(dividerX, 0), Max: image.Pt(dividerX+max(1, gtx.Dp(1)), gtx.Constraints.Max.Y)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)
}

func (a *App) layoutProofreadingPane(gtx layout.Context, role proofreadingDocumentRole) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutProofreadingPaneHeader(gtx, role)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if role == proofreadingRoleA {
				return a.layoutProofreadingSourcePage(gtx)
			}
			return a.layoutProofreadingObjectPage(gtx)
		}),
	)
}

func (a *App) layoutProofreadingPaneHeader(gtx layout.Context, role proofreadingDocumentRole) layout.Dimensions {
	state := a.proofreading
	gtx.Constraints.Min.Y = gtx.Dp(44)
	gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			background := proofreadSourceCanvas
			if role == proofreadingRoleX {
				background = proofreadXCanvas
			}
			paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			name := a.textID("proofread.role.a")
			if role == proofreadingRoleX {
				name = a.textID("proofread.role.x")
			}
			return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(17)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Dp(16)
						gtx.Constraints = layout.Exact(image.Pt(size, size))
						icon, ok := svgIconTintAtSize("home", "file-text", max(1, size), color.NRGBA{R: 94, G: 116, B: 136, A: 255})
						if !ok {
							return layout.Dimensions{Size: image.Pt(size, size)}
						}
						return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain}.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							label := material.Label(a.theme, 13, name)
							label.Color = proofreadInk
							label.Font.Weight = font.SemiBold
							return layoutProofreadingOpticallyCenteredLabel(gtx, label)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if role != proofreadingRoleX || state.bDocument == nil {
							return layout.Dimensions{}
						}
						return a.layoutProofreadingXTextSizeButton(gtx, state, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if role != proofreadingRoleX || state.bDocument == nil {
							return layout.Dimensions{}
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(6), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if role != proofreadingRoleX || state.bDocument == nil {
							return layout.Dimensions{}
						}
						return a.layoutProofreadingXTextSizeButton(gtx, state, -1)
					}),
				)
			})
		},
	)
}

func (a *App) layoutProofreadingSourcePage(gtx layout.Context) layout.Dimensions {
	if a.proofreading.sourceLoading {
		return a.layoutProofreadingPageMessage(gtx, a.textID("proofread.loading.source"), false)
	}
	if a.proofreading.sourceError != "" {
		return a.layoutProofreadingPageMessage(gtx, a.proofreading.sourceError, true)
	}
	if a.pdfDoc == nil {
		return a.layoutProofreadingPageMessage(gtx, a.textID("proofread.source.unavailable"), true)
	}
	// Source loading sets the initial fit. Keep fit cleared after a pinch so the
	// shared A/X zoom remains under the reader's control.
	return a.layoutPDFCanvas(gtx)
}

func (a *App) layoutProofreadingObjectPage(gtx layout.Context) layout.Dimensions {
	state := a.proofreading
	if state.bDocument == nil {
		return a.layoutProofreadingEmptyObject(gtx)
	}
	if state.draftRestoring {
		return a.layoutProofreadingProjectionPages(gtx, a.textID("proofread.loading.recovery"), false)
	}
	if state.draftLoading {
		return a.layoutProofreadingProjectionPages(gtx, a.textID("proofread.loading.object"), false)
	}
	if state.draftError != "" {
		return a.layoutProofreadingProjectionPages(gtx, state.draftError, true)
	}
	if state.frameLoading && len(state.xPages) == 0 {
		return a.layoutProofreadingProjectionPages(gtx, a.textID("proofread.loading.frames"), false)
	}
	if state.frameError != "" && len(state.xPages) == 0 {
		return a.layoutProofreadingProjectionPages(gtx, state.frameError, true)
	}
	return a.layoutProofreadingProjectionPages(gtx, "", false)
}

func (a *App) layoutProofreadingPageMessage(gtx layout.Context, message string, isError bool) layout.Dimensions {
	return a.layoutProofreadingPageMessageOnCanvas(gtx, message, isError, proofreadSourceCanvas)
}

func (a *App) layoutProofreadingPageMessageOnCanvas(gtx layout.Context, message string, isError bool, canvas color.NRGBA) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, canvas, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(a.theme, message)
				style.Color = proofreadMuted
				if isError {
					style.Color = color.NRGBA{R: 157, G: 61, B: 54, A: 255}
				}
				style.TextSize = unit.Sp(13)
				return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, style.Layout)
			})
		},
	)
}
