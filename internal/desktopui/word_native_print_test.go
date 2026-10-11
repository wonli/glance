package desktopui

import (
	"context"
	"testing"
	"time"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/format/docx"
	"glance/internal/printservice"
	"glance/internal/wordlayout"
)

type testWordPrinter struct {
	available bool
	calls     chan printedWordPDF
	err       error
}

type printedWordPDF struct {
	data    []byte
	options printservice.Options
}

func (printer *testWordPrinter) Available() bool { return printer.available }
func (printer *testWordPrinter) Print(_ context.Context, data []byte, options printservice.Options) error {
	printer.calls <- printedWordPDF{data: append([]byte(nil), data...), options: options}
	return printer.err
}

func newNativePrintTestApp(t *testing.T, printer *testWordPrinter) (*App, *documentSessionState, *docx.Document) {
	t.Helper()
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = word.Close() })
	word.SetDirty(true)
	state := newDocumentSessionState("source.docx")
	state.word = word
	state.documentID = "native-print-test"
	state.setWordBlocks(word.WordBlocks())
	key, _, ok := currentWordLayoutKey(state)
	if !ok {
		t.Fatal("could not build Word layout key")
	}
	state.wordLayoutAcceptedKey = &key
	state.wordLayoutResult = &wordlayout.Result{
		DocumentID: key.DocumentID,
		Revision:   key.Revision,
		CanPreview: true,
		Pages:      []wordlayout.PageScene{{Index: 0, Clip: wordlayout.Rect{WidthPT: 595, HeightPT: 842}}},
	}
	app := &App{
		documentSessionState: state,
		activeTabID:          "native-print-test",
		nativePrinter:        printer,
		wordPrintResults:     make(chan wordPrintResult, 1),
		shutdown:             make(chan struct{}),
	}
	return app, state, word
}

func TestWordPrintDisabledWhenNativePrinterUnavailable(t *testing.T) {
	printer := &testWordPrinter{available: false, calls: make(chan printedWordPDF, 1)}
	app, _, _ := newNativePrintTestApp(t, printer)
	if app.wordToolbarActionEnabled(findWordFileTestAction(t, "print")) {
		t.Fatal("Word Print should be disabled when the platform has no native printer")
	}
}

func TestWordPrintRequiresWarningAndPreviewConfirmationsAndPrintsSamePageScene(t *testing.T) {
	printer := &testWordPrinter{available: true, calls: make(chan printedWordPDF, 1)}
	app, state, word := newNativePrintTestApp(t, printer)
	state.wordLayoutResult.Diagnostics = []wordlayout.Diagnostic{{
		Severity: wordlayout.DiagnosticWarning, Code: "font-fallback", Detail: "使用了替代字体",
	}}

	if !app.wordToolbarActionEnabled(findWordFileTestAction(t, "print")) {
		t.Fatal("Word Print should be enabled when a native printer is available")
	}
	app.executeWordToolbarAction("print")
	if !app.modal.Visible || app.modal.Title != "打印前确认版式提示" {
		t.Fatalf("warning modal visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("warning confirmation should begin PDF preparation")
	}
	var prepared wordPrintResult
	select {
	case prepared = <-app.wordPrintResults:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out preparing native print PDF")
	}
	app.applyWordPrintResult(prepared)
	if !app.modal.Visible || app.modal.Title != "打印预览已准备" {
		t.Fatalf("preview modal visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if state.wordViewMode != wordViewPrintLayout {
		t.Fatal("printing did not switch the document to its shared print page view")
	}
	select {
	case <-printer.calls:
		t.Fatal("native printer ran before the preview confirmation")
	default:
	}
	app.modal.OnAction("confirm")
	var call printedWordPDF
	select {
	case call = <-printer.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("native print adapter was not called after preview confirmation")
	}
	reader, err := gopdf.NewReader(call.data)
	if err != nil {
		t.Fatalf("printer received invalid PDF: %v", err)
	}
	if reader.NumPages() != 1 {
		t.Fatalf("printer received %d pages, want the one-page shared scene", reader.NumPages())
	}
	select {
	case result := <-app.wordPrintResults:
		app.applyWordPrintResult(result)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for native print completion")
	}
	if state.formatRunning.Load() || state.wordPrintRunning {
		t.Fatal("print completion left the Word session busy")
	}
	if !word.Dirty() || state.path != "source.docx" {
		t.Fatalf("printing changed source state: dirty=%v path=%q", word.Dirty(), state.path)
	}
}

func TestWordPrintCancelAtPreviewDoesNotInvokePrinter(t *testing.T) {
	printer := &testWordPrinter{available: true, calls: make(chan printedWordPDF, 1)}
	app, _, _ := newNativePrintTestApp(t, printer)
	app.executeWordToolbarAction("print")
	var prepared wordPrintResult
	select {
	case prepared = <-app.wordPrintResults:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out preparing native print PDF")
	}
	app.applyWordPrintResult(prepared)
	if !app.modal.Visible || app.modal.Title != "打印预览已准备" {
		t.Fatalf("preview modal visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("cancel")
	select {
	case <-printer.calls:
		t.Fatal("canceling preview unexpectedly invoked the native printer")
	default:
	}
	if app.status != "已取消打印" {
		t.Fatalf("cancel status = %q", app.status)
	}
}
