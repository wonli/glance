package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
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
	if ev, ok := gtx.Event(key.Filter{Focus: &a.formulaEditor, Name: key.NameEscape}); ok {
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press && a.documentSessionState.formulaReference.Active {
			a.restoreFormulaReferenceTarget()
			if a.virtualGrid != nil {
				gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
			}
		}
	}
	formulaCommitted := false
	for ev, ok := a.formulaEditor.Update(gtx); ok; ev, ok = a.formulaEditor.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit {
			if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
				a.setStatus("修改失败：" + err.Error())
				a.window.Invalidate()
			} else {
				formulaCommitted = true
				if a.virtualGrid != nil {
					gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
				}
			}
		}
	}
	if !formulaCommitted {
		a.syncFormulaReferenceInput(gtx)
	}
	if a.goToCellButton.Clicked(gtx) {
		a.goToCell()
	}
	if a.applyButton.Clicked(gtx) {
		if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
			a.setStatus("修改失败：" + err.Error())
			a.window.Invalidate()
		} else if a.virtualGrid != nil {
			gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
		}
	}
	height := gtx.Dp(unit.Dp(56))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	outerSize := gtx.Constraints.Max
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			barHeight := gtx.Dp(unit.Dp(44))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 244, G: 245, B: 246, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, barHeight)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			transform := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(19)))).Push(gtx.Ops)
			defer transform.Pop()
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(30)), gtx.Dp(unit.Dp(30))
			layout.Inset{Left: unit.Dp(20), Right: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutFormulaEditorField(gtx, a.theme, &a.nameBox, "A1", unit.Dp(110))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
					}),
					layout.Rigid(a.layoutFormulaFunctionIcon),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(10)), 0)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layoutFormulaEditorField(gtx, a.theme, &a.formulaEditor, "", 0)
					}),
				)
			})
			return layout.Dimensions{Size: outerSize}
		},
	)
}

func layoutFormulaEditorField(gtx layout.Context, theme *material.Theme, editor *widget.Editor, hint string, width unit.Dp) layout.Dimensions {
	height := gtx.Dp(unit.Dp(30))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	if width > 0 {
		fixedWidth := gtx.Dp(width)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = fixedWidth, fixedWidth
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(4)))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, gridLine, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				style := material.Editor(theme, editor, hint)
				style.TextSize = unit.Sp(13)
				style.Color = color.NRGBA{R: 76, G: 85, B: 96, A: 255}
				style.HintColor = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
				return style.Layout(gtx)
			})
		},
	)
}

func (a *App) layoutFormulaFunctionIcon(gtx layout.Context) layout.Dimensions {
	size := gtx.Dp(unit.Dp(20))
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	icon, _ := tintedToolbarIcon("sigma", color.NRGBA{R: 104, G: 113, B: 125, A: 255})
	return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 20.0 / 96.0}.Layout(gtx)
}

func (a *App) goToCell() {
	r, c, ok := parseCellAddress(a.nameBox.Text())
	if !ok || r > xlsxRows || c > xlsxColumns {
		a.setStatus("单元格地址无效")
		return
	}
	a.navigateToCell(r, c)
}

func (a *App) navigateToCell(row, column int) {
	if a.virtualGrid == nil || row < 1 || column < 1 || row > xlsxRows || column > xlsxColumns {
		return
	}
	v := a.virtualGrid
	a.updateGridSelection(v, CellPosition{Row: row - 1, Column: column - 1}, false, false)
	a.persistViewState()
	x, _ := v.Columns.Boundary(column - 1)
	y, _ := v.Rows.Boundary(row - 1)
	v.scroll(x-v.X, y-v.Y, gtxGridContentWidth(a), gtxGridContentHeight(a))
	v.invalidateWindow()
	v.PendingSelectionLoad = true
	if a.window != nil {
		a.window.Invalidate()
	}
}

func gtxGridContentWidth(a *App) float64  { return a.lastGridWidth }
func gtxGridContentHeight(a *App) float64 { return a.lastGridHeight }

