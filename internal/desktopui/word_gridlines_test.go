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
	"glance/internal/wordlayout"
)

func TestWordViewGridlinesAreDisplayOnlyInFlowAndPageViews(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)

	state := newDocumentSessionState("gridlines.docx")
	state.word = word
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-gridlines")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Gridlines should be enabled for a Word document")
	}
	if wordToolbarActionActive(state, action) {
		t.Fatal("gridlines should be hidden by default")
	}
	if got := wordGridlineSemantics(app); strings.Contains(got, "Word 网格线") {
		t.Fatal("Word gridlines are visible before the action is enabled")
	}

	app.executeWordToolbarAction(action.id)
	if !wordToolbarActionActive(state, action) {
		t.Fatal("View > Gridlines should be active after enabling gridlines")
	}
	if got := wordGridlineSemantics(app); !strings.Contains(got, "Word 网格线") {
		t.Fatalf("continuous-flow view has no visible Word gridlines: %s", got)
	}
	if word.Dirty() {
		t.Fatal("toggling view gridlines changed DOCX dirty state")
	}

	state.wordViewMode = wordViewPrintLayout
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Index: 0, Clip: wordlayout.Rect{WidthPT: 300, HeightPT: 300}}}}
	if got := wordGridlineSemantics(app); !strings.Contains(got, "Word 网格线") {
		t.Fatalf("print-layout view has no visible Word gridlines: %s", got)
	}
	if word.Dirty() {
		t.Fatal("viewing Word gridlines changed DOCX dirty state")
	}

	app.executeWordToolbarAction(action.id)
	if wordToolbarActionActive(state, action) {
		t.Fatal("View > Gridlines should be inactive after disabling gridlines")
	}
	if got := wordGridlineSemantics(app); strings.Contains(got, "Word 网格线") {
		t.Fatalf("Word gridlines remain visible after disabling: %s", got)
	}
}

func wordGridlineSemantics(app *App) string {
	var descriptions []string
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			descriptions = append(descriptions, node.Desc.Description)
			visit(node.Children)
		}
	}
	visit(wordFlowSemanticTree(app))
	return strings.Join(descriptions, " ")
}

func wordFlowSemanticTree(app *App) []input.SemanticNode {
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source: router.Source(), Ops: &ops, Now: time.Now(),
		Constraints: layout.Constraints{Max: image.Pt(1440, 900)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	return router.AppendSemantics(nil)
}
