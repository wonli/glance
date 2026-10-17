package desktopui

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"glance/internal/application"
	"glance/internal/document"
	csvdoc "glance/internal/format/csv"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/xlsx"
	"glance/internal/workspace"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type sessionViewSnapshot struct {
	MarkdownMode               int  `json:"markdown_mode"`
	MarkdownCollapsed          bool `json:"markdown_collapsed"`
	MarkdownStart, MarkdownEnd int
	MarkdownPreviewFirst       int
	MarkdownQuery              string
	Sheet                      string         `json:"sheet"`
	SelectedCell               string         `json:"selected_cell"`
	SelectedBlock              int            `json:"selected_block"`
	WordFirst                  int            `json:"word_first"`
	WordAutoSave               bool           `json:"word_auto_save"`
	GridX                      float64        `json:"grid_x"`
	GridY                      float64        `json:"grid_y"`
	GridScale                  float64        `json:"grid_scale"`
	SpreadsheetZoom            float64        `json:"spreadsheet_zoom,omitempty"`
	SpreadsheetZooms           map[string]int `json:"spreadsheet_zooms,omitempty"`
	SpreadsheetZoomPending     bool           `json:"spreadsheet_zoom_pending,omitempty"`
	SelectedRow                int            `json:"selected_row"`
	SelectedColumn             int            `json:"selected_column"`
	SelectionSet               bool           `json:"selection_set"`
	WordViewMode               wordViewMode   `json:"word_view_mode"`
	WordPageFitMode            bool           `json:"word_page_fit_mode"`
	WordMultiplePagesMode      bool           `json:"word_multiple_pages_mode"`
	WordLayoutZoom             float64        `json:"word_layout_zoom"`
	WordRulerHidden            bool           `json:"word_ruler_hidden"`
	WordGridlinesVisible       bool           `json:"word_gridlines_visible"`
	WordNavigationVisible      bool           `json:"word_navigation_visible"`
	WordNavigationQuery        string         `json:"word_navigation_query"`
	WordToolbarTab             wordToolbarTab `json:"word_toolbar_tab"`
	SelectedWordRunID          string         `json:"selected_word_run_id"`
	SelectedWordParagraphID    string         `json:"selected_word_paragraph_id"`
	SelectedWordImageID        string         `json:"selected_word_image_id"`
	SelectedWordSelectionStart int            `json:"selected_word_selection_start"`
	SelectedWordSelectionEnd   int            `json:"selected_word_selection_end"`
	WordPageFirst              int            `json:"word_page_first"`
}

func workspaceSessionID(path string) string {
	if path == "" {
		return "draft-" + fmt.Sprintf("%x", sha256.Sum256([]byte("new")))[:12]
	}
	path = canonicalDocumentPath(path)
	return fmt.Sprintf("doc-%x", sha256.Sum256([]byte(path)))[:20]
}

func canonicalDocumentPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	path = filepath.Clean(path)
	return path
}

func sameDocumentPath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	if canonicalDocumentPath(left) == canonicalDocumentPath(right) {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

var draftCounter atomic.Uint64

func newDraftSessionID(format string) string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err == nil {
		return fmt.Sprintf("draft-%s-%x", format, id[:])
	}
	return fmt.Sprintf("draft-%s-%d-%d", format, time.Now().UnixNano(), draftCounter.Add(1))
}

