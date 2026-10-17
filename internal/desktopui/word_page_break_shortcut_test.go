package desktopui

import (
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordPageAndColumnBreakShortcutsInsertAtFocusedBodyCaret(t *testing.T) {
	for _, test := range []struct {
		name      string
		modifiers key.Modifiers
		breakType document.WordBreakType
	}{
		{name: "page break", modifiers: key.ModShortcut, breakType: document.WordBreakPage},
		{name: "column break", modifiers: key.ModShortcut | key.ModShift, breakType: document.WordBreakColumn},
	} {
		t.Run(test.name, func(t *testing.T) {
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer word.Close()
			paragraph := word.WordBlocks()[0].Paragraph
			run := paragraph.Inlines[0].Text
			if err := word.ReplaceTextRun(run.ID, "前后"); err != nil {
				t.Fatal(err)
			}
			blocks := cloneWordBlockSnapshot(word.WordBlocks())
			paragraph = blocks[0].Paragraph
			run = paragraph.Inlines[0].Text
			state := newDocumentSessionState("word-break-shortcut.docx")
			state.word = word
			state.wordBlocks = blocks
			state.selectedWordParagraphID = paragraph.ID
			state.selectedWordRunID = run.ID
			state.wordFocusedRunID = run.ID
			editor := wordRunEditor(state, *run)
			editor.SetText("前后")
			editor.SetCaret(1, 1)
			app := &App{
				documentSessionState: state,
				wordCommandResults:   make(chan wordCommandResult, 4),
				shutdown:             make(chan struct{}),
				theme:                material.NewTheme(),
			}
			t.Cleanup(func() {
				close(app.shutdown)
				app.stopAndWaitManagedBackground()
				app.completeQueuedWordSplitsOnClose()
			})
			h := newOfficeShortcutHarness(app)
			h.draw = func(gtx layout.Context) {
				gtx.Execute(key.FocusCmd{Tag: editor})
				app.handleOfficeShortcuts(gtx)
				material.Editor(app.theme, editor, "").Layout(gtx)
			}
			h.frame()
			h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: test.modifiers})
			h.now += 30 * time.Millisecond
			h.frame()

			waitForWordBreakShortcut(t, app, state)
			paragraph = state.wordBlocks[0].Paragraph
			if len(paragraph.Inlines) != 3 || paragraph.Inlines[0].Text == nil || paragraph.Inlines[0].Text.Text != "前" || paragraph.Inlines[1].PageBreak == nil || paragraph.Inlines[1].PageBreak.Type != test.breakType || paragraph.Inlines[2].Text == nil || paragraph.Inlines[2].Text.Text != "后" {
				t.Fatalf("%s projection = %#v", test.name, paragraph.Inlines)
			}
		})
	}
}

func TestWordPageBreakShortcutRequiresFocusedBodyEditorAndNoIMEComposition(t *testing.T) {
	tests := []struct {
		name        string
		focusEditor bool
		composition bool
	}{
		{name: "focus outside body"},
		{name: "active IME composition", focusEditor: true, composition: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer word.Close()
			blocks := cloneWordBlockSnapshot(word.WordBlocks())
			paragraph := blocks[0].Paragraph
			run := paragraph.Inlines[0].Text
			state := newDocumentSessionState("word-break-shortcut-focus.docx")
			state.word = word
			state.wordBlocks = blocks
			state.selectedWordParagraphID = paragraph.ID
			state.selectedWordRunID = run.ID
			state.wordFocusedRunID = run.ID
			editor := wordRunEditor(state, *run)
			app := &App{
				documentSessionState: state,
				wordCommandResults:   make(chan wordCommandResult, 2),
				shutdown:             make(chan struct{}),
				theme:                material.NewTheme(),
			}
			t.Cleanup(func() {
				close(app.shutdown)
				app.stopAndWaitManagedBackground()
				app.completeQueuedWordSplitsOnClose()
			})
			h := newOfficeShortcutHarness(app)
			outside := new(struct{})
			h.draw = func(gtx layout.Context) {
				if test.focusEditor {
					gtx.Execute(key.FocusCmd{Tag: editor})
				} else {
					gtx.Execute(key.FocusCmd{Tag: outside})
				}
				app.handleOfficeShortcuts(gtx)
				material.Editor(app.theme, editor, "").Layout(gtx)
			}
			h.frame()
			if test.composition {
				h.router.Queue(key.CompositionEvent{Start: 0, End: 1}, key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "n"})
				h.now += 30 * time.Millisecond
				h.frame()
				if !editorHasActiveComposition(editor) {
					t.Fatal("test failed to activate the focused editor's IME composition")
				}
			}
			h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModShortcut})
			h.now += 30 * time.Millisecond
			h.frame()
			if state.wordPageBreakIntent != nil || state.formatRunning.Load() {
				t.Fatalf("page break started with focusEditor=%t composition=%t", test.focusEditor, test.composition)
			}
			if len(word.WordBlocks()[0].Paragraph.Inlines) != 1 {
				t.Fatal("page-break shortcut changed the DOCX while it should have been ignored")
			}
		})
	}
}

func waitForWordBreakShortcut(t *testing.T, app *App, state *documentSessionState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if state.wordPageBreakIntent == nil && !state.formatRunning.Load() {
			if strings.Contains(app.status, "失败") {
				t.Fatalf("Word break shortcut failed: %s", app.status)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for the Word break shortcut: intent=%+v formatRunning=%t status=%q", state.wordPageBreakIntent, state.formatRunning.Load(), app.status)
}
