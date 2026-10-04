package desktopui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

type wordViewMode uint8

const (
	wordViewContinuous wordViewMode = iota
	wordViewPrintLayout
	wordViewWebLayout
)

type wordLayoutKey struct {
	DocumentID   string
	Revision     uint64
	PageSetupKey string
}

type wordLayoutEngine interface {
	Layout(context.Context, wordlayout.Input) (wordlayout.Result, error)
}

type wordLayoutResult struct {
	state  *documentSessionState
	key    wordLayoutKey
	result wordlayout.Result
	err    error
}

var defaultWordPageSetup = document.WordPageSetup{
	WidthPT: 595.28, HeightPT: 841.89, Orientation: "portrait",
	MarginTopPT: 72, MarginRightPT: 72, MarginBottomPT: 72, MarginLeftPT: 72,
}

func (state *documentSessionState) setWordBlocks(blocks []document.WordBlock) {
	if state == nil {
		return
	}
	state.wordBlocksMu.Lock()
	if reflect.DeepEqual(state.wordBlocks, blocks) {
		state.wordBlocksMu.Unlock()
		return
	}
	state.wordBlocks = cloneWordBlockSnapshot(blocks)
	state.operationMu.Lock()
	revision := state.wordRevisionAtomic.Load() + 1
	state.wordRevision = revision
	state.wordRevisionAtomic.Store(revision)
	state.operationMu.Unlock()
	state.wordBlocksMu.Unlock()
	state.invalidateWordLayoutAfterBlocksChange()
}

func (state *documentSessionState) setWordBlocksAndTextState(blocks []document.WordBlock, clearDrafts bool) {
	if state == nil {
		return
	}
	committed := wordCommittedTextMap(blocks)
	state.wordBlocksMu.Lock()
	state.wordDraftMu.Lock()
	blocksChanged := !reflect.DeepEqual(state.wordBlocks, blocks)
	if blocksChanged {
		state.wordBlocks = cloneWordBlockSnapshot(blocks)
	}
	drafts := make(map[string]string)
	if !clearDrafts {
		for runID, value := range state.wordRunDrafts {
			if committedValue, exists := committed[runID]; exists && value != committedValue {
				drafts[runID] = value
			}
		}
	}
	textChanged := !sameWordTextMap(state.wordRunCommitted, committed) || !sameWordTextMap(state.wordRunDrafts, drafts)
	state.wordRunCommitted = committed
	state.wordRunDrafts = drafts
	state.wordTextResetEpoch.Add(1)
	if textChanged || clearDrafts {
		state.bumpWordDraftEpoch()
	}
	if blocksChanged {
		state.operationMu.Lock()
		revision := state.wordRevisionAtomic.Load() + 1
		state.wordRevision = revision
		state.wordRevisionAtomic.Store(revision)
		state.operationMu.Unlock()
	}
	state.wordDraftMu.Unlock()
	state.wordBlocksMu.Unlock()
	if blocksChanged {
		state.invalidateWordLayoutAfterBlocksChange()
	}
}

func (state *documentSessionState) invalidateWordLayoutAfterBlocksChange() {
	if state.wordPDFExportCancel != nil {
		state.wordPDFExportCancel()
	}
	if state.wordPrintCancel != nil {
		state.wordPrintCancel()
	}
	state.wordLayoutResult = nil
	state.wordLayoutPageCount.Store(0)
	state.wordLayoutAcceptedKey = nil
	state.wordLayoutErrorKey = nil
	state.wordLayoutError = ""
	state.wordLayoutWarningConfirmed = false
	state.wordLayoutPendingKey = nil
	if state.wordLayoutCancel != nil {
		state.wordLayoutCancel()
		state.wordLayoutCancel = nil
	}
}

