package desktopui

import (
	"image"
	"time"

	"gioui.org/gesture"
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/document"
)

type presentationShapeHitArea struct {
	elementOrigin image.Point
	hitOrigin     image.Point
	textOrigin    image.Point
	size          image.Point
}

type presentationViewState struct {
	snapshot                document.PresentationSnapshot
	activeSlide             int
	commandPending          bool
	commandResults          chan presentationCommandResult
	insertTextBox           bool
	insertShape             bool
	insertPicturePending    bool
	insertPictureData       []byte
	insertPictureMediaType  string
	insertPictureWidth      int
	insertPictureHeight     int
	insertPictureGeneration uint64
	zoom                    float64 // 1.0 = 100%
	fitMode                 bool    // auto fit to viewport
	notesExpanded           bool    // whether speaker notes panel is open
	rulerVisible            bool
	sorterMode              bool

	// Navigation sidebar (248 dp width)
	slideList      widget.List
	slideClicks    map[int]*widget.Clickable
	sorterList     widget.List
	sorterButtons  map[uint32]*widget.Clickable
	addSlideButton widget.Clickable
	delSlideButton widget.Clickable

	// Ribbon bar
	activeRibbonTab     string // "home", "insert", "design", "slideshow", "view"
	ribbonButtons       map[string]*widget.Clickable
	ribbonActionList    widget.List
	layoutList          widget.List
	layoutOptionButtons map[int]*widget.Clickable
	layoutOptions       []document.PresentationLayoutOption
	layoutSlideID       uint32
	layoutRevision      uint64
	stylePopover        string
	stylePopoverShapeID int64
	styleOptionList     widget.List
	styleOptionBtns     map[string]*widget.Clickable

	// Canvas & Elements
	canvasList       widget.List
	canvasBlankClick gesture.Click
	canvasCardOrigin image.Point
	canvasCardSize   image.Point
	canvasScale      float64
	editingArea      image.Rectangle
	selectedShape    int64
	editingShape     int64
	selectedShapes   map[int64]bool
	shapeClicks      map[int64]*widget.Clickable
	shapeHitAreas    map[int64]presentationShapeHitArea
	transformInputs  map[int64]*presentationShapeInteraction
	transformGesture presentationTransformGesture
	pictureCrop      presentationPictureCropState
	shapeEditor      widget.Editor
	shapeDraftText   string // Active text-box draft, committed on save or when leaving the shape.

	// Clipboard requests are window-local Gio operations; mutations still go
	// through the shared presentation capability catalog.
	clipboardTarget          struct{}
	clipboardReadPending     bool
	clipboardDocumentID      string
	clipboardDocumentVersion uint64
	clipboardPasteSlideID    uint32
	clipboardPasteElementID  int64
	clipboardWritePending    bool
	clipboardWriteText       string

	// Notes panel
	notesEditor        widget.Editor
	notesEditorBase    string
	notesEditorSlideID uint32
	notesDraftConflict bool
	notesToggleBtn     widget.Clickable
	notesReloadButton  widget.Clickable
	notesKeepButton    widget.Clickable
	lastNotesSlide     int

	// Status bar controls
	zoomInBtn    widget.Clickable
	zoomOutBtn   widget.Clickable
	zoomFitBtn   widget.Clickable
	slideshowBtn widget.Clickable

	// Fullscreen slideshow mode
	slideshowMode bool
	presenterMode bool
	prevSlideBtn  widget.Clickable
	nextSlideBtn  widget.Clickable
	exitShowBtn   widget.Clickable

	// Rehearsal timing is window-local and is not written into the PPTX.
	rehearsalStartedAt time.Time
	rehearsalElapsed   time.Duration
	rehearsalTickStop  chan struct{}

	// Cached thumb dimensions
	thumbAspect float64
}

