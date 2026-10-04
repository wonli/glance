package wordlayout

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"glance/internal/document"
)

const (
	emuPerPoint = 12700.0
	epsilonPT   = 1e-7
)

type Input struct {
	DocumentID       string
	Revision         uint64
	Blocks           []document.WordBlock
	Layout           document.WordLayoutData
	DefaultPageSetup document.WordPageSetup
	// EditorParagraphInsets makes page scenes reserve the same small paragraph
	// padding used by the editable Gio canvas, so screen and print share geometry.
	EditorParagraphInsets bool
}

type SourcePosition struct {
	RunID      string
	RuneOffset int
}

type Position struct {
	PageIndex int
	XPT       float64
	YPT       float64
}

type DiagnosticSeverity string

const (
	DiagnosticWarning DiagnosticSeverity = "warning"
	DiagnosticError   DiagnosticSeverity = "error"
)

type Diagnostic struct {
	Severity DiagnosticSeverity
	SourceID string
	Code     string
	Detail   string
}

type Rect struct {
	XPT      float64
	YPT      float64
	WidthPT  float64
	HeightPT float64
}

type PageScene struct {
	Index            int
	PageSetup        document.WordPageSetup
	Clip             Rect
	Content          Rect
	Text             []TextFragment
	Images           []ImagePlacement
	TableCells       []TableCellPlacement
	ParagraphBorders []ParagraphBorderPlacement
}

// ParagraphBorderPlacement is a paragraph decoration shared by screen and
// print renderers. Bounds.HeightPT stores the border thickness.
type ParagraphBorderPlacement struct {
	SourceID string
	Style    string
	Color    string
	Bounds   Rect
}

type TextFragment struct {
	RunID       string
	Text        string
	RuneStart   int
	RuneEnd     int
	Style       document.WordTextStyle
	XPT         float64
	BaselineYPT float64
	Glyphs      []PositionedGlyph
	Synthetic   bool
}

type PositionedGlyph struct {
	FontKey    string
	GlyphID    uint32
	RuneStart  int
	RuneEnd    int
	XPT        float64
	YPT        float64
	AdvanceXPT float64
	Outline    []GlyphPathCommand
}

type ImagePlacement struct {
	ImageID         string
	Bounds          Rect
	RotationDegrees float64
}

type TableCellPlacement struct {
	TableID   string
	RowIndex  int
	CellID    string
	FillColor string
	Bounds    Rect
}

type Result struct {
	DocumentID  string
	Revision    uint64
	Pages       []PageScene
	Diagnostics []Diagnostic
	Locations   map[SourcePosition]Position
	CanPreview  bool
}

type Engine struct {
	shaper TextShaper
}

func NewEngine(shaper TextShaper) *Engine {
	return &Engine{shaper: shaper}
}

