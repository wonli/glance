package desktopui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/proofreading"
)

type proofreadingMarkdownSourceSegment struct {
	projectionStart int
	projectionEnd   int
	sourceStart     int
	direct          bool
	math            bool
}

type proofreadingMarkdownLocalSegment struct {
	start  int
	end    int
	source int
	direct bool
	math   bool
}

type proofreadingMarkdownSourceEdit struct {
	start int
	end   int
	text  []rune
}

type proofreadingMarkdownSelectedItem struct {
	originalStart uint64
	originalEnd   uint64
	inserted      string
}

func proofreadingMarkdownProjection(source string) (string, []proofreadingMarkdownSourceSegment, error) {
	parsed, err := markdowndoc.Parse(source)
	if err != nil {
		return "", nil, err
	}
	units := proofreadingMarkdownContentUnits(parsed)
	expected, _ := proofreadingProjectionContent(units)
	sourceRunes := []rune(source)
	var projection strings.Builder
	segments := make([]proofreadingMarkdownSourceSegment, 0, len(parsed.Blocks)*2)
	projectionOffset := 0
	appendUnit := func(kind proofreadingUnitKind, text string, local []proofreadingMarkdownLocalSegment) {
		value := strings.ReplaceAll(text, "\r\n", "\n")
		value = strings.ReplaceAll(value, "\r", "\n")
		if kind == proofreadingUnitTableRow {
			value = strings.ReplaceAll(value, "\t", "│")
		} else {
			value = strings.ReplaceAll(value, "\t", " ")
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if projectionOffset > 0 {
			projection.WriteByte('\n')
			projectionOffset++
		}
		leading := len([]rune(value)) - len([]rune(strings.TrimLeftFunc(value, unicode.IsSpace)))
		trimmedLength := utf8.RuneCountInString(trimmed)
		// Tab replacement changes the glyph, but not its rune position. Only
		// CRLF normalization changes the positions of following characters.
		preserveOffsets := !strings.Contains(text, "\r\n")
		for _, item := range local {
			start := max(item.start, leading)
			end := min(item.end, leading+trimmedLength)
			if start >= end {
				continue
			}
			segments = append(segments, proofreadingMarkdownSourceSegment{
				projectionStart: projectionOffset + start - leading,
				projectionEnd:   projectionOffset + end - leading,
				sourceStart:     item.source + start - item.start,
				direct:          item.direct && preserveOffsets,
				math:            item.math,
			})
		}
		projection.WriteString(trimmed)
		projectionOffset += trimmedLength
	}
	for _, block := range parsed.Blocks {
		if block.Kind == "table" {
			for rowIndex, row := range block.Rows {
				var text strings.Builder
				local := make([]proofreadingMarkdownLocalSegment, 0, len(block.TableSpans[rowIndex])*2)
				runeOffset := 0
				for cellIndex, cell := range row {
					if cellIndex > 0 {
						text.WriteByte('\t')
						runeOffset++
					}
					text.WriteString(cell)
					spanOffset := runeOffset
					for _, span := range block.TableSpans[rowIndex][cellIndex] {
						if segment, ok := proofreadingMarkdownSpanSourceSegment(span, spanOffset, sourceRunes); ok {
							local = append(local, segment)
						}
						spanOffset += utf8.RuneCountInString(span.Text)
					}
					runeOffset += utf8.RuneCountInString(cell)
				}
				appendUnit(proofreadingUnitTableRow, text.String(), local)
			}
			continue
		}
		text := block.Text
		if block.Prefix != "" {
			text = block.Prefix + text
		}
		if block.Kind == "rule" {
			text = "────────"
		}
		var local []proofreadingMarkdownLocalSegment
		prefixLength := utf8.RuneCountInString(block.Prefix)
		if block.Kind == "rule" {
			local = nil
		} else if len(block.Spans) > 0 {
			spanText := strings.Builder{}
			spanOffset := prefixLength
			for _, span := range block.Spans {
				spanText.WriteString(span.Text)
				if segment, ok := proofreadingMarkdownSpanSourceSegment(span, spanOffset, sourceRunes); ok {
					local = append(local, segment)
				}
				spanOffset += utf8.RuneCountInString(span.Text)
			}
			if spanText.String() != block.Text {
				local = nil
			}
		} else if block.SourceEnd > block.SourceStart && block.SourceStart >= 0 && block.SourceEnd <= len(sourceRunes) {
			contentRunes := []rune(block.Text)
			if len(contentRunes) == block.SourceEnd-block.SourceStart && string(sourceRunes[block.SourceStart:block.SourceEnd]) == block.Text {
				local = append(local, proofreadingMarkdownLocalSegment{start: prefixLength, end: prefixLength + len(contentRunes), source: block.SourceStart, direct: true, math: block.Kind == "code" || block.Kind == "diagram"})
			} else if block.Kind == "math" {
				inner := string(sourceRunes[block.SourceStart:block.SourceEnd])
				if strings.HasPrefix(inner, "$$") && strings.HasSuffix(inner, "$$") {
					inner = inner[2 : len(inner)-2]
					if at := strings.Index(inner, block.Text); at >= 0 {
						local = append(local, proofreadingMarkdownLocalSegment{start: prefixLength, end: prefixLength + len(contentRunes), source: block.SourceStart + 2 + utf8.RuneCountInString(inner[:at]), direct: true, math: true})
					}
				}
			}
		}
		appendUnit(proofreadingUnitBlock, text, local)
	}
	if got := projection.String(); got != expected {
		return "", nil, errors.New("Markdown projection differs from the proofreading source text")
	}
	return expected, segments, nil
}

func proofreadingMarkdownSpanSourceSegment(span document.MarkdownSpan, localOffset int, source []rune) (proofreadingMarkdownLocalSegment, bool) {
	length := utf8.RuneCountInString(span.Text)
	if length == 0 || span.SourceStart < 0 || span.SourceEnd <= span.SourceStart || span.SourceEnd > len(source) {
		return proofreadingMarkdownLocalSegment{}, false
	}
	if span.Kind == "math" && span.SourceEnd-span.SourceStart >= length+2 && source[span.SourceStart] == '$' && source[span.SourceEnd-1] == '$' {
		inner := string(source[span.SourceStart+1 : span.SourceEnd-1])
		at := strings.Index(inner, span.Text)
		if at < 0 {
			return proofreadingMarkdownLocalSegment{}, false
		}
		innerOffset := utf8.RuneCountInString(inner[:at])
		return proofreadingMarkdownLocalSegment{
			start: localOffset, end: localOffset + length,
			source: span.SourceStart + 1 + innerOffset, direct: true, math: true,
		}, true
	}
	if span.SourceEnd-span.SourceStart != length || string(source[span.SourceStart:span.SourceEnd]) != span.Text {
		return proofreadingMarkdownLocalSegment{}, false
	}
	return proofreadingMarkdownLocalSegment{
		start: localOffset, end: localOffset + length,
		source: span.SourceStart, direct: true, math: span.Kind == "code",
	}, true
}

func proofreadingMarkdownSourceOffset(segments []proofreadingMarkdownSourceSegment, projectionOffset int) (int, bool, bool) {
	index := sort.Search(len(segments), func(i int) bool { return segments[i].projectionEnd > projectionOffset })
	if index >= len(segments) || projectionOffset < segments[index].projectionStart || !segments[index].direct {
		return 0, false, false
	}
	segment := segments[index]
	return segment.sourceStart + projectionOffset - segment.projectionStart, segment.math, true
}

func preserveProofreadingMarkdownFormatting(source, originalText string, selectedRuns []proofreading.Run) (string, error) {
	projection, segments, err := proofreadingMarkdownProjection(source)
	if err != nil {
		return "", fmt.Errorf("read Markdown formatting map: %w", err)
	}
	if projection != originalText {
		return "", errors.New("retained Markdown no longer matches the proofreading source text")
	}
	originalLength := uint64(utf8.RuneCountInString(originalText))
	items := make([]proofreadingMarkdownSelectedItem, 0, len(selectedRuns))
	var selectedText strings.Builder
	var previousEnd uint64
	for _, run := range selectedRuns {
		runes := []rune(run.Text)
		if len(runes) == 0 {
			continue
		}
		first := uint64(run.First)
		end := first + uint64(len(runes))
		if first == 0 || end <= first {
			return "", errors.New("proofreading export contains an invalid character identity range")
		}
		if first <= originalLength {
			originalEnd := min(end, originalLength+1)
			if first < previousEnd {
				return "", errors.New("proofreading export changed original character order")
			}
			count := int(originalEnd - first)
			items = append(items, proofreadingMarkdownSelectedItem{originalStart: first, originalEnd: originalEnd})
			selectedText.WriteString(string(runes[:count]))
			previousEnd = originalEnd
			if end > originalEnd {
				inserted := string(runes[count:])
				items = append(items, proofreadingMarkdownSelectedItem{inserted: inserted})
				selectedText.WriteString(inserted)
			}
		} else {
			inserted := string(runes)
			items = append(items, proofreadingMarkdownSelectedItem{inserted: inserted})
			selectedText.WriteString(inserted)
		}
	}
	if selectedText.String() == originalText {
		return source, nil
	}
	retained := make([][2]uint64, 0, len(items))
	for _, item := range items {
		if item.originalStart == 0 {
			continue
		}
		retained = append(retained, [2]uint64{item.originalStart, item.originalEnd})
	}
	var edits []proofreadingMarkdownSourceEdit
	addDeletion := func(first, end uint64) error {
		for id := first; id < end; id++ {
			sourceOffset, _, ok := proofreadingMarkdownSourceOffset(segments, int(id-1))
			if !ok {
				return fmt.Errorf("cannot preserve Markdown formatting for edited source character %d", id)
			}
			edits = append(edits, proofreadingMarkdownSourceEdit{start: sourceOffset, end: sourceOffset + 1})
		}
		return nil
	}
	cursor := uint64(1)
	for _, interval := range retained {
		if interval[0] > cursor {
			if err := addDeletion(cursor, interval[0]); err != nil {
				return "", err
			}
		}
		cursor = interval[1]
	}
	if cursor <= originalLength {
		if err := addDeletion(cursor, originalLength+1); err != nil {
			return "", err
		}
	}
	previousOriginalID := uint64(0)
	for itemIndex, item := range items {
		if item.originalStart != 0 {
			previousOriginalID = item.originalEnd - 1
			continue
		}
		if item.inserted == "" {
			continue
		}
		var nextOriginalID uint64
		for next := itemIndex + 1; next < len(items); next++ {
			if items[next].originalStart != 0 {
				nextOriginalID = items[next].originalStart
				break
			}
		}
		anchorID := uint64(0)
		atEnd := false
		if previousOriginalID+1 < nextOriginalID && previousOriginalID < originalLength {
			anchorID = previousOriginalID + 1
		} else if nextOriginalID != 0 {
			anchorID = nextOriginalID
		} else if previousOriginalID > 0 {
			anchorID, atEnd = previousOriginalID, true
		} else if originalLength > 0 {
			anchorID = 1
		}
		if anchorID == 0 {
			return "", errors.New("cannot locate an insertion point in the original Markdown")
		}
		sourceOffset, mathContext, ok := proofreadingMarkdownSourceOffset(segments, int(anchorID-1))
		// A paragraph separator or list prefix belongs to the projection, not
		// the source text. Anchor an insertion to the adjacent retained text
		// instead, keeping it inside that text's original formatting wrapper.
		if !ok && previousOriginalID > 0 {
			sourceOffset, mathContext, ok = proofreadingMarkdownSourceOffset(segments, int(previousOriginalID-1))
			if ok {
				atEnd = true
			}
		}
		if !ok && nextOriginalID > 0 {
			sourceOffset, mathContext, ok = proofreadingMarkdownSourceOffset(segments, int(nextOriginalID-1))
			atEnd = false
		}
		if !ok {
			return "", fmt.Errorf("cannot preserve Markdown formatting near source character %d", anchorID)
		}
		if atEnd {
			sourceOffset++
		}
		inserted := []rune(item.inserted)
		if !mathContext {
			inserted = []rune(escapeProofreadingMarkdownText(item.inserted))
		}
		edits = append(edits, proofreadingMarkdownSourceEdit{start: sourceOffset, end: sourceOffset, text: inserted})
	}
	return applyProofreadingMarkdownSourceEdits(source, edits)
}

func escapeProofreadingMarkdownText(value string) string {
	const punctuation = `\\` + "`*_{}[]<>()#+-.!|$"
	var escaped strings.Builder
	for _, r := range value {
		if r < utf8.RuneSelf && strings.ContainsRune(punctuation, r) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

func applyProofreadingMarkdownSourceEdits(source string, edits []proofreadingMarkdownSourceEdit) (string, error) {
	if len(edits) == 0 {
		return source, nil
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		// Insert replacement text before deleting the source rune at the same
		// offset. Stable ordering also keeps adjacent inserted runs in order.
		return edits[i].end < edits[j].end
	})
	sourceRunes := []rune(source)
	var output strings.Builder
	cursor := 0
	for _, edit := range edits {
		if edit.start < cursor || edit.end < edit.start || edit.end > len(sourceRunes) {
			return "", errors.New("Markdown source edits overlap or exceed the retained source")
		}
		output.WriteString(string(sourceRunes[cursor:edit.start]))
		output.WriteString(string(edit.text))
		cursor = edit.end
	}
	output.WriteString(string(sourceRunes[cursor:]))
	return output.String(), nil
}
