package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"glance/internal/agent/provider"
	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	ErrHostClosed            = errors.New("Agent Host is closed")
	ErrConversationBusy      = errors.New("conversation already has an active turn")
	ErrInvalidConversation   = errors.New("conversation ID is required")
	ErrEmptyMessage          = errors.New("message is empty")
	ErrMessageTooLarge       = errors.New("message exceeds the configured size limit")
	ErrPromptTooLarge        = errors.New("conversation context exceeds the configured size limit")
	ErrEmptyModelResponse    = errors.New("model returned neither text nor tool calls")
	ErrToolCallLimit         = errors.New("Agent turn exceeded the tool call limit")
	ErrToolRoundLimit        = errors.New("Agent turn exceeded the tool round limit")
	ErrToolArgumentsTooLarge = errors.New("tool arguments exceed the configured size limit")
	ErrToolResultTooLarge    = errors.New("tool result exceeds the configured size limit")
)

const maxListedTools = 1024

type HostOptions struct {
	Model                 string
	ModelTimeout          time.Duration
	MaxToolRounds         int
	MaxToolCalls          int
	MaxPromptBytes        int
	MaxMessageBytes       int
	MaxToolArgumentsBytes int
	MaxToolResultBytes    int
}

type ConversationSnapshot struct {
	ID       ConversationID
	Messages []provider.ModelMessage
	Busy     bool
}

type conversationState struct {
	mu       sync.Mutex
	messages []provider.ModelMessage
	busy     bool
	cancel   context.CancelFunc
}

type agentSubscriber struct {
	conversationID ConversationID
	handler        func(AgentEvent)
	stop           context.CancelFunc
	done           chan struct{}
}

type toolSpec struct {
	modelTool provider.ModelTool
	schema    map[string]any
}

type AgentHost struct {
	provider provider.ModelProvider
	client   *mcp.ClientSession
	model    string
	options  HostOptions
	tools    map[string]toolSpec

	mu                sync.Mutex
	closed            bool
	closeDone         chan struct{}
	conversations     map[ConversationID]*conversationState
	subscribers       map[uint64]*agentSubscriber
	nextSubID         atomic.Uint64
	turns             sync.WaitGroup
	subscriptionTasks sync.WaitGroup
}