func (e *Engine) Layout(ctx context.Context, input Input) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if e == nil || e.shaper == nil {
		return Result{}, errors.New("Word layout requires a text shaper")
	}

	setup := resolvePageSetup(input.Layout, input.DefaultPageSetup)
	content := pageContentRect(setup)
	result := Result{
		DocumentID: input.DocumentID,
		Revision:   input.Revision,
		Locations:  make(map[SourcePosition]Position),
		CanPreview: true,
	}
	cursor := newLayoutCursor(&result, setup, content)
	if usesDefaultPageSetup(input.Layout, input.DefaultPageSetup) {
		cursor.diagnostic(DiagnosticWarning, "section-0", "page-setup-defaulted", "The document does not define complete page geometry; product page defaults were used.")
	}

	if content.WidthPT <= 0 || content.HeightPT <= 0 {
		cursor.diagnostic(DiagnosticError, "document", "invalid-page-area", "Page dimensions and margins leave no printable area.")
	}
	if len(input.Layout.Sections) > 1 {
		cursor.diagnostic(DiagnosticError, "section-1", "multiple-sections", "Section-specific page layouts are not supported yet.")
	}
	for _, unsupported := range input.Layout.Unsupported {
		cursor.diagnostic(DiagnosticError, unsupported.SourceID, unsupported.Code, unsupported.Description)
	}

	styles := newStyleResolver(input.Layout, cursor)
	preparedParagraphs := make(map[int]preparedParagraph)
	for blockIndex, block := range input.Blocks {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		switch block.Kind {
		case document.WordParagraphBlock:
			if block.Paragraph == nil {
				cursor.diagnostic(DiagnosticError, block.ID, "invalid-paragraph", "The paragraph block has no paragraph data.")
				continue
			}
			prepared, ok := preparedParagraphs[blockIndex]
			if !ok {
				var err error
				prepared, err = prepareParagraph(ctx, e.shaper, styles, *block.Paragraph, block.ID, content.WidthPT, false, input.EditorParagraphInsets)
				if err != nil {
					return Result{}, err
				}
			}
			if prepared.borderSourceID == block.ID && paragraphBottomBorderExtent(prepared.style.paragraph) > 0 && blockIndex+1 < len(input.Blocks) {
				nextBlock := input.Blocks[blockIndex+1]
				if nextBlock.Kind == document.WordParagraphBlock && nextBlock.Paragraph != nil {
					nextPrepared, exists := preparedParagraphs[blockIndex+1]
					if !exists {
						var err error
						nextPrepared, err = prepareParagraph(ctx, e.shaper, styles, *nextBlock.Paragraph, nextBlock.ID, content.WidthPT, false, input.EditorParagraphInsets)
						if err != nil {
							return Result{}, err
						}
					}
					if nextPrepared.style.paragraph.Set&document.WordParagraphBottomBorderSet == 0 {
						nextPrepared.style.paragraph.BottomBorder = prepared.style.paragraph.BottomBorder
						nextPrepared.style.paragraph.Set |= document.WordParagraphBottomBorderSet
						nextPrepared.borderSourceID = block.ID
						preparedParagraphs[blockIndex+1] = nextPrepared
						prepared.style.paragraph.BottomBorder = document.WordParagraphBorder{}
						prepared.style.paragraph.Set |= document.WordParagraphBottomBorderSet
						prepared.borderSourceID = ""
					}
				}
			}
			if prepared.style.paragraph.KeepWithNext && !preparedHasPageBreak(prepared) {
				groupHeight := prepared.totalHeight()
				groupEnd := blockIndex
				for nextIndex := blockIndex + 1; nextIndex < len(input.Blocks); nextIndex++ {
					nextBlock := input.Blocks[nextIndex]
					if nextBlock.Kind != document.WordParagraphBlock || nextBlock.Paragraph == nil {
						cursor.diagnostic(DiagnosticWarning, block.ID, "keep-with-next-target-unsupported", "Keep-with-next is applied to consecutive paragraphs only; the following block cannot be kept with this paragraph.")
						break
					}
					nextPrepared, err := prepareParagraph(ctx, e.shaper, styles, *nextBlock.Paragraph, nextBlock.ID, content.WidthPT, false, input.EditorParagraphInsets)
					if err != nil {
						return Result{}, err
					}
					preparedParagraphs[nextIndex] = nextPrepared
					groupHeight += nextPrepared.totalHeight()
					groupEnd = nextIndex
					if !nextPrepared.style.paragraph.KeepWithNext || preparedHasPageBreak(nextPrepared) {
						break
					}
				}
				if groupEnd > blockIndex && !preparedHasPageBreak(prepared) && groupHeight <= content.HeightPT+epsilonPT && cursor.used && cursor.y+groupHeight > content.YPT+content.HeightPT+epsilonPT {
					cursor.nextPage()
				}
			}
			if err := cursor.placePreparedParagraph(ctx, prepared, content, true, -1, 0); err != nil {
				return Result{}, err
			}
		case document.WordTableBlock:
			if block.Table == nil {
				cursor.diagnostic(DiagnosticError, block.ID, "invalid-table", "The table block has no table data.")
				continue
			}
			if err := cursor.layoutTable(ctx, e.shaper, styles, block.ID, *block.Table, content, input.EditorParagraphInsets); err != nil {
				return Result{}, err
			}
		case document.WordUnsupportedBlock:
			detail := "This body content is not supported by Word pagination."
			if block.Unsupported != nil && block.Unsupported.Description != "" {
				detail = block.Unsupported.Description
			}
			sourceID := block.ID
			if block.Unsupported != nil && block.Unsupported.ID != "" {
				sourceID = block.Unsupported.ID
			}
			cursor.diagnostic(DiagnosticError, sourceID, "unsupported-block", detail)
		default:
			cursor.diagnostic(DiagnosticError, block.ID, "unknown-block-kind", fmt.Sprintf("Unsupported Word body block kind %d at index %d.", block.Kind, blockIndex))
		}
	}
	if len(result.Pages) == 0 {
		cursor.startPage()
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == DiagnosticError {
			result.CanPreview = false
			break
		}
	}
	return result, nil
}

