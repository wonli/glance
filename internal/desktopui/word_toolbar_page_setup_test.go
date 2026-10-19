package desktopui

import (
	"archive/zip"
	"bytes"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordPageSetupDialogsAndCommands(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state.word = doc
	state.setWordBlocks(doc.WordBlocks())
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	orientAction := ribbonAction{id: "layout-orientation", label: "方向"}
	marginsAction := ribbonAction{id: "layout-margins", label: "页边距"}
	sizeAction := ribbonAction{id: "layout-size", label: "纸张大小"}

	if !app.wordToolbarActionEnabled(orientAction) || !app.wordToolbarActionEnabled(marginsAction) || !app.wordToolbarActionEnabled(sizeAction) {
		t.Fatal("expected page setup actions to be enabled")
	}

	// 1. Orientation Dialog and Command
	app.executeWordToolbarAction("layout-orientation")
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "页面方向" {
		t.Fatalf("expected modal dialog for layout-orientation, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("landscape")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after landscape action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-orientation command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-orientation command result")
	}

	layout := doc.WordLayoutData()
	if len(layout.Sections) == 0 || layout.Sections[0].PageSetup.Orientation != "landscape" {
		t.Fatalf("expected orientation landscape, got %#v", layout.Sections)
	}

	// 2. Margins Dialog and Command
	app.executeWordToolbarAction("layout-margins")
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "页边距设置" {
		t.Fatalf("expected modal dialog for layout-margins, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("narrow")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after narrow action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-margins command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-margins command result")
	}

	layout = doc.WordLayoutData()
	setup := layout.Sections[0].PageSetup
	if setup.MarginTopPT != 36 || setup.MarginLeftPT != 36 {
		t.Fatalf("expected narrow margins (36pt), got top=%f left=%f", setup.MarginTopPT, setup.MarginLeftPT)
	}

	// 3. Page Size Dialog and Command
	app.executeWordToolbarAction("layout-size")
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "纸张大小设置" {
		t.Fatalf("expected modal dialog for layout-size, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("letter")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after letter action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-size command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-size command result")
	}

	layout = doc.WordLayoutData()
	setup = layout.Sections[0].PageSetup
	// Since current orientation is landscape, width > height (792 x 612)
	if (math.Abs(setup.WidthPT-792) > 1.0 && math.Abs(setup.WidthPT-612) > 1.0) || (math.Abs(setup.HeightPT-792) > 1.0 && math.Abs(setup.HeightPT-612) > 1.0) {
		t.Fatalf("expected Letter page size dimensions, got width=%f height=%f", setup.WidthPT, setup.HeightPT)
	}

	// 4. Columns Dialog and Command
	columnsAction := ribbonAction{id: "layout-columns", label: "分栏"}
	if !app.wordToolbarActionEnabled(columnsAction) {
		t.Fatal("expected layout-columns action to be enabled")
	}
	app.executeWordToolbarAction("layout-columns")
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "分栏设置" {
		t.Fatalf("expected modal dialog for layout-columns, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("col2")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after col2 action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-columns command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-columns command result")
	}
	snap, err := doc.Snapshot()
	if err != nil {
		t.Fatalf("doc.Snapshot failed: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(snap), int64(len(snap)))
	if err != nil {
		t.Fatal(err)
	}
	var docXML string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			docXML = string(b)
			break
		}
	}
	if !strings.Contains(docXML, `w:cols`) || !strings.Contains(docXML, `w:num="2"`) {
		t.Fatalf("expected 2 columns in word/document.xml, got: %s", docXML)
	}
}

func TestWordLineNumbersDialogAndCommand(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("failed to create blank docx: %v", err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = cloneWordBlockSnapshot(doc.WordBlocks())
	state.wordToolbarTab = wordTabLayout

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
	}

	var lineNumAction ribbonAction
	for _, a := range wordToolbarActions(wordTabLayout) {
		if a.id == "layout-line-numbers" {
			lineNumAction = a
			break
		}
	}
	if lineNumAction.id == "" {
		t.Fatal("layout-line-numbers action not found in wordTabLayout")
	}

	if !app.wordToolbarActionEnabled(lineNumAction) {
		t.Fatal("layout-line-numbers should be enabled")
	}

	// 1. Show line numbers dialog -> Click "continuous"
	app.showWordLineNumbersDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "行号设置" {
		t.Fatalf("expected modal dialog for line numbers, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}

	app.modal.OnAction("continuous")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("set line numbers command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for line numbers command result")
	}

	readXML := func() string {
		snap, err := doc.Snapshot()
		if err != nil {
			t.Fatalf("doc.Snapshot failed: %v", err)
		}
		zr, err := zip.NewReader(bytes.NewReader(snap), int64(len(snap)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				rc, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				b, _ := io.ReadAll(rc)
				_ = rc.Close()
				return string(b)
			}
		}
		return ""
	}

	xml := readXML()
	if !strings.Contains(xml, `w:restart="continuous"`) {
		t.Fatalf("expected continuous line numbers in xml: %s", xml)
	}

	// 2. Clear line numbers
	app.showWordLineNumbersDialog()
	app.modal.OnAction("none")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("clear line numbers command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for clear line numbers command result")
	}

	xml = readXML()
	if strings.Contains(xml, "lnNumType") {
		t.Fatalf("expected lnNumType removed from xml: %s", xml)
	}
}

func TestWordSelectAllAndPageColor(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	textEditor, _ := any(doc).(document.WordTextEditor)
	blocks := doc.WordBlocks()
	for _, b := range blocks {
		if b.Paragraph != nil {
			for _, in := range b.Paragraph.Inlines {
				if in.Kind == document.WordTextInline && in.Text != nil && in.Text.Editable {
					_ = textEditor.ReplaceTextRun(in.Text.ID, "Hello Word Document")
					break
				}
			}
		}
	}

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = doc.WordBlocks()
	app := &App{documentSessionState: state}

	// 1. Verify toolbar action enabled
	if !app.wordToolbarActionEnabled(ribbonAction{id: "select"}) {
		t.Fatal("select action should be enabled")
	}
	if !app.wordToolbarActionEnabled(ribbonAction{id: "design-page-color"}) {
		t.Fatal("design-page-color action should be enabled")
	}

	// 2. Test Select All
	app.selectAllWordText()
	if app.statusText() != "已全选文档正文" {
		t.Fatalf("unexpected status: %q", app.statusText())
	}
	if state.wordPageTextSelection == nil {
		t.Fatal("expected active wordPageTextSelection after select all")
	}
	selectedText, ok := wordPageSelectedText(state)
	if !ok || len(selectedText) == 0 {
		t.Fatalf("expected non-empty selected text, got ok=%v text=%q", ok, selectedText)
	}

	// 3. Test Page Color Dialog & Parchment
	app.showWordPageColorDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "页面颜色" {
		t.Fatalf("expected 页面颜色 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("parchment")
	if state.wordBackgroundColor == nil {
		t.Fatal("expected wordBackgroundColor to be set")
	}
	if state.wordBackgroundColor.R != 0xFA || state.wordBackgroundColor.G != 0xF8 || state.wordBackgroundColor.B != 0xF5 {
		t.Fatalf("unexpected wordBackgroundColor: %+v", *state.wordBackgroundColor)
	}
	editor, _ := any(doc).(document.WordPageBackgroundEditor)
	bg, err := editor.GetPageBackgroundColor()
	if err != nil || bg != "FAF8F5" {
		t.Fatalf("expected doc background FAF8F5, got %q (err=%v)", bg, err)
	}

	// 4. Clear Page Color (White)
	app.showWordPageColorDialog()
	app.modal.OnAction("white")
	if state.wordBackgroundColor != nil {
		t.Fatalf("expected wordBackgroundColor nil after white, got %+v", *state.wordBackgroundColor)
	}
	bg, _ = editor.GetPageBackgroundColor()
	if bg != "" {
		t.Fatalf("expected empty background color after white, got %q", bg)
	}
}

func TestWordToolbar_ColumnsDialogAndLayout(t *testing.T) {
	state := newDocumentSessionState("columns_test.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 2),
	}

	app.showWordColumnsDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "分栏设置" {
		t.Fatalf("modal not shown: visible=%v, title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}

	action := app.modal.Actions[1] // col2
	if action.ID != "col2" {
		t.Fatalf("action 1 = %q, want col2", action.ID)
	}
	app.modal.OnAction(action.ID)
	if app.modal.Visible {
		t.Fatal("modal should be hidden after action")
	}

	select {
	case res := <-app.wordCommandResults:
		if res.err != nil {
			t.Fatalf("command failed: %v", res.err)
		}
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command result")
	}

	layoutReader, ok := any(word).(document.WordLayoutReader)
	if !ok {
		t.Fatal("word does not implement WordLayoutReader")
	}
	data := layoutReader.WordLayoutData()
	if len(data.Sections) == 0 || data.Sections[0].PageSetup.Columns != 2 {
		t.Fatalf("expected 2 columns in section, got %+v", data.Sections)
	}
	for _, diag := range data.Unsupported {
		if diag.Code == "columns" {
			t.Fatalf("unexpected unsupported columns diagnostic: %+v", diag)
		}
	}
}
