package desktopui

import (
	"math"
	"strings"
	"unicode/utf8"

	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget/material"
	"glance/internal/application"
	"glance/internal/proofreading"
	"image"
)

func proofreadingTextMargins(width, height float64) (float64, float64) {
	return min(30.0, max(0, width)/10), min(28.0, max(0, height)/12)
}

func proofreadingPageContentHeight(shaped application.ProofreadingPageLayout, owned string) float64 {
	count := utf8.RuneCountInString(owned)
	if count == 0 {
		return 0
	}
	trailingEmptyLine := strings.HasSuffix(owned, "\n")
	height := 0.0
	for _, line := range shaped.Lines {
		if line.Start < count || trailingEmptyLine && line.Start == count {
			height = max(height, line.Bottom)
		}
	}
	return height
}

func clampProofreadingScrollTop(top, viewport, content float64) float64 {
	return max(0, min(max(0, content-viewport), top))
}

func proofreadingPageScrollbarRange(top, viewport, content float64) (start, end float32, scrollable bool) {
	if math.IsNaN(top) || math.IsInf(top, 0) || math.IsNaN(viewport) || math.IsInf(viewport, 0) || math.IsNaN(content) || math.IsInf(content, 0) || viewport <= 0 || content <= viewport+0.01 {
		return 0, 1, false
	}
	top = clampProofreadingScrollTop(top, viewport, content)
	start = float32(top / content)
	end = float32(min(1, (top+viewport)/content))
	return start, end, end > start
}

func proofreadingXPageAt(gtx layout.Context, view *pdfViewState, s *proofreadingWorkspace, point image.Point) (int, image.Rectangle) {
	top := -s.xCanvas.Position.Offset
	for i := s.xCanvas.Position.First; i < len(s.pageFrames) && top < gtx.Constraints.Max.Y; i++ {
		size := proofreadingXPagePixelSize(gtx, view, s, i)
		left := (gtx.Constraints.Max.X - size.X) / 2
		bounds := image.Rect(left, top+gtx.Dp(12), left+size.X, top+gtx.Dp(12)+size.Y)
		if point.In(bounds) {
			return i, bounds
		}
		top += size.Y + 2*gtx.Dp(12)
	}
	return -1, image.Rectangle{}
}
func (a *App) handleProofreadingXCanvasScroll(gtx layout.Context, view *pdfViewState) {
	s := a.proofreading
	if s == nil || view == nil {
		return
	}
	for index, delta := range s.pendingXPageWheel {
		delete(s.pendingXPageWheel, index)
		a.scrollProofreadingPageView(gtx, view, index, delta)
	}
	filter := pointer.Filter{Target: &s.xCanvasScrollPointerTag, Kinds: pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -(1 << 20), Max: 1 << 20}}
	for {
		v, ok := gtx.Event(filter)
		if !ok {
			break
		}
		e, ok := v.(pointer.Event)
		if !ok || e.Scroll.Y == 0 {
			continue
		}
		index, _ := proofreadingXPageAt(gtx, view, s, image.Pt(int(e.Position.X), int(e.Position.Y)))
		if index < 0 || index >= len(s.xPageScrollbars) {
			s.xCanvasScrollRemainder += e.Scroll.Y
			delta := int(s.xCanvasScrollRemainder)
			s.xCanvasScrollRemainder -= float32(delta)
			s.xCanvas.Position.Offset += delta
			continue
		}
		a.scrollProofreadingPageView(gtx, view, index, float64(e.Scroll.Y))
	}
}

