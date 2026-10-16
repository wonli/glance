package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
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

func documentCellValueKind(cellType excelize.CellType) document.CellValueKind {
	switch cellType {
	case excelize.CellTypeInlineString, excelize.CellTypeSharedString:
		return document.CellValueText
	case excelize.CellTypeNumber, excelize.CellTypeDate:
		return document.CellValueNumber
	case excelize.CellTypeBool:
		return document.CellValueBoolean
	default:
		return ""
	}
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
	if !contains(w.file.GetSheetList(), sheet) {
		return 0, 0, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	worksheetXML, err := w.worksheetXMLLocked(sheet)
	if err != nil {
		return 0, 0, err
	}
	maxColumn, maxRow = 1, 1
	decoder := xml.NewDecoder(bytes.NewReader(worksheetXML))
	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return 0, 0, fmt.Errorf("read worksheet dimensions: %w", tokenErr)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "dimension" {
			for _, attribute := range start.Attr {
				if attribute.Name.Local != "ref" {
					continue
				}
				bounds := strings.Split(attribute.Value, ":")
				end := bounds[len(bounds)-1]
				column, row, coordinateErr := excelize.CellNameToCoordinates(end)
				if coordinateErr != nil {
					continue
				}
				if column > maxColumn {
					maxColumn = column
				}
				if row > maxRow {
					maxRow = row
				}
			}
		}
		if start.Name.Local != "c" {
			continue
		}
		var cell worksheetNavigationCell
		if decodeErr := decoder.DecodeElement(&cell, &start); decodeErr != nil {
			return 0, 0, fmt.Errorf("decode worksheet cell dimension: %w", decodeErr)
		}
		if !cell.used() {
			continue
		}
		column, row, coordinateErr := excelize.CellNameToCoordinates(cell.Reference)
		if coordinateErr != nil {
			continue
		}
		if column > maxColumn {
			maxColumn = column
		}
		if row > maxRow {
			maxRow = row
		}
	}
	return maxColumn, maxRow, nil
}

type dataRegionCacheKey struct {
	sheet      string
	horizontal bool
	line       int
}

// DataRegionEdge finds the next non-empty cell run in the requested direction.
// It reads the current in-memory XLSX package so formula cells count as data
// even when their cached result is an empty string. Callers should run this
// operation off the UI frame loop for large workbooks.
func (w *Workbook) DataRegionEdge(sheet string, current document.CellCoordinate, direction document.SpreadsheetDirection) (document.CellCoordinate, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	positions, err := w.dataRegionPositionsLocked(sheet, current, direction)
	if err != nil {
		return document.CellCoordinate{}, err
	}
	var target int
	switch direction {
	case document.SpreadsheetDirectionLeft:
		target = previousDataRegionEdge(positions, current.Column)
		if target == 0 {
			target = 1
		}
		return document.CellCoordinate{Row: current.Row, Column: target}, nil
	case document.SpreadsheetDirectionRight:
		target = nextDataRegionEdge(positions, current.Column)
		if target == 0 {
			target = maxXLSXColumns
		}
		return document.CellCoordinate{Row: current.Row, Column: target}, nil
	case document.SpreadsheetDirectionUp:
		target = previousDataRegionEdge(positions, current.Row)
		if target == 0 {
			target = 1
		}
		return document.CellCoordinate{Row: target, Column: current.Column}, nil
	default:
		target = nextDataRegionEdge(positions, current.Row)
		if target == 0 {
			target = maxXLSXRows
		}
		return document.CellCoordinate{Row: target, Column: current.Column}, nil
	}
}

