package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
	"glance/internal/proofreading"
)

func proofreadingOCRStepConnectorABlock(step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) (int, proofreadingOCRDiffOpKind, bool) {
	frame, ok := proofreadingOCRCurrentDiffFrame(step, comparison)
	if !ok || frame.ABlockID < 0 {
		return -1, 0, false
	}
	return frame.ABlockID, frame.Kind, true
}

type proofreadingOCRConnection struct {
	from, to  image.Point
	kind      proofreadingOCRDiffOpKind
	active    bool
	frame     proofreadingOCRDiffFrame
	allFrames []proofreadingOCRDiffFrame
}

type proofreadingCenterBadge struct {
	index       int
	orderNumber int
	y           int
	rect        image.Rectangle
	active      bool
	kind        proofreadingOCRDiffOpKind
	resolved    bool
	frame       proofreadingOCRDiffFrame
}

type proofreadingCenterBadgeAnchor struct {
	connectionIndex int
	orderNumber     int
	pageFraction    float64
	kind            proofreadingOCRDiffOpKind
	frame           proofreadingOCRDiffFrame
}

func layoutProofreadingCenterBadges(gtx layout.Context, connections []proofreadingOCRConnection, divider int, resolved map[int]bool) []proofreadingCenterBadge {
	return layoutProofreadingCenterBadgesWithin(gtx, connections, divider, resolved, gtx.Dp(52), gtx.Constraints.Max.Y-gtx.Dp(24))
}

func layoutProofreadingCenterBadgesWithin(gtx layout.Context, connections []proofreadingOCRConnection, divider int, resolved map[int]bool, topLimit, botLimit int) []proofreadingCenterBadge {
	if len(connections) == 0 {
		return nil
	}
	type item struct {
		index  int
		idealY int
		active bool
		kind   proofreadingOCRDiffOpKind
	}
	items := make([]item, len(connections))
	for i, c := range connections {
		items[i] = item{
			index:  i,
			idealY: (c.from.Y + c.to.Y) / 2,
			active: c.active,
			kind:   c.kind,
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].idealY < items[j].idealY
	})

	badgeW := max(gtx.Dp(20), 20)
	badgeH := max(gtx.Dp(20), 20)
	minGap := badgeH + gtx.Dp(4)
	botLimit = max(topLimit, botLimit)
	if len(items) > 1 {
		minGap = min(minGap, (botLimit-topLimit)/(len(items)-1))
	} else {
		minGap = 0
	}

	ys := make([]int, len(items))
	for i, it := range items {
		y := max(topLimit, min(botLimit, it.idealY))
		if i > 0 && y < ys[i-1]+minGap {
			y = ys[i-1] + minGap
		}
		ys[i] = y
	}
	for i := len(ys) - 1; i >= 0; i-- {
		maxAllowed := botLimit - (len(ys)-1-i)*minGap
		if ys[i] > maxAllowed {
			ys[i] = maxAllowed
		}
		if i < len(ys)-1 && ys[i] > ys[i+1]-minGap {
			ys[i] = ys[i+1] - minGap
		}
	}
	for i := range ys {
		if i > 0 && ys[i] < ys[i-1]+minGap {
			ys[i] = ys[i-1] + minGap
		}
	}

	badges := make([]proofreadingCenterBadge, len(connections))
	for i, it := range items {
		y := ys[i]
		w := badgeW
		if i+1 >= 100 {
			w = max(gtx.Dp(26), 26)
		}
		rect := image.Rect(divider-w/2, y-badgeH/2, divider+w/2, y+badgeH/2)
		badges[it.index] = proofreadingCenterBadge{
			index:       it.index,
			orderNumber: i + 1,
			y:           y,
			rect:        rect,
			active:      it.active,
			kind:        it.kind,
			resolved:    resolved != nil && resolved[it.index],
			frame:       connections[it.index].frame,
		}
	}
	return badges
}

func proofreadingCenterBadgeAnchors(badges []proofreadingCenterBadge, top, bottom int) []proofreadingCenterBadgeAnchor {
	if len(badges) == 0 {
		return nil
	}
	anchors := make([]proofreadingCenterBadgeAnchor, len(badges))
	for index, badge := range badges {
		anchors[index] = proofreadingCenterBadgeAnchor{
			connectionIndex: badge.index,
			orderNumber:     badge.orderNumber,
			pageFraction:    float64(badge.y-top) / float64(max(1, bottom-top)),
			kind:            badge.kind,
			frame:           badge.frame,
		}
	}
	return anchors
}

