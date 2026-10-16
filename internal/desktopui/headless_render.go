package desktopui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	giouiapp "gioui.org/app"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/unit"
)

var ErrHeadlessUnavailable = errors.New("Gio headless renderer unavailable")

const (
	maxHeadlessDimension = 4096
	maxHeadlessPixels    = 16 << 20
	maxHeadlessSlides    = 64
)

type HeadlessRenderOptions struct {
	Width, Height int
	Scale         float32
}

type HeadlessTarget struct {
	Name  string
	Image *image.RGBA
}

type HeadlessDocument struct {
	mu     sync.Mutex
	app    *App
	closed bool
}

func OpenHeadlessDocument(path string) (*HeadlessDocument, error) {
	loaded, err := loadDocument(path)
	if err != nil {
		return nil, err
	}

	runtime := newDesktopRuntime()
	app := newAppWithRuntimeAndWindowID(nil, "", runtime, 0)
	// App layout uses Invalidate to request another frame. A zero-value Gio
	// window has no native driver, so those calls are harmless in headless mode.
	app.window = new(giouiapp.Window)
	if err := app.installDocument(loaded); err != nil {
		if state := app.documentSessionState; state != nil && state.documentID != "" {
			runtime.removeSession(state.documentID)
		} else {
			closeLoadedDocument(loaded)
		}
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		return nil, err
	}
	return &HeadlessDocument{app: app}, nil
}
func (d *HeadlessDocument) Render(ctx context.Context, opts HeadlessRenderOptions) ([]HeadlessTarget, error) {
	targets := make([]HeadlessTarget, 0, 1)
	err := d.RenderEach(ctx, opts, func(target HeadlessTarget) error {
		targets = append(targets, target)
		return nil
	})
	return targets, err
}

func (d *HeadlessDocument) RenderEach(ctx context.Context, opts HeadlessRenderOptions, emit func(HeadlessTarget) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if emit == nil {
		return errors.New("headless target callback is nil")
	}
	if d.closed || d.app == nil {
		return errors.New("headless document is closed")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	width, height, scale, err := headlessPixelSize(opts)
	if err != nil {
		return err
	}
	app := d.app
	if app.word != nil {
		if err := awaitHeadlessWordLayout(ctx, app); err != nil {
			return fmt.Errorf("prepare Word page layout: %w", err)
		}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	window, err := headless.NewWindow(width, height)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHeadlessUnavailable, err)
	}
	defer window.Release()

	frame := func() (*image.RGBA, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		app.ops.Reset()
		gtx := layout.Context{
			Ops:    &app.ops,
			Now:    time.Now(),
			Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Constraints{
				Max: image.Pt(width, height),
			},
		}
		app.layout(gtx)
		if err := window.Frame(&app.ops); err != nil {
			return nil, fmt.Errorf("render Gio frame: %w", err)
		}
		pixels := image.NewRGBA(image.Rect(0, 0, width, height))
		if err := window.Screenshot(pixels); err != nil {
			return nil, fmt.Errorf("capture Gio screenshot: %w", err)
		}
		return pixels, nil
	}

	if app.book != nil {
		if _, err := frame(); err != nil {
			return err
		}
		if err := awaitHeadlessGridLoad(ctx, app); err != nil {
			return fmt.Errorf("load first worksheet viewport: %w", err)
		}
		pixels, err := frame()
		if err != nil {
			return err
		}
		return emit(HeadlessTarget{Name: "worksheet-1", Image: pixels})
	}
	if app.word != nil {
		pixels, err := frame()
		if err != nil {
			return err
		}
		return emit(HeadlessTarget{Name: "viewport-1", Image: pixels})
	}
	if app.pptxDoc != nil {
		view := app.ensurePresentationView()
		slideCount := app.pptxDoc.Snapshot().SlideCount
		if slideCount == 0 {
			return errors.New("presentation has no renderable slides")
		}
		if slideCount > maxHeadlessSlides || int64(width)*int64(height)*int64(slideCount) > maxHeadlessPixels*4 {
			return fmt.Errorf("presentation exceeds headless render limit (%d slides, %d pixels per slide)", slideCount, width*height)
		}
		for index := 0; index < slideCount; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			view.activeSlide = index
			pixels, err := frame()
			if err != nil {
				return fmt.Errorf("render slide %d: %w", index+1, err)
			}
			if err := emit(HeadlessTarget{Name: fmt.Sprintf("slide-%d", index+1), Image: pixels}); err != nil {
				return fmt.Errorf("deliver slide %d: %w", index+1, err)
			}
		}
		return nil
	}
	return errors.New("document has no Office view to render")
}

