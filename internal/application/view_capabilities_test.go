package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type recordingViewIntentHandler struct {
	intents  []ViewIntent
	response json.RawMessage
}

func (h *recordingViewIntentHandler) HandleViewIntent(_ context.Context, intent ViewIntent) (json.RawMessage, error) {
	h.intents = append(h.intents, intent)
	return append(json.RawMessage(nil), h.response...), nil
}

func TestWindowCapabilitiesRouteValidRequestsAndRejectInvalidContext(t *testing.T) {
	tests := []struct {
		name       string
		format     Format
		capability string
		arguments  json.RawMessage
		response   json.RawMessage
	}{
		{
			name: "json tree view", format: FormatJSON, capability: "json.view.set",
			arguments: json.RawMessage(`{"operation":"expand_level","level":2}`),
			response:  json.RawMessage(`{"window_id":17,"operation":"expand_level","level":2,"changed":true}`),
		},
		{
			name: "markdown mode", format: FormatMarkdown, capability: "markdown.view.set",
			arguments: json.RawMessage(`{"mode":"preview"}`),
			response:  json.RawMessage(`{"window_id":17,"mode":"preview","changed":true}`),
		},
		{
			name: "pdf view", format: FormatPDF, capability: "pdf.view.set",
			arguments: json.RawMessage(`{"operation":"set_zoom","zoom_percent":125}`),
			response:  json.RawMessage(`{"window_id":17,"page_number":1,"page_count":1,"zoom_percent":125,"fit_mode":"custom","mode":"read","tool":"","thumbnails":false,"search_visible":false,"changed":true}`),
		},
		{
			name: "word zoom dialog", format: FormatDocx, capability: "word.zoom.dialog",
			arguments: json.RawMessage(`{}`), response: json.RawMessage(`{"window_id":17,"opened":true}`),
		},
		{
			name: "word new window", format: FormatDocx, capability: "word.view.new_window",
			arguments: json.RawMessage(`{}`), response: json.RawMessage(`{"source_window_id":17,"document_id":"doc-1","requested":true}`),
		},
		{
			name: "word side by side", format: FormatDocx, capability: "word.view.side_by_side",
			arguments: json.RawMessage(`{}`), response: json.RawMessage(`{"window_id":17,"strategy":"two_page"}`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			runtime, err := NewRuntime(ctx, RuntimeOptions{})
			if err != nil {
				t.Fatalf("NewRuntime: %v", err)
			}
			t.Cleanup(func() { _ = runtime.Close(context.Background()) })
			doc, err := runtime.CreateDocument(ctx, test.format)
			if err != nil {
				t.Fatalf("CreateDocument(%s): %v", test.format, err)
			}

			handler := &recordingViewIntentHandler{response: test.response}
			unregister := runtime.RegisterViewIntentHandler(17, handler)
			t.Cleanup(unregister)
			request := CapabilityRequest{
				RequestID: "valid-view-request", DocumentID: doc.DocumentID,
				CapabilityID: test.capability, ExpectedRevision: doc.Revision,
				Operator:  Operator{ID: "agent-1", Kind: OperatorExternalAgent},
				View:      ViewContext{WindowID: 17, DocumentID: doc.DocumentID, Revision: doc.Revision},
				Arguments: test.arguments,
			}
			result, err := runtime.InvokeCapability(ctx, request)
			if err != nil {
				t.Fatalf("InvokeCapability(%s): %v", test.capability, err)
			}
			if len(handler.intents) != 1 {
				t.Fatalf("view handler received %d requests; want 1", len(handler.intents))
			}
			intent := handler.intents[0]
			if intent.Action != test.capability || intent.WindowID != 17 || intent.DocumentID != doc.DocumentID || intent.Revision != doc.Revision || string(intent.Arguments) != string(test.arguments) {
				t.Fatalf("routed intent = %+v", intent)
			}
			if string(result.Data) != string(test.response) || result.Revision != doc.Revision {
				t.Fatalf("result data=%s revision=%d; want %s and unchanged revision %d", result.Data, result.Revision, test.response, doc.Revision)
			}

			request.RequestID = "invalid-view-arguments"
			request.Arguments = json.RawMessage(`{"unexpected":true}`)
			if _, err := runtime.InvokeCapability(ctx, request); !errors.Is(err, ErrInvalidCommandArguments) {
				t.Fatalf("unexpected view argument error = %v; want ErrInvalidCommandArguments", err)
			}
			if len(handler.intents) != 1 {
				t.Fatalf("invalid arguments reached view handler: %+v", handler.intents)
			}

			request.RequestID = "unattached-view"
			request.Arguments = test.arguments
			request.View.WindowID = 18
			if _, err := runtime.InvokeCapability(ctx, request); !errors.Is(err, ErrNoAttachedView) {
				t.Fatalf("unattached view error = %v; want ErrNoAttachedView", err)
			}
			if len(handler.intents) != 1 {
				t.Fatalf("unattached request reached view handler: %+v", handler.intents)
			}
		})
	}
}
