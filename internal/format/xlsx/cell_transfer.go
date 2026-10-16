package xlsx

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetCellTransferEditor = (*Workbook)(nil)

type clipboardFormulaGroup struct {
	kind   string
	ref    string
	bounds document.CellRange
}

func (w *Workbook) TransferCellRangeChanged(sheet string, source document.CellRange, targetRow, targetColumn int, cells [][]document.Cell) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	if err := validateRange(sheet, source, w.file.GetSheetList()); err != nil {
		return false, err
	}
	rows, columns := source.EndRow-source.StartRow+1, source.EndColumn-source.StartColumn+1
	if rows*columns > maxClipboardCells || len(cells) != rows {
		return false, fmt.Errorf("clipboard range dimensions do not match source")
	}
	for _, row := range cells {
		if len(row) != columns {
			return false, fmt.Errorf("clipboard range dimensions do not match source")
		}
	}
	destination := document.CellRange{
		StartRow: targetRow, StartColumn: targetColumn,
		EndRow: targetRow + rows - 1, EndColumn: targetColumn + columns - 1,
	}
	if err := validateRange(sheet, destination, w.file.GetSheetList()); err != nil {
		return false, fmt.Errorf("paste target: %w", err)
	}
	for _, row := range cells {
		for _, cell := range row {
			if cell.FormulaType != "" && cell.FormulaType != "normal" {
				return false, fmt.Errorf("copying %s formula groups is not supported", cell.FormulaType)
			}
			if cell.Formula == "" && cell.TypedValue == nil && cell.Value != "" {
				return false, fmt.Errorf("clipboard contains a cell type that cannot be preserved safely")
			}
			if cell.StyleID < 0 {
				return false, fmt.Errorf("clipboard contains an invalid cell style")
			}
		}
	}
	groups, err := w.clipboardFormulaGroupsLocked(sheet)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if rangesIntersect(group.bounds, destination) {
			return false, fmt.Errorf("paste would modify part of a %s formula range %s", group.kind, group.ref)
		}
	}

	definedNames := make(map[string]bool)
	for _, name := range w.file.GetDefinedName() {
		definedNames[strings.ToLower(name.Name)] = true
	}
	type preparedCell struct {
		ref     string
		formula string
		cell    document.Cell
	}
	prepared := make([]preparedCell, 0, rows*columns)
	for rowIndex, row := range cells {
		for columnIndex, cell := range row {
			destRow, destColumn := targetRow+rowIndex, targetColumn+columnIndex
			ref, err := excelize.CoordinatesToCellName(destColumn, destRow)
			if err != nil {
				return false, err
			}
			formula := ""
			if cell.Formula != "" {
				sourceFormula := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cell.Formula), "="))
				if sourceFormula == "" {
					return false, fmt.Errorf("copied formula for %s is empty", ref)
				}
				formula, err = translateClipboardFormula(sourceFormula, destRow-(source.StartRow+rowIndex), destColumn-(source.StartColumn+columnIndex), definedNames)
				if err != nil {
					return false, fmt.Errorf("adjust copied formula for %s: %w", ref, err)
				}
				formula = normalizeFormulaFunctionNames(formula)
				if !withinFormulaUTF16Limit(formula) {
					return false, fmt.Errorf("copied formula for %s exceeds 8192 UTF-16 units", ref)
				}
			}
			prepared = append(prepared, preparedCell{ref: ref, formula: formula, cell: cell})
		}
	}

	changed := false
	for _, edit := range prepared {
		currentFormula, err := w.file.GetCellFormula(sheet, edit.ref)
		if err != nil {
			return false, err
		}
		currentStyle, err := w.file.GetCellStyle(sheet, edit.ref)
		if err != nil {
			return false, err
		}
		if currentFormula != edit.formula || currentStyle != edit.cell.StyleID {
			changed = true
			break
		}
		if edit.formula != "" {
			continue
		}
		current, err := w.file.GetCellValue(sheet, edit.ref, excelize.Options{RawCellValue: true})
		if err != nil {
			return false, err
		}
		if edit.cell.TypedValue == nil {
			if current != "" {
				changed = true
				break
			}
			continue
		}
		currentTyped, err := readClipboardTypedValue(w.file, sheet, edit.ref)
		if err != nil {
			return false, err
		}
		if !sameClipboardValue(currentTyped, edit.cell.TypedValue) {
			changed = true
			break
		}
	}
	if !changed {
		return false, nil
	}

	before, err := w.snapshotLocked()
	if err != nil {
		return false, fmt.Errorf("capture clipboard history: %w", err)
	}
	rollback := func(cause error) (bool, error) {
		if rollbackErr := w.restoreSnapshotLocked(before); rollbackErr != nil {
			return false, fmt.Errorf("%w; restoring workbook after failed paste: %v", cause, rollbackErr)
		}
		return false, cause
	}
	for _, edit := range prepared {
		if err := w.file.SetCellFormula(sheet, edit.ref, ""); err != nil {
			return rollback(fmt.Errorf("clear target formula %s: %w", edit.ref, err))
		}
		if edit.formula != "" {
			if err := w.file.SetCellFormula(sheet, edit.ref, edit.formula); err != nil {
				return rollback(fmt.Errorf("paste formula %s: %w", edit.ref, err))
			}
		} else {
			value, err := clipboardCellValue(edit.cell)
			if err != nil {
				return rollback(fmt.Errorf("paste value %s: %w", edit.ref, err))
			}
			if err := w.file.SetCellValue(sheet, edit.ref, value); err != nil {
				return rollback(fmt.Errorf("paste value %s: %w", edit.ref, err))
			}
		}
		if err := w.file.SetCellStyle(sheet, edit.ref, edit.ref, edit.cell.StyleID); err != nil {
			return rollback(fmt.Errorf("paste style %s: %w", edit.ref, err))
		}
	}
	if err := w.file.UpdateLinkedValue(); err != nil {
		return rollback(fmt.Errorf("update copied formula results: %w", err))
	}
	if err := w.recordHistoryLocked(before); err != nil {
		return rollback(fmt.Errorf("record clipboard history: %w", err))
	}
	w.dirty = true
	return true, nil
}

