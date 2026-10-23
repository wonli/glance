package desktopui

import (
	"gioui.org/font"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/fontmanager/fontfallback"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"testing"
	"time"
	"unicode/utf8"
)

func TestWindowsEmojiEditorKeepsIMEAndSelection(t *testing.T) {
	theme := material.NewTheme()
	theme.Shaper = newTextShaperForOS("windows", nil)
	theme.Face = font.Typeface(fontfallback.PreferredTypefaceForOS("windows"))
	var editor widget.Editor
	editor.SingleLine = true
	editor.SetText("A👩🏽‍💻B")
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(120, 44))}
		for {
			if _, ok := editor.Update(gtx); !ok {
				break
			}
		}
		material.Editor(theme, &editor, "").Layout(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Source().Execute(key.FocusCmd{Tag: &editor})
	frame()
	editor.SetCaret(editor.Len(), editor.Len())
	router.Queue(key.EditEvent{Range: key.Range{Start: editor.Len(), End: editor.Len()}, Text: "中文"})
	frame()
	if editor.Text() != "A👩🏽‍💻B中文" {
		t.Fatalf("emoji or Chinese input lost: %q", editor.Text())
	}
	editor.SetCaret(1, 5)
	if got := editor.SelectedText(); got != "👩🏽‍💻" {
		t.Fatalf("selection split cluster: %q", got)
	}
	editor.SetCaret(editor.Len(), editor.Len())
	frame()
	if editor.Text() != "A👩🏽‍💻B中文" {
		t.Fatal("navigation altered text")
	}
}
func TestWindowsEmojiSequencesVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	theme := material.NewTheme()
	theme.Shaper = newTextShaperForOS("windows", nil, text.NoSystemFonts())
	theme.Face = "Glance Color Emoji"
	for _, s := range []string{"👩🏽‍💻", "👨‍👩‍👧‍👦", "🇨🇳", "1️⃣", "⚠️"} {
		for _, scale := range []float32{1, 2} {
			var ops op.Ops
			size := image.Pt(int(100*scale), int(80*scale))
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			style := material.Body1(theme, s)
			style.TextSize = 32
			style.Layout(gtx)
			w, e := headless.NewWindow(size.X, size.Y)
			if e != nil {
				t.Skip(e)
			}
			if e = w.Frame(&ops); e != nil {
				w.Release()
				t.Fatal(e)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			e = w.Screenshot(shot)
			w.Release()
			if e != nil {
				t.Fatal(e)
			}
			if s == "👨‍👩‍👧‍👦" && scale == 2 {
				if path := os.Getenv("GLANCE_WINDOWS_FAMILY_CAPTURE"); path != "" {
					f, e := os.Create(path)
					if e != nil {
						t.Fatal(e)
					}
					if e = png.Encode(f, shot); e != nil {
						t.Fatal(e)
					}
					f.Close()
				}
			}
			ink := 0
			colored := 0
			for y := 0; y < size.Y; y++ {
				for x := 0; x < size.X; x++ {
					c := shot.RGBAAt(x, y)
					if c.R < 150 && c.G < 150 && c.B < 150 {
						ink++
					}
					if max(c.R, c.G, c.B)-min(c.R, c.G, c.B) > 50 {
						colored++
					}
				}
			}
			// The pinned Noto family symbol is intentionally monochrome.
			if s == "👨‍👩‍👧‍👦" {
				theme.Shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: theme.Face}, PxPerEm: fixed.I(32), MaxWidth: 1000}, s)
				clusters := 0
				represented := 0
				for g, ok := theme.Shaper.NextGlyph(); ok; g, ok = theme.Shaper.NextGlyph() {
					if g.Runes > 0 {
						clusters++
						represented += int(g.Runes)
					}
				}
				if ink < 20 || clusters != 1 || represented != utf8.RuneCountInString(s) {
					t.Errorf("family failed to form one visible glyph cluster: ink=%d clusters=%d runes=%d", ink, clusters, represented)
				}
			} else if colored < 8 {
				t.Errorf("%s scale=%g colored=%d", s, scale, colored)
			}
		}
	}
}
