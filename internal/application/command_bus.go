package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const maxIdempotencyEntries = 4096

var ErrDocumentProtected = errors.New("DOCX document is protected against Agent edits")

type CommandRequest struct {
	RequestID        string          `json:"request_id"`
	IdempotencyKey   string          `json:"idempotency_key,omitempty"`
	DocumentID       string          `json:"document_id"`
	CommandID        string          `json:"command_id"`
	ExpectedRevision uint64          `json:"expected_revision,omitempty"`
	Operator         Operator        `json:"operator"`
	View             ViewContext     `json:"view,omitempty"`
	Arguments        json.RawMessage `json:"arguments"`
}

type CommandResult struct {
	RequestID  string          `json:"request_id"`
	DocumentID string          `json:"document_id"`
	Revision   uint64          `json:"revision"`
	Data       json.RawMessage `json:"data"`
}

type commandIdempotencyEntry struct {
	payloadHash string
	done        chan struct{}
	result      CommandResult
	err         error
}

type CommandBus struct {
	runtime *Runtime
	catalog *Catalog
	events  *EventBus
	mu      sync.Mutex
	entries map[string]*commandIdempotencyEntry
	order   []string
}

func newCommandBus(runtime *Runtime, catalog *Catalog, events *EventBus) *CommandBus {
	return &CommandBus{runtime: runtime, catalog: catalog, events: events, entries: make(map[string]*commandIdempotencyEntry)}
}

func (b *CommandBus) Execute(ctx context.Context, request CommandRequest) (CommandResult, error) {
	return b.execute(ctx, request, false)
}

// executeApproved is reachable only through the Runtime's trusted in-process
// confirmation flow. MCP tools never receive a flag that bypasses approval.
func (b *CommandBus) executeApproved(ctx context.Context, request CommandRequest) (CommandResult, error) {
	return b.execute(ctx, request, true)
}

func (b *CommandBus) execute(ctx context.Context, request CommandRequest, humanApproved bool) (CommandResult, error) {
	if b == nil || b.runtime == nil || b.catalog == nil {
		return CommandResult{}, ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	if strings.TrimSpace(request.RequestID) == "" || strings.TrimSpace(request.DocumentID) == "" || strings.TrimSpace(request.CommandID) == "" || strings.TrimSpace(request.Operator.ID) == "" || !validOperatorKind(request.Operator.Kind) {
		return CommandResult{}, ErrInvalidCapability
	}
	if request.ExpectedRevision == 0 {
		return CommandResult{}, ErrExpectedRevisionRequired
	}
	registration, ok := b.catalog.lookup(request.CommandID)
	if !ok {
		return CommandResult{}, fmt.Errorf("%w: %s", ErrCapabilityNotFound, request.CommandID)
	}
	if registration.descriptor.Category != CapabilityCommand {
		return CommandResult{}, ErrCapabilityNotCommand
	}
	arguments, err := canonicalJSON(request.Arguments)
	if err != nil {
		return CommandResult{}, fmt.Errorf("%w: %v", ErrInvalidCommandArguments, err)
	}
	if err := validateArguments(registration.descriptor.InputSchema, arguments); err != nil {
		return CommandResult{}, err
	}
	request.Arguments = arguments
	if request.View.DocumentID != "" && request.View.DocumentID != request.DocumentID {
		return CommandResult{}, ErrViewContextMismatch
	}
	if request.View.Selection != nil && request.View.Selection.TargetType == "" {
		return CommandResult{}, ErrViewContextMismatch
	}
	if request.IdempotencyKey == "" {
		result, event, executeErr := b.executeOnce(ctx, request, registration, humanApproved)
		b.publish(event)
		return result, executeErr
	}
	hash, err := commandRequestHash(request)
	if err != nil {
		return CommandResult{}, err
	}
	entry, owner, err := b.beginIdempotency(request.IdempotencyKey, hash)
	if err != nil {
		return CommandResult{}, err
	}
	if !owner {
		select {
		case <-ctx.Done():
			return CommandResult{}, ctx.Err()
		case <-entry.done:
			return cloneCommandResult(entry.result), entry.err
		}
	}
	result, event, executeErr := b.executeOnce(ctx, request, registration, humanApproved)
	b.finishIdempotency(entry, result, executeErr)
	b.publish(event)
	return result, executeErr
}

func (b *CommandBus) executeOnce(ctx context.Context, request CommandRequest, registration registeredCapability, humanApproved bool) (CommandResult, *Event, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, nil, err
	}
	descriptor := registration.descriptor
	if !humanApproved && (descriptor.Risk == RiskHigh || descriptor.Risk == RiskCritical) {
		return CommandResult{}, nil, ErrHumanConfirmationRequired
	}
	session, err := b.runtime.session(request.DocumentID)
	if err != nil {
		return CommandResult{}, nil, err
	}
	session.mu.Lock()
	if err := ctx.Err(); err != nil {
		session.mu.Unlock()
		return CommandResult{}, nil, err
	}
	if session.closed {
		session.mu.Unlock()
		return CommandResult{}, nil, ErrDocumentNotFound
	}
	info := session.infoLocked()
	if request.ExpectedRevision != 0 && request.ExpectedRevision != info.Revision || request.View.Revision != 0 && request.View.Revision != info.Revision {
		session.mu.Unlock()
		return CommandResult{}, nil, fmt.Errorf("%w: current=%d expected=%d", ErrRevisionConflict, info.Revision, request.ExpectedRevision)
	}
	if len(descriptor.Formats) > 0 && !containsFormat(descriptor.Formats, session.format) {
		session.mu.Unlock()
		return CommandResult{}, nil, fmt.Errorf("%w: %s does not support %s", ErrCapabilityUnsupported, descriptor.ID, session.format)
	}
	if session.adapter.ReadOnly() {
		session.mu.Unlock()
		return CommandResult{}, nil, ErrReadOnlyDocument
	}
	if session.format == FormatDocx && request.CommandID != "word.document.protection.set" && request.CommandID != "document.export_copy" {
		if reader, ok := session.adapter.(WordProtectionReader); ok {
			protected, protectionErr := reader.GetWordDocumentProtection(ctx)
			if protectionErr != nil {
				session.mu.Unlock()
				return CommandResult{}, nil, protectionErr
			}
			if protected {
				session.mu.Unlock()
				return CommandResult{}, nil, ErrDocumentProtected
			}
		}
	}
	outcome, err := registration.handler(ctx, CapabilityContext{
		RequestID: request.RequestID, Runtime: b.runtime, Document: info, Adapter: session.adapter,
		Operator: request.Operator, View: cloneViewContext(request.View),
	}, append(json.RawMessage(nil), request.Arguments...))
	if err != nil {
		session.mu.Unlock()
		return CommandResult{}, nil, err
	}
	if len(outcome.Data) == 0 {
		outcome.Data = json.RawMessage(`{}`)
	}
	if !json.Valid(outcome.Data) {
		session.mu.Unlock()
		return CommandResult{}, nil, ErrInvalidCommandResult
	}
	if err := validateArguments(descriptor.OutputSchema, outcome.Data); err != nil {
		session.mu.Unlock()
		return CommandResult{}, nil, ErrInvalidCommandResult
	}
	if outcome.Changed {
		session.revision++
	}
	committedRevision := session.revision
	observer, observesRevision := session.adapter.(CommandRevisionObserver)
	result := CommandResult{
		RequestID: request.RequestID, DocumentID: session.id,
		Revision: session.revision, Data: append(json.RawMessage(nil), outcome.Data...),
	}
	event := Event{
		Type: EventDocumentChanged, Source: request.Operator.Kind,
		DocumentID: session.id, RequestID: request.RequestID, CommandID: descriptor.ID,
		Revision: session.revision, Data: append(json.RawMessage(nil), outcome.Data...),
		View: cloneViewContext(request.View), OccurredAt: time.Now().UTC(),
	}
	if b.events != nil {
		event.EventID = b.events.newEventID()
	}
	session.mu.Unlock()
	if !outcome.Changed {
		return result, nil, nil
	}
	if observesRevision {
		observer.CommandRevisionChanged(committedRevision)
	}
	return result, &event, nil
}

