package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/application"
	"glance/internal/workspace"
	"path/filepath"
	"strings"
)

func snapshotState(state *documentSessionState) ([]byte, error) {
	if state != nil && state.book != nil {
		unlock := lockSessionAdapterWrite(state)
		defer unlock()
		if err := applySpreadsheetSheetZooms(state); err != nil {
			return nil, err
		}
		if snap, ok := state.book.(interface{ Snapshot() ([]byte, error) }); ok {
			return snap.Snapshot()
		}
		return nil, fmt.Errorf("当前表格格式不支持工作区快照")
	}
	unlock := lockSessionAdapterRead(state)
	defer unlock()
	switch {
	case state.word != nil:
		return state.word.Snapshot()
	case state.pdfDoc != nil:
		return state.pdfDoc.Snapshot()
	case state.markdownDoc != nil:
		return state.markdownDoc.Snapshot()
	case state.jsonDoc != nil:
		return state.jsonDoc.Snapshot()
	case state.pptxDoc != nil:
		return state.pptxDoc.SnapshotBytes()
	default:
		return nil, fmt.Errorf("没有可保存的活动文档")
	}
}

func sessionDirty(state *documentSessionState) bool {
	unlock := lockSessionAdapterRead(state)
	defer unlock()
	return state.book != nil && state.book.Dirty() || state.word != nil && state.word.Dirty() || state.jsonDoc != nil && state.jsonDoc.Dirty() || state.markdownDoc != nil && state.markdownDoc.Dirty() || state.pdfDoc != nil && state.pdfDoc.Dirty() || state.pptxDoc != nil && state.pptxDoc.Dirty()
}

func sessionPath(state *documentSessionState) string {
	unlock := lockSessionAdapterRead(state)
	defer unlock()
	if state == nil {
		return ""
	}
	return state.path
}

func (a *App) viewStateBytes() []byte {
	view := sessionViewSnapshot{Sheet: a.sheet, SelectedCell: a.selectedCell, SelectedBlock: a.selectedBlock, WordFirst: a.wordListState.Position.First, WordAutoSave: a.wordAutoSave}
	if state := a.documentSessionState; state != nil && state.word != nil {
		view.WordViewMode = state.wordViewMode
		view.WordPageFitMode = state.wordPageFitMode
		view.WordMultiplePagesMode = state.wordMultiplePagesMode
		view.WordLayoutZoom = state.wordLayoutZoom
		view.WordRulerHidden = state.wordRulerHidden
		view.WordGridlinesVisible = state.wordGridlinesVisible
		view.WordNavigationVisible = state.wordNavigationVisible
		view.WordNavigationQuery = state.wordNavigationSearchQuery
		view.WordToolbarTab = state.wordToolbarTab
		view.SelectedWordRunID = state.selectedWordRunID
		view.SelectedWordParagraphID = state.selectedWordParagraphID
		view.SelectedWordImageID = state.selectedWordImageID
		view.WordPageFirst = state.wordPageListState.Position.First
		if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
			view.SelectedWordSelectionStart, view.SelectedWordSelectionEnd = editor.Selection()
		}
	}
	if a.markdownView != nil {
		v := a.markdownView
		view.MarkdownMode = v.mode
		view.MarkdownCollapsed = v.collapsed
		view.MarkdownStart, view.MarkdownEnd = v.editor.Selection()
		view.MarkdownPreviewFirst = v.preview.Position.First
		view.MarkdownQuery = v.search.Text()
	}
	if a.virtualGrid != nil {
		view.GridX, view.GridY, view.GridScale = a.virtualGrid.scrollWorldX(), a.virtualGrid.scrollWorldY(), a.virtualGrid.Scale
		view.SpreadsheetZoom = a.spreadsheetZoom()
		view.SpreadsheetZooms = a.spreadsheetSheetZoomSnapshot()
		view.SpreadsheetZoomPending = a.documentSessionState.spreadsheetZoomNeedsSave()
		view.SelectedRow, view.SelectedColumn = a.virtualGrid.SelectedRow, a.virtualGrid.SelectedColumn
	}
	data, _ := json.Marshal(view)
	return data
}

