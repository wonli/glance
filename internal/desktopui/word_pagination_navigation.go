package desktopui

import (
	"fmt"

	"glance/internal/i18n"
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
	message := wordPaginationStatusMessage(state)
	switch message.ID {
	case "word.status.pagination.updating":
		return "正在更新页数…"
	case "word.status.pagination.calculating":
		return "正在计算页数…"
	case "word.status.pagination.failed":
		return "分页失败"
	case "word.status.pagination.not_computed":
		return "页数未计算"
	case "word.status.pagination.unsupported":
		return "版式不支持"
	case "word.status.pagination.empty":
		return "无页面"
	case "word.status.pagination.page":
		return fmt.Sprintf("第 %v 页，共 %v 页", message.Args["Current"], message.Args["Total"])
	default:
		return ""
	}
}

func wordPaginationStatusMessage(state *documentSessionState) i18n.Message {
	if state == nil {
		return i18n.Message{}
	}
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() || state.formatRunning.Load() {
		return i18n.Message{ID: "word.status.pagination.updating"}
	}
	if state.wordLayoutError != "" {
		return i18n.Message{ID: "word.status.pagination.failed"}
	}
	if state.wordLayoutPendingKey != nil {
		return i18n.Message{ID: "word.status.pagination.calculating"}
	}
	result := state.wordLayoutResult
	if result == nil {
		return i18n.Message{ID: "word.status.pagination.not_computed"}
	}
	if !result.CanPreview {
		return i18n.Message{ID: "word.status.pagination.unsupported"}
	}
	if len(result.Pages) == 0 {
		return i18n.Message{ID: "word.status.pagination.empty"}
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
	return i18n.Message{ID: "word.status.pagination.page", Args: i18n.Args{"Current": page + 1, "Total": len(result.Pages)}}
}

func (a *App) localizedWordPaginationStatusText(state *documentSessionState) string {
	message := wordPaginationStatusMessage(state)
	if message.ID == "" {
		return ""
	}
	return a.text(message)
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
		a.setStatusMessage(i18n.Message{ID: "word.status.view.continuous"}, feedbackInfo, notificationSuppressed)
	} else {
		state.wordViewMode = wordViewPrintLayout
		if state.wordMultiplePagesMode {
			currentPage /= 2
		}
		setWordPageListFirst(state, currentPage)
		a.startWordPagination(state)
		if state.wordLayoutPendingKey != nil {
			a.setStatusMessage(i18n.Message{ID: "word.status.view.print_layout_preparing"}, feedbackInfo, notificationSuppressed)
		} else if state.wordLayoutError != "" {
			a.setStatusDetailMessage(i18n.Message{ID: "word.status.pagination.failed"}, feedbackError, notificationToast, state.wordLayoutError)
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
	a.setStatusMessage(i18n.Message{ID: "word.status.view.single_page"}, feedbackInfo, notificationSuppressed)
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
	a.setStatusMessage(i18n.Message{ID: "word.status.view.multiple_pages"}, feedbackInfo, notificationSuppressed)
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
		a.setStatusMessage(i18n.Message{ID: "word.status.view.continuous"}, feedbackInfo, notificationSuppressed)
	} else {
		state.wordViewMode = wordViewWebLayout
		a.setStatusMessage(i18n.Message{ID: "word.status.view.web_layout"}, feedbackInfo, notificationSuppressed)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
