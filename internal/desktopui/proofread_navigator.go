package desktopui

import (
	"errors"
	"fmt"
	"github.com/go-text/typesetting/segmenter"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/application"
	"glance/internal/proofreading"
)

// Wheel distance is accumulated in logical dp, independent of paper zoom and
// display density. Fine input never accelerates or converts distance to lines.
const proofreadingNavigatorStepDP = 16

type proofreadingNavigatorIntent struct {
	steps int
	lines bool
}
type proofreadingNavigator struct {
	tag                          struct{}
	initialized                  bool
	page                         int
	remainder                    float64
	coarse                       bool
	hover                        bool
	hoverY                       int
	pending                      []proofreadingNavigatorIntent
	feedback                     int
	feedbackKey                  string
	sample                       string
	feedbackAt                   time.Time
	reveal                       bool
	highlightStart, highlightEnd int
}

func (a *App) layoutProofreadingXNavigatorCanvas(gtx layout.Context, view *pdfViewState, pages int, status string, isError bool) layout.Dimensions {
	s := a.proofreading
	n := &s.navigator
	// The source reading line is one third down the viewport. A remains the
	// authority even while X text and its local viewport change.
	target := max(0, min(pages-1, view.canvas.Position.First))
	top := -view.canvas.Position.Offset
	line := gtx.Constraints.Max.Y / 3
	for target+1 < pages {
		size := proofreadingXPagePixelSize(gtx, view, s, target)
		bottom := top + size.Y + 2*gtx.Dp(12)
		if bottom > line {
			break
		}
		top = bottom
		target++
	}
	if !n.initialized || n.page != target {
		*n = proofreadingNavigator{tag: n.tag, initialized: true, page: target, feedbackKey: "proofread.navigator.ready"}
		gtx.Execute(op.InvalidateCmd{})
	}
	a.processProofreadingNavigatorEvents(gtx)
	s.xPageAdjustmentPage = target
	s.xPageAdjustmentActive = target < len(s.pageOwnership) && !s.pageOwnership[target].Reviewed
	a.revealProofreadingNavigatorBoundary()
	// Use identical outer geometry for A and X; only X reserves the right strip.
	s.xCanvas.Axis = layout.Vertical
	s.xCanvas.Position = view.canvas.Position
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutProofreadingXPageList(gtx, view, pages, status, isError)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(54), gtx.Constraints.Max.Y))
			return a.layoutProofreadingNavigator(gtx, pages)
		}),
	)
}

func (a *App) processProofreadingNavigatorEvents(gtx layout.Context) {
	s := a.proofreading
	n := &s.navigator
	filter := pointer.Filter{Target: &n.tag, Kinds: pointer.Scroll | pointer.Enter | pointer.Leave | pointer.Move, ScrollY: pointer.ScrollRange{Min: -(1 << 20), Max: 1 << 20}, ScrollX: pointer.ScrollRange{Min: -(1 << 20), Max: 1 << 20}}
	for {
		ev, ok := gtx.Event(filter)
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Enter, pointer.Move:
			n.hover = true
			n.hoverY = int(e.Position.Y)
		case pointer.Leave:
			n.hover = false
			n.remainder = 0
		case pointer.Scroll:
			if s.controller == nil || !s.modelInitialAssigned || a.proofreadingEditorComposing() {
				n.remainder = 0
				continue
			}
			if n.page >= len(s.pageOwnership) || s.pageOwnership[n.page].Reviewed {
				n.feedbackKey = "proofread.navigator.reviewed"
				n.remainder = 0
				continue
			}
			coarse := e.Modifiers.Contain(key.ModShift)
			if coarse != n.coarse {
				n.remainder = 0
				n.coarse = coarse
			}
			delta := float64(e.Scroll.Y)
			// Some platforms turn Shift+wheel into horizontal deltas.
			if coarse && delta == 0 {
				delta = float64(e.Scroll.X)
			}
			delta /= float64(max(float32(0.01), gtx.Metric.PxPerDp))
			if math.IsNaN(delta) || math.IsInf(delta, 0) {
				continue
			}
			if delta*n.remainder < 0 {
				n.remainder = 0
			}
			n.remainder += delta
			steps := int(n.remainder / proofreadingNavigatorStepDP)
			n.remainder -= float64(steps * proofreadingNavigatorStepDP)
			if steps != 0 {
				steps = max(-256, min(256, steps))
				step := 1
				if steps < 0 {
					step = -1
					steps = -steps
				}
				for range steps {
					n.pending = append(n.pending, proofreadingNavigatorIntent{steps: step, lines: coarse})
				}
			}
		}
	}
	if a.proofreadingEditorComposing() {
		n.pending = nil
		return
	}
	changedModel := false
	started := time.Now()
	defer func() {
		if changedModel {
			a.proofreadingBoundaryChanged(n.page)
			gtx.Execute(op.InvalidateCmd{})
		}
	}()
	for processed := 0; len(n.pending) > 0; processed++ {
		if processed == 4 || processed > 0 && time.Since(started) > 6*time.Millisecond {
			gtx.Execute(op.InvalidateCmd{})
			break
		}
		intent := n.pending[0]
		if intent.lines && (s.controller.Layout.StyleRevision != s.xTextSizeStep || s.modelLayoutPending) {
			return
		}
		before := s.controller.Model.Text(proofreading.PageID(n.page + 1))
		moved, err := s.controller.AdjustPageUnits(proofreading.PageID(n.page+1), intent.steps, intent.lines)
		if errors.Is(err, application.ErrProofreadingLayoutPending) {
			if !s.modelLayoutPending {
				a.startProofreadingModelLayoutForPages(false, []proofreading.PageID{proofreading.PageID(n.page + 1)})
			}
			return
		}
		n.pending = n.pending[1:]
		if err != nil {
			n.feedbackKey = "proofread.navigator.reviewed"
			continue
		}
		if moved == 0 {
			n.feedbackKey = "proofread.navigator.boundary"
			n.remainder = 0
			continue
		}
		after := s.controller.Model.Text(proofreading.PageID(n.page + 1))
		changed := strings.TrimPrefix(after, before)
		if moved < 0 {
			changed = strings.TrimPrefix(before, after)
		}
		var changedSegments segmenter.Segmenter
		changedSegments.Init([]rune(changed))
		changedIt := changedSegments.GraphemeIterator()
		for changedIt.Next() {
			n.sample = string(changedIt.Grapheme().Text)
		}
		n.sample = strings.ReplaceAll(strings.ReplaceAll(n.sample, "\n", "↵"), " ", "␣")
		if gtx.Now.Sub(n.feedbackAt) > 500*time.Millisecond || n.feedback*moved < 0 {
			n.feedback = 0
		}
		n.feedback += moved
		n.feedbackAt = gtx.Now
		n.feedbackKey = ""
		n.reveal = true
		var sg segmenter.Segmenter
		sg.Init([]rune(after))
		it := sg.GraphemeIterator()
		n.highlightStart, n.highlightEnd = 0, 0
		for it.Next() {
			g := it.Grapheme()
			n.highlightStart = g.Offset
			n.highlightEnd = g.Offset + len(g.Text)
		}
		changedModel = true
	}
}

