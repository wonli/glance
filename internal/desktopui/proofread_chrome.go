package desktopui

import (
	"image"
	"image/color"
	"runtime"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	proofreadTabBarFill  = color.NRGBA{R: 238, G: 241, B: 244, A: 255}
	proofreadChromeLine  = color.NRGBA{R: 221, G: 226, B: 231, A: 255}
	proofreadToolbarFill = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	proofreadStatusFill  = color.NRGBA{R: 248, G: 249, B: 250, A: 255}
)

// layoutProofreadingOpticallyCenteredLabel aligns visible text ink with icons
// in fixed-height horizontal rows. Gio centers the label's layout box, which
// leaves the glyph ink high when the row's minimum height reaches the label.
func layoutProofreadingOpticallyCenteredLabel(gtx layout.Context, label material.LabelStyle) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	return layout.Inset{Top: proofreadingOpticalLabelInset(gtx, label.TextSize)}.Layout(gtx, label.Layout)
}

func proofreadingOpticalLabelInset(gtx layout.Context, textSize unit.Sp) unit.Dp {
	baseFontPixels := gtx.Sp(15)
	textPixels := gtx.Sp(textSize)
	if baseFontPixels <= 0 || gtx.Metric.PxPerDp <= 0 {
		return 0
	}
	baseInsetPixels := float32(gtx.Dp(homeTextOpticalCenterTopInset))
	return unit.Dp(baseInsetPixels * float32(textPixels) / float32(baseFontPixels) / gtx.Metric.PxPerDp)
}

func (a *App) layoutProofreadingChrome(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(42)
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			paint.FillShape(gtx.Ops, proofreadTabBarFill, clip.Rect{Max: size}.Op())
			paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Min: image.Pt(0, size.Y-gtx.Dp(1)), Max: size}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.NW}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return layoutWindowMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: gtx.Constraints.Max}
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(24)}.Layout(gtx, a.layoutProofreadingChromeItems)
				}),
			)
		},
	)
}

func (a *App) layoutProofreadingChromeItems(gtx layout.Context) layout.Dimensions {
	children := make([]layout.FlexChild, 0, 10)
	if a.clientTitlebar && runtime.GOOS == "darwin" {
		children = append(children, layout.Rigid(a.layoutMacWindowControls))
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := a.ensureProofreadingWorkbench()
			b := w.button("workbench-tab")
			return b.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(g layout.Context) layout.Dimensions {
						return layoutProofreadingIcon(g, "ribbon", "files", 20, color.NRGBA{R: 66, G: 109, B: 145, A: 255})
					}),
					layout.Rigid(func(g layout.Context) layout.Dimensions {
						return layout.Inset{Left: 10}.Layout(g, func(g layout.Context) layout.Dimensions {
							label := material.Label(a.theme, 16, a.textID("proofread.workspace.name"))
							label.Font.Weight = font.SemiBold
							return layoutProofreadingOpticallyCenteredLabel(g, label)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(gtx.Dp(30), 0)} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.ensureProofreadingWorkbench().button("manuscript-tab").Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layoutProofreadingActiveTab(gtx, a) })
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutProofreadingIcon(gtx, "home", "plus", 16, color.NRGBA{R: 123, G: 132, B: 144, A: 255})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			text := a.textID("proofread.workspace.status.empty")
			if a.proofreading != nil && a.proofreading.bDocument != nil {
				text = a.textID(proofreadingWorkspaceStatusKey(a.proofreading))
			}
			label := material.Label(a.theme, 11, text)
			label.Color = color.NRGBA{R: 102, G: 113, B: 125, A: 255}
			label.MaxLines = 1
			return layout.Inset{Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutProofreadingOpticallyCenteredLabel(gtx, label)
			})
		}),
		layout.Rigid(a.layoutProofreadingHistoryAction),
	)
	if a.clientTitlebar && runtime.GOOS != "darwin" {
		children = append(children, layout.Rigid(a.layoutStandardWindowControls))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func layoutProofreadingActiveTab(gtx layout.Context, a *App) layout.Dimensions {
	size := image.Pt(gtx.Dp(164), gtx.Dp(41))
	gtx.Constraints = layout.Exact(size)
	labelKey := "proofread.workspace.tab.empty"
	if a.proofreading != nil && a.proofreading.bDocument != nil {
		labelKey = "proofread.workspace.tab.loaded"
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 247, G: 248, B: 250, A: 255}, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(5)).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Max: image.Pt(size.X, gtx.Dp(1))}.Op())
			paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Max: image.Pt(gtx.Dp(1), size.Y)}.Op())
			paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Min: image.Pt(size.X-gtx.Dp(1), 0), Max: size}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(size)
			label := material.Label(a.theme, 12, a.textID(labelKey))
			label.Color = color.NRGBA{R: 47, G: 94, B: 131, A: 255}
			label.Font.Weight = font.SemiBold
			label.MaxLines = 1
			return layout.Center.Layout(gtx, label.Layout)
		},
	)
}

