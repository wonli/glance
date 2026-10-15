package desktopui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestPenRibbonEveryReferenceGlyphHasVisiblePixels(t *testing.T) {
	data, err := os.ReadFile("../../docs/ribbon-pen-icons.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Nodes []struct {
			Icons []struct {
				Icon  string
				Color string
			}
		}
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	for _, node := range reference.Nodes {
		for _, glyph := range node.Icons {
			for _, size := range []int{20, 40} {
				want := color.NRGBA{R: 33, G: 99, B: 65, A: 255}
				icon, err := renderSVGIcon("ribbon", glyph.Icon, size, &want)
				if err != nil {
					t.Errorf("%s: %v", glyph.Icon, err)
					continue
				}
				visible, opaque := 0, 0
				for y := 0; y < size; y++ {
					for x := 0; x < size; x++ {
						pixel := color.NRGBAModel.Convert(icon.At(x, y)).(color.NRGBA)
						if pixel.A == 0 {
							continue
						}
						visible++
						if pixel.A > 200 {
							opaque++
						}
						if !nearByte(pixel.R, want.R) || !nearByte(pixel.G, want.G) || !nearByte(pixel.B, want.B) {
							t.Fatalf("%s tint = %v", glyph.Icon, pixel)
						}
					}
				}
				if visible < size || opaque == 0 {
					t.Errorf("%s at %dpx has insufficient ink: visible=%d opaque=%d", glyph.Icon, size, visible, opaque)
				}
			}
		}
	}
}

func TestPenRibbonOverviewVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	wordState, closeDocument := newLongWordFlowState(t, 2)
	defer closeDocument()
	for _, scale := range []float32{1, 2} {
		for _, format := range []string{"word", "excel"} {
			t.Run(fmt.Sprintf("%s-%gx", format, scale), func(t *testing.T) {
				app := New(nil, "")
				state := wordState
				if format == "excel" {
					state = newDocumentSessionState("")
					state.book = &toolbarTestBook{editable: true}
					app.book = state.book
				}
				app.documentSessionState = state
				metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
				size := image.Pt(metric.Dp(1440), metric.Dp(8*134))
				var ops op.Ops
				paint.FillShape(&ops, contentSurface, clip.Rect{Max: size}.Op())
				for i := 0; i < 8; i++ {
					shift := op.Offset(image.Pt(0, metric.Dp(unit.Dp(i*134)))).Push(&ops)
					gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Constraints{Max: image.Pt(size.X, metric.Dp(122))}}
					if format == "word" {
						state.wordToolbarTab = wordToolbarTab(i)
						app.layoutWordRibbon(gtx)
					} else {
						state.toolbarTab = spreadsheetToolbarTab(i)
						app.layoutSpreadsheetToolbar(gtx)
					}
					shift.Pop()
				}
				window, err := headless.NewWindow(size.X, size.Y)
				if err != nil {
					t.Skipf("headless GPU unavailable: %v", err)
				}
				defer window.Release()
				if err := window.Frame(&ops); err != nil {
					t.Fatal(err)
				}
				shot := image.NewRGBA(image.Rectangle{Max: size})
				if err := window.Screenshot(shot); err != nil {
					t.Fatal(err)
				}
				if dir := os.Getenv("GLANCE_RIBBON_SCREENSHOTS"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%gx.png", format, scale)))
					if err != nil {
						t.Fatal(err)
					}
					defer file.Close()
					if err := png.Encode(file, shot); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}
