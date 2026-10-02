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
	for (state.formatRunning || state.wordPageBreakIntent != nil) && time.Now().Before(deadline) {
		app.processAsyncResults()
		time.Sleep(5 * time.Millisecond)
	}
	if state.formatRunning || state.wordPageBreakIntent != nil {
		t.Fatal("timed out waiting for the asynchronous page-break command")
	}
	if strings.Contains(app.status, "失败") {
		t.Fatalf("page-break command failed: %s", app.status)
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
	if !strings.Contains(app.status, "已插入分页符") {
		t.Fatalf("page-break status=%q", app.status)
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
