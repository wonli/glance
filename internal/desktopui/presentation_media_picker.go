package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"
	"path/filepath"
	"strings"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/pptx"
)

type presentationMediaPickerResult struct {
	state      *documentSessionState
	doc        *pptx.Document
	view       *presentationViewState
	generation uint64
	slideID    uint32
	bounds     document.PresentationRect
	kind       string
	path       string
	err        error
}

type presentationMediaCommandArguments struct {
	SlideID    uint32  `json:"slide_id"`
	SourcePath string  `json:"source_path"`
	MediaType  string  `json:"media_type"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Role       *string `json:"role,omitempty"`
	Decorative *bool   `json:"decorative,omitempty"`
}

func presentationMediaTypeForPath(path, kind string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		if kind == "audio" {
			return "audio/mpeg"
		}
	case ".wav":
		if kind == "audio" {
			return "audio/wav"
		}
	case ".mp4":
		if kind == "video" {
			return "video/mp4"
		}
	case ".webm":
		if kind == "video" {
			return "video/webm"
		}
	}
	return ""
}

func presentationMediaObjectBounds(slide *document.PresentationSlide, kind string) document.PresentationRect {
	if slide == nil {
		if kind == "audio" {
			return document.PresentationRect{X: 96, Y: 81, W: 48, H: 48}
		}
		return document.PresentationRect{X: 96, Y: 81, W: 480, H: 270}
	}
	width, height := slide.WidthPT, slide.HeightPT
	if width <= 0 {
		width = 960
	}
	if height <= 0 {
		height = 540
	}
	if kind == "audio" {
		size := min(48.0, width*0.08, height*0.08)
		return document.PresentationRect{X: width * 0.1, Y: height * 0.15, W: size, H: size}
	}
	w := min(480.0, width*0.5)
	h := min(w*9/16, height*0.5)
	return document.PresentationRect{X: width * 0.1, Y: height * 0.15, W: w, H: h}
}

func (a *App) startPresentationMediaInsertPicker(view *presentationViewState, kind string) bool {
	state, doc := a.documentSessionState, a.pptxDoc
	if view == nil || state == nil || doc == nil || state.pptxDoc != doc || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if kind != "audio" && kind != "video" {
		return false
	}
	if view.commandPending || a.pptxMediaPickerRunning {
		return true
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_slide_was_not_found_2b6179"}, feedbackWarning, notificationToast)
		return true
	}
	result := presentationMediaPickerResult{
		state: state, doc: doc, view: view, generation: a.documentGeneration,
		slideID: slide.ID, bounds: presentationMediaObjectBounds(slide, kind), kind: kind,
	}
	if a.pptxMediaPickerResults == nil {
		a.pptxMediaPickerResults = make(chan presentationMediaPickerResult, 1)
	}
	choose := a.pptxMediaFilePicker
	if choose == nil {
		pickerLabels := a.filePickerLabels()
		choose = func(kind string) (string, error) { return choosePresentationMediaFile(kind, pickerLabels) }
	}
	a.pptxMediaPickerRunning = true
	if kind == "audio" {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.choose_an_mp3_wav_audio_file_to_insert_4e7ba4"}, feedbackInfo, notificationToast)
	} else {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.choose_an_mp4_webm_video_file_to_insert_425be8"}, feedbackInfo, notificationToast)
	}
	results, shutdown, window := a.pptxMediaPickerResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		result.path, result.err = choose(kind)
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	return true
}

func (a *App) handlePresentationMediaPickerResult(result presentationMediaPickerResult) {
	a.pptxMediaPickerRunning = false
	if result.state == nil || result.state.closed.Load() || result.generation != a.documentGeneration || result.state != a.documentSessionState ||
		result.doc == nil || result.doc != a.pptxDoc || result.doc != result.state.pptxDoc || result.view == nil || result.view != a.pptxView {
		return
	}
	if result.err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
		return
	}
	if result.path == "" {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.media_file_selection_canceled_f49887"}, feedbackSuccess, notificationSuppressed)
		return
	}
	mediaType := presentationMediaTypeForPath(result.path, result.kind)
	if mediaType == "" {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.unsupported_media_format_audio_supports_mp_d37755"}, feedbackError, notificationToast)
		return
	}
	if result.view.commandPending || result.state.formatRunning.Load() || a.isSaving() {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_is_busy_try_inserting_the_28d4f5"}, feedbackWarning, notificationToast)
		return
	}
	if err := a.flushPresentationNotesDraft(result.view); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	token, ok := result.state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return
	}
	result.view.commandPending = true
	capabilityID := "presentation.element.audio.create"
	if result.kind == "video" {
		capabilityID = "presentation.element.video.create"
	}
	arguments := presentationMediaCommandArguments{
		SlideID: result.slideID, SourcePath: result.path, MediaType: mediaType,
		X: result.bounds.X, Y: result.bounds.Y, Width: result.bounds.W, Height: result.bounds.H,
	}
	if result.kind == "audio" {
		role := "effect"
		arguments.Role = &role
	} else {
		decorative := false
		arguments.Decorative = &decorative
	}
	state, view, generation := result.state, result.view, result.generation
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, capabilityID, "presentation_slide", fmt.Sprint(result.slideID), "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, err: err, createdAudio: result.kind == "audio", createdVideo: result.kind == "video"}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				MediaType string `json:"media_type"`
				Changed   bool   `json:"changed"`
			}
			if json.Unmarshal(command.Data, &data) != nil || data.SlideID != result.slideID || data.ElementID <= 0 || data.MediaType != mediaType || !data.Changed {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.createdSlideID = data.SlideID
				out.createdElementID = data.ElementID
			}
		}
		select {
		case view.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
}
