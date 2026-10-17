package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
	_ document.SpreadsheetSearcher            = (*Document)(nil)
)

// Document wraps a unified spreadsheet workbook with CSV persistence.
type Document struct {
	*xlsx.Workbook
	mu      sync.Mutex
	path    string
	hasBOM  bool
	useCRLF bool
	comma   rune
	closed  bool
}

// Open reads a CSV file from disk into a unified spreadsheet document.
func Open(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open csv %q: %w", path, err)
	}
	doc, err := openFromBytes(data, path)
	if err != nil {
		return nil, fmt.Errorf("open csv %q: %w", path, err)
	}
	return doc, nil
}

// OpenSnapshot restores a CSV document from recovery snapshot bytes.
func OpenSnapshot(payload []byte) (*Document, error) {
	if bytes.HasPrefix(payload, []byte("PK\x03\x04")) {
		wb, err := xlsx.OpenSnapshot(payload)
		if err != nil {
			return nil, err
		}
		return &Document{
			Workbook: wb,
			comma:    ',',
		}, nil
	}
	return openFromBytes(payload, "")
}

// NewBlank creates a clean unsaved CSV document draft.
func NewBlank() *Document {
	wb := xlsx.NewBlank()
	return &Document{
		Workbook: wb,
		comma:    ',',
	}
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

	file := excelize.NewFile()
	_ = file.SetDefaultFont("Aptos Narrow")
	sheet := file.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}

	type formulaRef struct {
		col     int
		formula string
	}

	for r, record := range records {
		if len(record) == 0 {
			continue
		}
		rowValues := make([]any, len(record))
		var formulas []formulaRef

		for c, field := range record {
			trimmed := strings.TrimSpace(field)
			if strings.HasPrefix(trimmed, "=") && len(trimmed) > 1 {
				formulas = append(formulas, formulaRef{col: c + 1, formula: strings.TrimPrefix(trimmed, "=")})
				rowValues[c] = ""
			} else if strings.EqualFold(trimmed, "true") {
				rowValues[c] = true
			} else if strings.EqualFold(trimmed, "false") {
				rowValues[c] = false
			} else if len(trimmed) > 1 && trimmed[0] == '0' && trimmed[1] >= '0' && trimmed[1] <= '9' {
				// Preserve string representation for numbers with leading zeros (IDs, postal codes)
				rowValues[c] = field
			} else if num, parseErr := strconv.ParseFloat(trimmed, 64); parseErr == nil && !math.IsNaN(num) && !math.IsInf(num, 0) {
				rowValues[c] = num
			} else {
				rowValues[c] = field
			}
		}

		cellRef, refErr := excelize.CoordinatesToCellName(1, r+1)
		if refErr == nil {
			_ = file.SetSheetRow(sheet, cellRef, &rowValues)
		}
		for _, f := range formulas {
			fRef, fErr := excelize.CoordinatesToCellName(f.col, r+1)
			if fErr == nil {
				_ = file.SetCellFormula(sheet, fRef, f.formula)
			}
		}
	}

	wb := xlsx.NewFromExcelize(file, path)
	wb.SetDirty(false)
	return &Document{
		Workbook: wb,
		path:     path,
		hasBOM:   hasBOM,
		useCRLF:  useCRLF,
		comma:    comma,
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
	d.Workbook.SetSourcePath(path)
}

// Dirty reports whether the document has unsaved modifications.
func (d *Document) Dirty() bool {
	return d.Workbook.Dirty()
}

// SetDirty sets the document's dirty flag.
func (d *Document) SetDirty(dirty bool) {
	d.Workbook.SetDirty(dirty)
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
	d.Workbook.RebaseHistory()
	d.Workbook.SetDirty(false)
	return nil
}

// SaveAs writes an independent copy of the document to path.
func (d *Document) SaveAs(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return d.Workbook.SaveAs(path)
	}
	return d.saveToLocked(path)
}

// SaveAsDocument writes to path and adopts it as the new source path.
func (d *Document) SaveAsDocument(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.EqualFold(filepath.Ext(path), ".xlsx") {
		if err := d.Workbook.SaveAsDocument(path); err != nil {
			return err
		}
		d.path = path
		return nil
	}
	if err := d.saveToLocked(path); err != nil {
		return err
	}
	d.path = path
	d.Workbook.SetSourcePath(path)
	d.Workbook.RebaseHistory()
	d.Workbook.SetDirty(false)
	return nil
}

func (d *Document) saveToLocked(path string) error {
	sheet := "Sheet1"
	sheets := d.Workbook.SheetNames()
	if len(sheets) > 0 {
		sheet = sheets[0]
	}
	rows, err := d.Workbook.SheetRows(sheet)
	if err != nil {
		return fmt.Errorf("read rows from worksheet: %w", err)
	}
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	for i := range rows {
		if len(rows[i]) < maxCols {
			padded := make([]string, maxCols)
			copy(padded, rows[i])
			rows[i] = padded
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".glance-*.csv")
	if err != nil {
		return fmt.Errorf("create temporary csv: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if d.hasBOM {
		if _, err := tmp.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	w := csv.NewWriter(tmp)
	if d.comma != 0 {
		w.Comma = d.comma
	}
	w.UseCRLF = d.useCRLF
	if err := w.WriteAll(rows); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write csv data: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return fileutil.ReplaceFile(tmpPath, path)
}

// Snapshot serializes the document to CSV bytes for session recovery.
func (d *Document) Snapshot() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	sheet := "Sheet1"
	sheets := d.Workbook.SheetNames()
	if len(sheets) > 0 {
		sheet = sheets[0]
	}
	rows, err := d.Workbook.SheetRows(sheet)
	if err != nil {
		return nil, err
	}
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	for i := range rows {
		if len(rows[i]) < maxCols {
			padded := make([]string, maxCols)
			copy(padded, rows[i])
			rows[i] = padded
		}
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
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Close closes the underlying workbook resources.
func (d *Document) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.Workbook.Close()
}
