package desktopui

import (
	"context"
	"gioui.org/unit"
	"image"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/fontfallback"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/i18n"
	"glance/internal/printservice"
	"glance/internal/workspace"
)

var preferredSystemTypeface = font.Typeface(fontfallback.PreferredTypeface())

type App struct {
	wordPointer         wordPointerState
	nativePinch         *nativePinchState
	nativePinchMonitor  nativePinchMonitor
	pinchGestureSession *documentSessionState
	pinchFrameSession   *documentSessionState
	pinchFrameSize      image.Point
	pinchFrameMetric    unit.Metric
	pinchRemainder      float64
	fontPicker          fontPickerState
	*documentSessionState
	*desktopRuntime
	windowID             uint64
	window               *app.Window
	renderedFrame        bool
	trayStartupScheduled bool
	trayStartupResults   chan error
	ops                  op.Ops
	theme                *material.Theme
	themeMode            ThemeMode
	designTokens         DesignTokens

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

	feedback                      feedbackState
	statusRef                     i18n.Message
	initialStatus                 i18n.Message
	activeSessionDocumentID       atomic.Value
	applicationRequestSeq         atomic.Uint64
	sessions                      map[string]*documentSessionState
	tabMeta                       map[string]workspace.Tab
	lastReconciledTabVersion      uint64
	activeTabID                   string
	workspaceError                string
	workspaceWriteErrorRef        i18n.Message
	tabButtons                    map[string]*widget.Clickable
	tabCloseButtons               map[string]*widget.Clickable
	newButton                     widget.Clickable
	recentTabButton               widget.Clickable
	workspaceTabsList             widget.List
	workspaceTabsListPendingReset bool
	workspaceTabsRevealedActiveID string
	workspaceTabsRevealedIndex    int
	workspaceTabScrollTag         int
	workspaceTabScrollRemainder   float32
	createActionsList             widget.List
	windowCloseButton             widget.Clickable
	windowOpenResults             chan error
	windowOpenSource              *App
	workspacePDFRestoreResults    chan pdfWorkspaceRestoreResult
	workspacePDFRestoreGeneration uint64
	pendingPDFRestoreTabID        string
	pendingPDFRestoreSaveOnReady  bool
	primaryPromotionPending       atomic.Bool
	windowMinimizeButton          widget.Clickable
	windowMaximizeButton          widget.Clickable
	windowMode                    app.WindowMode
	clientTitlebar                bool
	windowDecorations             widget.Decorations
	showNewMenu                   bool
	createMenuScrim               widget.Clickable
	createMenuCardClick           widget.Clickable
	modal                         modalDialog
	agentUI                       agentUIState
	mcpCopyConfigButton           widget.Clickable
	mcpConnectionViewConfig       string
	mcpConnectionConfig           string
	mcpConnectionError            string
	mcpTempExecutable             bool
	mcpConfigurationList          widget.List
	mcpConfigCopied               bool
	wordSymbolPickerVisible       bool
	wordSymbolPickerScrim         widget.Clickable
	wordSymbolPickerClose         widget.Clickable
	wordSymbolButtons             [len(wordSymbolChoices)]widget.Clickable
	wordSymbolRunID               string
	wordSymbolParagraphID         string
	wordSymbolStart               int
	wordSymbolEnd                 int
	wordSymbolRevision            uint64
	wordSymbolDraftEpoch          uint64
	pendingCloseTabID             string
	recentList                    widget.List
	homeSection                   homeSection
	settingsReturnSection         homeSection
	homeTypeFilter                string
	homeSearch                    widget.Editor
	homeSortOpenedDesc            bool
	homeContentList               widget.List
	homeActionButtons             map[string]*widget.Clickable
	homeNavButtons                map[string]*widget.Clickable
	homeFilterButtons             map[string]*widget.Clickable
	homeOpenButtons               map[string]*widget.Clickable
	homeFavoriteButtons           map[string]*widget.Clickable
	homeMoreButtons               map[string]*widget.Clickable
	homeRecentMenuPath            string
	homeRecentOpenPath            string
	homeRecentMenuIndex           int
	homeRecentMenuDismiss         widget.Clickable
	homeRecentMenuBounds          image.Rectangle
	homeRecentMenuAnchorBounds    image.Rectangle
	homeRecentMenuItems           [homeRecentMenuItemCount]widget.Clickable
	homeRecentUndo                *homeRecentUndoRecord
	homeRecentWriteGeneration     uint64
	homeRecentWriteResults        chan homeRecentWriteResult
	homeFolderButton              widget.Clickable
	homeNewFolderButton           widget.Clickable
	homeFolderAddButton           widget.Clickable
	homeFolderTreeList            widget.List
	homeFolderRows                map[string]*homeFolderRowWidgets
	homeFolderExpanded            map[string]bool
	homeSelectedFolderID          string
	homeFolderMenuFolderID        string
	homeFolderMenuIndex           int
	homeFolderMenuDismiss         widget.Clickable
	homeFolderBlank               widget.Clickable
	homeFolderMenuBounds          image.Rectangle
	homeFolderMenuAnchorBounds    image.Rectangle
	homeFolderTreeRootOrigin      image.Point
	homeFolderTreeViewport        image.Rectangle
	homeFolderDropTargets         []homeFolderDropTarget
	homeActiveDropFolderID        string
	homeContentRootOrigin         image.Point
	homeRecentTableRootOrigin     image.Point
	homeRecentListRootOrigin      image.Point
	homeDocumentDrags             map[string]*homeDocumentDragState
	homeActiveDragPath            string
	homeFolderPageID              string
	homeFolderPageSize            int
	homeContentViewportHeight     int
	homeSortButton                widget.Clickable
	homeFavorites                 map[string]bool
	homeMetadata                  map[string]homeFileMetadata
	aiHistoryDrawerOpen           bool
	aiHistoryDrawerOverlay        widget.Clickable
	aiHistoryDrawerCloseBtn       widget.Clickable
	aiDrawerOpenBtn               widget.Clickable
	aiSuggestionButtons           [3]widget.Clickable
	aiAttachButton                widget.Clickable
	aiModelSelectButton           widget.Clickable
	aiClearHistoryButton          widget.Clickable
	aiChatList                    widget.List
	homeMetadataResults           chan homeMetadataResult
	homeMetadataGeneration        uint64
	homeFolderContents            map[string][]workspace.Recent
	homeFolderContentsResults     chan homeFolderContentsResult
	homeFolderContentsGeneration  uint64
	homeFolderContentsLatest      map[string]uint64
	homeFolderContentsLoading     map[string]bool
	homeFolderContentsLastScan    map[string]time.Time
	homeCanonicalPaths            map[string]string
	fileCommandResults            chan fileCommandResult
	fileCommandRunner             func(fileCommandKind, string) fileCommandResult
	homeFolderWriteResults        chan homeFolderWriteResult
	gridLoadResults               chan gridLoadResult
	gridNavigationResults         chan spreadsheetGridNavigationResult
	gridPictureMetaResults        chan gridPictureMetadataResult
	gridPictureResults            chan gridPicturePreviewResult
	gridPictureCommands           chan gridPictureCommandResult
	formatResults                 chan spreadsheetFormatResult
	wordCommandResults            chan wordCommandResult
	wordEditorReviewResults       chan wordEditorReviewAsyncResult
	wordEditorReviewRequest       uint64
	wordCompareResults            chan wordCompareAsyncResult
	wordCompareRequest            uint64
	wordImageResults              chan wordImageDecodeResult
	wordImagePickerResults        chan wordImagePickerResult
	wordImagePickerRunning        bool
	wordImageFilePicker           func() (string, error)
	pptxPicPickerResults          chan presentationPicturePickerResult
	pptxPicCommandResults         chan presentationPictureCommandResult
	pptxPicPickerRunning          bool
	pptxPicFilePicker             func() (string, error)
	pptxMediaPickerResults        chan presentationMediaPickerResult
	pptxMediaPickerRunning        bool
	pptxMediaFilePicker           func(string) (string, error)
	xlsxImagePickerResults        chan spreadsheetImagePickerResult
	xlsxImagePickerRunning        bool
	xlsxImageFilePicker           func() (string, error)
	saveResults                   chan saveResult
	openResults                   chan openResult
	saving                        bool
	opening                       bool
	fileCommandRunning            bool
	documentGeneration            uint64
	shutdown                      chan struct{}
	shutdownOnce                  sync.Once
	managedBackgroundMu           sync.Mutex
	managedBackgroundClosed       bool
	managedBackgroundTasks        sync.WaitGroup

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
	wordVersionHistoryErrorRef    i18n.Message
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
		windowID:                      windowID,
		window:                        window,
		theme:                         material.NewTheme(),
		documentSessionState:          newDocumentSessionState(initialPath),
		activeTabID:                   workspace.RecentTabID,
		initialStatus:                 i18n.Message{ID: "status.initial.open_document"},
		gridLoadResults:               make(chan gridLoadResult, 8),
		gridNavigationResults:         make(chan spreadsheetGridNavigationResult, 16),
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
		workspacePDFRestoreResults:    make(chan pdfWorkspaceRestoreResult, 8),
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
		mcpConfigurationList:          widget.List{List: layout.List{Axis: layout.Vertical}},
		shutdown:                      make(chan struct{}),
		agentUI:                       newAgentUIState(),
		sessions:                      make(map[string]*documentSessionState),
		tabMeta:                       make(map[string]workspace.Tab),
		tabButtons:                    make(map[string]*widget.Clickable),
		tabCloseButtons:               make(map[string]*widget.Clickable),
		homeSection:                   homeSectionRecent,
		homeTypeFilter:                homeFilterAll,
		homeSortOpenedDesc:            true,
		homeActionButtons:             make(map[string]*widget.Clickable),
		homeNavButtons:                make(map[string]*widget.Clickable),
		homeFilterButtons:             make(map[string]*widget.Clickable),
		homeOpenButtons:               make(map[string]*widget.Clickable),
		homeFavoriteButtons:           make(map[string]*widget.Clickable),
		homeMoreButtons:               make(map[string]*widget.Clickable),
		homeRecentWriteResults:        make(chan homeRecentWriteResult, 1),
		homeFolderRows:                make(map[string]*homeFolderRowWidgets),
		homeFolderExpanded:            make(map[string]bool),
		homeDocumentDrags:             make(map[string]*homeDocumentDragState),
		homeFavorites:                 make(map[string]bool),
		homeMetadata:                  make(map[string]homeFileMetadata),
		homeMetadataResults:           make(chan homeMetadataResult, 8),
		homeFolderContents:            make(map[string][]workspace.Recent),
		homeFolderContentsResults:     make(chan homeFolderContentsResult, 8),
		homeFolderContentsLatest:      make(map[string]uint64),
		homeFolderContentsLoading:     make(map[string]bool),
		homeFolderContentsLastScan:    make(map[string]time.Time),
		homeFolderWriteResults:        make(chan homeFolderWriteResult, 1),
		workspaceTabsList:             widget.List{List: layout.List{Axis: layout.Horizontal}},
		createActionsList:             widget.List{List: layout.List{Axis: layout.Horizontal}},
		aiChatList:                    widget.List{List: layout.List{Axis: layout.Vertical}},
		homeContentList:               widget.List{List: layout.List{Axis: layout.Vertical}},
		recentList:                    widget.List{List: layout.List{Axis: layout.Vertical}},
		homeFolderTreeList:            widget.List{List: layout.List{Axis: layout.Vertical}},

		wordDocumentInfoResults:     make(chan wordDocumentInfoResult, 1),
		wordMacrosInspectionResults: make(chan wordMacrosInspectionResult, 1),
		wordLayoutResults:           make(chan wordLayoutResult, 8),
		windowOpenResults:           make(chan error, 1),
		trayStartupResults:          make(chan error, 1),
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
	if window != nil {
		a.startFontService()
	}
	a.theme.Face = preferredSystemTypeface
	a.themeMode = ThemeModeSystem
	a.designTokens = ResolveDesignTokens(a.themeMode, DefaultTypography)
	return a
}

