package wordlayout

import (
	"context"
	"fmt"
	"glance/internal/document"
	"math"
	"strings"
)

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

func (c *layoutCursor) placePreparedParagraph(ctx context.Context, prepared preparedParagraph, area Rect, allowPageFlow bool, fixedPageIndex int, fixedY float64) error {
	if allowPageFlow && (len(prepared.floatingImages) > 0 || len(c.floating) > 0) {
		return c.placeFloatAwareParagraph(ctx, prepared, area)
	}
	if len(prepared.lines) == 0 {
		return nil
	}
	if prepared.style.paragraph.PageBreakBefore {
		c.pendingBreak = true
	}
	canCollapseSpacing := allowPageFlow && c.hasPreviousParagraph && !c.pendingBreak && !preparedHasPageBreak(prepared)
	beforePT := prepared.style.paragraph.BeforePT
	if canCollapseSpacing {
		beforePT = paragraphLeadingAdjustment(c.previousParagraphAfterPT, c.previousParagraphLineGapPT, prepared)
	}
	firstHeight := beforePT + prepared.uiBeforePT + prepared.lines[0].height
	if len(prepared.lines) == 1 {
		firstHeight += prepared.uiAfterPT + paragraphBottomBorderExtent(prepared.style.paragraph)
	}
	totalHeight := prepared.totalHeight()
	if canCollapseSpacing {
		totalHeight += beforePT - prepared.style.paragraph.BeforePT
	}
	keepTogether := prepared.style.paragraph.KeepTogether || prepared.style.paragraph.KeepWithNext
	if allowPageFlow && keepTogether && totalHeight <= c.content.HeightPT+epsilonPT && c.used && c.y+totalHeight > c.content.YPT+c.content.HeightPT+epsilonPT {
		c.advanceColumn()
		canCollapseSpacing = false
		beforePT = prepared.style.paragraph.BeforePT
		firstHeight = beforePT + prepared.uiBeforePT + prepared.lines[0].height
		if len(prepared.lines) == 1 {
			firstHeight += prepared.uiAfterPT + paragraphBottomBorderExtent(prepared.style.paragraph)
		}
	}
	if allowPageFlow {
		// Collapsing a large trailing gap may reclaim more space than the
		// next line occupies. Rebase the cursor before checking the line;
		// the line itself must never be passed as a negative-height item.
		if beforePT < 0 {
			c.y += beforePT
			firstHeight -= beforePT
			beforePT = 0
		}
		oldPage, oldColumn := c.pageIndex, c.currentCol
		if !c.ensureSpace(firstHeight, prepared.sourceID) {
			return nil
		}
		if canCollapseSpacing && (c.pageIndex != oldPage || c.currentCol != oldColumn) {
			beforePT = prepared.style.paragraph.BeforePT
			firstHeight = beforePT + prepared.uiBeforePT + prepared.lines[0].height
			if len(prepared.lines) == 1 {
				firstHeight += prepared.uiAfterPT + paragraphBottomBorderExtent(prepared.style.paragraph)
			}
			if !c.ensureSpace(firstHeight, prepared.sourceID) {
				return nil
			}
		}
		c.y += beforePT + prepared.uiBeforePT
		fixedY = c.y
		fixedPageIndex = c.pageIndex
	} else {
		fixedY += beforePT + prepared.uiBeforePT
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
		currentArea := area
		if allowPageFlow {
			currentArea = c.currentColumnRect()
		}
		placeParagraphLine(c, prepared.sourceID, borderSourceID, fixedPageIndex, y, currentArea, prepared.style.paragraph, line, lineIndex == 0, lineIndex == len(prepared.lines)-1)
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
		c.hasPreviousParagraph = !preparedHasPageBreak(prepared)
		c.previousParagraphAfterPT = prepared.style.paragraph.AfterPT
		c.previousParagraphLineGapPT = paragraphLineGap(prepared, true)
	} else {
		y += prepared.style.paragraph.AfterPT
	}
	return nil
}

