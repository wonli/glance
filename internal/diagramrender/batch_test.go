package diagramrender

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"

	"glance/internal/nativewebview"
)

type batchRecordedEngine struct {
	calls  int
	script string
}

func (e *batchRecordedEngine) Close() error { return nil }

func (e *batchRecordedEngine) EvaluateAndSnapshot(_ context.Context, req nativewebview.RenderRequest) (nativewebview.RenderResult, error) {
	e.calls++
	e.script = req.Script
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	img.Set(2, 22, color.NRGBA{R: 255, A: 255})
	return nativewebview.RenderResult{Image: img, LogicalSize: image.Pt(20, 20), Data: []batchBounds{
		{Width: 20, Height: 10},
		{Y: 10, Width: 20, Height: 10},
		{Error: "invalid chart"},
	}}, nil
}

func TestRenderBatchOneCallAndIndependentResults(t *testing.T) {
	e := &batchRecordedEngine{}
	r := New(Bundle{})
	r.engine = e
	got := r.RenderBatch(context.Background(), []Request{
		{Kind: "math-inline", Source: []byte("E=mc^2"), Density: 2},
		{Kind: "mermaid", Source: []byte("graph TD; A-->B"), Density: 2},
		{Kind: "vega-lite", Source: []byte(testVegaLiteSpec), Density: 2},
	})
	if e.calls != 1 || len(got) != 3 {
		t.Fatalf("calls=%d results=%d", e.calls, len(got))
	}
	for i := range 2 {
		if got[i].Err != nil || got[i].Result.Image.Bounds().Size() != image.Pt(40, 20) || got[i].Result.LogicalSize != image.Pt(20, 10) {
			t.Fatalf("resource %d: %+v", i, got[i])
		}
	}
	red, _, _, alpha := got[1].Result.Image.At(2, 2).RGBA()
	if red != 65535 || alpha != 65535 || got[2].Err == nil {
		t.Fatal("resource bounds or independent error was lost")
	}
	for _, id := range []string{"glance-resource-0", "glance-resource-1", "glance-resource-2"} {
		if !strings.Contains(e.script, id) {
			t.Fatalf("missing resource ID %s", id)
		}
	}
}

func TestRenderBatchNativeMarkdownResources(t *testing.T) {
	r := testRenderer(t)
	requests := []Request{
		{Kind: "math-inline", Source: []byte("E=mc^2"), Density: 2},
		{Kind: "math", Source: []byte(`\begin{bmatrix}a&b\\c&d\end{bmatrix}^{-1}=\frac{1}{ad-bc}\begin{bmatrix}d&-b\\-c&a\end{bmatrix}`), Density: 2},
		{Kind: "math", Source: []byte(`\begin{align}\nabla\times\vec{B}-\frac{\partial\vec{E}}{\partial t}&=\mu_0\vec{j}\\\nabla\cdot\vec{E}&=\frac{\rho}{\epsilon_0}\end{align}`), Density: 2},
		{Kind: "mermaid", Source: []byte("graph TD\n A[开始] --> B{条件判断}\n B --> C[结束]"), Density: 2},
		{Kind: "vega-lite", Source: []byte(testVegaLiteSpec), Density: 2},
	}
	for i, result := range r.RenderBatch(context.Background(), requests) {
		if result.Err != nil {
			t.Fatalf("%s[%d]: %v", requests[i].Kind, i, result.Err)
		}
		img := result.Result.Image
		if img == nil || img.Bounds().Empty() {
			t.Fatalf("%s[%d]: empty image", requests[i].Kind, i)
		}
		wantPixels := result.Result.LogicalSize.Mul(2)
		if img.Bounds().Size() != wantPixels {
			t.Fatalf("%s[%d]: pixels=%v want 2x=%v", requests[i].Kind, i, img.Bounds().Size(), wantPixels)
		}
		ink := 0
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if a > 0 && (r < 60000 || g < 60000 || b < 60000) {
					ink++
				}
			}
		}
		if ink < 30 {
			t.Fatalf("%s[%d]: no visible content", requests[i].Kind, i)
		}
		t.Logf("%s[%d]: logical=%v pixels=%v visible=%d", requests[i].Kind, i, result.Result.LogicalSize, img.Bounds().Size(), ink)
	}
}
