package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/markdowndoc"
)

var (
	agentMarkdownHeading1Color   = color.NRGBA{R: 38, G: 49, B: 61, A: 255}
	agentMarkdownHeadingSubColor = color.NRGBA{R: 55, G: 70, B: 86, A: 255}
	agentMarkdownQuoteTextColor  = color.NRGBA{R: 101, G: 113, B: 128, A: 255}
	agentMarkdownCodeBgColor     = color.NRGBA{R: 241, G: 244, B: 248, A: 255}
	agentMarkdownCodeBorderColor = color.NRGBA{R: 222, G: 228, B: 236, A: 255}
	agentMarkdownInlineCodeBg    = color.NRGBA{R: 236, G: 240, B: 245, A: 255}
	agentMarkdownInlineCodeFg    = color.NRGBA{R: 45, G: 62, B: 80, A: 255}
	agentMarkdownRuleColor       = color.NRGBA{R: 224, G: 229, B: 235, A: 255}
	agentMarkdownLinkColor       = color.NRGBA{R: 53, G: 95, B: 150, A: 255}
)

func newAgentAssistantLine(text string) agentChatLine {
	line := agentChatLine{role: "assistant", text: text}
	line.ensureMarkdownBlocks()
	return line
}

func (l *agentChatLine) ensureMarkdownBlocks() []document.MarkdownBlock {
	if l == nil {
		return nil
	}
	if l.markdownSource == l.text && (l.text == "" || l.markdownBlocks != nil) {
		return l.markdownBlocks
	}
	l.markdownSource = l.text
	snapshot, err := markdowndoc.Parse(l.text)
	if err != nil {
		l.markdownBlocks = nil
		return nil
	}
	l.markdownBlocks = snapshot.Blocks
	return l.markdownBlocks
}

func (a *App) layoutAgentAssistantMarkdown(gtx layout.Context, line *agentChatLine) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	blocks := line.ensureMarkdownBlocks()
	if len(blocks) == 0 {
		body := material.Body1(a.theme, line.text)
		body.TextSize = unit.Sp(12)
		body.LineHeight = unit.Sp(20)
		body.Color = agentSidebarText
		body.MaxLines = 0
		return body.Layout(gtx)
	}
	children := make([]layout.FlexChild, 0, len(blocks))
	for i := range blocks {
		blockIndex := i
		topInset := unit.Dp(0)
		if blockIndex > 0 {
			prev := blocks[blockIndex-1]
			curr := blocks[blockIndex]
			switch {
			case curr.Kind == "heading":
				topInset = unit.Dp(10)
			case prev.Kind == "list" && curr.Kind == "list":
				topInset = unit.Dp(4)
			case prev.Kind == "heading":
				topInset = unit.Dp(4)
			default:
				topInset = unit.Dp(8)
			}
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Inset{Top: topInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentMarkdownBlock(gtx, blocks[blockIndex])
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutAgentMarkdownBlock(gtx layout.Context, b document.MarkdownBlock) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	switch b.Kind {
	case "rule":
		height := max(1, gtx.Dp(unit.Dp(1)))
		size := image.Pt(gtx.Constraints.Max.X, height)
		paint.FillShape(gtx.Ops, agentMarkdownRuleColor, clip.Rect{Max: size}.Op())
		return layout.Dimensions{Size: size}

	case "code":
		return a.layoutAgentMarkdownCodeBlock(gtx, b)

	case "table":
		return a.layoutAgentMarkdownTable(gtx, b)

	case "math":
		return agentProcessLeftBar(gtx, agentProcessLeftBarColor, func(gtx layout.Context) layout.Dimensions {
			style := homeLabelStyle(a.theme, strings.TrimSpace(b.Text), unit.Sp(11), agentSidebarText, font.Normal)
			style.Font.Typeface = jsonMonoFace
			style.MaxLines = 0
			return style.Layout(gtx)
		})

	case "quote":
		spans := b.Spans
		if b.Prefix != "" && b.Prefix != "> " {
			spans = append([]document.MarkdownSpan{{Text: b.Prefix}}, spans...)
		}
		return agentProcessLeftBar(gtx, agentProcessLeftBarColor, func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentMarkdownSpans(gtx, spans, unit.Sp(11.5), unit.Sp(19), agentMarkdownQuoteTextColor, font.Normal)
		})

	case "heading":
		size := unit.Sp(12)
		lineHeight := unit.Sp(19)
		fg := agentMarkdownHeadingSubColor
		weight := font.SemiBold
		switch b.Level {
		case 1:
			size = unit.Sp(14)
			lineHeight = unit.Sp(22)
			fg = agentMarkdownHeading1Color
			weight = font.Bold
		case 2:
			size = unit.Sp(13)
			lineHeight = unit.Sp(20)
			fg = agentMarkdownHeading1Color
		}
		return a.layoutAgentMarkdownSpans(gtx, b.Spans, size, lineHeight, fg, weight)

	case "list":
		return a.layoutAgentMarkdownListBlock(gtx, b)

	default:
		spans := b.Spans
		if b.Prefix != "" && (len(b.Spans) == 0 || b.Spans[0].Kind != "task") {
			spans = append([]document.MarkdownSpan{{Text: b.Prefix}}, spans...)
		}
		indent := max(0, b.Level-1) * 12
		indent = min(indent, max(0, gtx.Constraints.Max.X/3))
		return layout.Inset{Left: unit.Dp(indent)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentMarkdownSpans(gtx, spans, unit.Sp(12), unit.Sp(20), agentSidebarText, font.Normal)
		})
	}
}

