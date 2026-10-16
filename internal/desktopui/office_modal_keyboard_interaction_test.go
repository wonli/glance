package desktopui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/application"
)

func TestEscapeCancelsEveryModalLayout(t *testing.T) {
	tests := []struct {
		name       string
		wantCancel bool
		setup      func(*App, *bool)
	}{
		{
			name:       "message",
			wantCancel: true,
			setup: func(app *App, cancelCalled *bool) {
				app.modal.Show("提示", "说明", func(action string) {
					if action == "cancel" {
						*cancelCalled = true
						app.modal.Hide()
					}
				}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
			},
		},
		{
			name:       "close action",
			wantCancel: true,
			setup: func(app *App, cancelCalled *bool) {
				app.modal.Show("信息", "", func(action string) {
					if action == "close" {
						*cancelCalled = true
						app.modal.Hide()
					}
				}, modalDialogActionSpec{ID: "close", Label: "关闭"})
			},
		},
		{
			name:       "input",
			wantCancel: true,
			setup: func(app *App, cancelCalled *bool) {
				app.modal.ShowInput("输入", "说明", "草稿", "值", func(action string) {
					if action == "cancel" {
						*cancelCalled = true
						app.modal.Hide()
					}
				}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
				app.modal.FocusInputRequested = true
			},
		},
		{
			name:       "dual input",
			wantCancel: true,
			setup: func(app *App, cancelCalled *bool) {
				app.modal.ShowDualInput("查找和替换", "", "词", "查找内容", "语", "替换为", func(action string) {
					if action == "cancel" {
						*cancelCalled = true
						app.modal.Hide()
					}
				}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
			},
		},
		{
			name:       "form",
			wantCancel: true,
			setup: func(app *App, cancelCalled *bool) {
				app.modal.ShowForm("文档信息", "", []string{"标题"}, []string{"标题"}, []string{"报告"}, func(action string) {
					if action == "cancel" {
						*cancelCalled = true
						app.modal.Hide()
					}
				}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
			},
		},
		{
			name:       "custom layout",
			wantCancel: true,
			setup: func(app *App, cancelCalled *bool) {
				app.modal.ShowCustom("自定义", "说明", 440, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(120, 40)}
				}, func(action string) {
					if action == "cancel" {
						*cancelCalled = true
						app.modal.Hide()
					}
				}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
			},
		},
		{
			name: "search input",
			setup: func(app *App, _ *bool) {
				app.modal.ShowInput("查找和替换", "", "词语", "查找内容", nil)
				app.modal.SearchReplace = true
				app.modal.SearchReplaceHasReplace = true
				app.modal.FocusInputRequested = true
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &App{theme: material.NewTheme()}
			cancelCalled := false
			test.setup(app, &cancelCalled)
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{
					Ops:         &ops,
					Source:      router.Source(),
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Exact(image.Pt(820, 500)),
				}
				app.handleOverlayEscape(gtx)
				app.layoutModalDialog(gtx)
				router.Frame(&ops)
			}
			frame()
			router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
			frame()
			if app.modal.Visible {
				t.Fatal("Escape left the modal visible")
			}
			if cancelCalled != test.wantCancel {
				t.Fatalf("cancel callback called=%t; want %t", cancelCalled, test.wantCancel)
			}
		})
	}
}

func TestSearchReplaceCloseButtonClosesModal(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	app.modal.ShowInput("查找和替换", "", "词语", "查找内容", func(action string) {
		if action == "close" {
			app.modal.Hide()
		}
	})
	app.modal.SearchReplace = true
	app.modal.SearchReplaceHasReplace = true
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(820, 500)),
		}
		app.handleOverlayEscape(gtx)
		app.layoutModalDialog(gtx)
		router.Frame(&ops)
	}
	frame()
	pos := f32.Pt(650, 104)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pos})
	frame()
	frame()
	if app.modal.Visible {
		t.Fatal("clicking the close button left the modal visible")
	}
}

func TestSearchReplaceEnterUsesTheFocusedFieldAction(t *testing.T) {
	tests := []struct {
		name       string
		focusInput bool
		wantAction string
	}{
		{name: "find field", focusInput: true, wantAction: "find-next"},
		{name: "replace field", focusInput: false, wantAction: "replace"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &App{theme: material.NewTheme()}
			var actions []string
			app.modal.ShowDualInput("查找和替换", "", "needle", "查找内容", "replacement", "替换为", func(action string) {
				actions = append(actions, action)
			})
			app.modal.SearchReplace = true
			app.modal.SearchReplaceHasReplace = true
			app.modal.SearchReplaceReplaceTab = true
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{
					Ops:         &ops,
					Source:      router.Source(),
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Exact(image.Pt(820, 500)),
				}
				app.layoutModalDialog(gtx)
				router.Frame(&ops)
			}
			frame()
			editor := &app.modal.SecondInput
			if test.focusInput {
				editor = &app.modal.Input
			}
			router.Source().Execute(key.FocusCmd{Tag: editor})
			frame()
			if !router.Source().Focused(editor) {
				t.Fatal("requested search field did not receive focus")
			}
			router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
			frame()
			if len(actions) != 1 || actions[0] != test.wantAction {
				t.Fatalf("Enter actions = %v; want [%s]", actions, test.wantAction)
			}
		})
	}
}

