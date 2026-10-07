package desktopui

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	legacyxls "glance/internal/format/xls"
	"glance/internal/format/xlsx"
	"glance/internal/printservice"
	"glance/internal/workspace"
)

const (
	maxPreviewRows    = 300
	maxPreviewColumns = 16
)

const preferredSystemTypeface = font.Typeface("Inter, PingFang SC, Hiragino Sans GB, Microsoft YaHei, Noto Sans CJK SC, WenQuanYi Micro Hei, sans-serif")

type App struct {
	*documentSessionState
	*desktopRuntime
	window *app.Window
	ops    op.Ops
	theme  *material.Theme

	actionOpenFile        widget.Clickable
	actionOpenFolder      widget.Clickable
	actionSave            widget.Clickable
	actionSaveAs          widget.Clickable
	actionNewXLSX         widget.Clickable
	actionNewDOCX         widget.Clickable
	actionNewJSON         widget.Clickable
	actionNewMarkdown     widget.Clickable
	actionNewPDF          widget.Clickable
	actionNewPPTX         widget.Clickable
	actionCloseCreateMenu widget.Clickable
	applyButton           widget.Clickable
	goToCellButton        widget.Clickable

	feedback                feedbackState
	status                  string
	activeSessionDocumentID atomic.Value
	sessions                map[string]*documentSessionState
	tabMeta                 map[string]workspace.Tab
	activeTabID             string
	workspaceError          string
	workspaceWriteError     string
	tabButtons              map[string]*widget.Clickable
	tabCloseButtons         map[string]*widget.Clickable
	newButton               widget.Clickable
	recentTabButton         widget.Clickable
	workspaceTabsList       widget.List
	createActionsList       widget.List
	windowCloseButton       widget.Clickable
	windowOpenResults       chan error
	windowOpenSource        *App
	primaryPromotionPending atomic.Bool
	windowMinimizeButton    widget.Clickable
	windowMaximizeButton    widget.Clickable
	windowMode              app.WindowMode
	clientTitlebar          bool
	windowDecorations       widget.Decorations
	showNewMenu             bool
	modal                   modalDialog
	wordSymbolPickerVisible bool
	wordSymbolPickerScrim   widget.Clickable
	wordSymbolPickerClose   widget.Clickable
	wordSymbolButtons       [len(wordSymbolChoices)]widget.Clickable
	wordSymbolRunID         string
	wordSymbolParagraphID   string
	wordSymbolStart         int
	wordSymbolEnd           int
	wordSymbolRevision      uint64
	wordSymbolDraftEpoch    uint64
	pendingCloseTabID       string
	recentList              widget.List
	homeSection             homeSection
	homeTypeFilter          string
	homeSearch              widget.Editor
	homeSortOpenedDesc      bool
	homeContentList         widget.List
	homeActionButtons       map[string]*widget.Clickable
	homeNavButtons          map[string]*widget.Clickable
	homeFilterButtons       map[string]*widget.Clickable
	homeOpenButtons         map[string]*widget.Clickable
	homeFavoriteButtons     map[string]*widget.Clickable
	homeMoreButtons         map[string]*widget.Clickable
	homeFolderButton        widget.Clickable
	homeNewFolderButton     widget.Clickable
	homeFolderAddButton     widget.Clickable
	homeSortButton          widget.Clickable
	homeFavorites           map[string]bool
	homeMetadata            map[string]homeFileMetadata
	homeMetadataResults     chan homeMetadataResult
	homeMetadataGeneration  uint64
	fileCommandResults      chan fileCommandResult
	fileCommandRunner       func(fileCommandKind, string) fileCommandResult
	gridLoadResults         chan gridLoadResult
	gridPictureMetaResults  chan gridPictureMetadataResult
	gridPictureResults      chan gridPicturePreviewResult
	gridPictureCommands     chan gridPictureCommandResult
	formatResults           chan spreadsheetFormatResult
	wordCommandResults      chan wordCommandResult
	wordImageResults        chan wordImageDecodeResult
	wordImagePickerResults  chan wordImagePickerResult
	wordImagePickerRunning  bool
	wordImageFilePicker     func() (string, error)
	xlsxImagePickerResults  chan spreadsheetImagePickerResult
	xlsxImagePickerRunning  bool
	xlsxImageFilePicker     func() (string, error)
	saveResults             chan saveResult
	openResults             chan openResult
	saving                  bool
	opening                 bool
	fileCommandRunning      bool
	documentGeneration      uint64
	shutdown                chan struct{}
	shutdownOnce            sync.Once

	wordDocumentInfoResults      chan wordDocumentInfoResult
	wordLayoutResults            chan wordLayoutResult
	wordLayoutEngine             wordLayoutEngine
	wordPDFExportResults         chan wordPDFExportResult
	wordPDFPickerRequest         *wordPDFPickerRequest
	spreadsheetPDFExportResults  chan spreadsheetPDFExportResult
	spreadsheetPDFPickerRequest  *spreadsheetPDFPickerRequest
	pdfExportPreview             pdfExportPreviewState
	spreadsheetPDFPreviewResults chan spreadsheetPDFPreviewResult
	wordPDFPreviewResults        chan wordPDFPreviewResult
	wordShareCopyResults         chan wordShareCopyResult
	wordShareCopyRequest         *wordShareCopyPickerRequest
	spreadsheetShareCopyResults  chan spreadsheetShareCopyResult
	spreadsheetShareCopyRequest  *spreadsheetShareCopyPickerRequest
	wordPDFFontSource            wordpdf.FontSource
	wordPrinter                  printservice.Printer
	wordPrintResults             chan wordPrintResult
	wordVersionHistoryResults    chan wordVersionHistoryResult
	wordVersionRestoreResults    chan wordVersionRestoreResult

	wordVersionHistoryVisible     bool
	wordVersionHistoryLoading     bool
	wordVersionHistoryError       string
	wordVersionHistoryPathKey     string
	wordVersionHistoryVersions    []workspace.WordVersion
	wordVersionHistorySelectedID  int64
	wordVersionHistoryList        widget.List
	wordVersionHistoryRowButtons  map[int64]*widget.Clickable
	wordVersionHistoryCloseButton widget.Clickable
	wordVersionHistoryScrim       widget.Clickable
	wordVersionHistoryRestore     widget.Clickable
}

