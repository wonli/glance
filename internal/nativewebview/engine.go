package nativewebview

import (
	"context"
	"errors"
	"image"
	"math"
	"runtime"
	"sync"
)

// RenderRequest specifies the web evaluation and snapshot parameters.
type RenderRequest struct {
	// HTML is the initial document markup. If empty, a minimal container is provided.
	HTML string
	// Script is an async JavaScript snippet executed in the page context.
	// It should return an object with width and height properties (and optional data).
	Script string
	// Args is a key-value mapping passed to the async script execution.
	Args map[string]any
	// Density is the device pixel ratio (e.g. 2 for 2x Retina). Defaults to 2.
	Density int
}

// RenderResult contains the rasterized snapshot and measured logical dimensions.
type RenderResult struct {
	// Image is the captured bitmap with transparency preserved.
	Image image.Image
	// LogicalSize represents the CSS layout dimensions in points/pixels.
	LogicalSize image.Point
	// Data holds any structured object returned by the script.
	Data any
}

// Engine defines the cross-platform contract for offscreen web evaluation and capture.
type Engine interface {
	// EvaluateAndSnapshot runs the requested markup and script, then captures a high-resolution snapshot.
	EvaluateAndSnapshot(ctx context.Context, req RenderRequest) (RenderResult, error)
	// Close releases any platform webview or browser subprocess resources.
	Close() error
}

// PointerEventKind identifies a CSS-pixel pointer or wheel event delivered to
// a persistent chart page.
type PointerEventKind string

const (
	PointerMove  PointerEventKind = "move"
	PointerDown  PointerEventKind = "down"
	PointerUp    PointerEventKind = "up"
	PointerLeave PointerEventKind = "leave"
	PointerWheel PointerEventKind = "wheel"
)

// PointerEvent describes one Gio input event in chart-local CSS pixels.
type PointerEvent struct {
	Kind    PointerEventKind `json:"kind"`
	X       float64          `json:"x"`
	Y       float64          `json:"y"`
	Button  int              `json:"button"`
	Buttons uint32           `json:"buttons"`
	DeltaX  float64          `json:"deltaX"`
	DeltaY  float64          `json:"deltaY"`
	Shift   bool             `json:"shift"`
	Control bool             `json:"control"`
}

func validatePointerEvent(event PointerEvent) error {
	switch event.Kind {
	case PointerMove, PointerDown, PointerUp, PointerLeave, PointerWheel:
	default:
		return errors.New("unknown pointer event kind")
	}
	for _, value := range []float64{event.X, event.Y, event.DeltaX, event.DeltaY} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("pointer event contains a non-finite coordinate")
		}
	}
	if event.Button < 0 || event.Button > 4 {
		return errors.New("pointer event button is out of range")
	}
	return nil
}

// SessionRequest loads a persistent page and runs Script once to initialize
// its content before the first snapshot.
type SessionRequest struct {
	HTML, Script string
	Density      int
}

// Session is a persistent WebView target whose state survives event dispatches.
type Session interface {
	Dispatch(context.Context, PointerEvent) (RenderResult, error)
	Snapshot(context.Context) (RenderResult, error)
	Close() error
}

// SessionEngine is an optional extension for clients that need live input.
// One-shot render clients can continue to depend on Engine alone.
type SessionEngine interface {
	Engine
	OpenSession(context.Context, SessionRequest) (Session, error)
}

var (
	defaultMu     sync.Mutex
	defaultEngine Engine
)

// Default returns a shared platform engine instance (macOS WebKit or Windows Edge/Chrome via CDP).
func Default() (Engine, error) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultEngine != nil {
		return defaultEngine, nil
	}

	eng, err := newPlatformEngine()
	if err != nil {
		return nil, err
	}
	defaultEngine = eng
	return defaultEngine, nil
}

// CloseDefault closes and clears the shared platform engine if initialized.
func CloseDefault() error {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultEngine != nil {
		err := defaultEngine.Close()
		defaultEngine = nil
		return err
	}
	return nil
}

// PlatformSupported reports whether an offscreen native web engine is available on this OS.
func PlatformSupported() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

var (
	ErrEngineClosed    = errors.New("native webview engine closed")
	ErrUnsupportedKind = errors.New("unsupported platform for native webview")
	ErrSessionClosed   = errors.New("native webview session closed")
)

func blockedExternalRequestPatterns() []string {
	return []string{"http://*/*", "https://*/*", "file://*/*", "ftp://*/*", "ws://*/*", "wss://*/*"}
}
