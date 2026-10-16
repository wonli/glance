package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image"
	"testing"
	"time"
)

func TestFontPickerSearchBlankEdgesFocusAndChineseInput(t *testing.T) {
	for _, y := range []float32{2, 18, 34} {
		t.Run(image.Pt(0, int(y)).String(), func(t *testing.T) {
			a := New(nil, "")
			a.initFontPicker()
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(388, 48)}}
				a.layoutFontPickerSearch(gtx)
				router.Frame(&ops)
			}
			frame()
			point := f32.Pt(190, y)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: point, Buttons: pointer.ButtonPrimary})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
			frame()
			frame()
			if !router.Source().Focused(&a.fontPicker.search) {
				t.Fatal("search padding did not focus editor")
			}
			router.Queue(key.EditEvent{Text: "中文"})
			frame()
			if a.fontPicker.search.Text() != "中文" {
				t.Fatal("lost Chinese input")
			}
		})
	}
}

func TestFontPickerEscapeDoesNotDismissActiveIMEComposition(t *testing.T) {
	state := newDocumentSessionState("x.xlsx")
	state.book = &toolbarTestBook{editable: true}
	state.sheet = "Sheet1"
	state.selectionSet = true
	app := &App{theme: material.NewTheme(), documentSessionState: state}
	app.initFontPicker()
	app.fontPicker.open = true
	app.fontPicker.focusRequested = true
	app.fontPicker.target = fontPickerTarget{state: state, sheet: state.sheet, selection: state.selection, revision: state.wordRevisionAtomic.Load()}
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(800, 700)}}
		app.processFontPickerEvents(gtx)
		app.layoutFontPicker(gtx)
		router.Frame(&ops)
	}
	frame()
	frame()
	if !router.Source().Focused(&app.fontPicker.search) {
		t.Fatal("font picker search did not receive focus")
	}
	router.Queue(key.CompositionEvent{Start: 0, End: 1}, key.EditEvent{Text: "中"})
	frame()
	if !editorHasActiveComposition(&app.fontPicker.search) {
		t.Fatal("font picker search did not retain the active IME composition")
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if !app.fontPicker.open {
		t.Fatal("Escape dismissed the font picker before the IME composition ended")
	}
	router.Queue(key.CompositionEvent{Start: -1, End: -1})
	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Release})
	frame()
	if editorHasActiveComposition(&app.fontPicker.search) || !router.Source().Focused(&app.fontPicker.search) {
		t.Fatalf("composition end state = active %t, focused %t", editorHasActiveComposition(&app.fontPicker.search), router.Source().Focused(&app.fontPicker.search))
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	if app.fontPicker.open {
		t.Fatalf("Escape did not dismiss the font picker after the composition ended (composition active=%t, search focused=%t)", editorHasActiveComposition(&app.fontPicker.search), router.Source().Focused(&app.fontPicker.search))
	}
}

func TestFontPickerPanelBlankDoesNotActivateUnderlyingButton(t *testing.T) {
	a := New(nil, "")
	a.initFontPicker()
	s := newDocumentSessionState("x.xlsx")
	s.book = &toolbarTestBook{editable: true}
	s.sheet = "Sheet1"
	s.selectionSet = true
	a.documentSessionState = s
	a.fontPicker.open = true
	a.fontPicker.target = fontPickerTarget{state: s, sheet: s.sheet, selection: s.selection, revision: s.wordRevisionAtomic.Load()}
	var router input.Router
	var ops op.Ops
	var background widget.Clickable
	clicked := false
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(800, 700), Max: image.Pt(800, 700)}}
		if background.Clicked(gtx) {
			clicked = true
		}
		background.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
		a.processFontPickerEvents(gtx)
		a.layoutFontPicker(gtx)
		router.Frame(&ops)
	}
	frame()
	point := a.fontPicker.bounds.Min.Add(image.Pt(180, 10))
	pos := f32.Pt(float32(point.X), float32(point.Y))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: pos, Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pos})
	frame()
	frame()
	if clicked {
		t.Fatal("click inside popup reached underlying editor")
	}
}

func TestFontPickerMixedRangeAppliesActiveCellFont(t *testing.T) {
	fake := &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
	s := newDocumentSessionState("x.xlsx")
	s.book = fake
	s.sheet = "Sheet1"
	s.selectionSet = true
	s.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{Row: 1}}
	a := &App{documentSessionState: s, formatResults: make(chan spreadsheetFormatResult, 2)}
	a.initFontPicker()
	a.fontPicker.open = true
	a.fontPicker.target = fontPickerTarget{state: s, sheet: s.sheet, selection: s.selection, current: "Go", revision: s.wordRevisionAtomic.Load()}
	a.applyFontChoice("Go")
	select {
	case r := <-a.formatResults:
		if r.err != nil {
			t.Fatal(r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mixed range treated as no-op")
	}
	if fake.lastPatch.FontFamily == nil || *fake.lastPatch.FontFamily != "Go" {
		t.Fatal("font not applied to selection")
	}
}
