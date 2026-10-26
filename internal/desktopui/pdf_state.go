package desktopui

import (
	"gioui.org/gesture"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/document"
	"glance/internal/i18n"
	"image"
	"strings"
	"sync"
)

type pdfPageResult struct {
	page     document.PDFPage
	pixels   image.Image
	imageOp  paint.ImageOp
	warning  string
	err      error
	info     document.PDFInfo
	render   pdfRenderKind
	dpi      float64
	mutation bool
	notice   i18n.Message
	search   []int
}

type pdfRenderKind uint8

const (
	pdfRenderNone pdfRenderKind = iota
	pdfRenderPage
	pdfRenderThumbnail
)

type pdfViewState struct {
	lastCommand                       document.PDFCommand
	info                              document.PDFInfo
	page, selected                    int
	pageMap                           []int
	selectStart, selectEnd            int
	zoom                              float64
	fit                               string
	mode, tool                        string
	thumbnails, showSearch            bool
	visualOnly                        bool
	toolbar, thumbList, canvas, notes widget.List
	paginationScrollbar               widget.Scrollbar
	proofreadingReadingLine           int
	proofreadingPageGap               int
	horizontal                        widget.List
	buttons                           map[string]*widget.Clickable
	search                            widget.Editor
	zoomScroll                        gesture.Scroll
	zoomScrollRemainder               float64
	canvasScrollRemainderX            float64
	canvasScrollRemainderY            float64
	results                           chan pdfPageResult
	pending                           bool
	renderPending                     bool
	cache                             map[int]pdfPageResult
	thumbCache                        map[int]paint.ImageOp
	thumbErrors                       map[int]string
	message                           string
	messageRef                        i18n.Message
	tag                               struct{}
	scale                             float64
	pageSizeHintW, pageSizeHintH      float64
	scene                             document.PDFPage
	ink                               []document.PDFPoint
	pointerDown                       bool

	workerMu      sync.Mutex
	workerDone    chan struct{}
	workersClosed bool
}

func (v *pdfViewState) workerSignal() chan struct{} {
	v.workerMu.Lock()
	defer v.workerMu.Unlock()
	if v.workerDone == nil {
		v.workerDone = make(chan struct{})
		if v.workersClosed {
			close(v.workerDone)
		}
	}
	return v.workerDone
}

func (v *pdfViewState) cancelWorkers() {
	v.workerMu.Lock()
	if !v.workersClosed {
		v.workersClosed = true
		if v.workerDone != nil {
			close(v.workerDone)
		}
	}
	v.workerMu.Unlock()
}

func (v *pdfViewState) currentZoom(metric unit.Metric) float64 {
	base := float64(metric.PxPerDp) * (4.0 / 3.0)
	if base > 0 && v.scale > 0 {
		return v.scale / base
	}
	if v.zoom > 0 {
		return v.zoom
	}
	return 1.0
}

func (a *App) ensurePDFView() *pdfViewState {
	if a.pdfView == nil {
		a.pdfView = &pdfViewState{info: a.pdfDoc.Info(), selected: -1, selectStart: -1, selectEnd: -1, zoom: 1, fit: "page", mode: "read", thumbnails: true, buttons: map[string]*widget.Clickable{}, results: make(chan pdfPageResult, 1), cache: map[int]pdfPageResult{}, thumbCache: map[int]paint.ImageOp{}, thumbErrors: map[int]string{}}
		a.pdfView.toolbar.Axis = layout.Horizontal
		a.pdfView.thumbList.Axis = layout.Vertical
		a.pdfView.canvas.Axis = layout.Vertical
		a.pdfView.horizontal.Axis = layout.Horizontal
		a.pdfView.notes.Axis = layout.Vertical
		a.pdfView.search.SingleLine = true
	}
	return a.pdfView
}

