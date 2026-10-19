package desktopui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordListToolbarActionsApplyAndToggleOffLists(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-list.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	for _, step := range []struct {
		action string
		kind   document.WordListKind
		marker string
	}{
		{action: "bullet-list", kind: document.WordListBullet, marker: "•"},
		{action: "numbered-list", kind: document.WordListNumbered, marker: "1."},
	} {
		action := findWordTestAction(t, step.action)
		if !app.wordToolbarActionEnabled(action) {
			t.Fatalf("Word list action %q should be enabled for a selected paragraph", step.action)
		}
		app.executeWordToolbarAction(step.action)
		if !state.formatRunning.Load() {
			t.Fatalf("%s did not start a Word list command", step.action)
		}
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", step.action)
		}
		updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
		if !ok || updated.ListKind != step.kind || updated.ListMarker != step.marker {
			t.Fatalf("%s paragraph list = %#v, found=%v; want %q marker %q", step.action, updated, ok, step.kind, step.marker)
		}
		if !wordToolbarActionActive(state, action) {
			t.Errorf("%s is not shown as active after applying the list", step.action)
		}
		app.executeWordToolbarAction(step.action)
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("toggle off %s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out toggling off %s", step.action)
		}
		updated, ok = selectedWordParagraph(state.wordBlocks, paragraph.ID)
		if !ok || updated.ListKind != document.WordListNone || updated.ListMarker != "" {
			t.Fatalf("toggle off %s left list %#v, found=%v", step.action, updated, ok)
		}
	}
}

func TestWordListToolbarDisablesUnsupportedNestedLists(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-list.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	blocks := wordBlocksForState(state)
	blocks[0].Paragraph.ListKind = document.WordListUnsupported
	blocks[0].Paragraph.ListLevel = 1
	state.wordBlocks = blocks
	app := &App{documentSessionState: state}
	for _, id := range []string{"bullet-list", "numbered-list"} {
		if app.wordToolbarActionEnabled(findWordTestAction(t, id)) {
			t.Errorf("list action %q should be disabled for unsupported nested numbering", id)
		}
	}
}

func TestWordIndentToolbarMovesListThroughNestedLevels(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-nested-list.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	apply := func(action string) {
		t.Helper()
		app.executeWordToolbarAction(action)
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", action)
		}
	}
	apply("bullet-list")
	apply("increase-indent")
	updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || updated.ListKind != document.WordListBullet || updated.ListLevel != 1 || updated.ListMarker != "◦" {
		t.Fatalf("increase list level = %#v, found=%v; want bullet level 1 marker ◦", updated, ok)
	}
	apply("decrease-indent")
	updated, ok = selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || updated.ListKind != document.WordListBullet || updated.ListLevel != 0 || updated.ListMarker != "•" {
		t.Fatalf("decrease list level = %#v, found=%v; want bullet level 0 marker •", updated, ok)
	}
	apply("decrease-indent")
	updated, ok = selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || updated.ListKind != document.WordListNone || updated.ListMarker != "" {
		t.Fatalf("decrease at list level 0 = %#v, found=%v; want plain paragraph", updated, ok)
	}
}

func TestWordIndentToolbarActionsStepLeftIndentAndKeepRightIndent(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	textEditor, ok := any(word).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph indent editing")
	}
	const initialLeft, right int64 = 914400, 228600
	if err := textEditor.SetParagraphIndents(paragraph.ID, initialLeft, right); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-indent.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	for _, step := range []struct {
		action string
		want   int64
	}{
		{action: "decrease-indent", want: initialLeft - 457200},
		{action: "increase-indent", want: initialLeft},
	} {
		action := findWordTestAction(t, step.action)
		if !app.wordToolbarActionEnabled(action) {
			t.Fatalf("Word indent action %q should be enabled for the selected paragraph", step.action)
		}
		app.executeWordToolbarAction(step.action)
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", step.action)
		}
		updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
		if !ok || updated.LeftIndentEMU != step.want || updated.RightIndentEMU != right {
			t.Fatalf("%s indents = %d/%d, found=%v; want %d/%d EMU", step.action, updated.LeftIndentEMU, updated.RightIndentEMU, ok, step.want, right)
		}
	}
	output := filepath.Join(t.TempDir(), "word-indent.docx")
	if err := word.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := docx.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedParagraph := reopened.WordBlocks()[0].Paragraph
	if reopenedParagraph.LeftIndentEMU != initialLeft || reopenedParagraph.RightIndentEMU != right {
		t.Fatalf("saved/reopened paragraph indents = %d/%d; want %d/%d EMU", reopenedParagraph.LeftIndentEMU, reopenedParagraph.RightIndentEMU, initialLeft, right)
	}
}

