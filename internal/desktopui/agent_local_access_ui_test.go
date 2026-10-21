package desktopui

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/localaccess"
)

type localAccessRequestResult struct {
	scope localaccess.ScopeSummary
	err   error
}

func newLocalAccessTestRuntime(t *testing.T, picker func(context.Context, filePickerLabels) (string, error)) (*desktopRuntime, *App, *localaccess.Broker) {
	t.Helper()
	runtime := &desktopRuntime{windows: make(map[uint64]*App), closingWindows: make(map[uint64]bool)}
	app := &App{
		desktopRuntime: runtime,
		windowID:       19,
		shutdown:       make(chan struct{}),
		theme:          material.NewTheme(),
		agentUI:        newAgentUIState(),
	}
	app.agentUI.localAccess.pickerRunner = picker
	runtime.attachWindow(app.windowID, app)
	broker := localaccess.NewBroker(runtime)
	t.Cleanup(func() {
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		broker.Close()
		app.stopAndWaitManagedBackground()
		runtime.detachWindow(app.windowID)
	})
	return runtime, app, broker
}

func startLocalAccessRequest(ctx context.Context, broker *localaccess.Broker, windowID uint64) <-chan localAccessRequestResult {
	result := make(chan localAccessRequestResult, 1)
	go func() {
		scope, err := broker.RequestDirectory(ctx, "turn-local-access-ui", windowID,
			"汇总本月工作", "请总结本月工作，并查看这个目录中的 Git 提交记录。")
		result <- localAccessRequestResult{scope: scope, err: err}
	}()
	return result
}

func pumpLocalAccessUntil(t *testing.T, app *App, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		app.processAgentLocalAccessUI()
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for local-access UI state")
}

func awaitLocalAccessResult(t *testing.T, app *App, result <-chan localAccessRequestResult) localAccessRequestResult {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		app.processAgentLocalAccessUI()
		select {
		case got := <-result:
			return got
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for local-access request result")
	return localAccessRequestResult{}
}

func TestAgentLocalAccessPromptAllowOnceAndRenderRequestDetails(t *testing.T) {
	root := t.TempDir()
	var pickerCalls atomic.Int32
	_, app, broker := newLocalAccessTestRuntime(t, func(context.Context, filePickerLabels) (string, error) {
		pickerCalls.Add(1)
		return root, nil
	})
	result := startLocalAccessRequest(context.Background(), broker, app.windowID)
	pumpLocalAccessUntil(t, app, func() bool { return app.activeAgentLocalAccessConsent() != nil })
	consent := app.activeAgentLocalAccessConsent()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonicalize test root: %v", err)
	}
	if consent.request.FolderPath != canonicalRoot || consent.request.FolderName != filepath.Base(root) {
		t.Fatalf("consent scope = path %q, name %q; want %q / %q", consent.request.FolderPath, consent.request.FolderName, canonicalRoot, filepath.Base(root))
	}
	if consent.request.Purpose != "汇总本月工作" || !strings.Contains(consent.request.UserRequestExcerpt, "Git 提交记录") {
		t.Fatalf("consent request details were lost: %+v", consent.request)
	}
	if pickerCalls.Load() != 1 {
		t.Fatalf("folder picker calls = %d, want 1", pickerCalls.Load())
	}

	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1280, 800))}
	dims := app.layoutAgentConfirmationModal(gtx)
	if dims.Size != image.Pt(1280, 800) {
		t.Fatalf("local-access modal dimensions = %v", dims.Size)
	}
	consent.allow.Click()
	app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentLocalAccessConsent() != nil {
		t.Fatal("allow action did not dismiss consent")
	}
	got := awaitLocalAccessResult(t, app, result)
	if got.err != nil || got.scope.ID == "" {
		t.Fatalf("allow this time result = %+v", got)
	}
	if len(got.scope.Operations) != 3 {
		t.Fatalf("allowed operations = %v, want read-only directory listing, text read, and Git metadata", got.scope.Operations)
	}
	broker.ReleaseTurn("turn-local-access-ui")
}

func TestAgentLocalAccessPromptDenyDoesNotCreateScope(t *testing.T) {
	root := t.TempDir()
	var pickerCalls atomic.Int32
	_, app, broker := newLocalAccessTestRuntime(t, func(context.Context, filePickerLabels) (string, error) {
		pickerCalls.Add(1)
		return root, nil
	})
	result := startLocalAccessRequest(context.Background(), broker, app.windowID)
	pumpLocalAccessUntil(t, app, func() bool { return app.activeAgentLocalAccessConsent() != nil })
	consent := app.activeAgentLocalAccessConsent()
	consent.reject.Click()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1000, 700))}
	app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentLocalAccessConsent() != nil {
		t.Fatal("reject action did not dismiss consent")
	}
	got := awaitLocalAccessResult(t, app, result)
	if !errors.Is(got.err, localaccess.ErrAccessDenied) {
		t.Fatalf("denied request error = %v, want ErrAccessDenied", got.err)
	}
	if got.scope.ID != "" || pickerCalls.Load() != 1 {
		t.Fatalf("denied request created a scope or reopened picker: scope=%+v pickerCalls=%d", got.scope, pickerCalls.Load())
	}
	second := startLocalAccessRequest(context.Background(), broker, app.windowID)
	if got := awaitLocalAccessResult(t, app, second); !errors.Is(got.err, localaccess.ErrAccessDenied) {
		t.Fatalf("second request after denial = %v, want ErrAccessDenied", got.err)
	}
	if app.activeAgentLocalAccessConsent() != nil || pickerCalls.Load() != 1 {
		t.Fatalf("denied directory reopened consent: consent=%v pickerCalls=%d", app.activeAgentLocalAccessConsent() != nil, pickerCalls.Load())
	}
}