func clipboardCellValue(cell document.Cell) (any, error) {
	if cell.TypedValue == nil {
		if cell.Value == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("source cell value has no supported type")
	}
	switch cell.TypedValue.Kind {
	case document.CellValueText:
		return cell.TypedValue.Text, nil
	case document.CellValueNumber:
		if !finiteNumber(cell.TypedValue.Number) {
			return nil, fmt.Errorf("source cell contains a non-finite number")
		}
		return cell.TypedValue.Number, nil
	case document.CellValueBoolean:
		return cell.TypedValue.Boolean, nil
	default:
		return nil, fmt.Errorf("source cell value type %q is unsupported", cell.TypedValue.Kind)
	}
}

func sameClipboardValue(a, b *document.CellValue) bool {
	if a == nil || b == nil || a.Kind != b.Kind {
		return a == nil && b == nil
	}
	switch a.Kind {
	case document.CellValueText:
		return a.Text == b.Text
	case document.CellValueNumber:
		return a.Number == b.Number
	case document.CellValueBoolean:
		return a.Boolean == b.Boolean
	default:
		return false
	}
}

func (w *Workbook) clipboardFormulaGroupsLocked(sheet string) ([]clipboardFormulaGroup, error) {
	snapshot, err := w.snapshotLocked()
	if err != nil {
		return nil, err
	}
	files, target, err := scopePackage(snapshot, sheet)
	if err != nil {
		return nil, err
	}
	root, err := parseScopeXML(files[target])
	if err != nil {
		return nil, err
	}
	worksheet := root.child("worksheet")
	if worksheet == nil || worksheet.child("sheetData") == nil {
		return nil, nil
	}
	var groups []clipboardFormulaGroup
	for _, row := range worksheet.child("sheetData").children("row") {
		for _, cell := range row.children("c") {
			formula := cell.child("f")
			if formula == nil {
				continue
			}
			kind := formula.attrs["t"]
			if kind != "shared" && kind != "array" && kind != "dataTable" {
				continue
			}
			ref := formula.attrs["ref"]
			if ref == "" {
				ref = cell.attrs["r"]
			}
			bounds, err := formulaReferenceBounds(ref)
			if err != nil {
				return nil, fmt.Errorf("cannot safely identify %s formula group at %s: %w", kind, cell.attrs["r"], err)
			}
			groups = append(groups, clipboardFormulaGroup{kind: kind, ref: ref, bounds: bounds})
		}
	}
	return groups, nil
}

func (w *Workbook) clipboardFormulaTypesLocked(sheet string) (map[string]string, error) {
	snapshot, err := w.snapshotLocked()
	if err != nil {
		return nil, err
	}
	files, target, err := scopePackage(snapshot, sheet)
	if err != nil {
		return nil, err
	}
	root, err := parseScopeXML(files[target])
	if err != nil {
		return nil, err
	}
	types := make(map[string]string)
	worksheet := root.child("worksheet")
	if worksheet == nil || worksheet.child("sheetData") == nil {
		return types, nil
	}
	for _, row := range worksheet.child("sheetData").children("row") {
		for _, cell := range row.children("c") {
			formula := cell.child("f")
			if formula != nil && formula.attrs["t"] != "" {
				types[cell.attrs["r"]] = formula.attrs["t"]
			}
		}
	}
	return types, nil
}

func formulaReferenceBounds(ref string) (document.CellRange, error) {
	parts := strings.Split(strings.ReplaceAll(ref, "$", ""), ":")
	if len(parts) == 1 {
		parts = append(parts, parts[0])
	}
	if len(parts) != 2 {
		return document.CellRange{}, fmt.Errorf("invalid cell reference %q", ref)
	}
	startColumn, startRow, err := excelize.CellNameToCoordinates(parts[0])
	if err != nil {
		return document.CellRange{}, err
	}
	endColumn, endRow, err := excelize.CellNameToCoordinates(parts[1])
	if err != nil || startColumn > endColumn || startRow > endRow {
		return document.CellRange{}, fmt.Errorf("invalid cell range %q", ref)
	}
	return document.CellRange{StartRow: startRow, StartColumn: startColumn, EndRow: endRow, EndColumn: endColumn}, nil
}

func rangesIntersect(a, b document.CellRange) bool {
	return a.StartRow <= b.EndRow && a.EndRow >= b.StartRow && a.StartColumn <= b.EndColumn && a.EndColumn >= b.StartColumn
}
