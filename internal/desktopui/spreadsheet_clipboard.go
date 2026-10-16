package desktopui

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"

	"glance/internal/document"
)

const maxClipboardCellsUI = 10000

type spreadsheetClipboardPasteTarget struct {
	sheet  string
	bounds document.CellRange
}

type spreadsheetClipboardData struct {
	documentID string
	sheet      string
	bounds     document.CellRange
	text       string
	cells      [][]document.Cell
}

func (a *App) requestSpreadsheetClipboardPaste(gtx layout.Context, state *documentSessionState) {
	if state == nil || state.book == nil || !state.selectionSet {
		a.setStatus("请先选择粘贴起始单元格")
		return
	}
	if state.clipboardPasteTarget != nil {
		a.setStatus("已有一项粘贴正在读取剪贴板")
		return
	}
	state.clipboardPasteTarget = &spreadsheetClipboardPasteTarget{sheet: state.sheet, bounds: state.selection.Bounds()}
	gtx.Execute(clipboard.ReadCmd{Tag: &state.clipboardTarget})
}

func encodeTSV(cells [][]document.Cell) string {
	values := make([][]string, len(cells))
	for rowIndex, row := range cells {
		values[rowIndex] = make([]string, len(row))
		for columnIndex, cell := range row {
			values[rowIndex][columnIndex] = cell.Value
		}
	}
	return encodeTSVValues(values)
}

func encodeTSVValues(rows [][]string) string {
	var output strings.Builder
	w := csv.NewWriter(&output)
	w.Comma = '\t'
	w.UseCRLF = false
	for _, row := range rows {
		_ = w.Write(row)
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

type spreadsheetRangeCopyArguments struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

func (a *App) copySelection() (string, error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet {
		return "", fmt.Errorf("请先选择单元格")
	}
	reader, ok := state.book.(document.SpreadsheetRangeReader)
	if !ok {
		return "", fmt.Errorf("当前表格不支持区域复制")
	}
	if a.applicationSessionRegistered(state.documentID) {
		token, ok := state.beginOperation(sessionOperationRead)
		if !ok {
			return "", fmt.Errorf("表格正在处理其他操作")
		}
		bounds := state.selection.Bounds()
		result, err := a.executeRibbonApplicationCommand(token, "spreadsheet.range.copy", "spreadsheet_range", "clipboard", state.sheet, spreadsheetRangeCopyArguments{Sheet: state.sheet, Bounds: bounds})
		a.completeSessionOperation(state, token, err, false)
		if err != nil {
			return "", err
		}
		var copied struct {
			Rows  [][]string        `json:"rows"`
			Cells [][]document.Cell `json:"cells"`
		}
		if err := json.Unmarshal(result.Data, &copied); err != nil {
			return "", err
		}
		text := encodeTSVValues(copied.Rows)
		a.rememberSpreadsheetClipboard(state.documentID, state.sheet, bounds, text, copied.Cells)
		return text, nil
	}
	unlock := lockSessionAdapterRead(state)
	var cells [][]document.Cell
	var err error
	if clipboardReader, ok := state.book.(document.SpreadsheetClipboardRangeReader); ok {
		cells, err = clipboardReader.ReadClipboardRange(state.sheet, state.selection.Bounds())
	} else {
		cells, err = reader.ReadRange(state.sheet, state.selection.Bounds())
	}
	unlock()
	if err != nil {
		return "", err
	}
	text := encodeTSV(cells)
	a.rememberSpreadsheetClipboard(state.documentID, state.sheet, state.selection.Bounds(), text, cells)
	return text, nil
}

func (a *App) rememberSpreadsheetClipboard(documentID, sheet string, bounds document.CellRange, text string, cells [][]document.Cell) {
	if a.desktopRuntime == nil || len(cells) == 0 {
		return
	}
	a.desktopRuntime.spreadsheetClipboardMu.Lock()
	a.desktopRuntime.spreadsheetClipboard = &spreadsheetClipboardData{
		documentID: documentID, sheet: sheet, bounds: bounds, text: text,
		cells: cloneSpreadsheetClipboardCells(cells),
	}
	a.desktopRuntime.spreadsheetClipboardMu.Unlock()
}

func (a *App) matchingSpreadsheetClipboard(documentID, text string) *spreadsheetClipboardData {
	if a.desktopRuntime == nil {
		return nil
	}
	a.desktopRuntime.spreadsheetClipboardMu.RLock()
	defer a.desktopRuntime.spreadsheetClipboardMu.RUnlock()
	cached := a.desktopRuntime.spreadsheetClipboard
	if cached == nil || cached.documentID != documentID || cached.text != text {
		return nil
	}
	return &spreadsheetClipboardData{
		documentID: cached.documentID, sheet: cached.sheet, bounds: cached.bounds,
		text: cached.text, cells: cloneSpreadsheetClipboardCells(cached.cells),
	}
}

func cloneSpreadsheetClipboardCells(cells [][]document.Cell) [][]document.Cell {
	copyOfCells := make([][]document.Cell, len(cells))
	for rowIndex, row := range cells {
		copyOfCells[rowIndex] = make([]document.Cell, len(row))
		for columnIndex, cell := range row {
			copyOfCells[rowIndex][columnIndex] = cell
			if cell.TypedValue != nil {
				value := *cell.TypedValue
				copyOfCells[rowIndex][columnIndex].TypedValue = &value
			}
		}
	}
	return copyOfCells
}

func supportsRichSpreadsheetClipboard(cells [][]document.Cell) bool {
	for _, row := range cells {
		for _, cell := range row {
			if cell.FormulaType != "" && cell.FormulaType != "normal" {
				return false
			}
			if cell.Formula == "" && cell.TypedValue == nil && cell.Value != "" {
				return false
			}
		}
	}
	return true
}

func (a *App) writeSpreadsheetSelectionToClipboard(gtx layout.Context, cut bool) {
	text, err := a.copySelection()
	if err != nil {
		a.setStatus("剪贴板操作失败：" + err.Error())
		return
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
	if cut {
		a.clearSelectionAsync()
	}
}

func (a *App) pasteClipboardAt(sheet string, bounds document.CellRange, text string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return fmt.Errorf("请先选择粘贴起始单元格")
	}
	if _, ok := state.book.(document.SpreadsheetCellTransferEditor); ok {
		if cached := a.matchingSpreadsheetClipboard(state.documentID, text); cached != nil {
			if supportsRichSpreadsheetClipboard(cached.cells) {
				return a.applyCellRangeTransferForSheet(cached.bounds, bounds.StartRow, bounds.StartColumn, cached.cells, sheet)
			}
		}
	}
	rows, err := decodeTSV(text)
	if err != nil {
		return err
	}
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
	return a.applyBatchEditsForSheet(edits, "粘贴", sheet)
}
