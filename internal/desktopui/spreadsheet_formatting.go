package desktopui

import (
	"encoding/json"
	"errors"
	"glance/internal/i18n"

	"glance/internal/document"
)

func cellStyleToFormatPatch(style document.CellStyle) document.CellFormatPatch {
	bold := style.Bold
	italic := style.Italic
	underline := style.Underline
	wrapText := style.WrapText
	patch := document.CellFormatPatch{
		Bold:      &bold,
		Italic:    &italic,
		Underline: &underline,
		WrapText:  &wrapText,
	}
	if style.FontColor != "" {
		color := style.FontColor
		patch.FontColor = &color
	}
	if style.FillColor != "" {
		fill := style.FillColor
		patch.FillColor = &fill
	}
	if style.HorizontalAlignment != "" {
		align := style.HorizontalAlignment
		patch.HorizontalAlignment = &align
	}
	if style.NumberFormat != "" {
		numFmt := style.NumberFormat
		patch.NumberFormat = &numFmt
	}
	if style.FontSize > 0 {
		size := style.FontSize
		patch.FontSize = &size
	}
	if style.FontFamily != "" {
		family := style.FontFamily
		patch.FontFamily = &family
	}
	hasBorder := false
	for _, b := range style.Borders {
		if b.Style != 0 {
			hasBorder = true
			break
		}
	}
	if hasBorder {
		all := "all"
		patch.BorderPreset = &all
	}
	return patch
}

func (a *App) toggleSpreadsheetFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if state.excelFormatPainterActive {
		a.cancelSpreadsheetFormatPainter()
		return
	}
	if !state.selectionSet {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.select_the_cell_whose_formatting_ec1592"}, feedbackWarning, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	style := a.selectedCellStyle()
	patch := cellStyleToFormatPatch(style)
	state.excelFormatPainterActive = true
	state.excelFormatPainterPatch = &patch
	state.excelFormatPainterSourceCell = a.selectedCell
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.format_painter_is_active_select_542a5a"}, feedbackSuccess, notificationToast)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) cancelSpreadsheetFormatPainter() {
	state := a.documentSessionState
	if state == nil {
		return
	}
	state.excelFormatPainterActive = false
	state.excelFormatPainterPatch = nil
	state.excelFormatPainterSourceCell = ""
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.format_painter_canceled_1e3acc"}, feedbackSuccess, notificationToast)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) applySpreadsheetFormatPainter() {
	state := a.documentSessionState
	if state == nil || !state.excelFormatPainterActive || state.excelFormatPainterPatch == nil || !state.selectionSet {
		return
	}
	patch := *state.excelFormatPainterPatch
	sourceCell := state.excelFormatPainterSourceCell
	targetBounds := state.selection.Bounds()
	state.excelFormatPainterActive = false
	state.excelFormatPainterPatch = nil
	state.excelFormatPainterSourceCell = ""
	_, hasExactPainter := state.book.(document.SpreadsheetCellFormatPainter)
	if sourceCell != "" && (hasExactPainter || a.applicationSessionRegistered(state.documentID)) {
		a.runSpreadsheetFormatPaintIntent(sourceCell, targetBounds)
		return
	}
	a.applyCellFormat(patch)
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.format_painter_applied_to_the_3a1558"}, feedbackSuccess, notificationToast)
	if a.window != nil {
		a.window.Invalidate()
	}
}

type spreadsheetFormatPaintArguments struct {
	Sheet        string             `json:"sheet"`
	SourceCell   string             `json:"source_cell"`
	TargetBounds document.CellRange `json:"target_bounds"`
}

func (a *App) runSpreadsheetFormatPaintIntent(sourceCell string, targetBounds document.CellRange) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	a.launchManagedBackground(func() {
		var changed bool
		var err error
		commandRouted := a.applicationSessionRegistered(token.documentID)
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.format.paint", "spreadsheet_range", "selection", sheet, spreadsheetFormatPaintArguments{Sheet: sheet, SourceCell: sourceCell, TargetBounds: targetBounds})
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else if painter, ok := state.book.(document.SpreadsheetCellFormatPainter); ok {
			unlock := lockSessionAdapterWrite(state)
			changed, err = painter.PaintCellFormat(sheet, sourceCell, targetBounds)
			unlock()
		} else {
			err = errors.New("当前表格不支持完整格式刷")
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "格式刷", applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) showSpreadsheetFontFamilyDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet {
		return
	}
	a.openFontPicker(fontPickerTarget{state: state, sheet: state.sheet, selection: state.selection, revision: state.wordRevisionAtomic.Load(), current: a.selectedCellStyle().FontFamily})
}
