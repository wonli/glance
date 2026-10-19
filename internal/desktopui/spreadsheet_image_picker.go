package desktopui

import (
	"bytes"
	"fmt"
	"glance/internal/i18n"
	"image"
	"path/filepath"

	"glance/internal/document"
)

type spreadsheetImagePickerOperation uint8

const (
	spreadsheetImageInsert spreadsheetImagePickerOperation = iota + 1
	spreadsheetImageReplace
)

type spreadsheetImageFile struct {
	data      []byte
	mediaType string
}

type spreadsheetImagePickerResult struct {
	state      *documentSessionState
	generation uint64
	sheet      string
	operation  spreadsheetImagePickerOperation
	row        int
	column     int
	imageID    string
	path       string
	data       []byte
	mediaType  string
	err        error
}

func loadSpreadsheetImageFile(path string) (spreadsheetImageFile, error) {
	file, err := loadWordImageFile(path)
	if err != nil {
		return spreadsheetImageFile{}, err
	}
	return spreadsheetImageFile{data: file.data, mediaType: file.mediaType}, nil
}

func (a *App) startSpreadsheetImagePicker(operation spreadsheetImagePickerOperation) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() || a.xlsxImagePickerRunning {
		return
	}
	if _, ok := state.book.(document.SpreadsheetImageEditor); !ok {
		return
	}
	result := spreadsheetImagePickerResult{state: state, generation: a.documentGeneration, sheet: state.sheet, operation: operation}
	switch operation {
	case spreadsheetImageInsert:
		position := state.selection.Active
		if !state.selectionSet {
			position = CellPosition{}
		}
		result.row, result.column = position.Row+1, position.Column+1
	case spreadsheetImageReplace:
		picture, ok := gridPictureByID(activeGridPictures(state, state.sheet), state.selectedGridPictureID)
		if !ok || !picture.Editable {
			return
		}
		result.imageID = picture.ID
	default:
		return
	}
	if a.xlsxImagePickerResults == nil {
		a.xlsxImagePickerResults = make(chan spreadsheetImagePickerResult, 1)
	}
	a.xlsxImagePickerRunning = true
	if operation == spreadsheetImageInsert {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.choose_a_png_or_jpeg_2076ca"}, feedbackInfo, notificationToast)
	} else {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.choose_a_png_or_jpeg_5f614f"}, feedbackInfo, notificationToast)
	}
	choose := a.xlsxImageFilePicker
	if choose == nil {
		pickerLabels := a.filePickerLabels()
		choose = func() (string, error) { return chooseOfficeImageFile(pickerLabels) }
	}
	results, shutdown, window := a.xlsxImagePickerResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		result.path, result.err = choose()
		if result.err == nil && result.path != "" {
			var file spreadsheetImageFile
			file, result.err = loadSpreadsheetImageFile(result.path)
			result.data, result.mediaType = file.data, file.mediaType
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

func (a *App) handleSpreadsheetImagePickerResult(result spreadsheetImagePickerResult) {
	a.xlsxImagePickerRunning = false
	state := result.state
	if state == nil || state.closed.Load() || result.generation != a.documentGeneration || state != a.documentSessionState || result.sheet == "" || state.sheet != result.sheet {
		return
	}
	if result.err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, result.err, notificationToast)
		return
	}
	if result.path == "" {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.image_selection_canceled_5978be"}, feedbackSuccess, notificationSuppressed)
		return
	}
	if state.formatRunning.Load() || a.isSaving() {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_workbook_is_busy_try_cb4e59"}, feedbackWarning, notificationToast)
		return
	}
	switch result.operation {
	case spreadsheetImageInsert:
		width, height, err := spreadsheetPictureInitialSize(result.data)
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
			return
		}
		name := filepath.Base(result.path)
		placement := document.SpreadsheetImagePlacement{Sheet: result.sheet, Row: result.row, Column: result.column, WidthPx: width, HeightPx: height, MediaType: result.mediaType, Name: name, AltText: name}
		a.startGridPictureCommand(state, gridPictureCommandRequest{kind: gridPictureCommandInsert, operation: "插入图片", placement: placement, data: result.data, findFreeAnchor: true})
	case spreadsheetImageReplace:
		picture, ok := gridPictureByID(activeGridPictures(state, result.sheet), result.imageID)
		if !ok || !picture.Editable || result.imageID != state.selectedGridPictureID {
			return
		}
		a.startGridPictureCommand(state, gridPictureCommandRequest{kind: gridPictureCommandReplace, operation: "替换图片", imageID: result.imageID, mediaType: result.mediaType, data: result.data})
	default:
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.invalid_image_operation_def800"}, feedbackError, notificationToast)
	}
}

func spreadsheetPictureInitialSize(data []byte) (int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 0, 0, fmt.Errorf("图片尺寸无效")
	}
	const maxWidth, maxHeight = 240, 180
	scale := min(float64(maxWidth)/float64(config.Width), float64(maxHeight)/float64(config.Height))
	if scale > 1 {
		scale = 1
	}
	width, height := int(float64(config.Width)*scale+0.5), int(float64(config.Height)*scale+0.5)
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return width, height, nil
}

func nearbyFreeGridPictureAnchor(pictures []document.SpreadsheetImage, row, column int) (int, int) {
	const maxRow, maxColumn = 1048576, 16384
	if row < 1 {
		row = 1
	}
	if column < 1 {
		column = 1
	}
	occupied := make(map[[2]int]struct{}, len(pictures))
	for _, picture := range pictures {
		occupied[[2]int{picture.Row, picture.Column}] = struct{}{}
	}
	isOccupied := func(candidateRow, candidateColumn int) bool {
		_, ok := occupied[[2]int{candidateRow, candidateColumn}]
		return ok
	}
	for radius := 0; radius < 10000; radius++ {
		candidates := [][2]int{
			{row, column + radius},
			{row + radius, column},
			{row + radius, column + radius},
			{row, column - radius},
			{row - radius, column},
			{row - radius, column - radius},
			{row + radius, column - radius},
			{row - radius, column + radius},
		}
		for _, candidate := range candidates {
			r, c := candidate[0], candidate[1]
			if r < 1 || r > maxRow || c < 1 || c > maxColumn || isOccupied(r, c) {
				continue
			}
			return r, c
		}
	}
	const maxCells = int64(maxRow) * int64(maxColumn)
	start := int64(row-1)*int64(maxColumn) + int64(column-1)
	for offset := int64(0); offset <= int64(len(pictures)); offset++ {
		index := (start + offset) % maxCells
		candidateRow, candidateColumn := int(index/int64(maxColumn))+1, int(index%int64(maxColumn))+1
		if !isOccupied(candidateRow, candidateColumn) {
			return candidateRow, candidateColumn
		}
	}
	return 0, 0
}
