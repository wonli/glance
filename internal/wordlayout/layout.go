package wordlayout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	Previous         *Result
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
	Editable    bool
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
	DocumentID        string
	Revision          uint64
	Pages             []PageScene
	Diagnostics       []Diagnostic
	Locations         map[SourcePosition]Position
	CanPreview        bool
	blockFingerprints [][32]byte
	blockKinds        []document.WordBlockKind
	blockHazards      []bool
	layoutFingerprint string
	blockCheckpoints  []layoutCheckpoint
	runBlocks         map[string]int
	diagnosticBlocks  []int
}

type layoutCheckpoint struct {
	PageIndex            int
	Col                  int
	YPT                  float64
	Used                 bool
	PendingBreak         bool
	TextCount            int
	ImageCount           int
	TableCellCount       int
	ParagraphBorderCount int
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
	fingerprints, err := fingerprintBlocks(ctx, input.Blocks)
	if err != nil {
		return Result{}, err
	}
	layoutFingerprint := fingerprintLayout(input)
	result := Result{
		DocumentID:        input.DocumentID,
		Revision:          input.Revision,
		Locations:         make(map[SourcePosition]Position),
		CanPreview:        true,
		blockFingerprints: fingerprints,
		blockKinds:        wordBlockKinds(input.Blocks),
		blockHazards:      wordBlockHazards(input.Blocks),
		layoutFingerprint: layoutFingerprint,
		blockCheckpoints:  make([]layoutCheckpoint, len(input.Blocks)+1),
	}
	cursor := newLayoutCursor(&result, setup, content)
	cursor.sourceBlockIndexes, err = wordSourceBlockIndexes(ctx, input.Blocks)
	if err != nil {
		return Result{}, err
	}
	result.runBlocks, err = wordRunBlockIndexes(ctx, input.Blocks)
	if err != nil {
		return Result{}, err
	}
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
	startBlock := 0
	previous := input.Previous
	incrementalStarted := false
	if previous != nil && previous.DocumentID == input.DocumentID && previous.layoutFingerprint == layoutFingerprint && len(previous.blockCheckpoints) == len(previous.blockFingerprints)+1 {
		if dirtyBlock := firstChangedBlock(fingerprints, previous.blockFingerprints); dirtyBlock < 0 {
			unchanged := cloneLayoutResult(*previous)
			unchanged.Revision = input.Revision
			return unchanged, nil
		} else if incrementalLayoutSafe(input, previous, dirtyBlock) {
			checkpoint := previous.blockCheckpoints[dirtyBlock]
			if checkpoint.PageIndex >= 0 && checkpoint.PageIndex < len(previous.Pages) {
				startBlock = dirtyBlock
				incrementalStarted = true
				result.Pages = nil
				result.Pages = append(result.Pages, previous.Pages[:checkpoint.PageIndex]...)
				current := clonePagePrefix(previous.Pages[checkpoint.PageIndex], checkpoint)
				result.Pages = append(result.Pages, current)
				for position, value := range previous.Locations {
					if previous.runBlocks[position.RunID] < dirtyBlock {
						result.Locations[position] = value
					}
				}
				for index, diagnostic := range previous.Diagnostics {
					if index < len(previous.diagnosticBlocks) && previous.diagnosticBlocks[index] >= dirtyBlock {
						continue
					}
					appendUniqueDiagnostic(&result, diagnostic, -1)
				}
				copy(result.blockCheckpoints[:dirtyBlock], previous.blockCheckpoints[:dirtyBlock])
				cursor.pageIndex = checkpoint.PageIndex
				cursor.currentCol = checkpoint.Col
				cursor.y = checkpoint.YPT
				cursor.used = checkpoint.Used
				cursor.pendingBreak = checkpoint.PendingBreak
			}
		}
	}
	converged := false
	for blockIndex := startBlock; blockIndex < len(input.Blocks); blockIndex++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		cursor.currentBlockIndex = blockIndex
		result.blockCheckpoints[blockIndex] = cursor.checkpoint()
		if previous != nil && incrementalStarted && blockIndex > startBlock && suffixFingerprintsEqual(fingerprints, previous.blockFingerprints, blockIndex) {
			old := previous.blockCheckpoints[blockIndex]
			if layoutFlowMatches(cursor, old) {
				appendReusedLayoutSuffix(&result, cursor, *previous, blockIndex)
				converged = true
				break
			}
		}
		block := input.Blocks[blockIndex]
		switch block.Kind {
		case document.WordParagraphBlock:
			if block.Paragraph == nil {
				cursor.diagnostic(DiagnosticError, block.ID, "invalid-paragraph", "The paragraph block has no paragraph data.")
				continue
			}
			prepared, ok := preparedParagraphs[blockIndex]
			if !ok {
				var err error
				prepared, err = prepareParagraph(ctx, e.shaper, styles, *block.Paragraph, block.ID, cursor.columnWidth, false, input.EditorParagraphInsets)
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
						nextPrepared, err = prepareParagraph(ctx, e.shaper, styles, *nextBlock.Paragraph, nextBlock.ID, cursor.columnWidth, false, input.EditorParagraphInsets)
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
					nextPrepared, err := prepareParagraph(ctx, e.shaper, styles, *nextBlock.Paragraph, nextBlock.ID, cursor.columnWidth, false, input.EditorParagraphInsets)
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
					cursor.advanceColumn()
				}
			}
			if err := cursor.placePreparedParagraph(ctx, prepared, cursor.currentColumnRect(), true, -1, 0); err != nil {
				return Result{}, err
			}
		case document.WordTableBlock:
			if block.Table == nil {
				cursor.diagnostic(DiagnosticError, block.ID, "invalid-table", "The table block has no table data.")
				continue
			}
			if err := cursor.layoutTable(ctx, e.shaper, styles, block.ID, *block.Table, cursor.currentColumnRect(), input.EditorParagraphInsets); err != nil {
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
	if !converged {
		cursor.currentBlockIndex = len(input.Blocks)
		result.blockCheckpoints[len(input.Blocks)] = cursor.checkpoint()
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
	cols := 1
	if section.Columns >= 1 {
		cols = section.Columns
	} else if fallback.Columns >= 1 {
		cols = fallback.Columns
	}
	colSpace := 36.0
	if section.Set&document.WordPageColumnSpacingSet != 0 || section.ColumnSpacingPT > 0 {
		colSpace = section.ColumnSpacingPT
	} else if fallback.Set&document.WordPageColumnSpacingSet != 0 || fallback.ColumnSpacingPT > 0 {
		colSpace = fallback.ColumnSpacingPT
	}
	setup := document.WordPageSetup{
		WidthPT:         value(section.Set, document.WordPageWidthSet, section.WidthPT, fallback.WidthPT),
		HeightPT:        value(section.Set, document.WordPageHeightSet, section.HeightPT, fallback.HeightPT),
		MarginTopPT:     value(section.Set, document.WordPageMarginTopSet, section.MarginTopPT, fallback.MarginTopPT),
		MarginRightPT:   value(section.Set, document.WordPageMarginRightSet, section.MarginRightPT, fallback.MarginRightPT),
		MarginBottomPT:  value(section.Set, document.WordPageMarginBottomSet, section.MarginBottomPT, fallback.MarginBottomPT),
		MarginLeftPT:    value(section.Set, document.WordPageMarginLeftSet, section.MarginLeftPT, fallback.MarginLeftPT),
		Columns:         cols,
		ColumnSpacingPT: colSpace,
		Orientation:     strings.ToLower(section.Orientation),
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
	result             *Result
	setup              document.WordPageSetup
	content            Rect
	pageIndex          int
	currentCol         int
	columns            int
	columnWidth        float64
	columnSpacing      float64
	y                  float64
	used               bool
	pendingBreak       bool
	glyphPaths         map[glyphPathCacheKey][]GlyphPathCommand
	currentBlockIndex  int
	sourceBlockIndexes map[string]int
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
		result:             result,
		setup:              setup,
		content:            content,
		columns:            cols,
		columnWidth:        colWidth,
		columnSpacing:      colSpacing,
		glyphPaths:         make(map[glyphPathCacheKey][]GlyphPathCommand),
		currentBlockIndex:  -1,
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

func fingerprintBlocks(ctx context.Context, blocks []document.WordBlock) ([][32]byte, error) {
	fingerprints := make([][32]byte, len(blocks))
	for index, block := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(block)
		fingerprints[index] = sha256.Sum256(encoded)
	}
	return fingerprints, nil
}

func fingerprintLayout(input Input) string {
	value := struct {
		Layout                document.WordLayoutData
		DefaultPageSetup      document.WordPageSetup
		EditorParagraphInsets bool
	}{input.Layout, input.DefaultPageSetup, input.EditorParagraphInsets}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func firstChangedBlock(current, previous [][32]byte) int {
	limit := min(len(current), len(previous))
	for index := 0; index < limit; index++ {
		if current[index] != previous[index] {
			return index
		}
	}
	if len(current) != len(previous) {
		return limit
	}
	return -1
}

func suffixFingerprintsEqual(current, previous [][32]byte, start int) bool {
	if len(current) != len(previous) || start < 0 || start > len(current) {
		return false
	}
	for index := start; index < len(current); index++ {
		if current[index] != previous[index] {
			return false
		}
	}
	return true
}

func wordBlockKinds(blocks []document.WordBlock) []document.WordBlockKind {
	kinds := make([]document.WordBlockKind, len(blocks))
	for index := range blocks {
		kinds[index] = blocks[index].Kind
	}
	return kinds
}

func wordBlockHazards(blocks []document.WordBlock) []bool {
	hazards := make([]bool, len(blocks))
	for index, block := range blocks {
		hazards[index] = blockHasFlowHazard(block)
	}
	return hazards
}

func blockHasFlowHazard(block document.WordBlock) bool {
	return block.Paragraph != nil && (block.Paragraph.Layout.KeepWithNext || block.Paragraph.Layout.BottomBorder.SizePT > 0)
}

func wordRunBlockIndexes(ctx context.Context, blocks []document.WordBlock) (map[string]int, error) {
	indexes := make(map[string]int)
	addParagraph := func(paragraph document.WordParagraph, blockIndex int) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID != "" {
				indexes[inline.Text.ID] = blockIndex
			}
		}
	}
	for blockIndex, block := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if block.Paragraph != nil {
			addParagraph(*block.Paragraph, blockIndex)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						addParagraph(paragraph, blockIndex)
					}
				}
			}
		}
	}
	return indexes, nil
}

