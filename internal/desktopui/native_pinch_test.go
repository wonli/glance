package desktopui

import (
	"image"
	"math"
	"sync"
	"testing"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/wordlayout"
)

func TestNativePinchAccumulatesRelativeMagnification(t *testing.T) {
	zoom, remainder, changed := zoomFromPinch(2, .1, 0, .1, 4)
	if !changed || zoom != 2.2 || math.Abs(remainder) > 1e-8 {
		t.Fatalf("pinch=%v remainder=%v changed=%v", zoom, remainder, changed)
	}
	zoom, remainder, changed = zoomFromPinch(1, .003, 0, .1, 4)
	if changed || zoom != 1 {
		t.Fatal("fine delta rounded too early")
	}
	zoom, _, changed = zoomFromPinch(zoom, .003, remainder, .1, 4)
	if !changed || zoom != 1.01 {
		t.Fatal("fine delta was lost")
	}
	for _, delta := range []float64{math.NaN(), math.Inf(1), -1, -2} {
		zoom, _, changed = zoomFromPinch(1, delta, 0, .1, 4)
		if changed || zoom != 1 {
			t.Fatal("invalid magnification changed zoom")
		}
	}
	zoom, remainder, changed = zoomFromPinch(4, .1, 0, .1, 4)
	if changed || zoom != 4 || remainder != 0 {
		t.Fatal("maximum did not clamp")
	}
	zoom, _, changed = zoomFromPinch(2, -.1, 0, .1, 4)
	if !changed || zoom != 1.8 {
		t.Fatal("pinch together did not shrink")
	}
}

func TestNativePinchQueueCoalescesRelativeUpdates(t *testing.T) {
	state := new(nativePinchState)
	state.enqueue(nativePinchEvent{Begin: true})
	for range 1000 {
		state.enqueue(nativePinchEvent{Magnification: .001})
	}
	state.enqueue(nativePinchEvent{End: true})
	events := state.drain()
	if len(events) != 3 || !events[0].Begin || !events[2].End {
		t.Fatalf("lost boundaries: %v", events)
	}
	if math.Abs(events[1].Magnification-(math.Pow(1.001, 1000)-1)) > 1e-8 {
		t.Fatal("coalescing lost scale")
	}
	if len(state.drain()) != 0 {
		t.Fatal("events replayed")
	}
}

func TestSpreadsheetNativePinchUsesWindowCoordinatesAndNoModifier(t *testing.T) {
	for _, density := range []float32{1, 2} {
		h := newFullGridTextHarness(t, false, "value")
		a := h.app
		a.window = nil
		a.nativePinch = new(nativePinchState)
		metric := unit.Metric{PxPerDp: density, PxPerSp: density}
		size := image.Pt(int(900*density), int(600*density))
		origin := image.Pt(int(70*density), int(140*density))
		a.ops.Reset()
		offset := op.Offset(origin).Push(&a.ops)
		gtx := layout.Context{Ops: &a.ops, Source: h.router.Source(), Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(int(700*density), int(400*density))}}
		a.layoutVirtualGrid(gtx)
		offset.Pop()
		a.virtualGrid.X, a.virtualGrid.Y = 300, 300
		a.pinchFrameSession = a.documentSessionState
		a.pinchFrameSize, a.pinchFrameMetric = size, metric
		point := f32.Pt(200*density, 60*density)
		x, y := float64(point.X)-float64(gtx.Dp(gridRowHeader)), float64(point.Y)-float64(gtx.Dp(gridColumnHeader))
		beforeX, beforeY := (a.virtualGrid.X+x)/a.virtualGrid.Scale, (a.virtualGrid.Y+y)/a.virtualGrid.Scale
		event := nativePinchEvent{Begin: true, Position: point.Add(f32.Pt(float32(origin.X), float32(origin.Y))), Magnification: .1}
		a.nativePinch.enqueue(event)
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		if a.spreadsheetZoom() != 1.1 {
			t.Fatalf("pinch zoom=%v", a.spreadsheetZoom())
		}
		if math.Abs((a.virtualGrid.X+x)/a.virtualGrid.Scale-beforeX) > 1e-8 || math.Abs((a.virtualGrid.Y+y)/a.virtualGrid.Scale-beforeY) > 1e-8 {
			t.Fatal("pinch moved anchor")
		}
		before := a.spreadsheetZoom()
		event.Position = f32.Pt(10, 10)
		a.nativePinch.enqueue(event)
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		if a.spreadsheetZoom() != before {
			t.Fatal("outside body changed zoom")
		}
		event.Position = point.Add(f32.Pt(float32(origin.X), float32(origin.Y)))
		a.modal.Visible = true
		a.nativePinch.enqueue(event)
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		if a.spreadsheetZoom() != before {
			t.Fatal("pinch through modal changed zoom")
		}
		a.modal.Visible = false
		a.officeRibbonMenu.kind = officeRibbonMenuSpreadsheet
		a.nativePinch.enqueue(event)
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		if a.spreadsheetZoom() != before {
			t.Fatal("pinch through Ribbon menu changed zoom")
		}
		a.officeRibbonMenu.kind = officeRibbonMenuClosed
		a.fontPicker.open = true
		a.nativePinch.enqueue(event)
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		if a.spreadsheetZoom() != before {
			t.Fatal("pinch through font picker changed zoom")
		}
		a.fontPicker.open = false
		a.pinchFrameSession = newDocumentSessionState("stale.xlsx")
		a.nativePinch.enqueue(event)
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		if a.spreadsheetZoom() != before {
			t.Fatal("stale frame changed zoom")
		}
	}
}