func (a *App) layoutAgentMarkdownListBlock(gtx layout.Context, b document.MarkdownBlock) layout.Dimensions {
	indent := max(0, b.Level-1) * 12
	indent = min(indent, max(0, gtx.Constraints.Max.X/3))
	marker := b.Prefix
	spans := b.Spans
	if len(spans) > 0 && spans[0].Kind == "task" {
		marker = spans[0].Text
		spans = spans[1:]
	}
	return layout.Inset{Left: unit.Dp(indent)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if marker == "" {
			return a.layoutAgentMarkdownSpans(gtx, spans, unit.Sp(12), unit.Sp(20), agentSidebarText, font.Normal)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				label := material.Body1(a.theme, marker)
				label.TextSize = unit.Sp(12)
				label.LineHeight = unit.Sp(20)
				label.Color = agentMarkdownHeadingSubColor
				label.Font.Weight = font.SemiBold
				label.MaxLines = 1
				dims := label.Layout(gtx)
				minWidth := gtx.Dp(unit.Dp(14))
				if dims.Size.X < minWidth {
					dims.Size.X = minWidth
				}
				return dims
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentMarkdownSpans(gtx, spans, unit.Sp(12), unit.Sp(20), agentSidebarText, font.Normal)
			}),
		)
	})
}

func (a *App) layoutAgentMarkdownCodeBlock(gtx layout.Context, b document.MarkdownBlock) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return agentHistoryMeasuredPanel(gtx, agentMarkdownCodeBgColor, agentMarkdownCodeBorderColor, unit.Dp(6), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			text := strings.TrimSuffix(b.Text, "\n")
			if len(b.CodeSpans) > 0 {
				spans := make([]document.MarkdownSpan, len(b.CodeSpans))
				copy(spans, b.CodeSpans)
				for i := range spans {
					if spans[i].CodeColor == "" {
						spans[i].CodeColor = "2D3E50"
					}
				}
				last := len(spans) - 1
				spans[last].Text = strings.TrimSuffix(spans[last].Text, "\n")
				return a.layoutAgentMarkdownRichSpans(gtx, spans, text, unit.Sp(11), unit.Sp(17), agentMarkdownInlineCodeFg, font.Normal, true)
			}
			style := homeLabelStyle(a.theme, text, unit.Sp(11), agentMarkdownInlineCodeFg, font.Normal)
			style.Font.Typeface = jsonMonoFace
			style.MaxLines = 0
			return style.Layout(gtx)
		})
	})
}

