package desktopui

import (
	"image"
	"image/color"

	"reflect"

	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

func TestPresentationTextEditorKeepsVisibleDefaultAlignment(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	elem := &h.view.snapshot.Slides[0].Elements[0]
	elem.Bounds = document.PresentationRect{X: 100, Y: 100, W: 480, H: 220}
	elem.PlaceholderType = "title"
	elem.WrapText = false
	elem.Paragraphs = []document.PresentationParagraph{{Runs: []document.PresentationTextRun{{
		Text: "Left aligned title",
	}}}}
	h.elem = *elem
	h.frame()

	point := h.textClickPoint(20, 10)
	h.click(point)
	h.click(point)
	if h.view.editingShape != h.elem.ID {
		t.Fatal("clicking visible title text did not enter edit mode")
	}
	if got := h.view.shapeEditor.Alignment; got != text.Start {
		t.Fatalf("editor alignment = %v, want %v to match the left-aligned preview", got, text.Start)
	}
}

func TestPresentationEnteringEditDoesNotChangeTextStyleOrGeometry(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	before := h.app.pptxDoc.Snapshot()
	beforeElement := presentationElementByID(t, &before.Slides[0], h.elem.ID)
	revision := h.app.pptxDoc.Revision()
	dirty := h.app.pptxDoc.IsDirty()
	h.view.beginPresentationTextEdit(beforeElement)
	if h.view.editingShape != h.elem.ID {
		t.Fatal("text object did not enter edit mode")
	}
	after := h.app.pptxDoc.Snapshot()
	afterElement := presentationElementByID(t, &after.Slides[0], h.elem.ID)
	if !reflect.DeepEqual(afterElement.Paragraphs, beforeElement.Paragraphs) || afterElement.Bounds != beforeElement.Bounds || afterElement.RotationDeg != beforeElement.RotationDeg {
		t.Fatalf("entering edit mode changed text style or geometry: before=%+v after=%+v", beforeElement, afterElement)
	}
	if h.app.pptxDoc.Revision() != revision || h.app.pptxDoc.IsDirty() != dirty {
		t.Fatalf("entering edit mode changed dirty/revision state: before=%t/%d after=%t/%d", dirty, revision, h.app.pptxDoc.IsDirty(), h.app.pptxDoc.Revision())
	}
}

func TestPresentationStyleCommandsKeepEditDraftAndOnlyChangeRequestedStyle(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	h.view.editingShape = h.elem.ID
	h.view.shapeEditor.SetText("尚未提交的草稿")
	h.view.shapeEditor.SetCaret(3, 3)
	h.view.shapeDraftText = h.view.shapeEditor.Text()
	selectionStart, selectionEnd := h.view.shapeEditor.Selection()
	caretBefore := [2]int{selectionStart, selectionEnd}
	before := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if len(before.Paragraphs) == 0 || len(before.Paragraphs[0].Runs) == 0 {
		t.Fatal("selected fixture has no paragraph or run")
	}
	revision := h.app.pptxDoc.Revision()
	h.app.handlePresentationRibbonAction(h.view, "home_bold")
	if got := h.app.pptxDoc.Revision(); got != revision+1 {
		t.Fatalf("bold command revision = %d, want %d", got, revision+1)
	}
	styled := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	for _, paragraph := range styled.Paragraphs {
		for _, run := range paragraph.Runs {
			if !run.Bold {
				t.Fatalf("bold command left run unstyled: %+v", run)
			}
		}
	}
	selectionStart, selectionEnd = h.view.shapeEditor.Selection()
	if h.view.editingShape != h.elem.ID || h.view.selectedShape != h.elem.ID || h.view.shapeEditor.Text() != "尚未提交的草稿" || [2]int{selectionStart, selectionEnd} != caretBefore {
		t.Fatalf("style command disturbed edit state: selected=%d editing=%d draft=%q selection=%v want=%v", h.view.selectedShape, h.view.editingShape, h.view.shapeEditor.Text(), [2]int{selectionStart, selectionEnd}, caretBefore)
	}
	spacingBefore, spacingAfter := styled.Paragraphs[0].LineSpacing, before.Paragraphs[0].LineSpacing
	spacingMatches := spacingBefore == nil && spacingAfter == nil || spacingBefore != nil && spacingAfter != nil && *spacingBefore == *spacingAfter
	if styled.Paragraphs[0].Runs[0].SizePT != before.Paragraphs[0].Runs[0].SizePT || styled.Paragraphs[0].Align != before.Paragraphs[0].Align || !spacingMatches {
		t.Fatalf("bold command changed unrelated text metrics: before=%+v after=%+v", before.Paragraphs[0], styled.Paragraphs[0])
	}
	h.app.handlePresentationRibbonAction(h.view, "home_font_color")
	if h.view.stylePopover != "text_color" || len(h.app.presentationStyleOptions(h.view)) == 0 {
		t.Fatalf("font color action did not open its persistent palette: popover=%q", h.view.stylePopover)
	}
	h.app.handlePresentationRibbonAction(h.view, "home_line_spacing")
	spacingOptions := h.app.presentationStyleOptions(h.view)
	if h.view.stylePopover != "line_spacing" || len(spacingOptions) < 3 {
		t.Fatalf("line spacing action did not open its presets: popover=%q options=%d", h.view.stylePopover, len(spacingOptions))
	}
	if !h.app.applyPresentationStylePatch(h.view, spacingOptions[2].patch) {
		t.Fatalf("applying the 1.5x line-spacing preset failed: status=%q", h.app.statusText())
	}
	spaced := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if spacing := spaced.Paragraphs[0].LineSpacing; spacing == nil || *spacing != (document.PresentationSpacing{Kind: "pct", Value: 1.5}) {
		t.Fatalf("line spacing preset = %+v, want 1.5x", spacing)
	}
	if metrics := presentationParagraphMetrics(spaced.Paragraphs[0], presentationParagraphBaseFontSize(spaced.Paragraphs[0])); metrics.lineHeightScale != 1.5 {
		t.Fatalf("edit preview metrics = %+v, want 1.5x", metrics)
	}
	selectionStart, selectionEnd = h.view.shapeEditor.Selection()
	if [2]int{selectionStart, selectionEnd} != caretBefore || h.view.shapeEditor.Text() != "尚未提交的草稿" {
		t.Fatalf("line spacing changed the active editor draft or caret: selection=%v draft=%q", [2]int{selectionStart, selectionEnd}, h.view.shapeEditor.Text())
	}
}

func TestPresentationTextEditorUsesVisibleFontMetricsAndInsets(t *testing.T) {
	elem := document.PresentationElement{
		PlaceholderType: "title",
		Paragraphs: []document.PresentationParagraph{{Runs: []document.PresentationTextRun{
			{Text: "", Font: "Arial", SizePT: 11},
			{Text: "Visible title", Font: "Arial", SizePT: 32, Bold: true},
		}}},
	}
	if got := presentationElementFontSize(elem); got != 32 {
		t.Fatalf("editor base font size = %.1f pt, want first visible preview run at 32 pt", got)
	}
	if presentationEditorInsetX != presentationTextInsetX || presentationEditorInsetY != presentationTextInsetY {
		t.Fatalf("editor insets %g/%g dp differ from preview insets %g/%g dp", presentationEditorInsetX, presentationEditorInsetY, presentationTextInsetX, presentationTextInsetY)
	}
}

func TestPresentationTextEditorUsesProjectedTextColor(t *testing.T) {
	elem := document.PresentationElement{Paragraphs: []document.PresentationParagraph{{Runs: []document.PresentationTextRun{{Text: "colored", ColorHex: "#12ABEF"}}}}}
	if got, want := presentationElementEditorColor(elem), (color.NRGBA{R: 18, G: 171, B: 239, A: 255}); got != want {
		t.Fatalf("editor color = %+v, want projected run color %+v", got, want)
	}
}

func TestPresentationEditorUsesParagraphLineAndOuterSpacing(t *testing.T) {
	paragraph := document.PresentationParagraph{
		Runs:        []document.PresentationTextRun{{Text: "line one\nline two", SizePT: 20}},
		LineSpacing: &document.PresentationSpacing{Kind: "pct", Value: 1.5},
		SpaceBefore: &document.PresentationSpacing{Kind: "pts", Value: 6},
		SpaceAfter:  &document.PresentationSpacing{Kind: "pct", Value: 0.5},
	}
	if !presentationParagraphNeedsRichLayout(paragraph) {
		t.Fatal("paragraph with explicit spacing used the plain-label path, which drops PPTX metrics")
	}
	metrics := presentationParagraphMetrics(paragraph, 20)
	if metrics.lineHeightScale != 1.5 || metrics.lineHeightPT != 0 {
		t.Fatalf("percentage line spacing metrics = %+v, want 1.5x line height", metrics)
	}
	if metrics.spaceBeforePT != 6 || metrics.spaceAfterPT != 10 {
		t.Fatalf("paragraph spacing metrics = %+v, want 6pt before and 10pt after", metrics)
	}

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(500, 500)},
	}
	style := material.EditorStyle{}
	elem := document.PresentationElement{Paragraphs: []document.PresentationParagraph{paragraph}}
	applyPresentationEditorParagraphMetrics(&style, gtx, elem, 1)
	if style.LineHeightScale != 1 || style.LineHeight != unit.Sp(30) {
		t.Fatalf("editor line metrics = scale %.2f, absolute %g, want fixed 30sp", style.LineHeightScale, style.LineHeight)
	}
	inset := presentationEditorLayoutInset(gtx, elem, 1)
	if gtx.Dp(inset.Top) != 7 || gtx.Dp(inset.Bottom) != 11 {
		t.Fatalf("editor vertical insets = %d/%d px, want 7/11 including PPTX spacing", gtx.Dp(inset.Top), gtx.Dp(inset.Bottom))
	}

	base := paragraph
	base.LineSpacing, base.SpaceBefore, base.SpaceAfter = nil, nil, nil
	baseDims := layoutPresentationRichParagraph(gtx, material.NewTheme(), base, false, 1)
	spacedDims := layoutPresentationRichParagraph(gtx, material.NewTheme(), paragraph, false, 1)
	if spacedDims.Size.Y <= baseDims.Size.Y+16 {
		t.Fatalf("preview line and paragraph spacing height = %d, base %d; want explicit paragraph spacing plus a larger inter-line gap", spacedDims.Size.Y, baseDims.Size.Y)
	}

	paragraph.LineSpacing = &document.PresentationSpacing{Kind: "pts", Value: 30}
	metrics = presentationParagraphMetrics(paragraph, 20)
	if metrics.lineHeightPT != 30 || metrics.lineHeightScale != 0 {
		t.Fatalf("absolute line spacing metrics = %+v, want 30pt baseline distance", metrics)
	}
	elem.Paragraphs[0] = paragraph
	style = material.EditorStyle{}
	applyPresentationEditorParagraphMetrics(&style, gtx, elem, 1)
	if style.LineHeight != unit.Sp(30) || style.LineHeightScale != 1 {
		t.Fatalf("editor absolute line metrics = scale %.2f, absolute %g, want 30sp", style.LineHeightScale, style.LineHeight)
	}
}