func NewHost(ctx context.Context, modelProvider provider.ModelProvider, client *mcp.ClientSession, options HostOptions) (*AgentHost, error) {
	if modelProvider == nil || client == nil {
		return nil, errors.New("model provider and MCP client session are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	options = normalizeHostOptions(options)
	tools, err := loadTools(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("load MCP tools for Agent Host: %w", err)
	}
	return &AgentHost{
		provider: modelProvider, client: client, model: strings.TrimSpace(options.Model), options: options,
		tools: tools, conversations: make(map[ConversationID]*conversationState),
		subscribers: make(map[uint64]*agentSubscriber), closeDone: make(chan struct{}),
	}, nil
}

func normalizeHostOptions(options HostOptions) HostOptions {
	if options.ModelTimeout <= 0 {
		options.ModelTimeout = 3 * time.Minute
	}
	if options.MaxToolRounds <= 0 {
		options.MaxToolRounds = 4
	}
	if options.MaxToolCalls <= 0 {
		options.MaxToolCalls = 8
	}
	if options.MaxPromptBytes <= 0 {
		options.MaxPromptBytes = 1 << 20
	}
	if options.MaxMessageBytes <= 0 {
		options.MaxMessageBytes = 64 << 10
	}
	if options.MaxToolArgumentsBytes <= 0 {
		options.MaxToolArgumentsBytes = 64 << 10
	}
	if options.MaxToolResultBytes <= 0 {
		options.MaxToolResultBytes = 1 << 20
	}
	return options
}

func loadTools(ctx context.Context, client *mcp.ClientSession) (map[string]toolSpec, error) {
	tools := make(map[string]toolSpec)
	cursor := ""
	for {
		listed, err := client.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, tool := range listed.Tools {
			if strings.TrimSpace(tool.Name) == "" {
				return nil, errors.New("MCP server returned a tool with an empty name")
			}
			if len(tools) >= maxListedTools {
				return nil, fmt.Errorf("MCP tool catalog exceeds %d tools", maxListedTools)
			}
			if _, exists := tools[tool.Name]; exists {
				return nil, fmt.Errorf("MCP tool catalog contains duplicate %q", tool.Name)
			}
			schema, err := json.Marshal(tool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("encode schema for MCP tool %s: %w", tool.Name, err)
			}
			var schemaObject map[string]any
			if err := json.Unmarshal(schema, &schemaObject); err != nil || schemaObject == nil {
				return nil, fmt.Errorf("MCP tool %s has an invalid input schema", tool.Name)
			}
			tools[tool.Name] = toolSpec{
				modelTool: provider.ModelTool{Type: "function", Function: provider.ModelToolFunction{
					Name: tool.Name, Description: tool.Description, Parameters: append(json.RawMessage(nil), schema...),
				}},
				schema: schemaObject,
			}
		}
		cursor = listed.NextCursor
		if cursor == "" {
			return tools, nil
		}
	}
}

func (h *AgentHost) Send(ctx context.Context, conversationID ConversationID, message string, view application.ViewContext) error {
	if h == nil {
		return ErrHostClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(string(conversationID)) == "" {
		return ErrInvalidConversation
	}
	if strings.TrimSpace(message) == "" {
		return ErrEmptyMessage
	}
	if len(message) > h.options.MaxMessageBytes {
		return ErrMessageTooLarge
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrHostClosed
	}
	state := h.conversations[conversationID]
	if state == nil {
		state = &conversationState{}
		h.conversations[conversationID] = state
	}
	// Add while holding h.mu so Close cannot start waiting before this turn is
	// registered. The same lock also prevents Close from missing its cancel
	// function while the turn is being initialized.
	h.turns.Add(1)

	turnCtx, cancel := context.WithCancel(ctx)
	state.mu.Lock()
	if state.busy {
		state.mu.Unlock()
		h.mu.Unlock()
		cancel()
		h.turns.Done()
		return ErrConversationBusy
	}
	state.busy = true
	state.cancel = cancel
	state.messages = append(state.messages, provider.ModelMessage{Role: "user", Content: message})
	state.mu.Unlock()
	h.mu.Unlock()
	defer h.turns.Done()

	err := h.runTurn(turnCtx, conversationID, state, view)
	cancel()
	state.mu.Lock()
	state.busy = false
	state.cancel = nil
	state.mu.Unlock()
	if err != nil {
		code, message := DescribeTurnError(err)
		h.publish(AgentEvent{ConversationID: conversationID, Type: EventTurnFailed, ErrorCode: code, Error: message, IsError: true})
		return err
	}
	h.publish(AgentEvent{ConversationID: conversationID, Type: EventTurnCompleted})
	return nil
}

func (h *AgentHost) runTurn(ctx context.Context, conversationID ConversationID, state *conversationState, view application.ViewContext) error {
	totalCalls := 0
	for round := 0; round < h.options.MaxToolRounds; round++ {
		messages := h.requestMessages(state, view)
		if promptSize(messages) > h.options.MaxPromptBytes {
			return ErrPromptTooLarge
		}
		request := provider.ModelRequest{Model: h.model, Messages: messages, Tools: h.modelTools()}
		var text strings.Builder
		calls := make(map[int]*toolCallAccumulator)
		streamCtx, cancelStream := context.WithTimeout(ctx, h.options.ModelTimeout)
		err := h.provider.Stream(streamCtx, request, func(event provider.ModelEvent) error {
			if streamCtx.Err() != nil {
				return streamCtx.Err()
			}
			if event.TextDelta != "" {
				if text.Len()+len(event.TextDelta) > h.options.MaxToolResultBytes {
					return ErrMessageTooLarge
				}
				text.WriteString(event.TextDelta)
				h.publish(AgentEvent{ConversationID: conversationID, Type: EventAssistantDelta, Text: event.TextDelta})
			}
			if event.ToolCallDelta != nil {
				if err := mergeToolCall(calls, *event.ToolCallDelta, h.options.MaxToolArgumentsBytes); err != nil {
					return err
				}
			}
			return nil
		})
		cancelStream()
		if err != nil {
			return err
		}
		orderedCalls := orderedToolCalls(calls)
		if len(orderedCalls) == 0 {
			if strings.TrimSpace(text.String()) == "" {
				return ErrEmptyModelResponse
			}
			state.mu.Lock()
			state.messages = append(state.messages, provider.ModelMessage{Role: "assistant", Content: text.String()})
			state.mu.Unlock()
			return nil
		}
		totalCalls += len(orderedCalls)
		if totalCalls > h.options.MaxToolCalls {
			return ErrToolCallLimit
		}
		if round == h.options.MaxToolRounds-1 {
			return ErrToolRoundLimit
		}
		assistantMessage := provider.ModelMessage{Role: "assistant", Content: text.String()}
		for index, call := range orderedCalls {
			if call.id == "" {
				call.id = fmt.Sprintf("agent-%d-%d", round+1, index+1)
			}
			if call.callType == "" {
				call.callType = "function"
			}
			assistantMessage.ToolCalls = append(assistantMessage.ToolCalls, provider.ModelToolCall{
				ID: call.id, Type: call.callType,
				Function: provider.ModelToolCallFunction{Name: call.name, Arguments: call.arguments.String()},
			})
		}
		state.mu.Lock()
		state.messages = append(state.messages, assistantMessage)
		state.mu.Unlock()

		for callIndex, call := range orderedCalls {
			if ctx.Err() != nil {
				h.appendCanceledToolCallResults(conversationID, state, orderedCalls[callIndex:])
				return ctx.Err()
			}
			if err := h.executeToolCall(ctx, conversationID, state, call, view); err != nil {
				return err
			}
			view = h.updatedViewFromLastToolResult(state, view)
		}
	}
	return ErrToolRoundLimit
}

func (h *AgentHost) appendCanceledToolCallResults(conversationID ConversationID, state *conversationState, calls []*toolCallAccumulator) {
	if state == nil || len(calls) == 0 {
		return
	}
	const content = `{"error":"tool_call_cancelled","message":"Agent 轮次已取消，工具调用未执行。"}`
	for _, call := range calls {
		h.publish(AgentEvent{
			ConversationID: conversationID, Type: EventToolStarted,
			ToolName: call.name, ToolCallID: call.id,
		})
		state.mu.Lock()
		state.messages = append(state.messages, provider.ModelMessage{
			Role: "tool", ToolCallID: call.id, ToolName: call.name,
			Content: content,
		})
		state.mu.Unlock()
		h.publish(AgentEvent{
			ConversationID: conversationID, Type: EventToolCompleted,
			ToolName: call.name, ToolCallID: call.id, Result: json.RawMessage(content), IsError: true,
		})
	}
}

type toolCallAccumulator struct {
	index     int
	id        string
	callType  string
	name      string
	arguments strings.Builder
}

func mergeToolCall(calls map[int]*toolCallAccumulator, delta provider.ToolCallDelta, maxArguments int) error {
	call := calls[delta.Index]
	if call == nil {
		call = &toolCallAccumulator{index: delta.Index}
		calls[delta.Index] = call
	}
	if delta.ID != "" {
		call.id = delta.ID
	}
	if delta.Type != "" {
		call.callType = delta.Type
	}
	if delta.Name != "" {
		call.name += delta.Name
	}
	if call.arguments.Len()+len(delta.ArgumentsDelta) > maxArguments {
		return ErrToolArgumentsTooLarge
	}
	call.arguments.WriteString(delta.ArgumentsDelta)
	return nil
}

func orderedToolCalls(calls map[int]*toolCallAccumulator) []*toolCallAccumulator {
	ordered := make([]*toolCallAccumulator, 0, len(calls))
	indices := make([]int, 0, len(calls))
	for index := range calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		ordered = append(ordered, calls[index])
	}
	return ordered
}

func (h *AgentHost) executeToolCall(ctx context.Context, conversationID ConversationID, state *conversationState, call *toolCallAccumulator, view application.ViewContext) error {
	h.publish(AgentEvent{ConversationID: conversationID, Type: EventToolStarted, ToolName: call.name, ToolCallID: call.id})
	resultContent, isError, _ := h.invokeTool(ctx, call, view)
	if len(resultContent) > h.options.MaxToolResultBytes {
		resultContent, _ = json.Marshal(map[string]string{"error": "tool_result_too_large", "message": ErrToolResultTooLarge.Error()})
		isError = true
	}
	state.mu.Lock()
	state.messages = append(state.messages, provider.ModelMessage{
		Role: "tool", ToolCallID: call.id, ToolName: call.name, Content: string(resultContent),
	})
	state.mu.Unlock()
	h.publish(AgentEvent{ConversationID: conversationID, Type: EventToolCompleted, ToolName: call.name, ToolCallID: call.id, Result: append(json.RawMessage(nil), resultContent...), IsError: isError})
	return nil
}

func (h *AgentHost) invokeTool(ctx context.Context, call *toolCallAccumulator, view application.ViewContext) ([]byte, bool, application.ViewContext) {
	tool, registered := h.tools[call.name]
	if !registered {
		result, _ := json.Marshal(map[string]string{"error": "unknown_tool", "message": "该工具未发布在 Glance 能力目录中。"})
		return result, true, view
	}
	if call.arguments.Len() > h.options.MaxToolArgumentsBytes {
		result, _ := json.Marshal(map[string]string{"error": "invalid_arguments", "message": ErrToolArgumentsTooLarge.Error()})
		return result, true, view
	}
	arguments := make(map[string]any)
	if err := json.Unmarshal([]byte(call.arguments.String()), &arguments); err != nil || arguments == nil {
		result, _ := json.Marshal(map[string]string{"error": "invalid_arguments", "message": "工具参数必须是 JSON 对象。"})
		return result, true, view
	}
	arguments = injectViewDefaults(arguments, tool.schema, view)
	result, err := h.client.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: arguments})
	if err != nil {
		payload, _ := json.Marshal(map[string]string{"error": "mcp_call_failed", "message": err.Error()})
		return payload, true, view
	}
	content := marshalToolResult(result)
	updatedView := viewFromToolResult(result, view)
	return content, result.IsError, updatedView
}

