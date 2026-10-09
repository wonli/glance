package pdf

import (
	"glance/internal/document"
	"strings"
)

// ComputeSpreadsheetPagination calculates layout metrics and pagination for a sheet snapshot.
func ComputeSpreadsheetPagination(window document.SheetWindow, options SpreadsheetPDFOptions, images SpreadsheetImageSource) SpreadsheetPagination {
	var sheetImages []document.SpreadsheetImage
	if images != nil && options.SheetName != "" {
		sheetImages, _ = images.SpreadsheetImages(options.SheetName)
	}

	trimmedWindow, usedRows, usedCols := TrimSheetWindow(window, sheetImages)
	return computeSpreadsheetPaginationForTrimmedWindow(trimmedWindow, usedRows, usedCols, options)
}

// ComputeSpreadsheetPaginationForTrimmedWindow avoids scanning cells again when
// the snapshot has already been trimmed outside the UI frame loop.
func ComputeSpreadsheetPaginationForTrimmedWindow(window document.SheetWindow, options SpreadsheetPDFOptions) SpreadsheetPagination {
	rows, cols := len(window.Cells), 0
	if rows > 0 {
		cols = len(window.Cells[0])
	}
	return computeSpreadsheetPaginationForTrimmedWindow(window, rows, cols, options)
}