func (c *layoutCursor) placeFloatAwareParagraph(ctx context.Context, prepared preparedParagraph, area Rect) error {
	if len(prepared.lines) == 0 {
		return nil
	}
	paragraph := prepared.style.paragraph
	if paragraph.PageBreakBefore {
		c.pendingBreak = true
	}
	canCollapseSpacing := c.hasPreviousParagraph && !c.pendingBreak && !preparedHasPageBreak(prepared)
	beforePT := paragraph.BeforePT
	if canCollapseSpacing {
		beforePT = paragraphLeadingAdjustment(c.previousParagraphAfterPT, c.previousParagraphLineGapPT, prepared)
	}
	firstHeight := beforePT + prepared.uiBeforePT + prepared.lines[0].height
	if len(prepared.lines) == 1 {
		firstHeight += prepared.uiAfterPT + paragraphBottomBorderExtent(paragraph)
	}
	totalHeight := prepared.totalHeight()
	if canCollapseSpacing {
		totalHeight += beforePT - paragraph.BeforePT
	}
	keepTogether := paragraph.KeepTogether || paragraph.KeepWithNext
	if keepTogether && totalHeight <= c.content.HeightPT+epsilonPT && c.used && c.y+totalHeight > c.content.YPT+c.content.HeightPT+epsilonPT {
		c.advanceColumn()
		canCollapseSpacing = false
		beforePT = paragraph.BeforePT
		firstHeight = beforePT + prepared.uiBeforePT + prepared.lines[0].height
		if len(prepared.lines) == 1 {
			firstHeight += prepared.uiAfterPT + paragraphBottomBorderExtent(paragraph)
		}
	}
	if beforePT < 0 {
		c.y += beforePT
		firstHeight -= beforePT
		beforePT = 0
	}
	oldPage, oldColumn := c.pageIndex, c.currentCol
	if !c.ensureSpace(firstHeight, prepared.sourceID) {
		return nil
	}
	if canCollapseSpacing && (c.pageIndex != oldPage || c.currentCol != oldColumn) {
		beforePT = paragraph.BeforePT
		firstHeight = beforePT + prepared.uiBeforePT + prepared.lines[0].height
		if len(prepared.lines) == 1 {
			firstHeight += prepared.uiAfterPT + paragraphBottomBorderExtent(paragraph)
		}
		if !c.ensureSpace(firstHeight, prepared.sourceID) {
			return nil
		}
	}
	c.y += beforePT + prepared.uiBeforePT
	area = c.currentColumnRect()
	c.placeFloatingImages(prepared.floatingImages, area, paragraph, c.y)

	laidOut := make([]paragraphLine, 0, len(prepared.lines))
	atoms := prepared.atoms
	atomIndex := 0
	lineNumber := 0
	placeLine := func(line paragraphLine, interval horizontalInterval, y float64, firstLine, lastLine bool) {
		line.rangeXPT = interval.left
		line.rangeWidthPT = interval.right - interval.left
		line.hasRange = true
		borderSourceID := prepared.borderSourceID
		if borderSourceID == "" {
			borderSourceID = prepared.sourceID
		}
		placeParagraphLine(c, prepared.sourceID, borderSourceID, c.pageIndex, y, c.currentColumnRect(), paragraph, line, firstLine, lastLine)
		laidOut = append(laidOut, line)
		c.used = true
		if line.pageBreakAfter {
			c.pendingBreak = true
		}
	}

	if len(atoms) == 0 {
		line := prepared.lines[0]
		for {
			page, column := c.pageIndex, c.currentCol
			if !c.ensureSpace(line.height, prepared.sourceID) {
				return nil
			}
			if page != c.pageIndex || column != c.currentCol {
				continue
			}
			intervals, nextY := c.paragraphLineIntervals(c.currentColumnRect(), paragraph, true, c.y, line.height)
			if len(intervals) == 0 {
				if nextY <= c.y+epsilonPT {
					nextY = c.y + math.Max(line.height, 1)
				}
				c.y = nextY
				continue
			}
			baselineY := c.y
			placeLine(line, intervals[0], baselineY, true, true)
			c.y += line.height
			break
		}
	} else {
		for atomIndex < len(atoms) {
			if err := ctx.Err(); err != nil {
				return err
			}
			lineHeightGuess := prepared.lines[0].height
			if lineHeightGuess <= 0 {
				lineHeightGuess = lineHeight(paragraphLine{ascent: prepared.style.text.FontSize * 0.8, descent: prepared.style.text.FontSize * 0.2}, paragraph)
			}
			page, column := c.pageIndex, c.currentCol
			if !c.ensureSpace(lineHeightGuess, prepared.sourceID) {
				return nil
			}
			if page != c.pageIndex || column != c.currentCol {
				continue
			}
			area = c.currentColumnRect()
			firstVisualLine := lineNumber == 0
			baseInterval := paragraphBaseInterval(area, paragraph, firstVisualLine)
			bandHeight := lineHeightGuess
			var segments []paragraphFlowSegment
			var nextAtomIndex int
			var baselineHeight, baselineAscent, baselineDescent float64
			for {
				intervals, nextY := c.paragraphLineIntervals(area, paragraph, firstVisualLine, c.y, bandHeight)
				segments, nextAtomIndex = wrapParagraphAtomsAcrossIntervals(atoms, atomIndex, intervals, baseInterval, paragraph, prepared.defaultTabStopPT)
				if len(segments) == 0 {
					if atoms[atomIndex].kind == atomPageBreak {
						c.pendingBreak = true
						atomIndex++
						break
					}
					if atoms[atomIndex].kind == atomLineBreak {
						atomIndex++
						c.y += bandHeight
						lineNumber++
						break
					}
					if nextY <= c.y+epsilonPT {
						nextY = c.y + math.Max(bandHeight, 1)
					}
					c.y = nextY
					break
				}
				if nextAtomIndex <= atomIndex {
					c.diagnostic(DiagnosticError, prepared.sourceID, "paragraph-reflow-stalled", "Text wrapping could not advance through this paragraph.")
					return nil
				}
				baselineHeight, baselineAscent, baselineDescent = 0, 0, 0
				for _, segment := range segments {
					baselineAscent = math.Max(baselineAscent, segment.line.ascent)
					baselineDescent = math.Max(baselineDescent, segment.line.descent)
				}
				baselineHeight = lineHeight(paragraphLine{ascent: baselineAscent, descent: baselineDescent}, paragraph)
				if baselineHeight <= bandHeight+epsilonPT {
					break
				}
				bandHeight = baselineHeight
			}
			if len(segments) == 0 {
				continue
			}
			page, column = c.pageIndex, c.currentCol
			for _, segment := range segments {
				if segment.line.pageBreakBefore {
					c.pendingBreak = true
					break
				}
			}
			if !c.ensureSpace(baselineHeight, prepared.sourceID) {
				return nil
			}
			if page != c.pageIndex || column != c.currentCol {
				continue
			}
			baselineY := c.y
			for index, segment := range segments {
				segment.line.ascent = baselineAscent
				segment.line.descent = baselineDescent
				segment.line.height = baselineHeight
				lastLine := nextAtomIndex >= len(atoms) && index == len(segments)-1
				placeLine(segment.line, segment.interval, baselineY, firstVisualLine, lastLine)
			}
			c.y = baselineY + baselineHeight
			atomIndex = nextAtomIndex
			lineNumber++
		}
	}
	prepared.lines = laidOut
	c.y += prepared.uiAfterPT + paragraphBottomBorderExtent(paragraph) + paragraph.AfterPT
	c.hasPreviousParagraph = !preparedHasPageBreak(prepared)
	c.previousParagraphAfterPT = paragraph.AfterPT
	c.previousParagraphLineGapPT = paragraphLineGap(prepared, true)
	return nil
}

