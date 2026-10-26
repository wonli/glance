package desktopui

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"glance/internal/ocr"
	"glance/internal/textalign"
)

type proofreadingOCRCharacterDiffResult struct {
	Matches       int
	AOnly         int
	BOnly         int
	MatchedText   string
	AOnlyText     string
	BOnlyText     string
	HasEvidence   bool
	Complete      bool
	MatchedRanges []textalign.Range
	BOnlyRanges   []textalign.Range
	AOnlyBlockIDs []int
	Frames        []proofreadingOCRDiffFrame
}

type proofreadingOCRDiffSource struct {
	characters []rune
	starts     []int
}

type proofreadingOCRMappedPageText struct {
	text         string
	blockIDs     []int
	blockOffsets []int
}

type proofreadingOCRDiffOpKind uint8

const (
	proofreadingOCRDiffEqual proofreadingOCRDiffOpKind = iota + 1
	proofreadingOCRDiffAOnly
	proofreadingOCRDiffBOnly
	proofreadingOCRDiffReplace
)

type proofreadingOCRDiffOp struct {
	kind proofreadingOCRDiffOpKind
	a    int
	b    int
}

type proofreadingOCRDiffFrame struct {
	Kind      proofreadingOCRDiffOpKind
	ABlockID  int
	AOffset   int
	AChar     rune
	BPosition int
	BChar     rune
}

func newProofreadingOCRDiffSource(value string, exceptions ...map[rune]bool) proofreadingOCRDiffSource {
	var exc map[rune]bool
	if len(exceptions) > 0 {
		exc = exceptions[0]
	}
	runes := []rune(value)
	source := proofreadingOCRDiffSource{
		characters: make([]rune, 0, len(runes)),
		starts:     make([]int, 0, len(runes)),
	}
	for index, current := range runes {
		if unicode.IsSpace(current) {
			continue
		}
		if len(exc) > 0 && exc[current] {
			continue
		}
		source.characters = append(source.characters, current)
		source.starts = append(source.starts, index)
	}
	return source
}

func proofreadingOCRPageText(layout ocr.Layout) proofreadingOCRMappedPageText {
	blocks := append([]ocr.LayoutBlock(nil), layout.Blocks...)
	sort.SliceStable(blocks, func(i, j int) bool {
		left, right := blocks[i].Order, blocks[j].Order
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return *left < *right
	})
	var text strings.Builder
	var blockIDs []int
	var blockOffsets []int
	for _, block := range blocks {
		if proofreadingOCRDiffFurniture(block.Label) || block.Text == "" {
			continue
		}
		if text.Len() > 0 {
			text.WriteRune('\n')
			blockIDs = append(blockIDs, -1)
			blockOffsets = append(blockOffsets, -1)
		}
		for offset, value := range []rune(block.Text) {
			text.WriteRune(value)
			blockIDs = append(blockIDs, block.ID)
			blockOffsets = append(blockOffsets, offset)
		}
	}
	return proofreadingOCRMappedPageText{text: text.String(), blockIDs: blockIDs, blockOffsets: blockOffsets}
}

func proofreadingOCRDiffFurniture(label string) bool {
	switch label {
	case "number", "header", "footer", "header_image", "footer_image":
		return true
	default:
		return false
	}
}

