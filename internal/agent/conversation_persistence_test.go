package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"glance/internal/agent/provider"
	"glance/internal/application"
)

func TestConversationTitleAndContextSurviveProviderReplacement(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := newMemoryConversationStore()
	view := application.ViewContext{DocumentID: "document-a"}

	firstProvider := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(request provider.ModelRequest) []provider.ModelEvent {
			if len(request.Messages) != 1 || !strings.Contains(request.Messages[0].Content, "对话标题") {
				t.Errorf("first provider request did not ask for a title: %+v", request.Messages)
			}
			return []provider.ModelEvent{{TextDelta: "季度营收复盘"}, {Done: true}}
		},
		func(request provider.ModelRequest) []provider.ModelEvent {
			if request.Model != "model-a" || len(request.Messages) < 2 || request.Messages[len(request.Messages)-1].Content != "查看本季度营业额" {
				t.Errorf("first reply request=%+v", request)
			}
			return []provider.ModelEvent{{ReasoningDelta: "先检查文档结构。", TextDelta: "我会检查本季度数据。"}, {Done: true}}
		},
	}}
	hostA, err := NewHost(ctx, firstProvider, client.Session, HostOptions{Model: "model-a", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	created, err := hostA.CreateConversation(ctx, view.DocumentID)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := hostA.Send(ctx, created.ID, "查看本季度营业额", view); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	snapshot, ok := hostA.Snapshot(created.ID)
	if !ok || snapshot.Title != "季度营收复盘" || len(snapshot.Messages) != 2 || len(snapshot.Reasoning) != 1 {
		t.Fatalf("persisted conversation snapshot=%+v found=%v", snapshot, ok)
	}
	hostA.Close()

	secondProvider := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(request provider.ModelRequest) []provider.ModelEvent {
			if request.Model != "model-b" {
				t.Errorf("provider replacement did not use model-b: %q", request.Model)
			}
			if request.ReasoningByMessage[2] != "先检查文档结构。" {
				t.Errorf("stored reasoning was not associated with its assistant message: %v", request.ReasoningByMessage)
			}
			for _, message := range request.Messages {
				if strings.Contains(message.Content, "先检查文档结构") {
					t.Error("reasoning transcript was sent as model context")
				}
			}
			if len(request.Messages) < 4 {
				t.Errorf("previous context was lost after provider replacement: %+v", request.Messages)
			} else if request.Messages[len(request.Messages)-3].Content != "查看本季度营业额" || request.Messages[len(request.Messages)-2].Content != "我会检查本季度数据。" || request.Messages[len(request.Messages)-1].Content != "再给我一个总结" {
				t.Errorf("restored context order=%+v", request.Messages[len(request.Messages)-3:])
			}
			return []provider.ModelEvent{{TextDelta: "本季度营业额保持增长。"}, {Done: true}}
		},
	}}
	hostB, err := NewHost(ctx, secondProvider, client.Session, HostOptions{Model: "model-b", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer hostB.Close()
	loaded, found, err := hostB.LoadConversation(ctx, view.DocumentID, created.ID)
	if err != nil || !found || loaded.Title != "季度营收复盘" || len(loaded.Messages) != 2 || len(loaded.Reasoning) != 1 {
		t.Fatalf("LoadConversation snapshot=%+v found=%v error=%v", loaded, found, err)
	}
	if err := hostB.Send(ctx, created.ID, "再给我一个总结", view); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if secondProvider.calls != 1 {
		t.Fatalf("provider received %d requests; title should not be regenerated", secondProvider.calls)
	}
	if _, _, err := hostB.LoadConversation(ctx, "document-b", created.ID); !errors.Is(err, ErrConversationScopeMismatch) {
		t.Fatalf("cross-document load error=%v", err)
	}
}

func TestConversationTitleFailureIsRetriedBeforeReply(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := newMemoryConversationStore()
	model := &scriptedProvider{
		t:          t,
		stepErrors: []error{errors.New("temporary title failure"), nil, nil},
		steps: []func(provider.ModelRequest) []provider.ModelEvent{
			func(request provider.ModelRequest) []provider.ModelEvent {
				if len(request.Tools) != 0 || !strings.Contains(request.Messages[0].Content, "第一条任务") {
					t.Errorf("title generation request=%+v", request)
				}
				return nil
			},
			func(request provider.ModelRequest) []provider.ModelEvent {
				if len(request.Tools) != 0 || !strings.Contains(request.Messages[0].Content, "第一条任务") {
					t.Errorf("retry did not use the first user message: %+v", request)
				}
				return []provider.ModelEvent{{TextDelta: "月度数据审阅"}, {Done: true}}
			},
			func(request provider.ModelRequest) []provider.ModelEvent {
				if request.Messages[len(request.Messages)-1].Content != "第一条任务" {
					t.Errorf("normal reply started before successful title: %+v", request.Messages)
				}
				return []provider.ModelEvent{{TextDelta: "第二项检查完成。"}, {Done: true}}
			},
		},
	}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "retry-title", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	conversation, err := host.CreateConversation(ctx, "doc-title")
	if err != nil {
		t.Fatal(err)
	}
	view := application.ViewContext{DocumentID: "doc-title"}
	if err := host.Send(ctx, conversation.ID, "第一条任务", view); err == nil {
		t.Fatal("first title request unexpectedly succeeded")
	}
	if model.calls != 1 {
		t.Fatalf("normal reply request ran after title failure; calls=%d", model.calls)
	}
	if err := host.ResumeConversation(ctx, conversation.ID, view); err != nil {
		t.Fatalf("ResumeConversation: %v", err)
	}
	snapshot, found := host.Snapshot(conversation.ID)
	if !found || snapshot.Title != "月度数据审阅" || len(snapshot.Messages) != 2 || snapshot.Messages[0].Content != "第一条任务" || model.calls != 3 {
		t.Fatalf("title retry snapshot=%+v calls=%d", snapshot, model.calls)
	}
}

func TestConversationTitleRejectsOverlongModelOutputWithoutTruncating(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := newMemoryConversationStore()
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{TextDelta: strings.Repeat("标", 33)}, {Done: true}}
		},
		func(provider.ModelRequest) []provider.ModelEvent {
			return []provider.ModelEvent{{TextDelta: "不应开始普通回复"}, {Done: true}}
		},
	}}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "title-validation", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	conversation, err := host.CreateConversation(ctx, "doc-title-validation")
	if err != nil {
		t.Fatal(err)
	}
	err = host.Send(ctx, conversation.ID, "检查文档", application.ViewContext{DocumentID: "doc-title-validation"})
	if err == nil {
		t.Fatal("overlong generated title was accepted")
	}
	snapshot, found := host.Snapshot(conversation.ID)
	if !found || snapshot.Title != "" || len(snapshot.Messages) != 1 || model.calls != 1 {
		t.Fatalf("overlong title was truncated or normal reply started: snapshot=%+v calls=%d", snapshot, model.calls)
	}
}

