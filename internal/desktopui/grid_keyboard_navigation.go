package desktopui

import (
	"fmt"
	"math"
	"runtime"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/document"
)

func (a *App) handleSpreadsheetGridNavigationKey(gtx layout.Context, v *gridViewport, event key.Event) bool {
	state := a.documentSessionState
	if state == nil || v == nil || state.cellEdit.Editing || state.formulaReference.Active ||
		state.selectedGridPictureID != "" || state.selectionDragging || v.Resize != nil ||
		state.gridPictureGesture.Mode != gridPictureGestureNone {
		return false
	}
	if a.handleSpreadsheetSheetNavigationKey(gtx, event) {
		return true
	}

	direction, isArrow := spreadsheetDirectionForArrow(event.Name)
	active := spreadsheetActiveCell(state, a.selectedCell)
	shortcut := event.Modifiers.Contain(key.ModShortcut)
	extend := event.Modifiers.Contain(key.ModShift)
	if isArrow {
		if event.Modifiers&^(key.ModShift|key.ModShortcut) != 0 {
			return false
		}
		endMode := state.gridEndMode
		state.gridEndMode = false
		if shortcut || endMode {
			if shortcut {
				a.startSpreadsheetDataRegionJump(v, direction, extend)
			} else {
				a.startSpreadsheetNextNonEmptyCellJump(v, direction, extend)
			}
			return true
		}
		state.gridNavigationSequence++
		target, _ := nextGridArrowPosition(active, event.Name, v.merges())
		if target == active {
			return true
		}
		a.applySpreadsheetGridNavigationSelection(v, target, extend)
		return true
	}

	switch event.Name {
	case key.NameHome:
		homeEndModifier := spreadsheetHomeEndModifier()
		if event.Modifiers&^(key.ModShift|homeEndModifier) != 0 {
			return false
		}
		state.gridEndMode = false
		state.gridNavigationSequence++
		target := CellPosition{Row: active.Row}
		if event.Modifiers.Contain(homeEndModifier) {
			target = CellPosition{}
		}
		a.applySpreadsheetGridNavigationSelection(v, target, extend)
		return true
	case key.NameEnd:
		homeEndModifier := spreadsheetHomeEndModifier()
		if event.Modifiers.Contain(homeEndModifier) {
			if event.Modifiers&^(key.ModShift|homeEndModifier) != 0 {
				return false
			}
			state.gridEndMode = false
			a.startSpreadsheetUsedRangeJump(extend)
			return true
		}
		if event.Modifiers == 0 {
			state.gridNavigationSequence++
			state.gridEndMode = !state.gridEndMode
			if a.window != nil {
				a.window.Invalidate()
			}
			return true
		}
		if event.Modifiers == key.ModShift {
			state.gridEndMode = false
			state.gridNavigationSequence++
			a.applySpreadsheetGridNavigationSelection(v, CellPosition{Row: active.Row, Column: xlsxColumns - 1}, true)
			return true
		}
		return false
	case key.NameTab:
		if event.Modifiers == 0 {
			target, _ := nextGridArrowPosition(active, key.NameRightArrow, v.merges())
			a.applySpreadsheetGridNavigationSelection(v, target, false)
			return true
		}
		if event.Modifiers == key.ModShift {
			target, _ := nextGridArrowPosition(active, key.NameLeftArrow, v.merges())
			a.applySpreadsheetGridNavigationSelection(v, target, false)
			return true
		}
		return false
	case key.NameSpace:
		if event.Modifiers == key.ModShift|key.ModCtrl || event.Modifiers == key.ModShift|key.ModShortcut {
			a.startSpreadsheetSelectAll(v)
			return true
		}
		var scope cellSelectionScope
		switch event.Modifiers {
		case key.ModShift:
			scope = selectionRows
		case key.ModCtrl:
			scope = selectionColumns
		default:
			return false
		}
		a.updateGridHeaderSelection(v, scope, active, state.selectionSet && state.selection.Scope == scope)
		if a.window != nil {
			a.window.Invalidate()
		}
		return true
	case key.Name("A"):
		if event.Modifiers != key.ModShortcut {
			return false
		}
		a.startSpreadsheetSelectAll(v)
		return true
	case key.Name(";"), key.Name(":"):
		if spreadsheetDateShortcut(event.Name, event.Modifiers) {
			if err := a.insertCurrentDateIntoSpreadsheetCell(); err != nil {
				a.setStatus("插入日期失败：" + err.Error())
			}
			return true
		}
		if spreadsheetTimeShortcut(event.Name, event.Modifiers, runtime.GOOS) {
			if err := a.insertCurrentTimeIntoSpreadsheetCell(); err != nil {
				a.setStatus("插入时间失败：" + err.Error())
			}
			return true
		}
		return false
	case key.Name("D"):
		fillDownShortcut := event.Modifiers == key.ModShortcut || runtime.GOOS == "darwin" && event.Modifiers == key.ModCtrl
		if !fillDownShortcut {
			return false
		}
		if err := a.fillDownSpreadsheetSelection(); err != nil {
			a.setStatus("向下填充失败：" + err.Error())
		}
		return true
	case key.Name("R"):
		fillRightShortcut := event.Modifiers == key.ModShortcut || runtime.GOOS == "darwin" && event.Modifiers == key.ModCtrl
		if !fillRightShortcut {
			return false
		}
		if err := a.fillRightSpreadsheetSelection(); err != nil {
			a.setStatus("向右填充失败：" + err.Error())
		}
		return true
	default:
		return false
	}
}