func (a *App) ensurePresentationView() *presentationViewState {
	if a.pptxView == nil {
		a.pptxView = &presentationViewState{
			activeSlide:         0,
			commandResults:      make(chan presentationCommandResult, 1),
			zoom:                1.0,
			fitMode:             true,
			notesExpanded:       false,
			activeRibbonTab:     "home",
			slideClicks:         make(map[int]*widget.Clickable),
			sorterList:          widget.List{List: layout.List{Axis: layout.Vertical}},
			sorterButtons:       make(map[uint32]*widget.Clickable),
			shapeClicks:         make(map[int64]*widget.Clickable),
			shapeHitAreas:       make(map[int64]presentationShapeHitArea),
			transformInputs:     make(map[int64]*presentationShapeInteraction),
			ribbonButtons:       make(map[string]*widget.Clickable),
			layoutList:          widget.List{List: layout.List{Axis: layout.Vertical}},
			layoutOptionButtons: make(map[int]*widget.Clickable),
			styleOptionBtns:     make(map[string]*widget.Clickable),
			thumbAspect:         16.0 / 9.0,
			lastNotesSlide:      -1,
		}
		a.pptxView.slideList.Axis = layout.Vertical
		a.pptxView.canvasList.Axis = layout.Vertical
		a.pptxView.ribbonActionList.Axis = layout.Horizontal
		a.pptxView.styleOptionList.Axis = layout.Horizontal
		a.pptxView.shapeEditor.SingleLine = false
		a.pptxView.notesEditor.SingleLine = false
	}
	a.pollPresentationCommandResult(a.pptxView)
	if a.pptxDoc != nil {
		a.pptxView.snapshot = a.pptxDoc.Snapshot()
		if a.pptxView.activeSlide >= a.pptxView.snapshot.SlideCount && a.pptxView.snapshot.SlideCount > 0 {
			a.pptxView.activeSlide = a.pptxView.snapshot.SlideCount - 1
		}
	}
	if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
		a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTextDraft, presentationTextDraftDirty(a.pptxView))
		a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTextEditorOpen, a.pptxView.editingShape != 0)
	}
	a.syncPresentationNotesDraft(a.pptxView)
	return a.pptxView
}

func (a *App) resetPresentationNotesEditor(v *presentationViewState, slide *document.PresentationSlide) {
	if v == nil || slide == nil {
		return
	}
	v.notesEditor.SetText(slide.Notes)
	v.notesEditorBase = slide.Notes
	v.notesEditorSlideID = slide.ID
	v.lastNotesSlide = v.activeSlide
	v.notesDraftConflict = false
	if a != nil && a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
		a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationNotesDraft, false)
	}
}

func (a *App) markPresentationTextEditorOpen(v *presentationViewState, open bool) {
	if a == nil || v == nil || a.documentSessionState == nil || a.documentSessionState.pptxDoc != a.pptxDoc {
		return
	}
	a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTextEditorOpen, open)
}

func (a *App) syncPresentationNotesDraft(v *presentationViewState) {
	if v == nil {
		return
	}
	slide := v.currentSlide()
	if slide == nil {
		return
	}
	if v.notesEditorSlideID != slide.ID {
		a.resetPresentationNotesEditor(v, slide)
		return
	}
	text := v.notesEditor.Text()
	if text == v.notesEditorBase {
		if slide.Notes != v.notesEditorBase {
			a.resetPresentationNotesEditor(v, slide)
		}
		return
	}
	if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
		a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationNotesDraft, true)
	}
	if v.notesDraftConflict {
		return
	}
	if slide.Notes == text {
		a.resetPresentationNotesEditor(v, slide)
		return
	}
	if slide.Notes != v.notesEditorBase {
		v.notesDraftConflict = true
		a.setStatus("演讲者备注已被其他操作修改；请重新加载或明确保留本地草稿")
	}
}

func (v *presentationViewState) slideClick(index int) *widget.Clickable {
	if btn, ok := v.slideClicks[index]; ok {
		return btn
	}
	btn := &widget.Clickable{}
	v.slideClicks[index] = btn
	return btn
}

func (v *presentationViewState) shapeClick(id int64) *widget.Clickable {
	if btn, ok := v.shapeClicks[id]; ok {
		return btn
	}
	btn := &widget.Clickable{}
	v.shapeClicks[id] = btn
	return btn
}

func (v *presentationViewState) ribbonBtn(id string) *widget.Clickable {
	if btn, ok := v.ribbonButtons[id]; ok {
		return btn
	}
	btn := &widget.Clickable{}
	v.ribbonButtons[id] = btn
	return btn
}

func (v *presentationViewState) currentSlide() *document.PresentationSlide {
	if len(v.snapshot.Slides) == 0 {
		return nil
	}
	if v.activeSlide < 0 || v.activeSlide >= len(v.snapshot.Slides) {
		return &v.snapshot.Slides[0]
	}
	return &v.snapshot.Slides[v.activeSlide]
}
