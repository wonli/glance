package desktopui

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"gioui.org/layout"
	"glance/internal/document"
)

const maxSpreadsheetImagePreviewBytes = 128 << 20

type gridPictureMetadataResult struct {
	state      *documentSessionState
	generation uint64
	sheet      string
	images     []document.SpreadsheetImage
	err        error
}

type gridPicturePreviewResult struct {
	state      *documentSessionState
	generation uint64
	sheet      string
	id         string
	version    uint64
	preview    image.Image
	err        error
}

type gridPictureCommandResult struct {
	state      *documentSessionState
	token      sessionOperationToken
	revision   uint64
	generation uint64
	sheet      string
	operation  string
	images     []document.SpreadsheetImage
	selectedID string
	mutated    bool
	err        error
}

type gridPictureCommandKind uint8

const (
	gridPictureCommandMove gridPictureCommandKind = iota + 1
	gridPictureCommandResize
	gridPictureCommandInsert
	gridPictureCommandReplace
	gridPictureCommandDelete
)

type gridPictureCommandRequest struct {
	kind           gridPictureCommandKind
	operation      string
	imageID        string
	placement      document.SpreadsheetImagePlacement
	widthPx        int
	target         *gridPicturePlacementTarget
	preview        *gridPictureGeometry
	data           []byte
	mediaType      string
	findFreeAnchor bool
}

