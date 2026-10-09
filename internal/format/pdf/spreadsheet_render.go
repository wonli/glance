package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"glance/internal/document"
	"io"
	"strconv"
	"strings"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

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
