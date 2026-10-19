package desktopui

import (
	"glance/internal/document"
	"image/color"
	"strings"
)

const spreadsheetCurrencyNumberFormat = `"¥"#,##0.00`

func toolbarPickerTitle(picker spreadsheetToolbarPicker) string {
	switch picker {
	case pickerFontColor:
		return "字体颜色"
	case pickerFillColor:
		return "填充颜色"
	case pickerAlignment:
		return "对齐方式"
	case pickerNumberFormat:
		return "数字格式"
	case pickerBorder:
		return "单元格边框"
	default:
		return ""
	}
}

func toolbarPickerTitleMessageID(picker spreadsheetToolbarPicker) string {
	switch picker {
	case pickerFontColor:
		return "spreadsheet.picker.title.font_color"
	case pickerFillColor:
		return "spreadsheet.picker.title.fill_color"
	case pickerAlignment:
		return "spreadsheet.picker.title.alignment"
	case pickerNumberFormat:
		return "spreadsheet.picker.title.number_format"
	case pickerBorder:
		return "spreadsheet.picker.title.border"
	default:
		return "common.translation_missing"
	}
}

func toolbarPickerOptionMessageID(kind spreadsheetToolbarPicker, index int) string {
	var ids []string
	switch kind {
	case pickerFontColor:
		ids = []string{"spreadsheet.picker.color.black", "spreadsheet.picker.color.red", "spreadsheet.picker.color.orange", "spreadsheet.picker.color.yellow", "spreadsheet.picker.color.green", "spreadsheet.picker.color.blue", "spreadsheet.picker.color.purple", "spreadsheet.picker.color.white"}
	case pickerFillColor:
		ids = []string{"spreadsheet.picker.fill.none", "spreadsheet.picker.fill.red", "spreadsheet.picker.fill.orange", "spreadsheet.picker.fill.yellow", "spreadsheet.picker.fill.green", "spreadsheet.picker.fill.blue", "spreadsheet.picker.fill.purple", "spreadsheet.picker.fill.gray"}
	case pickerAlignment:
		ids = []string{"spreadsheet.picker.alignment.general", "spreadsheet.picker.alignment.left", "spreadsheet.picker.alignment.center", "spreadsheet.picker.alignment.right"}
	case pickerNumberFormat:
		ids = []string{"spreadsheet.picker.number_format.general", "spreadsheet.picker.number_format.integer", "spreadsheet.picker.number_format.number", "spreadsheet.picker.number_format.thousands", "spreadsheet.picker.number_format.percent_two_decimal", "spreadsheet.picker.number_format.percentage", "spreadsheet.picker.number_format.date", "spreadsheet.picker.number_format.date_mdy", "spreadsheet.picker.number_format.time", "spreadsheet.picker.number_format.scientific", "spreadsheet.picker.number_format.currency"}
	case pickerBorder:
		ids = []string{"spreadsheet.picker.border.all", "spreadsheet.picker.border.outside", "spreadsheet.picker.border.top", "spreadsheet.picker.border.right", "spreadsheet.picker.border.bottom", "spreadsheet.picker.border.left", "spreadsheet.picker.border.inside", "spreadsheet.picker.border.none"}
	}
	if index < 0 || index >= len(ids) {
		return "common.translation_missing"
	}
	return ids[index]
}

func spreadsheetToolbarNumberFormatMessageID(format string) string {
	label := spreadsheetToolbarNumberFormatLabel(format)
	switch label {
	case "常规":
		return "spreadsheet.picker.number_format.general"
	case "整数":
		return "spreadsheet.picker.number_format.integer"
	case "数值":
		return "spreadsheet.picker.number_format.number"
	case "千分位":
		return "spreadsheet.picker.number_format.thousands"
	case "百分比（两位）":
		return "spreadsheet.picker.number_format.percent_two_decimal"
	case "百分比":
		return "spreadsheet.picker.number_format.percentage"
	case "日期":
		return "spreadsheet.picker.number_format.date"
	case "日期（月/日/年）":
		return "spreadsheet.picker.number_format.date_mdy"
	case "时间":
		return "spreadsheet.picker.number_format.time"
	case "科学计数":
		return "spreadsheet.picker.number_format.scientific"
	case "货币":
		return "spreadsheet.picker.number_format.currency"
	default:
		return "spreadsheet.picker.number_format.custom"
	}
}