func injectViewDefaults(arguments map[string]any, schema map[string]any, view application.ViewContext) map[string]any {
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil || view.DocumentID == "" {
		return arguments
	}
	if _, exists := properties["document_id"]; exists {
		if _, provided := arguments["document_id"]; !provided {
			arguments["document_id"] = view.DocumentID
		}
	}
	targetDocumentID, _ := arguments["document_id"].(string)
	if targetDocumentID != view.DocumentID {
		return arguments
	}
	if _, exists := properties["expected_revision"]; exists && view.Revision != 0 {
		if _, provided := arguments["expected_revision"]; !provided {
			arguments["expected_revision"] = view.Revision
		}
	}
	if _, exists := properties["view"]; exists {
		if _, provided := arguments["view"]; !provided {
			if encoded, err := json.Marshal(view); err == nil {
				var viewObject map[string]any
				if json.Unmarshal(encoded, &viewObject) == nil {
					arguments["view"] = viewObject
				}
			}
		}
	}
	return arguments
}

func marshalToolResult(result *mcp.CallToolResult) []byte {
	if result == nil {
		return []byte(`{"error":"empty_tool_result"}`)
	}
	if result.StructuredContent != nil {
		if encoded, err := json.Marshal(result.StructuredContent); err == nil {
			return encoded
		}
	}
	texts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	encoded, _ := json.Marshal(map[string]any{"is_error": result.IsError, "content": texts})
	return encoded
}

