package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestSpreadsheetFormatCellsCategoriesFitNarrowWidth(t *testing.T) {
	for _, width := range []int{220, 320, 520, 720} {
		for category := spreadsheetFormatCellsNumber; category <= spreadsheetFormatCellsFill; category++ {
			name := spreadsheetFormatCellsCategoryName(category)
			t.Run(fmt.Sprintf("%ddp/%s", width, name), func(t *testing.T) {
				_, _, dims := layoutSpreadsheetFormatCellsForTest(width, 640, category)
				if dims.Size.X > width {
					t.Fatalf("dialog content width = %d; exceeds available width %d", dims.Size.X, width)
				}
			})
		}
	}
}

func TestSpreadsheetFormatCellsBorderActionsIncludeCellEdgesAndInterior(t *testing.T) {
	state := &spreadsheetFormatCellsDialogState{}
	state.init()
	want := map[string]bool{
		"all": true, "outside": true, "top": true, "right": true,
		"bottom": true, "left": true, "inside": true, "none": true,
	}
	for _, choice := range state.borderChoices {
		if !want[choice.value] {
			t.Fatalf("unexpected border action %q", choice.value)
		}
		delete(want, choice.value)
	}
	if len(want) != 0 {
		t.Fatalf("missing border actions: %v", want)
	}
	state.borderPreset, state.borderTouched = "inside", true
	patch, changed := state.patch()
	if !changed || patch.BorderPreset == nil || *patch.BorderPreset != "inside" {
		t.Fatalf("inside border draft patch = %+v, changed=%t", patch, changed)
	}
}

func TestSpreadsheetFormatCellsDoesNotPaintOutsideNarrowWidth(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const width, canvasWidth, height = 220, 800, 640
	window, err := headless.NewWindow(canvasWidth, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	background := color.NRGBA{R: 255, G: 0, B: 255, A: 255}

	for category := spreadsheetFormatCellsNumber; category <= spreadsheetFormatCellsFill; category++ {
		t.Run(spreadsheetFormatCellsCategoryName(category), func(t *testing.T) {
			ops := new(op.Ops)
			gtx := layout.Context{
				Ops:         ops,
				Constraints: layout.Constraints{Max: image.Pt(width, height)},
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			}
			paint.FillShape(gtx.Ops, background, clip.Rect{Max: image.Pt(canvasWidth, height)}.Op())
			app := &App{theme: material.NewTheme()}
			state := &spreadsheetFormatCellsDialogState{}
			state.init()
			state.category = category
			app.layoutSpreadsheetFormatCellsContents(gtx, state)
			if err := window.Frame(ops); err != nil {
				t.Fatalf("render format dialog: %v", err)
			}
			screenshot := image.NewRGBA(image.Rect(0, 0, canvasWidth, height))
			if err := window.Screenshot(screenshot); err != nil {
				t.Fatalf("capture format dialog: %v", err)
			}
			for x := width; x < canvasWidth; x++ {
				for y := 0; y < height; y++ {
					pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
					if pixel != background {
						t.Fatalf("format dialog painted outside available width at (%d,%d): %v", x, y, pixel)
					}
				}
			}
		})
	}
}

func layoutSpreadsheetFormatCellsForTest(width, height int, category spreadsheetFormatCellsCategory) (*op.Ops, *spreadsheetFormatCellsDialogState, layout.Dimensions) {
	app := &App{theme: material.NewTheme()}
	state := &spreadsheetFormatCellsDialogState{}
	state.init()
	state.category = category
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Constraints: layout.Constraints{Max: image.Pt(width, height)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	dims := app.layoutSpreadsheetFormatCellsContents(gtx, state)
	return ops, state, dims
}

func spreadsheetFormatCellsCategoryName(category spreadsheetFormatCellsCategory) string {
	switch category {
	case spreadsheetFormatCellsNumber:
		return "number"
	case spreadsheetFormatCellsAlignment:
		return "alignment"
	case spreadsheetFormatCellsFont:
		return "font"
	case spreadsheetFormatCellsBorder:
		return "border"
	case spreadsheetFormatCellsFill:
		return "fill"
	default:
		return "unknown"
	}
}
