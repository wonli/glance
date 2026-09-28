package desktopui

import (
	"fmt"
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func (a *App) layoutGridControls(gtx layout.Context) layout.Dimensions {
	for ev, ok := a.nameBox.Update(gtx); ok; ev, ok = a.nameBox.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit {
			a.goToCell()
		}
	}
	for ev, ok := a.formulaEditor.Update(gtx); ok; ev, ok = a.formulaEditor.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit {
			if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
				a.status = "修改失败：" + err.Error()
				a.window.Invalidate()
			}
		}
	}
	if a.goToCellButton.Clicked(gtx) {
		a.goToCell()
	}
	if a.applyButton.Clicked(gtx) {
		if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
			a.status = "修改失败：" + err.Error()
			a.window.Invalidate()
		}
	}
	return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Editor(a.theme, &a.nameBox, "A1").Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, &a.goToCellButton, "前往").Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Editor(a.theme, &a.formulaEditor, "单元格内容").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, &a.applyButton, "✓").Layout(gtx)
				})
			}),
		)
	})
}

func (a *App) goToCell() {
	r, c, ok := parseCellAddress(a.nameBox.Text())
	if !ok || r > xlsxRows || c > xlsxColumns {
		a.status = "单元格地址无效"
		return
	}
	if a.virtualGrid == nil {
		return
	}
	v := a.virtualGrid
	v.SelectedRow, v.SelectedColumn = r, c
	a.selectedCell = xlsx.CellName(c, r)
	a.cellEdit.selectCell(a.selectedCell, "")
	a.nameBox.SetText(a.selectedCell)
	a.formulaEditor.SetText("")
	x, _ := v.Columns.Boundary(c - 1)
	y, _ := v.Rows.Boundary(r - 1)
	v.scroll(x-v.X, y-v.Y, gtxGridContentWidth(a), gtxGridContentHeight(a))
	v.invalidateWindow()
	v.PendingSelectionLoad = true
	a.window.Invalidate()
}

func gtxGridContentWidth(a *App) float64  { return a.lastGridWidth }
func gtxGridContentHeight(a *App) float64 { return a.lastGridHeight }