func TestResumeConversationRetriesInterruptedReplyWithoutAppendingAnotherUser(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := newMemoryConversationStore()
	model := &scriptedProvider{
		t:          t,
		stepErrors: []error{nil, errors.New("reply stream interrupted"), nil},
		steps: []func(provider.ModelRequest) []provider.ModelEvent{
			func(provider.ModelRequest) []provider.ModelEvent {
				return []provider.ModelEvent{{TextDelta: "文档范围确认"}, {Done: true}}
			},
			func(request provider.ModelRequest) []provider.ModelEvent {
				if request.Messages[len(request.Messages)-1].Content != "检查第二季度销售额" {
					t.Errorf("first reply request=%+v", request.Messages)
				}
				return nil
			},
			func(request provider.ModelRequest) []provider.ModelEvent {
				if request.Messages[len(request.Messages)-1].Content != "检查第二季度销售额" {
					t.Errorf("resume duplicated or lost the pending user message: %+v", request.Messages)
				}
				return []provider.ModelEvent{{TextDelta: "第二季度销售额已核对。"}, {Done: true}}
			},
		},
	}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "resume-reply", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	conversation, err := host.CreateConversation(ctx, "doc-resume-reply")
	if err != nil {
		t.Fatal(err)
	}
	view := application.ViewContext{DocumentID: "doc-resume-reply"}
	if err := host.Send(ctx, conversation.ID, "检查第二季度销售额", view); err == nil {
		t.Fatal("interrupted model reply unexpectedly succeeded")
	}
	interrupted, found := host.Snapshot(conversation.ID)
	if !found || !interrupted.Interrupted || len(interrupted.Messages) != 1 {
		t.Fatalf("interrupted conversation snapshot=%+v found=%v", interrupted, found)
	}
	if err := host.ResumeConversation(ctx, conversation.ID, view); err != nil {
		t.Fatalf("ResumeConversation: %v", err)
	}
	completed, found := host.Snapshot(conversation.ID)
	if !found || completed.Interrupted || len(completed.Messages) != 2 || completed.Messages[0].Content != "检查第二季度销售额" || completed.Messages[1].Content != "第二季度销售额已核对。" {
		t.Fatalf("resumed conversation snapshot=%+v found=%v", completed, found)
	}
}

