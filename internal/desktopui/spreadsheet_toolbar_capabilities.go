package desktopui

import "glance/internal/document"

func (a *App) toolbarActionEnabled(slot int) bool {
	state := a.documentSessionState
	if state == nil || slot < 0 || slot >= len(state.toolbarActionButtons) {
		return false
	}
	if unimplementedSpreadsheetToolbarAction(slot) {
		return state.book != nil && !state.formatRunning.Load() && !a.isSaving()
	}
	switch slot {
	case toolbarActionZoomIn, toolbarActionZoom100, toolbarActionViewGridlines, toolbarActionLayoutGridlines:
		return state.book != nil && state.virtualGrid != nil && !state.formatRunning.Load()
	case toolbarActionNormalView, toolbarActionPageBreakPreview, toolbarActionRuler:
		_, ok := state.book.(document.SpreadsheetViewsEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionProtectSheet:
		_, ok := state.book.(document.SpreadsheetProtectionEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionProtectWorkbook:
		_, ok := state.book.(document.SpreadsheetWorkbookProtectionEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionInsertTable, toolbarActionTableFormat:
		_, ok := state.book.(document.SpreadsheetTableEditor)
		return ok && state.selectionSet && state.selection.Bounds().EndRow > state.selection.Bounds().StartRow && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionFreezePanes:
		_, ok := state.book.(document.SpreadsheetPanesEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionTextToColumns:
		_, ok := state.book.(document.SpreadsheetTextToColumnsEditor)
		return ok && state.selectionSet && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionRemoveDuplicates:
		_, ok := state.book.(document.SpreadsheetDuplicatesEditor)
		return ok && state.selectionSet && state.selection.Bounds().EndRow > state.selection.Bounds().StartRow && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionInsertFunction, toolbarActionRecentFunctions, toolbarActionLogicalFunctions, toolbarActionTextFunctions, toolbarActionDateTimeFunctions:
		_, ok := state.book.(document.SpreadsheetFormulaEditor)
		return ok && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionNameManager:
		_, ok := state.book.(document.SpreadsheetDefinedNameEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionFlashFill:
		_, ok := state.book.(document.SpreadsheetFlashFillEditor)
		return ok && state.selectionSet && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionDataValidation:
		_, ok := state.book.(document.SpreadsheetDataValidationEditor)
		return ok && state.selectionSet && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionConditionalFormat:
		_, ok := state.book.(document.SpreadsheetConditionalFormatEditor)
		return ok && state.selectionSet && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionNewComment, toolbarActionDeleteComment:
		_, ok := state.book.(document.SpreadsheetCommentEditor)
		return ok && state.selectionSet && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionSpellCheck, toolbarActionTranslate:
		return state.book != nil && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionRecalculate, toolbarActionRefreshAll:
		_, ok := state.book.(document.SpreadsheetRecalculateEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionLayoutMargins, toolbarActionLayoutOrientation, toolbarActionLayoutSize, toolbarActionPrintArea, toolbarActionHeaderRow:
		_, ok := state.book.(document.SpreadsheetPageLayoutEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionInsertLink:
		_, ok := state.book.(document.SpreadsheetHyperlinkEditor)
		return ok && state.selectionSet && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionInsertShapes, toolbarActionInsertTextBox:
		_, ok := state.book.(document.SpreadsheetShapeEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionInsertColumnChart, toolbarActionInsertLineChart, toolbarActionInsertPieChart:
		_, ok := state.book.(document.SpreadsheetChartEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionPivotTable:
		_, ok := state.book.(document.SpreadsheetPivotTableEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case 0, 1:
		_, ok := state.book.(document.SpreadsheetHistory)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case 4, 5, 6, 7, 8, 9, 10, 11, 16,
		toolbarActionFontFamily,
		toolbarActionAlignLeft, toolbarActionAlignRight, toolbarActionWrapText, toolbarActionPercentFormat:
		_, ok := state.book.(document.SpreadsheetFormatter)
		return ok && state.selectionSet && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionFormatPainter:
		_, ok := state.book.(document.SpreadsheetFormatter)
		return ok && (state.selectionSet || state.excelFormatPainterActive) && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionSum:
		_, reader := state.book.(document.SpreadsheetRangeReader)
		_, editor := state.book.(document.SpreadsheetBatchEditor)
		return reader && editor && state.selectionSet && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionFind:
		_, ok := state.book.(document.SpreadsheetSearcher)
		return ok && !state.formatRunning.Load()
	case 2, 3:
		_, reader := state.book.(document.SpreadsheetRangeReader)
		_, editor := state.book.(document.SpreadsheetBatchEditor)
		return reader && editor && state.selectionSet && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.formatRunning.Load()
	case 21:
		_, ok := state.book.(document.SpreadsheetBatchEditor)
		return ok && state.selectionSet && !state.formatRunning.Load()
	case 18, 19:
		_, ok := state.book.(document.SpreadsheetSorter)
		bounds := state.selection.Bounds()
		return ok && state.selectionSet && bounds.EndRow > bounds.StartRow && selectionCellCount(bounds) <= maxClipboardCellsUI && !state.formatRunning.Load() && !a.isSaving()
	case 20:
		_, ok := state.book.(document.SpreadsheetRangeReader)
		return state.valueFilter != nil || (ok && state.selectionSet && state.selection.Bounds().EndRow > state.selection.Bounds().StartRow && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.formatRunning.Load())
	case 12, 13, 14, 15, 22, 23, 24:
		_, ok := state.book.(document.SpreadsheetStructureEditor)
		if slot == 24 && len(state.sheets) <= 1 {
			return false
		}
		return ok && !state.formatRunning.Load() && !a.isSaving() && ((slot < 22) == state.selectionSet || slot >= 22)
	case 25:
		_, ok := state.book.(document.SpreadsheetImageEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving() && !a.xlsxImagePickerRunning
	case 26, 27, 28:
		editor, editorOK := state.book.(document.SpreadsheetImageEditor)
		picture, selected := gridPictureByID(activeGridPictures(state, state.sheet), state.selectedGridPictureID)
		return editorOK && editor != nil && selected && picture.Editable && !state.formatRunning.Load() && !a.isSaving() && !a.xlsxImagePickerRunning
	case 17:
		return state.book != nil && sessionDirty(state) && !a.isSaving() && sessionPath(state) != ""
	case toolbarActionOpenFile:
		return !state.formatRunning.Load()
	case toolbarActionSaveAs:
		return state.book != nil && !a.isSaving()
	case toolbarActionPrint:
		return state.book != nil && a.nativePrinter != nil && a.nativePrinter.Available() && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionExport:
		return state.book != nil && !state.formatRunning.Load() && !a.isSaving() && !state.spreadsheetPDFExportRunning
	case toolbarActionShare:
		return state.book != nil && !state.formatRunning.Load() && !a.isSaving() && !a.fileCommandRunning
	default:
		return false
	}
}

func selectionCellCount(bounds document.CellRange) int {
	return (bounds.EndRow - bounds.StartRow + 1) * (bounds.EndColumn - bounds.StartColumn + 1)
}
