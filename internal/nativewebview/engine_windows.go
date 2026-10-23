//go:build windows

package nativewebview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type windowsEdgeEngine struct {
	mu             sync.Mutex
	closed         bool
	executablePath string
	browserContext context.Context
	cancelBrowser  context.CancelFunc
	cancelAlloc    context.CancelFunc
	userDataDir    string
	sessions       map[*windowsChartSession]struct{}
}

type windowsChartSession struct {
	mu     sync.Mutex
	engine *windowsEdgeEngine
	ctx    context.Context
	cancel context.CancelFunc
	width  int
	height int
	closed bool
}

func newPlatformEngine() (Engine, error) {
	exe := findSystemBrowserWindows()
	if exe == "" {
		return nil, errors.New("no system Microsoft Edge or Chrome found on Windows")
	}
	return &windowsEdgeEngine{executablePath: exe}, nil
}

func findSystemBrowserWindows() string {
	var candidates []string
	progFiles86 := os.Getenv("ProgramFiles(x86)")
	progFiles := os.Getenv("ProgramFiles")
	localApp := os.Getenv("LocalAppData")

	if progFiles86 != "" {
		candidates = append(candidates, filepath.Join(progFiles86, "Microsoft", "Edge", "Application", "msedge.exe"))
		candidates = append(candidates, filepath.Join(progFiles86, "Google", "Chrome", "Application", "chrome.exe"))
	}
	if progFiles != "" {
		candidates = append(candidates, filepath.Join(progFiles, "Microsoft", "Edge", "Application", "msedge.exe"))
		candidates = append(candidates, filepath.Join(progFiles, "Google", "Chrome", "Application", "chrome.exe"))
	}
	if localApp != "" {
		candidates = append(candidates, filepath.Join(localApp, "Microsoft", "Edge", "Application", "msedge.exe"))
		candidates = append(candidates, filepath.Join(localApp, "Google", "Chrome", "Application", "chrome.exe"))
	}
	candidates = append(candidates,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	)

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func (e *windowsEdgeEngine) ensureBrowser() (context.Context, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, ErrEngineClosed
	}
	if e.browserContext != nil && e.browserContext.Err() == nil {
		return e.browserContext, nil
	}

	dir, err := os.MkdirTemp("", "glance-webview-")
	if err != nil {
		return nil, err
	}
	e.userDataDir = dir

	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.ExecPath(e.executablePath),
		chromedp.UserDataDir(dir),
		chromedp.Flag("no-sandbox", false),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	bCtx, cancelBrowser := chromedp.NewContext(alloc)
	e.browserContext = bCtx
	e.cancelAlloc = cancelAlloc
	e.cancelBrowser = cancelBrowser
	return bCtx, nil
}

func (e *windowsEdgeEngine) EvaluateAndSnapshot(ctx context.Context, req RenderRequest) (RenderResult, error) {
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}

	browser, err := e.ensureBrowser()
	if err != nil {
		return RenderResult{}, err
	}

	taskCtx, cancelTask := chromedp.NewContext(browser)
	defer cancelTask()

	html := req.HTML
	if html == "" {
		html = "<!doctype html><html><head><meta charset='utf-8'><style>html,body{margin:0;background:transparent;overflow:hidden;}</style></head><body><div id='output'></div></body></html>"
	}

	if err := chromedp.Do(taskCtx, chromedp.Navigate("about:blank")); err != nil {
		return RenderResult{}, err
	}
	if err := restrictExternalRequests(taskCtx); err != nil {
		return RenderResult{}, err
	}

	err = chromedp.Do(taskCtx, chromedp.Func(func(c context.Context, t *chromedp.Target) error {
		frame, e := cdp.Call(c, t, page.GetFrameTree, cdp.Empty{})
		if e != nil {
			return e
		}
		_, e = cdp.Call(c, t, page.SetDocumentContent, page.SetDocumentContentParams{FrameID: frame.FrameTree.Frame.ID, HTML: html})
		return e
	}))
	if err != nil {
		return RenderResult{}, err
	}

	evalScript, err := wrapEvalScript(req.Script, req.Args)
	if err != nil {
		return RenderResult{}, err
	}

	geom, err := chromedp.Run(taskCtx, chromedp.Evaluate[struct {
		Width  int `json:"width"`
		Height int `json:"height"`
		Data   any `json:"data"`
	}](evalScript, chromedp.EvalAwaitPromise))
	if err != nil {
		return RenderResult{}, err
	}
	if geom.Width <= 0 || geom.Height <= 0 || int64(geom.Width)*int64(geom.Height)*4 > 16<<20 {
		return RenderResult{}, errors.New("invalid layout dimensions or exceeds 16 megapixels")
	}

	scale := req.Density
	if scale <= 0 {
		scale = 2
	}
	if err := chromedp.Do(taskCtx, chromedp.EmulateViewport(int64(geom.Width), int64(geom.Height), chromedp.EmulateScale(float64(scale)))); err != nil {
		return RenderResult{}, err
	}

	data, err := chromedp.Run(taskCtx, chromedp.CaptureScreenshot())
	if err != nil {
		return RenderResult{}, err
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return RenderResult{}, fmt.Errorf("decode rendered PNG: %w", err)
	}

	return RenderResult{
		Image:       img,
		LogicalSize: image.Pt(geom.Width, geom.Height),
		Data:        geom.Data,
	}, nil
}

