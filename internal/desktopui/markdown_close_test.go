package desktopui

import (
	"context"
	"testing"
	"time"
)

type stalledMarkdownCloseSession struct {
	markdownChartTestSession
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (s *stalledMarkdownCloseSession) Close() error {
	close(s.started)
	<-s.release
	close(s.finished)
	return nil
}

func TestMarkdownViewReleaseDoesNotWaitForNativeClose(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	ctx, cancel := context.WithCancel(context.Background())
	session := &stalledMarkdownCloseSession{started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	defer close(session.release)
	state := &markdownChartState{session: session, ctx: ctx, cancel: cancel}
	v.charts = map[string]*markdownChartState{"chart": state}
	returned := make(chan struct{})
	go func() { a.releaseMarkdownViewResources(); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("closing Markdown blocked on native WebView teardown")
	}
	select {
	case <-session.started:
	case <-time.After(time.Second):
		t.Fatal("detached session was not scheduled for cleanup")
	}
	if a.markdownView != nil || ctx.Err() == nil {
		t.Fatal("closing Markdown did not detach its view and cancel work immediately")
	}
	state.mu.Lock()
	closed, detached := state.closed, state.session == nil
	state.mu.Unlock()
	if !closed || !detached {
		t.Fatal("session still attached to closed Markdown")
	}
}
