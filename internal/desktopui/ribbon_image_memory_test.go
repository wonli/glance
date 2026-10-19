package desktopui

import (
	"image"
	"runtime"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestRibbonIconFramesAvoidRasterCopies(t *testing.T) {
	icon, ok := wordToolbarActionIcon(wordToolbarAction{id: "paste", icon: "clipboard"}, true)
	if !ok {
		t.Fatal("missing ribbon fixture")
	}
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts())
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(80, 68))}
	frame := func() { ops.Reset(); layoutOfficeRibbonCommandContents(gtx, theme, icon, "") }
	for i := 0; i < 3; i++ {
		frame()
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	const frames = 20
	for i := 0; i < frames; i++ {
		frame()
	}
	runtime.ReadMemStats(&after)
	perFrame := (after.TotalAlloc - before.TotalAlloc) / frames
	pixelBytes := uint64(icon.Bounds().Dx() * icon.Bounds().Dy() * 4)
	if perFrame >= pixelBytes/4 {
		t.Fatalf("warm ribbon frame allocated %d B for %d B immutable icon", perFrame, pixelBytes)
	}
	runtime.KeepAlive(icon)
}

func BenchmarkRibbonIconFrame(b *testing.B) {
	icon, ok := wordToolbarActionIcon(wordToolbarAction{id: "paste", icon: "clipboard"}, true)
	if !ok {
		b.Fatal("missing ribbon fixture")
	}
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts())
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(80, 68))}
	layoutOfficeRibbonCommandContents(gtx, theme, icon, "")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ops.Reset()
		layoutOfficeRibbonCommandContents(gtx, theme, icon, "")
	}
}

func TestUnregisteredImagesKeepFreshUploadIdentity(t *testing.T) {
	for _, img := range []image.Image{image.NewRGBA(image.Rect(0, 0, 4, 4)), image.NewNRGBA(image.Rect(0, 0, 4, 4))} {
		if first, second := iconImageOp(img), iconImageOp(img); first == second {
			t.Fatal("an unregistered, potentially mutable image reused a GPU upload identity")
		}
	}
}
