package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRuntimeOnReceivesMessagesFromAnySourceWithoutForgingDomainEvents(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	seen := make([]Event, 0, 2)
	cancel := runtime.On(ctx, EventFilter{}, func(event Event) { seen = append(seen, event) })
	defer cancel()
	if err := runtime.Emit(ctx, IncomingMessage{
		Source: OperatorHuman, Name: "ribbon.click", Data: json.RawMessage(`{"action":"save"}`),
	}); err != nil {
		t.Fatalf("Emit human message: %v", err)
	}
	if err := runtime.Emit(ctx, IncomingMessage{
		Source: OperatorExternalAgent, Name: "mcp.request", Data: json.RawMessage(`{"type":"document.changed"}`),
	}); err != nil {
		t.Fatalf("Emit MCP message: %v", err)
	}
	if len(seen) != 2 || seen[0].Type != EventMessageReceived || seen[0].Source != OperatorHuman || seen[1].Type != EventMessageReceived || seen[1].Source != OperatorExternalAgent {
		t.Fatalf("On did not receive both origins as input messages: %+v", seen)
	}
	for _, event := range seen {
		if event.Revision != 0 || event.CommandID != "" {
			t.Fatalf("incoming message forged command result metadata: %+v", event)
		}
	}
}

func TestRuntimeOnFiltersAndStopsAfterCancellation(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var seen []Event
	filterCancel := runtime.On(ctx, EventFilter{Types: []string{EventDocumentChanged}}, func(event Event) { seen = append(seen, event) })
	if err := runtime.Emit(ctx, IncomingMessage{Source: OperatorHuman, Name: "ribbon.click", Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	filterCancel()
	if len(seen) != 0 {
		t.Fatalf("filtered subscriber received an input event: %+v", seen)
	}
}

func TestEventBusCloseWaitsForSubscriptionWatchers(t *testing.T) {
	bus := newEventBus()
	t.Cleanup(bus.close)
	stop := bus.subscribe(context.Background(), EventFilter{}, func(Event) {})
	bus.mu.Lock()
	var done <-chan struct{}
	for _, subscriber := range bus.subscribers {
		done = subscriber.done
	}
	bus.mu.Unlock()
	if done == nil {
		t.Fatal("subscription did not register a watcher completion signal")
	}

	bus.close()
	select {
	case <-done:
	default:
		t.Fatal("EventBus close returned before its subscription watcher exited")
	}
	stop()
}

func TestRuntimePublishesDocumentLifecycleEvents(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var seen []Event
	cancel := runtime.On(ctx, EventFilter{DocumentID: ""}, func(event Event) { seen = append(seen, event) })
	defer cancel()
	doc, err := runtime.CreateDocument(ctx, FormatJSON)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, filepath.Join(t.TempDir(), "saved.json")); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	want := []string{EventDocumentOpened, EventDocumentSaved, EventDocumentClosed}
	if len(seen) != len(want) {
		t.Fatalf("received %d lifecycle events, want %d: %+v", len(seen), len(want), seen)
	}
	for index, event := range seen {
		if event.Type != want[index] || event.DocumentID != doc.DocumentID || event.Source != OperatorSystem || event.Revision == 0 {
			t.Fatalf("lifecycle event %d = %+v, want type %s and document metadata", index, event, want[index])
		}
	}
}
