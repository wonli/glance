package desktopui

import (
	"context"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"glance/internal/diagramrender"
	"glance/internal/document"
	"glance/internal/nativewebview"
)

const markdownChartSnapshotInterval = 34 * time.Millisecond
const markdownChartMaxPendingEvents = 64

type markdownChartState struct {
	mu           sync.Mutex
	key          string
	resourceKey  string
	sourceStart  int
	source       string
	span         document.MarkdownSpan
	generation   uint64
	tag          struct{}
	image        image.Image
	logicalSize  image.Point
	session      diagramrender.ChartSession
	ctx          context.Context
	cancel       context.CancelFunc
	attempt      uint64
	opening      bool
	worker       bool
	exporting    bool
	hidden       bool
	closed       bool
	failed       bool
	lastSnapshot time.Time
	retryAfter   time.Time
	events       []nativewebview.PointerEvent
	err          string
	settleErr    string
}

func markdownChartPointerEvent(e pointer.Event, logicalSize, displaySize image.Point) (nativewebview.PointerEvent, bool) {
	if logicalSize.X <= 0 || logicalSize.Y <= 0 || displaySize.X <= 0 || displaySize.Y <= 0 {
		return nativewebview.PointerEvent{}, false
	}
	kind := nativewebview.PointerEventKind("")
	switch e.Kind {
	case pointer.Enter, pointer.Move, pointer.Drag:
		kind = nativewebview.PointerMove
	case pointer.Press:
		kind = nativewebview.PointerDown
	case pointer.Release:
		kind = nativewebview.PointerUp
	case pointer.Leave, pointer.Cancel:
		kind = nativewebview.PointerLeave
	case pointer.Scroll:
		if e.Modifiers&(key.ModShift|key.ModCtrl) == 0 {
			return nativewebview.PointerEvent{}, false
		}
		kind = nativewebview.PointerWheel
	default:
		return nativewebview.PointerEvent{}, false
	}
	x, y := float64(e.Position.X), float64(e.Position.Y)
	if kind == nativewebview.PointerLeave {
		x = max(0, min(x, float64(displaySize.X)))
		y = max(0, min(y, float64(displaySize.Y)))
	} else if x < 0 || y < 0 || x >= float64(displaySize.X) || y >= float64(displaySize.Y) {
		return nativewebview.PointerEvent{}, false
	}
	button := 0
	if e.Buttons.Contain(pointer.ButtonSecondary) {
		button = 2
	} else if e.Buttons.Contain(pointer.ButtonTertiary) {
		button = 1
	}
	return nativewebview.PointerEvent{
		Kind:    kind,
		X:       x * float64(logicalSize.X) / float64(displaySize.X),
		Y:       y * float64(logicalSize.Y) / float64(displaySize.Y),
		Button:  button,
		Buttons: uint32(e.Buttons),
		DeltaX:  float64(e.Scroll.X) * float64(logicalSize.X) / float64(displaySize.X),
		DeltaY:  float64(e.Scroll.Y) * float64(logicalSize.Y) / float64(displaySize.Y),
		Shift:   e.Modifiers&key.ModShift != 0,
		Control: e.Modifiers&key.ModCtrl != 0,
	}, true
}

func markdownChartIdentity(span document.MarkdownSpan) string {
	return fmt.Sprintf("%d:%d:%s", span.SourceStart, span.SourceEnd, markdownAssetKey(span))
}

func (a *App) ensureMarkdownChart(span document.MarkdownSpan, initial image.Image) *markdownChartState {
	v := a.ensureMarkdownView()
	key := markdownChartIdentity(span)
	generation, source := v.parseGeneration, v.snapshot.Source
	v.chartsMu.Lock()
	if v.charts == nil {
		v.charts = make(map[string]*markdownChartState)
	}
	state := v.charts[key]
	if state != nil {
		state.mu.Lock()
		valid := !state.closed && state.generation == generation && state.source == source
		if valid && state.image == nil && initial != nil {
			state.image = initial
			state.logicalSize = document.ResourceLogicalSize(initial)
		}
		if valid {
			state.hidden = false
		}
		state.mu.Unlock()
		if valid {
			v.chartsMu.Unlock()
			return state
		}
		delete(v.charts, key)
	}
	state = &markdownChartState{key: key, resourceKey: markdownAssetKey(span), sourceStart: span.SourceStart, source: source, span: span, generation: generation, image: initial}
	if initial != nil {
		state.logicalSize = document.ResourceLogicalSize(initial)
	}
	v.charts[key] = state
	v.chartsMu.Unlock()
	return state
}