func (a *App) initializeWorkspace() {
	runtime := a.ensureDesktopRuntime()
	runtime.workspaceInitMu.Lock()
	defer runtime.workspaceInitMu.Unlock()
	if runtime.workspaceStore == nil {
		path, err := workspace.DefaultDatabasePath()
		if err != nil {
			a.workspaceError = err.Error()
			a.setStatus("工作区数据库不可用：" + err.Error())
			return
		}
		var store *workspace.Store
		if runtime.applicationRuntime != nil {
			store, err = runtime.applicationRuntime.OpenWorkspace(path)
		} else {
			store, err = workspace.Open(path)
		}
		if err != nil {
			a.workspaceError = err.Error()
			a.setStatus("工作区数据库不可用：" + err.Error())
			return
		}
		runtime.workspaceStore = store
	}
	if runtime.collection == nil {
		collection, err := workspace.OpenCollection(runtime.workspaceStore)
		if err != nil {
			a.workspaceError = err.Error()
			a.setStatus("恢复工作区失败：" + err.Error())
			return
		}
		runtime.collection = collection
	}
	if err := runtime.loadHomeFolderState(); err != nil {
		a.setStatus("文件夹数据不可用：" + err.Error())
	}
	collection := runtime.collection
	a.loadHomeFavorites()
	a.loadAgentSidebarPreference()
	for _, record := range collection.Tabs() {
		a.tabMeta[record.Tab.ID] = record.Tab
		if !hasRestorableSnapshot(record) {
			continue
		}
		var state *documentSessionState
		if shared, ok := runtime.session(record.Tab.DocumentID); ok {
			state = newDocumentSessionView(shared, a.windowID)
		} else {
			var err error
			state, err = a.restoreSessionState(record)
			if err != nil {
				a.setStatus("恢复 " + record.Tab.Title + " 失败：" + err.Error())
				continue
			}
			state.documentID = record.Tab.DocumentID
			shared := runtime.registerSession(state.sharedDocumentSession)
			if state.sharedDocumentSession != shared {
				state = newDocumentSessionView(shared, a.windowID)
			}
		}
		if isEmptyDraftWithoutSnapshot(record) {
			payload, err := snapshotState(state)
			if err == nil {
				err = collection.Update(record.Tab.ID, payload, record.ViewState, sessionDirty(state))
			}
			if err != nil {
				a.setStatus("恢复 " + record.Tab.Title + " 后重建快照失败：" + err.Error())
			}
		}
		a.sessions[record.Tab.ID] = state
	}
	runtime.workspaceInitialized = true
	// Restore all document sessions, but only resume the prior active tab when
	// a tray-hidden window is being recreated in the same process.
	primaryWindow := runtime.isPrimaryWindow(a.windowID)
	resumeActiveTabID := ""
	if primaryWindow {
		resumeActiveTabID = runtime.takeResumeActiveTab()
	}
	a.activeTabID = workspace.RecentTabID
	a.documentSessionState = newDocumentSessionState("")
	if primaryWindow {
		if resumeActiveTabID != "" && resumeActiveTabID != workspace.RecentTabID && a.sessions[resumeActiveTabID] != nil {
			a.activateSession(resumeActiveTabID, a.sessions[resumeActiveTabID])
		} else if err := collection.Activate(workspace.RecentTabID); err != nil {
			a.setStatus(err.Error())
		}
	}
	a.refreshHomeMetadata()
}

func hasRestorableSnapshot(record workspace.Session) bool {
	return record.Format != "" && (len(record.Payload) > 0 || strings.EqualFold(record.Format, "json") || strings.EqualFold(record.Format, "markdown") || strings.EqualFold(record.Format, "md") || isEmptyDraftWithoutSnapshot(record))
}

func isEmptyDraftWithoutSnapshot(record workspace.Session) bool {
	if record.Tab.Kind != "draft" || record.Tab.SourcePath != "" || len(record.Payload) != 0 {
		return false
	}
	switch strings.ToLower(record.Format) {
	case "pdf", "docx", "xlsx", "json", "markdown", "md", "csv":
		return true
	default:
		return false
	}
}

