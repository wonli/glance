package desktopui

import (
	"context"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/agent"
)

func (a *App) ensureAgentChatDocument(documentID string) {
	if strings.TrimSpace(documentID) == "" {
		if a.agentUI.conversationDocumentID != "" {
			a.cancelAgentChatHistoryRequest()
			a.closeAgentSubscription()
			a.agentUI.conversationDocumentID = ""
			a.agentUI.conversationID = ""
			a.agentUI.conversationTitle = ""
			a.agentUI.conversationHistory = nil
			a.agentUI.chatLines = nil
			a.agentUI.activeReasoningLine = -1
			a.agentUI.activeAnswerStarted = false
			a.agentUI.conversationBusy = false
			a.agentUI.conversationInterrupted = false
		}
		a.agentUI.chatStatus = "请先打开一份文档，再开始与该文档关联的会话。"
		return
	}
	if a.agentUI.conversationDocumentID == documentID {
		if host, _ := a.ensureDesktopRuntime().currentAgentHost(); host != nil {
			if a.agentUI.conversationID != "" {
				a.subscribeToAgent(host)
			} else if !a.agentUI.conversationHistoryLoading {
				a.refreshAgentConversationHistory(true)
			}
		}
		if a.modal.Visible && a.agentUI.conversationHistory == nil {
			a.refreshAgentConversationHistory(true)
		}
		return
	}
	if a.agentUI.conversationBusy && a.agentUI.conversationID != "" {
		host, _ := a.ensureDesktopRuntime().currentAgentHost()
		id := a.agentUI.conversationID
		if host != nil {
			a.launchManagedBackground(func() { _ = host.Cancel(id) })
		}
	}
	a.closeAgentSubscription()
	a.cancelAgentChatHistoryRequest()
	a.agentUI.conversationDocumentID = documentID
	a.agentUI.conversationID = ""
	a.agentUI.conversationTitle = ""
	a.agentUI.conversationHistory = nil
	a.agentUI.conversationHistoryButtons = make(map[agent.ConversationID]*widget.Clickable)
	a.agentUI.chatLines = nil
	a.agentUI.activeReasoningLine = -1
	a.agentUI.activeAnswerStarted = false
	a.agentUI.conversationBusy = false
	a.agentUI.conversationInterrupted = false
	a.agentUI.conversationHistoryVisible = false
	a.agentUI.conversationHistoryError = ""
	a.agentUI.chatStatus = "正在加载此文档的会话历史…"
	a.refreshAgentConversationHistory(true)
}

func (a *App) cancelAgentChatHistoryRequest() {
	a.agentUI.conversationHistoryGeneration++
	if a.agentUI.conversationHistoryCancel != nil {
		a.agentUI.conversationHistoryCancel()
		a.agentUI.conversationHistoryCancel = nil
	}
	a.agentUI.conversationHistoryLoading = false
}

func (a *App) refreshAgentConversationHistory(selectLatest bool) {
	documentID := a.agentUI.conversationDocumentID
	if documentID == "" {
		return
	}
	if a.desktopRuntime == nil || a.desktopRuntime.workspaceStore == nil {
		a.agentUI.conversationHistoryError = "工作区会话存储不可用，无法加载历史会话。"
		a.agentUI.chatStatus = ""
		return
	}
	a.startAgentChatHistoryTask("list", documentID, selectLatest, func(ctx context.Context) (agentChatHistoryResult, error) {
		items, err := (workspaceAgentConversationStore{runtime: a.ensureDesktopRuntime()}).List(ctx, documentID)
		return agentChatHistoryResult{summaries: items}, err
	})
}

