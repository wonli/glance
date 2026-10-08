package desktopui

import (
	"image"
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

func TestPresentationPreviewAndEditorHaveSameBaselineDistance(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spacing *document.PresentationSpacing
		metric  float32
		scale   float64
	}{
		{name: "default"},
		{name: "percentage", spacing: &document.PresentationSpacing{Kind: "pct", Value: 1.5}},
		{name: "points", spacing: &document.PresentationSpacing{Kind: "pts", Value: 30}},
		{name: "retina_percentage", spacing: &document.PresentationSpacing{Kind: "pct", Value: 1.5}, metric: 2, scale: 1.7},
		{name: "retina_points", spacing: &document.PresentationSpacing{Kind: "pts", Value: 30}, metric: 2, scale: 0.65},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.metric == 0 {
				tc.metric = 1
			}
			if tc.scale == 0 {
				tc.scale = 1
			}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: tc.metric, PxPerSp: tc.metric}, Constraints: layout.Constraints{Max: image.Pt(500, 500)}}
			theme := material.NewTheme()
			paragraph := document.PresentationParagraph{Runs: []document.PresentationTextRun{{Text: "A\nA", Font: string(preferredSystemTypeface), SizePT: 20}}, LineSpacing: tc.spacing}
			elem := document.PresentationElement{Paragraphs: []document.PresentationParagraph{paragraph}}
			preview := layoutPresentationRichParagraph(gtx, theme, paragraph, false, tc.scale)
			oneLine := paragraph
			oneLine.Runs = append([]document.PresentationTextRun(nil), paragraph.Runs...)
			oneLine.Runs[0].Text = "A"
			previewOneLine := layoutPresentationRichParagraph(gtx, theme, oneLine, false, tc.scale)
			previewStride := float64(preview.Size.Y - previewOneLine.Size.Y)
			var editor widget.Editor
			editor.SetText(paragraph.PlainText())
			style := material.Editor(theme, &editor, "")
			style.Font = presentationElementEditorFont(elem)
			style.TextSize = presentationCanvasFontSize(gtx, 20, tc.scale)
			applyPresentationEditorParagraphMetrics(&style, gtx, elem, tc.scale)
			editor.SetCaret(0, 0)
			style.Layout(gtx)
			first := editor.CaretCoords().Y
			editor.SetCaret(2, 2)
			style.Layout(gtx)
			editorStride := float64(editor.CaretCoords().Y - first)
			if math.Abs(previewStride-editorStride) > 1 {
				t.Fatalf("preview baseline distance %.1fpx differs from editor %.1fpx", previewStride, editorStride)
			}
			point := image.Pt(1, int(math.Round(float64(editor.CaretCoords().Y))))
			caret := presentationCaretOffsetAt(gtx, theme, style.Font, editor.Text(), point, style.TextSize, 0, 500, editor.Alignment, presentationElementParagraphMetrics(elem), tc.scale)
			if caret != 2 {
				t.Fatalf("clicking the second line placed caret at %d, want 2", caret)
			}
		})
	}
}
