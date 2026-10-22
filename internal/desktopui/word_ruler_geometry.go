package desktopui

import (
	"math"

	"gioui.org/layout"
	"gioui.org/unit"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

const wordEMUPerDP = int64(9525)
const wordEMUPerCM = int64(360000)

type wordRulerGeometry struct {
	cellLimited                              bool
	minimumLeftEMU, minimumRightEMU          int64
	pageLeft, pageRight, textLeft, textRight int
	trackLeft, trackRight                    int
	pixelsPerEMU                             float64
	canonical                                bool
}

func wordRulerGeometryForState(gtx layout.Context, state *documentSessionState) wordRulerGeometry {
	width := gtx.Constraints.Max.X
	if result := state.wordLayoutResult; result != nil && result.CanPreview && len(result.Pages) > 0 {
		columns := 1
		if state.wordViewMode == wordViewPrintLayout && state.wordMultiplePagesMode {
			columns = 2
		}
		index := max(0, min(state.wordPageListState.Position.First*columns, len(result.Pages)-1))
		page := result.Pages[index]
		scale := wordScenePixelsPerPoint(gtx, page, state.wordLayoutZoom)
		pageWidth := int(math.Ceil(page.Clip.WidthPT * scale))
		total := pageWidth
		if columns == 2 && index+1 < len(result.Pages) {
			next := result.Pages[index+1]
			total += gtx.Dp(unit.Dp(wordMultiplePageGapDP)) + int(math.Ceil(next.Clip.WidthPT*scale))
		}
		scroll := math.Max(0, math.Min(float64(max(0, total-width)), state.wordPageScrollX))
		left := max(0, (width-total)/2) - int(math.Round(scroll))
		// Follow the caret, including a run that continues onto the paired page.
		active := wordRulerSourcePosition(state)
		pos, hasCaret := result.Locations[active]
		if !hasCaret {
			if p, ok := wordRulerParagraphForState(state); ok {
				for _, inline := range p.Inlines {
					if inline.Text != nil {
						active = wordlayout.SourcePosition{RunID: inline.Text.ID}
						pos, hasCaret = result.Locations[active]
						if hasCaret {
							break
						}
					}
				}
			}
		}
		if columns == 2 && index+1 < len(result.Pages) && hasCaret && pos.PageIndex == index+1 {
			left += pageWidth + gtx.Dp(unit.Dp(wordMultiplePageGapDP))
			page = result.Pages[index+1]
			scale = wordScenePixelsPerPoint(gtx, page, state.wordLayoutZoom)
			pageWidth = int(math.Ceil(page.Clip.WidthPT * scale))
		}
		markerArea := page.Content
		cellBounds, inCell := wordRulerSelectedCellBounds(state, page)
		if inCell {
			markerArea = wordlayout.TableCellTextArea(cellBounds)
		} else {
			matched := false
			for _, fragment := range page.Text {
				if fragment.RunID == active.RunID && fragment.RuneStart <= active.RuneOffset && active.RuneOffset < fragment.RuneEnd && fragment.ParagraphArea.WidthPT > 0 {
					markerArea = fragment.ParagraphArea
					matched = true
					break
				}
			}
			if !matched {
				for _, fragment := range page.Text {
					if fragment.RunID == active.RunID && fragment.RuneStart <= active.RuneOffset && active.RuneOffset <= fragment.RuneEnd && fragment.ParagraphArea.WidthPT > 0 {
						markerArea = fragment.ParagraphArea
						matched = true
						break
					}
				}
			}
			if !matched {
				if p, ok := wordRulerParagraphForState(state); ok {
					for _, inline := range p.Inlines {
						if inline.Text == nil {
							continue
						}
						for _, fragment := range page.Text {
							if fragment.RunID == inline.Text.ID && fragment.ParagraphArea.WidthPT > 0 {
								markerArea = fragment.ParagraphArea
								matched = true
								break
							}
						}
						if matched {
							break
						}
					}
				}
			}
		}
		g := wordRulerGeometry{
			pageLeft: left, pageRight: left + pageWidth,
			textLeft:     left + int(math.Round(markerArea.XPT*scale)),
			textRight:    left + int(math.Round((markerArea.XPT+markerArea.WidthPT)*scale)),
			trackLeft:    left + int(math.Round(page.Content.XPT*scale)),
			trackRight:   left + int(math.Round((page.Content.XPT+page.Content.WidthPT)*scale)),
			pixelsPerEMU: scale / 12700, canonical: true,
		}
		if inCell {
			g.cellLimited = true
			g.minimumLeftEMU = int64(math.Round((cellBounds.XPT - markerArea.XPT) * 12700))
			g.minimumRightEMU = int64(math.Round((markerArea.XPT + markerArea.WidthPT - cellBounds.XPT - cellBounds.WidthPT) * 12700))
		}
		return g
	}
	canvasWidth := state.wordCanvasWidthDP
	if canvasWidth <= 0 {
		canvasWidth = wordPaperMaxWidthDP
	}
	pageWidth, left := wordPageGeometry(width, gtx.Dp(unit.Dp(canvasWidth)))
	ml, mr, _ := wordCanvasPageMarginsDP(gtx, state)
	textLeft := left + wordPageHorizontalMargin(pageWidth, gtx.Dp(unit.Dp(ml)))
	textRight := left + pageWidth - wordPageHorizontalMargin(pageWidth, gtx.Dp(unit.Dp(mr)))
	return wordRulerGeometry{pageLeft: left, pageRight: left + pageWidth, textLeft: textLeft, textRight: textRight, trackLeft: textLeft, trackRight: textRight, pixelsPerEMU: float64(max32(.1, gtx.Metric.PxPerDp)) / float64(wordEMUPerDP)}
}

func (g wordRulerGeometry) pixels(emu int64) int {
	return int(math.Round(float64(emu) * g.pixelsPerEMU))
}
func (g wordRulerGeometry) emu(pixels float64) int64 {
	return int64(math.Round(pixels/g.pixelsPerEMU/635)) * 635
}

func wordEMUToPixels(gtx layout.Context, value int64) int {
	return int(math.Round(float64(value) / float64(wordEMUPerDP) * float64(gtx.Metric.PxPerDp)))
}

func wordRulerResolvedIndents(state *documentSessionState, p document.WordParagraph) wordRulerIndents {
	metrics := p.Layout
	metrics.LeftIndentEMU = p.LeftIndentEMU
	metrics.RightIndentEMU = p.RightIndentEMU
	if state.wordLayoutResult != nil {
		if resolved, ok := state.wordLayoutResult.ParagraphIndents[p.ID]; ok {
			metrics = resolved
		}
	}
	return wordRulerIndents{metrics.LeftIndentEMU, metrics.RightIndentEMU, metrics.FirstLineIndentEMU - metrics.HangingIndentEMU}
}

func wordRulerSelectedCellBounds(state *documentSessionState, page wordlayout.PageScene) (wordlayout.Rect, bool) {
	target, ok := wordRulerParagraphForState(state)
	if !ok {
		return wordlayout.Rect{}, false
	}
	for _, block := range wordBlocksForState(state) {
		if block.Table == nil {
			continue
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for _, p := range cell.Paragraphs {
					if p.ID != target.ID {
						continue
					}
					for _, placed := range page.TableCells {
						if placed.CellID == cell.ID {
							return placed.Bounds, true
						}
					}
				}
			}
		}
	}
	return wordlayout.Rect{}, false
}
