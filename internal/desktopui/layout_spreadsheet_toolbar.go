package desktopui

import (
	"image/color"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

type spreadsheetToolbarTab uint8

const (
	toolbarTabHome spreadsheetToolbarTab = iota
	toolbarTabData
)

type spreadsheetToolbarAction struct {
	slot  int
	label string
	group string
}

var homeToolbarActions = []spreadsheetToolbarAction{
	{slot: 0, label: "撤销", group: "历史"},
	{slot: 1, label: "重做", group: "历史"},
	{slot: 2, label: "复制", group: "剪贴板"},
	{slot: 3, label: "剪切", group: "剪贴板"},
	{slot: 4, label: "加粗", group: "字体"},
	{slot: 5, label: "斜体", group: "字体"},
	{slot: 6, label: "下划线", group: "字体"},
	{slot: 7, label: "字体颜色", group: "字体"},
	{slot: 8, label: "填充颜色", group: "字体"},
	{slot: 9, label: "对齐", group: "格式"},
	{slot: 10, label: "数字格式", group: "格式"},
	{slot: 11, label: "边框", group: "格式"},
	{slot: 12, label: "插入行", group: "单元格"},
	{slot: 13, label: "删除行", group: "单元格"},
	{slot: 14, label: "插入列", group: "单元格"},
	{slot: 15, label: "删除列", group: "单元格"},
	{slot: 16, label: "清除格式", group: "单元格"},
	{slot: 17, label: "保存", group: "文件"},
	{slot: 21, label: "粘贴", group: "剪贴板"},
	{slot: 22, label: "新建工作表", group: "工作表"},
	{slot: 23, label: "重命名工作表", group: "工作表"},
	{slot: 24, label: "删除工作表", group: "工作表"},
}

var dataToolbarActions = []spreadsheetToolbarAction{
	{slot: 18, label: "升序", group: "排序和筛选"},
	{slot: 19, label: "降序", group: "排序和筛选"},
	{slot: 20, label: "筛选", group: "排序和筛选"},
}

var (
	disabledToolbarBackground = color.NRGBA{R: 232, G: 235, B: 240, A: 255}
	disabledToolbarText       = color.NRGBA{R: 132, G: 140, B: 151, A: 255}
	mixedToolbarBackground    = color.NRGBA{R: 250, G: 239, B: 204, A: 255}
	activeToolbarText         = color.NRGBA{R: 35, G: 82, B: 154, A: 255}
)

func (a *App) spreadsheetToolbarVisible() bool {
	if a == nil || a.book == nil || !a.book.Editable() {
		return false
	}
	_, ok := a.book.(document.GridReader)
	return ok
}

func (a *App) handleSpreadsheetToolbarEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	if state.toolbarTabs[0].Clicked(gtx) {
		state.toolbarTab = toolbarTabHome
	}
	if state.toolbarTabs[1].Clicked(gtx) {
		state.toolbarTab = toolbarTabData
	}
	actions := homeToolbarActions
	if state.toolbarTab == toolbarTabData {
		actions = dataToolbarActions
	}
	for _, action := range actions {
		if !state.toolbarActionButtons[action.slot].Clicked(gtx) || !a.toolbarActionEnabled(action.slot) {
			continue
		}
		if action.slot == 0 || action.slot == 1 {
			a.applyHistory(action.slot == 0)
			continue
		}
		if action.slot == 2 || action.slot == 3 {
			text, err := a.copySelection()
			if err == nil {
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
				if action.slot == 3 {
					a.clearSelectionAsync()
				}
			} else {
				a.status = "剪贴板操作失败：" + err.Error()
			}
			continue
		}
		if action.slot == 21 {
			gtx.Execute(clipboard.ReadCmd{Tag: &state.clipboardTarget})
			continue
		}
		if action.slot == 18 || action.slot == 19 {
			bounds := state.selection.Bounds()
			a.applySort(document.SortRequest{Sheet: state.sheet, Bounds: bounds, KeyColumn: state.selection.Active.Column + 1, Descending: action.slot == 19})
			continue
		}
		if action.slot == 20 {
			a.toggleValueFilter()
			continue
		}
		if action.slot == 17 {
			a.startSave()
			continue
		}
		if action.slot >= 12 && action.slot <= 15 {
			pos := state.selection.Active
			kind := []document.StructureEditKind{document.StructureInsertRows, document.StructureDeleteRows, document.StructureInsertColumns, document.StructureDeleteColumns}[action.slot-12]
			index := pos.Row + 1
			if action.slot >= 14 {
				index = pos.Column + 1
			}
			a.applyStructure(document.StructureEdit{Kind: kind, Sheet: state.sheet, Index: index, Count: 1})
			continue
		}
		if action.slot == 22 {
			a.applyStructure(document.StructureEdit{Kind: document.StructureCreateSheet})
			continue
		}
		if action.slot == 23 {
			a.applyStructure(document.StructureEdit{Kind: document.StructureRenameSheet, Sheet: state.sheet, NewName: state.sheetNameEditor.Text()})
			continue
		}
		if action.slot == 24 {
			a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteSheet, Sheet: state.sheet})
			continue
		}
		style := a.selectedCellStyle()
		if patch, ok := formatPatchForToolbarAction(action.slot, style); ok {
			a.applyCellFormat(patch)
			continue
		}
		if picker := pickerForToolbarAction(action.slot); picker != pickerNone {
			if state.toolbarPicker == picker {
				state.toolbarPicker = pickerNone
			} else {
				state.toolbarPicker = picker
			}
		}
	}
	if state.toolbarPicker != pickerNone {
		for index := range state.toolbarPickerButtons {
			if state.toolbarPickerButtons[index].Clicked(gtx) {
				if patch, ok := toolbarPickerPatch(state.toolbarPicker, index); ok {
					a.applyCellFormat(patch)
				}
				state.toolbarPicker = pickerNone
				break
			}
		}
	}
}

