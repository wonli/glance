package wordlayout

import (
	"context"
	"fmt"
	"math"
	"sort"
	"unicode"

	"glance/internal/document"
)

type paragraphAtomKind uint8

const (
	atomText paragraphAtomKind = iota + 1
	atomImage
	atomPageBreak
)

type paragraphAtom struct {
	kind          paragraphAtomKind
	sourceID      string
	runID         string
	text          string
	runeStart     int
	runeEnd       int
	style         document.WordTextStyle
	glyphs        []ShapedGlyph
	outlines      [][]GlyphPathCommand
	advance       float64
	ascent        float64
	descent       float64
	imageID       string
	imageRotation float64
	imageWidth    float64
	imageHeight   float64
	synthetic     bool
	breakBefore   bool
	breakAfter    bool
}

type paragraphLine struct {
	atoms           []paragraphAtom
	width           float64
	ascent          float64
	descent         float64
	height          float64
	pageBreakBefore bool
	pageBreakAfter  bool
}

type preparedParagraph struct {
	sourceID       string
	borderSourceID string
	uiBeforePT     float64
	uiAfterPT      float64
	style          resolvedParagraphStyle
	lines          []paragraphLine
}

func (p preparedParagraph) totalHeight() float64 {
	height := p.uiBeforePT + p.uiAfterPT + p.style.paragraph.BeforePT + p.style.paragraph.AfterPT + paragraphBottomBorderExtent(p.style.paragraph)
	for _, line := range p.lines {
		height += line.height
	}
	return height
}

func paragraphBottomBorderExtent(paragraph document.WordParagraphLayout) float64 {
	border := paragraph.BottomBorder
	if border.Style == "none" || border.Style == "nil" || border.SizePT <= 0 {
		return 0
	}
	return border.SpacePT + border.SizePT
}

