package desktopui

import (
	"glance/internal/i18n"
	"image"
	"io"
	"math"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
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

func gtxGridContentWidth(a *App) float64  { return a.lastGridWidth }
func gtxGridContentHeight(a *App) float64 { return a.lastGridHeight }

func (a *App) layoutVirtualGrid(gtx layout.Context) layout.Dimensions {
	recordNativePinchSurface(gtx)
	reader := a.book.(document.GridReader)
	if a.virtualGrid == nil {
		if a.book != nil {
			a.virtualGrid = newGridViewportForSpreadsheet(a.book, a.sheet)
		} else {
			a.virtualGrid = newGridViewport()
		}
	}
	v := a.virtualGrid
	if v.WindowSheet != "" && v.WindowSheet != a.sheet {
		v.resetSheetWindow()
	}
	if v.KeyboardFocusRequested {
		gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
		v.KeyboardFocusRequested = false
	}
	if state := a.documentSessionState; state != nil && state.gridEndMode && !gtx.Focused(&v.PointerTag) {
		state.gridEndMode = false
		state.gridNavigationSequence++
	}
	v.setScale(float64(gtx.Metric.PxPerDp) * a.spreadsheetZoom())
	a.pollSpreadsheetGridNavigation(gtx, v)
	width, height := float64(gtx.Constraints.Max.X), float64(gtx.Constraints.Max.Y)
	rowHeaderPx := gtx.Dp(unit.Dp(gridRowHeader))
	columnHeaderPx := gtx.Dp(unit.Dp(gridColumnHeader))
	contentW, contentH := width-float64(rowHeaderPx), height-float64(columnHeaderPx)
	a.lastGridWidth, a.lastGridHeight = contentW, contentH
	if v.ScrollbarWidthPx <= 0 {
		v.ScrollbarWidthPx = gtx.Dp(unit.Dp(10))
	}
	a.pollGridLoad(v)
	visible := v.visiblePaneRanges(contentW, contentH)
	if imageReader, ok := any(a.book).(document.SpreadsheetImageReader); ok {
		a.requestGridPictureMetadata(a.documentSessionState, imageReader, a.sheet)
	} else if a.documentSessionState.gridPicturesSheet != "" {
		a.invalidateGridPictures(a.documentSessionState)
	}
	if !v.Loading && !v.windowSetLoaded(a.sheet, visible) && !(v.FailedSheet == a.sheet && v.FailedWindows == visible) {
		a.requestGridWindow(v, reader, a.sheet, visible)
	}
	if v.GeometryChanged {
		v.GeometryChanged = false
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if v.PendingSelectionLoad && v.cellLoaded(v.SelectedRow, v.SelectedColumn) {
		cell := v.cell(v.SelectedRow, v.SelectedColumn)
		if !a.documentSessionState.formulaReference.Active {
			value := cellEditorText(cell)
			a.cellEdit.selectCell(a.selectedCell, value)
			if a.selectionSet {
				a.formulaEditor.SetText(value)
			} else {
				a.formulaEditor.SetText("")
			}
		}
		v.PendingSelectionLoad = false
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &a.documentSessionState.clipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		state := a.documentSessionState
		target := state.clipboardPasteTarget
		state.clipboardPasteTarget = nil
		if _, cancelled := ev.(transfer.CancelEvent); cancelled {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.clipboard_read_canceled_7cb21f"}, feedbackSuccess, notificationSuppressed)
			continue
		}
		data, yes := ev.(transfer.DataEvent)
		if !yes {
			continue
		}
		if target == nil {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_paste_target_expired_paste_again_a7b7b9"}, feedbackWarning, notificationSuppressed)
			continue
		}
		reader := data.Open()
		raw, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		} else if err := a.pasteClipboardAt(target.sheet, target.bounds, string(raw)); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		}
	}
	for {
		ev, ok := gtx.Event(key.FocusFilter{Target: &v.PointerTag})
		if !ok {
			break
		}
		a.handleSpreadsheetGridFocusEvent(gtx, v, ev)
	}
	for {
		if a.cellEdit.Editing && !a.documentSessionState.formatRunning.Load() {
			if ev, ok := gtx.Event(key.Filter{Focus: &a.cellEditor, Name: key.NameEscape}); ok {
				if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
					if a.documentSessionState.formulaReference.Active {
						a.restoreFormulaReferenceTarget()
					} else {
						a.cellEdit.cancel()
						v.cancelCellEditRowPreview()
						a.cellEditor.SetText(a.cellEdit.Original)
						a.formulaEditor.SetText(a.cellEdit.Original)
					}
					if a.documentSessionState != nil {
						a.documentSessionState.gridInputComposing = false
					}
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
					a.window.Invalidate()
				}
			}
			if !a.documentSessionState.formatRunning.Load() && !a.handleSpreadsheetEditorNavigation(gtx, v, &a.cellEditor) {
				for ev, ok := a.cellEditor.Update(gtx); ok; ev, ok = a.cellEditor.Update(gtx) {
					if _, changed := ev.(widget.ChangeEvent); changed {
						a.cellEdit.setDraft(a.cellEditor.Text())
						start, end := a.cellEditor.Selection()
						a.formulaEditor.SetText(a.cellEditor.Text())
						a.formulaEditor.SetCaret(start, end)
					}
					if _, submit := ev.(widget.SubmitEvent); submit {
						if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
							a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
							a.window.Invalidate()
							gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
						} else {
							a.moveSpreadsheetSelectionAfterEntry(v, key.NameDownArrow)
							gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
						}
					}
				}
			}
		}
		a.syncFormulaReferenceInput(gtx)
		ev, ok := gtx.Event(key.Filter{Focus: &v.PointerTag, Optional: key.ModShift | key.ModShortcut | key.ModCtrl | key.ModAlt})
		if !ok {
			break
		}
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
			if a.modal.Visible {
				continue
			}
			if a.cellEdit.Editing && ke.Modifiers == 0 && (ke.Name == key.NameDeleteBackward || ke.Name == key.NameDeleteForward) {
				direction := -1
				if ke.Name == key.NameDeleteForward {
					direction = 1
				}
				if a.cellEditor.Delete(direction) != 0 {
					a.syncSpreadsheetGridTextDraft()
					a.updateSpreadsheetGridInputSnippet(gtx, v)
					a.window.Invalidate()
				}
			} else if a.cellEdit.Editing && ke.Modifiers == 0 && ke.Name == key.NameEscape {
				a.cellEdit.cancel()
				v.cancelCellEditRowPreview()
				a.cellEditor.SetText(a.cellEdit.Original)
				a.formulaEditor.SetText(a.cellEdit.Original)
				a.documentSessionState.gridInputComposing = false
				gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				a.window.Invalidate()
			} else if a.cellEdit.Editing && !a.documentSessionState.gridInputComposing && ke.Modifiers == 0 && (ke.Name == key.NameReturn || ke.Name == key.NameEnter) {
				a.commitSpreadsheetGridTextEntry(gtx, v, key.NameDownArrow)
			} else if a.cellEdit.Editing && !a.documentSessionState.gridInputComposing && (ke.Name == key.NameTab && (ke.Modifiers == 0 || ke.Modifiers == key.ModShift)) {
				direction := key.NameRightArrow
				if ke.Modifiers == key.ModShift {
					direction = key.NameLeftArrow
				}
				a.commitSpreadsheetGridTextEntry(gtx, v, direction)
			} else if a.handleSpreadsheetGridNavigationKey(gtx, v, ke) {
				continue
			} else if ke.Modifiers == 0 && !a.cellEdit.Editing && ke.Name == key.NameDeleteBackward && a.beginSpreadsheetCellBackspaceEdit(gtx, v) {
				continue
			} else if ke.Modifiers == 0 && !a.cellEdit.Editing && (ke.Name == key.NameDeleteBackward || ke.Name == key.NameDeleteForward) {
				a.clearSpreadsheetSelectionContents("清除内容")
			} else if ke.Modifiers == 0 && ke.Name == key.NameEscape && a.documentSessionState != nil && a.documentSessionState.gridEndMode {
				a.documentSessionState.gridEndMode = false
				a.documentSessionState.gridNavigationSequence++
				a.window.Invalidate()
			} else if ke.Modifiers == 0 && ke.Name == key.NameEscape && a.documentSessionState != nil && a.documentSessionState.excelFormatPainterActive {
				a.cancelSpreadsheetFormatPainter()
				a.window.Invalidate()
			} else if ke.Modifiers == 0 && ke.Name == key.NameF2 && a.selectedCell != "" {
				a.beginSpreadsheetCellEditFromF2(gtx, v)
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
			v.LastResizeClickAxis = 0
			if zoomScrollModifier(pe.Modifiers) {
				state := a.documentSessionState
				scrollDelta := zoomScrollAxisDelta(pe.Scroll)
				zoom, remainder, changed := zoomFromScroll(a.spreadsheetZoom(), scrollDelta, zoomScrollScale(), state.zoomScrollRemainder, spreadsheetZoomMinimum, spreadsheetZoomMaximum)
				state.zoomScrollRemainder = remainder
				if changed {
					a.setSpreadsheetZoom(zoom)
					v.zoomAt(float64(gtx.Metric.PxPerDp)*zoom, float64(pe.Position.X)-float64(rowHeaderPx), float64(pe.Position.Y)-float64(columnHeaderPx), contentW, contentH)
				}
				if a.window != nil {
					a.window.Invalidate()
				}
				break
			}
			dx, dy := gridScrollDelta(float64(pe.Scroll.X), float64(pe.Scroll.Y), pe.Modifiers.Contain(key.ModShift))
			v.scroll(dx, dy, contentW, contentH)
			if a.documentSessionState.gridPictureGesture.Mode != gridPictureGestureNone {
				point := gridPictureContentPointer(v, pe.Position, rowHeaderPx, columnHeaderPx, gtx.Constraints.Max)
				a.documentSessionState.gridPictureGesture.rebase(point)
			}
			if v.Resize != nil {
				rs := v.Resize.State
				edge := resizeScreenEdge(v, rs, gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
				point := rs.LastPointer
				rs.rebase(point, edge)
				v.updateResize(point)
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		case pointer.Move:
			if v.ScrollbarWidthPx > 0 && int(pe.Position.X) >= gtx.Constraints.Max.X-v.ScrollbarWidthPx && int(pe.Position.Y) >= columnHeaderPx {
				v.ResizeCursor = 0
				v.PictureResizeCursor, v.PictureMoveCursor = false, false
				break
			}
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
			if state := a.documentSessionState; state != nil && state.gridEndMode {
				state.gridEndMode = false
				state.gridNavigationSequence++
			}
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
						a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
						gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
						a.window.Invalidate()
						continue
					}
				}
			}
			if v.ScrollbarWidthPx > 0 && int(pe.Position.X) >= gtx.Constraints.Max.X-v.ScrollbarWidthPx && int(pe.Position.Y) >= columnHeaderPx {
				v.LastResizeClickAxis = 0
				break
			}
			axis, index, edge, sizePx, resizeHit := gridResizeHandleAt(v, int(pe.Position.X), int(pe.Position.Y), gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
			if resizeHit {
				pos := float64(pe.Position.X)
				if axis == resizeRows {
					pos = float64(pe.Position.Y)
				}
				doubleClick := v.LastResizeClickAxis == axis && v.LastResizeClickIndex == index &&
					pe.Time >= v.LastResizeClickTime && pe.Time-v.LastResizeClickTime <= 500*time.Millisecond
				v.LastResizeClickAxis = 0
				fileUnits := devicePixelsToPoints(sizePx, v.Scale)
				if axis == resizeColumns {
					fileUnits = devicePixelsToChars(sizePx, v.Scale)
				}
				v.beginResize(axis, index, pos, edge)
				v.Resize.State.OriginalFile = fileUnits
				v.Resize.AutoFitOnDoubleClick = doubleClick
				v.ResizeCursor = axis
				gtx.Execute(pointer.GrabCmd{Tag: &v.PointerTag, ID: pe.PointerID})
				break
			}
			v.LastResizeClickAxis = 0
			state := a.documentSessionState
			if scope, position, hit := gridHeaderAtPoint(v, image.Pt(int(pe.Position.X), int(pe.Position.Y)), rowHeaderPx, columnHeaderPx); hit {
				a.updateGridHeaderSelection(v, scope, position, pe.Modifiers.Contain(key.ModShift))
				a.selectionDragging = true
				v.LastClickTime = 0
				gtx.Execute(pointer.GrabCmd{Tag: &v.PointerTag, ID: pe.PointerID})
				gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				a.window.Invalidate()
				break
			}
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
					point := gridPictureContentPointer(v, pe.Position, rowHeaderPx, columnHeaderPx, gtx.Constraints.Max)
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
			row, col, hit := v.cellPoint(image.Pt(int(pe.Position.X), int(pe.Position.Y)), gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
			if hit {
				position := selectionAnchor(v.merges(), CellPosition{Row: row - 1, Column: col - 1})
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
					cell := v.cell(row, col)
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
				point := gridPictureContentPointer(v, pe.Position, rowHeaderPx, columnHeaderPx, gtx.Constraints.Max)
				a.documentSessionState.gridPictureGesture.update(point)
				if a.documentSessionState.gridPictureGesture.Mode == gridPictureGestureResize {
					v.PictureResizeCursor = true
				} else {
					v.PictureMoveCursor = true
				}
				a.window.Invalidate()
			} else if a.documentSessionState.formulaReference.Active && a.documentSessionState.formulaReference.Dragging {
				if position, hit := selectionAtPointWithHeaders(v, image.Pt(int(pe.Position.X), int(pe.Position.Y)), rowHeaderPx, columnHeaderPx); hit {
					position = selectionAnchor(v.merges(), position)
					a.extendFormulaReferenceDrag(position)
					a.window.Invalidate()
				}
			} else if a.selectionDragging {
				if a.selection.Scope != selectionCells {
					a.extendGridHeaderSelection(v, pe.Position, rowHeaderPx, columnHeaderPx)
					a.window.Invalidate()
					break
				}
				if position, hit := selectionAtPointWithHeaders(v, image.Pt(int(pe.Position.X), int(pe.Position.Y)), rowHeaderPx, columnHeaderPx); hit {
					position = selectionAnchor(v.merges(), position)
					a.updateGridSelection(v, position, true, false)
					a.window.Invalidate()
				}
			}
		case pointer.Release:
			if v.Resize != nil {
				if a.isSaving() {
					v.cancelResize()
					v.LastResizeClickAxis = 0
					v.ResizeCursor = 0
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.saving_is_in_progress_try_resizing_rows_or_b4e00d"}, feedbackInfo, notificationSuppressed)
					a.window.Invalidate()
					break
				}
				pos := float64(pe.Position.X)
				if v.Resize.State.Axis == resizeRows {
					pos = float64(pe.Position.Y)
				}
				v.updateResize(pos)
				resizedAxis, resizedIndex := v.Resize.State.Axis, v.Resize.State.Index
				moved := math.Abs(v.Resize.State.PreviewPx-v.Resize.State.OriginalPx) > 0.5
				if !moved {
					doubleClick := v.Resize.AutoFitOnDoubleClick
					v.cancelResize()
					v.ResizeCursor = 0
					if doubleClick {
						v.LastResizeClickAxis = 0
						a.startSpreadsheetResizeAutoFit(gtx, resizedAxis, resizedIndex)
					} else {
						v.LastResizeClickAxis = resizedAxis
						v.LastResizeClickIndex = resizedIndex
						v.LastResizeClickTime = pe.Time
					}
					a.window.Invalidate()
					break
				}
				v.LastResizeClickAxis = 0
				state := a.documentSessionState
				token, ok := state.beginOperation(sessionOperationMutation)
				if !ok {
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.another_operation_is_in_progress_try_again_17714f"}, feedbackInfo, notificationSuppressed)
					v.cancelResize()
					v.LastResizeClickAxis = 0
					v.ResizeCursor = 0
					a.window.Invalidate()
					break
				}
				unlock := lockSessionAdapterWrite(state)
				err := v.completeResize(a.book.(document.GridReader), a.sheet)
				unlock()
				a.completeSessionOperation(state, token, err, err == nil)
				if err != nil {
					a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
				} else {
					if resizedAxis == resizeRows {
						state.markGridRowHeightManual(a.sheet, resizedIndex)
					}
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.row_or_column_size_changed_save_to_write_i_0bcdc4"}, feedbackInfo, notificationSuppressed)
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
							a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
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
			v.LastResizeClickAxis = 0
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
	pass := pointer.PassOp{}.Push(gtx.Ops)
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
	pass.Pop()
	a.layoutGridVerticalScrollbar(gtx, v, contentW, contentH, columnHeaderPx)
	area.Pop()
	return dims
}

func (a *App) layoutGridVerticalScrollbar(gtx layout.Context, v *gridViewport, contentW, contentH float64, columnHeaderPx int) layout.Dimensions {
	if v == nil || v.Rows == nil || gtx.Constraints.Max.Y <= columnHeaderPx {
		return layout.Dimensions{}
	}
	trackLen := gtx.Constraints.Max.Y - columnHeaderPx
	if trackLen <= 1 {
		return layout.Dimensions{}
	}

	theme := a.theme
	if theme == nil {
		theme = material.NewTheme()
	}
	style := material.Scrollbar(theme, &v.VerticalScrollbar)
	trackWidth := gtx.Dp(style.Width())
	if trackWidth <= 0 {
		trackWidth = gtx.Dp(unit.Dp(10))
	}
	v.ScrollbarWidthPx = trackWidth

	frozenWidth, frozenHeight := v.frozenViewportExtent(contentW, contentH)
	_ = frozenWidth
	bodyHeight := math.Max(0, contentH-frozenHeight)
	actualHeight := v.actualContentHeight(a)
	totalHeight := math.Max(actualHeight, v.frozenRowBoundary()+v.Y+bodyHeight)
	maxY := math.Max(0, totalHeight-v.frozenRowBoundary()-bodyHeight)

	start, end := float32(0), float32(1)
	if maxY > 0 && totalHeight > 0 {
		minThumbPx := min(trackLen-1, max(16, gtx.Dp(style.Indicator.MajorMinLen)))
		rawThumbPx := int(math.Round(float64(bodyHeight) * float64(trackLen) / float64(totalHeight)))
		thumbPx := max(minThumbPx, min(trackLen-1, rawThumbPx))
		travelPx := trackLen - thumbPx
		startPx := int(math.Round(float64(v.Y) * float64(travelPx) / float64(maxY)))
		startPx = max(0, min(travelPx, startPx))
		start = float32(startPx) / float32(trackLen)
		end = float32(startPx+thumbPx) / float32(trackLen)
	}
	v.ScrollbarStart, v.ScrollbarEnd = start, end

	trackClip := clip.Rect{
		Min: image.Pt(max(0, gtx.Constraints.Max.X-trackWidth), columnHeaderPx),
		Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y),
	}.Push(gtx.Ops)
	semantic.DescriptionOp(a.textID("grid.accessibility.vertical_scrollbar")).Add(gtx.Ops)
	trackClip.Pop()

	offset := op.Offset(image.Pt(max(0, gtx.Constraints.Max.X-trackWidth), columnHeaderPx)).Push(gtx.Ops)
	sGTX := gtx
	sGTX.Constraints = layout.Constraints{
		Min: image.Pt(trackWidth, trackLen),
		Max: image.Pt(trackWidth, trackLen),
	}
	dims := style.Layout(sGTX, layout.Vertical, start, end)
	offset.Pop()

	if delta := v.VerticalScrollbar.ScrollDistance(); delta != 0 && maxY > 0 {
		travelFrac := 1 - (end - start)
		if travelFrac > 0 {
			progress := (start + delta) / travelFrac
			if progress < 0 {
				progress = 0
			} else if progress > 1 {
				progress = 1
			}
			v.Y = float64(progress) * maxY
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	}
	return dims
}

func (a *App) updateGridSelection(v *gridViewport, position CellPosition, extend, persist bool) {
	state := a.documentSessionState
	if state == nil || v == nil {
		return
	}
	state.gridNavigationSequence++
	state.gridEndMode = false
	setSelectedGridPicture(state, "")
	state.gridPictureGesture.cancel()
	state.selection = updatedSelection(state.selection, state.selectionSet, position, extend)
	state.selectionSet = true
	state.gridInputComposing = false
	position = state.selection.Active
	row, column := position.Row+1, position.Column+1
	v.SelectedRow, v.SelectedColumn = row, column
	a.selectedCell = xlsx.CellName(column, row)
	cell := document.Cell{}
	if v.WindowSheet == a.sheet && v.cellLoaded(row, column) {
		cell = v.cell(row, column)
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