func spreadsheetDateShortcut(name key.Name, modifiers key.Modifiers) bool {
	// Excel uses Control+; for a static date on both Windows and macOS.
	return name == key.Name(";") && modifiers == key.ModCtrl
}

func spreadsheetTimeShortcut(name key.Name, modifiers key.Modifiers, goos string) bool {
	if goos == "darwin" {
		// Excel for Mac uses Command+; for a static time.
		return name == key.Name(";") && modifiers == key.ModCommand
	}
	// Excel for Windows uses Ctrl+Shift+; . Some keyboard layouts report ':'
	// as the shifted key name, while others preserve ';' and its Shift modifier.
	return (name == key.Name(";") || name == key.Name(":")) && modifiers == key.ModCtrl|key.ModShift
}

// handleSpreadsheetEditorNavigation commits an edit for the navigation keys
// that Gio's text editor does not report as SubmitEvent. It runs before
// Editor.Update so Shift+Enter and Tab cannot insert text or move focus away.
func (a *App) handleSpreadsheetEditorNavigation(gtx layout.Context, v *gridViewport, editor *widget.Editor) bool {
	state := a.documentSessionState
	if editor == nil || state == nil {
		return false
	}
	if state.formatRunning.Load() {
		return true
	}
	if a.handleSpreadsheetFormulaReferenceCycle(gtx, v, editor) {
		return true
	}
	if !editorHasActiveComposition(editor) && !state.gridInputComposing {
		for ev, ok := gtx.Event(
			key.Filter{Focus: editor, Name: key.NameReturn, Required: key.ModCtrl},
			key.Filter{Focus: editor, Name: key.NameEnter, Required: key.ModCtrl},
		); ok; ev, ok = gtx.Event(
			key.Filter{Focus: editor, Name: key.NameReturn, Required: key.ModCtrl},
			key.Filter{Focus: editor, Name: key.NameEnter, Required: key.ModCtrl},
		) {
			pressed, ok := ev.(key.Event)
			if !ok || pressed.State != key.Press {
				continue
			}
			if err := a.enterSpreadsheetValueInSelection(editor.Text()); err != nil {
				a.setStatus("输入选区失败：" + err.Error())
				if a.window != nil {
					a.window.Invalidate()
				}
			}
			return true
		}
	}
	if state.formulaReference.Active {
		return false
	}
	filters := []event.Filter{
		key.Filter{Focus: editor, Name: key.NameReturn, Required: key.ModShift},
		key.Filter{Focus: editor, Name: key.NameEnter, Required: key.ModShift},
		key.Filter{Focus: editor, Name: key.NameTab, Optional: key.ModShift},
	}
	if editor == &a.cellEditor && !editorHasActiveComposition(editor) {
		for ev, ok := gtx.Event(
			key.Filter{Focus: editor, Name: key.NameReturn, Required: key.ModAlt, Optional: key.ModCtrl},
			key.Filter{Focus: editor, Name: key.NameEnter, Required: key.ModAlt, Optional: key.ModCtrl},
		); ok; ev, ok = gtx.Event(
			key.Filter{Focus: editor, Name: key.NameReturn, Required: key.ModAlt, Optional: key.ModCtrl},
			key.Filter{Focus: editor, Name: key.NameEnter, Required: key.ModAlt, Optional: key.ModCtrl},
		) {
			ke, ok := ev.(key.Event)
			if !ok || ke.State != key.Press {
				continue
			}
			editor.SingleLine = false
			editor.Insert("\n")
			a.syncSpreadsheetGridTextDraft()
			updateGridCellRowPreview(gtx, a, v)
			if a.window != nil {
				a.window.Invalidate()
			}
			return true
		}
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			return false
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		for pending, ok := editor.Update(gtx); ok; pending, ok = editor.Update(gtx) {
			if _, changed := pending.(widget.ChangeEvent); changed && editor == &a.cellEditor {
				state.cellEdit.setDraft(editor.Text())
				start, end := editor.Selection()
				a.formulaEditor.SetText(editor.Text())
				a.formulaEditor.SetCaret(start, end)
			}
			if _, submit := pending.(widget.SubmitEvent); submit {
				if err := a.commitGridCell(editor.Text()); err != nil {
					a.setStatus("修改失败：" + err.Error())
					gtx.Execute(key.FocusCmd{Tag: editor})
					if a.window != nil {
						a.window.Invalidate()
					}
					return true
				}
				a.moveSpreadsheetSelectionAfterEntry(v, key.NameDownArrow)
			}
		}
		direction := key.NameDownArrow
		switch ke.Name {
		case key.NameReturn, key.NameEnter:
			direction = key.NameUpArrow
		case key.NameTab:
			direction = key.NameRightArrow
			if ke.Modifiers.Contain(key.ModShift) {
				direction = key.NameLeftArrow
			}
		}
		if err := a.commitGridCell(editor.Text()); err != nil {
			a.setStatus("修改失败：" + err.Error())
			gtx.Execute(key.FocusCmd{Tag: editor})
			a.window.Invalidate()
			return true
		}
		a.moveSpreadsheetSelectionAfterEntry(v, direction)
		gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
		if a.window != nil {
			a.window.Invalidate()
		}
		return true
	}
}