func prepareParagraph(ctx context.Context, shaper TextShaper, styles *styleResolver, paragraph document.WordParagraph, sourceID string, availableWidth float64, inCell, editorInsets bool) (preparedParagraph, error) {
	prepared := preparedParagraph{sourceID: sourceID, borderSourceID: sourceID, style: styles.paragraphStyle(paragraph, sourceID)}
	if editorInsets {
		prepared.uiBeforePT, prepared.uiAfterPT = 3.75, 4.5
		if inCell {
			prepared.uiAfterPT = 2.25
		}
	}
	leftIndent := emuToPoints(prepared.style.paragraph.LeftIndentEMU)
	rightIndent := emuToPoints(prepared.style.paragraph.RightIndentEMU)
	firstOffset := emuToPoints(prepared.style.paragraph.FirstLineIndentEMU - prepared.style.paragraph.HangingIndentEMU)
	imageWidth := availableWidth - leftIndent - rightIndent - math.Max(0, firstOffset)
	if imageWidth <= 0 {
		imageWidth = 1
	}
	atoms := make([]paragraphAtom, 0, len(paragraph.Inlines)+1)
	if paragraph.ListKind == document.WordListUnsupported {
		styles.cursor.diagnostic(DiagnosticError, sourceID, "unsupported-list", "This paragraph uses a list definition that is outside the supported numbering subset.")
	} else if paragraph.ListKind != document.WordListNone && paragraph.ListMarker != "" {
		marker := paragraph.ListMarker + "\t"
		markerAtoms, err := shapeTextAtoms(ctx, shaper, styles.cursor.glyphPaths, paragraph.ID+".list-marker", marker, 0, prepared.style.text, true, sourceID)
		if err != nil {
			return preparedParagraph{}, err
		}
		atoms = append(atoms, markerAtoms...)
	}
	for _, inline := range paragraph.Inlines {
		if err := ctx.Err(); err != nil {
			return preparedParagraph{}, err
		}
		switch inline.Kind {
		case document.WordTextInline:
			if inline.Text == nil {
				styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-text-inline", "A text inline has no text run data.")
				continue
			}
			run := inline.Text
			runStyle := prepared.style.text
			styles.applyCharacterStyle(&runStyle, run.CharacterStyle, run.ID)
			mergeTextStyle(&runStyle, run.Style, run.StyleSet)
			if runStyle.FontSize <= 0 {
				runStyle.FontSize = 11
			}
			textAtoms, err := shapeTextAtoms(ctx, shaper, styles.cursor.glyphPaths, run.ID, run.Text, 0, runStyle, false, sourceID)
			if err != nil {
				styles.cursor.diagnostic(DiagnosticError, run.ID, "text-shaping", err.Error())
				continue
			}
			atoms = append(atoms, textAtoms...)
		case document.WordImageInline:
			if inline.Image == nil {
				styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-image-inline", "An image inline has no image data.")
				continue
			}
			image := inline.Image
			if !image.Resolved {
				styles.cursor.diagnostic(DiagnosticError, image.ID, "unresolved-image", "The inline image bytes are unavailable, so the page cannot be rendered faithfully.")
				continue
			}
			width := emuToPoints(image.WidthEMU)
			height := emuToPoints(image.HeightEMU)
			if width <= 0 || height <= 0 || !finite(width) || !finite(height) {
				styles.cursor.diagnostic(DiagnosticError, image.ID, "invalid-image-size", "The inline image has missing or invalid dimensions.")
				continue
			}
			if width > imageWidth && imageWidth > 0 {
				ratio := imageWidth / width
				width *= ratio
				height *= ratio
				styles.cursor.diagnostic(DiagnosticWarning, image.ID, "image-scaled-to-page", "The inline image was scaled down to fit the printable text width.")
			}
			atoms = append(atoms, paragraphAtom{kind: atomImage, sourceID: image.ID, imageID: image.ID, imageRotation: image.RotationDegrees, imageWidth: width, imageHeight: height, advance: width, ascent: height, breakBefore: true, breakAfter: true})
		case document.WordPageBreakInline:
			if inline.PageBreak == nil {
				styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-page-break", "A page break inline has no source identity.")
				continue
			}
			atoms = append(atoms, paragraphAtom{kind: atomPageBreak, sourceID: inline.PageBreak.ID})
		case document.WordUnsupportedInline:
			source := sourceID
			detail := "This paragraph contains unsupported inline content."
			if inline.Unsupported != nil {
				if inline.Unsupported.ID != "" {
					source = inline.Unsupported.ID
				}
				if inline.Unsupported.Description != "" {
					detail = inline.Unsupported.Description
				}
			}
			styles.cursor.diagnostic(DiagnosticError, source, "unsupported-inline", detail)
		default:
			styles.cursor.diagnostic(DiagnosticError, sourceID, "unknown-inline-kind", fmt.Sprintf("Unsupported Word inline kind %d.", inline.Kind))
		}
	}
	if len(atoms) == 0 {
		// Empty paragraphs still occupy one line, as in a Word document.
		prepared.lines = []paragraphLine{{ascent: prepared.style.text.FontSize * 0.8, descent: prepared.style.text.FontSize * 0.2}}
		prepared.lines[0].height = lineHeight(prepared.lines[0], prepared.style.paragraph)
		return prepared, nil
	}
	maxWidth := availableWidth - leftIndent - rightIndent
	if firstOffset > 0 {
		maxWidth -= firstOffset
	}
	if maxWidth <= 0 {
		styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-paragraph-width", "Paragraph indents leave no printable line width.")
		maxWidth = 1
	}
	prepared.lines = wrapParagraphAtoms(atoms, maxWidth, prepared.style.paragraph)
	if len(prepared.lines) == 0 {
		prepared.lines = []paragraphLine{{ascent: prepared.style.text.FontSize * 0.8, descent: prepared.style.text.FontSize * 0.2}}
		prepared.lines[0].height = lineHeight(prepared.lines[0], prepared.style.paragraph)
		for _, atom := range atoms {
			if atom.kind == atomPageBreak {
				prepared.lines[0].pageBreakAfter = true
				break
			}
		}
	}
	return prepared, nil
}

