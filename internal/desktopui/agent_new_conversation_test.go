package desktopui

import (
	"testing"

	"glance/internal/agent"
)

func TestNewAgentConversationClearsLimitErrorBeforeAsyncCreate(t *testing.T) {
	r := newDesktopRuntime()
	t.Cleanup(r.shutdown)
	a := newAppWithRuntime(nil, r)
	a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	a.documentID = "new-conversation-test"
	a.agentUI.conversationDocumentID = a.documentID
	a.agentUI.conversationID = "old-conversation"
	a.agentUI.conversationInterrupted = true
	a.agentUI.chatLines = []agentChatLine{{role: "error", text: "本轮工具调用次数达到上限"}}
	a.agentUI.chatInput.SetText("old draft")
	a.createAgentConversation()
	if a.agentUI.conversationID != "" || len(a.agentUI.chatLines) != 0 || a.agentUI.conversationInterrupted || a.agentUI.chatInput.Text() != "" {
		t.Fatal("new conversation kept old ID, error or draft while creating")
	}
	a.agentUI.chatEvents <- agent.AgentEvent{ConversationID: "old-conversation", Type: agent.EventTurnFailed, Error: "late old limit error"}
	a.processAgentChatEvents()
	if len(a.agentUI.chatLines) != 0 {
		t.Fatal("late failure from old conversation entered new conversation")
	}
}