func New(window *app.Window, initialPath string) *App {
	return newAppWithRuntimeAndWindowID(window, initialPath, newDesktopRuntime(), 0)
}

func newAppWithRuntime(window *app.Window, runtime *desktopRuntime) *App {
	return newAppWithRuntimeAndWindowID(window, "", runtime, 0)
}

func newAppWithRuntimeAndWindowID(window *app.Window, initialPath string, runtime *desktopRuntime, windowID uint64) *App {
	if runtime == nil {
		runtime = newDesktopRuntime()
	}
	a := &App{
		desktopRuntime:               runtime,
		window:                       window,
		theme:                        material.NewTheme(),
		documentSessionState:         newDocumentSessionState(initialPath),
		activeTabID:                  workspace.RecentTabID,
		status:                       "打开一个 .xlsx 或 .docx 文件开始预览",
		gridLoadResults:              make(chan gridLoadResult, 8),
		gridPictureMetaResults:       make(chan gridPictureMetadataResult, 8),
		gridPictureResults:           make(chan gridPicturePreviewResult, 64),
		gridPictureCommands:          make(chan gridPictureCommandResult, 8),
		formatResults:                make(chan spreadsheetFormatResult, 8),
		wordCommandResults:           make(chan wordCommandResult, 8),
		wordImageResults:             make(chan wordImageDecodeResult, 64),
		wordImagePickerResults:       make(chan wordImagePickerResult, 1),
		xlsxImagePickerResults:       make(chan spreadsheetImagePickerResult, 1),
		saveResults:                  make(chan saveResult, 1),
		openResults:                  make(chan openResult, 1),
		fileCommandResults:           make(chan fileCommandResult, 1),
		wordPDFExportResults:         make(chan wordPDFExportResult, 1),
		spreadsheetPDFExportResults:  make(chan spreadsheetPDFExportResult, 1),
		spreadsheetPDFPreviewResults: make(chan spreadsheetPDFPreviewResult, 1),
		wordPDFPreviewResults:        make(chan wordPDFPreviewResult, 1),
		wordShareCopyResults:         make(chan wordShareCopyResult, 1),
		spreadsheetShareCopyResults:  make(chan spreadsheetShareCopyResult, 1),
		wordPrinter:                  printservice.New(),
		wordPrintResults:             make(chan wordPrintResult, 1),
		wordVersionHistoryResults:    make(chan wordVersionHistoryResult, 1),
		wordVersionRestoreResults:    make(chan wordVersionRestoreResult, 1),
		wordVersionHistoryList:       widget.List{List: layout.List{Axis: layout.Vertical}},
		wordVersionHistoryRowButtons: make(map[int64]*widget.Clickable),
		shutdown:                     make(chan struct{}),
		sessions:                     make(map[string]*documentSessionState),
		tabMeta:                      make(map[string]workspace.Tab),
		tabButtons:                   make(map[string]*widget.Clickable),
		tabCloseButtons:              make(map[string]*widget.Clickable),
		homeSection:                  homeSectionRecent,
		homeTypeFilter:               "全部",
		homeSortOpenedDesc:           true,
		homeActionButtons:            make(map[string]*widget.Clickable),
		homeNavButtons:               make(map[string]*widget.Clickable),
		homeFilterButtons:            make(map[string]*widget.Clickable),
		homeOpenButtons:              make(map[string]*widget.Clickable),
		homeFavoriteButtons:          make(map[string]*widget.Clickable),
		homeMoreButtons:              make(map[string]*widget.Clickable),
		homeFavorites:                make(map[string]bool),
		homeMetadata:                 make(map[string]homeFileMetadata),
		homeMetadataResults:          make(chan homeMetadataResult, 8),
		workspaceTabsList:            widget.List{List: layout.List{Axis: layout.Horizontal}},
		createActionsList:            widget.List{List: layout.List{Axis: layout.Horizontal}},
		homeContentList:              widget.List{List: layout.List{Axis: layout.Vertical}},
		recentList:                   widget.List{List: layout.List{Axis: layout.Vertical}},

		wordDocumentInfoResults: make(chan wordDocumentInfoResult, 1),
		wordLayoutResults:       make(chan wordLayoutResult, 8),
		windowOpenResults:       make(chan error, 1),
	}

	if window != nil {
		a.attachWindow(windowID, a)
		a.initializeWorkspace()
		a.startWorkspaceWriter()
	}
	a.setActiveSessionID("")
	a.homeSearch.SingleLine = true
	if err := prepareWordFontScanner(); err != nil {
		log.Printf("prepare Word/system font scanner: %v", err)
	}
	a.theme.Shaper = text.NewShaper()
	a.theme.Face = preferredSystemTypeface
	a.feedback.initialized = true
	a.feedback.lastStatus = a.status
	return a
}

