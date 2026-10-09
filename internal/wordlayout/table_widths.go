package wordlayout

import "glance/internal/document"

func tableColumnWidths(table document.WordTable, totalWidth float64) ([]float64, bool) {
	columns := len(table.Layout.GridColumnsEMU)
	for _, row := range table.Rows {
		rowColumns := 0
		for _, cell := range row.Cells {
			rowColumns += tableCellGridSpan(cell)
		}
		if rowColumns > columns {
			columns = rowColumns
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
		gridColumn := 0
		for _, cell := range row.Cells {
			span := tableCellGridSpan(cell)
			if cell.WidthEMU > 0 && gridColumn < len(widths) && span <= len(widths)-gridColumn {
				knownWidth := 0.0
				missing := make([]int, 0, span)
				for index := gridColumn; index < gridColumn+span; index++ {
					if widths[index] > 0 {
						knownWidth += widths[index]
					} else {
						missing = append(missing, index)
					}
				}
				if len(missing) > 0 {
					remainingWidth := emuToPoints(cell.WidthEMU) - knownWidth
					if remainingWidth > 0 {
						for _, index := range missing {
							widths[index] = remainingWidth / float64(len(missing))
						}
					}
				}
			}
			gridColumn += span
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
