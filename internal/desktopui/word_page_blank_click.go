package desktopui

import (
	"glance/internal/document"
	"glance/internal/wordlayout"
	"math"

	"gioui.org/f32"
)

func isWordPageBottomBlank(page wordlayout.PageScene, pagePoint f32.Point) bool {
	contentBottom := wordPageContentBottomPT(page)
	return float64(pagePoint.Y) >= contentBottom-4
}

func wordPageContentBottomPT(page wordlayout.PageScene) float64 {
	bottom := page.Content.YPT
	for _, fragment := range page.Text {
		y := fragment.BaselineYPT + math.Max(1, fragment.Style.FontSize*0.3)
		if y > bottom {
			bottom = y
		}
	}
	for _, img := range page.Images {
		y := img.Bounds.YPT + img.Bounds.HeightPT
		if y > bottom {
			bottom = y
		}
	}
	for _, cell := range page.TableCells {
		y := cell.Bounds.YPT + cell.Bounds.HeightPT
		if y > bottom {
			bottom = y
		}
	}
	for _, border := range page.ParagraphBorders {
		y := border.Bounds.YPT + border.Bounds.HeightPT
		if y > bottom {
			bottom = y
		}
	}
	return bottom
}

func (a *App) handleWordBottomBlankClick(state *documentSessionState, page wordlayout.PageScene, pageIndex int) {
	if state == nil || state.wordInsertionRunning || state.formatRunning.Load() || a.isSaving() {
		return
	}
	afterParagraphID := ""
	isLastPage := state.wordLayoutResult == nil || pageIndex >= len(state.wordLayoutResult.Pages)-1
	if !isLastPage {
		afterParagraphID = lastWordParagraphIDOnPage(state, page)
	}
	a.queueWordParagraphInsert(state, afterParagraphID)
}

func lastWordParagraphIDOnPage(state *documentSessionState, page wordlayout.PageScene) string {
	for i := len(page.Text) - 1; i >= 0; i-- {
		frag := page.Text[i]
		if frag.RunID != "" {
			if _, para, ok := findEditableWordRunInState(state, frag.RunID); ok && para.ID != "" {
				return para.ID
			}
		}
	}
	blocks := wordBlocksForState(state)
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Kind == document.WordParagraphBlock && blocks[i].Paragraph != nil {
			return blocks[i].Paragraph.ID
		}
	}
	return ""
}