func proofreadingOCRCharacterDiff(layout ocr.Layout, markdown string, globalStart int, exceptions ...map[rune]bool) proofreadingOCRCharacterDiffResult {
	var exc map[rune]bool
	if len(exceptions) > 0 {
		exc = exceptions[0]
	}
	diff := proofreadingOCRCharacterDiffResult{Complete: true}
	aPage := proofreadingOCRPageText(layout)
	aSource := newProofreadingOCRDiffSource(aPage.text, exc)
	bSource := newProofreadingOCRDiffSource(markdown, exc)
	diff.HasEvidence = len(aSource.characters) > 0 || len(bSource.characters) > 0
	if !diff.HasEvidence {
		return diff
	}
	ops, ok := proofreadingOCRDiffOperations(aSource.characters, bSource.characters)
	if !ok {
		diff.Complete = false
		return diff
	}
	aOnlyBlocks := make(map[int]struct{})
	var matchedText, aOnlyText, bOnlyText strings.Builder
	bCursor := 0
	for _, operation := range ops {
		switch operation.kind {
		case proofreadingOCRDiffEqual:
			if globalB, valid := proofreadingOCRDiffGlobalBPosition(globalStart, bSource, operation.b); valid {
				diff.Matches++
				matchedText.WriteRune(bSource.characters[operation.b])
				diff.MatchedRanges = append(diff.MatchedRanges, textalign.Range{Start: globalB, End: globalB + 1})
				diff.Frames = append(diff.Frames, proofreadingOCRDiffFrame{
					Kind:     proofreadingOCRDiffEqual,
					ABlockID: proofreadingOCRDiffABlockID(aPage, aSource, operation.a),
					AOffset:  proofreadingOCRDiffAOffset(aPage, aSource, operation.a),
					AChar:    aSource.characters[operation.a], BPosition: globalB, BChar: bSource.characters[operation.b],
				})
			}
		case proofreadingOCRDiffAOnly:
			bPosition := proofreadingOCRDiffGlobalBBoundary(globalStart, markdown, bSource, bCursor)
			proofreadingOCRAppendAOnly(&diff, &aOnlyText, aOnlyBlocks, aPage, aSource, operation.a, bPosition)
		case proofreadingOCRDiffBOnly:
			diff.appendBOnly(operation.b, globalStart, bSource, &bOnlyText)
		case proofreadingOCRDiffReplace:
			frameStart := len(diff.Frames)
			bPosition := proofreadingOCRDiffGlobalBBoundary(globalStart, markdown, bSource, operation.b)
			proofreadingOCRAppendAOnly(&diff, &aOnlyText, aOnlyBlocks, aPage, aSource, operation.a, bPosition)
			diff.appendBOnly(operation.b, globalStart, bSource, &bOnlyText)
			if len(diff.Frames) == frameStart+2 {
				aFrame, bFrame := diff.Frames[frameStart], diff.Frames[frameStart+1]
				diff.Frames[frameStart] = proofreadingOCRDiffFrame{
					Kind: proofreadingOCRDiffReplace, ABlockID: aFrame.ABlockID, AOffset: aFrame.AOffset, AChar: aFrame.AChar,
					BPosition: bFrame.BPosition, BChar: bFrame.BChar,
				}
				diff.Frames = diff.Frames[:frameStart+1]
			}
		}
		if operation.kind != proofreadingOCRDiffAOnly {
			bCursor = operation.b + 1
		}
	}

	diff.MatchedText = matchedText.String()
	diff.AOnlyText = aOnlyText.String()
	diff.BOnlyText = bOnlyText.String()
	diff.MatchedRanges = mergeProofreadingOCRRanges(diff.MatchedRanges)
	diff.BOnlyRanges = mergeProofreadingOCRRanges(diff.BOnlyRanges)
	for blockID := range aOnlyBlocks {
		diff.AOnlyBlockIDs = append(diff.AOnlyBlockIDs, blockID)
	}
	sort.Ints(diff.AOnlyBlockIDs)
	return diff
}

func proofreadingOCRAppendAOnly(
	diff *proofreadingOCRCharacterDiffResult,
	text *strings.Builder,
	blocks map[int]struct{},
	page proofreadingOCRMappedPageText,
	source proofreadingOCRDiffSource,
	index int,
	bPosition int,
) {
	if index < 0 || index >= len(source.starts) {
		return
	}
	rawIndex := source.starts[index]
	if rawIndex < 0 || rawIndex >= len(page.blockIDs) || page.blockIDs[rawIndex] < 0 {
		return
	}
	blockID := page.blockIDs[rawIndex]
	diff.AOnly++
	text.WriteRune(source.characters[index])
	blocks[blockID] = struct{}{}
	diff.Frames = append(diff.Frames, proofreadingOCRDiffFrame{
		Kind: proofreadingOCRDiffAOnly, ABlockID: blockID, AOffset: page.blockOffsets[rawIndex], AChar: source.characters[index], BPosition: bPosition,
	})
}

func proofreadingOCRDiffABlockID(page proofreadingOCRMappedPageText, source proofreadingOCRDiffSource, index int) int {
	if index < 0 || index >= len(source.starts) {
		return -1
	}
	rawIndex := source.starts[index]
	if rawIndex < 0 || rawIndex >= len(page.blockIDs) {
		return -1
	}
	return page.blockIDs[rawIndex]
}

