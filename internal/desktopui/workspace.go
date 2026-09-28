package desktopui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

type sessionViewSnapshot struct {
	Sheet          string  `json:"sheet"`
	SelectedCell   string  `json:"selected_cell"`
	SelectedBlock  int     `json:"selected_block"`
	GridFirst      int     `json:"grid_first"`
	WordFirst      int     `json:"word_first"`
	GridX          float64 `json:"grid_x"`
	GridY          float64 `json:"grid_y"`
	GridScale      float64 `json:"grid_scale"`
	SelectedRow    int     `json:"selected_row"`
	SelectedColumn int     `json:"selected_column"`
}

func workspaceSessionID(path string) string {
	if path == "" {
		return "draft-" + fmt.Sprintf("%x", sha256.Sum256([]byte("new")))[:12]
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return fmt.Sprintf("doc-%x", sha256.Sum256([]byte(strings.ToLower(path))))[:20]
}

var draftCounter atomic.Uint64

func newDraftSessionID(format string) string {
	return fmt.Sprintf("draft-%s-%d", format, draftCounter.Add(1))
}

func (a *App) initializeWorkspace() {
	path, err := workspace.DefaultDatabasePath()
	if err != nil {
		a.workspaceError = err.Error()
		a.status = "工作区数据库不可用：" + err.Error()
		return
	}
	store, err := workspace.Open(path)
	if err != nil {
		a.workspaceError = err.Error()
		a.status = "工作区数据库不可用：" + err.Error()
		return
	}
	a.workspaceStore = store
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		a.workspaceError = err.Error()
		a.status = "恢复工作区失败：" + err.Error()
		return
	}
	a.collection = collection
	for _, record := range collection.Tabs() {
		a.tabMeta[record.Tab.ID] = record.Tab
		if record.Format == "" || len(record.Payload) == 0 {
			continue
		}
		state, err := a.restoreSessionState(record)
		if err != nil {
			a.status = "恢复 " + record.Tab.Title + " 失败：" + err.Error()
			continue
		}
		a.sessions[record.Tab.ID] = state
	}
	active := collection.ActiveTabID()
	if active != workspace.RecentTabID {
		if state := a.sessions[active]; state != nil {
			a.activateSession(active, state)
		} else {
			a.activeTabID = workspace.RecentTabID
		}
	}
}

func (a *App) restoreSessionState(record workspace.Session) (*documentSessionState, error) {
	state := newDocumentSessionState(record.Tab.SourcePath)
	switch strings.ToLower(record.Format) {
	case "xlsx":
		book, err := xlsx.OpenSnapshot(record.Payload)
		if err != nil {
			return nil, err
		}
		state.book = book
		book.SetSourcePath(record.Tab.SourcePath)
		book.SetDirty(record.Dirty)
		state.sheets = book.SheetNames()
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
	case "docx":
		word, err := docx.OpenSnapshot(record.Payload)
		if err != nil {
			return nil, err
		}
		state.word = word
		word.SetSourcePath(record.Tab.SourcePath)
		word.SetDirty(record.Dirty)
	case "json":
		jsonDocument, err := jsondoc.OpenSnapshot(record.Payload)
		if err != nil {
			return nil, err
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
	state.gridList.Position.First = view.GridFirst
	state.wordListState.Position.First = view.WordFirst
	if state.virtualGrid != nil {
		state.virtualGrid.X, state.virtualGrid.Y = view.GridX, view.GridY
		if view.GridScale > 0 {
			state.virtualGrid.setScale(view.GridScale)
		}
		if view.SelectedRow > 0 {
			state.virtualGrid.SelectedRow = view.SelectedRow
		}
		if view.SelectedColumn > 0 {
			state.virtualGrid.SelectedColumn = view.SelectedColumn
		}
	}
	state.nameBox.SetText(state.selectedCell)
	return state, nil
}

func (a *App) activateSession(id string, state *documentSessionState) {
	a.documentGeneration++
	if a.activeTabID != "" && a.documentSessionState != nil {
		a.sessions[a.activeTabID] = a.documentSessionState
	}
	a.documentSessionState = state
	a.activeTabID = id
	if a.collection != nil {
		if err := a.collection.Activate(id); err != nil {
			a.status = err.Error()
		}
	}
	if meta, ok := a.tabMeta[id]; ok && meta.SourcePath != "" {
		if _, err := os.Stat(meta.SourcePath); err != nil {
			a.status = "源文件不可用，正在显示本地恢复快照：" + meta.SourcePath
		}
	}
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
	case a.jsonDoc != nil:
		return a.jsonDoc.Snapshot()
	default:
		return nil, fmt.Errorf("没有可保存的活动文档")
	}
}

func (a *App) persistActiveSession() {
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
		a.status = "工作区保存失败：" + err.Error()
	}
}

