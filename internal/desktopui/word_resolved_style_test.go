package desktopui

import (
	"testing"
	"time"

	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordFontControlsUseInheritedPageSize(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	blocks := cloneWordBlockSnapshot(word.WordBlocks())
	run := blocks[0].Paragraph.Inlines[0].Text
	run.Style.FontSize = 0
	run.StyleSet = 0
	state := newDocumentSessionState("inherited.docx")
	state.word, state.wordBlocks, state.selectedWordRunID = word, blocks, run.ID
	state.wordLayoutResult = &wordlayout.Result{Pages: []wordlayout.PageScene{{Text: []wordlayout.TextFragment{{RunID: run.ID, Style: document.WordTextStyle{FontSize: 12, FontFamily: "仿宋", FontColor: "000000"}}}}}}
	if got := selectedWordFontSize(state); got != 12 {
		t.Errorf("selected font size=%g, want inherited 12pt", got)
	}
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.adjustWordFontSize(-1)
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		updated, ok := selectedWordRun(result.blocks, run.ID)
		if !ok || updated.Style.FontSize != 11 {
			t.Fatalf("A− size=%g, want 11pt from inherited 12pt", updated.Style.FontSize)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("font-size adjustment did not finish")
	}
}

func TestWordPageEditorUsesResolvedSceneTypography(t *testing.T) {
	theme := material.NewTheme()
	state := newDocumentSessionState("")
	run := document.WordTextRun{ID: "run", Text: "正文"}
	state.wordLayoutResult = &wordlayout.Result{Pages: []wordlayout.PageScene{{Text: []wordlayout.TextFragment{{RunID: "run", Style: document.WordTextStyle{FontSize: 12, FontFamily: "仿宋", FontColor: "000000"}}}}}}
	label := wordRunLabelForState(theme, state, document.WordParagraph{Style: "Heading1"}, run)
	if label.TextSize != unit.Sp(16) {
		t.Fatalf("editor size=%g sp, want 12pt=16sp", label.TextSize)
	}
	if label.Font.Typeface != wordRunTypeface("仿宋") {
		t.Fatalf("editor uses %s instead of resolved family", label.Font.Typeface)
	}
	if label.Color.R != 0 || label.Color.G != 0 || label.Color.B != 0 || label.Color.A != 255 {
		t.Fatalf("editor ink=%v, want opaque black", label.Color)
	}
}