func (a *App) moveSpreadsheetSelectionAfterEntry(v *gridViewport, direction key.Name) {
	if v == nil || a.documentSessionState == nil {
		return
	}
	active := spreadsheetActiveCell(a.documentSessionState, a.selectedCell)
	target, _ := nextGridArrowPosition(active, direction, v.merges())
	a.updateGridSelection(v, target, false, true)
	revealGridCell(v, target, a.lastGridWidth, a.lastGridHeight)
}

func spreadsheetHomeEndModifier() key.Modifiers {
	if runtime.GOOS == "darwin" {
		return key.ModCtrl
	}
	return key.ModShortcut
}

func (a *App) applySpreadsheetGridNavigationSelection(v *gridViewport, target CellPosition, extend bool) {
	position := selectionAnchor(v.merges(), target)
	a.updateGridSelection(v, position, extend, true)
	revealGridCell(v, position, a.lastGridWidth, a.lastGridHeight)
	if a.window != nil {
		a.window.Invalidate()
	}
}

type spreadsheetGridNavigationResult struct {
	state              *documentSessionState
	sequence           uint64
	documentGeneration uint64
	revision           uint64
	sheet              string
	active             document.CellCoordinate
	target             document.CellCoordinate
	extend             bool
	selectRange        bool
	selectionBounds    document.CellRange
	selectionActive    document.CellCoordinate
	selectionHasData   bool
	err                error
}

