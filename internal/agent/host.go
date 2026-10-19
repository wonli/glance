package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"glance/internal/agent/prompts"
	"glance/internal/agent/provider"
	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	ErrHostClosed                   = errors.New("Agent Host is closed")
	ErrConversationBusy             = errors.New("conversation already has an active turn")
	ErrInvalidConversation          = errors.New("conversation ID is required")
	ErrConversationDocumentRequired = errors.New("document ID is required for conversation persistence")
	ErrConversationScopeMismatch    = errors.New("conversation belongs to a different document")
	ErrConversationNotInterrupted   = errors.New("conversation has no interrupted turn to resume")
	ErrEmptyMessage                 = errors.New("message is empty")
	ErrMessageTooLarge              = errors.New("message exceeds the configured size limit")
	ErrPromptTooLarge               = errors.New("conversation context exceeds the configured size limit")
	ErrEmptyModelResponse           = errors.New("model returned neither text nor tool calls")
	ErrToolCallLimit                = errors.New("Agent turn exceeded the tool call limit")
	ErrToolRoundLimit               = errors.New("Agent turn exceeded the tool round limit")
	ErrToolArgumentsTooLarge        = errors.New("tool arguments exceed the configured size limit")
	ErrToolResultTooLarge           = errors.New("tool result exceeds the configured size limit")
)

const maxListedTools = 1024

type HostOptions struct {
	// LazyTools defers the MCP tool listing until the first turn. Default callers
	// keep eager validation; desktop startup opts in to avoid an unused catalog.
	LazyTools         bool
	Model             string
	ConversationStore ConversationStore
	ModelTimeout      time.Duration
	// MaxToolRounds bounds tool execution rounds, excluding the final answer.
	MaxToolRounds         int
	MaxToolCalls          int
	MaxPromptBytes        int
	MaxMessageBytes       int
	MaxToolArgumentsBytes int
	MaxToolResultBytes    int
}

type ConversationSnapshot struct {
	ID          ConversationID
	DocumentID  string
	Title       string
	Messages    []provider.ModelMessage
	Reasoning   []ReasoningBlock
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Busy        bool
	Interrupted bool
}

type conversationState struct {
	mu         sync.Mutex
	documentID string
	title      string
	messages   []provider.ModelMessage
	reasoning  []ReasoningBlock
	createdAt  time.Time
	updatedAt  time.Time
	busy       bool
	cancel     context.CancelFunc
	requestID  string
}

type agentSubscriber struct {
	conversationID ConversationID
	handler        func(AgentEvent)
	stop           context.CancelFunc
	done           chan struct{}
}

type toolSpec struct {
	modelTool provider.ModelTool
	schema    map[string]json.RawMessage
}

type AgentHost struct {
	provider     provider.ModelProvider
	client       *mcp.ClientSession
	model        string
	options      HostOptions
	store        ConversationStore
	tools        map[string]toolSpec
	toolsMu      sync.Mutex
	toolsLoading chan struct{}

	mu                   sync.Mutex
	closed               bool
	closeDone            chan struct{}
	conversations        map[ConversationID]*conversationState
	deletedConversations map[ConversationID]struct{}
	subscribers          map[uint64]*agentSubscriber
	nextSubID            atomic.Uint64
	nextRequestID        atomic.Uint64
	turns                sync.WaitGroup
	subscriptionTasks    sync.WaitGroup
}

