package wordlayout

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"glance/internal/document"
)

const (
	emuPerPoint = 12700.0
	epsilonPT   = 1e-7
)

const (
	DiagnosticWarning DiagnosticSeverity = "warning"
	DiagnosticError   DiagnosticSeverity = "error"
)

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
	preflight := Preflight(input)

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
		SupportReport:     preflight,
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
	if sectionPageSetupsDiffer(input.Layout.Sections, input.DefaultPageSetup) {
		cursor.diagnostic(DiagnosticError, "section-1", "multiple-sections", "Section-specific page layouts are not supported yet.")
	}
	for _, unsupported := range input.Layout.Unsupported {
		cursor.diagnostic(DiagnosticError, unsupported.SourceID, unsupported.Code, unsupported.Description)
	}

	styles := newStyleResolver(input.Layout, cursor)
	sectionStartTypes := make(map[int]string, len(input.Layout.Sections))
	sectionIndexes := make(map[int]int, len(input.Layout.Sections))
	for index, section := range input.Layout.Sections {
		sectionIndexes[section.StartBlock] = index
		if section.StartBlock > 0 && section.StartType != "" {
			sectionStartTypes[section.StartBlock] = strings.ToLower(section.StartType)
		}
	}
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
				cursor.sectionIndex = checkpoint.SectionIndex
				cursor.currentCol = checkpoint.Col
				cursor.y = checkpoint.YPT
				cursor.used = checkpoint.Used
				cursor.pendingBreak = checkpoint.PendingBreak
				cursor.automaticPageStart = checkpoint.AutomaticPageStart
				cursor.hasPreviousParagraph = checkpoint.HasPreviousParagraph
				cursor.previousParagraphAfterPT = checkpoint.PreviousParagraphAfterPT
				cursor.previousParagraphLineGapPT = checkpoint.PreviousParagraphLineGapPT
			}
		}
	}
	lastReportedPages := 0
	reportPartial := func() {
		if input.ProgressiveCallback == nil || ctx.Err() != nil {
			return
		}
		partial := cloneLayoutResult(result)
		if len(partial.Pages) == 0 {
			return
		}
		_ = appendSectionHeaders(ctx, e.shaper, input.Layout, cursor, &partial)
		partial.SupportReport = reportWithDiagnostics(preflight, partial.Diagnostics)
		partial.CanPreview = !partial.SupportReport.BlocksPreview()
		input.ProgressiveCallback(partial)
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
		if sectionIndex, ok := sectionIndexes[blockIndex]; ok {
			section := input.Layout.Sections[sectionIndex]
			if strings.EqualFold(section.StartType, "continuous") && cursor.used && cursor.sectionIndex >= 0 && cursor.sectionIndex < len(input.Layout.Sections) && input.Layout.Sections[cursor.sectionIndex].HeaderText != section.HeaderText {
				cursor.diagnostic(DiagnosticWarning, block.ID, "continuous-section-header-fallback", "Different headers in a continuous section share one page header preview.")
			}
			cursor.beginSection(sectionIndex, block.ID, section.StartType)
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
				if cursor.hasPreviousParagraph && !cursor.pendingBreak {
					groupHeight += paragraphLeadingAdjustment(cursor.previousParagraphAfterPT, cursor.previousParagraphLineGapPT, prepared) - prepared.style.paragraph.BeforePT
				}
				groupEnd := blockIndex
				previousPrepared := prepared
				for nextIndex := blockIndex + 1; nextIndex < len(input.Blocks); nextIndex++ {
					if startType, ok := sectionStartTypes[nextIndex]; ok && startType != "continuous" {
						// A section page break overrides the previous paragraph's
						// keep-with-next constraint; do not move both and then
						// advance again when the new section is placed.
						break
					}
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
					if !preparedHasPageBreak(previousPrepared) && !preparedHasPageBreak(nextPrepared) {
						groupHeight += paragraphSpacingAdjustment(previousPrepared, nextPrepared) - nextPrepared.style.paragraph.BeforePT
					}
					groupEnd = nextIndex
					previousPrepared = nextPrepared
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
			cursor.hasPreviousParagraph = false
			cursor.previousParagraphAfterPT = 0
			cursor.previousParagraphLineGapPT = 0
			if block.Table == nil {
				cursor.diagnostic(DiagnosticError, block.ID, "invalid-table", "The table block has no table data.")
				continue
			}
			if err := cursor.layoutTable(ctx, e.shaper, styles, block.ID, *block.Table, cursor.currentColumnRect(), input.EditorParagraphInsets); err != nil {
				return Result{}, err
			}
		case document.WordUnsupportedBlock:
			cursor.hasPreviousParagraph = false
			cursor.previousParagraphAfterPT = 0
			cursor.previousParagraphLineGapPT = 0
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
		if input.Previous == nil && input.ProgressiveCallback != nil && blockIndex < len(input.Blocks)-1 {
			minInitialPages := 2
			if input.TargetPage > 0 && input.TargetPage < 10 {
				minInitialPages = input.TargetPage + 1
			}
			if lastReportedPages == 0 && len(result.Pages) >= minInitialPages {
				reportPartial()
				lastReportedPages = len(result.Pages)
			} else if lastReportedPages > 0 && len(result.Pages) >= lastReportedPages+10 {
				reportPartial()
				lastReportedPages = len(result.Pages)
			}
			if blockIndex%10 == 0 {
				runtime.Gosched()
			}
		}
	}
	if !converged {
		cursor.currentBlockIndex = len(input.Blocks)
		result.blockCheckpoints[len(input.Blocks)] = cursor.checkpoint()
	}
	if len(result.Pages) == 0 {
		cursor.startPage()
	}
	if err := appendSectionHeaders(ctx, e.shaper, input.Layout, cursor, &result); err != nil {
		return Result{}, err
	}
	result.SupportReport = reportWithDiagnostics(preflight, result.Diagnostics)
	result.CanPreview = !result.SupportReport.BlocksPreview()
	return result, nil
}