func incrementalLayoutSafe(input Input, previous *Result, dirtyBlock int) bool {
	if previous == nil || dirtyBlock < 0 || dirtyBlock >= len(input.Blocks) || len(input.Blocks) != len(previous.blockFingerprints) {
		return false
	}
	if input.Blocks[dirtyBlock].Kind != document.WordParagraphBlock || dirtyBlock >= len(previous.blockKinds) || previous.blockKinds[dirtyBlock] != document.WordParagraphBlock {
		return false
	}
	for _, style := range input.Layout.Styles {
		if style.Paragraph.KeepWithNext || style.Paragraph.BottomBorder.SizePT > 0 {
			return false
		}
	}
	if input.Layout.Defaults.Paragraph.KeepWithNext || input.Layout.Defaults.Paragraph.BottomBorder.SizePT > 0 {
		return false
	}
	for index := max(0, dirtyBlock-1); index <= min(len(input.Blocks)-1, dirtyBlock+1); index++ {
		if (index < len(previous.blockHazards) && previous.blockHazards[index]) || wordBlockHazards(input.Blocks[index : index+1])[0] {
			return false
		}
	}
	return dirtyBlock < len(previous.blockCheckpoints)
}

func cloneLayoutResult(source Result) Result {
	clone := source
	clone.Pages = append([]PageScene(nil), source.Pages...)
	clone.Diagnostics = append([]Diagnostic(nil), source.Diagnostics...)
	clone.blockFingerprints = append([][32]byte(nil), source.blockFingerprints...)
	clone.blockKinds = append([]document.WordBlockKind(nil), source.blockKinds...)
	clone.blockHazards = append([]bool(nil), source.blockHazards...)
	clone.blockCheckpoints = append([]layoutCheckpoint(nil), source.blockCheckpoints...)
	clone.diagnosticBlocks = append([]int(nil), source.diagnosticBlocks...)
	clone.runBlocks = make(map[string]int, len(source.runBlocks))
	for runID, blockIndex := range source.runBlocks {
		clone.runBlocks[runID] = blockIndex
	}
	clone.Locations = make(map[SourcePosition]Position, len(source.Locations))
	for position, value := range source.Locations {
		clone.Locations[position] = value
	}
	return clone
}

