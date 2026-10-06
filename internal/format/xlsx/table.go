package xlsx

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"glance/internal/document"
)

var _ document.SpreadsheetTableEditor = (*Workbook)(nil)

// AddTable creates a structured table in the worksheet within the specified range.
func (w *Workbook) AddTable(spec document.SpreadsheetTableSpec) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}

	if spec.Range.StartRow <= 0 || spec.Range.EndRow <= spec.Range.StartRow {
		return errors.New("表格选区至少需要 2 行（包含标题行与数据行）")
	}
	if spec.Range.StartColumn <= 0 || spec.Range.EndColumn < spec.Range.StartColumn {
		return errors.New("无效的表格列范围")
	}
	if spec.Sheet == "" {
		spec.Sheet = w.file.GetSheetName(0)
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	// Ensure header cells in the top row are unique and non-empty strings
	usedHeaders := make(map[string]bool)
	for c := spec.Range.StartColumn; c <= spec.Range.EndColumn; c++ {
		cell := CellName(c, spec.Range.StartRow)
		val, _ := w.file.GetCellValue(spec.Sheet, cell)
		val = strings.TrimSpace(val)
		if val == "" || usedHeaders[val] {
			val = fmt.Sprintf("列%d", c-spec.Range.StartColumn+1)
			suffix := 1
			for usedHeaders[val] {
				val = fmt.Sprintf("列%d_%d", c-spec.Range.StartColumn+1, suffix)
				suffix++
			}
			_ = w.file.SetCellValue(spec.Sheet, cell, val)
		}
		usedHeaders[val] = true
	}

	rangeRef := fmt.Sprintf("%s:%s", CellName(spec.Range.StartColumn, spec.Range.StartRow), CellName(spec.Range.EndColumn, spec.Range.EndRow))
	styleName := spec.StyleName
	if styleName == "" {
		styleName = "TableStyleMedium2"
	}
	name := spec.Name
	if name == "" {
		name = fmt.Sprintf("Table_%d", time.Now().UnixNano()%1000000)
	}

	trueVal := true
	table := &excelize.Table{
		Range:          rangeRef,
		Name:           name,
		StyleName:      styleName,
		ShowRowStripes: &trueVal,
	}

	if err := w.file.AddTable(spec.Sheet, table); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("add table: %w", err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
