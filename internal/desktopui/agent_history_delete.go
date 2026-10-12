package desktopui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gioui.org/layout"
	"glance/internal/agent"
)

func (a *App) confirmAgentConversationDelete(item agent.ConversationSummary) {
	if a.agentUI.conversationDeletes[item.ID] != "" {
		return
	}
	doc := a.agentUI.conversationDocumentID
	title := item.Title
	if title == "" {
		title = "新会话"
	}
	a.showConfirmDialog(
		"删除这条会话？",
		fmt.Sprintf("「%s」及其全部消息将被永久删除，无法恢复。文档内容不受影响。", title),
		modalDialogDangerConfirmStyle,
		func(action string) {
			a.modal.Hide()
			if action != "delete" || doc != a.agentUI.conversationDocumentID {
				return
			}
			a.deleteAgentConversation(doc, item.ID)
		},
		modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary},
		modalDialogActionSpec{ID: "delete", Label: "删除", Style: modalDialogActionDanger},
	)
}

func (a *App) deleteAgentConversation(documentID string, id agent.ConversationID) {
	state := &a.agentUI
	if state.conversationDeletes == nil {
		state.conversationDeletes = make(map[agent.ConversationID]string)
	}
	if state.conversationDeletes[id] != "" {
		return
	}
	state.conversationDeletes[id] = documentID
	if state.conversationDeleteResults == nil {
		state.conversationDeleteResults = make(chan agentChatHistoryResult, 16)
	}
	results := state.conversationDeleteResults
	runtime := a.ensureDesktopRuntime()
	host, _ := runtime.currentAgentHost()
	if !a.launchManagedBackground(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		result := agentChatHistoryResult{kind: "delete", documentID: documentID, deletedID: id}
		if host == nil {
			result.err = (workspaceAgentConversationStore{runtime: runtime}).Delete(ctx, documentID, id)
		} else {
			if snap, ok := host.Snapshot(id); ok && snap.Busy {
				_ = host.Cancel(id)
			}
			for {
				result.err = host.DeleteConversation(ctx, documentID, id)
				if !errors.Is(result.err, agent.ErrConversationBusy) {
					break
				}
				timer := time.NewTimer(20 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					result.err = ctx.Err()
				case <-a.shutdown:
					timer.Stop()
					return
				case <-timer.C:
					continue
				}
				break
			}
		}
		select {
		case results <- result:
		case <-a.shutdown:
			return
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		delete(state.conversationDeletes, id)
		state.conversationHistoryError = "无法启动删除会话任务。"
	}
}

func (a *App) processAgentConversationDeletes() {
	state := &a.agentUI
	for {
		select {
		case result := <-state.conversationDeleteResults:
			delete(state.conversationDeletes, result.deletedID)
			if result.documentID != state.conversationDocumentID || result.documentID != a.agentViewContext().DocumentID {
				continue
			}
			if result.err != nil {
				state.conversationHistoryError = "删除会话失败：" + result.err.Error()
				continue
			}
			// Filter any list/load snapshots captured before removal without cancelling newer work.
			if state.conversationDeletedIDs == nil {
				state.conversationDeletedIDs = make(map[agent.ConversationID]struct{})
			}
			state.conversationDeletedIDs[result.deletedID] = struct{}{}
			if state.conversationID == result.deletedID {
				a.closeAgentSubscription()
				state.conversationID = ""
				state.conversationTitle = ""
				state.conversationStartedAt = time.Time{}
				state.chatLines = nil
				state.chatInput.SetText("")
				state.conversationBusy = false
				state.conversationInterrupted = false
				state.chatStatus = ""
				state.chatList.Position = layout.Position{}
				state.activeReasoningLine = -1
				state.activeAnswerStarted = false
			}
			for i, item := range state.conversationHistory {
				if item.ID == result.deletedID {
					state.conversationHistory = append(state.conversationHistory[:i], state.conversationHistory[i+1:]...)
					break
				}
			}
			delete(state.conversationHistoryButtons, result.deletedID)
			delete(state.conversationHistoryDeleteButtons, result.deletedID)
			state.conversationHistoryLoaded = true
			state.conversationHistoryError = ""
		default:
			return
		}
	}
}

func (a *App) layoutAgentDeleteDialog(gtx layout.Context) layout.Dimensions {
	return a.layoutConfirmDialogCard(gtx)
}
