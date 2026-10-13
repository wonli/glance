package desktopui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/agent"
	"glance/internal/agentsettings"
)

const GlobalAIAssistantDocumentID = "workspace-global-assistant"

var (
	aiEmptyTitleColor   = color.NRGBA{R: 35, G: 40, B: 48, A: 255}
	aiEmptyDescColor    = color.NRGBA{R: 146, G: 156, B: 170, A: 255}
	aiChipBorderColor   = color.NRGBA{R: 225, G: 230, B: 236, A: 255}
	aiChipTextColor     = color.NRGBA{R: 122, G: 135, B: 151, A: 255}
	aiInputCardBorder   = color.NRGBA{R: 215, G: 222, B: 231, A: 255}
	aiSendButtonFill    = color.NRGBA{R: 52, G: 59, B: 69, A: 255}
	aiDrawerBorderColor = color.NRGBA{R: 221, G: 227, B: 234, A: 255}
	aiDrawerBackdrop    = color.NRGBA{R: 37, G: 50, B: 70, A: 36}
	aiDrawerSearchFill  = color.NRGBA{R: 243, G: 245, B: 247, A: 255}
	aiDrawerItemTitle   = color.NRGBA{R: 52, G: 63, B: 77, A: 255}
	aiDrawerItemSub     = color.NRGBA{R: 146, G: 156, B: 170, A: 255}
	aiDrawerSelectedFill = color.NRGBA{R: 242, G: 244, B: 247, A: 255}
	aiAssistantText     = color.NRGBA{R: 69, G: 81, B: 95, A: 255}
	aiAssistantBorder   = color.NRGBA{R: 229, G: 233, B: 238, A: 255}
	aiUserBubbleBg      = color.NRGBA{R: 222, G: 230, B: 239, A: 255}
	aiUserText          = color.NRGBA{R: 63, G: 80, B: 100, A: 255}
)

var aiSuggestionTexts = [3]string{
	"总结一份文档",
	"梳理项目计划",
	"分析表格数据",
}

func (a *App) ensureGlobalAIAssistant() {
	a.ensureAgentSettingsLoaded()
	if a.agentUI.conversationDocumentID != GlobalAIAssistantDocumentID {
		a.ensureAgentChatDocument(GlobalAIAssistantDocumentID)
	} else if a.agentUI.conversationHistory == nil && !a.agentUI.conversationHistoryLoading {
		a.refreshAgentConversationHistory(true)
	}
}

func (a *App) currentAgentSettings() agentsettings.ModelSettings {
	if a.agentUI.settings.ProviderID != "" || a.agentUI.settings.Model != "" {
		return a.agentUI.settings
	}
	rt := a.ensureDesktopRuntime()
	if rt != nil {
		_, settings := rt.currentAgentHost()
		if settings.ProviderID != "" || settings.Model != "" {
			a.agentUI.settings = settings
			return settings
		}
		if rt.agentSettings != nil {
			if saved, err := rt.loadAgentSettings(context.Background()); err == nil {
				if saved.ProviderID != "" || saved.Model != "" {
					a.agentUI.settings = saved
					return saved
				}
			}
		}
	}
	return a.agentUI.settings
}

func (a *App) ensureAgentSettingsLoaded() {
	if a.agentUI.settings.ProviderID != "" || a.agentUI.settings.Model != "" {
		return
	}
	settings := a.currentAgentSettings()
	if settings.ProviderID != "" || settings.Model != "" {
		a.agentUI.settings = settings
		return
	}
	rt := a.ensureDesktopRuntime()
	if rt != nil && !a.agentUI.settingsLoading {
		a.agentUI.settingsLoading = true
		a.launchManagedBackground(func() {
			saved, err := rt.loadAgentSettings(context.Background())
			if err == nil && (saved.ProviderID != "" || saved.Model != "") {
				a.agentUI.settings = saved
				if a.window != nil {
					a.window.Invalidate()
				}
			}
			a.agentUI.settingsLoading = false
		})
	}
}