func wrapEvalScript(script string, args map[string]any) (string, error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("marshal script args: %w", err)
	}
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return "", errors.New("empty WebView script")
	}
	if strings.HasPrefix(trimmed, "(") || strings.HasPrefix(trimmed, "async") {
		return "(async()=>{const args=" + string(argsJSON) + ";return await (" + trimmed + ");})()", nil
	}
	return "(async()=>{const args=" + string(argsJSON) + ";\n" + script + "\n})()", nil
}

func (e *windowsEdgeEngine) OpenSession(ctx context.Context, req SessionRequest) (Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.HTML == "" || req.Script == "" {
		return nil, errors.New("session HTML and initialization script are required")
	}
	browser, err := e.ensureBrowser()
	if err != nil {
		return nil, err
	}
	targetCtx, cancelTarget := chromedp.NewContext(browser)
	loadCtx, cancelLoad := mergeContext(ctx, targetCtx)
	loadErr := loadHTML(loadCtx, req.HTML)
	cancelLoad()
	if loadErr != nil {
		cancelTarget()
		return nil, loadErr
	}
	geom, err := runSessionScript(ctx, targetCtx, req.Script)
	if err != nil {
		cancelTarget()
		return nil, err
	}
	density := req.Density
	if density <= 0 {
		density = 2
	}
	if err := chromedp.Do(targetCtx, chromedp.EmulateViewport(int64(geom.Width), int64(geom.Height), chromedp.EmulateScale(float64(density)))); err != nil {
		cancelTarget()
		return nil, err
	}
	session := &windowsChartSession{engine: e, ctx: targetCtx, cancel: cancelTarget, width: geom.Width, height: geom.Height}
	if _, err := session.snapshot(ctx); err != nil {
		session.Close()
		return nil, err
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		session.Close()
		return nil, ErrEngineClosed
	}
	if e.sessions == nil {
		e.sessions = make(map[*windowsChartSession]struct{})
	}
	e.sessions[session] = struct{}{}
	e.mu.Unlock()
	return session, nil
}

type sessionGeometry struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func loadHTML(ctx context.Context, html string) error {
	if err := chromedp.Do(ctx, chromedp.Navigate("about:blank")); err != nil {
		return err
	}
	if err := restrictExternalRequests(ctx); err != nil {
		return err
	}
	return chromedp.Do(ctx, chromedp.Func(func(c context.Context, target *chromedp.Target) error {
		tree, err := cdp.Call(c, target, page.GetFrameTree, cdp.Empty{})
		if err != nil {
			return err
		}
		_, err = cdp.Call(c, target, page.SetDocumentContent, page.SetDocumentContentParams{FrameID: tree.FrameTree.Frame.ID, HTML: html})
		return err
	}))
}

func restrictExternalRequests(ctx context.Context) error {
	return chromedp.Do(ctx, chromedp.Func(func(c context.Context, target *chromedp.Target) error {
		if _, err := cdp.Call(c, target, network.Enable, network.EnableParams{}); err != nil {
			return err
		}
		patterns := blockedExternalRequestPatterns()
		blocked := make([]*network.BlockPattern, 0, len(patterns))
		for _, pattern := range patterns {
			blocked = append(blocked, &network.BlockPattern{URLPattern: pattern, Block: true})
		}
		_, err := cdp.Call(c, target, network.SetBlockedURLs, network.SetBlockedURLsParams{URLPatterns: blocked})
		return err
	}))
}

func runSessionScript(ctx, targetCtx context.Context, script string) (sessionGeometry, error) {
	if err := ctx.Err(); err != nil {
		return sessionGeometry{}, err
	}
	wrapped := "(async()=>{return await (" + script + ");})()"
	callCtx, cancel := mergeContext(ctx, targetCtx)
	defer cancel()
	return chromedp.Run(callCtx, chromedp.Evaluate[sessionGeometry](wrapped, chromedp.EvalAwaitPromise))
}

