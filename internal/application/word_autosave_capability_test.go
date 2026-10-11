package application

import (
	"context"
	"encoding/json"
	"testing"
)

type wordAutosaveViewHandler struct {
	intent ViewIntent
}

func (h *wordAutosaveViewHandler) HandleViewIntent(_ context.Context, intent ViewIntent) (json.RawMessage, error) {
	h.intent = intent
	return json.RawMessage(`{"window_id":9,"enabled":true,"effective":true,"changed":true}`), nil
}

func TestWordAutosaveCapabilityRoutesExplicitStateToAttachedWindow(t *testing.T) {
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
	handler := &wordAutosaveViewHandler{}
	unregister := runtime.RegisterViewIntentHandler(9, handler)
	t.Cleanup(unregister)

	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "enable-word-autosave", DocumentID: doc.DocumentID,
		CapabilityID: "word.autosave.set", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "human", Kind: OperatorHuman},
		View:      ViewContext{WindowID: 9, DocumentID: doc.DocumentID, Revision: doc.Revision},
		Arguments: json.RawMessage(`{"enabled":true}`),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(word.autosave.set): %v", err)
	}
	if handler.intent.Action != "word.autosave.set" || handler.intent.WindowID != 9 || handler.intent.DocumentID != doc.DocumentID || string(handler.intent.Arguments) != `{"enabled":true}` {
		t.Fatalf("autosave view intent = %+v", handler.intent)
	}
	var data struct {
		Enabled   bool `json:"enabled"`
		Effective bool `json:"effective"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("decode autosave state: %v", err)
	}
	if !data.Enabled || !data.Effective || result.Revision != doc.Revision {
		t.Fatalf("autosave result=%+v revision=%d, want enabled and no document mutation", data, result.Revision)
	}
}

func TestWordAutosaveAgentRequestRequiresHumanConfirmation(t *testing.T) {
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
	handler := &wordAutosaveViewHandler{}
	unregister := runtime.RegisterViewIntentHandler(9, handler)
	t.Cleanup(unregister)
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "agent-enable-word-autosave", DocumentID: doc.DocumentID,
		CapabilityID: "word.autosave.set", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "agent", Kind: OperatorExternalAgent},
		View:      ViewContext{WindowID: 9, DocumentID: doc.DocumentID, Revision: doc.Revision},
		Arguments: json.RawMessage(`{"enabled":true}`),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(word.autosave.set): %v", err)
	}
	var confirmation struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(result.Data, &confirmation); err != nil {
		t.Fatalf("decode confirmation: %v", err)
	}
	if confirmation.Status != "confirmation_required" || handler.intent.Action != "" {
		t.Fatalf("Agent autosave result=%+v, prematurely executed intent=%+v", confirmation, handler.intent)
	}
}