func Run(initialPath string) <-chan struct{} {
	runtime := newDesktopRuntime()
	runtime.completion = make(chan struct{})
	w := new(app.Window)
	configureDesktopWindow(w)
	a := newAppWithRuntime(w, runtime)
	if initialPath != "" {
		if err := a.open(initialPath); err != nil {
			a.setStatus(err.Error())
		}
	}
	go runtime.runAppWindow(a)
	return runtime.completion
}

func (a *App) open(path string) error {
	if a.isSaving() {
		return fmt.Errorf("正在保存当前文件，请稍后再打开")
	}
	runtime := a.ensureDesktopRuntime()
	shared, openedFromDisk, err := runtime.openDocumentSessionWithStatus(path)
	if err != nil {
		return err
	}
	var historyErr error
	if openedFromDisk {
		historyErr = runtime.recordOpenedWord(shared)
	}
	if err := a.installDocument(loadedDocumentFromSharedSession(shared)); err != nil {
		return err
	}
	if historyErr != nil && shared.word != nil {
		a.setStatus("文档已打开，但历史版本暂不可用：" + historyErr.Error())
	}
	return nil
}

type loadedDocument struct {
	shared      *sharedDocumentSession
	path        string
	book        document.Spreadsheet
	word        *docx.Document
	wordBlocks  []document.WordBlock
	jsonDoc     *jsondoc.Document
	markdownDoc *markdowndoc.Document
	pdfDoc      *pdfdoc.Document
	pptxDoc     *pptx.Document
	sheets      []string
}
type openResult struct {
	loaded     loadedDocument
	err        error
	historyErr error
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
	case ".pdf":
		d, err := pdfdoc.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		return loadedDocument{path: path, pdfDoc: d}, nil
	case ".docx":
		word, err := docx.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		return loadedDocument{path: path, word: word, wordBlocks: cloneWordBlockSnapshot(word.WordBlocks())}, nil
	case ".md", ".markdown":
		d, err := markdowndoc.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		return loadedDocument{path: path, markdownDoc: d}, nil
	case ".json":
		jsonDocument, err := jsondoc.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		return loadedDocument{path: path, jsonDoc: jsonDocument}, nil
	case ".pptx":
		d, err := pptx.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		return loadedDocument{path: path, pptxDoc: d}, nil
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
		return loadedDocument{}, fmt.Errorf("暂不支持 %s 文件，目前支持 .xlsx、.docx、.pptx、.pdf、.json 和 Markdown", filepath.Ext(path))
	}
}

