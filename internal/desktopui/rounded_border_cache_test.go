package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func TestCachedRoundedBorderMatchesDirectRendering(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []int{1, 2} {
		size := image.Pt(180*scale, 90*scale)
		shape := clip.UniformRRect(image.Rect(5*scale, 5*scale, 170*scale, 80*scale), 14*scale)
		for _, border := range []color.NRGBA{{R: 225, G: 229, B: 234, A: 255}, {R: 20, G: 80, B: 180, A: 120}} {
			var direct, cached op.Ops
			for _, ops := range []*op.Ops{&direct, &cached} {
				paint.Fill(ops, color.NRGBA{R: 245, G: 245, B: 245, A: 255})
			}
			paint.FillShape(&direct, border, clip.Stroke{Path: shape.Path(&direct), Width: float32(scale)}.Op())
			paintRoundedBorder(&cached, shape, float32(scale), border)
			expected := renderPresentationTestOps(t, &direct, size)
			got := renderPresentationTestOps(t, &cached, size)
			if !bytes.Equal(expected.Pix, got.Pix) {
				t.Fatalf("cached border changes pixels at %dx, color %v", scale, border)
			}
		}
	}
}

func BenchmarkRoundedBorderFrame(b *testing.B) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, cached := range []bool{false, true} {
		name := "direct"
		if cached {
			name = "cached"
		}
		b.Run(name, func(b *testing.B) {
			window, err := headless.NewWindow(360, 180)
			if err != nil {
				b.Skipf("GPU unavailable: %v", err)
			}
			defer window.Release()
			var ops op.Ops
			draw := func() {
				ops.Reset()
				shape := clip.UniformRRect(image.Rect(5, 5, 350, 170), 28)
				if cached {
					paintRoundedBorder(&ops, shape, 2, homeBorder)
				} else {
					paint.FillShape(&ops, homeBorder, clip.Stroke{Path: shape.Path(&ops), Width: 2}.Op())
				}
				if err := window.Frame(&ops); err != nil {
					b.Fatal(err)
				}
			}
			draw()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				draw()
			}
		})
	}
}

func TestRoundedBorderCacheBoundsResizeHistory(t *testing.T) {
	var cache roundedBorderCache
	key := roundedBorderKey{shape: clip.UniformRRect(image.Rect(0, 0, 100, 60), 12), width: 1, color: homeBorder}
	first := cache.border(key)
	if cache.border(key) != first {
		t.Fatal("unchanged geometry rebuilt its recorded path")
	}
	for width := 101; width < 101+roundedBorderCacheLimit*2; width++ {
		next := key
		next.shape.Rect.Max.X = width
		cache.border(next)
	}
	if len(cache.items) > roundedBorderCacheLimit {
		t.Fatalf("resize history retains %d geometries", len(cache.items))
	}
	if _, exists := cache.items[key]; exists {
		t.Fatal("old resize geometry was not evicted")
	}
	// An evicted recording can still be referenced by an in-flight frame.
	var frame op.Ops
	first.Add(&frame)
	if cache.border(key) == first {
		t.Fatal("evicted geometry unexpectedly reused an expired cache entry")
	}
}

func TestRoundedBorderConcurrentReuse(t *testing.T) {
	var cache roundedBorderCache
	key := roundedBorderKey{shape: clip.UniformRRect(image.Rect(0, 0, 100, 60), 12), width: 1, color: homeBorder}
	results := make(chan op.CallOp, 16)
	for i := 0; i < cap(results); i++ {
		go func() { results <- cache.border(key) }()
	}
	first := <-results
	for i := 1; i < cap(results); i++ {
		if got := <-results; got != first {
			t.Fatal("concurrent border draws rebuilt the same geometry")
		}
	}
}
