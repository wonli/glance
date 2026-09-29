package desktopui

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"strings"
	"sync"

	"gioui.org/io/clipboard"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
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
	toolbarTabInsert
	toolbarTabPageLayout
	toolbarTabFormula
	toolbarTabData
	toolbarTabReview
	toolbarTabView
)

var toolbarCategoryLabels = []string{"主页", "插入", "页面布局", "公式", "数据", "审阅", "视图"}

type spreadsheetToolbarAction struct {
	slot  int
	label string
	group string
	icon  string
}

var homeToolbarActions = []spreadsheetToolbarAction{
	{slot: 0, label: "撤销", group: "历史", icon: "undo"},
	{slot: 1, label: "重做", group: "历史", icon: "redo"},
	{slot: 2, label: "复制", group: "剪贴板", icon: "copy"},
	{slot: 3, label: "剪切", group: "剪贴板", icon: "cut"},
	{slot: 4, label: "加粗", group: "字体", icon: "bold"},
	{slot: 5, label: "斜体", group: "字体", icon: "italic"},
	{slot: 6, label: "下划线", group: "字体", icon: "underline"},
	{slot: 7, label: "字体颜色", group: "字体", icon: "type"},
	{slot: 8, label: "填充颜色", group: "字体", icon: "paint-bucket"},
	{slot: 9, label: "对齐", group: "格式", icon: "align-center"},
	{slot: 10, label: "数字格式", group: "格式", icon: "hash"},
	{slot: 11, label: "边框", group: "格式", icon: "grid"},
	{slot: 12, label: "插入行", group: "单元格", icon: "row-insert"},
	{slot: 13, label: "删除行", group: "单元格", icon: "row-delete"},
	{slot: 14, label: "插入列", group: "单元格", icon: "column-insert"},
	{slot: 15, label: "删除列", group: "单元格", icon: "column-delete"},
	{slot: 16, label: "清除格式", group: "单元格", icon: "clear"},
	{slot: 17, label: "保存", group: "文件", icon: "save"},
	{slot: 21, label: "粘贴", group: "剪贴板", icon: "paste"},
	{slot: 22, label: "新建工作表", group: "工作表", icon: "sheet-plus"},
	{slot: 23, label: "重命名工作表", group: "工作表", icon: "sheet-edit"},
	{slot: 24, label: "删除工作表", group: "工作表", icon: "sheet-trash"},
}

var dataToolbarActions = []spreadsheetToolbarAction{
	{slot: 18, label: "升序", group: "排序和筛选", icon: "sort-asc"},
	{slot: 19, label: "降序", group: "排序和筛选", icon: "sort-desc"},
	{slot: 20, label: "筛选", group: "排序和筛选", icon: "filter"},
}

// homeRibbonActions follows the Pen reference's five compact command groups.
// Negative slots are visual-only controls whose workbook operation is not
// implemented yet; rendering them as disabled keeps the layout honest.
var homeRibbonActions = []spreadsheetToolbarAction{
	{slot: 3, label: "剪切", group: "剪贴板", icon: "cut"},
	{slot: 2, label: "复制", group: "剪贴板", icon: "copy"},
	{slot: 21, label: "粘贴", group: "剪贴板", icon: "paste"},
	{slot: 7, label: "字体颜色", group: "字体", icon: "type"},
	{slot: -1, label: "减小字号", group: "字体", icon: "minus"},
	{slot: -2, label: "增大字号", group: "字体", icon: "plus"},
	{slot: 4, label: "加粗", group: "字体", icon: "bold"},
	{slot: 5, label: "斜体", group: "字体", icon: "italic"},
	{slot: 6, label: "下划线", group: "字体", icon: "underline"},
	{slot: -3, label: "左对齐", group: "对齐", icon: "menu"},
	{slot: 9, label: "居中对齐", group: "对齐", icon: "align-center-vertical"},
	{slot: -4, label: "右对齐", group: "对齐", icon: "list"},
	{slot: -5, label: "分散对齐", group: "对齐", icon: "grip-horizontal"},
	{slot: -6, label: "自动换行", group: "对齐", icon: "text-wrap"},
	{slot: 8, label: "填充颜色", group: "格式", icon: "paintbrush"},
	{slot: -7, label: "百分比格式", group: "格式", icon: "percent"},
	{slot: 10, label: "数字格式", group: "格式", icon: "hash"},
	{slot: 12, label: "插入行", group: "单元格", icon: "square-plus"},
	{slot: 13, label: "删除行", group: "单元格", icon: "trash-2"},
	{slot: 11, label: "单元格边框", group: "单元格", icon: "settings-2"},
}

