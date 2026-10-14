package desktopui

import (
	"context"
	"errors"
	"fmt"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	legacyxls "glance/internal/format/xls"
	"glance/internal/format/xlsx"
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
		a.setStatus("文档已打开，但历史版本暂不可用：" + historyErr.Error())
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
	if strings.Contains(err.Error(), "cannot be preserved by the XLSX importer") {
		return "无法安全导入此 .xls：当前版本尚不能无损保留其中的内容；原文件未修改。"
	}
	return err.Error()
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
	default:
		return loadedDocument{}, fmt.Errorf("暂不支持 %s 文件，目前支持 .xlsx、.docx、.pptx、.pdf、.json 和 Markdown", filepath.Ext(path))
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
		a.virtualGrid = newGridViewport()
		a.selectedCell = "A1"
		state.selection = CellSelection{}
		state.selectionSet = true
		a.virtualGrid.SelectedRow, a.virtualGrid.SelectedColumn = 1, 1
		a.virtualGrid.PendingSelectionLoad = true
		a.cellEdit.selectCell("A1", "")
		a.nameBox.SetText("A1")
		a.formulaEditor.SetText("")
		if loaded.sourceFormat == application.FormatXLS {
			a.setStatus(fmt.Sprintf("已导入旧版 .xls 工作表「%s」：可浏览和编辑；请另存为 .xlsx", a.sheet))
		} else {
			a.setStatus(fmt.Sprintf("工作表「%s」：虚拟网格就绪，可浏览并编辑完整 XLSX 地址范围", a.sheet))
		}
		return nil
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
