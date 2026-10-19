package desktopui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestHomeIconWarmFramesDoNotAllocateImages(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Exact(image.Pt(40, 40))}
	draw := func() { ops.Reset(); homeIcon(gtx, "search", 20, color.NRGBA{A: 255}) }
	draw()
	if allocations := testing.AllocsPerRun(100, draw); allocations > 0 {
		t.Fatalf("warm home icon allocates %.1f objects per frame", allocations)
	}
}

func BenchmarkHomeIconWarmFrame(b *testing.B) {
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Exact(image.Pt(40, 40))}
	homeIcon(gtx, "search", 20, color.NRGBA{A: 255})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ops.Reset()
		homeIcon(gtx, "search", 20, color.NRGBA{A: 255})
	}
}

var benchmarkIconOp paint.ImageOp

func BenchmarkHomeIconImageOp(b *testing.B) {
	icon, err := renderSVGIcon("home", "search", 40, &homeText)
	if err != nil {
		b.Fatal(err)
	}
	for _, cached := range []bool{false, true} {
		name := "direct"
		if cached {
			name = "cached"
		}
		b.Run(name, func(b *testing.B) {
			benchmarkIconOp = iconImageOp(icon)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if cached {
					benchmarkIconOp = iconImageOp(icon)
				} else {
					benchmarkIconOp = paint.NewImageOp(icon)
				}
			}
		})
	}
}

func TestHomeIconConcurrentUploadIdentity(t *testing.T) {
	icon, err := renderSVGIcon("home", "search", 43, &color.NRGBA{R: 31, G: 72, B: 113, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan paint.ImageOp, 16)
	for i := 0; i < cap(results); i++ {
		go func() { results <- iconImageOp(icon) }()
	}
	first := <-results
	for i := 1; i < cap(results); i++ {
		if got := <-results; got != first {
			t.Fatal("concurrent icon draws created different upload identities")
		}
	}
}
