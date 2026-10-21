package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (a *App) layoutAgentLocalAccessModal(gtx layout.Context) layout.Dimensions {
	consent := a.activeAgentLocalAccessConsent()
	if consent == nil {
		return layout.Dimensions{}
	}
	if consent.scrim.Clicked(gtx) || consent.reject.Clicked(gtx) {
		a.resolveAgentLocalAccessConsent(consent, false)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	if consent.allow.Clicked(gtx) {
		a.resolveAgentLocalAccessConsent(consent, true)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return consent.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, agentPermissionBackdrop, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentLocalAccessCard(gtx, consent)
		}),
	)
}

func (a *App) layoutAgentLocalAccessCard(gtx layout.Context, consent *agentLocalAccessConsent) layout.Dimensions {
	cardWidth := gtx.Dp(unit.Dp(560))
	if maxWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(32)); maxWidth < cardWidth {
		cardWidth = maxWidth
	}
	if cardWidth <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWidth, cardWidth
	gtx.Constraints.Min.Y = 0

	rec := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(24), Bottom: unit.Dp(24), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Dp(unit.Dp(40))
						gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(size, size), image.Pt(size, size)
						shape := clip.UniformRRect(image.Rectangle{Max: image.Pt(size, size)}, gtx.Dp(unit.Dp(10)))
						paint.FillShape(gtx.Ops, agentPermissionShieldBg, shape.Op(gtx.Ops))
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeIcon(gtx, "shield-check", unit.Dp(22), agentPermissionShieldIconFg)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := homeLabelStyle(a.theme, a.textID("agent.local_access.modal.title"), unit.Sp(18), agentPermissionTitleFg, font.SemiBold)
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := homeLabelStyle(a.theme, a.textID("agent.local_access.modal.subtitle"), unit.Sp(12), agentPermissionSubtitleFg, font.Normal)
								return lbl.Layout(gtx)
							}),
						)
					}),
				)
			}),
			layout.Rigid(localAccessGap(gtx, 20)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return localAccessLongLabel(gtx, a.theme, a.textID("agent.local_access.modal.purpose_label"), unit.Sp(12), agentPermissionSubtitleFg, font.Medium)
			}),
			layout.Rigid(localAccessGap(gtx, 5)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return localAccessLongLabel(gtx, a.theme, consent.request.Purpose, unit.Sp(14), agentPermissionDescFg, font.Normal)
			}),
			layout.Rigid(localAccessGap(gtx, 14)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if consent.request.UserRequestExcerpt == "" {
					return layout.Dimensions{}
				}
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(8)))
						paint.FillShape(gtx.Ops, agentPermissionScopeBg, shape.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Min}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := homeLabelStyle(a.theme, a.textID("agent.local_access.modal.request_label"), unit.Sp(11), agentPermissionSubtitleFg, font.Medium)
									return lbl.Layout(gtx)
								}),
								layout.Rigid(localAccessGap(gtx, 4)),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return localAccessLongLabel(gtx, a.theme, consent.request.UserRequestExcerpt, unit.Sp(12), agentPermissionDescFg, font.Normal)
								}),
							)
						})
					},
				)
			}),
			layout.Rigid(localAccessGap(gtx, 14)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(8)))
						paint.FillShape(gtx.Ops, agentPermissionScopeBg, shape.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Min}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return homeIcon(gtx, "folder", unit.Dp(20), agentPermissionFolderIconFg)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return localAccessLongLabel(gtx, a.theme, consent.request.FolderName, unit.Sp(13), agentPermissionFolderNameFg, font.Medium)
										}),
										layout.Rigid(localAccessGap(gtx, 3)),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return localAccessLongLabel(gtx, a.theme, consent.request.FolderPath, unit.Sp(12), agentPermissionFolderPathFg, font.Normal)
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return localAccessLongLabel(gtx, a.theme, a.textID("agent.local_access.modal.read_only"), unit.Sp(11), agentPermissionBadgeFg, font.Medium)
								}),
							)
						})
					},
				)
			}),
			layout.Rigid(localAccessGap(gtx, 14)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := homeLabelStyle(a.theme, a.textID("agent.local_access.modal.data_flow"), unit.Sp(11), agentPermissionNoticeFg, font.Normal)
				lbl.MaxLines = 0
				return lbl.Layout(gtx)
			}),
			layout.Rigid(localAccessGap(gtx, 18)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btnHeight := unit.Dp(40)
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layoutModalActionButton(gtx, a.theme, &consent.reject, a.textID("agent.local_access.modal.reject"), 0, btnHeight,
							color.NRGBA{R: 255, G: 255, B: 255, A: 255}, agentPermissionRejectBdr, agentLightweightRejectFg, "")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(10)), 0)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layoutModalActionButton(gtx, a.theme, &consent.allow, a.textID("agent.local_access.modal.allow_once"), 0, btnHeight,
							agentPermissionOnceBg, agentPermissionOnceBg, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, "")
					}),
				)
			}),
		)
	})
	call := rec.Stop()

	bounds := image.Rectangle{Max: dims.Size}
	radius := gtx.Dp(unit.Dp(14))
	for _, shadow := range []struct {
		offset int
		alpha  uint8
	}{{12, 14}, {6, 18}, {2, 24}} {
		shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(shadow.offset))))
		shape := clip.UniformRRect(shadowBounds, radius)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 16, G: 24, B: 40, A: shadow.alpha}, shape.Op(gtx.Ops))
	}
	cardShape := clip.UniformRRect(bounds, radius)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, cardShape.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, agentPermissionBorder, clip.Stroke{Path: cardShape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	clipScope := cardShape.Push(gtx.Ops)
	call.Add(gtx.Ops)
	clipScope.Pop()
	return dims
}

func localAccessGap(gtx layout.Context, dp unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(0, gtx.Dp(dp))}
	}
}

func localAccessLongLabel(gtx layout.Context, theme *material.Theme, text string, size unit.Sp, fg color.NRGBA, weight font.Weight) layout.Dimensions {
	style := homeLabelStyle(theme, text, size, fg, weight)
	style.MaxLines = 0
	return style.Layout(gtx)
}
