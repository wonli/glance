package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestWordDocumentProtectionCapabilityRequiresHumanAndRoundTrips(t *testing.T) {
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
	agent := Operator{ID: "protection-agent", Kind: OperatorExternalAgent}
	human := Operator{ID: "desktop-user", Kind: OperatorHuman}

	getProtection := func(requestID, documentID string, revision uint64, want bool) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "word.document.protection.get",
			ExpectedRevision: revision, Operator: agent, Arguments: mustJSON(t, map[string]any{}),
		})
		if invokeErr != nil {
			t.Fatalf("read Word protection: %v", invokeErr)
		}
		var output struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil || output.Enabled != want || result.Revision != revision {
			t.Fatalf("Word protection query=%s revision=%d error=%v; want enabled=%v revision=%d", result.Data, result.Revision, err, want, revision)
		}
	}
	getProtection("word-protection-initial", doc.DocumentID, doc.Revision, false)

	pending, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-protection-agent-enable", DocumentID: doc.DocumentID,
		CapabilityID: "word.document.protection.set", ExpectedRevision: doc.Revision,
		Operator: agent, Arguments: mustJSON(t, map[string]any{"enabled": true}),
	})
	if err != nil {
		t.Fatalf("stage Word protection: %v", err)
	}
	var staged struct {
		Status         string `json:"status"`
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
		t.Fatalf("Word protection confirmation=%s error=%v", pending.Data, err)
	}
	if beforeApproval := mustGetDocumentInfo(t, runtime, doc.DocumentID); beforeApproval.Revision != doc.Revision || beforeApproval.Dirty != doc.Dirty {
		t.Fatalf("Agent changed protection before confirmation: %+v", beforeApproval)
	}
	getProtection("word-protection-still-off", doc.DocumentID, doc.Revision, false)
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, agent); !errors.Is(err, ErrHumanApprovalRequired) {
		t.Fatalf("Agent approval error=%v, want ErrHumanApprovalRequired", err)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, human); err != nil {
		t.Fatalf("human approval: %v", err)
	}
	confirmation := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationCompleted)
	if confirmation.Result == nil {
		t.Fatalf("completed protection confirmation has no result: %+v", confirmation)
	}
	var enabledResult struct {
		Changed bool `json:"changed"`
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(confirmation.Result.Data, &enabledResult); err != nil || !enabledResult.Changed || !enabledResult.Enabled {
		t.Fatalf("approved Word protection result=%s error=%v", confirmation.Result.Data, err)
	}
	enabledInfo := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if enabledInfo.Revision != doc.Revision+1 || !enabledInfo.Dirty {
		t.Fatalf("approved Word protection state=%+v; opened=%+v", enabledInfo, doc)
	}
	getProtection("word-protection-enabled", doc.DocumentID, enabledInfo.Revision, true)

	noOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-protection-enable-noop", DocumentID: doc.DocumentID,
		CapabilityID: "word.document.protection.set", ExpectedRevision: enabledInfo.Revision,
		Operator: human, Arguments: mustJSON(t, map[string]any{"enabled": true}),
	})
	if err != nil {
		t.Fatalf("repeat Word protection setting: %v", err)
	}
	var noOpResult struct {
		Changed bool `json:"changed"`
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(noOp.Data, &noOpResult); err != nil || noOpResult.Changed || !noOpResult.Enabled || noOp.Revision != enabledInfo.Revision {
		t.Fatalf("Word protection no-op=%s revision=%d error=%v", noOp.Data, noOp.Revision, err)
	}
	if afterNoOp := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterNoOp.Revision != enabledInfo.Revision || !afterNoOp.Dirty {
		t.Fatalf("Word protection no-op changed document state: %+v", afterNoOp)
	}

	disabled, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-protection-disable", DocumentID: doc.DocumentID,
		CapabilityID: "word.document.protection.set", ExpectedRevision: enabledInfo.Revision,
		Operator: human, Arguments: mustJSON(t, map[string]any{"enabled": false}),
	})
	if err != nil || disabled.Revision != enabledInfo.Revision+1 {
		t.Fatalf("disable Word protection: result=%+v error=%v", disabled, err)
	}
	var disabledResult struct {
		Changed bool `json:"changed"`
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(disabled.Data, &disabledResult); err != nil || !disabledResult.Changed || disabledResult.Enabled {
		t.Fatalf("disabled Word protection result=%s error=%v", disabled.Data, err)
	}

	savedPath := filepath.Join(t.TempDir(), "protection.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, savedPath); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, savedPath)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	if reopened.Dirty {
		t.Fatalf("reopened protected DOCX is dirty: %+v", reopened)
	}
	getProtection("word-protection-reopened", reopened.DocumentID, reopened.Revision, false)
}
