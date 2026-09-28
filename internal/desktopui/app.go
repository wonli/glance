package desktopui

import (
	"fmt"
	"path/filepath"
	"strings"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
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

type App struct {
	window *app.Window
	ops    op.Ops
	theme  *material.Theme

	pathEditor   widget.Editor
	cellEditor   widget.Editor
	wordEditor   widget.Editor
	openButton   widget.Clickable
	chooseButton widget.Clickable
	saveButton   widget.Clickable
	applyButton  widget.Clickable

	path          string
	status        string
	book          document.Spreadsheet
	word          *docx.Document
	sheets        []string
	sheet         string
	grid          [][]document.Cell
	gridList      widget.List
	wordListState widget.List

	cellButtons           map[string]*widget.Clickable
	sheetButtons          map[string]*widget.Clickable
	blockButtons          map[int]*widget.Clickable
	selectedCell          string
	selectedBlock         int
	selectedBlockEditable bool
	virtualGrid           *gridViewport
	gridPointerTag        struct{}
}

func New(window *app.Window, initialPath string) *App {
	a := &App{
		window:        window,
		theme:         material.NewTheme(),
		path:          initialPath,
		status:        "打开一个 .xlsx 或 .docx 文件开始预览",
		gridList:      widget.List{List: layout.List{Axis: layout.Vertical}},
		wordListState: widget.List{List: layout.List{Axis: layout.Vertical}},
		cellButtons:   make(map[string]*widget.Clickable),
		sheetButtons:  make(map[string]*widget.Clickable),
		blockButtons:  make(map[int]*widget.Clickable),
		selectedBlock: -1,
	}
	a.pathEditor.SingleLine = true
	a.pathEditor.SetText(initialPath)
	a.cellEditor.SingleLine = true
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
	if err := a.closeDocument(); err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx":
		book, err := xlsx.Open(path)
		if err != nil {
			return err
		}
		a.book = book
		a.word = nil
		a.sheets = book.SheetNames()
		if len(a.sheets) == 0 {
			return fmt.Errorf("工作簿没有可显示的工作表")
		}
		a.sheet = a.sheets[0]
		a.path = path
		a.pathEditor.SetText(path)
		a.cellButtons = make(map[string]*widget.Clickable)
		a.virtualGrid = newGridViewport()
		a.status = fmt.Sprintf("工作表「%s」：虚拟网格就绪，可浏览并编辑完整 XLSX 地址范围", a.sheet)
		return nil
	case ".docx":
		word, err := docx.Open(path)
		if err != nil {
			return err
		}
		a.word = word
		a.book = nil
		a.path = path
		a.pathEditor.SetText(path)
		a.blockButtons = make(map[int]*widget.Clickable)
		a.selectedBlock = -1
		a.status = fmt.Sprintf("已打开 %s；读取 %d 个内容块", path, len(word.Blocks()))
		return nil
	case ".xls":
		book, err := legacyxls.Open(path)
		if err != nil {
			return err
		}
		a.book = book
		a.word = nil
		a.sheets = book.SheetNames()
		if len(a.sheets) == 0 {
			return fmt.Errorf("旧版工作簿没有可显示的工作表")
		}
		a.sheet = a.sheets[0]
		a.path = path
		a.pathEditor.SetText(path)
		a.cellButtons = make(map[string]*widget.Clickable)
		a.status = "旧版 .xls 只读预览（当前显示首张工作表）"
		return a.loadSheet()
	default:
		return fmt.Errorf("暂不支持 %s 文件，目前支持 .xlsx 和 .docx", filepath.Ext(path))
	}
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
