package desktopui

import (
	"context"
	"errors"
	"fmt"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/diagramrender"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/nativewebview"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
)

func TestMarkdownVegaPointerCoordinatesAndWheelRouting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		display image.Point
		point   f32.Point
		want    image.Point
	}{{"1x", image.Pt(200, 100), f32.Pt(50, 25), image.Pt(50, 25)}, {"2x", image.Pt(400, 200), f32.Pt(100, 50), image.Pt(50, 25)}, {"scaled-to-fit", image.Pt(100, 50), f32.Pt(50, 25), image.Pt(100, 50)}} {
		t.Run(tc.name, func(t *testing.T) {
			event, ok := markdownChartPointerEvent(pointer.Event{Kind: pointer.Move, Position: tc.point}, image.Pt(200, 100), tc.display)
			if !ok || event.Kind != nativewebview.PointerMove || event.X != float64(tc.want.X) || event.Y != float64(tc.want.Y) {
				t.Fatalf("mapped event=%+v ok=%t want point=%v", event, ok, tc.want)
			}
		})
	}
	if _, ok := markdownChartPointerEvent(pointer.Event{Kind: pointer.Move, Position: f32.Pt(201, 20)}, image.Pt(200, 100), image.Pt(200, 100)); ok {
		t.Fatal("pointer outside chart hit bounds was forwarded")
	}
	if _, ok := markdownChartPointerEvent(pointer.Event{Kind: pointer.Scroll, Scroll: f32.Pt(0, 40)}, image.Pt(200, 100), image.Pt(400, 200)); ok {
		t.Fatal("ordinary wheel should remain with Markdown scrolling")
	}
	wheel, ok := markdownChartPointerEvent(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(80, 40), Scroll: f32.Pt(20, -40), Modifiers: key.ModShift}, image.Pt(200, 100), image.Pt(400, 200))
	if !ok || wheel.Kind != nativewebview.PointerWheel || wheel.DeltaX != 10 || wheel.DeltaY != -20 || !wheel.Shift {
		t.Fatalf("modifier wheel event=%+v ok=%t", wheel, ok)
	}
	leave, ok := markdownChartPointerEvent(pointer.Event{Kind: pointer.Leave, Position: f32.Pt(-2, 105)}, image.Pt(200, 100), image.Pt(400, 200))
	if !ok || leave.Kind != nativewebview.PointerLeave {
		t.Fatalf("leave event was dropped outside the hit bounds: %+v, %t", leave, ok)
	}
}

func TestMarkdownChartLayoutDoesNotOpenWebView(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	v.openChart = func(context.Context, diagramrender.Request) (diagramrender.ChartSession, error) {
		t.Error("static preview allocated a live WebView")
		return nil, errors.New("unexpected open")
	}
	span := document.MarkdownSpan{Kind: "diagram", Language: "vega-lite", Text: `{"mark":"bar"}`}
	initial := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for range 3 {
		state := a.ensureMarkdownChart(span, initial)
		state.mu.Lock()
		opening, session, img := state.opening, state.session, state.image
		state.mu.Unlock()
		if opening || session != nil || img != initial {
			t.Fatal("static preview did not retain its image without a live session")
		}
	}
}

type markdownChartTestSession struct {
	events          chan nativewebview.PointerEvent
	started         chan struct{}
	closed          chan struct{}
	image           image.Image
	snapshotImage   image.Image
	order           chan string
	block           bool
	blockUntil      chan struct{}
	blockKind       nativewebview.PointerEventKind
	dispatchErr     error
	dispatchErrKind nativewebview.PointerEventKind
	snapshotErr     error
}