func (a *App) revealProofreadingNavigatorBoundary() {
	s := a.proofreading
	n := &s.navigator
	if !n.reveal || n.page < 0 || n.page >= len(s.xPages) || s.controller.Layout.Revision != s.controller.Model.Revision() {
		return
	}
	p := s.xPages[n.page]
	shaped, ok := s.controller.Layout.Pages[proofreading.PageID(n.page+1)]
	if !ok {
		return
	}
	_, margin := proofreadingTextMargins(p.Width, p.Height)
	s.xPageScrollOffsets[n.page] = max(0, proofreadingPageContentHeight(shaped, p.Content)-max(1, p.Height-2*margin))
	n.reveal = false
}

func (a *App) layoutProofreadingNavigator(gtx layout.Context, pages int) layout.Dimensions {
	s := a.proofreading
	n := &s.navigator
	size := gtx.Constraints.Max
	bg := color.NRGBA{R: 244, G: 246, B: 248, A: 255}
	if n.hover {
		bg = color.NRGBA{R: 234, G: 239, B: 244, A: 255}
	}
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
	bounds := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &n.tag)
	pointer.CursorNorthSouthResize.Add(gtx.Ops)
	semantic.DescriptionOp(fmt.Sprintf(a.textID("proofread.navigator.description"), n.page+1)).Add(gtx.Ops)
	label := func(y int, value string, ink color.NRGBA) {
		offset := op.Offset(image.Pt(gtx.Dp(3), y)).Push(gtx.Ops)
		child := gtx
		child.Constraints = layout.Constraints{Max: image.Pt(max(1, size.X-gtx.Dp(6)), gtx.Dp(38))}
		child.Constraints.Min.X = child.Constraints.Max.X
		l := material.Label(a.theme, unit.Sp(9), value)
		l.Color = ink
		l.MaxLines = 2
		layout.Center.Layout(child, l.Layout)
		offset.Pop()
	}
	label(gtx.Dp(10), fmt.Sprintf(a.textID("proofread.navigator.page"), n.page+1), proofreadMuted)
	label(gtx.Dp(34), a.textID("proofread.navigator.wheel"), proofreadMuted)
	trackTop, trackBottom := gtx.Dp(86), size.Y-gtx.Dp(112)
	if trackBottom > trackTop {
		total, owned := 0, 0
		if len(s.pageOwnership) > 0 {
			total = s.pageOwnership[len(s.pageOwnership)-1].End
			if n.page < len(s.pageOwnership) {
				owned = s.pageOwnership[n.page].End
			}
		}
		marker := trackTop + int(float64(trackBottom-trackTop)*float64(owned)/float64(max(1, total)))
		for y := trackTop; y <= trackBottom; y += gtx.Dp(19) {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 202, G: 208, B: 213, A: 255}, clip.Rect(image.Rect(gtx.Dp(27), y, gtx.Dp(43), y+gtx.Dp(3))).Op())
		}
		paint.FillShape(gtx.Ops, proofreadBlue, clip.Rect(image.Rect(gtx.Dp(17), marker, gtx.Dp(43), marker+gtx.Dp(3))).Op())
		if n.hover {
			y := max(trackTop, min(trackBottom, n.hoverY))
			paint.FillShape(gtx.Ops, proofreadMuted, clip.Rect(image.Rect(gtx.Dp(23), y, gtx.Dp(43), y+gtx.Dp(1))).Op())
		}
	}
	message := a.textID(n.feedbackKey)
	if n.feedbackKey == "" {
		message = fmt.Sprintf("%+d", n.feedback) + a.textID("proofread.navigator.unit")
	}
	if n.page < len(s.pageOwnership) && s.pageOwnership[n.page].Reviewed {
		message = a.textID("proofread.navigator.reviewed")
	}
	label(size.Y-gtx.Dp(100), message, proofreadBlue)
	label(size.Y-gtx.Dp(76), n.sample, proofreadInk)
	label(size.Y-gtx.Dp(50), a.textID("proofread.navigator.shift"), proofreadMuted)
	label(size.Y-gtx.Dp(22), fmt.Sprintf("%02d / %02d", n.page+1, pages), proofreadMuted)
	bounds.Pop()
	return layout.Dimensions{Size: size}
}
