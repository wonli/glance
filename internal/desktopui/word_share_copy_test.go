package desktopui

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordShareActionOpensDOCXCopyPickerAndCancelPreservesSession(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(true)
	state := newDocumentSessionState("source.docx")
	state.word = word
	state.wordAutoSave = true
	app := &App{
		documentSessionState: state,
		activeTabID:          "tab-current",
		fileCommandResults:   make(chan fileCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
	pickerCalls := make(chan struct {
		kind   fileCommandKind
		format string
	}, 1)
	app.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls <- struct {
			kind   fileCommandKind
			format string
		}{kind: kind, format: format}
		return fileCommandResult{kind: kind}
	})

	share := findWordFileTestAction(t, "share")
	if !app.wordToolbarActionEnabled(share) {
		t.Fatal("Word File > Share should be enabled for an open DOCX")
	}
	app.executeWordToolbarAction("share")
	select {
	case call := <-pickerCalls:
		if call.kind != commandShareCopy || call.format != "docx" {
			t.Fatalf("share picker request = %#v; want DOCX copy picker", call)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Word File > Share did not open the DOCX copy picker")
	}
	select {
	case result := <-app.fileCommandResults:
		app.fileCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("share picker cancellation was not delivered")
	}

	if app.status != "已取消共享副本导出" {
		t.Fatalf("share cancellation status = %q", app.status)
	}
	if app.word != word || app.path != "source.docx" || state.path != "source.docx" || !word.Dirty() || !state.wordAutoSave {
		t.Fatalf("cancel changed source session: active=%p path=%q statePath=%q dirty=%v autosave=%v", app.word, app.path, state.path, word.Dirty(), state.wordAutoSave)
	}
}

func TestWordShareExportsAtomicDOCXCopyWithoutChangingSourceSession(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.docx")
	destinationPath := filepath.Join(t.TempDir(), "shared-copy.docx")
	word := newWordShareFixture(t)
	if err := word.SaveAsDocument(sourcePath); err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	wordRun := firstEditableWordRun(t, word)
	if err := any(word).(document.WordTextEditor).ReplaceTextRun(wordRun, "共享版本 updated"); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState(sourcePath)
	state.word = word
	state.wordAutoSave = true
	state.wordRevision = 7
	state.wordRevisionAtomic.Store(7)
	app := newWordShareTestApp(state)
	app.wordShareCopyRequest = &wordShareCopyPickerRequest{state: state, revision: 7, draftEpoch: state.wordDraftEpoch.Load()}
	app.startWordShareCopyFile(app.wordShareCopyRequest, destinationPath)
	waitWordShareResult(t, app)

	if state.path != sourcePath || !word.Dirty() || !state.wordAutoSave {
		t.Fatalf("export changed active source session: path=%q dirty=%v autosave=%v", state.path, word.Dirty(), state.wordAutoSave)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, sourceBefore) {
		t.Fatalf("source DOCX changed: read error=%v equal=%v", err, bytes.Equal(got, sourceBefore))
	}

	copyDoc, err := docx.Open(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDoc.Close()
	if got := firstWordParagraphText(t, copyDoc); got != "共享版本 updated" {
		t.Fatalf("shared copy text = %q", got)
	}
	if got := readZipPart(t, destinationPath, "customXml/item1.xml"); got != "preserve this package part" {
		t.Fatalf("untouched package part = %q", got)
	}
}

func TestWordShareRejectsSourceOverwriteAndStalePickerChoice(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.docx")
	word := newWordShareFixture(t)
	if err := word.SaveAsDocument(sourcePath); err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState(sourcePath)
	state.word = word
	state.wordRevisionAtomic.Store(4)
	app := newWordShareTestApp(state)

	app.startWordShareCopyFile(&wordShareCopyPickerRequest{state: state, revision: 4}, sourcePath)
	if app.status != "共享副本不能覆盖当前 DOCX" {
		t.Fatalf("same-source protection status = %q", app.status)
	}
	if state.formatRunning.Load() {
		t.Fatal("same-source selection started an export")
	}

	stalePath := filepath.Join(t.TempDir(), "stale.docx")
	app.startWordShareCopyFile(&wordShareCopyPickerRequest{state: state, revision: 3}, stalePath)
	if state.formatRunning.Load() {
		t.Fatal("stale revision started an export")
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale picker wrote target: stat error=%v", err)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, sourceBefore) {
		t.Fatalf("source DOCX changed after rejected choices: read error=%v", err)
	}
}

func TestWordShareWriteFailureKeepsSourceDirtyAndClearsBusyState(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.docx")
	word := newWordShareFixture(t)
	if err := word.SaveAsDocument(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := any(word).(document.WordTextEditor).ReplaceTextRun(firstEditableWordRun(t, word), "dirty"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState(sourcePath)
	state.word = word
	state.wordAutoSave = true
	state.wordRevisionAtomic.Store(2)
	app := newWordShareTestApp(state)
	app.startWordShareCopyFile(&wordShareCopyPickerRequest{state: state, revision: 2}, filepath.Join(t.TempDir(), "missing", "copy.docx"))
	waitWordShareResult(t, app)
	if !word.Dirty() || state.path != sourcePath || !state.wordAutoSave || state.formatRunning.Load() {
		t.Fatalf("failed export changed source state: path=%q dirty=%v autosave=%v busy=%v", state.path, word.Dirty(), state.wordAutoSave, state.formatRunning.Load())
	}
	if !strings.Contains(app.status, "导出共享副本失败：") {
		t.Fatalf("write failure status = %q", app.status)
	}
}

func TestWordShareFlushesPendingTextDraftBeforeOpeningPicker(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	runID := firstEditableWordRun(t, word)
	state := newDocumentSessionState("source.docx")
	state.word = word
	state.wordRunCommitted = map[string]string{runID: ""}
	captureWordTextDraft(state, runID, "uncommitted draft")
	state.wordRevisionAtomic.Store(1)
	app := newWordShareTestApp(state)
	app.activeTabID = "current"
	app.documentGeneration = 1
	pickerCalls := make(chan struct{}, 1)
	app.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		if kind != commandShareCopy || format != "docx" {
			t.Errorf("picker request = kind %v, format %q", kind, format)
		}
		pickerCalls <- struct{}{}
		return fileCommandResult{kind: kind}
	})

	app.requestWordShareCopy()
	if !state.wordFlushRequested || state.wordDeferredToolbarAction != "share" {
		t.Fatalf("share did not defer for pending text: flush=%v deferred=%q", state.wordFlushRequested, state.wordDeferredToolbarAction)
	}
	select {
	case <-pickerCalls:
		t.Fatal("share picker opened before pending text was committed")
	default:
	}
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("pending Word text draft was not committed")
	}
	select {
	case <-pickerCalls:
	case <-time.After(3 * time.Second):
		t.Fatal("share picker did not open after pending text commit")
	}
	if got := firstWordParagraphText(t, word); got != "uncommitted draft" {
		t.Fatalf("committed draft text = %q", got)
	}
	select {
	case result := <-app.fileCommandResults:
		app.fileCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("share picker result was not delivered")
	}
	if app.status != "已取消共享副本导出" || word.Dirty() == false {
		t.Fatalf("share cancel status=%q dirty=%v", app.status, word.Dirty())
	}
}

func newWordShareTestApp(state *documentSessionState) *App {
	return &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 1),
		wordShareCopyResults: make(chan wordShareCopyResult, 1),
		fileCommandResults:   make(chan fileCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
}

func waitWordShareResult(t *testing.T, app *App) {
	t.Helper()
	select {
	case result := <-app.wordShareCopyResults:
		app.applyWordShareCopyResult(result)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Word shared copy export")
	}
}

func newWordShareFixture(t *testing.T) *docx.Document {
	t.Helper()
	blank, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := blank.Snapshot()
	_ = blank.Close()
	if err != nil {
		t.Fatal(err)
	}
	snapshot = addZipPart(t, snapshot, "customXml/item1.xml", []byte("preserve this package part"))
	word, err := docx.OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = word.Close() })
	return word
}