func TestConversationSnapshotMarksPendingToolTurnInterrupted(t *testing.T) {
	cases := []struct {
		name     string
		messages []provider.ModelMessage
		want     bool
	}{
		{name: "empty", want: false},
		{name: "pending user", messages: []provider.ModelMessage{{Role: "user", Content: "检查"}}, want: true},
		{name: "pending tool call", messages: []provider.ModelMessage{{Role: "user", Content: "检查"}, {Role: "assistant", ToolCalls: []provider.ModelToolCall{{ID: "call-1"}}}}, want: true},
		{name: "tool result without final answer", messages: []provider.ModelMessage{{Role: "user", Content: "检查"}, {Role: "tool", ToolCallID: "call-1", Content: "完成"}}, want: true},
		{name: "completed reply", messages: []provider.ModelMessage{{Role: "user", Content: "检查"}, {Role: "assistant", Content: "已完成"}}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			snapshot := snapshotConversation("conversation-1", &conversationState{messages: test.messages})
			if snapshot.Interrupted != test.want {
				t.Fatalf("Interrupted=%v, want %v", snapshot.Interrupted, test.want)
			}
		})
	}
}

func TestResumeConversationDoesNotRepeatToolWithoutSavedResult(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := newMemoryConversationStore()
	conversationID := ConversationID("conversation-interrupted-tool")
	if err := store.Save(ctx, ConversationRecord{
		ID: conversationID, DocumentID: "doc-interrupted-tool", Title: "检查文档",
		Messages: []provider.ModelMessage{
			{Role: "user", Content: "检查并修改文档"},
			{Role: "assistant", ToolCalls: []provider.ModelToolCall{{
				ID: "call-write", Type: "function", Function: provider.ModelToolCallFunction{Name: "word.paragraph.append", Arguments: `{"text":"重复修改"}`},
			}}},
		},
	}); err != nil {
		t.Fatalf("Save interrupted conversation: %v", err)
	}
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
		func(request provider.ModelRequest) []provider.ModelEvent {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "call-write" || !strings.Contains(last.Content, "interrupted_tool_call") {
				t.Errorf("resume did not report the skipped persisted tool call: %+v", request.Messages)
			}
			return []provider.ModelEvent{{TextDelta: "我保留了中断状态，没有重复执行修改。"}, {Done: true}}
		},
	}}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "resume-tool", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if err := host.ResumeConversation(ctx, conversationID, application.ViewContext{DocumentID: "doc-interrupted-tool"}); err != nil {
		t.Fatalf("ResumeConversation: %v", err)
	}
	snapshot, found := host.Snapshot(conversationID)
	if !found || snapshot.Interrupted || len(snapshot.Messages) != 4 || model.calls != 1 {
		t.Fatalf("recovered tool conversation=%+v found=%v calls=%d", snapshot, found, model.calls)
	}
}

