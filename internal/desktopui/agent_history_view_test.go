package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/agent"
	"glance/internal/i18n"
	"image"
	"testing"
	"time"
)

func TestAgentHistoryUsesFullSidebarHeight(t *testing.T) {
	a := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(a.desktopRuntime.shutdown)
	a.agentUI.conversationHistoryVisible = true
	for i := 0; i < 30; i++ {
		a.agentUI.conversationHistory = append(a.agentUI.conversationHistory, agent.ConversationSummary{ID: agent.ConversationID(string(rune('a' + i))), Title: "历史会话"})
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(226, 531))}
	dims := a.layoutAgentConversationHistory(gtx)
	if dims.Size.Y < 500 {
		t.Fatalf("history height=%d; want full sidebar viewport", dims.Size.Y)
	}
}

func TestAgentHistoryDeleteRequiresConfirmation(t *testing.T) {
	a := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(a.desktopRuntime.shutdown)
	a.applyLocaleSelection(i18n.SelectionChinese)
	a.agentUI.conversationDocumentID = "doc"
	item := agent.ConversationSummary{ID: "chat", Title: "优化推广计划措辞"}
	a.agentUI.conversationHistory = []agent.ConversationSummary{item}
	a.confirmAgentConversationDelete(item)
	if !a.modal.Visible || a.modal.DialogID != "agent.history.delete" || a.messageText(a.modal.TitleRef, a.modal.Title) != "删除这条会话？" {
		t.Fatal("delete confirmation missing")
	}
	if a.agentUI.conversationHistoryLoading {
		t.Fatal("deletion started before confirmation")
	}
	a.modal.OnAction("cancel")
	if a.modal.Visible || len(a.agentUI.conversationHistory) != 1 || a.agentUI.conversationHistoryLoading {
		t.Fatal("cancel changed history")
	}
}

func TestAgentHistorySearchAndDateGroups(t *testing.T) {
	a := New(nil, "")
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	a.agentUI.conversationHistory = []agent.ConversationSummary{
		{ID: "today", Title: "优化推广计划", UpdatedAt: now},
		{ID: "yesterday", Title: "渠道执行计划", UpdatedAt: now.AddDate(0, 0, -1)},
		{ID: "week", Title: "优化预算", UpdatedAt: now.AddDate(0, 0, -4)},
		{ID: "older", Title: "优化结构", UpdatedAt: now.AddDate(0, 0, -10)},
	}
	rows := a.agentHistoryRows(now)
	for _, localeCase := range []struct {
		selection i18n.Selection
		want      []string
	}{
		{i18n.SelectionEnglish, []string{"Today", "Yesterday", "Past 7 days", "Earlier"}},
		{i18n.SelectionChinese, []string{"今天", "昨天", "过去 7 天", "更早"}},
	} {
		a.applyLocaleSelection(localeCase.selection)
		rows = a.agentHistoryRows(now)
		for i, want := range localeCase.want {
			if rows[i].group != want {
				t.Fatalf("locale=%s group=%s want=%s", localeCase.selection, rows[i].group, want)
			}
		}
	}
	a.agentUI.historySearch.SetText("  优化  ")
	rows = a.agentHistoryRows(now)
	if len(rows) != 3 || rows[1].item.ID != "week" {
		t.Fatalf("Chinese title search=%+v", rows)
	}
	a.agentUI.historySearch.SetText("无匹配")
	if len(a.agentHistoryRows(now)) != 0 {
		t.Fatal("unmatched search includes conversations")
	}
}

func TestAgentHistoryDeleteFailureKeepsTranscriptAndAllowsRetry(t *testing.T) {
	r := newDesktopRuntime()
	t.Cleanup(r.shutdown)
	a := newAppWithRuntime(nil, r)
	t.Cleanup(func() { a.shutdownOnce.Do(func() { close(a.shutdown) }); a.stopAndWaitManagedBackground() })
	a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	a.documentID = "doc"
	a.agentUI.conversationDocumentID = "doc"
	a.agentUI.conversationID = "chat"
	a.agentUI.chatLines = []agentChatLine{{role: "assistant", text: "保留正文"}}
	item := agent.ConversationSummary{ID: "chat", Title: "保留会话"}
	a.agentUI.conversationHistory = []agent.ConversationSummary{item}
	a.confirmAgentConversationDelete(item)
	a.modal.OnAction("delete")
	deadline := time.Now().Add(2 * time.Second)
	for len(a.agentUI.conversationDeletes) > 0 && time.Now().Before(deadline) {
		a.processAgentChatHistoryResults()
		time.Sleep(time.Millisecond)
	}
	if a.agentUI.conversationHistoryError == "" || a.agentUI.conversationID != "chat" || a.agentUI.chatLines[0].text != "保留正文" || len(a.agentUI.conversationHistory) != 1 {
		t.Fatal("failed deletion lost conversation state")
	}
	a.confirmAgentConversationDelete(item)
	if !a.modal.Visible {
		t.Fatal("failed deletion cannot be retried")
	}
	a.modal.OnAction("cancel")
}
