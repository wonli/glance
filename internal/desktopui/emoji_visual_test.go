package desktopui

import (
	"fmt"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/document"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMarkdownEmojiVisual(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS Apple Color Emoji")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := markdownTestApp(t)
	for _, scale := range []float32{1, 2} {
		for _, kind := range []string{"normal", "heading", "code"} {
			for _, emoji := range "🚀📑🎨📊🧮📈🎮🔖✨✅" {
				t.Run(fmt.Sprintf("%g/%s/%U", scale, kind, emoji), func(t *testing.T) {
					var ops op.Ops
					size := image.Pt(int(80*scale), int(100*scale))
					gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
					paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
					a.layoutMarkdownSpans(gtx, []document.MarkdownSpan{{Text: string(emoji), Kind: kind}}, 32, color.NRGBA{A: 255}, kind == "heading", "emoji")
					w, err := headless.NewWindow(size.X, size.Y)
					if err != nil {
						t.Skipf("GPU unavailable: %v", err)
					}
					err = w.Frame(&ops)
					if err != nil {
						w.Release()
						t.Fatal(err)
					}
					shot := image.NewRGBA(image.Rectangle{Max: size})
					err = w.Screenshot(shot)
					w.Release()
					if err != nil {
						t.Fatal(err)
					}
					if p := os.Getenv("GLANCE_EMOJI_CAPTURE"); p != "" {
						f, e := os.Create(filepath.Join(p, fmt.Sprintf("emoji-%g-%s-%x.png", scale, kind, emoji)))
						if e != nil {
							t.Fatal(e)
						}
						if err := png.Encode(f, shot); err != nil {
							f.Close()
							t.Fatal(err)
						}
						if err := f.Close(); err != nil {
							t.Fatal(err)
						}
					}
					colored := 0
					for y := 0; y < size.Y; y++ {
						for x := 0; x < size.X; x++ {
							c := shot.RGBAAt(x, y)
							if max(c.R, c.G, c.B)-min(c.R, c.G, c.B) > 50 {
								colored++
							}
						}
					}
					// The page emoji is mostly gray; count only a small colored
					// accent, while still rejecting the monochrome missing glyph.
					if colored < int(8*scale*scale) {
						t.Errorf("scale %g: only %d colored pixels; emoji not rendered", scale, colored)
					}
				})
			}
		}
	}
}
