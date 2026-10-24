package csv

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"glance/internal/document"
	"glance/internal/fileutil"
	"glance/internal/format/xlsx"

	"github.com/xuri/excelize/v2"
)

var (
	_ document.Spreadsheet                    = (*Document)(nil)
	_ document.GridReader                     = (*Document)(nil)
	_ document.SpreadsheetDimensionReader     = (*Document)(nil)
	_ document.SpreadsheetDimensionCellReader = (*Document)(nil)
	_ document.SpreadsheetDimensionRangeCellReader = (*Document)(nil)
	_ document.SpreadsheetDimensionBatchEditor = (*Document)(nil)
	_ document.SpreadsheetDimensionEditGrouper = (*Document)(nil)
	_ document.SpreadsheetDataRegionReader    = (*Document)(nil)
	_ document.SpreadsheetCurrentDataRegionReader = (*Document)(nil)
	_ document.SpreadsheetNextNonEmptyCellReader = (*Document)(nil)
	_ document.SpreadsheetHistory             = (*Document)(nil)
	_ document.SpreadsheetBatchChangeEditor   = (*Document)(nil)
	_ document.SpreadsheetCellValueEditor     = (*Document)(nil)
	_ document.SpreadsheetRangeReader         = (*Document)(nil)
	_ document.SpreadsheetRangeContentEditor  = (*Document)(nil)
	_ document.SpreadsheetCellTransferEditor  = (*Document)(nil)
	_ document.SpreadsheetSorter              = (*Document)(nil)
	_ document.SpreadsheetSortChangeEditor    = (*Document)(nil)
	_ document.SpreadsheetSearcher            = (*Document)(nil)
)

type cellKey struct {
	row int // 1-based
	col int // 1-based
}

type editAction struct {
	row    int
	col    int
	oldVal string
	newVal string
}

type editBatchAction struct {
	edits []editAction
}

// snapshotMetadata represents a compact workspace recovery record for large disk-backed CSVs.
type snapshotMetadata struct {
	Kind    string       `json:"kind"`
	Source  string       `json:"source"`
	HasBOM  bool         `json:"has_bom"`
	UseCRLF bool         `json:"use_crlf"`
	Comma   rune         `json:"comma"`
	Edits   []editAction `json:"edits,omitempty"`
}

// Document provides high-performance tabular CSV storage implementing the unified spreadsheet interface.
type Document struct {
	mu sync.Mutex

	path    string
	hasBOM  bool
	useCRLF bool
	comma   rune
	closed  bool
	dirty   bool

	// File-backed storage (for files opened from disk)
	file  *os.File
	index *fileIndex

	// In-memory storage (for blank drafts or small byte snapshots)
	records [][]string

	// Active grid extents (1-based)
	maxRow int
	maxCol int

	// Sparse cell modifications overlay: (row, col) -> value
	edits map[cellKey]string

	// Undo / Redo history
	undoStack []editBatchAction
	redoStack []editBatchAction

	// Sizing overrides
	rowHeights map[int]float64
	colWidths  map[int]float64
}

// Open reads a CSV file from disk into a unified spreadsheet document.
// Uses fast line-offset indexing so files with millions of rows open in milliseconds.
func Open(path string) (*Document, error) {
	cleanPath := filepath.Clean(path)
	f, _, err := fileutil.OpenStream(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("open csv %q: %w", path, err)
	}

	idx, err := scanFileIndex(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("index csv %q: %w", path, err)
	}

	maxRow := idx.totalRows
	if maxRow < 1 {
		maxRow = 1
	}
	maxCol := idx.totalCols
	if maxCol < 1 {
		maxCol = 1
	}

	doc := &Document{
		path:       cleanPath,
		hasBOM:     idx.hasBOM,
		useCRLF:    idx.useCRLF,
		comma:      idx.comma,
		file:       f,
		index:      idx,
		maxRow:     maxRow,
		maxCol:     maxCol,
		edits:      make(map[cellKey]string),
		rowHeights: make(map[int]float64),
		colWidths:  make(map[int]float64),
	}
	return doc, nil
}

