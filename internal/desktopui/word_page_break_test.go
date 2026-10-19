package desktopui

import (
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordInsertPageBreakToolbarCommitsDraftAndReturnsCaret(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	run := paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(run.ID, "A原文B"); err != nil {
		t.Fatal(err)
	}
	blocks := cloneWordBlockSnapshot(word.WordBlocks())
	paragraph = blocks[0].Paragraph
	run = paragraph.Inlines[0].Text
	state := newDocumentSessionState("page-break.docx")
	state.word = word
	state.wordBlocks = blocks
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = run.ID
	editor := wordRunEditor(state, *run)
	editor.SetText("A草稿B")
	editor.SetCaret(2, 2)
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
		shutdown:             make(chan struct{}),
	}
	var action ribbonAction
	found := false
	for _, candidate := range wordToolbarActions(wordTabInsert) {
		if candidate.id == "page-break" {
			action, found = candidate, true
			break
		}
	}
	if !found || !app.wordToolbarActionEnabled(action) {
		t.Fatal("Insert > Page Break should be enabled for an editable body-text caret")
	}
	app.executeWordToolbarAction("page-break")
	deadline := time.Now().Add(3 * time.Second)
	for (state.formatRunning.Load() || state.wordPageBreakIntent != nil) && time.Now().Before(deadline) {
		app.processAsyncResults()
		time.Sleep(5 * time.Millisecond)
	}
	if state.formatRunning.Load() || state.wordPageBreakIntent != nil {
		t.Fatal("timed out waiting for the asynchronous page-break command")
	}
	if strings.Contains(app.statusText(), "失败") {
		t.Fatalf("page-break command failed: %s", app.statusText())
	}
	paragraph = state.wordBlocks[0].Paragraph
	if len(paragraph.Inlines) != 3 || paragraph.Inlines[0].Text == nil || paragraph.Inlines[0].Text.Text != "A草" || paragraph.Inlines[1].PageBreak == nil || paragraph.Inlines[2].Text == nil || paragraph.Inlines[2].Text.Text != "稿B" {
		t.Fatalf("page-break toolbar projection = %#v", paragraph.Inlines)
	}
	if state.selectedWordParagraphID != paragraph.ID || state.selectedWordRunID != paragraph.Inlines[2].Text.ID {
		t.Fatalf("selection after inserting page break = %q/%q", state.selectedWordParagraphID, state.selectedWordRunID)
	}
	if start, end := state.wordRunEditors[state.selectedWordRunID].Selection(); start != 0 || end != 0 {
		t.Fatalf("caret after page break = %d:%d; want 0:0", start, end)
	}
	if !strings.Contains(app.statusText(), "已插入分页符") {
		t.Fatalf("page-break status=%q", app.statusText())
	}
}

func TestWordInsertPageBreakToolbarStaysDisabledForTableRuns(t *testing.T) {
	word := syntheticWordTableDocument(t)
	defer word.Close()
	blocks := cloneWordBlockSnapshot(word.WordBlocks())
	tableRunID := ""
	for _, row := range blocks[1].Table.Rows {
		for _, cell := range row.Cells {
			for _, paragraph := range cell.Paragraphs {
				for _, inline := range paragraph.Inlines {
					if inline.Text != nil {
						tableRunID = inline.Text.ID
					}
				}
			}
		}
	}
	state := newDocumentSessionState("table.docx")
	state.word = word
	state.wordBlocks = blocks
	state.selectedWordRunID = tableRunID
	app := &App{documentSessionState: state}
	action := ribbonAction{}
	for _, candidate := range wordToolbarActions(wordTabInsert) {
		if candidate.id == "page-break" {
			action = candidate
		}
	}
	if tableRunID == "" || app.wordToolbarActionEnabled(action) {
		t.Fatalf("page-break action enabled in table cell for run %q", tableRunID)
	}
}

var _ document.WordPageBreakEditor = (*docx.Document)(nil)