func shapeTextAtoms(ctx context.Context, shaper TextShaper, pathCache map[glyphPathCacheKey][]GlyphPathCommand, runID, text string, runeOffset int, style document.WordTextStyle, synthetic bool, sourceID string) ([]paragraphAtom, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runes := []rune(text)
	for _, r := range runes {
		if isRTL(r) {
			return nil, fmt.Errorf("right-to-left text shaping is not supported in this Word layout slice")
		}
	}
	shaped, err := shaper.Shape(text, style)
	if err != nil {
		return nil, err
	}
	if err := ValidateShapedText(text, shaped); err != nil {
		return nil, err
	}
	if len(runes) == 0 {
		size := style.FontSize
		if size <= 0 {
			size = 11
		}
		return []paragraphAtom{{kind: atomText, sourceID: sourceID, runID: runID, style: style, runeStart: runeOffset, runeEnd: runeOffset, ascent: size * 0.8, descent: size * 0.2}}, nil
	}
	type cluster struct {
		start  int
		end    int
		glyphs []ShapedGlyph
	}
	clustersByRange := make(map[[2]int]*cluster)
	clusters := make([]*cluster, 0, len(shaped.Glyphs))
	for _, glyph := range shaped.Glyphs {
		key := [2]int{glyph.RuneStart, glyph.RuneEnd}
		current := clustersByRange[key]
		if current == nil {
			current = &cluster{start: glyph.RuneStart, end: glyph.RuneEnd}
			clustersByRange[key] = current
			clusters = append(clusters, current)
		}
		current.glyphs = append(current.glyphs, glyph)
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].start == clusters[j].start {
			return clusters[i].end < clusters[j].end
		}
		return clusters[i].start < clusters[j].start
	})
	atoms := make([]paragraphAtom, 0, len(clusters))
	for _, current := range clusters {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		advance := 0.0
		ascent, descent := shaped.AscentPT, shaped.DescentPT
		outlines := make([][]GlyphPathCommand, len(current.glyphs))
		for _, glyph := range current.glyphs {
			advance += glyph.AdvanceXPT
		}
		clusterText := string(runes[current.start:current.end])
		blankCluster := true
		for _, r := range clusterText {
			if !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r) {
				blankCluster = false
				break
			}
		}
		if !blankCluster {
			outliner, ok := shaper.(GlyphOutlineSource)
			if !ok {
				return nil, fmt.Errorf("text shaper cannot resolve vector outlines for visible glyphs")
			}
			for index, glyph := range current.glyphs {
				key := glyphPathCacheKey{fontKey: glyph.FontKey, glyphID: glyph.GlyphID, fontSize: math.Float64bits(style.FontSize)}
				path, cached := pathCache[key]
				if !cached {
					outline, err := outliner.GlyphOutline(glyph.FontKey, glyph.GlyphID)
					if err != nil {
						return nil, err
					}
					path = scaleGlyphOutline(outline, style.FontSize)
					pathCache[key] = path
				}
				outlines[index] = path
			}
		}
		first, last := runes[current.start], runes[current.end-1]
		atoms = append(atoms, paragraphAtom{
			kind: atomText, sourceID: sourceID, runID: runID, text: clusterText,
			runeStart: runeOffset + current.start, runeEnd: runeOffset + current.end,
			style: style, glyphs: append([]ShapedGlyph(nil), current.glyphs...), outlines: outlines, advance: advance,
			ascent: ascent, descent: descent, synthetic: synthetic,
			breakBefore: isCJK(first), breakAfter: unicode.IsSpace(last),
		})
	}
	return atoms, nil
}

func preparedHasPageBreak(prepared preparedParagraph) bool {
	if prepared.style.paragraph.PageBreakBefore {
		return true
	}
	for _, line := range prepared.lines {
		if line.pageBreakBefore || line.pageBreakAfter {
			return true
		}
	}
	return false
}

