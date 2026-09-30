package desktopui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	legacyxls "glance/internal/format/xls"
	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

const (
	maxPreviewRows    = 300
	maxPreviewColumns = 16
)

const preferredSystemTypeface = font.Typeface("Inter, PingFang SC, Hiragino Sans GB, Microsoft YaHei, Noto Sans CJK SC, WenQuanYi Micro Hei, sans-serif")

type App struct {
	*documentSessionState
	window *app.Window
	ops    op.Ops
	theme  *material.Theme

	actionOpenFile   widget.Clickable
	actionOpenFolder widget.Clickable
	actionSave       widget.Clickable
	actionSaveAs     widget.Clickable
	actionNewXLSX    widget.Clickable
	actionNewDOCX    widget.Clickable
	actionNewJSON    widget.Clickable
	applyButton      widget.Clickable
	goToCellButton   widget.Clickable

	status                 string
	collection             *workspace.Collection
	workspaceStore         *workspace.Store
	sessions               map[string]*documentSessionState
	tabMeta                map[string]workspace.Tab
	activeTabID            string
	workspaceError         string
	workspaceWriteError    string
	workspaceDBMu          sync.Mutex
	tabButtons             map[string]*widget.Clickable
	tabCloseButtons        map[string]*widget.Clickable
	newButton              widget.Clickable
	recentTabButton        widget.Clickable
	workspaceTabsList      widget.List
	createActionsList      widget.List
	createMenuList         widget.List
	windowCloseButton      widget.Clickable
	windowMinimizeButton   widget.Clickable
	windowMaximizeButton   widget.Clickable
	windowMode             app.WindowMode
	clientTitlebar         bool
	windowDecorations      widget.Decorations
	showNewMenu            bool
	modal                  modalDialog
	pendingCloseTabID      string
	recentList             widget.List
	homeSection            homeSection
	homeTypeFilter         string
	homeSearch             widget.Editor
	homeSortModifiedDesc   bool
	homeContentList        widget.List
	homeActionButtons      map[string]*widget.Clickable
	homeNavButtons         map[string]*widget.Clickable
	homeFilterButtons      map[string]*widget.Clickable
	homeOpenButtons        map[string]*widget.Clickable
	homeFavoriteButtons    map[string]*widget.Clickable
	homeMoreButtons        map[string]*widget.Clickable
	homeFolderButton       widget.Clickable
	homeNewFolderButton    widget.Clickable
	homeFolderAddButton    widget.Clickable
	homeSortButton         widget.Clickable
	homeFavorites          map[string]bool
	homeMetadata           map[string]homeFileMetadata
	homeMetadataResults    chan homeMetadataResult
	homeMetadataGeneration uint64
	workspaceWriteJobs     chan workspaceWriteJob
	workspaceWriteResults  chan error
	workspaceWriterDone    chan struct{}
	fileCommandResults     chan fileCommandResult
	gridLoadResults        chan gridLoadResult
	gridPictureMetaResults chan gridPictureMetadataResult
	gridPictureResults     chan gridPicturePreviewResult
	gridPictureCommands    chan gridPictureCommandResult
	formatResults          chan spreadsheetFormatResult
	wordCommandResults     chan wordCommandResult
	wordImageResults       chan wordImageDecodeResult
	wordImagePickerResults chan wordImagePickerResult
	wordImagePickerRunning bool
	wordImageFilePicker    func() (string, error)
	xlsxImagePickerResults chan spreadsheetImagePickerResult
	xlsxImagePickerRunning bool
	xlsxImageFilePicker    func() (string, error)
	saveResults            chan saveResult
	openResults            chan openResult
	saving                 bool
	opening                bool
	fileCommandRunning     bool
	documentGeneration     uint64
	shutdown               chan struct{}
	shutdownOnce           sync.Once
}

