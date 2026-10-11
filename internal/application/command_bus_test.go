package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type commandTestAdapter struct{ *testDocumentAdapter }

func (a *commandTestAdapter) markDirty() {
	a.mu.Lock()
	a.dirty = true
	a.mu.Unlock()
}

func TestCommandBusChecksRevisionSchemaAndReplaysIdempotentResult(t *testing.T) {
	ctx := context.Background()
	adapter := &commandTestAdapter{testDocumentAdapter: &testDocumentAdapter{format: FormatJSON}}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "command.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	var calls atomic.Int32
	if err := runtime.Catalog().Register(testCommandRegistration(func(_ context.Context, execution CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
		calls.Add(1)
		execution.Adapter.(*commandTestAdapter).markDirty()
		return CapabilityResult{Data: json.RawMessage(`{"changed":true}`), Changed: true}, nil
	})); err != nil {
		t.Fatalf("register command: %v", err)
	}
	request := CommandRequest{
		RequestID: "req-1", IdempotencyKey: "idem-1", DocumentID: doc.DocumentID,
		CommandID: "test.change", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "person-7", Kind: OperatorHuman},
		Arguments: json.RawMessage(`{"value":"new"}`),
	}
	first, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	request.RequestID = "req-retry"
	second, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if calls.Load() != 1 || first.Revision != doc.Revision+1 || second.Revision != first.Revision || string(second.Data) != `{"changed":true}` {
		t.Fatalf("retry re-executed or returned a different result: calls=%d first=%+v second=%+v", calls.Load(), first, second)
	}
	if current, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || !current.Dirty || current.Revision != first.Revision {
		t.Fatalf("successful command did not update live session: %+v err=%v", current, err)
	}

	request.Arguments = json.RawMessage(`{"value":"different"}`)
	if _, err := runtime.Commands().Execute(ctx, request); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency payload conflict = %v, want ErrIdempotencyConflict", err)
	}
	request.IdempotencyKey = "idem-stale"
	request.ExpectedRevision = doc.Revision
	if _, err := runtime.Commands().Execute(ctx, request); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v, want ErrRevisionConflict", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler ran after idempotency conflict or stale revision: %d calls", calls.Load())
	}
}

func TestCommandBusRejectsInvalidArgumentsAndUnsupportedFormats(t *testing.T) {
	ctx := context.Background()
	adapter := &testDocumentAdapter{format: FormatJSON, readOnly: true}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "readonly.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	var calls atomic.Int32
	if err := runtime.Catalog().Register(testCommandRegistration(func(context.Context, CapabilityContext, json.RawMessage) (CapabilityResult, error) {
		calls.Add(1)
		return CapabilityResult{Changed: true}, nil
	})); err != nil {
		t.Fatalf("register command: %v", err)
	}
	base := CommandRequest{
		RequestID: "req", DocumentID: doc.DocumentID, CommandID: "test.change",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorExternalAgent},
		Arguments: json.RawMessage(`{"value":1}`),
	}
	if _, err := runtime.Commands().Execute(ctx, base); !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("invalid argument type error = %v, want ErrInvalidCommandArguments", err)
	}
	base.Arguments = json.RawMessage(`{"value":"ok"}`)
	if _, err := runtime.Commands().Execute(ctx, base); !errors.Is(err, ErrReadOnlyDocument) {
		t.Fatalf("read-only command error = %v, want ErrReadOnlyDocument", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("handler ran for invalid or read-only command: %d calls", calls.Load())
	}
}

