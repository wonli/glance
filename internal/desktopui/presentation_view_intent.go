package desktopui

import (
	"encoding/json"
	"image"
	"math"
	"time"

	"glance/internal/application"
	"glance/internal/document"
)

type presentationViewArguments struct {
	Operation  string `json:"operation"`
	Mode       string `json:"mode,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
	PageNumber *int   `json:"page_number,omitempty"`
	CropAction string `json:"crop_action,omitempty"`
	SlideID    uint32 `json:"slide_id,omitempty"`
	ElementID  int64  `json:"element_id,omitempty"`
}

func (a *App) applyPresentationViewIntent(action string, raw json.RawMessage) (json.RawMessage, error) {
	if a == nil {
		return nil, application.ErrNoAttachedView
	}
	state := a.documentSessionState
	if a.window == nil || state == nil || state.pptxDoc == nil || state.pptxDoc != a.pptxDoc {
		return nil, application.ErrNoAttachedView
	}
	v := a.ensurePresentationView()
	if action == "presentation.view.get" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) != 0 {
			return nil, application.ErrInvalidViewIntent
		}
		return marshalPresentationViewState(a.windowID, v, false)
	}
	if action != "presentation.view.set" || application.ValidatePresentationViewArguments(raw) != nil {
		return nil, application.ErrInvalidViewIntent
	}
	var input struct {
		Operation   string `json:"operation"`
		PageNumber  *int   `json:"page_number"`
		ZoomPercent *int   `json:"zoom_percent"`
		Enabled     *bool  `json:"enabled"`
		Mode        string `json:"mode"`
		CropAction  string `json:"crop_action"`
		SlideID     uint32 `json:"slide_id"`
		ElementID   int64  `json:"element_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, application.ErrInvalidViewIntent
	}
	v.snapshot = state.pptxDoc.Snapshot()
	oldSlide, oldZoom, oldFit, oldNotes, oldRuler, oldSlideshow, oldPresenter, oldSorter := v.activeSlide, v.zoom, v.fitMode, v.notesExpanded, v.rulerVisible, v.slideshowMode, v.presenterMode, v.sorterMode
	oldRehearsalRunning := v.rehearsalRunning()
	oldRehearsalElapsed := v.rehearsalDuration(time.Now())
	oldCropActive, oldCropSlide, oldCropElement, oldCropPreview := v.pictureCrop.active, v.pictureCrop.slideIndex, v.pictureCrop.shapeID, v.pictureCrop.preview
	switch input.Operation {
	case "set_slide":
		if *input.PageNumber > v.snapshot.SlideCount {
			return nil, application.ErrInvalidCommandArguments
		}
		if v.pictureCrop.active && *input.PageNumber-1 != v.activeSlide {
			return nil, application.ErrPendingEditorDraft
		}
		if presentationTextDraftDirty(v) {
			return nil, application.ErrPendingEditorDraft
		}
		if presentationNotesDraftDirty(v) {
			return nil, application.ErrPendingEditorDraft
		}
		v.activeSlide = *input.PageNumber - 1
	case "set_zoom":
		v.zoom = float64(*input.ZoomPercent) / 100
		v.fitMode = false
	case "fit":
		v.fitMode = true
		v.zoom = 1
	case "set_notes_visible":
		v.notesExpanded = *input.Enabled
	case "set_ruler_visible":
		v.rulerVisible = *input.Enabled
	case "set_mode":
		if input.PageNumber != nil && *input.PageNumber > v.snapshot.SlideCount {
			return nil, application.ErrInvalidCommandArguments
		}
		if (input.Mode == "slideshow" || input.Mode == "presenter" || input.Mode == "sorter") && presentationTextDraftDirty(v) {
			return nil, application.ErrPendingEditorDraft
		}
		if (input.Mode == "slideshow" || input.Mode == "presenter" || input.Mode == "sorter") && v.pictureCrop.active {
			return nil, application.ErrPendingEditorDraft
		}
		if (input.Mode == "slideshow" || input.Mode == "presenter" || input.Mode == "sorter") && presentationNotesDraftDirty(v) {
			return nil, application.ErrPendingEditorDraft
		}
		if input.Mode == "slideshow" || input.Mode == "presenter" || input.Mode == "sorter" {
			v.editingShape = 0
			v.editingArea = image.Rectangle{}
			v.shapeDraftText = ""
			a.markPresentationTextEditorOpen(v, false)
		}
		if input.Mode != "slideshow" && input.Mode != "presenter" {
			stopPresentationRehearsal(v, time.Now())
		}
		v.slideshowMode = input.Mode == "slideshow" || input.Mode == "presenter"
		v.presenterMode = input.Mode == "presenter"
		v.sorterMode = input.Mode == "sorter"
		if input.Mode == "presenter" && !v.rehearsalRunning() {
			if !oldSlideshow {
				resetPresentationRehearsal(v, time.Now())
			}
			a.startPresentationRehearsal(v)
		}
		if input.PageNumber != nil {
			v.activeSlide = *input.PageNumber - 1
		}
	case "rehearsal_start":
		if presentationTextDraftDirty(v) || presentationNotesDraftDirty(v) || v.pictureCrop.active {
			return nil, application.ErrPendingEditorDraft
		}
		v.editingShape = 0
		v.editingArea = image.Rectangle{}
		v.shapeDraftText = ""
		a.markPresentationTextEditorOpen(v, false)
		v.slideshowMode = true
		v.presenterMode = false
		v.sorterMode = false
		a.startPresentationRehearsal(v)
	case "rehearsal_stop":
		stopPresentationRehearsal(v, time.Now())
	case "rehearsal_reset":
		resetPresentationRehearsal(v, time.Now())
	case "picture_crop_mode":
		targetSlide := -1
		var targetPicture *document.PresentationElement
		for index := range v.snapshot.Slides {
			slide := &v.snapshot.Slides[index]
			if slide.ID != input.SlideID {
				continue
			}
			targetSlide = index
			for elementIndex := range slide.Elements {
				if slide.Elements[elementIndex].ID == input.ElementID {
					element := slide.Elements[elementIndex]
					targetPicture = &element
					break
				}
			}
			break
		}
		if targetSlide < 0 || targetPicture == nil || targetPicture.Kind != document.ElementKindPicture || !targetPicture.CanEditPicture {
			return nil, application.ErrCapabilityUnsupported
		}
		switch input.CropAction {
		case "begin":
			if v.slideshowMode {
				return nil, application.ErrInvalidViewIntent
			}
			if v.editingShape != 0 || presentationTextDraftDirty(v) || presentationNotesDraftDirty(v) {
				return nil, application.ErrPendingEditorDraft
			}
			if targetSlide != v.activeSlide {
				v.activeSlide = targetSlide
				v.slideList.Position.First = targetSlide
				clearPresentationSelection(v)
				v.editingShape = 0
				v.shapeDraftText = ""
				a.markPresentationTextEditorOpen(v, false)
				if slide := v.currentSlide(); slide != nil {
					a.resetPresentationNotesEditor(v, slide)
				}
			}
			setPresentationSingleSelection(v, input.ElementID)
			if !a.beginPresentationPictureCrop(v) {
				return nil, application.ErrCapabilityUnsupported
			}
		case "reset", "cancel":
			if !v.pictureCrop.active || v.pictureCrop.slideIndex != targetSlide || v.pictureCrop.shapeID != input.ElementID {
				return nil, application.ErrInvalidViewIntent
			}
			if input.CropAction == "reset" {
				a.resetPresentationPictureCrop(v)
			} else {
				a.cancelPresentationPictureCrop(v)
			}
		}
	}
	changed := oldSlide != v.activeSlide || oldZoom != v.zoom || oldFit != v.fitMode || oldNotes != v.notesExpanded || oldRuler != v.rulerVisible || oldSlideshow != v.slideshowMode || oldPresenter != v.presenterMode || oldSorter != v.sorterMode || oldRehearsalRunning != v.rehearsalRunning() || oldRehearsalElapsed != v.rehearsalDuration(time.Now()) || oldCropActive != v.pictureCrop.active || oldCropSlide != v.pictureCrop.slideIndex || oldCropElement != v.pictureCrop.shapeID || oldCropPreview != v.pictureCrop.preview
	if oldSlide != v.activeSlide {
		v.slideList.Position.First = v.activeSlide
		clearPresentationSelection(v)
		v.editingShape = 0
		a.markPresentationTextEditorOpen(v, false)
		v.shapeDraftText = ""
		if slide := v.currentSlide(); slide != nil {
			a.resetPresentationNotesEditor(v, slide)
		}
	}
	if changed && a.window != nil {
		a.window.Invalidate()
	}
	return marshalPresentationViewState(a.windowID, v, changed)
}

