package desktopui

import (
	"glance/internal/i18n"
	"math"
	"strconv"
	"strings"

	"glance/internal/document"
)

func (a *App) showSpreadsheetRowHeightDialog() {
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil || state.book == nil || v == nil || !state.selectionSet {
		return
	}
	row := state.selection.Active.Row + 1
	current := devicePixelsToPoints(v.Rows.sizeAt(row-1), v.Scale)
	a.modal.ShowInputMessage("spreadsheet.dimension.row", i18n.Message{ID: "spreadsheet.dimension.row.dialog.title"}, i18n.Message{ID: "spreadsheet.dimension.row.dialog.body", Args: i18n.Args{"Row": row}}, strconv.FormatFloat(current, 'f', 1, 64), i18n.Message{ID: "spreadsheet.dimension.row.dialog.hint"}, func(action string) {
		value, err := strconv.ParseFloat(strings.TrimSpace(a.modal.Input.Text()), 64)
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if err != nil || !finite(value) || value < 0.1 || value > 409.5 {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.row_height_must_be_a_04728f"}, feedbackError, notificationToast)
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		if math.Abs(value-current) <= 0.01 {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.row_height_is_unchanged_3c158d"}, feedbackWarning, notificationToast)
			return
		}
		a.applySpreadsheetDimension(row, value, true)
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

func (a *App) showSpreadsheetColumnWidthDialog() {
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil || state.book == nil || v == nil || !state.selectionSet {
		return
	}
	column := state.selection.Active.Column + 1
	current := devicePixelsToChars(v.Columns.sizeAt(column-1), v.Scale)
	a.modal.ShowInputMessage("spreadsheet.dimension.column", i18n.Message{ID: "spreadsheet.dimension.column.dialog.title"}, i18n.Message{ID: "spreadsheet.dimension.column.dialog.body", Args: i18n.Args{"Column": column}}, strconv.FormatFloat(current, 'f', 2, 64), i18n.Message{ID: "spreadsheet.dimension.column.dialog.hint"}, func(action string) {
		value, err := strconv.ParseFloat(strings.TrimSpace(a.modal.Input.Text()), 64)
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if err != nil || !finite(value) || value < 0.1 || value > 255 {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.column_width_must_be_a_01a7e6"}, feedbackError, notificationToast)
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		if math.Abs(value-current) <= 0.01 {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.column_width_is_unchanged_2c5b94"}, feedbackWarning, notificationToast)
			return
		}
		a.applySpreadsheetDimension(column, value, false)
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

func (a *App) applySpreadsheetDimension(index int, value float64, row bool) {
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil || state.book == nil || v == nil || index < 1 || state.formatRunning.Load() || a.isSaving() {
		return
	}
	reader, ok := state.book.(document.GridReader)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_b3613c"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.a_table_operation_is_in_12e4b9"}, feedbackInfo, notificationSuppressed)
		return
	}
	sheet, generation, window := state.sheet, a.documentGeneration, v
	change := &spreadsheetDimensionChange{index: index, value: value, row: row}
	operation := "调整列宽"
	if row {
		operation = "调整行高"
	}
	if !a.launchManagedBackground(func() {
		unlock := lockSessionAdapterWrite(state)
		var err error
		if row {
			err = reader.SetRowHeight(sheet, index, value)
		} else {
			err = reader.SetColumnWidth(sheet, index, value)
		}
		unlock()
		a.completeSessionOperation(state, token, err, err == nil)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state:      state,
			token:      token,
			revision:   state.wordRevisionAtomic.Load(),
			generation: generation,
			sheet:      sheet,
			window:     window,
			err:        err,
			operation:  operation,
			applied:    err == nil,
			dimensions: []spreadsheetDimensionChange{*change},
		}:
		case <-a.shutdown:
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}) {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.could_not_start_row_or_f24e3a"}, feedbackWarning, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.setting_row_height_03e570"}, feedbackInfo, notificationToast)
	if !row {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.setting_column_width_81fef3"}, feedbackInfo, notificationToast)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func setGridColumnSizePreservingViewport(v *gridViewport, column int, size float64) {
	if v == nil || v.Columns == nil || column < 0 || !finite(size) || size <= 0 || v.Columns.sizeAt(column) == size {
		return
	}
	anchor := v.Columns.IndexAt(v.scrollWorldX())
	within := 0.0
	if anchor >= 0 {
		if edge, err := v.Columns.Boundary(anchor); err == nil {
			within = v.scrollWorldX() - edge
		}
	}
	if v.Columns.SetSize(column, size) != nil {
		return
	}
	if anchor >= 0 {
		if edge, err := v.Columns.Boundary(anchor); err == nil {
			v.X = math.Max(0, edge+within-v.frozenColumnBoundary())
		}
	}
	v.GeometryChanged = true
}
