package application

import (
	"context"
	"encoding/json"
	"testing"
)

type wordSelectionViewHandler struct {
	intent ViewIntent
}

func (h *wordSelectionViewHandler) HandleViewIntent(_ context.Context, intent ViewIntent) (json.RawMessage, error) {
	h.intent = intent
	return json.RawMessage(`{"window_id":9,"mode":"continuous","reading_mode":false,"ruler_visible":false,"gridlines_visible":false,"navigation_visible":false,"page_fit":false,"multiple_pages":false,"zoom_percent":100,"changed":true,"selection":{"scope":"range","start_run_id":"run-a","start_offset":1,"end_run_id":"run-b","end_offset":3,"selected_runes":5}}`), nil
}

func TestWordViewSetRoutesExplicitTextSelectionToAttachedWindow(t *testing.T) {
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
	handler := &wordSelectionViewHandler{}
	unregister := runtime.RegisterViewIntentHandler(9, handler)
	t.Cleanup(unregister)
	arguments := json.RawMessage(`{"selection":{"scope":"range","start_run_id":"run-a","start_offset":1,"end_run_id":"run-b","end_offset":3}}`)
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "select-word-range", DocumentID: doc.DocumentID, CapabilityID: "word.view.set",
		ExpectedRevision: doc.Revision,
		Operator:         Operator{ID: "agent", Kind: OperatorExternalAgent},
		View:             ViewContext{WindowID: 9, DocumentID: doc.DocumentID, Revision: doc.Revision},
		Arguments:        arguments,
	})
	if err != nil {
		t.Fatalf("InvokeCapability(word.view.set selection): %v", err)
	}
	if handler.intent.Action != "word.view.set" || string(handler.intent.Arguments) != string(arguments) {
		t.Fatalf("selection view intent = %+v", handler.intent)
	}
	var selection struct {
		Scope         string `json:"scope"`
		StartRunID    string `json:"start_run_id"`
		StartOffset   int    `json:"start_offset"`
		EndRunID      string `json:"end_run_id"`
		EndOffset     int    `json:"end_offset"`
		SelectedRunes int    `json:"selected_runes"`
	}
	var output struct {
		Selection struct {
			Scope         string `json:"scope"`
			StartRunID    string `json:"start_run_id"`
			StartOffset   int    `json:"start_offset"`
			EndRunID      string `json:"end_run_id"`
			EndOffset     int    `json:"end_offset"`
			SelectedRunes int    `json:"selected_runes"`
		} `json:"selection"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil {
		t.Fatalf("decode view result: %v", err)
	}
	selection = output.Selection
	if selection.Scope != "range" || selection.StartRunID != "run-a" || selection.StartOffset != 1 || selection.EndRunID != "run-b" || selection.EndOffset != 3 || selection.SelectedRunes != 5 {
		t.Fatalf("selected Word range = %+v", selection)
	}
	if result.Revision != doc.Revision {
		t.Fatalf("selection changed document revision to %d, want %d", result.Revision, doc.Revision)
	}
}
