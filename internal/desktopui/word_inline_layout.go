package desktopui

import (
	"encoding/hex"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"

	"glance/internal/document"
	"glance/internal/wordfont"
	"glance/internal/wordlayout"
)

type wordInlineLayoutChild struct {
	width   int
	runID   string
	overlay *wordInlineLayoutOverlay
	layout  layout.Widget
}

type wordInlineLayoutOverlay struct {
	call  op.CallOp
	ready bool
}

func wordInlineGeometry(maxWidth int, sizes []image.Point, alignment document.WordAlignment) ([]image.Point, int) {
	if len(sizes) == 0 || maxWidth <= 0 {
		return make([]image.Point, len(sizes)), 0
	}
	positions := make([]image.Point, len(sizes))
	lineWidths := []int{0}
	lineHeights := []int{0}
	lineForItem := make([]int, len(sizes))
	line := 0
	for index, size := range sizes {
		width := min(maxWidth, max(0, size.X))
		if lineWidths[line] > 0 && lineWidths[line]+width > maxWidth {
			line++
			lineWidths = append(lineWidths, 0)
			lineHeights = append(lineHeights, 0)
		}
		positions[index].X = lineWidths[line]
		lineWidths[line] += width
		lineHeights[line] = max(lineHeights[line], max(0, size.Y))
		lineForItem[index] = line
	}
	lineTops := make([]int, len(lineHeights))
	for index := 1; index < len(lineTops); index++ {
		lineTops[index] = lineTops[index-1] + lineHeights[index-1]
	}
	for index, line := range lineForItem {
		positions[index].Y = lineTops[line]
		alignOffset := 0
		switch alignment {
		case document.WordAlignCenter:
			alignOffset = (maxWidth - lineWidths[line]) / 2
		case document.WordAlignRight:
			alignOffset = maxWidth - lineWidths[line]
		}
		positions[index].X += max(0, alignOffset)
	}
	totalHeight := lineTops[len(lineTops)-1] + lineHeights[len(lineHeights)-1]
	return positions, totalHeight
}

func layoutWordInlineFlow(gtx layout.Context, state *documentSessionState, alignment document.WordAlignment, children []wordInlineLayoutChild) layout.Dimensions {
	maxWidth := max(1, gtx.Constraints.Max.X)
	type measuredChild struct {
		call  op.CallOp
		size  image.Point
		runID string
	}
	measured := make([]measuredChild, len(children))
	sizes := make([]image.Point, len(children))
	for index, child := range children {
		width := min(maxWidth, max(1, child.width))
		childGtx := gtx
		childGtx.Constraints.Min.X = width
		childGtx.Constraints.Max.X = width
		recorder := op.Record(gtx.Ops)
		dims := child.layout(childGtx)
		call := recorder.Stop()
		measured[index] = measuredChild{call: call, size: dims.Size, runID: child.runID}
		sizes[index] = dims.Size
	}
	positions, height := wordInlineGeometry(maxWidth, sizes, alignment)
	for index, child := range measured {
		if child.runID != "" {
			if state.wordNavLayoutsNext == nil {
				state.wordNavLayoutsNext = make(map[string]wordNavigationGeometry)
			}
			state.wordNavLayoutsNext[child.runID] = wordNavigationGeometry{x: positions[index].X, y: positions[index].Y}
			if state.wordPendingCaretRunID == child.runID && state.wordPendingCaretByX {
				if editor := state.wordRunEditors[child.runID]; editor != nil {
					offset := wordClosestCaretOffsetToX(editor, state.wordPendingCaretX-positions[index].X)
					editor.SetCaret(offset, offset)
				}
				state.wordPendingCaretRunID = ""
				state.wordPendingCaretByX = false
			}
		}
		transform := op.Offset(positions[index]).Push(gtx.Ops)
		child.call.Add(gtx.Ops)
		transform.Pop()
	}
	for index := range measured {
		if children[index].overlay == nil || !children[index].overlay.ready {
			continue
		}
		transform := op.Offset(positions[index]).Push(gtx.Ops)
		children[index].overlay.call.Add(gtx.Ops)
		transform.Pop()
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, height))}
}