func TestWordToolbarParagraphSpacingBeforeAndAfter(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	state.setWordBlocks(word.WordBlocks())
	paragraph := allWordParagraphs(state.wordBlocks)[0]
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	beforeAction := ribbonAction{id: "layout-before", label: "段前"}
	afterAction := ribbonAction{id: "layout-after", label: "段后"}

	state.selectedWordParagraphID = ""
	if app.wordToolbarActionEnabled(beforeAction) || app.wordToolbarActionEnabled(afterAction) {
		t.Fatal("paragraph spacing should be disabled when no paragraph is selected")
	}

	state.selectedWordParagraphID = paragraph.ID
	if !app.wordToolbarActionEnabled(beforeAction) || !app.wordToolbarActionEnabled(afterAction) {
		t.Fatal("paragraph spacing should be enabled when paragraph is selected")
	}

	// Test layout-before
	app.executeWordToolbarAction("layout-before")
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "段前间距" {
		t.Fatalf("expected modal dialog for layout-before, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("12")
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after confirm")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-before failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-before command result")
	}

	updated := allWordParagraphs(state.wordBlocks)[0]
	if updated.Layout.BeforePT != 12 {
		t.Fatalf("expected BeforePT=12, got %g", updated.Layout.BeforePT)
	}

	// Test layout-after
	app.executeWordToolbarAction("layout-after")
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "段后间距" {
		t.Fatalf("expected modal dialog for layout-after, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("18")
	app.modal.OnAction("confirm")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-after failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-after command result")
	}

	updated = allWordParagraphs(state.wordBlocks)[0]
	if updated.Layout.AfterPT != 18 {
		t.Fatalf("expected AfterPT=18, got %g", updated.Layout.AfterPT)
	}

	// Test undo restores AfterPT
	app.applyWordHistory(true)
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("undo failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for undo result")
	}
	updated = allWordParagraphs(state.wordBlocks)[0]
	if updated.Layout.AfterPT != 0 {
		t.Fatalf("expected AfterPT restored to 0 after undo, got %g", updated.Layout.AfterPT)
	}

	// Test invalid input
	app.executeWordToolbarAction("layout-before")
	app.modal.Input.SetText("2000")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.statusText(), "请输入 0 到 1584 磅之间的数值") {
		t.Fatalf("expected out-of-range status error, got %q", app.statusText())
	}
}

func TestWordParagraphIndentDialogAndCommand(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state.word = doc
	state.setWordBlocks(doc.WordBlocks())
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	blocks := doc.WordBlocks()
	if len(blocks) == 0 || blocks[0].Paragraph == nil {
		t.Fatal("expected at least one paragraph block")
	}
	state.selectedWordParagraphID = blocks[0].Paragraph.ID
	if len(blocks[0].Paragraph.Inlines) > 0 && blocks[0].Paragraph.Inlines[0].Text != nil {
		state.selectedWordRunID = blocks[0].Paragraph.Inlines[0].Text.ID
	}

	indentAction := ribbonAction{id: "layout-indent", label: "缩进"}
	if !app.wordToolbarActionEnabled(indentAction) {
		t.Fatal("expected layout-indent action to be enabled")
	}

	// 1. Open dialog and set custom indent
	app.executeWordToolbarAction("layout-indent")
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "段落缩进" {
		t.Fatalf("expected modal dialog for layout-indent, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	if len(app.modal.FormInputs) < 2 {
		t.Fatalf("expected at least 2 form inputs, got %d", len(app.modal.FormInputs))
	}
	app.modal.FormInputs[0].SetText("72")
	app.modal.FormInputs[1].SetText("36")
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after confirm action")
	}

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-indent command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-indent command result")
	}

	p := doc.WordBlocks()[0].Paragraph
	const wantLeftEMU, wantRightEMU int64 = 72 * 12700, 36 * 12700
	if p.LeftIndentEMU != wantLeftEMU || p.RightIndentEMU != wantRightEMU {
		t.Fatalf("expected paragraph indents %d/%d EMU, got %d/%d EMU", wantLeftEMU, wantRightEMU, p.LeftIndentEMU, p.RightIndentEMU)
	}

	// 2. Open dialog and click "none" (reset to 0)
	app.executeWordToolbarAction("layout-indent")
	if !app.modal.Visible {
		t.Fatal("modal should be visible")
	}
	app.modal.OnAction("none")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after none action")
	}

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-indent none command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-indent none command result")
	}

	p = doc.WordBlocks()[0].Paragraph
	if p.LeftIndentEMU != 0 || p.RightIndentEMU != 0 {
		t.Fatalf("expected indents reset to 0, got %d/%d EMU", p.LeftIndentEMU, p.RightIndentEMU)
	}
}

func TestWordDesignSpacingPresets(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state := newDocumentSessionState("design.docx")
	state.word = doc
	state.wordBlocks = cloneWordBlockSnapshot(doc.WordBlocks())
	state.selectedWordParagraphID = doc.WordBlocks()[0].Paragraph.ID
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
		shutdown:             make(chan struct{}),
	}

	var compactAction, spaciousAction ribbonAction
	for _, a := range wordToolbarActions(wordTabDesign) {
		if a.id == "design-compact" {
			compactAction = a
		} else if a.id == "design-spacious" {
			spaciousAction = a
		}
	}
	if compactAction.id == "" || spaciousAction.id == "" {
		t.Fatal("design actions not found in wordTabDesign")
	}

	if !app.wordToolbarActionEnabled(compactAction) {
		t.Fatal("design-compact should be enabled")
	}
	if !app.wordToolbarActionEnabled(spaciousAction) {
		t.Fatal("design-spacious should be enabled")
	}

	// 1. Apply compact spacing
	app.executeWordToolbarAction("design-compact")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("design-compact failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for design-compact result")
	}

	p := doc.WordBlocks()[0].Paragraph
	if p.Layout.BeforePT != 0 || p.Layout.AfterPT != 4 {
		t.Fatalf("expected compact spacing 0/4 pt, got %g/%g pt", p.Layout.BeforePT, p.Layout.AfterPT)
	}

	// 2. Apply spacious spacing
	app.executeWordToolbarAction("design-spacious")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("design-spacious failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for design-spacious result")
	}

	p = doc.WordBlocks()[0].Paragraph
	if p.Layout.BeforePT != 0 || p.Layout.AfterPT != 12 {
		t.Fatalf("expected spacious spacing 0/12 pt, got %g/%g pt", p.Layout.BeforePT, p.Layout.AfterPT)
	}
}