func (a *App) toolbarActionEnabled(slot int) bool {
	state := a.documentSessionState
	if state == nil {
		return false
	}
	switch slot {
	case 0, 1:
		_, ok := state.book.(document.SpreadsheetHistory)
		return ok && !state.formatRunning && !a.saving
	case 4, 5, 6, 7, 8, 9, 10, 11, 16:
		_, ok := state.book.(document.SpreadsheetFormatter)
		return ok && state.selectionSet && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.cellEdit.Editing && !state.formatRunning && !a.saving
	case 2, 3:
		_, reader := state.book.(document.SpreadsheetRangeReader)
		_, editor := state.book.(document.SpreadsheetBatchEditor)
		return reader && editor && state.selectionSet && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.formatRunning
	case 21:
		_, ok := state.book.(document.SpreadsheetBatchEditor)
		return ok && state.selectionSet && !state.formatRunning
	case 18, 19:
		_, ok := state.book.(document.SpreadsheetSorter)
		bounds := state.selection.Bounds()
		return ok && state.selectionSet && bounds.EndRow > bounds.StartRow && selectionCellCount(bounds) <= maxClipboardCellsUI && !state.formatRunning && !a.saving
	case 20:
		_, ok := state.book.(document.SpreadsheetRangeReader)
		return state.valueFilter != nil || (ok && state.selectionSet && state.selection.Bounds().EndRow > state.selection.Bounds().StartRow && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.formatRunning)
	case 12, 13, 14, 15, 22, 23, 24:
		_, ok := state.book.(document.SpreadsheetStructureEditor)
		if slot == 23 && strings.TrimSpace(state.sheetNameEditor.Text()) == "" {
			return false
		}
		if slot == 24 && len(state.sheets) <= 1 {
			return false
		}
		return ok && !state.formatRunning && !a.saving && ((slot < 22) == state.selectionSet || slot >= 22)
	case 17:
		return state.book != nil && state.book.Dirty() && !a.saving && state.path != ""
	default:
		return false
	}
}

