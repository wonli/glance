package desktopui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
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
