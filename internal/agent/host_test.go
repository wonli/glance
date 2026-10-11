package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"glance/internal/agent/provider"
	"glance/internal/application"
	"glance/internal/document"
)

func TestAgentHostUsesInProcessMCPWithViewDefaults(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	documentInfo, err := runtime.CreateDocument(ctx, application.FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(request provider.ModelRequest) []provider.ModelEvent {
			if request.Model != "test-model" || len(request.Tools) != len(runtime.Catalog().Descriptors()) {
				t.Errorf("model request did not receive configured model/catalog: model=%q tools=%d", request.Model, len(request.Tools))
			}
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "write-cell", Type: "function", Name: "spreadsheet.cell.set",
				ArgumentsDelta: `{"sheet":"Sheet1","cell":"A1","value":"Agent 写入"}`,
			}}}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) < 4 || request.Messages[len(request.Messages)-1].Role != "tool" {
				t.Errorf("tool result was not sent back to model: %+v", request.Messages)
			} else if !strings.Contains(request.Messages[len(request.Messages)-1].Content, documentInfo.DocumentID) {
				t.Errorf("tool result omitted created document ID: %s", request.Messages[len(request.Messages)-1].Content)
			}
			return []provider.ModelEvent{{TextDelta: "已写入单元格 A1。"}, {Done: true}}
		},
	}}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)

	var events []AgentEvent
	host.Subscribe(ctx, "conversation-1", func(event AgentEvent) { events = append(events, event) })
	applicationEvents := make(chan application.Event, 1)
	stopApplicationEvents := runtime.On(ctx, application.EventFilter{Types: []string{application.EventDocumentChanged}}, func(event application.Event) {
		applicationEvents <- event
	})
	defer stopApplicationEvents()
	view := application.ViewContext{
		WindowID: 4, DocumentID: documentInfo.DocumentID, Revision: documentInfo.Revision,
		Selection: &application.SelectionContext{TargetType: "cell", Sheet: "Sheet1", Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}},
	}
	if err := host.Send(ctx, "conversation-1", "在 A1 写入 Agent 写入", view); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case event := <-applicationEvents:
		if event.Source != application.OperatorBuiltInAgent || event.DocumentID != documentInfo.DocumentID {
			t.Fatalf("MCP command event source/document=%q/%q", event.Source, event.DocumentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-process MCP command did not publish an application event")
	}

	query, err := runtime.Queries().InspectDocument(ctx, application.InspectRequest{
		DocumentID: documentInfo.DocumentID, Sheet: "Sheet1", Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1},
	})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	if !strings.Contains(string(query.Content), "Agent 写入") {
		t.Fatalf("Agent MCP command did not update the live document: %s", query.Content)
	}
	updated, err := runtime.GetDocument(ctx, documentInfo.DocumentID)
	if err != nil || updated.Revision != documentInfo.Revision+1 {
		t.Fatalf("updated document revision=%d error=%v; want %d", updated.Revision, err, documentInfo.Revision+1)
	}

	snapshot, ok := host.Snapshot("conversation-1")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 4 || snapshot.Messages[2].Role != "tool" || snapshot.Messages[3].Content != "已写入单元格 A1。" {
		t.Fatalf("unexpected conversation snapshot: %+v", snapshot)
	}
	seen := map[EventType]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, eventType := range []EventType{EventAssistantDelta, EventToolStarted, EventToolCompleted, EventTurnCompleted} {
		if !seen[eventType] {
			t.Errorf("Agent event stream omitted %q: %+v", eventType, events)
		}
	}
}

