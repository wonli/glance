package desktopui

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	wordpdf "glance/internal/format/pdf"
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
	applicationRequestSeq   atomic.Uint64
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
	createMenuScrim         widget.Clickable
	createMenuCardClick     widget.Clickable
	modal                   modalDialog
	agentUI                 agentUIState
	mcpCopyConfigButton     widget.Clickable
	mcpConnectionConfig     string
	mcpConnectionError      string
	mcpConfigCopied         bool
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
	wordEditorReviewResults chan wordEditorReviewAsyncResult
	wordEditorReviewRequest uint64
	wordCompareResults      chan wordCompareAsyncResult
	wordCompareRequest      uint64
	wordImageResults        chan wordImageDecodeResult
	wordImagePickerResults  chan wordImagePickerResult
	wordImagePickerRunning  bool
	wordImageFilePicker     func() (string, error)
	pptxPicPickerResults    chan presentationPicturePickerResult
	pptxPicCommandResults   chan presentationPictureCommandResult
	pptxPicPickerRunning    bool
	pptxPicFilePicker       func() (string, error)
	pptxMediaPickerResults  chan presentationMediaPickerResult
	pptxMediaPickerRunning  bool
	pptxMediaFilePicker     func(string) (string, error)
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
	managedBackgroundMu     sync.Mutex
	managedBackgroundClosed bool
	managedBackgroundTasks  sync.WaitGroup

	wordDocumentInfoResults       chan wordDocumentInfoResult
	wordMacrosInspectionResults   chan wordMacrosInspectionResult
	wordMacrosInspectionRequest   uint64
	wordLayoutResults             chan wordLayoutResult
	wordLayoutEngine              wordLayoutEngine
	wordPDFExportResults          chan wordPDFExportResult
	wordPDFPickerRequest          *wordPDFPickerRequest
	spreadsheetPDFExportResults   chan spreadsheetPDFExportResult
	spreadsheetPrintResults       chan spreadsheetPrintResult
	spreadsheetPDFPickerRequest   *spreadsheetPDFPickerRequest
	pdfExportPreview              pdfExportPreviewState
	spreadsheetPDFPreviewResults  chan spreadsheetPDFPreviewResult
	wordPDFPreviewResults         chan wordPDFPreviewResult
	presentationPDFPreviewResults chan presentationPDFPreviewResult
	presentationPDFExportResults  chan presentationPDFExportResult
	presentationPDFPickerRequest  *presentationPDFPickerRequest
	presentationPDFPreviewGen     uint64
	wordShareCopyResults          chan wordShareCopyResult
	wordShareCopyRequest          *wordShareCopyPickerRequest
	spreadsheetShareCopyResults   chan spreadsheetShareCopyResult
	spreadsheetShareCopyRequest   *spreadsheetShareCopyPickerRequest
	wordPDFFontSource             wordpdf.FontSource
	nativePrinter                 printservice.Printer
	wordPrintResults              chan wordPrintResult
	wordVersionHistoryResults     chan wordVersionHistoryResult
	wordVersionRestoreResults     chan wordVersionRestoreResult
	viewIntentRequests            chan desktopViewIntentRequest

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
	wordCompareDocumentList       widget.List
	wordCompareDocumentButtons    map[string]*widget.Clickable
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
	printer := runtime.nativePrinter
	if printer == nil {
		printer = printservice.New()
	}
	a := &App{
		desktopRuntime:                runtime,
		window:                        window,
		theme:                         material.NewTheme(),
		documentSessionState:          newDocumentSessionState(initialPath),
		activeTabID:                   workspace.RecentTabID,
		status:                        "打开一个 .xlsx 或 .docx 文件开始预览",
		gridLoadResults:               make(chan gridLoadResult, 8),
		gridPictureMetaResults:        make(chan gridPictureMetadataResult, 8),
		gridPictureResults:            make(chan gridPicturePreviewResult, 64),
		gridPictureCommands:           make(chan gridPictureCommandResult, 8),
		formatResults:                 make(chan spreadsheetFormatResult, 8),
		wordCommandResults:            make(chan wordCommandResult, 8),
		wordEditorReviewResults:       make(chan wordEditorReviewAsyncResult, 1),
		wordCompareResults:            make(chan wordCompareAsyncResult, 1),
		wordImageResults:              make(chan wordImageDecodeResult, 64),
		wordImagePickerResults:        make(chan wordImagePickerResult, 1),
		pptxPicPickerResults:          make(chan presentationPicturePickerResult, 1),
		pptxPicCommandResults:         make(chan presentationPictureCommandResult, 1),
		pptxMediaPickerResults:        make(chan presentationMediaPickerResult, 1),
		xlsxImagePickerResults:        make(chan spreadsheetImagePickerResult, 1),
		saveResults:                   make(chan saveResult, 1),
		openResults:                   make(chan openResult, 1),
		fileCommandResults:            make(chan fileCommandResult, 1),
		wordPDFExportResults:          make(chan wordPDFExportResult, 1),
		spreadsheetPDFExportResults:   make(chan spreadsheetPDFExportResult, 1),
		spreadsheetPrintResults:       make(chan spreadsheetPrintResult, 1),
		spreadsheetPDFPreviewResults:  make(chan spreadsheetPDFPreviewResult, 1),
		wordPDFPreviewResults:         make(chan wordPDFPreviewResult, 1),
		presentationPDFPreviewResults: make(chan presentationPDFPreviewResult, 1),
		presentationPDFExportResults:  make(chan presentationPDFExportResult, 1),
		wordShareCopyResults:          make(chan wordShareCopyResult, 1),
		spreadsheetShareCopyResults:   make(chan spreadsheetShareCopyResult, 1),
		nativePrinter:                 printer,
		wordPrintResults:              make(chan wordPrintResult, 1),
		wordVersionHistoryResults:     make(chan wordVersionHistoryResult, 1),
		wordVersionRestoreResults:     make(chan wordVersionRestoreResult, 1),
		viewIntentRequests:            make(chan desktopViewIntentRequest, 16),
		wordVersionHistoryList:        widget.List{List: layout.List{Axis: layout.Vertical}},
		wordVersionHistoryRowButtons:  make(map[int64]*widget.Clickable),
		wordCompareDocumentList:       widget.List{List: layout.List{Axis: layout.Vertical}},
		wordCompareDocumentButtons:    make(map[string]*widget.Clickable),
		shutdown:                      make(chan struct{}),
		agentUI:                       newAgentUIState(),
		sessions:                      make(map[string]*documentSessionState),
		tabMeta:                       make(map[string]workspace.Tab),
		tabButtons:                    make(map[string]*widget.Clickable),
		tabCloseButtons:               make(map[string]*widget.Clickable),
		homeSection:                   homeSectionRecent,
		homeTypeFilter:                "全部",
		homeSortOpenedDesc:            true,
		homeActionButtons:             make(map[string]*widget.Clickable),
		homeNavButtons:                make(map[string]*widget.Clickable),
		homeFilterButtons:             make(map[string]*widget.Clickable),
		homeOpenButtons:               make(map[string]*widget.Clickable),
		homeFavoriteButtons:           make(map[string]*widget.Clickable),
		homeMoreButtons:               make(map[string]*widget.Clickable),
		homeFavorites:                 make(map[string]bool),
		homeMetadata:                  make(map[string]homeFileMetadata),
		homeMetadataResults:           make(chan homeMetadataResult, 8),
		workspaceTabsList:             widget.List{List: layout.List{Axis: layout.Horizontal}},
		createActionsList:             widget.List{List: layout.List{Axis: layout.Horizontal}},
		homeContentList:               widget.List{List: layout.List{Axis: layout.Vertical}},
		recentList:                    widget.List{List: layout.List{Axis: layout.Vertical}},

		wordDocumentInfoResults:     make(chan wordDocumentInfoResult, 1),
		wordMacrosInspectionResults: make(chan wordMacrosInspectionResult, 1),
		wordLayoutResults:           make(chan wordLayoutResult, 8),
		windowOpenResults:           make(chan error, 1),
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
	if err := runtime.startDesktopMCPEndpoint(); err != nil {
		log.Printf("start desktop MCP endpoint: %v", err)
		a.setStatus("MCP 服务启动失败：" + err.Error())
	}
	restoreCtx, cancelRestore := context.WithCancel(context.Background())
	if a.launchManagedBackground(func() {
		select {
		case <-a.shutdown:
			cancelRestore()
		case <-restoreCtx.Done():
		}
	}) {
		if !a.launchManagedBackground(func() {
			defer cancelRestore()
			if err := runtime.restoreAgentHost(restoreCtx); err != nil {
				log.Printf("restore configured Agent: %v", err)
			}
		}) {
			cancelRestore()
		}
	} else {
		cancelRestore()
	}
	go runtime.runAppWindow(a)
	return runtime.completion
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