func TestInterruptedStreamPersistsReasoningButNotPartialAnswer(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := newMemoryConversationStore()
	model := &partialReasoningFailureProvider{}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "partial-reasoning", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	conversation, err := host.CreateConversation(ctx, "doc-partial-reasoning")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Send(ctx, conversation.ID, "检查这份文档", application.ViewContext{DocumentID: "doc-partial-reasoning"}); err == nil {
		t.Fatal("interrupted model stream unexpectedly succeeded")
	}
	snapshot, found := host.Snapshot(conversation.ID)
	if !found || len(snapshot.Messages) != 1 || snapshot.Messages[0].Role != "user" || len(snapshot.Reasoning) != 1 || snapshot.Reasoning[0].Content != "我已开始检查文档结构。" {
		t.Fatalf("interrupted transcript=%+v found=%v", snapshot, found)
	}
}

func TestFailedModelResponsesPersistTheirReasoning(t *testing.T) {
	for _, test := range []struct {
		name         string
		maxToolCalls int
		response     func(provider.ModelRequest) []provider.ModelEvent
		wantError    error
	}{
		{
			name: "reasoning without answer",
			response: func(provider.ModelRequest) []provider.ModelEvent {
				return []provider.ModelEvent{{ReasoningDelta: "先检查工作表结构。"}}
			},
			wantError: ErrEmptyModelResponse,
		},
		{
			name:         "tool call limit",
			maxToolCalls: 1,
			response: func(provider.ModelRequest) []provider.ModelEvent {
				return []provider.ModelEvent{
					{ReasoningDelta: "需要读取两个区域。"},
					{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "call-1", Type: "function", Name: "read.one", ArgumentsDelta: "{}"}},
					{ToolCallDelta: &provider.ToolCallDelta{Index: 1, ID: "call-2", Type: "function", Name: "read.two", ArgumentsDelta: "{}"}},
				}
			},
			wantError: ErrToolCallLimit,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runtime.Close(ctx) })
			client, err := ConnectMCP(ctx, runtime)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			store := newMemoryConversationStore()
			model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{
				func(provider.ModelRequest) []provider.ModelEvent {
					return []provider.ModelEvent{{TextDelta: "工作表检查"}}
				},
				test.response,
			}}
			host, err := NewHost(ctx, model, client.Session, HostOptions{
				Model: "reasoning-failure", ConversationStore: store, MaxToolCalls: test.maxToolCalls,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer host.Close()
			conversation, err := host.CreateConversation(ctx, "doc-reasoning-failure")
			if err != nil {
				t.Fatal(err)
			}
			err = host.Send(ctx, conversation.ID, "检查文档", application.ViewContext{DocumentID: "doc-reasoning-failure"})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Send error=%v; want %v", err, test.wantError)
			}
			snapshot, found := host.Snapshot(conversation.ID)
			if !found || len(snapshot.Reasoning) != 1 || snapshot.Reasoning[0].Content == "" || len(snapshot.Messages) != 1 {
				t.Fatalf("failed response transcript=%+v found=%v", snapshot, found)
			}
			record, found, err := store.Load(ctx, "doc-reasoning-failure", conversation.ID)
			if err != nil || !found || len(record.Reasoning) != 1 {
				t.Fatalf("persisted reasoning=%+v found=%v err=%v", record.Reasoning, found, err)
			}
		})
	}
}