func (a *App) scrollProofreadingPageView(gtx layout.Context, view *pdfViewState, index int, delta float64) {
	s := a.proofreading
	if s == nil || s.controller == nil || index < 0 || index >= len(s.xPages) || index >= len(s.xPageScrollOffsets) {
		return
	}
	page := s.xPages[index]
	size := proofreadingXPagePixelSize(gtx, view, s, index)
	scale := float64(size.X) / max(1, page.Width)
	if s.controller.Layout.Revision != s.controller.Model.Revision() || s.controller.Layout.StyleRevision != s.xTextSizeStep {
		if s.pendingXPageWheel == nil {
			s.pendingXPageWheel = map[int]float64{}
		}
		s.pendingXPageWheel[index] += delta
		return
	}
	shaped, ok := s.controller.Layout.Pages[proofreading.PageID(index+1)]
	if !ok || !strings.HasPrefix(shaped.Text, page.Content) {
		if s.pendingXPageWheel == nil {
			s.pendingXPageWheel = map[int]float64{}
		}
		s.pendingXPageWheel[index] += delta
		return
	}
	_, marginY := proofreadingTextMargins(page.Width, page.Height)
	height := max(1, page.Height-2*marginY)
	contentHeight := proofreadingPageContentHeight(shaped, page.Content)
	top := clampProofreadingScrollTop(s.xPageScrollOffsets[index]+delta/scale, height, contentHeight)
	if top != s.xPageScrollOffsets[index] {
		s.xPageScrollOffsets[index] = top
		if a.window != nil {
			a.window.Invalidate()
		}
	}
}

func (a *App) layoutProofreadingXPageScrollbar(gtx layout.Context, page proofreadingXPage, pageSize image.Point) {
	s := a.proofreading
	if s == nil || s.controller == nil || page.PageIndex < 0 || page.PageIndex >= len(s.xPageScrollbars) || page.PageIndex >= len(s.xPageScrollOffsets) {
		return
	}
	pageID := proofreading.PageID(page.PageIndex + 1)
	if s.controller.Layout.Revision != s.controller.Model.Revision() || s.controller.Layout.StyleRevision != s.xTextSizeStep {
		return
	}
	shaped, ok := s.controller.Layout.Pages[pageID]
	if !ok || !strings.HasPrefix(shaped.Text, page.Content) {
		return
	}
	scale := min(float64(pageSize.X)/max(1, page.Width), float64(pageSize.Y)/max(1, page.Height))
	marginDocX, marginDocY := proofreadingTextMargins(page.Width, page.Height)
	viewport := max(1, page.Height-2*marginDocY)
	contentHeight := proofreadingPageContentHeight(shaped, page.Content)
	top := clampProofreadingScrollTop(s.xPageScrollOffsets[page.PageIndex], viewport, contentHeight)
	s.xPageScrollOffsets[page.PageIndex] = top
	start, end, scrollable := proofreadingPageScrollbarRange(top, viewport, contentHeight)
	if !scrollable {
		return
	}
	style := material.Scrollbar(a.theme, &s.xPageScrollbars[page.PageIndex])
	width := gtx.Dp(style.Width())
	marginPxX := int(math.Round(marginDocX * scale))
	marginPxY := int(math.Round(marginDocY * scale))
	trackHeight := min(max(1, pageSize.Y-2*marginPxY), max(1, int(math.Round(viewport*scale))))
	trackBounds := image.Rect(pageSize.X-marginPxX+gtx.Dp(4), marginPxY, pageSize.X-marginPxX+gtx.Dp(4)+width, marginPxY+trackHeight)
	trackClip := clip.Rect(trackBounds).Push(gtx.Ops)
	semantic.DescriptionOp(a.textID("proofread.x_content_scrollbar.description")).Add(gtx.Ops)
	trackClip.Pop()
	position := op.Offset(trackBounds.Min).Push(gtx.Ops)
	barGtx := gtx
	barGtx.Constraints = layout.Exact(image.Pt(width, trackHeight))
	style.Layout(barGtx, layout.Vertical, start, end)
	position.Pop()
	if delta := s.xPageScrollbars[page.PageIndex].ScrollDistance(); delta != 0 {
		s.xPageScrollOffsets[page.PageIndex] = clampProofreadingScrollTop(top+float64(delta)*contentHeight, viewport, contentHeight)
		if a.window != nil {
			a.window.Invalidate()
		}
	}
}