func (a *App) startAgentChatHistoryTask(kind, documentID string, selectLatest bool, run func(context.Context) (agentChatHistoryResult, error)) {
	a.cancelAgentChatHistoryRequest()
	state := &a.agentUI
	state.conversationHistoryGeneration++
	generation := state.conversationHistoryGeneration
	ctx, cancel := context.WithCancel(context.Background())
	state.conversationHistoryCancel = cancel
	state.conversationHistoryLoading = true
	state.conversationHistoryError = ""
	results := state.conversationHistoryResults
	if results == nil {
		results = make(chan agentChatHistoryResult, 8)
		state.conversationHistoryResults = results
	}
	started := a.launchManagedBackground(func() {
		defer cancel()
		result, err := run(ctx)
		result.kind, result.generation, result.documentID = kind, generation, documentID
		result.selectLatest, result.err = selectLatest, err
		select {
		case results <- result:
		case <-ctx.Done():
			return
		case <-a.shutdown:
			return
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	})
	if !started {
		cancel()
		state.conversationHistoryCancel = nil
		state.conversationHistoryLoading = false
		state.conversationHistoryError = "无法启动会话历史请求。"
	}
}

func (a *App) createAgentConversation() {
	if a.agentUI.conversationDocumentID == "" {
		a.agentUI.conversationHistoryError = "请先打开一份文档。"
		return
	}
	runtime := a.ensureDesktopRuntime()
	documentID := a.agentUI.conversationDocumentID
	a.startAgentChatHistoryTask("create", documentID, false, func(ctx context.Context) (agentChatHistoryResult, error) {
		record, err := runtime.createAgentConversation(ctx, documentID)
		if err != nil {
			return agentChatHistoryResult{}, err
		}
		snapshot := agent.ConversationSnapshot{
			ID: record.ID, DocumentID: record.DocumentID, Title: record.Title,
			Messages: record.Messages, Reasoning: record.Reasoning,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		}
		return agentChatHistoryResult{snapshot: snapshot, found: err == nil}, err
	})
}

func (a *App) loadAgentConversation(id agent.ConversationID) {
	documentID := a.agentUI.conversationDocumentID
	if documentID == "" || id == "" {
		return
	}
	host, _ := a.ensureDesktopRuntime().currentAgentHost()
	a.startAgentChatHistoryTask("load", documentID, false, func(ctx context.Context) (agentChatHistoryResult, error) {
		if host != nil {
			snapshot, found, err := host.LoadConversation(ctx, documentID, id)
			return agentChatHistoryResult{snapshot: snapshot, found: found}, err
		}
		record, found, err := (workspaceAgentConversationStore{runtime: a.ensureDesktopRuntime()}).Load(ctx, documentID, id)
		if err != nil || !found {
			return agentChatHistoryResult{found: found}, err
		}
		return agentChatHistoryResult{found: true, snapshot: agent.ConversationSnapshot{
			ID: record.ID, DocumentID: record.DocumentID, Title: record.Title, Messages: record.Messages,
			Reasoning: record.Reasoning, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
			Interrupted: agent.ConversationInterrupted(record.Messages),
		}}, nil
	})
}

func (a *App) processAgentChatHistoryResults() {
	results := a.agentUI.conversationHistoryResults
	if results == nil {
		return
	}
	for {
		select {
		case result := <-results:
			state := &a.agentUI
			if result.generation != state.conversationHistoryGeneration || result.documentID != state.conversationDocumentID ||
				result.documentID != a.agentViewContext().DocumentID {
				continue
			}
			if state.conversationHistoryCancel != nil {
				state.conversationHistoryCancel()
				state.conversationHistoryCancel = nil
			}
			state.conversationHistoryLoading = false
			if result.err != nil {
				state.conversationHistoryError = result.err.Error()
				state.chatStatus = ""
				continue
			}
			switch result.kind {
			case "list":
				state.conversationHistory = result.summaries
				state.conversationHistoryError = ""
				if result.selectLatest {
					if len(result.summaries) == 0 {
						a.createAgentConversation()
					} else {
						a.loadAgentConversation(result.summaries[0].ID)
					}
				}
			case "create", "load":
				if !result.found {
					state.conversationHistoryError = "此文档中没有找到该会话。"
					continue
				}
				state.conversationID = result.snapshot.ID
				state.conversationTitle = result.snapshot.Title
				state.chatLines = agentConversationChatLines(result.snapshot)
				state.activeReasoningLine = -1
				state.activeAnswerStarted = false
				state.conversationBusy = result.snapshot.Busy
				state.conversationInterrupted = result.snapshot.Interrupted
				state.chatStatus = ""
				if result.snapshot.Interrupted {
					if result.snapshot.Title == "" && a.ensureDesktopRuntime().workspaceStore != nil {
						state.chatStatus = "首条消息已保存，标题或回复尚未完成；点击顶部标题旁的“继续”重试。"
					} else {
						state.chatStatus = "上一轮尚未完成；点击顶部的“继续”恢复。"
					}
				}
				state.conversationHistoryVisible = false
				if host, _ := a.ensureDesktopRuntime().currentAgentHost(); host != nil {
					a.subscribeToAgent(host)
				}
				if result.kind == "create" {
					a.refreshAgentConversationHistory(false)
				}
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		default:
			return
		}
	}
}

func agentConversationChatLines(snapshot agent.ConversationSnapshot) []agentChatLine {
	lines := make([]agentChatLine, 0, len(snapshot.Messages)+len(snapshot.Reasoning))
	reasoningByIndex := make(map[int][]string)
	for _, block := range snapshot.Reasoning {
		index := block.BeforeMessage
		if index < 0 || index > len(snapshot.Messages) {
			index = len(snapshot.Messages)
		}
		reasoningByIndex[index] = append(reasoningByIndex[index], block.Content)
	}
	for index := 0; index <= len(snapshot.Messages); index++ {
		for _, reasoning := range reasoningByIndex[index] {
			lines = appendAgentChatLine(lines, newAgentReasoningLine(reasoning, true))
		}
		if index == len(snapshot.Messages) {
			break
		}
		message := snapshot.Messages[index]
		switch message.Role {
		case "user":
			lines = appendAgentChatLine(lines, agentChatLine{role: "user", text: message.Content})
		case "assistant":
			if message.Content != "" {
				lines = appendAgentChatLine(lines, agentChatLine{role: "assistant", text: message.Content})
			} else if len(message.ToolCalls) > 0 {
				lines = appendAgentChatLine(lines, agentChatLine{role: "tool", text: "Agent 请求调用工具"})
			}
		case "tool":
			name := message.ToolName
			if name == "" {
				name = "工具"
			}
			lines = appendAgentChatLine(lines, agentChatLine{role: "tool", text: name + " 的结果已保存在会话上下文中"})
		}
	}
	return lines
}

func (a *App) layoutAgentConversationToolbar(gtx layout.Context) layout.Dimensions {
	if a.agentUI.conversationTitleButton.Clicked(gtx) {
		a.agentUI.conversationHistoryVisible = !a.agentUI.conversationHistoryVisible
		if a.agentUI.conversationHistory == nil {
			a.refreshAgentConversationHistory(false)
		}
	}
	if a.agentUI.newConversationButton.Clicked(gtx) {
		a.createAgentConversation()
	}
	if a.agentUI.resumeConversationButton.Clicked(gtx) {
		a.resumeAgentChatConversation()
	}
	title := strings.TrimSpace(a.agentUI.conversationTitle)
	if title == "" {
		title = "新会话"
	}
	title += "　⌄"
	return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				style := material.Button(a.theme, &a.agentUI.conversationTitleButton, title)
				return style.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !a.agentUI.conversationInterrupted || a.agentUI.conversationBusy {
					return layout.Dimensions{}
				}
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, &a.agentUI.resumeConversationButton, "继续").Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Button(a.theme, &a.agentUI.newConversationButton, "新建会话").Layout(gtx)
			}),
		)
	})
}

