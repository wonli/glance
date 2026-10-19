package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"glance/internal/i18n"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"glance/internal/agent"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/workspace"
)

func (a *App) showAgentChat() {
	view := a.agentViewContext()
	if view.DocumentID != "" && view.DocumentID != GlobalAIAssistantDocumentID {
		a.agentUI.sidebarVisible = true
		a.ensureAgentChatDocument(view.DocumentID)
		if a.modal.Visible && a.modal.DialogID == dialogIDAgentAssistant {
			a.modal.Hide()
		}
		return
	}
	a.resetHomeContentScroll()
	a.ensureGlobalAIAssistant()
	a.homeSection = homeSectionAIAssistant
	if a.modal.Visible && a.modal.DialogID == dialogIDAgentAssistant {
		a.modal.Hide()
	}
}

// layoutAgentChat keeps the existing interaction harness pointed at the same
// chat body while production opens it only in the document workspace sidebar.
func (a *App) layoutAgentChat(gtx layout.Context) layout.Dimensions {
	return a.layoutAgentSidebar(gtx)
}

func (a *App) layoutAgentConfirmations(gtx layout.Context) layout.Dimensions {
	return a.layoutAgentConfirmationModal(gtx)
}

func (a *App) resolveAgentConfirmation(card *agentConfirmationCard, approved bool) {
	if a == nil || card == nil || card.info.Status != application.ConfirmationPending {
		return
	}
	runtime := a.ensureDesktopRuntime().applicationRuntime
	if runtime == nil {
		card.info.ResolvedBy = fmt.Sprintf("desktop-window-%d", a.windowID)
		if approved {
			card.info.Status = application.ConfirmationRunning
		} else {
			card.info.Status = application.ConfirmationRejected
		}
		a.updateAgentConfirmationChatLine(card.info, false)
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	operator := application.Operator{ID: fmt.Sprintf("desktop-window-%d", a.windowID), Kind: application.OperatorHuman}
	if err := runtime.ResolveConfirmation(context.Background(), card.info.ConfirmationID, approved, operator); err != nil {
		if !errors.Is(err, application.ErrConfirmationNotFound) {
			a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
			return
		}
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

func (a *App) sendAgentChatMessage() {
	if a.agentUI.conversationDeletes[a.agentUI.conversationID] != "" {
		return
	}
	if editorHasActiveComposition(&a.agentUI.chatInput) {
		return
	}
	if a.agentUI.conversationInterrupted {
		if strings.TrimSpace(a.agentUI.chatInput.Text()) == a.textID("common.action.continue") {
			a.agentUI.chatInput.SetText("")
			a.resumeAgentChatConversation()
		} else {
			a.clearAgentChatStatus()
		}
		return
	}
	message := strings.TrimSpace(a.agentUI.chatInput.Text())
	if message == "" || a.agentUI.conversationBusy {
		return
	}
	view := a.agentViewContext()
	if view.DocumentID == "" {
		a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.no_document"})
		return
	}
	if a.agentUI.conversationID == "" {
		a.ensureAgentChatDocument(view.DocumentID)
		a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.preparing_document"})
		return
	}
	host, _ := a.ensureDesktopRuntime().currentAgentHost()
	if host == nil {
		a.agentUI.chatLines = appendAgentChatLine(a.agentUI.chatLines, agentChatLine{role: "error", textRef: i18n.Message{ID: "agent.chat.error.configure_model"}})
		a.agentUI.chatInput.SetText("")
		return
	}
	a.subscribeToAgent(host)
	a.agentUI.activeReasoningLine = -1
	a.agentUI.activeAnswerStarted = false
	a.agentUI.chatInput.SetText("")
	a.agentUI.chatLines = appendAgentChatLine(a.agentUI.chatLines, agentChatLine{role: "user", text: message})
	a.agentUI.conversationBusy = true
	a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.contacting_model"})
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
		a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.no_model"})
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
	a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.resuming"})
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
		a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.resume_failed"})
	}
}

func (a *App) stopAgentChatTurn() {
	host, _ := a.ensureDesktopRuntime().currentAgentHost()
	if host == nil || !a.agentUI.conversationBusy {
		return
	}
	a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.stopping"})
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
		a.setAgentChatStatus(i18n.Message{ID: "agent.chat.status.stop_failed"})
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
						a.agentUI.chatLines[lineIndex].inProgress = false
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
					line.inProgress = true
					line.startedAt = time.Now()
					a.appendLiveAgentChatLine(line)
					a.agentUI.activeReasoningLine = len(a.agentUI.chatLines) - 1
				}
			case agent.EventAssistantDelta:
				a.agentUI.activeAnswerStarted = true
				a.collapseCurrentAgentReasoning()
				a.completeActiveAgentProcessLines()
				if len(a.agentUI.chatLines) > 0 && a.agentUI.chatLines[len(a.agentUI.chatLines)-1].role == "assistant" {
					last := &a.agentUI.chatLines[len(a.agentUI.chatLines)-1]
					last.text += event.Text
					last.ensureMarkdownBlocks()
				} else {
					a.appendLiveAgentChatLine(newAgentAssistantLine(event.Text))
				}
			case agent.EventConversationTitle:
				a.agentUI.conversationTitle = event.Title
				a.refreshAgentConversationHistory(false)
			case agent.EventToolStarted:
				a.collapseCurrentAgentReasoning()
				a.agentUI.activeReasoningLine = -1
				a.agentUI.activeAnswerStarted = false
				a.setAgentChatStatus(i18n.Message{ID: "agent.tool.status.running", Args: i18n.Args{"Name": a.agentToolDisplayName(event.ToolName)}})
				a.appendLiveAgentChatLine(agentChatLine{
					role:           "tool",
					toolName:       event.ToolName,
					toolSummaryRef: i18n.Message{ID: "agent.tool.result.running"},
					inProgress:     true,
					collapsed:      true,
					startedAt:      time.Now(),
				})
			case agent.EventToolCompleted:
				a.clearAgentChatStatus()
				needsAuth := agentToolResultNeedsConfirmation(event.Result)
				summaryRef, summary := agentToolResultSummaryReference(event.ToolName, event.Result, event.IsError)
				if len(a.agentUI.chatLines) > 0 && a.agentUI.chatLines[len(a.agentUI.chatLines)-1].role == "tool" {
					last := &a.agentUI.chatLines[len(a.agentUI.chatLines)-1]
					last.toolName = event.ToolName
					last.toolSummary = summary
					last.toolSummaryRef = summaryRef
					last.inProgress = false
					last.isError = event.IsError
					last.needsAuth = needsAuth
					last.collapsed = true
					if !last.startedAt.IsZero() && last.duration == 0 {
						last.duration = time.Since(last.startedAt)
					}
				} else {
					a.appendLiveAgentChatLine(agentChatLine{
						role:           "tool",
						toolName:       event.ToolName,
						toolSummary:    summary,
						toolSummaryRef: summaryRef,
						isError:        event.IsError,
						needsAuth:      needsAuth,
						collapsed:      true,
					})
				}
			case agent.EventTurnCompleted:
				a.collapseCurrentAgentReasoning()
				a.completeActiveAgentProcessLines()
				a.agentUI.conversationBusy = false
				a.agentUI.conversationInterrupted = false
				a.clearAgentChatStatus()
				a.agentUI.activeReasoningLine = -1
				a.agentUI.activeAnswerStarted = false
				a.refreshAgentConversationHistory(false)
			case agent.EventTurnFailed:
				a.collapseCurrentAgentReasoning()
				a.completeActiveAgentProcessLines()
				a.agentUI.conversationBusy = false
				a.clearAgentChatStatus()
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
						if !snapshot.Interrupted && optimisticMessage != "" {
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
					a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
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
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.a_high_impact_action_is_waiting_for_approv_d96fc1"}, feedbackInfo, notificationSuppressed)
					if a.window != nil && !a.modal.Visible && !a.agentUI.sidebarVisible {
						a.showAgentChat()
					}
				} else {
					a.updateAgentConfirmationChatLine(info, false)
					switch info.Status {
					case application.ConfirmationCompleted:
						a.setStatusMessage(i18n.Message{ID: "agent.confirmation.completed", Args: i18n.Args{"Capability": info.CapabilityName}}, feedbackSuccess, notificationToast)
					case application.ConfirmationRejected:
						a.setStatusMessage(i18n.Message{ID: "agent.confirmation.rejected", Args: i18n.Args{"Capability": info.CapabilityName}}, feedbackWarning, notificationToast)
					case application.ConfirmationFailed:
						a.setStatusDetailMessage(i18n.Message{ID: "agent.confirmation.failed"}, feedbackError, notificationToast, info.Error)
					case application.ConfirmationExpired:
						a.setStatusMessage(i18n.Message{ID: "agent.confirmation.expired", Args: i18n.Args{"Capability": info.CapabilityName}}, feedbackWarning, notificationToast)
					case application.ConfirmationCancelled:
						a.setStatusMessage(i18n.Message{ID: "agent.confirmation.cancelled", Args: i18n.Args{"Capability": info.CapabilityName}}, feedbackInfo, notificationSuppressed)
					}
				}
				continue
			}
			if event.Type != application.EventDocumentOpened && event.Type != application.EventDocumentSaved {
				continue
			}
			var info application.DocumentInfo
			if err := json.Unmarshal(event.Data, &info); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
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
					a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
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
						a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
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
	infoSnapshot := info
	text := a.agentConfirmationChatSummary(info)
	for index := len(a.agentUI.chatLines) - 1; index >= 0; index-- {
		if a.agentUI.chatLines[index].confirmationID == info.ConfirmationID {
			a.agentUI.chatLines[index].text = text
			a.agentUI.chatLines[index].confirmation = &infoSnapshot
			return
		}
	}
	if !allowAppend || info.Status != application.ConfirmationPending {
		return
	}
	a.agentUI.chatLines = appendAgentChatLine(a.agentUI.chatLines, agentChatLine{
		role: "confirmation", text: text, confirmationID: info.ConfirmationID, confirmation: &infoSnapshot,
	})
}

