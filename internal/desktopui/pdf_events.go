package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/i18n"
)

func pdfPoint(x, y float64) f32.Point { return f32.Pt(float32(x), float32(y)) }

type handlePDFEventsSetToolArguments struct {
	Operation string `json:"operation"`
	Tool      string `json:"tool"`
}

func (a *App) handlePDFEvents(gtx layout.Context) {
	v := a.ensurePDFView()
	v.syncInfo(a.pdfDoc.Info())
	select {
	case r := <-v.results:
		v.accept(r)
		if r.mutation && r.err != nil && (v.lastCommand.Kind == "replace" || v.lastCommand.Kind == "note") {
			c := v.lastCommand
			a.showPDFDraft(c)
			a.setStatusErrorMessage(i18n.Message{ID: "pdf.error.operation_failed"}, r.err, notificationToast)
		}
		if r.mutation && r.err == nil {
			a.persistActiveSession()
		}
	default:
	}
	if !a.modal.Visible {
		for {
			if _, ok := v.search.Update(gtx); !ok {
				break
			}
		}
		for id, b := range v.buttons {
			if b.Clicked(gtx) {
				a.handlePDFAction(gtx, id)
			}
		}
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: &v.tag, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
			if !ok {
				break
			}
			e := ev.(pointer.Event)
			if v.pending || v.scale <= 0 || v.scene.Index != v.page || v.scene.Revision != v.info.Revision {
				continue
			}
			x, y := float64(e.Position.X)/v.scale, float64(e.Position.Y)/v.scale
			x = max(0, min(v.scene.Width, x))
			y = max(0, min(v.scene.Height, y))
			switch e.Kind {
			case pointer.Press:
				v.pointerDown = true
				if v.tool == "ink" {
					v.ink = []document.PDFPoint{{X: x, Y: y}}
					continue
				}
				hit := pdfHit(v.scene, x, y)
				v.selected = hit
				v.selectStart = hit
				v.selectEnd = hit
				if v.tool == "highlight" && v.selected >= 0 {
					a.handlePDFAction(gtx, "highlight")
					a.runPDFViewAction("clear annotation tool", handlePDFEventsSetToolArguments{Operation: "set_tool", Tool: ""})
				} else if v.mode == "edit" && v.selected >= 0 {
					a.pdfTextDialog("replace")
				}
			case pointer.Drag:
				if v.pointerDown && v.tool == "ink" && len(v.ink) < 10000 {
					v.ink = append(v.ink, document.PDFPoint{X: x, Y: y})
				} else if v.pointerDown && v.tool != "ink" {
					hit := pdfHitOrNearest(v.scene, x, y)
					if hit >= 0 {
						if v.selectStart < 0 {
							v.selectStart = hit
						}
						v.selectEnd = hit
						v.selected = hit
					}
				}
			case pointer.Release:
				if v.pointerDown && len(v.ink) > 1 {
					a.startPDFCommand(document.PDFCommand{Kind: "ink", Page: v.page, Revision: v.scene.Revision, Points: append([]document.PDFPoint(nil), v.ink...), X: v.ink[0].X, Y: v.ink[0].Y})
				}
				v.pointerDown = false
				v.ink = nil
			case pointer.Cancel:
				v.pointerDown = false
				v.ink = nil
			}
		}
	}
	if a.modal.Visible {
		return
	}
	if v.scale > 0 {
		a.requestPDFPage(v.page, pdfCanvasDPI(v.scale))
	}
	// Keep the independently scrolling thumbnail strip ahead of lower-priority
	// canvas prefetches. The current page request above always gets first access
	// to the shared render slot.
	if v.thumbnails && !v.pending && !v.renderPending {
		first := max(0, v.thumbList.Position.First)
		visibleCount := v.thumbnailPrefetchCount()
		for i := first; i < min(v.info.Pages, first+visibleCount); i++ {
			if !v.thumbnailInCacheWindow(i, first) {
				continue
			}
			if v.thumbCache[i].Size().X == 0 && v.thumbErrors[i] == "" {
				a.requestPDFThumbnail(i)
				break
			}
		}
	}
	// Prefetch nearby canvas pages after the current page and visible thumbnails.
	if v.scale > 0 && !v.pending && !v.renderPending {
		first := max(0, v.canvas.Position.First)
		dpi := pdfCanvasDPI(v.scale)
		if v.pageMap != nil {
			for row := first; row < min(len(v.pageMap), first+2); row++ {
				page := v.pageMap[row]
				if !v.pageInCacheWindow(page) {
					continue
				}
				if !v.canPrefetchPage(page, dpi) {
					break
				}
				cached, ok := v.cache[page]
				if !ok || (cached.err == nil && cached.dpi < dpi) {
					a.requestPDFPage(page, dpi)
					break
				}
			}
		} else {
			for page := first; page < min(v.info.Pages, first+2); page++ {
				if !v.pageInCacheWindow(page) || !v.canPrefetchPage(page, dpi) {
					continue
				}
				cached, ok := v.cache[page]
				if !ok || (cached.err == nil && cached.dpi < dpi) {
					a.requestPDFPage(page, dpi)
					break
				}
			}
		}
	}
}