func formatStyleSlot(slot int) bool {
	return (slot >= 4 && slot <= 11) ||
		slot == toolbarActionFontFamily ||
		slot == toolbarActionFontSize ||
		slot == toolbarActionAlignLeft ||
		slot == toolbarActionAlignRight ||
		slot == toolbarActionWrapText ||
		slot == toolbarActionPercentFormat ||
		slot == toolbarActionCurrencyFormat
}

func spreadsheetToolbarFontFamily(style document.CellStyle) string {
	if family := strings.TrimSpace(style.FontFamily); family != "" {
		return family
	}
	return "Aptos Narrow"
}

func spreadsheetToolbarFontFamilyKey(family string) string {
	family = strings.ToLower(strings.TrimSpace(family))
	switch family {
	case "dengxian", "等线":
		return "dengxian"
	case "simsun", "宋体":
		return "simsun"
	case "microsoft yahei", "ms yahei", "微软雅黑":
		return "microsoft yahei"
	default:
		return family
	}
}

func spreadsheetToolbarFontSize(style document.CellStyle) float64 {
	if finite(style.FontSize) && style.FontSize > 0 {
		return style.FontSize
	}
	return 11
}

func spreadsheetIsCNCurrencyNumberFormat(format string) bool {
	format = strings.ToLower(strings.TrimSpace(format))
	return format == strings.ToLower(spreadsheetCurrencyNumberFormat) ||
		format == `¥#,##0.00` ||
		format == `[$¥-804]#,##0.00` ||
		format == `builtin:164` ||
		format == `builtin:193`
}

func spreadsheetToolbarNumberFormatLabel(format string) string {
	format = strings.TrimSpace(format)
	if spreadsheetIsCNCurrencyNumberFormat(format) {
		return "货币"
	}
	switch strings.ToLower(format) {
	case "", "general":
		return "常规"
	case "0":
		return "整数"
	case "0.00":
		return "数值"
	case "#,##0.00":
		return "千分位"
	case "0%", "0.00%":
		return "百分比"
	case "yyyy-mm-dd", "m/d/yy", "builtin:14":
		return "日期"
	case "h:mm AM/PM", "h:mm", "builtin:18", "builtin:20":
		return "时间"
	case "0.00E+00", "builtin:11":
		return "科学计数"
	default:
		return "自定义"
	}
}

func formatStyleActive(slot int, style document.CellStyle) bool {
	switch slot {
	case 4:
		return style.Bold
	case 5:
		return style.Italic
	case 6:
		return style.Underline
	case 7:
		return style.FontColor != ""
	case 8:
		return style.FillColor != ""
	case 9:
		return style.HorizontalAlignment == "center"
	case toolbarActionAlignLeft:
		return style.HorizontalAlignment == "left"
	case toolbarActionAlignRight:
		return style.HorizontalAlignment == "right"
	case toolbarActionWrapText:
		return style.WrapText
	case toolbarActionPercentFormat:
		return style.NumberFormat == "0%" || style.NumberFormat == "0.00%"
	case toolbarActionCurrencyFormat:
		return spreadsheetIsCNCurrencyNumberFormat(style.NumberFormat)
	case 10:
		return style.NumberFormat != "" && style.NumberFormat != "General"
	case 11:
		for _, b := range style.Borders {
			if b.Style != 0 {
				return true
			}
		}
	}
	return false
}

func (a *App) selectionFormatMixed(slot int) bool {
	if _, mixed, ok := a.spreadsheetTextSelectionStyle(slot); ok {
		return mixed
	}
	state := a.documentSessionState
	if state == nil || !state.selectionSet {
		return false
	}
	bounds := state.selection.Bounds()
	v := state.virtualGrid
	if v == nil || v.WindowSheet != state.sheet || !v.containsLoadedRange(bounds) {
		return true
	}
	base := v.cell(state.selection.Active.Row+1, state.selection.Active.Column+1).Style
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			current := v.cell(row, column).Style
			var equal bool
			switch slot {
			case 4:
				equal = current.Bold == base.Bold
			case 5:
				equal = current.Italic == base.Italic
			case 6:
				equal = current.Underline == base.Underline
			case 7:
				equal = current.FontColor == base.FontColor
			case 8:
				equal = current.FillColor == base.FillColor
			case 9, toolbarActionAlignLeft, toolbarActionAlignRight:
				equal = current.HorizontalAlignment == base.HorizontalAlignment
			case toolbarActionWrapText:
				equal = current.WrapText == base.WrapText
			case 10, toolbarActionPercentFormat, toolbarActionCurrencyFormat:
				equal = current.NumberFormat == base.NumberFormat
			case 11:
				equal = current.Borders == base.Borders
			case toolbarActionFontFamily:
				equal = spreadsheetToolbarFontFamilyKey(spreadsheetToolbarFontFamily(current)) == spreadsheetToolbarFontFamilyKey(spreadsheetToolbarFontFamily(base))
			case toolbarActionFontSize:
				equal = spreadsheetToolbarFontSize(current) == spreadsheetToolbarFontSize(base)
			}
			if !equal {
				return true
			}
		}
	}
	return false
}

