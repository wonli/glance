package desktopui

import (
	"testing"

	"glance/internal/document"
)

func TestAgentSettingsDialogMasksAPIKeyAndDoesNotPrefillIt(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)

	app.showAgentSettings(false)
	if !app.modal.Visible || app.modal.Title != "模型设置" {
		t.Fatalf("settings dialog visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if app.agentUI.apiKeyInput.Mask == 0 {
		t.Fatal("API key editor is not masked")
	}
	if app.agentUI.apiKeyInput.Text() != "" {
		t.Fatal("settings dialog unexpectedly populated the API key editor")
	}
	if app.agentUI.settingsInfo == "" {
		t.Fatal("settings dialog omitted the system credential storage explanation")
	}
}

func TestAgentViewContextIncludesWindowDocumentAndSpreadsheetSelection(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(app.desktopRuntime.shutdown)
	state := app.documentSessionState
	state.documentID = "doc-current"
	state.wordRevisionAtomic.Store(17)
	state.book = struct{ document.Spreadsheet }{}
	app.windowID = 9
	app.sheet = "Sheet2"
	app.selectedCell = "C4"
	state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 1}, Active: CellPosition{Row: 3, Column: 2}}
	state.selectionSet = true

	view := app.agentViewContext()
	if view.WindowID != 9 || view.DocumentID != "doc-current" || view.Revision != 17 {
		t.Fatalf("view identity = %+v", view)
	}
	if view.Selection == nil || view.Selection.TargetType != "spreadsheet.cell" || view.Selection.TargetID != "C4" || view.Selection.Sheet != "Sheet2" {
		t.Fatalf("spreadsheet selection = %+v", view.Selection)
	}
	if got := view.Selection.Range; got == nil || got.StartRow != 2 || got.StartColumn != 2 || got.EndRow != 4 || got.EndColumn != 3 {
		t.Fatalf("spreadsheet selection range = %+v", got)
	}
}

func TestAgentViewContextTracksActiveDocumentAndRevision(t *testing.T) {
	app := newAppWithRuntime(nil, newDesktopRuntime())
	t.Cleanup(app.desktopRuntime.shutdown)
	app.windowID = 9

	first := newDocumentSessionView(newSharedDocumentSession(""), 9)
	first.documentID = "doc-first"
	first.book = struct{ document.Spreadsheet }{}
	first.wordRevisionAtomic.Store(3)
	first.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 1}, Active: CellPosition{Row: 1, Column: 1}}
	first.selectionSet = true
	app.documentSessionState = first
	app.activeTabID = "doc-first"
	app.sessions["doc-first"] = first
	app.sheet = "Sheet1"
	app.selectedCell = "A1"
	firstView := app.agentViewContext()

	second := newDocumentSessionView(newSharedDocumentSession(""), 9)
	second.documentID = "doc-second"
	second.book = struct{ document.Spreadsheet }{}
	second.wordRevisionAtomic.Store(8)
	second.selection = CellSelection{Anchor: CellPosition{Row: 4, Column: 2}, Active: CellPosition{Row: 5, Column: 3}}
	second.selectionSet = true
	app.activateSession("doc-second", second)
	app.sheet = "Payroll"
	app.selectedCell = "C5"

	view := app.agentViewContext()
	if view.WindowID != 9 || view.DocumentID != "doc-second" || view.Revision != 8 {
		t.Fatalf("view after active document switch = %+v", view)
	}
	if view.Selection == nil || view.Selection.TargetID != "C5" || view.Selection.Sheet != "Payroll" {
		t.Fatalf("selection after active document switch = %+v", view.Selection)
	}
	if got := view.Selection.Range; got == nil || got.StartRow != 5 || got.StartColumn != 3 || got.EndRow != 6 || got.EndColumn != 4 {
		t.Fatalf("range after active document switch = %+v", got)
	}

	second.wordRevisionAtomic.Store(9)
	if refreshed := app.agentViewContext(); refreshed.Revision != 9 {
		t.Fatalf("view did not refresh the active document revision: %+v", refreshed)
	}
	if firstView.DocumentID != "doc-first" || firstView.Revision != 3 {
		t.Fatalf("previously captured view was mutated: %+v", firstView)
	}
}
