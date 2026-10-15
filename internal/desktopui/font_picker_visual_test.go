package desktopui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/fontcatalog"
	"glance/internal/fontdownload"
	"glance/internal/format/docx"
)

func TestFontPickerVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []int{1, 2} {
		t.Run(string(rune('0'+scale)), func(t *testing.T) {
			a := New(nil, "")
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer word.Close()
			state := newDocumentSessionState("fonts.docx")
			state.word = word
			state.wordBlocks = word.WordBlocks()
			p := state.wordBlocks[0].Paragraph
			state.selectedWordRunID = p.Inlines[0].Text.ID
			a.documentSessionState = state
			a.initFontPicker()
			a.fontPicker.open = true
			a.fontPicker.target = fontPickerTarget{state: state, word: true, runID: state.selectedWordRunID, current: "Go", revision: state.wordRevisionAtomic.Load()}
			a.fontPicker.snapshot = fontcatalog.NewSnapshot(1, []fontcatalog.Face{{ID: "1", Family: "Go", Source: fontcatalog.System}, {ID: "2", Family: "Noto Serif SC", Source: fontcatalog.Private}})
			a.fontPicker.documentNames = []string{"Go", "Missing"}
			a.fontPicker.entries = []fontdownload.Entry{{ID: "online", Family: "Example Online"}}
			var ops op.Ops
			var router input.Router
			paint.Fill(&ops, color.NRGBA{R: 225, G: 225, B: 225, A: 255})
			size := image.Pt(700*scale, 650*scale)
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}, Constraints: layout.Constraints{Max: size}}
			a.layoutFontPicker(gtx)
			router.Frame(&ops)
			bounds := a.fontPicker.bounds
			if bounds.Dx() != 388*scale || !bounds.In(image.Rectangle{Max: size}) {
				t.Fatalf("invalid panel %v", bounds)
			}
			img := renderPresentationTestOps(t, &ops, size)
			footer := image.Rect(bounds.Max.X-100*scale, bounds.Max.Y-32*scale, bounds.Max.X-8*scale, bounds.Max.Y)
			if _, _, ok := darkInkVerticalBounds(img, footer); !ok {
				t.Fatal("refresh button has no visible ink")
			}
			if dir := os.Getenv("GLANCE_FONT_SCREENSHOT"); dir != "" {
				if e := os.MkdirAll(dir, 0700); e != nil {
					t.Fatal(e)
				}
				path := filepath.Join(dir, "picker-"+string(rune('0'+scale))+"x.png")
				f, e := os.Create(path)
				if e != nil {
					t.Fatal(e)
				}
				if e := png.Encode(f, img); e != nil {
					t.Fatal(e)
				}
				f.Close()
			}
		})
	}
}
