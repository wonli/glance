package desktopui

import (
	"testing"

	"gioui.org/widget"

	"glance/internal/format/docx"
)

func TestDocumentViewsShareSessionButOwnGioState(t *testing.T) {
	shared := newSharedDocumentSession("draft.docx")
	shared.documentID = "doc-1"
	shared.word = &docx.Document{}
	shared.wordRevision = 7
	shared.wordRevisionAtomic.Store(7)
	shared.publishedRevision.Store(7)
	shared.wordRunCommitted = map[string]string{"run-1": "committed"}
	shared.wordRunDrafts = map[string]string{"run-1": "shared draft"}

	first := newDocumentSessionView(shared, 1)
	second := newDocumentSessionView(shared, 2)
	if first.sharedDocumentSession != second.sharedDocumentSession || first.sharedDocumentSession != shared {
		t.Fatal("document views do not point to the same shared session")
	}
	if first.windowID == second.windowID {
		t.Fatalf("window IDs = %d and %d; want distinct IDs", first.windowID, second.windowID)
	}
	if first.word != second.word || first.word != shared.word {
		t.Fatal("document views do not share the Word adapter")
	}
	if first.wordRevision != second.wordRevision || first.wordRevisionAtomic.Load() != second.wordRevisionAtomic.Load() {
		t.Fatalf("shared revisions = %d/%d and atomic %d/%d", first.wordRevision, second.wordRevision, first.wordRevisionAtomic.Load(), second.wordRevisionAtomic.Load())
	}
	first.formatRunning.Store(true)
	if !second.formatRunning.Load() {
		t.Fatal("document operation state is not shared")
	}
	first.wordRunDrafts["run-1"] = "updated shared draft"
	if got := second.wordRunDrafts["run-1"]; got != "updated shared draft" {
		t.Fatalf("second view draft = %q; want shared update", got)
	}
	first.wordRevision = 8
	first.wordRevisionAtomic.Store(8)
	if second.wordRevision != 8 || second.wordRevisionAtomic.Load() != 8 {
		t.Fatalf("second view revision = %d/%d; want 8/8", second.wordRevision, second.wordRevisionAtomic.Load())
	}
	first.closed.Store(true)
	if second.closed.Load() || shared.finalized.Load() {
		t.Fatal("closing one view finalized the shared session or closed its sibling view")
	}

	if &first.wordEditor == &second.wordEditor || &first.wordListState == &second.wordListState || &first.wordPageListState == &second.wordPageListState || &first.wordToolbarTabList == &second.wordToolbarTabList || &first.wordToolbarActionList == &second.wordToolbarActionList {
		t.Fatal("document views share persistent Gio widgets")
	}
	first.wordListState.Position.First = 3
	first.wordPageListState.Position.First = 2
	first.wordLayoutZoom = 1.5
	first.wordViewMode = wordViewWebLayout
	first.wordToolbarTab = wordTabView
	if second.wordListState.Position.First != 0 || second.wordPageListState.Position.First != 0 || second.wordLayoutZoom == 1.5 || second.wordViewMode == wordViewWebLayout || second.wordToolbarTab == wordTabView {
		t.Fatal("changing one view's scroll, zoom, mode, or Ribbon tab changed the other view")
	}

	firstEditor := &widget.Editor{}
	first.wordRunEditors["run-1"] = firstEditor
	if _, exists := second.wordRunEditors["run-1"]; exists {
		t.Fatal("run editor maps are shared between document views")
	}
	firstEditor.SetText("local editor")
	firstEditor.SetCaret(2, 5)
	secondEditor := &widget.Editor{}
	second.wordRunEditors["run-1"] = secondEditor
	if first.wordRunEditors["run-1"] == second.wordRunEditors["run-1"] {
		t.Fatal("run editor widget is shared between document views")
	}
	if start, end := firstEditor.Selection(); start != 2 || end != 5 {
		t.Fatalf("first editor selection = %d:%d; want 2:5", start, end)
	}
	if start, end := secondEditor.Selection(); start != 0 || end != 0 {
		t.Fatalf("second editor selection = %d:%d; want 0:0", start, end)
	}

	first.wordToolbarActionButtons["save"] = &widget.Clickable{}
	if _, exists := second.wordToolbarActionButtons["save"]; exists {
		t.Fatal("toolbar clickable maps are shared between document views")
	}
}