func addZipPart(t *testing.T, packageData []byte, name string, data []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	reader, err := zip.NewReader(bytes.NewReader(packageData), int64(len(packageData)))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range reader.File {
		entry, err := writer.CreateHeader(&part.FileHeader)
		if err != nil {
			t.Fatal(err)
		}
		r, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(entry, r); err != nil {
			_ = r.Close()
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func readZipPart(t *testing.T, packagePath, name string) string {
	t.Helper()
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, part := range reader.File {
		if part.Name != name {
			continue
		}
		r, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		return string(data)
	}
	t.Fatalf("package is missing part %q", name)
	return ""
}

func firstEditableWordRun(t *testing.T, word *docx.Document) string {
	t.Helper()
	blocks := word.WordBlocks()
	for _, block := range blocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.Editable {
				return inline.Text.ID
			}
		}
	}
	t.Fatal("fixture has no editable Word text run")
	return ""
}

func firstWordParagraphText(t *testing.T, word *docx.Document) string {
	t.Helper()
	blocks := word.WordBlocks()
	for _, block := range blocks {
		if block.Paragraph != nil {
			var text strings.Builder
			for _, inline := range block.Paragraph.Inlines {
				if inline.Text != nil {
					text.WriteString(inline.Text.Text)
				}
			}
			return text.String()
		}
	}
	t.Fatal("document has no Word paragraph")
	return ""
}
