package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
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
	overlayGtx := gtx
	a.processProofreadingOCRBatchControls(gtx)
	a.processProofreadingOCRScan(gtx)
	a.processProofreadingOCRStep(gtx)
	a.processProofreadingOCRBatch(gtx)
	gtx = a.proofreadingOCRBatchContentContext(gtx)
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
	if a.documentSessionState != nil && a.sharedDocumentSession != nil && a.pdfDoc != nil {
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
	return layout.Stack{Alignment: layout.Center}.Layout(overlayGtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return content(a.proofreadingOCRBatchContentContext(gtx)) }),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.layoutProofreadingDiffInspector(a.proofreadingOCRBatchContentContext(gtx))
		}),
		layout.Expanded(a.layoutProofreadingOCRBatchDialog),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if a.proofreading != nil && a.proofreading.exceptionsModalOpen {
				return a.layoutProofreadingSessionExceptionsDialog(gtx)
			}
			return layout.Dimensions{}
		}),
	)
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
	reserveNavigator := a.pdfDoc != nil && a.proofreading != nil && a.proofreading.bDocument != nil
	splitX := proofreadingComparePaneSplit(gtx, reserveNavigator)
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = splitX
					gtx.Constraints.Max.X = splitX
					return a.layoutProofreadingPane(gtx, proofreadingRoleA)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutProofreadingPane(gtx, proofreadingRoleX)
				}),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			dividerX := proofreadingComparePaneSplit(gtx, reserveNavigator)
			paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Min: image.Pt(dividerX, 0), Max: image.Pt(dividerX+max(1, gtx.Dp(1)), gtx.Constraints.Max.Y)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			a.drawProofreadingOCRStepConnector(gtx)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)
}