func (s *markdownChartTestSession) Dispatch(ctx context.Context, event nativewebview.PointerEvent) (diagramrender.Result, error) {
	if s.order != nil {
		s.order <- "dispatch"
	}
	select {
	case s.events <- event:
	default:
	}
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.block {
		<-ctx.Done()
		return diagramrender.Result{Image: s.image, LogicalSize: image.Pt(200, 100)}, nil
	}
	if s.blockUntil != nil && event.Kind == s.blockKind {
		select {
		case <-s.blockUntil:
		case <-ctx.Done():
			return diagramrender.Result{}, ctx.Err()
		}
	}
	if event.Kind == s.dispatchErrKind && s.dispatchErr != nil {
		return diagramrender.Result{}, s.dispatchErr
	}
	return diagramrender.Result{Image: s.image, LogicalSize: image.Pt(200, 100)}, nil
}

func (s *markdownChartTestSession) Snapshot(context.Context) (diagramrender.Result, error) {
	if s.order != nil {
		s.order <- "snapshot"
	}
	snapshotImage := s.snapshotImage
	if snapshotImage == nil {
		snapshotImage = s.image
	}
	if s.snapshotErr != nil {
		return diagramrender.Result{}, s.snapshotErr
	}
	return diagramrender.Result{Image: snapshotImage, LogicalSize: image.Pt(200, 100)}, nil
}

func (s *markdownChartTestSession) Close() error {
	if s.closed != nil {
		select {
		case <-s.closed:
		default:
			close(s.closed)
		}
	}
	return nil
}

func TestMarkdownVegaPointerEventsReachSessionAtOneAndTwoX(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("%gx", scale), func(t *testing.T) {
			a := markdownTestApp(t)
			v := a.markdownView
			chartImage := image.NewRGBA(image.Rect(0, 0, 200, 100))
			session := &markdownChartTestSession{events: make(chan nativewebview.PointerEvent, 8), closed: make(chan struct{}), image: chartImage}
			state := &markdownChartState{key: "chart", source: "source", generation: 1, image: chartImage, logicalSize: image.Pt(200, 100), session: session, ctx: context.Background()}
			v.charts[state.key] = state
			var router input.Router
			var ops op.Ops
			size := image.Pt(int(200*scale), int(100*scale))
			frame := func() {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
				a.layoutMarkdownChartPointer(gtx, v, state, image.Pt(200, 100), func(layout.Context) layout.Dimensions { return layout.Dimensions{Size: size} })
				router.Frame(&ops)
			}
			frame()
			pos := f32.Pt(50*scale, 25*scale)
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: pos, PointerID: 1})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: pos, Buttons: pointer.ButtonPrimary, PointerID: 1})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos, PointerID: 1})
			frame()

			wantKinds := []nativewebview.PointerEventKind{nativewebview.PointerMove, nativewebview.PointerDown, nativewebview.PointerUp}
			for _, kind := range wantKinds {
				deadline := time.After(2 * time.Second)
				for {
					select {
					case got := <-session.events:
						if got.Kind == nativewebview.PointerLeave {
							continue
						}
						if got.Kind != kind || got.X != 50 || got.Y != 25 {
							t.Fatalf("event=%+v; want kind=%q at chart coordinate (50,25)", got, kind)
						}
						goto matched
					case <-deadline:
						t.Fatalf("timed out waiting for %q event", kind)
					}
				}
			matched:
			}
			a.closeMarkdownCharts(v)
			select {
			case <-session.closed:
			case <-time.After(2 * time.Second):
				t.Fatal("chart session was not closed")
			}
			a.stopAndWaitManagedBackground()
		})
	}
}

