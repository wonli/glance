package desktopui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
)

func TestWordWebLayoutIsResponsiveAndRibbonTogglesIt(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)

	state := newDocumentSessionState("web-layout.docx")
	state.word = word
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-web-layout")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Web Layout should be enabled for a Word document")
	}
	if wordToolbarActionActive(state, action) {
		t.Fatal("Web Layout should be inactive initially")
	}

	app.executeWordToolbarAction(action.id)
	if !wordToolbarActionActive(state, action) {
		t.Fatal("Ribbon Web Layout action did not activate Web Layout")
	}
	if word.Dirty() {
		t.Fatal("switching to Web Layout changed DOCX dirty state")
	}

	semantics, bounds := wordWebLayoutSemanticsAndBounds(t, app, image.Pt(1440, 900))
	if !strings.Contains(semantics, "Word Web 文档区域") {
		t.Fatalf("Web Layout semantic region is missing: %s", semantics)
	}
	if strings.Contains(semantics, "Word 文档画布") || strings.Contains(semantics, "拖动标尺调整画布宽度") {
		t.Fatalf("Web Layout still exposes a paper canvas or paper resize handle: %s", semantics)
	}
	if strings.Contains(semantics, "Word 水平标尺") || strings.Contains(semantics, "Word 垂直标尺") {
		t.Fatalf("Web Layout still exposes page rulers: %s", semantics)
	}
	if bounds.Min.X != 0 || bounds.Dx() != 1440 {
		t.Fatalf("Web canvas bounds at 1440 dp = %v; want full viewport width", bounds)
	}
	_, narrowBounds := wordWebLayoutSemanticsAndBounds(t, app, image.Pt(760, 900))
	if narrowBounds.Min.X != 0 || narrowBounds.Dx() != 760 {
		t.Fatalf("Web canvas bounds at 760 dp = %v; want full available width", narrowBounds)
	}

	app.executeWordToolbarAction(action.id)
	if wordToolbarActionActive(state, action) {
		t.Fatal("Ribbon Web Layout action did not return to continuous view")
	}
	if word.Dirty() {
		t.Fatal("switching back from Web Layout changed DOCX dirty state")
	}

	state.wordViewMode = wordViewPrintLayout
	app.executeWordToolbarAction(action.id)
	if !wordToolbarActionActive(state, action) {
		t.Fatal("Ribbon Web Layout action did not enter Web Layout from Print Layout")
	}
	app.toggleWordPrintLayout()
	if state.wordViewMode != wordViewPrintLayout {
		t.Fatal("Print Layout action did not enter paginated view from Web Layout")
	}
	app.toggleWordPrintLayout()
	if state.wordViewMode == wordViewPrintLayout {
		t.Fatal("Print Layout action did not return to continuous view")
	}
	if word.Dirty() {
		t.Fatal("switching Word view modes changed DOCX dirty state")
	}
}

func TestWordStatusWebLayoutButtonTogglesView(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state := newDocumentSessionState("web-status-layout.docx")
	state.word = word
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := newWordWebLayoutContextWithSource(image.Pt(1440, wordStatusBarHeightDP), router.Source(), &ops)

	app.layoutWordStatusBar(gtx, state)
	state.wordStatusWebLayoutButton.Click()
	app.layoutWordStatusBar(gtx, state)
	if state.wordViewMode != wordViewWebLayout {
		t.Fatal("FsQrk Web Layout button did not enter Web Layout")
	}
	state.wordStatusWebLayoutButton.Click()
	app.layoutWordStatusBar(gtx, state)
	if state.wordViewMode != wordViewContinuous {
		t.Fatal("FsQrk Web Layout button did not return to continuous view")
	}
	if word.Dirty() {
		t.Fatal("using the FsQrk Web Layout button changed DOCX dirty state")
	}
}

func wordWebLayoutSemanticsAndBounds(t *testing.T, app *App, size image.Point) (string, image.Rectangle) {
	t.Helper()
	router := new(input.Router)
	var ops op.Ops
	gtx := newWordWebLayoutContextWithSource(size, router.Source(), &ops)
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	nodes := router.AppendSemantics(nil)
	var descriptions []string
	webBounds := image.Rectangle{}
	var visit func([]input.SemanticNode)
	visit = func(children []input.SemanticNode) {
		for _, node := range children {
			descriptions = append(descriptions, node.Desc.Description)
			if node.Desc.Description == "Word Web 文档区域" {
				webBounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(nodes)
	return strings.Join(descriptions, " "), webBounds
}

func newWordWebLayoutContext(size image.Point) layout.Context {
	router := new(input.Router)
	var ops op.Ops
	return newWordWebLayoutContextWithSource(size, router.Source(), &ops)
}

func newWordWebLayoutContextWithSource(size image.Point, source input.Source, ops *op.Ops) layout.Context {
	return layout.Context{
		Source: source, Ops: ops, Now: time.Now(),
		Constraints: layout.Constraints{Max: size},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
}