func (a *App) restoreSessionState(record workspace.Session) (*documentSessionState, error) {
	state := newDocumentSessionState(record.Tab.SourcePath)
	state.documentID = record.Tab.DocumentID
	switch strings.ToLower(record.Format) {
	case "xlsx":
		var book *xlsx.Workbook
		if isEmptyDraftWithoutSnapshot(record) {
			book = xlsx.NewBlank()
		} else {
			var err error
			book, err = xlsx.OpenSnapshot(record.Payload)
			if err != nil {
				return nil, err
			}
		}
		state.book = book
		book.SetSourcePath(record.Tab.SourcePath)
		book.SetDirty(record.Dirty)
		state.sheets = book.SheetNames()
		initializeSpreadsheetSheetZooms(state.sharedDocumentSession, book, state.sheets)
		if len(state.sheets) > 0 {
			state.sheet = state.sheets[0]
		}
		if len(state.sheets) > 0 {
			state.virtualGrid = newGridViewport()
			state.selectedCell = "A1"
			state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = 1, 1
			state.virtualGrid.PendingSelectionLoad = true
			state.cellEdit.selectCell("A1", "")
		}
	case "csv":
		var book *csvdoc.Document
		if isEmptyDraftWithoutSnapshot(record) {
			book = csvdoc.NewBlank()
		} else if len(record.Payload) == 0 && record.Tab.SourcePath != "" {
			var err error
			book, err = csvdoc.Open(record.Tab.SourcePath)
			if err != nil {
				return nil, err
			}
		} else {
			var err error
			book, err = csvdoc.OpenSnapshot(record.Payload)
			if err != nil {
				return nil, err
			}
		}
		state.book = book
		state.sourceFormat = application.FormatCSV
		book.SetSourcePath(record.Tab.SourcePath)
		book.SetDirty(record.Dirty)
		state.sheets = book.SheetNames()
		initializeSpreadsheetSheetZooms(state.sharedDocumentSession, book, state.sheets)
		if len(state.sheets) > 0 {
			state.sheet = state.sheets[0]
		}
		if len(state.sheets) > 0 {
			state.virtualGrid = newGridViewportForSpreadsheet(book, state.sheet)
			state.selectedCell = "A1"
			state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = 1, 1
			state.virtualGrid.PendingSelectionLoad = true
			state.cellEdit.selectCell("A1", "")
		}
	case "docx":
		var word *docx.Document
		if isEmptyDraftWithoutSnapshot(record) {
			var err error
			word, err = docx.NewBlank()
			if err != nil {
				return nil, err
			}
		} else {
			var err error
			word, err = docx.OpenSnapshot(record.Payload)
			if err != nil {
				return nil, err
			}
		}
		state.word = word
		if protEditor, ok := any(word).(document.WordDocumentProtectionEditor); ok {
			state.wordProtected, _ = protEditor.GetWordDocumentProtection()
		}
		if revEditor, ok := any(word).(document.WordTrackRevisionsEditor); ok {
			state.wordTrackRevisions, _ = revEditor.GetTrackRevisions()
		}
		state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
		if strings.EqualFold(filepath.Ext(record.Tab.SourcePath), ".doc") {
			// Recovery snapshots for imported DOC files contain the normalized
			// DOCX package; never attach the legacy source path to its DOCX saver.
			word.SetSourcePath("")
			state.sourceFormat = application.FormatDOC
			state.saveAsRequired = true
		} else {
			word.SetSourcePath(record.Tab.SourcePath)
		}
		word.SetDirty(record.Dirty)
	case "pdf":
		var d *pdfdoc.Document
		var err error
		if isEmptyDraftWithoutSnapshot(record) {
			d, err = pdfdoc.NewBlank()
		} else {
			d, err = pdfdoc.OpenSnapshot(record.Payload)
		}
		if err != nil {
			return nil, err
		}
		d.SetSourcePath(record.Tab.SourcePath)
		d.SetDirty(record.Dirty)
		state.pdfDoc = d
	case "md", "markdown":
		d, err := markdowndoc.OpenSnapshot(record.Payload)
		if err != nil {
			return nil, err
		}
		d.SetSourcePath(record.Tab.SourcePath)
		d.SetDirty(record.Dirty)
		state.markdownDoc = d
	case "json":
		var jsonDocument *jsondoc.Document
		if isEmptyDraftWithoutSnapshot(record) {
			jsonDocument = jsondoc.NewBlank()
		} else {
			var err error
			jsonDocument, err = jsondoc.OpenSnapshot(record.Payload)
			if err != nil {
				return nil, err
			}
		}
		state.jsonDoc = jsonDocument
		jsonDocument.SetSourcePath(record.Tab.SourcePath)
		jsonDocument.SetDirty(record.Dirty)
	default:
		return nil, fmt.Errorf("不支持恢复格式 %q", record.Format)
	}
	var view sessionViewSnapshot
	if len(record.ViewState) != 0 {
		_ = json.Unmarshal(record.ViewState, &view)
	}
	if view.Sheet != "" && containsString(state.sheets, view.Sheet) {
		state.sheet = view.Sheet
	}
	state.selectedCell = view.SelectedCell
	if state.selectedCell == "" && state.book != nil {
		state.selectedCell = "A1"
	}
	state.selectedBlock = view.SelectedBlock
	state.wordListState.Position.First = view.WordFirst
	state.wordAutoSave = view.WordAutoSave
	if state.word != nil {
		state.wordViewMode = clampWordViewMode(view.WordViewMode)
		state.wordPageFitMode = view.WordPageFitMode
		state.wordMultiplePagesMode = view.WordMultiplePagesMode
		if view.WordLayoutZoom > 0 {
			state.wordLayoutZoom = wordStatusZoomClamp(view.WordLayoutZoom)
		}
		state.wordRulerHidden = view.WordRulerHidden
		state.wordGridlinesVisible = view.WordGridlinesVisible
		state.wordNavigationVisible = view.WordNavigationVisible
		state.wordNavigationSearchQuery = view.WordNavigationQuery
		state.wordNavigationSearchEditor.SetText(view.WordNavigationQuery)
		state.wordToolbarTab = clampWordToolbarTab(view.WordToolbarTab)
		state.wordPageListState.Position.First = max(0, view.WordPageFirst)
		restorePersistedWordSelection(state, view)
	}
	if state.virtualGrid != nil {
		state.virtualGrid.X, state.virtualGrid.Y = view.GridX, view.GridY
		state.restoreSpreadsheetSheetZooms(view.SpreadsheetZooms)
		if len(view.SpreadsheetZooms) == 0 && view.SpreadsheetZoom > 0 && state.sheet != "" {
			state.setSpreadsheetSheetZoom(state.sheet, int(math.Round(spreadsheetZoomClamp(view.SpreadsheetZoom)*100)))
		}
		if view.SpreadsheetZoomPending {
			state.restoreSpreadsheetZoomPending()
		}
		state.spreadsheetZoom = state.spreadsheetSheetZoom(state.sheet)
		if view.GridScale > 0 {
			state.virtualGrid.setScale(view.GridScale)
		}
		if view.SelectedRow > 0 {
			state.virtualGrid.SelectedRow = view.SelectedRow
		}
		if view.SelectedColumn > 0 {
			state.virtualGrid.SelectedColumn = view.SelectedColumn
		}
		position := clampCellPosition(CellPosition{Row: state.virtualGrid.SelectedRow - 1, Column: state.virtualGrid.SelectedColumn - 1})
		state.selection = CellSelection{Anchor: position, Active: position}
		state.selectionSet = view.SelectionSet
	}
	if state.markdownDoc != nil {
		v := newMarkdownView(state.markdownDoc.Text())
		v.mode = min(2, max(0, view.MarkdownMode))
		v.collapsed = view.MarkdownCollapsed
		start, end := clampEditorSelection(view.MarkdownStart, view.MarkdownEnd, v.editor.Text())
		v.editor.SetCaret(start, end)
		v.preview.Position.First = max(0, view.MarkdownPreviewFirst)
		v.search.SetText(view.MarkdownQuery)
		state.markdownView = v
	}
	if state.selectionSet {
		state.nameBox.SetText(state.selectedCell)
	} else {
		state.nameBox.SetText("")
	}
	return state, nil
}

