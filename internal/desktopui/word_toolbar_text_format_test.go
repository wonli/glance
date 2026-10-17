package desktopui

import (
	"strings"
	"testing"
	"time"

	"gioui.org/layout"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordFontSizeActionsRequireAndUseSelectedEditableRun(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	app := &App{documentSessionState: state}
	findAction := func(id string) wordToolbarAction {
		t.Helper()
		for _, action := range wordToolbarActions(wordTabHome) {
			if action.id == id {
				return action
			}
		}
		t.Fatalf("Word Home action %q not found", id)
		return wordToolbarAction{}
	}
	for _, id := range []string{"font-size-decrease", "font-size-increase"} {
		if app.wordToolbarActionEnabled(findAction(id)) {
			t.Errorf("Word font-size action %q should be disabled without a selected run", id)
		}
	}
	paragraph := word.WordBlocks()[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	for _, id := range []string{"font-size-decrease", "font-size-increase"} {
		if !app.wordToolbarActionEnabled(findAction(id)) {
			t.Errorf("Word font-size action %q should be enabled for a selected editable run", id)
		}
	}
}

func TestWordFontSizeToolbarActionsChangeSelectedRunByOnePoint(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	state := newDocumentSessionState("font-size.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	for _, step := range []struct {
		action string
		want   float64
	}{
		{action: "font-size-increase", want: 14},
		{action: "font-size-decrease", want: 13},
	} {
		app.executeWordToolbarAction(step.action)
		if !state.formatRunning.Load() {
			t.Fatalf("%s did not start a Word formatting command", step.action)
		}
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", step.action)
		}
		run, ok := selectedWordRun(state.wordBlocks, runID)
		if !ok || run.Style.FontSize != step.want {
			t.Fatalf("%s font size = %g, found=%v; want %g", step.action, run.Style.FontSize, ok, step.want)
		}
	}
}

func TestWordFontFamilyActionRequiresSelectedEditableRun(t *testing.T) {
	state := newDocumentSessionState("font-family.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	app := &App{documentSessionState: state}
	var action ribbonAction
	for _, candidate := range wordToolbarActions(wordTabHome) {
		if candidate.id == "font-family" {
			action = candidate
			break
		}
	}
	if action.id == "" {
		t.Fatal("font-family action not found")
	}
	if app.wordToolbarActionEnabled(action) {
		t.Fatal("font-family action should be disabled without a selected run")
	}
	paragraph := word.WordBlocks()[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("font-family action should be enabled for a selected editable run")
	}
}

func TestWordFontFamilyToolbarActionOpensDialogAndChangesSelectedRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	state := newDocumentSessionState("font-family.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	app.executeWordToolbarAction("font-family")
	if !app.fontPicker.open {
		t.Fatal("font picker did not open")
	}
	app.applyFontChoice("Noto Sans CJK SC")
	if app.fontPicker.open {
		t.Fatal("font picker remained open")
	}
	if !state.formatRunning.Load() {
		t.Fatal("confirm did not start a Word formatting command")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for font-family update")
	}
	run, ok := selectedWordRun(state.wordBlocks, runID)
	if !ok || run.Style.FontFamily != "Noto Sans CJK SC" {
		t.Fatalf("selected run font family = %q, found=%v; want Noto Sans CJK SC", run.Style.FontFamily, ok)
	}
}

func TestWordFormatPainterAppliesSourceFormattingToNextFocusedRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	sourceParagraph := word.WordBlocks()[0].Paragraph
	targetParagraphID, err := any(word).(document.WordBodyEditor).InsertBodyParagraph()
	if err != nil {
		t.Fatal(err)
	}
	blocks := word.WordBlocks()
	targetParagraph, ok := selectedWordParagraph(blocks, targetParagraphID)
	if !ok {
		t.Fatalf("inserted paragraph %q was not projected", targetParagraphID)
	}
	sourceRunID := sourceParagraph.Inlines[0].Text.ID
	targetRunID := targetParagraph.Inlines[0].Text.ID
	textEditor := any(word).(document.WordTextEditor)
	if err := textEditor.ReplaceTextRun(sourceRunID, "Source"); err != nil {
		t.Fatal(err)
	}
	if err := textEditor.ReplaceTextRun(targetRunID, "Target"); err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := textEditor.SetTextRunStyle(sourceRunID, document.WordTextFormatPatch{Bold: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := textEditor.SetParagraphAlignment(sourceParagraph.ID, document.WordAlignCenter); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("format-painter.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = sourceParagraph.ID
	state.selectedWordRunID = sourceRunID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	action := ribbonAction{id: "format-painter"}
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("format painter should be enabled for an editable source run")
	}
	app.executeWordToolbarAction("format-painter")
	if app.status != "已启动格式刷；单击目标文字应用，页面视图可拖选范围应用" {
		t.Fatalf("arming format painter status = %q", app.status)
	}
	targetSelector, ok := any(app).(interface {
		queueWordFormatPainterTarget(paragraphID, runID string)
	})
	if !ok {
		t.Fatal("Word editor does not route the next focused run to the active format painter")
	}
	targetSelector.queueWordFormatPainterTarget(targetParagraph.ID, targetRunID)
	if !state.formatRunning.Load() {
		t.Fatal("selecting a target run did not start the formatting operation")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for format painter")
	}
	paintedParagraph, ok := selectedWordParagraph(state.wordBlocks, targetParagraph.ID)
	if !ok {
		t.Fatal("painted paragraph is missing")
	}
	paintedRun := paintedParagraph.Inlines[0].Text
	if paintedRun.Text != "Target" || !paintedRun.Style.Bold || paintedParagraph.Alignment != document.WordAlignCenter {
		t.Fatalf("painted target text/style/alignment = %q/%#v/%q", paintedRun.Text, paintedRun.Style, paintedParagraph.Alignment)
	}
}

func TestFailedDraftFlushCancelsDeferredFormatPainter(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	sourceParagraph := word.WordBlocks()[0].Paragraph
	targetParagraphID, err := any(word).(document.WordBodyEditor).InsertBodyParagraph()
	if err != nil {
		t.Fatal(err)
	}
	blocks := word.WordBlocks()
	targetParagraph, ok := selectedWordParagraph(blocks, targetParagraphID)
	if !ok {
		t.Fatalf("inserted paragraph %q was not projected", targetParagraphID)
	}
	state := newDocumentSessionState("format-painter.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(blocks)
	state.selectedWordParagraphID = sourceParagraph.ID
	state.selectedWordRunID = sourceParagraph.Inlines[0].Text.ID
	state.wordRunDrafts = map[string]string{"missing-run": "draft"}
	state.wordRunCommitted = map[string]string{"missing-run": ""}
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.toggleWordFormatPainter()
	app.queueWordFormatPainterTarget(targetParagraph.ID, targetParagraph.Inlines[0].Text.ID)
	if state.wordDeferredToolbarAction != "apply-format-painter" {
		t.Fatalf("deferred action = %q; want format painter while drafts flush", state.wordDeferredToolbarAction)
	}
	app.requestWordTextFlush(true)
	select {
	case result := <-app.wordCommandResults:
		if result.err == nil {
			t.Fatal("invalid draft run should fail before the format painter starts")
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for invalid draft commit")
	}
	if state.wordDeferredToolbarAction != "" || state.wordFormatPainterSourceRunID != "" || state.wordFormatPainterTargetRunID != "" {
		t.Fatalf("failed draft flush left a deferred format painter: action=%q source=%q target=%q", state.wordDeferredToolbarAction, state.wordFormatPainterSourceRunID, state.wordFormatPainterTargetRunID)
	}
}

func TestWordFormatPainterAppliesOnlyTheSelectedTargetCharacters(t *testing.T) {
	app, state, _, release := newWordSelectionPageTest(t, `<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>Source</w:t></w:r></w:p><w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:rPr><w:i/></w:rPr><w:t>前目标后</w:t></w:r></w:p>`)
	defer release()
	blocks := wordBlocksForState(state)
	sourceParagraph, targetParagraph := blocks[0].Paragraph, blocks[1].Paragraph
	sourceRun, targetRun := sourceParagraph.Inlines[0].Text, targetParagraph.Inlines[0].Text
	state.selectedWordParagraphID = sourceParagraph.ID
	state.selectedWordRunID = sourceRun.ID
	state.wordFormatPainterActive = true
	state.wordFormatPainterSourceRunID = sourceRun.ID
	startOffset, endOffset := 1, 3
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: targetRun.ID, runeOffset: startOffset},
		focus: wordPageCaretPosition{runID: targetRun.ID, runeOffset: endOffset}, runOrder: map[string]int{targetRun.ID: 0},
	}

	app.queueWordFormatPainterTarget(targetParagraph.ID, targetRun.ID)
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for selected format-painter operation")
	}
	painted, ok := selectedWordParagraph(state.wordBlocks, targetParagraph.ID)
	if !ok || len(painted.Inlines) != 3 {
		t.Fatalf("painted target paragraph = %#v; want prefix/selected/suffix fragments", painted)
	}
	prefix, selected, suffix := painted.Inlines[0].Text, painted.Inlines[1].Text, painted.Inlines[2].Text
	if prefix.Text != "前" || selected.Text != "目标" || suffix.Text != "后" || !prefix.Style.Italic || prefix.Style.Bold || !selected.Style.Bold || !suffix.Style.Italic || suffix.Style.Bold {
		t.Fatalf("format painter did not limit character formatting to selection: %#v", painted.Inlines)
	}
	if painted.Alignment != sourceParagraph.Alignment {
		t.Fatalf("selected format painter alignment = %q, want source paragraph %q", painted.Alignment, sourceParagraph.Alignment)
	}
}

func TestWordFormatPainterReadsFocusedEditorSelection(t *testing.T) {
	state := newDocumentSessionState("format-painter-selection.docx")
	run := document.WordTextRun{ID: "target", Text: "前目标后", Editable: true}
	paragraph := document.WordParagraph{ID: "paragraph", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &run}}}
	state.wordBlocks = []document.WordBlock{{ID: paragraph.ID, Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	editor := wordRunEditor(state, run)
	editor.SetCaret(1, 3)

	startRunID, startOffset, endRunID, endOffset, selectedText, ok := wordFormatPainterTargetSelection(state, run.ID)
	if !ok || startRunID != run.ID || startOffset != 1 || endRunID != run.ID || endOffset != 3 || selectedText != "目标" {
		t.Fatalf("focused editor selection = %q %d:%q %d text=%q ok=%v", startRunID, startOffset, endRunID, endOffset, selectedText, ok)
	}
}

func TestWordFormatPainterSelectionStillMatchesExactRange(t *testing.T) {
	state := newDocumentSessionState("format-painter-repeated-selection.docx")
	run := document.WordTextRun{ID: "target", Text: "目标-目标", Editable: true}
	paragraph := document.WordParagraph{ID: "paragraph", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &run}}}
	state.wordBlocks = []document.WordBlock{{ID: paragraph.ID, Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: run.ID, runeOffset: 0},
		focus: wordPageCaretPosition{runID: run.ID, runeOffset: 2}, runOrder: map[string]int{run.ID: 0},
	}
	if !wordFormatPainterSelectionStillMatches(state, run.ID, 0, run.ID, 2, "目标") {
		t.Fatal("the captured text range was not recognized")
	}
	state.wordPageTextSelection.anchor.runeOffset = 3
	state.wordPageTextSelection.focus.runeOffset = 5
	if wordFormatPainterSelectionStillMatches(state, run.ID, 0, run.ID, 2, "目标") {
		t.Fatal("a different range with identical text was treated as the captured selection")
	}
}

