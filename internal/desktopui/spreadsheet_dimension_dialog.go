package desktopui

import (
	"fmt"
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
	a.modal.ShowInput("行高", fmt.Sprintf("设置当前行（第 %d 行）的高度，单位为磅。", row), strconv.FormatFloat(current, 'f', 1, 64), "0.1–409.5 磅", func(action string) {
		value, err := strconv.ParseFloat(strings.TrimSpace(a.modal.Input.Text()), 64)
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if err != nil || !finite(value) || value < 0.1 || value > 409.5 {
			a.setStatus("行高必须是 0.1 到 409.5 磅之间的有限数值")
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		if math.Abs(value-current) <= 0.01 {
			a.setStatus("行高未变化")
			return
		}
		a.applySpreadsheetDimension(row, value, true)
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) showSpreadsheetColumnWidthDialog() {
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil || state.book == nil || v == nil || !state.selectionSet {
		return
	}
	column := state.selection.Active.Column + 1
	current := devicePixelsToChars(v.Columns.sizeAt(column-1), v.Scale)
	a.modal.ShowInput("列宽", fmt.Sprintf("设置当前列（第 %d 列）的列宽，单位为字符。", column), strconv.FormatFloat(current, 'f', 2, 64), "0.1–255 字符", func(action string) {
		value, err := strconv.ParseFloat(strings.TrimSpace(a.modal.Input.Text()), 64)
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if err != nil || !finite(value) || value < 0.1 || value > 255 {
			a.setStatus("列宽必须是 0.1 到 255 字符之间的有限数值")
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		if math.Abs(value-current) <= 0.01 {
			a.setStatus("列宽未变化")
			return
		}
		a.applySpreadsheetDimension(column, value, false)
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) applySpreadsheetDimension(index int, value float64, row bool) {
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil || state.book == nil || v == nil || index < 1 || state.formatRunning.Load() || a.isSaving() {
		return
	}
	reader, ok := state.book.(document.GridReader)
	if !ok {
		a.setStatus("当前工作簿不支持行列尺寸调整")
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("表格正在处理其他操作，请稍后再试")
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
		a.setStatus("无法启动行列尺寸调整")
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.setStatus("正在设置行高…")
	if !row {
		a.setStatus("正在设置列宽…")
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
