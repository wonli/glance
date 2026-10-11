package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrNonCommandCapabilityChangedDocument = errors.New("non-command capability attempted to change document content")

type CapabilityRequest struct {
	RequestID           string          `json:"request_id"`
	IdempotencyKey      string          `json:"idempotency_key,omitempty"`
	DocumentID          string          `json:"document_id,omitempty"`
	CapabilityID        string          `json:"capability_id"`
	ExpectedRevision    uint64          `json:"expected_revision,omitempty"`
	Operator            Operator        `json:"operator"`
	View                ViewContext     `json:"view,omitempty"`
	Arguments           json.RawMessage `json:"arguments"`
	expectedRevisionSet bool
}

type CapabilityInvocationResult struct {
	RequestID  string          `json:"request_id"`
	DocumentID string          `json:"document_id,omitempty"`
	Revision   uint64          `json:"revision,omitempty"`
	Data       json.RawMessage `json:"data"`
}

// InvokeCapability dispatches commands through CommandBus and query/workflow
// capabilities through their registered handlers.
func (r *Runtime) InvokeCapability(ctx context.Context, request CapabilityRequest) (CapabilityInvocationResult, error) {
	return r.invokeCapability(ctx, request, false)
}

func (r *Runtime) invokeCapability(ctx context.Context, request CapabilityRequest, humanApproved bool) (CapabilityInvocationResult, error) {
	if r == nil {
		return CapabilityInvocationResult{}, ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return CapabilityInvocationResult{}, err
	}
	if strings.TrimSpace(request.RequestID) == "" || strings.TrimSpace(request.CapabilityID) == "" || strings.TrimSpace(request.Operator.ID) == "" || !validOperatorKind(request.Operator.Kind) {
		return CapabilityInvocationResult{}, ErrInvalidCapability
	}
	registration, ok := r.catalog.lookup(request.CapabilityID)
	if !ok {
		return CapabilityInvocationResult{}, fmt.Errorf("%w: %s", ErrCapabilityNotFound, request.CapabilityID)
	}
	// A deliberate Gio action by a human is an explicit approval even when the
	// capability is a workflow. Remote MCP callers receive Agent operator kinds
	// from the server and cannot opt into this bypass.
	humanApproved = humanApproved || request.Operator.Kind == OperatorHuman
	if registration.descriptor.Category == CapabilityCommand {
		command := CommandRequest{
			RequestID: request.RequestID, IdempotencyKey: request.IdempotencyKey,
			DocumentID: request.DocumentID, CommandID: request.CapabilityID,
			ExpectedRevision: request.ExpectedRevision, Operator: request.Operator,
			View: request.View, Arguments: request.Arguments,
		}
		if !humanApproved && highRisk(registration.descriptor.Risk) {
			return r.stageCommandConfirmation(ctx, request, command, registration.descriptor)
		}
		var result CommandResult
		var err error
		if humanApproved {
			result, err = r.commands.executeApproved(ctx, command)
		} else {
			result, err = r.commands.Execute(ctx, command)
		}
		if err != nil {
			return CapabilityInvocationResult{}, err
		}
		return CapabilityInvocationResult{RequestID: result.RequestID, DocumentID: result.DocumentID, Revision: result.Revision, Data: result.Data}, nil
	}
	if request.ExpectedRevision == 0 && registration.descriptor.RequiresRevision {
		return CapabilityInvocationResult{}, ErrExpectedRevisionRequired
	}
	if registration.descriptor.RequiresDocument && strings.TrimSpace(request.DocumentID) == "" {
		return CapabilityInvocationResult{}, ErrInvalidDocumentID
	}
	if registration.descriptor.RequiresView {
		if request.View.WindowID == 0 {
			return CapabilityInvocationResult{}, ErrNoAttachedView
		}
		if request.View.DocumentID != request.DocumentID || request.View.Revision == 0 {
			return CapabilityInvocationResult{}, ErrViewContextMismatch
		}
	}
	if err := validateArguments(registration.descriptor.InputSchema, request.Arguments); err != nil {
		return CapabilityInvocationResult{}, err
	}
	if request.View.DocumentID != "" && request.View.DocumentID != request.DocumentID {
		return CapabilityInvocationResult{}, ErrViewContextMismatch
	}
	var info DocumentInfo
	var adapter DocumentAdapter
	if request.DocumentID != "" {
		session, err := r.session(request.DocumentID)
		if err != nil {
			return CapabilityInvocationResult{}, err
		}
		session.mu.Lock()
		if session.closed {
			session.mu.Unlock()
			return CapabilityInvocationResult{}, ErrDocumentNotFound
		}
		info = session.infoLocked()
		adapter = session.adapter
		session.mu.Unlock()
		if (request.ExpectedRevision != 0 || request.expectedRevisionSet) && request.ExpectedRevision != info.Revision || request.View.Revision != 0 && request.View.Revision != info.Revision {
			return CapabilityInvocationResult{}, fmt.Errorf("%w: current=%d expected=%d", ErrRevisionConflict, info.Revision, request.ExpectedRevision)
		}
		if len(registration.descriptor.Formats) > 0 && !containsFormat(registration.descriptor.Formats, info.Format) {
			return CapabilityInvocationResult{}, fmt.Errorf("%w: %s does not support %s", ErrCapabilityUnsupported, registration.descriptor.ID, info.Format)
		}
	}
	if !humanApproved && highRisk(registration.descriptor.Risk) {
		if request.DocumentID != "" && request.ExpectedRevision == 0 {
			request.ExpectedRevision = info.Revision
		}
		if request.DocumentID != "" {
			request.expectedRevisionSet = true
		}
		info, err := r.confirmations.stage(ctx, request, registration.descriptor)
		if err != nil {
			return CapabilityInvocationResult{}, err
		}
		return confirmationInvocationResult(request, info)
	}
	outcome, err := registration.handler(ctx, CapabilityContext{
		RequestID: request.RequestID, Runtime: r, Document: info, Adapter: adapter,
		Operator: request.Operator, View: cloneViewContext(request.View),
	}, append(json.RawMessage(nil), request.Arguments...))
	if err != nil {
		return CapabilityInvocationResult{}, err
	}
	if outcome.Changed {
		return CapabilityInvocationResult{}, ErrNonCommandCapabilityChangedDocument
	}
	if len(outcome.Data) == 0 {
		outcome.Data = json.RawMessage(`{}`)
	}
	if !json.Valid(outcome.Data) || validateArguments(registration.descriptor.OutputSchema, outcome.Data) != nil {
		return CapabilityInvocationResult{}, ErrInvalidCommandResult
	}
	revision := info.Revision
	if request.DocumentID != "" {
		if current, err := r.GetDocument(ctx, request.DocumentID); err == nil {
			revision = current.Revision
		}
	}
	return CapabilityInvocationResult{
		RequestID: request.RequestID, DocumentID: request.DocumentID,
		Revision: revision, Data: append(json.RawMessage(nil), outcome.Data...),
	}, nil
}

func schemaForDocumentInfo() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"document_id":{"type":"string"},"format":{"type":"string"},"source_path":{"type":"string"},"revision":{"type":"integer"},"dirty":{"type":"boolean"},"read_only":{"type":"boolean"},"supported_operations":{"type":"array","items":{"type":"string"}}}}`)
}

func schemaForInvocationResult(dataSchema json.RawMessage) json.RawMessage {
	if len(dataSchema) == 0 {
		dataSchema = json.RawMessage(`{}`)
	}
	return append(json.RawMessage(`{"type":"object","properties":{"request_id":{"type":"string"},"document_id":{"type":"string"},"revision":{"type":"integer"},"data":`), append(dataSchema, []byte(`},"required":["request_id","data"]}`)...)...)
}