func selectionCellCount(bounds document.CellRange) int {
	return (bounds.EndRow - bounds.StartRow + 1) * (bounds.EndColumn - bounds.StartColumn + 1)
}

func (a *App) selectedCellStyle() document.CellStyle {
	state := a.documentSessionState
	if state == nil || state.virtualGrid == nil || !state.selectionSet {
		return document.CellStyle{}
	}
	pos := state.selection.Active
	return windowCell(state.virtualGrid.Window, pos.Row+1, pos.Column+1).Style
}

func (a *App) layoutSpreadsheetToolbar(gtx layout.Context) layout.Dimensions {
	if !a.spreadsheetToolbarVisible() {
		return layout.Dimensions{}
	}
	height := gtx.Dp(unit.Dp(94))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	state := a.documentSessionState
	if state.toolbarPicker != pickerNone {
		height = gtx.Dp(unit.Dp(130))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, chromeSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return toolbarTabButton(gtx, a.theme, &state.toolbarTabs[0], "主页", state.toolbarTab == toolbarTabHome)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return toolbarTabButton(gtx, a.theme, &state.toolbarTabs[1], "数据", state.toolbarTab == toolbarTabData)
						}),
					)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					actions := homeToolbarActions
					if state.toolbarTab == toolbarTabData {
						actions = dataToolbarActions
					}
					return state.toolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
						action := actions[index]
						label := action.label
						style := a.selectedCellStyle()
						mixed := formatStyleSlot(action.slot) && a.selectionFormatMixed(action.slot)
						if mixed {
							label += " ◇"
						}
						button := material.Button(a.theme, &state.toolbarActionButtons[action.slot], label)
						enabled := a.toolbarActionEnabled(action.slot)
						if enabled {
							button.Background = contentSurface
							button.Color = mutedText
							if mixed {
								button.Background = mixedToolbarBackground
							}
							if !mixed && formatStyleActive(action.slot, style) {
								button.Background = selectedFill
								button.Color = activeToolbarText
							}
						} else {
							button.Background = disabledToolbarBackground
							button.Color = disabledToolbarText
						}
						button.TextSize = a.theme.TextSize * 12 / 16
						return layout.Inset{Left: unit.Dp(3), Right: unit.Dp(3), Top: unit.Dp(2), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Dp(unit.Dp(75))
									gtx.Constraints.Max.X = gtx.Dp(unit.Dp(75))
									return button.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									caption := material.Caption(a.theme, action.group)
									caption.Color = mutedText
									return layout.Center.Layout(gtx, caption.Layout)
								}),
							)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.toolbarPicker == pickerNone {
						return layout.Dimensions{}
					}
					options := toolbarPickerOptions(state.toolbarPicker)
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, func() []layout.FlexChild {
						children := make([]layout.FlexChild, 0, len(options))
						for i, option := range options {
							index, label := i, option
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								b := material.Button(a.theme, &state.toolbarPickerButtons[index], label)
								b.TextSize = a.theme.TextSize * 11 / 16
								return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, b.Layout)
							}))
						}
						return children
					}()...)
				}),
			)
		},
	)
}

func formatStyleSlot(slot int) bool { return slot >= 4 && slot <= 11 }

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
		return style.HorizontalAlignment != "" && style.HorizontalAlignment != "general"
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
			case 9:
				equal = current.HorizontalAlignment == base.HorizontalAlignment
			case 10:
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

func toolbarTabButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, selected bool) layout.Dimensions {
	style := material.Button(theme, button, label)
	if selected {
		style.Background = contentSurface
		style.Color = color.NRGBA{R: 35, G: 82, B: 154, A: 255}
	} else {
		style.Background = chromeSurface
		style.Color = mutedText
	}
	style.TextSize = theme.TextSize * 12 / 16
	return layout.Inset{Left: unit.Dp(5), Right: unit.Dp(5), Bottom: unit.Dp(2)}.Layout(gtx, style.Layout)
}