func (state *documentSessionState) wordProjectionSnapshot() ([]document.WordBlock, map[string]string, map[string]string, uint64, uint64, uint64) {
	if state == nil {
		return nil, map[string]string{}, map[string]string{}, 0, 0, 0
	}
	state.wordBlocksMu.RLock()
	state.wordDraftMu.RLock()
	blocks := cloneWordBlockSnapshot(state.wordBlocks)
	committed := make(map[string]string, len(state.wordRunCommitted))
	for runID, value := range state.wordRunCommitted {
		committed[runID] = value
	}
	drafts := make(map[string]string, len(state.wordRunDrafts))
	for runID, value := range state.wordRunDrafts {
		drafts[runID] = value
	}
	revision := state.wordRevisionAtomic.Load()
	draftEpoch := state.wordDraftEpoch.Load()
	resetEpoch := state.wordTextResetEpoch.Load()
	state.wordDraftMu.RUnlock()
	state.wordBlocksMu.RUnlock()
	return blocks, committed, drafts, revision, draftEpoch, resetEpoch
}

func wordLayoutDataForState(state *documentSessionState) (document.WordLayoutData, bool) {
	if state == nil || state.word == nil {
		return document.WordLayoutData{}, false
	}
	reader, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		return document.WordLayoutData{}, false
	}
	unlock := lockSessionAdapterRead(state)
	data := reader.WordLayoutData()
	unlock()
	return data, true
}

