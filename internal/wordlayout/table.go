package wordlayout

import (
	"context"
	"math"

	"glance/internal/document"
)

const tableCellPaddingPT = 2.0

type preparedTableCell struct {
	cell       document.WordTableCell
	paragraphs []preparedParagraph
	contentH   float64
}

type preparedTableRow struct {
	index  int
	cells  []preparedTableCell
	height float64
}

func (c *layoutCursor) layoutTable(ctx context.Context, shaper TextShaper, styles *styleResolver, tableID string, table document.WordTable, area Rect, editorInsets bool) error {
	if len(table.Rows) == 0 {
		return nil
	}
	widths, inferredWidths := tableColumnWidths(table, area.WidthPT)
	if inferredWidths {
		c.diagnostic(DiagnosticWarning, tableID, "table-grid-column-width-inferred", "One or more table grid column widths are missing; inferred widths require review before output.")
	}
	for rowIndex, sourceRow := range table.Rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := preparedTableRow{index: rowIndex, height: tableCellPaddingPT * 2}
		for cellIndex, cell := range sourceRow.Cells {
			cellWidth := area.WidthPT / float64(max(1, len(widths)))
			if cellIndex < len(widths) {
				cellWidth = widths[cellIndex]
			}
			preparedCell := preparedTableCell{cell: cell}
			innerWidth := math.Max(1, cellWidth-tableCellPaddingPT*2)
			for _, paragraph := range cell.Paragraphs {
				prepared, err := prepareParagraph(ctx, shaper, styles, paragraph, paragraph.ID, innerWidth, true, editorInsets)
				if err != nil {
					return err
				}
				for _, line := range prepared.lines {
					if line.pageBreakBefore || line.pageBreakAfter {
						c.diagnostic(DiagnosticError, paragraph.ID, "table-cell-page-break", "Page breaks inside a table row are not supported.")
					}
				}
				preparedCell.contentH += prepared.totalHeight()
				preparedCell.paragraphs = append(preparedCell.paragraphs, prepared)
			}
			row.height = math.Max(row.height, preparedCell.contentH+tableCellPaddingPT*2)
			row.cells = append(row.cells, preparedCell)
		}
		rowID := tableID + ".r" + itoa(rowIndex)
		if row.height > c.content.HeightPT+epsilonPT {
			c.diagnostic(DiagnosticError, rowID, "table-row-overflow", "This table row is taller than the printable page area and cannot be split safely.")
			return nil
		}
		if !c.ensureSpace(row.height, rowID) {
			// A fresh page cannot fit this row, so do not retry it indefinitely.
			return nil
		}
		y := c.y
		x := area.XPT
		if c.columns > 1 {
			x = c.currentColumnRect().XPT
		}
		for cellIndex, cell := range row.cells {
			cellWidth := area.WidthPT / float64(max(1, len(widths)))
			if cellIndex < len(widths) {
				cellWidth = widths[cellIndex]
			}
			cellBounds := Rect{XPT: x, YPT: y, WidthPT: cellWidth, HeightPT: row.height}
			fillColor := cell.cell.FillColor
			if fillColor == "" {
				fillColor = table.Layout.FillColor
			}
			c.result.Pages[c.pageIndex].TableCells = append(c.result.Pages[c.pageIndex].TableCells, TableCellPlacement{
				TableID: tableID, RowIndex: row.index, CellID: cell.cell.ID, FillColor: fillColor, Bounds: cellBounds,
			})
			inner := Rect{
				XPT: x + tableCellPaddingPT, YPT: y + tableCellPaddingPT,
				WidthPT: math.Max(1, cellWidth-tableCellPaddingPT*2), HeightPT: math.Max(0, row.height-tableCellPaddingPT*2),
			}
			cellY := inner.YPT
			for _, paragraph := range cell.paragraphs {
				if err := c.placePreparedParagraph(ctx, paragraph, inner, false, c.pageIndex, cellY); err != nil {
					return err
				}
				cellY += paragraph.totalHeight()
			}
			x += cellWidth
		}
		c.y += row.height
		c.used = true
	}
	return nil
}

func tableColumnWidths(table document.WordTable, totalWidth float64) ([]float64, bool) {
	columns := len(table.Layout.GridColumnsEMU)
	for _, row := range table.Rows {
		if len(row.Cells) > columns {
			columns = len(row.Cells)
		}
	}
	if columns == 0 {
		return nil, false
	}
	widths := make([]float64, columns)
	for index, emu := range table.Layout.GridColumnsEMU {
		widths[index] = emuToPoints(emu)
	}
	for _, row := range table.Rows {
		for index, cell := range row.Cells {
			if widths[index] <= 0 && cell.WidthEMU > 0 {
				widths[index] = emuToPoints(cell.WidthEMU)
			}
		}
	}
	inferred := false
	for _, width := range widths {
		if width <= 0 {
			inferred = true
			break
		}
	}
	sum := 0.0
	knownCount := 0
	for _, width := range widths {
		if width > 0 {
			sum += width
			knownCount++
		}
	}
	if knownCount == 0 || sum <= 0 {
		for index := range widths {
			widths[index] = totalWidth / float64(columns)
		}
		return widths, inferred
	}
	for index, width := range widths {
		if width <= 0 {
			widths[index] = sum / float64(knownCount)
		}
	}
	sum = 0
	for _, width := range widths {
		sum += width
	}
	scale := totalWidth / sum
	for index := range widths {
		widths[index] *= scale
	}
	return widths, inferred
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	if value < 0 {
		return "-" + itoa(-value)
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
