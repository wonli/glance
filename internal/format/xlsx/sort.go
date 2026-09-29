package xlsx

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetSorter = (*Workbook)(nil)

type sortCell struct {
	value any
	style int
}
type sortRow struct {
	cells []sortCell
	key   string
}

func (w *Workbook) SortRange(request document.SortRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	b := request.Bounds
	if err := validateRange(request.Sheet, b, w.file.GetSheetList()); err != nil {
		return err
	}
	if request.KeyColumn < b.StartColumn || request.KeyColumn > b.EndColumn {
		return errors.New("sort key column is outside the selected range")
	}
	rows, cols := b.EndRow-b.StartRow+1, b.EndColumn-b.StartColumn+1
	if rows < 2 {
		return nil
	}
	if rows*cols > maxClipboardCells {
		return fmt.Errorf("sort range exceeds %d cells", maxClipboardCells)
	}
	merges, err := w.file.GetMergeCells(request.Sheet)
	if err != nil {
		return err
	}
	for _, merge := range merges {
		r1, c1, _ := excelize.CellNameToCoordinates(merge.GetStartAxis())
		r2, c2, _ := excelize.CellNameToCoordinates(merge.GetEndAxis())
		if r1 <= b.EndRow && r2 >= b.StartRow && c1 <= b.EndColumn && c2 >= b.StartColumn {
			return errors.New("cannot sort a range containing merged cells")
		}
	}
	data := make([]sortRow, rows)
	for r := 0; r < rows; r++ {
		data[r].cells = make([]sortCell, cols)
		for c := 0; c < cols; c++ {
			ref, _ := excelize.CoordinatesToCellName(b.StartColumn+c, b.StartRow+r)
			formula, err := w.file.GetCellFormula(request.Sheet, ref)
			if err != nil {
				return err
			}
			if formula != "" {
				return fmt.Errorf("cannot sort range containing formula at %s", ref)
			}
			raw, err := w.file.GetCellValue(request.Sheet, ref, excelize.Options{RawCellValue: true})
			if err != nil {
				return err
			}
			cellType, err := w.file.GetCellType(request.Sheet, ref)
			if err != nil {
				return err
			}
			var value any
			switch cellType {
			case excelize.CellTypeUnset:
				if raw == "" {
					value = ""
				} else {
					n, parseErr := parseFiniteNumber(raw)
					if parseErr != nil {
						return fmt.Errorf("cannot safely sort untyped cell %s", ref)
					}
					value = n
				}
			case excelize.CellTypeNumber, excelize.CellTypeDate:
				n, parseErr := parseFiniteNumber(raw)
				if parseErr != nil {
					return fmt.Errorf("cannot safely sort numeric cell %s", ref)
				}
				value = n
			case excelize.CellTypeBool:
				value = raw == "1" || strings.EqualFold(raw, "true")
			case excelize.CellTypeInlineString, excelize.CellTypeSharedString:
				value = raw
			default:
				return fmt.Errorf("cannot safely sort cell %s with type %d", ref, cellType)
			}
			style, err := w.file.GetCellStyle(request.Sheet, ref)
			if err != nil {
				return err
			}
			data[r].cells[c] = sortCell{value: value, style: style}
			if b.StartColumn+c == request.KeyColumn {
				data[r].key = raw
			}
		}
	}
	sort.SliceStable(data, func(i, j int) bool { return compareSortValues(data[i].key, data[j].key, request.Descending) < 0 })
	before, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	for r, row := range data {
		for c, cell := range row.cells {
			ref, _ := excelize.CoordinatesToCellName(b.StartColumn+c, b.StartRow+r)
			var err error
			if text, ok := cell.value.(string); ok {
				err = w.file.SetCellStr(request.Sheet, ref, text)
			} else {
				err = w.file.SetCellValue(request.Sheet, ref, cell.value)
			}
			if err != nil {
				_ = w.restoreSnapshotLocked(before)
				return err
			}
			if err := w.file.SetCellStyle(request.Sheet, ref, ref, cell.style); err != nil {
				_ = w.restoreSnapshotLocked(before)
				return err
			}
		}
	}
	if err := w.recordHistoryLocked(before); err != nil {
		_ = w.restoreSnapshotLocked(before)
		return err
	}
	w.dirty = true
	return nil
}

func compareSortValues(a, b string, descending bool) int {
	aBlank, bBlank := strings.TrimSpace(a) == "", strings.TrimSpace(b) == ""
	if aBlank != bBlank {
		if aBlank {
			return 1
		}
		return -1
	}
	na, ea := parseFiniteNumber(strings.TrimSpace(a))
	nb, eb := parseFiniteNumber(strings.TrimSpace(b))
	result := 0
	switch {
	case ea == nil && eb == nil:
		if na < nb {
			result = -1
		} else if na > nb {
			result = 1
		}
	case ea == nil:
		result = -1
	case eb == nil:
		result = 1
	default:
		la, lb := strings.ToLower(a), strings.ToLower(b)
		if la < lb {
			result = -1
		} else if la > lb {
			result = 1
		}
	}
	if descending {
		return -result
	}
	return result
}
