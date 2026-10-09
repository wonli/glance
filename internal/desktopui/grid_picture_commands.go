package desktopui

import (
	"fmt"

	"glance/internal/document"
)

func (a *App) startGridPictureMoveResize(state *documentSessionState, picture document.SpreadsheetImage, mode gridPictureGestureMode, target gridPicturePlacementTarget, preview gridPictureGeometry) {
	kind, operation := gridPictureCommandMove, "移动图片"
	if mode == gridPictureGestureResize {
		kind, operation = gridPictureCommandResize, "调整图片尺寸"
	}
	a.startGridPictureCommand(state, gridPictureCommandRequest{kind: kind, operation: operation, imageID: picture.ID, widthPx: target.WidthPx, target: &target, preview: &preview})
}

func (a *App) startGridPictureCommand(state *documentSessionState, request gridPictureCommandRequest) {
	if state == nil || state.formatRunning.Load() || a.isSaving() {
		if a.window != nil {
			a.setStatus("正在处理其他操作，请稍后重试")
			a.window.Invalidate()
		}
		return
	}
	editor, ok := any(state.book).(document.SpreadsheetImageEditor)
	if !ok {
		if a.window != nil {
			a.setStatus("当前工作簿不支持编辑图片")
			a.window.Invalidate()
		}
		return
	}
	if request.operation == "" {
		request.operation = pictureCommandOperationName(request.kind)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		if a.window != nil {
			a.setStatus("正在处理其他操作，请稍后重试")
			a.window.Invalidate()
		}
		return
	}
	state.gridPictureCommitID = request.imageID
	state.gridPictureCommitPreview = gridPictureGeometry{}
	state.gridPictureCommitActive = false
	if request.preview != nil {
		state.gridPictureCommitPreview = *request.preview
		state.gridPictureCommitActive = true
	}
	generation, sheet := a.documentGeneration, state.sheet
	results, shutdown, window := a.gridPictureCommands, a.shutdown, a.window
	go func() {
		result := gridPictureCommandResult{state: state, token: token, generation: generation, sheet: sheet, operation: request.operation}
		unlock := lockSessionAdapterWrite(state)
		switch request.kind {
		case gridPictureCommandMove:
			if request.target == nil {
				result.err = fmt.Errorf("missing picture placement")
			} else {
				target := request.target
				result.err = editor.MoveSpreadsheetImage(request.imageID, target.Row, target.Column, target.OffsetX, target.OffsetY)
			}
		case gridPictureCommandResize:
			result.err = editor.ResizeSpreadsheetImage(request.imageID, request.widthPx)
		case gridPictureCommandInsert:
			placement := request.placement
			if request.findFreeAnchor {
				reader, supported := any(state.book).(document.SpreadsheetImageReader)
				if !supported {
					result.err = fmt.Errorf("workbook cannot find an open picture anchor")
				} else {
					var existing []document.SpreadsheetImage
					existing, result.err = reader.SpreadsheetImages(sheet)
					if result.err == nil {
						placement.Row, placement.Column = nearbyFreeGridPictureAnchor(existing, placement.Row, placement.Column)
					}
				}
			}
			if result.err != nil {
				break
			}
			var imageID string
			imageID, result.err = editor.InsertSpreadsheetImage(placement, request.data)
			result.selectedID = imageID
			request.placement = placement
		case gridPictureCommandReplace:
			result.err = editor.ReplaceSpreadsheetImage(request.imageID, request.mediaType, request.data)
			result.selectedID = request.imageID
		case gridPictureCommandDelete:
			result.err = editor.DeleteSpreadsheetImage(request.imageID)
		default:
			result.err = fmt.Errorf("unknown picture operation")
		}
		if result.err == nil {
			result.mutated = true
			reader, supported := any(state.book).(document.SpreadsheetImageReader)
			if !supported {
				result.err = fmt.Errorf("workbook cannot refresh picture metadata")
			} else {
				result.images, result.err = reader.SpreadsheetImages(sheet)
			}
			if result.err == nil {
				switch request.kind {
				case gridPictureCommandInsert:
					if !containsGridPicture(result.images, result.selectedID) {
						result.selectedID = pictureIDAtCell(result.images, request.placement.Row, request.placement.Column)
					}
				case gridPictureCommandMove:
					if request.target != nil {
						result.selectedID = pictureIDAtCell(result.images, request.target.Row, request.target.Column)
					}
				case gridPictureCommandResize, gridPictureCommandReplace:
					if !containsGridPicture(result.images, result.selectedID) {
						result.selectedID = request.imageID
					}
				case gridPictureCommandDelete:
					result.selectedID = ""
				}
			}
		}
		unlock()
		if result.mutated {
			a.ensureDesktopRuntime().publishSessionRevision(token.documentID, token.revision+1)
		}
		state.finishOperation(token)
		result.revision = state.wordRevisionAtomic.Load()
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func pictureCommandOperationName(kind gridPictureCommandKind) string {
	switch kind {
	case gridPictureCommandMove:
		return "移动图片"
	case gridPictureCommandResize:
		return "调整图片尺寸"
	case gridPictureCommandInsert:
		return "插入图片"
	case gridPictureCommandReplace:
		return "替换图片"
	case gridPictureCommandDelete:
		return "删除图片"
	default:
		return "处理图片"
	}
}

func pictureIDAtCell(pictures []document.SpreadsheetImage, row, column int) string {
	for _, picture := range pictures {
		if picture.Row == row && picture.Column == column {
			return picture.ID
		}
	}
	return ""
}

func (a *App) applyGridPictureCommand(result gridPictureCommandResult) {
	state := result.state
	if state == nil || state.closed.Load() {
		return
	}
	state.gridPictureCommitID = ""
	state.gridPictureCommitPreview = gridPictureGeometry{}
	state.gridPictureCommitActive = false
	state.gridPictureGesture.cancel()
	if result.token.generation != 0 && (!state.acceptsResult(result.token) || state.wordRevisionAtomic.Load() != result.revision) {
		if result.mutated {
			a.invalidateGridPictures(state)
		}
		return
	}
	if result.generation != a.documentGeneration || state != a.documentSessionState || result.sheet != a.sheet {
		if result.mutated {
			a.invalidateGridPictures(state)
		}
		return
	}
	if result.err != nil {
		if result.mutated {
			a.invalidateGridPictures(state)
		}
		a.setStatus(result.operation + "失败：" + result.err.Error())
	} else {
		state.gridPictures = append([]document.SpreadsheetImage(nil), result.images...)
		state.gridPicturesSheet = result.sheet
		state.gridPicturesRequestedSheet = ""
		state.gridPicturesLoading = false
		state.gridPicturesError = ""
		setSelectedGridPicture(state, result.selectedID)
		invalidateGridPicturePreviews(state)
		a.setStatus("已" + result.operation)
		a.persistActiveSession()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
