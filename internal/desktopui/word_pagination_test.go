package desktopui

import (
	"context"
	"strings"
	"testing"
	"time"

	"glance/internal/format/docx"
	"glance/internal/wordlayout"
	"glance/internal/workspace"
)

func TestWordPageForCaretUsesRuneSourceMap(t *testing.T) {
	result := wordlayout.Result{Pages: []wordlayout.PageScene{{Index: 0}, {Index: 1}}, Locations: map[wordlayout.SourcePosition]wordlayout.Position{
		{RunID: "run-1", RuneOffset: 0}: {PageIndex: 0, XPT: 20, YPT: 40},
		{RunID: "run-1", RuneOffset: 2}: {PageIndex: 1, XPT: 25, YPT: 35},
	}}
	page, ok := wordPageForCaret(result, "run-1", 2)
	if !ok || page != 1 {
		t.Fatalf("page for caret = %d, %v; want zero-based page 1", page, ok)
	}
	if _, ok := wordPageForCaret(result, "missing", 0); ok {
		t.Fatal("unknown run unexpectedly mapped to a page")
	}
}

func TestWordStatusShowsLayoutErrorInsteadOfEstimatedPageCount(t *testing.T) {
	state := newDocumentSessionState("")
	state.wordLayoutError = "unsupported-columns"
	got := wordPaginationStatusText(state)
	if strings.Contains(got, "第") || strings.Contains(got, "共") || got == "" {
		t.Fatalf("status = %q; want an explicit error state without a guessed page count", got)
	}
	state.wordLayoutError = ""
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: "doc-1", Revision: 3, PageSetupKey: "a4"}
	if got = wordPaginationStatusText(state); !strings.Contains(got, "计算") || strings.Contains(got, "共") {
		t.Fatalf("pending status = %q; want calculating with no estimated page count", got)
	}
}

func TestWordStatusHidesAcceptedCountWhileTextDraftIsPending(t *testing.T) {
	state := newDocumentSessionState("")
	state.wordLayoutResult = &wordlayout.Result{Pages: []wordlayout.PageScene{{Index: 0}, {Index: 1}}, CanPreview: true}
	state.wordRunCommitted["run-1"] = "old"
	state.wordRunDrafts["run-1"] = "new text that may repaginate"
	if got := wordPaginationStatusText(state); !strings.Contains(got, "更新") || strings.Contains(got, "共 2 页") {
		t.Fatalf("status = %q; want pending status instead of stale accepted page count", got)
	}
}

type blockingWordLayoutEngine struct {
	started chan wordlayout.Input
	release chan struct{}
}

func (e *blockingWordLayoutEngine) Layout(ctx context.Context, input wordlayout.Input) (wordlayout.Result, error) {
	e.started <- input
	select {
	case <-ctx.Done():
		return wordlayout.Result{}, ctx.Err()
	case <-e.release:
		return wordlayout.Result{
			DocumentID: input.DocumentID, Revision: input.Revision,
			Pages: []wordlayout.PageScene{{Index: 0}}, CanPreview: true,
		}, nil
	}
}

