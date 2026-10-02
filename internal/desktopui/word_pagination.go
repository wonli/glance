package desktopui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

type wordViewMode uint8

const (
	wordViewContinuous wordViewMode = iota
	wordViewPrintLayout
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
	state.wordExportCommitMu.Lock()
	state.wordBlocks = blocks
	state.wordRevision++
	state.wordRevisionAtomic.Store(state.wordRevision)
	state.wordExportCommitMu.Unlock()
	if state.wordPDFExportCancel != nil {
		state.wordPDFExportCancel()
	}
	state.wordLayoutResult = nil
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
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested || state.formatRunning {
		return
	}
	// Supported mutations bump wordRevision through setWordBlocks. Avoid cloning
	// and hashing the adapter's layout projection on every Gio frame.
	if state.wordLayoutPendingKey != nil && state.wordLayoutPendingKey.DocumentID == state.documentID && state.wordLayoutPendingKey.Revision == state.wordRevision {
		return
	}
	if state.wordLayoutResult != nil && state.wordLayoutAcceptedKey != nil && state.wordLayoutAcceptedKey.DocumentID == state.documentID && state.wordLayoutAcceptedKey.Revision == state.wordRevision {
		return
	}
	if state.wordLayoutErrorKey != nil && state.wordLayoutErrorKey.DocumentID == state.documentID && state.wordLayoutErrorKey.Revision == state.wordRevision {
		return
	}
	reader, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		state.wordLayoutError = "DOCX 适配器未提供分页元数据"
		state.wordLayoutErrorKey = &wordLayoutKey{DocumentID: state.documentID, Revision: state.wordRevision}
		return
	}
	data := reader.WordLayoutData()
	key := wordLayoutKey{
		DocumentID: state.documentID, Revision: state.wordRevision,
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
		state.wordLayoutAcceptedKey = nil
	}
	if state.wordLayoutCancel != nil {
		state.wordLayoutCancel()
	}
	blocks := cloneWordBlockSnapshot(wordBlocksForState(state))
	input := wordlayout.Input{
		DocumentID: key.DocumentID, Revision: key.Revision,
		Blocks: blocks, Layout: data, DefaultPageSetup: defaultWordPageSetup,
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
	if state.documentID != result.key.DocumentID || state.wordRevision != result.key.Revision {
		return
	}
	reader, ok := any(state.word).(document.WordLayoutReader)
	if !ok || wordLayoutPageSetupKey(reader.WordLayoutData(), defaultWordPageSetup) != result.key.PageSetupKey {
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
	} else {
		accepted := result.result
		state.wordLayoutResult = &accepted
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
	if wordHasPendingTextDrafts(state) || state.wordFlushRequested || state.formatRunning {
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
	if runID := state.selectedWordRunID; runID != "" {
		caret := 0
		if editor := state.wordRunEditors[runID]; editor != nil {
			_, caret = editor.Selection()
		}
		if mapped, ok := wordPageForCaret(*result, runID, caret); ok {
			page = mapped
		}
	} else if state.wordViewMode == wordViewPrintLayout {
		page = state.wordPageListState.Position.First
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