// OpenSnapshot restores a CSV document from recovery snapshot bytes.
func OpenSnapshot(payload []byte) (*Document, error) {
	if bytes.HasPrefix(payload, []byte("PK\x03\x04")) {
		// Legacy snapshot stored as xlsx package
		wb, err := xlsx.OpenSnapshot(payload)
		if err != nil {
			return nil, err
		}
		rows, err := wb.SheetRows("Sheet1")
		if err != nil {
			_ = wb.Close()
			return nil, err
		}
		_ = wb.Close()
		return newFromRecords(rows, "")
	}

	if bytes.HasPrefix(payload, []byte(`{"kind":"glance-csv-v1"`)) {
		var meta snapshotMetadata
		if err := json.Unmarshal(payload, &meta); err == nil && meta.Source != "" {
			if _, statErr := os.Stat(meta.Source); statErr == nil {
				doc, openErr := Open(meta.Source)
				if openErr == nil {
					for _, e := range meta.Edits {
						doc.edits[cellKey{row: e.row, col: e.col}] = e.newVal
						if e.row > doc.maxRow {
							doc.maxRow = e.row
						}
						if e.col > doc.maxCol {
							doc.maxCol = e.col
						}
					}
					if len(meta.Edits) > 0 {
						doc.dirty = true
					}
					return doc, nil
				}
			}
		}
	}

	return openFromBytes(payload, "")
}

// NewBlank creates a clean unsaved CSV document draft.
func NewBlank() *Document {
	records := make([][]string, 1)
	records[0] = make([]string, 1)
	return &Document{
		comma:      ',',
		records:    records,
		maxRow:     1,
		maxCol:     1,
		edits:      make(map[cellKey]string),
		rowHeights: make(map[int]float64),
		colWidths:  make(map[int]float64),
	}
}

func newFromRecords(records [][]string, path string) (*Document, error) {
	maxRow := len(records)
	maxCol := 0
	for _, r := range records {
		if len(r) > maxCol {
			maxCol = len(r)
		}
	}
	if maxRow < 1 {
		maxRow = 1
	}
	if maxCol < 1 {
		maxCol = 1
	}
	return &Document{
		path:       path,
		comma:      ',',
		records:    records,
		maxRow:     maxRow,
		maxCol:     maxCol,
		edits:      make(map[cellKey]string),
		rowHeights: make(map[int]float64),
		colWidths:  make(map[int]float64),
	}, nil
}

func openFromBytes(data []byte, path string) (*Document, error) {
	hasBOM := false
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		hasBOM = true
		data = data[3:]
	}
	useCRLF := bytes.Contains(data, []byte("\r\n"))
	comma := detectDelimiter(data)

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse csv: %w", err)
	}

	maxRow := len(records)
	maxCol := 0
	for _, r := range records {
		if len(r) > maxCol {
			maxCol = len(r)
		}
	}
	if maxRow < 1 {
		maxRow = 1
	}
	if maxCol < 1 {
		maxCol = 1
	}

	return &Document{
		path:       path,
		hasBOM:     hasBOM,
		useCRLF:    useCRLF,
		comma:      comma,
		records:    records,
		maxRow:     maxRow,
		maxCol:     maxCol,
		edits:      make(map[cellKey]string),
		rowHeights: make(map[int]float64),
		colWidths:  make(map[int]float64),
	}, nil
}

func detectDelimiter(data []byte) rune {
	firstLine := data
	if idx := bytes.IndexAny(data, "\r\n"); idx >= 0 {
		firstLine = data[:idx]
	}
	commas := bytes.Count(firstLine, []byte{','})
	tabs := bytes.Count(firstLine, []byte{'\t'})
	semicolons := bytes.Count(firstLine, []byte{';'})

	if commas > 0 {
		return ','
	}
	if tabs > 0 && tabs > semicolons {
		return '\t'
	}
	if semicolons > 0 {
		return ';'
	}
	return ','
}

// SheetNames returns the single worksheet name for CSV.
func (d *Document) SheetNames() []string {
	return []string{"Sheet1"}
}

// Path returns the current file path of the document.
func (d *Document) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.path
}

// SetSourcePath updates the document's file path.
func (d *Document) SetSourcePath(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.path = path
}

// Dirty reports whether the document has unsaved modifications.
func (d *Document) Dirty() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dirty
}

// SetDirty sets the document's dirty flag.
func (d *Document) SetDirty(dirty bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dirty = dirty
}

// Editable reports whether this document supports user editing.
func (d *Document) Editable() bool {
	return true
}

// Save writes changes back to the source CSV file.
func (d *Document) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.path == "" {
		return errors.New("csv document has no save path")
	}
	if err := d.saveToLocked(d.path); err != nil {
		return err
	}
	d.dirty = false
	return nil
}

// SaveAs writes an independent copy of the document to path.
func (d *Document) SaveAs(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return d.exportToXLSXLocked(path)
	}
	return d.saveToLocked(path)
}

