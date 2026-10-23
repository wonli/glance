package desktopui

import (
	"fmt"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/i18n"
	"image"
	"image/color"
	"strings"
	"unicode"
)

func (a *App) layoutMarkdownPreview(gtx layout.Context) layout.Dimensions {
	v := a.ensureMarkdownView()
	return jsonPanel(gtx, contentSurface, true, false, func(gtx layout.Context) layout.Dimensions {
		return markdownColumn(gtx, func(gtx layout.Context) layout.Dimensions {
			if v.err != nil {
				return jsonLabel(gtx, a, a.text(i18n.Message{ID: "markdown.error.parse_failed"}), 14, mutedText, false, false)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if v.pending {
					return layout.Dimensions{}
				}
				count, _ := markdownImageWarnings(v.snapshot, v.images, v.errors, a.text)
				if count == 0 {
					return layout.Dimensions{}
				}
				return layout.Inset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return jsonLabel(gtx, a, a.text(i18n.Message{ID: "markdown.preview.images_unavailable", Args: i18n.Args{"Count": count}}), 14, color.NRGBA{R: 128, G: 91, B: 36, A: 255}, false, false)
				})
			}), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				beforeScroll := v.preview.Position
				v.consumeChartScroll = false
				v.visibleCharts = make(map[string]struct{})
				dims := v.preview.Layout(gtx, len(v.snapshot.Blocks), func(gtx layout.Context, index int) layout.Dimensions {
					b := v.snapshot.Blocks[index]
					return layout.Inset{Bottom: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.layoutMarkdownBlock(gtx, b, index) })
				})
				if v.consumeChartScroll {
					v.preview.Position = beforeScroll
				} else {
					a.closeHiddenMarkdownCharts(v, v.visibleCharts)
				}
				return dims
			}))
		})
	})
}
func (a *App) layoutMarkdownBlock(gtx layout.Context, b document.MarkdownBlock, index int) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	switch b.Kind {
	case "rule":
		return jsonPanel(jsonFixedHeight(gtx, 1), jsonBorderColor, false, false, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
	case "code":
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, jsonSidebarColor, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(4)).Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(14).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if len(b.CodeSpans) > 0 {
					spans := append([]document.MarkdownSpan(nil), b.CodeSpans...)
					for i := range spans {
						if spans[i].CodeColor == "" {
							spans[i].CodeColor = "374151"
						}
					}
					last := len(spans) - 1
					spans[last].Text = strings.TrimSuffix(spans[last].Text, "\n")
					dims := a.layoutMarkdownSpans(gtx, spans, 13, homeText, false, fmt.Sprintf("code-%d", index))
					dims.Size.X = max(dims.Size.X, gtx.Constraints.Min.X)
					return dims
				}
				style := homeLabelStyle(a.theme, strings.TrimSuffix(b.Text, "\n"), 13, homeText, font.Normal)
				style.Font.Typeface = jsonMonoFace
				style.MaxLines = 0
				return style.Layout(gtx)
			})
		})
	case "diagram":
		return a.layoutMarkdownAsset(gtx, markdowndoc.DiagramSpan(b), false)
	case "math":
		return a.layoutMarkdownAsset(gtx, document.MarkdownSpan{Kind: "math", Text: b.Text, SourceStart: b.SourceStart, SourceEnd: b.SourceEnd}, true)
	case "table":
		return a.layoutMarkdownTable(gtx, b, index)

	}
	size := unit.Sp(16)
	fg := homeText
	if b.Kind == "heading" {
		size = unit.Sp(max(17, 32-(b.Level-1)*4))
		fg = jsonBlue
	}
	spans := append([]document.MarkdownSpan{}, b.Spans...)
	if b.Prefix != "" && (len(b.Spans) == 0 || b.Spans[0].Kind != "task") {
		spans = append([]document.MarkdownSpan{{Text: b.Prefix}}, spans...)
	}
	indent := 0
	if b.Kind == "list" || (b.Kind == "paragraph" && b.Level > 0) {
		indent = max(0, b.Level-1) * 24
	}
	if b.Kind == "quote" {
		indent = max(1, b.Level) * 18
		if b.Prefix == "> " {
			spans = b.Spans
		}
	}
	indent = min(indent, max(0, gtx.Constraints.Max.X/2))
	return layout.Inset{Left: unit.Dp(indent)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.layoutMarkdownSpans(gtx, spans, size, fg, b.Kind == "heading", index)
	})
}
func (a *App) layoutMarkdownAsset(gtx layout.Context, s document.MarkdownSpan, display bool) layout.Dimensions {
	v := a.ensureMarkdownView()
	s.DisplayMath = s.DisplayMath || display
	key := markdownAssetKey(s)
	img := v.images[key]
	if img == nil {
		label := s.Text
		if label == "" {
			label = s.Destination
		}
		if v.pending {
			label += "（正在加载…）"
		}
		if err := v.errors[key]; err != "" {
			label = "图片无法加载：" + label + "（" + err + "）"
			if s.Kind == "diagram" {
				label = a.text(i18n.Message{ID: "markdown.diagram.error", Args: i18n.Args{"Reason": err}}) + "\n" + s.Text
			}
			if s.Kind == "math" {
				label = "公式无法渲染：" + s.Text + "（" + err + "）"
			}
		}
		style := homeLabelStyle(a.theme, label, 14, mutedText, font.Normal)
		style.MaxLines = 0
		return a.layoutMarkdownFormulaTarget(gtx, s, style.Layout)
	}
	logical := document.ResourceLogicalSize(img)
	var chart *markdownChartState
	if markdownChartIsVega(s) {
		chart = a.ensureMarkdownChart(s, img)
		if v.visibleCharts == nil {
			v.visibleCharts = make(map[string]struct{})
		}
		v.visibleCharts[chart.key] = struct{}{}
		if current, currentSize, _ := markdownChartSnapshot(chart); current != nil {
			img = current
			logical = currentSize
		}
	}
	width, height := logical.X, logical.Y
	if _, ok := img.(document.ResourceImage); ok {
		width, height = gtx.Dp(unit.Dp(width)), gtx.Dp(unit.Dp(height))
	} else if s.Kind == "math" {
		width, height = gtx.Sp(unit.Sp(width)/2), gtx.Sp(unit.Sp(height)/2)
	} else {
		width, height = gtx.Dp(unit.Dp(width)), gtx.Dp(unit.Dp(height))
	}
	maxWidth := gtx.Constraints.Max.X
	if width > maxWidth && width > 0 {
		height = height * maxWidth / width
		width = maxWidth
	}
	outerConstraints := gtx.Constraints
	chartSize := image.Pt(width, height)
	gtx.Constraints = layout.Exact(chartSize)
	draw := func(gtx layout.Context) layout.Dimensions {
		return widget.Image{Src: paint.NewImageOp(img), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
	}
	if chart != nil {
		chartDraw := func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(chartSize)
			return a.layoutMarkdownChartPointer(gtx, v, chart, logical, draw)
		}
		if _, _, chartErr := markdownChartSnapshot(chart); chartErr != "" {
			gtx.Constraints = outerConstraints
			gtx.Constraints.Min.Y = 0
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(chartDraw),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					message := a.text(i18n.Message{ID: "markdown.diagram.error", Args: i18n.Args{"Reason": chartErr}})
					return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, message, 12, mutedText, false, false)
					})
				}),
			)
		}
		return a.layoutMarkdownFormulaTarget(gtx, s, chartDraw)
	}
	return a.layoutMarkdownFormulaTarget(gtx, s, draw)
}

