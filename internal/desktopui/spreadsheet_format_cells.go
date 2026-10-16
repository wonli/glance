package desktopui

import (
	"image/color"
	"strconv"
	"strings"

	"gioui.org/widget"
	"glance/internal/document"
)

type spreadsheetFormatCellsCategory uint8

const (
	spreadsheetFormatCellsNumber spreadsheetFormatCellsCategory = iota
	spreadsheetFormatCellsAlignment
	spreadsheetFormatCellsFont
	spreadsheetFormatCellsBorder
	spreadsheetFormatCellsFill
)

type spreadsheetFormatCellChoice struct {
	button widget.Clickable
	label  string
	value  string
}

type spreadsheetFormatCellColorChoice struct {
	button widget.Clickable
	label  string
	value  string
	swatch color.NRGBA
	none   bool
}

// spreadsheetFormatCellsDialogState keeps every clickable stable while the
// modal is open. Draft values are applied only after the user confirms.
type spreadsheetFormatCellsDialogState struct {
	initialized bool
	category    spreadsheetFormatCellsCategory
	categories  [5]widget.Clickable
	sheet       string
	selection   CellSelection

	numberChoices   []spreadsheetFormatCellChoice
	alignChoices    []spreadsheetFormatCellChoice
	verticalChoices []spreadsheetFormatCellChoice
	familyChoices   []spreadsheetFormatCellChoice
	sizeChoices     []spreadsheetFormatCellChoice
	borderChoices   []spreadsheetFormatCellChoice
	fontColors      []spreadsheetFormatCellColorChoice
	fillColors      []spreadsheetFormatCellColorChoice

	wrapButton             widget.Clickable
	boldButton             widget.Clickable
	italicButton           widget.Clickable
	underlineButton        widget.Clickable
	fontSize               float64
	numberFormat           string
	alignment              string
	verticalAlignment      string
	fontFamily             string
	fontColor              string
	fillColor              string
	borderPreset           string
	wrapText, bold, italic bool
	underline              bool
	numberTouched          bool
	alignmentTouched       bool
	verticalTouched        bool
	wrapTouched            bool
	familyTouched          bool
	sizeTouched            bool
	boldTouched            bool
	italicTouched          bool
	underlineTouched       bool
	fontColorTouched       bool
	fillTouched            bool
	borderTouched          bool
}

