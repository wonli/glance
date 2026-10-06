package desktopui

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op"
	"glance/internal/document"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMarkdownParsePublishesTextBeforeImagesAndCancelsStaleFetch(t *testing.T) {
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer server.Close()
	a := markdownTestApp(t)
	v := a.markdownView
	defer func() {
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}()
	source := fmt.Sprintf("# Immediate text\n\n![slow](%s/image.png)", server.URL)
	v.editor.SetText(source)
	var ops op.Ops
	a.handleMarkdownEvents(layout.Context{Ops: &ops})
	select {
	case result := <-v.results:
		if !result.resourcesPending || !v.accept(result) || len(v.snapshot.Headings) != 1 {
			t.Fatal("text was not published before resources")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("text parsing waited for network")
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("image request did not start")
	}
	v.editor.SetText("# New text")
	ops.Reset()
	a.handleMarkdownEvents(layout.Context{Ops: &ops})
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("stale image request was not canceled")
	}
	deadline := time.After(3 * time.Second)
	for v.pending {
		select {
		case result := <-v.results:
			v.accept(result)
		case <-deadline:
			t.Fatal("new parse did not finish")
		}
	}
	if v.snapshot.Source != "# New text" {
		t.Fatal("stale parse replaced current text")
	}
}
func TestMarkdownStaleGenerationCannotFinishNewResourceJob(t *testing.T) {
	v := newMarkdownView("same")
	v.requested = "same"
	v.parseGeneration = 2
	v.pending = true
	if v.accept(markdownParseResult{source: "same", generation: 1, snapshot: document.MarkdownSnapshot{Source: "same"}}) || !v.pending {
		t.Fatal("old generation changed pending state")
	}
}

func TestMarkdownDepartureCancelsResourceJobAndAllowsResume(t *testing.T) {
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer server.Close()
	a := markdownTestApp(t)
	v := a.markdownView
	defer func() {
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}()
	v.editor.SetText(fmt.Sprintf("![slow](%s/image.png)", server.URL))
	a.flushMarkdownDraft()
	a.requestMarkdownParse()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("fetch did not start")
	}
	// Leave the first result unconsumed: resource publication must still terminate.
	a.prepareMarkdownDeparture()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("departure did not cancel fetch")
	}
	if v.pending || v.images != nil {
		t.Fatal("departure left incomplete resources marked current")
	}
	<-v.results
	a.requestMarkdownParse()
	if !v.pending {
		t.Fatal("return did not restart canceled parse")
	}
}