func nextFlowAtomWidth(atoms []paragraphAtom) float64 {
	for _, atom := range atoms {
		switch atom.kind {
		case atomPageBreak, atomLineBreak:
			continue
		case atomTab:
			return 0
		default:
			return atom.advance
		}
	}
	return 0
}

type paragraphFlowSegment struct {
	line     paragraphLine
	interval horizontalInterval
}

func wrapParagraphAtomsAcrossIntervals(atoms []paragraphAtom, start int, intervals []horizontalInterval, base horizontalInterval, paragraph document.WordParagraphLayout, defaultTabStopPT float64) ([]paragraphFlowSegment, int) {
	segments := make([]paragraphFlowSegment, 0, len(intervals))
	next := start
	for _, interval := range intervals {
		if next >= len(atoms) {
			break
		}
		width := interval.right - interval.left
		if width <= epsilonPT {
			continue
		}
		if nextFlowAtomWidth(atoms[next:]) > width+epsilonPT && !sameHorizontalInterval(interval, base) {
			continue
		}
		lines := wrapParagraphAtomsLimit(atoms[next:], width, paragraph, defaultTabStopPT, 1)
		if len(lines) == 0 {
			continue
		}
		line := lines[0]
		if line.nextAtomIndex <= next {
			continue
		}
		segments = append(segments, paragraphFlowSegment{line: line, interval: interval})
		next = line.nextAtomIndex
		if line.pageBreakAfter || next > 0 && next <= len(atoms) && atoms[next-1].kind == atomLineBreak {
			break
		}
	}
	return segments, next
}