func viewFromToolResult(result *mcp.CallToolResult, current application.ViewContext) application.ViewContext {
	if result == nil || result.StructuredContent == nil {
		return current
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return current
	}
	var output struct {
		DocumentID string `json:"document_id"`
		Revision   uint64 `json:"revision"`
	}
	if json.Unmarshal(encoded, &output) != nil {
		return current
	}
	if output.DocumentID != "" {
		current.DocumentID = output.DocumentID
	}
	if output.Revision != 0 {
		current.Revision = output.Revision
	}
	return current
}

func (h *AgentHost) updatedViewFromLastToolResult(state *conversationState, current application.ViewContext) application.ViewContext {
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.messages) == 0 {
		return current
	}
	last := state.messages[len(state.messages)-1]
	if last.Role != "tool" {
		return current
	}
	var output struct {
		DocumentID string `json:"document_id"`
		Revision   uint64 `json:"revision"`
	}
	if json.Unmarshal([]byte(last.Content), &output) != nil {
		return current
	}
	if output.DocumentID != "" {
		current.DocumentID = output.DocumentID
	}
	if output.Revision != 0 {
		current.Revision = output.Revision
	}
	return current
}

func (h *AgentHost) requestMessages(state *conversationState, view application.ViewContext) []provider.ModelMessage {
	state.mu.Lock()
	messages := cloneMessages(state.messages)
	state.mu.Unlock()
	instruction := "你是 Glance 办公 Agent。先通过已注册工具检查用户明确指定的文档，再执行需要的修改。工具调用必须使用能力目录提供的参数。"
	if view.DocumentID != "" {
		encoded, _ := json.Marshal(view)
		instruction += " 当前 Gio 视图上下文为 " + string(encoded) + "。未提供目标时可使用其中的文档 ID、修订和选区；工具返回新修订后使用新修订。"
	}
	return append([]provider.ModelMessage{{Role: "system", Content: instruction}}, messages...)
}