func (a *App) layoutProofreadingToolbar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(54)
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			paint.FillShape(gtx.Ops, proofreadToolbarFill, clip.Rect{Max: size}.Op())
			paint.FillShape(gtx.Ops, proofreadLine, clip.Rect{Min: image.Pt(0, size.Y-gtx.Dp(1)), Max: size}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			if a.proofreading == nil || a.proofreading.bDocument == nil {
				message := "proofread.toolbar.progress.empty"
				if a.proofreading != nil && a.proofreading.sourceLoading {
					message = "proofread.toolbar.progress.source_loading"
				}
				return layout.Inset{Left: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Label(a.theme, 11, a.textID(message))
					label.Color = color.NRGBA{R: 104, G: 120, B: 135, A: 255}
					return label.Layout(gtx)
				})
			}
			return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(30)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutProofreadingToolbarPill(gtx, a, 118, "check", "proofread.toolbar.reviewed_filter")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(gtx.Dp(6), 0)} }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutProofreadingToolbarPill(gtx, a, 126, "clock-3", "proofread.toolbar.unreviewed_filter")
					}),
				)
			})
		},
	)
}

func layoutProofreadingToolbarPill(gtx layout.Context, a *App, width unit.Dp, iconName, labelKey string) layout.Dimensions {
	if a != nil && a.proofreading != nil {
		var button *widget.Clickable
		var requested proofreadingPageFilter
		switch labelKey {
		case "proofread.toolbar.reviewed_filter":
			button, requested = &a.proofreading.filterReviewedButton, proofreadingPageFilterReviewed
		case "proofread.toolbar.unreviewed_filter":
			button, requested = &a.proofreading.filterUnreviewedButton, proofreadingPageFilterUnreviewed
		}
		if button != nil {
			if button.Clicked(gtx) {
				a.setProofreadingPageFilter(requested)
				a.clearProofreadingFocusOutsideFilter(gtx)
			}
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutProofreadingToolbarPillVisual(gtx, a, width, iconName, labelKey)
			})
		}
	}
	return layoutProofreadingToolbarPillVisual(gtx, a, width, iconName, labelKey)
}

func layoutProofreadingToolbarPillVisual(gtx layout.Context, a *App, width unit.Dp, iconName, labelKey string) layout.Dimensions {
	size := image.Pt(gtx.Dp(width), gtx.Dp(32))
	labelSize := unit.Sp(11)
	leftInset, labelGap := unit.Dp(10), unit.Dp(7)
	labelInk := color.NRGBA{R: 150, G: 159, B: 168, A: 255}
	iconInk := labelInk
	fill := color.NRGBA{R: 250, G: 251, B: 252, A: 255}
	border := color.NRGBA{R: 226, G: 230, B: 234, A: 255}
	switch labelKey {
	case "proofread.toolbar.reviewed_filter":
		leftInset, labelGap = 22, 6
		labelInk = color.NRGBA{R: 105, G: 115, B: 126, A: 255}
		iconInk = color.NRGBA{R: 118, G: 130, B: 141, A: 255}
		fill = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		border = color.NRGBA{R: 213, G: 221, B: 229, A: 255}
		if a.proofreading != nil && a.proofreading.pageFilter == proofreadingPageFilterReviewed {
			labelInk = color.NRGBA{R: 47, G: 94, B: 131, A: 255}
			iconInk = labelInk
			fill = color.NRGBA{R: 239, G: 245, B: 251, A: 255}
			border = color.NRGBA{R: 157, G: 188, B: 215, A: 255}
		}
	case "proofread.toolbar.unreviewed_filter":
		labelSize = 12
		leftInset = 21.5
		labelInk = color.NRGBA{R: 105, G: 115, B: 126, A: 255}
		iconInk = color.NRGBA{R: 118, G: 130, B: 141, A: 255}
		fill = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		border = color.NRGBA{R: 213, G: 221, B: 229, A: 255}
		if a.proofreading != nil && a.proofreading.pageFilter == proofreadingPageFilterUnreviewed {
			labelInk = color.NRGBA{R: 166, G: 71, B: 62, A: 255}
			iconInk = color.NRGBA{R: 181, G: 78, B: 66, A: 255}
			fill = color.NRGBA{R: 255, G: 241, B: 237, A: 255}
			border = color.NRGBA{R: 233, G: 195, B: 187, A: 255}
		}
	}
	gtx.Constraints = layout.Exact(size)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(6)).Op(gtx.Ops))
			proofreadRoundedBorder(gtx, size, 6, border, max(1, gtx.Dp(1)))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: leftInset, Right: leftInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutProofreadingIcon(gtx, "ribbon", iconName, 15, iconInk)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: labelGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							label := material.Label(a.theme, labelSize, a.textID(labelKey))
							label.Color = labelInk
							label.Font.Weight = font.SemiBold
							label.MaxLines = 1
							return layoutProofreadingOpticallyCenteredLabel(gtx, label)
						})
					}),
				)
			})
		},
	)
}

