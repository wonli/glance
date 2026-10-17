package desktopui

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *App) handleSpreadsheetToolbarEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || a.modal.Visible {
		return
	}
	if a.handleOfficeRibbonMenuEvents(gtx) {
		return
	}
	labels := toolbarCategoryLabelsForState(state)
	for index := 0; index < len(labels) && index < len(state.toolbarTabs); index++ {
		if state.toolbarTabs[index].Clicked(gtx) {
			a.closeOfficeRibbonMenu()
			state.toolbarTab = spreadsheetToolbarTab(index)
			state.toolbarPicker = pickerNone
			state.toolbarActionList.List.Position = layout.Position{}
			state.toolbarGroupList.List.Position = layout.Position{}
			break
		}
	}
	actions := toolbarActionsForTab(state.toolbarTab)
	for _, action := range actions {
		if action.slot < 0 || !state.toolbarActionButtons[action.slot].Clicked(gtx) {
			continue
		}
		if a.toggleSpreadsheetOfficeRibbonMenu(action.slot) {
			state.toolbarPicker = pickerNone
			continue
		}
		if unimplementedSpreadsheetToolbarAction(action.slot) {
			a.showSpreadsheetFeatureMissing(action.label)
			continue
		}
		if !a.toolbarActionEnabled(action.slot) {
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
			a.writeSpreadsheetSelectionToClipboard(gtx, action.slot == 3)
			continue
		}
		if action.slot == 21 {
			a.requestSpreadsheetClipboardPaste(gtx, state)
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
		if action.slot == toolbarActionOpenFile {
			a.startOpenFilePicker()
			continue
		}
		if action.slot == toolbarActionSaveAs {
			ext := "xlsx"
			if state.sourceFormat == application.FormatCSV {
				ext = "csv"
			}
			a.startSaveAsPicker(ext)
			continue
		}
		if action.slot == toolbarActionPrint {
			a.requestSpreadsheetPrint()
			continue
		}
		if action.slot == toolbarActionExport {
			a.requestSpreadsheetPDFExport()
			continue
		}
		if action.slot == toolbarActionShare {
			a.requestSpreadsheetShareCopy()
			continue
		}
		if action.slot >= 12 && action.slot <= 15 {
			if edit, ok := spreadsheetStructureEditForToolbarAction(action.slot, state.sheet, state.selection); ok {
				a.applyStructure(edit)
			}
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
					a.setStatus("工作表名称不能为空")
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
		switch action.slot {
		case 25:
			a.startSpreadsheetImagePicker(spreadsheetImageInsert)
			continue
		case 26:
			a.startSpreadsheetImagePicker(spreadsheetImageReplace)
			continue
		case 27:
			if picture, ok := gridPictureByID(activeGridPictures(state, state.sheet), state.selectedGridPictureID); ok {
				a.startGridPictureCommand(state, gridPictureCommandRequest{operation: "删除图片", kind: gridPictureCommandDelete, imageID: picture.ID})
			}
			continue
		case 28:
			if picture, ok := gridPictureByID(activeGridPictures(state, state.sheet), state.selectedGridPictureID); ok {
				a.modal.ShowInput("设置图片宽度", "输入图片宽度（像素）", fmt.Sprint(picture.WidthPx), "1–10000", func(action string) {
					draft := strings.TrimSpace(a.modal.Input.Text())
					a.modal.Hide()
					if action != "confirm" {
						return
					}
					width, err := strconv.Atoi(draft)
					if err != nil || width < 1 || width > 10000 {
						a.setStatus("图片宽度必须是 1 到 10000 之间的整数")
						if a.window != nil {
							a.window.Invalidate()
						}
						return
					}
					a.startGridPictureCommand(state, gridPictureCommandRequest{operation: "调整图片尺寸", kind: gridPictureCommandResize, imageID: picture.ID, widthPx: width})
				}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
			}
			continue
		}
		if action.slot == toolbarActionSum {
			a.applySpreadsheetAutoSum()
			continue
		}
		if action.slot == toolbarActionMergeCenter {
			if err := a.mergeSpreadsheetSelection(); err != nil {
				a.setStatus(err.Error())
			}
			continue
		}
		if action.slot == toolbarActionFind {
			a.showSpreadsheetFindDialog()
			continue
		}
		if action.slot == toolbarActionFormatPainter {
			a.toggleSpreadsheetFormatPainter()
			continue
		}
		if action.slot == toolbarActionFontFamily {
			a.showSpreadsheetFontFamilyDialog()
			continue
		}
		if action.slot == toolbarActionZoomIn {
			a.runSpreadsheetZoomIntent("zoom_in", 0)
			continue
		}
		if action.slot == toolbarActionZoom100 {
			a.runSpreadsheetZoomIntent("reset", 0)
			continue
		}
		if action.slot == toolbarActionLayoutMargins {
			a.showSpreadsheetMarginsDialog()
			continue
		}
		if action.slot == toolbarActionLayoutOrientation {
			a.showSpreadsheetOrientationDialog()
			continue
		}
		if action.slot == toolbarActionLayoutSize {
			a.showSpreadsheetPageSizeDialog()
			continue
		}
		if action.slot == toolbarActionPrintArea {
			a.showSpreadsheetPrintAreaDialog()
			continue
		}
		if action.slot == toolbarActionViewGridlines || action.slot == toolbarActionLayoutGridlines {
			state.spreadsheetGridlinesHidden = !state.spreadsheetGridlinesHidden
			show := !state.spreadsheetGridlinesHidden
			if state.virtualGrid != nil {
				state.virtualGrid.CellPaintCache = nil
				state.virtualGrid.CellPaintFIFO = nil
			}
			if state.spreadsheetGridlinesHidden {
				a.setStatus("已隐藏网格线")
			} else {
				a.setStatus("已显示网格线")
			}
			operation := "隐藏网格线"
			if show {
				operation = "显示网格线"
			}
			a.runSpreadsheetGridlinesCommand(operation, show)
			if a.window != nil {
				a.window.Invalidate()
			}
			continue
		}
		if action.slot == toolbarActionRecalculate {
			a.recalculateSpreadsheet()
			continue
		}
		if action.slot == toolbarActionRefreshAll {
			a.refreshAllSpreadsheet()
			continue
		}
		if action.slot == toolbarActionInsertTable || action.slot == toolbarActionTableFormat {
			a.showSpreadsheetInsertTableDialog("")
			continue
		}
		if action.slot == toolbarActionPivotTable {
			a.showSpreadsheetPivotTableDialog()
			continue
		}
		if action.slot == toolbarActionInsertLink {
			a.showSpreadsheetLinkDialog()
			continue
		}
		if action.slot == toolbarActionInsertShapes {
			a.showSpreadsheetInsertShapeDialog()
			continue
		}
		if action.slot == toolbarActionInsertTextBox {
			a.showSpreadsheetInsertTextBoxDialog()
			continue
		}
		if action.slot == toolbarActionInsertColumnChart {
			a.showSpreadsheetInsertChartDialog(document.SpreadsheetChartColumn)
			continue
		}
		if action.slot == toolbarActionInsertLineChart {
			a.showSpreadsheetInsertChartDialog(document.SpreadsheetChartLine)
			continue
		}
		if action.slot == toolbarActionInsertPieChart {
			a.showSpreadsheetInsertChartDialog(document.SpreadsheetChartPie)
			continue
		}
		if action.slot == toolbarActionHeaderRow {
			a.showSpreadsheetPrintTitlesDialog()
			continue
		}
		if action.slot == toolbarActionTextToColumns {
			a.showSpreadsheetTextToColumnsDialog()
			continue
		}
		if action.slot == toolbarActionRemoveDuplicates {
			a.showSpreadsheetRemoveDuplicatesDialog()
			continue
		}
		if action.slot == toolbarActionInsertFunction {
			a.showSpreadsheetInsertFunctionDialog("")
			continue
		}
		if action.slot == toolbarActionRecentFunctions {
			a.showSpreadsheetInsertFunctionDialog("recent")
			continue
		}
		if action.slot == toolbarActionLogicalFunctions {
			a.showSpreadsheetInsertFunctionDialog("logical")
			continue
		}
		if action.slot == toolbarActionTextFunctions {
			a.showSpreadsheetInsertFunctionDialog("text")
			continue
		}
		if action.slot == toolbarActionDateTimeFunctions {
			a.showSpreadsheetInsertFunctionDialog("datetime")
			continue
		}
		if action.slot == toolbarActionNameManager {
			a.showSpreadsheetNameManagerDialog()
			continue
		}
		if action.slot == toolbarActionFlashFill {
			a.applySpreadsheetFlashFill()
			continue
		}
		if action.slot == toolbarActionNormalView {
			a.setSpreadsheetViewMode("normal")
			continue
		}
		if action.slot == toolbarActionPageBreakPreview {
			a.setSpreadsheetViewMode("pageBreakPreview")
			continue
		}
		if action.slot == toolbarActionRuler {
			a.toggleSpreadsheetRuler()
			continue
		}
		if action.slot == toolbarActionProtectSheet {
			a.showSpreadsheetProtectSheetDialog()
			continue
		}
		if action.slot == toolbarActionProtectWorkbook {
			a.showSpreadsheetProtectWorkbookDialog()
			continue
		}
		if action.slot == toolbarActionDataValidation {
			a.showSpreadsheetDataValidationDialog()
			continue
		}
		if action.slot == toolbarActionNewComment {
			a.showSpreadsheetNewCommentDialog()
			continue
		}
		if action.slot == toolbarActionDeleteComment {
			a.deleteSpreadsheetComment()
			continue
		}
		if action.slot == toolbarActionConditionalFormat {
			a.showSpreadsheetConditionalFormatDialog()
			continue
		}
		if action.slot == toolbarActionSpellCheck {
			a.showSpreadsheetSpellCheckDialog()
			continue
		}
		if action.slot == toolbarActionTranslate {
			a.showSpreadsheetTranslateDialog()
			continue
		}
		a.applySpreadsheetFormatAction(action.slot)
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

func (a *App) applySpreadsheetFormatAction(slot int) {
	patch, ok := formatPatchForToolbarAction(slot, a.selectedCellStyle())
	if !ok || !a.toolbarActionEnabled(slot) {
		return
	}
	a.applyCellFormat(patch)
}
