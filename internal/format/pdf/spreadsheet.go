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
	PageWidthPT     float64
	PageHeightPT    float64
	MarginPT        float64
	DefaultFontKey  string
	DefaultFontSize float64
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

	margin := options.MarginPT
	if margin <= 0 {
		margin = 36.0
	}
	pageWidth := options.PageWidthPT
	if pageWidth <= 0 {
		pageWidth = 595.0 // A4 width
	}
	pageHeight := options.PageHeightPT
	if pageHeight <= 0 {
		pageHeight = 842.0 // A4 height
	}
	defaultFontSize := options.DefaultFontSize
	if defaultFontSize <= 0 {
		defaultFontSize = 10.0
	}

	numRows := len(window.Cells)
	numCols := 0
	if numRows > 0 {
		numCols = len(window.Cells[0])
	}

	colWidths := make([]float64, numCols)
	totalGridWidth := 0.0
	for c := 0; c < numCols; c++ {
		w := 54.0 // ~8 chars default width in points
		if c < len(window.ColumnWidthsChars) && window.ColumnWidthsChars[c] > 0 {
			w = window.ColumnWidthsChars[c] * 7.0
			if w < 24.0 {
				w = 24.0
			}
		}
		colWidths[c] = w
		totalGridWidth += w
	}

	rowHeights := make([]float64, numRows)
	for r := 0; r < numRows; r++ {
		h := 18.0
		if r < len(window.RowHeightsPoints) && window.RowHeightsPoints[r] > 0 {
			h = window.RowHeightsPoints[r]
			if h < 12.0 {
				h = 12.0
			}
		}
		rowHeights[r] = h
	}

	// Switch to landscape if grid fits better
	if totalGridWidth > (pageWidth-2*margin) && totalGridWidth <= (pageHeight-2*margin) {
		pageWidth, pageHeight = pageHeight, pageWidth
	}

	availW := pageWidth - 2*margin
	availH := pageHeight - 2*margin

	type colBand struct {
		start, end int
	}
	var bands []colBand
	if numCols == 0 {
		bands = append(bands, colBand{0, 0})
	} else {
		bandStart := 0
		curW := 0.0
		for c := 0; c < numCols; c++ {
			if c > bandStart && curW+colWidths[c] > availW {
				bands = append(bands, colBand{start: bandStart, end: c})
				bandStart = c
				curW = colWidths[c]
			} else {
				curW += colWidths[c]
			}
		}
		bands = append(bands, colBand{start: bandStart, end: numCols})
	}

	type mergeCellInfo struct {
		anchorRow, anchorCol int
		widthPT, heightPT    float64
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
		if numRows == 0 {
			page := documentPDF.AddPageSize(gopdf.PageSize{W: pageWidth, H: pageHeight})
			page.ClipRect(0, 0, pageWidth, pageHeight)
			continue
		}

		y := margin
		var curPage *gopdf.Page
		for r := 0; r < numRows; r++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			rh := rowHeights[r]
			if curPage == nil || (y+rh > pageHeight-margin && y > margin) {
				curPage = documentPDF.AddPageSize(gopdf.PageSize{W: pageWidth, H: pageHeight})
				curPage.ClipRect(0, 0, pageWidth, pageHeight)
				y = margin
			}

			x := margin
			for c := band.start; c < band.end; c++ {
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

				// Draw borders or light gridlines
				hasBorders := false
				for _, b := range cell.Style.Borders {
					if b.Style != 0 {
						hasBorders = true
						break
					}
				}
				if hasBorders {
					drawStyledBorders(curPage, x, y, cellW, cellH, cell.Style.Borders)
				} else {
					curPage.Push()
					curPage.SetStrokeColor(gopdf.RGB(220, 220, 220))
					curPage.SetLineWidth(0.5)
					curPage.Rect(x, y, cellW, cellH, gopdf.Stroke)
					curPage.Pop()
				}

				// Draw text
				val := cell.Value
				if val != "" {
					fontSize := cell.Style.FontSize
					if fontSize <= 0 {
						fontSize = defaultFontSize
					}
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
					baselineY := y + (cellH+fontSize*0.7)/2
					textPad := 3.0
					if cellW > 2*textPad {
						curPage.TextAligned(x+textPad, baselineY, cellW-2*textPad, align, val)
					}
					curPage.Pop()
				}

				x += cw
			}
			y += rh
		}

		// Draw over-cell images that fit within this column band
		for _, img := range sheetImages {
			if img.WidthPx <= 0 || img.HeightPx <= 0 {
				continue
			}
			rowIdx := img.Row - window.StartRow
			colIdx := img.Column - window.StartColumn
			if rowIdx < 0 || rowIdx >= numRows || colIdx < band.start || colIdx >= band.end {
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
			var imgX float64 = margin + float64(img.OffsetX)*0.75
			for c := band.start; c < colIdx; c++ {
				imgX += colWidths[c]
			}
			var imgY float64 = margin + float64(img.OffsetY)*0.75
			for r := 0; r < rowIdx && r < numRows; r++ {
				imgY += rowHeights[r]
			}
			imgW := float64(img.WidthPx) * 0.75
			imgH := float64(img.HeightPx) * 0.75

			if curPage != nil && imgX+imgW <= pageWidth && imgY+imgH <= pageHeight {
				curPage.Push()
				curPage.DrawImage(pdfImage, imgX, imgY, imgW, imgH)
				curPage.Pop()
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := documentPDF.WriteTo(contextWriter{ctx: ctx, dst: dst}); err != nil {
		return fmt.Errorf("serialize spreadsheet PDF: %w", err)
	}
	_ = availH
	return nil
}

func drawStyledBorders(page *gopdf.Page, x, y, w, h float64, borders [4]document.CellBorder) {
	// Top: 0, Right: 1, Bottom: 2, Left: 3
	drawEdge := func(b document.CellBorder, x1, y1, x2, y2 float64) {
		if b.Style == 0 {
			return
		}
		page.Push()
		page.SetStrokeColor(textColor(b.Color))
		page.SetLineWidth(1.0)
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
