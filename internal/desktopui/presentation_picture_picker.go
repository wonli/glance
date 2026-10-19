package desktopui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"glance/internal/i18n"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/pptx"
)

const (
	maxPresentationPickerPictureBytes  = 32 << 20
	maxPresentationInsertPictureBytes  = 32 << 20
	maxPresentationPickerPictureEdge   = 32768
	maxPresentationPickerPicturePixels = 64 << 20
)

type presentationPictureFile struct {
	data      []byte
	mediaType string
	width     int
	height    int
}

type presentationPicturePickerResult struct {
	state      *documentSessionState
	doc        *pptx.Document
	generation uint64
	revision   uint64
	slideIndex int
	slideID    uint32
	shapeID    int64
	insert     bool
	path       string
	data       []byte
	mediaType  string
	width      int
	height     int
	err        error
}

type presentationPictureCommandResult struct {
	state      *documentSessionState
	doc        *pptx.Document
	token      sessionOperationToken
	operation  string
	revision   uint64
	generation uint64
	slideIndex int
	shapeID    int64
	crop       document.PresentationCrop
	mutated    bool
	snapshot   document.PresentationSnapshot
	err        error
}

func loadPresentationPictureFile(path string) (presentationPictureFile, error) {
	var mediaType, wantFormat string
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mediaType, wantFormat = "image/png", "png"
	case ".jpg", ".jpeg":
		mediaType, wantFormat = "image/jpeg", "jpeg"
	default:
		return presentationPictureFile{}, fmt.Errorf("仅支持 PNG/JPEG 图片")
	}
	info, err := os.Stat(path)
	if err != nil {
		return presentationPictureFile{}, fmt.Errorf("读取图片文件: %w", err)
	}
	if info.Size() > maxPresentationPickerPictureBytes {
		return presentationPictureFile{}, fmt.Errorf("图片文件超过 %d MiB", maxPresentationPickerPictureBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return presentationPictureFile{}, fmt.Errorf("读取图片文件: %w", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return presentationPictureFile{}, fmt.Errorf("图片文件损坏: %w", err)
	}
	if format != wantFormat {
		return presentationPictureFile{}, fmt.Errorf("图片扩展名与实际 %s 格式不匹配", format)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxPresentationPickerPictureEdge || config.Height > maxPresentationPickerPictureEdge || uint64(config.Width)*uint64(config.Height) > maxPresentationPickerPicturePixels {
		return presentationPictureFile{}, fmt.Errorf("图片尺寸超过安全上限")
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return presentationPictureFile{}, fmt.Errorf("图片文件损坏: %w", err)
	}
	if decodedFormat != wantFormat || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return presentationPictureFile{}, fmt.Errorf("图片解码信息不一致")
	}
	return presentationPictureFile{data: data, mediaType: mediaType, width: config.Width, height: config.Height}, nil
}

func (a *App) startPresentationPictureInsertPicker() {
	state, doc := a.documentSessionState, a.pptxDoc
	if state == nil || doc == nil || state.pptxDoc != doc || state.formatRunning.Load() || a.isSaving() || a.pptxPicPickerRunning {
		return
	}
	v := a.pptxView
	if v == nil || v.commandPending {
		return
	}
	if err := a.flushPresentationNotesDraft(v); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	v.insertTextBox = false
	v.insertShape = false
	v.insertPicturePending = false
	v.insertPictureData = nil
	v.insertPictureMediaType = ""
	v.insertPictureWidth, v.insertPictureHeight = 0, 0
	result := presentationPicturePickerResult{state: state, doc: doc, generation: a.documentGeneration, insert: true}
	if a.pptxPicPickerResults == nil {
		a.pptxPicPickerResults = make(chan presentationPicturePickerResult, 1)
	}
	a.pptxPicPickerRunning = true
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.choose_a_png_or_jpeg_2076ca"}, feedbackInfo, notificationSuppressed)
	choose := a.pptxPicFilePicker
	if choose == nil {
		pickerLabels := a.filePickerLabels()
		choose = func() (string, error) { return chooseOfficeImageFile(pickerLabels) }
	}
	results, shutdown, window := a.pptxPicPickerResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		result.path, result.err = choose()
		if result.err == nil && result.path != "" {
			var file presentationPictureFile
			file, result.err = loadPresentationPictureFile(result.path)
			if result.err == nil && len(file.data) > maxPresentationInsertPictureBytes {
				result.err = fmt.Errorf("插入图片不能超过 %d MiB", maxPresentationInsertPictureBytes>>20)
			}
			result.data, result.mediaType, result.width, result.height = file.data, file.mediaType, file.width, file.height
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

func (a *App) startPresentationPicturePicker() {
	state, doc := a.documentSessionState, a.pptxDoc
	v := a.pptxView
	if state == nil || doc == nil || state.pptxDoc != doc || state.formatRunning.Load() || a.isSaving() || a.pptxPicPickerRunning {
		return
	}
	if v == nil {
		return
	}
	slideIndex, picture, selected := presentationSelectedEditablePicture(v)
	if !selected {
		return
	}
	if err := a.flushPresentationNotesDraft(v); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	if slideIndex < 0 || slideIndex >= len(v.snapshot.Slides) {
		return
	}
	result := presentationPicturePickerResult{state: state, doc: doc, generation: a.documentGeneration, revision: state.wordRevisionAtomic.Load(), slideIndex: slideIndex, slideID: v.snapshot.Slides[slideIndex].ID, shapeID: picture.ID}
	if a.pptxPicPickerResults == nil {
		a.pptxPicPickerResults = make(chan presentationPicturePickerResult, 1)
	}
	a.pptxPicPickerRunning = true
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.choose_a_png_or_jpeg_5f614f"}, feedbackInfo, notificationSuppressed)
	choose := a.pptxPicFilePicker
	if choose == nil {
		pickerLabels := a.filePickerLabels()
		choose = func() (string, error) { return chooseOfficeImageFile(pickerLabels) }
	}
	results, shutdown, window := a.pptxPicPickerResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		result.path, result.err = choose()
		if result.err == nil && result.path != "" {
			var file presentationPictureFile
			file, result.err = loadPresentationPictureFile(result.path)
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

func (a *App) handlePresentationPicturePickerResult(result presentationPicturePickerResult) {
	a.pptxPicPickerRunning = false
	if result.state == nil || result.state.closed.Load() || result.generation != a.documentGeneration || result.state != a.documentSessionState || result.doc == nil || result.doc != a.pptxDoc || result.doc != result.state.pptxDoc {
		return
	}
	v := a.pptxView
	if v == nil {
		return
	}
	if result.insert {
		if result.err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			return
		}
		if result.path == "" {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.image_selection_canceled_5978be"}, feedbackWarning, notificationSuppressed)
			return
		}
		if len(result.data) == 0 || len(result.data) > maxPresentationInsertPictureBytes || result.width <= 0 || result.height <= 0 {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_image_data_is_invalid_or_exceeds_the_i_ed21ae"}, feedbackError, notificationToast)
			return
		}
		if result.state.formatRunning.Load() || a.isSaving() {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_is_busy_try_inserting_the_b6485a"}, feedbackWarning, notificationToast)
			return
		}
		v.insertTextBox = false
		v.insertShape = false
		v.insertPicturePending = true
		v.insertPictureData = result.data
		v.insertPictureMediaType = result.mediaType
		v.insertPictureWidth = result.width
		v.insertPictureHeight = result.height
		v.insertPictureGeneration = result.generation
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.click_a_position_on_the_slide_to_place_the_130e91"}, feedbackWarning, notificationSuppressed)
		return
	}
	slideIndex, picture, selected := presentationSelectedEditablePicture(v)
	if !selected || slideIndex != result.slideIndex || picture.ID != result.shapeID {
		return
	}
	if result.err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
		return
	}
	if result.path == "" {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.image_selection_canceled_5978be"}, feedbackWarning, notificationSuppressed)
		return
	}
	if result.state.formatRunning.Load() || a.isSaving() {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_is_busy_try_again_shortly_10cf71"}, feedbackWarning, notificationToast)
		return
	}
	if result.revision != 0 && result.state.wordRevisionAtomic.Load() != result.revision {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_picture_changed_while_the_picker_was_o_365f48"}, feedbackInfo, notificationToast)
		return
	}
	a.startPresentationPictureReplacement(result)
}

type presentationpictureSlideIdElementIdMediaTypeArguments struct {
	SlideId     uint32 `json:"slide_id"`
	ElementId   int64  `json:"element_id"`
	MediaType   string `json:"media_type"`
	ImageBase64 []byte `json:"image_base64"`
}

func (a *App) startPresentationPictureReplacement(request presentationPicturePickerResult) {
	state, doc := request.state, request.doc
	if state == nil || doc == nil || state.closed.Load() || state.pptxDoc != doc || state.formatRunning.Load() || a.isSaving() {
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_is_busy_try_again_shortly_10cf71"}, feedbackWarning, notificationToast)
		return
	}
	if request.revision != 0 && token.revision != request.revision {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_picture_changed_while_the_picker_was_o_365f48"}, feedbackInfo, notificationToast)
		return
	}
	if a.pptxPicCommandResults == nil {
		a.pptxPicCommandResults = make(chan presentationPictureCommandResult, 1)
	}
	result := presentationPictureCommandResult{
		state: state, doc: doc, token: token, operation: "替换图片", generation: request.generation,
		slideIndex: request.slideIndex, shapeID: request.shapeID,
	}
	results, shutdown, window := a.pptxPicCommandResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		beforeRevision := doc.Revision()
		commandResult, err := a.executeRibbonApplicationCommand(token, "presentation.element.picture.replace", "presentation_picture", fmt.Sprint(request.shapeID), "", presentationpictureSlideIdElementIdMediaTypeArguments{SlideId: request.slideID, ElementId: request.shapeID, MediaType: request.mediaType, ImageBase64: request.data})
		result.err = err
		if err == nil {
			var payload struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				MediaType string `json:"media_type"`
				Changed   bool   `json:"changed"`
			}
			if json.Unmarshal(commandResult.Data, &payload) != nil || payload.SlideID != request.slideID || payload.ElementID != request.shapeID || payload.MediaType != request.mediaType {
				result.err = application.ErrInvalidCommandResult
			} else {
				result.mutated = payload.Changed
			}
		}
		if result.err == nil {
			result.snapshot = doc.Snapshot()
			result.mutated = result.mutated && doc.Revision() != beforeRevision
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
	})
}

func (a *App) applyPresentationPictureCommandResult(result presentationPictureCommandResult) {
	state := result.state
	if state == nil || state.closed.Load() {
		return
	}
	if result.token.generation != 0 && (!state.acceptsResult(result.token) || state.wordRevisionAtomic.Load() != result.revision) {
		return
	}
	if result.err != nil {
		if result.operation == "裁剪图片" && a.pptxView != nil && a.pptxView.pictureCrop.active && a.pptxView.pictureCrop.shapeID == result.shapeID && a.pptxView.pictureCrop.slideIndex == result.slideIndex {
			a.pptxView.pictureCrop.committing = false
			a.markPresentationCropDraft(a.pptxView, true)
		}
		if result.generation == a.documentGeneration && state == a.documentSessionState && result.doc == a.pptxDoc {
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
		}
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.generation != a.documentGeneration || state != a.documentSessionState || result.doc == nil || result.doc != a.pptxDoc {
		return
	}
	if v := a.pptxView; v != nil {
		v.snapshot = result.snapshot
		if result.operation == "裁剪图片" && v.pictureCrop.active && v.pictureCrop.shapeID == result.shapeID && v.pictureCrop.slideIndex == result.slideIndex {
			v.pictureCrop = presentationPictureCropState{}
			a.markPresentationCropDraft(v, false)
		}
	}
	switch result.operation {
	case "裁剪图片":
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.picture_crop_completed_d5f25c"}, feedbackSuccess, notificationToast)
	default:
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.picture_replaced_510413"}, feedbackSuccess, notificationToast)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
