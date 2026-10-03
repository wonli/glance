package desktopui

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"glance/internal/document"
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
	WordAutoSave   bool    `json:"word_auto_save"`
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
	a.loadHomeFavorites()
	for _, record := range collection.Tabs() {
		a.tabMeta[record.Tab.ID] = record.Tab
		if !hasRestorableSnapshot(record) {
			continue
		}
		state, err := a.restoreSessionState(record)
		if err != nil {
			a.status = "恢复 " + record.Tab.Title + " 失败：" + err.Error()
			continue
		}
		a.sessions[record.Tab.ID] = state
	}
	// Restore all document sessions, but start on Glance's home tab.
	a.activeTabID = workspace.RecentTabID
	a.documentSessionState = newDocumentSessionState("")
	if err := collection.Activate(workspace.RecentTabID); err != nil {
		a.status = err.Error()
	}
	a.refreshHomeMetadata()
}

func hasRestorableSnapshot(record workspace.Session) bool {
	return record.Format != "" && (len(record.Payload) > 0 || strings.EqualFold(record.Format, "json"))
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
		state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
		state.documentID = record.Tab.DocumentID
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
	state.wordAutoSave = view.WordAutoSave
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
		position := clampCellPosition(CellPosition{Row: state.virtualGrid.SelectedRow - 1, Column: state.virtualGrid.SelectedColumn - 1})
		state.selection = CellSelection{Anchor: position, Active: position}
		state.selectionSet = true
	}
	state.nameBox.SetText(state.selectedCell)
	return state, nil
}

func (a *App) activateSession(id string, state *documentSessionState) {
	a.documentGeneration++
	if a.activeTabID != "" && a.documentSessionState != nil {
		if a.documentSessionState.valueFilter != nil {
			a.clearValueFilter(a.documentSessionState)
		}
		a.sessions[a.activeTabID] = a.documentSessionState
	}
	a.documentSessionState = state
	a.activeTabID = id
	state.documentID = id
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
	if state.word != nil {
		a.startWordPagination(state)
	}
	a.maybeAutoSaveWord(state)
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
	recents            []workspace.Recent
	favoritePath       string
	favoriteValue      bool
	saveFavorite       bool
}

type fileCommandKind uint8

const (
	commandOpenFile fileCommandKind = iota + 1
	commandOpenFolder
	commandSaveAs
	commandExportPDF
	commandShareCopy
)

type fileCommandResult struct {
	kind    fileCommandKind
	path    string
	recents []workspace.Recent
	err     error
}

func (a *App) startOpenFilePicker() {
	a.startFileCommand(commandOpenFile, "")
}

func (a *App) startFolderPicker() {
	a.startFileCommand(commandOpenFolder, "")
}

func (a *App) startSaveAsPicker(format string) {
	a.startFileCommand(commandSaveAs, format)
}

func (a *App) startWordPDFSavePicker(state *documentSessionState, key wordLayoutKey) {
	if state == nil || state.word == nil || a.fileCommandRunning {
		return
	}
	a.wordPDFPickerRequest = &wordPDFPickerRequest{state: state, key: key}
	a.startFileCommand(commandExportPDF, "pdf")
}

// setWordFileCommandRunner is a narrow injection seam for exercising the Word
// ribbon's file commands without opening a native platform picker.
func (a *App) setWordFileCommandRunner(runner func(fileCommandKind, string) fileCommandResult) {
	a.fileCommandRunner = runner
}