var insertToolbarActions = []spreadsheetToolbarAction{
	{slot: 12, label: "插入行", group: "行列", icon: "row-insert"},
	{slot: 13, label: "删除行", group: "行列", icon: "row-delete"},
	{slot: 14, label: "插入列", group: "行列", icon: "column-insert"},
	{slot: 15, label: "删除列", group: "行列", icon: "column-delete"},
	{slot: 22, label: "新建工作表", group: "工作表", icon: "sheet-plus"},
	{slot: 23, label: "重命名工作表", group: "工作表", icon: "sheet-edit"},
	{slot: 24, label: "删除工作表", group: "工作表", icon: "sheet-trash"},
}

var reviewToolbarActions = []spreadsheetToolbarAction{
	{slot: 0, label: "撤销", group: "历史", icon: "undo"},
	{slot: 1, label: "重做", group: "历史", icon: "redo"},
	{slot: 17, label: "保存", group: "文件", icon: "save"},
	{slot: 16, label: "清除格式", group: "格式", icon: "clear"},
}

//go:embed assets/toolbar-icons.png
var toolbarIconSpritePNG []byte

var (
	toolbarIconSpriteOnce sync.Once
	toolbarIconSprite     *image.RGBA
	toolbarIconSpriteErr  error
	toolbarIconTintCache  = struct {
		sync.RWMutex
		icons map[toolbarIconTintKey]*image.NRGBA
	}{icons: make(map[toolbarIconTintKey]*image.NRGBA)}
)

type toolbarIconTintKey struct {
	name  string
	color color.NRGBA
}

func toolbarIconData(name string) (image.Image, bool) {
	cell, ok := toolbarIconCells[name]
	if !ok {
		return nil, false
	}
	toolbarIconSpriteOnce.Do(func() {
		decoded, err := png.Decode(bytes.NewReader(toolbarIconSpritePNG))
		if err != nil {
			toolbarIconSpriteErr = err
			return
		}
		toolbarIconSprite = image.NewRGBA(decoded.Bounds())
		draw.Draw(toolbarIconSprite, toolbarIconSprite.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	})
	if toolbarIconSpriteErr != nil || toolbarIconSprite == nil {
		return nil, false
	}
	const cellSize = 96
	return toolbarIconSprite.SubImage(image.Rect(cell.X*cellSize, cell.Y*cellSize, (cell.X+1)*cellSize, (cell.Y+1)*cellSize)), true
}

func tintedToolbarIcon(name string, tint color.NRGBA) (image.Image, bool) {
	key := toolbarIconTintKey{name: name, color: tint}
	toolbarIconTintCache.RLock()
	icon, ok := toolbarIconTintCache.icons[key]
	toolbarIconTintCache.RUnlock()
	if ok {
		return icon, true
	}
	mask, ok := toolbarIconData(name)
	if !ok {
		return nil, false
	}
	icon = tintToolbarIcon(mask, tint)
	toolbarIconTintCache.Lock()
	if cached, exists := toolbarIconTintCache.icons[key]; exists {
		icon = cached
	} else {
		toolbarIconTintCache.icons[key] = icon
	}
	toolbarIconTintCache.Unlock()
	return icon, true
}

func tintToolbarIcon(mask image.Image, tint color.NRGBA) *image.NRGBA {
	bounds := mask.Bounds()
	tinted := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			pixel := color.NRGBAModel.Convert(mask.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			tinted.SetNRGBA(x, y, color.NRGBA{R: tint.R, G: tint.G, B: tint.B, A: pixel.A})
		}
	}
	return tinted
}

func toolbarIconColor(enabled bool) color.NRGBA {
	if !enabled {
		return disabledToolbarText
	}
	return toolbarGlyphText
}

func toolbarPickerPanelX(slot int, actions []spreadsheetToolbarAction, position layout.Position, normalWidth, groupWidth, viewportWidth, panelWidth int) int {
	if viewportWidth <= 0 {
		return 0
	}
	maxX := viewportWidth - panelWidth
	if maxX < 0 {
		maxX = 0
	}
	target := -1
	for index, action := range actions {
		if action.slot == slot {
			target = index
			break
		}
	}
	if target < 0 || target < position.First {
		return 0
	}
	x := -position.Offset
	for index := position.First; index < target; index++ {
		width := normalWidth
		if index > 0 && actions[index-1].group != actions[index].group {
			width = groupWidth
		}
		x += width
	}
	if x < 0 {
		return 0
	}
	if x > maxX {
		return maxX
	}
	return x
}

