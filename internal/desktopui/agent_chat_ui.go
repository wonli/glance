package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/agent"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/workspace"
)

func (a *App) showAgentChat() {
	a.ensureAgentChatDocument(a.agentViewContext().DocumentID)
	a.modal.ShowCustom("AI 助手", "Agent 会按需通过 Glance 能力查询文档；只有被工具读取的内容会发送到所配置的模型服务。", 820,
		a.layoutAgentChat,
		func(action string) {
			switch action {
			case "settings":
				a.modal.Hide()
				a.showAgentSettings(true)
			case "close":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "settings", Label: "模型设置"},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}

func (a *App) layoutAgentChat(gtx layout.Context) layout.Dimensions {
	if view := a.agentViewContext(); view.DocumentID != a.agentUI.conversationDocumentID {
		a.ensureAgentChatDocument(view.DocumentID)
	}
	gtx.Constraints.Min.Y = 0
	maxHeight := gtx.Dp(unit.Dp(500))
	if gtx.Constraints.Max.Y > 0 && gtx.Constraints.Max.Y < maxHeight {
		maxHeight = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Max.Y = maxHeight
	host, settings := a.ensureDesktopRuntime().currentAgentHost()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			status := "已连接模型：" + settings.ProviderID + " / " + settings.Model
			if host == nil {
				switch {
				case settings.ProviderID == "":
					status = "尚未配置模型，请打开“模型设置”。"
				case settings.ProviderID != "ollama" && settings.CredentialRef == "":
					status = "设置已保存，请输入 API Key 后开始使用。"
				default:
					status = "模型暂不可用，请检查模型设置和本机服务状态。"
				}
			}
			style := material.Caption(a.theme, status)
			style.Color = homeMuted
			style.MaxLines = 2
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, style.Layout)
		}),
		layout.Rigid(a.layoutAgentConversationToolbar),
		layout.Rigid(a.layoutAgentConversationHistory),
		layout.Rigid(a.layoutAgentConfirmations),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(a.theme, &a.agentUI.chatList).Layout(gtx, len(a.agentUI.chatLines), func(gtx layout.Context, index int) layout.Dimensions {
				line := a.agentUI.chatLines[index]
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return homeRoundedPanel(gtx, agentChatLineColor(line.role), homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							role := line.role
							switch role {
							case "user":
								role = "你"
							case "assistant":
								role = "Glance Agent"
							case "tool":
								role = "工具"
							case "confirmation":
								role = "人工确认"
							case "error":
								role = "错误"
							case "reasoning":
								role = "思考过程"
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if line.role == "reasoning" && line.toggle != nil {
										if line.toggle.Clicked(gtx) {
											a.agentUI.chatLines[index].collapsed = !a.agentUI.chatLines[index].collapsed
										}
										if line.collapsed {
											role += "（已折叠，点击展开）"
										} else {
											role += "（点击折叠）"
										}
										return material.Button(a.theme, line.toggle, role).Layout(gtx)
									}
									header := material.Caption(a.theme, role)
									header.Color = homeMuted
									header.MaxLines = 1
									return header.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if line.role == "reasoning" && line.collapsed {
										return layout.Dimensions{}
									}
									body := material.Body1(a.theme, line.text)
									body.MaxLines = 0
									return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, body.Layout)
								}),
							)
						})
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.agentUI.chatStatus == "" {
				return layout.Dimensions{}
			}
			style := material.Caption(a.theme, a.agentUI.chatStatus)
			style.Color = homeMuted
			style.MaxLines = 2
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, style.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			style := material.Editor(a.theme, &a.agentUI.chatInput, "输入消息…")
			return style.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			sendClicked := a.agentUI.sendButton.Clicked(gtx)
			stopClicked := a.agentUI.stopButton.Clicked(gtx)
			dimensions := layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if a.agentUI.conversationBusy {
							return material.Caption(a.theme, "Agent 正在处理请求；可以随时停止。 ").Layout(gtx)
						}
						return material.Caption(a.theme, "当前视图上下文会随消息提供；不会自动上传整份文档。").Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.agentUI.conversationBusy {
							return material.Button(a.theme, &a.agentUI.stopButton, "停止").Layout(gtx)
						}
						return material.Button(a.theme, &a.agentUI.sendButton, "发送").Layout(gtx)
					}),
				)
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
			return dimensions
		}),
	)
}

