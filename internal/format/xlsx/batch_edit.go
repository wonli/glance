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
	out := make([][]document.Cell, rows)
	for r := 0; r < rows; r++ {
		out[r] = make([]document.Cell, cols)
		for c := 0; c < cols; c++ {
			ref, _ := excelize.CoordinatesToCellName(bounds.StartColumn+c, bounds.StartRow+r)
			value, err := w.file.GetCellValue(sheet, ref)
			if err != nil {
				return nil, err
			}
			out[r][c] = document.Cell{Value: value}
		}
	}
	return out, nil
}

func (w *Workbook) SetCells(sheet string, edits []document.CellEdit) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if len(edits) == 0 {
		return nil
	}
	if len(edits) > maxClipboardCells {
		return fmt.Errorf("clipboard edit exceeds %d cells", maxClipboardCells)
	}
	type valueEdit struct {
		ref   string
		value any
	}
	prepared := make([]valueEdit, len(edits))
	seen := map[string]bool{}
	for i, e := range edits {
		if e.Row < 1 || e.Column < 1 || e.Row > maxXLSXRows || e.Column > maxXLSXColumns {
			return errors.New("clipboard edit exceeds XLSX worksheet limits")
		}
		if strings.HasPrefix(strings.TrimSpace(e.Value), "=") {
			return errors.New("formula input is not supported")
		}
		ref, _ := excelize.CoordinatesToCellName(e.Column, e.Row)
		if seen[ref] {
			return fmt.Errorf("duplicate clipboard destination %s", ref)
		}
		seen[ref] = true
		var val any = e.Value
		trimmed := strings.TrimSpace(e.Value)
		if strings.EqualFold(trimmed, "true") {
			val = true
		} else if strings.EqualFold(trimmed, "false") {
			val = false
		} else if n, err := parseFiniteNumber(trimmed); err == nil {
			val = n
		}
		prepared[i] = valueEdit{ref: ref, value: val}
	}
	changed := false
	for _, e := range prepared {
		value, err := w.file.GetCellValue(sheet, e.ref)
		if err != nil {
			return err
		}
		if value != fmt.Sprint(e.value) {
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}
	before, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	for _, e := range prepared {
		if err := w.file.SetCellValue(sheet, e.ref, e.value); err != nil {
			w.restoreSnapshotLocked(before)
			return fmt.Errorf("write %s: %w", e.ref, err)
		}
	}
	if err := w.recordHistoryLocked(before); err != nil {
		w.restoreSnapshotLocked(before)
		return err
	}
	w.dirty = true
	return nil
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
