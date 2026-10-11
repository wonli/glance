package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync/atomic"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverVersion = "0.1.0"

type Server struct {
	runtime      *application.Runtime
	server       *mcp.Server
	operatorKind application.OperatorKind
	sequence     atomic.Uint64
}

func New(runtime *application.Runtime) (*Server, error) {
	return NewWithOperator(runtime, application.OperatorExternalAgent)
}

func NewWithOperator(runtime *application.Runtime, operatorKind application.OperatorKind) (*Server, error) {
	if runtime == nil {
		return nil, errors.New("application runtime is required")
	}
	if operatorKind != application.OperatorExternalAgent && operatorKind != application.OperatorBuiltInAgent {
		return nil, errors.New("MCP server operator must be built-in or external Agent")
	}
	instance := &Server{runtime: runtime, operatorKind: operatorKind}
	instance.server = mcp.NewServer(&mcp.Implementation{Name: "glance", Version: serverVersion}, nil)
	descriptors := runtime.Catalog().Descriptors()
	for _, descriptor := range descriptors {
		inputSchema, err := mcpInputSchema(descriptor)
		if err != nil {
			return nil, fmt.Errorf("build MCP schema for %s: %w", descriptor.ID, err)
		}
		tool := &mcp.Tool{
			Name: descriptor.ID, Title: descriptor.Name, Description: descriptor.Description,
			Annotations: &mcp.ToolAnnotations{
				Title:        descriptor.Name,
				ReadOnlyHint: descriptor.Category == application.CapabilityQuery,
			},
			InputSchema: inputSchema, OutputSchema: mcpOutputSchema(descriptor),
		}
		captured := descriptor
		instance.server.AddTool(tool, instance.toolHandler(captured))
	}
	return instance, nil
}

func (s *Server) RunStdio(ctx context.Context) error {
	if s == nil || s.server == nil {
		return errors.New("MCP server is not initialized")
	}
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

// CloseSessions terminates connected MCP sessions. Call it after the transport
// owner stops accepting new connections.
func (s *Server) CloseSessions() error {
	if s == nil || s.server == nil {
		return nil
	}
	var closeErrors []error
	for session := range s.server.Sessions() {
		if err := session.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func (s *Server) HTTPHandler(options *mcp.StreamableHTTPOptions) http.Handler {
	if s == nil || s.server == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "MCP server is not initialized", http.StatusServiceUnavailable)
		})
	}
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.server }, options)
}

func (s *Server) ConnectInMemory(ctx context.Context) (*mcp.ClientSession, func() error, error) {
	if s == nil || s.server == nil {
		return nil, nil, errors.New("MCP server is not initialized")
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := s.server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "glance-internal-client", Version: serverVersion}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		return nil, nil, err
	}
	closeSession := func() error {
		clientErr := clientSession.Close()
		serverErr := serverSession.Close()
		return errors.Join(clientErr, serverErr)
	}
	return clientSession, closeSession, nil
}