func (a *App) layoutAgentConfirmations(gtx layout.Context) layout.Dimensions {
	ids := make([]string, 0, len(a.agentUI.confirmationOrder))
	for _, id := range a.agentUI.confirmationOrder {
		card := a.agentUI.confirmations[id]
		if card != nil && (card.info.Status == application.ConfirmationPending || card.info.Status == application.ConfirmationRunning) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return layout.Dimensions{}
	}
	maxHeight := gtx.Dp(unit.Dp(220))
	if gtx.Constraints.Max.Y > 0 && gtx.Constraints.Max.Y < maxHeight {
		maxHeight = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Min.Y = 0
	gtx.Constraints.Max.Y = maxHeight
	return material.List(a.theme, &a.agentUI.confirmationList).Layout(gtx, len(ids), func(gtx layout.Context, index int) layout.Dimensions {
		card := a.agentUI.confirmations[ids[index]]
		if card == nil {
			return layout.Dimensions{}
		}
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return homeRoundedPanel(gtx, color.NRGBA{R: 255, G: 250, B: 235, A: 255}, color.NRGBA{R: 224, G: 180, B: 95, A: 255}, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(9), Left: unit.Dp(11), Right: unit.Dp(11)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Body1(a.theme, "等待人类确认："+card.info.CapabilityName).Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							text := card.info.ImpactSummary
							if card.info.DocumentID != "" {
								label := card.info.DocumentLabel
								if label == "" {
									label = card.info.DocumentID
								}
								text += fmt.Sprintf("\n文档 %s (%s) · 修订版本 %d", label, card.info.DocumentID, card.info.ExpectedRevision)
							}
							text += "\n有效至 " + card.info.ExpiresAt.Local().Format("15:04:05")
							style := material.Caption(a.theme, text)
							style.MaxLines = 5
							return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, style.Layout)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if card.info.Status == application.ConfirmationRunning {
								style := material.Caption(a.theme, "已批准，正在执行…")
								return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, style.Layout)
							}
							approveClicked := card.approve.Clicked(gtx)
							rejectClicked := card.reject.Clicked(gtx)
							dimensions := layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return material.Button(a.theme, &card.reject, "拒绝").Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return material.Button(a.theme, &card.approve, "批准并执行").Layout(gtx)
									}),
								)
							})
							if approveClicked {
								a.resolveAgentConfirmation(card, true)
							} else if rejectClicked {
								a.resolveAgentConfirmation(card, false)
							}
							return dimensions
						}),
					)
				})
			})
		})
	})
}