func (a *App) startMarkdownChartSession(v *markdownViewState, state *markdownChartState, span document.MarkdownSpan) {
	state.mu.Lock()
	if state.closed || state.hidden || state.exporting || state.failed || state.opening || state.session != nil || time.Now().Before(state.retryAfter) {
		state.mu.Unlock()
		return
	}
	state.opening = true
	state.ctx, state.cancel = context.WithCancel(context.Background())
	state.attempt++
	attempt := state.attempt
	ctx := state.ctx
	state.mu.Unlock()

	started := a.launchManagedBackground(func() {
		kind := strings.ToLower(strings.TrimSpace(span.Language))
		open := v.openChart
		if open == nil {
			open = func(ctx context.Context, request diagramrender.Request) (diagramrender.ChartSession, error) {
				renderer, err := diagramrender.Default()
				if err != nil {
					return nil, err
				}
				return renderer.OpenChart(ctx, request)
			}
		}
		openCtx, cancelOpen := context.WithTimeout(ctx, 10*time.Second)
		session, err := open(openCtx, diagramrender.Request{Kind: kind, Source: []byte(span.Text), Density: 2})
		cancelOpen()
		var snapshot diagramrender.Result
		if err == nil {
			snapshotCtx, cancelSnapshot := context.WithTimeout(ctx, 5*time.Second)
			snapshot, err = session.Snapshot(snapshotCtx)
			cancelSnapshot()
		}
		if err != nil {
			if session != nil {
				_ = session.Close()
			}
			a.finishMarkdownChartOpen(v, state, attempt, nil, diagramrender.Result{}, err)
			return
		}
		if !a.finishMarkdownChartOpen(v, state, attempt, session, snapshot, nil) {
			_ = session.Close()
			return
		}
		a.startMarkdownChartWorker(v, state)
	})
	if !started {
		state.mu.Lock()
		state.opening = false
		state.closed = true
		if state.cancel != nil {
			state.cancel()
		}
		state.mu.Unlock()
	}
}

func (a *App) finishMarkdownChartOpen(v *markdownViewState, state *markdownChartState, attempt uint64, session diagramrender.ChartSession, snapshot diagramrender.Result, err error) bool {
	v.chartsMu.Lock()
	current := v.charts[state.key] == state
	state.mu.Lock()
	valid := current && !state.closed && state.attempt == attempt && state.ctx != nil && state.ctx.Err() == nil
	state.opening = false
	if valid && err == nil {
		state.session = session
		state.failed = false
		state.retryAfter = time.Time{}
		if snapshot.Image != nil {
			state.image = document.ResourceImage{Image: snapshot.Image, Size: snapshot.LogicalSize}
			state.logicalSize = snapshot.LogicalSize
		}
		state.err = ""
		state.settleErr = ""
		state.lastSnapshot = time.Now()
	} else if valid && err != nil {
		state.failed = !strings.Contains(strings.ToLower(err.Error()), "too many live charts")
		if !state.failed {
			state.retryAfter = time.Now().Add(250 * time.Millisecond)
		}
		state.err = err.Error()
		state.events = nil
		if state.cancel != nil {
			state.cancel()
		}
	}
	state.mu.Unlock()
	v.chartsMu.Unlock()
	if valid && a.window != nil {
		a.window.Invalidate()
	}
	return valid && err == nil
}

func (a *App) enqueueMarkdownChartEvent(v *markdownViewState, state *markdownChartState, event nativewebview.PointerEvent) {
	state.mu.Lock()
	if state.closed || state.exporting || state.ctx == nil || state.ctx.Err() != nil {
		state.mu.Unlock()
		return
	}
	state.events = appendMarkdownChartEvent(state.events, event)
	startWorker := state.session != nil && !state.worker
	if startWorker {
		state.worker = true
	}
	state.mu.Unlock()
	if startWorker {
		a.launchMarkdownChartWorker(v, state)
	}
}

func appendMarkdownChartEvent(events []nativewebview.PointerEvent, event nativewebview.PointerEvent) []nativewebview.PointerEvent {
	if len(events) > 0 {
		last := len(events) - 1
		queued := &events[last]
		if event.Kind == nativewebview.PointerMove && queued.Kind == nativewebview.PointerMove && queued.Buttons == event.Buttons && queued.Shift == event.Shift && queued.Control == event.Control {
			*queued = event
			return events
		}
		if event.Kind == nativewebview.PointerWheel && queued.Kind == nativewebview.PointerWheel && queued.Shift == event.Shift && queued.Control == event.Control {
			queued.X = event.X
			queued.Y = event.Y
			queued.DeltaX += event.DeltaX
			queued.DeltaY += event.DeltaY
			return events
		}
	}
	if len(events) < markdownChartMaxPendingEvents {
		return append(events, event)
	}
	for i, queued := range events {
		if queued.Kind != nativewebview.PointerMove && queued.Kind != nativewebview.PointerWheel {
			continue
		}
		copy(events[i:], events[i+1:])
		events = events[:len(events)-1]
		return append(events, event)
	}
	if event.Kind == nativewebview.PointerMove || event.Kind == nativewebview.PointerWheel {
		return events
	}
	// The queue is saturated with non-coalescible transitions. Reset stale
	// gesture history and keep the latest transition deliverable and bounded.
	events = events[:0]
	events = append(events, nativewebview.PointerEvent{Kind: nativewebview.PointerLeave})
	if event.Kind != nativewebview.PointerLeave {
		events = append(events, event)
	}
	return events
}