func (a *App) agentConfirmationChatSummary(info application.ConfirmationInfo) string {
	statusID := "agent.confirmation.status.pending"
	switch info.Status {
	case application.ConfirmationRunning:
		statusID = "agent.confirmation.status.running"
	case application.ConfirmationCompleted:
		statusID = "agent.confirmation.status.completed"
	case application.ConfirmationRejected:
		statusID = "agent.confirmation.status.rejected"
	case application.ConfirmationFailed:
		statusID = "agent.confirmation.status.failed"
	case application.ConfirmationExpired:
		statusID = "agent.confirmation.status.expired"
	case application.ConfirmationCancelled:
		statusID = "agent.confirmation.status.cancelled"
	}
	documentLabel := info.DocumentLabel
	if documentLabel == "" {
		documentLabel = info.DocumentID
	}
	resultRevision := ""
	if info.Status == application.ConfirmationCompleted && info.Result != nil && info.Result.Revision != 0 {
		resultRevision = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(info.Result.Revision))
	}
	return a.textArgs("agent.confirmation.chat_line", i18n.Args{
		"Status": a.textID(statusID), "Action": a.agentConfirmationActionLabel(info), "Document": documentLabel,
		"DocumentID": info.DocumentID, "Revision": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(info.ExpectedRevision)),
		"ResultRevision": resultRevision, "Error": info.Error,
	})
}

