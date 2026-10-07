package xlsx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"glance/internal/document"
)

var _ document.SpreadsheetPivotTableEditor = (*Workbook)(nil)

// AddPivotTable creates and inserts a structured pivot table into the worksheet.
func (w *Workbook) AddPivotTable(spec document.SpreadsheetPivotTableSpec) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}

	sheet := strings.TrimSpace(spec.Sheet)
	if sheet == "" {
		if w.file.SheetCount <= 0 {
			return errors.New("工作簿中没有可用的工作表")
		}
		sheet = w.file.GetSheetName(0)
	}

	r := spec.DataRange
	if r.StartRow <= 0 {
		r.StartRow = 1
	}
	if r.StartColumn <= 0 {
		r.StartColumn = 1
	}
	if r.EndRow < r.StartRow {
		r.EndRow = r.StartRow + 3
	}
	if r.EndColumn < r.StartColumn {
		r.EndColumn = r.StartColumn + 1
	}

	startCell, err := excelize.CoordinatesToCellName(r.StartColumn, r.StartRow)
	if err != nil {
		return fmt.Errorf("invalid start coordinate: %w", err)
	}
	endCell, err := excelize.CoordinatesToCellName(r.EndColumn, r.EndRow)
	if err != nil {
		return fmt.Errorf("invalid end coordinate: %w", err)
	}
	dataRange := fmt.Sprintf("%s!%s:%s", sheet, startCell, endCell)

	pivotRange := strings.TrimSpace(spec.PivotTableRange)
	if pivotRange == "" {
		pStartCol := r.EndColumn + 2
		pEndCol := pStartCol + 4
		pStart, err := excelize.CoordinatesToCellName(pStartCol, 2)
		if err != nil {
			return fmt.Errorf("invalid pivot start coordinate: %w", err)
		}
		pEnd, err := excelize.CoordinatesToCellName(pEndCol, 15)
		if err != nil {
			return fmt.Errorf("invalid pivot end coordinate: %w", err)
		}
		pivotRange = fmt.Sprintf("%s!%s:%s", sheet, pStart, pEnd)
	} else if !strings.Contains(pivotRange, "!") {
		pivotRange = fmt.Sprintf("%s!%s", sheet, pivotRange)
	}

	// Read or synthesize headers
	headers := make([]string, 0, r.EndColumn-r.StartColumn+1)
	for col := r.StartColumn; col <= r.EndColumn; col++ {
		cellRef, _ := excelize.CoordinatesToCellName(col, r.StartRow)
		val, _ := w.file.GetCellValue(sheet, cellRef)
		val = strings.TrimSpace(val)
		if val == "" {
			val = fmt.Sprintf("字段%d", col-r.StartColumn+1)
			_ = w.file.SetCellValue(sheet, cellRef, val)
		}
		headers = append(headers, val)
	}

	rowFields := spec.Rows
	if len(rowFields) == 0 && len(headers) > 0 {
		rowFields = []string{headers[0]}
	}

	dataFields := spec.Data
	if len(dataFields) == 0 && len(headers) > 1 {
		dataFields = []string{headers[len(headers)-1]}
	} else if len(dataFields) == 0 && len(headers) == 1 {
		dataFields = []string{headers[0]}
	}

	colFields := spec.Columns
	if len(colFields) == 0 && len(headers) > 2 {
		colFields = []string{headers[1]}
	}

	rows := make([]excelize.PivotTableField, 0, len(rowFields))
	for _, rf := range rowFields {
		rows = append(rows, excelize.PivotTableField{Data: rf, DefaultSubtotal: true})
	}

	cols := make([]excelize.PivotTableField, 0, len(colFields))
	for _, cf := range colFields {
		cols = append(cols, excelize.PivotTableField{Data: cf, DefaultSubtotal: true})
	}

	data := make([]excelize.PivotTableField, 0, len(dataFields))
	for _, df := range dataFields {
		data = append(data, excelize.PivotTableField{Data: df, Name: df + " 求和", Subtotal: "Sum"})
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	opts := &excelize.PivotTableOptions{
		DataRange:           dataRange,
		PivotTableRange:     pivotRange,
		Rows:                rows,
		Columns:             cols,
		Data:                data,
		RowGrandTotals:      true,
		ColGrandTotals:      true,
		ShowDrill:           true,
		ShowRowHeaders:      true,
		ShowColHeaders:      true,
		ShowLastColumn:      true,
		PivotTableStyleName: "PivotStyleMedium9",
	}

	if err := w.file.AddPivotTable(opts); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("add pivot table to sheet %q: %w", sheet, err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