func resolvePageSetup(layout document.WordLayoutData, fallback document.WordPageSetup) document.WordPageSetup {
	if fallback.WidthPT <= 0 || fallback.HeightPT <= 0 {
		fallback = document.WordPageSetup{
			WidthPT: 595.28, HeightPT: 841.89, Orientation: "portrait",
			MarginTopPT: 72, MarginRightPT: 72, MarginBottomPT: 72, MarginLeftPT: 72,
		}
	}
	section := document.WordPageSetup{}
	if len(layout.Sections) > 0 {
		section = layout.Sections[0].PageSetup
	}
	value := func(mask document.WordPageSetupMask, bit document.WordPageSetupMask, actual, defaultValue float64) float64 {
		if mask&bit != 0 || actual != 0 {
			return actual
		}
		return defaultValue
	}
	setup := document.WordPageSetup{
		WidthPT:        value(section.Set, document.WordPageWidthSet, section.WidthPT, fallback.WidthPT),
		HeightPT:       value(section.Set, document.WordPageHeightSet, section.HeightPT, fallback.HeightPT),
		MarginTopPT:    value(section.Set, document.WordPageMarginTopSet, section.MarginTopPT, fallback.MarginTopPT),
		MarginRightPT:  value(section.Set, document.WordPageMarginRightSet, section.MarginRightPT, fallback.MarginRightPT),
		MarginBottomPT: value(section.Set, document.WordPageMarginBottomSet, section.MarginBottomPT, fallback.MarginBottomPT),
		MarginLeftPT:   value(section.Set, document.WordPageMarginLeftSet, section.MarginLeftPT, fallback.MarginLeftPT),
		Orientation:    strings.ToLower(section.Orientation),
	}
	explicitOrientation := section.Set&document.WordPageOrientationSet != 0 || section.Orientation != ""
	if setup.Orientation == "" && section.WidthPT == 0 && section.HeightPT == 0 {
		setup.Orientation = strings.ToLower(fallback.Orientation)
		explicitOrientation = setup.Orientation != ""
	}
	if setup.Orientation == "" {
		if setup.WidthPT > setup.HeightPT {
			setup.Orientation = "landscape"
		} else {
			setup.Orientation = "portrait"
		}
	}
	if explicitOrientation && ((setup.Orientation == "landscape" && setup.WidthPT < setup.HeightPT) || (setup.Orientation == "portrait" && setup.WidthPT > setup.HeightPT)) {
		setup.WidthPT, setup.HeightPT = setup.HeightPT, setup.WidthPT
	}
	return setup
}

