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
	legacyxls "glance/internal/format/xls"
	"glance/internal/format/xlsx"
)

const (
	maxPreviewRows    = 300
	maxPreviewColumns = 16
)

const preferredSystemTypeface = font.Typeface("PingFang SC, Hiragino Sans GB, Microsoft YaHei, Noto Sans CJK SC, WenQuanYi Micro Hei, sans-serif")

type App struct {
	window *app.Window
	ops    op.Ops
	theme  *material.Theme

	pathEditor     widget.Editor
	cellEditor     widget.Editor
	formulaEditor  widget.Editor
	nameBox        widget.Editor
	wordEditor     widget.Editor
	openButton     widget.Clickable
	chooseButton   widget.Clickable
	saveButton     widget.Clickable
	applyButton    widget.Clickable
	goToCellButton widget.Clickable

	path          string
	status        string
	book          document.Spreadsheet
	word          *docx.Document
	sheets        []string
	sheet         string
	grid          [][]document.Cell
	gridList      widget.List
	wordListState widget.List

	cellButtons                   map[string]*widget.Clickable
	sheetButtons                  map[string]*widget.Clickable
	blockButtons                  map[int]*widget.Clickable
	selectedCell                  string
	selectedBlock                 int
	selectedBlockEditable         bool
	cellEdit                      cellEditState
	lastGridWidth, lastGridHeight float64
	virtualGrid                   *gridViewport
	gridLoadResults               chan gridLoadResult
	saveResults                   chan saveResult
	openResults                   chan openResult
	saving                        bool
	opening                       bool
	documentGeneration            uint64
	shutdown                      chan struct{}
	shutdownOnce                  sync.Once
}

func New(window *app.Window, initialPath string) *App {
	a := &App{
		window:          window,
		theme:           material.NewTheme(),
		path:            initialPath,
		status:          "打开一个 .xlsx 或 .docx 文件开始预览",
		gridList:        widget.List{List: layout.List{Axis: layout.Vertical}},
		wordListState:   widget.List{List: layout.List{Axis: layout.Vertical}},
		cellButtons:     make(map[string]*widget.Clickable),
		sheetButtons:    make(map[string]*widget.Clickable),
		blockButtons:    make(map[int]*widget.Clickable),
		selectedBlock:   -1,
		gridLoadResults: make(chan gridLoadResult, 8),
		saveResults:     make(chan saveResult, 1),
		openResults:     make(chan openResult, 1),
		shutdown:        make(chan struct{}),
	}
	a.theme.Shaper = text.NewShaper()
	a.theme.Face = preferredSystemTypeface
	a.pathEditor.SingleLine = true
	a.pathEditor.SetText(initialPath)
	a.cellEditor.SingleLine = true
	a.cellEditor.Submit = true
	a.formulaEditor.SingleLine = true
	a.formulaEditor.Submit = true
	a.nameBox.SingleLine = true
	a.nameBox.Submit = true
	a.wordEditor.SingleLine = true
	return a
}

func Run(initialPath string) {
	w := new(app.Window)
	w.Option(
		app.Title("Glance Office"),
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
			_ = a.closeDocument()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&a.ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *App) open(path string) error {
	if a.isDirty() {
		return fmt.Errorf("当前文件有未保存的修改，请先保存再打开其他文件")
	}
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
	path   string
	book   document.Spreadsheet
	word   *docx.Document
	sheets []string
}
type openResult struct {
	loaded loadedDocument
	err    error
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
		return loadedDocument{path: path, word: word}, nil
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
		return loadedDocument{}, fmt.Errorf("暂不支持 %s 文件，目前支持 .xlsx 和 .docx", filepath.Ext(path))
	}
}

func (a *App) installDocument(loaded loadedDocument) error {
	if err := a.closeDocument(); err != nil {
		if loaded.book != nil {
			_ = loaded.book.Close()
		}
		if loaded.word != nil {
			_ = loaded.word.Close()
		}
		return err
	}
	a.book, a.word, a.sheets = loaded.book, loaded.word, loaded.sheets
	a.path = loaded.path
	a.pathEditor.SetText(loaded.path)
	if loaded.book != nil {
		a.sheet = loaded.sheets[0]
		a.cellButtons = make(map[string]*widget.Clickable)
		if _, ok := loaded.book.(document.GridReader); ok {
			a.virtualGrid = newGridViewport()
			a.selectedCell = "A1"
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
	a.status = fmt.Sprintf("已打开 %s；读取 %d 个内容块", loaded.path, len(a.word.Blocks()))
	return nil
}

func (a *App) startOpen(path string) {
	if a.opening || a.saving {
		return
	}
	if a.isDirty() {
		a.status = "当前文件有未保存的修改，请先保存再打开其他文件"
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
	path string
	err  error
}

func (a *App) startSave() {
	if a.saving || !a.isDirty() {
		return
	}
	book, word, path := a.book, a.word, a.path
	a.saving = true
	go func() {
		var err error
		if book != nil {
			err = book.Save()
		} else if word != nil {
			err = word.Save()
		}
		select {
		case a.saveResults <- saveResult{path: path, err: err}:
			a.window.Invalidate()
		case <-a.shutdown:
		}
	}()
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
	return a.loadSheet()
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
	return a.word.ReplaceParagraph(a.selectedBlock, a.wordEditor.Text())
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
	default:
		return fmt.Errorf("没有打开的 Office 文件")
	}
	return nil
}

func (a *App) isDirty() bool {
	return (a.book != nil && a.book.Dirty()) || (a.word != nil && a.word.Dirty())
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
