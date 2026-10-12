package desktopui

import (
	"image"
	"image/color"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const agentSidebarWidthDP = 250

var (
	agentSidebarFill       = color.NRGBA{R: 236, G: 238, B: 241, A: 255}
	agentSidebarBorder     = color.NRGBA{R: 201, G: 208, B: 217, A: 255}
	agentSidebarText       = color.NRGBA{R: 48, G: 59, B: 71, A: 255}
	agentSidebarMuted      = color.NRGBA{R: 116, G: 127, B: 140, A: 255}
	agentSidebarUserBubble = color.NRGBA{R: 222, G: 230, B: 239, A: 255}
	agentSidebarInputFill  = color.NRGBA{R: 248, G: 249, B: 251, A: 255}
	agentSidebarInputLine  = color.NRGBA{R: 212, G: 218, B: 226, A: 255}
	agentSidebarSendFill   = color.NRGBA{R: 63, G: 68, B: 76, A: 255}
)

func (a *App) toggleAgentSidebar() {
	if a.agentUI.sidebarVisible {
		a.agentUI.sidebarVisible = false
		return
	}
	view := a.agentViewContext()
	if view.DocumentID == "" {
		a.setStatus("请先打开一份文档，再开始与该文档关联的会话。")
		return
	}
	a.agentUI.sidebarVisible = true
	a.ensureAgentChatDocument(view.DocumentID)
}

func (a *App) layoutAgentRibbonEntry(gtx layout.Context) layout.Dimensions {
	button := &a.agentUI.ribbonEntryButton
	if !a.modal.Visible && button.Clicked(gtx) {
		a.toggleAgentSidebar()
	}
	width, height := gtx.Dp(unit.Dp(40)), gtx.Dp(unit.Dp(56))
	if gtx.Constraints.Max.X > 0 && width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	if gtx.Constraints.Max.Y > 0 && height > gtx.Constraints.Max.Y {
		height = gtx.Constraints.Max.Y
	}
	gtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("AI 助手").Add(gtx.Ops)
		bounds := image.Rectangle{Max: image.Pt(width, height)}
		paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: bounds.Max}.Op())
		if button.Hovered() || a.agentUI.sidebarVisible {
			fill := color.NRGBA{R: 239, G: 244, B: 250, A: 255}
			paint.FillShape(gtx.Ops, fill, clip.Rect{Max: bounds.Max}.Op())
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 225, G: 229, B: 234, A: 255}, clip.Rect{Min: image.Pt(width-1, gtx.Dp(unit.Dp(12))), Max: image.Pt(width, height-gtx.Dp(unit.Dp(12)))}.Op())
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Label(a.theme, unit.Sp(13), "AI")
					label.Color = color.NRGBA{R: 53, G: 95, B: 150, A: 255}
					label.Font.Weight = 600
					dims := label.Layout(gtx)
					// The Latin font's line box includes bottom whitespace.
					// Remove it from the gap before the smaller Chinese label.
					trim := gtx.Dp(unit.Dp(6))
					dims.Size.Y = max(0, dims.Size.Y-trim)
					dims.Baseline = max(0, dims.Baseline-trim)
					return dims
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Label(a.theme, unit.Sp(9), "助手")
					label.Color = color.NRGBA{R: 115, G: 125, B: 136, A: 255}
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func (a *App) layoutAgentFallbackToolbar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(56))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(a.layoutAgentRibbonEntry),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
	)
}

func (a *App) layoutAgentSidebarWorkspace(gtx layout.Context, content layout.Widget) layout.Dimensions {
	if !a.modal.Visible && a.agentUI.historyBackButton.Clicked(gtx) {
		a.agentUI.conversationHistoryVisible = false
		if a.agentUI.conversationID == "" && !a.agentUI.conversationHistoryLoading {
			a.createAgentConversation()
		}
	}
	if a.agentUI.sidebarCloseButton.Clicked(gtx) {
		a.agentUI.sidebarVisible = false
	}
	if !a.agentUI.sidebarVisible {
		a.agentUI.sidebarResizing = false
		return content(gtx)
	}
	view := a.agentViewContext()
	if view.DocumentID == "" {
		return content(gtx)
	}
	a.ensureAgentChatDocument(view.DocumentID)
	if !a.modal.Visible && a.agentUI.conversationTitleButton.Clicked(gtx) {
		a.agentUI.conversationHistoryVisible = true
		if a.agentUI.conversationHistory == nil && !a.agentUI.conversationHistoryLoading {
			a.refreshAgentConversationHistory(false)
		}
	}
	if !a.modal.Visible && a.agentUI.newConversationButton.Clicked(gtx) {
		a.createAgentConversation()
	}
	a.updateAgentSidebarResize(gtx)
	width := a.agentSidebarWidth(gtx)
	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return a.layoutAgentSidebar(gtx)
		}),
		layout.Flexed(1, content),
	)
	a.layoutAgentSidebarResizeHandle(gtx, width)

	return dims
}