func wrapParagraphAtoms(atoms []paragraphAtom, maxWidth float64, paragraph document.WordParagraphLayout) []paragraphLine {
	lines := make([]paragraphLine, 0, 2)
	current := make([]paragraphAtom, 0, 8)
	pendingPageBreak := false
	flush := func(pageBreakAfter bool) {
		if len(current) == 0 {
			return
		}
		line := paragraphLine{atoms: append([]paragraphAtom(nil), current...), pageBreakBefore: pendingPageBreak, pageBreakAfter: pageBreakAfter}
		pendingPageBreak = false
		for _, atom := range current {
			line.width += atom.advance
			if atom.ascent > line.ascent {
				line.ascent = atom.ascent
			}
			if atom.descent > line.descent {
				line.descent = atom.descent
			}
		}
		line.height = lineHeight(line, paragraph)
		lines = append(lines, line)
		current = current[:0]
	}
	for _, atom := range atoms {
		if atom.kind == atomPageBreak {
			if len(current) > 0 {
				flush(true)
			} else if len(lines) > 0 {
				lines[len(lines)-1].pageBreakAfter = true
			} else {
				pendingPageBreak = true
			}
			continue
		}
		if len(current) > 0 && sumAdvance(current)+atom.advance > maxWidth+epsilonPT {
			if atom.breakBefore {
				flush(false)
			} else {
				breakAt := -1
				for index := len(current) - 1; index >= 0; index-- {
					if current[index].breakAfter {
						breakAt = index + 1
						break
					}
				}
				if breakAt > 0 && breakAt < len(current) {
					carry := append([]paragraphAtom(nil), current[breakAt:]...)
					current = current[:breakAt]
					flush(false)
					current = append(current, carry...)
				} else {
					flush(false)
				}
			}
		}
		current = append(current, atom)
	}
	flush(false)
	if pendingPageBreak && len(lines) > 0 {
		lines[len(lines)-1].pageBreakAfter = true
	}
	return lines
}

func sumAdvance(atoms []paragraphAtom) float64 {
	width := 0.0
	for _, atom := range atoms {
		width += atom.advance
	}
	return width
}

func lineHeight(line paragraphLine, paragraph document.WordParagraphLayout) float64 {
	base := line.ascent + line.descent
	if base <= 0 {
		base = 11
	}
	switch paragraph.LineSpacingRule {
	case "exact":
		if paragraph.LineSpacingPT > 0 {
			return paragraph.LineSpacingPT
		}
	case "atleast":
		return math.Max(base, paragraph.LineSpacingPT)
	case "auto", "":
		if paragraph.LineSpacingMultiple > 0 {
			return base * paragraph.LineSpacingMultiple
		}
	default:
		if paragraph.LineSpacingPT > 0 {
			return math.Max(base, paragraph.LineSpacingPT)
		}
	}
	return base
}

