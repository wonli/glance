package xlsx

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

const maxClipboardCells = 10000

var _ document.SpreadsheetBatchEditor = (*Workbook)(nil)
var _ document.SpreadsheetRangeReader = (*Workbook)(nil)

func (w *Workbook) ReadRange(sheet string, bounds document.CellRange) ([][]document.Cell, error) {
	return w.readRange(sheet, bounds, false)
}

func (w *Workbook) ReadClipboardRange(sheet string, bounds document.CellRange) ([][]document.Cell, error) {
	return w.readRange(sheet, bounds, true)
}

func (w *Workbook) readRange(sheet string, bounds document.CellRange, includeClipboardMetadata bool) ([][]document.Cell, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	if err := validateRange(sheet, bounds, w.file.GetSheetList()); err != nil {
		return nil, err
	}
	rows, cols := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	if rows*cols > maxClipboardCells {
		return nil, fmt.Errorf("clipboard selection exceeds %d cells", maxClipboardCells)
	}
	formulaTypes := map[string]string(nil)
	if includeClipboardMetadata {
		var err error
		formulaTypes, err = w.clipboardFormulaTypesLocked(sheet)
		if err != nil {
			return nil, fmt.Errorf("read copied formula types: %w", err)
		}
	}
	out := make([][]document.Cell, rows)
	for r := 0; r < rows; r++ {
		out[r] = make([]document.Cell, cols)
		for c := 0; c < cols; c++ {
			ref, _ := excelize.CoordinatesToCellName(bounds.StartColumn+c, bounds.StartRow+r)
			value, err := w.file.GetCellValue(sheet, ref)
			if err != nil {
				return nil, err
			}
			formula, err := w.file.GetCellFormula(sheet, ref)
			if err != nil {
				return nil, fmt.Errorf("read cell %s formula: %w", ref, err)
			}
			styleID, err := w.file.GetCellStyle(sheet, ref)
			if err != nil {
				return nil, fmt.Errorf("read cell %s style: %w", ref, err)
			}
			style, err := readCellStyle(w.file, styleID)
			if err != nil {
				return nil, fmt.Errorf("read cell %s style details: %w", ref, err)
			}
			cell := document.Cell{Value: value, Formula: formula, FormulaType: formulaTypes[ref], StyleID: styleID, Style: style}
			if formula == "" {
				cell.TypedValue, err = readClipboardTypedValue(w.file, sheet, ref)
				if err != nil {
					return nil, fmt.Errorf("read cell %s typed value: %w", ref, err)
				}
			}
			out[r][c] = cell
		}
	}
	return out, nil
}

func readClipboardTypedValue(file *excelize.File, sheet, cell string) (*document.CellValue, error) {
	cellType, err := file.GetCellType(sheet, cell)
	if err != nil {
		return nil, err
	}
	if cellType == excelize.CellTypeDate {
		// ISO date cells are stored as strings, while the neutral value model
		// does not yet carry an ISO date kind. Same-workbook paste uses the
		// displayed TSV value until that typed value is modeled explicitly.
		return nil, nil
	}
	kind := documentCellValueKind(cellType)
	raw, err := file.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, err
	}
	if kind == "" {
		if raw == "" && cellType == excelize.CellTypeUnset {
			return nil, nil
		}
		if cellType == excelize.CellTypeUnset {
			if number, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil && finiteNumber(number) {
				return &document.CellValue{Kind: document.CellValueNumber, Number: number}, nil
			}
		}
		return nil, nil
	}
	value := &document.CellValue{Kind: kind}
	switch kind {
	case document.CellValueText:
		value.Text = raw
	case document.CellValueNumber:
		value.Number, err = strconv.ParseFloat(raw, 64)
		if err != nil || !finiteNumber(value.Number) {
			return nil, fmt.Errorf("invalid stored numeric value %q", raw)
		}
	case document.CellValueBoolean:
		value.Boolean = strings.EqualFold(raw, "true") || raw == "1"
	}
	return value, nil
}

func (w *Workbook) SetCells(sheet string, edits []document.CellEdit) error {
	_, err := w.SetCellsChanged(sheet, edits)
	return err
}

