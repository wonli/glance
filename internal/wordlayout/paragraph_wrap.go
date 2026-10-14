package wordlayout

import (
	"glance/internal/document"
	"math"
	"strings"
)

func wrapParagraphAtoms(atoms []paragraphAtom, maxWidth float64, paragraph document.WordParagraphLayout, defaultTabStopPT float64, gridPitch ...float64) []paragraphLine {
	firstWidth := math.Max(1, maxWidth-emuToPoints(paragraph.FirstLineIndentEMU-paragraph.HangingIndentEMU))
	return wrapParagraphAtomsWidths(atoms, firstWidth, maxWidth, paragraph, defaultTabStopPT, 0, gridPitch...)
}

func wrapParagraphAtomsLimit(atoms []paragraphAtom, maxWidth float64, paragraph document.WordParagraphLayout, defaultTabStopPT float64, lineLimit int, gridPitch ...float64) []paragraphLine {
	return wrapParagraphAtomsWidths(atoms, maxWidth, maxWidth, paragraph, defaultTabStopPT, lineLimit, gridPitch...)
}

func wrapParagraphAtomsWidths(atoms []paragraphAtom, firstWidth, maxWidth float64, paragraph document.WordParagraphLayout, defaultTabStopPT float64, lineLimit int, gridPitch ...float64) []paragraphLine {
	lines := make([]paragraphLine, 0, 2)
	current := make([]paragraphAtom, 0, 8)
	lineWidth := func() float64 {
		if len(lines) == 0 {
			return firstWidth
		}
		return maxWidth
	}
	pendingPageBreak := false
	stopped := false
	flush := func(pageBreakAfter bool, nextAtomIndex int) {
		if len(current) == 0 {
			return
		}
		line := paragraphLine{atoms: append([]paragraphAtom(nil), current...), pageBreakBefore: pendingPageBreak, pageBreakAfter: pageBreakAfter, nextAtomIndex: nextAtomIndex}
		pendingPageBreak = false
		for _, atom := range current {
			line.width += atom.advance
			if atom.ascent > line.ascent {
				line.ascent = atom.ascent
			}
			if atom.descent > line.descent {
				line.descent = atom.descent
			}
		}
		for _, atom := range current {
			line.gap = math.Max(line.gap, atom.gap)
		}
		line.height = lineHeight(line, paragraph, gridPitch...)
		lines = append(lines, line)
		current = current[:0]
		stopped = lineLimit > 0 && len(lines) >= lineLimit
	}
	for atomIndex, atom := range atoms {
		if stopped {
			break
		}
		if atom.kind == atomLineBreak {
			flush(false, atom.sourceIndex+1)
			continue
		}
		if atom.kind == atomPageBreak {
			if len(current) > 0 {
				flush(true, atom.sourceIndex+1)
			} else if len(lines) > 0 {
				lines[len(lines)-1].pageBreakAfter = true
				lines[len(lines)-1].nextAtomIndex = atom.sourceIndex + 1
			} else {
				pendingPageBreak = true
			}
			continue
		}
		if atom.kind == atomTab {
			atom.advance = paragraphTabAdvance(sumAdvance(current), atoms[atomIndex+1:], paragraph.TabStops, defaultTabStopPT, lineWidth())
		}
		if len(current) > 0 && sumAdvance(current)+atom.advance > lineWidth()+epsilonPT {
			if atom.breakBefore {
				flush(false, atom.sourceIndex)
				if stopped {
					break
				}
				if atom.kind == atomTab {
					atom.advance = paragraphTabAdvance(0, atoms[atomIndex+1:], paragraph.TabStops, defaultTabStopPT, lineWidth())
				}
			} else {
				breakAt := -1
				for index := len(current) - 1; index >= 0; index-- {
					if current[index].breakAfter {
						breakAt = index + 1
						break
					}
				}
				if breakAt > 0 && breakAt < len(current) {
					carry := append([]paragraphAtom(nil), current[breakAt:]...)
					nextAtomIndex := current[breakAt-1].sourceIndex + 1
					current = current[:breakAt]
					flush(false, nextAtomIndex)
					if stopped {
						break
					}
					current = append(current, carry...)
				} else {
					nextAtomIndex := current[len(current)-1].sourceIndex + 1
					flush(false, nextAtomIndex)
					if stopped {
						break
					}
				}
			}
		}
		current = append(current, atom)
	}
	if !stopped {
		nextAtomIndex := len(atoms)
		if len(current) > 0 {
			nextAtomIndex = current[len(current)-1].sourceIndex + 1
		}
		flush(false, nextAtomIndex)
	}
	if pendingPageBreak && len(lines) > 0 {
		lines[len(lines)-1].pageBreakAfter = true
	}
	return lines
}

