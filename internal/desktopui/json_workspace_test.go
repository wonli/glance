package desktopui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/format/jsondoc"
)

func TestJSONTreeCollapseSearchAndSelection(t *testing.T) {
	app := newTestAppWithJSON(t, `{"user":{"name":"林墨","roles":["admin"]},"ok":true}`)
	v := app.ensureJSONView()
	tree, err := jsondoc.ParseTree(app.jsonDoc.Text())
	if err != nil {
		t.Fatal(err)
	}
	v.accept(jsonTreeResult{text: tree.Source, tree: tree})
	v.collapsed[1] = true
	v.rebuildRows()
	if len(v.rows) != 4 {
		t.Fatalf("collapsed rows=%d; want root, user, ok, closing root", len(v.rows))
	}
	v.search.SetText("林墨")
	v.rebuildRows()
	if len(v.rows) != 5 {
		t.Fatalf("search should reveal matching child and ancestors: %+v", v.rows)
	}
	v.selectNode(2)
	if v.value.Text() != "林墨" {
		t.Fatalf("selected string draft=%q", v.value.Text())
	}
	v.value.SetText("中文输入")
	if err := app.commitJSONNodeValue(); err != nil {
		t.Fatal(err)
	}
	if app.jsonDoc.Text() != `{"user":{"name":"中文输入","roles":["admin"]},"ok":true}` {
		t.Fatalf("node edit lost untouched content: %s", app.jsonDoc.Text())
	}
}

func TestJSONTreeIgnoresStaleResultAndKeepsInvalidSource(t *testing.T) {
	app := newTestAppWithJSON(t, `{"ok":true}`)
	v := app.ensureJSONView()
	tree, _ := jsondoc.ParseTree(app.jsonDoc.Text())
	v.requested = `{"broken":`
	v.accept(jsonTreeResult{text: tree.Source, tree: tree})
	if v.tree != nil {
		t.Fatal("stale parse result installed")
	}
	app.jsonEditor.SetText(`{"broken":`)
	app.commitJSONSourceDraft()
	if app.jsonDoc.Text() != `{"broken":` || !app.isDirty() {
		t.Fatal("invalid intermediate source should stay dirty and recoverable")
	}
}

func TestJSONToolbarModesAndExpandLevels(t *testing.T) {
	app := newTestAppWithJSON(t, `{"user":{"name":"Glance"}}`)
	v := app.ensureJSONView()
	tree, _ := jsondoc.ParseTree(app.jsonDoc.Text())
	v.accept(jsonTreeResult{text: tree.Source, tree: tree})
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1440, 54))}
	jsonToolbarButton(app.documentSessionState, app, "collapse-all").Click()
	app.handleJSONToolbarEvents(gtx)
	if len(v.rows) != 1 {
		t.Fatalf("collapse all rows=%d", len(v.rows))
	}
	jsonToolbarButton(app.documentSessionState, app, "expand-all").Click()
	app.handleJSONToolbarEvents(gtx)
	if len(v.rows) != 5 {
		t.Fatalf("expand all rows=%d", len(v.rows))
	}
	jsonToolbarButton(app.documentSessionState, app, "mode-source").Click()
	app.handleJSONToolbarEvents(gtx)
	if v.mode != jsonModeSource {
		t.Fatal("source mode did not switch")
	}
}

func TestJSONContainerInspectorDoesNotLayoutWholeDocument(t *testing.T) {
	app := newTestAppWithJSON(t, `{"s":"\u0041","items":[1,2,3]}`)
	v := app.ensureJSONView()
	tree, _ := jsondoc.ParseTree(app.jsonDoc.Text())
	v.requested = tree.Source
	v.accept(jsonTreeResult{text: tree.Source, tree: tree})
	if !v.value.ReadOnly || v.value.Text() == tree.Source {
		t.Fatal("root inspector must use a bounded summary, never edit/layout the whole document")
	}
	v.selectNode(1)
	if err := app.commitJSONNodeValue(); err != nil {
		t.Fatal(err)
	}
	if app.jsonDoc.Text() != `{"s":"\u0041","items":[1,2,3]}` || app.isDirty() {
		t.Fatal("unchanged decoded string rewrote source")
	}
	v.requested = ""
	other, _ := jsondoc.ParseTree(`{"other":true}`)
	v.accept(jsonTreeResult{text: other.Source, tree: other})
	if v.parsed != tree.Source {
		t.Fatal("stale result changed the accepted source")
	}
	if v.tree == nil {
		t.Fatal("stale result should leave the last accepted tree untouched")
	}
}

