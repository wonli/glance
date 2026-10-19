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
		size := image.Pt(int(1440*density), int(28*density))
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
		check(1400, 16, wordRulerMargin)
		check(400, 16, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		check(405, 23, wordRulerMajor)
		check(382.5, 23, wordRulerMinor)
		for _, rect := range []image.Rectangle{image.Rect(355, 0, 366, 8), image.Rect(355, 18, 366, 25), image.Rect(355, 25, 366, 28), image.Rect(1075, 19, 1086, 27)} {
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

func TestWordRulerSurfaceFillsWorkspaceWithoutStretchingTicks(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, density := range []float32{1, 2} {
		for _, sidebar := range []bool{false, true} {
			state, closeDoc := newLongWordFlowState(t, 1)
			defer closeDoc()
			state.documentID = "full-width-ruler"
			state.wordLayoutZoom = 1
			state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 60, WidthPT: 480}}}}
			app := New(nil, "")
			app.documentSessionState = state
			app.agentUI.sidebarVisible = sidebar
			app.agentUI.conversationDocumentID = state.documentID
			var ops op.Ops
			var router input.Router
			size := image.Pt(int(1400*density), int(100*density))
			var rulerWidth int
			var geometry wordRulerGeometry
			app.layoutAgentSidebarWorkspace(layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(size)}, func(gtx layout.Context) layout.Dimensions {
				rulerWidth = gtx.Constraints.Max.X
				geometry = wordRulerGeometryForState(gtx, state)
				return app.layoutWordHorizontalRuler(gtx, state)
			})
			img := renderPresentationTestOps(t, &ops, size)
			border := color.NRGBA{R: 204, G: 210, B: 219, A: 255}
			origin := size.X - rulerWidth
			for _, x := range []int{int(12 * density), rulerWidth - int(12*density)} {
				if got := color.NRGBAModel.Convert(img.At(origin+x, int(12*density))).(color.NRGBA); got != wordRulerMargin {
					t.Fatalf("density=%v sidebar=%v non-text region does not fill workspace at %d: %v", density, sidebar, x, got)
				}
				if got := color.NRGBAModel.Convert(img.At(origin+x, int(27*density))).(color.NRGBA); got != border {
					t.Fatalf("border does not fill workspace at %d: %v", x, got)
				}
			}
			// The latest Pen has a white text span and separate page-edge lines
			// inside the full-width gray non-text area.
			for _, sample := range []struct {
				x    int
				want color.NRGBA
			}{
				{origin + (geometry.textLeft+geometry.textRight)/2, color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
				{origin + geometry.pageLeft, color.NRGBA{R: 184, G: 193, B: 206, A: 255}},
				{origin + geometry.pageRight, color.NRGBA{R: 184, G: 193, B: 206, A: 255}},
				{origin, color.NRGBA{R: 224, G: 228, B: 234, A: 255}},
			} {
				if got := color.NRGBAModel.Convert(img.At(sample.x, int(12*density))).(color.NRGBA); got != sample.want {
					t.Fatalf("Pen region at x=%d got %v want %v", sample.x, got, sample.want)
				}
			}
			// A major tick at 1 cm must still be measured from the page text origin.
			tickX := origin + geometry.textLeft + geometry.pixels(wordEMUPerCM)
			got := color.NRGBAModel.Convert(img.At(tickX, int(23*density))).(color.NRGBA)
			if got != wordRulerMajor {
				t.Fatalf("tick lost page alignment: x=%d color=%v", tickX, got)
			}
			if sidebar && rulerWidth >= size.X {
				t.Fatal("ruler extended through sidebar")
			}
		}
	}
}
