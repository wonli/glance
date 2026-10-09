package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strconv"
	"strings"
)

func (a *App) runSpreadsheetPageLayoutCommand(operation string, edit func(document.SpreadsheetPageLayoutEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetOrientationDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	currentOrient := "portrait"
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil && layout.Orientation != "" {
		currentOrient = layout.Orientation
	}
	label := "纵向"
	if currentOrient == "landscape" {
		label = "横向"
	}
	message := fmt.Sprintf("当前工作表打印方向：%s\n请选择目标方向：", label)
	sheet := state.sheet
	a.modal.Show("纸张方向", message, func(action string) {
		a.modal.Hide()
		if action == "portrait" || action == "landscape" {
			a.runSpreadsheetPageLayoutCommand("修改纸张方向", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPageOrientation(sheet, action)
			})
		}
	},
		modalDialogActionSpec{ID: "portrait", Label: "纵向"},
		modalDialogActionSpec{ID: "landscape", Label: "横向"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetMarginsDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	top, right, bottom, left := 0.75, 0.7, 0.75, 0.7
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil {
		if layout.MarginTop > 0 {
			top = layout.MarginTop
		}
		if layout.MarginRight > 0 {
			right = layout.MarginRight
		}
		if layout.MarginBottom > 0 {
			bottom = layout.MarginBottom
		}
		if layout.MarginLeft > 0 {
			left = layout.MarginLeft
		}
	}
	labels := []string{"上边距（英寸）", "右边距（英寸）", "下边距（英寸）", "左边距（英寸）"}
	hints := []string{"0.75", "0.7", "0.75", "0.7"}
	initial := []string{
		strconv.FormatFloat(top, 'f', -1, 64),
		strconv.FormatFloat(right, 'f', -1, 64),
		strconv.FormatFloat(bottom, 'f', -1, 64),
		strconv.FormatFloat(left, 'f', -1, 64),
	}
	sheet := state.sheet
	a.modal.ShowForm("页边距设置", "可选择快速预设或输入自定义页边距（英寸，1 英寸 = 2.54 厘米）：",
		labels, hints, initial,
		func(action string) {
			switch action {
			case "normal":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置普通页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 0.75, 0.7, 0.75, 0.7)
				})
			case "wide":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置宽页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 1.0, 1.0, 1.0, 1.0)
				})
			case "narrow":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置窄页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 0.75, 0.25, 0.75, 0.25)
				})
			case "confirm":
				tVal, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				rVal, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				bVal, err3 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[2].Text()), 64)
				lVal, err4 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[3].Text()), 64)
				if err1 != nil || err2 != nil || err3 != nil || err4 != nil ||
					tVal < 0 || rVal < 0 || bVal < 0 || lVal < 0 ||
					tVal > 20 || rVal > 20 || bVal > 20 || lVal > 20 {
					a.setStatus("页边距必须是 0 到 20 英寸之间的数值")
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置页边距", func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, tVal, rVal, bVal, lVal)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "normal", Label: "普通 (0.75/0.7\")"},
		modalDialogActionSpec{ID: "narrow", Label: "窄 (0.75/0.25\")"},
		modalDialogActionSpec{ID: "wide", Label: "宽 (1.0\")"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetPageSizeDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	currentSize := 9
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil && layout.PaperSize > 0 {
		currentSize = layout.PaperSize
	}
	sizeName := "A4 (210×297mm)"
	if currentSize == 1 {
		sizeName = "Letter (8.5×11in)"
	} else if currentSize == 8 {
		sizeName = "A3 (297×420mm)"
	}
	message := fmt.Sprintf("当前工作表纸张大小：%s\n请选择常用纸张规格：", sizeName)
	sheet := state.sheet
	a.modal.Show("纸张大小设置", message, func(action string) {
		a.modal.Hide()
		var paperSize int
		switch action {
		case "a4":
			paperSize = 9
		case "letter":
			paperSize = 1
		case "a3":
			paperSize = 8
		default:
			return
		}
		a.runSpreadsheetPageLayoutCommand("设置纸张大小", func(ed document.SpreadsheetPageLayoutEditor) error {
			return ed.SetSheetPageSize(sheet, paperSize)
		})
	},
		modalDialogActionSpec{ID: "a4", Label: "A4 (210×297mm)"},
		modalDialogActionSpec{ID: "letter", Label: "Letter (8.5×11in)"},
		modalDialogActionSpec{ID: "a3", Label: "A3 (297×420mm)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetPrintAreaDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	layout, _ := editor.GetSheetPageLayout(sheet)
	var selRef string
	if state.selectionSet {
		bounds := state.selection.Bounds()
		selRef = fmt.Sprintf("$%s$%d:$%s$%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	}

	var message string
	if layout.PrintArea != "" {
		message = fmt.Sprintf("当前打印区域：%s\n\n您可将当前选区设为新的打印区域，或清除现有打印区域。", layout.PrintArea)
	} else {
		message = "当前未设置打印区域。\n\n您可将当前选区设为打印区域。"
	}

	actions := make([]modalDialogActionSpec, 0, 3)
	if selRef != "" {
		actions = append(actions, modalDialogActionSpec{ID: "set", Label: "设为打印区域 (" + selRef + ")"})
	}
	if layout.PrintArea != "" {
		actions = append(actions, modalDialogActionSpec{ID: "clear", Label: "清除打印区域"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	a.modal.Show("打印区域设置", message, func(action string) {
		a.modal.Hide()
		switch action {
		case "set":
			if selRef == "" {
				return
			}
			a.runSpreadsheetPageLayoutCommand("设置打印区域", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintArea(sheet, selRef)
			})
			a.setStatus("已设置打印区域：" + selRef)
		case "clear":
			a.runSpreadsheetPageLayoutCommand("清除打印区域", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintArea(sheet, "")
			})
			a.setStatus("已清除打印区域")
		}
	}, actions...)
}

func (a *App) showSpreadsheetPrintTitlesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	layout, _ := editor.GetSheetPageLayout(sheet)
	var selRowsRef string
	if state.selectionSet {
		bounds := state.selection.Bounds()
		selRowsRef = fmt.Sprintf("$%d:$%d", bounds.StartRow, bounds.EndRow)
	}

	var message string
	if layout.PrintTitles != "" {
		message = fmt.Sprintf("当前打印标题行：%s\n\n您可将当前选中行设为顶端标题行，或清除现有打印标题。", layout.PrintTitles)
	} else {
		message = "当前未设置打印标题行。\n\n在打印多页表格时，打印标题行会在每一页顶端重复打印。\n您可将当前选中行设为顶端标题行。"
	}

	actions := make([]modalDialogActionSpec, 0, 3)
	if selRowsRef != "" {
		actions = append(actions, modalDialogActionSpec{ID: "set", Label: "设为标题行 (" + selRowsRef + ")"})
	}
	if layout.PrintTitles != "" {
		actions = append(actions, modalDialogActionSpec{ID: "clear", Label: "清除标题行"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	a.modal.Show("打印标题设置", message, func(action string) {
		a.modal.Hide()
		switch action {
		case "set":
			if selRowsRef == "" {
				return
			}
			a.runSpreadsheetPageLayoutCommand("设置打印标题行", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintTitles(sheet, selRowsRef)
			})
			a.setStatus("已设置打印标题行：" + selRowsRef)
		case "clear":
			a.runSpreadsheetPageLayoutCommand("清除打印标题行", func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintTitles(sheet, "")
			})
			a.setStatus("已清除打印标题行")
		}
	}, actions...)
}
