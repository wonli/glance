package xls

import (
	"fmt"
	"os"
	"strings"

	legacy "github.com/extrame/xls"

	"glance/internal/document"
)

const (
	previewRows    = 300
	previewColumns = 16
)

// Reader provides a bounded, read-only preview for legacy BIFF .xls files.
// The selected Go library exposes rows across sheets as one flattened result,
// so this first adapter intentionally exposes only the first worksheet.
type Reader struct {
	sheet string
	rows  [][]document.Cell
}

var _ document.Spreadsheet = (*Reader)(nil)

func Open(path string) (*Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open legacy xls %q: %w", path, err)
	}
	book, err := legacy.OpenReader(file, "utf-8")
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("parse legacy xls %q: %w", path, err)
	}
	if book.NumSheets() == 0 {
		_ = file.Close()
		return nil, fmt.Errorf("legacy workbook contains no worksheets")
	}
	sheet := book.GetSheet(0)
	rowCount := int(sheet.MaxRow) + 1
	if rowCount < 1 {
		rowCount = 1
	}
	if rowCount > previewRows {
		rowCount = previewRows
	}
	legacyRows := book.ReadAllCells(rowCount)
	_ = file.Close()
	rows := make([][]document.Cell, len(legacyRows))
	for r, sourceRow := range legacyRows {
		if len(sourceRow) > previewColumns {
			sourceRow = sourceRow[:previewColumns]
		}
		rows[r] = make([]document.Cell, previewColumns)
		for c, value := range sourceRow {
			rows[r][c] = document.Cell{Value: value}
		}
	}
	if len(rows) > rowCount {
		rows = rows[:rowCount]
	}
	return &Reader{sheet: strings.TrimSpace(sheet.Name), rows: rows}, nil
}

func (r *Reader) SheetNames() []string {
	if r.sheet == "" {
		return nil
	}
	return []string{r.sheet}
}

func (r *Reader) Range(sheet string, maxRows, maxColumns int) ([][]document.Cell, error) {
	if sheet != r.sheet {
		return nil, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if maxRows < 1 || maxColumns < 1 {
		return nil, fmt.Errorf("range limits must be positive")
	}
	if maxRows > len(r.rows) {
		maxRows = len(r.rows)
	}
	if maxColumns > previewColumns {
		maxColumns = previewColumns
	}
	result := make([][]document.Cell, maxRows)
	for row := 0; row < maxRows; row++ {
		result[row] = append([]document.Cell(nil), r.rows[row][:maxColumns]...)
	}
	return result, nil
}

func (r *Reader) SetCellText(string, string, string) error { return document.ErrReadOnly }
func (r *Reader) Save() error                              { return document.ErrReadOnly }
func (r *Reader) Dirty() bool                              { return false }
func (r *Reader) Editable() bool                           { return false }
func (r *Reader) Close() error                             { return nil }