func computeSpreadsheetPaginationForTrimmedWindow(trimmedWindow document.SheetWindow, usedRows, usedCols int, options SpreadsheetPDFOptions) SpreadsheetPagination {

	margin := options.MarginPT
	if margin <= 0 {
		margin = 36.0
	}
	pageWidth := options.PageWidthPT
	if pageWidth <= 0 {
		pageWidth = 595.0
	}
	pageHeight := options.PageHeightPT
	if pageHeight <= 0 {
		pageHeight = 842.0
	}

	numRows := len(trimmedWindow.Cells)
	numCols := 0
	if numRows > 0 {
		numCols = len(trimmedWindow.Cells[0])
	}

	colWidths := make([]float64, numCols)
	totalGridWidth := 0.0
	for c := 0; c < numCols; c++ {
		w := 54.0
		if c < len(trimmedWindow.ColumnWidthsChars) && trimmedWindow.ColumnWidthsChars[c] > 0 {
			w = trimmedWindow.ColumnWidthsChars[c] * 7.0
			if w < 24.0 {
				w = 24.0
			}
		}
		colWidths[c] = w
		totalGridWidth += w
	}

	rowHeights := make([]float64, numRows)
	totalGridHeight := 0.0
	defaultFontSize := options.DefaultFontSize
	if defaultFontSize <= 0 {
		defaultFontSize = 10
	}
	for r := 0; r < numRows; r++ {
		h := 18.0
		if r < len(trimmedWindow.RowHeightsPoints) && trimmedWindow.RowHeightsPoints[r] > 0 {
			h = trimmedWindow.RowHeightsPoints[r]
			if h < 12.0 {
				h = 12.0
			}
		}
		rowHeights[r] = h
	}
	covered := make(map[[2]int]bool)
	mergeWidths := make(map[[2]int]float64, len(trimmedWindow.Merges))
	for _, merge := range trimmedWindow.Merges {
		ar := merge.Range.StartRow - trimmedWindow.StartRow
		ac := merge.Range.StartColumn - trimmedWindow.StartColumn
		if ar < 0 || ar >= numRows || ac < 0 || ac >= numCols {
			continue
		}
		width := 0.0
		for c := ac; c < numCols && c <= merge.Range.EndColumn-trimmedWindow.StartColumn; c++ {
			width += colWidths[c]
		}
		mergeWidths[[2]int{ar, ac}] = width
		for r := ar; r < numRows && r <= merge.Range.EndRow-trimmedWindow.StartRow; r++ {
			for c := ac; c < numCols && c <= merge.Range.EndColumn-trimmedWindow.StartColumn; c++ {
				if r != ar || c != ac {
					covered[[2]int{r, c}] = true
				}
			}
		}
	}
	for r, row := range trimmedWindow.Cells {
		for c, cell := range row {
			if cell.Value == "" || covered[[2]int{r, c}] {
				continue
			}
			cellWidth := colWidths[c]
			if mergedWidth := mergeWidths[[2]int{r, c}]; mergedWidth > 0 {
				cellWidth = mergedWidth
			}
			fontSize := cell.Style.FontSize
			if fontSize <= 0 {
				fontSize = defaultFontSize
			}
			lines := spreadsheetCellLines(cell.Value, cell.Style.WrapText, cellWidth-6, fontSize)
			required := float64(len(lines))*fontSize*1.25 + 6
			if required > rowHeights[r] {
				rowHeights[r] = required
			}
		}
	}
	for _, h := range rowHeights {
		totalGridHeight += h
	}

	orientation := strings.ToLower(options.Orientation)
	if orientation == "landscape" {
		if pageWidth < pageHeight {
			pageWidth, pageHeight = pageHeight, pageWidth
		}
	} else if orientation == "portrait" {
		if pageWidth > pageHeight {
			pageWidth, pageHeight = pageHeight, pageWidth
		}
	} else {
		// auto: switch to landscape if grid fits better
		if totalGridWidth > (pageWidth-2*margin) && totalGridWidth <= (pageHeight-2*margin) {
			pageWidth, pageHeight = pageHeight, pageWidth
			orientation = "landscape"
		} else {
			orientation = "portrait"
		}
	}

	availW := pageWidth - 2*margin
	availH := pageHeight - 2*margin

	scale := 1.0
	// Scale the complete sheet, including row heights, text, and images.
	if options.FitToPageWidth && totalGridWidth > availW && totalGridWidth > 0 && availW > 0 {
		scale = availW / totalGridWidth
		for c := range colWidths {
			colWidths[c] *= scale
		}
		for r := range rowHeights {
			rowHeights[r] *= scale
		}
		totalGridWidth = availW
		totalGridHeight *= scale
	}

	var colBands []SpreadsheetBand
	if numCols == 0 {
		colBands = append(colBands, SpreadsheetBand{Start: 0, End: 0})
	} else {
		bandStart := 0
		curW := 0.0
		for c := 0; c < numCols; c++ {
			if c > bandStart && curW+colWidths[c] > availW {
				colBands = append(colBands, SpreadsheetBand{Start: bandStart, End: c})
				bandStart = c
				curW = colWidths[c]
			} else {
				curW += colWidths[c]
			}
		}
		colBands = append(colBands, SpreadsheetBand{Start: bandStart, End: numCols})
	}

	var rowBands []SpreadsheetBand
	if numRows == 0 {
		rowBands = append(rowBands, SpreadsheetBand{Start: 0, End: 0})
	} else {
		rowStart := 0
		curH := 0.0
		for r := 0; r < numRows; r++ {
			if r > rowStart && curH+rowHeights[r] > availH {
				rowBands = append(rowBands, SpreadsheetBand{Start: rowStart, End: r})
				rowStart = r
				curH = rowHeights[r]
			} else {
				curH += rowHeights[r]
			}
		}
		rowBands = append(rowBands, SpreadsheetBand{Start: rowStart, End: numRows})
	}

	totalPages := len(colBands) * len(rowBands)
	if totalPages == 0 {
		totalPages = 1
	}

	return SpreadsheetPagination{
		TrimmedWindow:   trimmedWindow,
		Orientation:     orientation,
		PageWidthPT:     pageWidth,
		PageHeightPT:    pageHeight,
		MarginPT:        margin,
		TotalGridWidth:  totalGridWidth,
		TotalGridHeight: totalGridHeight,
		Scale:           scale,
		ColWidths:       colWidths,
		RowHeights:      rowHeights,
		ColBands:        colBands,
		RowBands:        rowBands,
		TotalPages:      totalPages,
		UsedRows:        usedRows,
		UsedCols:        usedCols,
	}
}
