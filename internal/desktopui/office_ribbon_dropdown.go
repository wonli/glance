package desktopui

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

const officeRibbonMenuButtonCount = 12

type officeRibbonMenuKind uint8

const (
	officeRibbonMenuClosed officeRibbonMenuKind = iota
	officeRibbonMenuWord
	officeRibbonMenuSpreadsheet
)

type officeRibbonDropdownState struct {
	kind       officeRibbonMenuKind
	wordAction string
	sheetSlot  int
	bounds     image.Rectangle
	anchor     image.Rectangle
	dismiss    widget.Clickable
	buttons    [officeRibbonMenuButtonCount]widget.Clickable
}

type officeRibbonMenuOption struct {
	label   string
	command string
	enabled bool
	checked bool
}

type officeRibbonMenuDefinition struct {
	title   string
	options []officeRibbonMenuOption
}

func (a *App) officeRibbonMenuDefinition() officeRibbonMenuDefinition {
	state := a.documentSessionState
	if state == nil {
		return officeRibbonMenuDefinition{}
	}
	switch state.officeRibbonMenu.kind {
	case officeRibbonMenuWord:
		return wordOfficeRibbonMenu(state.officeRibbonMenu.wordAction)
	case officeRibbonMenuSpreadsheet:
		return spreadsheetOfficeRibbonMenu(state.officeRibbonMenu.sheetSlot)
	default:
		return officeRibbonMenuDefinition{}
	}
}

func (a *App) officeRibbonMenuOptionEnabled(option officeRibbonMenuOption) bool {
	if !option.enabled || option.command == "" {
		return false
	}
	state := a.documentSessionState
	if state == nil {
		return false
	}
	switch state.officeRibbonMenu.kind {
	case officeRibbonMenuWord:
		if option.command == "paste-text" {
			return a.wordToolbarActionEnabled(ribbonAction{id: "paste"})
		}
		if option.command == "select-all" {
			return state.word != nil && len(wordBlocksForState(state)) > 0
		}
		return a.wordToolbarActionEnabled(ribbonAction{id: option.command})
	case officeRibbonMenuSpreadsheet:
		switch option.command {
		case "excel-open":
			return a.toolbarActionEnabled(toolbarActionOpenFile)
		case "excel-recent":
			return state.book != nil && !state.formatRunning.Load()
		case "excel-paste-text":
			return a.toolbarActionEnabled(21)
		case "excel-font-dengxian", "excel-font-simsun", "excel-font-yahei", "excel-font-arial", "excel-font-custom":
			return a.toolbarActionEnabled(toolbarActionFontFamily)
		case "excel-size-10", "excel-size-11", "excel-size-12", "excel-size-14", "excel-size-16", "excel-size-custom":
			return a.toolbarActionEnabled(toolbarActionFontSize)
		case "excel-style-normal", "excel-style-emphasis", "excel-style-title":
			return a.toolbarActionEnabled(toolbarActionCellStyles)
		case "excel-delete-row", "excel-delete-column":
			return a.toolbarActionEnabled(13)
		case "excel-delete-sheet":
			_, editable := state.book.(document.SpreadsheetStructureEditor)
			return editable && len(state.sheets) > 1 && !state.formatRunning.Load() && !a.isSaving()
		case "excel-sum":
			return a.toolbarActionEnabled(toolbarActionSum)
		case "excel-sort-asc":
			return a.toolbarActionEnabled(18)
		case "excel-sort-desc":
			return a.toolbarActionEnabled(19)
		case "excel-filter", "excel-clear-filter":
			return a.toolbarActionEnabled(20)
		case "excel-find":
			return a.toolbarActionEnabled(toolbarActionFind)
		case "excel-insert-table":
			return a.toolbarActionEnabled(toolbarActionInsertTable)
		case "excel-margin-normal", "excel-margin-narrow", "excel-margin-wide", "excel-margin-custom":
			return a.toolbarActionEnabled(toolbarActionLayoutMargins)
		case "excel-view-normal":
			return a.toolbarActionEnabled(toolbarActionNormalView)
		case "excel-view-page-break":
			return a.toolbarActionEnabled(toolbarActionPageBreakPreview)
		default:
			return false
		}
	default:
		return false
	}
}