func wordLayoutPageSetupKey(data document.WordLayoutData, fallback document.WordPageSetup) string {
	value := struct {
		Sections []document.WordSection
		Default  document.WordPageSetup
	}{Sections: data.Sections, Default: fallback}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (a *App) startWordPagination(state *documentSessionState) {
	if state == nil || state.word == nil || state.closed.Load() {
		return
	}
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested.Load() || state.formatRunning.Load() {
		return
	}
	// Supported mutations bump wordRevision through setWordBlocks. Avoid cloning
	// and hashing the adapter's layout projection on every Gio frame.
	revision := state.wordRevisionAtomic.Load()
	if state.wordLayoutPendingKey != nil && state.wordLayoutPendingKey.DocumentID == state.documentID && state.wordLayoutPendingKey.Revision == revision {
		return
	}
	if state.wordLayoutResult != nil && state.wordLayoutAcceptedKey != nil && state.wordLayoutAcceptedKey.DocumentID == state.documentID && state.wordLayoutAcceptedKey.Revision == revision {
		return
	}
	if state.wordLayoutErrorKey != nil && state.wordLayoutErrorKey.DocumentID == state.documentID && state.wordLayoutErrorKey.Revision == revision {
		return
	}
	_, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		state.wordLayoutError = "DOCX 适配器未提供分页元数据"
		state.wordLayoutErrorKey = &wordLayoutKey{DocumentID: state.documentID, Revision: revision}
		return
	}
	data, _ := wordLayoutDataForState(state)
	key := wordLayoutKey{
		DocumentID: state.documentID, Revision: revision,
		PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup),
	}
	if state.wordLayoutResult != nil && state.wordLayoutAcceptedKey != nil && *state.wordLayoutAcceptedKey == key {
		return
	}
	if state.wordLayoutPendingKey != nil && *state.wordLayoutPendingKey == key {
		return
	}
	if state.wordLayoutAcceptedKey != nil && *state.wordLayoutAcceptedKey != key {
		state.wordLayoutResult = nil
		state.wordLayoutPageCount.Store(0)
		state.wordLayoutAcceptedKey = nil
	}
	if state.wordLayoutCancel != nil {
		state.wordLayoutCancel()
	}
	blocks := cloneWordBlockSnapshot(wordBlocksForState(state))
	input := wordlayout.Input{
		DocumentID: key.DocumentID, Revision: key.Revision,
		Blocks: blocks, Layout: data, DefaultPageSetup: defaultWordPageSetup,
		EditorParagraphInsets: true,
	}
	engine := a.wordLayoutEngine
	if engine == nil {
		shaper, err := newWordTextShaper()
		if err != nil {
			state.wordLayoutError = err.Error()
			state.wordLayoutErrorKey = &key
			state.wordLayoutPendingKey = nil
			return
		}
		engine = wordlayout.NewEngine(shaper)
		a.wordLayoutEngine = engine
		a.wordPDFFontSource = shaper
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.wordLayoutCancel = cancel
	state.wordLayoutPendingKey = &key
	state.wordLayoutError = ""
	state.wordLayoutWarningConfirmed = false
	results, shutdown, window := a.wordLayoutResults, a.shutdown, a.window
	go func() {
		result, err := engine.Layout(ctx, input)
		if ctx.Err() != nil {
			return
		}
		select {
		case results <- wordLayoutResult{state: state, key: key, result: result, err: err}:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func (a *App) applyWordLayoutResult(result wordLayoutResult) {
	state := result.state
	if state == nil || state.closed.Load() || state.word == nil || state.wordLayoutPendingKey == nil || *state.wordLayoutPendingKey != result.key {
		return
	}
	if state.documentID != result.key.DocumentID || state.wordRevisionAtomic.Load() != result.key.Revision {
		return
	}
	data, ok := wordLayoutDataForState(state)
	if !ok || wordLayoutPageSetupKey(data, defaultWordPageSetup) != result.key.PageSetupKey {
		return
	}
	state.wordLayoutPendingKey = nil
	state.wordLayoutCancel = nil
	state.wordLayoutActivePage = 0
	if result.err != nil {
		state.wordLayoutError = result.err.Error()
		state.wordLayoutErrorKey = &result.key
		state.wordLayoutResult = nil
		state.wordLayoutAcceptedKey = nil
		state.wordLayoutPageCount.Store(0)
	} else {
		accepted := result.result
		state.wordLayoutResult = &accepted
		state.wordLayoutPageCount.Store(int64(len(accepted.Pages)))
		pageListLast := max(0, len(accepted.Pages)-1)
		if state.wordMultiplePagesMode {
			pageListLast /= 2
		}
		state.wordPageListState.Position.First = min(max(0, state.wordPageListState.Position.First), pageListLast)
		acceptedKey := result.key
		state.wordLayoutAcceptedKey = &acceptedKey
		state.wordLayoutErrorKey = nil
		state.wordLayoutError = ""
	}
	if state.wordPDFExportAfterLayout {
		state.wordPDFExportAfterLayout = false
		if result.err != nil {
			state.wordPDFExportAfterLayout = false
			a.status = "PDF 导出前分页失败：" + result.err.Error()
		} else {
			a.beginWordPDFExport(state)
		}
	}
	if state.wordPrintAfterLayout {
		state.wordPrintAfterLayout = false
		if result.err != nil {
			a.status = "打印前分页失败：" + result.err.Error()
		} else {
			a.beginWordPrint(state)
		}
	}
}

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
	if state.wordViewMode == wordViewPrintLayout {
		state.wordViewMode = wordViewContinuous
		a.status = "已切换到连续视图"
		return
	}
	state.wordViewMode = wordViewPrintLayout
	a.startWordPagination(state)
	if state.wordLayoutPendingKey != nil {
		a.status = "正在生成打印布局"
	} else if state.wordLayoutError != "" {
		a.status = "分页失败：" + state.wordLayoutError
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
	a.startWordPagination(state)
	a.status = "已切换到单页视图"
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
	a.startWordPagination(state)
	a.status = "已切换到多页视图"
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
	if state == nil || state.wordViewMode != wordViewPrintLayout || state.wordLayoutResult == nil {
		return
	}
	page, ok := wordPageForCaret(*state.wordLayoutResult, runID, runeOffset)
	if !ok {
		return
	}
	if state.wordMultiplePagesMode {
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
		a.status = "已切换到连续视图"
	} else {
		state.wordViewMode = wordViewWebLayout
		a.status = "已切换到 Web 版式"
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