func (a *App) requestGridPictureMetadata(state *documentSessionState, reader document.SpreadsheetImageReader, sheet string) {
	if state == nil || reader == nil || sheet == "" || state.closed.Load() || a.gridPictureMetaResults == nil {
		return
	}
	if state.gridPicturesSheet == sheet || (state.gridPicturesLoading && state.gridPicturesRequestedSheet == sheet) {
		return
	}
	if state.gridPicturesSheet != sheet {
		state.gridPictures = nil
		state.gridPicturesSheet = ""
		setSelectedGridPicture(state, "")
		state.gridPictureGesture.cancel()
		invalidateGridPicturePreviews(state)
	}
	state.gridPicturesRequestedSheet = sheet
	state.gridPicturesLoading = true
	state.gridPicturesError = ""
	generation := a.documentGeneration
	results := a.gridPictureMetaResults
	shutdown, window := a.shutdown, a.window
	go func() {
		unlock := lockSessionAdapterRead(state)
		images, err := reader.SpreadsheetImages(sheet)
		unlock()
		result := gridPictureMetadataResult{state: state, generation: generation, sheet: sheet, images: images, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func invalidateGridPicturePreviews(state *documentSessionState) {
	if state == nil {
		return
	}
	if state.gridPictureCache != nil {
		state.gridPictureCache.Clear()
	}
	if state.gridPictureVersions == nil {
		state.gridPictureVersions = make(map[string]uint64)
	}
	for id := range state.gridPicturePending {
		state.gridPictureVersions[id]++
	}
	state.gridPicturePending = make(map[string]uint64)
	state.gridPictureErrors = make(map[string]string)
}

func (a *App) invalidateGridPictures(state *documentSessionState) {
	if state == nil {
		return
	}
	state.gridPictures = nil
	state.gridPicturesSheet = ""
	state.gridPicturesRequestedSheet = ""
	state.gridPicturesLoading = false
	state.gridPicturesError = ""
	state.gridPictureGesture.cancel()
	invalidateGridPicturePreviews(state)
}

func (a *App) pollGridPictureResults() {
	for {
		select {
		case result := <-a.gridPictureMetaResults:
			a.applyGridPictureMetadata(result)
		default:
			goto previews
		}
	}

previews:
	for {
		select {
		case result := <-a.gridPictureResults:
			a.applyGridPicturePreview(result)
		default:
			goto commands
		}
	}

commands:
	for {
		select {
		case result := <-a.gridPictureCommands:
			a.applyGridPictureCommand(result)
		default:
			return
		}
	}
}

func (a *App) applyGridPictureMetadata(result gridPictureMetadataResult) {
	state := result.state
	if state == nil || state.closed.Load() {
		return
	}
	requested := state.gridPicturesRequestedSheet == result.sheet
	if result.generation != a.documentGeneration || state != a.documentSessionState || result.sheet != a.sheet || !requested {
		if requested {
			state.gridPicturesLoading = false
			state.gridPicturesRequestedSheet = ""
		}
		return
	}
	state.gridPicturesLoading = false
	state.gridPicturesRequestedSheet = ""
	state.gridPictureGesture.cancel()
	state.gridPictureCache.Clear()
	state.gridPicturePending = make(map[string]uint64)
	state.gridPictureErrors = make(map[string]string)
	if result.err != nil {
		state.gridPictures = nil
		state.gridPicturesSheet = result.sheet
		state.gridPicturesError = result.err.Error()
		if a.window != nil {
			a.setStatus("读取工作表图片失败：" + result.err.Error())
		}
	} else {
		state.gridPictures = append([]document.SpreadsheetImage(nil), result.images...)
		state.gridPicturesSheet = result.sheet
		state.gridPicturesError = ""
		if state.selectedGridPictureID != "" && !containsGridPicture(state.gridPictures, state.selectedGridPictureID) {
			setSelectedGridPicture(state, "")
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) requestGridPicturePreview(state *documentSessionState, picture document.SpreadsheetImage) {
	if state == nil || state.closed.Load() || picture.ID == "" || state.gridPicturesSheet == "" {
		return
	}
	if _, ok := state.gridPictureCache.Get(picture.ID); ok || state.gridPictureErrors[picture.ID] != "" {
		return
	}
	if _, pending := state.gridPicturePending[picture.ID]; pending {
		return
	}
	reader, ok := any(state.book).(document.SpreadsheetImageReader)
	if !ok {
		state.gridPictureErrors[picture.ID] = "此工作簿不支持读取图片"
		return
	}
	switch picture.MediaType {
	case "image/png", "image/jpeg", "image/gif":
	default:
		state.gridPictureErrors[picture.ID] = picture.ReadOnlyReason
		if state.gridPictureErrors[picture.ID] == "" {
			state.gridPictureErrors[picture.ID] = "此图片格式暂不支持预览"
		}
		return
	}
	if state.gridPicturePending == nil {
		state.gridPicturePending = make(map[string]uint64)
	}
	if state.gridPictureVersions == nil {
		state.gridPictureVersions = make(map[string]uint64)
	}
	state.gridPictureVersions[picture.ID]++
	version := state.gridPictureVersions[picture.ID]
	state.gridPicturePending[picture.ID] = version
	results := a.gridPictureResults
	if results == nil {
		delete(state.gridPicturePending, picture.ID)
		return
	}
	generation, sheet, window := a.documentGeneration, state.gridPicturesSheet, a.window
	shutdown := a.shutdown
	go func() {
		unlock := lockSessionAdapterRead(state)
		data, err := reader.SpreadsheetImageBytes(picture.ID)
		unlock()
		var preview image.Image
		if err == nil {
			if len(data) > maxSpreadsheetImagePreviewBytes {
				err = fmt.Errorf("图片文件超过 %d MiB", maxSpreadsheetImagePreviewBytes>>20)
			} else {
				preview, err = decodeSpreadsheetPicturePreview(data)
			}
		}
		result := gridPicturePreviewResult{state: state, generation: generation, sheet: sheet, id: picture.ID, version: version, preview: preview, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func decodeSpreadsheetPicturePreview(data []byte) (image.Image, error) {
	if len(data) == 0 || len(data) > maxSpreadsheetImagePreviewBytes {
		return nil, fmt.Errorf("图片文件大小无效")
	}
	return decodeWordPreview(data)
}

func (a *App) applyGridPicturePreview(result gridPicturePreviewResult) {
	state := result.state
	if state == nil || state.closed.Load() {
		return
	}
	if version, pending := state.gridPicturePending[result.id]; pending && version == result.version {
		delete(state.gridPicturePending, result.id)
	}
	if result.generation != a.documentGeneration || state != a.documentSessionState || result.sheet != a.sheet || state.gridPicturesSheet != result.sheet || state.gridPictureVersions[result.id] != result.version || !containsGridPicture(state.gridPictures, result.id) {
		return
	}
	if result.err != nil {
		state.gridPictureErrors[result.id] = result.err.Error()
		if a.window != nil {
			a.setStatus("图片预览失败：" + result.err.Error())
		}
	} else if !state.gridPictureCache.Put(result.id, result.preview) {
		state.gridPictureErrors[result.id] = "图片无法放入工作簿预览缓存"
	} else {
		delete(state.gridPictureErrors, result.id)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func containsGridPicture(pictures []document.SpreadsheetImage, id string) bool {
	for _, picture := range pictures {
		if picture.ID == id {
			return true
		}
	}
	return false
}

func activeGridPictures(state *documentSessionState, sheet string) []document.SpreadsheetImage {
	if state == nil || sheet == "" || state.gridPicturesSheet != sheet {
		return nil
	}
	return state.gridPictures
}

func setSelectedGridPicture(state *documentSessionState, imageID string) {
	if state == nil {
		return
	}
	state.selectedGridPictureID = imageID
	if imageID == "" {
		if state.toolbarTab == toolbarTabPicture {
			state.toolbarTab = toolbarTabHome
		}
		return
	}
	if state.toolbarTab != toolbarTabPicture {
		state.toolbarActionList.List.Position = layout.Position{}
	}
	state.toolbarTab = toolbarTabPicture
}

func gridPictureByID(pictures []document.SpreadsheetImage, id string) (document.SpreadsheetImage, bool) {
	for _, picture := range pictures {
		if picture.ID == id {
			return picture, true
		}
	}
	return document.SpreadsheetImage{}, false
}