func (a *App) agentModelDisplayLabel() string {
	settings := a.currentAgentSettings()
	providerID := strings.TrimSpace(settings.ProviderID)
	baseURL := strings.TrimSpace(settings.BaseURL)
	model := strings.TrimSpace(settings.Model)

	if providerID == "" && model == "" {
		providerID = strings.TrimSpace(a.agentUI.providerInput.Text())
		baseURL = strings.TrimSpace(a.agentUI.baseURLInput.Text())
		model = strings.TrimSpace(a.agentUI.modelInput.Text())
	}

	tab := settingsTabFor(providerID, baseURL)
	providerName := settingsTabLabel(tab)
	if providerName == "当前服务" && providerID != "" {
		providerName = strings.ToUpper(providerID[:1]) + providerID[1:]
	}
	if model == "" {
		model = defaultAgentModelForTab(tab)
	}
	return providerName + " · " + model + "  ⌄"
}

func (a *App) layoutAIAssistantWorkspace(gtx layout.Context) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, homeCanvas) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := gtx.Dp(unit.Dp(250))
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					return a.layoutHomeSidebar(gtx)
				}),
				layout.Flexed(1, a.layoutAIAssistantMain),
			)
		},
	)
}

func (a *App) layoutAIAssistantMain(gtx layout.Context) layout.Dimensions {
	a.processAIAssistantEvents(gtx)

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Top:   unit.Dp(28),
						Left:  unit.Dp(32),
						Right: unit.Dp(32),
					}.Layout(gtx, a.layoutAIAssistantTopBar)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(16), Bottom: unit.Dp(16)}.Layout(gtx, a.layoutAIAssistantBody)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Bottom: unit.Dp(28),
					}.Layout(gtx, a.layoutAIAssistantInputCard)
				}),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !a.aiHistoryDrawerOpen {
				return layout.Dimensions{}
			}
			return a.layoutAIAssistantHistoryDrawer(gtx)
		}),
	)
}

func (a *App) processAIAssistantEvents(gtx layout.Context) {
	if a.aiDrawerOpenBtn.Clicked(gtx) {
		a.aiHistoryDrawerOpen = !a.aiHistoryDrawerOpen
		if a.aiHistoryDrawerOpen && a.agentUI.conversationHistory == nil && !a.agentUI.conversationHistoryLoading {
			a.refreshAgentConversationHistory(false)
		}
	}
	if a.aiHistoryDrawerCloseBtn.Clicked(gtx) || a.aiHistoryDrawerOverlay.Clicked(gtx) {
		a.aiHistoryDrawerOpen = false
	}
	for i := range a.aiSuggestionButtons {
		if a.aiSuggestionButtons[i].Clicked(gtx) {
			a.agentUI.chatInput.SetText(aiSuggestionTexts[i])
			gtx.Execute(key.FocusCmd{Tag: &a.agentUI.chatInput})
		}
	}
	if a.aiModelSelectButton.Clicked(gtx) {
		a.resetHomeContentScroll()
		a.prepareAgentSettingsPage()
		a.homeSection = homeSectionSettings
	}
	if a.aiAttachButton.Clicked(gtx) {
		a.setStatus("附加文档：请选择或拖入本地文件。")
	}
	if a.aiClearHistoryButton.Clicked(gtx) {
		a.confirmClearAllAIConversations()
	}
}

func (a *App) confirmClearAllAIConversations() {
	if len(a.agentUI.conversationHistory) == 0 {
		return
	}
	a.showConfirmDialog(
		"清空全部历史会话？",
		"所有历史会话将被永久删除，无法恢复。本地文档内容不受影响。",
		modalDialogDangerConfirmStyle,
		func(action string) {
			a.modal.Hide()
			if action != "delete" {
				return
			}
			doc := a.agentUI.conversationDocumentID
			for _, item := range a.agentUI.conversationHistory {
				a.deleteAgentConversation(doc, item.ID)
			}
			a.createAgentConversation()
		},
		modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary},
		modalDialogActionSpec{ID: "delete", Label: "清空", Style: modalDialogActionDanger},
	)
}

func (a *App) layoutAIAssistantTopBar(gtx layout.Context) layout.Dimensions {
	title := strings.TrimSpace(a.agentUI.conversationTitle)
	if title == "" {
		title = "新对话"
	}
	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, homeSidebarBorder, clip.Rect{Min: image.Pt(0, gtx.Constraints.Min.Y-gtx.Dp(unit.Dp(1))), Max: image.Pt(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									label := material.Label(a.theme, unit.Sp(17), title)
									label.Font.Weight = font.SemiBold
									label.Color = color.NRGBA{R: 35, G: 40, B: 48, A: 255}
									return label.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									label := material.Label(a.theme, unit.Sp(12), "AI 助手")
									label.Color = color.NRGBA{R: 146, G: 156, B: 170, A: 255}
									return label.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := &a.aiDrawerOpenBtn
							size := gtx.Dp(unit.Dp(32))
							gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(size, size), image.Pt(size, size)
							return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								bg := color.NRGBA{A: 0}
								if btn.Hovered() || a.aiHistoryDrawerOpen {
									bg = homeHoverFill
								}
								shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(5)))
								paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeIcon(gtx, "menu", 17, color.NRGBA{R: 104, G: 117, B: 130, A: 255})
								})
							})
						}),
					)
				})
			},
		)
	})
}