func wordRunPreferredWidth(gtx layout.Context, theme *material.Theme, paragraph document.WordParagraph, run document.WordTextRun) int {
	if run.Text == "" {
		return max(1, gtx.Dp(unit.Dp(12)))
	}
	label := wordRunLabel(theme, paragraph, run)
	theme.Shaper.LayoutString(text.Parameters{Font: label.Font, PxPerEm: fixed.I(gtx.Sp(label.TextSize)), MaxWidth: 1 << 20, Locale: gtx.Locale}, run.Text)
	var width fixed.Int26_6
	for glyph, ok := theme.Shaper.NextGlyph(); ok; glyph, ok = theme.Shaper.NextGlyph() {
		right := glyph.X + glyph.Advance
		if right > width {
			width = right
		}
	}
	pixels := (int(width) + 63) / 64
	return max(1, max(pixels, wordRunFallbackWidth(gtx, label.TextSize, run.Text))+gtx.Dp(unit.Dp(4)))
}

func wordRunFallbackWidth(gtx layout.Context, size unit.Sp, value string) int {
	em := max(1, gtx.Sp(size))
	width := 0
	for _, r := range value {
		switch {
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == '\n' || r == '\r':
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r), unicode.Is(unicode.Bopomofo, r),
			r >= 0x3000 && r <= 0x303F, r >= 0xFF01 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6, r >= 0x1F300 && r <= 0x1FAFF:
			width += em
		default:
			width += max(1, int(math.Round(float64(em)*0.55)))
		}
	}
	return width
}

func (a *App) wordImagePreferredWidth(gtx layout.Context, item document.WordImage, maxWidth int) int {
	preview, previewMode := wordImageLayoutPreview(wordImageInteractionFor(a.documentSessionState, item.ID))
	if preview.Active && preview.ImageID == item.ID {
		switch previewMode {
		case wordImageGestureResize:
			item.WidthEMU, item.HeightEMU = preview.WidthEMU, preview.HeightEMU
		case wordImageGestureRotate:
			item.RotationDegrees = preview.RotationDegrees
		}
	}
	if decoded, ok := a.documentSessionState.wordImageCache.Get(item.ID); ok {
		width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
		if item.WidthEMU > 0 && item.HeightEMU > 0 {
			width = wordImageEMUToPixels(item.WidthEMU, gtx.Metric.PxPerDp)
			height = wordImageEMUToPixels(item.HeightEMU, gtx.Metric.PxPerDp)
		}
		source := fitRotatedWordImageSize(width, height, maxWidth, gtx.Dp(unit.Dp(360)), item.RotationDegrees)
		return max(1, rotatedWordImageBounds(source, item.RotationDegrees).X)
	}
	width, height := wordImageEMUToPixels(item.WidthEMU, gtx.Metric.PxPerDp), wordImageEMUToPixels(item.HeightEMU, gtx.Metric.PxPerDp)
	if width <= 0 || height <= 0 {
		return min(maxWidth, gtx.Dp(unit.Dp(240)))
	}
	source := fitRotatedWordImageSize(width, height, maxWidth, gtx.Dp(unit.Dp(360)), item.RotationDegrees)
	return max(1, rotatedWordImageBounds(source, item.RotationDegrees).X)
}

func wordRunLabel(theme *material.Theme, paragraph document.WordParagraph, run document.WordTextRun) material.LabelStyle {
	label := material.Body1(theme, run.Text)
	label.Alignment = wordParagraphAlignment(paragraph.Alignment)
	label.Font.Typeface = wordRunTypeface(run.Style.FontFamily)
	if run.Style.Bold {
		label.Font.Weight = font.Bold
	}
	if run.Style.Italic {
		label.Font.Style |= font.Italic
	}
	fontSizePT := run.Style.FontSize
	if fontSizePT <= 0 || math.IsNaN(fontSizePT) || math.IsInf(fontSizePT, 0) {
		fontSizePT = wordlayout.DefaultFontSizePT
	}
	label.TextSize = unit.Sp(fontSizePT * (96.0 / 72.0))
	if fontColor, ok := parseWordRunColor(run.Style.FontColor); ok {
		label.Color = fontColor
	}
	switch strings.ToLower(paragraph.Style) {
	case "heading1":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(20)
	case "heading2":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(18)
	case "heading3":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(16)
	}
	return label
}