func spreadsheetDirectionForArrow(name key.Name) (document.SpreadsheetDirection, bool) {
	switch name {
	case key.NameUpArrow:
		return document.SpreadsheetDirectionUp, true
	case key.NameDownArrow:
		return document.SpreadsheetDirectionDown, true
	case key.NameLeftArrow:
		return document.SpreadsheetDirectionLeft, true
	case key.NameRightArrow:
		return document.SpreadsheetDirectionRight, true
	default:
		return 0, false
	}
}

func spreadsheetActiveCell(state *documentSessionState, address string) CellPosition {
	if state != nil && state.selectionSet {
		return clampCellPosition(state.selection.Active)
	}
	if row, column, ok := parseCellAddress(address); ok {
		return clampCellPosition(CellPosition{Row: row - 1, Column: column - 1})
	}
	return CellPosition{}
}

func (a *App) startSpreadsheetDataRegionJump(v *gridViewport, direction document.SpreadsheetDirection, extend bool) {
	a.startSpreadsheetGridJump(v, direction, extend, false)
}

func (a *App) startSpreadsheetNextNonEmptyCellJump(v *gridViewport, direction document.SpreadsheetDirection, extend bool) {
	a.startSpreadsheetGridJump(v, direction, extend, true)
}

func (a *App) startSpreadsheetGridJump(v *gridViewport, direction document.SpreadsheetDirection, extend, nextNonEmpty bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	var readTarget func(string, document.CellCoordinate, document.SpreadsheetDirection) (document.CellCoordinate, error)
	if nextNonEmpty {
		nextNavigator, supportsNextCell := state.book.(document.SpreadsheetNextNonEmptyCellReader)
		if !supportsNextCell {
			a.setStatus("当前表格不支持 End 模式跳转")
			return
		}
		readTarget = nextNavigator.NextNonEmptyCell
	} else {
		navigator, ok := state.book.(document.SpreadsheetDataRegionReader)
		if !ok {
			a.setStatus("当前表格不支持数据区跳转")
			return
		}
		readTarget = navigator.DataRegionEdge
	}
	active := spreadsheetActiveCell(state, a.selectedCell)
	state.gridNavigationSequence++
	sequence := state.gridNavigationSequence
	resultChannel := a.gridNavigationResults
	if resultChannel == nil {
		resultChannel = make(chan spreadsheetGridNavigationResult, 16)
		a.gridNavigationResults = resultChannel
	}
	result := spreadsheetGridNavigationResult{
		state:              state,
		sequence:           sequence,
		documentGeneration: a.documentGeneration,
		revision:           state.wordRevisionAtomic.Load(),
		sheet:              state.sheet,
		active:             document.CellCoordinate{Row: active.Row + 1, Column: active.Column + 1},
		extend:             extend,
	}
	if !a.launchManagedBackground(func() {
		unlock := lockSessionAdapterRead(state)
		target, err := readTarget(result.sheet, result.active, direction)
		unlock()
		result.target, result.err = target, err
		select {
		case resultChannel <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}) {
		a.setStatus("数据区跳转已取消")
	}
}

func (a *App) startSpreadsheetUsedRangeJump(extend bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	dimensions, ok := state.book.(document.SpreadsheetDimensionReader)
	if !ok {
		a.setStatus("当前表格不支持已用区域跳转")
		return
	}
	active := spreadsheetActiveCell(state, a.selectedCell)
	state.gridNavigationSequence++
	sequence := state.gridNavigationSequence
	resultChannel := a.gridNavigationResults
	if resultChannel == nil {
		resultChannel = make(chan spreadsheetGridNavigationResult, 16)
		a.gridNavigationResults = resultChannel
	}
	result := spreadsheetGridNavigationResult{
		state:              state,
		sequence:           sequence,
		documentGeneration: a.documentGeneration,
		revision:           state.wordRevisionAtomic.Load(),
		sheet:              state.sheet,
		active:             document.CellCoordinate{Row: active.Row + 1, Column: active.Column + 1},
		extend:             extend,
	}
	if !a.launchManagedBackground(func() {
		unlock := lockSessionAdapterRead(state)
		maxColumn, maxRow, err := dimensions.Dimension(result.sheet)
		unlock()
		if err == nil {
			if maxColumn < 1 || maxRow < 1 {
				err = fmt.Errorf("worksheet returned invalid used range %d,%d", maxColumn, maxRow)
			} else {
				result.target = document.CellCoordinate{Row: maxRow, Column: maxColumn}
			}
		}
		result.err = err
		select {
		case resultChannel <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}) {
		a.setStatus("已用区域跳转已取消")
	}
}

func (a *App) startSpreadsheetSelectAll(v *gridViewport) {
	state := a.documentSessionState
	if state == nil || state.book == nil || v == nil {
		return
	}
	active := spreadsheetActiveCell(state, a.selectedCell)
	if pending := state.gridSelectAllPendingSequence; pending != 0 {
		state.gridSelectAllPendingSequence = 0
		if pending == state.gridNavigationSequence {
			a.updateGridHeaderSelection(v, selectionSheet, active, false)
			return
		}
	}
	if state.selectionSet && state.selection.Scope == selectionSheet {
		return
	}
	if state.selectionSet && state.selection.Scope != selectionCells {
		a.updateGridHeaderSelection(v, selectionSheet, active, false)
		return
	}
	reader, ok := state.book.(document.SpreadsheetCurrentDataRegionReader)
	if !ok {
		a.updateGridHeaderSelection(v, selectionSheet, active, false)
		return
	}
	state.gridNavigationSequence++
	result := spreadsheetGridNavigationResult{
		state:              state,
		sequence:           state.gridNavigationSequence,
		documentGeneration: a.documentGeneration,
		revision:           state.wordRevisionAtomic.Load(),
		sheet:              state.sheet,
		active:             document.CellCoordinate{Row: active.Row + 1, Column: active.Column + 1},
		selectRange:        true,
	}
	state.gridSelectAllPendingSequence = result.sequence
	resultChannel := a.gridNavigationResults
	if resultChannel == nil {
		resultChannel = make(chan spreadsheetGridNavigationResult, 16)
		a.gridNavigationResults = resultChannel
	}
	if !a.launchManagedBackground(func() {
		unlock := lockSessionAdapterRead(state)
		result.selectionBounds, result.selectionActive, result.selectionHasData, result.err = reader.CurrentDataRegion(result.sheet, result.active)
		unlock()
		select {
		case resultChannel <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	}) {
		state.gridSelectAllPendingSequence = 0
		a.setStatus("选择数据区域已取消")
	}
}

func (a *App) pollSpreadsheetGridNavigation(gtx layout.Context, v *gridViewport) {
	if a.gridNavigationResults == nil {
		return
	}
	for {
		select {
		case result := <-a.gridNavigationResults:
			if result.selectRange && result.state != nil && result.state.gridSelectAllPendingSequence == result.sequence {
				result.state.gridSelectAllPendingSequence = 0
			}
			state := a.documentSessionState
			if state == nil || !gtx.Focused(&v.PointerTag) || result.state != state || result.sheet != state.sheet ||
				result.sequence != state.gridNavigationSequence || result.documentGeneration != a.documentGeneration ||
				result.revision != state.wordRevisionAtomic.Load() || state.cellEdit.Editing ||
				gtx.Focused(&a.cellEditor) || gtx.Focused(&a.formulaEditor) || state.selectedGridPictureID != "" {
				continue
			}
			active := spreadsheetActiveCell(state, a.selectedCell)
			if result.active.Row != active.Row+1 || result.active.Column != active.Column+1 {
				continue
			}
			if result.err != nil {
				if result.selectRange {
					a.setStatus("选择数据区域失败：" + result.err.Error())
				} else {
					a.setStatus("单元格跳转失败：" + result.err.Error())
				}
				continue
			}
			if result.selectRange {
				bounds := result.selectionBounds
				if bounds.StartRow < 1 || bounds.StartColumn < 1 || bounds.EndRow < bounds.StartRow || bounds.EndColumn < bounds.StartColumn ||
					bounds.EndRow > xlsxRows || bounds.EndColumn > xlsxColumns {
					a.setStatus("无法确定当前数据区域")
					continue
				}
				wholeSheet := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: xlsxRows, EndColumn: xlsxColumns}
				if !result.selectionHasData || bounds == wholeSheet || (state.selectionSet && state.selection.Scope == selectionCells && state.selection.Bounds() == bounds) {
					a.updateGridHeaderSelection(v, selectionSheet, active, false)
				} else {
					selectionActive := clampCellPosition(CellPosition{Row: result.selectionActive.Row - 1, Column: result.selectionActive.Column - 1})
					a.updateGridSelection(v, selectionActive, false, true)
					state.selection.boundsOverride = &bounds
				}
				revealGridCell(v, spreadsheetActiveCell(state, a.selectedCell), a.lastGridWidth, a.lastGridHeight)
				if a.window != nil {
					a.window.Invalidate()
				}
				continue
			}
			position := clampCellPosition(CellPosition{Row: result.target.Row - 1, Column: result.target.Column - 1})
			position = selectionAnchor(v.merges(), position)
			if position == active {
				continue
			}
			a.updateGridSelection(v, position, result.extend, true)
			revealGridCell(v, position, a.lastGridWidth, a.lastGridHeight)
			if a.window != nil {
				a.window.Invalidate()
			}
		default:
			return
		}
	}
}

func nextGridArrowPosition(active CellPosition, name key.Name, merges []document.MergeRegion) (CellPosition, bool) {
	target := active
	var rowDelta, columnDelta int
	switch name {
	case key.NameUpArrow:
		rowDelta = -1
	case key.NameDownArrow:
		rowDelta = 1
	case key.NameLeftArrow:
		columnDelta = -1
	case key.NameRightArrow:
		columnDelta = 1
	default:
		return active, false
	}

	for _, merge := range merges {
		rangeOf := merge.Range
		if active.Row+1 < rangeOf.StartRow || active.Row+1 > rangeOf.EndRow ||
			active.Column+1 < rangeOf.StartColumn || active.Column+1 > rangeOf.EndColumn {
			continue
		}
		switch {
		case rowDelta < 0:
			target.Row = rangeOf.StartRow - 2
		case rowDelta > 0:
			target.Row = rangeOf.EndRow
		case columnDelta < 0:
			target.Column = rangeOf.StartColumn - 2
		case columnDelta > 0:
			target.Column = rangeOf.EndColumn
		}
		return clampCellPosition(target), true
	}

	target.Row += rowDelta
	target.Column += columnDelta
	return clampCellPosition(target), true
}

func revealGridCell(v *gridViewport, position CellPosition, viewportWidth, viewportHeight float64) {
	if v == nil || v.Columns == nil || v.Rows == nil || viewportWidth <= 0 || viewportHeight <= 0 {
		return
	}
	left, _ := v.Columns.Boundary(position.Column)
	right, _ := v.Columns.Boundary(position.Column + 1)
	top, _ := v.Rows.Boundary(position.Row)
	bottom, _ := v.Rows.Boundary(position.Row + 1)
	dx, dy := 0.0, 0.0
	frozenWidth, frozenHeight := v.frozenViewportExtent(viewportWidth, viewportHeight)
	columnSplit, rowSplit := v.paneSplits()
	if position.Column >= columnSplit {
		bodyStart := v.frozenColumnBoundary() + v.X
		bodyExtent := math.Max(0, viewportWidth-frozenWidth)
		if left < bodyStart {
			dx = left - bodyStart
		} else if right > bodyStart+bodyExtent {
			dx = right - bodyStart - bodyExtent
		}
	}
	if position.Row >= rowSplit {
		bodyStart := v.frozenRowBoundary() + v.Y
		bodyExtent := math.Max(0, viewportHeight-frozenHeight)
		if top < bodyStart {
			dy = top - bodyStart
		} else if bottom > bodyStart+bodyExtent {
			dy = bottom - bodyStart - bodyExtent
		}
	}
	if dx != 0 || dy != 0 {
		v.scroll(dx, dy, viewportWidth, viewportHeight)
		v.invalidateWindow()
	}
}