func (a *App) layoutVirtualGrid(gtx layout.Context) layout.Dimensions {
	reader := a.book.(document.GridReader)
	if a.virtualGrid == nil {
		a.virtualGrid = newGridViewport()
	}
	v := a.virtualGrid
	v.setScale(float64(gtx.Metric.PxPerDp) * a.spreadsheetZoom())
	width, height := float64(gtx.Constraints.Max.X), float64(gtx.Constraints.Max.Y)
	rowHeaderPx := gtx.Dp(unit.Dp(gridRowHeader))
	columnHeaderPx := gtx.Dp(unit.Dp(gridColumnHeader))
	contentW, contentH := width-float64(rowHeaderPx), height-float64(columnHeaderPx)
	a.lastGridWidth, a.lastGridHeight = contentW, contentH
	visible := visibleCellWindow(v.Rows, v.Columns, v.X, v.Y, contentW, contentH)
	a.pollGridLoad(v)
	if imageReader, ok := any(a.book).(document.SpreadsheetImageReader); ok {
		a.requestGridPictureMetadata(a.documentSessionState, imageReader, a.sheet)
	} else if a.documentSessionState.gridPicturesSheet != "" {
		a.invalidateGridPictures(a.documentSessionState)
	}
	if !v.Loading && !(v.WindowSheet == a.sheet && containsRange(v.WindowRange, visible)) && !(v.FailedSheet == a.sheet && v.FailedRange == visible) {
		a.requestGridWindow(v, reader, a.sheet, visible)
	}
	if v.GeometryChanged {
		v.GeometryChanged = false
		a.window.Invalidate()
	}
	if v.PendingSelectionLoad && v.SelectedRow >= v.WindowRange.StartRow && v.SelectedRow <= v.WindowRange.EndRow && v.SelectedColumn >= v.WindowRange.StartColumn && v.SelectedColumn <= v.WindowRange.EndColumn {
		cell := windowCell(v.Window, v.SelectedRow, v.SelectedColumn)
		if !a.documentSessionState.formulaReference.Active {
			value := cellEditorText(cell)
			a.cellEdit.selectCell(a.selectedCell, value)
			a.formulaEditor.SetText(value)
		}
		v.PendingSelectionLoad = false
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &a.documentSessionState.clipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		if data, yes := ev.(transfer.DataEvent); yes {
			reader := data.Open()
			raw, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				a.setStatus("读取剪贴板失败：" + err.Error())
			} else if err := a.pasteClipboard(string(raw)); err != nil {
				a.setStatus("粘贴失败：" + err.Error())
			}
		}
	}
	for {
		if a.cellEdit.Editing {
			if ev, ok := gtx.Event(key.Filter{Focus: &a.cellEditor, Name: key.NameEscape}); ok {
				if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
					if a.documentSessionState.formulaReference.Active {
						a.restoreFormulaReferenceTarget()
					} else {
						a.cellEdit.cancel()
						a.cellEditor.SetText(a.cellEdit.Original)
						a.formulaEditor.SetText(a.cellEdit.Original)
					}
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
					a.window.Invalidate()
				}
			}
			for ev, ok := a.cellEditor.Update(gtx); ok; ev, ok = a.cellEditor.Update(gtx) {
				if _, changed := ev.(widget.ChangeEvent); changed {
					a.cellEdit.setDraft(a.cellEditor.Text())
					start, end := a.cellEditor.Selection()
					a.formulaEditor.SetText(a.cellEditor.Text())
					a.formulaEditor.SetCaret(start, end)
				}
				if _, submit := ev.(widget.SubmitEvent); submit {
					if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
						a.setStatus("修改失败：" + err.Error())
						a.window.Invalidate()
						gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
					} else {
						gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
					}
				}
			}
		}
		a.syncFormulaReferenceInput(gtx)
		ev, ok := gtx.Event(key.Filter{Focus: &v.PointerTag})
		if !ok {
			break
		}
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
			if ke.Name == key.NameEscape && a.documentSessionState != nil && a.documentSessionState.excelFormatPainterActive {
				a.cancelSpreadsheetFormatPainter()
				a.window.Invalidate()
			} else if ke.Name == key.NameF2 && a.selectedCell != "" {
				if a.cellEdit.begin() {
					a.cellEditor.SetText(a.cellEdit.Original)
					gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
					a.window.Invalidate()
				}
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
			if a.documentSessionState.gridPictureGesture.Mode == gridPictureGestureNone {
				v.PictureResizeCursor = false
				v.PictureMoveCursor = false
			}
		case pointer.Scroll:
			dx, dy := gridScrollDelta(float64(pe.Scroll.X), float64(pe.Scroll.Y), pe.Modifiers.Contain(key.ModShift))
			v.scroll(dx, dy, contentW, contentH)
			if a.documentSessionState.gridPictureGesture.Mode != gridPictureGestureNone {
				point := gridPictureContentPointer(v, pe.Position, rowHeaderPx, columnHeaderPx)
				a.documentSessionState.gridPictureGesture.rebase(point)
			}
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
			v.PictureResizeCursor, v.PictureMoveCursor = false, false
			if v.Resize == nil && v.ResizeCursor == 0 {
				pictureHit := gridPictureHitAt(v, activeGridPictures(a.documentSessionState, a.sheet), a.documentSessionState.selectedGridPictureID, image.Pt(int(pe.Position.X), int(pe.Position.Y)), image.Rect(rowHeaderPx, columnHeaderPx, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), rowHeaderPx, columnHeaderPx, gridPictureResizeHandleSize(gtx))
				v.PictureResizeCursor = pictureHit.ResizeHandle
				v.PictureMoveCursor = pictureHit.ImageID != "" && !pictureHit.ResizeHandle
			}
			if v.Resize != nil {
				v.ResizeCursor = v.Resize.State.Axis
			}
			if a.documentSessionState.gridPictureGesture.Mode == gridPictureGestureResize {
				v.PictureResizeCursor = true
			} else if a.documentSessionState.gridPictureGesture.Mode == gridPictureGestureMove {
				v.PictureMoveCursor = true
			}
		case pointer.Press:
			if a.cellEdit.Editing {
				editRect := editingCellRect(v, a.cellEdit.Address, rowHeaderPx, columnHeaderPx)
				point := image.Pt(int(pe.Position.X), int(pe.Position.Y))
				formulaReference := a.documentSessionState.formulaReference
				if formulaReference.Active && formulaReference.InCell && formulaReference.TargetSheet == a.sheet && point.In(editRect) {
					continue
				}
				if point.In(editRect) && (!formulaReference.Active || formulaReference.TargetSheet == a.sheet) {
					// The editor's own pointer handler places the caret. Let it consume
					// the event without changing the selected cell or ending editing.
					continue
				}
				if !a.documentSessionState.formulaReference.Active {
					if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
						a.setStatus("修改失败：" + err.Error())
						gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
						a.window.Invalidate()
						continue
					}
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
			state := a.documentSessionState
			pictureHit := gridPictureHitAt(v, activeGridPictures(state, a.sheet), state.selectedGridPictureID, image.Pt(int(pe.Position.X), int(pe.Position.Y)), image.Rect(rowHeaderPx, columnHeaderPx, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), rowHeaderPx, columnHeaderPx, gridPictureResizeHandleSize(gtx))
			if pictureHit.ImageID != "" {
				setSelectedGridPicture(state, pictureHit.ImageID)
				a.selectionDragging = false
				if picture, found := gridPictureByID(state.gridPictures, pictureHit.ImageID); found && picture.Editable && !state.formatRunning.Load() && !a.isSaving() {
					mode := gridPictureGestureMove
					if pictureHit.ResizeHandle {
						mode = gridPictureGestureResize
					}
					geometry := gridPictureContentGeometry(v, picture)
					point := gridPictureContentPointer(v, pe.Position, rowHeaderPx, columnHeaderPx)
					state.gridPictureGesture.begin(picture.ID, mode, geometry, point)
					gtx.Execute(pointer.GrabCmd{Tag: &v.PointerTag, ID: pe.PointerID})
				}
				gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				v.PictureResizeCursor = pictureHit.ResizeHandle
				v.PictureMoveCursor = !pictureHit.ResizeHandle
				a.window.Invalidate()
				break
			}
			setSelectedGridPicture(state, "")
			row, col, hit := cellAtPoint(v.Rows, v.Columns, v.X, v.Y, image.Pt(int(pe.Position.X), int(pe.Position.Y)), gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
			if hit {
				position := selectionAnchor(v.Window.Merges, CellPosition{Row: row - 1, Column: col - 1})
				extend := pe.Modifiers.Contain(key.ModShift)
				if a.beginFormulaReferenceDrag(position, extend) {
					a.selectionDragging = false
					gtx.Execute(pointer.GrabCmd{Tag: &v.PointerTag, ID: pe.PointerID})
					if editor := a.formulaReferenceEditor(); editor != nil {
						gtx.Execute(key.FocusCmd{Tag: editor})
					}
					v.LastClickTime = 0
					a.window.Invalidate()
					break
				}
				row, col = position.Row+1, position.Column+1
				previousAddress := a.selectedCell
				address := xlsx.CellName(col, row)
				a.updateGridSelection(v, position, extend, false)
				a.selectionDragging = true
				gtx.Execute(pointer.GrabCmd{Tag: &v.PointerTag, ID: pe.PointerID})
				if !extend && pe.Time-v.LastClickTime <= 500*time.Millisecond && v.LastClickAddress == address && previousAddress == address && (state == nil || !state.excelFormatPainterActive) {
					cell := windowCell(v.Window, row, col)
					a.cellEdit.begin()
					a.cellEditor.SetText(cellEditorText(cell))
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
			} else if a.documentSessionState.gridPictureGesture.Mode != gridPictureGestureNone {
				point := gridPictureContentPointer(v, pe.Position, rowHeaderPx, columnHeaderPx)
				a.documentSessionState.gridPictureGesture.update(point)
				if a.documentSessionState.gridPictureGesture.Mode == gridPictureGestureResize {
					v.PictureResizeCursor = true
				} else {
					v.PictureMoveCursor = true
				}
				a.window.Invalidate()
			} else if a.documentSessionState.formulaReference.Active && a.documentSessionState.formulaReference.Dragging {
				if position, hit := selectionAtPointWithHeaders(v, image.Pt(int(pe.Position.X), int(pe.Position.Y)), rowHeaderPx, columnHeaderPx); hit {
					position = selectionAnchor(v.Window.Merges, position)
					a.extendFormulaReferenceDrag(position)
					a.window.Invalidate()
				}
			} else if a.selectionDragging {
				if position, hit := selectionAtPointWithHeaders(v, image.Pt(int(pe.Position.X), int(pe.Position.Y)), rowHeaderPx, columnHeaderPx); hit {
					position = selectionAnchor(v.Window.Merges, position)
					a.updateGridSelection(v, position, true, false)
					a.window.Invalidate()
				}
			}
		case pointer.Release:
			if v.Resize != nil {
				if a.isSaving() {
					v.cancelResize()
					v.ResizeCursor = 0
					a.setStatus("正在保存，请稍后调整行列尺寸")
					a.window.Invalidate()
					break
				}
				pos := float64(pe.Position.X)
				if v.Resize.State.Axis == resizeRows {
					pos = float64(pe.Position.Y)
				}
				v.updateResize(pos)
				state := a.documentSessionState
				token, ok := state.beginOperation(sessionOperationMutation)
				if !ok {
					a.setStatus("正在处理其他操作，请稍后重试")
					v.cancelResize()
					v.ResizeCursor = 0
					a.window.Invalidate()
					break
				}
				unlock := lockSessionAdapterWrite(state)
				err := v.completeResize(a.book.(document.GridReader), a.sheet)
				unlock()
				a.completeSessionOperation(state, token, err, err == nil)
				if err != nil {
					a.setStatus("尺寸调整失败：" + err.Error())
				} else {
					a.setStatus("行列尺寸已调整；点击保存写入原文件")
					a.persistActiveSession()
				}
				v.ResizeCursor = 0
				a.window.Invalidate()
			} else if a.documentSessionState.gridPictureGesture.Mode != gridPictureGestureNone {
				state := a.documentSessionState
				mode, id := state.gridPictureGesture.Mode, state.gridPictureGesture.ImageID
				preview, changed := state.gridPictureGesture.finish()
				v.PictureResizeCursor, v.PictureMoveCursor = false, false
				if changed {
					if picture, found := gridPictureByID(state.gridPictures, id); found {
						target, err := gridPicturePlacementAt(v, picture, preview)
						if err != nil {
							a.setStatus("图片位置无效：" + err.Error())
						} else {
							a.startGridPictureMoveResize(state, picture, mode, target, preview)
						}
					}
				}
				a.window.Invalidate()
			} else if a.documentSessionState.formulaReference.Active && a.documentSessionState.formulaReference.Dragging {
				a.documentSessionState.formulaReference.Dragging = false
				a.window.Invalidate()
			} else if a.selectionDragging {
				a.selectionDragging = false
				a.persistViewState()
				if a.documentSessionState != nil && a.documentSessionState.excelFormatPainterActive {
					a.applySpreadsheetFormatPainter()
				}
				a.window.Invalidate()
			}
		case pointer.Cancel:
			if v.Resize != nil {
				v.cancelResize()
				v.ResizeCursor = 0
				a.window.Invalidate()
			} else if a.documentSessionState.gridPictureGesture.Mode != gridPictureGestureNone {
				a.documentSessionState.gridPictureGesture.cancel()
				v.PictureResizeCursor, v.PictureMoveCursor = false, false
				a.window.Invalidate()
			} else {
				a.documentSessionState.formulaReference.Dragging = false
				a.selectionDragging = false
			}
		}
	}
	gridBounds := image.Rectangle{Max: gtx.Constraints.Max}
	gridShape := clip.UniformRRect(gridBounds, gtx.Dp(unit.Dp(4)))
	area := gridShape.Push(gtx.Ops)
	event.Op(gtx.Ops, &v.PointerTag)
	key.InputHintOp{Tag: &v.PointerTag, Hint: key.HintText}.Add(gtx.Ops)
	pointer.PassOp{}.Push(gtx.Ops)
	if v.ResizeCursor == resizeColumns {
		pointer.CursorEastWestResize.Add(gtx.Ops)
	} else if v.ResizeCursor == resizeRows {
		pointer.CursorNorthSouthResize.Add(gtx.Ops)
	} else if v.PictureResizeCursor {
		pointer.CursorSouthEastResize.Add(gtx.Ops)
	} else if v.PictureMoveCursor {
		if a.documentSessionState.gridPictureGesture.Mode == gridPictureGestureMove {
			pointer.CursorGrabbing.Add(gtx.Ops)
		} else {
			pointer.CursorGrab.Add(gtx.Ops)
		}
	} else if a.documentSessionState != nil && a.documentSessionState.excelFormatPainterActive {
		pointer.CursorCrosshair.Add(gtx.Ops)
	}
	dims := paintVirtualGrid(gtx, a, v)
	if a.documentSessionState == nil || !a.documentSessionState.spreadsheetGridlinesHidden {
		paint.FillShape(gtx.Ops, gridLine, clip.Stroke{Path: gridShape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	}
	area.Pop()
	return dims
}

func (a *App) updateGridSelection(v *gridViewport, position CellPosition, extend, persist bool) {
	state := a.documentSessionState
	if state == nil || v == nil {
		return
	}
	setSelectedGridPicture(state, "")
	state.gridPictureGesture.cancel()
	state.selection = updatedSelection(state.selection, state.selectionSet, position, extend)
	state.selectionSet = true
	position = state.selection.Active
	row, column := position.Row+1, position.Column+1
	v.SelectedRow, v.SelectedColumn = row, column
	a.selectedCell = xlsx.CellName(column, row)
	cell := document.Cell{}
	if v.WindowSheet == a.sheet && row >= v.WindowRange.StartRow && row <= v.WindowRange.EndRow && column >= v.WindowRange.StartColumn && column <= v.WindowRange.EndColumn {
		cell = windowCell(v.Window, row, column)
	} else {
		v.PendingSelectionLoad = true
	}
	value := cellEditorText(cell)
	a.cellEdit.selectCell(a.selectedCell, value)
	a.nameBox.SetText(a.selectedCell)
	a.formulaEditor.SetText(value)
	if persist {
		a.persistViewState()
	}
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
		unlock := lockSessionAdapterRead(a.documentSessionState)
		w, err := reader.Window(sheet, r.StartRow, r.StartColumn, rows, cols)
		unlock()
		select {
		case a.gridLoadResults <- gridLoadResult{generation: generation, viewport: v, sheet: sheet, rangeOf: r, window: w, err: err}:
			a.window.Invalidate()
		case <-a.shutdown:
		}
	}()
	a.setStatus(fmt.Sprintf("正在读取工作表「%s」的可见单元格…", sheet))
	a.window.Invalidate()
}

func (a *App) pollGridLoad(v *gridViewport) {
	for {
		select {
		case result := <-a.gridLoadResults:
			if result.viewport == nil || result.viewport.RequestedSheet != result.sheet || result.viewport.RequestedRange != result.rangeOf {
				continue
			}
			result.viewport.Loading = false
			if result.viewport != v {
				continue
			}
			if result.generation != a.documentGeneration || result.sheet != a.sheet {
				a.window.Invalidate()
				continue
			}
			if result.err != nil {
				v.WindowError = result.err
				v.FailedRange = result.rangeOf
				v.FailedSheet = result.sheet
				a.setStatus(gridWindowError(result.sheet, result.err))
				a.window.Invalidate()
				continue
			}
			_ = v.applyWindow(result.sheet, result.rangeOf, result.window)
			a.applyValueFilterWindow(a.documentSessionState, v)
			a.setStatus("就绪")
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