func (a *App) layoutAgentMarkdownTable(gtx layout.Context, b document.MarkdownBlock) layout.Dimensions {
	width := gtx.Constraints.Max.X
	if width <= 0 || len(b.Rows) == 0 {
		return layout.Dimensions{}
	}
	y := 0
	padX := gtx.Dp(unit.Dp(6))
	padY := gtx.Dp(unit.Dp(5))
	for rowIndex, row := range b.Rows {
		if len(row) == 0 {
			continue
		}
		calls := make([]op.CallOp, len(row))
		dims := make([]layout.Dimensions, len(row))
		rowHeight := 0
		for col, cell := range row {
			cellWidth := width*(col+1)/len(row) - width*col/len(row)
			inner := max(1, cellWidth-2*padX)
			cellGtx := gtx
			cellGtx.Constraints = layout.Constraints{Max: image.Pt(inner, 1<<20)}
			spans := []document.MarkdownSpan{{Text: cell}}
			if rowIndex < len(b.TableSpans) && col < len(b.TableSpans[rowIndex]) && len(b.TableSpans[rowIndex][col]) > 0 {
				spans = b.TableSpans[rowIndex][col]
			}
			weight := font.Normal
			if rowIndex == 0 {
				weight = font.SemiBold
			}
			record := op.Record(gtx.Ops)
			dims[col] = a.layoutAgentMarkdownSpans(cellGtx, spans, unit.Sp(11), unit.Sp(16), agentSidebarText, weight)
			calls[col] = record.Stop()
			rowHeight = max(rowHeight, dims[col].Size.Y+padY*2)
		}
		for col := range row {
			x := width * col / len(row)
			cellWidth := width*(col+1)/len(row) - x
			push := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			bounds := clip.Rect{Max: image.Pt(cellWidth, rowHeight)}
			fill := color.NRGBA{R: 252, G: 253, B: 254, A: 255}
			if rowIndex == 0 {
				fill = agentMarkdownCodeBgColor
			}
			paint.FillShape(gtx.Ops, fill, bounds.Op())
			paint.FillShape(gtx.Ops, agentMarkdownCodeBorderColor, clip.Stroke{Path: bounds.Path(), Width: float32(max(1, gtx.Dp(unit.Dp(1))))}.Op())
			textX := padX
			extra := max(0, cellWidth-2*padX-dims[col].Size.X)
			if col < len(b.TableAlignments) {
				switch b.TableAlignments[col] {
				case "center":
					textX += extra / 2
				case "right":
					textX += extra
				}
			}
			offset := op.Offset(image.Pt(textX, padY)).Push(gtx.Ops)
			calls[col].Add(gtx.Ops)
			offset.Pop()
			push.Pop()
		}
		y += rowHeight
	}
	return layout.Dimensions{Size: image.Pt(width, y)}
}

func agentMarkdownSpansArePlainUniform(spans []document.MarkdownSpan) (string, bool) {
	var builder strings.Builder
	for _, s := range spans {
		if s.Kind != "" || s.Bold || s.Italic || s.Strike || s.Destination != "" {
			return "", false
		}
		builder.WriteString(s.Text)
	}
	return builder.String(), true
}

func agentMarkdownPlainString(spans []document.MarkdownSpan) string {
	var builder strings.Builder
	for _, s := range spans {
		builder.WriteString(s.Text)
	}
	return builder.String()
}

func (a *App) layoutAgentMarkdownSpans(gtx layout.Context, spans []document.MarkdownSpan, size, lineHeight unit.Sp, fg color.NRGBA, weight font.Weight) layout.Dimensions {
	if plainText, uniform := agentMarkdownSpansArePlainUniform(spans); uniform {
		body := material.Body1(a.theme, plainText)
		body.TextSize = size
		body.LineHeight = lineHeight
		body.Color = fg
		body.Font.Weight = weight
		body.MaxLines = 0
		return body.Layout(gtx)
	}
	return a.layoutAgentMarkdownRichSpans(gtx, spans, agentMarkdownPlainString(spans), size, lineHeight, fg, weight, false)
}