func TestWordOpenStartsBackgroundPagination(t *testing.T) {
	engine := &blockingWordLayoutEngine{started: make(chan wordlayout.Input, 1), release: make(chan struct{})}
	app := &App{
		documentSessionState: newDocumentSessionState(""),
		sessions:             make(map[string]*documentSessionState),
		tabMeta:              make(map[string]workspace.Tab),
		wordLayoutResults:    make(chan wordLayoutResult, 1),
		wordLayoutEngine:     engine,
		shutdown:             make(chan struct{}),
	}
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	installed := make(chan error, 1)
	go func() { installed <- app.installDocument(loadedDocument{word: word, wordBlocks: word.WordBlocks()}) }()
	select {
	case err := <-installed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("installDocument blocked on pagination")
	}
	select {
	case input := <-engine.started:
		if input.DocumentID != app.activeTabID || input.Revision == 0 {
			t.Fatalf("background layout input key = %q/%d", input.DocumentID, input.Revision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("opening a DOCX did not start background pagination")
	}
	close(engine.release)
}

func TestWordPaginationIgnoresResultForSupersededRevision(t *testing.T) {
	state := newDocumentSessionState("")
	state.documentID = "doc-1"
	state.wordRevision = 4
	key := wordLayoutKey{DocumentID: "doc-1", Revision: 4, PageSetupKey: "a4"}
	state.wordLayoutPendingKey = &key
	app := &App{documentSessionState: state}
	app.applyWordLayoutResult(wordLayoutResult{
		state: state,
		key:   wordLayoutKey{DocumentID: "doc-1", Revision: 3, PageSetupKey: "a4"},
		result: wordlayout.Result{
			DocumentID: "doc-1", Revision: 3, Pages: []wordlayout.PageScene{{Index: 0}}, CanPreview: true,
		},
	})
	if state.wordLayoutResult != nil || state.wordLayoutPendingKey == nil || *state.wordLayoutPendingKey != key {
		t.Fatalf("stale result changed pagination state: result=%+v pending=%+v", state.wordLayoutResult, state.wordLayoutPendingKey)
	}
}

func TestWordPaginationSurfacesKnownApproximationAfterOpening(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("support-report.docx")
	state.word = word
	state.setWordBlocks(word.WordBlocks())
	data, ok := wordLayoutDataForState(state)
	if !ok {
		t.Fatal("blank DOCX does not expose layout data")
	}
	key := wordLayoutKey{
		DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load(), DraftEpoch: state.wordDraftEpoch.Load(),
		PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup),
	}
	state.wordLayoutPendingKey = &key
	app := &App{documentSessionState: state}
	app.applyWordLayoutResult(wordLayoutResult{
		state: state, key: key,
		result: wordlayout.Result{
			DocumentID: state.documentID, Revision: key.Revision,
			CanPreview: true, Pages: []wordlayout.PageScene{{Index: 0}},
			SupportReport: wordlayout.SupportReport{Findings: []wordlayout.SupportFinding{{
				SourceID: "b4.p", Code: "unsupported-list", Detail: "Unsupported list format.",
				Impact: wordlayout.SupportImpactApproximation, Fallback: "The marker is omitted.",
			}}},
		},
	})
	if !strings.Contains(app.status, "版式检查发现") || !strings.Contains(app.status, "需确认") {
		t.Fatalf("opening status = %q; want a visible notice that known layout gaps need confirmation", app.status)
	}
}

func TestWordPaginationPreservesVisiblePageWhenAcceptingReflow(t *testing.T) {
	tests := []struct {
		name          string
		pageCount     int
		firstRow      int
		multiplePages bool
		wantRow       int
		wantPage      int
	}{
		{name: "single-page row", pageCount: 8, firstRow: 3, wantRow: 3, wantPage: 3},
		{name: "two-page spread", pageCount: 8, firstRow: 2, multiplePages: true, wantRow: 2, wantPage: 4},
		{name: "clamp after page count shrinks", pageCount: 3, firstRow: 4, multiplePages: true, wantRow: 1, wantPage: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer word.Close()
			state := newDocumentSessionState("visible-page.docx")
			state.word = word
			state.wordViewMode = wordViewPrintLayout
			state.wordMultiplePagesMode = test.multiplePages
			state.wordPageListState.Position.First = test.firstRow
			data, ok := wordLayoutDataForState(state)
			if !ok {
				t.Fatal("blank DOCX does not expose layout data")
			}
			key := wordLayoutKey{
				DocumentID:   state.documentID,
				Revision:     state.wordRevisionAtomic.Load(),
				DraftEpoch:   state.wordDraftEpoch.Load(),
				PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup),
			}
			state.wordLayoutPendingKey = &key
			app := &App{documentSessionState: state}

			app.applyWordLayoutResult(wordLayoutResult{
				state: state,
				key:   key,
				result: wordlayout.Result{
					DocumentID: state.documentID, Revision: key.Revision,
					Pages: make([]wordlayout.PageScene, test.pageCount), CanPreview: true,
				},
			})

			if state.wordPageListState.Position.First != test.wantRow || state.wordLayoutActivePage != test.wantPage {
				t.Fatalf("accepted layout reset visible position: row=%d active page=%d; want row=%d page=%d", state.wordPageListState.Position.First, state.wordLayoutActivePage, test.wantRow, test.wantPage)
			}
		})
	}
}

func TestWordPrintLayoutToggle(t *testing.T) {
	state := newDocumentSessionState("")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	state.wordViewMode = wordViewContinuous
	app := &App{documentSessionState: state}
	app.toggleWordPrintLayout()
	if state.wordViewMode != wordViewPrintLayout {
		t.Fatalf("view mode = %v; want print layout", state.wordViewMode)
	}
	app.toggleWordPrintLayout()
	if state.wordViewMode != wordViewContinuous {
		t.Fatalf("view mode = %v; want continuous flow", state.wordViewMode)
	}
}
