package desktopui

import (
	"context"
	"errors"
	"fmt"
	"glance/internal/application"
	"glance/internal/document"
	csvdoc "glance/internal/format/csv"
	legacydoc "glance/internal/format/doc"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	legacyxls "glance/internal/format/xls"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
	"glance/internal/workspace"
	"path/filepath"
	"strings"

	"gioui.org/widget"
)

func (a *App) open(path string) error {
	if a.isSaving() {
		return fmt.Errorf("正在保存当前文件，请稍后再打开")
	}
	runtime := a.ensureDesktopRuntime()
	shared, openedFromDisk, err := runtime.openLiveDocument(path)
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
		a.setStatusDetailMessage(i18n.Message{ID: "word.history.warning.unavailable"}, feedbackWarning, notificationToast, historyErr.Error())
	}
	return nil
}

type loadedDocument struct {
	shared         *sharedDocumentSession
	path           string
	book           document.Spreadsheet
	word           *docx.Document
	wordBlocks     []document.WordBlock
	jsonDoc        *jsondoc.Document
	markdownDoc    *markdowndoc.Document
	pdfDoc         *pdfdoc.Document
	pptxDoc        *pptx.Document
	sheets         []string
	sourceFormat   application.Format
	saveAsRequired bool
}

type openResult struct {
	loaded     loadedDocument
	err        error
	historyErr error
}

func documentOpenFailureStatus(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	if strings.Contains(errStr, "cannot be preserved by the XLSX importer") {
		return "无法安全导入此 .xls：当前版本尚不能无损保留其中的内容；原文件未修改。"
	}
	if strings.Contains(errStr, "zip: not a valid zip file") || strings.Contains(errStr, "zip: not a valid zip archive") {
		return "文档格式校验失败：该文件不是有效的 Office 格式压缩包，可能已损坏、并非真实文档，或扩展名与实际格式不符。"
	}
	if strings.Contains(errStr, "no such file or directory") {
		return "未找到指定文件，该文件可能已被移动、重命名或删除。"
	}
	if strings.Contains(errStr, "permission denied") {
		return "无法访问该文件：没有足够的系统读取权限。"
	}
	return errStr
}

func documentOpenFailureMessage(err error) i18n.Message {
	if err == nil {
		return i18n.Message{ID: "common.feedback.operation_failed"}
	}
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "cannot be preserved by the XLSX importer"):
		return i18n.Message{ID: "document.open.failure.unsupported_xls"}
	case strings.Contains(errStr, "zip: not a valid zip file"), strings.Contains(errStr, "zip: not a valid zip archive"):
		return i18n.Message{ID: "document.open.failure.invalid_office"}
	case strings.Contains(errStr, "no such file or directory"):
		return i18n.Message{ID: "document.open.failure.missing"}
	case strings.Contains(errStr, "permission denied"):
		return i18n.Message{ID: "document.open.failure.permission"}
	default:
		return i18n.Message{ID: "common.feedback.operation_failed"}
	}
}

func (a *App) showDocumentOpenFailure(err error) {
	message := documentOpenFailureMessage(err)
	a.setStatusErrorMessage(message, err, notificationToast)
	a.modal.ShowConfirmMessage("document.open.failure", i18n.Message{ID: "document.open.failure.title"}, message, modalDialogWarningConfirmStyle, func(string) {
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}, Style: modalDialogActionPrimary})
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
		return loadedDocument{path: path, book: book, sheets: sheets, sourceFormat: application.FormatXLSX}, nil
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
	case ".doc":
		converted, err := legacydoc.ConvertToTempDOCX(context.Background(), path)
		if err != nil {
			return loadedDocument{}, err
		}
		word, openErr := docx.Open(converted.Path)
		cleanupErr := converted.Cleanup()
		if openErr != nil {
			return loadedDocument{}, errors.Join(openErr, cleanupErr)
		}
		if cleanupErr != nil {
			_ = word.Close()
			return loadedDocument{}, cleanupErr
		}
		// The converted package is only a staging file. The loaded DOCX adapter
		// owns the parsed package; edits must be saved to a new .docx destination.
		word.SetSourcePath("")
		return loadedDocument{path: path, word: word, wordBlocks: cloneWordBlockSnapshot(word.WordBlocks()), sourceFormat: application.FormatDOC, saveAsRequired: true}, nil
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
		converted, err := legacyxls.ConvertToTempXLSX(context.Background(), path)
		if err != nil {
			return loadedDocument{}, err
		}
		book, openErr := xlsx.Open(converted.Path)
		cleanupErr := converted.Cleanup()
		if openErr != nil {
			return loadedDocument{}, errors.Join(openErr, cleanupErr)
		}
		if cleanupErr != nil {
			_ = book.Close()
			return loadedDocument{}, cleanupErr
		}
		// The imported package is a staging file; saving must go through Save As.
		book.SetSourcePath("")
		sheets := book.SheetNames()
		if len(sheets) == 0 {
			_ = book.Close()
			return loadedDocument{}, fmt.Errorf("导入后的工作簿没有可显示的工作表")
		}
		return loadedDocument{path: path, book: book, sheets: sheets, sourceFormat: application.FormatXLS, saveAsRequired: true}, nil
	case ".csv":
		book, err := csvdoc.Open(path)
		if err != nil {
			return loadedDocument{}, err
		}
		sheets := book.SheetNames()
		if len(sheets) == 0 {
			_ = book.Close()
			return loadedDocument{}, fmt.Errorf("CSV 文件没有可显示的数据")
		}
		return loadedDocument{path: path, book: book, sheets: sheets, sourceFormat: application.FormatCSV}, nil
	default:
		return loadedDocument{}, fmt.Errorf("暂不支持 %s 文件，目前支持 .xlsx、.xls、.csv、.doc、.docx、.pptx、.pdf、.json 和 Markdown", filepath.Ext(path))
	}
}