func (a *App) startMarkdownChartWorker(v *markdownViewState, state *markdownChartState) {
	state.mu.Lock()
	if state.closed || state.session == nil || state.worker || len(state.events) == 0 {
		state.mu.Unlock()
		return
	}
	state.worker = true
	state.mu.Unlock()
	a.launchMarkdownChartWorker(v, state)
}

func (a *App) launchMarkdownChartWorker(v *markdownViewState, state *markdownChartState) {
	if !a.launchManagedBackground(func() { a.runMarkdownChartEvents(v, state) }) {
		state.mu.Lock()
		state.worker = false
		state.events = nil
		state.mu.Unlock()
	}
}

func (a *App) runMarkdownChartEvents(v *markdownViewState, state *markdownChartState) {
	for {
		state.mu.Lock()
		if state.closed || state.session == nil {
			state.worker = false
			state.mu.Unlock()
			return
		}
		if len(state.events) == 0 && state.hidden {
			session, ctx := state.session, state.ctx
			state.mu.Unlock()
			result, err := settleHiddenMarkdownChart(ctx, session)
			state.mu.Lock()
			current := !state.closed && state.session == session && state.ctx == ctx
			if current && result.Image != nil {
				state.image = document.ResourceImage{Image: result.Image, Size: result.LogicalSize}
				state.logicalSize = result.LogicalSize
			}
			if current && err != nil {
				state.err = err.Error()
				state.settleErr = err.Error()
			} else if current {
				state.settleErr = ""
				state.err = ""
			}
			closeSession := current && state.hidden && len(state.events) == 0
			var cancel context.CancelFunc
			if closeSession {
				state.attempt++
				state.session = nil
				state.worker = false
				state.events = nil
				cancel = state.cancel
				state.cancel = nil
				state.ctx = nil
			} else if !current || len(state.events) == 0 {
				state.worker = false
			}
			restart := current && !closeSession && len(state.events) > 0
			state.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			if closeSession {
				a.closeMarkdownChartSession(v, session)
				return
			}
			if restart {
				continue
			}
			return
		}
		if len(state.events) == 0 {
			state.worker = false
			state.mu.Unlock()
			return
		}
		wait := time.Duration(0)
		if state.events[0].Kind == nativewebview.PointerMove && !state.lastSnapshot.IsZero() {
			wait = time.Until(state.lastSnapshot.Add(markdownChartSnapshotInterval))
		}
		ctx, session := state.ctx, state.session
		state.mu.Unlock()
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				state.mu.Lock()
				state.worker = false
				restart := !state.closed && state.session != nil && len(state.events) > 0
				if restart {
					state.worker = true
				}
				state.mu.Unlock()
				if restart {
					a.launchMarkdownChartWorker(v, state)
				}
				return
			case <-timer.C:
			}
		}
		state.mu.Lock()
		if state.closed || state.session != session || len(state.events) == 0 {
			state.mu.Unlock()
			continue
		}
		event := state.events[0]
		state.events = state.events[1:]
		ctx = state.ctx
		state.mu.Unlock()

		result, err := session.Dispatch(ctx, event)
		state.mu.Lock()
		current := !state.closed && state.session == session && ctx.Err() == nil
		state.lastSnapshot = time.Now()
		if current && err != nil {
			state.err = err.Error()
		} else if current && result.Image != nil {
			state.image = document.ResourceImage{Image: result.Image, Size: result.LogicalSize}
			state.logicalSize = result.LogicalSize
			state.err = ""
			state.settleErr = ""
		}
		state.mu.Unlock()
		if current && a.window != nil {
			a.window.Invalidate()
		}
	}
}

func settleHiddenMarkdownChart(ctx context.Context, session diagramrender.ChartSession) (diagramrender.Result, error) {
	if ctx == nil {
		return diagramrender.Result{}, context.Canceled
	}
	settleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	leaveResult, err := session.Dispatch(settleCtx, nativewebview.PointerEvent{Kind: nativewebview.PointerLeave})
	if err != nil {
		return diagramrender.Result{}, err
	}
	result, snapshotErr := session.Snapshot(settleCtx)
	if snapshotErr == nil && result.Image != nil {
		return result, nil
	}
	if leaveResult.Image != nil {
		return leaveResult, nil
	}
	if snapshotErr != nil {
		return diagramrender.Result{}, snapshotErr
	}
	return diagramrender.Result{}, fmt.Errorf("hidden chart produced no settled image")
}

