package desktopui

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/application"
	"glance/internal/i18n"
	"glance/ui"
)

var (
	// Node EV5Zd (轻量确认 · 插入文档)
	agentLightweightBackdrop   = color.NRGBA{R: 23, G: 32, B: 46, A: 0x4D}   // #17202E4D
	agentLightweightBorder     = color.NRGBA{R: 221, G: 228, B: 236, A: 255} // #DDE4EC
	agentLightweightPromptFg   = color.NRGBA{R: 48, G: 59, B: 73, A: 255}    // #303B49
	agentLightweightBarBg      = color.NRGBA{R: 246, G: 248, B: 251, A: 255} // #F6F8FB
	agentLightweightBarBorder  = color.NRGBA{R: 233, G: 237, B: 243, A: 255} // #E9EDF3
	agentLightweightAIBadgeBg  = color.NRGBA{R: 237, G: 242, B: 250, A: 255} // #EDF2FA
	agentLightweightAIBadgeBdr = color.NRGBA{R: 224, G: 232, B: 244, A: 255} // #E0E8F4
	agentLightweightAIIconFg   = color.NRGBA{R: 89, G: 116, B: 154, A: 255}  // #59749A
	agentLightweightRejectFg   = color.NRGBA{R: 86, G: 97, B: 112, A: 255}   // #566170
	agentLightweightRejectBdr  = color.NRGBA{R: 220, G: 227, B: 236, A: 255} // #DCE3EC
	agentLightweightApproveBg  = color.NRGBA{R: 52, G: 63, B: 80, A: 255}    // #343F50

	// Node yInjn (全局权限确认 · 底部居中)
	agentPermissionBackdrop     = color.NRGBA{R: 23, G: 32, B: 46, A: 0x66}   // #17202E66
	agentPermissionBorder       = color.NRGBA{R: 225, G: 229, B: 235, A: 255} // #E1E5EB
	agentPermissionShieldBg     = color.NRGBA{R: 238, G: 243, B: 249, A: 255} // #EEF3F9
	agentPermissionShieldIconFg = color.NRGBA{R: 72, G: 101, B: 129, A: 255}  // #486581
	agentPermissionTitleFg      = color.NRGBA{R: 35, G: 40, B: 48, A: 255}    // #232830
	agentPermissionSubtitleFg   = color.NRGBA{R: 116, G: 128, B: 145, A: 255} // #748091
	agentPermissionDescFg       = color.NRGBA{R: 86, G: 97, B: 112, A: 255}   // #566170
	agentPermissionScopeBg      = color.NRGBA{R: 245, G: 247, B: 249, A: 255} // #F5F7F9
	agentPermissionFolderIconFg = color.NRGBA{R: 104, G: 117, B: 130, A: 255} // #687582
	agentPermissionFolderNameFg = color.NRGBA{R: 50, G: 60, B: 72, A: 255}    // #323C48
	agentPermissionFolderPathFg = color.NRGBA{R: 116, G: 128, B: 145, A: 255} // #748091
	agentPermissionBadgeFg      = color.NRGBA{R: 72, G: 101, B: 129, A: 255}  // #486581
	agentPermissionRejectBdr    = color.NRGBA{R: 220, G: 225, B: 231, A: 255} // #DCE1E7
	agentPermissionOnceBg       = color.NRGBA{R: 52, G: 59, B: 69, A: 255}    // #343B45
	agentPermissionNoticeFg     = color.NRGBA{R: 122, G: 135, B: 151, A: 255} // #7A8797
)

func (a *App) activeAgentConfirmation() *agentConfirmationCard {
	if a == nil || a.agentUI.confirmations == nil {
		return nil
	}
	for _, id := range a.agentUI.confirmationOrder {
		card := a.agentUI.confirmations[id]
		if card != nil && card.info.Status == application.ConfirmationPending {
			if card.info.WindowID == 0 || a.windowID == 0 || card.info.WindowID == a.windowID {
				return card
			}
		}
	}
	return nil
}

func isPermissionConfirmation(info application.ConfirmationInfo) bool {
	lowerID := strings.ToLower(info.CapabilityID)
	return strings.Contains(lowerID, "permission") ||
		strings.Contains(lowerID, "folder") ||
		strings.Contains(lowerID, "dir") ||
		strings.Contains(lowerID, "access")
}

func (a *App) layoutAgentConfirmationModal(gtx layout.Context) layout.Dimensions {
	if a.activeAgentLocalAccessConsent() != nil {
		return a.layoutAgentLocalAccessModal(gtx)
	}
	card := a.activeAgentConfirmation()
	if card == nil {
		return layout.Dimensions{}
	}
	if isPermissionConfirmation(card.info) {
		return a.layoutAgentPermissionConfirmModal(gtx, card)
	}
	return a.layoutAgentLightweightConfirmModal(gtx, card)
}

