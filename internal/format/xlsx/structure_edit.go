package xlsx

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetStructureEditor = (*Workbook)(nil)

func (w *Workbook) EditStructure(edit document.StructureEdit) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	sheets := w.file.GetSheetList()
	sheet := strings.TrimSpace(edit.Sheet)
	if edit.Kind != document.StructureCreateSheet && !contains(sheets, sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if err := validateStructureEdit(edit, sheets); err != nil {
		return err
	}
	if edit.Kind == document.StructureInsertRows || edit.Kind == document.StructureInsertColumns {
		ref, dimErr := w.file.GetSheetDimension(sheet)
		if dimErr != nil {
			return dimErr
		}
		if ref != "" {
			parts := strings.Split(ref, ":")
			col, row, coordErr := excelize.CellNameToCoordinates(parts[len(parts)-1])
			if coordErr == nil {
				used, limit := row, maxXLSXRows
				if edit.Kind == document.StructureInsertColumns {
					used, limit = col, maxXLSXColumns
				}
				if used+edit.Count > limit {
					return errors.New("insert would exceed XLSX worksheet limits")
				}
			}
		}
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	switch edit.Kind {
	case document.StructureCreateSheet:
		name := strings.TrimSpace(edit.NewName)
		if name == "" {
			name = nextSheetName(sheets)
		}
		_, err = w.file.NewSheet(name)
	case document.StructureRenameSheet:
		err = w.file.SetSheetName(sheet, strings.TrimSpace(edit.NewName))
	case document.StructureDeleteSheet:
		err = w.file.DeleteSheet(sheet)
	case document.StructureInsertRows:
		err = w.file.InsertRows(sheet, edit.Index, edit.Count)
	case document.StructureDeleteRows:
		for i := 0; i < edit.Count && err == nil; i++ {
			err = w.file.RemoveRow(sheet, edit.Index)
		}
	case document.StructureInsertColumns:
		col, _ := excelize.ColumnNumberToName(edit.Index)
		err = w.file.InsertCols(sheet, col, edit.Count)
	case document.StructureDeleteColumns:
		col, _ := excelize.ColumnNumberToName(edit.Index)
		for i := 0; i < edit.Count && err == nil; i++ {
			err = w.file.RemoveCol(sheet, col)
		}
	default:
		return errors.New("unsupported worksheet structure operation")
	}
	if err != nil {
		_ = w.restoreSnapshotLocked(before)
		return fmt.Errorf("edit worksheet structure: %w", err)
	}
	if err := w.recordHistoryLocked(before); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return err
	}
	w.dirty = true
	return nil
}

func validateStructureEdit(edit document.StructureEdit, sheets []string) error {
	switch edit.Kind {
	case document.StructureCreateSheet, document.StructureRenameSheet:
		name := strings.TrimSpace(edit.NewName)
		if name == "" && edit.Kind == document.StructureCreateSheet {
			return nil
		}
		if name == "" || utf8.RuneCountInString(name) > 31 || strings.ContainsAny(name, "[]:*?/\\") || strings.HasPrefix(name, "'") || strings.HasSuffix(name, "'") {
			return fmt.Errorf("invalid worksheet name %q", name)
		}
		for _, existing := range sheets {
			if strings.EqualFold(existing, name) && !(edit.Kind == document.StructureRenameSheet && strings.EqualFold(existing, edit.Sheet)) {
				return fmt.Errorf("worksheet %q already exists", name)
			}
		}
	case document.StructureDeleteSheet:
		if len(sheets) <= 1 {
			return errors.New("cannot delete the last worksheet")
		}
	case document.StructureInsertRows, document.StructureDeleteRows, document.StructureInsertColumns, document.StructureDeleteColumns:
		max := maxXLSXRows
		if edit.Kind == document.StructureInsertColumns || edit.Kind == document.StructureDeleteColumns {
			max = maxXLSXColumns
		}
		if edit.Index < 1 || edit.Count < 1 || edit.Index > max || edit.Count > max-edit.Index+1 {
			return errors.New("worksheet structure range exceeds XLSX limits")
		}
	default:
		return errors.New("unsupported worksheet structure operation")
	}
	return nil
}

func nextSheetName(sheets []string) string {
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("Sheet%d", i)
		found := false
		for _, s := range sheets {
			if strings.EqualFold(s, candidate) {
				found = true
				break
			}
		}
		if !found {
			return candidate
		}
	}
}