func (a *App) startFileCommand(kind fileCommandKind, format string) {
	if a.fileCommandRunning {
		return
	}
	a.fileCommandRunning = true
	switch kind {
	case commandOpenFile:
		a.status = "选择要打开的文件…"
	case commandOpenFolder:
		a.status = "选择要浏览的文件夹…"
	case commandSaveAs:
		a.status = "选择另存为位置…"
	case commandExportPDF:
		a.status = "选择 PDF 保存位置…"
	case commandShareCopy:
		a.status = "选择共享 DOCX 副本保存位置…"
	}
	runner := a.fileCommandRunner
	go func() {
		result := fileCommandResult{kind: kind}
		if runner != nil {
			result = runner(kind, format)
			result.kind = kind
		} else {
			switch kind {
			case commandOpenFile:
				result.path, result.err = chooseOfficeFile()
			case commandOpenFolder:
				result.path, result.err = chooseOfficeFolder()
				if result.err == nil && result.path != "" {
					result.recents, result.err = scanOfficeFolder(result.path)
				}
			case commandSaveAs, commandShareCopy:
				result.path, result.err = chooseSavePath(format)
			}
		}
		select {
		case a.fileCommandResults <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}()
}

func scanOfficeFolder(path string) ([]workspace.Recent, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var recents []workspace.Recent
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".xlsx" && ext != ".xls" && ext != ".docx" && ext != ".json" {
			continue
		}
		recents = append(recents, workspace.Recent{Path: filepath.Join(path, entry.Name()), Kind: strings.TrimPrefix(ext, ".")})
	}
	return recents, nil
}

func (a *App) persistRecentsAsync() {
	if a.collection == nil {
		return
	}
	recents := a.collection.Recents()
	if a.workspaceWriteJobs != nil {
		a.workspaceWriteJobs <- workspaceWriteJob{recents: recents}
		return
	}
	if a.workspaceStore != nil {
		if err := a.workspaceStore.SaveRecents(recents); err != nil {
			a.status = "工作区保存失败：" + err.Error()
		}
	}
}

