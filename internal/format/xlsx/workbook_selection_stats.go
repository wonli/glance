package xlsx

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetSelectionStatsReader = (*Workbook)(nil)

// SelectionStats computes Count from the selected rectangle's dimensions and
// streams occupied cells for numeric statistics. It doesn't allocate a dense
// matrix for whole-row, whole-column, or sheet selections.
func (w *Workbook) SelectionStats(ctx context.Context, sheet string, bounds document.CellRange) (document.SpreadsheetSelectionStats, error) {
	var stats document.SpreadsheetSelectionStats
	if ctx == nil {
		return stats, fmt.Errorf("selection statistics context is nil")
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return stats, err
	}
	if err := validateRange(sheet, bounds, w.file.GetSheetList()); err != nil {
		return stats, err
	}
	stats.Count = int64(bounds.EndRow-bounds.StartRow+1) * int64(bounds.EndColumn-bounds.StartColumn+1)
	worksheetXML, err := w.worksheetXMLLocked(sheet)
	if err != nil {
		return stats, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(worksheetXML))
	for {
		if err := ctx.Err(); err != nil {
			return document.SpreadsheetSelectionStats{}, err
		}
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return document.SpreadsheetSelectionStats{}, fmt.Errorf("read selection statistics: %w", tokenErr)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "c" {
			continue
		}
		var cell worksheetNavigationCell
		if err := decoder.DecodeElement(&cell, &start); err != nil {
			return document.SpreadsheetSelectionStats{}, fmt.Errorf("decode selection cell: %w", err)
		}
		if !cell.occupied() {
			continue
		}
		column, row, coordinateErr := excelize.CellNameToCoordinates(cell.Reference)
		if coordinateErr != nil || row < bounds.StartRow || row > bounds.EndRow || column < bounds.StartColumn || column > bounds.EndColumn {
			continue
		}
		number, numeric := w.selectionCellNumberLocked(ctx, sheet, cell)
		if err := ctx.Err(); err != nil {
			return document.SpreadsheetSelectionStats{}, err
		}
		if !numeric {
			continue
		}
		next := stats.Sum + number
		if math.IsInf(next, 0) || math.IsNaN(next) {
			return document.SpreadsheetSelectionStats{}, fmt.Errorf("selection sum is outside the supported numeric range")
		}
		stats.Sum = next
		stats.NumericCount++
	}
	return stats, nil
}

func (w *Workbook) selectionCellNumberLocked(ctx context.Context, sheet string, cell worksheetNavigationCell) (float64, bool) {
	if err := ctx.Err(); err != nil {
		return 0, false
	}
	if cell.Formula == nil && cell.Type != "" && cell.Type != "n" {
		return 0, false
	}
	if cell.Formula != nil && (cell.Type == "b" || cell.Type == "e" || cell.Type == "s" || cell.Type == "inlineStr") {
		return 0, false
	}
	if cell.Formula != nil && cell.Type == "str" {
		if cell.Value != "" {
			return 0, false
		}
		formula, err := w.file.GetCellFormula(sheet, cell.Reference)
		if err != nil || spreadsheetFormulaIsStringLiteral(formula) {
			return 0, false
		}
	}
	value, err := w.file.GetCellValue(sheet, cell.Reference, excelize.Options{RawCellValue: true})
	if err != nil {
		return 0, false
	}
	if cell.Formula != nil {
		calculated, calcErr := w.file.CalcCellValue(sheet, cell.Reference)
		if calcErr == nil {
			value = calculated
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func spreadsheetFormulaIsStringLiteral(formula string) bool {
	formula = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(formula), "="))
	return len(formula) >= 2 && formula[0] == '"' && formula[len(formula)-1] == '"'
}
