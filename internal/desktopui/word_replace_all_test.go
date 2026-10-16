package desktopui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordReplaceTabShowsReplaceAll(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("word-replace-all.docx")
	state.word = word
	a := &App{documentSessionState: state}

	a.showWordReplaceDialog()
	if !a.modal.Visible || !a.modal.SearchReplaceReplaceTab {
		t.Fatal("Ctrl+H did not open the Word Replace tab")
	}
	for _, action := range a.modal.Actions {
		if action.ID == "replace-all" {
			return
		}
	}
	t.Fatal("Word Replace tab does not provide Replace All")
}

func TestPlanWordTextReplaceAllIsCaseInsensitiveAcrossEditableRuns(t *testing.T) {
	paragraph := document.WordParagraph{Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "editable", Text: "Cat cat CAT", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "readonly", Text: "cat", Editable: false}},
	}}
	plan := planWordTextReplaceAll([]document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &paragraph}}, nil, "cat", "豆")
	if plan.matches != 3 || plan.truncated || len(plan.replacements) != 1 || plan.replacements[0].Replacement != "豆 豆 豆" {
		t.Fatalf("case-insensitive editable-run plan = %+v", plan)
	}

	paragraph.Inlines = []document.WordInline{{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: "many", Text: strings.Repeat("x", wordReplaceAllMaxMatches+1), Editable: true},
	}}
	plan = planWordTextReplaceAll([]document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &paragraph}}, nil, "x", "y")
	if !plan.truncated || plan.matches != wordReplaceAllMaxMatches+1 || len(plan.replacements) != 0 {
		t.Fatalf("over-limit plan = %+v; want no partial edit after %d matches", plan, wordReplaceAllMaxMatches)
	}
	limitParagraph := document.WordParagraph{ID: "limit", Inlines: []document.WordInline{{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: "limit-run", Text: "x", Editable: true},
	}}}
	limitChunk := wordSearchChunks([]document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &limitParagraph}}, nil)[0]
	limitMatches, _ := wordMatchesInChunk(limitChunk, "x", -1)
	if _, _, truncated := replaceWordTextMatchesInChunk(limitChunk, limitMatches, "long", 3); !truncated {
		t.Fatal("replacement plan exceeded its resulting-text byte limit without truncating")
	}
}

func TestPlanWordTextReplaceAllAcrossFormattedRunsKeepsRunFormatting(t *testing.T) {
	paragraph := document.WordParagraph{ID: "paragraph", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "bold", Text: "preBe", Editable: true, Style: document.WordTextStyle{Bold: true}}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "italic", Text: "taSuf", Editable: true, Style: document.WordTextStyle{Italic: true}}},
	}}
	plan := planWordTextReplaceAll([]document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &paragraph}}, nil, "beta", "X")
	if plan.matches != 1 || plan.truncated || len(plan.replacements) != 2 {
		t.Fatalf("cross-run Replace All plan = %+v; want one match updating both formatted runs", plan)
	}
	if plan.replacements[0].RunID != "bold" || plan.replacements[0].Replacement != "preX" || plan.replacements[1].RunID != "italic" || plan.replacements[1].Replacement != "Suf" {
		t.Fatalf("cross-run output = %#v; want replacement in first run and untouched suffix in second", plan.replacements)
	}
}

func TestPlanWordTextReplaceAllDoesNotCrossReadOnlyInlineOrParagraphBoundaries(t *testing.T) {
	first := document.WordParagraph{ID: "first", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "before-tab", Text: "Be", Editable: true}},
		{Kind: document.WordTabInline, Tab: &document.WordTab{ID: "tab"}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "after-tab", Text: "ta", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "readonly", Text: "ta", Editable: false}},
	}}
	second := document.WordParagraph{ID: "second", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "next-paragraph", Text: "ta", Editable: true}},
	}}
	blocks := []document.WordBlock{
		{Kind: document.WordParagraphBlock, Paragraph: &first},
		{Kind: document.WordParagraphBlock, Paragraph: &second},
	}
	plan := planWordTextReplaceAll(blocks, nil, "beta", "X")
	if plan.matches != 0 || len(plan.replacements) != 0 || plan.truncated {
		t.Fatalf("Replace All crossed a read-only or structural boundary: %+v", plan)
	}
}