func TestSearchReplaceShiftEnterUsesPreviousMatchAction(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	var actions []string
	app.modal.ShowDualInput("查找和替换", "", "needle", "查找内容", "replacement", "替换为", func(action string) {
		actions = append(actions, action)
	})
	app.modal.SearchReplace = true
	app.modal.SearchReplaceHasReplace = true
	app.modal.SearchReplaceReplaceTab = true
	app.modal.FocusInputRequested = true
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(820, 500)),
		}
		app.layoutModalDialog(gtx)
		router.Frame(&ops)
	}
	frame()
	if !router.Source().Focused(&app.modal.Input) {
		t.Fatal("requested search field did not receive focus")
	}
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModShift})
	frame()
	if len(actions) != 1 || actions[0] != "find-previous" {
		t.Fatalf("Shift+Enter actions = %v; want [find-previous]", actions)
	}
}

func TestEscapeCancelsCompositionBeforeClosingInputModal(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	app.modal.ShowInput("输入", "", "", "值", nil)
	app.modal.FocusInputRequested = true
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(820, 500)),
		}
		app.handleOverlayEscape(gtx)
		app.layoutModalDialog(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Queue(key.CompositionEvent{Start: 0, End: 1}, key.EditEvent{Text: "中"})
	frame()
	if !editorHasActiveComposition(&app.modal.Input) {
		t.Fatal("input editor did not retain the active Chinese composition")
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if !app.modal.Visible {
		t.Fatal("Escape closed the modal while the IME composition was active")
	}
	router.Queue(key.CompositionEvent{Start: -1, End: -1})
	frame()
	if editorHasActiveComposition(&app.modal.Input) {
		t.Fatal("input editor did not clear the active Chinese composition")
	}
	if !router.Source().Focused(&app.modal.Input) {
		t.Fatal("input editor lost keyboard focus after composition ended")
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Release})
	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if app.modal.Visible {
		t.Fatal("Escape did not close the modal after composition ended")
	}
}

func TestEscapeDismissesSymbolPicker(t *testing.T) {
	app := &App{theme: material.NewTheme(), wordSymbolPickerVisible: true}
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source()}
		app.handleOverlayEscape(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if app.wordSymbolPickerVisible {
		t.Fatal("Escape left the symbol picker open")
	}
}

func TestEscapeDismissesWordToolbarColorPicker(t *testing.T) {
	for _, test := range []struct {
		name string
		set  func(*documentSessionState)
	}{
		{name: "font color", set: func(state *documentSessionState) { state.wordToolbarColorPicker = true }},
		{name: "highlight color", set: func(state *documentSessionState) { state.wordToolbarHighlightPicker = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newDocumentSessionState("sample.docx")
			test.set(state)
			app := &App{documentSessionState: state}
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source()}
				app.handleOverlayEscape(gtx)
				router.Frame(&ops)
			}
			frame()
			router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
			frame()
			if state.wordToolbarColorPicker || state.wordToolbarHighlightPicker {
				t.Fatal("Escape left the Word color picker open")
			}
		})
	}
}

func TestEscapeRejectsAgentConfirmationBeforeUnderlyingDialog(t *testing.T) {
	app := newTestConfirmationApp(t)
	card := stageTestConfirmation(app, application.ConfirmationInfo{
		ConfirmationID: "escape-priority",
		CapabilityID:   "word.text.insert",
		CapabilityName: "插入文档",
	})
	cancelCalled := false
	app.modal.Show("底层对话框", "", func(action string) {
		if action == "cancel" {
			cancelCalled = true
			app.modal.Hide()
		}
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1440, 900)),
		}
		app.layout(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if card.info.Status != application.ConfirmationRejected {
		t.Fatalf("Escape status=%q; want rejected", card.info.Status)
	}
	if !app.modal.Visible || cancelCalled {
		t.Fatal("Escape passed through the topmost confirmation and canceled the underlying dialog")
	}
}

func TestEscapeDismissesWorkspaceAndRibbonPopupsInLayerOrder(t *testing.T) {
	app := &App{showNewMenu: true, wordVersionHistoryVisible: true}
	state := &documentSessionState{}
	state.officeRibbonMenu.kind = officeRibbonMenuWord
	state.toolbarPicker = pickerAlignment
	app.documentSessionState = state
	var router input.Router
	var ops op.Ops
	pressEscape := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source()}
		app.handleOverlayEscape(gtx)
		router.Frame(&ops)
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
		ops.Reset()
		gtx = layout.Context{Ops: &ops, Source: router.Source()}
		app.handleOverlayEscape(gtx)
		router.Frame(&ops)
		router.Queue(key.Event{Name: key.NameEscape, State: key.Release})
		ops.Reset()
		gtx = layout.Context{Ops: &ops, Source: router.Source()}
		app.handleOverlayEscape(gtx)
		router.Frame(&ops)
	}
	pressEscape()
	if app.wordVersionHistoryVisible || !app.showNewMenu {
		t.Fatal("Escape did not close the topmost version history panel first")
	}
	pressEscape()
	if app.showNewMenu {
		t.Fatal("Escape did not close the new menu after version history")
	}
	pressEscape()
	if state.officeRibbonMenu.kind != officeRibbonMenuClosed || state.toolbarPicker != pickerAlignment {
		t.Fatal("Escape did not close only the topmost ribbon dropdown")
	}
	pressEscape()
	if state.toolbarPicker != pickerNone {
		t.Fatal("Escape did not close the toolbar picker")
	}
}
