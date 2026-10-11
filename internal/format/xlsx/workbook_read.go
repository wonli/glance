package xlsx

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

func CellName(column, row int) string {
	name, err := excelize.CoordinatesToCellName(column, row)
	if err != nil {
		return ""
	}
	return name
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
	value, err := w.file.GetCellValue(sheet, cell)
	if err != nil {
		return "", err
	}
	formula, err := w.file.GetCellFormula(sheet, cell)
	if err != nil {
		return "", err
	}
	if formula != "" {
		if calculated, calcErr := w.file.CalcCellValue(sheet, cell); calcErr == nil {
			value = calculated
		}
	}
	return value, nil
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
			formula, err := w.file.GetCellFormula(sheet, ref)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("read cell %s formula: %w", ref, err)
			}
			if formula != "" {
				if calculated, calcErr := w.file.CalcCellValue(sheet, ref); calcErr == nil {
					value = calculated
				}
			}
			previewRow[c] = document.Cell{Value: value, Formula: formula, StyleID: styleID}
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

// Window reads one bounded, one-based worksheet rectangle without depending
// on the worksheet's used range. This allows the UI to inspect blank cells in
// the full legal XLSX address space while allocating only the requested area.
func (w *Workbook) Window(sheet string, startRow, startColumn, rowCount, columnCount int) (document.SheetWindow, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.SheetWindow{}, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return document.SheetWindow{}, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if startRow < 1 || startColumn < 1 || rowCount < 1 || columnCount < 1 {
		return document.SheetWindow{}, errors.New("worksheet window coordinates and sizes must be positive")
	}
	if startRow > maxXLSXRows || rowCount > maxXLSXRows-startRow+1 {
		return document.SheetWindow{}, fmt.Errorf("worksheet window exceeds the %d-row XLSX limit", maxXLSXRows)
	}
	if startColumn > maxXLSXColumns || columnCount > maxXLSXColumns-startColumn+1 {
		return document.SheetWindow{}, fmt.Errorf("worksheet window exceeds the %d-column XLSX limit", maxXLSXColumns)
	}

	window := document.SheetWindow{
		StartRow:          startRow,
		StartColumn:       startColumn,
		Cells:             make([][]document.Cell, rowCount),
		RowHeightsPoints:  make([]float64, rowCount),
		ColumnWidthsChars: make([]float64, columnCount),
	}
	styleCache := map[int]document.CellStyle{0: {NumberFormat: "General"}}
	for r := 0; r < rowCount; r++ {
		row := startRow + r
		window.Cells[r] = make([]document.Cell, columnCount)
		height, err := w.file.GetRowHeight(sheet, row)
		if err != nil {
			return document.SheetWindow{}, fmt.Errorf("read row %d height: %w", row, err)
		}
		window.RowHeightsPoints[r] = height
		for c := 0; c < columnCount; c++ {
			column := startColumn + c
			cellRef, err := excelize.CoordinatesToCellName(column, row)
			if err != nil {
				return document.SheetWindow{}, err
			}
			value, err := w.file.GetCellValue(sheet, cellRef)
			if err != nil {
				return document.SheetWindow{}, fmt.Errorf("read cell %s: %w", cellRef, err)
			}
			formula, err := w.file.GetCellFormula(sheet, cellRef)
			if err != nil {
				return document.SheetWindow{}, fmt.Errorf("read cell %s formula: %w", cellRef, err)
			}
			if formula != "" {
				if calculated, calcErr := w.file.CalcCellValue(sheet, cellRef); calcErr == nil {
					value = calculated
				}
			}
			styleID, err := w.file.GetCellStyle(sheet, cellRef)
			if err != nil {
				return document.SheetWindow{}, fmt.Errorf("read cell %s style: %w", cellRef, err)
			}
			style, ok := styleCache[styleID]
			if !ok && styleID != 0 {
				style, err = readCellStyle(w.file, styleID)
				if err != nil {
					return document.SheetWindow{}, fmt.Errorf("read style %d: %w", styleID, err)
				}
				styleCache[styleID] = style
			}
			window.Cells[r][c] = document.Cell{Value: value, Formula: formula, StyleID: styleID, Style: style}
		}
	}
	for c := 0; c < columnCount; c++ {
		column := startColumn + c
		name, err := excelize.ColumnNumberToName(column)
		if err != nil {
			return document.SheetWindow{}, err
		}
		width, err := w.file.GetColWidth(sheet, name)
		if err != nil {
			return document.SheetWindow{}, fmt.Errorf("read column %s width: %w", name, err)
		}
		window.ColumnWidthsChars[c] = width
	}

	windowEndRow := startRow + rowCount - 1
	windowEndColumn := startColumn + columnCount - 1
	merges, err := w.file.GetMergeCells(sheet, true)
	if err != nil {
		return document.SheetWindow{}, fmt.Errorf("read merged ranges: %w", err)
	}
	for _, merge := range merges {
		anchorRef, endRef := merge.GetStartAxis(), merge.GetEndAxis()
		anchorColumn, anchorRow, err := excelize.CellNameToCoordinates(anchorRef)
		if err != nil {
			return document.SheetWindow{}, fmt.Errorf("parse merge anchor %q: %w", anchorRef, err)
		}
		endColumn, endRow, err := excelize.CellNameToCoordinates(endRef)
		if err != nil {
			return document.SheetWindow{}, fmt.Errorf("parse merge end %q: %w", endRef, err)
		}
		if anchorRow > windowEndRow || endRow < startRow || anchorColumn > windowEndColumn || endColumn < startColumn {
			continue
		}
		value, err := w.file.GetCellValue(sheet, anchorRef)
		if err != nil {
			return document.SheetWindow{}, fmt.Errorf("read merged anchor %s: %w", anchorRef, err)
		}
		formula, err := w.file.GetCellFormula(sheet, anchorRef)
		if err != nil {
			return document.SheetWindow{}, fmt.Errorf("read merged anchor %s formula: %w", anchorRef, err)
		}
		if formula != "" {
			if calculated, calcErr := w.file.CalcCellValue(sheet, anchorRef); calcErr == nil {
				value = calculated
			}
		}
		styleID, err := w.file.GetCellStyle(sheet, anchorRef)
		if err != nil {
			return document.SheetWindow{}, fmt.Errorf("read merged anchor %s style: %w", anchorRef, err)
		}
		style, ok := styleCache[styleID]
		if !ok && styleID != 0 {
			style, err = readCellStyle(w.file, styleID)
			if err != nil {
				return document.SheetWindow{}, fmt.Errorf("read merged anchor style %d: %w", styleID, err)
			}
			styleCache[styleID] = style
		}
		window.Merges = append(window.Merges, document.MergeRegion{
			Range:  document.CellRange{StartRow: anchorRow, StartColumn: anchorColumn, EndRow: endRow, EndColumn: endColumn},
			Anchor: document.Cell{Value: value, Formula: formula, StyleID: styleID, Style: style},
		})
	}
	return window, nil
}