func (a *App) installDocument(loaded loadedDocument) error {
	a.prepareMarkdownDeparture()
	runtime := a.ensureDesktopRuntime()
	a.reconcileWorkspaceTabs()
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
		if loaded.pdfDoc != nil {
			loaded.pdfDoc.SetDirty(false)
		}
		if loaded.markdownDoc != nil {
			loaded.markdownDoc.SetDirty(false)
		}
		if loaded.jsonDoc != nil {
			loaded.jsonDoc.SetDirty(false)
		}
		if loaded.pptxDoc != nil {
			loaded.pptxDoc.SetDirty(false)
		}
	}
	shared := loaded.shared
	if shared == nil {
		shared = runtime.registerSession(sharedSessionFromLoadedDocument(loaded))
	} else {
		shared = runtime.registerSession(shared)
	}
	loaded = loadedDocumentFromSharedSession(shared)
	for id, meta := range a.tabMeta {
		if meta.DocumentID == shared.documentID || sameDocumentPath(meta.SourcePath, shared.path) {
			state := a.viewForTab(id)
			if state == nil {
				state = newDocumentSessionView(shared, a.windowID)
				a.sessions[id] = state
			}
			a.activateSession(id, state)
			return nil
		}
	}
	if a.activeTabID != "" && a.documentSessionState != nil {
		a.sessions[a.activeTabID] = a.documentSessionState
	}
	state := newDocumentSessionView(shared, a.windowID)
	a.documentSessionState = state
	a.book, a.word, a.sheets = loaded.book, loaded.word, loaded.sheets
	a.pdfDoc = loaded.pdfDoc
	a.markdownDoc = loaded.markdownDoc
	a.jsonDoc = loaded.jsonDoc
	a.pptxDoc = loaded.pptxDoc
	a.documentSessionState.jsonDoc = loaded.jsonDoc
	a.documentSessionState.pptxDoc = loaded.pptxDoc
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
	id := shared.documentID
	if id == "" {
		id = workspaceSessionID(loaded.path)
	}
	kind := "document"
	if loaded.path == "" {
		kind = "draft"
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(loaded.path)), ".")
	if loaded.markdownDoc != nil {
		format = "markdown"
	}
	if format == "" {
		switch {
		case loaded.book != nil:
			format = "xlsx"
		case loaded.word != nil:
			format = "docx"
		case loaded.pptxDoc != nil:
			format = "pptx"
		case loaded.pdfDoc != nil:
			format = "pdf"
		case loaded.markdownDoc != nil:
			format = "markdown"
		case loaded.jsonDoc != nil:
			format = "json"
		}
	}
	title := filepath.Base(loaded.path)
	if loaded.path == "" {
		if loaded.pptxDoc != nil {
			title = "新建演示文稿.pptx"
		} else {
			title = "新建文档"
		}
	}
	meta := workspace.Tab{ID: id, Kind: kind, Title: title, DocumentID: id, SourcePath: loaded.path, Position: len(a.tabMeta)}
	a.activeTabID = id
	a.setActiveSessionID(id)
	state.documentID = id
	a.tabMeta[id] = meta
	a.sessions[id] = state
	if a.collection != nil {
		if format != "xls" {
			if _, exists := a.collection.Get(meta.ID); !exists {
				var err error
				if loaded.path == "" {
					var payload []byte
					payload, err = snapshotState(state)
					if err == nil {
						err = a.collection.AddDocument(workspace.Session{
							Tab: meta, Format: format, Payload: payload,
							Dirty: sessionDirty(state),
						}, runtime.isPrimaryWindow(a.windowID))
					}
				} else {
					if runtime.isPrimaryWindow(a.windowID) {
						err = a.collection.AddTab(meta, format)
					} else {
						err = a.collection.AddTabInactive(meta, format)
					}
				}
				if err != nil {
					a.setStatus("新建文档的恢复快照保存失败：" + err.Error())
					return err
				}
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
			a.setStatus(fmt.Sprintf("工作表「%s」：虚拟网格就绪，可浏览并编辑完整 XLSX 地址范围", a.sheet))
			return nil
		}
		a.sheet = loaded.sheets[0]
		a.cellButtons = make(map[string]*widget.Clickable)
		a.setStatus("旧版 .xls 只读预览（当前显示首张工作表）")
		return a.loadSheet()
	}
	a.blockButtons = make(map[int]*widget.Clickable)
	a.selectedBlock = -1
	if loaded.word != nil {
		a.setStatus(fmt.Sprintf("已打开 %s；读取 %d 个内容块", loaded.path, len(a.word.Blocks())))
		a.startWordPagination(state)
	} else if loaded.pptxDoc != nil {
		snap := loaded.pptxDoc.Snapshot()
		a.setStatus(fmt.Sprintf("已打开演示文稿；包含 %d 页幻灯片", snap.SlideCount))
	} else if loaded.jsonDoc != nil {
		a.setStatus("已打开 JSON 文本")
	}
	return nil
}

