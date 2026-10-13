package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type testDocumentsaveEmptyArguments struct {
}

type testDocumentsaveTextArguments struct {
	Text string `json:"text"`
}

func TestDocumentSaveCapabilityRequiresHumanAndWritesLiveSession(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	path := filepath.Join(t.TempDir(), "weekly-report.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs initial file: %v", err)
	}
	clean, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || clean.Dirty {
		t.Fatalf("initial clean state=%+v error=%v", clean, err)
	}
	initialBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read initial DOCX: %v", err)
	}

	edit, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "save-fixture-edit", DocumentID: doc.DocumentID, CapabilityID: "word.paragraph.append",
		ExpectedRevision: clean.Revision, Operator: Operator{ID: "save-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, testDocumentsaveTextArguments{Text: "本周汇总已完成"}),
	})
	if err != nil {
		t.Fatalf("edit live document: %v", err)
	}
	if !mustGetDocumentInfo(t, runtime, doc.DocumentID).Dirty {
		t.Fatal("Word edit did not mark the source session dirty")
	}
	agent := Operator{ID: "save-agent", Kind: OperatorExternalAgent}
	human := Operator{ID: "desktop-user", Kind: OperatorHuman}
	pending, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "agent-save-existing-file", DocumentID: doc.DocumentID, CapabilityID: "document.save",
		ExpectedRevision: edit.Revision, Operator: agent, Arguments: mustJSON(t, testDocumentsaveEmptyArguments{}),
	})
	if err != nil {
		t.Fatalf("stage Agent save: %v", err)
	}
	var staged struct {
		Status         string `json:"status"`
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
		t.Fatalf("staged save=%s error=%v", pending.Data, err)
	}
	confirmationState, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "agent-poll-save-confirmation", CapabilityID: "confirmation.get", Operator: agent,
		Arguments: mustJSON(t, map[string]string{"confirmation_id": staged.ConfirmationID}),
	})
	if err != nil {
		t.Fatalf("Agent read pending confirmation: %v", err)
	}
	var pendingState struct {
		Status       string `json:"status"`
		CapabilityID string `json:"capability_id"`
		Result       any    `json:"result"`
	}
	if err := json.Unmarshal(confirmationState.Data, &pendingState); err != nil || pendingState.Status != string(ConfirmationPending) || pendingState.CapabilityID != "document.save" || pendingState.Result != nil {
		t.Fatalf("Agent confirmation status=%s error=%v", confirmationState.Data, err)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, agent); err == nil {
		t.Fatal("Agent approved its own write to the source file")
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != string(initialBytes) {
		t.Fatalf("source file changed before human approval: read_error=%v", err)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, human); err != nil {
		t.Fatalf("human approval: %v", err)
	}
	confirmation := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationCompleted)
	if confirmation.Result == nil {
		t.Fatalf("completed save confirmation has no result: %+v", confirmation)
	}
	completedState, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "agent-poll-completed-save", CapabilityID: "confirmation.get", Operator: agent,
		Arguments: mustJSON(t, map[string]string{"confirmation_id": staged.ConfirmationID}),
	})
	if err != nil || !containsJSONText(completedState.Data, `"status":"completed"`) {
		t.Fatalf("Agent read completed confirmation: data=%s error=%v", completedState.Data, err)
	}
	var saved struct {
		Saved    bool         `json:"saved"`
		Document DocumentInfo `json:"document"`
	}
	if err := json.Unmarshal(confirmation.Result.Data, &saved); err != nil || !saved.Saved || saved.Document.Dirty || saved.Document.Revision != edit.Revision {
		t.Fatalf("approved save result=%s error=%v", confirmation.Result.Data, err)
	}
	if after := mustGetDocumentInfo(t, runtime, doc.DocumentID); after.Dirty || after.Revision != edit.Revision {
		t.Fatalf("save changed session revision or left it dirty: %+v", after)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close saved document: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen saved DOCX: %v", err)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID, Limit: 1000})
	if err != nil || !containsJSONText(snapshot.Content, "本周汇总已完成") {
		t.Fatalf("saved DOCX content missing after reopen: content=%s error=%v", snapshot.Content, err)
	}

	noOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "human-save-clean-document", DocumentID: reopened.DocumentID, CapabilityID: "document.save",
		ExpectedRevision: reopened.Revision, Operator: human, Arguments: mustJSON(t, testDocumentsaveEmptyArguments{}),
	})
	if err != nil {
		t.Fatalf("save clean document: %v", err)
	}
	var noOpResult struct {
		Saved    bool         `json:"saved"`
		Document DocumentInfo `json:"document"`
	}
	if err := json.Unmarshal(noOp.Data, &noOpResult); err != nil || noOpResult.Saved || noOpResult.Document.Dirty || noOp.Revision != reopened.Revision {
		t.Fatalf("clean save result=%s revision=%d error=%v", noOp.Data, noOp.Revision, err)
	}
}