func (a *App) layoutAIAssistantBody(gtx layout.Context) layout.Dimensions {
	if len(a.agentUI.chatLines) == 0 {
		return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32)}.Layout(gtx, a.layoutAIAssistantEmptyState)
	}
	return a.layoutAIAssistantMessages(gtx)
}

func (a *App) layoutAIAssistantEmptyState(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				title := material.Label(a.theme, unit.Sp(28), "今天想一起完成什么？")
				title.Font.Weight = font.SemiBold
				title.Color = aiEmptyTitleColor
				return title.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				desc := material.Label(a.theme, unit.Sp(14), "提问、整理思路，或添加文件开始工作。")
				desc.Color = aiEmptyDescColor
				return desc.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				chips := make([]layout.FlexChild, 0, len(aiSuggestionTexts)*2-1)
				for i := range aiSuggestionTexts {
					idx := i
					if idx > 0 {
						chips = append(chips, layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout))
					}
					chips = append(chips, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := &a.aiSuggestionButtons[idx]
						gtx.Constraints.Min.Y = 0
						return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
							if btn.Hovered() {
								bg = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
							}
							return layout.Background{}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									size := gtx.Constraints.Min
									shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(6)))
									paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
									paint.FillShape(gtx.Ops, aiChipBorderColor, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
									return layout.Dimensions{Size: size}
								},
								func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(9), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.Y = 0
										label := material.Label(a.theme, unit.Sp(13), aiSuggestionTexts[idx])
										label.Color = aiChipTextColor
										return label.Layout(gtx)
									})
								},
							)
						})
					}))
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, chips...)
			}),
		)
	})
}

func (a *App) layoutAIAssistantMessages(gtx layout.Context) layout.Dimensions {
	maxColWidth := gtx.Dp(unit.Dp(780))
	sidePadding := gtx.Dp(unit.Dp(32))
	groups := agentSidebarMessageGroups(a.agentUI.chatLines)
	lastAssistant := lastAgentAssistantLine(a.agentUI.chatLines)
	a.aiChatList.Axis = layout.Vertical

	availableColWidth := gtx.Constraints.Max.X - sidePadding*2
	colWidth := min(availableColWidth, maxColWidth)
	if colWidth < 0 {
		colWidth = 0
	}

	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	style := material.List(a.theme, &a.aiChatList)
	style.AnchorStrategy = material.Overlay

	return style.Layout(gtx, len(groups), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = colWidth
			gtx.Constraints.Max.X = colWidth
			return a.layoutAIAssistantMessageGroup(gtx, groups[index], lastAssistant)
		})
	})
}

func (a *App) layoutAIAssistantMessageGroup(gtx layout.Context, group [2]int, lastAssistant int) layout.Dimensions {
	first := a.agentUI.chatLines[group[0]]
	if first.role == "user" {
		return a.layoutAIAssistantUserRow(gtx, group[0])
	}
	return a.layoutAIAssistantAssistantRow(gtx, group, lastAssistant)
}

func (a *App) layoutAIAssistantUserRow(gtx layout.Context, index int) layout.Dimensions {
	line := a.agentUI.chatLines[index]
	maxBubbleWidth := gtx.Dp(unit.Dp(440))

	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, maxBubbleWidth)

			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					size := gtx.Constraints.Min
					radius := gtx.Dp(unit.Dp(10))
					small := gtx.Dp(unit.Dp(3))
					shape := clip.RRect{Rect: image.Rectangle{Max: size}, NW: radius, NE: radius, SW: radius, SE: small}
					paint.FillShape(gtx.Ops, aiUserBubbleBg, shape.Op(gtx.Ops))
					return layout.Dimensions{Size: size}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(14), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						body := material.Body1(a.theme, line.text)
						body.TextSize = unit.Sp(14)
						body.LineHeight = unit.Sp(24)
						body.Color = aiUserText
						body.MaxLines = 0
						return body.Layout(gtx)
					})
				},
			)
		})
	})
}