// SaveAsDocument writes to path and adopts it as the new source path.
func (d *Document) SaveAsDocument(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.EqualFold(filepath.Ext(path), ".xlsx") {
		if err := d.exportToXLSXLocked(path); err != nil {
			return err
		}
		d.path = path
		return nil
	}
	if err := d.saveToLocked(path); err != nil {
		return err
	}
	d.path = path
	d.dirty = false
	d.undoStack = nil
	d.redoStack = nil
	return nil
}

func (d *Document) exportToXLSXLocked(path string) error {
	const maxXLSXRows = 1048576
	if d.maxRow > maxXLSXRows {
		return fmt.Errorf("CSV 包含 %d 行，超过了 XLSX 格式上限（1,048,576 行），无法另存为 XLSX", d.maxRow)
	}

	file := excelize.NewFile()
	_ = file.SetDefaultFont("Aptos Narrow")
	sheet := file.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}

	for r := 1; r <= d.maxRow; r++ {
		rowVals := make([]any, d.maxCol)
		hasContent := false
		for c := 1; c <= d.maxCol; c++ {
			val := d.getCellValueLocked(r, c)
			if val != "" {
				rowVals[c-1] = val
				hasContent = true
			} else {
				rowVals[c-1] = ""
			}
		}
		if hasContent {
			cellRef, err := excelize.CoordinatesToCellName(1, r)
			if err == nil {
				_ = file.SetSheetRow(sheet, cellRef, &rowVals)
			}
		}
	}

	return file.SaveAs(path)
}

func (d *Document) saveToLocked(path string) error {
	err := fileutil.AtomicWrite(path, func(w io.Writer) error {
		if d.hasBOM {
			if _, err := w.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
				return err
			}
		}

		writer := csv.NewWriter(w)
		if d.comma != 0 {
			writer.Comma = d.comma
		}
		writer.UseCRLF = d.useCRLF

		// Stream write all rows
		for r := 1; r <= d.maxRow; r++ {
			rowFields := make([]string, d.maxCol)
			for c := 1; c <= d.maxCol; c++ {
				rowFields[c-1] = d.getCellValueLocked(r, c)
			}
			if err := writer.Write(rowFields); err != nil {
				return fmt.Errorf("write csv row %d: %w", r, err)
			}
		}

		writer.Flush()
		return writer.Error()
	})
	if err != nil {
		return err
	}

	// Reopen file handle and re-scan index if path matches
	if filepath.Clean(d.path) == filepath.Clean(path) && d.file != nil {
		_ = d.file.Close()
		newF, err := os.Open(d.path)
		if err == nil {
			newIdx, idxErr := scanFileIndex(newF)
			if idxErr == nil {
				d.file = newF
				d.index = newIdx
				d.edits = make(map[cellKey]string)
			} else {
				_ = newF.Close()
			}
		}
	}

	return nil
}

// Snapshot serializes the document state for workspace recovery.
// For large files (> 2MB) on disk, writes compact recovery metadata.
// For small files or drafts without a path, writes full CSV bytes.
func (d *Document) Snapshot() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Compact snapshot for large files on disk
	if d.file != nil && d.index != nil && d.index.fileSize > 2*1024*1024 && d.path != "" {
		edits := make([]editAction, 0, len(d.edits))
		for k, v := range d.edits {
			edits = append(edits, editAction{row: k.row, col: k.col, newVal: v})
		}
		meta := snapshotMetadata{
			Kind:    "glance-csv-v1",
			Source:  d.path,
			HasBOM:  d.hasBOM,
			UseCRLF: d.useCRLF,
			Comma:   d.comma,
			Edits:   edits,
		}
		return json.Marshal(meta)
	}

	var buf bytes.Buffer
	if d.hasBOM {
		buf.Write([]byte{0xef, 0xbb, 0xbf})
	}
	w := csv.NewWriter(&buf)
	if d.comma != 0 {
		w.Comma = d.comma
	}
	w.UseCRLF = d.useCRLF

	for r := 1; r <= d.maxRow; r++ {
		rowFields := make([]string, d.maxCol)
		for c := 1; c <= d.maxCol; c++ {
			rowFields[c-1] = d.getCellValueLocked(r, c)
		}
		if err := w.Write(rowFields); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Close closes any open file handles and releases memory.
func (d *Document) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	var err error
	if d.file != nil {
		err = d.file.Close()
		d.file = nil
	}
	if d.index != nil {
		d.index.cacheMu.Lock()
		d.index.offsets = nil
		d.index.rowCache = nil
		d.index.rowFIFO = nil
		d.index.cacheMu.Unlock()
		d.index = nil
	}
	d.records = nil
	d.edits = nil
	d.undoStack = nil
	d.redoStack = nil
	d.rowHeights = nil
	d.colWidths = nil
	return err
}