func (v *pdfViewState) selectionRange() (int, int) {
	if v.selectStart < 0 && v.selectEnd < 0 {
		if v.selected >= 0 {
			return v.selected, v.selected
		}
		return -1, -1
	}
	s := v.selectStart
	e := v.selectEnd
	if s < 0 {
		s = e
	}
	if e < 0 {
		e = s
	}
	if s > e {
		s, e = e, s
	}
	return s, e
}

func (v *pdfViewState) selectedText() string {
	s, e := v.selectionRange()
	if s < 0 || e < 0 || len(v.scene.Text) == 0 {
		return ""
	}
	s = max(0, min(s, len(v.scene.Text)-1))
	e = max(0, min(e, len(v.scene.Text)-1))
	var parts []string
	for i := s; i <= e; i++ {
		parts = append(parts, v.scene.Text[i].Text)
	}
	return strings.Join(parts, "\n")
}

func (v *pdfViewState) clearSelection() {
	v.selected = -1
	v.selectStart = -1
	v.selectEnd = -1
}

// followPDFThumbnailPage keeps the current body page visible in the thumbnail
// list without overriding a useful position when the page is already visible.
func (v *pdfViewState) followPDFThumbnailPage(page int) bool {
	if v == nil || v.info.Pages <= 0 {
		return false
	}
	page = max(0, min(v.info.Pages-1, page))
	position := &v.thumbList.Position
	visibleCount := position.Count
	if visibleCount <= 0 {
		visibleCount = 1
	}
	if page >= position.First && page < position.First+visibleCount {
		return false
	}
	if page < position.First {
		position.First = page
	} else {
		position.First = page - visibleCount + 1
	}
	position.First = max(0, position.First)
	position.Offset = 0
	position.OffsetLast = 0
	position.BeforeEnd = false
	return true
}

func (v *pdfViewState) syncPageFromCanvas() bool {
	if v == nil {
		return false
	}
	page := v.canvas.Position.First
	if v.pageMap != nil {
		if page < 0 || page >= len(v.pageMap) {
			return false
		}
		page = v.pageMap[page]
	}
	return v.syncPage(page)
}

func (v *pdfViewState) syncPage(page int) bool {
	if v == nil {
		return false
	}
	if page < 0 || page >= max(1, v.info.Pages) || v.page == page {
		return false
	}
	v.page = page
	v.followPDFThumbnailPage(page)
	if r, ok := v.cache[page]; ok {
		v.scene = r.page
		v.pageSizeHintW, v.pageSizeHintH = r.page.Width, r.page.Height
	} else {
		v.scene = document.PDFPage{}
	}
	v.clearSelection()
	v.trimCacheWindow()
	return true
}

func (v *pdfViewState) button(id string) *widget.Clickable {
	if v.buttons[id] == nil {
		v.buttons[id] = new(widget.Clickable)
	}
	return v.buttons[id]
}
func (v *pdfViewState) accept(r pdfPageResult) {
	hasImage := r.hasImage()
	if r.render == pdfRenderThumbnail {
		v.renderPending = false
	} else {
		v.pending = false
		if r.render == pdfRenderPage || hasImage {
			v.renderPending = false
		}
	}
	if r.info.Revision < v.info.Revision || (hasImage && r.page.Revision != r.info.Revision) {
		return
	}
	if r.render == pdfRenderPage && !v.pageInCacheWindow(r.page.Index) {
		return
	}
	if r.render == pdfRenderThumbnail && !v.thumbnailInCacheWindow(r.page.Index, v.thumbList.Position.First) {
		return
	}
	if r.err != nil && r.render != pdfRenderThumbnail {
		v.message = r.err.Error()
		v.messageRef = i18n.Message{}
	}
	if r.info.Revision != v.info.Revision {
		v.cache = map[int]pdfPageResult{}
		v.thumbCache = map[int]paint.ImageOp{}
		v.thumbErrors = map[int]string{}
		v.pageSizeHintW, v.pageSizeHintH = 0, 0
		v.clearSelection()
	}
	v.info = r.info
	if r.page.Index == v.page && r.page.Width > 0 && r.page.Height > 0 {
		v.pageSizeHintW, v.pageSizeHintH = r.page.Width, r.page.Height
	}
	if r.render == pdfRenderThumbnail {
		if r.err != nil {
			v.rememberThumbnailError(r.page.Index, r.err.Error())
		} else if hasImage {
			r.prepareImageOp()
			v.cacheThumbnail(r.page.Index, r.imageOp)
		}
		return
	}
	if r.render == pdfRenderPage || hasImage {
		if hasImage || r.err != nil {
			r.prepareImageOp()
			v.cachePage(r)
		}
		if r.err == nil {
			v.message = r.warning
			v.messageRef = i18n.Message{}
		}
		return
	}
	if r.err != nil {
		return
	}
	if r.notice.ID != "" {
		v.message = ""
		v.messageRef = r.notice
		return
	}
	if r.mutation {
		v.message = ""
		v.messageRef = i18n.Message{ID: "pdf.status.change_applied"}
		return
	}
	if len(r.search) > 0 {
		v.page = r.search[0]
		v.canvas.Position.First = v.page
		v.canvas.Position.Offset = 0
		v.canvas.Position.OffsetLast = 0
		v.canvas.Position.BeforeEnd = false
		v.followPDFThumbnailPage(v.page)
		v.trimCacheWindow()
		v.clearSelection()
		v.message = ""
		v.messageRef = i18n.Message{ID: "pdf.status.search_result_located"}
		return
	}
	v.message = r.warning
	v.messageRef = i18n.Message{}
}

