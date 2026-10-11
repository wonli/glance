package desktopui

import (
	"image"
	"math"
	"strings"
	"unicode"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

type presentationRichFragment struct {
	call     op.CallOp
	width    int
	baseline int
}

type presentationRichLine struct {
	fragments []presentationRichFragment
	width     int
	ascent    int
	descent   int
}

type presentationParagraphLayoutMetrics struct {
	lineHeightScale float32
	lineHeightPT    float64
	fontSizePT      float64
	spaceBeforePT   float64
	spaceAfterPT    float64
}

func presentationParagraphMetrics(paragraph document.PresentationParagraph, baseFontSizePT float64) presentationParagraphLayoutMetrics {
	metrics := presentationParagraphLayoutMetrics{
		lineHeightScale: presentationTextLineHeightScale,
		fontSizePT:      baseFontSizePT,
	}
	if paragraph.LineSpacing != nil && paragraph.LineSpacing.Value > 0 && !math.IsInf(paragraph.LineSpacing.Value, 0) && !math.IsNaN(paragraph.LineSpacing.Value) {
		switch paragraph.LineSpacing.Kind {
		case "pct":
			metrics.lineHeightScale = float32(paragraph.LineSpacing.Value)
			metrics.lineHeightPT = 0
		case "pts":
			metrics.lineHeightPT = paragraph.LineSpacing.Value
			metrics.lineHeightScale = 0
		}
	}
	metrics.spaceBeforePT = presentationSpacingPoints(paragraph.SpaceBefore, baseFontSizePT)
	metrics.spaceAfterPT = presentationSpacingPoints(paragraph.SpaceAfter, baseFontSizePT)
	return metrics
}

func presentationSpacingPoints(spacing *document.PresentationSpacing, baseFontSizePT float64) float64 {
	if spacing == nil || spacing.Value <= 0 || math.IsInf(spacing.Value, 0) || math.IsNaN(spacing.Value) {
		return 0
	}
	switch spacing.Kind {
	case "pct":
		return spacing.Value * baseFontSizePT
	case "pts":
		return spacing.Value
	default:
		return 0
	}
}

func presentationParagraphBaseFontSize(paragraph document.PresentationParagraph) float64 {
	size := 0.0
	for _, run := range paragraph.Runs {
		if strings.TrimSpace(run.Text) != "" && run.SizePT > 0 {
			size = max(size, run.SizePT)
		}
	}
	if size > 0 {
		return size
	}
	return 14
}

func presentationElementParagraphMetrics(elem document.PresentationElement) presentationParagraphLayoutMetrics {
	paragraphs := presentationElementDisplayParagraphs(elem)
	if len(paragraphs) == 0 {
		return presentationParagraphMetrics(document.PresentationParagraph{}, presentationElementFontSize(elem))
	}
	return presentationParagraphMetrics(paragraphs[0], presentationParagraphBaseFontSize(paragraphs[0]))
}

// Baseline spacing is independent of the glyph ink/bounding-box height. Gio
// interprets a zero LineHeightScale as 1.2, so callers using this fixed value
// must explicitly set their scale to one.
func presentationParagraphLineHeight(gtx layout.Context, metrics presentationParagraphLayoutMetrics, scale float64) unit.Sp {
	points := metrics.lineHeightPT
	if points <= 0 {
		points = metrics.fontSizePT * float64(metrics.lineHeightScale)
	}
	return presentationCanvasFontSize(gtx, points, scale)
}

func applyPresentationEditorParagraphMetrics(style *material.EditorStyle, gtx layout.Context, elem document.PresentationElement, scale float64) {
	if style == nil {
		return
	}
	metrics := presentationElementParagraphMetrics(elem)
	style.LineHeight = presentationParagraphLineHeight(gtx, metrics, scale)
	style.LineHeightScale = 1
}

func presentationEditorLayoutInset(gtx layout.Context, elem document.PresentationElement, scale float64) layout.Inset {
	metrics := presentationElementParagraphMetrics(elem)
	return layout.Inset{
		Left:   presentationEditorInsetX,
		Right:  presentationEditorInsetX,
		Top:    presentationEditorInsetY + presentationSpacingDP(gtx, metrics.spaceBeforePT, scale),
		Bottom: presentationEditorInsetY + presentationSpacingDP(gtx, metrics.spaceAfterPT, scale),
	}
}

func presentationSpacingDP(gtx layout.Context, points, scale float64) unit.Dp {
	if points <= 0 || scale <= 0 {
		return 0
	}
	pixels := gtx.Sp(presentationCanvasFontSize(gtx, points, scale))
	pixelsPerDP := float64(gtx.Metric.PxPerDp)
	if pixelsPerDP <= 0 {
		pixelsPerDP = 1
	}
	return unit.Dp(float32(float64(pixels) / pixelsPerDP))
}

func presentationParagraphNeedsRichLayout(paragraph document.PresentationParagraph) bool {
	if paragraph.LineSpacing != nil || paragraph.SpaceBefore != nil || paragraph.SpaceAfter != nil {
		return true
	}
	if len(paragraph.Runs) < 2 {
		for _, run := range paragraph.Runs {
			if run.Baseline != 0 || run.Underline {
				return true
			}
		}
		return false
	}
	first := paragraph.Runs[0]
	for _, run := range paragraph.Runs[1:] {
		if run.Baseline != first.Baseline || run.Font != first.Font || run.SizePT != first.SizePT ||
			run.Bold != first.Bold || run.Italic != first.Italic || run.Underline != first.Underline || run.ColorHex != first.ColorHex {
			return true
		}
	}
	return first.Baseline != 0 || first.Underline
}

func layoutPresentationRichParagraph(gtx layout.Context, theme *material.Theme, paragraph document.PresentationParagraph, wrap bool, scale float64) layout.Dimensions {
	metrics := presentationParagraphMetrics(paragraph, presentationParagraphBaseFontSize(paragraph))
	spaceBefore := gtx.Sp(presentationCanvasFontSize(gtx, metrics.spaceBeforePT, scale))
	spaceAfter := gtx.Sp(presentationCanvasFontSize(gtx, metrics.spaceAfterPT, scale))
	maxWidth := gtx.Constraints.Max.X
	if maxWidth <= 0 {
		maxWidth = 100000
	}
	measureGtx := gtx
	measureGtx.Constraints = layout.Constraints{Max: image.Pt(100000, 100000)}

	lines := make([]presentationRichLine, 0, 2)
	line := presentationRichLine{}
	finishLine := func() {
		if len(line.fragments) == 0 {
			line.ascent = max(1, gtx.Sp(presentationCanvasFontSize(gtx, metrics.fontSizePT, scale)))
		}
		lines = append(lines, line)
		line = presentationRichLine{}
	}

	runs := paragraph.Runs
	if paragraph.Bullet {
		bullet := document.PresentationTextRun{Text: "• ", SizePT: metrics.fontSizePT, ColorHex: "#1F2937"}
		if len(runs) > 0 {
			bullet.Font = runs[0].Font
			bullet.SizePT = runs[0].SizePT
			bullet.ColorHex = runs[0].ColorHex
		}
		runs = append([]document.PresentationTextRun{bullet}, runs...)
	}
	for _, run := range runs {
		for _, fragmentText := range splitPresentationText(run.Text) {
			if fragmentText == "\n" {
				finishLine()
				continue
			}
			if wrap && line.width == 0 && strings.TrimSpace(fragmentText) == "" {
				continue
			}

			sizePT := run.SizePT
			if sizePT <= 0 {
				sizePT = 14
			}
			sizeScale := 1.0
			if run.Baseline != 0 {
				sizeScale = 0.65
			}
			label := material.Label(theme, presentationCanvasFontSize(gtx, sizePT*sizeScale, scale), fragmentText)
			label.LineHeightScale = metrics.lineHeightScale
			if metrics.lineHeightPT > 0 {
				label.LineHeight = presentationCanvasFontSize(gtx, metrics.lineHeightPT, scale)
				label.LineHeightScale = 1
			}
			label.Font.Typeface = presentationTextTypeface(run.Font)
			label.Color = parsePresentationHexColor(run.ColorHex, pptxTextDark)
			if run.Bold {
				label.Font.Weight = font.Bold
			}
			if run.Italic {
				label.Font.Style = font.Italic
			}
			label.MaxLines = 1

			recorder := op.Record(gtx.Ops)
			dims := label.Layout(measureGtx)
			if run.Underline && dims.Size.X > 0 && dims.Size.Y > 0 {
				underlineY := max(0, dims.Size.Y-2)
				paint.FillShape(gtx.Ops, label.Color, clip.Rect{Min: image.Pt(0, underlineY), Max: image.Pt(dims.Size.X, underlineY+1)}.Op())
			}
			call := recorder.Stop()
			if wrap && maxWidth < 100000 && len(line.fragments) > 0 && line.width+dims.Size.X > maxWidth {
				finishLine()
				if strings.TrimSpace(fragmentText) == "" {
					continue
				}
			}
			shift := 0
			if run.Baseline != 0 {
				shift = int(-float64(run.Baseline) / 100000 * sizePT * scale)
			}
			ascent := dims.Size.Y - dims.Baseline
			fragment := presentationRichFragment{call: call, width: dims.Size.X, baseline: shift - ascent}
			line.fragments = append(line.fragments, fragment)
			line.width += fragment.width
			line.ascent = max(line.ascent, ascent-shift)
			line.descent = max(line.descent, dims.Baseline+shift)
		}
	}
	if len(line.fragments) > 0 || len(lines) == 0 {
		finishLine()
	}

	usedWidth := 0
	totalHeight := 0
	baselineY := spaceBefore + lines[0].ascent
	baselineDistance := max(1, gtx.Sp(presentationParagraphLineHeight(gtx, metrics, scale)))
	for _, renderedLine := range lines {
		usedWidth = max(usedWidth, renderedLine.width)
		lineOffset := 0
		if wrap && maxWidth < 100000 {
			switch paragraph.Align {
			case "center":
				lineOffset = max(0, (maxWidth-renderedLine.width)/2)
			case "right":
				lineOffset = max(0, maxWidth-renderedLine.width)
			}
		}
		x := lineOffset
		for _, fragment := range renderedLine.fragments {
			y := baselineY + fragment.baseline
			off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			fragment.call.Add(gtx.Ops)
			off.Pop()
			x += fragment.width
		}
		totalHeight = max(totalHeight, baselineY+renderedLine.descent)
		baselineY += baselineDistance
	}
	if wrap && maxWidth < 100000 {
		usedWidth = maxWidth
	}
	return layout.Dimensions{Size: image.Pt(usedWidth, totalHeight+spaceAfter)}
}

func splitPresentationText(text string) []string {
	var fragments []string
	var current strings.Builder
	currentSpace := false
	hasCurrent := false
	flush := func() {
		if current.Len() > 0 {
			fragments = append(fragments, current.String())
			current.Reset()
			hasCurrent = false
		}
	}
	for _, r := range text {
		if r == '\n' {
			flush()
			fragments = append(fragments, "\n")
			continue
		}
		isSpace := unicode.IsSpace(r)
		if hasCurrent && isSpace != currentSpace {
			flush()
		}
		if !hasCurrent {
			currentSpace = isSpace
			hasCurrent = true
		}
		current.WriteRune(r)
	}
	flush()
	return fragments
}
