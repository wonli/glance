package desktopui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/format/jsondoc"
	"glance/internal/workspace"
)

func TestJSONToolbarActionsAndIcons(t *testing.T) {
	required := map[string]string{
		"format":    "braces",
		"minify":    "minimize-2",
		"sort-keys": "arrow-up-down",
		"validate":  "circle-check",
	}

	found := make(map[string]bool)
	for _, action := range jsonToolbarActions {
		if iconName, ok := required[action.id]; ok {
			found[action.id] = true
			if action.icon != iconName {
				t.Errorf("action %s icon = %s; want %s", action.id, action.icon, iconName)
			}
			img, ok := jsonToolbarActionIcon(action, true)
			if !ok || img == nil {
				t.Errorf("failed to load icon for action %s (%s)", action.id, action.icon)
			}
		}
	}

	for id := range required {
		if !found[id] {
			t.Errorf("missing required action: %s", id)
		}
	}
}

func TestJSONToolbarFormatOperation(t *testing.T) {
	app := newTestAppWithJSON(t, `{"b":2,"a":1}`)
	app.formatJSONDocument()

	expected := "{\n  \"b\": 2,\n  \"a\": 1\n}\n"
	if app.jsonDoc.Text() != expected {
		t.Fatalf("expected formatted text %q, got %q", expected, app.jsonDoc.Text())
	}
	if app.jsonEditor.Text() != expected {
		t.Fatalf("expected editor text %q, got %q", expected, app.jsonEditor.Text())
	}
	if app.status != "已格式化 JSON" {
		t.Fatalf("unexpected status: %s", app.status)
	}

	// Error handling with invalid JSON
	_ = app.jsonDoc.SetText(`{"broken":`)
	app.jsonEditor.SetText(`{"broken":`)
	app.formatJSONDocument()
	if !strings.Contains(app.status, "格式化失败") {
		t.Fatalf("expected failure status, got: %s", app.status)
	}
	if app.jsonDoc.Text() != `{"broken":` {
		t.Fatalf("invalid text should not be overwritten on error")
	}
}

func TestJSONToolbarMinifyOperation(t *testing.T) {
	app := newTestAppWithJSON(t, "{\n  \"b\": 2,\n  \"a\": 1\n}\n")
	app.minifyJSONDocument()

	expected := `{"b":2,"a":1}`
	if app.jsonDoc.Text() != expected {
		t.Fatalf("expected minified text %q, got %q", expected, app.jsonDoc.Text())
	}
	if app.jsonEditor.Text() != expected {
		t.Fatalf("expected editor text %q, got %q", expected, app.jsonEditor.Text())
	}
	if app.status != "已压缩 JSON" {
		t.Fatalf("unexpected status: %s", app.status)
	}
}

func TestJSONToolbarSortKeysOperation(t *testing.T) {
	app := newTestAppWithJSON(t, `{"z": 10, "b": 2, "a": {"k": 9, "c": 8}}`)
	app.sortJSONKeysDocument()

	expected := "{\n  \"a\": {\n    \"c\": 8,\n    \"k\": 9\n  },\n  \"b\": 2,\n  \"z\": 10\n}\n"
	if app.jsonDoc.Text() != expected {
		t.Fatalf("expected sorted text %q, got %q", expected, app.jsonDoc.Text())
	}
	if app.status != "已对 JSON 键排序" {
		t.Fatalf("unexpected status: %s", app.status)
	}
}

func TestJSONToolbarValidateOperation(t *testing.T) {
	app := newTestAppWithJSON(t, `{"valid": true}`)
	app.validateJSONDocument()
	if app.status != "JSON 格式正确" {
		t.Fatalf("expected valid status, got: %s", app.status)
	}

	_ = app.jsonDoc.SetText(`{"invalid":`)
	app.jsonEditor.SetText(`{"invalid":`)
	app.validateJSONDocument()
	if !strings.Contains(app.status, "验证失败") {
		t.Fatalf("expected validation failure, got: %s", app.status)
	}
}

func TestJSONLayoutRender(t *testing.T) {
	app := newTestAppWithJSON(t, `{"name": "Glance", "ok": true}`)
	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Constraints: layout.Constraints{
			Min: image.Pt(800, 600),
			Max: image.Pt(800, 600),
		},
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := app.layoutJSON(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("expected positive layout dimensions, got %#v", dims)
	}
}

func newTestAppWithJSON(t *testing.T, initialText string) *App {
	t.Helper()
	app := New(nil, "")
	shared := newSharedDocumentSession("test.json")
	doc := jsondoc.NewBlank()
	_ = doc.SetText(initialText)
	doc.SetDirty(false)
	shared.jsonDoc = doc
	state := newDocumentSessionView(shared, 0)
	state.jsonEditor.SetText(initialText)
	app.documentSessionState = state
	app.tabMeta = make(map[string]workspace.Tab)
	return app
}