// layoutAgentLightweightConfirmModal renders Node EV5Zd:
// "Glance / 轻量确认 · 插入文档"
// - 全局遮罩阻止背景交互，点击遮罩不关闭
// - 底部居中悬浮卡片 (420dp)
// - AI 星光图标与一句话确认提示
// - 浅色操作区包含“拒绝”与“允许”按钮
func (a *App) layoutAgentLightweightConfirmModal(gtx layout.Context, card *agentConfirmationCard) layout.Dimensions {
	for card.scrim.Clicked(gtx) {
	}
	if card.reject.Clicked(gtx) {
		a.resolveAgentConfirmation(card, false)
		return layout.Dimensions{}
	}
	if card.approve.Clicked(gtx) {
		a.resolveAgentConfirmation(card, true)
		return layout.Dimensions{}
	}

	return layout.Stack{Alignment: layout.S}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return card.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, agentLightweightBackdrop, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			bottomInset := unit.Dp(110)
			if gtx.Constraints.Max.Y > 0 && gtx.Constraints.Max.Y < gtx.Dp(unit.Dp(500)) {
				bottomInset = unit.Dp(24)
			}
			return layout.Inset{Bottom: bottomInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutAgentLightweightConfirmCard(gtx, card)
				})
			})
		}),
	)
}

func (a *App) layoutAgentLightweightConfirmCard(gtx layout.Context, card *agentConfirmationCard) layout.Dimensions {
	cardWidth := gtx.Dp(unit.Dp(420))
	if maxWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(32)); maxWidth < cardWidth {
		cardWidth = maxWidth
	}
	if cardWidth <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWidth, cardWidth
	gtx.Constraints.Min.Y = 0

	rec := op.Record(gtx.Ops)
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Upper: 确认内容区
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(22),
				Bottom: unit.Dp(22),
				Left:   unit.Dp(20),
				Right:  unit.Dp(20),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Dp(unit.Dp(34))
						gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(size, size), image.Pt(size, size)
						radius := gtx.Dp(unit.Dp(11))
						shape := clip.UniformRRect(image.Rectangle{Max: image.Pt(size, size)}, radius)
						paint.FillShape(gtx.Ops, agentLightweightAIBadgeBg, shape.Op(gtx.Ops))
						paint.FillShape(gtx.Ops, agentLightweightAIBadgeBdr, clip.Stroke{
							Path:  shape.Path(gtx.Ops),
							Width: float32(gtx.Dp(unit.Dp(1))),
						}.Op())
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeIcon(gtx, "sparkles", unit.Dp(18), agentLightweightAIIconFg)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						prompt := a.agentLightweightPromptText(card.info)
						lbl := homeLabelStyle(a.theme, prompt, unit.Sp(14), agentLightweightPromptFg, font.Medium)
						lbl.LineHeight = unit.Sp(21)
						return lbl.Layout(gtx)
					}),
				)
			})
		}),
		// Lower: 浅色操作区
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			barHeight := gtx.Dp(unit.Dp(60))
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = barHeight, barHeight
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, barHeight)
					paint.FillShape(gtx.Ops, agentLightweightBarBg, clip.Rect{Max: size}.Op())
					paint.FillShape(gtx.Ops, agentLightweightBarBorder, clip.Rect{Max: image.Pt(size.X, gtx.Dp(unit.Dp(1)))}.Op())
					return layout.Dimensions{Size: size}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
							layout.Flexed(1, layout.Spacer{}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutModalActionButton(gtx, a.theme, &card.reject, a.textID("agent.confirmation.action.reject"), unit.Dp(80), unit.Dp(36),
									color.NRGBA{R: 255, G: 255, B: 255, A: 255}, agentLightweightRejectBdr, agentLightweightRejectFg, "")
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(10)), 0)}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutModalActionButton(gtx, a.theme, &card.approve, a.textID("agent.confirmation.action.allow"), unit.Dp(96), unit.Dp(36),
									agentLightweightApproveBg, agentLightweightApproveBg, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, "check")
							}),
						)
					})
				},
			)
		}),
	)
	call := rec.Stop()

	bounds := image.Rectangle{Max: dims.Size}
	radius := gtx.Dp(unit.Dp(14))

	// Outer drop shadows
	for _, s := range []struct {
		offset int
		alpha  uint8
	}{
		{offset: 8, alpha: 14},
		{offset: 4, alpha: 18},
		{offset: 2, alpha: 24},
	} {
		shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(s.offset))))
		shadowShape := clip.UniformRRect(shadowBounds, radius)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 16, G: 24, B: 40, A: s.alpha}, shadowShape.Op(gtx.Ops))
	}

	// Card white background and border
	cardShape := clip.UniformRRect(bounds, radius)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, cardShape.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, agentLightweightBorder, clip.Stroke{
		Path:  cardShape.Path(gtx.Ops),
		Width: float32(gtx.Dp(unit.Dp(1))),
	}.Op())

	// Clip inner contents to card rounded corners
	clipScope := cardShape.Push(gtx.Ops)
	call.Add(gtx.Ops)
	clipScope.Pop()

	return dims
}