func readCellStyle(file *excelize.File, styleID int) (document.CellStyle, error) {
	style, err := file.GetStyle(styleID)
	if err != nil {
		return document.CellStyle{}, err
	}
	result := document.CellStyle{}
	if len(style.Fill.Color) > 0 {
		result.FillColor = normalizeRGBColor(style.Fill.Color[0])
	}
	if style.Font != nil {
		result.FontColor = normalizeRGBColor(style.Font.Color)
		result.FontFamily = style.Font.Family
		result.Bold = style.Font.Bold
		result.Italic = style.Font.Italic
		result.Underline = style.Font.Underline != ""
		result.FontSize = style.Font.Size
	}
	result.NumberFormat = cellNumberFormat(style)
	if style.Alignment != nil {
		result.HorizontalAlignment = style.Alignment.Horizontal
		result.VerticalAlignment = style.Alignment.Vertical
		result.WrapText = style.Alignment.WrapText
	}
	for _, border := range style.Border {
		index := -1
		switch strings.ToLower(border.Type) {
		case "top":
			index = 0
		case "right":
			index = 1
		case "bottom":
			index = 2
		case "left":
			index = 3
		}
		if index >= 0 {
			result.Borders[index] = document.CellBorder{Color: normalizeRGBColor(border.Color), Style: border.Style}
		}
	}
	return result, nil
}

func normalizeRGBColor(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
	if len(value) == 8 { // Excel ARGB colors store alpha in the first byte.
		value = value[2:]
	}
	if len(value) != 6 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return "#" + strings.ToUpper(value)
}