type spreadsheetToolbarPicker uint8

const (
	pickerNone spreadsheetToolbarPicker = iota
	pickerFontColor
	pickerFillColor
	pickerAlignment
	pickerNumberFormat
	pickerBorder
)

func pickerForToolbarAction(slot int) spreadsheetToolbarPicker {
	switch slot {
	case 7:
		return pickerFontColor
	case 8:
		return pickerFillColor
	case 9:
		return pickerAlignment
	case 10:
		return pickerNumberFormat
	case 11:
		return pickerBorder
	}
	return pickerNone
}

func formatPatchForToolbarAction(slot int, style document.CellStyle) (document.CellFormatPatch, bool) {
	switch slot {
	case 4:
		v := !style.Bold
		return document.CellFormatPatch{Bold: &v}, true
	case 5:
		v := !style.Italic
		return document.CellFormatPatch{Italic: &v}, true
	case 6:
		v := !style.Underline
		return document.CellFormatPatch{Underline: &v}, true
	case 16:
		return document.CellFormatPatch{ClearFormat: true}, true
	case toolbarActionAlignLeft:
		align := "left"
		if strings.EqualFold(style.HorizontalAlignment, "left") {
			align = "general"
		}
		return document.CellFormatPatch{HorizontalAlignment: &align}, true
	case toolbarActionAlignRight:
		align := "right"
		if strings.EqualFold(style.HorizontalAlignment, "right") {
			align = "general"
		}
		return document.CellFormatPatch{HorizontalAlignment: &align}, true
	case toolbarActionWrapText:
		v := !style.WrapText
		return document.CellFormatPatch{WrapText: &v}, true
	case toolbarActionPercentFormat:
		format := "0.00%"
		if style.NumberFormat == "0.00%" || style.NumberFormat == "0%" {
			format = "General"
		}
		return document.CellFormatPatch{NumberFormat: &format}, true
	case toolbarActionCurrencyFormat:
		format := spreadsheetCurrencyNumberFormat
		return document.CellFormatPatch{NumberFormat: &format}, true
	case toolbarActionIncreaseDecimal:
		delta := 1
		return document.CellFormatPatch{DecimalPlacesDelta: &delta}, true
	case toolbarActionDecreaseDecimal:
		delta := -1
		return document.CellFormatPatch{DecimalPlacesDelta: &delta}, true
	case toolbarActionDecreaseFontSize:
		step := -1
		return document.CellFormatPatch{FontSizeStep: &step}, true
	case toolbarActionIncreaseFontSize:
		step := 1
		return document.CellFormatPatch{FontSizeStep: &step}, true
	default:
		return document.CellFormatPatch{}, false
	}
}

func toolbarPickerOptions(kind spreadsheetToolbarPicker) []string {
	switch kind {
	case pickerFontColor:
		return []string{"黑色", "红色", "橙色", "黄色", "绿色", "蓝色", "紫色", "白色"}
	case pickerFillColor:
		return []string{"无填充", "浅红", "浅橙", "浅黄", "浅绿", "浅蓝", "浅紫", "灰色"}
	case pickerAlignment:
		return []string{"常规", "左对齐", "居中", "右对齐"}
	case pickerNumberFormat:
		return []string{"常规", "整数", "数值", "千分位", "百分比（两位）", "百分比", "日期", "日期（月/日/年）", "时间", "科学计数", "货币"}
	case pickerBorder:
		return []string{"所有边框", "外侧边框", "上框线", "右框线", "下框线", "左框线", "内侧边框", "无边框"}
	default:
		return nil
	}
}