func NewHost(ctx context.Context, modelProvider provider.ModelProvider, client *mcp.ClientSession, options HostOptions) (*AgentHost, error) {
	if modelProvider == nil || client == nil {
		return nil, errors.New("model provider and MCP client session are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	options = normalizeHostOptions(options)
	var tools map[string]toolSpec
	if !options.LazyTools {
		var err error
		tools, err = loadTools(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("load MCP tools for Agent Host: %w", err)
		}
	}
	return &AgentHost{
		provider: modelProvider, client: client, model: strings.TrimSpace(options.Model), options: options,
		store: options.ConversationStore,
		tools: tools, conversations: make(map[ConversationID]*conversationState),
		subscribers: make(map[uint64]*agentSubscriber), closeDone: make(chan struct{}),
	}, nil
}

func normalizeHostOptions(options HostOptions) HostOptions {
	if options.ModelTimeout <= 0 {
		options.ModelTimeout = 3 * time.Minute
	}
	if options.MaxToolRounds <= 0 {
		options.MaxToolRounds = 200
	}
	if options.MaxToolCalls <= 0 {
		options.MaxToolCalls = 600
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

// Publish a successful immutable catalog once. Failed/canceled loads can retry;
// waiters can cancel independently without waiting for another turn's request.
func (h *AgentHost) ensureTools(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h.toolsMu.Lock()
		if h.tools != nil {
			h.toolsMu.Unlock()
			return nil
		}
		if pending := h.toolsLoading; pending != nil {
			h.toolsMu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		pending := make(chan struct{})
		h.toolsLoading = pending
		h.toolsMu.Unlock()
		tools, err := loadTools(ctx, h.client)
		h.toolsMu.Lock()
		if err == nil {
			h.tools = tools
		}
		h.toolsLoading = nil
		close(pending)
		h.toolsMu.Unlock()
		if err != nil {
			return fmt.Errorf("load MCP tools for Agent Host: %w", err)
		}
		return nil
	}
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
			var schemaObject map[string]json.RawMessage
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
	if h.store != nil && strings.TrimSpace(view.DocumentID) == "" {
		return ErrConversationDocumentRequired
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrHostClosed
	}
	if _, deleted := h.deletedConversations[conversationID]; deleted {
		h.mu.Unlock()
		return ErrInvalidConversation
	}
	state := h.conversations[conversationID]
	if state == nil {
		state = &conversationState{documentID: view.DocumentID, createdAt: time.Now().UTC()}
		if h.store != nil {
			record, found, err := h.store.Load(ctx, view.DocumentID, conversationID)
			if err != nil {
				h.mu.Unlock()
				return fmt.Errorf("load Agent conversation: %w", err)
			}
			if found {
				state = conversationStateFromRecord(record)
			}
		}
		h.conversations[conversationID] = state
	}
	// Add while holding h.mu so Close cannot start waiting before this turn is
	// registered. The same lock also prevents Close from missing its cancel
	// function while the turn is being initialized.
	h.turns.Add(1)

	turnCtx, cancel := context.WithCancel(ctx)
	requestID := fmt.Sprintf("agent-turn-%d", h.nextRequestID.Add(1))
	state.mu.Lock()
	if h.store != nil && state.documentID != view.DocumentID {
		state.mu.Unlock()
		h.mu.Unlock()
		cancel()
		h.turns.Done()
		return ErrConversationScopeMismatch
	}
	if state.busy {
		state.mu.Unlock()
		h.mu.Unlock()
		cancel()
		h.turns.Done()
		return ErrConversationBusy
	}
	state.busy = true
	state.cancel = cancel
	state.requestID = requestID
	if state.documentID == "" {
		state.documentID = view.DocumentID
	}
	state.messages = append(state.messages, provider.ModelMessage{Role: "user", Content: message})
	state.updatedAt = time.Now().UTC()
	state.mu.Unlock()
	h.mu.Unlock()
	defer h.turns.Done()

	if err := h.persistConversation(turnCtx, conversationID, state); err != nil {
		cancel()
		state.mu.Lock()
		if len(state.messages) > 0 {
			last := state.messages[len(state.messages)-1]
			if last.Role == "user" && last.Content == message {
				state.messages = state.messages[:len(state.messages)-1]
			}
		}
		state.busy, state.cancel, state.requestID = false, nil, ""
		state.mu.Unlock()
		return fmt.Errorf("save user message before model request: %w", err)
	}
	return h.completeTurn(turnCtx, cancel, conversationID, state, view, requestID)
}

// ResumeConversation continues the last persisted user turn without adding a
// duplicate user message. It is used after title generation, a stream, or the
// application was interrupted.
func (h *AgentHost) ResumeConversation(ctx context.Context, conversationID ConversationID, view application.ViewContext) error {
	if h == nil {
		return ErrHostClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(string(conversationID)) == "" {
		return ErrInvalidConversation
	}
	if h.store != nil && strings.TrimSpace(view.DocumentID) == "" {
		return ErrConversationDocumentRequired
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrHostClosed
	}
	if _, deleted := h.deletedConversations[conversationID]; deleted {
		h.mu.Unlock()
		return ErrInvalidConversation
	}
	state := h.conversations[conversationID]
	if state == nil {
		if h.store == nil {
			h.mu.Unlock()
			return ErrInvalidConversation
		}
		record, found, err := h.store.Load(ctx, view.DocumentID, conversationID)
		if err != nil {
			h.mu.Unlock()
			return fmt.Errorf("load Agent conversation: %w", err)
		}
		if !found {
			h.mu.Unlock()
			return ErrInvalidConversation
		}
		state = conversationStateFromRecord(record)
		h.conversations[conversationID] = state
	}
	h.turns.Add(1)
	turnCtx, cancel := context.WithCancel(ctx)
	requestID := fmt.Sprintf("agent-turn-%d", h.nextRequestID.Add(1))
	state.mu.Lock()
	if h.store != nil && state.documentID != view.DocumentID {
		state.mu.Unlock()
		h.mu.Unlock()
		cancel()
		h.turns.Done()
		return ErrConversationScopeMismatch
	}
	if state.busy {
		state.mu.Unlock()
		h.mu.Unlock()
		cancel()
		h.turns.Done()
		return ErrConversationBusy
	}
	if !ConversationInterrupted(state.messages) {
		state.mu.Unlock()
		h.mu.Unlock()
		cancel()
		h.turns.Done()
		return ErrConversationNotInterrupted
	}
	if state.documentID == "" {
		state.documentID = view.DocumentID
	}
	state.busy, state.cancel, state.requestID = true, cancel, requestID
	state.mu.Unlock()
	h.mu.Unlock()
	defer h.turns.Done()
	return h.completeTurn(turnCtx, cancel, conversationID, state, view, requestID)
}

func (h *AgentHost) completeTurn(ctx context.Context, cancel context.CancelFunc, conversationID ConversationID, state *conversationState, view application.ViewContext, requestID string) error {
	err := h.ensureConversationTitle(ctx, conversationID, state, requestID)
	if err == nil {
		err = h.repairInterruptedToolCalls(ctx, conversationID, state)
	}
	if err == nil {
		err = h.runTurn(ctx, conversationID, state, view, requestID)
	}
	cancel()
	state.mu.Lock()
	state.busy = false
	state.cancel = nil
	state.requestID = ""
	state.mu.Unlock()
	if err != nil {
		code, message := DescribeTurnError(err)
		h.publish(AgentEvent{ConversationID: conversationID, Type: EventTurnFailed, ErrorCode: code, Error: message, IsError: true})
		return err
	}
	h.publish(AgentEvent{ConversationID: conversationID, Type: EventTurnCompleted})
	return nil
}

func (h *AgentHost) runTurn(ctx context.Context, conversationID ConversationID, state *conversationState, view application.ViewContext, requestID string) error {
	if err := h.ensureTools(ctx); err != nil {
		return err
	}
	totalCalls := 0
	for round := 0; round <= h.options.MaxToolRounds; round++ {
		messages, reasoningByMessage, err := h.requestMessages(state, view)
		if err != nil {
			return err
		}
		request := provider.ModelRequest{
			RequestID: requestID, ReasoningByMessage: reasoningByMessage,
			Model: h.model, Messages: messages, Tools: h.modelTools(),
		}
		finalAnswerOnly := round == h.options.MaxToolRounds || totalCalls >= h.options.MaxToolCalls
		if finalAnswerOnly {
			request.Tools = nil
			request.Messages[0].Content += "\n本轮工具执行预算已用尽，不能再调用工具。请根据已完成的工具结果回复用户，明确说明失败原因或尚未完成的部分；不能声称未执行的操作已经完成。"
		}
		if promptSize(request.Messages) > h.options.MaxPromptBytes {
			return ErrPromptTooLarge
		}
		var text strings.Builder
		var reasoning strings.Builder
		calls := make(map[int]*toolCallAccumulator)
		streamCtx, cancelStream := context.WithTimeout(ctx, h.options.ModelTimeout)
		err = h.provider.Stream(streamCtx, request, func(event provider.ModelEvent) error {
			if streamCtx.Err() != nil {
				return streamCtx.Err()
			}
			if event.ReasoningDelta != "" {
				if reasoning.Len()+len(event.ReasoningDelta) > h.options.MaxToolResultBytes {
					return ErrMessageTooLarge
				}
				reasoning.WriteString(event.ReasoningDelta)
				h.publish(AgentEvent{ConversationID: conversationID, Type: EventReasoningDelta, Reasoning: event.ReasoningDelta})
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
			if persistErr := h.persistInterruptedReasoning(ctx, conversationID, state, reasoning.String()); persistErr != nil {
				return persistErr
			}
			return err
		}
		orderedCalls := orderedToolCalls(calls)
		if len(orderedCalls) == 0 {
			if strings.TrimSpace(text.String()) == "" {
				if persistErr := h.persistInterruptedReasoning(ctx, conversationID, state, reasoning.String()); persistErr != nil {
					return persistErr
				}
				return ErrEmptyModelResponse
			}
			state.mu.Lock()
			messageCount, reasoningCount := len(state.messages), len(state.reasoning)
			appendReasoningBlock(state, reasoning.String())
			state.messages = append(state.messages, provider.ModelMessage{Role: "assistant", Content: text.String()})
			state.updatedAt = time.Now().UTC()
			state.mu.Unlock()
			if err := h.persistConversation(ctx, conversationID, state); err != nil {
				state.mu.Lock()
				state.messages = state.messages[:messageCount]
				state.reasoning = state.reasoning[:reasoningCount]
				state.mu.Unlock()
				return fmt.Errorf("save assistant response: %w", err)
			}
			return nil
		}
		totalCalls += len(orderedCalls)
		if round == h.options.MaxToolRounds {
			if persistErr := h.persistInterruptedReasoning(ctx, conversationID, state, reasoning.String()); persistErr != nil {
				return persistErr
			}
			return ErrToolRoundLimit
		}
		if totalCalls > h.options.MaxToolCalls {
			if persistErr := h.persistInterruptedReasoning(ctx, conversationID, state, reasoning.String()); persistErr != nil {
				return persistErr
			}
			return ErrToolCallLimit
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
		messageCount, reasoningCount := len(state.messages), len(state.reasoning)
		appendReasoningBlock(state, reasoning.String())
		state.messages = append(state.messages, assistantMessage)
		state.updatedAt = time.Now().UTC()
		state.mu.Unlock()
		if err := h.persistConversation(ctx, conversationID, state); err != nil {
			state.mu.Lock()
			state.messages = state.messages[:messageCount]
			state.reasoning = state.reasoning[:reasoningCount]
			state.mu.Unlock()
			return fmt.Errorf("save assistant tool calls before execution: %w", err)
		}

		for callIndex, call := range orderedCalls {
			if ctx.Err() != nil {
				if err := h.appendCanceledToolCallResults(ctx, conversationID, state, orderedCalls[callIndex:]); err != nil {
					return err
				}
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

func (h *AgentHost) appendCanceledToolCallResults(ctx context.Context, conversationID ConversationID, state *conversationState, calls []*toolCallAccumulator) error {
	if state == nil || len(calls) == 0 {
		return nil
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
		if err := h.persistConversation(ctx, conversationID, state); err != nil {
			return fmt.Errorf("save canceled tool result: %w", err)
		}
		h.publish(AgentEvent{
			ConversationID: conversationID, Type: EventToolCompleted,
			ToolName: call.name, ToolCallID: call.id, Result: json.RawMessage(content), IsError: true,
		})
	}
	return nil
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
	state.updatedAt = time.Now().UTC()
	state.mu.Unlock()
	if err := h.persistConversation(ctx, conversationID, state); err != nil {
		return fmt.Errorf("save tool result: %w", err)
	}
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
	arguments := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(call.arguments.String()), &arguments); err != nil || arguments == nil {
		result, _ := json.Marshal(map[string]string{"error": "invalid_arguments", "message": "工具参数必须是 JSON 对象。"})
		return result, true, view
	}
	arguments = injectViewDefaults(arguments, tool.schema, view)
	encodedArguments, err := json.Marshal(arguments)
	if err != nil {
		result, _ := json.Marshal(map[string]string{"error": "invalid_arguments", "message": "工具参数无法编码为 JSON。"})
		return result, true, view
	}
	params, err := newCallToolParams(call.name, encodedArguments)
	if err != nil {
		result, _ := json.Marshal(map[string]string{"error": "invalid_arguments", "message": "工具参数必须是 JSON 对象。"})
		return result, true, view
	}
	result, err := h.client.CallTool(ctx, params)
	if err != nil {
		payload, _ := json.Marshal(map[string]string{"error": "mcp_call_failed", "message": err.Error()})
		return payload, true, view
	}
	content := marshalToolResult(result)
	updatedView := viewFromToolResult(result, view)
	return content, result.IsError, updatedView
}

func newCallToolParams(name string, arguments json.RawMessage) (*mcp.CallToolParams, error) {
	params := &mcp.CallToolParams{Name: name}
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.UseNumber()
	if err := decoder.Decode(&params.Arguments); err != nil || params.Arguments == nil {
		return nil, errors.New("MCP arguments must be a JSON object")
	}
	return params, nil
}

func injectViewDefaults(arguments map[string]json.RawMessage, schema map[string]json.RawMessage, view application.ViewContext) map[string]json.RawMessage {
	var properties map[string]json.RawMessage
	_ = json.Unmarshal(schema["properties"], &properties)
	if properties == nil || view.DocumentID == "" {
		return arguments
	}
	if _, exists := properties["document_id"]; exists {
		if _, provided := arguments["document_id"]; !provided {
			arguments["document_id"], _ = json.Marshal(view.DocumentID)
		}
	}
	var targetDocumentID string
	_ = json.Unmarshal(arguments["document_id"], &targetDocumentID)
	if targetDocumentID != view.DocumentID {
		return arguments
	}
	if _, exists := properties["expected_revision"]; exists && view.Revision != 0 {
		if _, provided := arguments["expected_revision"]; !provided {
			arguments["expected_revision"], _ = json.Marshal(view.Revision)
		}
	}
	if _, exists := properties["view"]; exists {
		if _, provided := arguments["view"]; !provided {
			arguments["view"], _ = json.Marshal(view)
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
	encoded, _ := json.Marshal(toolResultPayload{IsError: result.IsError, Content: texts})
	return encoded
}

type toolResultPayload struct {
	IsError bool     `json:"is_error"`
	Content []string `json:"content"`
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

func appendReasoningBlock(state *conversationState, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	state.reasoning = append(state.reasoning, ReasoningBlock{BeforeMessage: len(state.messages), Content: content})
}

func (h *AgentHost) persistInterruptedReasoning(ctx context.Context, conversationID ConversationID, state *conversationState, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	state.mu.Lock()
	messageCount, reasoningCount, oldUpdatedAt := len(state.messages), len(state.reasoning), state.updatedAt
	state.reasoning = append(state.reasoning, ReasoningBlock{BeforeMessage: -1, Content: content})
	state.updatedAt = time.Now().UTC()
	state.mu.Unlock()
	if err := h.persistConversation(ctx, conversationID, state); err != nil {
		state.mu.Lock()
		state.messages = state.messages[:messageCount]
		state.reasoning = state.reasoning[:reasoningCount]
		state.updatedAt = oldUpdatedAt
		state.mu.Unlock()
		return fmt.Errorf("save interrupted reasoning transcript: %w", err)
	}
	return nil
}

func (h *AgentHost) repairInterruptedToolCalls(ctx context.Context, conversationID ConversationID, state *conversationState) error {
	state.mu.Lock()
	messages := cloneMessages(state.messages)
	missing := make([]provider.ModelToolCall, 0)
	for messageIndex, message := range messages {
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.ID == "" {
				continue
			}
			found := false
			for _, later := range messages[messageIndex+1:] {
				if later.Role == "tool" && later.ToolCallID == call.ID {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, call)
			}
		}
	}
	if len(missing) == 0 {
		state.mu.Unlock()
		return nil
	}
	messageCount := len(state.messages)
	oldUpdatedAt := state.updatedAt
	for _, call := range missing {
		content := `{"error":"interrupted_tool_call","message":"The previous run stopped before this tool result was saved. The action was not repeated automatically."}`
		state.messages = append(state.messages, provider.ModelMessage{
			Role: "tool", ToolCallID: call.ID, ToolName: call.Function.Name, Content: content,
		})
	}
	state.updatedAt = time.Now().UTC()
	state.mu.Unlock()
	if err := h.persistConversation(ctx, conversationID, state); err != nil {
		state.mu.Lock()
		state.messages = state.messages[:messageCount]
		state.updatedAt = oldUpdatedAt
		state.mu.Unlock()
		return fmt.Errorf("save interrupted tool results: %w", err)
	}
	return nil
}

func (h *AgentHost) requestMessages(state *conversationState, view application.ViewContext) ([]provider.ModelMessage, map[int]string, error) {
	state.mu.Lock()
	messages := cloneMessages(state.messages)
	reasoning := append([]ReasoningBlock(nil), state.reasoning...)
	state.mu.Unlock()
	viewContext := ""
	if view.DocumentID != "" {
		encoded, _ := json.Marshal(view)
		viewContext = string(encoded)
	}
	instruction, err := prompts.System(prompts.SystemData{ViewContext: viewContext})
	if err != nil {
		return nil, nil, err
	}
	reasoningByMessage := make(map[int]string, len(reasoning))
	for _, block := range reasoning {
		messageIndex := block.BeforeMessage
		if messageIndex < 0 || messageIndex >= len(messages) || messages[messageIndex].Role != "assistant" {
			continue
		}
		// The system instruction is prepended to the request, shifting stored
		// message indexes by one. Providers that need prior thinking can map
		// this metadata into their own protocol fields.
		requestIndex := messageIndex + 1
		previous := reasoningByMessage[requestIndex]
		if previous != "" {
			previous += "\n"
		}
		reasoningByMessage[requestIndex] = previous + block.Content
	}
	return append([]provider.ModelMessage{{Role: "system", Content: instruction}}, messages...), reasoningByMessage, nil
}

func (h *AgentHost) ensureConversationTitle(ctx context.Context, conversationID ConversationID, state *conversationState, requestID string) error {
	if h.store == nil {
		return nil
	}
	state.mu.Lock()
	if state.title != "" {
		state.mu.Unlock()
		return nil
	}
	firstMessage := ""
	for _, message := range state.messages {
		if message.Role == "user" {
			firstMessage = message.Content
			break
		}
	}
	state.mu.Unlock()
	if firstMessage == "" {
		return ErrEmptyMessage
	}
	titlePrompt, err := prompts.ConversationTitle(prompts.TitleData{UserMessage: firstMessage})
	if err != nil {
		return err
	}
	request := provider.ModelRequest{
		RequestID: requestID,
		Model:     h.model,
		Messages:  []provider.ModelMessage{{Role: "user", Content: titlePrompt}},
	}
	titleCtx, cancel := context.WithTimeout(ctx, h.options.ModelTimeout)
	defer cancel()
	var title strings.Builder
	err = h.provider.Stream(titleCtx, request, func(event provider.ModelEvent) error {
		if titleCtx.Err() != nil {
			return titleCtx.Err()
		}
		if event.ToolCallDelta != nil {
			return errors.New("title response unexpectedly contained a tool call")
		}
		if title.Len()+len(event.TextDelta) > 1024 {
			return errors.New("conversation title response exceeds limit")
		}
		title.WriteString(event.TextDelta)
		return nil
	})
	if err != nil {
		return fmt.Errorf("generate conversation title: %w", err)
	}
	generated := strings.Trim(strings.TrimSpace(title.String()), "\"'“”‘’#*` \r\n\t")
	if generated == "" {
		return errors.New("model returned an empty conversation title")
	}
	if len([]rune(generated)) > 32 {
		return errors.New("model returned a conversation title that is too long")
	}
	state.mu.Lock()
	oldTitle, oldUpdatedAt := state.title, state.updatedAt
	state.title = generated
	state.updatedAt = time.Now().UTC()
	state.mu.Unlock()
	if err := h.persistConversation(ctx, conversationID, state); err != nil {
		state.mu.Lock()
		state.title, state.updatedAt = oldTitle, oldUpdatedAt
		state.mu.Unlock()
		return fmt.Errorf("save conversation title: %w", err)
	}
	h.publish(AgentEvent{ConversationID: conversationID, Type: EventConversationTitle, Title: generated})
	return nil
}

func (h *AgentHost) persistConversation(ctx context.Context, id ConversationID, state *conversationState) error {
	if h.store == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state.mu.Lock()
	record := ConversationRecord{
		ID: id, DocumentID: state.documentID, Title: state.title,
		Messages: cloneMessages(state.messages), Reasoning: append([]ReasoningBlock(nil), state.reasoning...),
		CreatedAt: state.createdAt, UpdatedAt: state.updatedAt,
	}
	state.mu.Unlock()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	saveCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		saveCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	return h.store.Save(saveCtx, record)
}

func conversationStateFromRecord(record ConversationRecord) *conversationState {
	return &conversationState{
		documentID: record.DocumentID, title: record.Title, messages: cloneMessages(record.Messages),
		reasoning: append([]ReasoningBlock(nil), record.Reasoning...), createdAt: record.CreatedAt, updatedAt: record.UpdatedAt,
	}
}

func (h *AgentHost) CreateConversation(ctx context.Context, documentID string) (ConversationSnapshot, error) {
	if h == nil || h.store == nil {
		return ConversationSnapshot{}, errors.New("conversation storage is unavailable")
	}
	if strings.TrimSpace(documentID) == "" {
		return ConversationSnapshot{}, ErrConversationDocumentRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id, err := NewConversationID()
	if err != nil {
		return ConversationSnapshot{}, err
	}
	now := time.Now().UTC()
	state := &conversationState{documentID: documentID, createdAt: now, updatedAt: now}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ConversationSnapshot{}, ErrHostClosed
	}
	if err := h.store.Save(ctx, ConversationRecord{ID: id, DocumentID: documentID, Messages: []provider.ModelMessage{}, Reasoning: []ReasoningBlock{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		h.mu.Unlock()
		return ConversationSnapshot{}, fmt.Errorf("create Agent conversation: %w", err)
	}
	h.conversations[id] = state
	h.mu.Unlock()
	return snapshotConversation(id, state), nil
}

func (h *AgentHost) ListConversations(ctx context.Context, documentID string) ([]ConversationSummary, error) {
	if h == nil || h.store == nil {
		return nil, errors.New("conversation storage is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := h.store.List(ctx, documentID)
	return items, err
}

func (h *AgentHost) LoadConversation(ctx context.Context, documentID string, id ConversationID) (ConversationSnapshot, bool, error) {
	if h == nil || h.store == nil {
		return ConversationSnapshot{}, false, errors.New("conversation storage is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	h.mu.Lock()
	if _, deleted := h.deletedConversations[id]; deleted {
		h.mu.Unlock()
		return ConversationSnapshot{}, false, ErrInvalidConversation
	}
	record, found, err := h.store.Load(ctx, documentID, id)
	if err != nil || !found {
		h.mu.Unlock()
		return ConversationSnapshot{}, found, err
	}
	state := conversationStateFromRecord(record)
	if h.closed {
		h.mu.Unlock()
		return ConversationSnapshot{}, false, ErrHostClosed
	}
	if existing := h.conversations[id]; existing != nil {
		existing.mu.Lock()
		if existing.busy {
			existing.mu.Unlock()
			h.mu.Unlock()
			return ConversationSnapshot{}, false, ErrConversationBusy
		}
		existing.mu.Unlock()
	}
	h.conversations[id] = state
	h.mu.Unlock()
	return snapshotConversation(id, state), true, nil
}

func snapshotConversation(id ConversationID, state *conversationState) ConversationSnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()
	return ConversationSnapshot{
		ID: id, DocumentID: state.documentID, Title: state.title, Messages: cloneMessages(state.messages),
		Reasoning: append([]ReasoningBlock(nil), state.reasoning...), CreatedAt: state.createdAt, UpdatedAt: state.updatedAt,
		Busy: state.busy, Interrupted: ConversationInterrupted(state.messages),
	}
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
	return snapshotConversation(conversationID, state), true
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
	requestID := state.requestID
	state.mu.Unlock()
	if cancel == nil {
		return nil
	}
	if requestID != "" {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = h.provider.Stop(stopCtx, requestID)
		stopCancel()
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
		requestID := state.requestID
		state.mu.Unlock()
		if requestID != "" {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = h.provider.Stop(stopCtx, requestID)
			stopCancel()
		}
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
