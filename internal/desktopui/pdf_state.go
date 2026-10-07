package desktopui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/document"
	"image"
	"strings"
	"sync"
)

type pdfPageResult struct {
	page     document.PDFPage
	pixels   image.Image
	warning  string
	err      error
	info     document.PDFInfo
	mutation bool
	notice   string
	search   []int
}
type pdfViewState struct {
	lastCommand                       document.PDFCommand
	info                              document.PDFInfo
	page, selected                    int
	selectStart, selectEnd            int
	zoom                              float64
	fit                               string
	mode, tool                        string
	thumbnails, showSearch            bool
	toolbar, thumbList, canvas, notes widget.List
	horizontal                        widget.List
	buttons                           map[string]*widget.Clickable
	search                            widget.Editor
	results                           chan pdfPageResult
	pending                           bool
	cache                             map[int]pdfPageResult
	thumbCache                        map[int]image.Image
	message                           string
	tag                               struct{}
	scale                             float64
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
		a.pdfView = &pdfViewState{info: a.pdfDoc.Info(), selected: -1, selectStart: -1, selectEnd: -1, zoom: 1, fit: "page", mode: "read", thumbnails: true, buttons: map[string]*widget.Clickable{}, results: make(chan pdfPageResult, 1), cache: map[int]pdfPageResult{}, thumbCache: map[int]image.Image{}}
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
func (v *pdfViewState) button(id string) *widget.Clickable {
	if v.buttons[id] == nil {
		v.buttons[id] = new(widget.Clickable)
	}
	return v.buttons[id]
}
func (v *pdfViewState) accept(r pdfPageResult) {
	v.pending = false
	if r.info.Revision < v.info.Revision || (r.pixels != nil && r.page.Revision != r.info.Revision) {
		return
	}
	if r.err != nil {
		v.message = r.err.Error()
		if !r.mutation && r.notice == "" {
			v.cache[r.page.Index] = r
		}
		return
	}
	if r.info.Revision != v.info.Revision {
		v.cache = map[int]pdfPageResult{}
		v.thumbCache = map[int]image.Image{}
		v.clearSelection()
	}
	v.info = r.info
	if r.notice != "" {
		v.message = r.notice
		return
	}
	if r.mutation {
		v.message = "修改已应用"
		return
	}
	if len(r.search) > 0 {
		v.page = r.search[0]
		v.clearSelection()
		v.message = "已定位搜索结果"
		return
	}
	if r.pixels != nil {
		if len(v.cache) >= 12 {
			for k := range v.cache {
				if k != v.page {
					delete(v.cache, k)
					break
				}
			}
		}
		v.cache[r.page.Index] = r
		v.thumbCache[r.page.Index] = r.pixels
		if len(v.thumbCache) > 16 {
			for k := range v.thumbCache {
				if k != v.page && k != r.page.Index {
					delete(v.thumbCache, k)
					break
				}
			}
		}
	}
	v.message = r.warning
}
func (a *App) requestPDFPage(index int) {
	v := a.ensurePDFView()
	if v.pending || index < 0 || index >= v.info.Pages {
		return
	}
	if _, ok := v.cache[index]; ok {
		return
	}
	v.pending = true
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	done := v.workerSignal()
	go func() {
		pixels, p, w, e := d.Render(index, 144)
		p.Index = index
		r := pdfPageResult{page: p, pixels: pixels, warning: w, err: e, info: d.Info()}
		select {
		case v.results <- r:
			if window != nil {
				window.Invalidate()
			}
		case <-done:
		case <-shutdown:
		}
	}()
}

func (v *pdfViewState) syncInfo(info document.PDFInfo) {
	if info.Revision != v.info.Revision {
		v.cache = map[int]pdfPageResult{}
		v.thumbCache = map[int]image.Image{}
		v.clearSelection()
		v.scene = document.PDFPage{}
	}
	v.info = info
}