func layoutProofreadingCenterBadgesAtAnchors(gtx layout.Context, anchors []proofreadingCenterBadgeAnchor, divider, top, bottom int, resolved map[int]bool, activeFrame *proofreadingOCRDiffFrame) []proofreadingCenterBadge {
	if len(anchors) == 0 {
		return nil
	}
	badges := make([]proofreadingCenterBadge, len(anchors))
	width := max(gtx.Dp(20), 20)
	height := max(gtx.Dp(20), 20)
	for index, anchor := range anchors {
		badgeWidth := width
		if anchor.orderNumber >= 100 {
			badgeWidth = max(gtx.Dp(26), 26)
		}
		center := image.Pt(divider, top+int(math.Round(anchor.pageFraction*float64(bottom-top))))
		badges[index] = proofreadingCenterBadge{
			index:       anchor.connectionIndex,
			orderNumber: anchor.orderNumber,
			y:           center.Y,
			rect:        image.Rect(center.X-badgeWidth/2, center.Y-height/2, center.X+badgeWidth/2, center.Y+height/2),
			active:      activeFrame != nil && anchor.frame == *activeFrame,
			kind:        anchor.kind,
			resolved:    resolved != nil && resolved[anchor.connectionIndex],
			frame:       anchor.frame,
		}
	}
	return badges
}

func proofreadingOCRStepPairDivider(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, pair proofreadingOCRStepPair) (int, bool) {
	aOrigin, bOrigin, ok := proofreadingOCRStepConnectorPageOrigins(gtx, view, state, pair.APage, pair.BPage)
	if !ok {
		return 0, false
	}
	aSize := proofreadingOCRStepSourcePagePixelSize(view, pair.APage)
	bSize := proofreadingXPagePixelSize(gtx, view, state, pair.BPage)
	divider := proofreadingComparePaneSplit(gtx, state != nil && state.bDocument != nil)
	if aSize.X > 0 && bSize.X > 0 {
		aRight := aOrigin.X + aSize.X
		bLeft := bOrigin.X
		if bLeft > aRight {
			divider = (aRight + bLeft) / 2
		}
	}
	return divider, true
}

func proofreadingOCRStepBadgeVerticalBounds(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, pair proofreadingOCRStepPair) (int, int, bool) {
	aOrigin, bOrigin, ok := proofreadingOCRStepConnectorPageOrigins(gtx, view, state, pair.APage, pair.BPage)
	if !ok {
		return 0, 0, false
	}
	aSize := proofreadingOCRStepSourcePagePixelSize(view, pair.APage)
	bSize := proofreadingXPagePixelSize(gtx, view, state, pair.BPage)
	if aSize.Y <= 0 || bSize.Y <= 0 {
		return 0, 0, false
	}
	halfBadge := max(gtx.Dp(20), 20) / 2
	top := max(aOrigin.Y, bOrigin.Y) + halfBadge
	bottom := min(aOrigin.Y+aSize.Y, bOrigin.Y+bSize.Y) - halfBadge
	if bottom < top {
		return 0, 0, false
	}
	return top, bottom, true
}

func proofreadingOCRVisiblePages(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, position layout.Position, xCanvas bool) map[int]bool {
	visible := make(map[int]bool)
	if view == nil || view.info.Pages <= 0 {
		return visible
	}
	rowCount := view.info.Pages
	if view.pageMap != nil {
		rowCount = len(view.pageMap)
	}
	rowTop := -position.Offset
	for row := max(0, position.First); row < rowCount; row++ {
		pageIndex, ok := view.pageForCanvasRow(row)
		if !ok {
			continue
		}
		var size image.Point
		if xCanvas {
			size = proofreadingXPagePixelSize(gtx, view, state, pageIndex)
		} else {
			size = proofreadingOCRStepSourcePagePixelSize(view, pageIndex)
		}
		pageTop := gtx.Dp(44) + rowTop + gtx.Dp(12)
		if pageTop >= gtx.Constraints.Max.Y {
			break
		}
		if pageTop+size.Y > gtx.Dp(44) {
			visible[pageIndex] = true
		}
		rowTop += size.Y + 2*gtx.Dp(12)
	}
	return visible
}