func headlessPixelSize(opts HeadlessRenderOptions) (width, height int, scale float32, err error) {
	if opts.Width <= 0 || opts.Height <= 0 || opts.Width > maxHeadlessDimension || opts.Height > maxHeadlessDimension {
		return 0, 0, 0, fmt.Errorf("headless viewport must be between 1 and %d logical pixels", maxHeadlessDimension)
	}
	if math.IsNaN(float64(opts.Scale)) || math.IsInf(float64(opts.Scale), 0) || opts.Scale < 0.25 || opts.Scale > 4 {
		return 0, 0, 0, errors.New("headless scale must be between 0.25 and 4")
	}
	width = int(math.Ceil(float64(opts.Width) * float64(opts.Scale)))
	height = int(math.Ceil(float64(opts.Height) * float64(opts.Scale)))
	if width <= 0 || height <= 0 || width > maxHeadlessDimension || height > maxHeadlessDimension || int64(width)*int64(height) > maxHeadlessPixels {
		return 0, 0, 0, errors.New("scaled headless viewport exceeds the image dimension or pixel limit")
	}
	return width, height, opts.Scale, nil
}

func awaitHeadlessWordLayout(ctx context.Context, app *App) error {
	state := app.documentSessionState
	if state == nil {
		return errors.New("Word session is unavailable")
	}
	for {
		if state.wordLayoutResult != nil {
			return nil
		}
		select {
		case result := <-app.wordLayoutResults:
			app.applyWordLayoutResult(result)
			if result.err != nil {
				return result.err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-app.shutdown:
			return errors.New("headless document closed during Word layout")
		}
	}
}

func awaitHeadlessGridLoad(ctx context.Context, app *App) error {
	v := app.virtualGrid
	if v == nil {
		return errors.New("spreadsheet viewport is unavailable")
	}
	for {
		if v.LoadedSheet == app.sheet && !v.LoadedWindows.empty() {
			return nil
		}
		if v.WindowError != nil {
			return v.WindowError
		}
		select {
		case result := <-app.gridLoadResults:
			if result.viewport == nil || result.viewport.RequestedID != result.requestID || result.viewport.RequestedSheet != result.sheet || result.viewport.RequestedWindows != result.windows {
				continue
			}
			result.viewport.Loading = false
			if result.viewport != v || result.generation != app.documentGeneration || result.sheet != app.sheet {
				continue
			}
			if result.err != nil {
				v.WindowError = result.err
				return result.err
			}
			if err := v.applyLoadResult(result); err != nil {
				return err
			}
			app.applyValueFilterWindow(app.documentSessionState, v)
		case <-ctx.Done():
			return ctx.Err()
		case <-app.shutdown:
			return errors.New("headless document closed during worksheet loading")
		}
	}
}

func (d *HeadlessDocument) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	if d.app == nil {
		return nil
	}
	app := d.app
	d.app = nil
	app.shutdownOnce.Do(func() { close(app.shutdown) })
	app.closeAgentSubscription()
	app.stopAndWaitManagedBackground()
	if app.documentSessionState != nil && app.documentSessionState.documentID != "" && app.desktopRuntime != nil {
		app.desktopRuntime.removeSession(app.documentSessionState.documentID)
	} else {
		_ = app.closeDocument()
	}
	return nil
}

func (d *HeadlessDocument) Diagnostics() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.app == nil || d.app.documentSessionState == nil || d.app.documentSessionState.wordLayoutResult == nil {
		return nil
	}
	result := d.app.documentSessionState.wordLayoutResult
	diagnostics := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		code := strings.TrimSpace(diagnostic.Code)
		if code == "" {
			code = "unspecified"
		}
		diagnostics = append(diagnostics, string(diagnostic.Severity)+":"+code)
	}
	return diagnostics
}