func sameHorizontalInterval(left, right horizontalInterval) bool {
	return math.Abs(left.left-right.left) <= epsilonPT && math.Abs(left.right-right.right) <= epsilonPT
}

func placeParagraphLine(cursor *layoutCursor, sourceID, borderSourceID string, pageIndex int, y float64, area Rect, paragraph document.WordParagraphLayout, line paragraphLine, firstLine, lastLine bool) {
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
	startX := area.XPT + left + firstOffset
	if line.hasRange {
		available = math.Max(0, line.rangeWidthPT)
		startX = line.rangeXPT
	}
	if line.width > available+epsilonPT && paragraphLineHasVisibleContent(line) {
		cursor.diagnostic(DiagnosticError, sourceID, "paragraph-line-overflow", fmt.Sprintf("A shaped text or inline object exceeds the available paragraph line width (%.2fpt > %.2fpt).", line.width, available))
	}
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
			fragment := textFragmentAt(atom, x, baseline)
			cursor.result.Pages[pageIndex].Text = append(cursor.result.Pages[pageIndex].Text, fragment)
			if !atom.synthetic && atom.runID != "" {
				for offset := atom.runeStart; offset < atom.runeEnd; offset++ {
					position := SourcePosition{RunID: atom.runID, RuneOffset: offset}
					cursor.result.Locations[position] = Position{PageIndex: pageIndex, XPT: x, YPT: baseline}
				}
				position := SourcePosition{RunID: atom.runID, RuneOffset: atom.runeEnd}
				cursor.result.Locations[position] = Position{PageIndex: pageIndex, XPT: x + atom.advance, YPT: baseline}
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
					YPT:      y + line.height + border.SpacePT,
					WidthPT:  math.Max(0, area.WidthPT-left-right),
					HeightPT: border.SizePT,
				},
			})
		}
	}
}

func paragraphLineHasVisibleContent(line paragraphLine) bool {
	for _, atom := range line.atoms {
		if atom.kind == atomImage || atom.kind == atomText && strings.TrimSpace(atom.text) != "" {
			return true
		}
	}
	return false
}

func textFragmentAt(atom paragraphAtom, x, baseline float64) TextFragment {
	fragment := TextFragment{
		RunID: atom.runID, Text: atom.text, RuneStart: atom.runeStart, RuneEnd: atom.runeEnd,
		Editable: atom.editable, Style: atom.style, XPT: x, BaselineYPT: baseline, Synthetic: atom.synthetic,
	}
	glyphPen := 0.0
	for glyphIndex, glyph := range atom.glyphs {
		fragment.Glyphs = append(fragment.Glyphs, PositionedGlyph{
			FontKey: glyph.FontKey, GlyphID: glyph.GlyphID, RuneStart: glyph.RuneStart + atom.glyphRuneOffset,
			RuneEnd: glyph.RuneEnd + atom.glyphRuneOffset, XPT: x + glyphPen + glyph.OffsetXPT,
			YPT: baseline - glyph.OffsetYPT, AdvanceXPT: glyph.AdvanceXPT,
			Outline: atom.outlines[glyphIndex],
		})
		glyphPen += glyph.AdvanceXPT
	}
	return fragment
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