func proofreadingOCRDiffAOffset(page proofreadingOCRMappedPageText, source proofreadingOCRDiffSource, index int) int {
	if index < 0 || index >= len(source.starts) {
		return -1
	}
	raw := source.starts[index]
	if raw < 0 || raw >= len(page.blockOffsets) {
		return -1
	}
	return page.blockOffsets[raw]
}

func (diff *proofreadingOCRCharacterDiffResult) appendBOnly(index, globalStart int, source proofreadingOCRDiffSource, text *strings.Builder) {
	globalPosition, valid := proofreadingOCRDiffGlobalBPosition(globalStart, source, index)
	if !valid {
		return
	}
	diff.BOnly++
	text.WriteRune(source.characters[index])
	diff.BOnlyRanges = append(diff.BOnlyRanges, textalign.Range{Start: globalPosition, End: globalPosition + 1})
	diff.Frames = append(diff.Frames, proofreadingOCRDiffFrame{
		Kind: proofreadingOCRDiffBOnly, ABlockID: -1, AOffset: -1, BPosition: globalPosition, BChar: source.characters[index],
	})
}

func proofreadingOCRDiffGlobalBPosition(globalStart int, source proofreadingOCRDiffSource, index int) (int, bool) {
	if index < 0 || index >= len(source.starts) {
		return 0, false
	}
	return globalStart + source.starts[index], true
}

func proofreadingOCRDiffGlobalBBoundary(globalStart int, markdown string, source proofreadingOCRDiffSource, index int) int {
	if index >= 0 && index < len(source.starts) {
		return globalStart + source.starts[index]
	}
	return globalStart + utf8.RuneCountInString(markdown)
}

func proofreadingOCRDiffOperations(a, b []rune) ([]proofreadingOCRDiffOp, bool) {
	const maxCells = 4_000_000
	if len(a) == 0 && len(b) == 0 {
		return nil, true
	}
	rows, cols := len(a)+1, len(b)+1
	if rows > maxCells/cols {
		return nil, false
	}
	trace := make([]byte, rows*cols)
	previous, current := make([]int, cols), make([]int, cols)
	for j := 1; j < cols; j++ {
		previous[j] = j
		trace[j] = byte(proofreadingOCRDiffBOnly)
	}
	for i := 1; i < rows; i++ {
		current[0] = i
		trace[i*cols] = byte(proofreadingOCRDiffAOnly)
		for j := 1; j < cols; j++ {
			if a[i-1] == b[j-1] {
				current[j] = previous[j-1]
				trace[i*cols+j] = byte(proofreadingOCRDiffEqual)
				continue
			}
			best, direction := previous[j-1]+1, proofreadingOCRDiffReplace
			if previous[j]+1 < best {
				best, direction = previous[j]+1, proofreadingOCRDiffAOnly
			}
			if current[j-1]+1 < best {
				best, direction = current[j-1]+1, proofreadingOCRDiffBOnly
			}
			current[j] = best
			trace[i*cols+j] = byte(direction)
		}
		previous, current = current, previous
	}

	operations := make([]proofreadingOCRDiffOp, 0, max(len(a), len(b)))
	i, j := len(a), len(b)
	for i > 0 || j > 0 {
		switch proofreadingOCRDiffOpKind(trace[i*cols+j]) {
		case proofreadingOCRDiffEqual:
			operations = append(operations, proofreadingOCRDiffOp{kind: proofreadingOCRDiffEqual, a: i - 1, b: j - 1})
			i--
			j--
		case proofreadingOCRDiffReplace:
			operations = append(operations, proofreadingOCRDiffOp{kind: proofreadingOCRDiffReplace, a: i - 1, b: j - 1})
			i--
			j--
		case proofreadingOCRDiffAOnly:
			operations = append(operations, proofreadingOCRDiffOp{kind: proofreadingOCRDiffAOnly, a: i - 1, b: -1})
			i--
		case proofreadingOCRDiffBOnly:
			operations = append(operations, proofreadingOCRDiffOp{kind: proofreadingOCRDiffBOnly, a: -1, b: j - 1})
			j--
		default:
			return nil, false
		}
	}
	for left, right := 0, len(operations)-1; left < right; left, right = left+1, right-1 {
		operations[left], operations[right] = operations[right], operations[left]
	}
	return operations, true
}
