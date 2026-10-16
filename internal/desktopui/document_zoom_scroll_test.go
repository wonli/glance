package desktopui

import (
	"image"
	"math"
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
	"gioui.org/widget/material"
)

func TestZoomFromScrollAccumulatesFineTrackpadDeltas(t *testing.T) {
	zoom, remainder, changed := zoomFromScroll(1, -0.4, 0.01, 0, 0.5, 2)
	if changed || zoom != 1 || math.Abs(remainder-0.004) > 1e-9 {
		t.Fatalf("first partial zoom = %.2f, remainder %.4f, changed %v; want 1.00, .0040, false", zoom, remainder, changed)
	}
	zoom, remainder, changed = zoomFromScroll(zoom, -0.3, 0.01, remainder, 0.5, 2)
	if !changed || zoom != 1.01 || math.Abs(remainder+0.003) > 1e-9 {
		t.Fatalf("accumulated zoom = %.2f, remainder %.4f, changed %v; want 1.01, -.0030, true", zoom, remainder, changed)
	}
}

func TestZoomScrollAcceptsShortcutModifierOnMacAndControlModifier(t *testing.T) {
	if !zoomScrollModifier(key.ModShortcut) {
		t.Fatal("platform shortcut modifier must enable zoom scrolling")
	}
	if !zoomScrollModifier(key.ModCtrl) {
		t.Fatal("Control modifier must enable zoom scrolling")
	}
	if zoomScrollModifier(key.ModShift) {
		t.Fatal("Shift alone must not enable zoom scrolling")
	}
}

func TestSpreadsheetBodyShortcutWheelZoomsWithoutScrollingGrid(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	a.window = nil
	v := a.virtualGrid
	v.X, v.Y = 300, 300
	beforeX, beforeY := v.X, v.Y
	beforeZoom := a.spreadsheetZoom()

	delta := float32(-10)
	h.router.Queue(pointer.Event{
		Kind: pointer.Scroll, Source: pointer.Mouse,
		Position: f32.Pt(200, 60), Scroll: f32.Pt(0, delta),
		Modifiers: key.ModShortcut,
	})
	h.frame()

	if got := a.spreadsheetZoom(); got <= beforeZoom {
		t.Fatalf("shortcut wheel zoom = %.2f; want greater than %.2f", got, beforeZoom)
	}
	if v.X != beforeX || v.Y != beforeY {
		t.Fatalf("shortcut wheel moved grid from (%v,%v) to (%v,%v)", beforeX, beforeY, v.X, v.Y)
	}
}

func TestSpreadsheetBodyPlainWheelStillScrollsGridWithoutZooming(t *testing.T) {
	h := newFullGridTextHarness(t, false, "value")
	a := h.app
	a.window = nil
	v := a.virtualGrid
	v.X, v.Y = 300, 300
	beforeZoom := a.spreadsheetZoom()
	beforeY := v.Y

	h.router.Queue(pointer.Event{
		Kind: pointer.Scroll, Source: pointer.Mouse,
		Position: f32.Pt(200, 60), Scroll: f32.Pt(0, 10),
	})
	h.frame()

	if got := a.spreadsheetZoom(); got != beforeZoom {
		t.Fatalf("plain wheel changed zoom to %.2f; want %.2f", got, beforeZoom)
	}
	if v.Y == beforeY {
		t.Fatal("plain wheel did not scroll the grid")
	}
}

func TestWordBodyShortcutWheelZoomsWithoutScrollingDocument(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 160)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var router input.Router
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1100, 700)},
		}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	frame()
	frame()
	beforeZoom := state.wordLayoutZoom
	beforePosition := state.wordListState.Position

	router.Queue(pointer.Event{
		Kind: pointer.Scroll, Source: pointer.Mouse,
		Position: f32.Pt(540, 320), Scroll: f32.Pt(0, -10),
		Modifiers: key.ModShortcut,
	})
	frame()

	if state.wordLayoutZoom <= beforeZoom {
		t.Fatalf("shortcut wheel zoom = %.2f; want greater than %.2f", state.wordLayoutZoom, beforeZoom)
	}
	if state.wordListState.Position.First != beforePosition.First || state.wordListState.Position.Offset != beforePosition.Offset {
		t.Fatalf("shortcut wheel moved Word list from %+v to %+v", beforePosition, state.wordListState.Position)
	}
}

func TestWordBodyZoomScrollPassesClicksToChildWidgets(t *testing.T) {
	state := newDocumentSessionState("click.docx")
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var router input.Router
	var ops op.Ops
	var clicked bool
	var btn widget.Clickable
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(400, 300)},
		}
		app.layoutWordZoomableBody(gtx, state, nil, func(gtx layout.Context) layout.Dimensions {
			if btn.Clicked(gtx) {
				clicked = true
			}
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(200, 100)}
			})
		})
		router.Frame(&ops)
	}
	frame()

	router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(50, 50),
	})
	frame()
	router.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse,
		PointerID: 1, Position: f32.Pt(50, 50),
	})
	frame()

	if !clicked {
		t.Fatal("child widget inside layoutWordZoomableBody did not receive click; wordZoomScroll must use PassOp")
	}
}