func paragraphTabAdvance(currentWidth float64, following []paragraphAtom, stops []document.WordTabStop, defaultStopPT, maxWidth float64) float64 {
	if defaultStopPT <= 0 || !finite(defaultStopPT) {
		defaultStopPT = 36
	}
	if currentWidth <= epsilonPT {
		for _, atom := range following {
			if atom.kind == atomTab {
				// Generated TOC paragraphs commonly start with a default indent
				// tab and use a later explicit right tab for the page number.
				return math.Min(defaultStopPT, maxWidth)
			}
			if atom.kind == atomPageBreak || atom.kind == atomLineBreak {
				break
			}
		}
	}
	stopPosition := (math.Floor(currentWidth/defaultStopPT) + 1) * defaultStopPT
	alignment := "left"
	foundExplicitStop := false
	for _, stop := range stops {
		if stop.PositionPT <= currentWidth+epsilonPT || stop.PositionPT < 0 || !finite(stop.PositionPT) {
			continue
		}
		if !foundExplicitStop || stop.PositionPT < stopPosition {
			stopPosition = stop.PositionPT
			alignment = stop.Alignment
			foundExplicitStop = true
		}
	}
	stopPosition = math.Min(stopPosition, maxWidth)
	followingWidth := 0.0
	decimalWidth := 0.0
	decimalFound := false
	for _, atom := range following {
		if atom.kind == atomTab || atom.kind == atomPageBreak || atom.kind == atomLineBreak {
			break
		}
		followingWidth += atom.advance
		if !decimalFound && atom.kind == atomText {
			for _, r := range atom.text {
				if r == '.' || r == ',' {
					decimalFound = true
					break
				}
				decimalWidth += atom.advance / math.Max(1, float64(len([]rune(atom.text))))
			}
		}
	}
	anchorWidth := 0.0
	switch strings.ToLower(alignment) {
	case "right", "end":
		anchorWidth = followingWidth
	case "center":
		anchorWidth = followingWidth / 2
	case "decimal":
		if decimalFound {
			anchorWidth = decimalWidth
		} else {
			anchorWidth = followingWidth
		}
	}
	return math.Max(0, stopPosition-currentWidth-anchorWidth)
}

func sumAdvance(atoms []paragraphAtom) float64 {
	width := 0.0
	for _, atom := range atoms {
		width += atom.advance
	}
	return width
}

func lineHeight(line paragraphLine, paragraph document.WordParagraphLayout, gridPitch ...float64) float64 {
	base := line.ascent + line.descent + math.Max(0, line.gap)
	if base <= 0 {
		base = 11
	}
	height := base
	switch paragraph.LineSpacingRule {
	case "exact":
		if paragraph.LineSpacingPT > 0 {
			return paragraph.LineSpacingPT
		}
	case "atleast":
		height = math.Max(base, paragraph.LineSpacingPT)
	case "auto", "":
		if paragraph.LineSpacingMultiple > 0 {
			height = base * paragraph.LineSpacingMultiple
		}
	default:
		if paragraph.LineSpacingPT > 0 {
			height = math.Max(base, paragraph.LineSpacingPT)
		}
	}
	if len(gridPitch) > 0 && finite(gridPitch[0]) && gridPitch[0] > 0 && !(paragraph.Set&document.WordParagraphSnapToGridSet != 0 && !paragraph.SnapToGrid) {
		height = math.Ceil((height-epsilonPT)/gridPitch[0]) * gridPitch[0]
	}
	return height
}
