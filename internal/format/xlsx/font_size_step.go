package xlsx

import (
	"math"

	"github.com/xuri/excelize/v2"
)

var commonExcelFontSizes = [...]float64{8, 9, 10, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 36, 48, 72}

func stepExcelFontSize(style *excelize.Style, step int) {
	if style == nil || (step != -1 && step != 1) {
		return
	}
	current := 11.0
	if style.Font != nil && style.Font.Size > 0 && !math.IsNaN(style.Font.Size) && !math.IsInf(style.Font.Size, 0) {
		current = style.Font.Size
	}
	current = math.Min(409, current)
	next := adjacentExcelFontSize(current, step)
	if next == current {
		return
	}
	if style.Font == nil {
		style.Font = &excelize.Font{}
	}
	style.Font.Size = next
}

func adjacentExcelFontSize(current float64, step int) float64 {
	if current < commonExcelFontSizes[0] {
		if step > 0 {
			return math.Min(409, current+1)
		}
		return math.Max(1, current-1)
	}
	if current > commonExcelFontSizes[len(commonExcelFontSizes)-1] {
		if step > 0 {
			return math.Min(409, current+1)
		}
		return math.Max(commonExcelFontSizes[len(commonExcelFontSizes)-1], current-1)
	}
	if step > 0 {
		for _, size := range commonExcelFontSizes {
			if size > current {
				return size
			}
		}
		if current >= 409 {
			return current
		}
		return math.Min(409, current+1)
	}
	for index := len(commonExcelFontSizes) - 1; index >= 0; index-- {
		if commonExcelFontSizes[index] < current {
			return commonExcelFontSizes[index]
		}
	}
	return math.Max(1, current-1)
}
