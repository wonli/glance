package wordlayout

import (
	"glance/internal/document"
	"math"
)

type layoutCursor struct {
	result                     *Result
	setup                      document.WordPageSetup
	content                    Rect
	pageIndex                  int
	sectionIndex               int
	currentCol                 int
	columns                    int
	columnWidth                float64
	columnSpacing              float64
	y                          float64
	used                       bool
	pendingBreak               bool
	automaticPageStart         bool
	hasPreviousParagraph       bool
	previousParagraphAfterPT   float64
	previousParagraphLineGapPT float64
	glyphPaths                 map[glyphPathCacheKey][]GlyphPathCommand
	currentBlockIndex          int
	sourceBlockIndexes         map[string]int
	floating                   []floatingObstacle
}

type glyphPathCacheKey struct {
	fontKey  string
	glyphID  uint32
	fontSize uint64
}

func newLayoutCursor(result *Result, setup document.WordPageSetup, content Rect) *layoutCursor {
	cols := setup.Columns
	if cols < 1 {
		cols = 1
	}
	colSpacing := setup.ColumnSpacingPT
	if colSpacing < 0 {
		colSpacing = 0
	}
	colWidth := content.WidthPT
	if cols > 1 {
		totalGaps := float64(cols-1) * colSpacing
		if content.WidthPT > totalGaps {
			colWidth = (content.WidthPT - totalGaps) / float64(cols)
		} else {
			cols = 1
			colWidth = content.WidthPT
		}
	}
	cursor := &layoutCursor{
		result:            result,
		setup:             setup,
		content:           content,
		columns:           cols,
		columnWidth:       colWidth,
		columnSpacing:     colSpacing,
		glyphPaths:        make(map[glyphPathCacheKey][]GlyphPathCommand),
		currentBlockIndex: -1,
	}
	cursor.startPage()
	return cursor
}

func (c *layoutCursor) currentColumnRect() Rect {
	x := c.content.XPT
	if c.columns > 1 && c.currentCol > 0 {
		x += float64(c.currentCol) * (c.columnWidth + c.columnSpacing)
	}
	return Rect{
		XPT:      x,
		YPT:      c.content.YPT,
		WidthPT:  c.columnWidth,
		HeightPT: c.content.HeightPT,
	}
}

func (c *layoutCursor) startPage() {
	c.pageIndex = len(c.result.Pages)
	c.result.Pages = append(c.result.Pages, PageScene{
		Index: c.pageIndex, PageSetup: c.setup,
		SectionIndex: c.sectionIndex,
		Clip:         Rect{WidthPT: c.setup.WidthPT, HeightPT: c.setup.HeightPT}, Content: c.content,
	})
	c.currentCol = 0
	c.automaticPageStart = false
	c.y = c.content.YPT
	c.used = false
	c.hasPreviousParagraph = false
	c.previousParagraphAfterPT = 0
	c.previousParagraphLineGapPT = 0
	c.floating = nil
}

func (c *layoutCursor) nextPage() {
	c.startPage()
}

func (c *layoutCursor) advanceColumn() {
	if c.currentCol+1 < c.columns {
		c.currentCol++
		c.floating = nil
		c.y = c.content.YPT
		c.used = false
		c.hasPreviousParagraph = false
		c.previousParagraphAfterPT = 0
		c.previousParagraphLineGapPT = 0
	} else {
		c.nextPage()
		c.automaticPageStart = true
	}
}

func (c *layoutCursor) diagnostic(severity DiagnosticSeverity, sourceID, code, detail string) {
	for _, existing := range c.result.Diagnostics {
		if existing.SourceID == sourceID && existing.Code == code {
			return
		}
	}
	c.result.Diagnostics = append(c.result.Diagnostics, Diagnostic{Severity: severity, SourceID: sourceID, Code: code, Detail: detail})
	blockIndex, known := c.sourceBlockIndexes[sourceID]
	if !known {
		blockIndex = c.currentBlockIndex
	}
	c.result.diagnosticBlocks = append(c.result.diagnosticBlocks, blockIndex)
}

func (c *layoutCursor) checkpoint() layoutCheckpoint {
	if c.pageIndex < 0 || c.pageIndex >= len(c.result.Pages) {
		return layoutCheckpoint{PageIndex: -1, Col: 0, YPT: c.y, Used: c.used, PendingBreak: c.pendingBreak, AutomaticPageStart: c.automaticPageStart, HasPreviousParagraph: c.hasPreviousParagraph, PreviousParagraphLineGapPT: c.previousParagraphLineGapPT, PreviousParagraphAfterPT: c.previousParagraphAfterPT}
	}
	page := c.result.Pages[c.pageIndex]
	return layoutCheckpoint{
		PageIndex: c.pageIndex, Col: c.currentCol, YPT: c.y, Used: c.used, PendingBreak: c.pendingBreak, AutomaticPageStart: c.automaticPageStart,
		HasPreviousParagraph: c.hasPreviousParagraph, PreviousParagraphLineGapPT: c.previousParagraphLineGapPT, PreviousParagraphAfterPT: c.previousParagraphAfterPT,
		TextCount: len(page.Text), ImageCount: len(page.Images),
		TableCellCount: len(page.TableCells), ParagraphBorderCount: len(page.ParagraphBorders),
		SectionIndex: c.sectionIndex,
	}
}

func (c *layoutCursor) ensureSpace(height float64, sourceID string) bool {
	if height < 0 || math.IsNaN(height) || math.IsInf(height, 0) {
		c.diagnostic(DiagnosticError, sourceID, "invalid-layout-height", "A Word layout item has invalid height.")
		return false
	}
	if c.pendingBreak {
		if c.used {
			c.nextPage()
		}
		c.pendingBreak = false
	}
	if height > c.content.HeightPT+epsilonPT {
		c.diagnostic(DiagnosticError, sourceID, "item-overflow", "A Word layout item is taller than the printable page area.")
		return false
	}
	if c.y+height > c.content.YPT+c.content.HeightPT+epsilonPT {
		if c.used {
			c.advanceColumn()
		} else {
			c.diagnostic(DiagnosticError, sourceID, "item-overflow", "A Word layout item does not fit in the printable page area.")
			return false
		}
	}
	return true
}
