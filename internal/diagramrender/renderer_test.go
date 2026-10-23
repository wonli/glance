package diagramrender

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"glance/internal/nativewebview"
)

type captureEngine struct {
	sync.Mutex
	calls []nativewebview.RenderRequest
}

type blockingFirstRenderEngine struct {
	sync.Mutex
	calls        int
	firstStarted chan struct{}
	releaseFirst chan struct{}
}

type fakeSessionEngine struct {
	*captureEngine
	openRequest nativewebview.SessionRequest
	session     *fakeChartSession
	opened      int
}

func (e *fakeSessionEngine) OpenSession(_ context.Context, req nativewebview.SessionRequest) (nativewebview.Session, error) {
	e.openRequest = req
	e.opened++
	e.session = &fakeChartSession{result: nativewebview.RenderResult{Image: image.NewNRGBA(image.Rect(0, 0, 20, 10)), LogicalSize: image.Pt(20, 10)}}
	return e.session, nil
}

type fakeChartSession struct {
	result     nativewebview.RenderResult
	dispatches []nativewebview.PointerEvent
	closed     bool
	closeCalls int
}

func (s *fakeChartSession) Dispatch(_ context.Context, event nativewebview.PointerEvent) (nativewebview.RenderResult, error) {
	if s.closed {
		return nativewebview.RenderResult{}, nativewebview.ErrSessionClosed
	}
	s.dispatches = append(s.dispatches, event)
	return s.result, nil
}

func (s *fakeChartSession) Snapshot(context.Context) (nativewebview.RenderResult, error) {
	if s.closed {
		return nativewebview.RenderResult{}, nativewebview.ErrSessionClosed
	}
	return s.result, nil
}

func (s *fakeChartSession) Close() error {
	s.closeCalls++
	s.closed = true
	return nil
}

func (e *captureEngine) EvaluateAndSnapshot(_ context.Context, req nativewebview.RenderRequest) (nativewebview.RenderResult, error) {
	e.Lock()
	e.calls = append(e.calls, req)
	e.Unlock()
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	img.SetNRGBA(10, 10, color.NRGBA{R: 20, G: 80, B: 180, A: 255})
	return nativewebview.RenderResult{Image: img, LogicalSize: image.Pt(20, 10)}, nil
}

func (e *blockingFirstRenderEngine) EvaluateAndSnapshot(_ context.Context, _ nativewebview.RenderRequest) (nativewebview.RenderResult, error) {
	e.Lock()
	e.calls++
	call := e.calls
	e.Unlock()
	if call == 1 {
		close(e.firstStarted)
		<-e.releaseFirst
	}
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	return nativewebview.RenderResult{Image: img, LogicalSize: image.Pt(20, 10)}, nil
}

func (*blockingFirstRenderEngine) Close() error { return nil }

func (*captureEngine) Close() error { return nil }

const testVegaLiteSpec = `{"$schema":"https://vega.github.io/schema/vega-lite/v6.json","width":160,"height":90,"data":{"values":[{"label":"A","value":2},{"label":"B","value":5}]},"transform":[{"calculate":"datum.value+1","as":"adjusted"}],"mark":"bar","encoding":{"x":{"field":"label","type":"nominal"},"y":{"field":"adjusted","type":"quantitative"}}}`

const testVegaSpec = `{"$schema":"https://vega.github.io/schema/vega/v6.json","width":160,"height":90,"data":[{"name":"table","values":[{"x":30,"y":20}]}],"marks":[{"type":"symbol","from":{"data":"table"},"encode":{"update":{"x":{"field":"x"},"y":{"field":"y"},"size":{"value":800},"fill":{"value":"#2563eb"}}}}]}`

const testInteractiveVegaSpec = `{"$schema":"https://vega.github.io/schema/vega/v6.json","width":100,"height":100,"padding":0,"signals":[{"name":"selected","value":false,"on":[{"events":"click","update":"!selected"}]}],"data":[{"name":"table","values":[{"x":50,"y":50}]}],"marks":[{"type":"symbol","from":{"data":"table"},"encode":{"update":{"x":{"field":"x"},"y":{"field":"y"},"size":{"value":2500},"fill":{"signal":"selected ? '#ef4444' : '#2563eb'"}}}}]}`

