package wordlayout

import "math"

func preparedParagraphsHeight(paragraphs []preparedParagraph) float64 {
	height := 0.0
	for index, paragraph := range paragraphs {
		height += paragraph.totalHeight()
		if index == 0 || preparedHasPageBreak(paragraphs[index-1]) || preparedHasPageBreak(paragraph) {
			continue
		}
		height += paragraphSpacingAdjustment(paragraphs[index-1], paragraph) - paragraph.style.paragraph.BeforePT
	}
	return height
}

// Paragraph gaps include the inter-line whitespace already reserved below the
// preceding line. Count the largest requirement once, rather than adding the
// paragraph spacing to a multiple-spaced line box.
func paragraphLeadingAdjustment(previousAfterPT, previousLineGapPT float64, paragraph preparedParagraph) float64 {
	beforePT := paragraph.style.paragraph.BeforePT
	nextGap := paragraphLineGap(paragraph, false)
	gap := math.Max(math.Max(previousAfterPT, beforePT), math.Max(previousLineGapPT, nextGap))
	return gap - previousAfterPT - previousLineGapPT
}

func paragraphSpacingAdjustment(previous, next preparedParagraph) float64 {
	return paragraphLeadingAdjustment(previous.style.paragraph.AfterPT, paragraphLineGap(previous, true), next)
}

func paragraphLineGap(paragraph preparedParagraph, last bool) float64 {
	if len(paragraph.lines) == 0 {
		return 0
	}
	index := 0
	if last {
		index = len(paragraph.lines) - 1
	}
	line := paragraph.lines[index]
	return math.Max(0, line.height-line.ascent-line.descent)
}
