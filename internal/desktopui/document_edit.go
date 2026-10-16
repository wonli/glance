package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"math"
	"strings"
)

func (a *App) applyCellEdit() error {
	if a.book == nil || a.selectedCell == "" {
		return fmt.Errorf("请先选择单元格")
	}
	if _, ok := a.book.(document.GridReader); !ok {
		return fmt.Errorf("当前工作簿未提供统一网格读取能力")
	}
	state := a.documentSessionState
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	var changed bool
	var err error
	commandRouted := a.applicationSessionRegistered(token.documentID)
	if commandRouted {
		result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.cell.set", "spreadsheet.cell", a.selectedCell, a.sheet, map[string]string{
			"sheet": a.sheet, "cell": a.selectedCell, "value": a.cellEditor.Text(),
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
		err = a.book.SetCellText(a.sheet, a.selectedCell, a.cellEditor.Text())
		unlock()
		changed = err == nil
	}
	a.completeSessionOperation(state, token, err, err == nil && !commandRouted)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if a.virtualGrid != nil {
		a.virtualGrid.invalidateWindow()
	}
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
	formulaTargetSheet := a.sheet
	formulaReference := state.formulaReference
	if formulaReference.Active && formulaReference.TargetCell == a.cellEdit.Address {
		formulaTargetSheet = formulaReference.TargetSheet
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	var commandChanged bool
	var commandRouted bool
	err := a.cellEdit.commit(func(address, value string) error {
		if strings.HasPrefix(strings.TrimSpace(value), "=") {
			editor, ok := a.book.(document.SpreadsheetFormulaEditor)
			if !ok {
				return fmt.Errorf("当前工作簿不支持公式编辑")
			}
			if a.applicationSessionRegistered(token.documentID) {
				commandRouted = true
				result, err := a.executeRibbonApplicationCommand(token, "spreadsheet.formula.set", "spreadsheet.cell", address, formulaTargetSheet, map[string]string{
					"sheet": formulaTargetSheet, "cell": address, "formula": value,
				})
				if err != nil {
					return err
				}
				var outcome struct {
					Changed bool `json:"changed"`
				}
				if err := json.Unmarshal(result.Data, &outcome); err != nil {
					return err
				}
				commandChanged = outcome.Changed
				return nil
			}
			unlock := lockSessionAdapterWrite(state)
			defer unlock()
			return editor.SetCellFormula(formulaTargetSheet, address, value)
		}
		if !a.applicationSessionRegistered(token.documentID) {
			unlock := lockSessionAdapterWrite(state)
			defer unlock()
			return a.book.SetCellText(formulaTargetSheet, address, value)
		}
		commandRouted = true
		result, err := a.executeRibbonApplicationCommand(token, "spreadsheet.cell.set", "spreadsheet.cell", address, formulaTargetSheet, map[string]string{
			"sheet": formulaTargetSheet, "cell": address, "value": value,
		})
		if err != nil {
			return err
		}
		var outcome struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &outcome); err != nil {
			return err
		}
		commandChanged = outcome.Changed
		return nil
	})
	a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
	if err != nil {
		return err
	}
	a.selectedCell = a.cellEdit.Address
	if formulaReference.Active {
		state.formulaReference = formulaReference
		a.restoreFormulaReferenceTarget()
	}
	changed = changed && (!commandRouted || commandChanged)
	if changed && a.virtualGrid != nil {
		a.virtualGrid.invalidateWindow()
	}
	a.cellEditor.SetText(a.cellEdit.Original)
	a.formulaEditor.SetText(a.cellEdit.Original)
	if changed {
		a.setStatus(fmt.Sprintf("%s 已修改；点击保存写入原文件", a.selectedCell))
	}
	rowHeightChanged, rowHeightErr := a.persistCellEditRowHeightPreview(changed)
	if rowHeightErr != nil {
		a.setStatus("单元格已修改，但自动调整行高失败：" + rowHeightErr.Error())
	} else if rowHeightChanged {
		a.setStatus(fmt.Sprintf("%s 已修改并调整行高；点击保存写入原文件", a.selectedCell))
	}
	if changed || rowHeightChanged {
		a.persistActiveSession()
	}
	return nil
}

func (a *App) persistCellEditRowHeightPreview(cellChanged bool) (bool, error) {
	if a.virtualGrid == nil || a.virtualGrid.CellEditRowPreview == nil {
		return false, nil
	}
	preview := a.virtualGrid.CellEditRowPreview
	if !cellChanged || math.Abs(preview.PreviewPx-preview.OriginalPx) <= 0.5 {
		a.virtualGrid.finishCellEditRowPreview(true)
		return false, nil
	}
	setRowHeight := func() error {
		if editor, ok := a.book.(document.SpreadsheetDimensionEditGrouper); ok {
			return editor.SetRowHeightAsPartOfLatestEdit(preview.Sheet, preview.Row+1, devicePixelsToPoints(preview.PreviewPx, a.virtualGrid.Scale))
		}
		reader, ok := a.book.(interface {
			SetRowHeight(string, int, float64) error
		})
		if !ok {
			return fmt.Errorf("当前工作簿不支持自动调整行高")
		}
		return reader.SetRowHeight(preview.Sheet, preview.Row+1, devicePixelsToPoints(preview.PreviewPx, a.virtualGrid.Scale))
	}
	if a.book == nil {
		a.virtualGrid.finishCellEditRowPreview(false)
		return false, fmt.Errorf("当前工作簿不支持自动调整行高")
	}
	state := a.documentSessionState
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.virtualGrid.finishCellEditRowPreview(false)
		return false, fmt.Errorf("表格正在处理其他操作")
	}
	unlock := lockSessionAdapterWrite(state)
	err := setRowHeight()
	unlock()
	a.completeSessionOperation(state, token, err, err == nil)
	if err != nil {
		a.virtualGrid.finishCellEditRowPreview(false)
		return false, err
	}
	key := gridAutoFitRow{Sheet: preview.Sheet, Row: preview.Row}
	if preview.PreviewPx <= preview.BasePx+0.5 {
		delete(state.spreadsheetAutoFitRowBasePoints, key)
	} else {
		if state.spreadsheetAutoFitRowBasePoints == nil {
			state.spreadsheetAutoFitRowBasePoints = make(map[gridAutoFitRow]float64)
		}
		state.spreadsheetAutoFitRowBasePoints[key] = devicePixelsToPoints(preview.BasePx, a.virtualGrid.Scale)
	}
	a.virtualGrid.finishCellEditRowPreview(false)
	return true, nil
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

func (a *App) isDirty() bool {
	return markdownHasPendingDraft(a.documentSessionState) || presentationTextDraftDirty(a.pptxView) || sessionDirty(a.documentSessionState) || wordHasPendingTextDrafts(a.documentSessionState) || (a.documentSessionState != nil && (a.documentSessionState.wordParagraphSplitIntent != nil || a.documentSessionState.wordParagraphJoinIntent != nil))
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
	a.sheets, a.sheet = nil, ""
	a.virtualGrid = nil
	shared := a.cellEdit.session
	a.cellEdit.cancel()
	a.cellEdit = cellEditState{session: shared}
	a.selectedCell = ""
	a.nameBox.SetText("")
	a.formulaEditor.SetText("")
	a.path = ""
	return err
}