func (a *App) toggleWordOfficeRibbonMenu(actionID string) bool {
	definition := wordOfficeRibbonMenu(actionID)
	state := a.documentSessionState
	if state == nil || definition.title == "" {
		return false
	}
	menu := &state.officeRibbonMenu
	if menu.kind == officeRibbonMenuWord && menu.wordAction == actionID {
		menu.kind = officeRibbonMenuClosed
		menu.wordAction = ""
		return true
	}
	menu.kind = officeRibbonMenuWord
	menu.wordAction = actionID
	menu.sheetSlot = 0
	return true
}

func (a *App) toggleSpreadsheetOfficeRibbonMenu(slot int) bool {
	definition := spreadsheetOfficeRibbonMenu(slot)
	state := a.documentSessionState
	if state == nil || definition.title == "" {
		return false
	}
	menu := &state.officeRibbonMenu
	if menu.kind == officeRibbonMenuSpreadsheet && menu.sheetSlot == slot {
		menu.kind = officeRibbonMenuClosed
		menu.wordAction = ""
		return true
	}
	menu.kind = officeRibbonMenuSpreadsheet
	menu.sheetSlot = slot
	menu.wordAction = ""
	return true
}

func (a *App) closeOfficeRibbonMenu() {
	if state := a.documentSessionState; state != nil {
		state.officeRibbonMenu.kind = officeRibbonMenuClosed
		state.officeRibbonMenu.wordAction = ""
		state.officeRibbonMenu.bounds = image.Rectangle{}
		state.officeRibbonMenu.anchor = image.Rectangle{}
	}
}

func (a *App) handleOfficeRibbonMenuEvents(gtx layout.Context) bool {
	state := a.documentSessionState
	if state == nil || state.officeRibbonMenu.kind == officeRibbonMenuClosed {
		return false
	}
	if state.officeRibbonMenu.dismiss.Clicked(gtx) {
		history := state.officeRibbonMenu.dismiss.History()
		if len(history) != 0 {
			point := history[len(history)-1].Position
			if !point.In(state.officeRibbonMenu.bounds) && !point.In(state.officeRibbonMenu.anchor) {
				a.closeOfficeRibbonMenu()
			}
		}
	}
	definition := a.officeRibbonMenuDefinition()
	for index, option := range definition.options {
		if index >= len(state.officeRibbonMenu.buttons) {
			break
		}
		if a.officeRibbonMenuOptionEnabled(option) && state.officeRibbonMenu.buttons[index].Clicked(gtx) {
			kind := state.officeRibbonMenu.kind
			command := option.command
			a.closeOfficeRibbonMenu()
			a.executeOfficeRibbonMenuCommand(gtx, kind, command)
			return true
		}
	}
	return false
}