func (a *App) layoutAIAssistantAssistantRow(gtx layout.Context, group [2]int, lastAssistant int) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, group[1]-group[0]+2)

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			label := material.Label(a.theme, unit.Sp(12), "AI 助手")
			label.Color = color.NRGBA{R: 122, G: 135, B: 151, A: 255}
			label.Font.Weight = font.SemiBold
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, label.Layout)
		}))

		for i := group[0]; i < group[1]; i++ {
			row := i
			line := a.agentUI.chatLines[row]
			if line.role == "assistant" && strings.TrimSpace(line.text) == "" {
				continue
			}
			if isAgentProcessLine(line) {
				indices := collectSubTurnProcessLines(a.agentUI.chatLines, row)
				if len(indices) == 0 || indices[0] != row {
					continue
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutAgentSidebarProcessGroup(gtx, indices)
					})
				}))
				continue
			}

			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						size := gtx.Constraints.Min
						radius := gtx.Dp(unit.Dp(10))
						small := gtx.Dp(unit.Dp(3))
						shape := clip.RRect{Rect: image.Rectangle{Max: size}, NW: small, NE: radius, SW: radius, SE: radius}
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, shape.Op(gtx.Ops))
						paint.FillShape(gtx.Ops, aiAssistantBorder, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
						return layout.Dimensions{Size: size}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutAgentAssistantMarkdown(gtx, &a.agentUI.chatLines[row])
						})
					},
				)
			}))

			if row == lastAssistant && !a.agentUI.conversationBusy {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(4)}.Layout(gtx, a.layoutAIAssistantResponseActions)
				}))
			}
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (a *App) layoutAIAssistantResponseActions(gtx layout.Context) layout.Dimensions {
	copyClicked := a.agentUI.copyResponseButton.Clicked(gtx)
	regenerateClicked := a.agentUI.regenerateResponseButton.Clicked(gtx)

	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := &a.agentUI.copyResponseButton
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, "copy", 14, color.NRGBA{R: 123, G: 135, B: 148, A: 255})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := &a.agentUI.regenerateResponseButton
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeIcon(gtx, "rotate-cw", 14, color.NRGBA{R: 123, G: 135, B: 148, A: 255})
			})
		}),
	)

	if copyClicked {
		if index := lastAgentAssistantLine(a.agentUI.chatLines); index >= 0 {
			text := strings.TrimSpace(a.agentUI.chatLines[index].text)
			gtx.Execute(clipboard.WriteCmd{Type: "text/plain", Data: io.NopCloser(strings.NewReader(text))})
			a.setStatus("已复制回答。")
		}
	}
	if regenerateClicked {
		a.sendAgentSidebarPrompt("请重新生成回答。")
	}
	return dims
}