func usesDefaultPageSetup(layout document.WordLayoutData, fallback document.WordPageSetup) bool {
	if fallback.WidthPT <= 0 || fallback.HeightPT <= 0 || len(layout.Sections) == 0 {
		return true
	}
	setup := layout.Sections[0].PageSetup
	hasValue := func(bit document.WordPageSetupMask, actual float64) bool {
		return setup.Set&bit != 0 || actual != 0
	}
	return !hasValue(document.WordPageWidthSet, setup.WidthPT) ||
		!hasValue(document.WordPageHeightSet, setup.HeightPT) ||
		!hasValue(document.WordPageMarginTopSet, setup.MarginTopPT) ||
		!hasValue(document.WordPageMarginRightSet, setup.MarginRightPT) ||
		!hasValue(document.WordPageMarginBottomSet, setup.MarginBottomPT) ||
		!hasValue(document.WordPageMarginLeftSet, setup.MarginLeftPT)
}

func pageContentRect(setup document.WordPageSetup) Rect {
	return Rect{
		XPT:      setup.MarginLeftPT,
		YPT:      setup.MarginTopPT,
		WidthPT:  math.Max(0, setup.WidthPT-setup.MarginLeftPT-setup.MarginRightPT),
		HeightPT: math.Max(0, setup.HeightPT-setup.MarginTopPT-setup.MarginBottomPT),
	}
}

type layoutCursor struct {
	result       *Result
	setup        document.WordPageSetup
	content      Rect
	pageIndex    int
	y            float64
	used         bool
	pendingBreak bool
	glyphPaths   map[glyphPathCacheKey][]GlyphPathCommand
}

type glyphPathCacheKey struct {
	fontKey  string
	glyphID  uint32
	fontSize uint64
}

func newLayoutCursor(result *Result, setup document.WordPageSetup, content Rect) *layoutCursor {
	cursor := &layoutCursor{result: result, setup: setup, content: content, glyphPaths: make(map[glyphPathCacheKey][]GlyphPathCommand)}
	cursor.startPage()
	return cursor
}

func (c *layoutCursor) startPage() {
	c.pageIndex = len(c.result.Pages)
	c.result.Pages = append(c.result.Pages, PageScene{
		Index: c.pageIndex, PageSetup: c.setup,
		Clip: Rect{WidthPT: c.setup.WidthPT, HeightPT: c.setup.HeightPT}, Content: c.content,
	})
	c.y = c.content.YPT
	c.used = false
}

func (c *layoutCursor) nextPage() {
	c.startPage()
}

func (c *layoutCursor) diagnostic(severity DiagnosticSeverity, sourceID, code, detail string) {
	for _, existing := range c.result.Diagnostics {
		if existing.SourceID == sourceID && existing.Code == code {
			return
		}
	}
	c.result.Diagnostics = append(c.result.Diagnostics, Diagnostic{Severity: severity, SourceID: sourceID, Code: code, Detail: detail})
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
			c.nextPage()
		} else {
			c.diagnostic(DiagnosticError, sourceID, "item-overflow", "A Word layout item does not fit in the printable page area.")
			return false
		}
	}
	return true
}

type styleResolver struct {
	defaults document.WordStyleDefaults
	styles   map[string]document.WordStyle
	cursor   *layoutCursor
}

func newStyleResolver(layout document.WordLayoutData, cursor *layoutCursor) *styleResolver {
	resolver := &styleResolver{defaults: layout.Defaults, styles: make(map[string]document.WordStyle, len(layout.Styles)), cursor: cursor}
	for _, style := range layout.Styles {
		if style.ID == "" {
			continue
		}
		if _, duplicate := resolver.styles[style.ID]; duplicate {
			cursor.diagnostic(DiagnosticError, style.ID, "duplicate-style", "The DOCX contains duplicate style identifiers.")
			continue
		}
		resolver.styles[style.ID] = style
	}
	return resolver
}

type resolvedParagraphStyle struct {
	text      document.WordTextStyle
	paragraph document.WordParagraphLayout
}