func toolbarPickerSwatch(kind spreadsheetToolbarPicker, index int) (color.NRGBA, bool, bool) {
	var value string
	switch kind {
	case pickerFontColor:
		values := [...]string{"000000", "F44336", "FF9800", "FFC107", "4CAF50", "2196F3", "9C27B0", "FFFFFF"}
		if index < 0 || index >= len(values) {
			return color.NRGBA{}, false, false
		}
		value = values[index]
	case pickerFillColor:
		values := [...]string{"", "F4CCCC", "FCE4D6", "FFF2CC", "E2F0D9", "DDEBF7", "E4DFEC", "D9E1F2"}
		if index < 0 || index >= len(values) {
			return color.NRGBA{}, false, false
		}
		value = values[index]
	default:
		return color.NRGBA{}, false, false
	}
	if value == "" {
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}, true, true
	}
	return parseGridColor(value, color.NRGBA{}), true, false
}

func toolbarPickerOptionSelected(kind spreadsheetToolbarPicker, index int, style document.CellStyle) bool {
	normalize := func(value string) string {
		return strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	}
	switch kind {
	case pickerFontColor:
		values := [...]string{"000000", "F44336", "FF9800", "FFC107", "4CAF50", "2196F3", "9C27B0", "FFFFFF"}
		if index < 0 || index >= len(values) {
			return false
		}
		current := normalize(style.FontColor)
		if current == "" {
			current = "000000"
		}
		return current == values[index]
	case pickerFillColor:
		values := [...]string{"", "F4CCCC", "FCE4D6", "FFF2CC", "E2F0D9", "DDEBF7", "E4DFEC", "D9E1F2"}
		if index < 0 || index >= len(values) {
			return false
		}
		return normalize(style.FillColor) == values[index]
	case pickerAlignment:
		values := [...]string{"general", "left", "center", "right"}
		if index < 0 || index >= len(values) {
			return false
		}
		current := strings.TrimSpace(style.HorizontalAlignment)
		if current == "" {
			current = "general"
		}
		return strings.EqualFold(current, values[index])
	case pickerNumberFormat:
		current := strings.TrimSpace(style.NumberFormat)
		if current == "" {
			current = "General"
		}
		switch index {
		case 0:
			return strings.EqualFold(current, "General")
		case 1:
			return current == "0"
		case 2:
			return current == "0.00"
		case 3:
			return current == "0%" || current == "0.00%"
		case 4:
			return strings.EqualFold(current, "yyyy-mm-dd") || strings.EqualFold(current, "m/d/yy") || strings.EqualFold(current, "builtin:14")
		case 5:
			return spreadsheetIsCNCurrencyNumberFormat(current)
		default:
			return false
		}
	case pickerBorder:
		if index < 0 || index > 7 {
			return false
		}
		top, right, bottom, left := style.Borders[0].Style != 0, style.Borders[1].Style != 0, style.Borders[2].Style != 0, style.Borders[3].Style != 0
		switch index {
		case 0, 1:
			return top && right && bottom && left
		case 2:
			return top
		case 3:
			return right
		case 4:
			return bottom
		case 5:
			return left
		case 7:
			return !top && !right && !bottom && !left
		default:
			return false
		}
	default:
		return false
	}
}

func toolbarPickerPatch(kind spreadsheetToolbarPicker, index int) (document.CellFormatPatch, bool) {
	values := map[spreadsheetToolbarPicker][]string{pickerFontColor: {"000000", "F44336", "FF9800", "FFC107", "4CAF50", "2196F3", "9C27B0", "FFFFFF"}, pickerFillColor: {"", "F4CCCC", "FCE4D6", "FFF2CC", "E2F0D9", "DDEBF7", "E4DFEC", "D9E1F2"}, pickerAlignment: {"general", "left", "center", "right"}, pickerNumberFormat: {"General", "0", "0.00", "#,##0.00", "0.00%", "0%", "yyyy-mm-dd", "m/d/yy", "h:mm AM/PM", "0.00E+00", spreadsheetCurrencyNumberFormat}, pickerBorder: {"all", "outside", "top", "right", "bottom", "left", "inside", "none"}}
	items, ok := values[kind]
	if !ok || index < 0 || index >= len(items) {
		return document.CellFormatPatch{}, false
	}
	v := items[index]
	switch kind {
	case pickerFontColor:
		return document.CellFormatPatch{FontColor: &v}, true
	case pickerFillColor:
		return document.CellFormatPatch{FillColor: &v}, true
	case pickerAlignment:
		return document.CellFormatPatch{HorizontalAlignment: &v}, true
	case pickerNumberFormat:
		return document.CellFormatPatch{NumberFormat: &v}, true
	case pickerBorder:
		return document.CellFormatPatch{BorderPreset: &v}, true
	}
	return document.CellFormatPatch{}, false
}
