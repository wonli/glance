package xlsx

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"glance/internal/document"
	"glance/internal/fileutil"

	"github.com/xuri/excelize/v2"
)

// Workbook keeps Excelize behind the format adapter boundary.
type Workbook struct {
	mu                sync.Mutex
	path              string
	file              *excelize.File
	dirty             bool
	closed            bool
	pictureIndexes    map[string]spreadsheetPictureIndex
	undo              []workbookHistoryEntry
	redo              []workbookHistoryEntry
	historyState      uint64
	savedHistoryState uint64
	nextHistoryState  uint64
}

var _ document.Spreadsheet = (*Workbook)(nil)
var _ document.GridReader = (*Workbook)(nil)
var _ document.SpreadsheetHistory = (*Workbook)(nil)
var _ document.SpreadsheetImageReader = (*Workbook)(nil)

const (
	maxXLSXRows    = 1_048_576
	maxXLSXColumns = 16_384
)

func CellName(column, row int) string {
	name, err := excelize.CoordinatesToCellName(column, row)
	if err != nil {
		return ""
	}
	return name
}

func Open(path string) (*Workbook, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx %q: %w", path, err)
	}
	w := &Workbook{path: path, file: f, historyState: 1, savedHistoryState: 1, nextHistoryState: 2}
	return w, nil
}

// OpenSnapshot restores a self-contained XLSX package persisted for recovery.
func OpenSnapshot(payload []byte) (*Workbook, error) {
	f, err := excelize.OpenReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("restore XLSX snapshot: %w", err)
	}
	w := &Workbook{file: f, historyState: 1, savedHistoryState: 1, nextHistoryState: 2}
	return w, nil
}

// NewBlank creates an unsaved workbook draft.
func NewBlank() *Workbook {
	return &Workbook{file: excelize.NewFile(), dirty: true, historyState: 1, nextHistoryState: 2}
}

// Snapshot serializes the current in-memory workbook without changing dirty state.
func (w *Workbook) Snapshot() ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	buf, err := w.file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.Bytes()...), nil
}

func (w *Workbook) SetSourcePath(path string) { w.mu.Lock(); defer w.mu.Unlock(); w.path = path }
func (w *Workbook) SetDirty(dirty bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dirty = dirty
	if !dirty {
		w.savedHistoryState = w.historyState
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
	return w.file.GetCellValue(sheet, cell)
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
			previewRow[c] = document.Cell{Value: value, StyleID: styleID}
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
			window.Cells[r][c] = document.Cell{Value: value, StyleID: styleID, Style: style}
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
			Anchor: document.Cell{Value: value, StyleID: styleID, Style: style},
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

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (w *Workbook) SetRowHeight(sheet string, row int, heightPoints float64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if row < 1 || row > maxXLSXRows {
		return fmt.Errorf("row %d outside [1,%d]", row, maxXLSXRows)
	}
	if !finiteNumber(heightPoints) || heightPoints <= 0 || heightPoints > 409.5 {
		return fmt.Errorf("row height must be finite and within (0,409.5]: %v", heightPoints)
	}
	current, err := w.file.GetRowHeight(sheet, row)
	if err != nil {
		return err
	}
	if current == heightPoints {
		return nil
	}
	if err := w.file.SetRowHeight(sheet, row, heightPoints); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

func (w *Workbook) SetColumnWidth(sheet string, column int, widthChars float64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if column < 1 || column > maxXLSXColumns {
		return fmt.Errorf("column %d outside [1,%d]", column, maxXLSXColumns)
	}
	if !finiteNumber(widthChars) || widthChars <= 0 || widthChars > 255 {
		return fmt.Errorf("column width must be finite and within (0,255]: %v", widthChars)
	}
	columnName, err := excelize.ColumnNumberToName(column)
	if err != nil {
		return err
	}
	current, err := w.file.GetColWidth(sheet, columnName)
	if err != nil {
		return err
	}
	if current == widthChars {
		return nil
	}
	if err := w.file.SetColWidth(sheet, columnName, columnName, widthChars); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// SetCellText edits a cell as a literal string. Formula entry is deliberately
// deferred until calculation and round-trip behavior are defined.
func (w *Workbook) SetCellText(sheet, cell, value string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if strings.HasPrefix(strings.TrimSpace(value), "=") {
		return errors.New("formula input is not supported")
	}
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return fmt.Errorf("invalid cell reference %q: %w", cell, err)
	}
	if row > maxXLSXRows || column > maxXLSXColumns {
		return fmt.Errorf("cell reference %q exceeds XLSX worksheet limits", cell)
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return fmt.Errorf("worksheet %q does not exist", sheet)
	}
	current, err := w.file.GetCellValue(sheet, cell)
	if err != nil {
		return err
	}
	if current == value {
		return nil
	}
	var typedValue any = value
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TRUE":
		typedValue = true
	case "FALSE":
		typedValue = false
	default:
		if number, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil {
			if math.IsInf(number, 0) || math.IsNaN(number) {
				return errors.New("numeric cell values must be finite")
			}
			typedValue = number
		} else if strings.EqualFold(strings.TrimSpace(value), "nan") || strings.EqualFold(strings.TrimSpace(value), "inf") || strings.EqualFold(strings.TrimSpace(value), "+inf") || strings.EqualFold(strings.TrimSpace(value), "-inf") {
			return errors.New("numeric cell values must be finite")
		}
	}
	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return fmt.Errorf("capture cell edit history: %w", err)
	}
	if err := w.file.SetCellValue(sheet, cell, typedValue); err != nil {
		return err
	}
	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return err
	}
	w.dirty = true
	return nil
}

func (w *Workbook) Dirty() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dirty
}

func (w *Workbook) Editable() bool { return true }

func (w *Workbook) Save() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if w.path == "" {
		return errors.New("workbook has no save path")
	}
	if err := w.saveTo(w.path); err != nil {
		return err
	}
	if err := w.rebaseHistoryLocked(); err != nil {
		w.dirty = true
		return err
	}
	return nil
}

// SaveAs writes an independent copy without changing the workbook's source path.
func (w *Workbook) SaveAs(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if err := w.saveTo(path); err != nil {
		return err
	}
	return nil
}

// SaveAsDocument writes and adopts a new source path for Save operations that follow.
func (w *Workbook) SaveAsDocument(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if err := w.saveTo(path); err != nil {
		return err
	}
	w.path = path
	if err := w.rebaseHistoryLocked(); err != nil {
		w.dirty = true
		return err
	}
	return nil
}

func (w *Workbook) saveTo(path string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".glance-*.xlsx")
	if err != nil {
		return fmt.Errorf("create temporary workbook: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	if err := w.file.SaveAs(tmpPath); err != nil {
		return fmt.Errorf("write workbook: %w", err)
	}
	written, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	if err := written.Sync(); err != nil {
		_ = written.Close()
		return err
	}
	if err := written.Close(); err != nil {
		return err
	}
	if err := fileutil.ReplaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace workbook: %w", err)
	}
	return nil
}

func (w *Workbook) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *Workbook) checkOpen() error {
	if w.closed || w.file == nil {
		return errors.New("workbook is closed")
	}
	return nil
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