func marshalPresentationViewState(windowID uint64, view *presentationViewState, changed bool) (json.RawMessage, error) {
	slideCount := max(1, view.snapshot.SlideCount)
	slideNumber := min(slideCount, max(1, view.activeSlide+1))
	mode := "normal"
	if view.slideshowMode {
		mode = "slideshow"
		if view.presenterMode {
			mode = "presenter"
		}
	} else if view.sorterMode {
		mode = "sorter"
	}
	cropSlideID, cropElementID := uint32(0), int64(0)
	if view.pictureCrop.active && view.pictureCrop.slideIndex >= 0 && view.pictureCrop.slideIndex < len(view.snapshot.Slides) {
		cropSlideID = view.snapshot.Slides[view.pictureCrop.slideIndex].ID
		cropElementID = view.pictureCrop.shapeID
	}
	return json.Marshal(struct {
		WindowID           uint64 `json:"window_id"`
		SlideNumber        int    `json:"slide_number"`
		SlideCount         int    `json:"slide_count"`
		ZoomPercent        int    `json:"zoom_percent"`
		FitMode            bool   `json:"fit_mode"`
		Notes              bool   `json:"notes_visible"`
		Ruler              bool   `json:"ruler_visible"`
		Mode               string `json:"mode"`
		RehearsalRunning   bool   `json:"rehearsal_running"`
		RehearsalElapsedMS int64  `json:"rehearsal_elapsed_ms"`
		CropActive         bool   `json:"picture_crop_active"`
		CropSlideID        uint32 `json:"picture_crop_slide_id"`
		CropElementID      int64  `json:"picture_crop_element_id"`
		CropPreview        struct {
			Left   int `json:"left"`
			Top    int `json:"top"`
			Right  int `json:"right"`
			Bottom int `json:"bottom"`
		} `json:"picture_crop_preview"`
		Changed bool `json:"changed"`
	}{
		WindowID: windowID, SlideNumber: slideNumber, SlideCount: slideCount,
		ZoomPercent: int(math.Round(view.zoom * 100)), FitMode: view.fitMode,
		Notes: view.notesExpanded, Ruler: view.rulerVisible, Mode: mode, CropActive: view.pictureCrop.active,
		RehearsalRunning: view.rehearsalRunning(), RehearsalElapsedMS: int64(view.rehearsalDuration(time.Now()) / time.Millisecond),
		CropSlideID: cropSlideID, CropElementID: cropElementID,
		CropPreview: struct {
			Left   int `json:"left"`
			Top    int `json:"top"`
			Right  int `json:"right"`
			Bottom int `json:"bottom"`
		}{Left: view.pictureCrop.preview.Left, Top: view.pictureCrop.preview.Top, Right: view.pictureCrop.preview.Right, Bottom: view.pictureCrop.preview.Bottom},
		Changed: changed,
	})
}

