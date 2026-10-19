package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/document"
)

type spreadsheetImageMoveArguments struct {
	Sheet   string `json:"sheet"`
	ImageID string `json:"image_id"`
	Row     int    `json:"row"`
	Column  int    `json:"column"`
	OffsetX int    `json:"offset_x"`
	OffsetY int    `json:"offset_y"`
}

type spreadsheetImageResizeArguments struct {
	Sheet   string `json:"sheet"`
	ImageID string `json:"image_id"`
	WidthPx int    `json:"width_px"`
}

type spreadsheetImageInsertArguments struct {
	Sheet       string `json:"sheet"`
	Row         int    `json:"row"`
	Column      int    `json:"column"`
	OffsetX     int    `json:"offset_x"`
	OffsetY     int    `json:"offset_y"`
	WidthPx     int    `json:"width_px"`
	HeightPx    int    `json:"height_px"`
	MediaType   string `json:"media_type"`
	Name        string `json:"name"`
	AltText     string `json:"alt_text"`
	ImageBase64 []byte `json:"image_base64"`
}

type spreadsheetImageReplaceArguments struct {
	Sheet       string `json:"sheet"`
	ImageID     string `json:"image_id"`
	MediaType   string `json:"media_type"`
	ImageBase64 []byte `json:"image_base64"`
}

type spreadsheetImageDeleteArguments struct {
	Sheet   string `json:"sheet"`
	ImageID string `json:"image_id"`
}

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
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.another_operation_is_in_progress_try_again_17714f"}, feedbackInfo, notificationSuppressed)
			a.window.Invalidate()
		}
		return
	}
	_, ok := any(state.book).(document.SpreadsheetImageEditor)
	if !ok {
		if a.window != nil {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.this_workbook_does_not_support_picture_edi_10f34c"}, feedbackWarning, notificationToast)
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
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.another_operation_is_in_progress_try_again_17714f"}, feedbackInfo, notificationSuppressed)
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
	a.launchManagedBackground(func() {
		result := gridPictureCommandResult{state: state, token: token, generation: generation, sheet: sheet, operation: request.operation}
		commandID := ""
		var arguments any
		targetType, targetID := "spreadsheet_image", request.imageID
		switch request.kind {
		case gridPictureCommandMove:
			if request.target == nil {
				result.err = fmt.Errorf("missing picture placement")
			} else {
				target := request.target
				commandID = "spreadsheet.image.move"
				arguments = spreadsheetImageMoveArguments{Sheet: sheet, ImageID: request.imageID, Row: target.Row, Column: target.Column, OffsetX: target.OffsetX, OffsetY: target.OffsetY}
			}
		case gridPictureCommandResize:
			commandID = "spreadsheet.image.resize"
			arguments = spreadsheetImageResizeArguments{Sheet: sheet, ImageID: request.imageID, WidthPx: request.widthPx}
		case gridPictureCommandInsert:
			placement := request.placement
			if request.findFreeAnchor {
				reader, supported := any(state.book).(document.SpreadsheetImageReader)
				if !supported {
					result.err = fmt.Errorf("workbook cannot find an open picture anchor")
				} else {
					var existing []document.SpreadsheetImage
					unlock := lockSessionAdapterRead(state)
					existing, result.err = reader.SpreadsheetImages(sheet)
					unlock()
					if result.err == nil {
						placement.Row, placement.Column = nearbyFreeGridPictureAnchor(existing, placement.Row, placement.Column)
					}
				}
			}
			if result.err != nil {
				break
			}
			commandID = "spreadsheet.image.insert"
			arguments = spreadsheetImageInsertArguments{
				Sheet: placement.Sheet, Row: placement.Row, Column: placement.Column,
				OffsetX: placement.OffsetX, OffsetY: placement.OffsetY,
				WidthPx: placement.WidthPx, HeightPx: placement.HeightPx,
				MediaType: placement.MediaType, Name: placement.Name, AltText: placement.AltText,
				ImageBase64: request.data,
			}
			targetType, targetID = "spreadsheet_cell", fmt.Sprintf("%s!%s%d", placement.Sheet, columnName(placement.Column), placement.Row)
			request.placement = placement
		case gridPictureCommandReplace:
			commandID = "spreadsheet.image.replace"
			arguments = spreadsheetImageReplaceArguments{Sheet: sheet, ImageID: request.imageID, MediaType: request.mediaType, ImageBase64: request.data}
			result.selectedID = request.imageID
		case gridPictureCommandDelete:
			commandID = "spreadsheet.image.delete"
			arguments = spreadsheetImageDeleteArguments{Sheet: sheet, ImageID: request.imageID}
		default:
			result.err = fmt.Errorf("unknown picture operation")
		}
		if result.err == nil && commandID != "" {
			commandResult, err := a.executeRibbonApplicationCommand(token, commandID, targetType, targetID, sheet, arguments)
			result.err = err
			if err == nil {
				result.revision = commandResult.Revision
				var outcome struct {
					Changed bool   `json:"changed"`
					ImageID string `json:"image_id"`
				}
				if err := json.Unmarshal(commandResult.Data, &outcome); err != nil {
					result.err = fmt.Errorf("decode spreadsheet image command result: %w", err)
				} else {
					result.mutated = outcome.Changed
					if request.kind == gridPictureCommandInsert {
						result.selectedID = outcome.ImageID
					}
				}
			}
		}
		if result.err == nil {
			reader, supported := any(state.book).(document.SpreadsheetImageReader)
			if !supported {
				result.err = fmt.Errorf("workbook cannot refresh picture metadata")
			} else {
				unlock := lockSessionAdapterRead(state)
				result.images, result.err = reader.SpreadsheetImages(sheet)
				unlock()
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
		state.finishOperation(token)
		if result.revision == 0 {
			result.revision = state.wordRevisionAtomic.Load()
		}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
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
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, result.err, notificationToast)
	} else {
		state.gridPictures = append([]document.SpreadsheetImage(nil), result.images...)
		state.gridPicturesSheet = result.sheet
		state.gridPicturesRequestedSheet = ""
		state.gridPicturesLoading = false
		state.gridPicturesError = ""
		setSelectedGridPicture(state, result.selectedID)
		invalidateGridPicturePreviews(state)
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.picture_operation_completed", Args: i18n.Args{"Operation": a.localizedSpreadsheetToolbarText(result.operation)}}, feedbackSuccess, notificationToast)
		a.persistActiveSession()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
