package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/wordlayout"
)

func TestWordRulerPenVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, density := range []float32{1, 2} {
		state, closeDoc := newLongWordFlowState(t, 1)
		defer closeDoc()
		state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
		state.wordLayoutZoom = 45 * 2.54 / 96
		state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 20 * 72 / 2.54, HeightPT: 29.7 * 72 / 2.54}, Content: wordlayout.Rect{XPT: 2 * 72 / 2.54, WidthPT: 16 * 72 / 2.54}}}}
		app := &App{documentSessionState: state, theme: material.NewTheme()}
		var ops op.Ops
		var router input.Router
		size := image.Pt(int(900*density), int(28*density))
		app.layoutWordHorizontalRuler(layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(size)}, state)
		img := renderPresentationTestOps(t, &ops, size)
		if dir := os.Getenv("GLANCE_RULER_SCREENSHOT_DIR"); dir != "" {
			path := fmt.Sprintf("%s/word-ruler-%gx.png", dir, density)
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		check := func(x, y float64, want color.NRGBA) {
			t.Helper()
			got := color.NRGBAModel.Convert(img.At(int(math.Round(x*float64(density))), int(math.Round(y*float64(density))))).(color.NRGBA)
			if got != want {
				t.Fatalf("%vx pixel %g,%g = %v want %v", density, x, y, got, want)
			}
		}
		check(40, 16, wordRulerMargin)
		check(860, 16, wordRulerMargin)
		check(400, 16, wordRulerBackground)
		check(135, 23, wordRulerMajor)
		check(112.5, 23, wordRulerMinor)
		for _, rect := range []image.Rectangle{image.Rect(85, 0, 96, 8), image.Rect(85, 18, 96, 25), image.Rect(85, 25, 96, 28), image.Rect(805, 19, 816, 27)} {
			count := 0
			for y := int(float32(rect.Min.Y) * density); y < int(float32(rect.Max.Y)*density); y++ {
				for x := int(float32(rect.Min.X) * density); x < int(float32(rect.Max.X)*density); x++ {
					if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) == wordRulerGlyph {
						count++
					}
				}
			}
			if count < int(3*density*density) {
				t.Fatalf("missing marker at %v: %d pixels", rect, count)
			}
		}

	}
}
