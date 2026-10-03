package desktopui

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordPDFExportConfirmsLayoutWarningsBeforePicker(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("source.docx")
	state.word = word
	state.documentID = "word-export-warning"
	state.setWordBlocks(word.WordBlocks())
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		t.Fatal("could not build Word layout key")
	}
	state.wordLayoutAcceptedKey = &key
	state.wordLayoutResult = &wordlayout.Result{
		CanPreview: true,
		Pages:      []wordlayout.PageScene{{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}}},
		Diagnostics: []wordlayout.Diagnostic{{
			Severity: wordlayout.DiagnosticWarning, Code: "font-fallback", Detail: "使用了替代字体",
		}},
	}
	pickerCalls := 0
	app := &App{
		documentSessionState: state,
		activeTabID:          "tab-word",
		fileCommandResults:   make(chan fileCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
	app.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls++
		return fileCommandResult{kind: kind, path: ""}
	})
	app.requestWordPDFExport()
	if !app.modal.Visible || app.modal.Title != "导出 PDF 前确认版式提示" {
		t.Fatalf("warning modal visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if !bytes.Contains([]byte(app.modal.Message), []byte("使用了替代字体")) {
		t.Fatalf("warning modal message = %q", app.modal.Message)
	}
	app.modal.OnAction("cancel")
	if app.modal.Visible || pickerCalls != 0 || app.status != "已取消导出 PDF" {
		t.Fatalf("cancel state: visible=%v picker calls=%d status=%q", app.modal.Visible, pickerCalls, app.status)
	}
}

func TestWordPDFExportRibbonUsesPDFPickerAndCancelPreservesSource(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(true)
	state := newDocumentSessionState("source.docx")
	state.word = word
	state.documentID = "word-export-picker"
	state.setWordBlocks(word.WordBlocks())
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		t.Fatal("could not build Word layout key")
	}
	state.wordLayoutAcceptedKey = &key
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}}}}
	pickerCalls := make(chan struct {
		kind   fileCommandKind
		format string
	}, 1)
	app := &App{
		documentSessionState: state,
		activeTabID:          "word-export-picker",
		fileCommandResults:   make(chan fileCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
	app.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls <- struct {
			kind   fileCommandKind
			format string
		}{kind: kind, format: format}
		return fileCommandResult{kind: kind}
	})
	if !app.wordToolbarActionEnabled(findWordFileTestAction(t, "export-pdf")) {
		t.Fatal("Word File > Export PDF should be enabled for a DOCX")
	}
	app.executeWordToolbarAction("export-pdf")
	select {
	case call := <-pickerCalls:
		if call.kind != commandExportPDF || call.format != "pdf" {
			t.Fatalf("PDF picker = %#v; want export command for pdf", call)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("export-pdf did not reach the PDF save picker")
	}
	select {
	case result := <-app.fileCommandResults:
		app.fileCommandResults <- result
	case <-time.After(3 * time.Second):
		t.Fatal("PDF picker did not finish")
	}
	app.processAsyncResults()
	if app.status != "已取消导出 PDF" || !word.Dirty() || state.path != "source.docx" {
		t.Fatalf("cancel result: status=%q dirty=%v source path=%q", app.status, word.Dirty(), state.path)
	}
}

func TestWordPDFExportWritesValidatedPDFWithoutChangingDOCX(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(true)
	before, err := word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("source.docx")
	state.word = word
	state.documentID = "word-export"
	state.setWordBlocks(word.WordBlocks())
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		t.Fatal("could not build Word layout key")
	}
	state.wordLayoutAcceptedKey = &key
	state.wordLayoutResult = &wordlayout.Result{
		DocumentID: key.DocumentID, Revision: key.Revision, CanPreview: true,
		Pages: []wordlayout.PageScene{{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}}},
	}
	path := filepath.Join(t.TempDir(), "export.pdf")
	app := &App{
		documentSessionState: state,
		activeTabID:          "word-export",
		wordPDFExportResults: make(chan wordPDFExportResult, 1),
		shutdown:             make(chan struct{}),
		wordPDFFontSource:    nil,
	}
	app.startWordPDFFileExport(state, key, path)
	var result wordPDFExportResult
	select {
	case result = <-app.wordPDFExportResults:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for PDF export")
	}
	app.applyWordPDFExportResult(result)
	if result.err != nil {
		t.Fatalf("PDF export error: %v", result.err)
	}
	if state.formatRunning.Load() || state.wordPDFExportRunning {
		t.Fatal("PDF export completion left the session busy")
	}
	if word.Dirty() != true {
		t.Fatal("PDF export changed DOCX dirty state")
	}
	after, err := word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("PDF export changed the source DOCX snapshot")
	}
	reader, err := gopdf.Open(path)
	if err != nil {
		t.Fatalf("open generated PDF: %v", err)
	}
	if reader.NumPages() != 1 {
		t.Fatalf("generated PDF pages = %d, want 1", reader.NumPages())
	}
	if app.status != "PDF 已导出：export.pdf" {
		t.Fatalf("export success status = %q", app.status)
	}
}

func TestWordPDFExportIgnoresStaleCompletion(t *testing.T) {
	state := newDocumentSessionState("source.docx")
	state.wordRevision = 5
	state.wordRevisionAtomic.Store(5)
	state.documentID = "stale-export"
	app := &App{documentSessionState: state, status: "unchanged"}
	app.applyWordPDFExportResult(wordPDFExportResult{
		state: state,
		key:   wordLayoutKey{DocumentID: state.documentID, Revision: 4},
		path:  "stale.pdf",
	})
	if app.status != "文档已更改，旧分页结果未用于当前文档" {
		t.Fatalf("stale export status = %q", app.status)
	}
}

func TestWordDraftEpochChangesOnlyWhenDraftValueChanges(t *testing.T) {
	state := newDocumentSessionState("source.docx")
	state.wordRunCommitted["run-1"] = "old"
	captureWordTextDraft(state, "run-1", "new")
	first := state.wordDraftEpoch.Load()
	captureWordTextDraft(state, "run-1", "new")
	if got := state.wordDraftEpoch.Load(); got != first {
		t.Fatalf("repeated draft incremented epoch from %d to %d", first, got)
	}
	captureWordTextDraft(state, "run-1", "newer")
	if got := state.wordDraftEpoch.Load(); got != first+1 {
		t.Fatalf("changed draft epoch = %d, want %d", got, first+1)
	}
	captureWordTextDraft(state, "run-1", "old")
	if wordHasPendingTextDrafts(state) {
		t.Fatal("committed text value left a pending draft")
	}
}
