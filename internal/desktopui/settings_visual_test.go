package desktopui

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/i18n"
)

func TestSettingsFieldsVerticallyCenterText(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, value := range []string{"", "gpt-4.1", "中文模型"} {
		for _, key := range []bool{false, true} {
			t.Run(value+map[bool]string{true: "/key", false: "/model"}[key], func(t *testing.T) {
				app := New(nil, "")
				app.agentUI.apiKeyInput.SingleLine = true
				app.agentUI.apiKeyInput.SetText(value)
				editor := &widget.Editor{SingleLine: true}
				editor.SetText(value)
				var ops op.Ops
				paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(400, 0), Max: image.Pt(400, 150)}}
				var fieldTop int
				if key {
					app.layoutAgentSettingsKeyField(gtx)
					fieldTop = 27
				} else {
					agentSettingsPageField(gtx, app.theme, app.tokens().Typography, "默认模型", editor, "输入模型")
					fieldTop = 27
				}
				img := renderPresentationTestOps(t, &ops, image.Pt(400, 150))
				top, bottom, ok := darkInkVerticalBounds(img, image.Rect(12, fieldTop+1, 220, fieldTop+43))
				if !ok {
					t.Fatal("field has no visible text")
				}
				if center := float64(top+bottom) / 2; math.Abs(center-float64(fieldTop+22)) > 2 {
					t.Errorf("text center %.1f, want field center %d (ink %d..%d)", center, fieldTop+22, top, bottom)
				}
			})
		}
	}
}

func TestSettingsNavigationCentersIconAndText(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := New(nil, "")
	app.agentUI.settingsPageTab = "providers"
	var ops op.Ops
	paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(214, 0), Max: image.Pt(214, 60)}}
	app.layoutSettingsNavItem(gtx, "providers", "AI 提供商", "bot")
	img := renderPresentationTestOps(t, &ops, image.Pt(214, 60))
	it, ib, iok := darkInkVerticalBounds(img, image.Rect(10, 0, 31, 44))
	tt, tb, tok := darkInkVerticalBounds(img, image.Rect(42, 0, 170, 44))
	if !iok || !tok {
		t.Fatal("navigation icon or text is missing")
	}
	if math.Abs(float64(it+ib-tt-tb)/2) > 2 {
		t.Errorf("icon center %.1f != text center %.1f", float64(it+ib)/2, float64(tt+tb)/2)
	}
}

func TestSettingsBlackIconsRenderDarkStrokes(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, name := range []string{"bot", "plug", "shield-check", "eye", "eye-off", "info"} {
		t.Run(name, func(t *testing.T) {
			var ops op.Ops
			paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(24, 24)}}
			homeIcon(gtx, name, 20, settingsBlack)
			img := renderPresentationTestOps(t, &ops, image.Pt(24, 24))
			count := 0
			for y := 0; y < 20; y++ {
				for x := 0; x < 20; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r < 80*257 && g < 80*257 && b < 80*257 {
						count++
					}
				}
			}
			if count < 8 {
				t.Errorf("%s has only %d dark stroke pixels; black icon appears faded", name, count)
			}
		})
	}
}

func TestSettingsVisualSnapshot(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := New(nil, "")
	app.homeSection = homeSectionSettings
	app.agentUI.settingsPageTab = "providers"
	app.agentUI.settingsTab = "openai"
	app.agentUI.providerInput.SetText("openai-compatible")
	app.agentUI.baseURLInput.SetText("https://api.openai.com/v1")
	app.agentUI.modelInput.SetText("gpt-4.1")
	var ops op.Ops
	size := image.Pt(1440, 900)
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: size}}
	dims := app.layoutRecent(gtx)
	if dims.Size != size {
		t.Fatalf("settings page size %v, want %v", dims.Size, size)
	}
	img := renderPresentationTestOps(t, &ops, size)
	if path := os.Getenv("GLANCE_SETTINGS_SCREENSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSettingsIconsKeepStrokeWeightAtHiDPI(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	counts := make([]int, 2)
	for index, scale := range []float32{1, 2} {
		var ops op.Ops
		paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		size := image.Pt(int(20*scale), int(20*scale))
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: size}}
		homeIcon(gtx, "bot", 20, settingsBlack)
		img := renderPresentationTestOps(t, &ops, size)
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 80*257 && g < 80*257 && b < 80*257 {
					counts[index]++
				}
			}
		}
	}
	if counts[1] < counts[0]*3 {
		t.Fatalf("HiDPI stroke loses weight: dark pixels at 1x=%d, 2x=%d", counts[0], counts[1])
	}
}

