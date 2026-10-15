package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/document"
)

// Check actual Word/Excel button rendering, including palette overrides, so a
// hardcoded fill or a single-color tint cannot pass as the new brand artwork.
func TestPenRibbonBrandAndActiveStateVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	wordState, closeWord := newLongWordFlowState(t, 1)
	defer closeWord()
	run := wordState.wordBlocks[0].Paragraph.Inlines[0].Text
	run.Style.Bold = true
	wordState.selectedWordRunID = run.ID
	for _, scale := range []float32{1, 2} {
		for _, format := range []string{"word", "excel"} {
			for _, custom := range []bool{false, true} {
				// Exercise alternate palettes at 2x, where the tiny star has opaque ink.
				if custom && scale == 1 {
					continue
				}
				t.Run(fmt.Sprintf("%s-%gx-custom%t", format, scale, custom), func(t *testing.T) {
					a := New(nil, "")
					a.designTokens = ResolveDesignTokens(ThemeModeLight, DefaultTypography)
					if custom {
						a.designTokens.Colors.Toolbar.AccentAI = color.NRGBA{R: 201, G: 20, B: 30, A: 255}
						a.designTokens.Colors.Toolbar.AccentAISecondary = color.NRGBA{R: 30, G: 180, B: 80, A: 255}
						a.designTokens.Colors.Toolbar.AccentAIStar = color.NRGBA{R: 255, G: 230, B: 0, A: 255}
						a.designTokens.Colors.Toolbar.ActiveGlyph = color.NRGBA{R: 160, G: 20, B: 180, A: 255}
						a.designTokens.Colors.Toolbar.ActiveBackground = color.NRGBA{R: 245, G: 220, B: 245, A: 255}
						a.designTokens.Colors.Toolbar.ActiveBorder = color.NRGBA{R: 200, G: 90, B: 190, A: 255}
					}
					state := wordState
					if format == "excel" {
						state = newDocumentSessionState("")
						state.book = &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
						state.sheet = "Sheet1"
						state.selectionSet = true
						state.virtualGrid = newGridViewport()
						state.virtualGrid.WindowSheet = "Sheet1"
						state.virtualGrid.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}
						state.virtualGrid.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{{Style: document.CellStyle{Bold: true}}}}}
					}
					a.documentSessionState = state
					a.book = state.book
					metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
					size := image.Pt(metric.Dp(144), metric.Dp(80))
					var ops op.Ops
					paint.Fill(&ops, a.tokens().Colors.Toolbar.Background)
					gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(size)}
					a.layoutOfficeRibbonAIEntry(gtx, 80)
					shift := op.Offset(image.Pt(metric.Dp(90), metric.Dp(20))).Push(&ops)
					if format == "word" {
						a.layoutWordRibbonAction(gtx, state, ribbonAction{id: "bold", label: "加粗"}, 35, 34, false)
					} else {
						a.layoutSpreadsheetRibbonAction(gtx, state, ribbonAction{slot: 4, icon: "bold", label: "加粗"}, 35, 34, spreadsheetRibbonActionIconOnly)
					}
					shift.Pop()
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
					colors := a.tokens().Colors.Toolbar
					for label, want := range map[string]color.NRGBA{"AI front": colors.AccentAI, "AI back": colors.AccentAISecondary, "AI star": colors.AccentAIStar, "selected glyph": colors.ActiveGlyph, "selected fill": colors.ActiveBackground, "selected border": colors.ActiveBorder} {
						count := 0
						for y := 0; y < size.Y; y++ {
							for x := 0; x < size.X; x++ {
								p := color.NRGBAModel.Convert(shot.At(x, y)).(color.NRGBA)
								matches := nearByte(p.R, want.R) && nearByte(p.G, want.G) && nearByte(p.B, want.B)
								if label == "AI star" {
									matches = ribbonPixelMatchesInk(p, want, colors.AccentAI)
								}
								if label == "selected glyph" {
									matches = ribbonPixelMatchesInk(p, want, colors.ActiveBackground)
								}
								if matches {
									count++
								}
							}
						}
						if count < 2 {
							t.Errorf("%s has %d visible pixels of token %v", label, count, want)
						}
					}
					if dir := os.Getenv("GLANCE_RIBBON_SCREENSHOTS"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						file, err := os.Create(filepath.Join(dir, fmt.Sprintf("brand-%s-%gx-custom%t.png", format, scale, custom)))
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
}

// Small glyphs can be entirely antialiased at 1x. Require visible ink whose
// channels follow the expected foreground/background blend, not opaque pixels.
func ribbonPixelMatchesInk(pixel, ink, background color.NRGBA) bool {
	linear := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	p := [3]float64{linear(pixel.R), linear(pixel.G), linear(pixel.B)}
	f := [3]float64{linear(ink.R), linear(ink.G), linear(ink.B)}
	b := [3]float64{linear(background.R), linear(background.G), linear(background.B)}
	channel := 0
	for i := 1; i < 3; i++ {
		if math.Abs(f[i]-b[i]) > math.Abs(f[channel]-b[channel]) {
			channel = i
		}
	}
	delta := f[channel] - b[channel]
	if delta == 0 {
		return false
	}
	alpha := (p[channel] - b[channel]) / delta
	if alpha < 0.35 || alpha > 1.05 {
		return false
	}
	for i := 0; i < 3; i++ {
		expected := b[i] + alpha*(f[i]-b[i])
		if math.Abs(p[i]-expected) > 0.025 {
			return false
		}
	}
	return true
}
