package desktopui

import (
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"
)

func TestWordHyperlinkShortcutLinksSelectedTextOnly(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲ABC乙</w:t></w:r></w:p>`)
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: run.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: run.ID, runeOffset: 4}, runOrder: map[string]int{run.ID: 0},
	}
	state.selectedWordRunID = run.ID
	h := newOfficeShortcutHarness(app)

	pressOfficeShortcut(t, h, "K", key.ModShortcut)
	if !app.modal.Visible || app.modal.Title != "插入超链接" {
		t.Fatalf("Word Ctrl/Cmd+K modal visible=%v title=%q; want Insert Hyperlink", app.modal.Visible, app.modal.Title)
	}
	if got := app.modal.SecondInput.Text(); got != "ABC" {
		t.Fatalf("hyperlink display text = %q; want selected text ABC", got)
	}
	app.modal.Input.SetText("https://example.com")
	app.modal.SecondInput.SetText("链接")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "已成功插入超链接") {
		t.Fatalf("hyperlink action status = %q", app.status)
	}
	var text strings.Builder
	for _, block := range state.word.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil {
				text.WriteString(inline.Text.Text)
			}
		}
	}
	if got := text.String(); got != "甲链接乙" {
		t.Fatalf("hyperlink changed text outside selection: %q; want 甲链接乙", got)
	}
}

func TestWordHyperlinkShortcutWaitsForFocusedDraftCommit(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲ABC乙</w:t></w:r></w:p>`)
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	state.wordPageTextSelection = &wordPageTextSelection{
		scope: "range", anchor: wordPageCaretPosition{runID: run.ID, runeOffset: 1},
		focus: wordPageCaretPosition{runID: run.ID, runeOffset: 4}, runOrder: map[string]int{run.ID: 0},
	}
	state.selectedWordRunID = run.ID
	state.wordFocusedRunID = run.ID
	captureWordTextDraft(state, run.ID, "甲AXZ乙")
	h := newOfficeShortcutHarness(app)

	pressOfficeShortcut(t, h, "K", key.ModShortcut)
	if app.modal.Visible {
		t.Fatal("Word hyperlink dialog opened before the focused text draft was committed")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if app.modal.Visible && !state.formatRunning.Load() && len(app.wordCommandResults) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !app.modal.Visible || app.modal.Title != "插入超链接" {
		t.Fatalf("deferred hyperlink dialog visible=%v title=%q status=%q", app.modal.Visible, app.modal.Title, app.status)
	}
	if got := app.modal.SecondInput.Text(); got != "AXZ" {
		t.Fatalf("hyperlink display text after draft commit = %q; want AXZ", got)
	}
}
