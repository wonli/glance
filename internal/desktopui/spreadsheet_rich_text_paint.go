package desktopui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
)

// Use the editor's visible rune regions so formatting never moves its hit areas,
// selection, scrolling or IME anchor. The persistent editor still owns all input.
func paintEditorRichRuns(gtx layout.Context, theme *material.Theme, editor *widget.Editor, runs []document.CellRichTextRun, base material.EditorStyle, foreground color.NRGBA) {
	bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	defer bounds.Pop()
	offset := 0
	var regions []widget.Region
	for _, run := range runs {
		for _, r := range run.Text {
			regions = editor.Regions(offset, offset+1, regions[:0])
			offset++
			if r == '\n' || r == '\r' {
				continue
			}
			for _, region := range regions {
				label := material.Body2(theme, string(r))
				label.TextSize, label.Font, label.Color = base.TextSize, base.Font, foreground
				label.Font.Weight, label.Font.Style = font.Normal, font.Regular
				if run.Bold {
					label.Font.Weight = font.Bold
				}

				if run.FontColor != "" {
					label.Color = parseGridColor(run.FontColor, foreground)
				}
				measure := gtx
				measure.Constraints = layout.Constraints{Max: image.Pt(100000, 100000)}
				rec := op.Record(gtx.Ops)
				dims := label.Layout(measure)
				call := rec.Stop()
				baseline := region.Bounds.Max.Y - region.Baseline
				transform := op.Offset(image.Pt(region.Bounds.Min.X, baseline-(dims.Size.Y-dims.Baseline))).Push(gtx.Ops)
				paintCellRichGlyphs(gtx, call, dims, run.Italic)
				transform.Pop()
				if run.Underline {
					y := baseline + max(1, gtx.Dp(1))
					paint.FillShape(gtx.Ops, label.Color, clip.Rect(image.Rect(region.Bounds.Min.X, y, region.Bounds.Max.X, y+max(1, gtx.Dp(1)))).Op())
				}
			}
		}
	}
	start, end := editor.Selection()
	if start == end && gtx.Focused(editor) {
		pos := editor.CaretCoords()
		height := gtx.Sp(base.TextSize)
		paint.FillShape(gtx.Ops, foreground, clip.Rect(image.Rect(int(pos.X), int(pos.Y)-height, int(pos.X)+max(1, gtx.Dp(1)), int(pos.Y)+max(1, gtx.Dp(2)))).Op())
	}
}

// CJK faces frequently lack an italic face. Both editing and browsing apply
// the same oblique transform to regular glyphs without changing layout metrics.
func paintCellRichGlyphs(gtx layout.Context, call op.CallOp, dims layout.Dimensions, italic bool) {
	if !italic {
		call.Add(gtx.Ops)
		return
	}
	slant := op.Affine(f32.NewAffine2D(1, -0.2, 0.2*float32(dims.Size.Y-dims.Baseline), 0, 1, 0)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	slant.Pop()
}