func (a *App) processPrimaryPromotion() {
	if a == nil || !a.primaryPromotionPending.Swap(false) {
		return
	}
	runtime := a.ensureDesktopRuntime()
	if !runtime.isPrimaryWindow(a.windowID) {
		return
	}
	if a.collection != nil {
		activeTabID := a.activeTabID
		if activeTabID == "" {
			activeTabID = workspace.RecentTabID
		}
		if activeTabID != workspace.RecentTabID {
			if _, exists := a.collection.Get(activeTabID); !exists {
				activeTabID = workspace.RecentTabID
			}
		}
		a.activeTabID = activeTabID
		if activeTabID == workspace.RecentTabID {
			a.setActiveSessionID("")
		}
		if err := a.collection.Activate(activeTabID); err != nil {
			a.setStatus("活动文档恢复失败：" + err.Error())
		}
	}
	a.persistActiveSession()
}

func (a *App) persistViewState() {
	runtime := a.ensureDesktopRuntime()
	if !runtime.isPrimaryWindow(a.windowID) {
		return
	}
	if a.collection == nil || a.activeTabID == "" || a.activeTabID == workspace.RecentTabID {
		return
	}
	if _, exists := a.collection.Get(a.activeTabID); !exists && a.documentSessionState != nil && a.documentSessionState.sourceFormat == application.FormatXLS {
		// Imported XLS tabs stay local until Save As creates a durable XLSX
		// workspace entry. Their view state already lives in this session.
		return
	}
	if err := a.collection.SaveViewState(a.activeTabID, a.viewStateBytes()); err != nil {
		a.setStatus("视图状态保存失败：" + err.Error())
	}
}

func (a *App) updatePersistedTab(meta workspace.Tab) {
	runtime := a.ensureDesktopRuntime()
	if !runtime.isPrimaryWindow(a.windowID) {
		return
	}
	if a.collection == nil {
		return
	}
	state := a.sessions[meta.ID]
	if state == nil {
		return
	}
	format := sessionFormat(meta, state)
	if err := a.collection.UpdateTabMeta(meta); err != nil {
		a.setStatus("工作区保存失败：" + err.Error())
		return
	}
	job := workspaceWriteJob{documentID: meta.DocumentID, format: format, state: state, viewState: a.viewStateBytesForState(state)}
	if a.workspaceWriteJobs != nil {
		a.workspaceWriteJobs <- job
	} else {
		payload, err := snapshotState(state)
		if err == nil {
			err = a.collection.Update(meta.ID, payload, job.viewState, sessionDirty(state))
		}
		if err != nil {
			a.setStatus(err.Error())
		}
	}
}

func sessionFormat(meta workspace.Tab, state *documentSessionState) string {
	if state != nil && state.pdfDoc != nil {
		return "pdf"
	}
	if state != nil && state.markdownDoc != nil {
		return "markdown"
	}
	if state != nil && state.sourceFormat == application.FormatDOC {
		return "docx"
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(meta.SourcePath)), ".")
	if format == "" {
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(meta.Title)), ".")
	}
	if format != "" {
		return format
	}
	switch {
	case state != nil && state.book != nil:
		if meta.Kind == "document" && strings.EqualFold(filepath.Ext(meta.SourcePath), ".xls") {
			return "xls"
		}
		return "xlsx"
	case state != nil && state.word != nil:
		return "docx"
	case state != nil && state.jsonDoc != nil:
		return "json"
	default:
		return ""
	}
}

func (a *App) viewStateBytesForState(s *documentSessionState) []byte {
	old := a.documentSessionState
	a.documentSessionState = s
	b := a.viewStateBytes()
	a.documentSessionState = old
	return b
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
