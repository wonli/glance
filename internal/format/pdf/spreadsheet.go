package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/document"
	"glance/internal/fileutil"
)

// SpreadsheetPDFOptions controls worksheet page setup and default layout.
type SpreadsheetPDFOptions struct {
	SheetName       string
	Orientation     string // "portrait" or "landscape" (or empty for auto)
	PageWidthPT     float64
	PageHeightPT    float64
	MarginPT        float64
	DefaultFontKey  string
	DefaultFontSize float64
	FitToPageWidth  bool
}

// SpreadsheetImageSource returns over-cell picture metadata and original image bytes.
type SpreadsheetImageSource interface {
	SpreadsheetImages(sheet string) ([]document.SpreadsheetImage, error)
	SpreadsheetImageBytes(imageID string) ([]byte, error)
}

// SpreadsheetFontResolver is an optional extension on FontSource that picks
// a suitable system font key for a given cell style and text content.
type SpreadsheetFontResolver interface {
	ResolveSpreadsheetFont(style document.CellStyle, text string) (string, error)
}

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

// SpreadsheetBand represents a half-open range [Start, End) of column or row indices.
type SpreadsheetBand struct {
	Start int
	End   int
}

// SpreadsheetPagination contains computed page dimensions, bands, and trimmed window.
type SpreadsheetPagination struct {
	TrimmedWindow   document.SheetWindow
	Orientation     string
	PageWidthPT     float64
	PageHeightPT    float64
	MarginPT        float64
	TotalGridWidth  float64
	TotalGridHeight float64
	Scale           float64
	ColWidths       []float64
	RowHeights      []float64
	ColBands        []SpreadsheetBand
	RowBands        []SpreadsheetBand
	TotalPages      int
	UsedRows        int
	UsedCols        int
}

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