func (s *Server) toolHandler(descriptor application.CapabilityDescriptor) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(request.Params.Arguments, &fields); err != nil || fields == nil {
			return toolError(fmt.Errorf("decode MCP tool arguments: %w", application.ErrInvalidCommandArguments)), nil
		}
		var documentID, idempotencyKey string
		var expectedRevision uint64
		var view application.ViewContext
		if raw := fields["document_id"]; len(raw) != 0 {
			if err := json.Unmarshal(raw, &documentID); err != nil {
				return toolError(fmt.Errorf("decode document_id: %w", application.ErrInvalidCommandArguments)), nil
			}
			delete(fields, "document_id")
		}
		if raw := fields["expected_revision"]; len(raw) != 0 {
			if !descriptor.RequiresDocument {
				return toolError(application.ErrInvalidCommandArguments), nil
			}
			if err := json.Unmarshal(raw, &expectedRevision); err != nil {
				return toolError(fmt.Errorf("decode expected_revision: %w", application.ErrInvalidCommandArguments)), nil
			}
			delete(fields, "expected_revision")
		}
		if raw := fields["idempotency_key"]; len(raw) != 0 {
			if descriptor.Category != application.CapabilityCommand {
				return toolError(application.ErrInvalidCommandArguments), nil
			}
			if err := json.Unmarshal(raw, &idempotencyKey); err != nil {
				return toolError(fmt.Errorf("decode idempotency_key: %w", application.ErrInvalidCommandArguments)), nil
			}
			delete(fields, "idempotency_key")
		}
		if raw := fields["view"]; len(raw) != 0 {
			if !descriptor.RequiresDocument {
				return toolError(application.ErrInvalidCommandArguments), nil
			}
			if err := json.Unmarshal(raw, &view); err != nil {
				return toolError(fmt.Errorf("decode view context: %w", application.ErrInvalidCommandArguments)), nil
			}
			delete(fields, "view")
		}
		arguments, err := json.Marshal(fields)
		if err != nil {
			return toolError(err), nil
		}
		clientID := "mcp-client"
		if info := request.ClientInfo(); info != nil && info.Name != "" {
			clientID = info.Name
		}
		sessionID := ""
		if request.Session != nil {
			sessionID = request.Session.ID()
		}
		requestID := fmt.Sprintf("mcp-%d", s.sequence.Add(1))
		if sessionID != "" {
			requestID = sessionID + "/" + requestID
		}
		result, err := s.runtime.InvokeCapability(ctx, application.CapabilityRequest{
			RequestID: requestID, IdempotencyKey: idempotencyKey, DocumentID: documentID,
			CapabilityID: descriptor.ID, ExpectedRevision: expectedRevision,
			Operator: application.Operator{ID: clientID, Kind: s.operatorKind},
			View:     view, Arguments: arguments,
		})
		if err != nil {
			return toolError(fmt.Errorf("invoke capability %s: %w", descriptor.ID, err)), nil
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return toolError(err), nil
		}
		var structured any
		if err := json.Unmarshal(encoded, &structured); err != nil {
			return toolError(err), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}, StructuredContent: structured,
		}, nil
	}
}

func toolError(err error) *mcp.CallToolResult {
	code := errorCode(err)
	content, _ := json.Marshal(map[string]string{"error": code, "message": err.Error()})
	var structured any
	_ = json.Unmarshal(content, &structured)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(content)}}, StructuredContent: structured, IsError: true}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, application.ErrDocumentNotFound):
		return "document_not_found"
	case errors.Is(err, application.ErrWorkspaceUnavailable):
		return "workspace_unavailable"
	case errors.Is(err, application.ErrRevisionConflict):
		return "revision_conflict"
	case errors.Is(err, application.ErrExpectedRevisionRequired):
		return "expected_revision_required"
	case errors.Is(err, application.ErrCapabilityUnsupported):
		return "capability_unsupported"
	case errors.Is(err, application.ErrDocumentCompareFormatMismatch):
		return "document_format_mismatch"
	case errors.Is(err, application.ErrReadOnlyDocument):
		return "read_only_document"
	case errors.Is(err, application.ErrDocumentProtected):
		return "document_protected"
	case errors.Is(err, application.ErrTargetExists):
		return "target_exists"
	case errors.Is(err, application.ErrHumanConfirmationRequired):
		return "human_confirmation_required"
	case errors.Is(err, application.ErrConfirmationNotFound):
		return "confirmation_not_found"
	case errors.Is(err, application.ErrConfirmationNotPending):
		return "confirmation_not_pending"
	case errors.Is(err, application.ErrConfirmationQueueFull):
		return "confirmation_queue_full"
	case errors.Is(err, application.ErrDocumentPlanUnavailable):
		return "document_plan_unavailable"
	case errors.Is(err, application.ErrDocumentPlanStoreFull):
		return "document_plan_store_full"
	case errors.Is(err, application.ErrInvalidCommandArguments):
		return "invalid_arguments"
	case errors.Is(err, application.ErrNoAttachedView):
		return "no_attached_view"
	case errors.Is(err, application.ErrViewContextMismatch):
		return "view_context_mismatch"
	case errors.Is(err, application.ErrInvalidViewIntent):
		return "invalid_view_intent"
	case errors.Is(err, application.ErrCapabilityNotFound):
		return "capability_not_found"
	case errors.Is(err, application.ErrPendingEditorDraft):
		return "pending_editor_draft"
	default:
		return "execution_failed"
	}
}

