package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	EventMessageReceived       = "message.received"
	EventDocumentChanged       = "document.changed"
	EventDocumentSaved         = "document.saved"
	EventDocumentOpened        = "document.opened"
	EventDocumentClosed        = "document.closed"
	EventOperationFailed       = "operation.failed"
	EventConfirmationRequested = "confirmation.requested"
	EventConfirmationUpdated   = "confirmation.updated"
	MaxIncomingMessageBytes    = 1 << 20
)

type Event struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	Source     OperatorKind    `json:"source"`
	DocumentID string          `json:"document_id,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	CommandID  string          `json:"command_id,omitempty"`
	Revision   uint64          `json:"revision,omitempty"`
	Name       string          `json:"name,omitempty"`
	View       ViewContext     `json:"view,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type EventFilter struct {
	Types      []string
	DocumentID string
	Source     OperatorKind
}

// IncomingMessage is transport-neutral input. Runtime.Emit always wraps it as
// message.received; callers cannot choose a domain event type or revision.
type IncomingMessage struct {
	Source     OperatorKind    `json:"source"`
	Name       string          `json:"name"`
	DocumentID string          `json:"document_id,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	View       ViewContext     `json:"view,omitempty"`
	Data       json.RawMessage `json:"data"`
}

type eventSubscriber struct {
	filter   EventFilter
	handler  func(Event)
	stop     context.CancelFunc
	canceled atomic.Bool
	done     chan struct{}
}

type EventBus struct {
	mu          sync.Mutex
	closed      bool
	closeDone   chan struct{}
	nextID      uint64
	eventNumber atomic.Uint64
	subscribers map[uint64]*eventSubscriber
	watchers    sync.WaitGroup
}

func newEventBus() *EventBus {
	return &EventBus{subscribers: make(map[uint64]*eventSubscriber), closeDone: make(chan struct{})}
}

func (b *EventBus) subscribe(ctx context.Context, filter EventFilter, handler func(Event)) func() {
	if b == nil || handler == nil {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return func() {}
	}
	ctx, stop := context.WithCancel(ctx)
	subscriber := &eventSubscriber{filter: cloneEventFilter(filter), handler: handler, stop: stop, done: make(chan struct{})}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		stop()
		return func() {}
	}
	b.nextID++
	id := b.nextID
	b.subscribers[id] = subscriber
	b.watchers.Add(1)
	b.mu.Unlock()
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			subscriber.canceled.Store(true)
			stop()
			b.mu.Lock()
			delete(b.subscribers, id)
			b.mu.Unlock()
		})
	}
	go func() {
		defer b.watchers.Done()
		defer close(subscriber.done)
		<-ctx.Done()
		cleanup()
	}()
	return func() {
		cleanup()
		<-subscriber.done
	}
}

func (b *EventBus) publish(event Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	subscribers := make([]*eventSubscriber, 0, len(b.subscribers))
	for _, subscriber := range b.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	b.mu.Unlock()
	for _, subscriber := range subscribers {
		if subscriber.canceled.Load() || !eventMatches(subscriber.filter, event) {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			subscriber.handler(cloneEvent(event))
		}()
	}
}

func (b *EventBus) newEventID() string {
	return fmt.Sprintf("event-%d", b.eventNumber.Add(1))
}

func (b *EventBus) close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		done := b.closeDone
		b.mu.Unlock()
		if done != nil {
			<-done
		}
		return
	}
	if b.closeDone == nil {
		b.closeDone = make(chan struct{})
	}
	b.closed = true
	subscribers := b.subscribers
	b.subscribers = make(map[uint64]*eventSubscriber)
	b.mu.Unlock()
	for _, subscriber := range subscribers {
		subscriber.canceled.Store(true)
		subscriber.stop()
	}
	b.watchers.Wait()
	close(b.closeDone)
}

func cloneEventFilter(filter EventFilter) EventFilter {
	filter.Types = append([]string(nil), filter.Types...)
	return filter
}

func cloneEvent(event Event) Event {
	event.Data = append(json.RawMessage(nil), event.Data...)
	if event.View.Selection != nil {
		selection := *event.View.Selection
		if selection.Range != nil {
			rangeValue := *selection.Range
			selection.Range = &rangeValue
		}
		event.View.Selection = &selection
	}
	return event
}

func eventMatches(filter EventFilter, event Event) bool {
	if filter.DocumentID != "" && filter.DocumentID != event.DocumentID {
		return false
	}
	if filter.Source != "" && filter.Source != event.Source {
		return false
	}
	if len(filter.Types) > 0 {
		for _, eventType := range filter.Types {
			if eventType == event.Type {
				return true
			}
		}
		return false
	}
	return true
}

func (r *Runtime) On(ctx context.Context, filter EventFilter, handler func(Event)) func() {
	if r == nil || r.events == nil {
		return func() {}
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return func() {}
	}
	return r.events.subscribe(ctx, filter, handler)
}

func (r *Runtime) publishDomainEvent(event Event) {
	if r == nil || r.events == nil {
		return
	}
	if event.EventID == "" {
		event.EventID = r.events.newEventID()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	r.events.publish(event)
}

func (r *Runtime) publishDocumentEvent(eventType string, info DocumentInfo) {
	event := documentEvent(eventType, info)
	if event != nil {
		r.publishDomainEvent(*event)
	}
}

func documentEvent(eventType string, info DocumentInfo) *Event {
	data, err := json.Marshal(info)
	if err != nil {
		return nil
	}
	return &Event{
		Type: eventType, Source: OperatorSystem, DocumentID: info.DocumentID,
		Revision: info.Revision, Data: data,
	}
}

func (r *Runtime) Emit(ctx context.Context, message IncomingMessage) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || r.events == nil {
		return ErrEventBusClosed
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return ErrEventBusClosed
	}
	if message.Name == "" || !validOperatorKind(message.Source) {
		return ErrInvalidIncomingMessage
	}
	if len(message.Data) == 0 {
		message.Data = json.RawMessage(`{}`)
	}
	if !json.Valid(message.Data) || len(message.Data) > MaxIncomingMessageBytes {
		return ErrInvalidIncomingMessage
	}
	if message.View.DocumentID != "" && message.DocumentID != "" && message.View.DocumentID != message.DocumentID {
		return ErrInvalidIncomingMessage
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	event := Event{
		EventID: r.events.newEventID(), Type: EventMessageReceived, Source: message.Source,
		DocumentID: message.DocumentID, RequestID: message.RequestID, Name: message.Name,
		View: message.View, Data: append(json.RawMessage(nil), message.Data...), OccurredAt: time.Now().UTC(),
	}
	r.events.publish(event)
	return nil
}

func validOperatorKind(kind OperatorKind) bool {
	return kind == OperatorHuman || kind == OperatorBuiltInAgent || kind == OperatorExternalAgent || kind == OperatorSystem
}
