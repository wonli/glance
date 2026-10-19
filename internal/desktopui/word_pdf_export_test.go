package desktopui

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/i18n"
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
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls++
		return fileCommandResult{kind: kind, path: ""}
	})
	app.requestWordPDFExport()
	if !app.modal.Visible || app.text(app.modal.TitleRef) != "导出 PDF 前确认版式提示" {
		t.Fatalf("warning modal visible=%v title=%q", app.modal.Visible, app.text(app.modal.TitleRef))
	}
	if !bytes.Contains([]byte(app.messageText(app.modal.MessageRef, app.modal.Message)), []byte("使用了替代字体")) {
		t.Fatalf("warning modal message = %q", app.messageText(app.modal.MessageRef, app.modal.Message))
	}
	app.modal.OnAction("cancel")
	if app.modal.Visible || pickerCalls != 0 || app.statusText() != "已取消导出 PDF" {
		t.Fatalf("cancel state: visible=%v picker calls=%d status=%q", app.modal.Visible, pickerCalls, app.statusText())
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
	app.applyLocaleSelection(i18n.SelectionChinese)
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
	if !app.modal.Visible || app.text(app.modal.TitleRef) != "导出 PDF 预览" {
		t.Fatalf("expected PDF preview before picker, got visible=%v title=%q", app.modal.Visible, app.text(app.modal.TitleRef))
	}
	app.modal.OnAction("confirm")
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
	if app.statusText() != "已取消导出 PDF" || !word.Dirty() || state.path != "source.docx" {
		t.Fatalf("cancel result: status=%q dirty=%v source path=%q", app.statusText(), word.Dirty(), state.path)
	}
}

func TestWordPDFPreviewReflowsLandscapeBeforeExport(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("source.docx")
	state.word = word
	state.documentID = "word-preview-orientation"
	state.setWordBlocks(word.WordBlocks())
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		t.Fatal("could not build Word layout key")
	}
	state.wordLayoutAcceptedKey = &key
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}}}}
	app := &App{documentSessionState: state, shutdown: make(chan struct{})}
	app.openWordPDFExportPreview(state, key)
	if err := app.updateWordPreviewOrientation("landscape"); err != nil {
		t.Fatal(err)
	}
	if !app.pdfExportPreview.wordPreviewPending {
		t.Fatal("orientation reflow should be pending")
	}
	app.modal.OnAction("confirm")
	if !app.modal.Visible || app.fileCommandRunning {
		t.Fatal("export proceeded before landscape preview was ready")
	}
	select {
	case result := <-app.wordPDFPreviewResults:
		app.applyWordPDFPreviewResult(result)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Word preview reflow")
	}
	if app.pdfExportPreview.wordPreviewPending || app.pdfExportPreview.Orientation != "landscape" {
		t.Fatalf("preview orientation = %q, pending=%v, status=%q", app.pdfExportPreview.Orientation, app.pdfExportPreview.wordPreviewPending, app.statusText())
	}
	if app.pdfExportPreview.wordResult == nil || len(app.pdfExportPreview.wordResult.Pages) == 0 ||
		app.pdfExportPreview.wordResult.Pages[0].Clip.WidthPT <= app.pdfExportPreview.wordResult.Pages[0].Clip.HeightPT {
		t.Fatal("landscape preview did not produce a landscape page")
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
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.startWordPDFFileExport(state, key, path, "portrait")
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
	if app.statusText() != "PDF 已导出：export.pdf" {
		t.Fatalf("export success status = %q", app.statusText())
	}
}

func TestWordPDFExportIgnoresStaleCompletion(t *testing.T) {
	state := newDocumentSessionState("source.docx")
	state.wordRevision = 5
	state.wordRevisionAtomic.Store(5)
	state.documentID = "stale-export"
	app := &App{documentSessionState: state, statusRef: i18n.Message{ID: "status.initial.open_document"}}
	app.applyLocaleSelection(i18n.SelectionChinese)
	app.applyWordPDFExportResult(wordPDFExportResult{
		state: state,
		key:   wordLayoutKey{DocumentID: state.documentID, Revision: 4},
		path:  "stale.pdf",
	})
	if app.statusText() != "文档已更改，旧分页结果未用于当前文档" {
		t.Fatalf("stale export status = %q", app.statusText())
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

func TestWordPDFExportWithTextAndCJK(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	shaper, err := newWordTextShaper()
	if err != nil {
		t.Skipf("cannot init text shaper: %v", err)
	}

	state := newDocumentSessionState("source.docx")
	state.word = word
	state.documentID = "word-cjk-export"
	state.setWordBlocks(word.WordBlocks())
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		t.Fatal("could not build Word layout key")
	}

	shaped, err := shaper.Shape("Word测试内容", document.WordTextStyle{FontSize: 12})
	if err != nil {
		t.Skipf("shape error: %v", err)
	}
	if len(shaped.Glyphs) == 0 {
		t.Skip("no glyphs shaped")
	}

	glyphs := make([]wordlayout.PositionedGlyph, len(shaped.Glyphs))
	var advance float64
	for i, g := range shaped.Glyphs {
		glyphs[i] = wordlayout.PositionedGlyph{
			FontKey:    g.FontKey,
			GlyphID:    g.GlyphID,
			RuneStart:  g.RuneStart,
			RuneEnd:    g.RuneEnd,
			XPT:        20 + advance,
			YPT:        40,
			AdvanceXPT: g.AdvanceXPT,
		}
		advance += g.AdvanceXPT
	}

	state.wordLayoutAcceptedKey = &key
	state.wordLayoutResult = &wordlayout.Result{
		DocumentID: key.DocumentID, Revision: key.Revision, CanPreview: true,
		Pages: []wordlayout.PageScene{{
			Index: 0,
			Clip:  wordlayout.Rect{WidthPT: 595, HeightPT: 842},
			Text: []wordlayout.TextFragment{{
				RunID:       "run-1",
				Text:        "Word测试内容",
				RuneEnd:     len([]rune("Word测试内容")),
				Style:       document.WordTextStyle{FontSize: 12},
				XPT:         20,
				BaselineYPT: 40,
				Glyphs:      glyphs,
			}},
		}},
	}

	path := filepath.Join(t.TempDir(), "export-cjk.pdf")
	app := &App{
		documentSessionState: state,
		activeTabID:          "word-cjk-export",
		wordPDFExportResults: make(chan wordPDFExportResult, 1),
		shutdown:             make(chan struct{}),
		wordPDFFontSource:    shaper,
	}

	app.startWordPDFFileExport(state, key, path, "portrait")
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

	reader, err := gopdf.Open(path)
	if err != nil {
		t.Fatalf("open generated PDF: %v", err)
	}
	if reader.NumPages() != 1 {
		t.Fatalf("generated PDF pages = %d, want 1", reader.NumPages())
	}
	text, err := reader.PageText(0)
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if text != "Word测试内容" {
		t.Fatalf("extracted text = %q, want 'Word测试内容'", text)
	}
}