func (b *CommandBus) publish(event *Event) {
	if event != nil && b.events != nil {
		b.events.publish(*event)
	}
}

func (b *CommandBus) beginIdempotency(key, hash string) (*commandIdempotencyEntry, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if entry := b.entries[key]; entry != nil {
		if entry.payloadHash != hash {
			return nil, false, ErrIdempotencyConflict
		}
		return entry, false, nil
	}
	for len(b.entries) >= maxIdempotencyEntries {
		removed := false
		for index, oldestKey := range b.order {
			oldest := b.entries[oldestKey]
			if oldest == nil {
				b.order = append(b.order[:index], b.order[index+1:]...)
				removed = true
				break
			}
			select {
			case <-oldest.done:
				delete(b.entries, oldestKey)
				b.order = append(b.order[:index], b.order[index+1:]...)
				removed = true
			default:
			}
			if removed {
				break
			}
		}
		if !removed {
			return nil, false, ErrIdempotencyStoreFull
		}
	}
	entry := &commandIdempotencyEntry{payloadHash: hash, done: make(chan struct{})}
	b.entries[key] = entry
	b.order = append(b.order, key)
	return entry, true, nil
}

func (b *CommandBus) finishIdempotency(entry *commandIdempotencyEntry, result CommandResult, err error) {
	b.mu.Lock()
	entry.result = cloneCommandResult(result)
	entry.err = err
	close(entry.done)
	b.mu.Unlock()
}

func commandRequestHash(request CommandRequest) (string, error) {
	payload, err := json.Marshal(struct {
		DocumentID       string          `json:"document_id"`
		CommandID        string          `json:"command_id"`
		ExpectedRevision uint64          `json:"expected_revision"`
		Operator         Operator        `json:"operator"`
		View             ViewContext     `json:"view"`
		Arguments        json.RawMessage `json:"arguments"`
	}{request.DocumentID, request.CommandID, request.ExpectedRevision, request.Operator, request.View, request.Arguments})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	canonical, err := json.Marshal(value)
	return canonical, err
}

func cloneCommandResult(result CommandResult) CommandResult {
	result.Data = append(json.RawMessage(nil), result.Data...)
	return result
}

func cloneViewContext(view ViewContext) ViewContext {
	if view.Selection != nil {
		selection := *view.Selection
		if selection.Range != nil {
			rangeValue := *selection.Range
			selection.Range = &rangeValue
		}
		view.Selection = &selection
	}
	return view
}

func containsFormat(formats []Format, target Format) bool {
	for _, format := range formats {
		if format == target {
			return true
		}
	}
	return false
}