func (s *spreadsheetFormatCellsDialogState) init() {
	if s.initialized {
		return
	}
	s.initialized = true
	s.numberChoices = []spreadsheetFormatCellChoice{
		{label: "常规", value: "General"},
		{label: "整数", value: "0"},
		{label: "数值（两位小数）", value: "0.00"},
		{label: "数值（千位分隔，两位小数）", value: "#,##0.00"},
		{label: "百分比", value: "0%"},
		{label: "百分比（两位小数）", value: "0.00%"},
		{label: "日期（年-月-日）", value: "yyyy-mm-dd"},
		{label: "日期（月/日/年）", value: "m/d/yy"},
		{label: "时间（12 小时制）", value: "h:mm AM/PM"},
		{label: "科学计数（两位小数）", value: "0.00E+00"},
		{label: "货币（人民币，两位小数）", value: spreadsheetCurrencyNumberFormat},
	}
	s.alignChoices = []spreadsheetFormatCellChoice{
		{label: "常规", value: "general"},
		{label: "左对齐", value: "left"},
		{label: "居中", value: "center"},
		{label: "右对齐", value: "right"},
	}
	s.verticalChoices = []spreadsheetFormatCellChoice{
		{label: "顶端", value: "top"},
		{label: "居中", value: "center"},
		{label: "底端", value: "bottom"},
	}
	s.familyChoices = []spreadsheetFormatCellChoice{
		{label: "Aptos Narrow", value: "Aptos Narrow"},
		{label: "等线", value: "等线"},
		{label: "宋体", value: "宋体"},
		{label: "微软雅黑", value: "Microsoft YaHei"},
		{label: "Arial", value: "Arial"},
		{label: "Calibri", value: "Calibri"},
	}
	for _, size := range []float64{9, 10, 11, 12, 14, 16, 18, 20, 24} {
		s.sizeChoices = append(s.sizeChoices, spreadsheetFormatCellChoice{label: formatSpreadsheetFontSize(size), value: formatSpreadsheetFontSize(size)})
	}
	s.borderChoices = []spreadsheetFormatCellChoice{
		{label: "所有边框", value: "all"},
		{label: "外侧边框", value: "outside"},
		{label: "下框线", value: "bottom"},
		{label: "无边框", value: "none"},
	}
	s.fontColors = []spreadsheetFormatCellColorChoice{
		{label: "黑色", value: "000000", swatch: color.NRGBA{R: 0, G: 0, B: 0, A: 255}},
		{label: "红色", value: "F44336", swatch: color.NRGBA{R: 244, G: 67, B: 54, A: 255}},
		{label: "橙色", value: "FF9800", swatch: color.NRGBA{R: 255, G: 152, B: 0, A: 255}},
		{label: "黄色", value: "FFC107", swatch: color.NRGBA{R: 255, G: 193, B: 7, A: 255}},
		{label: "绿色", value: "4CAF50", swatch: color.NRGBA{R: 76, G: 175, B: 80, A: 255}},
		{label: "蓝色", value: "2196F3", swatch: color.NRGBA{R: 33, G: 150, B: 243, A: 255}},
		{label: "紫色", value: "9C27B0", swatch: color.NRGBA{R: 156, G: 39, B: 176, A: 255}},
		{label: "白色", value: "FFFFFF", swatch: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	}
	s.fillColors = []spreadsheetFormatCellColorChoice{
		{label: "无填充", value: "", swatch: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, none: true},
		{label: "浅红", value: "F4CCCC", swatch: color.NRGBA{R: 244, G: 204, B: 204, A: 255}},
		{label: "浅橙", value: "FCE4D6", swatch: color.NRGBA{R: 252, G: 228, B: 214, A: 255}},
		{label: "浅黄", value: "FFF2CC", swatch: color.NRGBA{R: 255, G: 242, B: 204, A: 255}},
		{label: "浅绿", value: "E2F0D9", swatch: color.NRGBA{R: 226, G: 240, B: 217, A: 255}},
		{label: "浅蓝", value: "DDEBF7", swatch: color.NRGBA{R: 221, G: 235, B: 247, A: 255}},
		{label: "浅紫", value: "E4DFEC", swatch: color.NRGBA{R: 228, G: 223, B: 236, A: 255}},
		{label: "浅灰", value: "D9E1F2", swatch: color.NRGBA{R: 217, G: 225, B: 242, A: 255}},
	}
}

func formatSpreadsheetFontSize(size float64) string {
	return strconv.FormatFloat(size, 'f', -1, 64)
}

func (a *App) showSpreadsheetFormatCellsDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil || !a.toolbarActionEnabled(4) {
		return
	}
	if _, ok := state.book.(document.SpreadsheetFormatter); !ok {
		return
	}
	draft := &state.spreadsheetFormatCells
	draft.init()
	draft.category = spreadsheetFormatCellsNumber
	draft.sheet, draft.selection = state.sheet, state.selection
	style := a.selectedCellStyle()
	draft.numberFormat = style.NumberFormat
	if draft.numberFormat == "" {
		draft.numberFormat = "General"
	}
	draft.alignment = style.HorizontalAlignment
	if draft.alignment == "" {
		draft.alignment = "general"
	}
	draft.verticalAlignment = style.VerticalAlignment
	if draft.verticalAlignment == "" {
		draft.verticalAlignment = "center"
	}
	draft.fontFamily = style.FontFamily
	if draft.fontFamily == "" {
		draft.fontFamily = "Aptos Narrow"
	}
	draft.fontSize = style.FontSize
	if draft.fontSize <= 0 {
		draft.fontSize = 11
	}
	draft.fontColor = strings.TrimPrefix(strings.TrimPrefix(strings.ToUpper(style.FontColor), "#"), "#")
	if draft.fontColor == "" {
		draft.fontColor = "000000"
	}
	draft.fillColor = strings.TrimPrefix(strings.TrimPrefix(strings.ToUpper(style.FillColor), "#"), "#")
	draft.borderPreset = spreadsheetBorderPresetForStyle(style)
	draft.wrapText = style.WrapText
	draft.bold, draft.italic, draft.underline = style.Bold, style.Italic, style.Underline
	draft.resetTouched()

	selectionMessage := "仅修改下方选项；未改动的格式会保留。"
	a.modal.ShowCustom("设置单元格格式", selectionMessage, 720, a.layoutSpreadsheetFormatCellsDialog, func(action string) {
		a.modal.Hide()
		if state.virtualGrid != nil {
			state.virtualGrid.KeyboardFocusRequested = true
		}
		if action != "confirm" {
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		patch, changed := draft.patch()
		if !changed {
			a.setStatus("格式未更改")
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		if a.documentSessionState == state {
			a.applyCellFormatTo(state, draft.sheet, draft.selection.Bounds(), patch)
		}
	})
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (s *spreadsheetFormatCellsDialogState) resetTouched() {
	s.numberTouched = false
	s.alignmentTouched = false
	s.verticalTouched = false
	s.wrapTouched = false
	s.familyTouched = false
	s.sizeTouched = false
	s.boldTouched = false
	s.italicTouched = false
	s.underlineTouched = false
	s.fontColorTouched = false
	s.fillTouched = false
	s.borderTouched = false
}

func spreadsheetBorderPresetForStyle(style document.CellStyle) string {
	top, right, bottom, left := style.Borders[0].Style != 0, style.Borders[1].Style != 0, style.Borders[2].Style != 0, style.Borders[3].Style != 0
	switch {
	case top && right && bottom && left:
		return ""
	case !top && !right && bottom && !left:
		return "bottom"
	case !top && !right && !bottom && !left:
		return "none"
	default:
		return ""
	}
}

func (s *spreadsheetFormatCellsDialogState) patch() (document.CellFormatPatch, bool) {
	var patch document.CellFormatPatch
	if s.numberTouched {
		value := s.numberFormat
		patch.NumberFormat = &value
	}
	if s.alignmentTouched {
		value := s.alignment
		patch.HorizontalAlignment = &value
	}
	if s.verticalTouched {
		value := s.verticalAlignment
		patch.VerticalAlignment = &value
	}
	if s.wrapTouched {
		value := s.wrapText
		patch.WrapText = &value
	}
	if s.familyTouched {
		value := s.fontFamily
		patch.FontFamily = &value
	}
	if s.sizeTouched {
		value := s.fontSize
		patch.FontSize = &value
	}
	if s.boldTouched {
		value := s.bold
		patch.Bold = &value
	}
	if s.italicTouched {
		value := s.italic
		patch.Italic = &value
	}
	if s.underlineTouched {
		value := s.underline
		patch.Underline = &value
	}
	if s.fontColorTouched {
		value := s.fontColor
		patch.FontColor = &value
	}
	if s.fillTouched {
		value := s.fillColor
		patch.FillColor = &value
	}
	if s.borderTouched {
		value := s.borderPreset
		patch.BorderPreset = &value
	}
	return patch, patch.Bold != nil || patch.Italic != nil || patch.Underline != nil || patch.FontColor != nil || patch.FillColor != nil || patch.HorizontalAlignment != nil || patch.VerticalAlignment != nil || patch.NumberFormat != nil || patch.BorderPreset != nil || patch.WrapText != nil || patch.FontSize != nil || patch.FontFamily != nil
}

func parseSpreadsheetFontSize(value string) float64 {
	size, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 11
	}
	return size
}