func (w *Workbook) SetCellsChanged(sheet string, edits []document.CellEdit) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return false, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if len(edits) == 0 {
		return false, nil
	}
	if len(edits) > maxClipboardCells {
		return false, fmt.Errorf("clipboard edit exceeds %d cells", maxClipboardCells)
	}
	type valueEdit struct {
		ref   string
		value any
	}
	prepared := make([]valueEdit, len(edits))
	seen := map[string]bool{}
	for i, e := range edits {
		if e.Row < 1 || e.Column < 1 || e.Row > maxXLSXRows || e.Column > maxXLSXColumns {
			return false, errors.New("clipboard edit exceeds XLSX worksheet limits")
		}
		ref, _ := excelize.CoordinatesToCellName(e.Column, e.Row)
		if seen[ref] {
			return false, fmt.Errorf("duplicate clipboard destination %s", ref)
		}
		seen[ref] = true
		val, err := spreadsheetEditValue(e)
		if err != nil {
			return false, fmt.Errorf("invalid value for %s: %w", ref, err)
		}
		prepared[i] = valueEdit{ref: ref, value: val}
	}
	changed := false
	for i, e := range prepared {
		value, err := w.file.GetCellValue(sheet, e.ref)
		if err != nil {
			return false, err
		}
		if edits[i].TypedValue != nil {
			value, err = w.file.GetCellValue(sheet, e.ref, excelize.Options{RawCellValue: true})
			if err != nil {
				return false, err
			}
		}
		if !editValueMatchesCell(w.file, sheet, e.ref, edits[i], e.value, value) {
			changed = true
			break
		}
	}
	if !changed {
		return false, nil
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return false, err
	}
	for _, e := range prepared {
		if err := w.file.SetCellValue(sheet, e.ref, e.value); err != nil {
			w.restoreSnapshotLocked(before)
			return false, fmt.Errorf("write %s: %w", e.ref, err)
		}
	}
	if err := w.recordHistoryLocked(before); err != nil {
		w.restoreSnapshotLocked(before)
		return false, err
	}
	w.dirty = true
	return true, nil
}

func spreadsheetEditValue(edit document.CellEdit) (any, error) {
	if edit.TypedValue != nil {
		switch edit.TypedValue.Kind {
		case document.CellValueText:
			if strings.HasPrefix(strings.TrimSpace(edit.TypedValue.Text), "=") {
				return nil, errors.New("formula input is not supported")
			}
			return edit.TypedValue.Text, nil
		case document.CellValueNumber:
			if !finiteNumber(edit.TypedValue.Number) {
				return nil, errors.New("numeric cell values must be finite")
			}
			return edit.TypedValue.Number, nil
		case document.CellValueBoolean:
			return edit.TypedValue.Boolean, nil
		default:
			return nil, errors.New("unsupported explicit cell value type")
		}
	}
	if strings.HasPrefix(strings.TrimSpace(edit.Value), "=") {
		return nil, errors.New("formula input is not supported")
	}
	var value any = edit.Value
	trimmed := strings.TrimSpace(edit.Value)
	if strings.EqualFold(trimmed, "true") {
		value = true
	} else if strings.EqualFold(trimmed, "false") {
		value = false
	} else if n, err := parseFiniteNumber(trimmed); err == nil {
		value = n
	}
	return value, nil
}

func editValueMatchesCell(file *excelize.File, sheet, cell string, edit document.CellEdit, value any, current string) bool {
	if edit.TypedValue == nil {
		return current == displayCellValue(value)
	}
	cellType, err := file.GetCellType(sheet, cell)
	if err != nil {
		return false
	}
	switch edit.TypedValue.Kind {
	case document.CellValueText:
		if edit.TypedValue.Text == "" && cellType == excelize.CellTypeUnset && current == "" {
			return true
		}
		return (cellType == excelize.CellTypeInlineString || cellType == excelize.CellTypeSharedString) && current == edit.TypedValue.Text
	case document.CellValueNumber:
		return (cellType == excelize.CellTypeNumber || cellType == excelize.CellTypeUnset) && current == displayCellValue(value)
	case document.CellValueBoolean:
		if cellType != excelize.CellTypeBool {
			return false
		}
		boolean, _ := value.(bool)
		return strings.EqualFold(current, displayCellValue(value)) || (boolean && current == "1") || (!boolean && current == "0")
	default:
		return false
	}
}

func displayCellValue(value any) string {
	if boolean, ok := value.(bool); ok {
		return strings.ToUpper(strconv.FormatBool(boolean))
	}
	return fmt.Sprint(value)
}

func (w *Workbook) SetCellValueChanged(sheet, cell string, value document.CellValue) (bool, error) {
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return false, fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	edit := document.CellEdit{Row: row, Column: column, TypedValue: &value}
	return w.SetCellsChanged(sheet, []document.CellEdit{edit})
}

func parseFiniteNumber(s string) (float64, error) {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, errors.New("not finite number")
	}
	return n, nil
}

func (w *Workbook) restoreSnapshotLocked(snapshot []byte) error {
	restored, err := excelize.OpenReader(bytes.NewReader(snapshot))
	if err != nil {
		return err
	}
	previous := w.file
	w.file = restored
	w.pictureIndexes = nil
	w.invalidateStyleCacheLocked()
	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

func validateRange(sheet string, b document.CellRange, sheets []string) error {
	if !contains(sheets, sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if b.StartRow < 1 || b.StartColumn < 1 || b.EndRow < b.StartRow || b.EndColumn < b.StartColumn || b.EndRow > maxXLSXRows || b.EndColumn > maxXLSXColumns {
		return errors.New("range is outside XLSX worksheet limits")
	}
	return nil
}
