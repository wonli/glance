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
	DraftEpoch   uint64
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
	state.wordLayoutErrorKey = nil
	state.wordLayoutError = ""
	state.wordLayoutWarningConfirmed = false
	state.cancelWordLayout()
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

func (state *documentSessionState) wordLayoutProjectionSnapshot() ([]document.WordBlock, map[string]string, uint64, uint64) {
	if state == nil {
		return nil, map[string]string{}, 0, 0
	}
	state.wordBlocksMu.RLock()
	state.wordDraftMu.RLock()
	blocks := state.wordBlocks
	drafts := make(map[string]string, len(state.wordRunDrafts))
	for runID, value := range state.wordRunDrafts {
		drafts[runID] = value
	}
	revision := state.wordRevisionAtomic.Load()
	draftEpoch := state.wordDraftEpoch.Load()
	state.wordDraftMu.RUnlock()
	state.wordBlocksMu.RUnlock()
	return blocks, drafts, revision, draftEpoch
}

// wordBlocksWithDrafts returns a private layout snapshot with editor drafts
// projected into editable text runs. It clones the nested paragraph/table
// structures it writes so the session's committed neutral blocks stay intact.
func wordBlocksWithDrafts(blocks []document.WordBlock, drafts map[string]string) []document.WordBlock {
	projected := append([]document.WordBlock(nil), blocks...)
	projectParagraph := func(source document.WordParagraph) (document.WordParagraph, bool) {
		changed := false
		for _, inline := range source.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if draft, exists := drafts[inline.Text.ID]; exists && draft != inline.Text.Text {
				changed = true
				break
			}
		}
		if !changed {
			return source, false
		}
		paragraph := source
		paragraph.Inlines = append([]document.WordInline(nil), source.Inlines...)
		for index := range paragraph.Inlines {
			run := paragraph.Inlines[index].Text
			if run == nil || !run.Editable {
				continue
			}
			if draft, exists := drafts[run.ID]; exists && draft != run.Text {
				updated := *run
				updated.Text = draft
				paragraph.Inlines[index].Text = &updated
			}
		}
		return paragraph, true
	}
	for blockIndex := range projected {
		block := &projected[blockIndex]
		if block.Paragraph != nil {
			if paragraph, changed := projectParagraph(*block.Paragraph); changed {
				block.Paragraph = &paragraph
			}
		}
		if block.Table == nil {
			continue
		}
		var table *document.WordTable
		for rowIndex, sourceRow := range block.Table.Rows {
			var row *document.WordTableRow
			for cellIndex, sourceCell := range sourceRow.Cells {
				var cell *document.WordTableCell
				for paragraphIndex, sourceParagraph := range sourceCell.Paragraphs {
					paragraph, changed := projectParagraph(sourceParagraph)
					if !changed {
						continue
					}
					if table == nil {
						copyTable := *block.Table
						copyTable.Rows = append([]document.WordTableRow(nil), block.Table.Rows...)
						block.Table = &copyTable
						table = block.Table
					}
					if row == nil {
						copyRow := table.Rows[rowIndex]
						copyRow.Cells = append([]document.WordTableCell(nil), copyRow.Cells...)
						table.Rows[rowIndex] = copyRow
						row = &table.Rows[rowIndex]
					}
					if cell == nil {
						copyCell := row.Cells[cellIndex]
						copyCell.Paragraphs = append([]document.WordParagraph(nil), copyCell.Paragraphs...)
						row.Cells[cellIndex] = copyCell
						cell = &row.Cells[cellIndex]
					}
					cell.Paragraphs[paragraphIndex] = paragraph
				}
			}
		}
	}
	return projected
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
	if state == nil || state.word == nil || state.closed.Load() || state.finalized.Load() || state.wordLayoutReleased.Load() {
		return
	}
	if state.wordFlushRequested.Load() || state.formatRunning.Load() {
		return
	}
	_, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		state.cancelWordLayout()
		key := wordLayoutKey{
			DocumentID: state.documentID, Revision: state.wordRevisionAtomic.Load(),
			DraftEpoch: state.wordDraftEpoch.Load(),
		}
		if state.wordLayoutErrorKey == nil || *state.wordLayoutErrorKey != key {
			state.wordLayoutError = "DOCX 适配器未提供分页元数据"
			state.wordLayoutErrorKey = &key
		}
		return
	}
	data, ok := wordLayoutDataForState(state)
	if !ok {
		return
	}
	// Text edits bump the draft epoch; committed structural edits bump the
	// revision. The page-setup digest completes the identity without cloning the
	// document blocks on every Gio frame.
	revision := state.wordRevisionAtomic.Load()
	draftEpoch := state.wordDraftEpoch.Load()
	key := wordLayoutKey{
		DocumentID:   state.documentID,
		Revision:     revision,
		DraftEpoch:   draftEpoch,
		PageSetupKey: wordLayoutPageSetupKey(data, defaultWordPageSetup),
	}
	sameKey := func(existing *wordLayoutKey) bool {
		if existing == nil {
			return false
		}
		if existing.PageSetupKey == "" {
			return existing.DocumentID == key.DocumentID && existing.Revision == key.Revision && existing.DraftEpoch == key.DraftEpoch
		}
		return *existing == key
	}
	if sameKey(state.wordLayoutPendingKey) {
		return
	}
	if state.wordLayoutResult != nil && sameKey(state.wordLayoutAcceptedKey) {
		return
	}
	if sameKey(state.wordLayoutErrorKey) {
		return
	}
	blocks, drafts, snapshotRevision, snapshotDraftEpoch := state.wordLayoutProjectionSnapshot()
	if snapshotRevision != revision || snapshotDraftEpoch != draftEpoch {
		return
	}
	input := wordlayout.Input{
		DocumentID: key.DocumentID, Revision: key.Revision,
		Blocks: wordBlocksWithDrafts(blocks, drafts), Layout: data, DefaultPageSetup: defaultWordPageSetup,
		Previous: state.wordLayoutResult,
	}
	engine := a.wordLayoutEngine
	if engine == nil {
		shaper, err := newWordTextShaper()
		if err != nil {
			state.cancelWordLayout()
			state.wordLayoutError = err.Error()
			state.wordLayoutErrorKey = &key
			return
		}
		engine = wordlayout.NewEngine(shaper)
		a.wordLayoutEngine = engine
		a.wordPDFFontSource = shaper
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := state.sharedDocumentSession
	if session == nil {
		cancel()
		return
	}
	session.wordLayoutCancelMu.Lock()
	if session.finalized.Load() || session.wordLayoutReleased.Load() {
		session.wordLayoutCancelMu.Unlock()
		cancel()
		return
	}
	if state.wordLayoutPendingKey != nil && *state.wordLayoutPendingKey == key {
		session.wordLayoutCancelMu.Unlock()
		cancel()
		return
	}
	if state.wordLayoutResult != nil && state.wordLayoutAcceptedKey != nil && *state.wordLayoutAcceptedKey == key {
		session.wordLayoutCancelMu.Unlock()
		cancel()
		return
	}
	if state.wordLayoutCancel != nil {
		state.wordLayoutCancel()
	}
	state.wordLayoutCancel = cancel
	state.wordLayoutPendingKey = &key
	state.wordLayoutPendingView = state
	state.wordLayoutError = ""
	state.wordLayoutWarningConfirmed = false
	session.wordLayoutCancelMu.Unlock()
	results, shutdown, window := a.wordLayoutResults, a.shutdown, a.window
	go func() {
		result, err := engine.Layout(ctx, input)
		if ctx.Err() != nil || session.finalized.Load() || session.wordLayoutReleased.Load() {
			return
		}
		select {
		case results <- wordLayoutResult{state: state, key: key, result: result, err: err}:
			if window != nil {
				window.Invalidate()
			}
		case <-ctx.Done():
		case <-shutdown:
		}
	}()
}