func TestRenderVegaLiteProducesVisibleSnapshot(t *testing.T) {
	r := testRenderer(t)
	out, err := r.Render(context.Background(), Request{Kind: "vega-lite", Source: []byte(testVegaLiteSpec), Density: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Image == nil || out.Image.Bounds().Empty() || out.LogicalSize.X <= 0 || out.LogicalSize.Y <= 0 {
		t.Fatalf("empty Vega-Lite output: %+v", out)
	}
	nonWhite := 0
	for y := out.Image.Bounds().Min.Y; y < out.Image.Bounds().Max.Y; y++ {
		for x := out.Image.Bounds().Min.X; x < out.Image.Bounds().Max.X; x++ {
			r, g, b, a := out.Image.At(x, y).RGBA()
			if a > 0 && (r < 60000 || g < 60000 || b < 60000) {
				nonWhite++
			}
		}
	}
	if nonWhite < 100 {
		t.Fatal("Vega-Lite snapshot has no visible marks")
	}
}

func TestRenderVegaProducesVisibleSnapshot(t *testing.T) {
	r := testRenderer(t)
	out, err := r.Render(context.Background(), Request{Kind: "vega", Source: []byte(testVegaSpec), Density: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Image == nil || out.Image.Bounds().Empty() || out.LogicalSize.X <= 0 || out.LogicalSize.Y <= 0 {
		t.Fatalf("empty Vega output: %+v", out)
	}
	nonWhite := 0
	for y := out.Image.Bounds().Min.Y; y < out.Image.Bounds().Max.Y; y++ {
		for x := out.Image.Bounds().Min.X; x < out.Image.Bounds().Max.X; x++ {
			r, g, b, a := out.Image.At(x, y).RGBA()
			if a > 0 && (r < 60000 || g < 60000 || b < 60000) {
				nonWhite++
			}
		}
	}
	if nonWhite < 100 {
		t.Fatal("Vega snapshot has no visible marks")
	}
}

func TestVegaChartSessionPreservesInteractionState(t *testing.T) {
	r := testRenderer(t)
	chart, err := r.OpenChart(context.Background(), Request{Kind: "vega", Source: []byte(testInteractiveVegaSpec), Density: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = chart.Close() })
	before, err := chart.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chart.Dispatch(context.Background(), nativewebview.PointerEvent{Kind: nativewebview.PointerDown, X: 50, Y: 50, Buttons: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := chart.Dispatch(context.Background(), nativewebview.PointerEvent{Kind: nativewebview.PointerUp, X: 50, Y: 50}); err != nil {
		t.Fatal(err)
	}
	after, err := chart.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	x, y := before.Image.Bounds().Dx()/2, before.Image.Bounds().Dy()/2
	br, bg, bb, _ := before.Image.At(x, y).RGBA()
	ar, ag, ab, _ := after.Image.At(x, y).RGBA()
	if br == ar && bg == ag && bb == ab {
		t.Fatalf("Vega signal did not survive pointer dispatch: center pixel unchanged (%04x,%04x,%04x)", br, bg, bb)
	}
	if err := chart.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := chart.Snapshot(context.Background()); !errors.Is(err, nativewebview.ErrSessionClosed) {
		t.Fatalf("closed chart snapshot error = %v, want ErrSessionClosed", err)
	}
}

func TestVegaChartSessionsRemainIndependentAtLiveLimit(t *testing.T) {
	r := testRenderer(t)
	started := time.Now()
	charts := make([]ChartSession, 0, maxLiveChartSessions)
	for i := 0; i < maxLiveChartSessions; i++ {
		chart, err := r.OpenChart(context.Background(), Request{Kind: "vega", Source: []byte(testInteractiveVegaSpec), Density: 2})
		if err != nil {
			t.Fatalf("open chart %d: %v", i, err)
		}
		charts = append(charts, chart)
	}
	t.Logf("opened %d persistent chart sessions in %s", len(charts), time.Since(started).Round(time.Millisecond))
	defer func() {
		for _, chart := range charts {
			_ = chart.Close()
		}
	}()
	otherBefore, err := charts[1].Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []nativewebview.PointerEvent{
		{Kind: nativewebview.PointerDown, X: 50, Y: 50, Buttons: 1},
		{Kind: nativewebview.PointerUp, X: 50, Y: 50},
	} {
		if _, err := charts[0].Dispatch(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := charts[0].Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	otherAfter, err := charts[1].Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	center := func(result Result) [3]uint32 {
		x, y := result.Image.Bounds().Dx()/2, result.Image.Bounds().Dy()/2
		r, g, b, _ := result.Image.At(x, y).RGBA()
		return [3]uint32{r, g, b}
	}
	if center(changed) == center(otherBefore) {
		t.Fatal("first chart did not update after click")
	}
	if center(otherAfter) != center(otherBefore) {
		t.Fatal("pointer input in one session changed another chart")
	}
	if _, err := r.OpenChart(context.Background(), Request{Kind: "vega", Source: []byte(testInteractiveVegaSpec), Density: 2}); err == nil {
		t.Fatal("renderer exceeded live chart session limit")
	}
	if err := charts[0].Close(); err != nil {
		t.Fatal(err)
	}
	replacement, err := r.OpenChart(context.Background(), Request{Kind: "vega", Source: []byte(testInteractiveVegaSpec), Density: 2})
	if err != nil {
		t.Fatalf("released session slot was not reusable: %v", err)
	}
	_ = replacement.Close()
}

func TestRenderInvalidVegaLiteSpecReturnsDiagnostic(t *testing.T) {
	r := testRenderer(t)
	_, err := r.Render(context.Background(), Request{Kind: "vega-lite", Source: []byte(`{"data":{"values":[]},"mark":"unsupported-mark"}`), Density: 2})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "vega-lite") {
		t.Fatalf("invalid Vega-Lite spec returned no useful diagnostic: %v", err)
	}
}

func TestRendererCacheSeparatesVegaKinds(t *testing.T) {
	engine := &captureEngine{}
	r := New(Bundle{})
	r.engine = engine
	defer r.Close()
	for _, kind := range []string{"vega", "vega-lite"} {
		if _, err := r.Render(context.Background(), Request{Kind: kind, Source: []byte(`{"data":{"values":[]}}`), Density: 2}); err != nil {
			t.Fatal(err)
		}
	}
	engine.Lock()
	calls := len(engine.calls)
	engine.Unlock()
	if calls != 2 {
		t.Fatalf("same source under Vega and Vega-Lite reused one render, calls=%d", calls)
	}
}

func TestOpenChartForwardsEventsAndClosesWithRenderer(t *testing.T) {
	engine := &fakeSessionEngine{captureEngine: &captureEngine{}}
	r := New(Bundle{})
	r.engine = engine
	chart, err := r.OpenChart(context.Background(), Request{Kind: "vega-lite", Source: []byte(testVegaLiteSpec), Density: 2})
	if err != nil {
		t.Fatal(err)
	}
	if engine.openRequest.HTML == "" || engine.openRequest.Script == "" || engine.openRequest.Density != 2 {
		t.Fatalf("incomplete persistent session request: %+v", engine.openRequest)
	}
	event := nativewebview.PointerEvent{Kind: nativewebview.PointerDown, X: 11.5, Y: 8.25, Button: 0, Buttons: 1, Shift: true}
	if _, err := chart.Dispatch(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(engine.session.dispatches) != 1 || engine.session.dispatches[0] != event {
		t.Fatalf("event was not forwarded unchanged: %+v", engine.session.dispatches)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if engine.session.closeCalls != 1 {
		t.Fatalf("renderer close released %d sessions, want 1", engine.session.closeCalls)
	}
	if _, err := chart.Snapshot(context.Background()); err == nil {
		t.Fatal("closed chart accepted a snapshot")
	}
}

func TestOpenChartRejectsNonInteractiveKinds(t *testing.T) {
	r := New(Bundle{})
	defer r.Close()
	if _, err := r.OpenChart(context.Background(), Request{Kind: "mermaid", Source: []byte("flowchart TD; A-->B"), Density: 2}); err == nil {
		t.Fatal("Mermaid should not open as a Vega chart session")
	}
}

func TestOpenChartBoundsLiveSessionsAndReusesReleasedSlot(t *testing.T) {
	engine := &fakeSessionEngine{captureEngine: &captureEngine{}}
	r := New(Bundle{})
	r.engine = engine
	defer r.Close()
	charts := make([]ChartSession, 0, maxLiveChartSessions)
	for i := 0; i < maxLiveChartSessions; i++ {
		chart, err := r.OpenChart(context.Background(), Request{Kind: "vega", Source: []byte(testVegaSpec), Density: 2})
		if err != nil {
			t.Fatalf("open chart %d: %v", i, err)
		}
		charts = append(charts, chart)
	}
	if _, err := r.OpenChart(context.Background(), Request{Kind: "vega", Source: []byte(testVegaSpec), Density: 2}); err == nil {
		t.Fatal("renderer exceeded live chart session limit")
	}
	if err := charts[0].Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.OpenChart(context.Background(), Request{Kind: "vega", Source: []byte(testVegaSpec), Density: 2}); err != nil {
		t.Fatalf("released session slot was not reusable: %v", err)
	}
}

func testRenderer(t *testing.T) *Renderer {
	t.Helper()
	eng, err := nativewebview.Default()
	if err == nil {
		r := &Renderer{
			bundle: Bundle{Version: "native-webview", MermaidVersion: "11.4.1"},
			queue:  make(chan struct{}, 16),
			active: make(chan struct{}, 1),
			engine: eng,
		}
		t.Cleanup(func() { r.Close() })
		return r
	}
	path := os.Getenv("GLANCE_TEST_DIAGRAM_RUNTIME")
	if path == "" {
		t.Skip("set GLANCE_TEST_DIAGRAM_RUNTIME to run actual offline renderer tests")
	}
	b, err := ResolveBundle(filepath.Join(path, "..", "glance-office"))
	if err != nil {
		t.Fatal(err)
	}
	r := New(b)
	t.Cleanup(func() { r.Close() })
	return r
}
func TestRenderChineseFlowchart(t *testing.T) {
	r := testRenderer(t)
	o, e := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte("flowchart TD\n A[开始] --> B{条件}\n B -->|是| C[完成]"), Density: 2})
	if e != nil {
		t.Fatal(e)
	}
	if o.LogicalSize.X < 50 || o.LogicalSize.Y < 50 || o.Image.Bounds().Dx() != o.LogicalSize.X*2 {
		t.Fatalf("bad dimensions %+v %v", o.LogicalSize, o.Image.Bounds())
	}
	pixels := 0
	for y := 0; y < o.Image.Bounds().Dy(); y++ {
		for x := 0; x < o.Image.Bounds().Dx(); x++ {
			red, green, blue, a := o.Image.At(x, y).RGBA()
			if a > 0 && (red < 60000 || green < 60000 || blue < 60000) {
				pixels++
			}
		}
	}
	if pixels < 100 {
		t.Fatal("diagram has no visible pixels")
	}
}
func TestRenderSVGTextAndForeignObject(t *testing.T) {
	r := testRenderer(t)
	o, e := r.Render(context.Background(), Request{Kind: "svg", Density: 2, Source: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><rect width="120" height="60" fill="red"/><foreignObject width="120" height="60"><div xmlns="http://www.w3.org/1999/xhtml">中文标签</div></foreignObject></svg>`)})
	if e != nil {
		t.Fatal(e)
	}
	if o.LogicalSize.X != 120 || o.Image.Bounds().Dx() != 240 {
		t.Fatal(o.LogicalSize)
	}
}
func TestRenderEnforcesLimits(t *testing.T) {
	r := New(Bundle{})
	defer r.Close()
	for _, q := range []Request{{Kind: "mermaid", Density: 2, Source: []byte(strings.Repeat("a", 65537))}, {Kind: "svg", Density: 2, Source: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100000" height="100000"/>`)}} {
		ctx, c := context.WithTimeout(context.Background(), time.Second)
		_, e := r.Render(ctx, q)
		c()
		if e == nil {
			t.Fatal("unbounded request accepted")
		}
	}
}

func TestQueuedRenderGetsItsTimeoutAfterRendererAdmission(t *testing.T) {
	engine := &blockingFirstRenderEngine{firstStarted: make(chan struct{}), releaseFirst: make(chan struct{})}
	r := New(Bundle{})
	r.engine = engine
	t.Cleanup(func() {
		select {
		case <-engine.releaseFirst:
		default:
			close(engine.releaseFirst)
		}
		_ = r.Close()
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte("graph TD; A-->B"), Density: 2})
		firstDone <- err
	}()
	select {
	case <-engine.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first WebView render did not start")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte("graph TD; C-->D"), Density: 2})
		secondDone <- err
	}()

	// The first WebView call is deliberately uninterruptible, as a native
	// WebView navigation can outlive its Go context while it is being canceled.
	// A queued render must not spend its own execution timeout waiting here.
	time.Sleep(11 * time.Second)
	select {
	case err := <-secondDone:
		t.Fatalf("queued render finished before admission: %v", err)
	default:
	}

	close(engine.releaseFirst)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first WebView render did not finish after release")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("queued render inherited the first render's timeout: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued render did not run after admission")
	}
}
func TestCanceledRenderAllowsNextJob(t *testing.T) {
	r := testRenderer(t)
	ctx, c := context.WithCancel(context.Background())
	c()
	if _, e := r.Render(ctx, Request{Kind: "mermaid", Density: 2, Source: []byte("graph TD;A-->B")}); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e := r.Render(context.Background(), Request{Kind: "mermaid", Density: 2, Source: []byte("graph TD;A-->B")}); e != nil {
		t.Fatal(e)
	}
}

func TestRenderMathFormula(t *testing.T) {
	r := testRenderer(t)
	// Matrix formula from complex_markdown.md
	tex := `\begin{bmatrix} a & b \\ c & d \end{bmatrix}^{-1} = \frac{1}{ad-bc} \begin{bmatrix} d & -b \\ -c & a \end{bmatrix}`
	o, err := r.Render(context.Background(), Request{Kind: "math", Source: []byte(tex), Density: 2})
	if err != nil {
		t.Fatalf("render math failed: %v", err)
	}
	t.Logf("Matrix formula logical size: %+v, image size: %+v", o.LogicalSize, o.Image.Bounds())
	if o.LogicalSize.X < 50 || o.LogicalSize.Y < 20 || o.Image.Bounds().Dx() != o.LogicalSize.X*2 {
		t.Fatalf("bad math dimensions %+v %v", o.LogicalSize, o.Image.Bounds())
	}
	pixels := 0
	for y := 0; y < o.Image.Bounds().Dy(); y++ {
		for x := 0; x < o.Image.Bounds().Dx(); x++ {
			_, _, _, a := o.Image.At(x, y).RGBA()
			if a > 0 {
				pixels++
			}
		}
	}
	if pixels < 100 {
		t.Fatal("rendered math formula has no visible pixels")
	}

	// Align formula from complex_markdown.md
	texAlign := "\\begin{align}\n\\nabla \\times \\vec{\\mathbf{B}} -\\, \\frac{\\partial \\vec{\\mathbf{E}}}{\\partial t} &= \\mu_0 \\vec{\\mathbf{j}} \\\\\n\\nabla \\cdot \\vec{\\mathbf{E}} &= \\frac{\\rho}{\\epsilon_0}\n\\end{align}"
	o2, err := r.Render(context.Background(), Request{Kind: "math", Source: []byte(texAlign), Density: 2})
	if err != nil {
		t.Fatalf("render align math failed: %v", err)
	}
	if o2.LogicalSize.X < 50 || o2.LogicalSize.Y < 20 {
		t.Fatalf("bad align math dimensions %+v", o2.LogicalSize)
	}
}
