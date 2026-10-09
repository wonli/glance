package wordlayout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"glance/internal/document"
	"math"
)

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
	if block.Paragraph != nil && (block.Paragraph.Layout.KeepWithNext || block.Paragraph.Layout.BottomBorder.SizePT > 0) {
		return true
	}
	return blockHasFloatingImage(block)
}

func hasFloatingImage(blocks []document.WordBlock) bool {
	for _, block := range blocks {
		if blockHasFloatingImage(block) {
			return true
		}
	}
	return false
}

func blockHasFloatingImage(block document.WordBlock) bool {
	if block.Paragraph != nil && paragraphHasFloatingImage(*block.Paragraph) {
		return true
	}
	if block.Table != nil {
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for _, paragraph := range cell.Paragraphs {
					if paragraphHasFloatingImage(paragraph) {
						return true
					}
				}
			}
		}
	}
	return false
}

func paragraphHasFloatingImage(paragraph document.WordParagraph) bool {
	for _, inline := range paragraph.Inlines {
		if inline.Image != nil && inline.Image.Floating != nil {
			return true
		}
	}
	return false
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
	if hasFloatingImage(input.Blocks) {
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
	return old.PageIndex >= 0 && cursor.used == old.Used && cursor.pendingBreak == old.PendingBreak && cursor.automaticPageStart == old.AutomaticPageStart &&
		cursor.hasPreviousParagraph == old.HasPreviousParagraph &&
		math.Abs(cursor.previousParagraphAfterPT-old.PreviousParagraphAfterPT) <= 1e-4 &&
		math.Abs(cursor.previousParagraphLineGapPT-old.PreviousParagraphLineGapPT) <= 1e-4 &&
		math.Abs(cursor.y-old.YPT) <= 1e-4
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
