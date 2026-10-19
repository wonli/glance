package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"glance/internal/agent/provider"
	"glance/internal/application"
)

func lazyToolsClient(t *testing.T, onList func(context.Context) error) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "test.query", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				if err := onList(ctx); err != nil {
					return nil, err
				}
			}
			return next(ctx, method, req)
		}
	})
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		ss.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return cs
}

func TestLazyToolsLoadOnFirstTurnAndRetryFailure(t *testing.T) {
	var calls atomic.Int32
	session := lazyToolsClient(t, func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("temporary list failure")
		}
		return nil
	})
	model := &scriptedProvider{t: t, steps: []func(provider.ModelRequest) []provider.ModelEvent{func(r provider.ModelRequest) []provider.ModelEvent {
		if len(r.Tools) != 1 || r.Tools[0].Function.Name != "test.query" {
			t.Errorf("missing lazy catalog: %+v", r.Tools)
		}
		return []provider.ModelEvent{{TextDelta: "ok"}, {Done: true}}
	}}}
	host, err := NewHost(context.Background(), model, session, HostOptions{LazyTools: true})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if calls.Load() != 0 {
		t.Fatal("constructed host eagerly listed tools")
	}
	if err := host.Send(context.Background(), "one", "first", application.ViewContext{}); err == nil {
		t.Fatal("list failure was lost")
	}
	if err := host.Send(context.Background(), "one", "retry", application.ViewContext{}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("list calls=%d", calls.Load())
	}
}

func TestLazyToolsCloseCancelsPendingLoad(t *testing.T) {
	entered := make(chan struct{})
	session := lazyToolsClient(t, func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() })
	host, err := NewHost(context.Background(), &scriptedProvider{t: t}, session, HostOptions{LazyTools: true})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- host.Send(context.Background(), "one", "hello", application.ViewContext{}) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("list not started")
	}
	closed := make(chan struct{})
	go func() { host.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not cancel lazy load")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled turn succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Send stuck")
	}
}

type waitingToolContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *waitingToolContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestLazyToolsConcurrentWaiterCancelsIndependently(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	session := lazyToolsClient(t, func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	host, err := NewHost(context.Background(), &scriptedProvider{t: t}, session, HostOptions{LazyTools: true})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := make(chan error, 1)
	go func() { first <- host.ensureTools(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first list did not start")
	}
	waiting, stop := context.WithCancel(context.Background())
	observed := &waitingToolContext{Context: waiting, waiting: make(chan struct{})}
	second := make(chan error, 1)
	go func() { second <- host.ensureTools(observed) }()
	select {
	case <-observed.waiting:
	case <-ctx.Done():
		t.Fatal("second call did not enter pending-load wait")
	}
	stop()
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter cancellation blocked on first load")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := host.ensureTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("successful catalog was loaded %d times", calls.Load())
	}
}
