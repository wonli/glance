package diagramrender

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"os"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"time"

	"glance/internal/nativewebview"
)

type Request struct {
	Kind    string
	Source  []byte
	Density int
	batch   []Request
}

type Result struct {
	Image       image.Image
	LogicalSize image.Point
	data        any
}

// ChartSession owns one live Vega or Vega-Lite view and its native WebView.
type ChartSession interface {
	Dispatch(context.Context, nativewebview.PointerEvent) (Result, error)
	Snapshot(context.Context) (Result, error)
	Close() error
}

type cacheEntry struct {
	key    [32]byte
	result Result
	cost   int64
}

var fontVersion atomic.Uint64

var (
	sharedHTML     string
	sharedHTMLOnce sync.Once
)

const (
	renderAdmissionTimeout = 30 * time.Second
	renderExecutionTimeout = 10 * time.Second
)

func getSharedHTML() string {
	sharedHTMLOnce.Do(func() {
		jsBytes, _ := assets.ReadFile("assets/mermaid.min.js")
		katexJS, _ := assets.ReadFile("assets/katex.min.js")
		vegaJS, _ := assets.ReadFile("assets/vega.min.js")
		vegaLiteJS, _ := assets.ReadFile("assets/vega-lite.min.js")
		vegaEmbedJS, _ := assets.ReadFile("assets/vega-embed.min.js")
		vegaInterpreterJS, _ := assets.ReadFile("assets/vega-interpreter.min.js")
		// KaTeX CSS is omitted: we use output:'mathml' so no font-dependent HTML renderer is used.
		sharedHTML = fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'; font-src 'none'; object-src 'none'; frame-src 'none'">
<style>
html,body{margin:0;background:transparent;overflow:hidden;}
#output{display:inline-block;padding:4px;}
math{display:block;}
math[display="inline"]{display:inline;}
svg{max-width:none!important;}
</style>
<script>%s</script>
<script>%s</script>
<script>%s</script>
<script>%s</script>
<script>%s</script>
<script>%s</script>
</head><body><div id="output"></div></body></html>`, string(jsBytes), string(katexJS), string(vegaJS), string(vegaLiteJS), string(vegaEmbedJS), string(vegaInterpreterJS))
	})
	return sharedHTML
}

// SetFontVersion invalidates future diagram results after catalog publication.
func SetFontVersion(version uint64) {
	for old := fontVersion.Load(); version > old; old = fontVersion.Load() {
		if fontVersion.CompareAndSwap(old, version) {
			return
		}
	}
}

type Renderer struct {
	bundle          Bundle
	seenFontVersion uint64
	queue           chan struct{}
	active          chan struct{}
	mu              sync.Mutex
	closed          bool
	cache           []cacheEntry
	cacheBytes      int64
	engine          nativewebview.Engine
	sessions        map[uint64]*rendererChartSession
	nextSessionID   uint64
	openingSessions int
}

const maxLiveChartSessions = 4

func New(bundle Bundle) *Renderer {
	return &Renderer{
		bundle: bundle,
		queue:  make(chan struct{}, 16),
		active: make(chan struct{}, 1),
	}
}

var defaults struct {
	sync.Mutex
	r *Renderer
}

var defaultUses atomic.Uint64

func Default() (*Renderer, error) {
	defaultUses.Add(1)
	defaults.Lock()
	defer defaults.Unlock()
	if defaults.r == nil {
		eng, err := nativewebview.Default()
		if err != nil {
			return nil, err
		}
		versions, err := bundledAssetVersions()
		if err != nil {
			return nil, err
		}
		versions.Version = "native-webview"
		r := &Renderer{
			bundle: versions,
			queue:  make(chan struct{}, 16),
			active: make(chan struct{}, 1),
			engine: eng,
		}
		defaults.r = r
	}
	return defaults.r, nil
}

func (r *Renderer) Close() error {
	r.mu.Lock()
	r.closed = true
	r.cache = nil
	r.cacheBytes = 0
	sessions := make([]*rendererChartSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	var firstErr error
	for _, session := range sessions {
		if err := session.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Renderer) OpenChart(ctx context.Context, q Request) (ChartSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.Kind != "vega" && q.Kind != "vega-lite" {
		return nil, errors.New("only Vega and Vega-Lite charts support interactive sessions")
	}
	if err := validate(q); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("diagram renderer closed")
	}
	if len(r.sessions)+r.openingSessions >= maxLiveChartSessions {
		r.mu.Unlock()
		return nil, fmt.Errorf("too many live charts (limit %d)", maxLiveChartSessions)
	}
	r.openingSessions++
	eng := r.engine
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.openingSessions--
		r.mu.Unlock()
	}()

	if eng == nil {
		var err error
		eng, err = nativewebview.Default()
		if err != nil {
			return nil, err
		}
		r.mu.Lock()
		if r.engine == nil {
			r.engine = eng
		} else {
			eng = r.engine
		}
		r.mu.Unlock()
	}
	sessionEngine, ok := eng.(nativewebview.SessionEngine)
	if !ok {
		return nil, errors.New("native WebView does not support persistent chart sessions")
	}
	session, err := sessionEngine.OpenSession(ctx, nativewebview.SessionRequest{
		HTML:    getSharedHTML(),
		Script:  renderVegaScript(q),
		Density: q.Density,
	})
	if err != nil {
		return nil, fmt.Errorf("%s chart session failed: %w", q.Kind, err)
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = session.Close()
		return nil, errors.New("diagram renderer closed")
	}
	if r.sessions == nil {
		r.sessions = make(map[uint64]*rendererChartSession)
	}
	r.nextSessionID++
	chart := &rendererChartSession{id: r.nextSessionID, renderer: r, session: session}
	r.sessions[chart.id] = chart
	r.mu.Unlock()
	return chart, nil
}

type rendererChartSession struct {
	mu       sync.Mutex
	id       uint64
	renderer *Renderer
	session  nativewebview.Session
	closed   bool
}

func (s *rendererChartSession) Dispatch(ctx context.Context, event nativewebview.PointerEvent) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Result{}, nativewebview.ErrSessionClosed
	}
	res, err := s.session.Dispatch(ctx, event)
	return diagramResult(res), err
}

func (s *rendererChartSession) Snapshot(ctx context.Context) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Result{}, nativewebview.ErrSessionClosed
	}
	res, err := s.session.Snapshot(ctx)
	return diagramResult(res), err
}

func (s *rendererChartSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	err := s.session.Close()
	s.mu.Unlock()
	s.renderer.mu.Lock()
	delete(s.renderer.sessions, s.id)
	s.renderer.mu.Unlock()
	return err
}

func diagramResult(res nativewebview.RenderResult) Result {
	return Result{Image: res.Image, LogicalSize: res.LogicalSize}
}

func CloseDefault() error {
	defaults.Lock()
	defer defaults.Unlock()
	return closeDefaultLocked()
}

// IdleCleanup captures the current usage generation without taking a lock on
// the window thread. Run the returned task on a background worker. A renderer
// requested by a newly opened document makes stale cleanup a no-op.
func IdleCleanup() func() {
	generation := defaultUses.Load()
	return func() {
		defaults.Lock()
		defer defaults.Unlock()
		if defaultUses.Load() != generation || defaults.r == nil {
			return
		}
		_ = closeDefaultLocked()
	}
}

func closeDefaultLocked() error {
	r := defaults.r
	defaults.r = nil
	if r != nil {
		r.Close()
		// Finish any cancelled one-shot operation before destroying its page.
		r.active <- struct{}{}
		defer func() { <-r.active }()
	}
	return nativewebview.CloseDefault()
}

func (r *Renderer) Render(ctx context.Context, q Request) (out Result, err error) {
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if err = validate(q); err != nil {
		return out, err
	}
	admissionCtx, cancelAdmission := context.WithTimeout(ctx, renderAdmissionTimeout)
	defer cancelAdmission()

	select {
	case r.queue <- struct{}{}:
	case <-admissionCtx.Done():
		return out, admissionCtx.Err()
	}
	defer func() { <-r.queue }()

	select {
	case r.active <- struct{}{}:
	case <-admissionCtx.Done():
		return out, admissionCtx.Err()
	}
	defer func() { <-r.active }()

	// The execution budget starts only after this request owns the renderer.
	// Markdown preview submits several resources concurrently, but the native
	// WebView is serialized; queue time must not consume a render's budget.
	renderCtx, cancelRender := context.WithTimeout(ctx, renderExecutionTimeout)
	defer cancelRender()

	if e := renderCtx.Err(); e != nil {
		return out, e
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return out, errors.New("diagram renderer closed")
	}

	version := fontVersion.Load()
	if r.seenFontVersion != version {
		r.mu.Lock()
		r.cache = nil
		r.cacheBytes = 0
		r.seenFontVersion = version
		r.mu.Unlock()
	}

	key := sha256.Sum256(append([]byte(fmt.Sprintf("%s/%s/%s/%s/%s/%s/%d/%s\x00", r.bundle.Version, r.bundle.MermaidVersion, r.bundle.VegaVersion, r.bundle.VegaLiteVersion, r.bundle.VegaEmbedVersion, r.bundle.InterpreterVersion, q.Density, fmt.Sprintf("%s/%s/%d/%s/%s", goruntime.GOOS, goruntime.GOARCH, version, os.Getenv("LANG"), q.Kind))), q.Source...))

	r.mu.Lock()
	for i, v := range r.cache {
		if v.key == key {
			r.cache = append(append(r.cache[:i:i], r.cache[i+1:]...), v)
			r.mu.Unlock()
			return v.result, nil
		}
	}
	r.mu.Unlock()

	eng := r.engine
	if eng == nil {
		var e error
		eng, e = nativewebview.Default()
		if e != nil {
			return out, e
		}
		r.engine = eng
	}

	html := getSharedHTML()

	req := nativewebview.RenderRequest{
		HTML:    html,
		Script:  renderScript(q),
		Density: q.Density,
	}

	res, renderErr := eng.EvaluateAndSnapshot(renderCtx, req)
	if contextErr := renderCtx.Err(); contextErr != nil {
		return out, contextErr
	}
	if renderErr != nil {
		if q.Kind == "vega" || q.Kind == "vega-lite" {
			return out, fmt.Errorf("%s chart render failed: %w", q.Kind, renderErr)
		}
		return out, renderErr
	}

	out = Result{
		Image:       res.Image,
		LogicalSize: res.LogicalSize,
		data:        res.Data,
	}

	cost := int64(out.Image.Bounds().Dx()) * int64(out.Image.Bounds().Dy()) * 4
	if cost <= 32<<20 {
		r.mu.Lock()
		for len(r.cache) > 0 && (len(r.cache) >= 32 || r.cacheBytes+cost > 32<<20) {
			r.cacheBytes -= r.cache[0].cost
			r.cache = r.cache[1:]
		}
		r.cache = append(r.cache, cacheEntry{key, out, cost})
		r.cacheBytes += cost
		r.mu.Unlock()
	}

	return out, nil
}