func (a *App) startWorkspaceWriter() {
	a.workspaceWriteJobs = make(chan workspaceWriteJob, 128)
	a.workspaceWriteResults = make(chan error, 1)
	a.workspaceWriterDone = make(chan struct{})
	go func() {
		defer close(a.workspaceWriterDone)
		for job := range a.workspaceWriteJobs {
			var err error
			if job.recents != nil {
				err = a.workspaceStore.SaveRecents(job.recents)
			} else if job.saveFavorite {
				err = a.workspaceStore.SaveFavorite(job.favoritePath, job.favoriteValue)
			} else {
				err = a.writeWorkspaceSnapshot(job)
			}
			a.publishWorkspaceWriteResult(err)
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	}()
}

func (a *App) publishWorkspaceWriteResult(err error) {
	if a.workspaceWriteResults == nil {
		return
	}
	select {
	case a.workspaceWriteResults <- err:
		return
	default:
	}
	select {
	case <-a.workspaceWriteResults:
	default:
	}
	select {
	case a.workspaceWriteResults <- err:
	default:
	}
}

func (a *App) writeWorkspaceSnapshot(job workspaceWriteJob) error {
	state := job.state
	if state.closed.Load() {
		return nil
	}
	payload, err := snapshotState(state)
	if err != nil {
		return err
	}
	if a.workspaceStore == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	a.workspaceDBMu.Lock()
	defer a.workspaceDBMu.Unlock()
	if state.closed.Load() {
		return nil
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
	view := sessionViewSnapshot{Sheet: a.sheet, SelectedCell: a.selectedCell, SelectedBlock: a.selectedBlock, GridFirst: a.gridList.Position.First, WordFirst: a.wordListState.Position.First, WordAutoSave: a.wordAutoSave}
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

type wordImagePickerOperation uint8

const (
	wordImageInsert wordImagePickerOperation = iota + 1
	wordImageReplace
)

const maxWordImageFileBytes = 128 << 20

type wordImageFile struct {
	data      []byte
	mediaType string
}

type wordImagePickerResult struct {
	state         *documentSessionState
	generation    uint64
	operation     wordImagePickerOperation
	paragraphID   string
	afterInlineID string
	imageID       string
	path          string
	data          []byte
	mediaType     string
	err           error
}

func (a *App) startWordImagePicker(operation wordImagePickerOperation) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning || a.saving || a.wordImagePickerRunning {
		return
	}
	if _, ok := any(state.word).(document.WordImageEditor); !ok {
		return
	}
	result := wordImagePickerResult{state: state, generation: a.documentGeneration, operation: operation}
	switch operation {
	case wordImageInsert:
		if _, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID); !ok {
			return
		}
		result.paragraphID = state.selectedWordParagraphID
		if state.selectedWordRunID != "" {
			result.afterInlineID = state.selectedWordRunID
		} else if state.selectedWordImageID != "" {
			result.afterInlineID = state.selectedWordImageID
		}
	case wordImageReplace:
		if findWordImage(wordBlocksForState(state), state.selectedWordImageID) == nil {
			return
		}
		result.imageID = state.selectedWordImageID
	default:
		return
	}
	if a.wordImagePickerResults == nil {
		a.wordImagePickerResults = make(chan wordImagePickerResult, 1)
	}
	a.wordImagePickerRunning = true
	if operation == wordImageInsert {
		a.status = "选择要插入的 PNG/JPEG 图片…"
	} else {
		a.status = "选择用于替换的 PNG/JPEG 图片…"
	}
	choose := a.wordImageFilePicker
	if choose == nil {
		choose = chooseOfficeImageFile
	}
	shutdown, results, window := a.shutdown, a.wordImagePickerResults, a.window
	go func() {
		result.path, result.err = choose()
		if result.err == nil && result.path != "" {
			var file wordImageFile
			file, result.err = loadWordImageFile(result.path)
			result.data, result.mediaType = file.data, file.mediaType
		}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func loadWordImageFile(path string) (wordImageFile, error) {
	var mediaType, wantFormat string
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mediaType, wantFormat = "image/png", "png"
	case ".jpg", ".jpeg":
		mediaType, wantFormat = "image/jpeg", "jpeg"
	default:
		return wordImageFile{}, fmt.Errorf("仅支持 PNG/JPEG 图片")
	}
	info, err := os.Stat(path)
	if err != nil {
		return wordImageFile{}, fmt.Errorf("读取图片文件: %w", err)
	}
	if info.Size() > maxWordImageFileBytes {
		return wordImageFile{}, fmt.Errorf("图片文件超过 %d MiB", maxWordImageFileBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return wordImageFile{}, fmt.Errorf("读取图片文件: %w", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return wordImageFile{}, fmt.Errorf("图片文件损坏: %w", err)
	}
	if format != wantFormat {
		return wordImageFile{}, fmt.Errorf("图片扩展名与实际 %s 格式不匹配", format)
	}
	if err := validateWordPreviewConfig(config, format); err != nil {
		return wordImageFile{}, err
	}
	if _, err := decodeWordPreview(data); err != nil {
		return wordImageFile{}, fmt.Errorf("图片文件损坏: %w", err)
	}
	return wordImageFile{data: data, mediaType: mediaType}, nil
}

func (a *App) handleWordImagePickerResult(result wordImagePickerResult) {
	a.wordImagePickerRunning = false
	if result.state == nil || result.state.closed.Load() || result.generation != a.documentGeneration || result.state != a.documentSessionState {
		return
	}
	if result.err != nil {
		a.status = "图片读取失败：" + result.err.Error()
		return
	}
	if result.path == "" {
		a.status = "已取消选择图片"
		return
	}
	if result.state.formatRunning || a.saving {
		a.status = "文档正在处理其他操作，请稍后重试"
		return
	}
	switch result.operation {
	case wordImageInsert:
		a.runWordImageInsertCommand(result.paragraphID, result.afterInlineID, result.mediaType, result.data)
	case wordImageReplace:
		imageID := result.imageID
		a.runWordImageCommand("替换图片", imageID, func(editor document.WordImageEditor) error {
			return editor.ReplaceInlineImage(imageID, result.mediaType, result.data)
		}, false)
	default:
		a.status = "无效的图片操作"
	}
}

func (a *App) runWordImageInsertCommand(paragraphID, afterInlineID, mediaType string, data []byte) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning || a.saving {
		return
	}
	editor, ok := any(state.word).(document.WordImageEditor)
	if !ok {
		return
	}
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		imageID, err := editor.InsertInlineImage(paragraphID, afterInlineID, mediaType, data)
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "插入图片", err: err, imageID: imageID, selectInsertedImage: err == nil, blocks: blocks})
	}()
}