func (a *App) agentChatLineDisplayText(line agentChatLine) string {
	if line.confirmation != nil {
		return a.agentConfirmationChatSummary(*line.confirmation)
	}
	if line.textRef.ID != "" {
		return a.text(line.textRef)
	}
	return line.text
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
		line := &a.agentUI.chatLines[index]
		line.collapsed = true
		line.inProgress = false
		if !line.startedAt.IsZero() && line.duration == 0 {
			line.duration = time.Since(line.startedAt)
		}
	}
}

func (a *App) completeActiveAgentProcessLines() {
	for index := len(a.agentUI.chatLines) - 1; index >= 0; index-- {
		line := &a.agentUI.chatLines[index]
		if line.role == "user" {
			break
		}
		if line.role == "reasoning" || line.role == "tool" {
			line.collapsed = true
			line.inProgress = false
			if !line.startedAt.IsZero() && line.duration == 0 {
				line.duration = time.Since(line.startedAt)
			}
		}
	}
}

func (a *App) agentViewContext() application.ViewContext {
	view := application.ViewContext{WindowID: a.windowID}
	state := a.documentSessionState
	if a.homeSection == homeSectionAIAssistant && (a.activeTabID == "" || a.activeTabID == "recent" || state == nil || state.sharedDocumentSession == nil) {
		view.DocumentID = GlobalAIAssistantDocumentID
		return view
	}
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