func TestWordHighlightActionRequiresSelectedEditableRun(t *testing.T) {
	state := newDocumentSessionState("word-highlight.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	app := &App{documentSessionState: state}
	var action ribbonAction
	for _, candidate := range wordToolbarActions(wordTabHome) {
		if candidate.id == "text-highlight" {
			action = candidate
			break
		}
	}
	if action.id == "" {
		t.Fatal("text-highlight action not found")
	}
	if app.wordToolbarActionEnabled(action) {
		t.Fatal("text-highlight action should be disabled without a selected run")
	}
	paragraph := word.WordBlocks()[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("text-highlight action should be enabled for a selected editable run")
	}
}

func TestWordHighlightToolbarActionOpensPaletteAndAppliesHighlight(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	state := newDocumentSessionState("word-highlight.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	app.executeWordToolbarAction("text-highlight")
	if !state.wordToolbarHighlightPicker || state.wordToolbarColorPicker {
		t.Fatal("text-highlight action did not open its highlight palette")
	}
	if wordToolbarActionActive(state, findWordTestAction(t, "text-highlight")) {
		t.Fatal("opening the highlight palette marked the run highlighted")
	}
	app.chooseWordHighlightColor(0)
	if state.wordToolbarHighlightPicker {
		t.Fatal("highlight palette remained open after choosing a color")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for text highlight")
	}
	run, ok := selectedWordRun(state.wordBlocks, runID)
	if !ok || run.Style.Highlight != "yellow" {
		t.Fatalf("selected run highlight = %q, found=%v; want yellow", run.Style.Highlight, ok)
	}
	if !wordToolbarActionActive(state, findWordTestAction(t, "text-highlight")) {
		t.Fatal("highlight ribbon action is not active after applying a highlight")
	}
}

func TestWordEditing_SplitAfterFontColorAndHighlight(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("test.docx")
	state.word = word
	state.setWordBlocks(word.WordBlocks())

	paragraphs := allWordParagraphs(state.wordBlocks)
	if len(paragraphs) == 0 || len(paragraphs[0].Inlines) == 0 || paragraphs[0].Inlines[0].Text == nil {
		t.Fatal("expected paragraph with text run")
	}
	run := paragraphs[0].Inlines[0].Text
	state.selectedWordParagraphID = paragraphs[0].ID
	state.selectedWordRunID = run.ID

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 2),
	}

	// 1. Replace run text
	if err := word.ReplaceTextRun(run.ID, "换行测试文本"); err != nil {
		t.Fatal(err)
	}
	state.setWordBlocks(word.WordBlocks())

	// 2. Set font color and highlight
	colorVal := "FF0000"
	highlightVal := "yellow"
	if err := word.SetTextRunStyle(run.ID, document.WordTextFormatPatch{
		FontColor: &colorVal,
		Highlight: &highlightVal,
	}); err != nil {
		t.Fatal(err)
	}
	state.setWordBlocks(word.WordBlocks())

	// 3. Initiate split intent at caret index 2 ("换行" / "测试文本")
	state.wordParagraphSplitIntent = &wordParagraphSplitIntent{
		runID:          run.ID,
		value:          "换行测试文本",
		selectionStart: 2,
		selectionEnd:   2,
	}
	app.startWordParagraphSplit(state)

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("split paragraph failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for paragraph split")
	}

	// Verify that blocks now have 2 paragraphs and no error status
	blocks := state.wordBlocks
	if len(blocks) < 2 {
		t.Fatalf("expected at least 2 paragraphs after split, got %d", len(blocks))
	}
	if strings.Contains(app.status, "失败") {
		t.Fatalf("status reported failure: %s", app.status)
	}
}

