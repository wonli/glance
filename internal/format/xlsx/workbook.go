package xlsx

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

// Workbook keeps Excelize behind the format adapter boundary.
type Workbook struct {
	mu                sync.Mutex
	path              string
	file              *excelize.File
	dirty             bool
	closed            bool
	pictureIndexes    map[string]spreadsheetPictureIndex
	undo              []workbookHistoryEntry
	redo              []workbookHistoryEntry
	historyState      uint64
	savedHistoryState uint64
	nextHistoryState  uint64
}

var _ document.Spreadsheet = (*Workbook)(nil)
var _ document.GridReader = (*Workbook)(nil)
var _ document.SpreadsheetHistory = (*Workbook)(nil)
var _ document.SpreadsheetImageReader = (*Workbook)(nil)

const (
	maxXLSXRows    = 1_048_576
	maxXLSXColumns = 16_384
)

func Open(path string) (*Workbook, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx %q: %w", path, err)
	}
	w := &Workbook{path: path, file: f, historyState: 1, savedHistoryState: 1, nextHistoryState: 2}
	return w, nil
}

// OpenSnapshot restores a self-contained XLSX package persisted for recovery.
func OpenSnapshot(payload []byte) (*Workbook, error) {
	f, err := excelize.OpenReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("restore XLSX snapshot: %w", err)
	}
	w := &Workbook{file: f, historyState: 1, savedHistoryState: 1, nextHistoryState: 2}
	return w, nil
}

// NewBlank creates an unsaved workbook draft.
func NewBlank() *Workbook {
	return &Workbook{file: excelize.NewFile(), dirty: true, historyState: 1, nextHistoryState: 2}
}

// Snapshot serializes the current in-memory workbook without changing dirty state.
func (w *Workbook) Snapshot() ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	buf, err := w.file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.Bytes()...), nil
}

func (w *Workbook) SetSourcePath(path string) { w.mu.Lock(); defer w.mu.Unlock(); w.path = path }
func (w *Workbook) SetDirty(dirty bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dirty = dirty
	if !dirty {
		w.savedHistoryState = w.historyState
	}
}

func (w *Workbook) Dirty() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dirty
}

func (w *Workbook) Editable() bool { return true }

func (w *Workbook) HistoryState() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.historyState
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

var _ document.SpreadsheetRecalculateEditor = (*Workbook)(nil)
var _ document.SpreadsheetFormulaEditor = (*Workbook)(nil)