func TestWordInsertBreaksDialogAndColumnBreak(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	run := paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(run.ID, "前缀后缀"); err != nil {
		t.Fatal(err)
	}
	blocks := cloneWordBlockSnapshot(word.WordBlocks())
	paragraph = blocks[0].Paragraph
	run = paragraph.Inlines[0].Text
	state := newDocumentSessionState("breaks.docx")
	state.word = word
	state.wordBlocks = blocks
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = run.ID
	editor := wordRunEditor(state, *run)
	editor.SetText("前缀后缀")
	editor.SetCaret(2, 2)
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
		shutdown:             make(chan struct{}),
	}
	var action ribbonAction
	found := false
	for _, candidate := range wordToolbarActions(wordTabLayout) {
		if candidate.id == "layout-breaks" {
			action, found = candidate, true
			break
		}
	}
	if !found || !app.wordToolbarActionEnabled(action) {
		t.Fatal("Layout > Breaks should be enabled for an editable body-text caret")
	}

	app.showWordBreaksDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入分隔符" {
		t.Fatalf("expected modal dialog for breaks, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}

	app.modal.OnAction("column")
	deadline := time.Now().Add(3 * time.Second)
	for (state.formatRunning.Load() || state.wordPageBreakIntent != nil) && time.Now().Before(deadline) {
		app.processAsyncResults()
		time.Sleep(5 * time.Millisecond)
	}
	if state.formatRunning.Load() || state.wordPageBreakIntent != nil {
		t.Fatal("timed out waiting for the asynchronous break command")
	}

	paragraph = state.wordBlocks[0].Paragraph
	if len(paragraph.Inlines) != 3 || paragraph.Inlines[1].PageBreak == nil || paragraph.Inlines[1].PageBreak.Type != document.WordBreakColumn {
		t.Fatalf("expected column break inline in paragraph, got %#v", paragraph.Inlines)
	}
	if !strings.Contains(app.statusText(), "已插入分栏符") {
		t.Fatalf("expected status '已插入分栏符', got %q", app.statusText())
	}
}

func TestWordInsertBlankPageToolbar(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	run := paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(run.ID, "第一页第二页"); err != nil {
		t.Fatal(err)
	}
	blocks := cloneWordBlockSnapshot(word.WordBlocks())
	paragraph = blocks[0].Paragraph
	run = paragraph.Inlines[0].Text
	state := newDocumentSessionState("blank-page.docx")
	state.word = word
	state.wordBlocks = blocks
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = run.ID
	editor := wordRunEditor(state, *run)
	editor.SetText("第一页第二页")
	editor.SetCaret(3, 3)
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
		shutdown:             make(chan struct{}),
	}
	var action ribbonAction
	found := false
	for _, candidate := range wordToolbarActions(wordTabInsert) {
		if candidate.id == "blank-page" {
			action, found = candidate, true
			break
		}
	}
	if !found || !app.wordToolbarActionEnabled(action) {
		t.Fatal("Insert > Blank Page should be enabled for an editable body-text caret")
	}

	app.executeWordToolbarAction("blank-page")
	deadline := time.Now().Add(3 * time.Second)
	for (state.formatRunning.Load() || state.wordPageBreakIntent != nil) && time.Now().Before(deadline) {
		app.processAsyncResults()
		time.Sleep(5 * time.Millisecond)
	}
	if state.formatRunning.Load() || state.wordPageBreakIntent != nil {
		t.Fatal("timed out waiting for the asynchronous blank-page command")
	}

	paragraph = state.wordBlocks[0].Paragraph
	if len(paragraph.Inlines) != 5 {
		t.Fatalf("expected 5 inlines after blank page, got %d: %#v", len(paragraph.Inlines), paragraph.Inlines)
	}
	if paragraph.Inlines[1].PageBreak == nil || paragraph.Inlines[3].PageBreak == nil {
		t.Fatalf("expected two page breaks surrounding blank page run, got %#v", paragraph.Inlines)
	}
	if paragraph.Inlines[2].Text == nil || state.selectedWordRunID != paragraph.Inlines[2].Text.ID {
		t.Fatalf("expected caret on middle run %v, got %q", paragraph.Inlines[2], state.selectedWordRunID)
	}
	if !strings.Contains(app.statusText(), "已插入空白页") {
		t.Fatalf("expected status '已插入空白页', got %q", app.statusText())
	}
}