func (c *layoutCursor) placePreparedParagraph(ctx context.Context, prepared preparedParagraph, area Rect, allowPageFlow bool, fixedPageIndex int, fixedY float64) error {
	if len(prepared.lines) == 0 {
		return nil
	}
	if prepared.style.paragraph.PageBreakBefore {
		c.pendingBreak = true
	}
	firstHeight := prepared.style.paragraph.BeforePT + prepared.uiBeforePT + prepared.lines[0].height
	if len(prepared.lines) == 1 {
		firstHeight += prepared.uiAfterPT + paragraphBottomBorderExtent(prepared.style.paragraph)
	}
	totalHeight := prepared.totalHeight()
	keepTogether := prepared.style.paragraph.KeepTogether || prepared.style.paragraph.KeepWithNext
	if allowPageFlow && keepTogether && totalHeight <= c.content.HeightPT+epsilonPT && c.used && c.y+totalHeight > c.content.YPT+c.content.HeightPT+epsilonPT {
		c.nextPage()
	}
	if allowPageFlow {
		if !c.ensureSpace(firstHeight, prepared.sourceID) {
			return nil
		}
		c.y += prepared.style.paragraph.BeforePT + prepared.uiBeforePT
		fixedY = c.y
		fixedPageIndex = c.pageIndex
	} else {
		fixedY += prepared.style.paragraph.BeforePT + prepared.uiBeforePT
	}
	y := fixedY
	for lineIndex, line := range prepared.lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		if line.pageBreakBefore && allowPageFlow {
			c.pendingBreak = true
		}
		if allowPageFlow {
			lineHeightWithBorder := line.height
			if lineIndex == len(prepared.lines)-1 {
				lineHeightWithBorder += prepared.uiAfterPT + paragraphBottomBorderExtent(prepared.style.paragraph)
			}
			if !c.ensureSpace(lineHeightWithBorder, prepared.sourceID) {
				return nil
			}
			y, fixedPageIndex = c.y, c.pageIndex
		}
		borderSourceID := prepared.borderSourceID
		if borderSourceID == "" {
			borderSourceID = prepared.sourceID
		}
		placeParagraphLine(c, prepared.sourceID, borderSourceID, fixedPageIndex, y, area, prepared.style.paragraph, line, lineIndex == 0, lineIndex == len(prepared.lines)-1, prepared.uiAfterPT)
		y += line.height
		if allowPageFlow {
			c.y = y
			c.used = true
			if line.pageBreakAfter {
				c.pendingBreak = true
			}
		}
	}
	if allowPageFlow {
		c.y += prepared.uiAfterPT + paragraphBottomBorderExtent(prepared.style.paragraph)
	} else {
		y += prepared.uiAfterPT + paragraphBottomBorderExtent(prepared.style.paragraph)
	}
	if allowPageFlow {
		c.y += prepared.style.paragraph.AfterPT
	} else {
		y += prepared.style.paragraph.AfterPT
	}
	return nil
}