func TestMarkdownVegaWheelRoutingPreservesMarkdownScroll(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	session := &markdownChartTestSession{events: make(chan nativewebview.PointerEvent, 4), closed: make(chan struct{}), image: image.NewRGBA(image.Rect(0, 0, 200, 100))}
	state := &markdownChartState{key: "chart", source: "source", generation: 1, logicalSize: image.Pt(200, 100), session: session, ctx: context.Background()}
	v.charts[state.key] = state
	var router input.Router
	var ops op.Ops
	size := image.Pt(400, 200)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Exact(size)}
		a.layoutMarkdownChartPointer(gtx, v, state, image.Pt(200, 100), func(layout.Context) layout.Dimensions { return layout.Dimensions{Size: size} })
		router.Frame(&ops)
	}
	frame()
	v.consumeChartScroll = false
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(80, 60), Scroll: f32.Pt(0, -40), PointerID: 2})
	frame()
	if v.consumeChartScroll {
		t.Fatal("ordinary wheel input was captured from Markdown scrolling")
	}
	for _, modifier := range []key.Modifiers{key.ModShift, key.ModCtrl} {
		v.consumeChartScroll = false
		router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(80, 60), Scroll: f32.Pt(0, -40), Modifiers: modifier, PointerID: 2})
		frame()
		if !v.consumeChartScroll {
			t.Fatalf("modifier wheel %v was not reserved for the chart", modifier)
		}
		deadline := time.After(2 * time.Second)
		for {
			select {
			case got := <-session.events:
				if got.Kind == nativewebview.PointerLeave {
					continue
				}
				if got.Kind != nativewebview.PointerWheel || got.DeltaY != -20 || got.Shift != (modifier == key.ModShift) || got.Control != (modifier == key.ModCtrl) {
					t.Fatalf("modifier wheel event=%+v, modifier=%v", got, modifier)
				}
				goto wheelMatched
			case <-deadline:
				t.Fatal("modifier wheel was not forwarded to Vega")
			}
		}
	wheelMatched:
	}
	select {
	case got := <-session.events:
		t.Fatalf("ordinary wheel was incorrectly forwarded to Vega: %+v", got)
	default:
	}
	a.closeMarkdownCharts(v)
	select {
	case <-session.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("chart session was not closed")
	}
	a.stopAndWaitManagedBackground()
}

func TestMarkdownVegaDropsSnapshotAfterChartSessionCloses(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	base := image.NewUniform(color.NRGBA{B: 255, A: 255})
	late := image.NewUniform(color.NRGBA{R: 255, A: 255})
	session := &markdownChartTestSession{events: make(chan nativewebview.PointerEvent, 1), started: make(chan struct{}, 1), closed: make(chan struct{}), image: late, block: true}
	ctx, cancel := context.WithCancel(context.Background())
	state := &markdownChartState{key: "chart", source: "source", generation: 1, image: base, logicalSize: image.Pt(200, 100), session: session, ctx: ctx, cancel: cancel}
	v.charts = map[string]*markdownChartState{state.key: state}
	a.enqueueMarkdownChartEvent(v, state, nativewebview.PointerEvent{Kind: nativewebview.PointerDown, X: 10, Y: 10})
	select {
	case <-session.started:
	case <-time.After(2 * time.Second):
		t.Fatal("chart event worker did not start")
	}
	a.closeMarkdownCharts(v)
	a.stopAndWaitManagedBackground()
	select {
	case <-session.closed:
	default:
		t.Fatal("chart session was not closed")
	}
	got, _, _ := markdownChartSnapshot(state)
	if got != base {
		t.Fatal("late chart snapshot replaced the last valid image after session close")
	}
}

func TestMarkdownVegaSessionOpeningAfterCloseDisposesLateSession(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	opened := make(chan struct{})
	releaseOpen := make(chan struct{})
	session := &markdownChartTestSession{events: make(chan nativewebview.PointerEvent, 1), closed: make(chan struct{}), image: image.NewRGBA(image.Rect(0, 0, 200, 100))}
	v.openChart = func(context.Context, diagramrender.Request) (diagramrender.ChartSession, error) {
		close(opened)
		<-releaseOpen
		return session, nil
	}
	state := &markdownChartState{key: "chart", source: "source", generation: 1}
	v.charts[state.key] = state
	a.startMarkdownChartSession(v, state, document.MarkdownSpan{Kind: "diagram", Language: "vega", Text: `{ "mark": "point" }`})
	select {
	case <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("chart session opening did not start")
	}
	a.closeMarkdownCharts(v)
	close(releaseOpen)
	select {
	case <-session.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("late chart session was not closed")
	}
	a.stopAndWaitManagedBackground()
	if got, _, _ := markdownChartSnapshot(state); got != nil {
		t.Fatal("late chart snapshot was published after chart session closed")
	}
}