var toolbarIconCells = map[string]image.Point{
	"cut": {X: 0, Y: 0}, "copy": {X: 1, Y: 0}, "paste": {X: 2, Y: 0}, "type": {X: 3, Y: 0},
	"minus": {X: 4, Y: 0}, "plus": {X: 5, Y: 0}, "bold": {X: 6, Y: 0}, "italic": {X: 7, Y: 0},
	"underline": {X: 0, Y: 1}, "align-left": {X: 1, Y: 1}, "align-center": {X: 2, Y: 1}, "align-right": {X: 3, Y: 1},
	"align-distributed": {X: 4, Y: 1}, "text-wrap": {X: 5, Y: 1}, "paintbrush": {X: 6, Y: 1}, "percent": {X: 7, Y: 1},
	"hash": {X: 0, Y: 2}, "square-plus": {X: 1, Y: 2}, "trash-2": {X: 2, Y: 2}, "settings-2": {X: 3, Y: 2},
	"undo": {X: 4, Y: 2}, "redo": {X: 5, Y: 2}, "save": {X: 6, Y: 2}, "clear": {X: 7, Y: 2},
	"row-insert": {X: 0, Y: 3}, "row-delete": {X: 1, Y: 3}, "column-insert": {X: 2, Y: 3}, "column-delete": {X: 3, Y: 3},
	"sheet-plus": {X: 4, Y: 3}, "sheet-edit": {X: 5, Y: 3}, "sheet-trash": {X: 6, Y: 3}, "sort-asc": {X: 7, Y: 3},
	"sort-desc": {X: 0, Y: 4}, "filter": {X: 1, Y: 4}, "house": {X: 2, Y: 4},
	"menu": {X: 0, Y: 5}, "align-center-vertical": {X: 1, Y: 5}, "list": {X: 2, Y: 5}, "grip-horizontal": {X: 3, Y: 5}, "sigma": {X: 4, Y: 5},
	"paint-bucket": {X: 6, Y: 1}, "grid": {X: 3, Y: 2},
}