func (a *App) layoutAIAssistantInputCard(gtx layout.Context) layout.Dimensions {
	maxColWidth := gtx.Dp(unit.Dp(780))
	sidePadding := gtx.Dp(unit.Dp(32))
	settingsFieldFocus(gtx, &a.agentUI.chatInput)

	sendClicked := a.agentUI.sendButton.Clicked(gtx)
	stopClicked := a.agentUI.stopButton.Clicked(gtx)

	placeholder := "向 AI 提问，或添加文件…"
	if len(a.agentUI.chatLines) > 0 {
		placeholder = "继续提问，或添加文件让 AI 帮你处理…"
	}

	enterPressed := false
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &a.agentUI.chatInput, Name: key.NameEnter},
			key.Filter{Focus: &a.agentUI.chatInput, Name: key.NameReturn},
		)
		if !ok {
			break
		}
		if e, ok := e.(key.Event); ok && e.State == key.Press {
			if !e.Modifiers.Contain(key.ModShift) {
				enterPressed = true
			}
		}
	}
	for event, ok := a.agentUI.chatInput.Update(gtx); ok; event, ok = a.agentUI.chatInput.Update(gtx) {
		if _, submit := event.(widget.SubmitEvent); submit {
			enterPressed = true
		}
	}

	if sendClicked || enterPressed {
		a.sendAgentChatMessage()
	}
	if stopClicked {
		a.stopAgentChatTurn()
	}

	availableColWidth := gtx.Constraints.Max.X - sidePadding*2
	colWidth := min(availableColWidth, maxColWidth)
	if colWidth < 0 {
		colWidth = 0
	}

	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = colWidth
		gtx.Constraints.Max.X = colWidth

		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(10)))
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, shape.Op(gtx.Ops))
				paint.FillShape(gtx.Ops, aiInputCardBorder, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(48))
							if gtx.Constraints.Max.Y > gtx.Dp(unit.Dp(160)) {
								gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(160))
							}
							style := material.Editor(a.theme, &a.agentUI.chatInput, placeholder)
							style.TextSize = unit.Sp(14)
							style.Color = color.NRGBA{R: 52, G: 59, B: 69, A: 255}
							style.HintColor = aiEmptyDescColor
							return style.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := &a.aiAttachButton
									return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return homeIcon(gtx, "paperclip", 18, color.NRGBA{R: 122, G: 135, B: 151, A: 255})
									})
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := &a.aiModelSelectButton
									return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.Y = 0
										label := material.Label(a.theme, unit.Sp(12), a.agentModelDisplayLabel())
										label.Color = color.NRGBA{R: 122, G: 135, B: 151, A: 255}
										return label.Layout(gtx)
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := &a.agentUI.sendButton
									if a.agentUI.conversationBusy {
										btn = &a.agentUI.stopButton
									}
									size := gtx.Dp(unit.Dp(32))
									gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(size, size), image.Pt(size, size)
									return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										bg := aiSendButtonFill
										if btn.Hovered() {
											bg = color.NRGBA{R: 70, G: 79, B: 92, A: 255}
										}
										shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(7)))
										paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											iconName := "arrow-up"
											if a.agentUI.conversationBusy {
												iconName = "square"
											}
											return homeIcon(gtx, iconName, 18, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
										})
									})
								}),
							)
						}),
					)
				})
			},
		)
	})
}

func (a *App) layoutAIAssistantHistoryDrawer(gtx layout.Context) layout.Dimensions {
	drawerWidth := gtx.Dp(unit.Dp(300))
	gtx.Constraints.Min = gtx.Constraints.Max

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.aiHistoryDrawerOverlay.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, aiDrawerBackdrop, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = drawerWidth, drawerWidth
				gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
						paint.FillShape(gtx.Ops, aiDrawerBorderColor, clip.Rect{Max: image.Pt(gtx.Dp(unit.Dp(1)), gtx.Constraints.Min.Y)}.Op())
						return layout.Dimensions{Size: gtx.Constraints.Min}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(24), Bottom: unit.Dp(18), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(a.layoutAIDrawerTitleBar),
								layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
								layout.Rigid(a.layoutAIDrawerSearchBox),
								layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
								layout.Flexed(1, a.layoutAIDrawerList),
								layout.Rigid(a.layoutAIDrawerBottomBar),
							)
						})
					},
				)
			})
		}),
	)
}

func (a *App) layoutAIDrawerTitleBar(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			title := material.Label(a.theme, unit.Sp(17), "历史会话")
			title.Font.Weight = font.SemiBold
			title.Color = aiDrawerItemTitle
			return title.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := &a.aiHistoryDrawerCloseBtn
			size := gtx.Dp(unit.Dp(28))
			gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(size, size), image.Pt(size, size)
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if btn.Hovered() {
					bg = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(5)))
				paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, "x", 17, color.NRGBA{R: 138, G: 150, B: 165, A: 255})
				})
			})
		}),
	)
}

func (a *App) layoutAIDrawerSearchBox(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

	return homeRoundedPanel(gtx, aiDrawerSearchFill, aiDrawerSearchFill, unit.Dp(6), func(gtx layout.Context) layout.Dimensions {
		settingsFieldFocus(gtx, &a.agentUI.historySearch)
		return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, "search", 14, color.NRGBA{R: 146, G: 156, B: 170, A: 255})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(8), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						e := material.Editor(a.theme, &a.agentUI.historySearch, "搜索会话")
						e.TextSize = unit.Sp(12)
						e.Color = aiDrawerItemTitle
						e.HintColor = aiDrawerItemSub
						return e.Layout(gtx)
					})
				}),
			)
		})
	})
}