func closeLoadedDocument(loaded loadedDocument) {
	if loaded.shared != nil {
		return
	}
	if loaded.pdfDoc != nil {
		_ = loaded.pdfDoc.Close()
	}
	if loaded.book != nil {
		_ = loaded.book.Close()
	}
	if loaded.word != nil {
		_ = loaded.word.Close()
	}
	if loaded.markdownDoc != nil {
		_ = loaded.markdownDoc.Close()
	}
	if loaded.jsonDoc != nil {
		_ = loaded.jsonDoc.Close()
	}
	if loaded.pptxDoc != nil {
		_ = loaded.pptxDoc.Close()
	}
}

func (a *App) startOpen(path string) {
	if a.opening || a.isSaving() {
		return
	}
	a.opening = true
	a.setStatus("正在打开 " + strings.TrimSpace(path))
	go func() {
		runtime := a.ensureDesktopRuntime()
		shared, openedFromDisk, err := runtime.openDocumentSessionWithStatus(path)
		var historyErr error
		if err == nil && openedFromDisk {
			historyErr = runtime.recordOpenedWord(shared)
		}
		loaded := loadedDocumentFromSharedSession(shared)
		select {
		case <-a.shutdown:
			return
		default:
		}
		select {
		case a.openResults <- openResult{loaded: loaded, err: err, historyErr: historyErr}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}()
}

type wordSaveReason uint8

const (
	wordSaveManual wordSaveReason = iota
	wordSaveAuto
)

func (reason wordSaveReason) historySource() workspace.WordVersionSource {
	if reason == wordSaveAuto {
		return workspace.WordVersionAutoSave
	}
	return workspace.WordVersionManualSave
}

type saveResult struct {
	path       string
	tabID      string
	saveAs     bool
	token      sessionOperationToken
	err        error
	historyErr error
}

func (a *App) startSave() {
	a.startSaveForReason(wordSaveManual)
}

func (a *App) startSaveForReason(reason wordSaveReason) {
	if !a.prepareMarkdownSave() {
		return
	}
	if state := a.documentSessionState; a.word != nil && state != nil && (state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil) {
		state.wordSaveAfterDrafts = true
		state.wordSaveAfterDraftsReason = reason
		state.wordForceDraftFlush.Store(true)
		a.requestWordTextFlush(true)
		return
	}
	if a.word != nil && wordHasPendingTextDrafts(a.documentSessionState) {
		a.documentSessionState.wordSaveAfterDrafts = true
		a.documentSessionState.wordSaveAfterDraftsReason = reason
		a.documentSessionState.wordForceDraftFlush.Store(true)
		a.requestWordTextFlush(true)
		return
	}
	if a.isSaving() || !a.isDirty() {
		return
	}
	state := a.documentSessionState
	if state == nil {
		return
	}
	token, ok := state.beginOperation(sessionOperationSave)
	if !ok {
		return
	}
	book, word, pptxDocument, jsonDocument, markdownDocument, pdfDocument, tabID := a.book, a.word, a.pptxDoc, a.jsonDoc, a.markdownDoc, a.pdfDoc, a.activeTabID
	path := sessionPath(state)
	a.saving = true
	go func() {
		unlock := lockSessionAdapterWrite(state)
		var err error
		var historyErr error
		if book != nil {
			err = book.Save()
		} else if word != nil {
			var snapshot []byte
			if saver, ok := any(word).(document.WordPackageSnapshotSaver); ok {
				snapshot, err = saver.SaveWithSnapshot()
			} else {
				err = word.Save()
			}
			unlock()
			state.finishOperation(token)
			if err == nil && snapshot != nil {
				historyErr = a.ensureDesktopRuntime().addWordSnapshot(path, reason.historySource(), snapshot)
			}
			select {
			case a.saveResults <- saveResult{path: path, tabID: tabID, token: token, err: err, historyErr: historyErr}:
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
			}
			return
		} else if pptxDocument != nil {
			err = pptxDocument.Save()
		} else if pdfDocument != nil {
			err = pdfDocument.Save()
		} else if markdownDocument != nil {
			err = markdownDocument.Save()
		} else if jsonDocument != nil {
			err = jsonDocument.Save()
		}
		unlock()
		state.finishOperation(token)
		select {
		case a.saveResults <- saveResult{path: path, tabID: tabID, token: token, err: err, historyErr: historyErr}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}()
}

func (a *App) startSaveAs(path string) {
	if !a.prepareMarkdownSave() {
		return
	}
	if a.isSaving() {
		return
	}
	format := "xlsx"
	if a.word != nil {
		format = "docx"
	}
	if a.pptxDoc != nil {
		format = "pptx"
	}
	if a.markdownDoc != nil {
		format = "markdown"
	}
	if a.pdfDoc != nil {
		format = "pdf"
	}
	if a.jsonDoc != nil {
		format = "json"
	}
	if err := validateSaveAsExtension(format, path); err != nil {
		a.setStatus(err.Error())
		if a.pendingCloseTabID != "" {
			a.showCloseConfirmation(a.pendingCloseTabID, "保存失败", "文件扩展名不匹配。可以重试保存、不保存并关闭，或取消关闭。")
		}
		return
	}
	state := a.documentSessionState
	if state == nil {
		a.setStatus("没有活动文档")
		return
	}
	token, ok := state.beginOperation(sessionOperationSave)
	if !ok {
		return
	}
	book, word, pptxDocument, jsonDocument, markdownDocument, pdfDocument, tabID := a.book, a.word, a.pptxDoc, a.jsonDoc, a.markdownDoc, a.pdfDoc, a.activeTabID
	a.saving = true
	go func() {
		unlock := lockSessionAdapterWrite(state)
		var err error
		var historyErr error
		switch {
		case book != nil:
			if saver, ok := book.(interface{ SaveAsDocument(string) error }); ok {
				err = saver.SaveAsDocument(path)
			} else {
				err = fmt.Errorf("当前表格格式不支持另存为")
			}
		case word != nil:
			var snapshot []byte
			if saver, ok := any(word).(document.WordPackageSnapshotSaver); ok {
				snapshot, err = saver.SaveAsDocumentWithSnapshot(path)
			} else {
				err = word.SaveAsDocument(path)
			}
			unlock()
			if err == nil {
				a.ensureDesktopRuntime().updateSessionPath(state.sharedDocumentSession, path)
			}
			state.finishOperation(token)
			if err == nil && snapshot != nil {
				historyErr = a.ensureDesktopRuntime().addWordSnapshot(path, workspace.WordVersionManualSave, snapshot)
			}
			select {
			case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, token: token, err: err, historyErr: historyErr}:
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
			}
			return
		case pptxDocument != nil:
			err = pptxDocument.SaveAs(path)
			unlock()
			if err == nil {
				a.ensureDesktopRuntime().updateSessionPath(state.sharedDocumentSession, path)
			}
			state.finishOperation(token)
			select {
			case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, token: token, err: err}:
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
			}
			return
		case pdfDocument != nil:
			err = pdfDocument.SaveAs(path)
		case markdownDocument != nil:
			err = markdownDocument.SaveAs(path)
		case jsonDocument != nil:
			err = jsonDocument.SaveAs(path)
		default:
			err = fmt.Errorf("没有活动文档")
		}
		unlock()
		state.finishOperation(token)
		select {
		case a.saveResults <- saveResult{path: path, tabID: tabID, saveAs: true, token: token, err: err, historyErr: historyErr}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}()
}

func validateSaveAsExtension(format, path string) error {
	if format == "markdown" {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" {
			return nil
		}
		return fmt.Errorf("另存为扩展名必须是 .md 或 .markdown")
	}
	want := map[string]string{"xlsx": ".xlsx", "docx": ".docx", "json": ".json", "pptx": ".pptx", "pdf": ".pdf"}[format]
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
	unlock := lockSessionAdapterRead(a.documentSessionState)
	rows, err := a.book.Range(a.sheet, maxPreviewRows, maxPreviewColumns)
	unlock()
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
		a.setStatus(fmt.Sprintf("工作表「%s」为空（未读到单元格内容）", a.sheet))
	} else if len(rows) == maxPreviewRows {
		a.setStatus(fmt.Sprintf("工作表「%s」：已读入前 %d 行，%d 个非空单元格", a.sheet, len(rows), nonEmpty))
	} else {
		a.setStatus(fmt.Sprintf("工作表「%s」：已读入 %d 行，%d 个非空单元格", a.sheet, len(rows), nonEmpty))
	}
	return nil
}