// WriteSpreadsheetPDF renders a bounded worksheet snapshot to dst as a PDF document.
func WriteSpreadsheetPDF(ctx context.Context, dst io.Writer, window document.SheetWindow, options SpreadsheetPDFOptions, fonts FontSource, images SpreadsheetImageSource) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if dst == nil {
		return errors.New("PDF destination is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	p := ComputeSpreadsheetPagination(window, options, images)
	window = p.TrimmedWindow
	margin := p.MarginPT
	pageWidth := p.PageWidthPT
	pageHeight := p.PageHeightPT
	colWidths := p.ColWidths
	rowHeights := p.RowHeights
	bands := p.ColBands
	for row, height := range rowHeights {
		if height > pageHeight-2*margin {
			return fmt.Errorf("worksheet row %d is taller than one PDF page; reduce its content or font size", window.StartRow+row)
		}
	}
	numRows := len(window.Cells)
	numCols := 0
	if numRows > 0 {
		numCols = len(window.Cells[0])
	}
	defaultFontSize := options.DefaultFontSize
	if defaultFontSize <= 0 {
		defaultFontSize = 10.0
	}

	type mergeCellInfo struct {
		anchorRow, anchorCol int
		widthPT, heightPT    float64
		borders              [4]document.CellBorder
		isAnchor             bool
		inMerge              bool
	}
	merges := make([][]mergeCellInfo, numRows)
	for r := 0; r < numRows; r++ {
		merges[r] = make([]mergeCellInfo, numCols)
	}

	for _, m := range window.Merges {
		ar := m.Range.StartRow - window.StartRow
		ac := m.Range.StartColumn - window.StartColumn
		er := m.Range.EndRow - window.StartRow
		ec := m.Range.EndColumn - window.StartColumn
		if ar < 0 || ar >= numRows || ac < 0 || ac >= numCols {
			continue
		}
		if er >= numRows {
			er = numRows - 1
		}
		if ec >= numCols {
			ec = numCols - 1
		}
		outerBorders := mergedCellOuterBorders(window.Cells, ar, ac, er, ec)
		var mw, mh float64
		for c := ac; c <= ec; c++ {
			mw += colWidths[c]
		}
		for r := ar; r <= er; r++ {
			mh += rowHeights[r]
		}
		for r := ar; r <= er; r++ {
			for c := ac; c <= ec; c++ {
				merges[r][c] = mergeCellInfo{
					anchorRow: ar,
					anchorCol: ac,
					widthPT:   mw,
					heightPT:  mh,
					borders:   outerBorders,
					isAnchor:  r == ar && c == ac,
					inMerge:   true,
				}
			}
		}
	}

	documentPDF := gopdf.New()
	documentPDF.CompressObjects = true
	fontCache := make(map[string]*gopdf.Font)
	imageCache := make(map[string]*gopdf.Image)

	getFont := func(key string) (*gopdf.Font, error) {
		if key == "" || fonts == nil {
			return nil, nil
		}
		if f := fontCache[key]; f != nil {
			return f, nil
		}
		raw, err := fonts.FontData(key)
		if err != nil {
			return nil, err
		}
		faceIdx, err := fontFaceIndex(key)
		if err != nil {
			return nil, err
		}
		standalone, err := extractSFNT(raw, faceIdx)
		if err != nil {
			return nil, err
		}
		if err := validateFontEmbedding(standalone, key); err != nil {
			return nil, err
		}
		f, err := gopdf.ParseFont(standalone)
		if err != nil {
			return nil, err
		}
		fontCache[key] = f
		return f, nil
	}

	resolveKey := func(style document.CellStyle, text string) string {
		if resolver, ok := fonts.(SpreadsheetFontResolver); ok && resolver != nil {
			if k, err := resolver.ResolveSpreadsheetFont(style, text); err == nil && k != "" {
				return k
			}
		}
		if options.DefaultFontKey != "" {
			return options.DefaultFontKey
		}
		return ""
	}

	var sheetImages []document.SpreadsheetImage
	if images != nil && options.SheetName != "" {
		sheetImages, _ = images.SpreadsheetImages(options.SheetName)
	}

	for _, band := range bands {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, rowBand := range p.RowBands {
			curPage := documentPDF.AddPageSize(gopdf.PageSize{W: pageWidth, H: pageHeight})
			curPage.ClipRect(0, 0, pageWidth, pageHeight)
			var edges []spreadsheetBorderBox
			y := margin
			for r := rowBand.Start; r < rowBand.End; r++ {
				if err := ctx.Err(); err != nil {
					return err
				}
				rh := rowHeights[r]
				x := margin
				for c := band.Start; c < band.End; c++ {
					cw := colWidths[c]
					merge := merges[r][c]
					if merge.inMerge && !merge.isAnchor {
						x += cw
						continue
					}

					cellW := cw
					cellH := rh
					if merge.inMerge && merge.isAnchor {
						cellW = merge.widthPT
						cellH = merge.heightPT
					}

					cell := window.Cells[r][c]

					// Fill background
					if cell.Style.FillColor != "" {
						curPage.Push()
						curPage.SetFillColor(textColor(cell.Style.FillColor))
						curPage.Rect(x, y, cellW, cellH, gopdf.Fill)
						curPage.Pop()
					}

					borders := cell.Style.Borders
					if merge.inMerge && merge.isAnchor {
						borders = merge.borders
					}
					edges = append(edges, spreadsheetBorderBox{x: x, y: y, w: cellW, h: cellH, borders: borders})

					// Draw text
					val := cell.Value
					if val != "" {
						fontSize := cell.Style.FontSize
						if fontSize <= 0 {
							fontSize = defaultFontSize
						}
						fontSize *= p.Scale
						key := resolveKey(cell.Style, val)
						pdfFont, _ := getFont(key)

						align := gopdf.AlignLeft
						switch cell.Style.HorizontalAlignment {
						case "center":
							align = gopdf.AlignCenter
						case "right":
							align = gopdf.AlignRight
						case "left":
							align = gopdf.AlignLeft
						default:
							if isNumeric(val) {
								align = gopdf.AlignRight
							}
						}

						curPage.Push()
						curPage.ClipRect(x+1, y+1, cellW-2, cellH-2)
						if pdfFont != nil {
							curPage.SetFont(pdfFont, fontSize)
						} else {
							curPage.SetFont(gopdf.Helvetica, fontSize)
						}
						curPage.SetFillColor(textColor(cell.Style.FontColor))
						textPad := 3.0 * p.Scale
						lines := spreadsheetCellLines(val, cell.Style.WrapText, cellW-2*textPad, fontSize)
						lineHeight := fontSize * 1.25
						textTop := y + (cellH-float64(len(lines))*lineHeight)/2
						if textTop < y+3*p.Scale {
							textTop = y + 3*p.Scale
						}
						for index, line := range lines {
							if line != "" && cellW > 2*textPad {
								curPage.TextAligned(x+textPad, textTop+fontSize+float64(index)*lineHeight, cellW-2*textPad, align, line)
							}
						}
						curPage.Pop()
					}

					x += cw
				}
				y += rh
			}

			// Paint all edges after fills so adjacent cells cannot erase half a stroke.
			for _, edge := range edges {
				curPage.Push()
				curPage.SetStrokeColor(gopdf.RGB(170, 170, 170))
				curPage.SetLineWidth(0.5 * p.Scale)
				curPage.Rect(edge.x, edge.y, edge.w, edge.h, gopdf.Stroke)
				curPage.Pop()
			}
			for _, edge := range edges {
				drawStyledBorders(curPage, edge.x, edge.y, edge.w, edge.h, edge.borders, p.Scale)
			}

			// Draw over-cell images that fit within this column band
			for _, img := range sheetImages {
				if img.WidthPx <= 0 || img.HeightPx <= 0 {
					continue
				}
				rowIdx := img.Row - window.StartRow
				colIdx := img.Column - window.StartColumn
				if rowIdx < rowBand.Start || rowIdx >= rowBand.End || colIdx < band.Start || colIdx >= band.End {
					continue
				}

				pdfImage := imageCache[img.ID]
				if pdfImage == nil && images != nil {
					data, err := images.SpreadsheetImageBytes(img.ID)
					if err == nil && len(data) > 0 {
						pdfImage, err = documentPDF.AddImageReader(bytes.NewReader(data))
						if err == nil {
							imageCache[img.ID] = pdfImage
						}
					}
				}
				if pdfImage == nil {
					continue
				}

				// Compute image X and Y in points (1 px = 0.75 pt)
				var imgX float64 = margin + float64(img.OffsetX)*0.75*p.Scale
				for c := band.Start; c < colIdx; c++ {
					imgX += colWidths[c]
				}
				var imgY float64 = margin + float64(img.OffsetY)*0.75*p.Scale
				for r := rowBand.Start; r < rowIdx; r++ {
					imgY += rowHeights[r]
				}
				imgW := float64(img.WidthPx) * 0.75 * p.Scale
				imgH := float64(img.HeightPx) * 0.75 * p.Scale

				if imgX+imgW <= pageWidth && imgY+imgH <= pageHeight {
					curPage.Push()
					curPage.DrawImage(pdfImage, imgX, imgY, imgW, imgH)
					curPage.Pop()
				}
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := documentPDF.WriteTo(contextWriter{ctx: ctx, dst: dst}); err != nil {
		return fmt.Errorf("serialize spreadsheet PDF: %w", err)
	}
	return nil
}

type spreadsheetBorderBox struct {
	x, y, w, h float64
	borders    [4]document.CellBorder
}

func drawStyledBorders(page *gopdf.Page, x, y, w, h float64, borders [4]document.CellBorder, scale float64) {
	// Top: 0, Right: 1, Bottom: 2, Left: 3
	drawEdge := func(b document.CellBorder, x1, y1, x2, y2 float64) {
		if b.Style == 0 {
			return
		}
		page.Push()
		page.SetStrokeColor(textColor(b.Color))
		width := 0.5
		switch b.Style {
		case 2, 8, 10, 12, 13:
			width = 1
		case 5:
			width = 1.5
		case 7:
			width = 0.25
		}
		page.SetLineWidth(width * scale)
		page.Line(x1, y1, x2, y2)
		page.Pop()
	}
	drawEdge(borders[0], x, y, x+w, y)
	drawEdge(borders[1], x+w, y, x+w, y+h)
	drawEdge(borders[2], x, y+h, x+w, y+h)
	drawEdge(borders[3], x, y, x, y+h)
}

func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// ExportSpreadsheetPDFFile writes a complete spreadsheet PDF beside path,
// validates it, and atomically replaces path.
func ExportSpreadsheetPDFFile(ctx context.Context, path string, window document.SheetWindow, options SpreadsheetPDFOptions, fonts FontSource, images SpreadsheetImageSource) error {
	return ExportSpreadsheetPDFFileWithCommitGuard(ctx, path, window, options, fonts, images, nil)
}

// ExportSpreadsheetPDFFileWithCommitGuard creates a temporary PDF, validates it,
// executes guard(), and replaces the destination path atomically.
func ExportSpreadsheetPDFFileWithCommitGuard(ctx context.Context, path string, window document.SheetWindow, options SpreadsheetPDFOptions, fonts FontSource, images SpreadsheetImageSource, guard func() (func(), error)) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("PDF destination path is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".spreadsheet-pdf-*.pdf")
	if err != nil {
		return fmt.Errorf("create temporary PDF beside destination: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := WriteSpreadsheetPDF(ctx, temporary, window, options, fonts, images); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary PDF: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary PDF: %w", err)
	}

	reader, err := gopdf.Open(temporaryPath)
	if err != nil {
		return fmt.Errorf("validate generated PDF: %w", err)
	}
	if reader.Repaired() {
		return errors.New("validate generated PDF: reader had to repair the file")
	}
	if reader.NumPages() < 1 {
		return errors.New("validate generated PDF: output has no pages")
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	var release func()
	if guard != nil {
		var err error
		release, err = guard()
		if err != nil {
			return err
		}
		if release != nil {
			defer release()
		}
	}
	if err := fileutil.ReplaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace PDF destination: %w", err)
	}
	return nil
}
