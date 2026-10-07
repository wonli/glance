package desktopui

import (
	"image"
	"strings"
	"unicode"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

type presentationRichFragment struct {
	call     op.CallOp
	width    int
	height   int
	baseline int
}

type presentationRichLine struct {
	fragments []presentationRichFragment
	width     int
	height    int
	ascent    int
}

func presentationParagraphNeedsRichLayout(paragraph document.PresentationParagraph) bool {
	if len(paragraph.Runs) < 2 {
		for _, run := range paragraph.Runs {
			if run.Baseline != 0 {
				return true
			}
		}
		return false
	}
	first := paragraph.Runs[0]
	for _, run := range paragraph.Runs[1:] {
		if run.Baseline != first.Baseline || run.Font != first.Font || run.SizePT != first.SizePT ||
			run.Bold != first.Bold || run.Italic != first.Italic || run.ColorHex != first.ColorHex {
			return true
		}
	}
	return first.Baseline != 0
}

func layoutPresentationRichParagraph(gtx layout.Context, theme *material.Theme, paragraph document.PresentationParagraph, wrap bool, scale float64) layout.Dimensions {
	maxWidth := gtx.Constraints.Max.X
	if maxWidth <= 0 {
		maxWidth = 100000
	}
	measureGtx := gtx
	measureGtx.Constraints = layout.Constraints{Max: image.Pt(100000, 100000)}

	lines := make([]presentationRichLine, 0, 2)
	line := presentationRichLine{}
	finishLine := func() {
		if line.height == 0 {
			line.height = max(1, gtx.Dp(unit.Dp(16)))
		}
		lines = append(lines, line)
		line = presentationRichLine{}
	}

	for _, run := range paragraph.Runs {
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
			fragment := presentationRichFragment{call: call, width: dims.Size.X, height: dims.Size.Y, baseline: shift}
			line.fragments = append(line.fragments, fragment)
			line.width += fragment.width
			line.height = max(line.height, fragment.height+max(0, shift)+max(0, -shift))
			line.ascent = max(line.ascent, max(0, -shift))
		}
	}
	if len(line.fragments) > 0 || len(lines) == 0 {
		finishLine()
	}

	usedWidth := 0
	totalHeight := 0
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
			y := totalHeight + renderedLine.ascent + fragment.baseline
			off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			fragment.call.Add(gtx.Ops)
			off.Pop()
			x += fragment.width
		}
		totalHeight += renderedLine.height
	}
	if wrap && maxWidth < 100000 {
		usedWidth = maxWidth
	}
	return layout.Dimensions{Size: image.Pt(usedWidth, totalHeight)}
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