func TestMarkdownVegaHiddenChartDrainsInputAndKeepsFinalSnapshot(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	initialImage := image.NewUniform(color.NRGBA{R: 255, A: 255})
	settledImage := image.NewUniform(color.NRGBA{B: 255, A: 255})
	session := &markdownChartTestSession{
		events: make(chan nativewebview.PointerEvent, 4), closed: make(chan struct{}),
		image: initialImage, snapshotImage: settledImage, order: make(chan string, 4),
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &markdownChartState{
		key: "chart", resourceKey: "chart-resource", sourceStart: 2, source: "source", generation: 1,
		image: initialImage, logicalSize: image.Pt(200, 100), session: session, ctx: ctx, cancel: cancel,
		events: []nativewebview.PointerEvent{{Kind: nativewebview.PointerDown}, {Kind: nativewebview.PointerUp}},
	}
	v.charts[state.key] = state
	a.closeHiddenMarkdownCharts(v, map[string]struct{}{})
	select {
	case <-session.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("offscreen chart session was not closed")
	}
	state.mu.Lock()
	closed, liveSession, liveContext, finalImage := state.closed, state.session, state.ctx, state.image
	state.mu.Unlock()
	if closed || liveSession != nil || liveContext != nil || finalImage == nil {
		t.Fatalf("offscreen chart state did not retain final snapshot: closed=%t session=%v context=%v image=%v", closed, liveSession, liveContext, finalImage)
	}
	if got := color.NRGBAModel.Convert(finalImage.At(0, 0)).(color.NRGBA); got.B < 200 || got.R != 0 {
		t.Fatalf("offscreen chart image = %v, want settled blue snapshot", got)
	}
	for _, want := range []nativewebview.PointerEventKind{nativewebview.PointerDown, nativewebview.PointerUp, nativewebview.PointerLeave} {
		if got := <-session.events; got.Kind != want {
			t.Fatalf("offscreen chart event = %+v, want %q", got, want)
		}
	}
	if got := []string{<-session.order, <-session.order, <-session.order, <-session.order}; strings.Join(got, ",") != "dispatch,dispatch,dispatch,snapshot" {
		t.Fatalf("offscreen chart settle sequence = %v", got)
	}
	frozen, err := freezeMarkdownChartResources(context.Background(), v, "source", 1, map[string]image.Image{"chart-resource": initialImage})
	if err != nil || frozen["chart-resource"] == nil {
		t.Fatalf("PDF did not reuse the offscreen settled image: image=%v err=%v", frozen["chart-resource"], err)
	}
	if got := color.NRGBAModel.Convert(frozen["chart-resource"].At(0, 0)).(color.NRGBA); got.B < 200 || got.R != 0 {
		t.Fatalf("PDF image = %v, want settled blue snapshot", got)
	}
	a.stopAndWaitManagedBackground()
}

func TestMarkdownVegaHiddenChartDoesNotExportStaleImageWhenSettleFails(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	initialImage := image.NewUniform(color.NRGBA{R: 255, A: 255})
	session := &markdownChartTestSession{
		events: make(chan nativewebview.PointerEvent, 4), closed: make(chan struct{}), image: initialImage,
		dispatchErrKind: nativewebview.PointerLeave, dispatchErr: errors.New("WebView pointer leave failed"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &markdownChartState{
		key: "chart", resourceKey: "chart-resource", sourceStart: 2, source: "source", generation: 1,
		image: initialImage, logicalSize: image.Pt(200, 100), session: session, ctx: ctx, cancel: cancel,
	}
	v.charts[state.key] = state
	a.closeHiddenMarkdownCharts(v, map[string]struct{}{})
	select {
	case <-session.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("offscreen chart session was not closed")
	}
	frozen, err := freezeMarkdownChartResources(context.Background(), v, "source", 1, map[string]image.Image{"chart-resource": initialImage})
	if err == nil || !strings.Contains(err.Error(), "pointer leave failed") {
		t.Fatalf("PDF export error = %v, want hidden chart settle failure", err)
	}
	if frozen != nil {
		t.Fatalf("PDF export returned stale frozen resources after settle failure: %v", frozen)
	}
	a.stopAndWaitManagedBackground()
}

func TestMarkdownVegaHiddenChartFallsBackToPointerLeaveImage(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	initialImage := image.NewUniform(color.NRGBA{R: 255, A: 255})
	settledImage := image.NewUniform(color.NRGBA{B: 255, A: 255})
	session := &markdownChartTestSession{
		events: make(chan nativewebview.PointerEvent, 4), closed: make(chan struct{}), image: settledImage,
		snapshotErr: errors.New("snapshot unavailable"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &markdownChartState{
		key: "chart", resourceKey: "chart-resource", sourceStart: 2, source: "source", generation: 1,
		image: initialImage, logicalSize: image.Pt(200, 100), session: session, ctx: ctx, cancel: cancel,
	}
	v.charts[state.key] = state
	a.closeHiddenMarkdownCharts(v, map[string]struct{}{})
	select {
	case <-session.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("offscreen chart session was not closed")
	}
	frozen, err := freezeMarkdownChartResources(context.Background(), v, "source", 1, map[string]image.Image{"chart-resource": initialImage})
	if err != nil || frozen["chart-resource"] == nil {
		t.Fatalf("PDF did not use pointer-leave image fallback: image=%v err=%v", frozen["chart-resource"], err)
	}
	if got := color.NRGBAModel.Convert(frozen["chart-resource"].At(0, 0)).(color.NRGBA); got.B < 200 || got.R != 0 {
		t.Fatalf("PDF image = %v, want pointer-leave blue image", got)
	}
	a.stopAndWaitManagedBackground()
}

func TestMarkdownPDFFallsBackToPointerLeaveImageWhenSnapshotFails(t *testing.T) {
	a := markdownTestApp(t)
	source := "```vega\n{\"mark\":\"point\"}\n```\n"
	snapshot, err := markdowndoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	span := markdowndoc.DiagramSpan(snapshot.Blocks[0])
	key := markdowndoc.ResourceKey(span)
	initialImage := image.NewUniform(color.NRGBA{R: 255, A: 255})
	settledImage := image.NewUniform(color.NRGBA{B: 255, A: 255})
	session := &markdownChartTestSession{
		events: make(chan nativewebview.PointerEvent, 2), closed: make(chan struct{}), image: settledImage,
		snapshotErr: errors.New("snapshot unavailable"),
	}
	view := newMarkdownView(source)
	view.snapshot = snapshot
	view.parseGeneration = 1
	view.charts[markdownChartIdentity(span)] = &markdownChartState{
		key: markdownChartIdentity(span), resourceKey: key, sourceStart: span.SourceStart, source: source,
		generation: 1, image: initialImage, session: session, ctx: context.Background(),
	}
	frozen, err := freezeMarkdownChartResources(context.Background(), view, source, 1, map[string]image.Image{key: initialImage})
	if err != nil || frozen[key] == nil {
		t.Fatalf("PDF did not use pointer-leave image fallback: image=%v err=%v", frozen[key], err)
	}
	if got := color.NRGBAModel.Convert(frozen[key].At(0, 0)).(color.NRGBA); got.B < 200 || got.R != 0 {
		t.Fatalf("PDF image = %v, want pointer-leave blue image", got)
	}
	a.stopAndWaitManagedBackground()
}

func TestMarkdownPDFUsesSnapshotWhileHiddenChartCloses(t *testing.T) {
	a := markdownTestApp(t)
	source := "```vega\n{\"mark\":\"point\"}\n```\n"
	snapshot, err := markdowndoc.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	span := markdowndoc.DiagramSpan(snapshot.Blocks[0])
	key := markdowndoc.ResourceKey(span)
	initialImage := image.NewUniform(color.NRGBA{R: 255, A: 255})
	settledImage := image.NewUniform(color.NRGBA{B: 255, A: 255})
	started := make(chan struct{}, 1)
	session := &markdownChartTestSession{
		events: make(chan nativewebview.PointerEvent, 2), started: started, closed: make(chan struct{}),
		image: initialImage, snapshotImage: settledImage, blockUntil: make(chan struct{}), blockKind: nativewebview.PointerLeave,
	}
	ctx, cancel := context.WithCancel(context.Background())
	view := newMarkdownView(source)
	view.snapshot = snapshot
	view.parseGeneration = 1
	view.charts[markdownChartIdentity(span)] = &markdownChartState{
		key: markdownChartIdentity(span), resourceKey: key, sourceStart: span.SourceStart, source: source,
		generation: 1, image: initialImage, session: session, ctx: ctx, cancel: cancel,
	}
	a.closeHiddenMarkdownCharts(view, map[string]struct{}{})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("hidden chart did not start finalizing")
	}
	result := make(chan struct {
		images map[string]image.Image
		err    error
	}, 1)
	go func() {
		images, err := freezeMarkdownChartResources(context.Background(), view, source, 1, map[string]image.Image{key: initialImage})
		result <- struct {
			images map[string]image.Image
			err    error
		}{images: images, err: err}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		view.chartsMu.Lock()
		state := view.charts[markdownChartIdentity(span)]
		view.chartsMu.Unlock()
		state.mu.Lock()
		exporting := state.exporting
		state.mu.Unlock()
		if exporting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("PDF snapshot did not mark the chart as exporting")
		}
		time.Sleep(time.Millisecond)
	}
	close(session.blockUntil)
	select {
	case got := <-result:
		if got.err != nil || got.images[key] == nil {
			t.Fatalf("PDF snapshot failed while hidden chart closed: image=%v err=%v", got.images[key], got.err)
		}
		if pixel := color.NRGBAModel.Convert(got.images[key].At(0, 0)).(color.NRGBA); pixel.B < 200 || pixel.R != 0 {
			t.Fatalf("PDF snapshot = %v, want settled blue image", pixel)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PDF snapshot blocked while hidden chart closed")
	}
	select {
	case <-session.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("hidden chart session was not closed")
	}
	cancel()
	a.stopAndWaitManagedBackground()
}

func TestMarkdownVegaCancelDuringMoveThrottleReleasesWorker(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	session := &markdownChartTestSession{events: make(chan nativewebview.PointerEvent, 1), closed: make(chan struct{}), image: image.NewRGBA(image.Rect(0, 0, 200, 100))}
	ctx, cancel := context.WithCancel(context.Background())
	state := &markdownChartState{key: "chart", source: "source", generation: 1, session: session, ctx: ctx, cancel: cancel, lastSnapshot: time.Now()}
	v.charts[state.key] = state
	a.enqueueMarkdownChartEvent(v, state, nativewebview.PointerEvent{Kind: nativewebview.PointerMove, X: 5, Y: 5})
	a.closeHiddenMarkdownCharts(v, map[string]struct{}{})
	a.stopAndWaitManagedBackground()
	state.mu.Lock()
	worker := state.worker
	state.mu.Unlock()
	if worker {
		t.Fatal("cancelled chart worker remained marked active after hiding")
	}
}

func TestMarkdownVegaPendingEventsStayBoundedAndResetSaturatedGesture(t *testing.T) {
	app := &App{}
	state := &markdownChartState{ctx: context.Background()}
	for i := 0; i < 200; i++ {
		app.enqueueMarkdownChartEvent(nil, state, nativewebview.PointerEvent{Kind: nativewebview.PointerDown, Buttons: 1})
		app.enqueueMarkdownChartEvent(nil, state, nativewebview.PointerEvent{Kind: nativewebview.PointerUp})
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.events) > 64 {
		t.Fatalf("pending chart events grew to %d, want a bounded queue", len(state.events))
	}
	if len(state.events) < 2 || state.events[0].Kind != nativewebview.PointerLeave || state.events[len(state.events)-1].Kind != nativewebview.PointerUp {
		t.Fatalf("saturated event queue did not reset and preserve the latest release: %+v", state.events)
	}
}

func TestMarkdownVegaCoalescesAdjacentMovesAndModifierWheels(t *testing.T) {
	app := &App{}
	state := &markdownChartState{ctx: context.Background()}
	app.enqueueMarkdownChartEvent(nil, state, nativewebview.PointerEvent{Kind: nativewebview.PointerMove, X: 1, Buttons: 0})
	app.enqueueMarkdownChartEvent(nil, state, nativewebview.PointerEvent{Kind: nativewebview.PointerMove, X: 2, Buttons: 0})
	state.mu.Lock()
	if len(state.events) != 1 || state.events[0].X != 2 {
		state.mu.Unlock()
		t.Fatalf("adjacent pointer moves were not coalesced: %+v", state.events)
	}
	state.events = nil
	state.mu.Unlock()
	app.enqueueMarkdownChartEvent(nil, state, nativewebview.PointerEvent{Kind: nativewebview.PointerWheel, DeltaY: 5, Shift: true})
	app.enqueueMarkdownChartEvent(nil, state, nativewebview.PointerEvent{Kind: nativewebview.PointerWheel, DeltaY: -2, Shift: true})
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.events) != 1 || state.events[0].DeltaY != 3 {
		t.Fatalf("adjacent modifier-wheel events were not coalesced: %+v", state.events)
	}
}

func TestMarkdownTaskPreviewClickChangesOnlyMarkerAndRetainsSelection(t *testing.T) {
	source := "\ufeff# 中文\r\n\r\n- [ ] 待办\r\n  - [X] 子项\r\n\r\n```\r\n- [ ] literal\r\n```\r\n"
	for _, scale := range []float32{1, 1.5, 2} {
		a := markdownTestApp(t)
		v := a.markdownView
		v.editor.SetText(source)
		a.flushMarkdownDraft()
		v.editor.SetCaret(2, 4)
		p, err := markdowndoc.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		v.requested = source
		v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(600, 900)}}
		for i, b := range p.Blocks {
			a.layoutMarkdownBlock(gtx, b, i)
		}
		offset := len([]rune("\ufeff# 中文\r\n\r\n- ["))
		id := fmt.Sprintf("task-%d", offset)
		click := v.buttons[id]
		if click == nil {
			t.Fatal("preview has no interactive checkbox")
		}
		for cycle := 0; cycle < 4; cycle++ {
			click.Click()
			a.handleMarkdownEvents(gtx)
			want := []rune(source)
			if cycle%2 == 0 {
				want[offset] = 'x'
			}
			if v.text() != string(want) || a.markdownDoc.Text() != string(want) {
				t.Fatalf("cycle%d changed wrong source: %q", cycle, v.text())
			}
			start, end := v.editor.Selection()
			if start != 2 || end != 4 {
				t.Fatalf("selection moved: %d,%d", start, end)
			}
		}
		if !a.isDirty() {
			t.Fatal("task edit was not marked dirty")
		}
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}
}
func TestMarkdownFormulaPreviewReopensAndReplacesOriginal(t *testing.T) {
	for _, tc := range []struct {
		source  string
		block   int
		id      string
		display bool
		want    string
	}{{"中文 $x^2$ 后文", 0, "math-3-8", false, "中文 $y_1$ 后文"}, {"前文\r\n\r\n$$\r\nx^2\r\n$$\r\n\r\n后文", 1, "math-6-17", true, "前文\r\n\r\n$$\r\ny_1\r\n$$\r\n\r\n后文"}} {
		a := markdownTestApp(t)
		v := a.markdownView
		v.editor.SetText(tc.source)
		a.flushMarkdownDraft()
		p, err := markdowndoc.Parse(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		v.requested = tc.source
		v.accept(markdownParseResult{source: tc.source, snapshot: p, images: map[string]image.Image{}})
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(600, 900)}}
		a.layoutMarkdownBlock(gtx, p.Blocks[tc.block], tc.block)
		button := v.buttons[tc.id]
		if button == nil {
			t.Fatalf("formula %q is not editable; buttons=%v", tc.id, v.buttons)
		}
		button.Click()
		a.handleMarkdownEvents(gtx)
		if !a.modal.Visible || a.modal.FormInputs[0].Text() != "x^2" {
			t.Fatal("formula edit dialog did not retain expression")
		}
		a.modal.FormInputs[0].SetText("y_1")
		a.modal.OnAction("confirm")
		if v.text() != tc.want || a.markdownDoc.Text() != tc.want {
			t.Fatalf("formula replacement got %q want %q", v.text(), tc.want)
		}
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}
}

