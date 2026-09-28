package xlsx

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"glance/internal/document"
	"glance/internal/fileutil"

	"github.com/xuri/excelize/v2"
)

// Workbook keeps Excelize behind the format adapter boundary.
type Workbook struct {
	mu     sync.Mutex
	path   string
	file   *excelize.File
	dirty  bool
	closed bool
}

var _ document.Spreadsheet = (*Workbook)(nil)

func CellName(column, row int) string {
	name, err := excelize.CoordinatesToCellName(column, row)
	if err != nil {
		return ""
	}
	return name
}

func Open(path string) (*Workbook, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx %q: %w", path, err)
	}
	return &Workbook{path: path, file: f}, nil
}

func (w *Workbook) SheetNames() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.file == nil {
		return nil
	}
	return append([]string(nil), w.file.GetSheetList()...)
}

// Dimension returns the used range's maximum column and row, in that order.
func (w *Workbook) Dimension(sheet string) (maxColumn, maxRow int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err = w.checkOpen(); err != nil {
		return 0, 0, err
	}
	ref, err := w.file.GetSheetDimension(sheet)
	if err != nil {
		return 0, 0, err
	}
	if ref == "" {
		return 1, 1, nil
	}
	end := strings.Split(ref, ":")[len(strings.Split(ref, ":"))-1]
	column, row, err := excelize.CellNameToCoordinates(end)
	if err != nil {
		return 0, 0, fmt.Errorf("parse used range %q: %w", ref, err)
	}
	return column, row, nil
}

func (w *Workbook) Value(sheet, cell string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return "", err
	}
	return w.file.GetCellValue(sheet, cell)
}

// Range returns a bounded, rectangular preview beginning at A1. Large sheets
// are intentionally truncated to keep UI memory and frame work predictable.
func (w *Workbook) Range(sheet string, maxRows, maxColumns int) ([][]document.Cell, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	if maxRows < 1 || maxColumns < 1 {
		return nil, errors.New("range limits must be positive")
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return nil, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	rows, err := w.file.Rows(sheet)
	if err != nil {
		return nil, err
	}
	result := make([][]document.Cell, 0, maxRows)
	for r := 0; r < maxRows && rows.Next(); r++ {
		values, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		if len(values) > maxColumns {
			values = values[:maxColumns]
		}
		previewRow := make([]document.Cell, maxColumns)
		for c, value := range values {
			ref, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			styleID, err := w.file.GetCellStyle(sheet, ref)
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			previewRow[c] = document.Cell{Value: value, StyleID: styleID}
		}
		result = append(result, previewRow)
	}
	if err := rows.Error(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return result, nil
}

// SetCellText edits a cell as a literal string. Formula entry is deliberately
// deferred until calculation and round-trip behavior are defined.
func (w *Workbook) SetCellText(sheet, cell, value string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if strings.HasPrefix(strings.TrimSpace(value), "=") {
		return errors.New("formula input is not supported")
	}
	if _, _, err := excelize.CellNameToCoordinates(cell); err != nil {
		return fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	current, err := w.file.GetCellValue(sheet, cell)
	if err != nil {
		return err
	}
	if current == value {
		return nil
	}
	var typedValue any = value
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TRUE":
		typedValue = true
	case "FALSE":
		typedValue = false
	default:
		if number, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil {
			if math.IsInf(number, 0) || math.IsNaN(number) {
				return errors.New("numeric cell values must be finite")
			}
			typedValue = number
		} else if strings.EqualFold(strings.TrimSpace(value), "nan") || strings.EqualFold(strings.TrimSpace(value), "inf") || strings.EqualFold(strings.TrimSpace(value), "+inf") || strings.EqualFold(strings.TrimSpace(value), "-inf") {
			return errors.New("numeric cell values must be finite")
		}
	}
	if err := w.file.SetCellValue(sheet, cell, typedValue); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

func (w *Workbook) Dirty() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dirty
}

func (w *Workbook) Editable() bool { return true }

func (w *Workbook) Save() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if w.path == "" {
		return errors.New("workbook has no save path")
	}
	if err := w.saveTo(w.path); err != nil {
		return err
	}
	w.dirty = false
	return nil
}

// SaveAs writes an independent copy without changing the workbook's source path.
func (w *Workbook) SaveAs(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	return w.saveTo(path)
}

func (w *Workbook) saveTo(path string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".glance-*.xlsx")
	if err != nil {
		return fmt.Errorf("create temporary workbook: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	if err := w.file.SaveAs(tmpPath); err != nil {
		return fmt.Errorf("write workbook: %w", err)
	}
	written, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	if err := written.Sync(); err != nil {
		_ = written.Close()
		return err
	}
	if err := written.Close(); err != nil {
		return err
	}
	if err := fileutil.ReplaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace workbook: %w", err)
	}
	return nil
}

func (w *Workbook) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *Workbook) checkOpen() error {
	if w.closed || w.file == nil {
		return errors.New("workbook is closed")
	}
	return nil
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