func (a *App) executeOfficeRibbonMenuCommand(gtx layout.Context, kind officeRibbonMenuKind, command string) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	if kind == officeRibbonMenuWord {
		switch command {
		case "paste-text":
			state.wordClipboardRunID = state.selectedWordRunID
			if wordHasPendingTextDrafts(state) {
				state.wordForceDraftFlush.Store(true)
				state.wordFlushRequested.Store(true)
				state.wordFlushAllDrafts.Store(true)
			}
			gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
		case "select-all":
		default:
		}
		if command != "paste-text" && wordHasPendingTextDrafts(state) {
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			if command == "save" {
				state.wordSaveAfterDrafts = true
				state.wordSaveAfterDraftsReason = wordSaveManual
			} else {
				state.wordDeferredToolbarAction = command
			}
			return
		}
		if command == "select-all" {
			a.selectAllWordText()
		} else if command != "paste-text" {
			a.executeWordToolbarAction(command)
		}
		return
	}
	if kind != officeRibbonMenuSpreadsheet {
		return
	}
	switch command {
	case "excel-open":
		a.startOpenFilePicker()
	case "excel-recent":
		a.activateRecent()
	case "excel-paste-text":
		gtx.Execute(clipboard.ReadCmd{Tag: &state.clipboardTarget})
	case "excel-font-dengxian", "excel-font-simsun", "excel-font-yahei", "excel-font-arial":
		family := map[string]string{"excel-font-dengxian": "等线", "excel-font-simsun": "宋体", "excel-font-yahei": "Microsoft YaHei", "excel-font-arial": "Arial"}[command]
		a.applyCellFormat(document.CellFormatPatch{FontFamily: &family})
	case "excel-font-custom":
		a.showSpreadsheetFontFamilyDialog()
	case "excel-size-10", "excel-size-11", "excel-size-12", "excel-size-14", "excel-size-16":
		size, _ := strconv.ParseFloat(strings.TrimPrefix(command, "excel-size-"), 64)
		a.applyCellFormat(document.CellFormatPatch{FontSize: &size})
	case "excel-size-custom":
		a.modal.ShowInput("设置字号", "输入字号（磅）", "11", "1–409 之间", func(action string) {
			value := strings.TrimSpace(a.modal.Input.Text())
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			size, err := strconv.ParseFloat(value, 64)
			if err != nil || size < 1 || size > 409 {
				a.setStatus("字号必须是 1 到 409 之间的数值")
				if a.window != nil {
					a.window.Invalidate()
				}
				return
			}
			a.applyCellFormat(document.CellFormatPatch{FontSize: &size})
		}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
	case "excel-style-normal":
		a.applyCellFormat(document.CellFormatPatch{ClearFormat: true})
	case "excel-style-emphasis":
		bold := true
		a.applyCellFormat(document.CellFormatPatch{Bold: &bold})
	case "excel-style-title":
		bold, size := true, 16.0
		a.applyCellFormat(document.CellFormatPatch{Bold: &bold, FontSize: &size})
	case "excel-delete-row":
		a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteRows, Sheet: state.sheet, Index: state.selection.Active.Row + 1, Count: 1})
	case "excel-delete-column":
		a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteColumns, Sheet: state.sheet, Index: state.selection.Active.Column + 1, Count: 1})
	case "excel-delete-sheet":
		a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteSheet, Sheet: state.sheet})
	case "excel-sum":
		a.applySpreadsheetAutoSum()
	case "excel-sort-asc", "excel-sort-desc":
		bounds := state.selection.Bounds()
		a.applySort(document.SortRequest{Sheet: state.sheet, Bounds: bounds, KeyColumn: state.selection.Active.Column + 1, Descending: command == "excel-sort-desc"})
	case "excel-filter":
		a.toggleValueFilter()
	case "excel-clear-filter":
		if state.valueFilter != nil {
			a.toggleValueFilter()
		}
	case "excel-find":
		a.showSpreadsheetFindDialog()
	case "excel-insert-table":
		a.showSpreadsheetInsertTableDialog("")
	case "excel-margin-normal", "excel-margin-narrow", "excel-margin-wide":
		top, side := 0.75, 0.7
		label := "普通"
		if command == "excel-margin-narrow" {
			side, label = 0.25, "窄"
		} else if command == "excel-margin-wide" {
			top, side, label = 1, 1, "宽"
		}
		sheet := state.sheet
		a.runSpreadsheetPageLayoutCommand("设置"+label+"页边距", showSpreadsheetMarginsDialogArguments{Sheet: sheet, Operation: "margins", MarginTop: top, MarginRight: side, MarginBottom: top, MarginLeft: side}, func(editor document.SpreadsheetPageLayoutEditor) error {
			return editor.SetSheetPageMargins(sheet, top, side, top, side)
		})
	case "excel-margin-custom":
		a.showSpreadsheetMarginsDialog()
	case "excel-view-normal":
		a.setSpreadsheetViewMode("normal")
	case "excel-view-page-break":
		a.setSpreadsheetViewMode("pageBreakPreview")
	}
}