func (a *App) layoutProofreadingStatus(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(34)
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			paint.FillShape(gtx.Ops, proofreadStatusFill, clip.Rect{Max: size}.Op())
			paint.FillShape(gtx.Ops, proofreadLine, clip.Rect{Max: image.Pt(size.X, max(1, gtx.Dp(1)))}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			labelKey := "proofread.status.waiting"
			if a.proofreading != nil && a.proofreading.bDocument != nil {
				labelKey = proofreadingStatusBarKey(a.proofreading)
			}
			return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutProofreadingIcon(gtx, "ribbon", "lock-keyhole", 15, color.NRGBA{R: 117, G: 128, B: 139, A: 255})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							message := a.textID(labelKey)
							if a.proofreading != nil && a.proofreading.history.notice != "" {
								message = a.proofreading.history.notice
							}
							label := material.Label(a.theme, 10, message)
							label.Color = color.NRGBA{R: 104, G: 115, B: 126, A: 255}
							return layoutProofreadingOpticallyCenteredLabel(gtx, label)
						})
					}),
				)
			})
		},
	)
}

func proofreadingWorkspaceStatusKey(state *proofreadingWorkspace) string {
	if state == nil || state.bDocument == nil {
		return "proofread.workspace.status.empty"
	}
	if state.draftRestoring {
		return "proofread.workspace.status.restoring"
	}
	if state.draftPersistError != "" {
		return "proofread.workspace.status.save_error"
	}
	if state.draftSavePending {
		return "proofread.workspace.status.saving"
	}
	if state.draftAutoSaved {
		return "proofread.workspace.status.saved"
	}
	return "proofread.workspace.status.loaded"
}

func proofreadingStatusBarKey(state *proofreadingWorkspace) string {
	if state == nil || state.bDocument == nil {
		return "proofread.status.waiting"
	}
	if state.draftRestoring {
		return "proofread.status.restoring"
	}
	if state.draftPersistError != "" {
		return "proofread.status.save_error"
	}
	if state.draftSavePending {
		return "proofread.status.saving"
	}
	if state.draftAutoSaved {
		return "proofread.status.saved"
	}
	return "proofread.status.loaded"
}

func layoutProofreadingIcon(gtx layout.Context, family, name string, size unit.Dp, tint color.NRGBA) layout.Dimensions {
	pixels := max(1, gtx.Dp(size))
	gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
	icon, ok := svgIconTintAtSize(family, name, pixels, tint)
	if !ok {
		for _, fallback := range []string{"word", "excel", "home", "ribbon"} {
			icon, ok = svgIconTintAtSize(fallback, name, pixels, tint)
			if ok {
				break
			}
		}
	}
	if !ok {
		return layout.Dimensions{Size: image.Pt(pixels, pixels)}
	}
	return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain}.Layout(gtx)
}

func (a *App) layoutProofreadingHistoryAction(g layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	b := w.button("workspace-history")
	if b.Clicked(g) && a.proofreading != nil && a.proofreading.projectDocumentID != "" {
		a.openProofreadingHistory(a.proofreadingProjectMetadata(), 0)
		g.Execute(op.InvalidateCmd{})
	}
	return b.Layout(g, func(g layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(g,
			layout.Rigid(func(g layout.Context) layout.Dimensions {
				return layoutProofreadingIcon(g, "ribbon", "history", 17, proofreadMuted)
			}),
			layout.Rigid(func(g layout.Context) layout.Dimensions {
				return layout.Inset{Left: 7}.Layout(g, material.Label(a.theme, 11, a.textID("proofread.workspace.history")).Layout)
			}),
		)
	})
}
