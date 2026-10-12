package desktopui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/workspace"
)

func TestWorkspaceAgentConversationStoreRoundTripsContextByDocument(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	runtime.workspaceStore = store
	repository := workspaceAgentConversationStore{runtime: runtime}
	now := time.Now().UTC().Truncate(time.Millisecond)
	want := agent.ConversationRecord{
		ID: "chat-1", DocumentID: "doc-1", Title: "分析预算",
		Messages:  []provider.ModelMessage{{Role: "user", Content: "检查 B2"}, {Role: "assistant", Content: "B2 是空白"}},
		Reasoning: []agent.ReasoningBlock{{BeforeMessage: 1, Content: "先检查表格。"}},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, found, err := repository.Load(context.Background(), "doc-1", want.ID)
	if err != nil || !found {
		t.Fatalf("Load found=%v error=%v", found, err)
	}
	if loaded.ID != want.ID || loaded.DocumentID != want.DocumentID || loaded.Title != want.Title || len(loaded.Messages) != 2 || loaded.Messages[1].Content != want.Messages[1].Content || len(loaded.Reasoning) != 1 || !loaded.CreatedAt.Equal(now) || !loaded.UpdatedAt.Equal(now) {
		t.Fatalf("loaded conversation=%+v want=%+v", loaded, want)
	}
	listed, err := repository.List(context.Background(), "doc-1")
	if err != nil || len(listed) != 1 || listed[0].ID != want.ID || listed[0].Title != want.Title {
		t.Fatalf("List=%+v error=%v", listed, err)
	}
	if _, _, err := repository.Load(context.Background(), "doc-2", want.ID); err == nil {
		t.Fatal("cross-document Load succeeded")
	}
}

func TestRuntimeCreatesDocumentConversationWithoutAgentHost(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	runtime.workspaceStore = store
	if host, _ := runtime.currentAgentHost(); host != nil {
		t.Fatal("test runtime unexpectedly has an AgentHost")
	}
	record, err := runtime.createAgentConversation(context.Background(), "doc-without-model")
	if err != nil {
		t.Fatalf("createAgentConversation: %v", err)
	}
	if record.ID == "" || record.DocumentID != "doc-without-model" || record.Title != "" {
		t.Fatalf("created conversation=%+v", record)
	}
	loaded, found, err := (workspaceAgentConversationStore{runtime: runtime}).Load(context.Background(), record.DocumentID, record.ID)
	if err != nil || !found || loaded.ID != record.ID {
		t.Fatalf("loaded conversation=%+v found=%v error=%v", loaded, found, err)
	}
}
