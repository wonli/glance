package xlsx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	if err := w.file.SetCellValue(sheet, cell, value); err != nil {
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
	if err := os.Rename(tmpPath, path); err != nil {
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
