package xlsx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

// VisitDimensionCells streams the populated cells in a row or column without
// expanding the full worksheet into a dense grid. Merged cells are excluded:
// Excel does not use their text for row-height or column-width AutoFit.
func (w *Workbook) VisitDimensionCells(sheet string, axis document.SpreadsheetDimensionAxis, index int, visit func(document.SpreadsheetDimensionCell) error) error {
	return w.VisitDimensionRangeCells(sheet, axis, index, index, visit)
}

// VisitDimensionRangeCells streams populated cells for an inclusive range of
// rows or columns without expanding the full worksheet into a dense grid.
func (w *Workbook) VisitDimensionRangeCells(sheet string, axis document.SpreadsheetDimensionAxis, start, end int, visit func(document.SpreadsheetDimensionCell) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if visit == nil {
		return fmt.Errorf("dimension cell visitor is nil")
	}
	switch axis {
	case document.SpreadsheetDimensionRow:
		if start < 1 || end > maxXLSXRows || start > end {
			return fmt.Errorf("row range [%d,%d] outside [1,%d]", start, end, maxXLSXRows)
		}
	case document.SpreadsheetDimensionColumn:
		if start < 1 || end > maxXLSXColumns || start > end {
			return fmt.Errorf("column range [%d,%d] outside [1,%d]", start, end, maxXLSXColumns)
		}
	default:
		return fmt.Errorf("unknown dimension axis %d", axis)
	}

	merged, err := w.file.GetMergeCells(sheet, true)
	if err != nil {
		return fmt.Errorf("read merged ranges: %w", err)
	}
	mergedAnchors := make(map[string]struct{}, len(merged))
	for _, region := range merged {
		mergedAnchors[region.GetStartAxis()] = struct{}{}
	}

	worksheetXML, err := w.worksheetXMLLocked(sheet)
	if err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(worksheetXML))
	styleCache := make(map[int]document.CellStyle)
	columnWidthCache := make(map[int]float64)
	dimensionSizeCache := make(map[int]float64)
	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return fmt.Errorf("read worksheet dimension cells: %w", tokenErr)
		}
		tokenStart, ok := token.(xml.StartElement)
		if !ok || tokenStart.Name.Local != "c" {
			continue
		}
		var cellXML worksheetNavigationCell
		if decodeErr := decoder.DecodeElement(&cellXML, &tokenStart); decodeErr != nil {
			return fmt.Errorf("decode worksheet cell: %w", decodeErr)
		}
		if !cellXML.occupied() {
			continue
		}
		column, row, coordinateErr := excelize.CellNameToCoordinates(cellXML.Reference)
		if coordinateErr != nil {
			continue
		}
		perpendicular := column
		dimensionIndex := row
		if axis == document.SpreadsheetDimensionRow {
			if row < start || row > end {
				continue
			}
		} else {
			if column < start || column > end {
				continue
			}
			perpendicular = row
			dimensionIndex = column
		}
		if _, isMergedAnchor := mergedAnchors[cellXML.Reference]; isMergedAnchor {
			continue
		}

		value, valueErr := w.file.GetCellValue(sheet, cellXML.Reference)
		if valueErr != nil {
			return fmt.Errorf("read cell %s value: %w", cellXML.Reference, valueErr)
		}
		formula, formulaErr := w.file.GetCellFormula(sheet, cellXML.Reference)
		if formulaErr != nil {
			return fmt.Errorf("read cell %s formula: %w", cellXML.Reference, formulaErr)
		}
		if formula != "" {
			if calculated, calcErr := w.file.CalcCellValue(sheet, cellXML.Reference); calcErr == nil {
				value = calculated
			}
		}
		dimensionSize, cachedSize := dimensionSizeCache[dimensionIndex]
		if !cachedSize {
			if axis == document.SpreadsheetDimensionRow {
				dimensionSize, err = w.file.GetRowHeight(sheet, dimensionIndex)
			} else {
				columnName, nameErr := excelize.ColumnNumberToName(dimensionIndex)
				if nameErr != nil {
					return fmt.Errorf("resolve dimension column %d: %w", dimensionIndex, nameErr)
				}
				dimensionSize, err = w.file.GetColWidth(sheet, columnName)
			}
			if err != nil {
				return fmt.Errorf("read dimension %d size: %w", dimensionIndex, err)
			}
			dimensionSizeCache[dimensionIndex] = dimensionSize
		}
		styleID, styleErr := w.file.GetCellStyle(sheet, cellXML.Reference)
		if styleErr != nil {
			return fmt.Errorf("read cell %s style: %w", cellXML.Reference, styleErr)
		}
		style, cached := styleCache[styleID]
		if !cached && styleID != 0 {
			style, styleErr = readCellStyle(w.file, styleID)
			if styleErr != nil {
				return fmt.Errorf("read style %d: %w", styleID, styleErr)
			}
			styleCache[styleID] = style
		}
		columnWidth := 0.0
		if axis == document.SpreadsheetDimensionRow {
			if cachedWidth, ok := columnWidthCache[column]; ok {
				columnWidth = cachedWidth
			} else {
				columnName, nameErr := excelize.ColumnNumberToName(column)
				if nameErr != nil {
					return fmt.Errorf("resolve cell %s column: %w", cellXML.Reference, nameErr)
				}
				width, widthErr := w.file.GetColWidth(sheet, columnName)
				if widthErr != nil {
					return fmt.Errorf("read column %s width: %w", columnName, widthErr)
				}
				columnWidth = width
				columnWidthCache[column] = columnWidth
			}
		}
		entry := document.SpreadsheetDimensionCell{
			DimensionIndex:   dimensionIndex,
			DimensionSize:    dimensionSize,
			Index:            perpendicular,
			Cell:             document.Cell{Value: value, Formula: formula, StyleID: styleID, Style: style},
			ColumnWidthChars: columnWidth,
		}
		if err := visit(entry); err != nil {
			return fmt.Errorf("visit cell %s: %w", cellXML.Reference, err)
		}
	}
	return nil
}