func (a *App) layoutAgentMarkdownRichSpans(gtx layout.Context, spans []document.MarkdownSpan, semanticText string, size, lineHeight unit.Sp, fg color.NRGBA, baseWeight font.Weight, codeBlock bool) layout.Dimensions {
	width := max(1, gtx.Constraints.Max.X)
	defaultRowHeight := max(gtx.Sp(lineHeight), gtx.Sp(size)*3/2)
	x, y, rowHeight := 0, 0, defaultRowHeight
	maxX := 0

	bodyRecord := op.Record(gtx.Ops)
	for _, s := range spans {
		text := s.Text
		if s.Kind == "image" && text == "" {
			text = s.Destination
		}
		inlineCode := s.Kind == "code" && !codeBlock
		padX := 0
		if inlineCode {
			padX = gtx.Dp(unit.Dp(3))
		}
		tokenSize := size
		if inlineCode {
			tokenSize = max(unit.Sp(10.5), size-unit.Sp(1))
		}
		makeStyle := func(segment string) material.LabelStyle {
			style := homeLabelStyle(a.theme, segment, tokenSize, fg, baseWeight)
			style.MaxLines = 1
			if s.Bold {
				style.Font.Weight = font.SemiBold
				if fg == agentSidebarText {
					style.Color = agentMarkdownHeading1Color
				}
			}
			if s.Italic {
				style.Font.Style = font.Italic
			}
			if s.Kind == "code" || s.Kind == "math" {
				style.Font.Typeface = jsonMonoFace
				style.Color = agentMarkdownInlineCodeFg
				if c, ok := parseWordRunColor(s.CodeColor); ok {
					style.Color = c
				}
			}
			if s.Destination != "" {
				style.Color = agentMarkdownLinkColor
			}
			return style
		}
		measureSegment := func(segment string) (layout.Dimensions, op.CallOp, color.NRGBA) {
			rec := op.Record(gtx.Ops)
			m := gtx
			m.Constraints = layout.Constraints{Max: image.Pt(1<<20, 1<<20)}
			st := makeStyle(segment)
			d := st.Layout(m)
			return d, rec.Stop(), st.Color
		}
		flushSegment := func(segment string) {
			if segment == "" {
				return
			}
			textDims, tokenCall, strokeColor := measureSegment(segment)
			totalW := textDims.Size.X + 2*padX
			totalH := textDims.Size.Y
			if x > 0 && x+totalW > width {
				y += rowHeight
				x = 0
				rowHeight = defaultRowHeight
			}
			push := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			if inlineCode {
				pill := clip.UniformRRect(image.Rectangle{
					Min: image.Pt(0, max(0, gtx.Dp(unit.Dp(1)))),
					Max: image.Pt(totalW, max(totalH-gtx.Dp(unit.Dp(1)), 1)),
				}, gtx.Dp(unit.Dp(3)))
				paint.FillShape(gtx.Ops, agentMarkdownInlineCodeBg, pill.Op(gtx.Ops))
			}
			textOffset := op.Offset(image.Pt(padX, 0)).Push(gtx.Ops)
			tokenCall.Add(gtx.Ops)
			if s.Strike {
				lineY := max(0, textDims.Size.Y-textDims.Baseline-gtx.Sp(tokenSize)*3/10)
				paint.FillShape(gtx.Ops, strokeColor, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(textDims.Size.X, lineY+max(1, gtx.Dp(unit.Dp(1))))}.Op())
			}
			if s.Destination != "" {
				lineY := max(0, textDims.Size.Y-max(1, gtx.Dp(unit.Dp(1))))
				paint.FillShape(gtx.Ops, strokeColor, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(textDims.Size.X, lineY+max(1, gtx.Dp(unit.Dp(1))))}.Op())
			}
			textOffset.Pop()
			push.Pop()

			x += totalW
			maxX = max(maxX, x)
			rowHeight = max(rowHeight, totalH)
		}

		words := markdownWrapTokens(text)
		var currentRun strings.Builder
		for _, word := range words {
			if word == "\n" {
				flushSegment(currentRun.String())
				currentRun.Reset()
				y += rowHeight
				x = 0
				rowHeight = defaultRowHeight
				continue
			}
			candidate := currentRun.String() + word
			candDims, _, _ := measureSegment(candidate)
			candW := candDims.Size.X + 2*padX
			if x+candW <= width || (x == 0 && currentRun.Len() == 0) {
				currentRun.WriteString(word)
				continue
			}
			if currentRun.Len() > 0 {
				flushSegment(currentRun.String())
				currentRun.Reset()
			}
			if x > 0 {
				y += rowHeight
				x = 0
				rowHeight = defaultRowHeight
			}
			currentRun.WriteString(word)
		}
		flushSegment(currentRun.String())
	}
	bodyCall := bodyRecord.Stop()
	finalSize := image.Pt(min(width, max(maxX, gtx.Constraints.Min.X)), y+rowHeight)
	if strings.TrimSpace(semanticText) != "" {
		area := clip.Rect{Max: finalSize}.Push(gtx.Ops)
		semantic.LabelOp(semanticText).Add(gtx.Ops)
		bodyCall.Add(gtx.Ops)
		area.Pop()
	} else {
		bodyCall.Add(gtx.Ops)
	}
	return layout.Dimensions{Size: finalSize}
}
