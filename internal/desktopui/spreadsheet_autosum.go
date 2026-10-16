package desktopui

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func (a *App) applySpreadsheetAutoSum() {
	a.applySpreadsheetAggregate("sum")
}

func (a *App) applySpreadsheetAggregate(operation string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.formatRunning.Load() {
		return
	}
	name := spreadsheetAggregateName(operation)
	reader, hasReader := state.book.(document.SpreadsheetRangeReader)
	_, hasEditor := state.book.(document.SpreadsheetFormulaBatchEditor)
	if !hasReader || !hasEditor {
		a.setStatus("当前表格不支持此项统计")
		return
	}
	bounds := state.selection.Bounds()
	edits := computeSpreadsheetAggregateFormulaEdits(reader, state.sheet, bounds, operation)
	if len(edits) == 0 {
		a.setStatus("未在选区中找到可计算" + name + "的连续数值")
		return
	}
	if err := a.applySpreadsheetFormulaBatch(edits, name); err != nil {
		a.setStatus(name + "失败：" + err.Error())
		return
	}
	a.navigateToCell(edits[0].Row, edits[0].Column)
	a.setStatus("正在插入" + name + "公式")
}

func spreadsheetAggregateName(operation string) string {
	switch operation {
	case "average":
		return "平均值"
	case "count":
		return "计数"
	case "max":
		return "最大值"
	case "min":
		return "最小值"
	default:
		return "求和"
	}
}

func computeAutoSumFormulaEdits(reader document.SpreadsheetRangeReader, sheet string, bounds document.CellRange) []document.CellFormulaEdit {
	return computeSpreadsheetAggregateFormulaEdits(reader, sheet, bounds, "sum")
}

func computeSpreadsheetAggregateFormulaEdits(reader document.SpreadsheetRangeReader, sheet string, bounds document.CellRange, operation string) []document.CellFormulaEdit {
	function, supported := spreadsheetAggregateFunction(operation)
	if !supported {
		return nil
	}
	// Case 1: Single cell selected
	if bounds.StartRow == bounds.EndRow && bounds.StartColumn == bounds.EndColumn {
		row, col := bounds.StartRow, bounds.StartColumn
		if row > 1 {
			scanStartRow := maxInt(1, row-100)
			scanBounds := document.CellRange{StartRow: scanStartRow, StartColumn: col, EndRow: row - 1, EndColumn: col}
			cells, err := reader.ReadRange(sheet, scanBounds)
			if err == nil && len(cells) > 0 {
				runStartRow := row
				found := false
				for r := len(cells) - 1; r >= 0; r-- {
					if len(cells[r]) == 0 {
						break
					}
					_, ok := parseNumericValue(cells[r][0].Value)
					if !ok {
						break
					}
					found = true
					runStartRow = scanStartRow + r
				}
				if found {
					formula := spreadsheetAggregateFormula(function, col, runStartRow, col, row-1)
					return []document.CellFormulaEdit{{Row: row, Column: col, Formula: formula}}
				}
			}
		}
		if col > 1 {
			scanStartCol := maxInt(1, col-50)
			scanBounds := document.CellRange{StartRow: row, StartColumn: scanStartCol, EndRow: row, EndColumn: col - 1}
			cells, err := reader.ReadRange(sheet, scanBounds)
			if err == nil && len(cells) > 0 && len(cells[0]) > 0 {
				runStartColumn := col
				found := false
				for c := len(cells[0]) - 1; c >= 0; c-- {
					_, ok := parseNumericValue(cells[0][c].Value)
					if !ok {
						break
					}
					found = true
					runStartColumn = scanStartCol + c
				}
				if found {
					formula := spreadsheetAggregateFormula(function, runStartColumn, row, col-1, row)
					return []document.CellFormulaEdit{{Row: row, Column: col, Formula: formula}}
				}
			}
		}
		return nil
	}

	// Case 2: Multi-cell selection
	cells, err := reader.ReadRange(sheet, bounds)
	if err != nil || len(cells) == 0 || len(cells[0]) == 0 {
		return nil
	}

	// Subcase 2A: Multi-row selection
	if bounds.EndRow > bounds.StartRow {
		bottomRowEmpty := true
		lastRowIdx := len(cells) - 1
		for c := 0; c < len(cells[lastRowIdx]); c++ {
			if strings.TrimSpace(cells[lastRowIdx][c].Value) != "" {
				bottomRowEmpty = false
				break
			}
		}
		dataEndRowIdx := lastRowIdx
		targetRow := bounds.EndRow + 1
		if bottomRowEmpty {
			dataEndRowIdx = lastRowIdx - 1
			targetRow = bounds.EndRow
		}
		if dataEndRowIdx < 0 {
			return nil
		}

		var edits []document.CellFormulaEdit
		cols := bounds.EndColumn - bounds.StartColumn + 1
		for c := 0; c < cols; c++ {
			found := false
			for r := 0; r <= dataEndRowIdx; r++ {
				if c < len(cells[r]) {
					if _, ok := parseNumericValue(cells[r][c].Value); ok {
						found = true
						break
					}
				}
			}
			if found {
				column := bounds.StartColumn + c
				edits = append(edits, document.CellFormulaEdit{
					Row: targetRow, Column: column,
					Formula: spreadsheetAggregateFormula(function, column, bounds.StartRow, column, bounds.StartRow+dataEndRowIdx),
				})
			}
		}
		return edits
	}

	// Subcase 2B: Single row, multi-column selection
	lastColEmpty := strings.TrimSpace(cells[0][len(cells[0])-1].Value) == ""
	dataEndColIdx := len(cells[0]) - 1
	targetCol := bounds.EndColumn + 1
	if lastColEmpty {
		dataEndColIdx = len(cells[0]) - 2
		targetCol = bounds.EndColumn
	}
	if dataEndColIdx < 0 || len(cells) == 0 || len(cells[0]) == 0 {
		return nil
	}
	found := false
	for c := 0; c <= dataEndColIdx; c++ {
		if c < len(cells[0]) {
			if _, ok := parseNumericValue(cells[0][c].Value); ok {
				found = true
				break
			}
		}
	}
	if found {
		return []document.CellFormulaEdit{{
			Row: bounds.StartRow, Column: targetCol,
			Formula: spreadsheetAggregateFormula(function, bounds.StartColumn, bounds.StartRow, bounds.StartColumn+dataEndColIdx, bounds.StartRow),
		}}
	}
	return nil
}