func (a *App) agentSidebarWidth(gtx layout.Context) int {
	widthDP := a.agentUI.sidebarWidthDP
	if widthDP <= 0 {
		widthDP = agentSidebarWidthDP
	}
	if a.agentUI.sidebarResizing {
		widthDP = a.agentUI.sidebarResizePreviewDP
	}
	minWidth, maxWidth := agentSidebarResizeBounds(gtx)
	return max(minWidth, min(maxWidth, gtx.Dp(widthDP)))
}

func (a *App) layoutAgentSidebar(gtx layout.Context) layout.Dimensions {
	if a.agentUI.conversationHistoryVisible {
		return a.layoutAgentHistorySidebar(gtx)
	}
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, agentSidebarFill, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, agentSidebarBorder, clip.Rect{Min: image.Pt(gtx.Constraints.Max.X-gtx.Dp(unit.Dp(1)), 0), Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(13), Bottom: unit.Dp(25)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return agentSidebarContentInset(gtx, a.layoutAgentSidebarHeader)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(7), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, color.NRGBA{R: 220, G: 225, B: 230, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return agentSidebarContentInset(gtx, a.layoutAgentConfirmations)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(4)}.Layout(gtx, a.layoutAgentSidebarMessages)
					}),

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return agentSidebarContentInset(gtx, a.layoutAgentSidebarStatus)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return agentSidebarContentInset(gtx, a.layoutAgentSidebarComposer)
					}),
				)
			})
		},
	)
}

func (a *App) layoutAgentSidebarHeader(gtx layout.Context) layout.Dimensions {
	state := &a.agentUI
	if !a.modal.Visible && state.conversationTitleButton.Clicked(gtx) {
		state.conversationHistoryVisible = !state.conversationHistoryVisible
		if state.conversationHistory == nil && !state.conversationHistoryLoading {
			a.refreshAgentConversationHistory(false)
		}
	}
	if !a.modal.Visible && state.newConversationButton.Clicked(gtx) {
		a.createAgentConversation()
	}
	if state.resumeConversationButton.Clicked(gtx) {
		a.resumeAgentChatConversation()
	}
	return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(30))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layoutAgentSidebarConversationTitle(gtx, a.theme, &state.conversationTitleButton, state.conversationTitle, state.conversationHistoryVisible)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !state.conversationInterrupted || state.conversationBusy {
					return layout.Dimensions{}
				}
				return agentSidebarResumeButton(gtx, a.theme, &state.resumeConversationButton)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutAgentSidebarIconButton(gtx, a.theme, &state.newConversationButton, "square-pen", "新建会话", false)
			}),
		)
	})
}

func layoutAgentSidebarConversationTitle(gtx layout.Context, theme *material.Theme, button *widget.Clickable, title string, open bool) layout.Dimensions {
	if strings.TrimSpace(title) == "" {
		title = "新会话"
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Point{}
					gtx.Constraints.Max.X = max(0, gtx.Constraints.Max.X-gtx.Dp(unit.Dp(19)))
					label := material.Label(theme, unit.Sp(12), title)
					label.Color = color.NRGBA{R: 65, G: 78, B: 92, A: 255}
					label.Font.Weight = 600
					label.MaxLines = 1
					return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, label.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Point{}
					icon := "triangle-down"
					if open {
						icon = "triangle-up"
					}
					return layout.Inset{Left: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, icon, 12, agentSidebarMuted) })
				}),
			)
		})
	})
}

func layoutAgentSidebarIconButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, icon, description string, active bool) layout.Dimensions {
	size := gtx.Dp(unit.Dp(30))
	gtx.Constraints = layout.Constraints{Min: image.Pt(size, size), Max: image.Pt(size, size)}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(description).Add(gtx.Ops)
		if button.Hovered() || active {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 231, B: 237, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, icon, 16, color.NRGBA{R: 99, G: 112, B: 126, A: 255})
		})
	})
}

func (a *App) layoutAgentSidebarResponseActions(gtx layout.Context) layout.Dimensions {
	copyClicked := a.agentUI.copyResponseButton.Clicked(gtx)
	regenerateClicked := a.agentUI.regenerateResponseButton.Clicked(gtx)
	insertClicked := a.agentUI.insertResponseButton.Clicked(gtx)
	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return agentSidebarTextAction(gtx, a.theme, &a.agentUI.copyResponseButton, "copy", "", "复制")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return agentSidebarTextAction(gtx, a.theme, &a.agentUI.regenerateResponseButton, "rotate-cw", "", "重新生成回答")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return agentSidebarTextAction(gtx, a.theme, &a.agentUI.insertResponseButton, "", "插入文档 ↗", "插入文档")
		}),
	)
	if copyClicked {
		if index := lastAgentAssistantLine(a.agentUI.chatLines); index >= 0 {
			text := strings.TrimSpace(a.agentUI.chatLines[index].text)
			gtx.Execute(clipboard.WriteCmd{Type: "text/plain", Data: io.NopCloser(strings.NewReader(text))})
			a.agentUI.chatStatus = "已复制回答。"
		}
	}
	if regenerateClicked {
		a.sendAgentSidebarPrompt("请重新生成你对我上一条问题的回答，给出不同但完整的表述。")
	}
	if insertClicked {
		a.sendAgentSidebarPrompt("请将你上一条回答的正文插入当前文档的合适位置。通过 Glance 能力完成修改，并按要求等待人工确认；如果当前文档格式不支持编辑，请说明原因。")
	}
	return dimensions
}