func (a *App) layoutOfficeRibbonDropdown(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.officeRibbonMenu.kind == officeRibbonMenuClosed {
		return layout.Dimensions{}
	}
	definition := a.officeRibbonMenuDefinition()
	if definition.title == "" {
		return layout.Dimensions{}
	}
	panelWidth := min(gtx.Dp(unit.Dp(272)), gtx.Constraints.Max.X)
	x := gtx.Dp(12)
	if state.officeRibbonMenu.kind == officeRibbonMenuSpreadsheet {
		actions := toolbarActionsForTab(state.toolbarTab)
		if state.toolbarTab == toolbarTabHome {
			groups := spreadsheetHomeRibbonGroups(actions)
			first := max(0, min(state.toolbarGroupList.List.Position.First, len(groups)))
			scrollDP := 0
			for i := 0; i < first; i++ {
				scrollDP += groups[i].widthDP + 4
			}
			scrollPX := gtx.Dp(unit.Dp(scrollDP)) + state.toolbarGroupList.List.Position.Offset
			x = gtx.Dp(unit.Dp(80+spreadsheetHomeActionAnchorOffset(groups, state.officeRibbonMenu.sheetSlot))) - scrollPX
		} else {
			x = spreadsheetPenCommandX(gtx, state, state.officeRibbonMenu.sheetSlot)
		}
	} else {
		x = wordPenCommandX(gtx, state, state.officeRibbonMenu.wordAction)
	}
	maxX := max(0, gtx.Constraints.Max.X-panelWidth)
	anchorX := x
	x = min(max(0, x), maxX)
	y := gtx.Dp(unit.Dp(wordRibbonHeightDP + 2))
	rowHeight := gtx.Dp(unit.Dp(20))
	desiredHeight := gtx.Dp(unit.Dp(8)) + len(definition.options)*rowHeight
	bottomLimit := max(0, gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(12)))
	if y+desiredHeight > bottomLimit {
		y = max(0, bottomLimit-desiredHeight)
	}
	panelHeight := min(desiredHeight, max(0, bottomLimit-y))
	state.officeRibbonMenu.bounds = image.Rect(x, y, x+panelWidth, y+panelHeight)
	anchorLeft := max(0, anchorX-gtx.Dp(unit.Dp(4)))
	anchorTop := gtx.Dp(unit.Dp(wordRibbonCategoryHeightDP))
	state.officeRibbonMenu.anchor = image.Rect(anchorLeft, anchorTop, min(gtx.Constraints.Max.X, anchorLeft+gtx.Dp(unit.Dp(84))), gtx.Dp(unit.Dp(wordRibbonHeightDP)))
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			pass := pointer.PassOp{}.Push(gtx.Ops)
			defer pass.Pop()
			return state.officeRibbonMenu.dismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			defer transform.Pop()
			gtx.Constraints = layout.Constraints{Min: image.Pt(panelWidth, panelHeight), Max: image.Pt(panelWidth, panelHeight)}
			return a.layoutOfficeRibbonDropdownPanel(gtx, definition)
		}),
	)
}

func (a *App) layoutOfficeRibbonDropdownPanel(gtx layout.Context, definition officeRibbonMenuDefinition) layout.Dimensions {
	colors := a.tokens().Colors
	rowHeight := gtx.Dp(unit.Dp(20))
	height := gtx.Constraints.Max.Y
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shadow := bounds.Add(image.Pt(1, 2))
			paint.FillShape(gtx.Ops, colors.Menu.Shadow, clip.UniformRRect(shadow, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.Menu.Border, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.Menu.Surface, clip.UniformRRect(bounds.Inset(1), gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(definition.options))
				for index, option := range definition.options {
					index, option := index, option
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if index >= len(a.documentSessionState.officeRibbonMenu.buttons) {
							return layout.Dimensions{}
						}
						button := &a.documentSessionState.officeRibbonMenu.buttons[index]
						enabled := a.officeRibbonMenuOptionEnabled(option)
						gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, rowHeight))
						label := option.label
						if option.checked {
							label = "✓  " + label
						}
						draw := func(gtx layout.Context) layout.Dimensions {
							background := colors.Menu.Surface
							textColor := colors.Text.PureBlack
							if enabled && option.checked {
								background = colors.Menu.Selected
							} else if enabled && button.Hovered() {
								background = colors.Menu.Hover
							}
							return layout.Background{}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
									return layout.Dimensions{Size: gtx.Constraints.Max}
								},
								func(gtx layout.Context) layout.Dimensions {
									style := material.Body2(a.theme, label)
									style.TextSize = unit.Sp(12)
									style.Color = textColor
									return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions { gtx.Constraints.Min.Y = 0; return style.Layout(gtx) }))
									})
								},
							)
						}
						if enabled {
							return button.Layout(gtx, draw)
						}
						return draw(gtx)
					}))
				}
				return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
			})
		},
	)
}