func TestInterruptedReasoningDoesNotAttachToAFutureAssistantMessage(t *testing.T) {
	host := &AgentHost{}
	state := &conversationState{messages: []provider.ModelMessage{{Role: "user", Content: "检查文档"}}}
	if err := host.persistInterruptedReasoning(context.Background(), "conversation-reasoning", state, "失败轮的思考"); err != nil {
		t.Fatalf("persistInterruptedReasoning: %v", err)
	}

	_, reasoningByMessage, err := host.requestMessages(state, application.ViewContext{})
	if err != nil {
		t.Fatalf("requestMessages before resume: %v", err)
	}
	if len(reasoningByMessage) != 0 {
		t.Fatalf("interrupted reasoning unexpectedly has an assistant message before resume: %v", reasoningByMessage)
	}

	state.mu.Lock()
	state.messages = append(state.messages, provider.ModelMessage{Role: "assistant", Content: "恢复后的回答"})
	state.mu.Unlock()
	_, reasoningByMessage, err = host.requestMessages(state, application.ViewContext{})
	if err != nil {
		t.Fatalf("requestMessages after resume: %v", err)
	}
	if len(reasoningByMessage) != 0 {
		t.Fatalf("failed-turn reasoning was associated with a future assistant message: %v", reasoningByMessage)
	}
}

type partialReasoningFailureProvider struct{ testModelProviderContract }

func (*partialReasoningFailureProvider) Stream(_ context.Context, request provider.ModelRequest, emit func(provider.ModelEvent) error) error {
	if len(request.Messages) == 1 && strings.Contains(request.Messages[0].Content, "对话标题") {
		if err := emit(provider.ModelEvent{TextDelta: "文档检查"}); err != nil {
			return err
		}
		return emit(provider.ModelEvent{Done: true})
	}
	if err := emit(provider.ModelEvent{ReasoningDelta: "我已开始检查文档结构。"}); err != nil {
		return err
	}
	return errors.New("stream interrupted")
}

func TestConversationSaveFailurePreventsProviderRequest(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	client, err := ConnectMCP(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := newMemoryConversationStore()
	model := &scriptedProvider{t: t}
	host, err := NewHost(ctx, model, client.Session, HostOptions{Model: "save-failure", ConversationStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	conversation, err := host.CreateConversation(ctx, "doc-save")
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.saveErr = errors.New("disk write failed")
	store.mu.Unlock()
	if err := host.Send(ctx, conversation.ID, "不要丢失", application.ViewContext{DocumentID: "doc-save"}); err == nil {
		t.Fatal("Send succeeded after conversation persistence failed")
	}
	if model.calls != 0 {
		t.Fatalf("provider received %d requests after save failure", model.calls)
	}
	snapshot, found := host.Snapshot(conversation.ID)
	if !found || len(snapshot.Messages) != 0 {
		t.Fatalf("failed user message remained in context: %+v", snapshot)
	}
}

type memoryConversationStore struct {
	mu      sync.Mutex
	items   map[ConversationID]ConversationRecord
	saveErr error
}

func newMemoryConversationStore() *memoryConversationStore {
	return &memoryConversationStore{items: make(map[ConversationID]ConversationRecord)}
}

func (s *memoryConversationStore) Save(ctx context.Context, record ConversationRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	if current, exists := s.items[record.ID]; exists && current.DocumentID != record.DocumentID {
		return ErrConversationScopeMismatch
	}
	s.items[record.ID] = cloneConversationRecord(record)
	return nil
}

func (s *memoryConversationStore) Load(ctx context.Context, documentID string, id ConversationID) (ConversationRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ConversationRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.items[id]
	if !exists {
		return ConversationRecord{}, false, nil
	}
	if current.DocumentID != documentID {
		return ConversationRecord{}, false, ErrConversationScopeMismatch
	}
	return cloneConversationRecord(current), true, nil
}

func (s *memoryConversationStore) List(ctx context.Context, documentID string) ([]ConversationSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ConversationSummary, 0)
	for _, record := range s.items {
		if record.DocumentID == documentID {
			items = append(items, ConversationSummary{ID: record.ID, Title: record.Title, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt})
		}
	}
	return items, nil
}

func cloneConversationRecord(record ConversationRecord) ConversationRecord {
	record.Messages = cloneMessages(record.Messages)
	record.Reasoning = append([]ReasoningBlock(nil), record.Reasoning...)
	return record
}