func (a *App) agentLightweightPromptText(info application.ConfirmationInfo) string {
	return a.textArgs("agent.confirmation.prompt", i18n.Args{"Action": a.agentConfirmationActionLabel(info)})
}

func (a *App) agentConfirmationActionLabel(info application.ConfirmationInfo) string {
	id := strings.ToLower(info.CapabilityID)
	switch {
	case strings.Contains(id, "insert"), strings.Contains(id, "create"), strings.Contains(id, "append"):
		return a.textID("agent.confirmation.action.add_content")
	case strings.Contains(id, "save"):
		return a.textID("agent.confirmation.action.save_document")
	case strings.Contains(id, "read"), strings.Contains(id, "get"), strings.Contains(id, "list"), strings.Contains(id, "inspect"), strings.Contains(id, "search"), strings.Contains(id, "audit"), strings.Contains(id, "statistics"), strings.Contains(id, "copy"):
		return a.textID("agent.confirmation.action.read_document")
	case strings.Contains(id, "delete"), strings.Contains(id, "remove"), strings.Contains(id, "clear"), strings.Contains(id, "replace"), strings.Contains(id, "set"), strings.Contains(id, "update"), strings.Contains(id, "move"), strings.Contains(id, "resize"), strings.Contains(id, "format"), strings.Contains(id, "apply"):
		return a.textID("agent.confirmation.action.change_document")
	default:
		return a.textID("agent.confirmation.action.perform_request")
	}
}

// layoutAgentPermissionConfirmModal renders Node yInjn:
// "Glance / 全局权限确认 · 底部居中"
// - 全局遮罩阻止背景交互，点击遮罩不关闭
// - 居中悬浮卡片 (560dp)
// - 权限安全图标、标题、用途说明
// - 授权范围（文件夹图标、名称、路径、只读标签）
// - 决策按钮：拒绝、仅允许本次
// - 当前操作结束后授权失效
func (a *App) layoutAgentPermissionConfirmModal(gtx layout.Context, card *agentConfirmationCard) layout.Dimensions {
	for card.scrim.Clicked(gtx) {
	}
	if card.reject.Clicked(gtx) {
		a.resolveAgentConfirmation(card, false)
		return layout.Dimensions{}
	}
	if card.allowOnce.Clicked(gtx) {
		a.resolveAgentConfirmation(card, true)
		return layout.Dimensions{}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return card.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, agentPermissionBackdrop, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentPermissionConfirmCard(gtx, card)
		}),
	)
}

