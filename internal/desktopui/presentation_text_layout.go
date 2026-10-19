package desktopui

import (
	"glance/internal/document"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"
)

func presentationCanvasFontSize(gtx layout.Context, sizePT, scale float64) unit.Sp {
	// scale already converts slide points to physical pixels. Convert back to
	// Gio scalable points so the display pixel density is not applied twice.
	pxPerSp := float64(gtx.Metric.PxPerSp)
	if pxPerSp <= 0 {
		pxPerSp = 1
	}
	return unit.Sp(float32(sizePT * scale / pxPerSp))
}

func presentationElementFontSize(elem document.PresentationElement) float64 {
	for _, paragraph := range elem.Paragraphs {
		for _, run := range paragraph.Runs {
			if strings.TrimSpace(run.Text) != "" && run.SizePT > 0 {
				return run.SizePT
			}
		}
	}
	switch elem.PlaceholderType {
	case "title", "ctrTitle":
		return 44
	case "subTitle":
		return 32
	case "body", "obj", "content":
		return 24
	}
	return 14
}

func presentationElementEditorColor(elem document.PresentationElement) color.NRGBA {
	for _, paragraph := range elem.Paragraphs {
		for _, run := range paragraph.Runs {
			if strings.TrimSpace(run.Text) != "" && run.ColorHex != "" {
				return parsePresentationHexColor(run.ColorHex, pptxTextDark)
			}
		}
	}
	return pptxTextDark
}

func presentationElementTextAlignment(elem document.PresentationElement) text.Alignment {
	for _, paragraph := range presentationElementDisplayParagraphs(elem) {
		if paragraph.Align != "" {
			return presentationParagraphTextAlignment(paragraph.Align)
		}
	}
	// Read-only rendering defaults paragraphs with no explicit alignment to
	// left. Keep the editor on that same effective alignment; placeholder type
	// alone must not override the text that is already visible on the slide.
	return text.Start
}

func presentationElementDisplayParagraphs(elem document.PresentationElement) []document.PresentationParagraph {
	if elem.PlaceholderType == "" || elem.PlainText() != "" {
		return elem.Paragraphs
	}
	label := presentationPlaceholderPrompt(elem.PlaceholderType)
	if label == "" {
		return elem.Paragraphs
	}
	var sourceSpacing document.PresentationParagraph
	if len(elem.Paragraphs) > 0 {
		sourceSpacing = elem.Paragraphs[0]
	}
	colorHex := "#888888"
	align := "left"
	switch elem.PlaceholderType {
	case "title", "ctrTitle":
		colorHex = "#111111"
		align = "center"
	case "subTitle":
		label = "单击此处添加副标题"
		align = "center"
	}
	return []document.PresentationParagraph{{
		Align:       align,
		LineSpacing: sourceSpacing.LineSpacing,
		SpaceBefore: sourceSpacing.SpaceBefore,
		SpaceAfter:  sourceSpacing.SpaceAfter,
		Runs: []document.PresentationTextRun{{
			Text: label, SizePT: presentationElementFontSize(elem), ColorHex: colorHex,
		}},
	}}
}

func (a *App) presentationElementDisplayParagraphs(elem document.PresentationElement) []document.PresentationParagraph {
	paragraphs := presentationElementDisplayParagraphs(elem)
	if elem.PlainText() != "" || len(paragraphs) != 1 || len(paragraphs[0].Runs) != 1 {
		return paragraphs
	}
	messageID := ""
	switch elem.PlaceholderType {
	case "title", "ctrTitle":
		messageID = "presentation.placeholder.title"
	case "subTitle":
		messageID = "presentation.placeholder.subtitle"
	case "body", "obj", "content", "vert", "vert270", "vertTitle", "vertBody":
		messageID = "presentation.placeholder.body"
	}
	if messageID != "" {
		paragraphs[0].Runs[0].Text = a.textID(messageID)
	}
	return paragraphs
}

func presentationElementHasPlaceholderPrompt(elem document.PresentationElement) bool {
	return elem.PlainText() == "" && presentationPlaceholderPrompt(elem.PlaceholderType) != ""
}

func presentationPlaceholderPrompt(placeholderType string) string {
	switch placeholderType {
	case "title", "ctrTitle":
		return "单击此处添加标题"
	case "subTitle":
		return "单击此处添加副标题"
	case "body", "obj", "content", "vert", "vert270", "vertTitle", "vertBody":
		return "单击此处添加文本"
	default:
		return ""
	}
}

func presentationParagraphTextAlignment(align string) text.Alignment {
	switch align {
	case "center":
		return text.Middle
	case "right":
		return text.End
	default:
		return text.Start
	}
}

func presentationEditorOrigin(elemX, elemY int) image.Point {
	return image.Pt(elemX, elemY)
}

func presentationElementTextMaxSize(elemW int, elem document.PresentationElement, scale float64) image.Point {
	maxW := 100000
	if elem.WrapText {
		maxW = max(elemW, int(40*scale))
	}
	return image.Pt(max(1, maxW), 100000)
}

