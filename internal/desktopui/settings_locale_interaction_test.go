package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/i18n"
)

func layoutBasicSettingsTestFrame(a *App, scale float32) layout.Dimensions {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC),
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Constraints{Max: image.Pt(int(1000*scale), int(760*scale))},
	}
	return a.layoutBasicSettingsPage(gtx)
}

func TestLanguageMenuRowsAlignBelowSelector(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		a := newAppWithRuntime(nil, newDesktopRuntime())
		defer a.desktopRuntime.shutdown()
		a.applyLocaleSelection(i18n.SelectionEnglish)
		var router input.Router
		var ops op.Ops
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(int(620*scale), int(240*scale))}}
			a.processLanguageMenuClicks(gtx)
			a.layoutLanguageSettingCard(gtx)
			router.Frame(&ops)
		}
		click := func(x, y float32) {
			position := f32.Pt(x*scale, y*scale)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: position, Buttons: pointer.ButtonPrimary})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
			frame()
		}
		frame()
		for i, option := range supportedLanguageOptions() {
			click(620-128, 41)
			if !a.agentUI.languageMenuOpen {
				t.Fatal("selector did not open")
			}
			click(620-128, float32(82+5+19+i*38))
			if a.agentUI.languageMenuOpen || a.localeSnapshot().Selection != option.Selection {
				t.Fatalf("scale=%v: menu option %d is not below the selector", scale, i)
			}
		}
	}
}

func TestBasicSettingsIsDefaultSettingsTab(t *testing.T) {
	a := newAppWithRuntime(nil, newDesktopRuntime())
	a.openAgentSettingsPage()
	if a.agentUI.settingsPageTab != "basic" {
		t.Fatalf("default settings tab = %q, want basic", a.agentUI.settingsPageTab)
	}
	if a.settingsPageTabButton("basic") != a.settingsPageTabButton("basic") {
		t.Fatal("basic tab button is not stable across frames")
	}
	a.selectSettingsPageTab("providers")
	if a.agentUI.settingsPageTab != "providers" {
		t.Fatal("provider settings are not directly selectable")
	}
	a.homeSection = homeSectionRecent
	a.openAgentSettingsPage()
	if a.agentUI.settingsPageTab != "basic" {
		t.Fatal("reopening Settings did not return to Basic")
	}
	a.selectSettingsPageTab("mcp")
	if a.agentUI.settingsPageTab != "mcp" {
		t.Fatal("MCP settings are not directly selectable")
	}
	a.selectSettingsPageTab("basic")
	if a.agentUI.settingsPageTab != "basic" {
		t.Fatal("basic settings are not directly selectable")
	}
}

func TestLanguageMenuSelectsAllSupportedModes(t *testing.T) {
	a := newAppWithRuntime(nil, newDesktopRuntime())
	a.openAgentSettingsPage()
	options := supportedLanguageOptions()
	for index, option := range options {
		a.agentUI.languageSelector.Click()
		layoutBasicSettingsTestFrame(a, 1)
		if !a.agentUI.languageMenuOpen {
			t.Fatal("language menu did not open")
		}
		a.agentUI.languageOptions[index].Click()
		layoutBasicSettingsTestFrame(a, 1)
		state := a.localeSnapshot()
		if state.Selection != option.Selection {
			t.Fatalf("option %q selected %q, want %q", option.MessageID, state.Selection, option.Selection)
		}
		if option.Selection != i18n.SelectionSystem && state.Resolved != i18n.Locale(option.Selection) {
			t.Fatalf("option %q resolved to %q", option.MessageID, state.Resolved)
		}
		if a.agentUI.languageMenuOpen {
			t.Fatalf("language menu stayed open after selecting %q", option.MessageID)
		}
	}
}

func TestLanguageSelectionPreservesDocumentAndEditorState(t *testing.T) {
	a := newAppWithRuntime(nil, newDesktopRuntime())
	a.documentID = "draft-document"
	a.selectedCell = "C7"
	a.documentSessionState.cellEditor.SetText("尚未提交的草稿")
	a.documentSessionState.cellEditor.SetCaret(5, 5)
	editor := &a.documentSessionState.cellEditor
	a.modal.Visible = true
	a.modal.Title = "Open dialog"
	a.modal.Input.SetText("dialog draft")
	a.selectLocale(i18n.SelectionEnglish)

	if a.documentID != "draft-document" || a.selectedCell != "C7" {
		t.Fatalf("document state changed: id=%q cell=%q", a.documentID, a.selectedCell)
	}
	if &a.documentSessionState.cellEditor != editor || editor.Text() != "尚未提交的草稿" {
		t.Fatal("locale selection replaced or changed the editor draft")
	}
	if start, end := editor.Selection(); start != 5 || end != 5 {
		t.Fatalf("editor caret changed: %d..%d", start, end)
	}
	if !a.modal.Visible || a.messageText(a.modal.TitleRef, a.modal.Title) != "Open dialog" || a.modal.Input.Text() != "dialog draft" {
		t.Fatal("locale selection replaced the open dialog or its draft")
	}
}

func TestLanguageMenuContainsOnlySupportedOptions(t *testing.T) {
	options := supportedLanguageOptions()
	want := []i18n.Selection{i18n.SelectionSystem, i18n.SelectionChinese, i18n.SelectionEnglish}
	if len(options) != len(want) {
		t.Fatalf("language menu options = %d, want %d", len(options), len(want))
	}
	for index, selection := range want {
		if options[index].Selection != selection {
			t.Errorf("option %d selection = %q, want %q", index, options[index].Selection, selection)
		}
	}
}
