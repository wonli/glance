package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
)

func TestWordViewRulerActionTogglesHorizontalRuler(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("ruler.docx")
	state.word = word
	state.wordLayoutPendingKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	action := findWordViewTestAction(t, "view-ruler")
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("View > Ruler should be enabled for a Word document")
	}
	if !wordToolbarActionActive(state, action) {
		t.Fatal("Word rulers should be visible by default")
	}
	assertWordRulerSemantics(t, app, true)

	app.executeWordToolbarAction(action.id)
	if wordToolbarActionActive(state, action) {
		t.Fatal("View > Ruler should be inactive after hiding rulers")
	}
	assertWordRulerSemantics(t, app, false)

	app.executeWordToolbarAction(action.id)
	if !wordToolbarActionActive(state, action) {
		t.Fatal("View > Ruler should be active after restoring rulers")
	}
	assertWordRulerSemantics(t, app, true)
}

func assertWordRulerSemantics(t *testing.T, app *App, visible bool) {
	t.Helper()
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source: router.Source(), Ops: &ops, Now: time.Now(),
		Constraints: layout.Constraints{Max: image.Pt(1440, 900)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	want := map[string]bool{"Word 水平标尺": false}
	verticalVisible := false
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if _, ok := want[node.Desc.Description]; ok && !node.Desc.Bounds.Empty() {
				want[node.Desc.Description] = true
			}
			if node.Desc.Description == "Word 垂直标尺" && !node.Desc.Bounds.Empty() {
				verticalVisible = true
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if verticalVisible {
		t.Error("Word view still exposes a vertical ruler")
	}
	for label, found := range want {
		if found != visible {
			t.Errorf("ruler %q visible=%v; want %v", label, found, visible)
		}
	}
}