var (
	disabledToolbarBackground = color.NRGBA{R: 232, G: 235, B: 240, A: 255}
	disabledToolbarText       = color.NRGBA{R: 150, G: 156, B: 165, A: 255}
	mixedToolbarBackground    = color.NRGBA{R: 239, G: 241, B: 243, A: 255}
	toolbarActiveBackground   = color.NRGBA{R: 241, G: 243, B: 245, A: 255}
	toolbarHoverBackground    = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	toolbarGlyphText          = color.NRGBA{R: 75, G: 83, B: 94, A: 255}
	ribbonDividerColor        = color.NRGBA{R: 225, G: 228, B: 232, A: 255}
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
	if state == nil || a.modal.Visible {
		return
	}
	for index := range toolbarCategoryLabels {
		if state.toolbarTabs[index].Clicked(gtx) {
			state.toolbarTab = spreadsheetToolbarTab(index)
			state.toolbarPicker = pickerNone
			state.toolbarActionList.List.Position = layout.Position{}
			break
		}
	}
	actions := toolbarActionsForTab(state.toolbarTab)
	for _, action := range actions {
		if action.slot < 0 {
			continue
		}
		if !state.toolbarActionButtons[action.slot].Clicked(gtx) || !a.toolbarActionEnabled(action.slot) {
			continue
		}
		if picker := pickerForToolbarAction(action.slot); picker != pickerNone {
			if state.toolbarPicker == picker {
				state.toolbarPicker = pickerNone
			} else {
				state.toolbarPicker = picker
				state.toolbarPickerSlot = action.slot
			}
			continue
		}
		state.toolbarPicker = pickerNone
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
			a.modal.ShowInput("重命名工作表", "输入新的工作表名称", state.sheet, "工作表名称", func(action string) {
				a.modal.Hide()
				if action != "confirm" {
					return
				}
				name := strings.TrimSpace(a.modal.Input.Text())
				if name == "" {
					a.status = "工作表名称不能为空"
					a.window.Invalidate()
					return
				}
				a.applyStructure(document.StructureEdit{Kind: document.StructureRenameSheet, Sheet: state.sheet, NewName: name})
			}, modalDialogActionSpec{ID: "confirm", Label: "重命名"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
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

func toolbarActionsForTab(tab spreadsheetToolbarTab) []spreadsheetToolbarAction {
	switch tab {
	case toolbarTabHome:
		return homeRibbonActions
	case toolbarTabInsert:
		return insertToolbarActions
	case toolbarTabData:
		return dataToolbarActions
	case toolbarTabReview:
		return reviewToolbarActions
	default:
		return nil
	}
}

func (a *App) toolbarActionEnabled(slot int) bool {
	state := a.documentSessionState
	if state == nil || slot < 0 || slot >= len(state.toolbarActionButtons) {
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
	height := gtx.Dp(unit.Dp(90))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	state := a.documentSessionState
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			shift := op.Offset(image.Pt(0, -gtx.Dp(unit.Dp(3)))).Push(gtx.Ops)
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			shift.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					shift := op.Offset(image.Pt(0, -gtx.Dp(unit.Dp(3)))).Push(gtx.Ops)
					defer shift.Pop()
					return layoutRibbonCategoryTabs(gtx, a.theme, &state.toolbarTabs, state.toolbarTab)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					stripHeight := gtx.Dp(unit.Dp(56))
					gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = stripHeight, stripHeight
					actions := toolbarActionsForTab(state.toolbarTab)
					shift := op.Offset(image.Pt(0, -gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
					defer shift.Pop()
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return state.toolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
							action := actions[index]
							style := a.selectedCellStyle()
							mixed := action.slot >= 0 && formatStyleSlot(action.slot) && a.selectionFormatMixed(action.slot)
							active := action.slot >= 0 && !mixed && formatStyleActive(action.slot, style)
							enabled := action.slot >= 0 && a.toolbarActionEnabled(action.slot)
							var button *widget.Clickable
							if action.slot >= 0 {
								button = &state.toolbarActionButtons[action.slot]
							}
							iconColor, background := toolbarIconColor(true), contentSurface
							if active {
								background = toolbarActiveBackground
							}
							if mixed {
								background = mixedToolbarBackground
							}
							if !enabled {
								background = contentSurface
							}
							icon, _ := tintedToolbarIcon(action.icon, iconColor)
							return layoutToolbarActionItem(gtx, action, index, actions, func(gtx layout.Context) layout.Dimensions {
								return toolbarIconButton(gtx, button, icon, action.label, enabled, mixed, background)
							})
						})
					})
				}),
			)
		},
	)
}

func layoutRibbonCategoryTabs(gtx layout.Context, theme *material.Theme, buttons *[7]widget.Clickable, selected spreadsheetToolbarTab) layout.Dimensions {
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	children := make([]layout.FlexChild, 0, len(toolbarCategoryLabels))
	for index, label := range toolbarCategoryLabels {
		index, label := index, label
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return toolbarTabButton(gtx, theme, &buttons[index], label, spreadsheetToolbarTab(index) == selected)
			})
		}))
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
		},
	)
}

func layoutToolbarActionItem(gtx layout.Context, action spreadsheetToolbarAction, index int, actions []spreadsheetToolbarAction, child layout.Widget) layout.Dimensions {
	groupStart := index == 0 || actions[index-1].group != action.group
	groupEnd := index == len(actions)-1 || actions[index+1].group != action.group
	children := make([]layout.FlexChild, 0, 5)
	if groupStart {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
		}))
	}
	children = append(children, layout.Rigid(child))
	gap := unit.Dp(6)
	if groupEnd {
		gap = unit.Dp(12)
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Dp(gap), 0)}
	}))
	if groupEnd && index < len(actions)-1 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(32)))
			gtx.Constraints.Max = gtx.Constraints.Min
			shift := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
			paint.FillShape(gtx.Ops, ribbonDividerColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
			shift.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (a *App) layoutSpreadsheetToolbarPickerOverlay(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.toolbarPicker == pickerNone {
		return layout.Dimensions{}
	}
	options := toolbarPickerOptions(state.toolbarPicker)
	if len(options) == 0 {
		return layout.Dimensions{}
	}
	actions := toolbarActionsForTab(state.toolbarTab)
	panelWidth := gtx.Dp(unit.Dp(276))
	if panelWidth > gtx.Constraints.Max.X {
		panelWidth = gtx.Constraints.Max.X
	}
	normalWidth := gtx.Dp(unit.Dp(42))
	groupWidth := gtx.Dp(unit.Dp(49))
	panelX := toolbarPickerPanelX(state.toolbarPickerSlot, actions, state.toolbarActionList.List.Position, normalWidth, groupWidth, gtx.Constraints.Max.X, panelWidth)
	panelX += gtx.Dp(unit.Dp(22))
	maxX := gtx.Constraints.Max.X - panelWidth
	if maxX < 0 {
		maxX = 0
	}
	if panelX > maxX {
		panelX = maxX
	}
	left := unit.Dp(0)
	if gtx.Metric.PxPerDp > 0 {
		left = unit.Dp(float32(panelX) / gtx.Metric.PxPerDp)
	}
	return layout.Inset{Left: left, Top: unit.Dp(94)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := panelWidth
		if width > gtx.Constraints.Max.X {
			width = gtx.Constraints.Max.X
		}
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return a.layoutToolbarPickerPanel(gtx, options)
	})
}

