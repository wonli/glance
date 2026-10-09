package desktopui

import (
	"fmt"

	"glance/internal/wordlayout"
)

func wordPageForCaret(result wordlayout.Result, runID string, runeOffset int) (page int, ok bool) {
	position, ok := result.Locations[wordlayout.SourcePosition{RunID: runID, RuneOffset: runeOffset}]
	if !ok || position.PageIndex < 0 || position.PageIndex >= len(result.Pages) {
		return 0, false
	}
	return position.PageIndex, true
}

func wordPaginationStatusText(state *documentSessionState) string {
	if state == nil {
		return ""
	}
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() || state.formatRunning.Load() {
		return "正在更新页数…"
	}
	if state.wordLayoutError != "" {
		return "分页失败"
	}
	if state.wordLayoutPendingKey != nil {
		return "正在计算页数…"
	}
	result := state.wordLayoutResult
	if result == nil {
		return "页数未计算"
	}
	if !result.CanPreview {
		return "版式不支持"
	}
	if len(result.Pages) == 0 {
		return "无页面"
	}
	page := state.wordLayoutActivePage
	if state.wordViewMode == wordViewPrintLayout {
		page = state.wordLayoutActivePage
	} else if runID := state.selectedWordRunID; runID != "" {
		caret := 0
		if editor := state.wordRunEditors[runID]; editor != nil {
			_, caret = editor.Selection()
		}
		if mapped, ok := wordPageForCaret(*result, runID, caret); ok {
			page = mapped
		}
	} else if state.wordViewMode == wordViewPrintLayout {
		page = state.wordPageListState.Position.First
		if state.wordMultiplePagesMode {
			page *= 2
		}
	}
	page = max(0, min(page, len(result.Pages)-1))
	return fmt.Sprintf("第 %d 页，共 %d 页", page+1, len(result.Pages))
}

func (a *App) toggleWordPrintLayout() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	currentPage := wordCurrentPageIndex(state)
	if state.wordViewMode == wordViewPrintLayout {
		state.wordViewMode = wordViewContinuous
		setWordPageListFirst(state, currentPage)
		a.setStatus("已切换到连续视图")
	} else {
		state.wordViewMode = wordViewPrintLayout
		if state.wordMultiplePagesMode {
			currentPage /= 2
		}
		setWordPageListFirst(state, currentPage)
		a.startWordPagination(state)
		if state.wordLayoutPendingKey != nil {
			a.setStatus("正在生成打印布局")
		} else if state.wordLayoutError != "" {
			a.setStatus("分页失败：" + state.wordLayoutError)
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) selectWordSinglePageView() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	currentPage := wordCurrentPageIndex(state)
	setWordPageListFirst(state, currentPage)
	state.wordViewMode = wordViewPrintLayout
	state.wordMultiplePagesMode = false
	state.wordPageFitMode = true
	state.wordPageFitPageSetupKey = ""
	a.startWordPagination(state)
	a.setStatus("已切换到单页视图")
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) selectWordMultiplePagesView() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	currentPage := wordCurrentPageIndex(state)
	setWordPageListFirst(state, currentPage/2)
	state.wordViewMode = wordViewPrintLayout
	state.wordMultiplePagesMode = true
	state.wordPageFitMode = true
	state.wordPageFitPageSetupKey = ""
	a.startWordPagination(state)
	a.setStatus("已切换到多页视图")
	if a.window != nil {
		a.window.Invalidate()
	}
}

func wordCurrentPageIndex(state *documentSessionState) int {
	if state == nil {
		return 0
	}
	if state.wordViewMode == wordViewPrintLayout {
		page := state.wordPageListState.Position.First
		if state.wordMultiplePagesMode {
			page *= 2
		}
		return max(0, page)
	}
	if result := state.wordLayoutResult; result != nil && state.selectedWordRunID != "" {
		caret := 0
		if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
			_, caret = editor.Selection()
		}
		if page, ok := wordPageForCaret(*result, state.selectedWordRunID, caret); ok {
			return page
		}
	}
	return max(0, state.wordLayoutActivePage)
}

func (a *App) navigateWordPageToRun(state *documentSessionState, runID string, runeOffset int) {
	if state == nil || state.wordViewMode == wordViewWebLayout || state.wordLayoutResult == nil {
		return
	}
	page, ok := wordPageForCaret(*state.wordLayoutResult, runID, runeOffset)
	if !ok {
		return
	}
	if state.wordViewMode == wordViewPrintLayout && state.wordMultiplePagesMode {
		page /= 2
	}
	setWordPageListFirst(state, page)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func setWordPageListFirst(state *documentSessionState, first int) {
	if state == nil {
		return
	}
	state.wordPageListState.Position.First = max(0, first)
	state.wordPageListState.Position.Offset = 0
	state.wordPageListState.Position.OffsetLast = 0
	state.wordPageListState.Position.BeforeEnd = false
}

func (a *App) toggleWordWebLayout() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if state.wordViewMode == wordViewWebLayout {
		state.wordViewMode = wordViewContinuous
		a.setStatus("已切换到连续视图")
	} else {
		state.wordViewMode = wordViewWebLayout
		a.setStatus("已切换到 Web 版式")
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