func mcpInputSchema(descriptor application.CapabilityDescriptor) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(descriptor.InputSchema, &schema); err != nil || schema == nil {
		return nil, errors.New("capability input schema must be a JSON object")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		properties = make(map[string]any)
		schema["properties"] = properties
	}
	required := schemaStringArray(schema["required"])
	if descriptor.RequiresDocument {
		properties["document_id"] = map[string]any{"type": "string", "description": "Glance 实时文档会话 ID"}
		required = append(required, "document_id")
		properties["view"] = viewContextSchema(descriptor.RequiresView)
		properties["expected_revision"] = map[string]any{"type": "integer", "description": "本次操作基于的当前文档修订版本"}
	}
	if descriptor.RequiresView {
		required = append(required, "view")
	}
	if descriptor.RequiresRevision {
		required = append(required, "expected_revision")
	}
	if descriptor.Category == application.CapabilityCommand {
		properties["idempotency_key"] = map[string]any{"type": "string", "description": "可选的幂等键，用于安全重试相同操作"}
	}
	schema["required"] = uniqueStrings(required)
	schema["additionalProperties"] = false
	return json.Marshal(schema)
}

func viewContextSchema(requireWindow bool) map[string]any {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"window_id":   map[string]any{"type": "integer"},
			"document_id": map[string]any{"type": "string"},
			"revision":    map[string]any{"type": "integer"},
			"selection": map[string]any{
				"type": "object", "properties": map[string]any{
					"target_type": map[string]any{"type": "string"},
					"target_id":   map[string]any{"type": "string"},
					"sheet":       map[string]any{"type": "string"},
					"range": map[string]any{"type": "object", "properties": map[string]any{
						"start_row": map[string]any{"type": "integer"}, "start_column": map[string]any{"type": "integer"},
						"end_row": map[string]any{"type": "integer"}, "end_column": map[string]any{"type": "integer"},
					}},
				}, "required": []string{"target_type"}, "additionalProperties": false,
			},
		}, "additionalProperties": false,
	}
	if requireWindow {
		schema["required"] = []string{"window_id", "document_id", "revision"}
	}
	return schema
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func schemaStringArray(raw any) []string {
	items, _ := raw.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func mcpOutputSchema(descriptor application.CapabilityDescriptor) map[string]any {
	var data any = map[string]any{}
	if len(descriptor.OutputSchema) != 0 {
		_ = json.Unmarshal(descriptor.OutputSchema, &data)
	}
	if descriptor.Risk == application.RiskHigh || descriptor.Risk == application.RiskCritical {
		data = map[string]any{
			"oneOf": []any{
				data,
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status":            map[string]any{"type": "string", "enum": []string{"confirmation_required", "confirmation_running", "confirmation_rejected", "confirmation_failed", "confirmation_expired", "confirmation_cancelled"}},
						"confirmation_id":   map[string]any{"type": "string"},
						"capability_id":     map[string]any{"type": "string"},
						"capability_name":   map[string]any{"type": "string"},
						"document_id":       map[string]any{"type": "string"},
						"document_label":    map[string]any{"type": "string"},
						"expected_revision": map[string]any{"type": "integer"},
						"impact_summary":    map[string]any{"type": "string"},
						"expires_at":        map[string]any{"type": "string"},
						"error":             map[string]any{"type": "string"},
					},
					"required":             []string{"status", "confirmation_id", "capability_id", "capability_name", "impact_summary", "expires_at"},
					"additionalProperties": false,
				},
			},
		}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request_id":  map[string]any{"type": "string"},
			"document_id": map[string]any{"type": "string"},
			"revision":    map[string]any{"type": "integer"},
			"data":        data,
		},
		"required": []string{"request_id", "data"},
	}
}