func (a *App) layoutToolbarPickerPanel(gtx layout.Context, options []string) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Min}
			shadowBounds := bounds.Add(image.Pt(1, 2))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 28, G: 38, B: 54, A: 28}, clip.UniformRRect(shadowBounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, gridLine, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			inner := bounds.Inset(1)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(inner, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := make([]layout.FlexChild, 0, (len(options)+1)/2+2)
				title := material.Body2(a.theme, toolbarPickerTitle(a.documentSessionState.toolbarPicker))
				title.Color = mutedText
				rows = append(rows, layout.Rigid(title.Layout))
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
				}))
				for row := 0; row < (len(options)+1)/2; row++ {
					rowIndex := row
					rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						children := make([]layout.FlexChild, 0, 2)
						for column := 0; column < 2; column++ {
							index := rowIndex*2 + column
							if index >= len(options) {
								children = append(children, layout.Flexed(1, func(layout.Context) layout.Dimensions { return layout.Dimensions{} }))
								continue
							}
							children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								button := material.Button(a.theme, &a.documentSessionState.toolbarPickerButtons[index], options[index])
								button.TextSize = a.theme.TextSize * 12 / 16
								button.Background = contentSurface
								button.Color = mutedText
								button.CornerRadius = 6
								button.Inset = layout.Inset{Left: unit.Dp(5), Right: unit.Dp(5), Top: unit.Dp(4), Bottom: unit.Dp(4)}
								return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, button.Layout)
							}))
						}
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		},
	)
}

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

func toolbarIconButton(gtx layout.Context, button *widget.Clickable, icon image.Image, label string, enabled, mixed bool, background color.NRGBA) layout.Dimensions {
	const buttonSize = unit.Dp(36)
	size := gtx.Dp(buttonSize)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	draw := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(label).Add(gtx.Ops)

		buttonBackground := background
		if button != nil && enabled && (button.Hovered() || gtx.Focused(button)) && !mixed {
			buttonBackground = toolbarHoverBackground
		}
		paint.FillShape(gtx.Ops, buttonBackground, clip.UniformRRect(image.Rectangle{Max: image.Pt(size, size)}, gtx.Dp(unit.Dp(7))).Op(gtx.Ops))
		if icon != nil {
			widget.Image{
				Src:      paint.NewImageOp(icon),
				Fit:      widget.Contain,
				Position: layout.Center,
				Scale:    20.0 / 96.0,
			}.Layout(gtx)
		}
		if mixed {
			dotSize := gtx.Dp(unit.Dp(5))
			inset := gtx.Dp(unit.Dp(5))
			dot := image.Rect(size-dotSize-inset, inset, size-inset, dotSize+inset)
			paint.FillShape(gtx.Ops, toolbarGlyphText, clip.Ellipse(dot).Op(gtx.Ops))
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	}
	if button == nil {
		return draw(gtx)
	}
	return button.Layout(gtx, draw)
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
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(30))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		style := material.Body2(theme, label)
		style.TextSize = unit.Sp(13)
		style.Color = color.NRGBA{R: 89, G: 98, B: 109, A: 255}
		if selected {
			style.Color = color.NRGBA{R: 53, G: 95, B: 150, A: 255}
		}
		dims := layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, style.Layout)
		})
		if selected {
			lineY := height - gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, style.Color, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(dims.Size.X, height)}.Op())
		}
		dims.Size.Y = height
		return dims
	})
}
