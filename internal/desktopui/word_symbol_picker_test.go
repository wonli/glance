package desktopui

import (
	"testing"
	"time"

	"gioui.org/widget"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordInsertSymbolRequiresEditableSelectedRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("symbols.docx")
	state.word = word
	app := &App{documentSessionState: state}
	action := findWordInsertSymbolAction(t)
	if app.wordToolbarActionEnabled(action) {
		t.Fatal("Insert > Symbol should be disabled without a selected editable run")
	}

	paragraph := word.WordBlocks()[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("Insert > Symbol should be enabled for an editable body-text run")
	}
}

func TestWordInsertSymbolIsDisabledForTableCellRuns(t *testing.T) {
	word := syntheticWordTableDocument(t)
	defer word.Close()
	blocks := word.WordBlocks()
	var tableRunID string
	for _, row := range blocks[1].Table.Rows {
		for _, cell := range row.Cells {
			for _, paragraph := range cell.Paragraphs {
				for _, inline := range paragraph.Inlines {
					if inline.Text != nil {
						tableRunID = inline.Text.ID
						break
					}
				}
			}
		}
	}
	state := newDocumentSessionState("table-symbols.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(blocks)
	state.selectedWordRunID = tableRunID
	app := &App{documentSessionState: state}
	if tableRunID == "" || app.wordToolbarActionEnabled(findWordInsertSymbolAction(t)) {
		t.Fatalf("Insert > Symbol should be disabled for table run %q", tableRunID)
	}
}

func findWordInsertSymbolAction(t *testing.T) ribbonAction {
	t.Helper()
	for _, action := range wordToolbarActions(wordTabInsert) {
		if action.id == "insert-symbol" {
			return action
		}
	}
	t.Fatal("Insert > Symbol action not found")
	return ribbonAction{}
}

func TestWordInsertSymbolAtCaretAndPlacesCaretAfterUnicodeRune(t *testing.T) {
	app, state, runID := newWordSymbolTestApp(t, "A🙂B")
	editor := state.wordRunEditors[runID]
	editor.SetCaret(2, 2)
	state.selectedWordRunID = runID
	app.executeWordToolbarAction("insert-symbol")
	if !app.wordSymbolPickerVisible || app.wordSymbolStart != 2 || app.wordSymbolEnd != 2 {
		t.Fatalf("symbol picker visible=%v target=%q range=%d:%d; want current caret 2:2", app.wordSymbolPickerVisible, app.wordSymbolRunID, app.wordSymbolStart, app.wordSymbolEnd)
	}
	app.chooseWordSymbol(0)
	waitForWordSymbolStatus(t, app, state, "已插入符号 ©。")
	assertWordSymbolTextAndCaret(t, state, runID, "A🙂©B", 3)
}

func TestWordInsertSymbolReplacesOnlySelectionAndPreservesFormatting(t *testing.T) {
	app, state, runID := newWordSymbolTestApp(t, "prefix target suffix")
	if err := any(state.word).(document.WordTextEditor).SetTextRunStyle(runID, document.WordTextFormatPatch{Bold: wordSymbolBoolPointer(true)}); err != nil {
		t.Fatal(err)
	}
	state.setWordBlocks(cloneWordBlockSnapshot(state.word.WordBlocks()))
	state.word.SetDirty(false)
	run, found := selectedWordRun(state.wordBlocks, runID)
	if !found {
		t.Fatal("selected run disappeared")
	}
	editor := wordRunEditor(state, run)
	editor.SetText("prefix target suffix")
	editor.SetCaret(7, 13)
	state.selectedWordRunID = runID
	app.executeWordToolbarAction("insert-symbol")
	if app.wordSymbolStart != 7 || app.wordSymbolEnd != 13 {
		t.Fatalf("picker selection range=%d:%d; want 7:13", app.wordSymbolStart, app.wordSymbolEnd)
	}
	app.chooseWordSymbol(6)
	waitForWordSymbolStatus(t, app, state, "已插入符号 €。")
	assertWordSymbolTextAndCaret(t, state, runID, "prefix € suffix", 8)
	run, ok := selectedWordRun(state.wordBlocks, runID)
	if !ok || !run.Style.Bold {
		t.Fatalf("replacement did not preserve run formatting: found=%v style=%#v", ok, run.Style)
	}
}

func TestWordInsertSymbolFailureKeepsTextAndSelection(t *testing.T) {
	app, state, runID := newWordSymbolTestApp(t, "unchanged")
	revision := state.wordRevisionAtomic.Load()
	if !app.runWordSymbolInsertion(runID, state.selectedWordParagraphID, 100, 100, revision, state.wordDraftEpoch.Load(), wordSymbolChoices[0]) {
		t.Fatal("failed-range insertion did not start")
	}
	waitForWordSymbolFailure(t, app, state)
	if got, found := selectedWordRun(state.wordBlocks, runID); !found || got.Text != "unchanged" {
		t.Fatalf("failed symbol insertion changed text: %#v", got)
	}
	if start, end := state.wordRunEditors[runID].Selection(); start != 0 || end != 0 {
		t.Fatalf("failed insertion changed selection to %d:%d", start, end)
	}
}

func TestWordInsertSymbolRejectsTargetChangedWhilePickerIsOpen(t *testing.T) {
	app, state, runID := newWordSymbolTestApp(t, "stable")
	app.executeWordToolbarAction("insert-symbol")
	state.wordDraftEpoch.Add(1)
	app.chooseWordSymbol(0)
	if app.statusText() != "文档内容或编辑草稿已变化，请提交后重新打开符号面板" {
		t.Fatalf("stale picker status = %q", app.statusText())
	}
	if state.formatRunning.Load() {
		t.Fatal("stale picker left a document operation active")
	}
	if run, found := selectedWordRun(state.wordBlocks, runID); !found || run.Text != "stable" {
		t.Fatalf("stale picker changed document text: %#v", run)
	}
}

func TestWordInsertSymbolPickerCancelDoesNotEdit(t *testing.T) {
	app, state, runID := newWordSymbolTestApp(t, "keep me")
	state.wordRunEditors[runID].SetCaret(4, 4)
	app.executeWordToolbarAction("insert-symbol")
	app.closeWordSymbolPicker()
	if app.wordSymbolPickerVisible || state.word.Dirty() {
		t.Fatalf("cancel changed picker/document state: visible=%v dirty=%v", app.wordSymbolPickerVisible, state.word.Dirty())
	}
	if run, found := selectedWordRun(state.wordBlocks, runID); !found || run.Text != "keep me" {
		t.Fatalf("cancel changed text: %#v", run)
	}
}

func newWordSymbolTestApp(t *testing.T, text string) (*App, *documentSessionState, string) {
	t.Helper()
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = word.Close() })
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	if err := word.ReplaceTextRun(runID, text); err != nil {
		t.Fatal(err)
	}
	word.SetDirty(false)
	blocks := cloneWordBlockSnapshot(word.WordBlocks())
	state := newDocumentSessionState("symbols.docx")
	state.word = word
	state.wordBlocks = blocks
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	state.wordRunEditors = make(map[string]*widget.Editor)
	run, found := selectedWordRun(blocks, runID)
	if !found {
		t.Fatal("created document has no editable run")
	}
	editor := wordRunEditor(state, run)
	editor.SetText(text)
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
		shutdown:             make(chan struct{}),
	}
	return app, state, runID
}

func assertWordSymbolTextAndCaret(t *testing.T, state *documentSessionState, runID, want string, caret int) {
	t.Helper()
	run, found := selectedWordRun(state.wordBlocks, runID)
	if !found || run.Text != want {
		t.Fatalf("inserted text = %#v; want %q", run, want)
	}
	if start, end := state.wordRunEditors[runID].Selection(); start != caret || end != caret {
		t.Fatalf("caret after insertion = %d:%d; want %d:%d", start, end, caret, caret)
	}
}

func waitForWordSymbolStatus(t *testing.T, app *App, state *documentSessionState, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if app.statusText() == want && !state.formatRunning.Load() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for symbol completion; status=%q running=%v", app.statusText(), state.formatRunning.Load())
}

func waitForWordSymbolFailure(t *testing.T, app *App, state *documentSessionState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if app.feedback.kind == feedbackError && app.feedback.detail != "" && !state.formatRunning.Load() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for expected insertion failure; status=%q running=%v", app.statusText(), state.formatRunning.Load())
}

func wordSymbolBoolPointer(value bool) *bool { return &value }