func TestCommandBusRequiresRevisionAndBlocksHighRiskCommands(t *testing.T) {
	ctx := context.Background()
	adapter := &testDocumentAdapter{format: FormatJSON}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "risk.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	var calls atomic.Int32
	registration := testCommandRegistration(func(context.Context, CapabilityContext, json.RawMessage) (CapabilityResult, error) {
		calls.Add(1)
		return CapabilityResult{Data: json.RawMessage(`{}`), Changed: true}, nil
	})
	registration.Descriptor.Risk = RiskHigh
	if err := runtime.Catalog().Register(registration); err != nil {
		t.Fatalf("register high-risk command: %v", err)
	}
	request := CommandRequest{
		RequestID: "req-risk", DocumentID: doc.DocumentID, CommandID: "test.change",
		Operator: Operator{ID: "person", Kind: OperatorHuman}, Arguments: json.RawMessage(`{"value":"x"}`),
	}
	if _, err := runtime.Commands().Execute(ctx, request); !errors.Is(err, ErrExpectedRevisionRequired) {
		t.Fatalf("missing revision error = %v, want ErrExpectedRevisionRequired", err)
	}
	request.ExpectedRevision = doc.Revision
	if _, err := runtime.Commands().Execute(ctx, request); !errors.Is(err, ErrHumanConfirmationRequired) {
		t.Fatalf("high-risk command error = %v, want ErrHumanConfirmationRequired", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("unconfirmed high-risk handler ran %d times", calls.Load())
	}
}

func TestCommandBusRejectsUnknownAndUnsupportedCapabilities(t *testing.T) {
	ctx := context.Background()
	adapter := &testDocumentAdapter{format: FormatJSON}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "unsupported.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	request := CommandRequest{
		RequestID: "req-unsupported", DocumentID: doc.DocumentID, CommandID: "unknown.command",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: json.RawMessage(`{}`),
	}
	if _, err := runtime.Commands().Execute(ctx, request); !errors.Is(err, ErrCapabilityNotFound) {
		t.Fatalf("unknown capability error = %v, want ErrCapabilityNotFound", err)
	}
	registration := testCommandRegistration(func(context.Context, CapabilityContext, json.RawMessage) (CapabilityResult, error) {
		return CapabilityResult{Data: json.RawMessage(`{}`), Changed: true}, nil
	})
	registration.Descriptor.Formats = []Format{FormatXLSX}
	if err := runtime.Catalog().Register(registration); err != nil {
		t.Fatalf("register XLSX command: %v", err)
	}
	request.CommandID = registration.Descriptor.ID
	request.Arguments = json.RawMessage(`{"value":"x"}`)
	if _, err := runtime.Commands().Execute(ctx, request); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("unsupported format error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestCommandBusPublishesTypedEventOnlyAfterSuccessfulMutation(t *testing.T) {
	ctx := context.Background()
	adapter := &commandTestAdapter{testDocumentAdapter: &testDocumentAdapter{format: FormatJSON}}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{&testFormatFactory{format: FormatJSON, openDoc: adapter}}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "events.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	var events []Event
	cancel := runtime.On(ctx, EventFilter{Types: []string{EventDocumentChanged}}, func(event Event) { events = append(events, event) })
	defer cancel()
	if err := runtime.Catalog().Register(testCommandRegistration(func(_ context.Context, execution CapabilityContext, args json.RawMessage) (CapabilityResult, error) {
		var payload struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return CapabilityResult{}, err
		}
		if payload.Value == "fail" {
			return CapabilityResult{}, errors.New("edit rejected")
		}
		if payload.Value == "noop" {
			return CapabilityResult{Data: json.RawMessage(`{"changed":false}`)}, nil
		}
		execution.Adapter.(*commandTestAdapter).markDirty()
		return CapabilityResult{Data: json.RawMessage(`{"changed":true}`), Changed: true}, nil
	})); err != nil {
		t.Fatalf("register command: %v", err)
	}
	request := CommandRequest{
		RequestID: "req-change", DocumentID: doc.DocumentID, CommandID: "test.change",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-2", Kind: OperatorBuiltInAgent},
		View: ViewContext{WindowID: 7, DocumentID: doc.DocumentID, Revision: doc.Revision,
			Selection: &SelectionContext{TargetType: "cell", TargetID: "Sheet1!A1"}},
		Arguments: json.RawMessage(`{"value":"edit"}`),
	}
	if _, err := runtime.Commands().Execute(ctx, request); err != nil {
		t.Fatalf("Execute change: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("successful mutation published %d events, want one", len(events))
	}
	if event := events[0]; event.Type != EventDocumentChanged || event.Source != OperatorBuiltInAgent || event.DocumentID != doc.DocumentID || event.RequestID != request.RequestID || event.CommandID != request.CommandID || event.Revision != doc.Revision+1 || event.View.WindowID != 7 {
		t.Fatalf("event is missing operation metadata: %+v", event)
	}
	request.RequestID = "req-noop"
	request.IdempotencyKey = ""
	request.ExpectedRevision = events[0].Revision
	request.View.Revision = events[0].Revision
	request.Arguments = json.RawMessage(`{"value":"noop"}`)
	if _, err := runtime.Commands().Execute(ctx, request); err != nil {
		t.Fatalf("Execute no-op: %v", err)
	}
	request.RequestID = "req-failed"
	request.Arguments = json.RawMessage(`{"value":"fail"}`)
	if _, err := runtime.Commands().Execute(ctx, request); err == nil {
		t.Fatal("failed handler returned no error")
	}
	if len(events) != 1 {
		t.Fatalf("no-op or failed command published a mutation event: %+v", events)
	}
}

func testCommandRegistration(handler CapabilityHandler) CapabilityRegistration {
	return CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "test.change", Name: "测试修改", Description: "修改测试文档。",
			Category:     CapabilityCommand,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object"}`),
			Formats:      []Format{FormatJSON}, Risk: RiskLow,
		},
		Handler: handler,
	}
}
