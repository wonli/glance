package desktopui

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"glance/internal/document"
)

const maxClipboardCellsUI = 10000

func encodeTSV(cells [][]document.Cell) string {
	var output strings.Builder
	w := csv.NewWriter(&output)
	w.Comma = '\t'
	w.UseCRLF = false
	for _, row := range cells {
		record := make([]string, len(row))
		for i, cell := range row {
			record[i] = cell.Value
		}
		_ = w.Write(record)
	}
	w.Flush()
	return output.String()
}

func decodeTSV(text string) ([][]string, error) {
	if len(text) > 1<<20 {
		return nil, fmt.Errorf("clipboard text exceeds 1 MiB")
	}
	if text == "" {
		return [][]string{{""}}, nil
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("parse clipboard TSV: %w", err)
	}
	if len(rows) == 0 {
		return [][]string{{""}}, nil
	}
	cellCount := 0
	for _, row := range rows {
		cellCount += len(row)
		if cellCount > maxClipboardCellsUI {
			return nil, fmt.Errorf("clipboard data exceeds %d cells", maxClipboardCellsUI)
		}
	}
	return rows, nil
}

func editsForPaste(bounds document.CellRange, rows [][]string) []document.CellEdit {
	out := make([]document.CellEdit, 0)
	maxRow, maxCol := documentCellLimit(bounds)
	for r, row := range rows {
		targetRow := bounds.StartRow + r
		if targetRow > maxRow {
			break
		}
		for c, value := range row {
			targetCol := bounds.StartColumn + c
			if targetCol > maxCol {
				break
			}
			out = append(out, document.CellEdit{Row: targetRow, Column: targetCol, Value: value})
		}
	}
	return out
}

func pasteFits(bounds document.CellRange, rows [][]string) bool {
	rowCount := len(rows)
	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	return rowCount == 0 || colCount == 0 || (bounds.StartRow+rowCount-1 <= 1048576 && bounds.StartColumn+colCount-1 <= 16384)
}

func documentCellLimit(document.CellRange) (int, int) { return 1048576, 16384 }

func (a *App) copySelection() (string, error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet {
		return "", fmt.Errorf("请先选择单元格")
	}
	reader, ok := state.book.(document.SpreadsheetRangeReader)
	if !ok {
		return "", fmt.Errorf("当前表格不支持区域复制")
	}
	cells, err := reader.ReadRange(state.sheet, state.selection.Bounds())
	if err != nil {
		return "", err
	}
	return encodeTSV(cells), nil
}

func (a *App) pasteClipboard(text string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet {
		return fmt.Errorf("请先选择粘贴起始单元格")
	}
	rows, err := decodeTSV(text)
	if err != nil {
		return err
	}
	bounds := state.selection.Bounds()
	if !pasteFits(bounds, rows) {
		return fmt.Errorf("粘贴区域超出工作表边界；未写入任何单元格")
	}
	edits := editsForPaste(bounds, rows)
	if len(edits) == 0 {
		return nil
	}
	if _, ok := state.book.(document.SpreadsheetBatchEditor); !ok {
		return fmt.Errorf("当前表格不支持批量粘贴")
	}
	return a.applyBatchEdits(edits, "粘贴")
}
