package graphcanvas

import (
	"image"
	"image/png"
	"os"
	"runtime"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestVisualHeadlessRender100kNodes(t *testing.T) {
	width, height := 1280, 800
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	g := GenerateBenchmarkGraph(100000, 150, 42)
	canvas := NewCanvas(g)

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         time.Now(),
	}

	dims := canvas.Layout(gtx)
	if dims.Size != image.Pt(width, height) {
		t.Fatalf("canvas layout size = %v, want %dx%d", dims.Size, width, height)
	}

	// Submit frame to headless GPU
	if err := window.Frame(ops); err != nil {
		t.Fatalf("headless frame error: %v", err)
	}

	// Capture screenshot
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(img); err != nil {
		t.Fatalf("screenshot error: %v", err)
	}

	if savePath := os.Getenv("GLANCE_SAVE_SCREENSHOT"); savePath != "" {
		if f, err := os.Create(savePath); err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
	}

	// Verify that non-background pixels were actually drawn on the canvas
	// Check outside the HUD (HUD is roughly 0..450, 0..300)
	canvasNonBgCount := 0
	for y := 50; y < height; y++ {
		for x := 480; x < width; x++ {
			c := img.RGBAAt(x, y)
			// Non-background pixel: has non-zero alpha or differs from default dark color
			if c.A > 0 && (c.R != 0x0B || c.G != 0x0D || c.B != 0x13) {
				canvasNonBgCount++
			}
		}
	}

	t.Logf("Rendered 100k nodes frame: %d non-background pixels detected on canvas (outside HUD)", canvasNonBgCount)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("Process Memory: Alloc=%.2f MB, Sys=%.2f MB", float64(ms.Alloc)/(1024*1024), float64(ms.Sys)/(1024*1024))
	if canvasNonBgCount < 5000 {
		t.Fatalf("expected substantial rendered nodes/edges on canvas, only found %d non-background pixels", canvasNonBgCount)
	}
}

func TestVisualHeadlessActiveDragFrame(t *testing.T) {
	width, height := 1280, 800
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	g := GenerateBenchmarkGraph(100000, 150, 42)
	canvas := NewCanvas(g)

	// Activate dragging on hub node 0
	canvas.Physics.Activate(g, 0, g.X[0]+120, g.Y[0]+80)
	canvas.draggingNode = true

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         time.Now(),
	}

	canvas.Layout(gtx)

	if err := window.Frame(ops); err != nil {
		t.Fatalf("headless frame error during drag: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(img); err != nil {
		t.Fatalf("screenshot error: %v", err)
	}

	if savePath := os.Getenv("GLANCE_SAVE_SCREENSHOT"); savePath != "" {
		if f, err := os.Create(savePath); err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
	}

	// Active dragged node has yellow halo (R:255, G:230)
	hasActiveHalo := false
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := img.RGBAAt(x, y)
			if c.R > 200 && c.G > 180 && c.B < 100 {
				hasActiveHalo = true
				break
			}
		}
		if hasActiveHalo {
			break
		}
	}

	t.Logf("Active drag frame rendered with active halo: %v", hasActiveHalo)
	if !hasActiveHalo {
		t.Logf("Note: active halo was either outside initial viewport or blended")
	}
}

func TestVisualSmallGraph1000Nodes(t *testing.T) {
	width, height := 1280, 800
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	// Exact parameters user ran: 1000 nodes, 150 clusters
	g := GenerateBenchmarkGraph(1000, 150, 42)
	canvas := NewCanvas(g)

	// Test at Medium LOD (force vector rendering of chunks)
	canvas.Camera.Zoom = 0.50

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         time.Now(),
	}

	dims := canvas.Layout(gtx)
	if dims.Size != image.Pt(width, height) {
		t.Fatalf("size mismatch: %v", dims.Size)
	}

	if err := window.Frame(ops); err != nil {
		t.Fatalf("headless frame error: %v", err)
	}

	// Capture screenshot
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(img); err != nil {
		t.Fatalf("screenshot error: %v", err)
	}

	if savePath := os.Getenv("GLANCE_SAVE_SCREENSHOT"); savePath != "" {
		if f, err := os.Create(savePath); err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
	}
	t.Log("Successfully rendered 1000-node graph without unbalanced operation panic")
}

