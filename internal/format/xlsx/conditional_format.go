package xlsx

import (
	"fmt"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetConditionalFormatEditor = (*Workbook)(nil)

// SetConditionalFormat applies preset conditional formatting rules to the given cell range.
func (w *Workbook) SetConditionalFormat(spec document.SpreadsheetConditionalFormatSpec) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	sheet := spec.Sheet
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}
	if spec.Range.StartColumn <= 0 || spec.Range.StartRow <= 0 {
		return fmt.Errorf("invalid range for conditional format: %v", spec.Range)
	}

	startCell := CellName(spec.Range.StartColumn, spec.Range.StartRow)
	endCell := CellName(spec.Range.EndColumn, spec.Range.EndRow)
	if startCell == "" || endCell == "" {
		return fmt.Errorf("invalid range bounds: col %d..%d row %d..%d",
			spec.Range.StartColumn, spec.Range.EndColumn, spec.Range.StartRow, spec.Range.EndRow)
	}
	rangeRef := fmt.Sprintf("%s:%s", startCell, endCell)

	var opts []excelize.ConditionalFormatOptions
	switch spec.Type {
	case document.ConditionalFormatDataBar:
		opts = []excelize.ConditionalFormatOptions{
			{
				Type:     "data_bar",
				Criteria: "=",
				MinType:  "min",
				MaxType:  "max",
				BarColor: "#638EC6",
			},
		}
	case document.ConditionalFormat3ColorScale:
		opts = []excelize.ConditionalFormatOptions{
			{
				Type:     "3_color_scale",
				Criteria: "=",
				MinType:  "min",
				MidType:  "percentile",
				MaxType:  "max",
				MinColor: "#F8696B",
				MidColor: "#FFEB84",
				MaxColor: "#63BE7B",
			},
		}
	case document.ConditionalFormatDuplicate:
		styleID, err := w.file.NewStyle(&excelize.Style{
			Font: &excelize.Font{Color: "9C0006"},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"FFC7CE"}, Pattern: 1},
		})
		if err != nil {
			return fmt.Errorf("create duplicate highlight style: %w", err)
		}
		opts = []excelize.ConditionalFormatOptions{
			{
				Type:     "duplicate",
				Criteria: "=",
				Format:   &styleID,
			},
		}
	case document.ConditionalFormatAboveAvg:
		styleID, err := w.file.NewStyle(&excelize.Style{
			Font: &excelize.Font{Color: "006100"},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"C6EFCE"}, Pattern: 1},
		})
		if err != nil {
			return fmt.Errorf("create above average highlight style: %w", err)
		}
		opts = []excelize.ConditionalFormatOptions{
			{
				Type:         "average",
				Criteria:     "=",
				Format:       &styleID,
				AboveAverage: true,
			},
		}
	default:
		return fmt.Errorf("unsupported conditional format type %q", spec.Type)
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	// Remove any existing conditional formats on this range first
	_ = w.file.UnsetConditionalFormat(sheet, rangeRef)

	if err := w.file.SetConditionalFormat(sheet, rangeRef, opts); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("set conditional format: %w", err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// ClearConditionalFormat clears conditional formatting on the specified range.
func (w *Workbook) ClearConditionalFormat(sheet string, bounds document.CellRange) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}

	startCell := CellName(bounds.StartColumn, bounds.StartRow)
	endCell := CellName(bounds.EndColumn, bounds.EndRow)
	if startCell == "" || endCell == "" {
		return fmt.Errorf("invalid bounds for clearing conditional format")
	}
	rangeRef := fmt.Sprintf("%s:%s", startCell, endCell)

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	if err := w.file.UnsetConditionalFormat(sheet, rangeRef); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("unset conditional format: %w", err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
