package desktopui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/i18n"
)

func TestAdaptiveChoiceSizesTranslatedLabels(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	for _, scale := range []float32{1, 2} {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(int(1200*scale), int(100*scale))}}
		options := []adaptiveChoiceOption{{ID: "1", Label: "1"}, {ID: "all", Label: "All"}}
		short := measureAdaptiveChoice(gtx, app.theme, "展开层级", options)
		long := measureAdaptiveChoice(gtx, app.theme, "Expand hierarchy", options)
		if long.width <= short.width {
			t.Fatalf("translated title did not grow: %d <= %d", long.width, short.width)
		}
		options[1].Label = strings.Repeat("Long translation ", 4)
		longer := measureAdaptiveChoice(gtx, app.theme, "Expand hierarchy", options)
		if longer.optionWidths[1] <= long.optionWidths[1] || longer.width <= long.width {
			t.Fatal("option width did not follow translated text")
		}
		var control adaptiveChoice
		gtx.Constraints.Max.X = gtx.Dp(150)
		dims := control.Layout(gtx, app.theme, "Expand hierarchy", options, "all")
		if !control.compact || dims.Size.X > gtx.Constraints.Max.X {
			t.Fatal("narrow control must collapse within available width")
		}
		gtx.Constraints.Max.X = gtx.Dp(1200)
		dims = control.Layout(gtx, app.theme, "Expand hierarchy", options, "all")
		if control.compact || dims.Size.X != longer.width {
			t.Fatal("wide control must expose every complete option")
		}
	}
}

func TestAdaptiveChoicePointerKeyboardAndOutsideClick(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		app := New(nil, "")
		defer close(app.shutdown)
		var control adaptiveChoice
		var router input.Router
		var ops op.Ops
		var outside widget.Clickable
		outsideClicks := 0
		selected := "3"
		options := []adaptiveChoiceOption{{"1", "1"}, {"2", "2"}, {"3", "3"}, {"all", "All"}}
		draw := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(400*scale), int(300*scale)))}
			if id := control.Update(gtx, options, selected); id != "" {
				selected = id
			}
			if outside.Clicked(gtx) {
				outsideClicks++
			}
			outside.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
			origin := image.Pt(gtx.Dp(220), gtx.Dp(240))
			shift := op.Offset(origin).Push(gtx.Ops)
			local := gtx
			local.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(150), gtx.Dp(34))}
			dims := control.Layout(local, app.theme, "Expand hierarchy", options, selected)
			shift.Pop()
			control.anchor = image.Rectangle{Min: origin, Max: origin.Add(dims.Size)}
			control.LayoutOverlay(gtx, app.theme, options, selected)
			router.Frame(&ops)
		}
		click := func(point image.Point) {
			position := f32.Pt(float32(point.X), float32(point.Y))
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: position, Buttons: pointer.ButtonPrimary})
			draw()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
			draw()
		}
		draw()
		click(control.anchor.Min.Add(control.anchor.Size().Div(2)))
		if !control.open {
			t.Fatal("trigger did not open dropdown")
		}
		if !control.bounds.In(image.Rect(0, 0, int(400*scale), int(300*scale))) {
			t.Fatalf("menu escaped viewport: %v", control.bounds)
		}
		if control.bounds.Max.Y >= control.anchor.Min.Y {
			t.Fatal("bottom anchor must flip dropdown above")
		}
		click(control.bounds.Min.Add(image.Pt(int(20*scale), int((4+3*32+16)*scale))))
		if selected != "all" || control.open {
			t.Fatalf("last option inaccessible: selected=%q open=%v", selected, control.open)
		}
		click(control.anchor.Min.Add(control.anchor.Size().Div(2)))
		options[3].Label = "全部"
		draw()
		if !control.open || selected != "all" {
			t.Fatal("translating an open menu lost state")
		}
		router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press}, key.Event{Name: key.NameEnter, State: key.Press})
		draw()
		if selected != "3" || control.open {
			t.Fatalf("keyboard selection failed: %q", selected)
		}
		click(control.anchor.Min.Add(control.anchor.Size().Div(2)))
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
		draw()
		if control.open {
			t.Fatal("Escape did not dismiss")
		}
		click(control.anchor.Min.Add(control.anchor.Size().Div(2)))
		click(image.Pt(int(10*scale), int(270*scale)))
		if control.open || outsideClicks != 1 {
			t.Fatalf("outside click must dismiss and pass through: open=%v clicks=%d", control.open, outsideClicks)
		}
	}
}

func TestJSONAdaptiveLevelControlClicksReachActions(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		app := newTestAppWithJSON(t, `{"a":{"b":1}}`)
		defer close(app.shutdown)
		app.applyLocaleSelection(i18n.SelectionEnglish)
		var router input.Router
		var ops op.Ops
		width := 1440
		v := app.ensureJSONView()
		draw := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(float32(width)*scale), int(400*scale)))}
			app.handleJSONToolbarEvents(gtx)
			app.layoutJSONToolbar(gtx)
			v.levelControl.LayoutOverlay(gtx, app.theme, app.jsonLevelOptions(), app.jsonLevelSelection())
			router.Frame(&ops)
		}
		click := func(point image.Point) {
			pos := f32.Pt(float32(point.X), float32(point.Y))
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: pos, Buttons: pointer.ButtonPrimary})
			draw()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pos})
			draw()
		}
		draw()
		bounds := v.levelControl.anchor
		click(image.Pt(bounds.Max.X-int(20*scale), bounds.Min.Y+bounds.Dy()/2))
		if v.expandedLevel != -1 {
			t.Fatalf("wide All click did not reach JSON action: %d", v.expandedLevel)
		}
		width = 480
		draw()
		click(v.levelControl.anchor.Min.Add(v.levelControl.anchor.Size().Div(2)))
		if !v.levelControl.open {
			t.Fatal("JSON dropdown anchor does not match trigger")
		}
		click(v.levelControl.bounds.Min.Add(image.Pt(int(20*scale), int(20*scale))))
		if v.expandedLevel != 1 || v.levelControl.open {
			t.Fatalf("dropdown selection did not reach JSON action: %d", v.expandedLevel)
		}
	}
}

func TestJSONAdaptiveLevelControlLocaleAndWindowWidth(t *testing.T) {
	app := newTestAppWithJSON(t, `{"a":{"b":1}}`)
	v := app.ensureJSONView()
	v.expandedLevel = 2
	for _, scale := range []float32{1, 2} {
		for _, locale := range []i18n.Selection{i18n.SelectionChinese, i18n.SelectionEnglish} {
			app.applyLocaleSelection(locale)
			for _, width := range []int{480, 1440} {
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(float32(width)*scale), int(100*scale)))}
				app.layoutJSONToolbar(gtx)
				if v.levelControl.compact != (width == 480) {
					t.Fatalf("locale=%v width=%d compact=%v", locale, width, v.levelControl.compact)
				}
				if app.jsonLevelSelection() != "level-2" || v.expandedLevel != 2 {
					t.Fatal("layout or locale change lost selection")
				}
			}
		}
	}
}