func (r *styleResolver) paragraphStyle(paragraph document.WordParagraph, sourceID string) resolvedParagraphStyle {
	out := resolvedParagraphStyle{}
	mergeTextStyle(&out.text, r.defaults.Text, r.defaults.TextSet)
	mergeParagraphLayout(&out.paragraph, r.defaults.Paragraph)
	if paragraph.Style != "" {
		chain := make([]document.WordStyle, 0, 4)
		seen := map[string]bool{}
		current := paragraph.Style
		for current != "" {
			if seen[current] {
				r.cursor.diagnostic(DiagnosticError, sourceID, "style-cycle", "A paragraph style inheritance cycle prevents deterministic layout.")
				break
			}
			seen[current] = true
			style, ok := r.styles[current]
			if !ok {
				r.cursor.diagnostic(DiagnosticError, sourceID, "missing-style", "The paragraph references a missing style: "+current)
				break
			}
			chain = append(chain, style)
			current = style.BasedOn
		}
		for index := len(chain) - 1; index >= 0; index-- {
			style := chain[index]
			mergeTextStyle(&out.text, style.Text, style.TextSet)
			mergeParagraphLayout(&out.paragraph, style.Paragraph)
		}
	}
	mergeParagraphLayout(&out.paragraph, paragraph.Layout)
	if paragraph.Layout.Set&document.WordParagraphAlignmentSet == 0 && paragraph.Alignment != document.WordAlignDefault {
		out.paragraph.Alignment = paragraph.Alignment
		out.paragraph.Set |= document.WordParagraphAlignmentSet
	}
	if paragraph.Layout.Set&document.WordParagraphLeftIndentSet == 0 && paragraph.LeftIndentEMU != 0 {
		out.paragraph.LeftIndentEMU = paragraph.LeftIndentEMU
		out.paragraph.Set |= document.WordParagraphLeftIndentSet
	}
	if paragraph.Layout.Set&document.WordParagraphRightIndentSet == 0 && paragraph.RightIndentEMU != 0 {
		out.paragraph.RightIndentEMU = paragraph.RightIndentEMU
		out.paragraph.Set |= document.WordParagraphRightIndentSet
	}
	if out.text.FontSize <= 0 {
		out.text.FontSize = 11
	}
	if out.text.FontFamily == "" {
		out.text.FontFamily = "sans-serif"
	}
	if out.text.FontColor == "" {
		out.text.FontColor = "000000"
	}
	if borderStyle := out.paragraph.BottomBorder.Style; borderStyle != "" && borderStyle != "single" && borderStyle != "none" && borderStyle != "nil" {
		r.cursor.diagnostic(DiagnosticError, sourceID, "paragraph-border-style", "The paragraph uses an unsupported bottom border style: "+borderStyle)
	}
	return out
}

func (r *styleResolver) applyCharacterStyle(target *document.WordTextStyle, styleID, sourceID string) {
	if styleID == "" {
		return
	}
	chain := make([]document.WordStyle, 0, 4)
	seen := map[string]bool{}
	current := styleID
	for current != "" {
		if seen[current] {
			r.cursor.diagnostic(DiagnosticError, sourceID, "style-cycle", "A character style inheritance cycle prevents deterministic layout.")
			break
		}
		seen[current] = true
		style, ok := r.styles[current]
		if !ok {
			r.cursor.diagnostic(DiagnosticError, sourceID, "missing-style", "A run references a missing character style: "+current)
			break
		}
		chain = append(chain, style)
		current = style.BasedOn
	}
	for index := len(chain) - 1; index >= 0; index-- {
		mergeTextStyle(target, chain[index].Text, chain[index].TextSet)
	}
}