func TestWordNativePinchPreservesPagePointWithRuler(t *testing.T) {
	for _, density := range []float32{1, 2} {
		state, closeDoc := newLongWordFlowState(t, 10)
		defer closeDoc()
		state.wordViewMode = wordViewPrintLayout
		state.wordMultiplePagesMode = true
		state.wordLayoutZoom = 1
		pages := make([]wordlayout.PageScene, 12)
		for i := range pages {
			pages[i] = wordlayout.PageScene{Index: i, Clip: wordlayout.Rect{WidthPT: 400, HeightPT: 700}}
		}
		state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: pages}
		a := &App{documentSessionState: state, theme: material.NewTheme(), nativePinch: new(nativePinchState)}
		var router input.Router
		metric := unit.Metric{PxPerDp: density, PxPerSp: density}
		size := image.Pt(int(700*density), int(500*density))
		frame := func() {
			a.ops.Reset()
			gtx := layout.Context{Ops: &a.ops, Source: router.Source(), Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: size}}
			a.layoutWordBodyArea(gtx, state, nil)
			router.Frame(&a.ops)
			a.pinchFrameSession = state
			a.pinchFrameSize, a.pinchFrameMetric = size, metric
		}
		frame()
		state.wordPageListState.Position.First = 2
		state.wordPageListState.Position.Offset = int(120 * density)
		frame()
		before := state.wordPageVisibleInteractions[0]
		point := f32.Pt(200*density, 180*density)
		localX := (float64(point.X) - float64(before.PageRect.Min.X)) / before.Transform.PixelsPerPoint
		localY := (float64(point.Y) - float64(before.PageRect.Min.Y)) / before.Transform.PixelsPerPoint
		a.nativePinch.enqueue(nativePinchEvent{Begin: true, Position: point.Add(f32.Pt(0, wordRulerHeightDP*density)), Magnification: .1})
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		frame()
		after, ok := visibleWordPageInteraction(state.wordPageVisibleInteractions, before.PageIndex)
		if !ok || state.wordLayoutZoom != 1.1 || !state.wordMultiplePagesMode {
			t.Fatal("pinch lost page or multiple-page mode")
		}
		if math.Abs(float64(after.PageRect.Min.X)+localX*after.Transform.PixelsPerPoint-float64(point.X)) > 2 || math.Abs(float64(after.PageRect.Min.Y)+localY*after.Transform.PixelsPerPoint-float64(point.Y)) > 2 {
			t.Fatal("page point moved")
		}
		a.nativePinch.enqueue(nativePinchEvent{Begin: true, Position: f32.Pt(200*density, 10*density), Magnification: .1})
		a.processNativePinch(app.FrameEvent{Size: size, Metric: metric})
		if state.wordLayoutZoom != 1.1 {
			t.Fatal("pinch over ruler changed zoom")
		}
	}
}

func TestNativePinchQueueConcurrentCallbacks(t *testing.T) {
	state := new(nativePinchState)
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				state.enqueue(nativePinchEvent{Magnification: .001})
			}
		}()
	}
	workers.Wait()
	events := state.drain()
	if len(events) != 1 || math.Abs(events[0].Magnification-(math.Pow(1.001, 800)-1)) > 1e-8 {
		t.Fatal("concurrent callbacks lost updates")
	}
}

func TestNativePinchCallbackNeverWaitsForWindowInvalidation(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	state := &nativePinchState{invalidate: func() { <-release }}
	returned := make(chan struct{})
	go func() { state.enqueue(nativePinchEvent{Begin: true, Magnification: .1}); close(returned) }()
	select {
	case <-returned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("native callback blocked on window invalidation; AppKit main thread must only enqueue")
	}
}

func TestNativePinchWorkerHandoffAndShutdown(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	state := newNativePinchState(func() {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
	})
	stopped := make(chan struct{})
	go func() { state.runInvalidations(nil); close(stopped) }()
	state.enqueue(nativePinchEvent{Begin: true})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not invalidate")
	}
	returned := make(chan struct{})
	go func() {
		for range 1000 {
			state.enqueue(nativePinchEvent{Magnification: .001})
		}
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("callback blocked behind busy invalidation worker")
	}
	state.stop()
	state.stop()
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("view teardown did not stop worker")
	}
	pending := len(state.drain())
	state.enqueue(nativePinchEvent{Magnification: .1})
	if pending == 0 || len(state.drain()) != 0 {
		t.Fatal("late callback accepted after teardown")
	}
}