// Place actual measured text runs into wrapped rows; CJK characters are break
// opportunities and links retain stable clickable state per inline span.
func (a *App) layoutMarkdownSpans(gtx layout.Context, spans []document.MarkdownSpan, size unit.Sp, fg color.NRGBA, bold bool, block any) layout.Dimensions {
	v := a.ensureMarkdownView()
	width := gtx.Constraints.Max.X
	x, y, rowHeight := 0, 0, gtx.Sp(size)*3/2
	maxX := 0
	for index, s := range spans {
		if s.Kind == "task" {
			dimsSize := gtx.Dp(24)
			if x > 0 && x+dimsSize > width {
				y += rowHeight
				x = 0
				rowHeight = gtx.Sp(size) * 3 / 2
			}
			push := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			taskGtx := gtx
			taskGtx.Constraints = layout.Exact(image.Pt(dimsSize, dimsSize))
			a.layoutMarkdownTask(taskGtx, s)
			push.Pop()
			x += dimsSize
			maxX = max(maxX, x)
			rowHeight = max(rowHeight, dimsSize)
			continue
		}

		if s.Kind == "math" || s.Kind == "image" {
			record := op.Record(gtx.Ops)
			assetGtx := gtx
			assetGtx.Constraints = layout.Constraints{Max: image.Pt(max(1, width), 1<<20)}
			dims := a.layoutMarkdownAsset(assetGtx, s, false)
			call := record.Stop()
			if x > 0 && (s.Kind == "image" || x+dims.Size.X > width) {
				y += rowHeight
				x = 0
				rowHeight = gtx.Sp(size) * 3 / 2
			}
			push := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			if s.LinkDestination != "" {
				id := fmt.Sprintf("link-%v-%d-asset", block, index)
				v.links[id] = s.LinkDestination
				clickGtx := gtx
				clickGtx.Constraints = layout.Exact(dims.Size)
				v.button(id).Layout(clickGtx, func(gtx layout.Context) layout.Dimensions { call.Add(gtx.Ops); return dims })
			} else {
				call.Add(gtx.Ops)
			}
			push.Pop()
			maxX = max(maxX, x+dims.Size.X)
			if s.Kind == "image" {
				y += dims.Size.Y + 6
				x = 0
			} else {
				x += dims.Size.X
				rowHeight = max(rowHeight, dims.Size.Y)
			}
			continue
		}
		words := markdownWrapTokens(s.Text)
		for token, word := range words {
			if word == "\n" {
				y += rowHeight
				x = 0
				rowHeight = gtx.Sp(size) * 3 / 2
				continue
			}
			record := op.Record(gtx.Ops)
			measure := gtx
			measure.Constraints = layout.Constraints{Max: image.Pt(max(1, width), 1<<20)}
			style := homeLabelStyle(a.theme, word, size, fg, font.Normal)
			style.MaxLines = 0
			if bold || s.Bold {
				style.Font.Weight = font.SemiBold
			}
			if s.Italic {
				style.Font.Style = font.Italic
			}
			if s.Kind == "code" {
				style.Font.Typeface = jsonMonoFace
				style.Color = jsonKeyColor
				if c, ok := parseWordRunColor(s.CodeColor); ok {
					style.Color = c
				}
			}
			if s.Destination != "" {
				style.Color = jsonBlue
			}
			dims := style.Layout(measure)
			if s.Strike {
				lineY := max(0, dims.Size.Y-dims.Baseline-gtx.Sp(size)*3/10)
				paint.FillShape(gtx.Ops, style.Color, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(dims.Size.X, lineY+max(1, gtx.Dp(1)))}.Op())
			}
			call := record.Stop()
			if x > 0 && x+dims.Size.X > width {
				y += rowHeight
				x = 0
				rowHeight = gtx.Sp(size) * 3 / 2
			}
			push := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			if s.Destination != "" {
				id := fmt.Sprintf("link-%v-%d-%d", block, index, token)
				v.links[id] = s.Destination
				clickGtx := gtx
				clickGtx.Constraints = layout.Exact(dims.Size)
				v.button(id).Layout(clickGtx, func(gtx layout.Context) layout.Dimensions { call.Add(gtx.Ops); return dims })
			} else {
				call.Add(gtx.Ops)
			}
			push.Pop()
			x += dims.Size.X
			maxX = max(maxX, x)
			rowHeight = max(rowHeight, dims.Size.Y)
		}
	}
	return layout.Dimensions{Size: image.Pt(min(width, maxX), y+rowHeight)}
}
func markdownWrapTokens(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	out := []string{}
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			out = append(out, word.String())
			word.Reset()
		}
	}
	for _, r := range value {
		if r == '\n' || unicode.Is(unicode.Han, r) {
			flush()
			out = append(out, string(r))
		} else {
			word.WriteRune(r)
			if unicode.IsSpace(r) {
				flush()
			}
		}
	}
	flush()
	return out
}
