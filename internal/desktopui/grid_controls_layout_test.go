package desktopui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func TestFormulaEditorFieldCentersTextVertically(t *testing.T) {
	app := New(nil, "")
	const width, height = 110, 30
	editor := &widget.Editor{SingleLine: true}
	editor.SetText("A1")
	var fieldOps op.Ops
	fieldGtx := layout.Context{
		Ops:         &fieldOps,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(width, height)},
	}
	fieldDims := layoutFormulaEditorField(fieldGtx, app.theme, editor, "A1", unit.Dp(width))

	naturalEditor := &widget.Editor{SingleLine: true}
	naturalEditor.SetText("A1")
	var naturalOps op.Ops
	naturalGtx := layout.Context{
		Ops:         &naturalOps,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(width-20, height)},
	}
	style := material.Editor(app.theme, naturalEditor, "A1")
	style.TextSize = unit.Sp(13)
	naturalDims := style.Layout(naturalGtx)
	opticalInset := fieldGtx.Dp(formulaEditorOpticalCenterTopInset)
	wantBaseline := naturalDims.Baseline + (height-naturalDims.Size.Y-opticalInset)/2
	if fieldDims.Size.Y != height {
		t.Fatalf("formula editor field height=%d; want %d", fieldDims.Size.Y, height)
	}
	if fieldDims.Baseline != wantBaseline {
		t.Fatalf("formula text baseline=%d; want vertically centered baseline %d", fieldDims.Baseline, wantBaseline)
	}
}
