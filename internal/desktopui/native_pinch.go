package desktopui

import (
	"image"
	"math"
	"sync"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

const nativePinchSurface = "文档正文"

type nativePinchEvent struct {
	Position      f32.Point // Physical pixels, top-left window origin.
	Magnification float64
	Begin, End    bool
}

type nativePinchState struct {
	mu         sync.Mutex
	pending    []nativePinchEvent
	invalidate func()
	wake       chan struct{}
	stopped    chan struct{}
	stopOnce   sync.Once
}

// Native callbacks never call Gio, even after releasing the queue mutex.
// Window.Invalidate can synchronously reenter the macOS event loop when called
// on its main thread. A separate managed task keeps that call off the callback.
func newNativePinchState(invalidate func()) *nativePinchState {
	return &nativePinchState{invalidate: invalidate, wake: make(chan struct{}, 1), stopped: make(chan struct{})}
}

func (s *nativePinchState) stop() {
	s.stopOnce.Do(func() {
		if s.stopped != nil {
			close(s.stopped)
		}
	})
}

func (s *nativePinchState) runInvalidations(shutdown <-chan struct{}) {
	for {
		select {
		case <-shutdown:
			return
		case <-s.stopped:
			return
		case <-s.wake:
			select {
			case <-shutdown:
				return
			case <-s.stopped:
				return
			default:
			}
			if s.invalidate != nil {
				s.invalidate()
			}
		}
	}
}

func (s *nativePinchState) enqueue(e nativePinchEvent) {
	select {
	case <-s.stopped:
		return
	default:
	}
	s.mu.Lock()
	// Bound backlog while retaining gesture boundaries. Adjacent updates combine
	// their relative scale factors, rather than dropping fine trackpad deltas.
	if n := len(s.pending); n > 0 && !e.Begin && !e.End && !s.pending[n-1].End && !s.pending[n-1].Begin {
		last := &s.pending[n-1]
		last.Magnification = (1+last.Magnification)*(1+e.Magnification) - 1
		last.Position = e.Position
	} else if len(s.pending) < 64 {
		s.pending = append(s.pending, e)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *nativePinchState) drain() []nativePinchEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.pending
	s.pending = nil
	return events
}

// The semantic marker gives the native bridge the actual clipped, transformed
// viewport bounds through Gio's public API, including tabs, Ribbon and ruler.
func recordNativePinchSurface(gtx layout.Context) {
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	semantic.DescriptionOp(nativePinchSurface).Add(gtx.Ops)
	area.Pop()
}

func findNativePinchSurface(nodes []input.SemanticNode, point image.Point) (image.Rectangle, bool) {
	for _, node := range nodes {
		if node.Desc.Description == nativePinchSurface && point.In(node.Desc.Bounds) {
			return node.Desc.Bounds, true
		}
		if bounds, ok := findNativePinchSurface(node.Children, point); ok {
			return bounds, true
		}
	}
	return image.Rectangle{}, false
}

// Read the preceding completed frame before app.NewContext resets its ops.
// Parsing the public semantic tree happens only when native gestures arrive.
func (a *App) processNativePinch(frame app.FrameEvent) {
	if a.nativePinch == nil {
		return
	}
	events := a.nativePinch.drain()
	if len(events) == 0 {
		return
	}
	validDocument := a.documentSessionState != nil || (a.proofreading != nil && a.pdfDoc != nil)
	proofreadingWindow := a.proofreading != nil && a.pdfDoc != nil
	activeDocument := proofreadingWindow || a.activeTabID != "recent"
	valid := a.pinchFrameSession == a.documentSessionState && validDocument && a.pinchFrameSize == frame.Size && a.pinchFrameMetric == frame.Metric && activeDocument && !a.modal.Visible && !a.showNewMenu && !a.wordSymbolPickerVisible && !a.fontPicker.open && (a.documentSessionState == nil || a.documentSessionState.officeRibbonMenu.kind == officeRibbonMenuClosed) && a.activeAgentConfirmation() == nil
	var nodes []input.SemanticNode
	if valid {
		var router input.Router
		router.Frame(&a.ops)
		nodes = router.AppendSemantics(nil)
	}
	for _, e := range events {
		bounds, inside := findNativePinchSurface(nodes, image.Pt(int(e.Position.X), int(e.Position.Y)))
		if e.Begin {
			a.pinchGestureSession = nil
			a.pinchGestureActive = false
			a.pinchRemainder = 0
			if valid && inside {
				a.pinchGestureSession = a.documentSessionState
				a.pinchGestureActive = true
			}
		}
		gestureMatchesDocument := a.pinchGestureSession == a.documentSessionState
		gestureStarted := a.documentSessionState != nil || a.pinchGestureActive
		if valid && inside && gestureMatchesDocument && gestureStarted {
			point := e.Position.Sub(f32.Pt(float32(bounds.Min.X), float32(bounds.Min.Y)))
			gtx := layout.Context{Metric: frame.Metric, Constraints: layout.Constraints{Max: bounds.Size()}}
			a.applyNativePinch(gtx, point, e.Magnification)
		}
		if e.End {
			a.pinchGestureSession = nil
			a.pinchGestureActive = false
			a.pinchRemainder = 0
		}
	}
}

func (a *App) applyNativePinch(gtx layout.Context, point f32.Point, magnification float64) {
	state := a.documentSessionState
	if state == nil {
		if a.proofreading == nil || a.pdfDoc == nil {
			return
		}
		v := a.ensurePDFView()
		zoom, remainder, changed := zoomFromPinch(clampPDFZoom(v.currentZoom(gtx.Metric)), magnification, a.pinchRemainder, pdfZoomMinimum, pdfZoomMaximum)
		a.pinchRemainder = remainder
		if changed {
			a.setPDFZoomAt(gtx, v, zoom, point)
			if a.window != nil {
				a.window.Invalidate()
			}
		}
		return
	}
	if state.book != nil && state.virtualGrid != nil {
		zoom, remainder, changed := zoomFromPinch(a.spreadsheetZoom(), magnification, a.pinchRemainder, spreadsheetZoomMinimum, spreadsheetZoomMaximum)
		a.pinchRemainder = remainder
		if changed {
			a.setSpreadsheetZoom(zoom)
			headerX, headerY := gtx.Dp(gridRowHeader), gtx.Dp(gridColumnHeader)
			state.virtualGrid.zoomAt(float64(gtx.Metric.PxPerDp)*zoom, float64(point.X)-float64(headerX), float64(point.Y)-float64(headerY), float64(gtx.Constraints.Max.X-headerX), float64(gtx.Constraints.Max.Y-headerY))
		}
	} else if state.pdfDoc != nil {
		v := state.pdfView
		if v == nil {
			return
		}
		zoom, remainder, changed := zoomFromPinch(clampPDFZoom(v.currentZoom(gtx.Metric)), magnification, a.pinchRemainder, pdfZoomMinimum, pdfZoomMaximum)
		a.pinchRemainder = remainder
		if changed {
			a.setPDFZoomAt(gtx, v, zoom, point)
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	} else if state.word != nil {
		zoom, remainder, changed := zoomFromPinch(wordStatusZoomClamp(state.wordLayoutZoom), magnification, a.pinchRemainder, wordStatusZoomMinimum, wordStatusZoomMaximum)
		a.pinchRemainder = remainder
		if changed {
			setWordWheelZoom(gtx, state, zoom, point)
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	}
}

func zoomFromPinch(current, magnification, remainder, minimum, maximum float64) (zoom, nextRemainder float64, changed bool) {
	if math.IsNaN(magnification) || math.IsInf(magnification, 0) || magnification <= -1 {
		return current, remainder, false
	}
	return zoomFromScroll(current, -(current+remainder)*magnification, 1, remainder, minimum, maximum)
}