func clonePagePrefix(source PageScene, checkpoint layoutCheckpoint) PageScene {
	clone := source
	clone.Text = append([]TextFragment(nil), source.Text[:min(len(source.Text), checkpoint.TextCount)]...)
	clone.Images = append([]ImagePlacement(nil), source.Images[:min(len(source.Images), checkpoint.ImageCount)]...)
	clone.TableCells = append([]TableCellPlacement(nil), source.TableCells[:min(len(source.TableCells), checkpoint.TableCellCount)]...)
	clone.ParagraphBorders = append([]ParagraphBorderPlacement(nil), source.ParagraphBorders[:min(len(source.ParagraphBorders), checkpoint.ParagraphBorderCount)]...)
	return clone
}

func layoutFlowMatches(cursor *layoutCursor, old layoutCheckpoint) bool {
	return old.PageIndex >= 0 && cursor.used == old.Used && cursor.pendingBreak == old.PendingBreak && math.Abs(cursor.y-old.YPT) <= 1e-4
}

func appendUniqueDiagnostic(result *Result, diagnostic Diagnostic, blockIndex int) {
	for _, existing := range result.Diagnostics {
		if existing.SourceID == diagnostic.SourceID && existing.Code == diagnostic.Code {
			return
		}
	}
	result.Diagnostics = append(result.Diagnostics, diagnostic)
	result.diagnosticBlocks = append(result.diagnosticBlocks, blockIndex)
}