func (a *App) requestPDFPage(index int, dpi float64) {
	v := a.ensurePDFView()
	if v.pending || v.renderPending || index < 0 || index >= v.info.Pages || !v.pageInCacheWindow(index) {
		return
	}
	if cached, ok := v.cache[index]; ok {
		if cached.err != nil || cached.dpi >= dpi {
			return
		}
	}
	v.pending = true
	v.renderPending = true
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	done := v.workerSignal()
	visualOnly := v.visualOnly
	a.launchManagedBackground(func() {
		var pixels image.Image
		var p document.PDFPage
		var w string
		var e error
		if visualOnly {
			pixels, p, e = d.RenderVisualPage(index, dpi)
		} else {
			pixels, p, w, e = d.Render(index, dpi)
		}
		p.Index = index
		r := pdfPageResult{page: p, warning: w, err: e, info: d.Info(), render: pdfRenderPage, dpi: dpi}
		r.setImage(pixels)
		select {
		case v.results <- r:
			if window != nil {
				window.Invalidate()
			}
		case <-done:
		case <-shutdown:
		}
	})
}

func (a *App) requestPDFThumbnail(index int) {
	v := a.ensurePDFView()
	if v.pending || v.renderPending || index < 0 || index >= v.info.Pages || !v.thumbnailInCacheWindow(index, v.thumbList.Position.First) || v.thumbCache[index].Size().X > 0 || v.thumbErrors[index] != "" {
		return
	}
	v.renderPending = true
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	done := v.workerSignal()
	a.launchManagedBackground(func() {
		pixels, p, err := d.RenderThumbnail(index, pdfThumbnailDPI)
		p.Index = index
		r := pdfPageResult{page: p, err: err, info: d.Info(), render: pdfRenderThumbnail, dpi: pdfThumbnailDPI}
		r.setImage(pixels)
		select {
		case v.results <- r:
			if window != nil {
				window.Invalidate()
			}
		case <-done:
		case <-shutdown:
		}
	})
}

func (v *pdfViewState) syncInfo(info document.PDFInfo) {
	if info.Revision != v.info.Revision {
		v.cache = map[int]pdfPageResult{}
		v.thumbCache = map[int]paint.ImageOp{}
		v.thumbErrors = map[int]string{}
		v.pageSizeHintW, v.pageSizeHintH = 0, 0
		v.clearSelection()
		v.scene = document.PDFPage{}
	}
	v.info = info
	v.trimCacheWindow()
	v.trimThumbnailCacheWindow()
}
