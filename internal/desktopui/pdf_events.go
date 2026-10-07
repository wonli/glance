package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"glance/internal/document"
)

func pdfPoint(x, y float64) f32.Point { return f32.Pt(float32(x), float32(y)) }
func (a *App) handlePDFEvents(gtx layout.Context) {
	v := a.ensurePDFView()
	v.syncInfo(a.pdfDoc.Info())
	select {
	case r := <-v.results:
		v.accept(r)
		if r.mutation && r.err != nil && (v.lastCommand.Kind == "replace" || v.lastCommand.Kind == "note") {
			c := v.lastCommand
			a.showPDFDraft(c, r.err.Error())
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
				v.selected = pdfHit(v.scene, x, y)
				if v.tool == "highlight" && v.selected >= 0 {
					a.handlePDFAction(gtx, "highlight")
					v.tool = ""
				} else if v.mode == "edit" && v.selected >= 0 {
					a.pdfTextDialog("replace")
				}
			case pointer.Drag:
				if v.pointerDown && v.tool == "ink" && len(v.ink) < 10000 {
					v.ink = append(v.ink, document.PDFPoint{X: x, Y: y})
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
	a.requestPDFPage(v.page)
	// Prefetch only the visible thumbnail strip. All requests share one worker slot.
	if v.thumbnails && !v.pending {
		for i := max(0, v.thumbList.Position.First); i < min(v.info.Pages, v.thumbList.Position.First+5); i++ {
			if v.thumbCache[i] == nil {
				a.requestPDFPage(i)
				break
			}
		}
	}
}