func TestSettingsLanguageControlIsReadableAndClickableAtOneAndTwoX(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		t.Run(map[float32]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			a := newAppWithRuntime(nil, newDesktopRuntime())
			a.applyLocaleSelection(i18n.SelectionEnglish)
			var router input.Router
			var ops op.Ops
			width, height := int(620*scale), int(240*scale)
			frame := func() layout.Dimensions {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(width, height)}}
				a.processLanguageMenuClicks(gtx)
				dims := a.layoutLanguageSettingCard(gtx)
				router.Frame(&ops)
				return dims
			}
			dims := frame()
			if dims.Size != image.Pt(width, int(82*scale)) {
				t.Fatalf("language row bounds = %v, want %v", dims.Size, image.Pt(width, int(82*scale)))
			}
			img := renderPresentationTestOps(t, &ops, image.Pt(width, int(82*scale)))
			// Include both title and muted help text, excluding the card border
			// and selector, to verify that the text group is centered as a whole.
			textArea := image.Rect(int(18*scale), int(2*scale), int(360*scale), int(80*scale))
			textTop, textBottom := textArea.Max.Y, textArea.Min.Y
			for y := textArea.Min.Y; y < textArea.Max.Y; y++ {
				for x := textArea.Min.X; x < textArea.Max.X; x++ {
					r, g, b, alpha := img.At(x, y).RGBA()
					if alpha > 0 && r < 200*257 && g < 200*257 && b < 200*257 {
						textTop, textBottom = min(textTop, y), max(textBottom, y)
					}
				}
			}
			if textTop > textBottom || math.Abs(float64(textTop+textBottom)/2-float64(41*scale)) > float64(5*scale) {
				t.Fatalf("language title/help group is not vertically centered: y=%d..%d", textTop, textBottom)
			}
			labelTop, labelBottom, ok := darkInkVerticalBounds(img, image.Rect(int(float32(width)-230*scale), int(22*scale), int(float32(width)-120*scale), int(60*scale)))
			if !ok {
				t.Fatal("English language selection is not visible in the row")
			}
			if center := float64(labelTop+labelBottom) / 2; math.Abs(center-float64(41*scale)) > float64(3*scale) {
				t.Fatalf("English language label center %.1f, want %.1f", center, float64(41*scale))
			}
			center := f32.Pt(float32(width)-128*scale, float32(41*scale))
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: center, Buttons: pointer.ButtonPrimary})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: center})
			frame()
			openDims := frame()
			if !a.agentUI.languageMenuOpen {
				t.Fatal("click at the language control center did not open the menu")
			}
			wantOpenHeight := int(201 * scale)
			if openDims.Size.Y != wantOpenHeight {
				t.Fatalf("open language menu bounds = %v, want height %d", openDims.Size, wantOpenHeight)
			}
			img = renderPresentationTestOps(t, &ops, image.Pt(width, wantOpenHeight))
			longLabel := image.Rect(int(float32(width)-230*scale), int(129*scale), int(float32(width)-35*scale), int(159*scale))
			if _, _, ok := darkInkVerticalBounds(img, longLabel); !ok {
				t.Fatal("long English option label is not visible within the menu bounds")
			}
		})
	}
}
