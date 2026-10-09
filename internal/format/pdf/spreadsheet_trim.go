package pdf

import (
	"glance/internal/document"
	"strings"
)

// TrimSheetWindow trims trailing rows and columns that contain no content,
// background colors, borders, merged regions, or images.
func TrimSheetWindow(window document.SheetWindow, images []document.SpreadsheetImage) (document.SheetWindow, int, int) {
	numRows := len(window.Cells)
	numCols := 0
	if numRows > 0 {
		numCols = len(window.Cells[0])
	}
	if numRows == 0 || numCols == 0 {
		return window, 0, 0
	}

	maxRow := -1
	maxCol := -1

	// Check cell contents and formatting
	for r := 0; r < numRows; r++ {
		row := window.Cells[r]
		for c := 0; c < len(row); c++ {
			if isCellUsed(row[c]) {
				if r > maxRow {
					maxRow = r
				}
				if c > maxCol {
					maxCol = c
				}
			}
		}
	}

	// Check merged cells
	for _, m := range window.Merges {
		ar := m.Range.StartRow - window.StartRow
		ac := m.Range.StartColumn - window.StartColumn
		er := m.Range.EndRow - window.StartRow
		ec := m.Range.EndColumn - window.StartColumn
		if ar >= 0 && ar < numRows && ac >= 0 && ac < numCols {
			if er > maxRow {
				if er >= numRows {
					maxRow = numRows - 1
				} else {
					maxRow = er
				}
			}
			if ec > maxCol {
				if ec >= numCols {
					maxCol = numCols - 1
				} else {
					maxCol = ec
				}
			}
		}
	}

	// Check images
	for _, img := range images {
		r := img.Row - window.StartRow
		c := img.Column - window.StartColumn
		if r >= 0 && r < numRows && c >= 0 && c < numCols {
			if r > maxRow {
				maxRow = r
			}
			if c > maxCol {
				maxCol = c
			}
		}
	}

	// If no content found at all, return minimal 1x1
	if maxRow < 0 {
		maxRow = 0
	}
	if maxCol < 0 {
		maxCol = 0
	}

	trimmedRows := maxRow + 1
	trimmedCols := maxCol + 1

	trimmedCells := make([][]document.Cell, trimmedRows)
	for r := 0; r < trimmedRows; r++ {
		trimmedCells[r] = make([]document.Cell, trimmedCols)
		for c := 0; c < trimmedCols; c++ {
			if c < len(window.Cells[r]) {
				trimmedCells[r][c] = window.Cells[r][c]
			}
		}
	}

	var trimmedRowHeights []float64
	if len(window.RowHeightsPoints) > 0 {
		hLen := trimmedRows
		if hLen > len(window.RowHeightsPoints) {
			hLen = len(window.RowHeightsPoints)
		}
		trimmedRowHeights = append([]float64(nil), window.RowHeightsPoints[:hLen]...)
	}

	var trimmedColWidths []float64
	if len(window.ColumnWidthsChars) > 0 {
		wLen := trimmedCols
		if wLen > len(window.ColumnWidthsChars) {
			wLen = len(window.ColumnWidthsChars)
		}
		trimmedColWidths = append([]float64(nil), window.ColumnWidthsChars[:wLen]...)
	}

	var trimmedMerges []document.MergeRegion
	for _, m := range window.Merges {
		ar := m.Range.StartRow - window.StartRow
		ac := m.Range.StartColumn - window.StartColumn
		if ar >= 0 && ar < trimmedRows && ac >= 0 && ac < trimmedCols {
			trimmedMerges = append(trimmedMerges, m)
		}
	}

	trimmed := document.SheetWindow{
		StartRow:          window.StartRow,
		StartColumn:       window.StartColumn,
		Cells:             trimmedCells,
		Merges:            trimmedMerges,
		RowHeightsPoints:  trimmedRowHeights,
		ColumnWidthsChars: trimmedColWidths,
	}
	return trimmed, trimmedRows, trimmedCols
}

func isCellUsed(cell document.Cell) bool {
	if strings.TrimSpace(cell.Value) != "" {
		return true
	}
	fill := strings.TrimPrefix(strings.ToUpper(cell.Style.FillColor), "#")
	if fill != "" && fill != "FFFFFF" && fill != "FFFFFFFF" {
		return true
	}
	for _, b := range cell.Style.Borders {
		if b.Style > 0 {
			return true
		}
	}
	return false
}

func spreadsheetCellLines(value string, wrap bool, widthPT, fontSize float64) []string {
	if value == "" {
		return nil
	}
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	paragraphs := strings.Split(value, "\n")
	if !wrap {
		return paragraphs
	}
	if widthPT <= 0 {
		widthPT = 1
	}
	var lines []string
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		var current []rune
		currentWidth := 0.0
		for _, r := range paragraph {
			runeWidth := fontSize * 1.15
			if r < 128 {
				runeWidth = fontSize * 0.85
				if r == ' ' {
					runeWidth = fontSize * 0.4
				}
			}
			if len(current) > 0 && currentWidth+runeWidth > widthPT {
				lines = append(lines, string(current))
				current = current[:0]
				currentWidth = 0
			}
			current = append(current, r)
			currentWidth += runeWidth
		}
		lines = append(lines, string(current))
	}
	return lines
}

func mergedCellOuterBorders(cells [][]document.Cell, startRow, startCol, endRow, endCol int) [4]document.CellBorder {
	var result [4]document.CellBorder
	if startRow < 0 || startRow >= len(cells) || startCol < 0 || startCol >= len(cells[startRow]) {
		return result
	}
	borderAt := func(row, col, side int) document.CellBorder {
		if row < 0 || row >= len(cells) || col < 0 || col >= len(cells[row]) {
			return document.CellBorder{}
		}
		return cells[row][col].Style.Borders[side]
	}
	for c := startCol; c <= endCol; c++ {
		if result[0].Style == 0 {
			result[0] = borderAt(startRow, c, 0)
		}
		if result[2].Style == 0 {
			result[2] = borderAt(endRow, c, 2)
		}
	}
	for r := startRow; r <= endRow; r++ {
		if result[1].Style == 0 {
			result[1] = borderAt(r, endCol, 1)
		}
		if result[3].Style == 0 {
			result[3] = borderAt(r, startCol, 3)
		}
	}
	return result
}