// CurrentDataRegion returns the active cell's region bounded by fully empty
// rows and columns. Blank cells inside a populated rectangle remain part of
// the region, while formulas with empty results still count as occupied. If
// the active cell is outside data, the nearest populated cell on its row or
// column anchors the region.
func (w *Workbook) CurrentDataRegion(sheet string, current document.CellCoordinate) (document.CellRange, document.CellCoordinate, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return document.CellRange{}, document.CellCoordinate{}, false, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return document.CellRange{}, document.CellCoordinate{}, false, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if current.Row < 1 || current.Row > maxXLSXRows || current.Column < 1 || current.Column > maxXLSXColumns {
		return document.CellRange{}, document.CellCoordinate{}, false, fmt.Errorf("active cell %d,%d is outside the XLSX worksheet bounds", current.Row, current.Column)
	}
	data, err := w.worksheetXMLLocked(sheet)
	if err != nil {
		return document.CellRange{}, document.CellCoordinate{}, false, err
	}
	var worksheet struct {
		Rows []struct {
			Cells []worksheetNavigationCell `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(data, &worksheet); err != nil {
		return document.CellRange{}, document.CellCoordinate{}, false, fmt.Errorf("parse worksheet current region: %w", err)
	}
	rowColumns := make(map[int][]int, len(worksheet.Rows))
	columnRows := make(map[int][]int)
	for _, row := range worksheet.Rows {
		for _, cell := range row.Cells {
			if !cell.occupied() {
				continue
			}
			column, rowIndex, err := excelize.CellNameToCoordinates(cell.Reference)
			if err != nil {
				continue
			}
			rowColumns[rowIndex] = append(rowColumns[rowIndex], column)
			columnRows[column] = append(columnRows[column], rowIndex)
		}
	}
	for _, columns := range rowColumns {
		sort.Ints(columns)
	}
	for _, rows := range columnRows {
		sort.Ints(rows)
	}
	rowHasDataInColumns := func(row, start, end int) bool {
		columns := rowColumns[row]
		index := sort.SearchInts(columns, start)
		return index < len(columns) && columns[index] <= end
	}
	columnHasDataInRows := func(column, start, end int) bool {
		rows := columnRows[column]
		index := sort.SearchInts(rows, start)
		return index < len(rows) && rows[index] <= end
	}
	axisDistance := func(a, b int) int {
		if a < b {
			return b - a
		}
		return a - b
	}
	regionFrom := func(seed document.CellCoordinate) (document.CellRange, bool) {
		bounds := document.CellRange{StartRow: seed.Row, EndRow: seed.Row, StartColumn: seed.Column, EndColumn: seed.Column}
		for {
			changed := false
			if bounds.StartRow > 1 && rowHasDataInColumns(bounds.StartRow-1, bounds.StartColumn, bounds.EndColumn) {
				bounds.StartRow--
				changed = true
			}
			if bounds.EndRow < maxXLSXRows && rowHasDataInColumns(bounds.EndRow+1, bounds.StartColumn, bounds.EndColumn) {
				bounds.EndRow++
				changed = true
			}
			if bounds.StartColumn > 1 && columnHasDataInRows(bounds.StartColumn-1, bounds.StartRow, bounds.EndRow) {
				bounds.StartColumn--
				changed = true
			}
			if bounds.EndColumn < maxXLSXColumns && columnHasDataInRows(bounds.EndColumn+1, bounds.StartRow, bounds.EndRow) {
				bounds.EndColumn++
				changed = true
			}
			if !changed {
				break
			}
		}
		hasData := false
		for rowIndex := bounds.StartRow; rowIndex <= bounds.EndRow && !hasData; rowIndex++ {
			hasData = rowHasDataInColumns(rowIndex, bounds.StartColumn, bounds.EndColumn)
		}
		return bounds, hasData
	}
	if bounds, hasData := regionFrom(current); hasData {
		return bounds, current, true, nil
	}
	nearest := func(positions []int, target int) (int, bool) {
		index := sort.SearchInts(positions, target)
		best, bestDistance := 0, int(^uint(0)>>1)
		if index < len(positions) && axisDistance(positions[index], target) < bestDistance {
			best, bestDistance = positions[index], axisDistance(positions[index], target)
		}
		if index > 0 && axisDistance(positions[index-1], target) < bestDistance {
			best, bestDistance = positions[index-1], axisDistance(positions[index-1], target)
		}
		return best, bestDistance != int(^uint(0)>>1)
	}
	rowCandidate, rowFound := nearest(rowColumns[current.Row], current.Column)
	columnCandidate, columnFound := nearest(columnRows[current.Column], current.Row)
	seed := document.CellCoordinate{}
	if rowFound && (!columnFound || axisDistance(rowCandidate, current.Column) <= axisDistance(columnCandidate, current.Row)) {
		seed = document.CellCoordinate{Row: current.Row, Column: rowCandidate}
	} else if columnFound {
		seed = document.CellCoordinate{Row: columnCandidate, Column: current.Column}
	}
	if seed.Row == 0 {
		return document.CellRange{StartRow: current.Row, EndRow: current.Row, StartColumn: current.Column, EndColumn: current.Column}, current, false, nil
	}
	bounds, hasData := regionFrom(seed)
	return bounds, seed, hasData, nil
}

// NextNonEmptyCell moves to the first occupied cell in a direction. It is the
// target behavior for Excel's one-shot End mode, distinct from Ctrl+Arrow's
// move to the edge of a contiguous data region.
func (w *Workbook) NextNonEmptyCell(sheet string, current document.CellCoordinate, direction document.SpreadsheetDirection) (document.CellCoordinate, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	positions, err := w.dataRegionPositionsLocked(sheet, current, direction)
	if err != nil {
		return document.CellCoordinate{}, err
	}
	var target int
	switch direction {
	case document.SpreadsheetDirectionLeft:
		target = nextNonEmptyPosition(positions, current.Column, false, 1)
		return document.CellCoordinate{Row: current.Row, Column: target}, nil
	case document.SpreadsheetDirectionRight:
		target = nextNonEmptyPosition(positions, current.Column, true, maxXLSXColumns)
		return document.CellCoordinate{Row: current.Row, Column: target}, nil
	case document.SpreadsheetDirectionUp:
		target = nextNonEmptyPosition(positions, current.Row, false, 1)
		return document.CellCoordinate{Row: target, Column: current.Column}, nil
	default:
		target = nextNonEmptyPosition(positions, current.Row, true, maxXLSXRows)
		return document.CellCoordinate{Row: target, Column: current.Column}, nil
	}
}

func (w *Workbook) dataRegionPositionsLocked(sheet string, current document.CellCoordinate, direction document.SpreadsheetDirection) ([]int, error) {
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return nil, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if current.Row < 1 || current.Row > maxXLSXRows || current.Column < 1 || current.Column > maxXLSXColumns {
		return nil, fmt.Errorf("active cell %d,%d is outside the XLSX worksheet bounds", current.Row, current.Column)
	}
	if direction < document.SpreadsheetDirectionUp || direction > document.SpreadsheetDirectionRight {
		return nil, fmt.Errorf("invalid spreadsheet direction %d", direction)
	}
	horizontal := direction == document.SpreadsheetDirectionLeft || direction == document.SpreadsheetDirectionRight
	line := current.Row
	if !horizontal {
		line = current.Column
	}
	cacheKey := dataRegionCacheKey{sheet: sheet, horizontal: horizontal, line: line}
	if !w.dataRegionCacheValid || w.dataRegionCacheKey != cacheKey || w.dataRegionCacheState != w.historyState {
		positions, err := w.loadDataRegionPositionsLocked(sheet, horizontal, line)
		if err != nil {
			return nil, fmt.Errorf("scan worksheet data region: %w", err)
		}
		w.dataRegionCacheKey = cacheKey
		w.dataRegionCacheState = w.historyState
		w.dataRegionPositions = positions
		w.dataRegionCacheValid = true
	}
	return w.dataRegionPositions, nil
}

type workbookRelationshipsDocument struct {
	Relationships []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

type workbookSheetsDocument struct {
	Sheets []struct {
		Name           string `xml:"name,attr"`
		RelationshipID string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type worksheetNavigationCell struct {
	Reference    string    `xml:"r,attr"`
	Style        string    `xml:"s,attr"`
	Formula      *struct{} `xml:"f"`
	Value        string    `xml:"v"`
	InlineString *struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

func (c worksheetNavigationCell) occupied() bool {
	if c.Formula != nil || c.Value != "" {
		return true
	}
	if c.InlineString == nil {
		return false
	}
	if c.InlineString.Text != "" {
		return true
	}
	for _, run := range c.InlineString.Runs {
		if run.Text != "" {
			return true
		}
	}
	return false
}

func (c worksheetNavigationCell) used() bool {
	return c.occupied() || c.Style != ""
}

func (w *Workbook) loadDataRegionPositionsLocked(sheet string, horizontal bool, line int) ([]int, error) {
	worksheetXML, err := w.worksheetXMLLocked(sheet)
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(worksheetXML))
	positions := make([]int, 0)
	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return nil, fmt.Errorf("read worksheet cells: %w", tokenErr)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "c" {
			continue
		}
		var cell worksheetNavigationCell
		if err := decoder.DecodeElement(&cell, &start); err != nil {
			return nil, fmt.Errorf("decode worksheet cell: %w", err)
		}
		if !cell.occupied() {
			continue
		}
		column, row, coordinateErr := excelize.CellNameToCoordinates(cell.Reference)
		if coordinateErr != nil {
			continue
		}
		if horizontal && row == line {
			positions = append(positions, column)
		} else if !horizontal && column == line {
			positions = append(positions, row)
		}
	}
	sort.Ints(positions)
	return positions, nil
}

func (w *Workbook) worksheetXMLLocked(sheet string) ([]byte, error) {
	buffer, err := w.file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		return nil, fmt.Errorf("read in-memory XLSX package: %w", err)
	}
	workbookXML, err := readZipMember(archive, "xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	var workbook workbookSheetsDocument
	if err := xml.Unmarshal(workbookXML, &workbook); err != nil {
		return nil, fmt.Errorf("parse workbook sheet list: %w", err)
	}
	relationshipID := ""
	for _, item := range workbook.Sheets {
		if item.Name == sheet {
			relationshipID = item.RelationshipID
			break
		}
	}
	if relationshipID == "" {
		return nil, fmt.Errorf("worksheet %q has no package relationship", sheet)
	}
	relationshipsXML, err := readZipMember(archive, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	var relationships workbookRelationshipsDocument
	if err := xml.Unmarshal(relationshipsXML, &relationships); err != nil {
		return nil, fmt.Errorf("parse workbook relationships: %w", err)
	}
	target := ""
	for _, relationship := range relationships.Relationships {
		if relationship.ID == relationshipID {
			target = relationship.Target
			break
		}
	}
	if target == "" {
		return nil, fmt.Errorf("worksheet %q package target is missing", sheet)
	}
	target = strings.TrimPrefix(target, "/")
	if !strings.HasPrefix(target, "xl/") {
		target = path.Join("xl", target)
	}
	return readZipMember(archive, path.Clean(target))
}

func readZipMember(archive *zip.Reader, name string) ([]byte, error) {
	for _, member := range archive.File {
		if member.Name != name {
			continue
		}
		reader, err := member.Open()
		if err != nil {
			return nil, fmt.Errorf("open XLSX package member %q: %w", name, err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read XLSX package member %q: %w", name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close XLSX package member %q: %w", name, closeErr)
		}
		return data, nil
	}
	return nil, fmt.Errorf("XLSX package member %q is missing", name)
}

func nextDataRegionEdge(positions []int, current int) int {
	index := sort.Search(len(positions), func(index int) bool { return positions[index] > current })
	if index == len(positions) {
		return 0
	}
	edge := positions[index]
	for index++; index < len(positions) && positions[index] == edge+1; index++ {
		edge = positions[index]
	}
	return edge
}

func nextNonEmptyPosition(positions []int, current int, forward bool, worksheetEdge int) int {
	if forward {
		index := sort.Search(len(positions), func(index int) bool { return positions[index] > current })
		if index < len(positions) {
			return positions[index]
		}
		return worksheetEdge
	}
	index := sort.Search(len(positions), func(index int) bool { return positions[index] >= current }) - 1
	if index >= 0 {
		return positions[index]
	}
	return worksheetEdge
}

func previousDataRegionEdge(positions []int, current int) int {
	index := sort.Search(len(positions), func(index int) bool { return positions[index] >= current }) - 1
	if index < 0 {
		return 0
	}
	edge := positions[index]
	for index--; index >= 0 && positions[index] == edge-1; index-- {
		edge = positions[index]
	}
	return edge
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

func (w *Workbook) StoredCellValueKind(sheet, cell string) (document.CellValueKind, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return "", err
	}
	cellType, err := w.file.GetCellType(sheet, cell)
	if err != nil {
		return "", err
	}
	return documentCellValueKind(cellType), nil
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
	if panes, err := w.file.GetPanes(sheet); err != nil {
		return document.SheetWindow{}, fmt.Errorf("read panes for worksheet %q: %w", sheet, err)
	} else {
		window.Panes = document.SpreadsheetPaneFreeze{Frozen: panes.Freeze, XSplit: panes.XSplit, YSplit: panes.YSplit}
	}
	defaultCellStyle := document.CellStyle{NumberFormat: "General"}
	if style, err := readCellStyle(w.file, 0); err == nil {
		defaultCellStyle = style
		if defaultCellStyle.NumberFormat == "" {
			defaultCellStyle.NumberFormat = "General"
		}
	}
	styleCache := map[int]document.CellStyle{0: defaultCellStyle}
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
	w.applyConditionalFormatDisplayLocked(sheet, &window)
	return window, nil
}

func readCellStyle(file *excelize.File, styleID int) (document.CellStyle, error) {
	style, err := file.GetStyle(styleID)
	if err != nil {
		return document.CellStyle{}, err
	}
	return neutralCellStyle(style), nil
}

func neutralCellStyle(style *excelize.Style) document.CellStyle {
	result := document.CellStyle{}
	if style == nil {
		return result
	}
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
	return result
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
