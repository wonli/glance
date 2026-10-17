package desktopui

import (
	"runtime"
	"testing"
	"time"

	"gioui.org/io/key"
	"glance/internal/format/docx"
)

func TestWordNewCommentShortcutBindingMatchesOfficePlatforms(t *testing.T) {
	for _, test := range []struct {
		goos      string
		name      key.Name
		modifiers key.Modifiers
	}{
		{goos: "darwin", name: key.Name("A"), modifiers: key.ModShortcut | key.ModAlt},
		{goos: "windows", name: key.Name("M"), modifiers: key.ModCtrl | key.ModAlt},
		{goos: "linux", name: key.Name("M"), modifiers: key.ModCtrl | key.ModAlt},
	} {
		name, modifiers := wordNewCommentShortcutBinding(test.goos)
		if name != test.name || modifiers != test.modifiers {
			t.Errorf("%s comment shortcut = %s + %v; want %s + %v", test.goos, name, modifiers, test.name, test.modifiers)
		}
	}
}

func TestWordNewCommentShortcutOpensExistingRibbonFlow(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("word-comment-shortcut.docx")
	state.word = word
	state.setWordBlocks(cloneWordBlockSnapshot(word.WordBlocks()))
	state.selectedWordRunID = state.wordBlocks[0].Paragraph.Inlines[0].Text.ID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	h := newOfficeShortcutHarness(app)
	name, modifiers := wordNewCommentShortcutBinding(runtime.GOOS)
	pressOfficeShortcut(t, h, string(name), modifiers)
	if !app.modal.Visible || app.modal.Title != "新建批注" {
		t.Fatalf("Word comment shortcut opened visible=%v title=%q; want 新建批注", app.modal.Visible, app.modal.Title)
	}
}

func TestWordNewCommentShortcutWaitsForFocusedDraftCommit(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>批注前</w:t></w:r></w:p>`)
	run := state.wordBlocks[0].Paragraph.Inlines[0].Text
	state.selectedWordRunID = run.ID
	state.wordFocusedRunID = run.ID
	captureWordTextDraft(state, run.ID, "已修改的批注正文")
	h := newOfficeShortcutHarness(app)
	name, modifiers := wordNewCommentShortcutBinding(runtime.GOOS)
	pressOfficeShortcut(t, h, string(name), modifiers)
	if app.modal.Visible {
		t.Fatal("Word comment dialog opened before the focused text draft was committed")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if app.modal.Visible && !state.formatRunning.Load() && len(app.wordCommandResults) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !app.modal.Visible || app.modal.Title != "新建批注" {
		t.Fatalf("deferred comment dialog visible=%v title=%q status=%q", app.modal.Visible, app.modal.Title, app.status)
	}
	if got := state.word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "已修改的批注正文" {
		t.Fatalf("body text after deferred comment shortcut = %q", got)
	}
}