func mergeContext(request, target context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(target)
	stop := context.AfterFunc(request, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (s *windowsChartSession) Dispatch(ctx context.Context, event PointerEvent) (RenderResult, error) {
	if err := validatePointerEvent(event); err != nil {
		return RenderResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RenderResult{}, ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return RenderResult{}, err
	}
	script := `(async()=>{
		const e=` + string(eventJSON) + `;
		const target=document.elementFromPoint(e.x,e.y)||document.getElementById('output');
		const init={bubbles:true,cancelable:true,clientX:e.x,clientY:e.y,button:e.button,buttons:e.buttons,shiftKey:e.shift,ctrlKey:e.control,pointerType:'mouse'};
		if(e.kind==='leave'){
			target.dispatchEvent(new PointerEvent('pointerout',init));
			target.dispatchEvent(new MouseEvent('mouseout',init));
			target.dispatchEvent(new PointerEvent('pointerleave',init));
			target.dispatchEvent(new MouseEvent('mouseleave',init));
		}else if(e.kind==='wheel'){
			target.dispatchEvent(new WheelEvent('wheel',{...init,deltaX:e.deltaX,deltaY:e.deltaY}));
		}else{
			const names={move:['pointermove','mousemove'],down:['pointerdown','mousedown'],up:['pointerup','mouseup']};
			for(const name of names[e.kind]||[]){const Ctor=name.startsWith('pointer')?PointerEvent:MouseEvent;target.dispatchEvent(new Ctor(name,init));}
			if(e.kind==='down')window.__glancePointerDown={x:e.x,y:e.y};
			if(e.kind==='up'){
				const down=window.__glancePointerDown;
				if(down&&Math.hypot(e.x-down.x,e.y-down.y)<=4)target.dispatchEvent(new MouseEvent('click',init));
				window.__glancePointerDown=null;
			}
		}
		if(window.__glanceVegaView)await window.__glanceVegaView.runAsync();
		return {width:window.__glanceVegaWidth,height:window.__glanceVegaHeight};
	})()`
	geom, err := runSessionScript(ctx, s.ctx, script)
	if err != nil {
		return RenderResult{}, err
	}
	if geom.Width != s.width || geom.Height != s.height {
		return RenderResult{}, errors.New("chart dimensions changed during session")
	}
	return s.snapshotLocked(ctx)
}

func (s *windowsChartSession) Snapshot(ctx context.Context) (RenderResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RenderResult{}, ErrSessionClosed
	}
	return s.snapshotLocked(ctx)
}

func (s *windowsChartSession) snapshot(ctx context.Context) (RenderResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked(ctx)
}

func (s *windowsChartSession) snapshotLocked(ctx context.Context) (RenderResult, error) {
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}
	callCtx, cancel := mergeContext(ctx, s.ctx)
	defer cancel()
	if _, err := chromedp.Run(callCtx, chromedp.Evaluate[bool](`(async()=>{if(window.__glanceVegaView)await window.__glanceVegaView.runAsync();return true;})()`, chromedp.EvalAwaitPromise)); err != nil {
		return RenderResult{}, err
	}
	data, err := chromedp.Run(callCtx, chromedp.CaptureScreenshot())
	if err != nil {
		return RenderResult{}, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return RenderResult{}, fmt.Errorf("decode rendered PNG: %w", err)
	}
	return RenderResult{Image: img, LogicalSize: image.Pt(s.width, s.height)}, nil
}

func (s *windowsChartSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.engine.mu.Lock()
	delete(s.engine.sessions, s)
	s.engine.mu.Unlock()
	return nil
}

func (e *windowsEdgeEngine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	sessions := make([]*windowsChartSession, 0, len(e.sessions))
	for session := range e.sessions {
		sessions = append(sessions, session)
	}
	e.sessions = nil
	cancelBrowser, cancelAlloc, userDataDir := e.cancelBrowser, e.cancelAlloc, e.userDataDir
	e.mu.Unlock()
	for _, session := range sessions {
		session.cancel()
		session.mu.Lock()
		session.closed = true
		session.mu.Unlock()
	}
	if cancelBrowser != nil {
		cancelBrowser()
	}
	if cancelAlloc != nil {
		cancelAlloc()
	}
	if userDataDir != "" {
		_ = os.RemoveAll(userDataDir)
		e.mu.Lock()
		e.userDataDir = ""
		e.mu.Unlock()
	}
	return nil
}