func spreadsheetAggregateFunction(operation string) (string, bool) {
	switch operation {
	case "sum":
		return "SUM", true
	case "average":
		return "AVERAGE", true
	case "count":
		return "COUNT", true
	case "max":
		return "MAX", true
	case "min":
		return "MIN", true
	default:
		return "", false
	}
}

func spreadsheetAggregateFormula(function string, startColumn, startRow, endColumn, endRow int) string {
	start := xlsx.CellName(startColumn, startRow)
	end := xlsx.CellName(endColumn, endRow)
	return fmt.Sprintf("=%s(%s:%s)", function, start, end)
}

func (a *App) applySpreadsheetFormulaBatch(edits []document.CellFormulaEdit, operation string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	formulaEditor, ok := state.book.(document.SpreadsheetFormulaBatchEditor)
	if !ok {
		return fmt.Errorf("当前表格不支持批量公式编辑")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	completedDetails := spreadsheetAggregateFormulaDetails(operation, edits)
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := len(edits) > 0
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.formulas.set", "spreadsheet_cells", "formula-batch", sheet, struct {
				Sheet string                     `json:"sheet"`
				Edits []document.CellFormulaEdit `json:"edits"`
			}{sheet, edits})
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
			changed, err = formulaEditor.SetCellFormulasChanged(sheet, edits)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil && !changed {
			completedDetails = "目标单元格中已存在相同公式，未作更改"
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed, noOp: err == nil && !changed, details: completedDetails}:
		case <-a.shutdown:
		}
	})
	return nil
}

func spreadsheetAggregateFormulaDetails(operation string, edits []document.CellFormulaEdit) string {
	if len(edits) == 1 {
		return fmt.Sprintf("已插入%s公式：%s", operation, xlsx.CellName(edits[0].Column, edits[0].Row))
	}
	names := make([]string, 0, minInt(len(edits), 4))
	for _, edit := range edits[:minInt(len(edits), 4)] {
		names = append(names, xlsx.CellName(edit.Column, edit.Row))
	}
	suffix := ""
	if len(edits) > len(names) {
		suffix = "等"
	}
	return fmt.Sprintf("已插入 %d 个%s公式：%s%s", len(edits), operation, strings.Join(names, "、"), suffix)
}

func parseNumericValue(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	accountingNegative := strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")
	if accountingNegative {
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "("), ")"))
	}
	for _, symbol := range []string{"¥", "￥", "$", "€", "£", "₹"} {
		s = strings.ReplaceAll(s, symbol, "")
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.TrimSpace(s)
	accountingNegative = accountingNegative || (strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"))
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
			if accountingNegative {
				v = -math.Abs(v)
			}
			return v / 100.0, true
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	if accountingNegative {
		v = -math.Abs(v)
	}
	return v, true
}