func (a *App) applyCellEdit() error {
	if a.book == nil || a.selectedCell == "" {
		return fmt.Errorf("请先选择单元格")
	}
	state := a.documentSessionState
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	unlock := lockSessionAdapterWrite(state)
	err := a.book.SetCellText(a.sheet, a.selectedCell, a.cellEditor.Text())
	_, isGridReader := a.book.(document.GridReader)
	unlock()
	a.completeSessionOperation(state, token, err, err == nil)
	if err != nil {
		return err
	}
	if isGridReader {
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
	if a.isSaving() {
		return fmt.Errorf("正在保存，请稍后再编辑单元格")
	}
	if a.book == nil || a.cellEdit.Address == "" {
		return fmt.Errorf("请先选择单元格")
	}
	state := a.documentSessionState
	a.cellEdit.setDraft(value)
	changed := a.cellEdit.Draft != a.cellEdit.Original
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	err := a.cellEdit.commit(func(address, value string) error {
		unlock := lockSessionAdapterWrite(state)
		defer unlock()
		return a.book.SetCellText(a.sheet, address, value)
	})
	a.completeSessionOperation(state, token, err, err == nil && changed)
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
		a.setStatus(fmt.Sprintf("%s 已修改；点击保存写入原文件", a.selectedCell))
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
	state := a.documentSessionState
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("Word 文档正在处理其他操作")
	}
	unlock := lockSessionAdapterWrite(state)
	err := a.word.ReplaceParagraph(a.selectedBlock, a.wordEditor.Text())
	var blocks []document.WordBlock
	if err == nil {
		blocks = cloneWordBlockSnapshot(a.word.WordBlocks())
	}
	unlock()
	if err != nil {
		state.finishOperation(token)
		return err
	}
	state.setWordBlocks(blocks)
	a.ensureDesktopRuntime().publishSessionRevision(state.documentID, max(token.revision+1, state.wordRevisionAtomic.Load()))
	state.finishOperation(token)
	a.startWordPagination(state)
	a.persistActiveSession()
	return nil
}