type workspaceWriteJob struct {
	documentID, format string
	state              *documentSessionState
	viewState          []byte
}

func (a *App) startWorkspaceWriter() {
	a.workspaceWriteJobs = make(chan workspaceWriteJob, 128)
	a.workspaceWriteResults = make(chan error, 8)
	a.workspaceWriterDone = make(chan struct{})
	go func() {
		defer close(a.workspaceWriterDone)
		for job := range a.workspaceWriteJobs {
			if err := a.writeWorkspaceSnapshot(job); err != nil {
				select {
				case a.workspaceWriteResults <- err:
				default:
				}
			}
		}
	}()
}

func (a *App) writeWorkspaceSnapshot(job workspaceWriteJob) error {
	state := job.state
	payload, err := snapshotState(state)
	if err != nil {
		return err
	}
	if a.workspaceStore == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	return a.workspaceStore.SaveDocument(workspace.Document{ID: job.documentID, Format: job.format, Payload: payload, ViewState: job.viewState, Dirty: sessionDirty(state), UpdatedAt: time.Now().UTC()})
}

func snapshotState(state *documentSessionState) ([]byte, error) {
	switch {
	case state.book != nil:
		if snap, ok := state.book.(interface{ Snapshot() ([]byte, error) }); ok {
			return snap.Snapshot()
		} else {
			return nil, fmt.Errorf("当前表格格式不支持工作区快照")
		}
	case state.word != nil:
		return state.word.Snapshot()
	case state.jsonDoc != nil:
		return state.jsonDoc.Snapshot()
	default:
		return nil, fmt.Errorf("没有可保存的活动文档")
	}
}

func sessionDirty(state *documentSessionState) bool {
	return state.book != nil && state.book.Dirty() || state.word != nil && state.word.Dirty() || state.jsonDoc != nil && state.jsonDoc.Dirty()
}

func (a *App) viewStateBytes() []byte {
	view := sessionViewSnapshot{Sheet: a.sheet, SelectedCell: a.selectedCell, SelectedBlock: a.selectedBlock, GridFirst: a.gridList.Position.First, WordFirst: a.wordListState.Position.First}
	if a.virtualGrid != nil {
		view.GridX, view.GridY, view.GridScale = a.virtualGrid.X, a.virtualGrid.Y, a.virtualGrid.Scale
		view.SelectedRow, view.SelectedColumn = a.virtualGrid.SelectedRow, a.virtualGrid.SelectedColumn
	}
	data, _ := json.Marshal(view)
	return data
}

func (a *App) persistViewState() {
	if a.collection == nil || a.activeTabID == "" || a.activeTabID == workspace.RecentTabID {
		return
	}
	if err := a.collection.SaveViewState(a.activeTabID, a.viewStateBytes()); err != nil {
		a.status = "视图状态保存失败：" + err.Error()
	}
}

func (a *App) updatePersistedTab(meta workspace.Tab) {
	if a.collection == nil {
		return
	}
	state := a.sessions[meta.ID]
	if state == nil {
		return
	}
	format := sessionFormat(meta, state)
	if err := a.collection.UpdateTabMeta(meta); err != nil {
		a.status = "工作区保存失败：" + err.Error()
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
			a.status = err.Error()
		}
	}
}

func sessionFormat(meta workspace.Tab, state *documentSessionState) string {
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