func clampWordViewMode(mode wordViewMode) wordViewMode {
	if mode > wordViewWebLayout {
		return wordViewContinuous
	}
	return mode
}

func clampWordToolbarTab(tab wordToolbarTab) wordToolbarTab {
	if tab > wordTabPicture {
		return wordTabHome
	}
	return tab
}

func restorePersistedWordSelection(state *documentSessionState, view sessionViewSnapshot) {
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	state.selectedWordRunID = ""
	state.selectedWordParagraphID = ""
	state.selectedWordImageID = ""
	if run, paragraph, ok := findEditableWordRun(blocks, view.SelectedWordRunID); ok {
		state.selectedWordRunID = run.ID
		state.selectedWordParagraphID = paragraph.ID
		state.selectedBlock = -1
		state.selectedBlockEditable = true
		if editor := wordRunEditor(state, run); editor != nil {
			start, end := clampEditorSelection(view.SelectedWordSelectionStart, view.SelectedWordSelectionEnd, editor.Text())
			editor.SetCaret(start, end)
		}
		return
	}
	if view.SelectedWordImageID != "" && findWordImage(blocks, view.SelectedWordImageID) != nil {
		state.selectedWordImageID = view.SelectedWordImageID
		if paragraph, ok := selectedWordParagraph(blocks, view.SelectedWordParagraphID); ok {
			state.selectedWordParagraphID = paragraph.ID
		}
		return
	}
	if paragraph, ok := selectedWordParagraph(blocks, view.SelectedWordParagraphID); ok {
		state.selectedWordParagraphID = paragraph.ID
	}
}