func (a *App) layoutAgentConversationHistory(gtx layout.Context) layout.Dimensions {
	state := &a.agentUI
	if !state.conversationHistoryVisible {
		return layout.Dimensions{}
	}
	if state.conversationHistoryLoading && len(state.conversationHistory) == 0 {
		style := material.Caption(a.theme, "正在加载会话历史…")
		style.Color = homeMuted
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, style.Layout)
	}
	if state.conversationHistoryError != "" {
		style := material.Caption(a.theme, state.conversationHistoryError)
		style.Color = color.NRGBA{R: 178, G: 54, B: 54, A: 255}
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, style.Layout)
	}
	if len(state.conversationHistory) == 0 {
		style := material.Caption(a.theme, "此文档还没有历史会话。")
		style.Color = homeMuted
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, style.Layout)
	}
	maxHeight := gtx.Dp(unit.Dp(160))
	gtx.Constraints.Min.Y = 0
	gtx.Constraints.Max.Y = maxHeight
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(a.theme, &state.conversationHistoryList).Layout(gtx, len(state.conversationHistory), func(gtx layout.Context, index int) layout.Dimensions {
			item := state.conversationHistory[index]
			button := state.conversationHistoryButtons[item.ID]
			if button == nil {
				button = new(widget.Clickable)
				state.conversationHistoryButtons[item.ID] = button
			}
			if button.Clicked(gtx) {
				a.loadAgentConversation(item.ID)
			}
			label := strings.TrimSpace(item.Title)
			if label == "" {
				label = "新会话"
			}
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Button(a.theme, button, label).Layout(gtx)
			})
		})
	})
}