func placeParagraphLine(cursor *layoutCursor, sourceID, borderSourceID string, pageIndex int, y float64, area Rect, paragraph document.WordParagraphLayout, line paragraphLine, firstLine, lastLine bool, uiAfterPT float64) {
	if pageIndex < 0 || pageIndex >= len(cursor.result.Pages) {
		return
	}
	left := emuToPoints(paragraph.LeftIndentEMU)
	right := emuToPoints(paragraph.RightIndentEMU)
	firstOffset := 0.0
	if firstLine {
		firstOffset = emuToPoints(paragraph.FirstLineIndentEMU - paragraph.HangingIndentEMU)
	}
	available := math.Max(0, area.WidthPT-left-right-firstOffset)
	if line.width > available+epsilonPT {
		cursor.diagnostic(DiagnosticError, sourceID, "paragraph-line-overflow", "A shaped text or inline object exceeds the available paragraph line width.")
	}
	startX := area.XPT + left + firstOffset
	switch paragraph.Alignment {
	case document.WordAlignCenter:
		startX += math.Max(0, (available-line.width)/2)
	case document.WordAlignRight:
		startX += math.Max(0, available-line.width)
	}
	spaceCount := 0
	if paragraph.Alignment == document.WordAlignJustified && !lastLine {
		for _, atom := range line.atoms {
			if atom.breakAfter {
				spaceCount++
			}
		}
	}
	extraPerSpace := 0.0
	if spaceCount > 0 && line.width < available {
		extraPerSpace = (available - line.width) / float64(spaceCount)
	}
	baseline := y + line.ascent
	x := startX
	for _, atom := range line.atoms {
		switch atom.kind {
		case atomText:
			fragment := TextFragment{
				RunID: atom.runID, Text: atom.text, RuneStart: atom.runeStart, RuneEnd: atom.runeEnd,
				Style: atom.style, XPT: x, BaselineYPT: baseline, Synthetic: atom.synthetic,
			}
			glyphPen := 0.0
			for glyphIndex, glyph := range atom.glyphs {
				fragment.Glyphs = append(fragment.Glyphs, PositionedGlyph{
					FontKey: glyph.FontKey, GlyphID: glyph.GlyphID, RuneStart: glyph.RuneStart + atom.runeStart,
					RuneEnd: glyph.RuneEnd + atom.runeStart, XPT: x + glyphPen + glyph.OffsetXPT,
					YPT: baseline - glyph.OffsetYPT, AdvanceXPT: glyph.AdvanceXPT,
					Outline: atom.outlines[glyphIndex],
				})
				glyphPen += glyph.AdvanceXPT
			}
			cursor.result.Pages[pageIndex].Text = append(cursor.result.Pages[pageIndex].Text, fragment)
			if !atom.synthetic && atom.runID != "" {
				for offset := atom.runeStart; offset < atom.runeEnd; offset++ {
					cursor.result.Locations[SourcePosition{RunID: atom.runID, RuneOffset: offset}] = Position{PageIndex: pageIndex, XPT: x, YPT: baseline}
				}
				cursor.result.Locations[SourcePosition{RunID: atom.runID, RuneOffset: atom.runeEnd}] = Position{PageIndex: pageIndex, XPT: x + atom.advance, YPT: baseline}
			}
		case atomImage:
			bounds := Rect{XPT: x, YPT: baseline - atom.imageHeight, WidthPT: atom.imageWidth, HeightPT: atom.imageHeight}
			cursor.result.Pages[pageIndex].Images = append(cursor.result.Pages[pageIndex].Images, ImagePlacement{ImageID: atom.imageID, Bounds: bounds, RotationDegrees: atom.imageRotation})
		}
		x += atom.advance
		if atom.breakAfter {
			x += extraPerSpace
		}
	}
	if lastLine {
		border := paragraph.BottomBorder
		style := border.Style
		if style == "" {
			style = "single"
		}
		if border.SizePT > 0 && style != "none" && style != "nil" {
			cursor.result.Pages[pageIndex].ParagraphBorders = append(cursor.result.Pages[pageIndex].ParagraphBorders, ParagraphBorderPlacement{
				SourceID: borderSourceID,
				Style:    style,
				Color:    border.Color,
				Bounds: Rect{
					XPT:      area.XPT + left,
					YPT:      y + line.height + uiAfterPT + border.SpacePT,
					WidthPT:  math.Max(0, area.WidthPT-left-right),
					HeightPT: border.SizePT,
				},
			})
		}
	}
}

func scaleGlyphOutline(outline GlyphOutline, fontSize float64) []GlyphPathCommand {
	if outline.UnitsPerEm <= 0 || fontSize <= 0 || len(outline.Commands) == 0 {
		return nil
	}
	scale := fontSize / outline.UnitsPerEm
	point := func(input GlyphOutlinePoint) GlyphPathPoint {
		return GlyphPathPoint{XPT: input.X * scale, YPT: -input.Y * scale}
	}
	commands := make([]GlyphPathCommand, 0, len(outline.Commands))
	for _, command := range outline.Commands {
		switch command.Op {
		case GlyphPathMoveTo:
			p := point(command.Points[0])
			commands = append(commands, GlyphPathCommand{Op: GlyphPathMoveTo, Points: [3]GlyphPathPoint{p}})
		case GlyphPathLineTo:
			p := point(command.Points[0])
			commands = append(commands, GlyphPathCommand{Op: GlyphPathLineTo, Points: [3]GlyphPathPoint{p}})
		case GlyphPathCurveTo:
			points := [3]GlyphPathPoint{point(command.Points[0]), point(command.Points[1]), point(command.Points[2])}
			commands = append(commands, GlyphPathCommand{Op: GlyphPathCurveTo, Points: points})
		}
	}
	return commands
}

func emuToPoints(value int64) float64 { return float64(value) / emuPerPoint }

func isCJK(r rune) bool {
	return (r >= 0x2e80 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0x20000 && r <= 0x3134f)
}

func isRTL(r rune) bool {
	return (r >= 0x0590 && r <= 0x08ff) || (r >= 0xfb1d && r <= 0xfdff) || (r >= 0xfe70 && r <= 0xfeff)
}
