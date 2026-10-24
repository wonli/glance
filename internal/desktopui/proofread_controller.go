package desktopui

import (
	"context"
	"fmt"
	"gioui.org/widget"
	"glance/internal/application"
	"glance/internal/proofreading"
	"strings"
	"unicode/utf8"
)

type proofreadingModelLayoutResult struct {
	generation uint64
	layout     application.ProofreadingLayoutSnapshot
	reflow     bool
	err        error
}

func (a *App) ensureProofreadingModel() bool {
	s := a.proofreading
	if s == nil || s.bDocument == nil || len(s.pageFrames) == 0 || s.draftRestoring {
		return false
	}
	if s.controller != nil {
		return true
	}
	ids := make([]proofreading.PageID, len(s.pageFrames))
	for i := range ids {
		ids[i] = proofreading.PageID(i + 1)
	}
	var m *proofreading.Model
	var err error
	if s.restoredModel != nil {
		m, err = proofreading.Restore(*s.restoredModel)
		if err == nil && len(m.Snapshot().Pages) != len(ids) {
			err = fmt.Errorf("saved proofreading page count differs from source")
		}
		if err == nil {
			for i, p := range m.Snapshot().Pages {
				if p.ID != ids[i] {
					err = fmt.Errorf("saved proofreading page identities differ from source")
					break
				}
			}
		}
		s.modelInitialAssigned = true
	} else {
		m, err = proofreading.New(s.bDraftText, ids)
		if err == nil && len(s.pageOwnership) > 0 {
			if len(s.pageOwnership) != len(ids) {
				err = fmt.Errorf("legacy proofreading page count is inconsistent")
			} else {
				ends := make([]int, len(ids))
				boundary := 0
				length := utf8.RuneCountInString(s.bDraftText)
				for i, p := range s.pageOwnership {
					if p.Start != boundary || p.End < p.Start || p.End > length {
						err = fmt.Errorf("legacy proofreading page ranges overlap or omit content; original copy retained")
						break
					}
					ends[i] = p.End
					boundary = p.End
				}
				if err == nil {
					err = m.AssignInitial(ends)
				}
				if err == nil {
					for i, p := range s.pageOwnership {
						if p.Reviewed {
							err = m.Review(ids[i], true)
						} else if p.Locked {
							err = m.Lock(ids[i], true)
						}
						if err != nil {
							break
						}
					}
				}
				if err == nil {
					m, err = proofreading.Restore(m.Snapshot())
				}
				s.modelInitialAssigned = err == nil
			}
		}
	}
	if err != nil {
		s.draftError = err.Error()
		s.draftPersistError = err.Error()
		return false
	}
	s.controller = application.NewProofreadingController(m)
	if s.modelLayoutResults == nil {
		s.modelLayoutResults = make(chan proofreadingModelLayoutResult, 1)
	}
	return true
}

// This view is derived exclusively from character ownership, never from capacity.
func (a *App) refreshProofreadingModelView() {
	s := a.proofreading
	if !a.ensureProofreadingModel() {
		return
	}
	snap := s.controller.Model.Snapshot()
	texts := make(map[proofreading.PageID]*strings.Builder, len(snap.Pages))
	for _, p := range snap.Pages {
		texts[p.ID] = &strings.Builder{}
	}
	var all strings.Builder
	for _, r := range snap.Runs {
		all.WriteString(r.Text)
		texts[r.Page].WriteString(r.Text)
	}
	s.bDraftText = all.String()
	count := len(snap.Pages)
	if len(s.xPageEditors) != count {
		s.xPageEditors = make([]widget.Editor, count)
		s.xPageScrollbars = make([]widget.Scrollbar, count)
		s.xEditorTexts = make([]string, count)
		s.xEditorWindows = make([]proofreadingEditorWindow, count)
		s.xEditorCarets = make([]proofreadingEditorCaret, count)
		s.xPageRenderScales = make([]float64, count)
		s.xPageViewportHeights = make([]float64, count)
		s.xPageScrollOffsets = make([]float64, count)
		s.xRevealCaret = make([]bool, count)
		s.xPagePointerTags = make([]proofreadingPagePointerTag, count)
		for i := range s.xPagePointerTags {
			s.xPagePointerTags[i].pageIndex = i
		}
	}
	previousPages := s.xPages
	s.xPages = make([]proofreadingXPage, count)
	s.pageOwnership = make([]proofreadingPageOwnership, count)
	start := 0
	s.xPageAdjustmentActive = false
	for i, p := range snap.Pages {
		text := texts[p.ID].String()
		if i >= len(previousPages) || previousPages[i].Content != text {
			if s.modelDirtyPages == nil {
				s.modelDirtyPages = map[proofreading.PageID]bool{}
			}
			s.modelDirtyPages[p.ID] = true
		}
		end := start + utf8.RuneCountInString(text)
		frame := s.pageFrames[i]
		s.xPages[i] = proofreadingXPage{PageIndex: i, BPageIndex: i, Width: frame.Width, Height: frame.Height, FrameUnavailable: frame.FrameUnavailable, ContentStart: start, ContentEnd: end, Content: text}
		s.pageOwnership[i] = proofreadingPageOwnership{Start: start, End: end, Locked: p.Locked, Reviewed: p.Reviewed}
		if p.Locked && !p.Reviewed {
			s.xPageAdjustmentActive = true
			s.xPageAdjustmentPage = i
		}
		start = end
	}
	if !s.modelLayoutPending && s.controller.Layout.Pages == nil {
		a.startProofreadingModelLayout(!s.modelInitialAssigned)
	}
}
func (a *App) proofreadingModelChanged(reflow bool) {
	s := a.proofreading
	s.bDraftDirty = true
	a.refreshProofreadingModelView()
	a.startProofreadingModelLayout(reflow)
	a.queueProofreadingDraftPersistence()
	if a.window != nil {
		a.window.Invalidate()
	}
}