func (a *App) installDocument(loaded loadedDocument) error {
	if loaded.book != nil {
		if _, ok := loaded.book.(document.GridReader); !ok {
			closeLoadedDocument(loaded)
			return fmt.Errorf("工作簿适配器未提供统一网格读取能力")
		}
	}
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
	previousActiveID := a.activeTabID
	if a.activeTabID != "" && a.documentSessionState != nil {
		if a.activeTabID != workspace.RecentTabID && a.activeTabID != shared.documentID {
			a.persistActiveSession()
		}
		a.sessions[a.activeTabID] = a.documentSessionState
		if a.documentSessionState.jsonLarge.Load() {
			a.documentSessionState.releaseJSONViewResources()
		}
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
		state.sheet = a.sheet
		state.spreadsheetZoom = state.spreadsheetSheetZoom(a.sheet)
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
	if loaded.sourceFormat == application.FormatDOC {
		// The workspace snapshot is the normalized Word package; the source path
		// remains the original .doc and is reopened through the import adapter.
		format = "docx"
	}
	if loaded.markdownDoc != nil {
		format = "markdown"
	}
	recentFormat := format
	if loaded.sourceFormat == application.FormatDOC {
		recentFormat = "doc"
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
		title = a.textID("workspace.document.untitled")
		if loaded.pptxDoc != nil {
			title = a.textID("workspace.document.untitled_presentation")
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
					a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
					return err
				}
			}
			a.reindexTabMetadata()
			if loaded.path != "" {
				a.collection.StageRecents([]workspace.Recent{{Path: loaded.path, Kind: recentFormat}})
				a.refreshHomeMetadata()
				a.persistRecentsAsync()
			}
			a.persistActiveSession()
		}
	}
	a.evictInactiveSessionsExcept(id, previousActiveID)
	a.releaseIdleEngines()
	if loaded.book != nil {
		a.sheet = loaded.sheets[0]
		a.cellButtons = make(map[string]*widget.Clickable)
		a.virtualGrid = newGridViewportForSpreadsheet(loaded.book, a.sheet)
		a.selectedCell = "A1"
		state.selection = CellSelection{}
		state.selectionSet = false
		a.virtualGrid.SelectedRow, a.virtualGrid.SelectedColumn = 1, 1
		a.virtualGrid.PendingSelectionLoad = true
		a.cellEdit.selectCell("A1", "")
		a.nameBox.SetText("")
		a.formulaEditor.SetText("")
		if loaded.sourceFormat == application.FormatXLS {
			a.setStatusMessage(i18n.Message{ID: "document.open.legacy_xls", Args: i18n.Args{"Sheet": a.sheet}}, feedbackInfo, notificationSuppressed)
		} else if loaded.sourceFormat == application.FormatCSV {
			a.setStatusMessage(i18n.Message{ID: "document.open.csv_grid", Args: i18n.Args{"Sheet": a.sheet}}, feedbackInfo, notificationSuppressed)
		} else {
			a.setStatusMessage(i18n.Message{ID: "document.open.xlsx_grid", Args: i18n.Args{"Sheet": a.sheet}}, feedbackInfo, notificationSuppressed)
		}
		return nil
	}
	a.blockButtons = make(map[int]*widget.Clickable)
	a.selectedBlock = -1
	if loaded.word != nil {
		if loaded.sourceFormat == application.FormatDOC {
			a.setStatusMessage(i18n.Message{ID: "document.open.legacy_doc", Args: i18n.Args{"Count": len(a.word.Blocks())}}, feedbackInfo, notificationSuppressed)
		} else {
			a.setStatusMessage(i18n.Message{ID: "document.open.word", Args: i18n.Args{"Path": loaded.path, "Count": len(a.word.Blocks())}}, feedbackInfo, notificationSuppressed)
		}
		a.startWordPagination(state)
	} else if loaded.pptxDoc != nil {
		snap := loaded.pptxDoc.Snapshot()
		a.setStatusMessage(i18n.Message{ID: "document.open.presentation", Args: i18n.Args{"Count": snap.SlideCount}}, feedbackInfo, notificationSuppressed)
	} else if loaded.jsonDoc != nil {
		a.setStatusMessage(i18n.Message{ID: "document.open.json"}, feedbackInfo, notificationSuppressed)
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
	a.setStatusMessage(i18n.Message{ID: "document.opening", Args: i18n.Args{"Path": strings.TrimSpace(path)}}, feedbackInfo, notificationSuppressed)
	a.launchManagedBackground(func() {
		runtime := a.ensureDesktopRuntime()
		shared, openedFromDisk, err := runtime.openLiveDocument(path)
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
	})
}