func appendReusedLayoutSuffix(result *Result, cursor *layoutCursor, previous Result, blockIndex int) {
	oldCheckpoint := previous.blockCheckpoints[blockIndex]
	newCheckpoint := cursor.checkpoint()
	pageDelta := cursor.pageIndex - oldCheckpoint.PageIndex
	oldPage := previous.Pages[oldCheckpoint.PageIndex]
	currentPage := &result.Pages[cursor.pageIndex]
	currentPage.Text = append(currentPage.Text, oldPage.Text[min(len(oldPage.Text), oldCheckpoint.TextCount):]...)
	currentPage.Images = append(currentPage.Images, oldPage.Images[min(len(oldPage.Images), oldCheckpoint.ImageCount):]...)
	currentPage.TableCells = append(currentPage.TableCells, oldPage.TableCells[min(len(oldPage.TableCells), oldCheckpoint.TableCellCount):]...)
	currentPage.ParagraphBorders = append(currentPage.ParagraphBorders, oldPage.ParagraphBorders[min(len(oldPage.ParagraphBorders), oldCheckpoint.ParagraphBorderCount):]...)
	for pageIndex := oldCheckpoint.PageIndex + 1; pageIndex < len(previous.Pages); pageIndex++ {
		page := previous.Pages[pageIndex]
		page.Index += pageDelta
		result.Pages = append(result.Pages, page)
	}
	for position, value := range previous.Locations {
		if previous.runBlocks[position.RunID] < blockIndex {
			continue
		}
		value.PageIndex += pageDelta
		result.Locations[position] = value
	}
	for index, diagnostic := range previous.Diagnostics {
		if index >= len(previous.diagnosticBlocks) || previous.diagnosticBlocks[index] < blockIndex {
			continue
		}
		appendUniqueDiagnostic(result, diagnostic, previous.diagnosticBlocks[index])
	}
	for index := blockIndex + 1; index < len(result.blockCheckpoints) && index < len(previous.blockCheckpoints); index++ {
		checkpoint := previous.blockCheckpoints[index]
		if checkpoint.PageIndex < oldCheckpoint.PageIndex {
			continue
		}
		checkpoint.PageIndex += pageDelta
		if previous.blockCheckpoints[index].PageIndex == oldCheckpoint.PageIndex {
			checkpoint.TextCount += newCheckpoint.TextCount - oldCheckpoint.TextCount
			checkpoint.ImageCount += newCheckpoint.ImageCount - oldCheckpoint.ImageCount
			checkpoint.TableCellCount += newCheckpoint.TableCellCount - oldCheckpoint.TableCellCount
			checkpoint.ParagraphBorderCount += newCheckpoint.ParagraphBorderCount - oldCheckpoint.ParagraphBorderCount
		}
		result.blockCheckpoints[index] = checkpoint
	}
}