func (a *App) runPresentationViewRibbonAction(view *presentationViewState, actionID string) bool {
	if view == nil {
		return false
	}
	var arguments presentationViewArguments
	switch actionID {
	case "view_normal":
		arguments = presentationViewArguments{Operation: "set_mode", Mode: "normal"}
	case "view_sorter":
		arguments = presentationViewArguments{Operation: "set_mode", Mode: "sorter"}
	case "view_notes", "home_notes":
		arguments = presentationViewArguments{Operation: "set_notes_visible", Enabled: commandArgumentPointer(!view.notesExpanded)}
	case "view_ruler":
		arguments = presentationViewArguments{Operation: "set_ruler_visible", Enabled: commandArgumentPointer(!view.rulerVisible)}
	case "view_fit":
		arguments = presentationViewArguments{Operation: "fit"}
	case "show_from_start", "home_slideshow":
		arguments = presentationViewArguments{Operation: "set_mode", Mode: "slideshow", PageNumber: commandArgumentPointer(1)}
	case "show_from_current":
		arguments = presentationViewArguments{Operation: "set_mode", Mode: "slideshow"}
	case "show_rehearse":
		arguments = presentationViewArguments{Operation: "rehearsal_start"}
	case "show_presenter":
		arguments = presentationViewArguments{Operation: "set_mode", Mode: "presenter"}
	case "picture_crop":
		slideIndex, picture, ok := presentationSelectedEditablePicture(view)
		if !ok || slideIndex < 0 || slideIndex >= len(view.snapshot.Slides) {
			a.setStatus("请先选择可编辑的图片")
			return true
		}
		arguments = presentationViewArguments{Operation: "picture_crop_mode", CropAction: "begin", SlideID: view.snapshot.Slides[slideIndex].ID, ElementID: picture.ID}
	case "picture_crop_cancel", "picture_crop_reset":
		if !view.pictureCrop.active || view.pictureCrop.slideIndex < 0 || view.pictureCrop.slideIndex >= len(view.snapshot.Slides) {
			return true
		}
		action := "cancel"
		if actionID == "picture_crop_reset" {
			action = "reset"
		}
		arguments = presentationViewArguments{Operation: "picture_crop_mode", CropAction: action, SlideID: view.snapshot.Slides[view.pictureCrop.slideIndex].ID, ElementID: view.pictureCrop.shapeID}
	default:
		return false
	}
	if actionID == "picture_crop" {
		if err := a.flushPresentationNotesDraft(view); err != nil {
			return true
		}
		if err := a.flushPresentationTextDraft(); err != nil {
			return true
		}
	}
	if actionID == "show_from_start" || actionID == "show_from_current" || actionID == "show_rehearse" || actionID == "show_presenter" || actionID == "home_slideshow" || actionID == "view_sorter" {
		if err := a.flushPresentationNotesDraft(view); err != nil {
			return true
		}
		if presentationTextDraftDirty(view) {
			if err := a.flushPresentationTextDraft(); err != nil {
				return true
			}
		}
	}
	raw, err := json.Marshal(arguments)
	if err == nil {
		_, err = a.applyPresentationViewIntent("presentation.view.set", raw)
	}
	if err != nil {
		a.setStatus("设置演示文稿视图失败：" + err.Error())
	} else if actionID == "show_rehearse" {
		a.setStatus("排练放映已开始；计时可通过演示文稿视图查询")
	}
	return true
}