func mergeTextStyle(target *document.WordTextStyle, source document.WordTextStyle, mask document.WordTextStyleMask) {
	if mask == 0 {
		if source.Bold {
			mask |= document.WordTextBoldSet
		}
		if source.Italic {
			mask |= document.WordTextItalicSet
		}
		if source.Underline {
			mask |= document.WordTextUnderlineSet
		}
		if source.FontFamily != "" {
			mask |= document.WordTextFontFamilySet
		}
		if source.FontSize != 0 {
			mask |= document.WordTextFontSizeSet
		}
		if source.FontColor != "" {
			mask |= document.WordTextFontColorSet
		}
		if source.Highlight != "" {
			mask |= document.WordTextHighlightSet
		}
	}
	if mask&document.WordTextBoldSet != 0 {
		target.Bold = source.Bold
	}
	if mask&document.WordTextItalicSet != 0 {
		target.Italic = source.Italic
	}
	if mask&document.WordTextUnderlineSet != 0 {
		target.Underline = source.Underline
	}
	if mask&document.WordTextFontFamilySet != 0 {
		target.FontFamily = source.FontFamily
	}
	if mask&document.WordTextFontSizeSet != 0 {
		target.FontSize = source.FontSize
	}
	if mask&document.WordTextFontColorSet != 0 {
		target.FontColor = source.FontColor
	}
	if mask&document.WordTextHighlightSet != 0 {
		target.Highlight = source.Highlight
	}
}

func mergeParagraphLayout(target *document.WordParagraphLayout, source document.WordParagraphLayout) {
	mask := source.Set
	if mask == 0 {
		if source.BeforePT != 0 {
			mask |= document.WordParagraphBeforeSet
		}
		if source.AfterPT != 0 {
			mask |= document.WordParagraphAfterSet
		}
		if source.LineSpacingPT != 0 || source.LineSpacingMultiple != 0 || source.LineSpacingRule != "" {
			mask |= document.WordParagraphLineSpacingSet
		}
		if source.FirstLineIndentEMU != 0 {
			mask |= document.WordParagraphFirstLineIndentSet
		}
		if source.HangingIndentEMU != 0 {
			mask |= document.WordParagraphHangingIndentSet
		}
		if source.Alignment != document.WordAlignDefault {
			mask |= document.WordParagraphAlignmentSet
		}
		if source.LeftIndentEMU != 0 {
			mask |= document.WordParagraphLeftIndentSet
		}
		if source.RightIndentEMU != 0 {
			mask |= document.WordParagraphRightIndentSet
		}
		if source.KeepWithNext {
			mask |= document.WordParagraphKeepWithNextSet
		}
		if source.KeepTogether {
			mask |= document.WordParagraphKeepTogetherSet
		}
		if source.PageBreakBefore {
			mask |= document.WordParagraphPageBreakBeforeSet
		}
		if source.BottomBorder != (document.WordParagraphBorder{}) {
			mask |= document.WordParagraphBottomBorderSet
		}
	}
	if mask&document.WordParagraphBeforeSet != 0 {
		target.BeforePT = source.BeforePT
	}
	if mask&document.WordParagraphAfterSet != 0 {
		target.AfterPT = source.AfterPT
	}
	if mask&document.WordParagraphLineSpacingSet != 0 {
		target.LineSpacingPT = source.LineSpacingPT
		target.LineSpacingMultiple = source.LineSpacingMultiple
		target.LineSpacingRule = source.LineSpacingRule
	}
	if mask&document.WordParagraphFirstLineIndentSet != 0 {
		target.FirstLineIndentEMU = source.FirstLineIndentEMU
	}
	if mask&document.WordParagraphHangingIndentSet != 0 {
		target.HangingIndentEMU = source.HangingIndentEMU
	}
	if mask&document.WordParagraphAlignmentSet != 0 {
		target.Alignment = source.Alignment
	}
	if mask&document.WordParagraphLeftIndentSet != 0 {
		target.LeftIndentEMU = source.LeftIndentEMU
	}
	if mask&document.WordParagraphRightIndentSet != 0 {
		target.RightIndentEMU = source.RightIndentEMU
	}
	if mask&document.WordParagraphKeepWithNextSet != 0 {
		target.KeepWithNext = source.KeepWithNext
	}
	if mask&document.WordParagraphKeepTogetherSet != 0 {
		target.KeepTogether = source.KeepTogether
	}
	if mask&document.WordParagraphPageBreakBeforeSet != 0 {
		target.PageBreakBefore = source.PageBreakBefore
	}
	if mask&document.WordParagraphBottomBorderSet != 0 {
		target.BottomBorder = source.BottomBorder
	}
	target.Set |= mask
}
