package desktopui

import (
	"context"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

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
			a.agentUI.conversationStartedAt = time.Time{}
			a.agentUI.conversationHistory = nil
			a.agentUI.conversationHistoryLoaded = false
			a.agentUI.chatLines = nil
			a.agentUI.chatList.Position = layout.Position{}
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
			}
		}
		if (a.agentUI.sidebarVisible || a.homeSection == homeSectionAIAssistant) && a.agentUI.conversationHistory == nil &&
			!a.agentUI.conversationHistoryLoaded && !a.agentUI.conversationHistoryLoading {
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
	a.agentUI.conversationStartedAt = time.Time{}
	a.agentUI.conversationHistory = nil
	a.agentUI.conversationHistoryLoaded = false
	a.agentUI.conversationHistoryButtons = make(map[agent.ConversationID]*widget.Clickable)
	a.agentUI.conversationHistoryDeleteButtons = make(map[agent.ConversationID]*widget.Clickable)
	a.agentUI.historySearch.SetText("")
	a.agentUI.conversationHistoryList.Position = layout.Position{}
	a.agentUI.chatLines = nil
	a.agentUI.chatList.Position = layout.Position{}
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
		if a.homeSection == homeSectionAIAssistant {
			documentID = GlobalAIAssistantDocumentID
			a.agentUI.conversationDocumentID = documentID
		} else {
			return
		}
	}
	if a.desktopRuntime == nil || a.desktopRuntime.workspaceStore == nil {
		a.agentUI.conversationHistoryLoaded = true
		a.agentUI.conversationHistoryError = "工作区会话存储不可用，无法加载历史会话。"
		a.agentUI.chatStatus = a.agentUI.conversationHistoryError
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
	if kind == "list" {
		state.conversationHistoryLoaded = false
	}
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
		if kind == "list" {
			state.conversationHistoryLoaded = true
		}
		state.conversationHistoryError = "无法启动会话历史请求。"
	}
}

func (a *App) createAgentConversation() {
	if a.agentUI.conversationDocumentID == "" {
		if a.homeSection == homeSectionAIAssistant {
			a.agentUI.conversationDocumentID = GlobalAIAssistantDocumentID
		} else {
			a.agentUI.conversationHistoryError = "请先打开一份文档。"
			return
		}
	}
	runtime := a.ensureDesktopRuntime()
	documentID := a.agentUI.conversationDocumentID
	oldID, oldBusy := a.agentUI.conversationID, a.agentUI.conversationBusy
	if oldBusy && oldID != "" {
		if host, _ := runtime.currentAgentHost(); host != nil {
			a.launchManagedBackground(func() { _ = host.Cancel(oldID) })
		}
	}
	a.closeAgentSubscription()
	// Detach immediately, before asynchronous creation. Old turn failures must
	// not be accepted while the new conversation has not received its ID yet.
	a.agentUI.conversationID = ""
	a.agentUI.conversationTitle = ""
	a.agentUI.conversationStartedAt = time.Time{}
	a.agentUI.conversationBusy = false
	a.agentUI.conversationInterrupted = false
	a.agentUI.conversationHistoryVisible = false
	a.agentUI.chatLines = nil
	a.agentUI.chatList.Position = layout.Position{}
	a.agentUI.chatInput.SetText("")
	a.agentUI.activeReasoningLine = -1
	a.agentUI.activeAnswerStarted = false
	a.agentUI.chatStatus = "正在创建新会话…"
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
	a.processAgentConversationDeletes()
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
			if result.kind == "list" {
				state.conversationHistoryLoaded = true
			}
			if result.err != nil {
				state.conversationHistoryError = result.err.Error()
				state.chatStatus = "会话操作失败：" + result.err.Error()
				continue
			}
			switch result.kind {
			case "list":
				items := result.summaries[:0]
				for _, item := range result.summaries {
					if _, deleted := state.conversationDeletedIDs[item.ID]; !deleted {
						items = append(items, item)
					}
				}
				result.summaries = items
				state.conversationHistory = items
				state.conversationHistoryError = ""
				if result.selectLatest {
					if len(result.summaries) == 0 {
						a.createAgentConversation()
					} else {
						a.loadAgentConversation(result.summaries[0].ID)
					}
				}
			case "create", "load":
				if _, deleted := state.conversationDeletedIDs[result.snapshot.ID]; deleted {
					continue
				}
				if !result.found {
					state.conversationHistoryError = "此文档中没有找到该会话。"
					continue
				}
				state.conversationID = result.snapshot.ID
				state.conversationTitle = result.snapshot.Title
				state.conversationStartedAt = result.snapshot.CreatedAt
				state.chatLines = agentConversationChatLines(result.snapshot)
				state.chatList.Position = layout.Position{}
				state.activeReasoningLine = -1
				state.activeAnswerStarted = false
				state.conversationBusy = result.snapshot.Busy
				state.conversationInterrupted = result.snapshot.Interrupted
				state.chatStatus = ""
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
	completedCallIDs := make(map[string]bool)
	for _, message := range snapshot.Messages {
		if message.Role == "tool" && message.ToolCallID != "" {
			completedCallIDs[message.ToolCallID] = true
		}
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
				lines = appendAgentChatLine(lines, newAgentAssistantLine(message.Content))
			} else if len(message.ToolCalls) > 0 {
				for _, call := range message.ToolCalls {
					if call.ID != "" && completedCallIDs[call.ID] {
						continue
					}
					name := call.Function.Name
					if name == "" {
						name = "工具"
					}
					lines = appendAgentChatLine(lines, agentChatLine{
						role:        "tool",
						text:        "Agent 请求调用 " + name,
						toolName:    name,
						toolSummary: "等待执行",
						collapsed:   true,
					})
				}
			}
		case "tool":
			name := message.ToolName
			if name == "" {
				name = "工具"
			}
			raw := []byte(message.Content)
			isErr := agentToolResultHasError(raw)
			needsAuth := agentToolResultNeedsConfirmation(raw)
			lines = appendAgentChatLine(lines, agentChatLine{
				role:        "tool",
				text:        name + " 的结果已保存在会话上下文中",
				toolName:    name,
				toolSummary: summarizeAgentToolResult(name, raw, isErr),
				isError:     isErr,
				needsAuth:   needsAuth,
				collapsed:   true,
			})
		}
	}
	return lines
}