func TestPlanWordTextReplaceAllStopsBeforeTooManyAffectedRuns(t *testing.T) {
	paragraph := document.WordParagraph{ID: "many-runs", Inlines: make([]document.WordInline, wordReplaceAllMaxMatches+2)}
	for index := range paragraph.Inlines {
		value := "a"
		if index%2 == 1 {
			value = "b"
		}
		paragraph.Inlines[index] = document.WordInline{
			Kind: document.WordTextInline,
			Text: &document.WordTextRun{ID: "run-" + strconv.Itoa(index), Text: value, Editable: true},
		}
	}
	plan := planWordTextReplaceAll([]document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &paragraph}}, nil, "ab", "x")
	if !plan.truncated || len(plan.replacements) != 0 {
		t.Fatalf("plan with too many affected runs: truncated=%v replacements=%d matches=%d; want a rejected batch without partial edits", plan.truncated, len(plan.replacements), plan.matches)
	}
}

func TestWordReplaceAllResultPreservesANewerDraft(t *testing.T) {
	state := newDocumentSessionState("word-replace-all-newer-draft.docx")
	state.captureWordTextDraft("run", "draft at action time")
	state.captureWordTextDraft("run", "newer user input")
	state.setCommittedWordTextAndClearExpectedDraft("run", "committed replacement", "draft at action time")
	if got, ok := state.wordDraft("run"); !ok || got != "newer user input" {
		t.Fatalf("newer draft after Replace All = %q, exists=%v; want preserved newer input", got, ok)
	}
	if got, ok := state.committedWordText("run"); !ok || got != "committed replacement" {
		t.Fatalf("committed run after Replace All = %q, exists=%v", got, ok)
	}
}

func TestWordReplaceAllUsesOneUndoableEditAndIncludesDraftText(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := any(word).(document.WordTextEditor).ReplaceTextRun(run.ID, "apple apple"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-replace-all-draft.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = word.WordBlocks()[0].Paragraph.ID
	draft := wordRunEditor(state, *word.WordBlocks()[0].Paragraph.Inlines[0].Text)
	state.captureWordTextDraft(run.ID, "draft apple apple")
	draft.SetText("draft apple apple")
	a := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 2)}

	a.showWordReplaceDialog()
	a.modal.Input.SetText("APPLE")
	a.modal.SecondInput.SetText("pear")
	a.modal.OnAction("replace-all")
	waitForWordReplaceAll(t, a)

	if got := word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "draft pear pear" {
		t.Fatalf("Replace All value = %q; want draft pear pear", got)
	}
	if got := a.modal.SearchReplaceSummary; got != "已替换 2 处" {
		t.Fatalf("Replace All summary = %q; want two replaced occurrences", got)
	}
	if got := draft.Text(); got != "draft pear pear" {
		t.Fatalf("Word run editor after Replace All = %q; want updated committed text", got)
	}
	if _, ok := state.wordDraft(run.ID); ok {
		t.Fatal("Replace All left the consumed source draft active")
	}
	if applied, err := any(word).(document.WordHistory).Undo(); err != nil || !applied {
		t.Fatalf("undo Replace All = %v, %v", applied, err)
	}
	if got := word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "apple apple" {
		t.Fatalf("one undo restored %q; want committed pre-replacement text", got)
	}
}

func TestWordReplaceAllReplacesAcrossAdjacentFormattedRunsInOneUndo(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>preBe</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>taSuf</w:t></w:r></w:p>`)
	app.showWordReplaceDialog()
	app.modal.Input.SetText("beta")
	app.modal.SecondInput.SetText("X")
	app.modal.OnAction("replace-all")
	waitForWordReplaceAll(t, app)
	blocks := state.word.WordBlocks()
	first, second := blocks[0].Paragraph.Inlines[0].Text, blocks[0].Paragraph.Inlines[1].Text
	if first.Text != "preX" || second.Text != "Suf" {
		t.Fatalf("cross-run Replace All left run text %q/%q; want preX/Suf", first.Text, second.Text)
	}
	if !first.Style.Bold || !second.Style.Italic {
		t.Fatal("cross-run Replace All lost formatting outside the matched text")
	}
	if applied, err := any(state.word).(document.WordHistory).Undo(); err != nil || !applied {
		t.Fatalf("undo cross-run Replace All = %v, %v", applied, err)
	}
	blocks = state.word.WordBlocks()
	if first, second := blocks[0].Paragraph.Inlines[0].Text.Text, blocks[0].Paragraph.Inlines[1].Text.Text; first != "preBe" || second != "taSuf" {
		t.Fatalf("undo restored run text %q/%q; want preBe/taSuf", first, second)
	}
}

func waitForWordReplaceAll(t *testing.T, a *App) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case result := <-a.wordCommandResults:
			if result.err != nil {
				t.Fatalf("Word Replace All failed: %v", result.err)
			}
			replaceAll := result.wordReplaceAll != nil
			a.wordCommandResults <- result
			a.processWordCommandAsyncResult()
			if replaceAll {
				return
			}
		case <-deadline.C:
			t.Fatal("Word Replace All did not complete")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