func TestAgentLocalAccessPromptCancelAndInvalidWindowFailClosed(t *testing.T) {
	var pickerCalls atomic.Int32
	_, app, broker := newLocalAccessTestRuntime(t, func(context.Context, filePickerLabels) (string, error) {
		pickerCalls.Add(1)
		return "", nil
	})
	if _, err := broker.RequestDirectory(context.Background(), "invalid-window-turn", 0, "读取日志", ""); !errors.Is(err, localaccess.ErrInteractiveAuthorizationUnavailable) {
		t.Fatalf("zero-window request error = %v", err)
	}
	if pickerCalls.Load() != 0 {
		t.Fatalf("invalid window invoked picker %d times", pickerCalls.Load())
	}

	result := startLocalAccessRequest(context.Background(), broker, app.windowID)
	got := awaitLocalAccessResult(t, app, result)
	if !errors.Is(got.err, localaccess.ErrDirectorySelectionCancelled) {
		t.Fatalf("cancelled picker error = %v, want ErrDirectorySelectionCancelled", got.err)
	}
	app.processAgentLocalAccessUI()
	if app.activeAgentLocalAccessConsent() != nil {
		t.Fatal("cancelled picker opened consent modal")
	}
}

func TestAgentLocalAccessPromptCancellationDismissesPendingConsent(t *testing.T) {
	root := t.TempDir()
	_, app, broker := newLocalAccessTestRuntime(t, func(context.Context, filePickerLabels) (string, error) {
		return root, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := startLocalAccessRequest(ctx, broker, app.windowID)
	pumpLocalAccessUntil(t, app, func() bool { return app.activeAgentLocalAccessConsent() != nil })
	cancel()
	pumpLocalAccessUntil(t, app, func() bool { return app.activeAgentLocalAccessConsent() == nil })
	got := awaitLocalAccessResult(t, app, result)
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("cancelled consent error = %v, want context.Canceled", got.err)
	}
}

func TestAgentLocalAccessScrimDismissesAsReject(t *testing.T) {
	_, app, _ := newLocalAccessTestRuntime(t, nil)
	response := make(chan bool, 1)
	consent := &agentLocalAccessConsent{context: context.Background(), response: response}
	app.agentUI.localAccess.activeConsent = consent
	consent.scrim.Click()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(900, 600))}
	app.layoutAgentLocalAccessModal(gtx)
	if app.activeAgentLocalAccessConsent() != nil {
		t.Fatal("scrim click left consent active")
	}
	select {
	case allowed := <-response:
		if allowed {
			t.Fatal("scrim click granted local access")
		}
	default:
		t.Fatal("scrim click did not resolve as denial")
	}
}

func TestAgentLocalAccessModalRendersAtOneAndTwoX(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rt := newDesktopRuntime()
	t.Cleanup(rt.shutdown)
	app := newAppWithRuntime(nil, rt)
	root := filepath.Join(t.TempDir(), "glance-local-access-review-project")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	app.agentUI.localAccess.activeConsent = &agentLocalAccessConsent{
		context: context.Background(),
		request: localaccess.ConsentRequest{
			Purpose:            "总结本月工作并根据提交记录整理重点",
			UserRequestExcerpt: "请总结本月工作，读取项目说明，并查看 Git 提交记录。",
			FolderName:         filepath.Base(canonicalRoot),
			FolderPath:         canonicalRoot,
		},
		response: make(chan bool, 1),
	}
	for _, scale := range []float32{1, 2} {
		t.Run(map[float32]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			logical := image.Pt(960, 680)
			size := image.Pt(int(float32(logical.X)*scale), int(float32(logical.Y)*scale))
			var ops op.Ops
			paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			dims := app.layoutAgentConfirmationModal(gtx)
			if dims.Size != size {
				t.Fatalf("modal covers %v at %.0fx, want %v", dims.Size, scale, size)
			}
			img := renderPresentationTestOps(t, &ops, size)
			if scale == 1 {
				if screenshotPath := os.Getenv("GLANCE_LOCAL_ACCESS_SCREENSHOT"); screenshotPath != "" {
					file, err := os.Create(screenshotPath)
					if err != nil {
						t.Fatal(err)
					}
					if err := png.Encode(file, img); err != nil {
						_ = file.Close()
						t.Fatal(err)
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			darkPixels := 0
			cardBounds := image.Rect(size.X/2-int(300*scale), size.Y/2-int(300*scale), size.X/2+int(300*scale), size.Y/2+int(300*scale)).Intersect(img.Bounds())
			for y := cardBounds.Min.Y; y < cardBounds.Max.Y; y++ {
				for x := cardBounds.Min.X; x < cardBounds.Max.X; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r < 130*257 && g < 130*257 && b < 130*257 {
						darkPixels++
					}
				}
			}
			if darkPixels < int(80*scale*scale) {
				t.Fatalf("local-access card has only %d dark rendered pixels at %.0fx", darkPixels, scale)
			}
		})
	}
}
