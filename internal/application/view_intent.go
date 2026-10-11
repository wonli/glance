package application

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrInvalidViewIntent = errors.New("view intent is invalid")

// ViewIntent is a serializable request to update window-owned presentation
// state. It never carries Gio widgets or layout contexts.
type ViewIntent struct {
	WindowID   uint64          `json:"window_id"`
	DocumentID string          `json:"document_id"`
	Revision   uint64          `json:"revision"`
	Action     string          `json:"action"`
	Arguments  json.RawMessage `json:"arguments"`
}

// ViewIntentHandler applies a view request on the owner of a live desktop
// window. Implementations must marshal work onto their UI event loop.
type ViewIntentHandler interface {
	HandleViewIntent(context.Context, ViewIntent) (json.RawMessage, error)
}

type viewIntentRegistration struct {
	id      uint64
	handler ViewIntentHandler
}

// RegisterViewIntentHandler attaches one live window to the Runtime. The
// returned function unregisters only this registration, so an old window
// cannot detach a newer window that reused its ID.
func (r *Runtime) RegisterViewIntentHandler(windowID uint64, handler ViewIntentHandler) func() {
	if r == nil || windowID == 0 || handler == nil {
		return func() {}
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return func() {}
	}
	r.nextViewHandler++
	registration := viewIntentRegistration{id: r.nextViewHandler, handler: handler}
	if r.viewHandlers == nil {
		r.viewHandlers = make(map[uint64]viewIntentRegistration)
	}
	r.viewHandlers[windowID] = registration
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		if current, ok := r.viewHandlers[windowID]; ok && current.id == registration.id {
			delete(r.viewHandlers, windowID)
		}
		r.mu.Unlock()
	}
}

// ApplyViewIntent routes a request only to the explicitly attached window.
func (r *Runtime) ApplyViewIntent(ctx context.Context, intent ViewIntent) (json.RawMessage, error) {
	if r == nil {
		return nil, ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if intent.WindowID == 0 || intent.DocumentID == "" || intent.Revision == 0 || intent.Action == "" || !json.Valid(intent.Arguments) {
		return nil, ErrInvalidViewIntent
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrRuntimeClosed
	}
	registration, ok := r.viewHandlers[intent.WindowID]
	r.mu.Unlock()
	if !ok || registration.handler == nil {
		return nil, ErrNoAttachedView
	}
	result, err := registration.handler.HandleViewIntent(ctx, intent)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(result) {
		return nil, ErrInvalidCommandResult
	}
	return append(json.RawMessage(nil), result...), nil
}