func wordSourceBlockIndexes(ctx context.Context, blocks []document.WordBlock) (map[string]int, error) {
	indexes := make(map[string]int)
	addParagraph := func(paragraph document.WordParagraph, blockIndex int) {
		if paragraph.ID != "" {
			indexes[paragraph.ID] = blockIndex
		}
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID != "" {
				indexes[inline.Text.ID] = blockIndex
			}
			if inline.Image != nil && inline.Image.ID != "" {
				indexes[inline.Image.ID] = blockIndex
			}
			if inline.Unsupported != nil && inline.Unsupported.ID != "" {
				indexes[inline.Unsupported.ID] = blockIndex
			}
		}
	}
	for blockIndex, block := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if block.ID != "" {
			indexes[block.ID] = blockIndex
		}
		if block.Paragraph != nil {
			addParagraph(*block.Paragraph, blockIndex)
		}
		if block.Unsupported != nil && block.Unsupported.ID != "" {
			indexes[block.Unsupported.ID] = blockIndex
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					if cell.ID != "" {
						indexes[cell.ID] = blockIndex
					}
					for _, paragraph := range cell.Paragraphs {
						addParagraph(paragraph, blockIndex)
					}
				}
			}
		}
	}
	return indexes, nil
}

func (c *layoutCursor) startPage() {
	c.pageIndex = len(c.result.Pages)
	c.result.Pages = append(c.result.Pages, PageScene{
		Index: c.pageIndex, PageSetup: c.setup,
		Clip: Rect{WidthPT: c.setup.WidthPT, HeightPT: c.setup.HeightPT}, Content: c.content,
	})
	c.currentCol = 0
	c.y = c.content.YPT
	c.used = false
}

func (c *layoutCursor) nextPage() {
	c.startPage()
}

func (c *layoutCursor) advanceColumn() {
	if c.currentCol+1 < c.columns {
		c.currentCol++
		c.y = c.content.YPT
		c.used = false
	} else {
		c.nextPage()
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
		return layoutCheckpoint{PageIndex: -1, Col: 0, YPT: c.y, Used: c.used, PendingBreak: c.pendingBreak}
	}
	page := c.result.Pages[c.pageIndex]
	return layoutCheckpoint{
		PageIndex: c.pageIndex, Col: c.currentCol, YPT: c.y, Used: c.used, PendingBreak: c.pendingBreak,
		TextCount: len(page.Text), ImageCount: len(page.Images),
		TableCellCount: len(page.TableCells), ParagraphBorderCount: len(page.ParagraphBorders),
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

type styleResolver struct {
	defaults                document.WordStyleDefaults
	styles                  map[string]document.WordStyle
	defaultParagraphStyleID string
	cursor                  *layoutCursor
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
		if style.DefaultParagraph && resolver.defaultParagraphStyleID == "" {
			resolver.defaultParagraphStyleID = style.ID
		}
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
	paragraphStyleID := paragraph.Style
	if paragraphStyleID == "" {
		paragraphStyleID = r.defaultParagraphStyleID
	}
	if paragraphStyleID != "" {
		chain := make([]document.WordStyle, 0, 4)
		seen := map[string]bool{}
		current := paragraphStyleID
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
		if source.Strike {
			mask |= document.WordTextStrikeSet
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
	if mask&document.WordTextStrikeSet != 0 {
		target.Strike = source.Strike
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
