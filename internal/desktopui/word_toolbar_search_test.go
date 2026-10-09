package desktopui

import (
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordFindCyclesCaseInsensitivelyAcrossEditableRunsAndDrafts(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	blocks := word.WordBlocks()
	firstRunID := blocks[0].Paragraph.Inlines[0].Text.ID
	textEditor, ok := any(word).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose Word text editing")
	}
	if err := textEditor.ReplaceTextRun(firstRunID, "there World world"); err != nil {
		t.Fatal(err)
	}
	bodyEditor, ok := any(word).(document.WordBodyEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose body paragraph insertion")
	}
	if _, err := bodyEditor.InsertBodyParagraph(); err != nil {
		t.Fatal(err)
	}
	blocks = word.WordBlocks()
	secondRunID := blocks[1].Paragraph.Inlines[0].Text.ID
	if err := textEditor.ReplaceTextRun(secondRunID, "other world"); err != nil {
		t.Fatal(err)
	}
	blocks = cloneWordBlockSnapshot(word.WordBlocks())
	find := func(currentRun string, currentEnd int, drafts map[string]string) wordTextMatch {
		t.Helper()
		match, found := findWordTextMatch(blocks, "world", currentRun, currentEnd, drafts)
		if !found {
			t.Fatal("expected to find world")
		}
		return match
	}
	if got := find("", 0, nil); got.RunID != firstRunID || got.Start != 6 || got.End != 11 {
		t.Fatalf("first match = %#v; want first run [6,11)", got)
	}
	if got := find(firstRunID, 11, nil); got.RunID != firstRunID || got.Start != 12 || got.End != 17 {
		t.Fatalf("next match = %#v; want second match in first run [12,17)", got)
	}
	if got := find(firstRunID, 17, nil); got.RunID != secondRunID || got.Start != 6 || got.End != 11 {
		t.Fatalf("cross-run match = %#v; want second run [6,11)", got)
	}
	if got := find(secondRunID, 11, nil); got.RunID != firstRunID || got.Start != 6 || got.End != 11 {
		t.Fatalf("wrapped match = %#v; want first run [6,11)", got)
	}
	if got, found := findWordTextMatch(blocks, "keyword", "", 0, map[string]string{secondRunID: "other keyword"}); !found || got.RunID != secondRunID || got.Start != 6 {
		t.Fatalf("draft match = %#v found=%v; want second run draft at 6", got, found)
	}
}

func TestWordFindToolbarOpensAndNavigatesToSelectedMatch(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	textEditor, ok := any(word).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose Word text editing")
	}
	if err := textEditor.ReplaceTextRun(runID, "find this"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-find.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}
	if !app.wordToolbarActionEnabled(findWordTestAction(t, "find")) {
		t.Fatal("Word Find should be enabled for a document with editable text")
	}
	app.executeWordToolbarAction("find")
	if !app.modal.Visible || !app.modal.HasInput {
		t.Fatal("Word Find did not open its query dialog")
	}
	app.modal.Input.SetText("this")
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("Word Find left the query dialog visible after confirming")
	}
	if state.selectedWordRunID != runID || state.selectedWordParagraphID != paragraph.ID {
		t.Fatalf("Word Find selection = %q/%q; want %q/%q", state.selectedWordRunID, state.selectedWordParagraphID, runID, paragraph.ID)
	}
	if editor := state.wordRunEditors[runID]; editor == nil {
		t.Fatal("Word Find did not initialize the persistent run editor")
	} else if start, end := editor.Selection(); start != 5 || end != 9 {
		t.Fatalf("Word Find selection = [%d,%d); want [5,9)", start, end)
	}
	if state.wordPendingFocusRunID != runID {
		t.Fatalf("pending focus run = %q; want %q", state.wordPendingFocusRunID, runID)
	}
}

func TestWordReplaceToolbarOpensReplacementDialog(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	if err := word.ReplaceTextRun(runID, "replace this"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-replace.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordRunID = runID
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state}

	app.executeWordToolbarAction("replace")
	if !app.modal.Visible || !app.modal.HasInput {
		t.Fatal("Word Replace did not open its search and replacement dialog")
	}
}

func TestWordReplaceReplacesSelectionAndSelectsNextMatch(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	if err := word.ReplaceTextRun(runID, "apple apple"); err != nil {
		t.Fatal(err)
	}
	paragraph = word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-replace.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordRunID = runID
	state.selectedWordParagraphID = paragraph.ID
	textEditor := wordRunEditor(state, *paragraph.Inlines[0].Text)
	textEditor.SetCaret(0, 5)
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.executeWordToolbarAction("replace")
	if !app.modal.HasSecondInput {
		t.Fatal("Word Replace did not show a replacement-value input")
	}
	app.modal.Input.SetText("APPLE")
	app.modal.SecondInput.SetText("pear")
	app.modal.OnAction("replace")
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
	case <-time.After(time.Second):
		t.Fatal("Word Replace did not publish its edit result")
	}
	app.processAsyncResults()

	blocks := word.WordBlocks()
	got := blocks[0].Paragraph.Inlines[0].Text.Text
	if got != "pear apple" {
		t.Fatalf("first selected match replacement = %q; want %q", got, "pear apple")
	}
	if start, end := textEditor.Selection(); start != 5 || end != 10 {
		t.Fatalf("next match selection = [%d,%d); want [5,10)", start, end)
	}
	if !app.modal.Visible {
		t.Fatal("Word Replace closed its dialog after replacing one match")
	}

	app.modal.OnAction("replace")
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
	case <-time.After(time.Second):
		t.Fatal("Word Replace did not publish the second edit result")
	}
	app.processAsyncResults()
	if got := word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "pear pear" {
		t.Fatalf("second selected match replacement = %q; want %q", got, "pear pear")
	}
}