func (a *App) layoutVirtualGrid(gtx layout.Context) layout.Dimensions {
	reader := a.book.(document.GridReader)
	if a.virtualGrid == nil {
		a.virtualGrid = newGridViewport()
	}
	v := a.virtualGrid
	v.setScale(float64(gtx.Metric.PxPerDp))
	width, height := float64(gtx.Constraints.Max.X), float64(gtx.Constraints.Max.Y)
	rowHeaderPx := gtx.Dp(unit.Dp(gridRowHeader))
	columnHeaderPx := gtx.Dp(unit.Dp(gridColumnHeader))
	contentW, contentH := width-float64(rowHeaderPx), height-float64(columnHeaderPx)
	a.lastGridWidth, a.lastGridHeight = contentW, contentH
	visible := visibleCellWindow(v.Rows, v.Columns, v.X, v.Y, contentW, contentH)
	a.pollGridLoad(v)
	if !v.Loading && !(v.WindowSheet == a.sheet && containsRange(v.WindowRange, visible)) && !(v.FailedSheet == a.sheet && v.FailedRange == visible) {
		a.requestGridWindow(v, reader, a.sheet, visible)
	}
	if v.GeometryChanged {
		v.GeometryChanged = false
		a.window.Invalidate()
	}
	if v.PendingSelectionLoad && v.SelectedRow >= v.WindowRange.StartRow && v.SelectedRow <= v.WindowRange.EndRow && v.SelectedColumn >= v.WindowRange.StartColumn && v.SelectedColumn <= v.WindowRange.EndColumn {
		cell := windowCell(v.Window, v.SelectedRow, v.SelectedColumn)
		a.cellEdit.selectCell(a.selectedCell, cell.Value)
		a.formulaEditor.SetText(cell.Value)
		v.PendingSelectionLoad = false
	}
	for {
		if a.cellEdit.Editing {
			if ev, ok := gtx.Event(key.Filter{Focus: &a.cellEditor, Name: key.NameEscape}); ok {
				if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
					a.cellEdit.cancel()
					a.cellEditor.SetText(a.cellEdit.Original)
					a.formulaEditor.SetText(a.cellEdit.Original)
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
					a.window.Invalidate()
				}
			}
			for ev, ok := a.cellEditor.Update(gtx); ok; ev, ok = a.cellEditor.Update(gtx) {
				if _, submit := ev.(widget.SubmitEvent); submit {
					if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
						a.status = "修改失败：" + err.Error()
						a.window.Invalidate()
					}
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				}
			}
		}
		ev, ok := gtx.Event(key.Filter{Focus: &v.PointerTag})
		if !ok {
			break
		}
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press && ke.Name == key.NameF2 && a.selectedCell != "" {
			if a.cellEdit.begin() {
				a.cellEditor.SetText(a.cellEdit.Original)
				gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
				a.window.Invalidate()
			}
		}
	}
	for {
		e, ok := gtx.Event(pointer.Filter{Target: &v.PointerTag, Kinds: pointer.Enter | pointer.Leave | pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Cancel | pointer.Scroll, ScrollX: pointer.ScrollRange{Min: -1e9, Max: 1e9}, ScrollY: pointer.ScrollRange{Min: -1e9, Max: 1e9}})
		if !ok {
			break
		}
		pe, ok := e.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Leave:
			if v.Resize == nil {
				v.ResizeCursor = 0
			}
		case pointer.Scroll:
			dx, dy := gridScrollDelta(float64(pe.Scroll.X), float64(pe.Scroll.Y), pe.Modifiers.Contain(key.ModShift))
			v.scroll(dx, dy, contentW, contentH)
			if v.Resize != nil {
				rs := v.Resize.State
				edge := resizeScreenEdge(v, rs, gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
				point := rs.LastPointer
				rs.rebase(point, edge)
				v.updateResize(point)
			}
			a.window.Invalidate()
		case pointer.Move:
			axis, _, _, _, hit := gridResizeHandleAt(v, int(pe.Position.X), int(pe.Position.Y), gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
			v.ResizeCursor = 0
			if hit {
				v.ResizeCursor = axis
			}
			if v.Resize != nil {
				v.ResizeCursor = v.Resize.State.Axis
			}
		case pointer.Press:
			if a.cellEdit.Editing {
				editRect := editingCellRect(v, a.cellEdit.Address, rowHeaderPx, columnHeaderPx)
				point := image.Pt(int(pe.Position.X), int(pe.Position.Y))
				if point.In(editRect) {
					// The editor's own pointer handler places the caret. Let it consume
					// the event without changing the selected cell or ending editing.
					continue
				}
				if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
					a.status = "修改失败：" + err.Error()
					gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
					a.window.Invalidate()
					continue
				}
			}
			axis, index, edge, sizePx, resizeHit := gridResizeHandleAt(v, int(pe.Position.X), int(pe.Position.Y), gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
			if resizeHit {
				pos := float64(pe.Position.X)
				if axis == resizeRows {
					pos = float64(pe.Position.Y)
				}
				fileUnits := devicePixelsToPoints(sizePx, v.Scale)
				if axis == resizeColumns {
					fileUnits = devicePixelsToChars(sizePx, v.Scale)
				}
				v.beginResize(axis, index, pos, edge)
				v.Resize.State.OriginalFile = fileUnits
				v.ResizeCursor = axis
				gtx.Execute(pointer.GrabCmd{Tag: &v.PointerTag, ID: pe.PointerID})
				break
			}
			row, col, hit := cellAtPoint(v.Rows, v.Columns, v.X, v.Y, image.Pt(int(pe.Position.X), int(pe.Position.Y)), gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
			if hit {
				row, col, _ = mergeAnchorAt(v.Window.Merges, row, col)
				v.SelectedRow, v.SelectedColumn = row, col
				a.selectedCell = xlsx.CellName(col, row)
				cell := windowCell(v.Window, row, col)
				a.cellEdit.selectCell(a.selectedCell, cell.Value)
				a.nameBox.SetText(a.selectedCell)
				a.formulaEditor.SetText(cell.Value)
				if pe.Time-v.LastClickTime <= 500*time.Millisecond && v.LastClickAddress == a.selectedCell {
					a.cellEdit.begin()
					a.cellEditor.SetText(cell.Value)
					gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
				} else {
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				}
				v.LastClickAddress, v.LastClickTime = a.selectedCell, pe.Time
				a.window.Invalidate()
			}
		case pointer.Drag:
			if v.Resize != nil {
				pos := float64(pe.Position.X)
				if v.Resize.State.Axis == resizeRows {
					pos = float64(pe.Position.Y)
				}
				v.updateResize(pos)
				v.ResizeCursor = v.Resize.State.Axis
				a.window.Invalidate()
			}
		case pointer.Release:
			if v.Resize != nil {
				if a.saving {
					v.cancelResize()
					v.ResizeCursor = 0
					a.status = "正在保存，请稍后调整行列尺寸"
					a.window.Invalidate()
					break
				}
				pos := float64(pe.Position.X)
				if v.Resize.State.Axis == resizeRows {
					pos = float64(pe.Position.Y)
				}
				v.updateResize(pos)
				if err := v.completeResize(a.book.(document.GridReader), a.sheet); err != nil {
					a.status = "尺寸调整失败：" + err.Error()
				} else {
					a.status = "行列尺寸已调整；点击保存写入原文件"
				}
				v.ResizeCursor = 0
				a.window.Invalidate()
			}
		case pointer.Cancel:
			if v.Resize != nil {
				v.cancelResize()
				v.ResizeCursor = 0
				a.window.Invalidate()
			}
		}
	}
	area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
	event.Op(gtx.Ops, &v.PointerTag)
	key.InputHintOp{Tag: &v.PointerTag, Hint: key.HintText}.Add(gtx.Ops)
	pointer.PassOp{}.Push(gtx.Ops)
	if v.ResizeCursor == resizeColumns {
		pointer.CursorEastWestResize.Add(gtx.Ops)
	} else if v.ResizeCursor == resizeRows {
		pointer.CursorNorthSouthResize.Add(gtx.Ops)
	}
	dims := paintVirtualGrid(gtx, a, v)
	area.Pop()
	return dims
}

func (a *App) requestGridWindow(v *gridViewport, reader document.GridReader, sheet string, r document.CellRange) {
	if r.StartRow <= 0 || r.StartColumn <= 0 {
		return
	}
	v.Loading = true
	v.RequestedRange = r
	v.RequestedSheet = sheet
	generation := a.documentGeneration
	rows, cols := r.EndRow-r.StartRow+1, r.EndColumn-r.StartColumn+1
	go func() {
		w, err := reader.Window(sheet, r.StartRow, r.StartColumn, rows, cols)
		select {
		case a.gridLoadResults <- gridLoadResult{generation: generation, viewport: v, sheet: sheet, rangeOf: r, window: w, err: err}:
			a.window.Invalidate()
		case <-a.shutdown:
		}
	}()
	a.status = fmt.Sprintf("正在读取工作表「%s」的可见单元格…", sheet)
	a.window.Invalidate()
}

func (a *App) pollGridLoad(v *gridViewport) {
	for {
		select {
		case result := <-a.gridLoadResults:
			if result.viewport != v {
				continue
			}
			v.Loading = false
			if result.generation != a.documentGeneration || result.sheet != a.sheet {
				a.window.Invalidate()
				continue
			}
			if result.err != nil {
				v.WindowError = result.err
				v.FailedRange = result.rangeOf
				v.FailedSheet = result.sheet
				a.status = gridWindowError(result.sheet, result.err)
				a.window.Invalidate()
				continue
			}
			previousSheet := v.WindowSheet
			_ = v.applyWindow(result.sheet, result.rangeOf, result.window)
			if previousSheet != result.sheet {
				start := xlsx.CellName(result.rangeOf.StartColumn, result.rangeOf.StartRow)
				end := xlsx.CellName(result.rangeOf.EndColumn, result.rangeOf.EndRow)
				a.status = fmt.Sprintf("工作表「%s」已读取 %s:%s（当前窗口）", a.sheet, start, end)
			}
			a.window.Invalidate()
		default:
			return
		}
	}
}

func windowCell(w document.SheetWindow, row, col int) document.Cell {
	for _, m := range w.Merges {
		if row == m.Range.StartRow && col == m.Range.StartColumn {
			return m.Anchor
		}
	}
	r, c := row-w.StartRow, col-w.StartColumn
	if r < 0 || r >= len(w.Cells) || c < 0 || c >= len(w.Cells[r]) {
		return document.Cell{}
	}
	return w.Cells[r][c]
}