func TestWordFontSizeRibbonActionAndMenu(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	state := newDocumentSessionState("font-size-ribbon.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID

	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 4)}

	// 1. Font size action must be enabled for selected run
	fontSizeAction := ribbonAction{id: "font-size", label: "字号", group: "字体"}
	if !app.wordToolbarActionEnabled(fontSizeAction) {
		t.Fatal("font-size ribbon action should be enabled for editable run")
	}

	// 2. Display text for default size (13 pt)
	if disp := selectedWordFontSizeDisplay(state); disp != "13" {
		t.Fatalf("selectedWordFontSizeDisplay = %q; want %q", disp, "13")
	}

	// Exercise the same event loop as a real ribbon click.
	wordToolbarButton(state, "font-size").Click()
	app.handleWordToolbarEvents(layout.Context{})
	if state.officeRibbonMenu.kind != officeRibbonMenuWord || state.officeRibbonMenu.wordAction != "font-size" {
		t.Fatalf("officeRibbonMenu = %+v; want kind=officeRibbonMenuWord wordAction=font-size", state.officeRibbonMenu)
	}
	menuDef := app.officeRibbonMenuDefinition()
	if menuDef.title != "字号" || len(menuDef.options) == 0 {
		t.Fatalf("menu definition = %+v; want title '字号' and non-empty options", menuDef)
	}

	// 4. Execute "word-size-12" (小四)
	app.executeOfficeRibbonMenuCommand(layout.Context{}, officeRibbonMenuWord, "word-size-12")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("word-size-12 error: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for word-size-12")
	}

	// 5. Verify run size is 12 and display name is "小四"
	run, ok := selectedWordRun(state.wordBlocks, runID)
	if !ok || run.Style.FontSize != 12 {
		t.Fatalf("run font size = %g; want 12", run.Style.FontSize)
	}
	if disp := selectedWordFontSizeDisplay(state); disp != "小四" {
		t.Fatalf("selectedWordFontSizeDisplay = %q; want %q", disp, "小四")
	}

	// 6. Execute "word-size-10.5" (五号)
	app.executeOfficeRibbonMenuCommand(layout.Context{}, officeRibbonMenuWord, "word-size-10.5")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("word-size-10.5 error: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for word-size-10.5")
	}
	if disp := selectedWordFontSizeDisplay(state); disp != "五号" {
		t.Fatalf("selectedWordFontSizeDisplay = %q; want %q", disp, "五号")
	}

	// 7. Click option index 12 ("11") through handleOfficeRibbonMenuEvents
	app.closeOfficeRibbonMenu()
	if !app.toggleWordOfficeRibbonMenu("font-size") {
		t.Fatal("failed to open font-size ribbon menu")
	}
	state.officeRibbonMenu.buttons[12].Click()
	if !app.handleOfficeRibbonMenuEvents(layout.Context{}) {
		t.Fatal("handleOfficeRibbonMenuEvents should handle button click on index 12")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("word-size-11 error: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for word-size-11")
	}
	if disp := selectedWordFontSizeDisplay(state); disp != "11" {
		t.Fatalf("selectedWordFontSizeDisplay = %q; want %q", disp, "11")
	}
}