func wordRunTypeface(documentFamily string) font.Typeface {
	families := append(wordfont.SplitFamilies(documentFamily), wordfont.FallbackFamilies()...)
	seen := make(map[string]struct{}, len(families))
	ordered := make([]string, 0, len(families))
	for _, family := range families {
		key := strings.ToLower(strings.TrimSpace(family))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ordered = append(ordered, family)
	}
	return font.Typeface(strings.Join(ordered, ", "))
}

func wordTextHighlightRects(gtx layout.Context, shaper *text.Shaper, label material.LabelStyle, value string) []image.Rectangle {
	width := gtx.Constraints.Max.X
	if shaper == nil || value == "" || width <= 0 {
		return nil
	}
	shaper.LayoutString(text.Parameters{
		Font:             label.Font,
		Alignment:        label.Alignment,
		PxPerEm:          fixed.I(gtx.Sp(label.TextSize)),
		MaxWidth:         width,
		MinWidth:         gtx.Constraints.Min.X,
		WrapPolicy:       label.WrapPolicy,
		LineHeight:       fixed.I(gtx.Sp(label.LineHeight)),
		LineHeightScale:  label.LineHeightScale,
		Locale:           gtx.Locale,
		DisableSpaceTrim: true,
	}, value)
	var rects []image.Rectangle
	lineBounds := image.Rectangle{}
	lineHasGlyph := false
	flushLine := func() {
		if lineHasGlyph && !lineBounds.Empty() {
			rects = append(rects, lineBounds)
		}
		lineBounds = image.Rectangle{}
		lineHasGlyph = false
	}
	for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
		if glyph.Flags&text.FlagParagraphBreak == 0 {
			bounds := image.Rect(glyph.X.Floor(), int(glyph.Y)-glyph.Ascent.Ceil(), (glyph.X + glyph.Advance).Ceil(), int(glyph.Y)+glyph.Descent.Ceil())
			if !bounds.Empty() {
				if !lineHasGlyph {
					lineBounds = bounds
					lineHasGlyph = true
				} else {
					lineBounds = lineBounds.Union(bounds)
				}
			}
		}
		if glyph.Flags&text.FlagLineBreak != 0 || glyph.Flags&text.FlagParagraphBreak != 0 {
			flushLine()
		}
	}
	flushLine()
	return rects
}

func wordHighlightDisplayColor(value string) (color.NRGBA, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "black":
		return color.NRGBA{A: 255}, true
	case "blue":
		return color.NRGBA{B: 255, A: 255}, true
	case "cyan":
		return color.NRGBA{G: 255, B: 255, A: 255}, true
	case "darkblue":
		return color.NRGBA{B: 128, A: 255}, true
	case "darkcyan":
		return color.NRGBA{G: 128, B: 128, A: 255}, true
	case "darkgray":
		return color.NRGBA{R: 128, G: 128, B: 128, A: 255}, true
	case "darkgreen":
		return color.NRGBA{G: 128, A: 255}, true
	case "darkmagenta":
		return color.NRGBA{R: 128, B: 128, A: 255}, true
	case "darkred":
		return color.NRGBA{R: 128, A: 255}, true
	case "darkyellow":
		return color.NRGBA{R: 128, G: 128, A: 255}, true
	case "green":
		return color.NRGBA{G: 255, A: 255}, true
	case "lightgray":
		return color.NRGBA{R: 192, G: 192, B: 192, A: 255}, true
	case "magenta":
		return color.NRGBA{R: 255, B: 255, A: 255}, true
	case "red":
		return color.NRGBA{R: 255, A: 255}, true
	case "white":
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}, true
	case "yellow":
		return color.NRGBA{R: 255, G: 255, A: 255}, true
	default:
		return color.NRGBA{}, false
	}
}

func wordParagraphAlignment(alignment document.WordAlignment) text.Alignment {
	switch alignment {
	case document.WordAlignCenter:
		return text.Middle
	case document.WordAlignRight:
		return text.End
	case document.WordAlignJustified:
		return text.Start
	default:
		return text.Start
	}
}

func parseWordRunColor(value string) (color.NRGBA, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return color.NRGBA{}, false
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: data[0], G: data[1], B: data[2], A: 255}, true
}