func TestMarkdownTaskPointerSurvivesResourceRefresh(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		a := markdownTestApp(t)
		v := a.markdownView
		source := "- [ ] 中文"
		v.editor.SetText(source)
		a.flushMarkdownDraft()
		v.focus = false
		v.requested = source
		p, _ := markdowndoc.Parse(source)
		v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
		var ops op.Ops
		var router input.Router
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(600, 900)}}
			a.handleMarkdownEvents(gtx)
			a.layoutMarkdownBlock(gtx, v.snapshot.Blocks[0], 0)
			router.Frame(&ops)
		}
		frame()
		button := v.buttons["task-3"]
		position := f32.Pt(8*scale, 10*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
		frame()
		v.accept(markdownParseResult{source: source, generation: v.parseGeneration, snapshot: v.snapshot, images: v.images})
		if v.buttons["task-3"] != button {
			t.Fatal("resource refresh replaced checkbox widget")
		}
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
		frame()
		frame()
		if v.text() != "- [x] 中文" {
			t.Fatalf("pointer click lost at scale %v: %q", scale, v.text())
		}
		if v.parseCancel != nil {
			v.parseCancel()
		}
	}
}
func TestMarkdownFormulaToolbarEditsAndCancelRetainsSource(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	source := "中文 $x^2$ 后文"
	v.editor.SetText(source)
	a.flushMarkdownDraft()
	v.editor.SetCaret(5, 5)
	p, _ := markdowndoc.Parse(source)
	v.requested = source
	v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
	var ops op.Ops
	gtx := layout.Context{Ops: &ops}
	a.handleMarkdownAction(gtx, "math")
	if !a.modal.Visible || a.modal.FormInputs[0].Text() != "x^2" {
		t.Fatal("toolbar did not recognize existing formula")
	}
	a.modal.FormInputs[0].SetText("y_1")
	a.modal.OnAction("cancel")
	if v.text() != source {
		t.Fatal("cancel changed formula")
	}
	start, end := v.editor.Selection()
	if start != 5 || end != 5 {
		t.Fatal("cancel moved source caret")
	}
	a.handleMarkdownAction(gtx, "math")
	v.editor.SetText(source + "改动")
	a.modal.OnAction("confirm")
	if v.text() != source+"改动" {
		t.Fatal("stale formula edit replaced newer text")
	}
}

func TestMarkdownTableFormulaToolbarReopensExistingExpression(t *testing.T) {
	a := markdownTestApp(t)
	v := a.markdownView
	source := "| 值 |\n| --- |\n| $x^2$ |\n"
	v.editor.SetText(source)
	a.flushMarkdownDraft()
	p, _ := markdowndoc.Parse(source)
	v.requested = source
	v.accept(markdownParseResult{source: source, snapshot: p, images: map[string]image.Image{}})
	formula := p.Blocks[0].TableSpans[1][0][0]
	v.editor.SetCaret(formula.SourceStart+2, formula.SourceStart+2)
	var ops op.Ops
	a.handleMarkdownAction(layout.Context{Ops: &ops}, "math")
	if !a.modal.Visible || a.modal.FormInputs[0].Text() != "x^2" {
		t.Fatal("table formula was treated as new insertion")
	}
	a.modal.FormInputs[0].SetText("y_1")
	a.modal.OnAction("confirm")
	if v.text() != "| 值 |\n| --- |\n| $y_1$ |\n" {
		t.Fatalf("table formula replacement %q", v.text())
	}
	if v.parseCancel != nil {
		v.parseCancel()
	}
}