func TestWordFontSizeAdjustSelection(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	p := word.WordBlocks()[0].Paragraph
	run := p.Inlines[0].Text

	state := newDocumentSessionState("font-size-selection.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = p.ID
	state.selectedWordRunID = run.ID

	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 4)}

	// Set text to "Hello World" and initial font size to 11
	if err := word.ReplaceTextRun(run.ID, "Hello World"); err != nil {
		t.Fatal(err)
	}
	if fontSizeEditor, ok := any(word).(document.WordTextFontSizeEditor); ok {
		if err := fontSizeEditor.SetTextRunFontSize(run.ID, 11); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("word does not implement WordTextFontSizeEditor")
	}
	state.setWordBlocks(word.WordBlocks())

	// Create selection for "World" (offsets 6 to 11)
	startPos := wordPageCaretPosition{runID: run.ID, runeOffset: 6}
	endPos := wordPageCaretPosition{runID: run.ID, runeOffset: 11}
	state.wordPageTextSelection = &wordPageTextSelection{
		anchor: startPos,
		focus:  endPos,
		scope:  "document",
	}

	// Increase font size by 2 pt on the selection
	app.adjustWordFontSize(2)

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("adjustWordFontSize selection error: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for adjustWordFontSize on selection")
	}

	// Verify that the paragraph now has split runs with different font sizes
	inlines := state.wordBlocks[0].Paragraph.Inlines
	if len(inlines) != 2 {
		t.Fatalf("expected 2 runs after selection font size adjustment, got %d", len(inlines))
	}
	if inlines[0].Text.Text != "Hello " || inlines[0].Text.Style.FontSize != 11 {
		t.Fatalf("prefix run = %q with size %g; want %q with size 11", inlines[0].Text.Text, inlines[0].Text.Style.FontSize, "Hello ")
	}
	if inlines[1].Text.Text != "World" || inlines[1].Text.Style.FontSize != 13 {
		t.Fatalf("selected run = %q with size %g; want %q with size 13", inlines[1].Text.Text, inlines[1].Text.Style.FontSize, "World")
	}
}