func (a *App) activateSession(id string, state *documentSessionState) {
	runtime := a.ensureDesktopRuntime()
	if state == nil || state.closed.Load() {
		return
	}
	if a.documentSessionState != state {
		a.prepareMarkdownDeparture()
	}
	a.documentGeneration++
	a.closeWordVersionHistory()
	if a.activeTabID != "" && a.documentSessionState != nil {
		if a.documentSessionState.valueFilter != nil {
			a.clearValueFilter(a.documentSessionState)
		}
		a.sessions[a.activeTabID] = a.documentSessionState
	}
	a.documentSessionState = state
	state.windowID = a.windowID
	state.sharedDocumentSession.attachView(a.windowID, state)
	a.activeTabID = id
	a.setActiveSessionID(id)
	state.documentID = id
	if a.collection != nil && runtime.isPrimaryWindow(a.windowID) {
		if err := a.collection.Activate(id); err != nil {
			a.setStatus(err.Error())
		}
	}
	if meta, ok := a.tabMeta[id]; ok && meta.SourcePath != "" {
		if _, err := os.Stat(meta.SourcePath); err != nil {
			a.setStatus("源文件不可用，正在显示本地恢复快照：" + meta.SourcePath)
		}
	}
	if state.word != nil {
		a.startWordPagination(state)
	}
	if state.book != nil {
		if state.sheet == "" && len(state.sheets) > 0 {
			state.sheet = state.sheets[0]
		}
		if state.sheet != "" {
			setSpreadsheetZoomView(state, state.spreadsheetSheetZoom(state.sheet))
		}
	}
	a.maybeAutoSaveWord(state)
	a.surfacePendingWordRestoreError(state)
}

func (a *App) snapshotActive() ([]byte, error) {
	switch {
	case a.book != nil:
		if snapshotter, ok := a.book.(interface{ Snapshot() ([]byte, error) }); ok {
			return snapshotter.Snapshot()
		}
		return nil, fmt.Errorf("当前表格格式不支持工作区快照")
	case a.word != nil:
		return a.word.Snapshot()
	case a.pdfDoc != nil:
		return a.pdfDoc.Snapshot()
	case a.markdownDoc != nil:
		return a.markdownDoc.Snapshot()
	case a.jsonDoc != nil:
		return a.jsonDoc.Snapshot()
	default:
		return nil, fmt.Errorf("没有可保存的活动文档")
	}
}

func (a *App) persistActiveSession() {
	runtime := a.ensureDesktopRuntime()
	if !runtime.isPrimaryWindow(a.windowID) {
		return
	}
	if a.collection == nil || a.activeTabID == "" || a.activeTabID == workspace.RecentTabID {
		return
	}
	state := a.documentSessionState
	meta := a.tabMeta[a.activeTabID]
	format := sessionFormat(meta, state)
	job := workspaceWriteJob{documentID: meta.DocumentID, format: format, state: state, viewState: a.viewStateBytes()}
	if a.workspaceWriteJobs != nil {
		a.workspaceWriteJobs <- job
		return
	}
	payload, err := snapshotState(state)
	if err == nil {
		err = a.collection.Update(a.activeTabID, payload, job.viewState, sessionDirty(state))
	}
	if err != nil {
		a.setStatus("工作区保存失败：" + err.Error())
	}
}