func New(window *app.Window, initialPath string) *App {
	a := &App{
		window:                 window,
		theme:                  material.NewTheme(),
		documentSessionState:   newDocumentSessionState(initialPath),
		activeTabID:            workspace.RecentTabID,
		status:                 "打开一个 .xlsx 或 .docx 文件开始预览",
		gridLoadResults:        make(chan gridLoadResult, 8),
		gridPictureMetaResults: make(chan gridPictureMetadataResult, 8),
		gridPictureResults:     make(chan gridPicturePreviewResult, 64),
		gridPictureCommands:    make(chan gridPictureCommandResult, 8),
		formatResults:          make(chan spreadsheetFormatResult, 8),
		wordCommandResults:     make(chan wordCommandResult, 8),
		wordImageResults:       make(chan wordImageDecodeResult, 64),
		wordImagePickerResults: make(chan wordImagePickerResult, 1),
		xlsxImagePickerResults: make(chan spreadsheetImagePickerResult, 1),
		saveResults:            make(chan saveResult, 1),
		openResults:            make(chan openResult, 1),
		fileCommandResults:     make(chan fileCommandResult, 1),
		shutdown:               make(chan struct{}),
		sessions:               make(map[string]*documentSessionState),
		tabMeta:                make(map[string]workspace.Tab),
		tabButtons:             make(map[string]*widget.Clickable),
		tabCloseButtons:        make(map[string]*widget.Clickable),
		homeSection:            homeSectionRecent,
		homeTypeFilter:         "全部",
		homeSortModifiedDesc:   true,
		homeActionButtons:      make(map[string]*widget.Clickable),
		homeNavButtons:         make(map[string]*widget.Clickable),
		homeFilterButtons:      make(map[string]*widget.Clickable),
		homeOpenButtons:        make(map[string]*widget.Clickable),
		homeFavoriteButtons:    make(map[string]*widget.Clickable),
		homeMoreButtons:        make(map[string]*widget.Clickable),
		homeFavorites:          make(map[string]bool),
		homeMetadata:           make(map[string]homeFileMetadata),
		homeMetadataResults:    make(chan homeMetadataResult, 8),
		workspaceTabsList:      widget.List{List: layout.List{Axis: layout.Horizontal}},
		createActionsList:      widget.List{List: layout.List{Axis: layout.Horizontal}},
		createMenuList:         widget.List{List: layout.List{Axis: layout.Vertical}},
		homeContentList:        widget.List{List: layout.List{Axis: layout.Vertical}},
		recentList:             widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	if window != nil {
		a.initializeWorkspace()
		if a.workspaceStore != nil {
			a.startWorkspaceWriter()
		}
	}
	a.homeSearch.SingleLine = true
	a.theme.Shaper = text.NewShaper()
	a.theme.Face = preferredSystemTypeface
	return a
}

func Run(initialPath string) {
	w := new(app.Window)
	w.Option(
		app.Title("Glance Office"),
		app.Decorated(false),
		app.Size(unit.Dp(1280), unit.Dp(820)),
		app.MinSize(unit.Dp(900), unit.Dp(600)),
	)
	a := New(w, initialPath)
	if initialPath != "" {
		if err := a.open(initialPath); err != nil {
			a.status = err.Error()
		}
	}
	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			a.shutdownOnce.Do(func() { close(a.shutdown) })
			a.closeWorkspace()
			return
		case app.ConfigEvent:
			a.applyWindowConfig(e.Config)
			a.window.Invalidate()
		case app.FrameEvent:
			gtx := app.NewContext(&a.ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *App) open(path string) error {
	if a.saving {
		return fmt.Errorf("正在保存当前文件，请稍后再打开")
	}
	loaded, err := loadDocument(path)
	if err != nil {
		return err
	}
	return a.installDocument(loaded)
}

type loadedDocument struct {
	path       string
	book       document.Spreadsheet
	word       *docx.Document
	wordBlocks []document.WordBlock
	jsonDoc    *jsondoc.Document
	sheets     []string
}
type openResult struct {
	loaded loadedDocument
	err    error
}

func cloneWordBlockSnapshot(blocks []document.WordBlock) []document.WordBlock {
	return append([]document.WordBlock{}, blocks...)
}

func loadDocument(path string) (loadedDocument, error) {
	path = strings.TrimSpace(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx":
		book, err := xlsx.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		sheets := book.SheetNames()
		if len(sheets) == 0 {
			_ = book.Close()
			return loadedDocument{}, fmt.Errorf("工作簿没有可显示的工作表")
		}
		return loadedDocument{path: path, book: book, sheets: sheets}, nil
	case ".docx":
		word, err := docx.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		return loadedDocument{path: path, word: word, wordBlocks: cloneWordBlockSnapshot(word.WordBlocks())}, nil
	case ".json":
		jsonDocument, err := jsondoc.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		return loadedDocument{path: path, jsonDoc: jsonDocument}, nil
	case ".xls":
		book, err := legacyxls.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		sheets := book.SheetNames()
		if len(sheets) == 0 {
			_ = book.Close()
			return loadedDocument{}, fmt.Errorf("旧版工作簿没有可显示的工作表")
		}
		return loadedDocument{path: path, book: book, sheets: sheets}, nil
	default:
		return loadedDocument{}, fmt.Errorf("暂不支持 %s 文件，目前支持 .xlsx、.docx 和 .json", filepath.Ext(path))
	}
}

func (a *App) installDocument(loaded loadedDocument) error {
	a.documentGeneration++
	// Blank adapter constructors mark their drafts dirty so standalone callers
	// know they have no saved path. In the workspace, a newly created empty
	// document has no user edits to protect yet and must be closable.
	if loaded.path == "" {
		if book, ok := loaded.book.(interface{ SetDirty(bool) }); ok {
			book.SetDirty(false)
		}
		if loaded.word != nil {
			loaded.word.SetDirty(false)
		}
		if loaded.jsonDoc != nil {
			loaded.jsonDoc.SetDirty(false)
		}
	}
	for id, meta := range a.tabMeta {
		if sameDocumentPath(meta.SourcePath, loaded.path) {
			if state := a.sessions[id]; state != nil {
				closeLoadedDocument(loaded)
				a.activateSession(id, state)
				return nil
			}
		}
	}
	if a.activeTabID != "" && a.documentSessionState != nil {
		a.sessions[a.activeTabID] = a.documentSessionState
	}
	state := newDocumentSessionState(loaded.path)
	state.book, state.word = loaded.book, loaded.word
	if loaded.word != nil {
		state.wordBlocks = cloneWordBlockSnapshot(loaded.wordBlocks)
		if loaded.wordBlocks == nil {
			state.wordBlocks = cloneWordBlockSnapshot(loaded.word.WordBlocks())
		}
	}
	a.documentSessionState = state
	a.book, a.word, a.sheets = loaded.book, loaded.word, loaded.sheets
	a.jsonDoc = loaded.jsonDoc
	a.documentSessionState.jsonDoc = loaded.jsonDoc
	a.path = loaded.path
	if loaded.book != nil && len(loaded.sheets) > 0 {
		a.sheet = loaded.sheets[0]
	}
	if loaded.book != nil {
		a.cellButtons = make(map[string]*widget.Clickable)
		a.sheetButtons = make(map[string]*widget.Clickable)
	}
	if loaded.word != nil {
		a.blockButtons = make(map[int]*widget.Clickable)
		a.selectedBlock = -1
	}
	id := workspaceSessionID(loaded.path)
	kind := "document"
	if loaded.path == "" {
		kind = "draft"
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(loaded.path)), ".")
	if format == "" {
		switch {
		case loaded.book != nil:
			format = "xlsx"
		case loaded.word != nil:
			format = "docx"
		case loaded.jsonDoc != nil:
			format = "json"
		}
	}
	if loaded.path == "" {
		id = newDraftSessionID(format)
	}
	title := filepath.Base(loaded.path)
	if loaded.path == "" {
		title = "新建文档"
	}
	meta := workspace.Tab{ID: id, Kind: kind, Title: title, DocumentID: id, SourcePath: loaded.path, Position: len(a.tabMeta)}
	a.activeTabID = id
	a.tabMeta[id] = meta
	a.sessions[id] = state
	if a.collection != nil {
		if format != "xls" {
			if err := a.collection.AddTab(meta, format); err != nil {
				return err
			}
			a.reindexTabMetadata()
			if loaded.path != "" {
				a.collection.StageRecents([]workspace.Recent{{Path: loaded.path, Kind: format}})
				a.refreshHomeMetadata()
				a.persistRecentsAsync()
			}
			a.persistActiveSession()
		}
	}
	if loaded.book != nil {
		a.sheet = loaded.sheets[0]
		a.cellButtons = make(map[string]*widget.Clickable)
		if _, ok := loaded.book.(document.GridReader); ok {
			a.virtualGrid = newGridViewport()
			a.selectedCell = "A1"
			state.selection = CellSelection{}
			state.selectionSet = true
			a.virtualGrid.SelectedRow, a.virtualGrid.SelectedColumn = 1, 1
			a.virtualGrid.PendingSelectionLoad = true
			a.cellEdit.selectCell("A1", "")
			a.nameBox.SetText("A1")
			a.formulaEditor.SetText("")
			a.status = fmt.Sprintf("工作表「%s」：虚拟网格就绪，可浏览并编辑完整 XLSX 地址范围", a.sheet)
			return nil
		}
		a.sheet = loaded.sheets[0]
		a.cellButtons = make(map[string]*widget.Clickable)
		a.status = "旧版 .xls 只读预览（当前显示首张工作表）"
		return a.loadSheet()
	}
	a.blockButtons = make(map[int]*widget.Clickable)
	a.selectedBlock = -1
	if loaded.word != nil {
		a.status = fmt.Sprintf("已打开 %s；读取 %d 个内容块", loaded.path, len(a.word.Blocks()))
	} else if loaded.jsonDoc != nil {
		a.status = "已打开 JSON 文本"
	}
	return nil
}

func closeLoadedDocument(loaded loadedDocument) {
	if loaded.book != nil {
		_ = loaded.book.Close()
	}
	if loaded.word != nil {
		_ = loaded.word.Close()
	}
	if loaded.jsonDoc != nil {
		_ = loaded.jsonDoc.Close()
	}
}

func (a *App) startOpen(path string) {
	if a.opening || a.saving {
		return
	}
	a.opening = true
	a.status = "正在打开 " + strings.TrimSpace(path)
	go func() {
		loaded, err := loadDocument(path)
		select {
		case <-a.shutdown:
			if loaded.book != nil {
				_ = loaded.book.Close()
			}
			if loaded.word != nil {
				_ = loaded.word.Close()
			}
			return
		default:
		}
		select {
		case a.openResults <- openResult{loaded: loaded, err: err}:
			a.window.Invalidate()
		case <-a.shutdown:
			if loaded.book != nil {
				_ = loaded.book.Close()
			}
			if loaded.word != nil {
				_ = loaded.word.Close()
			}
		}
	}()
}

type saveResult struct {
	path   string
	tabID  string
	saveAs bool
	err    error
}

func (a *App) startSave() {
	if a.saving || !a.isDirty() {
		return
	}
	book, word, jsonDocument, path, tabID := a.book, a.word, a.jsonDoc, a.path, a.activeTabID
	a.saving = true
	go func() {
		var err error
		if book != nil {
			err = book.Save()
		} else if word != nil {
			err = word.Save()
		} else if jsonDocument != nil {
			err = jsonDocument.Save()
		}
		select {
		case a.saveResults <- saveResult{path: path, tabID: tabID, err: err}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}()
}

func (a *App) startSaveAs(path string) {
	if a.saving {
		return
	}
	format := "xlsx"
	if a.word != nil {
		format = "docx"
	}
	if a.jsonDoc != nil {
		format = "json"
	}
	if err := validateSaveAsExtension(format, path); err != nil {
		a.status = err.Error()
		if a.pendingCloseTabID != "" {
			a.showCloseConfirmation(a.pendingCloseTabID, "保存失败", "文件扩展名不匹配。可以重试保存、不保存并关闭，或取消关闭。")
		}
		return
	}
	book, word, jsonDocument, tabID := a.book, a.word, a.jsonDoc, a.activeTabID
	a.saving = true
	go func() {
		var err error
		switch {
		case book != nil:
			if saver, ok := book.(interface{ SaveAsDocument(string) error }); ok {
				err = saver.SaveAsDocument(path)
			} else {
				err = fmt.Errorf("当前表格格式不支持另存为")
			}
		case word != nil:
			err = word.SaveAsDocument(path)
		case jsonDocument != nil:
			err = jsonDocument.SaveAs(path)
		default:
			err = fmt.Errorf("没有活动文档")
		}
		select {
		case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, err: err}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}()
}

func validateSaveAsExtension(format, path string) error {
	want := map[string]string{"xlsx": ".xlsx", "docx": ".docx", "json": ".json"}[format]
	if want == "" {
		return fmt.Errorf("不支持 %s 格式另存为", format)
	}
	if !strings.EqualFold(filepath.Ext(path), want) {
		return fmt.Errorf("另存为扩展名必须是 %s", want)
	}
	return nil
}

func (a *App) loadSheet() error {
	if a.book == nil {
		return nil
	}
	rows, err := a.book.Range(a.sheet, maxPreviewRows, maxPreviewColumns)
	if err != nil {
		return err
	}
	a.grid = rows
	a.gridList.Position.First = 0
	nonEmpty := 0
	for _, row := range rows {
		for _, cell := range row {
			if cell.Value != "" {
				nonEmpty++
			}
		}
	}
	if len(rows) == 0 {
		a.status = fmt.Sprintf("工作表「%s」为空（未读到单元格内容）", a.sheet)
	} else if len(rows) == maxPreviewRows {
		a.status = fmt.Sprintf("工作表「%s」：已读入前 %d 行，%d 个非空单元格", a.sheet, len(rows), nonEmpty)
	} else {
		a.status = fmt.Sprintf("工作表「%s」：已读入 %d 行，%d 个非空单元格", a.sheet, len(rows), nonEmpty)
	}
	return nil
}

func (a *App) applyCellEdit() error {
	if a.book == nil || a.selectedCell == "" {
		return fmt.Errorf("请先选择单元格")
	}
	if err := a.book.SetCellText(a.sheet, a.selectedCell, a.cellEditor.Text()); err != nil {
		return err
	}
	if _, ok := a.book.(document.GridReader); ok {
		if a.virtualGrid != nil {
			a.virtualGrid.invalidateWindow()
		}
		return nil
	}
	if err := a.loadSheet(); err != nil {
		return err
	}
	a.persistActiveSession()
	return nil
}

func (a *App) commitGridCell(value string) error {
	if a.saving {
		return fmt.Errorf("正在保存，请稍后再编辑单元格")
	}
	if a.book == nil || a.cellEdit.Address == "" {
		return fmt.Errorf("请先选择单元格")
	}
	a.cellEdit.setDraft(value)
	changed := a.cellEdit.Draft != a.cellEdit.Original
	err := a.cellEdit.commit(func(address, value string) error { return a.book.SetCellText(a.sheet, address, value) })
	if err != nil {
		return err
	}
	a.selectedCell = a.cellEdit.Address
	if changed && a.virtualGrid != nil {
		a.virtualGrid.invalidateWindow()
	}
	a.cellEditor.SetText(a.cellEdit.Original)
	a.formulaEditor.SetText(a.cellEdit.Original)
	if changed {
		a.status = fmt.Sprintf("%s 已修改；点击保存写入原文件", a.selectedCell)
		a.persistActiveSession()
	}
	return nil
}

func (a *App) applyWordEdit() error {
	if a.word == nil || a.selectedBlock < 0 {
		return fmt.Errorf("请先选择一个段落")
	}
	if !a.selectedBlockEditable {
		return fmt.Errorf("该段落包含多个格式片段，暂不支持编辑")
	}
	if err := a.word.ReplaceParagraph(a.selectedBlock, a.wordEditor.Text()); err != nil {
		return err
	}
	a.documentSessionState.wordBlocks = cloneWordBlockSnapshot(a.word.WordBlocks())
	a.persistActiveSession()
	return nil
}

func (a *App) save() error {
	switch {
	case a.book != nil:
		if err := a.book.Save(); err != nil {
			return err
		}
	case a.word != nil:
		if err := a.word.Save(); err != nil {
			return err
		}
	case a.jsonDoc != nil:
		if a.path == "" {
			return fmt.Errorf("JSON 草稿需要另存为")
		}
		if err := a.jsonDoc.Save(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("没有打开的 Office 文件")
	}
	return nil
}

func (a *App) isDirty() bool {
	return (a.book != nil && a.book.Dirty()) || (a.word != nil && a.word.Dirty()) || (a.jsonDoc != nil && a.jsonDoc.Dirty())
}

func (a *App) closeDocument() error {
	a.documentGeneration++
	var err error
	if a.book != nil {
		err = a.book.Close()
		a.book = nil
	}
	if a.word != nil {
		if closeErr := a.word.Close(); err == nil {
			err = closeErr
		}
		a.word = nil
	}
	if a.jsonDoc != nil {
		if closeErr := a.jsonDoc.Close(); err == nil {
			err = closeErr
		}
		a.jsonDoc = nil
	}
	a.sheets, a.sheet, a.grid = nil, "", nil
	a.virtualGrid = nil
	a.cellEdit = cellEditState{}
	a.selectedCell = ""
	a.nameBox.SetText("")
	a.formulaEditor.SetText("")
	a.path = ""
	return err
}

func (a *App) cellButton(address string) *widget.Clickable {
	if button := a.cellButtons[address]; button != nil {
		return button
	}
	button := new(widget.Clickable)
	a.cellButtons[address] = button
	return button
}

func (a *App) sheetButton(name string) *widget.Clickable {
	if button := a.sheetButtons[name]; button != nil {
		return button
	}
	button := new(widget.Clickable)
	a.sheetButtons[name] = button
	return button
}

func (a *App) blockButton(index int) *widget.Clickable {
	if button := a.blockButtons[index]; button != nil {
		return button
	}
	button := new(widget.Clickable)
	a.blockButtons[index] = button
	return button
}

func (a *App) wordList() *widget.List { return &a.wordListState }
