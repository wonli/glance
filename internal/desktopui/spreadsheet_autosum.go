package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"math"
	"strconv"
	"strings"
)

func (a *App) applySpreadsheetAutoSum() {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.formatRunning.Load() {
		return
	}
	reader, hasReader := state.book.(document.SpreadsheetRangeReader)
	_, hasEditor := state.book.(document.SpreadsheetBatchEditor)
	if !hasReader || !hasEditor {
		a.setStatus("当前表格不支持自动求和")
		return
	}
	bounds := state.selection.Bounds()
	edits := computeAutoSumEdits(reader, state.sheet, bounds)
	if len(edits) == 0 {
		a.setStatus("未在选区中找到可求和的连续数值")
		return
	}
	if err := a.applyBatchEdits(edits, "求和"); err != nil {
		a.setStatus("求和失败：" + err.Error())
		return
	}
	a.navigateToCell(edits[0].Row, edits[0].Column)
	if len(edits) == 1 {
		a.setStatus(fmt.Sprintf("求和完成：%s = %s", xlsx.CellName(edits[0].Column, edits[0].Row), edits[0].Value))
	} else {
		a.setStatus(fmt.Sprintf("求和完成，已填入 %d 个单元格", len(edits)))
	}
}

func computeAutoSumEdits(reader document.SpreadsheetRangeReader, sheet string, bounds document.CellRange) []document.CellEdit {
	// Case 1: Single cell selected
	if bounds.StartRow == bounds.EndRow && bounds.StartColumn == bounds.EndColumn {
		row, col := bounds.StartRow, bounds.StartColumn
		if row > 1 {
			scanStartRow := maxInt(1, row-100)
			scanBounds := document.CellRange{StartRow: scanStartRow, StartColumn: col, EndRow: row - 1, EndColumn: col}
			cells, err := reader.ReadRange(sheet, scanBounds)
			if err == nil && len(cells) > 0 {
				var sum float64
				var count int
				for r := len(cells) - 1; r >= 0; r-- {
					v, ok := parseNumericValue(cells[r][0].Value)
					if !ok {
						break
					}
					sum += v
					count++
				}
				if count > 0 {
					return []document.CellEdit{{Row: row, Column: col, Value: formatSumNumber(sum)}}
				}
			}
		}
		if col > 1 {
			scanStartCol := maxInt(1, col-50)
			scanBounds := document.CellRange{StartRow: row, StartColumn: scanStartCol, EndRow: row, EndColumn: col - 1}
			cells, err := reader.ReadRange(sheet, scanBounds)
			if err == nil && len(cells) > 0 && len(cells[0]) > 0 {
				var sum float64
				var count int
				for c := len(cells[0]) - 1; c >= 0; c-- {
					v, ok := parseNumericValue(cells[0][c].Value)
					if !ok {
						break
					}
					sum += v
					count++
				}
				if count > 0 {
					return []document.CellEdit{{Row: row, Column: col, Value: formatSumNumber(sum)}}
				}
			}
		}
		return nil
	}

	// Case 2: Multi-cell selection
	cells, err := reader.ReadRange(sheet, bounds)
	if err != nil || len(cells) == 0 {
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

		var edits []document.CellEdit
		cols := bounds.EndColumn - bounds.StartColumn + 1
		for c := 0; c < cols; c++ {
			var colSum float64
			var count int
			for r := 0; r <= dataEndRowIdx; r++ {
				if c < len(cells[r]) {
					v, ok := parseNumericValue(cells[r][c].Value)
					if ok {
						colSum += v
						count++
					}
				}
			}
			if count > 0 {
				edits = append(edits, document.CellEdit{
					Row:    targetRow,
					Column: bounds.StartColumn + c,
					Value:  formatSumNumber(colSum),
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
	if dataEndColIdx < 0 {
		return nil
	}
	var rowSum float64
	var count int
	for c := 0; c <= dataEndColIdx; c++ {
		v, ok := parseNumericValue(cells[0][c].Value)
		if ok {
			rowSum += v
			count++
		}
	}
	if count > 0 {
		return []document.CellEdit{{
			Row:    bounds.StartRow,
			Column: targetCol,
			Value:  formatSumNumber(rowSum),
		}}
	}
	return nil
}

func parseNumericValue(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, ",", "")
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
			return v / 100.0, true
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func formatSumNumber(v float64) string {
	if math.Abs(v-math.Round(v)) < 1e-9 {
		return fmt.Sprintf("%.0f", math.Round(v))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