func (a *App) tokens() DesignTokens {
	if a == nil || a.designTokens.Typography.Body == 0 {
		return defaultShellTokens
	}
	return a.designTokens
}

func (a *App) setDesignThemeMode(mode ThemeMode) {
	if a == nil {
		return
	}
	a.themeMode = mode
	a.designTokens = ResolveDesignTokens(mode, a.designTokens.Typography)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func Run(initialPath string) <-chan struct{} {
	return RunWithStartupError(initialPath, nil)
}

func RunWithStartupError(initialPath string, reportStartupError func(error)) <-chan struct{} {
	return RunWithActivation(initialPath, reportStartupError, nil)
}

// RunWithActivation starts the desktop UI and applies path-free activation
// requests from the process-level single-instance coordinator.
func RunWithActivation(initialPath string, reportStartupError func(error), activationRequests <-chan struct{}) <-chan struct{} {
	runtime := newDesktopRuntime()
	runtime.completion = make(chan struct{})
	runtime.startupErrorReporter = reportStartupError
	w := new(app.Window)
	configureDesktopWindow(w)
	a := newAppWithRuntime(w, runtime)
	if activationRequests != nil {
		go runtime.consumeActivationRequests(a, activationRequests)
	}
	if initialPath != "" {
		if err := a.open(initialPath); err != nil {
			a.showDocumentOpenFailure(err)
		}
	}
	if err := runtime.startDesktopMCPIPC(); err != nil {
		log.Printf("start desktop MCP IPC endpoint: %v", err)
		a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
	}
	if !trayStartupDeferredUntilWindowFrame() {
		if err := runtime.startSystemTray(); err != nil {
			log.Printf("start system tray: %v", err)
		}
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
			_, settings := runtime.currentAgentHost()
			if settings.ProviderID != "" || settings.Model != "" {
				a.agentUI.settings = settings
				if a.window != nil {
					a.window.Invalidate()
				}
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