func TestJSONConditionalMutationPreservesNewerSource(t *testing.T) {
	app := newTestAppWithJSON(t, `{"name":"original"}`)
	_ = app.jsonDoc.SetText(`{"name":"other window"}`)
	if app.applyJSONTextMutationIfSource(`{"name":"stale transform"}`, "formatted", `{"name":"original"}`) {
		t.Fatal("stale transformation accepted")
	}
	if app.jsonDoc.Text() != `{"name":"other window"}` {
		t.Fatal("newer cross-window text lost")
	}
}

func TestJSONTreeDoubleClickToggle(t *testing.T) {
	app := newTestAppWithJSON(t, `{"user":{"name":"林墨","roles":["admin"]},"ok":true}`)
	v := app.ensureJSONView()
	tree, err := jsondoc.ParseTree(app.jsonDoc.Text())
	if err != nil {
		t.Fatal(err)
	}
	v.accept(jsonTreeResult{text: tree.Source, tree: tree})

	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1440, 900))}
	app.layoutJSON(gtx)

	initRowCount := len(v.rows)
	if initRowCount != 9 {
		t.Fatalf("initial rows = %d, want 9", initRowCount)
	}

	// Single click row 1: selects node, does NOT collapse
	rowBtn := v.rowButtons[1]
	if rowBtn == nil {
		t.Fatal("rowButton 1 missing")
	}
	rowBtn.Click()
	app.handleJSONWorkspaceEvents(gtx)
	if v.selected != 1 {
		t.Fatalf("selected = %d, want 1", v.selected)
	}
	if v.isCollapsed(1) {
		t.Fatal("single click collapsed node 1")
	}
	if len(v.rows) != initRowCount {
		t.Fatalf("rows changed after single click: %d", len(v.rows))
	}

	// Double click row 1: toggles to collapsed!
	rowBtn.Click()
	rowBtn.Click()
	app.handleJSONWorkspaceEvents(gtx)
	if !v.isCollapsed(1) {
		t.Fatal("double click failed to collapse node 1")
	}
	if len(v.rows) >= initRowCount {
		t.Fatalf("rows after collapse = %d, want fewer than %d", len(v.rows), initRowCount)
	}

	// Double click row 1 again: toggles back to expanded!
	rowBtn.Click()
	rowBtn.Click()
	app.handleJSONWorkspaceEvents(gtx)
	if v.isCollapsed(1) {
		t.Fatal("second double click failed to expand node 1")
	}
	if len(v.rows) != initRowCount {
		t.Fatalf("rows after expand = %d, want %d", len(v.rows), initRowCount)
	}

	// Render layout again so closingButtons are populated
	app.layoutJSON(gtx)
	closingBtn := v.closingButtons[1]
	if closingBtn == nil {
		t.Fatal("closingButton 1 missing")
	}

	// Double click closing bracket row: collapses container!
	closingBtn.Click()
	closingBtn.Click()
	app.handleJSONWorkspaceEvents(gtx)
	if !v.isCollapsed(1) {
		t.Fatal("double click on closing row failed to collapse node 1")
	}
}

func TestJSONTreeButtonPruning(t *testing.T) {
	buttons := make(map[int]*widget.Clickable)
	for i := 0; i < 400; i++ {
		buttons[i] = new(widget.Clickable)
	}
	// Limit is 300, keep items 10, 20, 30
	pruneButtons(buttons, []int{10, 20, 30}, 300)
	if len(buttons) != 3 {
		t.Fatalf("pruned length = %d, want 3", len(buttons))
	}
	if buttons[10] == nil || buttons[20] == nil || buttons[30] == nil {
		t.Fatal("kept buttons were pruned")
	}
	if buttons[0] != nil || buttons[100] != nil {
		t.Fatal("unkept buttons were not pruned")
	}
}