// The X pane reserves a fixed right strip for page navigation. Split the
// remaining width evenly so the A canvas and X page list have equal space.
func proofreadingComparePaneSplit(gtx layout.Context, reserveNavigator bool) int {
	width := gtx.Constraints.Max.X
	if reserveNavigator {
		width -= gtx.Dp(proofreadingNavigatorWidthDP)
	}
	return max(0, width) / 2
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
						return layout.Dimensions{Size: image.Pt(gtx.Dp(6), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if role != proofreadingRoleX || state.bDocument == nil {
							return layout.Dimensions{}
						}
						return a.layoutProofreadingXTextSizeButton(gtx, state, -1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if role != proofreadingRoleX || state.bDocument == nil || !state.aiProofreading {
							return layout.Dimensions{}
						}
						return layout.Dimensions{Size: image.Pt(gtx.Dp(8), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if role != proofreadingRoleX {
							return layout.Dimensions{}
						}
						return a.layoutProofreadingOCRDiffHeaderControls(gtx)
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
	if a.documentSessionState == nil || a.sharedDocumentSession == nil || a.pdfDoc == nil {
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

func (a *App) layoutProofreadingSessionExceptionsDialog(gtx layout.Context) layout.Dimensions {
	state := a.proofreading
	if state == nil || !state.exceptionsModalOpen {
		return layout.Dimensions{}
	}
	step := a.proofreadingOCRStep()
	if step == nil {
		return layout.Dimensions{}
	}

	w, h := gtx.Constraints.Max.X, gtx.Constraints.Max.Y

	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if keyEv, isKey := ev.(key.Event); isKey && keyEv.State == key.Press {
			state.exceptionsModalOpen = false
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	for state.closeExceptionsBtn.Clicked(gtx) {
		state.exceptionsModalOpen = false
		gtx.Execute(op.InvalidateCmd{})
	}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 15, G: 23, B: 42, A: 140}, clip.Rect{Max: image.Pt(w, h)}.Op())

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		modalW := min(gtx.Dp(480), w-gtx.Dp(32))
		modalH := min(gtx.Dp(320), h-gtx.Dp(32))
		gtx.Constraints = layout.Exact(image.Pt(modalW, modalH))

		mRect := image.Rect(0, 0, modalW, modalH)
		mArea := clip.UniformRRect(mRect, gtx.Dp(8)).Push(gtx.Ops)
		defer mArea.Pop()

		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(mRect, gtx.Dp(8)).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, color.NRGBA{R: 203, G: 213, B: 225, A: 255}, clip.Stroke{
			Path:  clip.UniformRRect(mRect, gtx.Dp(8)).Path(gtx.Ops),
			Width: float32(max(1, gtx.Dp(1))),
		}.Op())

		if state.addExceptionBtn.Clicked(gtx) {
			text := strings.TrimSpace(state.exceptionInput.Text())
			if text != "" {
				for _, r := range text {
					step.inspector.exceptions.AddCharException(r)
				}
				state.exceptionInput.SetText("")
				a.applyExceptionsToCurrentPage(true)
			}
			gtx.Execute(op.InvalidateCmd{})
		}

		rules := step.inspector.exceptions.Rules
		for len(state.removeExceptionBtns) < len(rules) {
			state.removeExceptionBtns = append(state.removeExceptionBtns, widget.Clickable{})
		}

		return layout.Inset{Top: unit.Dp(16), Bottom: unit.Dp(16), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									t := material.Label(a.theme, 13, "对比例外管理")
									t.Font.Weight = font.Bold
									t.Color = color.NRGBA{R: 15, G: 23, B: 42, A: 255}
									return t.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									st := material.Label(a.theme, 10, "添加到例外列表的字将在逐字扫描比对前直接排除，不作为差异扫描。")
									st.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
									return st.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return state.closeExceptionsBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return a.layoutInspectorHeaderIconButton(gtx, "home", "x", "关闭例外管理")
							})
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(14))}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Background{}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									r := gtx.Dp(4)
									paint.FillShape(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 252, A: 255}, clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Dp(32)), r).Op(gtx.Ops))
									paint.FillShape(gtx.Ops, color.NRGBA{R: 203, G: 213, B: 225, A: 255}, clip.Stroke{
										Path:  clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Dp(32)), r).Path(gtx.Ops),
										Width: float32(max(1, gtx.Dp(1))),
									}.Op())
									return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(32))}
								},
								func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(a.theme, &state.exceptionInput, "输入要排除的任意字，如「（」或特定字符...")
										ed.TextSize = 11
										return ed.Layout(gtx)
									})
								},
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Dp(8), 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return state.addExceptionBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								btnW, btnH := gtx.Dp(96), gtx.Dp(32)
								gtx.Constraints = layout.Exact(image.Pt(btnW, btnH))
								paint.FillShape(gtx.Ops, color.NRGBA{R: 26, G: 115, B: 232, A: 255}, clip.UniformRRect(image.Rect(0, 0, btnW, btnH), gtx.Dp(4)).Op(gtx.Ops))
								lbl := material.Label(a.theme, 11, "+ 添加到例外")
								lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
								lbl.Font.Weight = font.Bold
								return layoutProofreadingInspectorCenteredLabel(gtx, image.Pt(btnW, btnH), lbl)
							})
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(14))}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(rules) == 0 {
						emptyLbl := material.Label(a.theme, 11, "当前对比会话暂无例外字。可在上方输入添加，或在差异处理弹窗中点击「添加到例外」。")
						emptyLbl.Color = color.NRGBA{R: 148, G: 163, B: 184, A: 255}
						return layout.Center.Layout(gtx, emptyLbl.Layout)
					}
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							r := gtx.Dp(4)
							rect := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
							paint.FillShape(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 252, A: 255}, clip.UniformRRect(rect, r).Op(gtx.Ops))
							return layout.Dimensions{Size: rect.Size()}
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								const perRow = 5
								var rowChildren []layout.FlexChild
								for r := 0; r < len(rules); r += perRow {
									end := min(r+perRow, len(rules))
									rowSlice := rules[r:end]
									startIdx := r
									rowChildren = append(rowChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										var tags []layout.FlexChild
										for j, rule := range rowSlice {
											ruleIdx := startIdx + j
											if state.removeExceptionBtns[ruleIdx].Clicked(gtx) {
												a.removeProofreadingDiffException(ruleIdx)
												gtx.Execute(op.InvalidateCmd{})
											}
											currRule := rule
											btn := &state.removeExceptionBtns[ruleIdx]
											tags = append(tags, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Right: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return a.layoutInspectorExceptionRuleTag(gtx, btn, currRule)
												})
											}))
										}
										return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, tags...)
									}))
								}
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)
							})
						},
					)
				}),
			)
		})
	})
}