func (a *App) resolveAgentConfirmation(card *agentConfirmationCard, approved bool) {
	if a == nil || card == nil || card.info.Status != application.ConfirmationPending {
		return
	}
	runtime := a.ensureDesktopRuntime().applicationRuntime
	if runtime == nil {
		a.setStatus("确认操作失败：应用运行时不可用")
		return
	}
	operator := application.Operator{ID: fmt.Sprintf("desktop-window-%d", a.windowID), Kind: application.OperatorHuman}
	if err := runtime.ResolveConfirmation(context.Background(), card.info.ConfirmationID, approved, operator); err != nil {
		a.setStatus("确认操作失败：" + err.Error())
		return
	}
	card.info.ResolvedBy = operator.ID
	if approved {
		card.info.Status = application.ConfirmationRunning
	} else {
		card.info.Status = application.ConfirmationRejected
	}
	a.updateAgentConfirmationChatLine(card.info, false)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func agentChatLineColor(role string) color.NRGBA {
	switch role {
	case "user":
		return color.NRGBA{R: 245, G: 248, B: 252, A: 255}
	case "error":
		return color.NRGBA{R: 255, G: 244, B: 242, A: 255}
	default:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
}

func (a *App) sendAgentChatMessage() {
	if editorHasActiveComposition(&a.agentUI.chatInput) {
		return
	}
	if a.agentUI.conversationInterrupted {
		a.agentUI.chatStatus = "上一轮尚未完成，请先点击顶部的“继续”恢复会话。"
		return
	}
	message := strings.TrimSpace(a.agentUI.chatInput.Text())
	if message == "" || a.agentUI.conversationBusy {
		return
	}
	view := a.agentViewContext()
	if view.DocumentID == "" {
		a.agentUI.chatStatus = "请先打开一份文档，再开始与该文档关联的会话。"
		return
	}
	if a.agentUI.conversationID == "" {
		a.ensureAgentChatDocument(view.DocumentID)
		a.agentUI.chatStatus = "正在准备文档会话，请稍后再发送。"
		return
	}
	host, _ := a.ensureDesktopRuntime().currentAgentHost()
	if host == nil {
		a.agentUI.chatLines = appendAgentChatLine(a.agentUI.chatLines, agentChatLine{role: "error", text: "请先在模型设置中配置可用的模型和 API Key。"})
		a.agentUI.chatInput.SetText("")
		return
	}
	a.subscribeToAgent(host)
	a.agentUI.activeReasoningLine = -1
	a.agentUI.activeAnswerStarted = false
	a.agentUI.chatInput.SetText("")
	a.agentUI.chatLines = appendAgentChatLine(a.agentUI.chatLines, agentChatLine{role: "user", text: message})
	a.agentUI.conversationBusy = true
	a.agentUI.chatStatus = "正在联系模型…"
	ctx, cancel := context.WithCancel(context.Background())
	conversationID := a.agentUI.conversationID
	watcherStarted := a.launchManagedBackground(func() {
		select {
		case <-a.shutdown:
			cancel()
		case <-ctx.Done():
		}
	})
	if !watcherStarted {
		cancel()
		return
	}
	if !a.launchManagedBackground(func() {
		defer cancel()
		if err := host.Send(ctx, conversationID, message, view); err != nil {
			code, message := agent.DescribeTurnError(err)
			select {
			case a.agentUI.chatEvents <- agent.AgentEvent{ConversationID: conversationID, Type: agent.EventTurnFailed, ErrorCode: code, Error: message}:
			case <-a.shutdown:
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	}) {
		cancel()
	}
}

func (a *App) resumeAgentChatConversation() {
	if a == nil || !a.agentUI.conversationInterrupted || a.agentUI.conversationBusy {
		return
	}
	host, _ := a.ensureDesktopRuntime().currentAgentHost()
	if host == nil {
		a.agentUI.chatStatus = "请先配置可用模型，再继续此会话。"
		return
	}
	id := a.agentUI.conversationID
	if id == "" {
		return
	}
	a.subscribeToAgent(host)
	a.agentUI.activeReasoningLine = -1
	a.agentUI.activeAnswerStarted = false
	a.agentUI.conversationBusy = true
	a.agentUI.chatStatus = "正在恢复已保存的会话…"
	view := a.agentViewContext()
	ctx, cancel := context.WithCancel(context.Background())
	watcherStarted := a.launchManagedBackground(func() {
		select {
		case <-a.shutdown:
			cancel()
		case <-ctx.Done():
		}
	})
	if !watcherStarted {
		cancel()
		a.agentUI.conversationBusy = false
		return
	}
	if !a.launchManagedBackground(func() {
		defer cancel()
		if err := host.ResumeConversation(ctx, id, view); err != nil {
			code, message := agent.DescribeTurnError(err)
			select {
			case a.agentUI.chatEvents <- agent.AgentEvent{ConversationID: id, Type: agent.EventTurnFailed, ErrorCode: code, Error: message}:
			case <-a.shutdown:
			}
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		cancel()
		a.agentUI.conversationBusy = false
		a.agentUI.chatStatus = "无法启动会话恢复请求。"
	}
}

func (a *App) stopAgentChatTurn() {
	host, _ := a.ensureDesktopRuntime().currentAgentHost()
	if host == nil || !a.agentUI.conversationBusy {
		return
	}
	a.agentUI.chatStatus = "正在停止…"
	conversationID := a.agentUI.conversationID
	if !a.launchManagedBackground(func() {
		if err := host.Cancel(conversationID); err != nil {
			select {
			case a.agentUI.chatEvents <- agent.AgentEvent{ConversationID: conversationID, Type: agent.EventTurnFailed, Error: err.Error()}:
			case <-a.shutdown:
			}
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		a.agentUI.chatStatus = "无法启动停止请求。"
	}
}

func (a *App) subscribeToAgent(host *agent.AgentHost) {
	if a == nil {
		return
	}
	if host == nil {
		a.closeAgentSubscription()
		return
	}
	if a.agentUI.subscribedHost == host {
		return
	}
	a.closeAgentSubscription()
	if a.agentUI.conversationID == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a.agentUI.unsubscribe = host.Subscribe(ctx, a.agentUI.conversationID, func(event agent.AgentEvent) {
		select {
		case a.agentUI.chatEvents <- event:
		case <-ctx.Done():
		case <-a.shutdown:
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	})
	a.agentUI.subscriptionStop = cancel
	a.agentUI.subscriptionDone = done
	a.agentUI.subscribedHost = host
	a.launchManagedBackground(func() {
		defer close(done)
		select {
		case <-a.shutdown:
			cancel()
		case <-ctx.Done():
		}
	})
}

func (a *App) closeAgentSubscription() {
	if a == nil {
		return
	}
	if a.agentUI.subscriptionStop != nil {
		a.agentUI.subscriptionStop()
		a.agentUI.subscriptionStop = nil
	}
	if a.agentUI.unsubscribe != nil {
		a.agentUI.unsubscribe()
		a.agentUI.unsubscribe = nil
	}
	if a.agentUI.subscriptionDone != nil {
		<-a.agentUI.subscriptionDone
		a.agentUI.subscriptionDone = nil
	}
	a.agentUI.subscribedHost = nil
}

func (a *App) processAgentChatEvents() {
	for {
		select {
		case event := <-a.agentUI.chatEvents:
			if event.ConversationID != a.agentUI.conversationID {
				continue
			}
			switch event.Type {
			case agent.EventReasoningDelta:
				lineIndex := a.agentUI.activeReasoningLine
				if lineIndex >= 0 && lineIndex < len(a.agentUI.chatLines) && a.agentUI.chatLines[lineIndex].role == "reasoning" {
					a.agentUI.chatLines[lineIndex].text += event.Reasoning
					if a.agentUI.activeAnswerStarted {
						a.agentUI.chatLines[lineIndex].collapsed = true
					}
				} else if a.agentUI.activeAnswerStarted {
					line := newAgentReasoningLine(event.Reasoning, true)
					answerIndex := lastAgentAssistantLine(a.agentUI.chatLines)
					if answerIndex >= 0 {
						a.agentUI.activeReasoningLine = a.insertLiveAgentChatLine(answerIndex, line)
					} else {
						a.appendLiveAgentChatLine(line)
						a.agentUI.activeReasoningLine = len(a.agentUI.chatLines) - 1
					}
				} else {
					line := newAgentReasoningLine(event.Reasoning, false)
					a.appendLiveAgentChatLine(line)
					a.agentUI.activeReasoningLine = len(a.agentUI.chatLines) - 1
				}
			case agent.EventAssistantDelta:
				a.agentUI.activeAnswerStarted = true
				a.collapseCurrentAgentReasoning()
				if len(a.agentUI.chatLines) > 0 && a.agentUI.chatLines[len(a.agentUI.chatLines)-1].role == "assistant" {
					a.agentUI.chatLines[len(a.agentUI.chatLines)-1].text += event.Text
				} else {
					a.appendLiveAgentChatLine(agentChatLine{role: "assistant", text: event.Text})
				}
			case agent.EventConversationTitle:
				a.agentUI.conversationTitle = event.Title
				a.refreshAgentConversationHistory(false)
			case agent.EventToolStarted:
				a.collapseCurrentAgentReasoning()
				a.agentUI.activeReasoningLine = -1
				a.agentUI.activeAnswerStarted = false
				a.agentUI.chatStatus = "正在调用工具：" + event.ToolName
				a.appendLiveAgentChatLine(agentChatLine{role: "tool", text: "正在执行 " + event.ToolName})
			case agent.EventToolCompleted:
				a.agentUI.chatStatus = ""
				message := "工具 " + event.ToolName + " 已完成"
				if event.IsError {
					message = "工具 " + event.ToolName + " 执行失败，Agent 会收到错误并决定下一步。"
				}
				if len(a.agentUI.chatLines) > 0 && a.agentUI.chatLines[len(a.agentUI.chatLines)-1].role == "tool" {
					a.agentUI.chatLines[len(a.agentUI.chatLines)-1].text = message
				} else {
					a.appendLiveAgentChatLine(agentChatLine{role: "tool", text: message})
				}
			case agent.EventTurnCompleted:
				a.agentUI.conversationBusy = false
				a.agentUI.conversationInterrupted = false
				a.agentUI.chatStatus = ""
				a.agentUI.activeReasoningLine = -1
				a.agentUI.activeAnswerStarted = false
				a.refreshAgentConversationHistory(false)
			case agent.EventTurnFailed:
				a.collapseCurrentAgentReasoning()
				a.agentUI.conversationBusy = false
				a.agentUI.chatStatus = ""
				a.agentUI.activeReasoningLine = -1
				a.agentUI.activeAnswerStarted = false
				optimisticMessage := ""
				if count := len(a.agentUI.chatLines); count > 0 && a.agentUI.chatLines[count-1].role == "user" {
					optimisticMessage = a.agentUI.chatLines[count-1].text
				}
				if host, _ := a.ensureDesktopRuntime().currentAgentHost(); host != nil {
					if snapshot, found := host.Snapshot(event.ConversationID); found {
						a.agentUI.conversationInterrupted = snapshot.Interrupted
						a.agentUI.conversationTitle = snapshot.Title
						a.agentUI.chatLines = agentConversationChatLines(snapshot)
						if snapshot.Interrupted {
							if snapshot.Title == "" && a.ensureDesktopRuntime().workspaceStore != nil {
								a.agentUI.chatStatus = "首条消息已保存，标题或回复尚未完成；点击顶部标题旁的“继续”重试。"
							} else {
								a.agentUI.chatStatus = "上一轮尚未完成；点击顶部的“继续”恢复。"
							}
						} else if optimisticMessage != "" {
							a.agentUI.chatInput.SetText(optimisticMessage)
						}
					}
				}
				if event.Error != "" {
					if len(a.agentUI.chatLines) == 0 || a.agentUI.chatLines[len(a.agentUI.chatLines)-1].role != "error" || a.agentUI.chatLines[len(a.agentUI.chatLines)-1].text != event.Error {
						a.appendLiveAgentChatLine(agentChatLine{role: "error", text: event.Error})
					}
				}
			}
		default:
			return
		}
	}
}

func (a *App) processAgentApplicationEvents() {
	for {
		select {
		case event := <-a.agentUI.applicationEvents:
			if event.Type == application.EventDocumentChanged {
				// Ribbon operations persist through their own completion path. Agent
				// commands can arrive from MCP or the built-in host, so persist the
				// active shared session when one of those commands changes it.
				if event.Source != application.OperatorHuman && a.documentSessionState != nil &&
					a.documentSessionState.documentID == event.DocumentID {
					a.persistActiveSession()
				}
				continue
			}
			if event.Type == application.EventConfirmationRequested || event.Type == application.EventConfirmationUpdated {
				var info application.ConfirmationInfo
				if err := json.Unmarshal(event.Data, &info); err != nil {
					a.setStatus("无法读取操作确认事件：" + err.Error())
					continue
				}
				if info.ConfirmationID == "" {
					continue
				}
				if info.WindowID == 0 {
					info.WindowID = event.View.WindowID
				}
				a.upsertAgentConfirmation(info)
				if event.Type == application.EventConfirmationRequested {
					if event.Source == application.OperatorBuiltInAgent && info.WindowID == a.windowID {
						a.updateAgentConfirmationChatLine(info, true)
					}
					a.setStatus("有高影响操作等待确认，请检查操作目标后批准或拒绝。")
					if a.window != nil && !a.modal.Visible {
						a.showAgentChat()
					}
				} else {
					a.updateAgentConfirmationChatLine(info, false)
					switch info.Status {
					case application.ConfirmationCompleted:
						a.setStatus("已完成人工批准的操作：" + info.CapabilityName)
					case application.ConfirmationRejected:
						a.setStatus("已拒绝操作：" + info.CapabilityName)
					case application.ConfirmationFailed:
						a.setStatus("批准后的操作执行失败：" + info.Error)
					case application.ConfirmationExpired:
						a.setStatus("操作确认已过期：" + info.CapabilityName)
					case application.ConfirmationCancelled:
						a.setStatus("应用关闭时取消了待确认操作：" + info.CapabilityName)
					}
				}
				continue
			}
			if event.Type != application.EventDocumentOpened && event.Type != application.EventDocumentSaved {
				continue
			}
			var info application.DocumentInfo
			if err := json.Unmarshal(event.Data, &info); err != nil {
				a.setStatus("无法读取新文档事件：" + err.Error())
				continue
			}
			if info.DocumentID == "" {
				info.DocumentID = event.DocumentID
			}
			if info.DocumentID == "" {
				continue
			}
			if _, exists := a.tabMeta[info.DocumentID]; !exists {
				shared, found := a.ensureDesktopRuntime().session(info.DocumentID)
				if !found || shared == nil {
					continue
				}
				if err := a.installDocument(loadedDocumentFromSharedSession(shared)); err != nil {
					a.setStatus("无法在桌面中打开 MCP 创建的文档：" + err.Error())
					continue
				}
			}
			if event.Type == application.EventDocumentSaved && info.SourcePath != "" {
				meta := a.tabMeta[info.DocumentID]
				meta.Kind = "document"
				meta.SourcePath = info.SourcePath
				meta.Title = filepath.Base(info.SourcePath)
				a.tabMeta[info.DocumentID] = meta
				if a.collection != nil {
					if err := a.collection.UpdateTabMeta(meta); err != nil {
						a.setStatus("文档已保存，但标签信息未能更新：" + err.Error())
					}
					a.collection.StageRecents([]workspace.Recent{{Path: info.SourcePath, Kind: string(info.Format)}})
					a.refreshHomeMetadata()
					a.persistRecentsAsync()
				}
				a.reconcileWorkspaceTabs()
			}
		default:
			return
		}
	}
}

func (a *App) upsertAgentConfirmation(info application.ConfirmationInfo) {
	if a.agentUI.confirmations == nil {
		a.agentUI.confirmations = make(map[string]*agentConfirmationCard)
	}
	card := a.agentUI.confirmations[info.ConfirmationID]
	if card == nil {
		card = &agentConfirmationCard{}
		a.agentUI.confirmations[info.ConfirmationID] = card
		a.agentUI.confirmationOrder = append(a.agentUI.confirmationOrder, info.ConfirmationID)
	}
	card.info = info
	for len(a.agentUI.confirmationOrder) > 64 {
		oldest := a.agentUI.confirmationOrder[0]
		oldestCard := a.agentUI.confirmations[oldest]
		if oldestCard != nil && (oldestCard.info.Status == application.ConfirmationPending || oldestCard.info.Status == application.ConfirmationRunning) {
			break
		}
		delete(a.agentUI.confirmations, oldest)
		a.agentUI.confirmationOrder = a.agentUI.confirmationOrder[1:]
	}
}

func (a *App) updateAgentConfirmationChatLine(info application.ConfirmationInfo, allowAppend bool) {
	if a == nil || info.ConfirmationID == "" || info.WindowID != a.windowID {
		return
	}
	text := confirmationChatSummary(info)
	for index := len(a.agentUI.chatLines) - 1; index >= 0; index-- {
		if a.agentUI.chatLines[index].confirmationID == info.ConfirmationID {
			a.agentUI.chatLines[index].text = text
			return
		}
	}
	if !allowAppend || info.Status != application.ConfirmationPending {
		return
	}
	a.agentUI.chatLines = appendAgentChatLine(a.agentUI.chatLines, agentChatLine{
		role: "confirmation", text: text, confirmationID: info.ConfirmationID,
	})
}

func confirmationChatSummary(info application.ConfirmationInfo) string {
	status := "等待批准"
	switch info.Status {
	case application.ConfirmationRunning:
		status = "已批准，正在执行"
	case application.ConfirmationCompleted:
		status = "已批准并执行完成"
	case application.ConfirmationRejected:
		status = "已拒绝"
	case application.ConfirmationFailed:
		status = "执行失败"
	case application.ConfirmationExpired:
		status = "已过期"
	case application.ConfirmationCancelled:
		status = "已取消"
	}
	text := fmt.Sprintf("%s：%s\n%s", status, info.CapabilityName, info.ImpactSummary)
	if info.DocumentID != "" {
		label := info.DocumentLabel
		if label == "" {
			label = info.DocumentID
		}
		text += fmt.Sprintf("\n文档 %s (%s) · 修订版本 %d", label, info.DocumentID, info.ExpectedRevision)
	}
	if info.Status == application.ConfirmationCompleted && info.Result != nil && info.Result.Revision != 0 {
		text += fmt.Sprintf("\n完成后修订版本 %d", info.Result.Revision)
	}
	if info.Error != "" {
		text += "\n" + info.Error
	}
	return text
}

func appendAgentChatLine(lines []agentChatLine, line agentChatLine) []agentChatLine {
	lines = append(lines, line)
	if len(lines) > 300 {
		lines = append([]agentChatLine(nil), lines[len(lines)-300:]...)
	}
	return lines
}

func newAgentReasoningLine(text string, collapsed bool) agentChatLine {
	return agentChatLine{role: "reasoning", text: text, collapsed: collapsed, toggle: new(widget.Clickable)}
}

func (a *App) appendLiveAgentChatLine(line agentChatLine) {
	previousLength := len(a.agentUI.chatLines)
	a.agentUI.chatLines = appendAgentChatLine(a.agentUI.chatLines, line)
	dropped := previousLength + 1 - len(a.agentUI.chatLines)
	if dropped > 0 && a.agentUI.activeReasoningLine >= 0 {
		a.agentUI.activeReasoningLine -= dropped
		if a.agentUI.activeReasoningLine < 0 {
			a.agentUI.activeReasoningLine = -1
		}
	}
}

func (a *App) insertLiveAgentChatLine(index int, line agentChatLine) int {
	lines := a.agentUI.chatLines
	if index < 0 {
		index = 0
	}
	if index > len(lines) {
		index = len(lines)
	}
	lines = append(lines, agentChatLine{})
	copy(lines[index+1:], lines[index:len(lines)-1])
	lines[index] = line
	dropped := 0
	if len(lines) > 300 {
		dropped = len(lines) - 300
		lines = append([]agentChatLine(nil), lines[dropped:]...)
	}
	if a.agentUI.activeReasoningLine >= index {
		a.agentUI.activeReasoningLine++
	}
	if dropped > 0 && a.agentUI.activeReasoningLine >= 0 {
		a.agentUI.activeReasoningLine -= dropped
		if a.agentUI.activeReasoningLine < 0 {
			a.agentUI.activeReasoningLine = -1
		}
	}
	a.agentUI.chatLines = lines
	return index - dropped
}

func lastAgentAssistantLine(lines []agentChatLine) int {
	for index := len(lines) - 1; index >= 0; index-- {
		if lines[index].role == "assistant" {
			return index
		}
	}
	return -1
}

func (a *App) collapseCurrentAgentReasoning() {
	index := a.agentUI.activeReasoningLine
	if index >= 0 && index < len(a.agentUI.chatLines) && a.agentUI.chatLines[index].role == "reasoning" {
		a.agentUI.chatLines[index].collapsed = true
	}
}

func (a *App) agentViewContext() application.ViewContext {
	view := application.ViewContext{WindowID: a.windowID}
	state := a.documentSessionState
	if state == nil || state.sharedDocumentSession == nil {
		return view
	}
	view.DocumentID = state.documentID
	view.Revision = state.wordRevisionAtomic.Load()
	if state.book != nil {
		selection := &application.SelectionContext{TargetType: "spreadsheet.cell", TargetID: a.selectedCell, Sheet: a.sheet}
		if state.selectionSet {
			selection.Range = cellSelectionBounds(state.selection)
		}
		view.Selection = selection
	} else if state.word != nil && state.selectedWordRunID != "" {
		view.Selection = &application.SelectionContext{TargetType: "word.text_run", TargetID: state.selectedWordRunID}
	}
	return view
}

func cellSelectionBounds(selection CellSelection) *document.CellRange {
	bounds := selection.Bounds()
	return &bounds
}