func (h *AgentHost) modelTools() []provider.ModelTool {
	tools := make([]provider.ModelTool, 0, len(h.tools))
	names := make([]string, 0, len(h.tools))
	for name := range h.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tools = append(tools, h.tools[name].modelTool)
	}
	return tools
}

func promptSize(messages []provider.ModelMessage) int {
	encoded, _ := json.Marshal(messages)
	return len(encoded)
}

func (h *AgentHost) Snapshot(conversationID ConversationID) (ConversationSnapshot, bool) {
	if h == nil {
		return ConversationSnapshot{}, false
	}
	h.mu.Lock()
	state := h.conversations[conversationID]
	h.mu.Unlock()
	if state == nil {
		return ConversationSnapshot{}, false
	}
	state.mu.Lock()
	snapshot := ConversationSnapshot{ID: conversationID, Messages: cloneMessages(state.messages), Busy: state.busy}
	state.mu.Unlock()
	return snapshot, true
}

func cloneMessages(messages []provider.ModelMessage) []provider.ModelMessage {
	cloned := append([]provider.ModelMessage(nil), messages...)
	for index := range cloned {
		cloned[index].ToolCalls = append([]provider.ModelToolCall(nil), messages[index].ToolCalls...)
	}
	return cloned
}

func (h *AgentHost) Cancel(conversationID ConversationID) error {
	if h == nil {
		return ErrHostClosed
	}
	h.mu.Lock()
	state := h.conversations[conversationID]
	h.mu.Unlock()
	if state == nil {
		return ErrInvalidConversation
	}
	state.mu.Lock()
	cancel := state.cancel
	state.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	return nil
}

func (h *AgentHost) Subscribe(ctx context.Context, conversationID ConversationID, handler func(AgentEvent)) func() {
	if h == nil || handler == nil {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return func() {}
	}
	subCtx, stop := context.WithCancel(ctx)
	item := &agentSubscriber{conversationID: conversationID, handler: handler, stop: stop, done: make(chan struct{})}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		stop()
		return func() {}
	}
	id := h.nextSubID.Add(1)
	h.subscribers[id] = item
	h.subscriptionTasks.Add(1)
	h.mu.Unlock()
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			stop()
			h.mu.Lock()
			delete(h.subscribers, id)
			h.mu.Unlock()
		})
	}
	go func() {
		defer h.subscriptionTasks.Done()
		defer close(item.done)
		<-subCtx.Done()
		cleanup()
	}()
	return func() {
		cleanup()
		<-item.done
	}
}

func (h *AgentHost) publish(event AgentEvent) {
	if h == nil {
		return
	}
	h.mu.Lock()
	subscribers := make([]*agentSubscriber, 0, len(h.subscribers))
	for _, subscriber := range h.subscribers {
		if subscriber.conversationID == "" || subscriber.conversationID == event.ConversationID {
			subscribers = append(subscribers, subscriber)
		}
	}
	h.mu.Unlock()
	for _, subscriber := range subscribers {
		func() {
			defer func() { _ = recover() }()
			copyEvent := event
			copyEvent.Result = append(json.RawMessage(nil), event.Result...)
			subscriber.handler(copyEvent)
		}()
	}
}

func (h *AgentHost) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		done := h.closeDone
		h.mu.Unlock()
		if done != nil {
			<-done
		}
		return
	}
	if h.closeDone == nil {
		h.closeDone = make(chan struct{})
	}
	h.closed = true
	states := make([]*conversationState, 0, len(h.conversations))
	for _, state := range h.conversations {
		states = append(states, state)
	}
	subscribers := h.subscribers
	h.subscribers = make(map[uint64]*agentSubscriber)
	h.mu.Unlock()
	for _, state := range states {
		state.mu.Lock()
		cancel := state.cancel
		state.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	for _, subscriber := range subscribers {
		subscriber.stop()
	}
	h.turns.Wait()
	h.subscriptionTasks.Wait()
	close(h.closeDone)
}