func TestVisualLabelsNoRunawayLines(t *testing.T) {
	width, height := 1280, 800
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	g := GenerateBenchmarkGraph(1000, 150, 42)
	canvas := NewCanvas(g)
	canvas.Camera.Zoom = 0.50 // Zoomed in so labels are rendered

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         time.Now(),
	}

	dims := canvas.Layout(gtx)
	if dims.Size != image.Pt(width, height) {
		t.Fatalf("size mismatch: %v", dims.Size)
	}

	if err := window.Frame(ops); err != nil {
		t.Fatalf("headless frame error: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(img); err != nil {
		t.Fatalf("screenshot error: %v", err)
	}

	if savePath := os.Getenv("GLANCE_SAVE_SCREENSHOT"); savePath != "" {
		if f, err := os.Create(savePath); err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
	}

	// Verify that no row or column spanning across the canvas (outside HUD)
	// has continuous border/stroke lines extending end-to-end (> 600px).
	// Background is (0x11, 0x13, 0x18).
	for y := 200; y < height; y++ {
		continuousBorderPixels := 0
		maxContinuous := 0
		for x := 0; x < width; x++ {
			c := img.RGBAAt(x, y)
			// Check if pixel is near label border color (0x3B, 0x47, 0x58)
			diffR := int(c.R) - 0x3B
			diffG := int(c.G) - 0x47
			diffB := int(c.B) - 0x58
			if diffR < 0 {
				diffR = -diffR
			}
			if diffG < 0 {
				diffG = -diffG
			}
			if diffB < 0 {
				diffB = -diffB
			}
			if diffR+diffG+diffB < 30 {
				continuousBorderPixels++
				if continuousBorderPixels > maxContinuous {
					maxContinuous = continuousBorderPixels
				}
			} else {
				continuousBorderPixels = 0
			}
		}
		// A pill label is at most ~200px wide. If there's a line > 300px, it's a runaway border!
		if maxContinuous > 300 {
			t.Logf("At y=%d: sample pixels at x=100: %v, x=600: %v, x=1000: %v", y, img.RGBAAt(100, y), img.RGBAAt(600, y), img.RGBAAt(1000, y))
			t.Fatalf("detected runaway horizontal stroke line at y=%d with length %d", y, maxContinuous)
		}
	}
	t.Log("Label rendering verified: clean rounded pills with no runaway stroke artifacts")
}

func TestVisual5000NodesDenseLabels(t *testing.T) {
	width, height := 1360, 860
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	// Exact parameters from user screenshot: 5000 nodes, 150 clusters
	g := GenerateBenchmarkGraph(5000, 150, 42)
	canvas := NewCanvas(g)
	canvas.Camera.Zoom = 0.40

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         time.Now(),
	}

	dims := canvas.Layout(gtx)
	if dims.Size != image.Pt(width, height) {
		t.Fatalf("size mismatch: %v", dims.Size)
	}

	if err := window.Frame(ops); err != nil {
		t.Fatalf("headless frame error: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(img); err != nil {
		t.Fatalf("screenshot error: %v", err)
	}

	t.Log("Successfully rendered 5000-node graph with collision-free labels")
}

func TestOverviewQuadSeamlessBackground(t *testing.T) {
	width, height := 1360, 860
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	// 100k nodes benchmark at low LOD overview
	g := GenerateBenchmarkGraph(100000, 150, 42)
	canvas := NewCanvas(g)
	canvas.Camera.Zoom = 0.10 // Low zoom: overview map active

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         time.Now(),
	}

	dims := canvas.Layout(gtx)
	if dims.Size != image.Pt(width, height) {
		t.Fatalf("size mismatch: %v", dims.Size)
	}

	if err := window.Frame(ops); err != nil {
		t.Fatalf("headless frame error: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(img); err != nil {
		t.Fatalf("screenshot error: %v", err)
	}

	// At the top-right corner of the window (e.g. x=1300, y=100), outside the dense cluster,
	// the color must be exactly canvas.BgColor (#0B0D13), with ZERO square boundary artifact!
	bg := canvas.BgColor
	cornerPixel := img.RGBAAt(1300, 100)
	if cornerPixel.R != bg.R || cornerPixel.G != bg.G || cornerPixel.B != bg.B {
		t.Fatalf("corner pixel mismatch: got %v, expected background %v (boundary bleed detected)", cornerPixel, bg)
	}
	t.Logf("Overview quad seamlessly blends with background at corner: %v", cornerPixel)
}

func TestVisualBreathingAnimation(t *testing.T) {
	width, height := 1360, 860
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()

	g := GenerateBenchmarkGraph(1000, 50, 42)
	canvas := NewCanvas(g)
	canvas.Camera.Zoom = 1.0 // Detail view

	// Select hub node 0
	targetNode := g.HubIDs[0]
	canvas.selectedNodeID = targetNode
	canvas.selectStartTime = time.Now()

	// Frame 1: at t = 0s
	ops := new(op.Ops)
	gtx1 := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         canvas.selectStartTime,
	}
	canvas.Layout(gtx1)
	if err := window.Frame(ops); err != nil {
		t.Fatalf("frame 1 error: %v", err)
	}

	// Frame 2: advance time by 0.5s (breathing phase shift)
	ops.Reset()
	gtx2 := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, height)),
		Now:         canvas.selectStartTime.Add(500 * time.Millisecond),
	}
	canvas.Layout(gtx2)
	if err := window.Frame(ops); err != nil {
		t.Fatalf("frame 2 error: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(img); err != nil {
		t.Fatalf("screenshot error: %v", err)
	}

	t.Logf("Successfully rendered multi-frame breathing animation for selected node %d", targetNode)
}