func TestAgentHostRejectsWriteWhenCapturedViewRevisionBecomesStale(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	doc, err := runtime.CreateDocument(ctx, application.FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(_ provider.ModelRequest) []provider.ModelEvent {
			// Simulate a human edit after the desktop captured ViewContext but
			// before the model's tool call reaches the live document session.
			_, editErr := runtime.Commands().Execute(ctx, application.CommandRequest{
				RequestID: "human-edit-during-agent-turn", DocumentID: doc.DocumentID,
				CommandID: "spreadsheet.cell.set", ExpectedRevision: doc.Revision,
				Operator:  application.Operator{ID: "human", Kind: application.OperatorHuman},
				Arguments: json.RawMessage(`{"sheet":"Sheet1","cell":"B1","value":"人工修改"}`),
			})
			if editErr != nil {
				t.Errorf("human edit during model request: %v", editErr)
			}
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "stale-write", Type: "function", Name: "spreadsheet.cell.set",
				ArgumentsDelta: `{"sheet":"Sheet1","cell":"A1","value":"过期写入"}`,
			}}}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) == 0 || request.Messages[len(request.Messages)-1].Role != "tool" || !strings.Contains(request.Messages[len(request.Messages)-1].Content, `"revision_conflict"`) {
				t.Errorf("model did not receive the stale write conflict: %+v", request.Messages)
			}
			return []provider.ModelEvent{{TextDelta: "检测到文档已被修改，我没有覆盖新内容。"}, {Done: true}}
		},
	}}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "stale-view-test"})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)

	view := application.ViewContext{WindowID: 4, DocumentID: doc.DocumentID, Revision: doc.Revision,
		Selection: &application.SelectionContext{TargetType: "spreadsheet.cell", TargetID: "A1", Sheet: "Sheet1"}}
	if err := host.Send(ctx, "stale-view", "在选中的单元格写入", view); err != nil {
		t.Fatalf("Send: %v", err)
	}

	query, err := runtime.Queries().InspectDocument(ctx, application.InspectRequest{
		DocumentID: doc.DocumentID, Sheet: "Sheet1",
		Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2},
	})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	var sheet struct {
		Rows [][]struct {
			Value string `json:"value"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(query.Content, &sheet); err != nil {
		t.Fatalf("decode inspected cells: %v", err)
	}
	if len(sheet.Rows) != 1 || len(sheet.Rows[0]) != 2 || sheet.Rows[0][0].Value != "" || sheet.Rows[0][1].Value != "人工修改" {
		t.Fatalf("stale Agent write changed live cells: %s", query.Content)
	}
}

func TestAgentHostCancelStopsProviderRequest(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	started := make(chan struct{})
	host, err := NewHost(ctx, blockingProvider{started: started}, client.Session, HostOptions{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)

	result := make(chan error, 1)
	go func() { result <- host.Send(ctx, "cancel-me", "hello", application.ViewContext{}) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider did not start")
	}
	if err := host.Cancel("cancel-me"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Send error=%v; want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled Agent turn did not stop")
	}
	snapshot, ok := host.Snapshot("cancel-me")
	if !ok || snapshot.Busy {
		t.Fatalf("conversation remained busy after cancellation: %+v", snapshot)
	}
}

func TestAgentHostEmptyModelResponseReleasesConversationForRetry(t *testing.T) {
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent { return nil },
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{TextDelta: "重试成功。"}}
		},
	}}
	host := newTestAgentHost(t, runtime, model, HostOptions{})
	if err := host.Send(context.Background(), "empty-reply", "第一次请求", application.ViewContext{}); !errors.Is(err, ErrEmptyModelResponse) {
		t.Fatalf("first Send error=%v; want ErrEmptyModelResponse", err)
	}
	snapshot, ok := host.Snapshot("empty-reply")
	if !ok || snapshot.Busy {
		t.Fatalf("conversation remained busy after empty model response: %+v", snapshot)
	}
	if err := host.Send(context.Background(), "empty-reply", "请重试", application.ViewContext{}); err != nil {
		t.Fatalf("Send after empty model response: %v", err)
	}
	snapshot, ok = host.Snapshot("empty-reply")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 3 || snapshot.Messages[2].Content != "重试成功。" {
		t.Fatalf("conversation did not recover after empty model response: %+v", snapshot)
	}
}

func TestAgentHostReturnsInvalidToolArgumentsToModel(t *testing.T) {
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "invalid-arguments", Type: "function", Name: "document.list", ArgumentsDelta: `[]`,
			}}}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) < 4 || request.Messages[len(request.Messages)-1].Role != "tool" ||
				!strings.Contains(request.Messages[len(request.Messages)-1].Content, `"error":"invalid_arguments"`) {
				t.Errorf("model did not receive the structured invalid-arguments result: %+v", request.Messages)
			}
			return []provider.ModelEvent{{TextDelta: "工具参数格式不正确，我已修正说明。"}}
		},
	}}
	host := newTestAgentHost(t, runtime, model, HostOptions{})
	var events []AgentEvent
	host.Subscribe(context.Background(), "invalid-arguments", func(event AgentEvent) { events = append(events, event) })
	if err := host.Send(context.Background(), "invalid-arguments", "查询文档", application.ViewContext{}); err != nil {
		t.Fatalf("Send after invalid tool arguments: %v", err)
	}
	snapshot, ok := host.Snapshot("invalid-arguments")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 4 || snapshot.Messages[2].Role != "tool" ||
		!strings.Contains(snapshot.Messages[2].Content, `"error":"invalid_arguments"`) {
		t.Fatalf("invalid arguments were not preserved as a tool result: %+v", snapshot)
	}
	for _, event := range events {
		if event.Type == EventToolCompleted && event.ToolCallID == "invalid-arguments" && event.IsError {
			return
		}
	}
	t.Fatalf("invalid tool arguments were not reported as an error event: %+v", events)
}

func TestAgentHostRejectsExcessiveToolCallsAndReleasesConversation(t *testing.T) {
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{
				{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "list-1", Type: "function", Name: "document.list", ArgumentsDelta: `{}`}},
				{ToolCallDelta: &provider.ToolCallDelta{Index: 1, ID: "list-2", Type: "function", Name: "document.list", ArgumentsDelta: `{}`}},
			}
		},
	}}
	host := newTestAgentHost(t, runtime, model, HostOptions{MaxToolCalls: 1})
	if err := host.Send(context.Background(), "too-many-tools", "查询", application.ViewContext{}); !errors.Is(err, ErrToolCallLimit) {
		t.Fatalf("Send error=%v; want ErrToolCallLimit", err)
	}
	snapshot, ok := host.Snapshot("too-many-tools")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 1 {
		t.Fatalf("conversation was left busy or tool calls were recorded after limit rejection: %+v", snapshot)
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	if calls != 1 {
		t.Fatalf("provider received %d calls; want the limit error to end the turn", calls)
	}
}

func TestAgentHostReturnsToolResultSizeErrorWithinConfiguredLimit(t *testing.T) {
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	schema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	if err := runtime.Catalog().Register(application.CapabilityRegistration{
		Descriptor: application.CapabilityDescriptor{
			ID: "agent.large.result", Name: "大型测试结果", Description: "返回较大的结构化测试结果。",
			Category: application.CapabilityQuery, Risk: application.RiskLow,
			InputSchema: schema, OutputSchema: json.RawMessage(`{"type":"object"}`),
		},
		Handler: func(context.Context, application.CapabilityContext, json.RawMessage) (application.CapabilityResult, error) {
			data, err := json.Marshal(map[string]string{"payload": strings.Repeat("x", 512)})
			return application.CapabilityResult{Data: data}, err
		},
	}); err != nil {
		t.Fatalf("register large-result capability: %v", err)
	}
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "large-result", Type: "function", Name: "agent.large.result", ArgumentsDelta: `{}`,
			}}}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) < 4 || request.Messages[len(request.Messages)-1].Role != "tool" {
				t.Errorf("model did not receive the large-result error: %+v", request.Messages)
			} else {
				content := request.Messages[len(request.Messages)-1].Content
				if len(content) > 256 || !strings.Contains(content, `"error":"tool_result_too_large"`) {
					t.Errorf("tool-result replacement length/content = %d/%s", len(content), content)
				}
			}
			return []provider.ModelEvent{{TextDelta: "结果太大，我已收到上限提示。"}}
		},
	}}
	host := newTestAgentHost(t, runtime, model, HostOptions{MaxToolResultBytes: 256})
	if err := host.Send(context.Background(), "large-result", "执行大型查询", application.ViewContext{}); err != nil {
		t.Fatalf("Send after large tool result: %v", err)
	}
	snapshot, ok := host.Snapshot("large-result")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 4 || len(snapshot.Messages[2].Content) > 256 ||
		!strings.Contains(snapshot.Messages[2].Content, `"error":"tool_result_too_large"`) {
		t.Fatalf("oversized tool result was not replaced by a bounded error: %+v", snapshot)
	}
}

func TestAgentHostRejectsOversizedToolArguments(t *testing.T) {
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "oversized-arguments", Type: "function", Name: "document.list", ArgumentsDelta: `{"padding":"too-large"}`,
			}}}
		},
	}}
	host := newTestAgentHost(t, runtime, model, HostOptions{MaxToolArgumentsBytes: 8})
	if err := host.Send(context.Background(), "oversized-arguments", "查询", application.ViewContext{}); !errors.Is(err, ErrToolArgumentsTooLarge) {
		t.Fatalf("Send error=%v; want ErrToolArgumentsTooLarge", err)
	}
	snapshot, ok := host.Snapshot("oversized-arguments")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 1 {
		t.Fatalf("conversation remained busy or stored oversized tool arguments: %+v", snapshot)
	}
}

func TestAgentHostCloseWaitsForActiveModelTurn(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	model := &closeBlockingProvider{
		started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{}),
	}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	var releaseOnce sync.Once
	releaseModel := func() { releaseOnce.Do(func() { close(model.release) }) }
	t.Cleanup(func() {
		releaseModel()
		host.Close()
	})
	host.Subscribe(ctx, "close-wait", func(AgentEvent) {})
	host.mu.Lock()
	var subscriptionDone <-chan struct{}
	for _, subscriber := range host.subscribers {
		subscriptionDone = subscriber.done
	}
	host.mu.Unlock()
	if subscriptionDone == nil {
		t.Fatal("Agent subscription did not register a watcher completion signal")
	}
	sendDone := make(chan error, 1)
	go func() { sendDone <- host.Send(ctx, "close-wait", "test", application.ViewContext{}) }()
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("model stream did not start")
	}

	closeDone := make(chan struct{})
	go func() {
		host.Close()
		close(closeDone)
	}()
	select {
	case <-model.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("closing Agent Host did not cancel the model request")
	}
	select {
	case <-closeDone:
		t.Fatal("Agent Host Close returned while its model turn was still running")
	default:
	}

	releaseModel()
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Agent Host Close did not finish after the model turn exited")
	}
	select {
	case <-subscriptionDone:
	default:
		t.Fatal("Agent Host Close returned before its subscription watcher exited")
	}
	select {
	case <-sendDone:
	case <-time.After(2 * time.Second):
		t.Fatal("model turn goroutine remained active after Agent Host Close")
	}
}

type closeBlockingProvider struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (p *closeBlockingProvider) Stream(ctx context.Context, _ provider.ModelRequest, _ func(provider.ModelEvent) error) error {
	close(p.started)
	<-ctx.Done()
	close(p.canceled)
	<-p.release
	return ctx.Err()
}

func TestAgentHostExecutesMultipleMCPToolsAndRefreshesRevision(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	documentInfo, err := runtime.CreateDocument(ctx, application.FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(request provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{
				{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "write-a1", Type: "function", Name: "spreadsheet.cell.set", ArgumentsDelta: `{"sheet":"Sheet1","cell":"A1","value":"一"}`}},
				{ToolCallDelta: &provider.ToolCallDelta{Index: 1, ID: "write-b1", Type: "function", Name: "spreadsheet.cell.set", ArgumentsDelta: `{"sheet":"Sheet1","cell":"B1","value":"二"}`}},
			}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) < 5 || request.Messages[len(request.Messages)-1].Role != "tool" {
				t.Errorf("model did not receive both tool results: %+v", request.Messages)
			}
			return []provider.ModelEvent{{TextDelta: "两格已填写。"}, {Done: true}}
		},
	}}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)
	if err := host.Send(ctx, "multi-tool", "填充 A1 和 B1", application.ViewContext{
		DocumentID: documentInfo.DocumentID, Revision: documentInfo.Revision,
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	updated, err := runtime.GetDocument(ctx, documentInfo.DocumentID)
	if err != nil || updated.Revision != documentInfo.Revision+2 {
		t.Fatalf("document revision=%d error=%v; want %d", updated.Revision, err, documentInfo.Revision+2)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, application.InspectRequest{
		DocumentID: documentInfo.DocumentID, Sheet: "Sheet1",
		Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2},
	})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	if !strings.Contains(string(snapshot.Content), `"一"`) || !strings.Contains(string(snapshot.Content), `"二"`) {
		t.Fatalf("multiple MCP tool calls were not applied: %s", snapshot.Content)
	}
}

func TestAgentHostCancellationCompletesPendingToolResultsAndAllowsNextTurn(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	started := make(chan struct{})
	schema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	for _, capability := range []struct {
		id      string
		handler application.CapabilityHandler
	}{
		{
			id: "agent.cancel.wait",
			handler: func(ctx context.Context, _ application.CapabilityContext, _ json.RawMessage) (application.CapabilityResult, error) {
				close(started)
				<-ctx.Done()
				return application.CapabilityResult{}, ctx.Err()
			},
		},
		{
			id: "agent.cancel.skip",
			handler: func(context.Context, application.CapabilityContext, json.RawMessage) (application.CapabilityResult, error) {
				return application.CapabilityResult{Data: json.RawMessage(`{"completed":true}`)}, nil
			},
		},
	} {
		if err := runtime.Catalog().Register(application.CapabilityRegistration{
			Descriptor: application.CapabilityDescriptor{
				ID: capability.id, Name: capability.id, Description: "Agent cancellation recovery fixture",
				Category: application.CapabilityQuery, Risk: application.RiskLow,
				InputSchema: schema, OutputSchema: schema,
			},
			Handler: capability.handler,
		}); err != nil {
			t.Fatalf("register %s: %v", capability.id, err)
		}
	}
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{
				{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "wait-call", Type: "function", Name: "agent.cancel.wait", ArgumentsDelta: `{}`}},
				{ToolCallDelta: &provider.ToolCallDelta{Index: 1, ID: "skip-call", Type: "function", Name: "agent.cancel.skip", ArgumentsDelta: `{}`}},
			}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			messages := request.Messages
			if len(messages) != 6 || messages[2].Role != "assistant" || len(messages[2].ToolCalls) != 2 ||
				messages[3].Role != "tool" || messages[3].ToolCallID != "wait-call" ||
				messages[4].Role != "tool" || messages[4].ToolCallID != "skip-call" ||
				messages[5].Role != "user" {
				t.Errorf("next turn received incomplete tool history: %+v", messages)
			}
			return []provider.ModelEvent{{TextDelta: "可以继续。"}, {Done: true}}
		},
	}}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)
	var events []AgentEvent
	host.Subscribe(ctx, "cancel-recovery", func(event AgentEvent) { events = append(events, event) })
	turnResult := make(chan error, 1)
	go func() {
		turnResult <- host.Send(ctx, "cancel-recovery", "先调用两个工具", application.ViewContext{})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first MCP tool did not start")
	}
	if err := host.Cancel("cancel-recovery"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case err := <-turnResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Send error=%v; want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled Agent turn did not stop")
	}
	snapshot, ok := host.Snapshot("cancel-recovery")
	if !ok || len(snapshot.Messages) != 4 || len(snapshot.Messages[1].ToolCalls) != 2 ||
		snapshot.Messages[2].ToolCallID != "wait-call" || snapshot.Messages[3].ToolCallID != "skip-call" {
		t.Fatalf("canceled turn left an incomplete tool history: %+v", snapshot)
	}
	var skippedToolReported bool
	for _, event := range events {
		if event.Type == EventToolCompleted && event.ToolCallID == "skip-call" && event.IsError {
			skippedToolReported = true
			break
		}
	}
	if !skippedToolReported {
		t.Fatalf("canceled pending tool call was not reported in the Agent event stream: %+v", events)
	}
	if err := host.Send(ctx, "cancel-recovery", "继续", application.ViewContext{}); err != nil {
		t.Fatalf("next turn after cancellation: %v", err)
	}
}

func TestAgentHostContinuesAfterMCPToolError(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	schema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	if err := runtime.Catalog().Register(application.CapabilityRegistration{
		Descriptor: application.CapabilityDescriptor{
			ID: "agent.error.fixture", Name: "失败工具", Description: "返回一个可恢复的错误供 Agent 判断。",
			Category: application.CapabilityQuery, Risk: application.RiskLow,
			InputSchema: schema, OutputSchema: schema,
		},
		Handler: func(context.Context, application.CapabilityContext, json.RawMessage) (application.CapabilityResult, error) {
			return application.CapabilityResult{}, errors.New("fixture capability failure")
		},
	}); err != nil {
		t.Fatalf("register failing capability: %v", err)
	}
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{ToolCallDelta: &provider.ToolCallDelta{
				Index: 0, ID: "failing-call", Type: "function", Name: "agent.error.fixture", ArgumentsDelta: `{}`,
			}}}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			messages := request.Messages
			if len(messages) < 4 || messages[len(messages)-1].Role != "tool" ||
				!strings.Contains(messages[len(messages)-1].Content, "fixture capability failure") {
				t.Errorf("model did not receive the MCP tool error: %+v", messages)
			}
			return []provider.ModelEvent{{TextDelta: "工具失败了，我会继续说明。"}, {Done: true}}
		},
	}}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)
	var events []AgentEvent
	host.Subscribe(ctx, "mcp-error-recovery", func(event AgentEvent) { events = append(events, event) })
	if err := host.Send(ctx, "mcp-error-recovery", "调用一个可能失败的工具", application.ViewContext{}); err != nil {
		t.Fatalf("Send after recoverable MCP tool error: %v", err)
	}
	snapshot, ok := host.Snapshot("mcp-error-recovery")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 4 || snapshot.Messages[2].Role != "tool" ||
		!strings.Contains(snapshot.Messages[2].Content, "fixture capability failure") ||
		snapshot.Messages[3].Content != "工具失败了，我会继续说明。" {
		t.Fatalf("MCP tool error did not leave a recoverable conversation: %+v", snapshot)
	}
	for _, event := range events {
		if event.Type == EventToolCompleted && event.ToolCallID == "failing-call" && event.IsError {
			return
		}
	}
	t.Fatalf("MCP tool error was not reported to the UI: %+v", events)
}

func TestAgentHostRecoversAfterModelStreamError(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	providerFailure := errors.New("fixture model stream failure")
	model := &failOnceProvider{firstError: providerFailure}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)
	var events []AgentEvent
	host.Subscribe(ctx, "model-error-recovery", func(event AgentEvent) { events = append(events, event) })
	if err := host.Send(ctx, "model-error-recovery", "第一次请求会失败", application.ViewContext{}); !errors.Is(err, providerFailure) {
		t.Fatalf("first Send error=%v; want provider failure", err)
	}
	snapshot, ok := host.Snapshot("model-error-recovery")
	if !ok || snapshot.Busy {
		t.Fatalf("conversation remained busy after model failure: %+v", snapshot)
	}
	if err := host.Send(ctx, "model-error-recovery", "请再试一次", application.ViewContext{}); err != nil {
		t.Fatalf("second Send after model error: %v", err)
	}
	snapshot, ok = host.Snapshot("model-error-recovery")
	if !ok || snapshot.Busy || len(snapshot.Messages) != 3 || snapshot.Messages[2].Content != "模型恢复后正常回复。" {
		t.Fatalf("conversation did not recover after model error: %+v", snapshot)
	}
	var turnFailed, turnCompleted bool
	for _, event := range events {
		turnFailed = turnFailed || event.Type == EventTurnFailed
		turnCompleted = turnCompleted || event.Type == EventTurnCompleted
	}
	if !turnFailed || !turnCompleted {
		t.Fatalf("model error/recovery events were incomplete: %+v", events)
	}
}

type scriptedProvider struct {
	t     *testing.T
	mu    sync.Mutex
	steps []func(provider.ModelRequest) []provider.ModelEvent
	calls int
}

func newTestAgentHost(t *testing.T, runtime *application.Runtime, model provider.ModelProvider, options HostOptions) *AgentHost {
	t.Helper()
	client, err := ConnectMCP(context.Background(), runtime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	host, err := NewHost(context.Background(), model, client.Session, options)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	t.Cleanup(host.Close)
	return host
}

type failOnceProvider struct {
	mu         sync.Mutex
	firstError error
	calls      int
}

func (p *failOnceProvider) Stream(_ context.Context, _ provider.ModelRequest, emit func(provider.ModelEvent) error) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		return p.firstError
	}
	if call > 2 {
		return errors.New("unexpected model request")
	}
	return emit(provider.ModelEvent{TextDelta: "模型恢复后正常回复。"})
}

func (p *scriptedProvider) Stream(_ context.Context, request provider.ModelRequest, emit func(provider.ModelEvent) error) error {
	p.mu.Lock()
	if p.calls >= len(p.steps) {
		p.mu.Unlock()
		p.t.Errorf("unexpected model request %d", p.calls+1)
		return errors.New("unexpected model request")
	}
	step := p.steps[p.calls]
	p.calls++
	p.mu.Unlock()
	for _, event := range step(request) {
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}

type blockingProvider struct {
	started chan struct{}
}

func (p blockingProvider) Stream(ctx context.Context, _ provider.ModelRequest, _ func(provider.ModelEvent) error) error {
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}