func TestWordSelectionFormatRefusalDoesNotFormatWholeRun(t *testing.T) {
	for _, action := range []string{"size", "increase", "color", "highlight"} {
		t.Run(action, func(t *testing.T) {
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer word.Close()
			run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
			if err := word.ReplaceTextRun(run.ID, "Hello"); err != nil {
				t.Fatal(err)
			}
			state := newDocumentSessionState("selection.docx")
			state.word = word
			state.setWordBlocks(word.WordBlocks())
			state.selectedWordRunID = run.ID
			state.wordPageTextSelection = &wordPageTextSelection{
				anchor: wordPageCaretPosition{runID: run.ID, runeOffset: 1},
				focus:  wordPageCaretPosition{runID: run.ID, runeOffset: 3}, scope: "document",
			}
			// A pending selection edit deliberately refuses a second formatting intent.
			state.wordSelectionEditIntent = &wordSelectionEditIntent{}
			app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
			switch action {
			case "size":
				app.setWordFontSize(24)
			case "increase":
				app.adjustWordFontSize(1)
			case "color":
				app.setWordFontColor(run.ID, "FF0000")
			case "highlight":
				app.setWordTextHighlight(run.ID, "yellow")
			}
			app.managedBackgroundTasks.Wait()
			if len(app.wordCommandResults) != 0 || state.formatRunning.Load() {
				t.Fatal("refused selection format fell through to a whole-run mutation")
			}
		})
	}
}
