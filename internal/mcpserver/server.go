package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync/atomic"

	"glance/internal/application"
	"glance/internal/jsonschema"

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

// RunStdioWithIO runs the MCP server over caller-provided newline-delimited
// streams. The caller retains ownership of both stream objects.
func (s *Server) RunStdioWithIO(ctx context.Context, reader io.ReadCloser, writer io.WriteCloser) error {
	if s == nil || s.server == nil {
		return errors.New("MCP server is not initialized")
	}
	if reader == nil || writer == nil {
		return errors.New("MCP stdio streams are required")
	}
	return s.server.Run(ctx, &mcp.IOTransport{Reader: reader, Writer: writer})
}

// ServeConn serves one MCP session over a bidirectional newline-delimited
// JSON-RPC connection. The caller owns and closes conn.
func (s *Server) ServeConn(ctx context.Context, conn net.Conn) error {
	if s == nil || s.server == nil {
		return errors.New("MCP server is not initialized")
	}
	if conn == nil {
		return errors.New("MCP connection is required")
	}
	session, err := s.server.Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
	if err != nil {
		return err
	}
	return session.Wait()
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
	var schema jsonschema.Definition
	if err := json.Unmarshal(descriptor.InputSchema, &schema); err != nil {
		return nil, errors.New("capability input schema must be a JSON object")
	}
	properties := schema.Properties
	if properties == nil {
		properties = make(map[string]jsonschema.Definition)
	}
	required := append([]string(nil), schema.Required...)
	if descriptor.RequiresDocument {
		properties["document_id"] = jsonschema.Definition{Type: jsonschema.StringType("string"), Description: "Glance 实时文档会话 ID"}
		required = append(required, "document_id")
		var viewSchema jsonschema.Definition
		if err := json.Unmarshal(viewContextSchema(descriptor.RequiresView), &viewSchema); err != nil {
			return nil, fmt.Errorf("build MCP view context schema: %w", err)
		}
		properties["view"] = viewSchema
		properties["expected_revision"] = jsonschema.Definition{Type: jsonschema.StringType("integer"), Description: "本次操作基于的当前文档修订版本"}
	}
	if descriptor.RequiresView {
		required = append(required, "view")
	}
	if descriptor.RequiresRevision {
		required = append(required, "expected_revision")
	}
	if descriptor.Category == application.CapabilityCommand {
		properties["idempotency_key"] = jsonschema.Definition{Type: jsonschema.StringType("string"), Description: "可选的幂等键，用于安全重试相同操作"}
	}
	schema.Properties = properties
	schema.Required = uniqueStrings(required)
	schema.AdditionalProperties = jsonschema.RawBool(false)
	return json.Marshal(schema)
}

type schemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func viewContextSchema(requireWindow bool) json.RawMessage {
	rangeProperties := map[string]jsonschema.Definition{
		"start_row":    {Type: jsonschema.StringType("integer")},
		"start_column": {Type: jsonschema.StringType("integer")},
		"end_row":      {Type: jsonschema.StringType("integer")},
		"end_column":   {Type: jsonschema.StringType("integer")},
	}
	selectionProperties := map[string]jsonschema.Definition{
		"target_type": {Type: jsonschema.StringType("string")},
		"target_id":   {Type: jsonschema.StringType("string")},
		"sheet":       {Type: jsonschema.StringType("string")},
		"range": {Type: jsonschema.StringType("object"), Properties: rangeProperties,
			AdditionalProperties: jsonschema.RawBool(false)},
	}
	rootProperties := map[string]jsonschema.Definition{
		"window_id":   {Type: jsonschema.StringType("integer")},
		"document_id": {Type: jsonschema.StringType("string")},
		"revision":    {Type: jsonschema.StringType("integer")},
		"selection": {Type: jsonschema.StringType("object"), Properties: selectionProperties,
			Required: []string{"target_type"}, AdditionalProperties: jsonschema.RawBool(false)},
	}
	required := []string(nil)
	if requireWindow {
		required = []string{"window_id", "document_id", "revision"}
	}
	return rawSchema(jsonschema.Definition{Type: jsonschema.StringType("object"), Properties: rootProperties,
		Required: required, AdditionalProperties: jsonschema.RawBool(false)})
}

func rawSchema[T any](value T) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
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

type outputSchemaDocument struct {
	Type       string           `json:"type"`
	Properties outputProperties `json:"properties"`
	Required   []string         `json:"required"`
}

type outputProperties struct {
	RequestID  schemaProperty  `json:"request_id"`
	DocumentID schemaProperty  `json:"document_id"`
	Revision   schemaProperty  `json:"revision"`
	Data       json.RawMessage `json:"data"`
}

type schemaAlternatives struct {
	OneOf []json.RawMessage `json:"oneOf"`
}

func outputDataSchema(descriptor application.CapabilityDescriptor) json.RawMessage {
	data := json.RawMessage(`{}`)
	if json.Valid(descriptor.OutputSchema) {
		data = append(json.RawMessage(nil), descriptor.OutputSchema...)
	}
	if descriptor.Risk != application.RiskHigh && descriptor.Risk != application.RiskCritical {
		return data
	}
	confirmationSchema := rawSchema(jsonschema.Definition{
		Type: jsonschema.StringType("object"),
		Properties: map[string]jsonschema.Definition{
			"status": {
				Type: jsonschema.StringType("string"),
				Enum: []json.RawMessage{
					rawSchema("confirmation_required"), rawSchema("confirmation_running"), rawSchema("confirmation_rejected"),
					rawSchema("confirmation_failed"), rawSchema("confirmation_expired"), rawSchema("confirmation_cancelled"),
				},
			},
			"confirmation_id":   {Type: jsonschema.StringType("string")},
			"capability_id":     {Type: jsonschema.StringType("string")},
			"capability_name":   {Type: jsonschema.StringType("string")},
			"document_id":       {Type: jsonschema.StringType("string")},
			"document_label":    {Type: jsonschema.StringType("string")},
			"expected_revision": {Type: jsonschema.StringType("integer")},
			"impact_summary":    {Type: jsonschema.StringType("string")},
			"expires_at":        {Type: jsonschema.StringType("string")},
			"error":             {Type: jsonschema.StringType("string")},
		},
		Required:             []string{"status", "confirmation_id", "capability_id", "capability_name", "impact_summary", "expires_at"},
		AdditionalProperties: jsonschema.RawBool(false),
	})
	return rawSchema(schemaAlternatives{OneOf: []json.RawMessage{data, confirmationSchema}})
}

func mcpOutputSchema(descriptor application.CapabilityDescriptor) json.RawMessage {
	return rawSchema(outputSchemaDocument{
		Type: "object",
		Properties: outputProperties{
			RequestID:  schemaProperty{Type: "string"},
			DocumentID: schemaProperty{Type: "string"},
			Revision:   schemaProperty{Type: "integer"},
			Data:       outputDataSchema(descriptor),
		},
		Required: []string{"request_id", "data"},
	})
}