func (a *App) layoutAgentPermissionConfirmCard(gtx layout.Context, card *agentConfirmationCard) layout.Dimensions {
	cardWidth := gtx.Dp(unit.Dp(560))
	if maxWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(32)); maxWidth < cardWidth {
		cardWidth = maxWidth
	}
	if cardWidth <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWidth, cardWidth
	gtx.Constraints.Min.Y = 0

	folderName, folderPath := parseFolderScope(card.info)
	if folderPath == "" {
		folderName = a.textID("agent.confirmation.permission.default_folder")
		folderPath = a.textID("agent.confirmation.permission.default_path")
	}
	title := a.textID("agent.confirmation.permission.title")
	purpose := a.textID("agent.confirmation.permission.purpose")

	rec := op.Record(gtx.Ops)
	dims := layout.Inset{
		Top:    unit.Dp(24),
		Bottom: unit.Dp(24),
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// 1. 权限标题区
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
								lbl := homeLabelStyle(a.theme, title, unit.Sp(18), agentPermissionTitleFg, font.SemiBold)
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := homeLabelStyle(a.theme, a.textID("agent.confirmation.permission.subtitle"), unit.Sp(12), agentPermissionSubtitleFg, font.Normal)
								return lbl.Layout(gtx)
							}),
						)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(20)))}
			}),
			// 2. 权限用途说明
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := homeLabelStyle(a.theme, purpose, unit.Sp(14), agentPermissionDescFg, font.Normal)
				lbl.LineHeight = unit.Sp(22)
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(0, gtx.Dp(20))}
			}),
			// 3. 授权范围
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(8)))
						paint.FillShape(gtx.Ops, agentPermissionScopeBg, shape.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Min}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Top:    unit.Dp(12),
							Bottom: unit.Dp(12),
							Left:   unit.Dp(14),
							Right:  unit.Dp(14),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
											lbl := homeLabelStyle(a.theme, folderName, unit.Sp(13), agentPermissionFolderNameFg, font.Medium)
											return lbl.Layout(gtx)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(3)))}
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := homeLabelStyle(a.theme, folderPath, unit.Sp(12), agentPermissionFolderPathFg, font.Normal)
											return lbl.Layout(gtx)
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := homeLabelStyle(a.theme, a.textID("agent.confirmation.permission.read_only"), unit.Sp(12), agentPermissionBadgeFg, font.Normal)
									return lbl.Layout(gtx)
								}),
							)
						})
					},
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(0, gtx.Dp(20))}
			}),
			// 4. 权限决策按钮 (2 等宽列)
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btnHeight := unit.Dp(40)
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layoutModalActionButton(gtx, a.theme, &card.reject, a.textID("agent.confirmation.action.reject"), 0, btnHeight,
							color.NRGBA{R: 255, G: 255, B: 255, A: 255}, agentPermissionRejectBdr, agentLightweightRejectFg, "")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(10)), 0)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layoutModalActionButton(gtx, a.theme, &card.allowOnce, a.textID("agent.confirmation.action.allow_once"), 0, btnHeight,
							agentPermissionOnceBg, agentPermissionOnceBg, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, "")
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(16)))}
			}),
			// 5. 可撤销提示
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := homeLabelStyle(a.theme, a.textID("agent.confirmation.permission.session_notice"), unit.Sp(11), agentPermissionNoticeFg, font.Normal)
					return lbl.Layout(gtx)
				})
			}),
		)
	})
	call := rec.Stop()

	bounds := image.Rectangle{Max: dims.Size}
	radius := gtx.Dp(unit.Dp(14))

	// Outer soft shadow
	for _, s := range []struct {
		offset int
		alpha  uint8
	}{
		{offset: 12, alpha: 14},
		{offset: 6, alpha: 18},
		{offset: 2, alpha: 24},
	} {
		shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(s.offset))))
		shadowShape := clip.UniformRRect(shadowBounds, radius)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 16, G: 24, B: 40, A: s.alpha}, shadowShape.Op(gtx.Ops))
	}

	cardShape := clip.UniformRRect(bounds, radius)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, cardShape.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, agentPermissionBorder, clip.Stroke{
		Path:  cardShape.Path(gtx.Ops),
		Width: float32(gtx.Dp(unit.Dp(1))),
	}.Op())

	clipScope := cardShape.Push(gtx.Ops)
	call.Add(gtx.Ops)
	clipScope.Pop()

	return dims
}

func parseFolderScope(info application.ConfirmationInfo) (name, path string) {
	if idx := strings.Index(info.ImpactSummary, `path="`); idx != -1 {
		rest := info.ImpactSummary[idx+6:]
		if end := strings.Index(rest, `"`); end != -1 {
			path = rest[:end]
		}
	}
	if path == "" && info.DocumentLabel != "" {
		path = info.DocumentLabel
	}
	if path == "" {
		return "", ""
	}
	name = filepath.Base(path)
	if name == "." || name == "/" || name == "" {
		name = path
	}
	return
}

func layoutModalActionButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, width, height unit.Dp, bg, border, fg color.NRGBA, icon string) layout.Dimensions {
	hover := bg
	if bg.A > 0 {
		hover.R = uint8(float32(bg.R) * 0.93)
		hover.G = uint8(float32(bg.G) * 0.93)
		hover.B = uint8(float32(bg.B) * 0.93)
	}
	var iconLayout layout.Widget
	if icon != "" {
		iconLayout = func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, icon, 14, fg)
		}
	}
	return (ui.IconTextButton{
		Button: button, Label: label, Icon: iconLayout,
		Style: ui.IconTextButtonStyle{
			Width: width, Height: height, IconSize: 14, Gap: 6, Radius: 7,
			Text: ui.TextStyle{Size: 13, Weight: font.Medium}, TextOffsetY: shellOpticalTextOffset(opticalConfirmationButton),
			Colors: ui.StateColors{Background: bg, Hover: hover, Border: border, Foreground: fg},
		},
	}).Layout(gtx, theme)
}
