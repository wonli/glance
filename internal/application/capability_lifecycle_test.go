package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentCanCreateEditAndSaveNewWordDocument(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	operator := Operator{ID: "codex", Kind: OperatorExternalAgent}
	created, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "create-word", CapabilityID: "document.create", Operator: operator,
		Arguments: json.RawMessage(`{"format":"docx"}`),
	})
	if err != nil {
		t.Fatalf("create Word document: %v", err)
	}
	var info DocumentInfo
	if err := json.Unmarshal(created.Data, &info); err != nil {
		t.Fatalf("decode created document: %v", err)
	}
	if info.DocumentID == "" || info.Format != FormatDocx {
		t.Fatalf("created document info = %+v", info)
	}
	snapshot, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "inspect-word", CapabilityID: "document.inspect", DocumentID: info.DocumentID,
		Operator: operator, Arguments: json.RawMessage(`{"limit":10}`),
	})
	if err != nil {
		t.Fatalf("inspect new Word document: %v", err)
	}
	var result struct {
		Content struct {
			Blocks []struct {
				Runs []struct {
					ID       string `json:"id"`
					Editable bool   `json:"editable"`
				} `json:"runs"`
			} `json:"blocks"`
		} `json:"content"`
	}
	if err := json.Unmarshal(snapshot.Data, &result); err != nil {
		t.Fatalf("decode Word snapshot: %v", err)
	}
	var runID string
	for _, block := range result.Content.Blocks {
		for _, run := range block.Runs {
			if run.Editable {
				runID = run.ID
				break
			}
		}
	}
	if runID == "" {
		t.Fatalf("new Word document has no editable text target: %s", snapshot.Data)
	}
	updated, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "write-word", CapabilityID: "word.text_run.replace", DocumentID: info.DocumentID,
		ExpectedRevision: info.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"run_id": runID, "value": "本周完成项目总结"}),
	})
	if err != nil {
		t.Fatalf("write Word text: %v", err)
	}
	path := filepath.Join(t.TempDir(), "周报.docx")
	saved, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "save-word", CapabilityID: "document.save_as", DocumentID: info.DocumentID,
		ExpectedRevision: updated.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"path": path}),
	})
	if err != nil {
		t.Fatalf("save Word document: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved Word file is missing: %v", err)
	}
	var savedInfo DocumentInfo
	if err := json.Unmarshal(saved.Data, &savedInfo); err != nil {
		t.Fatalf("decode saved document: %v", err)
	}
	if savedInfo.Dirty || savedInfo.SourcePath != canonicalPath(path) {
		t.Fatalf("saved document info = %+v", savedInfo)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "close-word", CapabilityID: "document.close", DocumentID: info.DocumentID,
		ExpectedRevision: saved.Revision, Operator: operator, Arguments: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("close saved Word document: %v", err)
	}
}