// Boundary motion preserves the complete text stream. Publish the adjacent
// boxes plus any refilled suffix; bDraftText is unchanged by ownership moves.
func (a *App) proofreadingBoundaryChanged(index int, affected ...proofreading.PageID) {
	s := a.proofreading
	if index < 0 || index >= len(s.xPages) {
		return
	}
	start := s.xPages[index].ContentStart
	last := min(index+1, len(s.xPages)-1)
	affected = append(affected, s.controller.Model.BoundaryAffectedPages()...)
	for _, id := range affected {
		last = max(last, int(id)-1)
	}
	last = min(last, len(s.xPages)-1)
	for i := index; i <= last; i++ {
		p := &s.xPages[i]
		value := s.controller.Model.Text(proofreading.PageID(i + 1))
		state, _ := s.controller.Model.Page(proofreading.PageID(i + 1))
		if p.Content != value {
			if s.modelDirtyPages == nil {
				s.modelDirtyPages = map[proofreading.PageID]bool{}
			}
			s.modelDirtyPages[proofreading.PageID(i+1)] = true
		}
		p.Content = value
		p.ContentStart = start
		p.ContentEnd = start + s.controller.Model.RuneCount(proofreading.PageID(i+1))
		owned := &s.pageOwnership[i]
		owned.Start = p.ContentStart
		owned.End = p.ContentEnd
		owned.Locked = state.Locked
		owned.Reviewed = state.Reviewed
		start = p.ContentEnd
	}
	s.bDraftDirty = true
	pages := []proofreading.PageID{proofreading.PageID(index + 1)}
	for i := index + 1; i <= last; i++ {
		pages = append(pages, proofreading.PageID(i+1))
	}
	a.startProofreadingModelLayoutForPages(false, pages)
	a.queueProofreadingDraftPersistence()
	if a.window != nil {
		a.window.Invalidate()
	}
}
func (a *App) startProofreadingModelLayout(fullLayout bool) {
	a.startProofreadingModelLayoutForPages(fullLayout, nil)
}
func (a *App) startProofreadingModelLayoutForPages(fullLayout bool, only []proofreading.PageID) {
	s := a.proofreading
	if s == nil || s.controller == nil {
		return
	}
	if s.modelLayoutCancel != nil {
		s.modelLayoutCancel()
	}
	s.modelLayoutGeneration++
	generation := s.modelLayoutGeneration
	ctx, cancel := context.WithCancel(context.Background())
	s.modelLayoutCancel = cancel
	s.modelLayoutPending = true
	captured := s.controller.Model.Capture()
	modelPages := captured.Pages()
	initial := !s.modelInitialAssigned
	full := only == nil && (fullLayout || initial || s.modelLayoutNeedsFull || s.controller.Layout.Pages == nil || s.controller.Layout.StyleRevision != s.xTextSizeStep)
	s.modelLayoutNeedsFull = full
	previous := s.controller.Layout
	if previous.StyleRevision != s.xTextSizeStep {
		previous.Pages = nil
	}
	var dirty []proofreading.PageID
	if only != nil {
		// Replacing a worker must not discard suffix pages still awaiting layout
		// after a refill in an earlier scroll frame.
		requested := make(map[proofreading.PageID]bool, len(only)+len(s.modelDirtyPages))
		for _, id := range only {
			requested[id] = true
		}
		for _, p := range modelPages {
			if requested[p.ID] || s.modelDirtyPages[p.ID] {
				dirty = append(dirty, p.ID)
			}
		}
	} else {
		for _, p := range modelPages {
			if full || s.modelDirtyPages[p.ID] {
				dirty = append(dirty, p.ID)
			}
		}
	}
	// During interactive ownership changes, shape the current pair and visible
	// boxes first. Offscreen roots already have correct ownership; keep them dirty
	// until they are visible rather than blocking feedback on the whole suffix.
	if !initial && only != nil && len(s.visibleEditorPages) > 0 {
		visible := map[proofreading.PageID]bool{}
		for _, index := range s.visibleEditorPages {
			visible[proofreading.PageID(index+1)] = true
		}
		for _, id := range only {
			if len(only) <= 2 {
				visible[id] = true
			}
		}
		if s.navigator.initialized {
			edge := s.navigator.page
			if s.navigator.leading {
				edge--
			}
			visible[proofreading.PageID(edge+1)] = true
			visible[proofreading.PageID(edge+2)] = true
		}
		selected := dirty[:0]
		for _, id := range dirty {
			if visible[id] {
				selected = append(selected, id)
			}
		}
		if len(selected) > 0 {
			dirty = selected
		}
	}
	s.modelLayoutActivePages = dirty
	geometry := make([]application.ProofreadingPageGeometry, len(s.pageFrames))
	for i, p := range s.pageFrames {
		marginX, marginY := proofreadingTextMargins(p.Width, p.Height)
		geometry[i] = application.ProofreadingPageGeometry{Page: proofreading.PageID(i + 1), Width: max(1, p.Width-2*marginX), Height: max(1, p.Height-2*marginY)}
	}
	style := application.ProofreadingTextStyle{Size: 12 * proofreadingXTextScale(s.xTextSizeStep), Family: string(a.theme.Face), Revision: s.xTextSizeStep}
	fontSnapshot := a.currentFontSnapshot()
	results, window := s.modelLayoutResults, a.window
	if !a.launchManagedBackground(func() {
		snap := captured.Snapshot()
		var result application.ProofreadingLayoutSnapshot
		var err error
		if initial {
			result, err = reflowProofreadingForEditor(ctx, snap, geometry, style, initial, fontSnapshot)
		} else {
			result, err = application.LayoutProofreadingOwnedPages(ctx, snap, geometry, style, previous, dirty, newProofreadingEditorLineShaper(fontSnapshot))
		}
		if ctx.Err() != nil {
			return
		}
		value := proofreadingModelLayoutResult{generation: generation, layout: result, reflow: initial, err: err}
		select {
		case results <- value:
		case <-ctx.Done():
			return
		case <-a.shutdown:
			return
		}
		if window != nil {
			window.Invalidate()
		}
	}) {
		s.modelLayoutPending = false
		s.draftPersistError = "could not start proofreading layout"
	}
}
func (a *App) acceptProofreadingModelLayout() {
	s := a.proofreading
	if s == nil || s.controller == nil {
		return
	}
	for {
		select {
		case r := <-s.modelLayoutResults:
			if r.generation != s.modelLayoutGeneration {
				continue
			}
			s.modelLayoutPending = false
			if r.err != nil {
				s.draftError = r.err.Error()
				continue
			}
			if r.layout.Revision != s.controller.Model.Revision() || r.layout.StyleRevision != s.xTextSizeStep {
				continue
			}
			if r.reflow {
				var err error
				if !s.modelInitialAssigned {
					ends := make([]int, len(s.pageFrames))
					for i := range ends {
						ends[i] = r.layout.Ends[proofreading.PageID(i+1)]
					}
					err = s.controller.Model.AssignInitial(ends)
					s.modelInitialAssigned = err == nil
				} else {
					err = s.controller.Model.ReflowDerived(r.layout.Ends)
				}
				if err != nil {
					s.draftError = err.Error()
					continue
				}
			}
			r.layout.Revision = s.controller.Model.Revision()
			s.controller.AcceptLayout(r.layout)
			s.modelLayoutNeedsFull = false
			if r.reflow {
				a.refreshProofreadingModelView()
				a.queueProofreadingDraftPersistence()
			}
			if r.reflow {
				s.modelDirtyPages = nil
			} else {
				for _, id := range s.modelLayoutActivePages {
					delete(s.modelDirtyPages, id)
				}
			}
		default:
			return
		}
	}
}
