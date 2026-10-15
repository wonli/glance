package desktopui

import (
	"glance/internal/document"
	"strings"
)

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

func formatStyleSlot(slot int) bool {
	return (slot >= 4 && slot <= 11) ||
		slot == toolbarActionAlignLeft ||
		slot == toolbarActionAlignRight ||
		slot == toolbarActionWrapText ||
		slot == toolbarActionPercentFormat
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
	state := a.documentSessionState
	if state == nil || !state.selectionSet {
		return false
	}
	bounds := state.selection.Bounds()
	v := state.virtualGrid
	if v == nil || v.WindowSheet != state.sheet || !containsRange(v.WindowRange, bounds) {
		return true
	}
	base := windowCell(v.Window, state.selection.Active.Row+1, state.selection.Active.Column+1).Style
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			current := windowCell(v.Window, row, column).Style
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
			case 10, toolbarActionPercentFormat:
				equal = current.NumberFormat == base.NumberFormat
			case 11:
				equal = current.Borders == base.Borders
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
		return []string{"常规", "整数", "两位小数", "日期"}
	case pickerBorder:
		return []string{"所有边框", "外侧边框", "下框线", "无边框"}
	default:
		return nil
	}
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
		values := [...]string{"", "FCE4D6", "FCE4D6", "FFF2CC", "E2F0D9", "DDEBF7", "E4DFEC", "D9E1F2"}
		if index < 0 || index >= len(values) {
			return false
		}
		for previous := 0; previous < index; previous++ {
			if values[previous] == values[index] {
				return false
			}
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
		values := [...]string{"General", "0", "0.00", "yyyy-mm-dd"}
		if index < 0 || index >= len(values) {
			return false
		}
		current := strings.TrimSpace(style.NumberFormat)
		if current == "" {
			current = "General"
		}
		return strings.EqualFold(current, values[index])
	default:
		return false
	}
}

func toolbarPickerPatch(kind spreadsheetToolbarPicker, index int) (document.CellFormatPatch, bool) {
	values := map[spreadsheetToolbarPicker][]string{pickerFontColor: {"000000", "F44336", "FF9800", "FFC107", "4CAF50", "2196F3", "9C27B0", "FFFFFF"}, pickerFillColor: {"", "FCE4D6", "FCE4D6", "FFF2CC", "E2F0D9", "DDEBF7", "E4DFEC", "D9E1F2"}, pickerAlignment: {"general", "left", "center", "right"}, pickerNumberFormat: {"General", "0", "0.00", "yyyy-mm-dd"}, pickerBorder: {"all", "outside", "bottom", "none"}}
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
