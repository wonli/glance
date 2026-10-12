package workspace

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentConversationsAreDocumentScopedAndSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	older := AgentConversation{ID: "chat-old", DocumentID: "doc-a", Title: "旧会话", Messages: []byte(`[ {"role":"user","content":"旧上下文"} ]`), Reasoning: []byte(`["先想一下"]`), CreatedAt: time.Unix(100, 0).UTC(), UpdatedAt: time.Unix(200, 0).UTC()}
	newer := AgentConversation{ID: "chat-new", DocumentID: "doc-a", Title: "新会话", Messages: []byte(`[ {"role":"assistant","content":"保留的回答"} ]`), CreatedAt: time.Unix(300, 0).UTC(), UpdatedAt: time.Unix(400, 0).UTC()}
	other := AgentConversation{ID: "chat-other", DocumentID: "doc-b", Title: "另一文档", Messages: []byte(`[]`), CreatedAt: time.Unix(500, 0).UTC(), UpdatedAt: time.Unix(600, 0).UTC()}
	for _, conversation := range []AgentConversation{older, newer, other} {
		if err := store.SaveAgentConversation(conversation); err != nil {
			t.Fatalf("SaveAgentConversation(%s): %v", conversation.ID, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	listed, err := store.ListAgentConversations("doc-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != "chat-new" || listed[1].ID != "chat-old" {
		t.Fatalf("document conversation history=%+v", listed)
	}
	loaded, found, err := store.LoadAgentConversation("doc-a", "chat-old")
	if err != nil || !found {
		t.Fatalf("LoadAgentConversation found=%v error=%v", found, err)
	}
	if loaded.Title != older.Title || !bytes.Equal(loaded.Messages, older.Messages) || !bytes.Equal(loaded.Reasoning, older.Reasoning) || !loaded.CreatedAt.Equal(older.CreatedAt) || !loaded.UpdatedAt.Equal(older.UpdatedAt) {
		t.Fatalf("conversation did not survive restart: got %+v want %+v", loaded, older)
	}
	if _, found, err := store.LoadAgentConversation("doc-b", "chat-old"); err == nil || found {
		t.Fatalf("cross-document load found=%v error=%v", found, err)
	}
	if _, err := store.ListAgentConversations(""); err == nil {
		t.Fatal("listing without a document ID succeeded")
	}
	if err := store.SaveAgentConversation(AgentConversation{ID: "chat-old", DocumentID: "doc-b", Title: "移出原文档", Messages: []byte(`[]`)}); err == nil {
		t.Fatal("conversation ID was reassigned to another document")
	}
}
