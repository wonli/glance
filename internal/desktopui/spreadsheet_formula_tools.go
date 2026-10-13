package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"strings"
)

func (a *App) runSpreadsheetFormulaCommand(operation, sheet, cell, formula string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetFormulaEditor)
	if !ok {
		return
	}
	generation, activeSheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.formula.set", "spreadsheet_cell", cell, sheet, map[string]string{
				"sheet": sheet, "cell": cell, "formula": formula,
			})
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			err = editor.SetCellFormula(sheet, cell, formula)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil {
			a.setStatus("已设置公式")
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: activeSheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) showSpreadsheetInsertFunctionDialog(category string) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	_, ok := state.book.(document.SpreadsheetFormulaEditor)
	if !ok {
		a.setStatus("当前工作簿不支持插入函数")
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := fmt.Sprintf("%s%d", columnName(activeCol), activeRow)

	defaultRef := "A1:A10"
	if state.selectionSet {
		b := state.selection.Bounds()
		if b.StartColumn == b.EndColumn && b.StartRow == b.EndRow {
			if activeRow > 1 {
				defaultRef = fmt.Sprintf("%s1:%s%d", columnName(activeCol), columnName(activeCol), activeRow-1)
			}
		} else {
			defaultRef = fmt.Sprintf("%s%d:%s%d", columnName(b.StartColumn), b.StartRow, columnName(b.EndColumn), b.EndRow)
		}
	}

	title := "插入函数"
	defaultFormula := fmt.Sprintf("=SUM(%s)", defaultRef)

	switch category {
	case "recent":
		title = "最近使用的函数"
		defaultFormula = fmt.Sprintf("=AVERAGE(%s)", defaultRef)
	case "logical":
		title = "逻辑函数"
		defaultFormula = fmt.Sprintf("=IF(%s1>0, 1, 0)", columnName(activeCol))
	case "text":
		title = "文本函数"
		defaultFormula = fmt.Sprintf("=CONCATENATE(%s1, \" \", %s2)", columnName(activeCol), columnName(activeCol))
	case "datetime":
		title = "日期时间函数"
		defaultFormula = "=TODAY()"
	}

	prompt := fmt.Sprintf("为单元格 %s 设置公式表达式：", cellName)
	a.modal.ShowInput(title, prompt, defaultFormula, "输入公式（例如 =SUM(A1:A10)）", func(action string) {
		formula := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || formula == "" {
			return
		}
		a.runSpreadsheetFormulaCommand("插入公式", state.sheet, cellName, formula)
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) runSpreadsheetDefinedNameCommand(name document.SpreadsheetDefinedName) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetDefinedNameEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.defined_names.set", "spreadsheet_workbook", token.documentID, sheet, name)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			err = editor.SetDefinedName(name)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err != nil {
			a.setStatus("创建定义名称失败：" + err.Error())
		} else {
			a.setStatus(fmt.Sprintf("已创建定义名称：“%s” -> %s", name.Name, name.Ref))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "名称管理器", applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) showSpreadsheetNameManagerDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	ed, ok := state.book.(document.SpreadsheetDefinedNameEditor)
	if !ok {
		a.setStatus("当前工作簿不支持名称管理器")
		return
	}
	existing, _ := ed.GetDefinedNames()
	b := state.selection.Bounds()
	defaultRef := fmt.Sprintf("'%s'!$%s$%d:$%s$%d", state.sheet, columnName(b.StartColumn), b.StartRow, columnName(b.EndColumn), b.EndRow)
	defaultName := fmt.Sprintf("区域%d", len(existing)+1)

	msg := fmt.Sprintf("当前已有 %d 个定义名称。\n请输入新名称并确认引用位置：", len(existing))
	a.modal.ShowInput("名称管理器", msg, defaultName, "名称", func(action string) {
		name := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || name == "" {
			return
		}
		a.runSpreadsheetDefinedNameCommand(document.SpreadsheetDefinedName{
			Name: name,
			Ref:  defaultRef,
		})
	}, modalDialogActionSpec{ID: "confirm", Label: "新建名称"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

type spreadsheetFlashFillApplyArguments struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

func (a *App) applySpreadsheetFlashFill() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetFlashFillEditor)
	if !ok {
		a.setStatus("当前工作簿不支持快速填充")
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	bounds := state.selection.Bounds()
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		var res document.SpreadsheetFlashFillResult
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.flash_fill.apply", "spreadsheet_cell_range", "selection", sheet, spreadsheetFlashFillApplyArguments{Sheet: sheet, Bounds: bounds})
			err = commandErr
			if err == nil {
				err = json.Unmarshal(result.Data, &res)
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			res, err = editor.FlashFill(sheet, bounds)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && res.RowsFilled > 0 && !commandRouted)
		if err == nil {
			a.setStatus(fmt.Sprintf("快速填充完成，已自动填充 %d 个单元格", res.RowsFilled))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "快速填充", applied: res.RowsFilled > 0}:
		case <-a.shutdown:
		}
	})
}