func (a *App) layoutAIDrawerList(gtx layout.Context) layout.Dimensions {
	now := gtx.Now
	if now.IsZero() {
		now = time.Now()
	}
	rows := a.agentHistoryRows(now)
	if len(rows) == 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Label(a.theme, unit.Sp(13), "暂无历史会话")
			label.Color = aiDrawerItemSub
			return label.Layout(gtx)
		})
	}

	a.agentUI.conversationHistoryList.Axis = layout.Vertical
	return material.List(a.theme, &a.agentUI.conversationHistoryList).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		row := rows[index]
		item := row.item
		button := a.agentHistoryItemButton(item.ID)
		deleteButton := a.agentHistoryDeleteButton(item.ID)

		if deleteButton.Clicked(gtx) {
			a.confirmAgentConversationDelete(item)
		} else if button.Clicked(gtx) {
			a.loadAgentConversation(item.ID)
			a.aiHistoryDrawerOpen = false
		}

		selected := a.agentUI.conversationID == item.ID

		children := make([]layout.FlexChild, 0, 2)
		if row.group != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(4), Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					groupLabel := material.Label(a.theme, unit.Sp(11), row.group)
					groupLabel.Color = aiDrawerItemSub
					return groupLabel.Layout(gtx)
				})
			}))
		}

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(unit.Dp(57))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height

				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
						if selected {
							bg = aiDrawerSelectedFill
						} else if button.Hovered() {
							bg = color.NRGBA{R: 248, G: 249, B: 250, A: 255}
						}
						shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6)))
						paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Max}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									title := strings.TrimSpace(item.Title)
									if title == "" {
										title = "新会话"
									}
									timeSnippet := item.UpdatedAt.Local().Format("15:04")
									return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min.Y = 0
												lbl := material.Label(a.theme, unit.Sp(13), title)
												lbl.Color = aiDrawerItemTitle
												lbl.Font.Weight = font.Normal
												if selected {
													lbl.Font.Weight = font.SemiBold
												}
												lbl.MaxLines = 1
												return lbl.Layout(gtx)
											}),
											layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min.Y = 0
												lbl := material.Label(a.theme, unit.Sp(11), timeSnippet)
												lbl.Color = aiDrawerItemSub
												lbl.MaxLines = 1
												return lbl.Layout(gtx)
											}),
										)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !button.Hovered() && !selected {
										return layout.Dimensions{}
									}
									delSize := gtx.Dp(unit.Dp(24))
									gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(delSize, delSize), image.Pt(delSize, delSize)
									return deleteButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeIcon(gtx, "trash-2", 14, color.NRGBA{R: 208, G: 69, B: 61, A: 255})
										})
									})
								}),
							)
						})
					},
				)
			})
		}))

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (a *App) layoutAIDrawerBottomBar(gtx layout.Context) layout.Dimensions {
	countText := fmt.Sprintf("%d 个会话", len(a.agentUI.conversationHistory))

	return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 233, G: 237, B: 242, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Min.X, gtx.Dp(unit.Dp(1)))}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							label := material.Label(a.theme, unit.Sp(11), countText)
							label.Color = aiDrawerItemSub
							return label.Layout(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := &a.aiClearHistoryButton
							return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								col := aiDrawerItemSub
								if btn.Hovered() {
									col = color.NRGBA{R: 208, G: 69, B: 61, A: 255}
								}
								label := material.Label(a.theme, unit.Sp(11), "清空历史")
								label.Color = col
								return label.Layout(gtx)
							})
						}),
					)
				})
			},
		)
	})
}

func (a *App) agentHistoryItemButton(id agent.ConversationID) *widget.Clickable {
	if a.agentUI.conversationHistoryButtons == nil {
		a.agentUI.conversationHistoryButtons = make(map[agent.ConversationID]*widget.Clickable)
	}
	btn := a.agentUI.conversationHistoryButtons[id]
	if btn == nil {
		btn = new(widget.Clickable)
		a.agentUI.conversationHistoryButtons[id] = btn
	}
	return btn
}

func (a *App) agentHistoryDeleteButton(id agent.ConversationID) *widget.Clickable {
	if a.agentUI.conversationHistoryDeleteButtons == nil {
		a.agentUI.conversationHistoryDeleteButtons = make(map[agent.ConversationID]*widget.Clickable)
	}
	btn := a.agentUI.conversationHistoryDeleteButtons[id]
	if btn == nil {
		btn = new(widget.Clickable)
		a.agentUI.conversationHistoryDeleteButtons[id] = btn
	}
	return btn
}