func (a *App) closeMarkdownChartSession(v *markdownViewState, session diagramrender.ChartSession) {
	if v.closeChartSessions != nil {
		v.closeChartSessions([]diagramrender.ChartSession{session})
		return
	}
	_ = session.Close()
}

// detachMarkdownChartSessions only cancels and detaches UI state. Native Close
// may wait for WebKit's main queue and must never run in a Gio frame.
func (v *markdownViewState) detachMarkdownChartSessions() []diagramrender.ChartSession {
	if v == nil {
		return nil
	}
	v.chartsMu.Lock()
	states := make([]*markdownChartState, 0, len(v.charts))
	for _, state := range v.charts {
		states = append(states, state)
	}
	v.charts = nil
	v.chartsMu.Unlock()
	var sessions []diagramrender.ChartSession
	for _, state := range states {
		state.mu.Lock()
		state.closed = true
		state.attempt++
		state.opening = false
		state.events = nil
		if state.cancel != nil {
			state.cancel()
		}
		if state.session != nil {
			sessions = append(sessions, state.session)
			state.session = nil
		}
		state.mu.Unlock()
	}
	return sessions
}

func (v *markdownViewState) releaseMarkdownCharts() {
	sessions := v.detachMarkdownChartSessions()
	if len(sessions) == 0 {
		return
	}
	if v.closeChartSessions != nil {
		v.closeChartSessions(sessions)
		return
	}
	// Views without a window-owned task runner still must not block their caller.
	go func() {
		for _, session := range sessions {
			_ = session.Close()
		}
	}()
}

func (a *App) closeMarkdownCharts(v *markdownViewState) {
	if v != nil {
		v.releaseMarkdownCharts()
	}
}

// closeHiddenMarkdownCharts releases browser slots for charts that have left
// the visible Markdown list while retaining their last image for later reuse.
func (a *App) closeHiddenMarkdownCharts(v *markdownViewState, visible map[string]struct{}) {
	if v == nil {
		return
	}
	v.chartsMu.Lock()
	states := make([]*markdownChartState, 0, len(v.charts))
	for key, state := range v.charts {
		if _, ok := visible[key]; !ok {
			states = append(states, state)
		}
	}
	v.chartsMu.Unlock()
	workers := make([]*markdownChartState, 0, len(states))
	for _, state := range states {
		state.mu.Lock()
		if !state.closed && !state.exporting && (state.session != nil || state.opening) {
			state.hidden = true
			if state.session == nil && state.opening {
				state.attempt++
				state.opening = false
				if state.cancel != nil {
					state.cancel()
					state.cancel = nil
					state.ctx = nil
				}
			} else if state.session != nil && !state.worker {
				state.worker = true
				workers = append(workers, state)
			}
		}
		state.mu.Unlock()
	}
	for _, state := range workers {
		a.launchMarkdownChartWorker(v, state)
	}
}

func (a *App) layoutMarkdownChartPointer(gtx layout.Context, v *markdownViewState, state *markdownChartState, logicalSize image.Point, draw layout.Widget) layout.Dimensions {
	dims := draw(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		return dims
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &state.tag)
	filter := pointer.Filter{Target: &state.tag, Kinds: pointer.Enter | pointer.Leave | pointer.Move | pointer.Press | pointer.Release | pointer.Drag | pointer.Cancel | pointer.Scroll, ScrollX: pointer.ScrollRange{Min: -(1 << 20), Max: 1 << 20}, ScrollY: pointer.ScrollRange{Min: -(1 << 20), Max: 1 << 20}}
	for value, ok := gtx.Event(filter); ok; value, ok = gtx.Event(filter) {
		pe, ok := value.(pointer.Event)
		if !ok {
			continue
		}
		mapped, ok := markdownChartPointerEvent(pe, logicalSize, dims.Size)
		if !ok {
			continue
		}
		if mapped.Kind == nativewebview.PointerWheel {
			v.consumeChartScroll = true
		}
		// Static preview comes from the document batch. Allocate a live WebView
		// only when input actually reaches this chart, never during layout.
		if mapped.Kind != nativewebview.PointerLeave {
			a.startMarkdownChartSession(v, state, state.span)
		}
		a.enqueueMarkdownChartEvent(v, state, mapped)
	}
	area.Pop()
	pass.Pop()
	return dims
}

func markdownChartSnapshot(state *markdownChartState) (image.Image, image.Point, string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.image, state.logicalSize, state.err
}

func markdownChartIsVega(span document.MarkdownSpan) bool {
	language := strings.ToLower(strings.TrimSpace(span.Language))
	return span.Kind == "diagram" && (language == "vega" || language == "vega-lite")
}