func (a *App) save() error {
	if !a.prepareMarkdownSave() {
		return fmt.Errorf("请先完成输入法选字")
	}
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	defer unlock()
	switch {
	case a.book != nil:
		if err := a.book.Save(); err != nil {
			return err
		}
	case a.word != nil:
		if err := a.word.Save(); err != nil {
			return err
		}
	case a.pdfDoc != nil:
		return a.pdfDoc.Save()
	case a.markdownDoc != nil:
		return a.markdownDoc.Save()
	case a.jsonDoc != nil:
		if a.path == "" {
			return fmt.Errorf("JSON 草稿需要另存为")
		}
		if err := a.jsonDoc.Save(); err != nil {
			return err
		}
	case a.pptxDoc != nil:
		return a.pptxDoc.Save()
	default:
		return fmt.Errorf("没有打开的 Office 文件")
	}
	return nil
}

func (a *App) isDirty() bool {
	return markdownHasPendingDraft(a.documentSessionState) || sessionDirty(a.documentSessionState) || wordHasPendingTextDrafts(a.documentSessionState) || (a.documentSessionState != nil && (a.documentSessionState.wordParagraphSplitIntent != nil || a.documentSessionState.wordParagraphJoinIntent != nil))
}

func (a *App) closeDocument() error {
	a.prepareMarkdownDeparture()
	a.documentGeneration++
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	defer unlock()
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
	if a.pdfDoc != nil {
		err = a.pdfDoc.Close()
		a.pdfDoc = nil
	}
	if a.markdownDoc != nil {
		err = a.markdownDoc.Close()
		a.markdownDoc = nil
	}
	if a.jsonDoc != nil {
		if closeErr := a.jsonDoc.Close(); err == nil {
			err = closeErr
		}
		a.jsonDoc = nil
	}
	if a.pptxDoc != nil {
		if closeErr := a.pptxDoc.Close(); err == nil {
			err = closeErr
		}
		a.pptxDoc = nil
	}
	a.pptxView = nil
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