func presentationCaretOffsetAt(gtx layout.Context, theme *material.Theme, face font.Font, value string, point image.Point, size unit.Sp, minWidth, maxWidth int, alignment text.Alignment, metrics presentationParagraphLayoutMetrics, scale float64) int {
	if theme == nil || theme.Shaper == nil || value == "" {
		return 0
	}
	if maxWidth < minWidth {
		maxWidth = minWidth
	}
	theme.Shaper.LayoutString(text.Parameters{
		Font:            face,
		Alignment:       alignment,
		PxPerEm:         fixed.I(gtx.Sp(size)),
		MinWidth:        minWidth,
		MaxWidth:        maxWidth,
		Locale:          gtx.Locale,
		WrapPolicy:      text.WrapHeuristically,
		LineHeight:      fixed.I(gtx.Sp(presentationParagraphLineHeight(gtx, metrics, scale))),
		LineHeightScale: 1,
	}, value)

	bestOffset := 0
	bestScore := float64(1 << 62)
	runeOffset := 0
	clusterMinX, clusterMaxX := 0.0, 0.0
	clusterHasGlyph := false
	clusterY := 0.0
	clusterAscent, clusterDescent := 0.0, 0.0
	for glyph, ok := theme.Shaper.NextGlyph(); ok; glyph, ok = theme.Shaper.NextGlyph() {
		x := float64(glyph.X) / 64
		advance := float64(glyph.Advance) / 64
		left, right := min(x, x+advance), max(x, x+advance)
		if !clusterHasGlyph {
			clusterMinX, clusterMaxX = left, right
			clusterHasGlyph = true
		} else {
			clusterMinX = min(clusterMinX, left)
			clusterMaxX = max(clusterMaxX, right)
		}
		clusterY = float64(glyph.Y)
		clusterAscent = float64(glyph.Ascent) / 64
		clusterDescent = float64(glyph.Descent) / 64
		if glyph.Flags&text.FlagClusterBreak == 0 {
			continue
		}

		clusterRunes := int(glyph.Runes)
		if clusterRunes > 0 {
			distanceX := math.Abs(float64(point.X) - max(clusterMinX, min(clusterMaxX, float64(point.X))))
			lineTop, lineBottom := clusterY-clusterAscent, clusterY+clusterDescent
			distanceY := math.Abs(float64(point.Y) - max(lineTop, min(lineBottom, float64(point.Y))))
			score := distanceY*4 + distanceX
			if score < bestScore {
				bestScore = score
				bestOffset = runeOffset
				if float64(point.X) > (clusterMinX+clusterMaxX)/2 {
					bestOffset += clusterRunes
				}
			}
			runeOffset += clusterRunes
		}
		clusterHasGlyph = false
	}
	return min(max(0, bestOffset), len([]rune(value)))
}

func presentationElementEditorFont(elem document.PresentationElement) font.Font {
	families := make([]string, 0)
	seenFamilies := make(map[string]struct{})
	var style font.Font
	styleSet := false
	for _, paragraph := range elem.Paragraphs {
		for _, run := range paragraph.Runs {
			family := strings.TrimSpace(run.Font)
			if family != "" {
				if _, exists := seenFamilies[family]; !exists {
					seenFamilies[family] = struct{}{}
					families = append(families, family)
				}
			}
			if !styleSet && strings.TrimSpace(run.Text) != "" {
				style.Weight = font.Normal
				style.Style = font.Regular
				if run.Bold {
					style.Weight = font.Bold
				}
				if run.Italic {
					style.Style = font.Italic
				}
				styleSet = true
			}
		}
	}
	style.Typeface = presentationTextTypeface(strings.Join(families, ", "))
	return style
}

func presentationTextTypeface(documentFamily string) font.Typeface {
	families := []string{
		strings.TrimSpace(documentFamily),
		"Cambria Math", "STIX Two Math", "STIXGeneral",
		"Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji",
		string(preferredSystemTypeface),
	}
	ordered := make([]string, 0, len(families))
	seen := make(map[string]struct{}, len(families))
	for _, family := range families {
		for _, candidate := range strings.Split(family, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate != "" {
				if _, exists := seen[candidate]; exists {
					continue
				}
				seen[candidate] = struct{}{}
				ordered = append(ordered, candidate)
			}
		}
	}
	return font.Typeface(strings.Join(ordered, ", "))
}

// parsePresentationHexColor parses "#RRGGBB", "RRGGBB", or "#RGB" hex color string.
func parsePresentationHexColor(hex string, def color.NRGBA) color.NRGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		r, _ := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
		g, _ := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
		b, _ := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	}
	if len(hex) == 6 {
		r, _ := strconv.ParseUint(hex[0:2], 16, 8)
		g, _ := strconv.ParseUint(hex[2:4], 16, 8)
		b, _ := strconv.ParseUint(hex[4:6], 16, 8)
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	}
	if len(hex) == 8 {
		r, _ := strconv.ParseUint(hex[0:2], 16, 8)
		g, _ := strconv.ParseUint(hex[2:4], 16, 8)
		b, _ := strconv.ParseUint(hex[4:6], 16, 8)
		a, _ := strconv.ParseUint(hex[6:8], 16, 8)
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
	}
	return def
}