func (a *App) drawProofreadingCenterBadge(gtx layout.Context, badge proofreadingCenterBadge) {
	bg := color.NRGBA{R: 255, G: 255, B: 255, A: 252}
	stroke := color.NRGBA{R: 217, G: 48, B: 37, A: 255}
	textInk := stroke
	statusID := "proofread.ai.step.badge.pending"
	if badge.resolved {
		stroke = color.NRGBA{R: 46, G: 160, B: 67, A: 255}
		textInk = stroke
		bg = color.NRGBA{R: 240, G: 249, B: 242, A: 252}
		statusID = "proofread.ai.step.badge.resolved"
	} else if badge.active {
		stroke = color.NRGBA{R: 26, G: 115, B: 232, A: 255}
		bg = stroke
		textInk = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		statusID = "proofread.ai.step.badge.processing"
	}
	semantic.DescriptionOp(a.textArgs(statusID, i18n.Args{"Index": fmt.Sprint(badge.orderNumber)})).Add(gtx.Ops)
	r := gtx.Dp(3)
	paint.FillShape(gtx.Ops, bg, clip.UniformRRect(badge.rect, r).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, stroke, clip.Stroke{Path: clip.UniformRRect(badge.rect, r).Path(gtx.Ops), Width: float32(max(1, gtx.Dp(1.5)))}.Op())

	macro := op.Record(gtx.Ops)
	lblText := fmt.Sprint(badge.orderNumber)
	lbl := material.Label(a.theme, 9, lblText)
	lbl.Color = textInk
	lbl.Font.Weight = font.Bold
	labelGtx := gtx
	labelGtx.Constraints.Min = image.Point{}
	dims := lbl.Layout(labelGtx)
	call := macro.Stop()

	offsetX := badge.rect.Min.X + (badge.rect.Dx()-dims.Size.X)/2
	offsetY := badge.rect.Min.Y + (badge.rect.Dy()-dims.Size.Y)/2
	trans := op.Offset(image.Pt(offsetX, offsetY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	trans.Pop()
}

func (a *App) drawProofreadingNoDiffBadge(gtx layout.Context, comparison proofreadingOCRStepComparison) {
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	divider, located := proofreadingOCRStepPairDivider(gtx, a.pdfView, a.proofreading, pair)
	top, bottom, boundsLocated := proofreadingOCRStepBadgeVerticalBounds(gtx, a.pdfView, a.proofreading, pair)
	if !located || !boundsLocated {
		return
	}
	centerY := (top + bottom) / 2

	lblText := "✓ 无差异"
	lbl := material.Label(a.theme, 11, lblText)
	greenColor := color.NRGBA{R: 46, G: 160, B: 67, A: 255}
	lbl.Color = greenColor
	lbl.Font.Weight = font.Bold

	macro := op.Record(gtx.Ops)
	labelGtx := gtx
	labelGtx.Constraints.Min = image.Point{}
	dims := lbl.Layout(labelGtx)
	call := macro.Stop()

	padX := gtx.Dp(12)
	padY := gtx.Dp(5)
	w := dims.Size.X + padX*2
	h := max(dims.Size.Y+padY*2, gtx.Dp(26))
	rect := image.Rect(divider-w/2, centerY-h/2, divider+w/2, centerY+h/2)

	r := gtx.Dp(13)
	bg := color.NRGBA{R: 240, G: 249, B: 242, A: 252}
	paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, r).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, greenColor, clip.Stroke{
		Path:  clip.UniformRRect(rect, r).Path(gtx.Ops),
		Width: float32(max(1, gtx.Dp(1.5))),
	}.Op())

	offsetX := rect.Min.X + (rect.Dx()-dims.Size.X)/2
	offsetY := rect.Min.Y + (rect.Dy()-dims.Size.Y)/2
	trans := op.Offset(image.Pt(offsetX, offsetY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	trans.Pop()
}

func (a *App) drawProofreadingOCRStepConnector(gtx layout.Context) {
	if a == nil || a.proofreading == nil || a.pdfView == nil || a.proofreading.xPageMenuOpen {
		return
	}
	state := a.proofreading
	step := state.ocrStep
	if step == nil {
		return
	}
	viewport := clip.Rect{Min: image.Pt(0, gtx.Dp(44)), Max: gtx.Constraints.Max}.Push(gtx.Ops)
	defer viewport.Pop()
	pair := proofreadingOCRStepPair{APage: max(0, min(a.pdfView.info.Pages-1, a.pdfView.page)), BPage: a.proofreadingCurrentBPage()}
	if step.badgeAnchors == nil {
		step.badgeAnchors = make(map[proofreadingOCRStepPair][]proofreadingCenterBadgeAnchor)
	}
	if step.badgeConnections == nil {
		step.badgeConnections = make(map[proofreadingOCRStepPair][]proofreadingOCRConnection)
	}
	if step.badgeClicksByPair == nil {
		step.badgeClicksByPair = make(map[proofreadingOCRStepPair][]widget.Clickable)
	}

	pairs := make([]proofreadingOCRStepPair, 0, len(step.comparisons))
	for candidate := range step.comparisons {
		pairs = append(pairs, candidate)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].APage != pairs[j].APage {
			return pairs[i].APage < pairs[j].APage
		}
		return pairs[i].BPage < pairs[j].BPage
	})
	visibleAPages := proofreadingOCRVisiblePages(gtx, a.pdfView, state, a.pdfView.canvas.Position, false)
	visibleBPages := proofreadingOCRVisiblePages(gtx, a.pdfView, state, state.xCanvas.Position, true)

	badgesByPair := make(map[proofreadingOCRStepPair][]proofreadingCenterBadge, len(pairs))
	connectionsByPair := make(map[proofreadingOCRStepPair][]proofreadingOCRConnection, len(pairs))
	for _, candidate := range pairs {
		comparison := step.comparisons[candidate]
		if candidate.BPage >= 0 && candidate.BPage < len(state.xPages) {
			page := state.xPages[candidate.BPage]
			if comparison.BText == page.Content && (comparison.BStart != page.ContentStart || comparison.BEnd != page.ContentEnd) {
				delta := page.ContentStart - comparison.BStart
				rebaseProofreadingOCRStepComparison(&comparison, page.ContentStart)
				step.comparisons[candidate] = comparison
				if connections, exists := step.badgeConnections[candidate]; exists {
					for ci := range connections {
						shiftProofreadingOCRConnection(&connections[ci], -1, delta)
					}
					step.badgeConnections[candidate] = connections
				}
				if anchors, exists := step.badgeAnchors[candidate]; exists {
					for ai := range anchors {
						anchors[ai].frame.BPosition += delta
					}
					step.badgeAnchors[candidate] = anchors
				}
			}
		}
		if !proofreadingOCRStepComparisonValid(state, comparison) {
			delete(step.badgeAnchors, candidate)
			delete(step.badgeConnections, candidate)
			delete(step.badgeClicksByPair, candidate)
			if step.inspector.open && step.inspector.pair == candidate {
				step.inspector.open = false
			}
			continue
		}
		pairVisible := visibleAPages[candidate.APage] && visibleBPages[candidate.BPage]
		if !pairVisible {
			continue
		}
		divider, located := proofreadingOCRStepPairDivider(gtx, a.pdfView, state, candidate)
		top, bottom, boundsLocated := proofreadingOCRStepBadgeVerticalBounds(gtx, a.pdfView, state, candidate)
		if !located || !boundsLocated {
			continue
		}
		anchors := step.badgeAnchors[candidate]
		if len(anchors) == 0 {
			badgeConnections := proofreadingOCRStepBadgeConnections(gtx, a.pdfView, state, step, comparison)
			if len(badgeConnections) > 0 {
				initial := layoutProofreadingCenterBadgesWithin(gtx, badgeConnections, divider, step.inspector.pageResolved(comparison.BPage), top, bottom)
				anchors = proofreadingCenterBadgeAnchors(initial, top, bottom)
				step.badgeAnchors[candidate] = anchors
				step.badgeConnections[candidate] = badgeConnections
				step.badgeClicksByPair[candidate] = make([]widget.Clickable, len(badgeConnections))
			}
		}
		connectionsByPair[candidate] = proofreadingOCRStepConnections(gtx, a.pdfView, state, step, comparison)
		if len(anchors) == 0 {
			continue
		}

		var activeFrame *proofreadingOCRDiffFrame
		if step.inspector.open && step.inspector.pair == candidate &&
			step.inspector.diffIndex >= 0 && step.inspector.diffIndex < len(step.badgeConnections[candidate]) {
			frame := step.badgeConnections[candidate][step.inspector.diffIndex].frame
			activeFrame = &frame
		} else if candidate == step.diffPair && (step.diffRunning || step.diffPaused) {
			if frame, active := proofreadingOCRCurrentDiffFrame(step, comparison); active {
				for _, saved := range step.badgeConnections[candidate] {
					containsActiveFrame := saved.frame == frame
					for _, grouped := range saved.allFrames {
						containsActiveFrame = containsActiveFrame || grouped == frame
					}
					if containsActiveFrame {
						badgeFrame := saved.frame
						activeFrame = &badgeFrame
						break
					}
				}
			}
		}
		badges := layoutProofreadingCenterBadgesAtAnchors(gtx, anchors, divider, top, bottom, step.inspector.pageResolved(comparison.BPage), activeFrame)
		badgesByPair[candidate] = badges
		clicks := step.badgeClicksByPair[candidate]
		for len(clicks) < len(step.badgeConnections[candidate]) {
			clicks = append(clicks, widget.Clickable{})
		}
		step.badgeClicksByPair[candidate] = clicks
	}

	step.currentConnections = connectionsByPair[pair]
	if len(step.badgeConnections[pair]) > 0 {
		step.currentConnections = step.badgeConnections[pair]
	}
	step.currentBadges = badgesByPair[pair]
	for _, pair := range pairs {
		current, ok := step.comparisons[pair]
		if !ok || !proofreadingOCRStepComparisonValid(state, current) || !visibleAPages[pair.APage] || !visibleBPages[pair.BPage] {
			continue
		}
		liveConnections := connectionsByPair[pair]
		if len(liveConnections) == 0 && current.HasEvidence && current.Complete && current.AOnly == 0 && current.BOnly == 0 &&
			!step.diffRunning && !step.diffPaused {
			a.drawProofreadingNoDiffBadge(gtx, current)
		}
		if len(liveConnections) > 0 {
			anchors := step.badgeAnchors[pair]
			storedConnections := step.badgeConnections[pair]
			badges := badgesByPair[pair]
			if len(anchors) > 0 && len(storedConnections) > 0 {
				badgeByConnection := make(map[int]proofreadingCenterBadge, len(badges))
				for _, badge := range badges {
					badgeByConnection[badge.index] = badge
				}
				lineConnections := make([]proofreadingOCRConnection, 0, len(liveConnections))
				lineBadges := make([]proofreadingCenterBadge, 0, len(liveConnections))
				for _, live := range liveConnections {
					for storedIndex, stored := range storedConnections {
						if !proofreadingOCRConnectionContainsFrame(stored, live.frame) {
							continue
						}
						if badge, exists := badgeByConnection[storedIndex]; exists {
							lineConnections = append(lineConnections, live)
							lineBadges = append(lineBadges, badge)
						}
						break
					}
				}
				if len(lineConnections) > 0 {
					a.drawProofreadingOCRStepConnectorLines(gtx, step, lineConnections, lineBadges, pair)
				}
			}
		}
	}

	for _, candidate := range pairs {
		comparison, ok := step.comparisons[candidate]
		if !ok || !proofreadingOCRStepComparisonValid(state, comparison) {
			continue
		}
		for _, badge := range badgesByPair[candidate] {
			a.drawProofreadingCenterBadge(gtx, badge)
			clicks := step.badgeClicksByPair[candidate]
			if badge.index < 0 || badge.index >= len(clicks) {
				continue
			}
			clicked := clicks[badge.index].Clicked(gtx)
			hitRect := image.Rect(badge.rect.Min.X-gtx.Dp(4), badge.rect.Min.Y-gtx.Dp(4), badge.rect.Max.X+gtx.Dp(4), badge.rect.Max.Y+gtx.Dp(4))
			trans := op.Offset(hitRect.Min).Push(gtx.Ops)
			c := gtx
			c.Constraints = layout.Exact(hitRect.Size())
			clicks[badge.index].Layout(c, func(c layout.Context) layout.Dimensions {
				pointer.CursorPointer.Add(c.Ops)
				return layout.Dimensions{Size: hitRect.Size()}
			})
			trans.Pop()
			if clicked {
				a.openProofreadingDiffInspectorForPair(candidate, badge.index)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
	if current, ok := step.comparisons[pair]; step.diffRunning && ok && proofreadingOCRStepComparisonValid(state, current) &&
		step.diffPair == pair && step.diffDraftHash == current.DraftHash {
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (a *App) drawProofreadingOCRStepConnectorLines(gtx layout.Context, step *proofreadingOCRStepState, connections []proofreadingOCRConnection, badges []proofreadingCenterBadge, pair proofreadingOCRStepPair) {
	if len(connections) == 0 || len(connections) != len(badges) {
		return
	}
	aOrigin, bOrigin, ok := proofreadingOCRStepConnectorPageOrigins(gtx, a.pdfView, a.proofreading, pair.APage, pair.BPage)
	if !ok {
		return
	}
	aSize := proofreadingOCRStepSourcePagePixelSize(a.pdfView, pair.APage)
	bSize := proofreadingXPagePixelSize(gtx, a.pdfView, a.proofreading, pair.BPage)
	reserveNavigator := a.proofreading != nil && a.proofreading.bDocument != nil && a.pdfDoc != nil
	divider := proofreadingComparePaneSplit(gtx, reserveNavigator)
	if aSize.X > 0 && bSize.X > 0 {
		aRight, bLeft := aOrigin.X+aSize.X, bOrigin.X
		if bLeft > aRight {
			divider = (aRight + bLeft) / 2
		}
	}
	gutter := image.Rect(min(divider-gtx.Dp(4), aOrigin.X+aSize.X+gtx.Dp(8)), gtx.Dp(44), max(divider+gtx.Dp(4), bOrigin.X-gtx.Dp(8)), gtx.Constraints.Max.Y)
	leftConns := make([]proofreadingOCRConnection, len(connections))
	rightConns := make([]proofreadingOCRConnection, len(connections))
	for index, connection := range connections {
		badge := badges[index]
		leftConns[index] = proofreadingOCRConnection{from: connection.from, to: image.Pt(badge.rect.Min.X, badge.y), kind: connection.kind, active: connection.active, frame: connection.frame}
		rightConns[index] = proofreadingOCRConnection{from: image.Pt(badge.rect.Max.X, badge.y), to: connection.to, kind: connection.kind, active: connection.active, frame: connection.frame}
	}
	leftGutter := image.Rect(gutter.Min.X, gutter.Min.Y, max(gutter.Min.X+gtx.Dp(4), divider-gtx.Dp(14)), gutter.Max.Y)
	rightGutter := image.Rect(min(gutter.Max.X-gtx.Dp(4), divider+gtx.Dp(14)), gutter.Min.Y, gutter.Max.X, gutter.Max.Y)
	leftRoutes := proofreadingCircuitRoutes(leftConns, leftGutter, gtx.Dp(6))
	rightRoutes := proofreadingCircuitRoutes(rightConns, rightGutter, gtx.Dp(6))
	area := clip.Rect{Min: image.Pt(0, gtx.Dp(44)), Max: gtx.Constraints.Max}.Push(gtx.Ops)
	defer area.Pop()
	for _, active := range []bool{false, true} {
		for index, connection := range connections {
			if connection.active != active {
				continue
			}
			startInk, endInk := proofreadingOCRStepConnectorColors(connection.kind)
			if badges[index].resolved {
				startInk = color.NRGBA{R: 46, G: 160, B: 67, A: 160}
				endInk = color.NRGBA{R: 46, G: 160, B: 67, A: 160}
			}
			if active {
				startInk, endInk = proofreadScanInk, proofreadScanInk
			}
			drawProofreadingCircuitRoute(gtx, leftRoutes[index], startInk, startInk, active)
			drawProofreadingCircuitRoute(gtx, rightRoutes[index], endInk, endInk, active)
			drawProofreadingOCRConnectorEndpoint(gtx, connection.from, startInk, active)
			drawProofreadingOCRConnectorEndpoint(gtx, connection.to, endInk, active)
			if active {
				combined := append(leftRoutes[index].points, rightRoutes[index].points...)
				progress := proofreadingOCRStepConnectorProgress(step, connection.kind, gtx.Now)
				drawProofreadingOCRConnectorPulse(gtx, proofreadingCircuitPoint(combined, progress), startInk)
			}
		}
	}
}

func proofreadingOCRStepConnections(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) []proofreadingOCRConnection {
	frames := proofreadingOCRScannedDiffFrames(step, comparison)
	if len(frames) == 0 {
		return nil
	}
	aOrigin, bOrigin, ok := proofreadingOCRStepConnectorPageOrigins(gtx, view, state, comparison.APage, comparison.BPage)
	if !ok {
		return nil
	}
	aSize := proofreadingOCRStepSourcePagePixelSize(view, comparison.APage)
	bSize := proofreadingXPagePixelSize(gtx, view, state, comparison.BPage)
	viewport := image.Rect(0, gtx.Dp(44), gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	type rowKey struct {
		blockID, runStart int
		badgeIndex        int
		y                 float64
		kind              proofreadingOCRDiffOpKind
	}
	rows := make(map[rowKey]int)
	badgeIndices := make(map[proofreadingOCRDiffFrame]int)
	if step != nil {
		for index, badge := range step.badgeConnections[proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}] {
			badgeIndices[badge.frame] = index
			for _, frame := range badge.allFrames {
				badgeIndices[frame] = index
			}
		}
	}
	anchors := make(map[[2]int]image.Point)
	var connections []proofreadingOCRConnection
	runStart := 0
	for index, frame := range frames {
		if index == 0 || frame.Kind != frames[index-1].Kind || frame.ABlockID != frames[index-1].ABlockID || frame.BPosition > frames[index-1].BPosition+1 {
			runStart = index
		}
		if frame.Kind == proofreadingOCRDiffEqual || frame.Kind == proofreadingOCRDiffBOnly {
			continue
		}
		if frame.ABlockID < 0 || frame.AOffset < 0 || frame.BPosition < comparison.BStart || frame.BPosition > comparison.BEnd {
			continue
		}
		anchorKey := [2]int{frame.ABlockID, frame.AOffset}
		aPoint, found := anchors[anchorKey]
		if !found {
			bounds, located := proofreadingOCRStepSourceCharacterBounds(step, comparison, frame, aSize)
			ok = located
			aPoint = image.Pt(bounds.Max.X, (bounds.Min.Y+bounds.Max.Y)/2)
			if !ok {
				continue
			}
			anchors[anchorKey] = aPoint
		}
		bPagePoint, ok := proofreadingOCRStepDraftPageAnchor(state, comparison.BPage, frame)
		if !ok {
			continue
		}
		window := state.xEditorWindows[comparison.BPage]
		geometry := proofreadingXPageTextGeometry(state.xPages[comparison.BPage], bSize, window.Top, state.xPageScrollOffsets[comparison.BPage])
		bPoint := geometry.pixelPoint(bPagePoint)
		if !bPoint.In(geometry.Content) {
			continue
		}
		connection := proofreadingOCRConnection{
			from: aOrigin.Add(aPoint), to: bOrigin.Add(bPoint), kind: frame.Kind,
			active:    index == len(frames)-1 && step != nil && (step.diffRunning || step.diffPaused),
			frame:     frame,
			allFrames: []proofreadingOCRDiffFrame{frame},
		}
		leftPane := image.Rect(0, viewport.Min.Y, proofreadingComparePaneSplit(gtx, state.bDocument != nil), viewport.Max.Y)
		rightPane := image.Rect(leftPane.Max.X, viewport.Min.Y, viewport.Max.X-gtx.Dp(proofreadingNavigatorWidthDP), viewport.Max.Y)
		if gtx.Constraints.Max.X > 0 && (!connection.from.In(leftPane) || !connection.to.In(rightPane)) {
			continue
		}
		badgeIndex := -1
		if index, found := badgeIndices[frame]; found {
			badgeIndex = index
		}
		key := rowKey{blockID: frame.ABlockID, runStart: runStart, badgeIndex: badgeIndex, y: bPagePoint.Y, kind: frame.Kind}
		if existing, found := rows[key]; found {
			connections[existing].allFrames = append(connections[existing].allFrames, frame)
			if connection.active {
				if connections[existing].from == connection.from && connections[existing].to == connection.to {
					connections[existing].active = true
				} else {
					connections = append(connections, connection)
				}
			}
			continue
		}
		rows[key] = len(connections)
		connections = append(connections, connection)
	}
	return connections
}

func proofreadingOCRResolvedDiffRuns(view *pdfViewState, state *proofreadingWorkspace, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) (map[[2]int]bool, map[int]bool) {
	resolvedA := make(map[[2]int]bool)
	resolvedB := make(map[int]bool)
	if step == nil {
		return resolvedA, resolvedB
	}
	resolvedMap := step.inspector.pageResolved(comparison.BPage)
	if len(resolvedMap) == 0 {
		return resolvedA, resolvedB
	}
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	connections := step.badgeConnections[pair]
	if len(connections) == 0 && step.diffPair == pair {
		connections = step.currentConnections
	}
	if len(connections) == 0 && view != nil && state != nil {
		connections = proofreadingOCRStepBadgeConnections(layout.Context{}, view, state, step, comparison)
	}
	for index, isRes := range resolvedMap {
		if !isRes || index < 0 || index >= len(connections) {
			continue
		}
		conn := connections[index]
		f := conn.frame
		if f.ABlockID >= 0 && f.AOffset >= 0 {
			resolvedA[[2]int{f.ABlockID, f.AOffset}] = true
		}
		if f.BPosition >= 0 {
			resolvedB[f.BPosition] = true
		}
		for _, extra := range conn.allFrames {
			if extra.ABlockID >= 0 && extra.AOffset >= 0 {
				resolvedA[[2]int{extra.ABlockID, extra.AOffset}] = true
			}
			if extra.BPosition >= 0 {
				resolvedB[extra.BPosition] = true
			}
		}
	}
	return resolvedA, resolvedB
}

func proofreadingOCRStepSourceAnchor(step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, blockID int, size image.Point) (image.Point, bool) {
	if size.X <= 0 || size.Y <= 0 {
		return image.Point{}, false
	}
	anchor, ok := proofreadingOCRStepSourcePageAnchor(step, comparison, blockID)
	if !ok {
		return image.Point{}, false
	}
	return image.Pt(int(math.Round(anchor.X*float64(size.X))), int(math.Round(anchor.Y*float64(size.Y)))), true
}

// OCR supplies image-pixel boxes. Normalize once to page coordinates, then
// project using the current displayed page size, independent of display DPI.
func proofreadingOCRStepSourcePageAnchor(step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, blockID int) (proofreadingPagePoint, bool) {
	if step == nil || comparison.APage < 0 {
		return proofreadingPagePoint{}, false
	}
	page, exists := step.pages[comparison.APage]
	if !exists || page.layout.Width <= 0 || page.layout.Height <= 0 {
		return proofreadingPagePoint{}, false
	}
	for _, block := range page.layout.Blocks {
		if block.ID != blockID {
			continue
		}
		bounds := block.Bounds
		if bounds[2] <= bounds[0] || bounds[3] <= bounds[1] {
			return proofreadingPagePoint{}, false
		}
		return proofreadingPagePoint{
			X: max(0, min(1, bounds[2]/float64(page.layout.Width))),
			Y: max(0, min(1, (bounds[1]+bounds[3])/2/float64(page.layout.Height))),
		}, true
	}
	return proofreadingPagePoint{}, false
}

func proofreadingOCRStepDraftPageAnchor(state *proofreadingWorkspace, pageIndex int, frame proofreadingOCRDiffFrame) (proofreadingTextPagePoint, bool) {
	if state == nil || pageIndex < 0 || pageIndex >= len(state.xPages) || pageIndex >= len(state.xPageEditors) || pageIndex >= len(state.xEditorWindows) || pageIndex >= len(state.xPageScrollOffsets) {
		return proofreadingTextPagePoint{}, false
	}
	page := state.xPages[pageIndex]
	if page.Width <= 0 || page.Height <= 0 || frame.BPosition < page.ContentStart || frame.BPosition > page.ContentEnd {
		return proofreadingTextPagePoint{}, false
	}
	marginX, marginY := proofreadingTextMargins(page.Width, page.Height)
	if page.ContentStart == page.ContentEnd {
		return proofreadingTextPagePoint{X: marginX + 20, Y: marginY + 13}, true
	}
	editor := &state.xPageEditors[pageIndex]
	window := state.xEditorWindows[pageIndex]
	var regions []widget.Region
	if frame.Kind == proofreadingOCRDiffAOnly {
		regions = proofreadingOCRDiffAnchorRegions(editor, page, window, frame.BPosition)
	} else {
		start := frame.BPosition - page.ContentStart
		local, valid := proofreadingOCRHighlightLocalRange(proofreadingOCRRangeSegment{
			PageIndex: pageIndex, Start: start, End: start + 1,
		}, window)
		if valid {
			regions = editor.Regions(local.Start, local.End, nil)
		}
	}
	if len(regions) == 0 {
		if state.controller == nil || state.controller.Model == nil ||
			state.controller.Layout.Revision != state.controller.Model.Revision() ||
			state.controller.Layout.StyleRevision != state.xTextSizeStep {
			return proofreadingTextPagePoint{}, false
		}
		shaped, exists := state.controller.Layout.Pages[proofreading.PageID(pageIndex+1)]
		if !exists || !strings.HasPrefix(shaped.Text, page.Content) {
			return proofreadingTextPagePoint{}, false
		}
		local := frame.BPosition - page.ContentStart
		contentLength := len([]rune(page.Content))
		if frame.Kind == proofreadingOCRDiffAOnly && local == contentLength {
			local--
		}
		for _, line := range shaped.Lines {
			if line.Start <= local && local < line.End {
				return proofreadingTextPagePoint{
					X: marginX + 20,
					Y: marginY + (line.Top+line.Bottom)/2,
				}, true
			}
		}
		return proofreadingTextPagePoint{}, false
	}
	geometry := proofreadingXTextGeometry{MarginX: marginX, MarginY: marginY, WindowTop: window.Top}
	return geometry.pagePoint(regions[0].Bounds), true
}

func proofreadingOCRStepSourcePagePixelSize(view *pdfViewState, pageIndex int) image.Point {
	if view == nil || pageIndex < 0 || pageIndex >= view.info.Pages || view.scale <= 0 {
		return image.Point{}
	}
	width, height := pdfCachedPageSize(view, pageIndex)
	return image.Pt(max(1, int(width*view.scale)), max(1, int(height*view.scale)))
}

func proofreadingOCRStepConnectorPageOrigins(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, aPage, bPage int) (image.Point, image.Point, bool) {
	if view == nil || state == nil || view.scale <= 0 || aPage < 0 || bPage < 0 || aPage >= view.info.Pages || bPage >= view.info.Pages {
		return image.Point{}, image.Point{}, false
	}
	aTop, ok := proofreadingOCRStepCanvasPageTop(gtx, view, state, aPage, view.canvas.Position, false)
	if !ok {
		return image.Point{}, image.Point{}, false
	}
	bTop, ok := proofreadingOCRStepCanvasPageTop(gtx, view, state, bPage, state.xCanvas.Position, true)
	if !ok {
		return image.Point{}, image.Point{}, false
	}
	aSize := proofreadingOCRStepSourcePagePixelSize(view, aPage)
	bSize := proofreadingXPagePixelSize(gtx, view, state, bPage)
	leftPaneWidth := proofreadingComparePaneSplit(gtx, state.bDocument != nil)
	rightPaneWidth := gtx.Constraints.Max.X - leftPaneWidth
	refWidth, _ := pdfReferencePageSize(view)
	aContentWidth := max(leftPaneWidth, int(refWidth*view.scale)+gtx.Dp(48))
	aLeft := (aContentWidth-aSize.X)/2 - view.horizontal.Position.Offset
	xListWidth := max(0, rightPaneWidth-gtx.Dp(proofreadingNavigatorWidthDP))
	bLeft := leftPaneWidth + (xListWidth-bSize.X)/2
	pageHeader := gtx.Dp(44)
	pageInset := gtx.Dp(12)
	return image.Pt(aLeft, pageHeader+aTop+pageInset), image.Pt(bLeft, pageHeader+bTop+pageInset), true
}

func proofreadingOCRStepCanvasPageTop(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, page int, position layout.Position, xCanvas bool) (int, bool) {
	row := page
	if view.pageMap != nil {
		row = proofreadingFilterRowForPage(view.pageMap, page)
	}
	first := max(0, position.First)
	if row < first || row < 0 {
		return 0, false
	}
	top := -position.Offset
	for currentRow := first; currentRow < row; currentRow++ {
		pageIndex, ok := view.pageForCanvasRow(currentRow)
		if !ok {
			continue
		}
		var size image.Point
		if xCanvas {
			size = proofreadingXPagePixelSize(gtx, view, state, pageIndex)
		} else {
			size = proofreadingOCRStepSourcePagePixelSize(view, pageIndex)
		}
		top += size.Y + 2*gtx.Dp(12)
	}
	return top, true
}

func proofreadingOCRStepConnectorColors(kind proofreadingOCRDiffOpKind) (color.NRGBA, color.NRGBA) {
	switch kind {
	case proofreadingOCRDiffAOnly:
		return proofreadDiffAOnlyInk, proofreadDiffAOnlyInk
	case proofreadingOCRDiffBOnly:
		return proofreadDiffAOnlyInk, proofreadDiffAOnlyInk
	case proofreadingOCRDiffReplace:
		return proofreadDiffAOnlyInk, proofreadDiffAOnlyInk
	default:
		return proofreadDiffSameInk, proofreadDiffSameInk
	}
}

func proofreadingOCRStepConnectorProgress(step *proofreadingOCRStepState, kind proofreadingOCRDiffOpKind, now time.Time) float64 {
	if step == nil || step.diffPaused || !step.diffRunning || step.diffNextAt.IsZero() {
		return 0.5
	}
	duration := proofreadingOCRDiffMatchFrameDelay
	if kind != proofreadingOCRDiffEqual {
		duration = proofreadingOCRDiffDifferenceFrameDelay
	}
	progress := 1 - float64(step.diffNextAt.Sub(now))/float64(duration)
	return max(0, min(1, progress))
}

func drawProofreadingOCRConnectorEndpoint(gtx layout.Context, point image.Point, ink color.NRGBA, active bool) {
	radius := max(1, gtx.Dp(2))
	if active {
		radius = max(2, gtx.Dp(4))
	}
	box := image.Rect(point.X-radius, point.Y-radius, point.X+radius, point.Y+radius)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Ellipse(box.Inset(-max(1, gtx.Dp(1)))).Op(gtx.Ops))
	ink.A = 255
	paint.FillShape(gtx.Ops, ink, clip.Ellipse(box).Op(gtx.Ops))
}

func drawProofreadingOCRConnectorPulse(gtx layout.Context, point image.Point, ink color.NRGBA) {
	radius := max(2, gtx.Dp(3))
	glow := ink
	glow.A = 150
	paint.FillShape(gtx.Ops, glow, clip.Ellipse(image.Rect(point.X-radius*2, point.Y-radius*2, point.X+radius*2, point.Y+radius*2)).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Ellipse(image.Rect(point.X-radius, point.Y-radius, point.X+radius, point.Y+radius)).Op(gtx.Ops))
}
