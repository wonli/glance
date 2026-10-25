package desktopui

import (
	"sort"
	"strings"
	"unicode"

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
}

type proofreadingOCRDiffSource struct {
	characters []rune
	starts     []int
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

type proofreadingOCRAOnlyPosition struct {
	blockID int
	start   int
}

func newProofreadingOCRDiffSource(value string) proofreadingOCRDiffSource {
	runes := []rune(value)
	source := proofreadingOCRDiffSource{
		characters: make([]rune, 0, len(runes)),
		starts:     make([]int, 0, len(runes)),
	}
	for index, current := range runes {
		if unicode.IsSpace(current) {
			continue
		}
		source.characters = append(source.characters, current)
		source.starts = append(source.starts, index)
	}
	return source
}

func proofreadingOCRCharacterDiff(layout ocr.Layout, result textalign.Result, markdown string, globalStart int) proofreadingOCRCharacterDiffResult {
	diff := proofreadingOCRCharacterDiffResult{Complete: true}
	candidate := proofreadingOCRComparisonCandidate(result)
	if candidate == nil || len(candidate.Evidence) == 0 {
		diff.Complete = false
		return diff
	}

	blocks := make(map[int]ocr.LayoutBlock, len(layout.Blocks))
	for _, block := range layout.Blocks {
		blocks[block.ID] = block
	}
	markdownRunes := []rune(markdown)
	seenEvidence := make(map[proofreadingOCRStepEvidenceKey]struct{}, len(candidate.Evidence))
	seenBPositions := make(map[int]struct{})
	seenAPositions := make(map[proofreadingOCRAOnlyPosition]struct{})
	aOnlyBlocks := make(map[int]struct{})
	var matchedText, aOnlyText, bOnlyText strings.Builder

	for _, evidence := range candidate.Evidence {
		key := proofreadingOCRStepEvidenceKey{blockID: evidence.BlockID, aRange: evidence.QueryRange, bRange: evidence.Range}
		if _, duplicate := seenEvidence[key]; duplicate {
			continue
		}
		seenEvidence[key] = struct{}{}

		block, ok := blocks[evidence.BlockID]
		if !ok {
			diff.Complete = false
			continue
		}
		querySource, err := textalign.NewSource(block.Text)
		if err != nil || evidence.QueryRange.Start < 0 || evidence.QueryRange.End <= evidence.QueryRange.Start || evidence.QueryRange.End > len(querySource.Text) {
			diff.Complete = false
			continue
		}
		aRange := querySource.OriginalRange(evidence.QueryRange.Start, evidence.QueryRange.End)
		localBRange := textalign.Range{Start: evidence.Range.Start - globalStart, End: evidence.Range.End - globalStart}
		if aRange.Start < 0 || aRange.End > len([]rune(block.Text)) || localBRange.Start < 0 || localBRange.End <= localBRange.Start || localBRange.End > len(markdownRunes) {
			diff.Complete = false
			continue
		}

		aRunes := []rune(block.Text)
		aStart, aEnd := expandProofreadingOCRDiffSpan(aRunes, aRange.Start, aRange.End)
		bStart, bEnd := expandProofreadingOCRDiffSpan(markdownRunes, localBRange.Start, localBRange.End)
		aSource := newProofreadingOCRDiffSource(string(aRunes[aStart:aEnd]))
		bSource := newProofreadingOCRDiffSource(string(markdownRunes[bStart:bEnd]))
		ops, ok := proofreadingOCRDiffOperations(aSource.characters, bSource.characters)
		if !ok {
			diff.Complete = false
			continue
		}
		diff.HasEvidence = true
		var matchedInEvidence, aOnlyInEvidence, bOnlyInEvidence strings.Builder
		for _, operation := range ops {
			switch operation.kind {
			case proofreadingOCRDiffEqual:
				if globalBStart, valid := proofreadingOCRDiffGlobalBPosition(globalStart, bStart, bSource, operation.b); valid {
					if _, duplicate := seenBPositions[globalBStart]; !duplicate {
						seenBPositions[globalBStart] = struct{}{}
						diff.Matches++
						matchedInEvidence.WriteRune(bSource.characters[operation.b])
						diff.MatchedRanges = append(diff.MatchedRanges, textalign.Range{Start: globalBStart, End: globalBStart + 1})
					}
				}
			case proofreadingOCRDiffAOnly:
				proofreadingOCRAppendAOnly(&diff, &aOnlyInEvidence, aOnlyBlocks, seenAPositions, evidence.BlockID, aStart, aSource, operation.a)
			case proofreadingOCRDiffBOnly:
				diff.appendBOnly(operation.b, globalStart, bStart, bSource, &bOnlyInEvidence, seenBPositions)
			case proofreadingOCRDiffReplace:
				proofreadingOCRAppendAOnly(&diff, &aOnlyInEvidence, aOnlyBlocks, seenAPositions, evidence.BlockID, aStart, aSource, operation.a)
				diff.appendBOnly(operation.b, globalStart, bStart, bSource, &bOnlyInEvidence, seenBPositions)
			}
		}
		appendProofreadingOCRDiffEvidenceText(&matchedText, matchedInEvidence.String())
		appendProofreadingOCRDiffEvidenceText(&aOnlyText, aOnlyInEvidence.String())
		appendProofreadingOCRDiffEvidenceText(&bOnlyText, bOnlyInEvidence.String())
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

type proofreadingOCRStepEvidenceKey struct {
	blockID int
	aRange  textalign.Range
	bRange  textalign.Range
}

func appendProofreadingOCRDiffEvidenceText(target *strings.Builder, value string) {
	if value == "" {
		return
	}
	if target.Len() > 0 {
		target.WriteString("\n")
	}
	target.WriteString(value)
}

func proofreadingOCRAppendAOnly(
	diff *proofreadingOCRCharacterDiffResult,
	text *strings.Builder,
	blocks map[int]struct{},
	seen map[proofreadingOCRAOnlyPosition]struct{},
	blockID, sourceStart int,
	source proofreadingOCRDiffSource,
	index int,
) {
	if index < 0 || index >= len(source.starts) {
		return
	}
	key := proofreadingOCRAOnlyPosition{blockID: blockID, start: sourceStart + source.starts[index]}
	if _, duplicate := seen[key]; duplicate {
		return
	}
	seen[key] = struct{}{}
	diff.AOnly++
	text.WriteRune(source.characters[index])
	blocks[blockID] = struct{}{}
}

func (diff *proofreadingOCRCharacterDiffResult) appendBOnly(index, globalStart, sourceStart int, source proofreadingOCRDiffSource, text *strings.Builder, seen map[int]struct{}) {
	globalPosition, valid := proofreadingOCRDiffGlobalBPosition(globalStart, sourceStart, source, index)
	if !valid {
		return
	}
	if _, duplicate := seen[globalPosition]; duplicate {
		return
	}
	seen[globalPosition] = struct{}{}
	diff.BOnly++
	text.WriteRune(source.characters[index])
	diff.BOnlyRanges = append(diff.BOnlyRanges, textalign.Range{Start: globalPosition, End: globalPosition + 1})
}

func proofreadingOCRDiffGlobalBPosition(globalStart, sourceStart int, source proofreadingOCRDiffSource, index int) (int, bool) {
	if index < 0 || index >= len(source.starts) {
		return 0, false
	}
	return globalStart + sourceStart + source.starts[index], true
}

func expandProofreadingOCRDiffSpan(value []rune, start, end int) (int, int) {
	if start < 0 || end <= start || end > len(value) {
		return start, end
	}
	for start > 0 && proofreadingOCRDiffBoundaryPunctuation(value[start-1]) {
		start--
	}
	for end < len(value) && proofreadingOCRDiffBoundaryPunctuation(value[end]) {
		end++
	}
	return start, end
}

func proofreadingOCRDiffBoundaryPunctuation(value rune) bool {
	return !unicode.IsSpace(value) && !unicode.IsLetter(value) && !unicode.IsNumber(value)
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