func (a *App) applyWordLayoutResult(result wordLayoutResult) {
	state := result.state
	if state == nil || state.closed.Load() || state.finalized.Load() || state.wordLayoutReleased.Load() || state.word == nil || state.wordLayoutPendingKey == nil || *state.wordLayoutPendingKey != result.key {
		return
	}
	if state.documentID != result.key.DocumentID || state.wordRevisionAtomic.Load() != result.key.Revision || state.wordDraftEpoch.Load() != result.key.DraftEpoch {
		return
	}
	data, ok := wordLayoutDataForState(state)
	if !ok || wordLayoutPageSetupKey(data, defaultWordPageSetup) != result.key.PageSetupKey {
		return
	}
	state.wordLayoutCancelMu.Lock()
	if state.finalized.Load() || state.wordLayoutReleased.Load() || state.documentID != result.key.DocumentID || state.wordRevisionAtomic.Load() != result.key.Revision || state.wordDraftEpoch.Load() != result.key.DraftEpoch || state.wordLayoutPendingKey == nil || *state.wordLayoutPendingKey != result.key {
		state.wordLayoutCancelMu.Unlock()
		return
	}
	if state.wordLayoutCancel != nil {
		state.wordLayoutCancel()
		state.wordLayoutCancel = nil
	}
	state.wordLayoutPendingKey = nil
	state.wordLayoutPendingView = nil
	if result.err != nil {
		state.wordLayoutError = result.err.Error()
		state.wordLayoutErrorKey = &result.key
	} else {
		accepted := result.result
		state.wordLayoutResult = &accepted
		state.wordLayoutPageCount.Store(int64(len(accepted.Pages)))
		pageListLast := max(0, len(accepted.Pages)-1)
		if state.wordViewMode == wordViewPrintLayout && state.wordMultiplePagesMode {
			pageListLast /= 2
		}
		state.wordPageListState.Position.First = min(max(0, state.wordPageListState.Position.First), pageListLast)
		state.wordLayoutActivePage = state.wordPageListState.Position.First
		if state.wordViewMode == wordViewPrintLayout && state.wordMultiplePagesMode {
			state.wordLayoutActivePage *= 2
		}
		state.wordLayoutActivePage = min(state.wordLayoutActivePage, max(0, len(accepted.Pages)-1))
		acceptedKey := result.key
		state.wordLayoutAcceptedKey = &acceptedKey
		state.wordLayoutErrorKey = nil
		state.wordLayoutError = ""
	}
	state.wordLayoutCancelMu.Unlock()
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
	currentPage := wordCurrentPageIndex(state)
	if state.wordViewMode == wordViewPrintLayout {
		state.wordViewMode = wordViewContinuous
		setWordPageListFirst(state, currentPage)
		a.status = "已切换到连续视图"
	} else {
		state.wordViewMode = wordViewPrintLayout
		if state.wordMultiplePagesMode {
			currentPage /= 2
		}
		setWordPageListFirst(state, currentPage)
		a.startWordPagination(state)
		if state.wordLayoutPendingKey != nil {
			a.status = "正在生成打印布局"
		} else if state.wordLayoutError != "" {
			a.status = "分页失败：" + state.wordLayoutError
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
	state.wordPageFitPageSetupKey = ""
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
		a.status = "已切换到连续视图"
	} else {
		state.wordViewMode = wordViewWebLayout
		a.status = "已切换到 Web 版式"
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