func agentSidebarTextAction(gtx layout.Context, theme *material.Theme, button *widget.Clickable, icon, label, description string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(20))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(description).Add(gtx.Ops)
		return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if icon == "" {
						return layout.Dimensions{}
					}
					return homeIcon(gtx, icon, 13, agentSidebarMuted)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if label == "" {
						return layout.Dimensions{}
					}
					gtx.Constraints.Min.Y = 0
					// Compensate the font line box so text ink shares the icons' center.
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						style := material.Caption(theme, label)
						style.TextSize = unit.Sp(10)
						style.Color = color.NRGBA{R: 67, G: 95, B: 128, A: 255}
						return style.Layout(gtx)
					})
				}),
			)
		})
	})
}

func (a *App) sendAgentSidebarPrompt(prompt string) {
	if a == nil || a.agentUI.conversationBusy || strings.TrimSpace(prompt) == "" {
		return
	}
	a.agentUI.chatInput.SetText(prompt)
	a.sendAgentChatMessage()
}

func (a *App) layoutAgentSidebarStatus(gtx layout.Context) layout.Dimensions {
	if strings.TrimSpace(a.agentUI.chatStatus) == "" {
		return layout.Dimensions{}
	}
	style := material.Caption(a.theme, a.agentUI.chatStatus)
	style.TextSize = unit.Sp(10)
	style.Color = agentSidebarMuted
	style.MaxLines = 2
	return layout.Inset{Bottom: unit.Dp(5)}.Layout(gtx, style.Layout)
}

func (a *App) layoutAgentSidebarComposer(gtx layout.Context) layout.Dimensions {
	compositionWasActive := editorHasActiveComposition(&a.agentUI.chatInput)
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
			enterPressed = true
		}
	}
	// A Background child must not inherit the whole remaining sidebar height.
	// Keep the draft viewport bounded so long prompts scroll within the field.
	height := min(gtx.Dp(unit.Dp(99)), gtx.Constraints.Max.Y)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	settingsFieldFocus(gtx, &a.agentUI.chatInput)
	sendClicked := a.agentUI.sendButton.Clicked(gtx)
	stopClicked := a.agentUI.stopButton.Clicked(gtx)
	dimensions := homeRoundedPanel(gtx, agentSidebarInputFill, agentSidebarInputLine, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(7), Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					style := material.Editor(a.theme, &a.agentUI.chatInput, "继续提问或输入 /")
					style.TextSize = unit.Sp(12)
					return style.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, "paperclip", 14, agentSidebarMuted) }),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											label := material.Caption(a.theme, "当前文档")
											label.TextSize = unit.Sp(10)
											label.Color = color.NRGBA{R: 105, G: 121, B: 137, A: 255}
											return label.Layout(gtx)
										})
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if a.agentUI.conversationBusy {
									return layoutAgentSidebarIconButton(gtx, a.theme, &a.agentUI.stopButton, "square", "停止生成", false)
								}
								return a.layoutAgentSidebarSendButton(gtx)
							}),
						)
					})
				}),
			)
		})
	})
	if sendClicked {
		a.sendAgentChatMessage()
	}
	if stopClicked {
		a.stopAgentChatTurn()
	}
	for event, ok := a.agentUI.chatInput.Update(gtx); ok; event, ok = a.agentUI.chatInput.Update(gtx) {
		if _, submit := event.(widget.SubmitEvent); submit {
			a.sendAgentChatMessage()
		}
	}
	if enterPressed && !compositionWasActive {
		// Process composition updates before deciding whether Enter can send.
		// Enter ending an existing composition only confirms the candidate.
		a.sendAgentChatMessage()
	}
	return dimensions
}

func (a *App) layoutAgentSidebarSendButton(gtx layout.Context) layout.Dimensions {
	button := &a.agentUI.sendButton
	size := gtx.Dp(unit.Dp(26))
	gtx.Constraints = layout.Constraints{Min: image.Pt(size, size), Max: image.Pt(size, size)}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, agentSidebarSendFill, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(7))).Op(gtx.Ops))
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return homeIcon(gtx, "arrow-up", 15, agentSidebarInputFill)
		})
	})
}

func agentSidebarContentInset(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, child)
}
