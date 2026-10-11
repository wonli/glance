package desktopui

import (
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/workspace"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkdownImagePickerRetainsDialogAndRejectsStaleResult(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	a.showMarkdownInsertDialog("image")
	epoch := v.dialogEpoch
	v.imagePicks <- markdownImagePickResult{path: "/tmp/picture.png", epoch: epoch}
	a.pollMarkdownImagePicker()
	if a.modal.FormInputs[1].Text() != "/tmp/picture.png" {
		t.Fatal("picker did not fill destination")
	}
	a.modal.OnAction("cancel")
	a.showMarkdownInsertDialog("image")
	v.imagePicks <- markdownImagePickResult{path: "/tmp/old.png", epoch: epoch}
	a.pollMarkdownImagePicker()
	if a.modal.FormInputs[1].Text() != "" {
		t.Fatal("stale picker changed new dialog")
	}
}

func TestMarkdownLinkDestinations(t *testing.T) {
	source := filepath.Join(t.TempDir(), "notes.md")
	target, local, err := markdownLinkTarget("./chapter%201.md", source)
	if err != nil || !local || target != filepath.Join(filepath.Dir(source), "chapter 1.md") {
		t.Fatalf("resolved %q %v %v", target, local, err)
	}
	if _, _, err = markdownLinkTarget("javascript:alert(1)", source); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
	if _, _, err = markdownLinkTarget("relative.md", ""); err == nil {
		t.Fatal("unsaved relative link accepted")
	}
}

func TestMarkdownEditorExactSourceRoundTrip(t *testing.T) {
	a := markdownTestApp(t)
	source := "\ufeff# 中文\r\n\r\nuntouched\r\n"
	a.markdownView.editor.SetText(source)
	a.flushMarkdownDraft()
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := a.markdownDoc.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != source {
		t.Fatalf("editor rewrote source %q: %v", data, err)
	}
}

func markdownTestApp(t *testing.T) *App {
	t.Helper()
	a := New(nil, "")
	if err := a.installDocument(loadedDocument{markdownDoc: markdowndoc.NewBlank()}); err != nil {
		t.Fatal(err)
	}
	a.ensureMarkdownView()
	return a
}
func TestMarkdownDraftFlush(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText("中文")
	a.flushMarkdownDraft()
	if a.markdownDoc.Text() != "中文" || !a.isDirty() {
		t.Fatal("draft not synchronized")
	}
	a.markdownDoc.SetDirty(false)
	a.flushMarkdownDraft()
	if a.isDirty() {
		t.Fatal("no-op became dirty")
	}
	v.editor.SetCaret(0, 2)
	if err := a.applyMarkdownEdit("bold", markdowndoc.EditArgs{}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-v.editResults:
		if result.err != nil {
			t.Fatalf("Markdown command failed: %v", result.err)
		}
		v.editResults <- result
		a.pollMarkdownEditResult(v)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Markdown command result")
	}
	if v.editor.Text() != "**中文**" || a.markdownDoc.Text() != "**中文**" {
		t.Fatal("command lost source")
	}
	s, e := v.editor.Selection()
	if s != 2 || e != 4 {
		t.Fatalf("selection %d,%d", s, e)
	}
}
func TestMarkdownStaleParseRejected(t *testing.T) {
	v := newMarkdownView("new")
	v.requested = "new"
	v.pending = true
	if v.accept(markdownParseResult{source: "old", snapshot: document.MarkdownSnapshot{Source: "old"}}) {
		t.Fatal("stale accepted")
	}
	if !v.accept(markdownParseResult{source: "new", snapshot: document.MarkdownSnapshot{Source: "new"}}) {
		t.Fatal("fresh rejected")
	}
}
func TestMarkdownDialogCancellation(t *testing.T) {
	a := markdownTestApp(t)
	a.markdownView.editor.SetText("中文")
	a.markdownView.editor.SetCaret(0, 2)
	a.showMarkdownInsertDialog("link")
	a.modal.OnAction("cancel")
	if a.markdownView.editor.Text() != "中文" {
		t.Fatal("cancellation mutated source")
	}
}

func TestMarkdownTabSwitchFlushesDraftAndInvalidatesExport(t *testing.T) {
	a := markdownTestApp(t)
	state := a.documentSessionState
	v := a.markdownView
	v.editor.SetText("切换前的草稿")
	epoch := v.draftEpoch.Load()
	other := newDocumentSessionState("")
	a.activateSession("recent", other)
	if state.markdownDoc.Text() != "切换前的草稿" {
		t.Fatal("tab switch lost draft")
	}
	if v.draftEpoch.Load() == epoch {
		t.Fatal("tab switch left export valid")
	}
}

func TestMarkdownRecoveryKeepsModeAndSelection(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	v.editor.SetText("# 中文\n")
	a.flushMarkdownDraft()
	v.mode = 2
	v.collapsed = true
	v.editor.SetCaret(2, 4)
	payload, _ := snapshotState(a.documentSessionState)
	state, err := a.restoreSessionState(workspace.Session{Tab: workspace.Tab{Kind: "draft"}, Format: "markdown", Payload: payload, Dirty: true, ViewState: a.viewStateBytes()})
	if err != nil {
		t.Fatal(err)
	}
	if state.markdownView == nil || state.markdownView.mode != 2 || !state.markdownView.collapsed {
		t.Fatal("view mode lost")
	}
	start, end := state.markdownView.editor.Selection()
	if start != 2 || end != 4 {
		t.Fatalf("recovered selection %d,%d", start, end)
	}
}
