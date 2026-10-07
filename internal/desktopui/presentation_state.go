package desktopui

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/document"
)

type presentationViewState struct {
	snapshot      document.PresentationSnapshot
	activeSlide   int
	zoom          float64 // 1.0 = 100%
	fitMode       bool    // auto fit to viewport
	notesExpanded bool    // whether speaker notes panel is open

	// Navigation sidebar (248 dp width)
	slideList      widget.List
	slideClicks    map[int]*widget.Clickable
	addSlideButton widget.Clickable
	delSlideButton widget.Clickable

	// Ribbon bar
	activeRibbonTab  string // "home", "insert", "design", "slideshow", "view"
	ribbonButtons    map[string]*widget.Clickable
	ribbonActionList widget.List

	// Canvas & Elements
	canvasList     widget.List
	selectedShape  int64
	editingShape   int64
	shapeClicks    map[int64]*widget.Clickable
	shapeEditor    widget.Editor
	shapeDraftText string

	// Notes panel
	notesEditor    widget.Editor
	notesToggleBtn widget.Clickable
	lastNotesSlide int

	// Status bar controls
	zoomInBtn    widget.Clickable
	zoomOutBtn   widget.Clickable
	zoomFitBtn   widget.Clickable
	slideshowBtn widget.Clickable

	// Fullscreen slideshow mode
	slideshowMode bool
	prevSlideBtn  widget.Clickable
	nextSlideBtn  widget.Clickable
	exitShowBtn   widget.Clickable

	// Cached thumb dimensions
	thumbAspect float64
}

func (a *App) ensurePresentationView() *presentationViewState {
	if a.pptxView == nil {
		a.pptxView = &presentationViewState{
			activeSlide:     0,
			zoom:            1.0,
			fitMode:         true,
			notesExpanded:   false,
			activeRibbonTab: "home",
			slideClicks:     make(map[int]*widget.Clickable),
			shapeClicks:     make(map[int64]*widget.Clickable),
			ribbonButtons:   make(map[string]*widget.Clickable),
			thumbAspect:     16.0 / 9.0,
			lastNotesSlide:  -1,
		}
		a.pptxView.slideList.Axis = layout.Vertical
		a.pptxView.canvasList.Axis = layout.Vertical
		a.pptxView.ribbonActionList.Axis = layout.Horizontal
		a.pptxView.shapeEditor.SingleLine = false
		a.pptxView.notesEditor.SingleLine = false
	}
	if a.pptxDoc != nil {
		a.pptxView.snapshot = a.pptxDoc.Snapshot()
		if a.pptxView.activeSlide >= a.pptxView.snapshot.SlideCount && a.pptxView.snapshot.SlideCount > 0 {
			a.pptxView.activeSlide = a.pptxView.snapshot.SlideCount - 1
		}
	}
	return a.pptxView
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
