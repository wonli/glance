package desktopui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"

	"glance/internal/proofreading"
)

type proofreadingDiffInspector struct {
	open             bool
	pair             proofreadingOCRStepPair
	connections      []proofreadingOCRConnection
	diffIndex        int
	resolved         map[int]bool
	resolvedByPage   map[int]map[int]bool
	allResolved      bool
	editor           widget.Editor
	editorText       string
	editorPage       int
	editorRangeStart int
	editorRangeEnd   int
	diffRelStart     int
	diffRelEnd       int

	adoptPdfBtn    widget.Clickable
	keepCurrentBtn widget.Clickable
	ignoreBtn      widget.Clickable
	prevBtn        widget.Clickable
	nextBtn        widget.Clickable
	closeBtn       widget.Clickable
	finishBtn      widget.Clickable
	scrimClick     widget.Clickable
	cardClick      widget.Clickable
}

func newProofreadingDiffInspector() proofreadingDiffInspector {
	return proofreadingDiffInspector{
		resolved:       make(map[int]bool),
		resolvedByPage: make(map[int]map[int]bool),
	}
}

func (insp *proofreadingDiffInspector) pageResolved(page int) map[int]bool {
	if insp.resolvedByPage == nil {
		insp.resolvedByPage = make(map[int]map[int]bool)
	}
	res, ok := insp.resolvedByPage[page]
	if !ok {
		res = make(map[int]bool)
		insp.resolvedByPage[page] = res
	}
	return res
}

func (a *App) openProofreadingDiffInspector(badgeIndex int) {
	if a == nil || a.proofreading == nil {
		return
	}
	comparison, ok := a.currentProofreadingOCRStepComparison()
	if !ok {
		return
	}
	a.openProofreadingDiffInspectorForPair(proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}, badgeIndex)
}

func (a *App) openProofreadingDiffInspectorForPair(pair proofreadingOCRStepPair, badgeIndex int) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil {
		return
	}
	step := a.proofreading.ocrStep
	comparison, ok := step.comparisons[pair]
	if !ok || !proofreadingOCRStepComparisonValid(a.proofreading, comparison) {
		return
	}
	connections := step.badgeConnections[pair]
	if len(connections) == 0 {
		if current, currentOK := a.currentProofreadingOCRStepComparison(); currentOK &&
			pair == (proofreadingOCRStepPair{APage: current.APage, BPage: current.BPage}) {
			connections = step.currentConnections
		}
	}
	if len(connections) == 0 {
		connections = proofreadingOCRStepConnections(layout.Context{}, a.pdfView, a.proofreading, step, comparison)
	}
	if badgeIndex < 0 || badgeIndex >= len(connections) {
		return
	}
	step.inspector.open = true
	step.inspector.pair = pair
	step.inspector.connections = append(step.inspector.connections[:0], connections...)
	step.inspector.diffIndex = badgeIndex
	step.inspector.editorPage = comparison.BPage
	step.inspector.resolved = step.inspector.pageResolved(comparison.BPage)
	step.inspector.allResolved = false
	a.syncProofreadingDiffInspectorState(badgeIndex)
}

func (a *App) syncProofreadingDiffInspectorState(badgeIndex int) {
	s := a.proofreading
	if s == nil || s.ocrStep == nil {
		return
	}
	step := s.ocrStep
	comparison, ok := step.comparisons[step.inspector.pair]
	if !ok || !proofreadingOCRStepComparisonValid(s, comparison) {
		step.inspector.open = false
		return
	}
	connections := step.inspector.connections
	if badgeIndex < 0 || badgeIndex >= len(connections) {
		return
	}
	conn := connections[badgeIndex]
	frame := conn.frame
	bPage := comparison.BPage
	if bPage < 0 || bPage >= len(s.xPages) {
		return
	}

	p := s.xPages[bPage]
	runes := []rune(p.Content)
	localB := frame.BPosition - p.ContentStart
	if localB < 0 {
		localB = 0
	}
	if localB > len(runes) {
		localB = len(runes)
	}

	ctxStart := max(0, localB-25)
	for i := localB - 1; i >= max(0, localB-40); i-- {
		if runes[i] == '\n' || runes[i] == '。' || runes[i] == '；' {
			ctxStart = i + 1
			break
		}
	}
	ctxEnd := min(len(runes), localB+25)
	for i := localB; i < min(len(runes), localB+40); i++ {
		if runes[i] == '\n' || runes[i] == '。' || runes[i] == '；' {
			ctxEnd = i + 1
			break
		}
	}

	snippet := string(runes[ctxStart:ctxEnd])
	relPos := localB - ctxStart
	diffLen := 1
	if frame.Kind == proofreadingOCRDiffAOnly {
		diffLen = 0
	}

	step.inspector.diffIndex = badgeIndex
	step.inspector.editorText = snippet
	step.inspector.editorPage = bPage
	step.inspector.editorRangeStart = ctxStart
	step.inspector.editorRangeEnd = ctxEnd
	step.inspector.diffRelStart = relPos
	step.inspector.diffRelEnd = relPos + diffLen
	step.inspector.editor.SingleLine = false
	step.inspector.editor.SetText(snippet)
	step.inspector.editor.SetCaret(relPos, relPos+diffLen)
}

func (a *App) adoptProofreadingDiffInspector() {
	s := a.proofreading
	if s == nil || s.ocrStep == nil {
		return
	}
	step := s.ocrStep
	connections := step.inspector.connections
	if step.inspector.diffIndex < 0 || step.inspector.diffIndex >= len(connections) {
		return
	}
	frame := connections[step.inspector.diffIndex].frame
	replacement := string(frame.AChar)
	if frame.Kind == proofreadingOCRDiffBOnly {
		replacement = ""
	}

	currentSnippet := []rune(step.inspector.editor.Text())
	relStart := step.inspector.diffRelStart
	relEnd := step.inspector.diffRelEnd
	var newSnippet string
	if relStart >= 0 && relStart <= len(currentSnippet) && relEnd >= relStart && relEnd <= len(currentSnippet) {
		newSnippet = string(append(currentSnippet[:relStart], append([]rune(replacement), currentSnippet[relEnd:]...)...))
	} else {
		newSnippet = string(currentSnippet)
	}

	a.commitInspectorTextChange(step.inspector.editorPage, newSnippet)
	res := step.inspector.pageResolved(step.inspector.editorPage)
	res[step.inspector.diffIndex] = true
	step.inspector.resolved = res
	a.advanceProofreadingDiffInspector(1)
}

func (a *App) keepProofreadingDiffInspector() {
	s := a.proofreading
	if s == nil || s.ocrStep == nil {
		return
	}
	step := s.ocrStep
	currentSnippet := step.inspector.editor.Text()
	if currentSnippet != step.inspector.editorText {
		a.commitInspectorTextChange(step.inspector.editorPage, currentSnippet)
	}
	res := step.inspector.pageResolved(step.inspector.editorPage)
	res[step.inspector.diffIndex] = true
	step.inspector.resolved = res
	a.advanceProofreadingDiffInspector(1)
}

func (a *App) advanceProofreadingDiffInspector(delta int) {
	s := a.proofreading
	if s == nil || s.ocrStep == nil {
		return
	}
	step := s.ocrStep
	connections := step.inspector.connections
	total := len(connections)
	if total == 0 {
		step.inspector.open = false
		return
	}

	res := step.inspector.pageResolved(step.inspector.editorPage)
	step.inspector.resolved = res

	next := step.inspector.diffIndex + delta
	if next >= total {
		allDone := true
		for i := 0; i < total; i++ {
			if !res[i] {
				allDone = false
				next = i
				break
			}
		}
		if allDone {
			step.inspector.allResolved = true
			return
		}
	} else if next < 0 {
		next = total - 1
	}

	step.inspector.diffIndex = next
	step.inspector.allResolved = false
	a.syncProofreadingDiffInspectorState(next)
}

func (a *App) commitInspectorTextChange(bPage int, newSnippet string) {
	s := a.proofreading
	if s == nil || bPage < 0 || bPage >= len(s.xPages) {
		return
	}
	p := s.xPages[bPage]
	pageRunes := []rune(p.Content)
	ctxStart := min(len(pageRunes), max(0, s.ocrStep.inspector.editorRangeStart))
	ctxEnd := min(len(pageRunes), max(ctxStart, s.ocrStep.inspector.editorRangeEnd))
	newPageText := string(append(pageRunes[:ctxStart], append([]rune(newSnippet), pageRunes[ctxEnd:]...)...))
	if bPage < len(s.xPageEditors) {
		s.xPageEditors[bPage].SetText(newPageText)
	}
	if bPage < len(s.xEditorTexts) {
		s.xEditorTexts[bPage] = newPageText
	}
	delta := len([]rune(newPageText)) - len(pageRunes)
	p.Content = newPageText
	p.ContentEnd += delta
	s.xPages[bPage] = p

	if s.controller != nil && s.controller.Model != nil && bPage < len(s.xEditorWindows) {
		w := s.xEditorWindows[bPage]
		id := proofreading.PageID(bPage + 1)
		if err := s.controller.Model.ReplaceRange(id, w.StartAnchor, w.EndAnchor, newPageText); err == nil {
			w.End = w.Start + len([]rune(newPageText))
			snap := s.controller.Model.PageSnapshot(id)
			w.StartAnchor, _ = proofreading.AnchorAt(snap, id, w.Start)
			w.EndAnchor, _ = proofreading.AnchorAt(snap, id, w.End)
			s.xEditorWindows[bPage] = w
			a.proofreadingModelChanged(false)
		}
	}

	pair := s.ocrStep.inspector.pair
	if comp, ok := s.ocrStep.comparisons[pair]; ok {
		comp.BText = newPageText
		comp.BEnd += delta
		comp.DraftHash = proofreadingOCRDraftSnapshotHash(newPageText)
		relB := s.ocrStep.inspector.diffRelStart + ctxStart
		if delta != 0 {
			for fi := range comp.Frames {
				if comp.Frames[fi].BPosition > relB {
					comp.Frames[fi].BPosition += delta
				}
			}
		}
		connections := s.ocrStep.badgeConnections[pair]
		for ci := range connections {
			shiftProofreadingOCRConnection(&connections[ci], relB, delta)
		}
		s.ocrStep.badgeConnections[pair] = connections
		for ci := range s.ocrStep.inspector.connections {
			shiftProofreadingOCRConnection(&s.ocrStep.inspector.connections[ci], relB, delta)
		}
		if s.ocrStep.diffPair == pair {
			for ci := range s.ocrStep.currentConnections {
				shiftProofreadingOCRConnection(&s.ocrStep.currentConnections[ci], relB, delta)
			}
		}
		for ci := range s.ocrStep.badgeAnchors[pair] {
			frame := &s.ocrStep.badgeAnchors[pair][ci].frame
			if frame.BPosition > relB && delta != 0 {
				frame.BPosition += delta
			}
		}
		s.ocrStep.comparisons[pair] = comp
		if s.ocrStep.diffPair == pair {
			s.ocrStep.diffDraftHash = comp.DraftHash
		}
		_ = saveProofreadingOCRComparisonCache(comp)
	}
}

func shiftProofreadingOCRConnection(connection *proofreadingOCRConnection, after, delta int) {
	if connection == nil || delta == 0 {
		return
	}
	if connection.frame.BPosition > after {
		connection.frame.BPosition += delta
	}
	for index := range connection.allFrames {
		if connection.allFrames[index].BPosition > after {
			connection.allFrames[index].BPosition += delta
		}
	}
}

func (a *App) layoutProofreadingDiffInspector(gtx layout.Context) layout.Dimensions {
	if a == nil || a.proofreading == nil {
		return layout.Dimensions{}
	}
	step := a.proofreading.ocrStep
	if step == nil || !step.inspector.open {
		return layout.Dimensions{}
	}

	comparison, hasComparison := step.comparisons[step.inspector.pair]
	connections := step.inspector.connections
	hasComparison = hasComparison && proofreadingOCRStepComparisonValid(a.proofreading, comparison)
	if !hasComparison || len(connections) == 0 {
		step.inspector.open = false
		return layout.Dimensions{}
	}

	// Handle Escape key
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if keyEv, isKey := ev.(key.Event); isKey && keyEv.State == key.Press {
			step.inspector.open = false
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	// Full-screen Scrim: Consume all pointer events across the entire window
	// to completely prevent any click-through to filmstrip / pagination or toolbar!
	for step.inspector.scrimClick.Clicked(gtx) {
		step.inspector.open = false
		gtx.Execute(op.InvalidateCmd{})
	}
	step.inspector.scrimClick.Layout(gtx, func(c layout.Context) layout.Dimensions {
		paint.FillShape(c.Ops, color.NRGBA{R: 15, G: 23, B: 42, A: 140}, clip.Rect{Max: c.Constraints.Max}.Op())
		return layout.Dimensions{Size: c.Constraints.Max}
	})

	// Centered Inspector Card: absolute geometric center in the window
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		for step.inspector.cardClick.Clicked(gtx) {
		}
		cardW := min(gtx.Constraints.Max.X-gtx.Dp(48), gtx.Dp(820))
		cardH := min(gtx.Constraints.Max.Y-gtx.Dp(48), gtx.Dp(400))
		gtx.Constraints = layout.Exact(image.Pt(cardW, cardH))

		return step.inspector.cardClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			cardR := gtx.Dp(10)
			cardRect := image.Rect(0, 0, cardW, cardH)
			cardArea := clip.UniformRRect(cardRect, cardR).Push(gtx.Ops)
			defer cardArea.Pop()

			// Background & Border
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(cardRect, cardR).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 218, G: 224, B: 233, A: 255}, clip.Stroke{
				Path:  clip.UniformRRect(cardRect, cardR).Path(gtx.Ops),
				Width: float32(max(1, gtx.Dp(1))),
			}.Op())

			// Card content layout
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutProofreadingInspectorHeader(gtx, step, len(connections))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(cardW, max(1, gtx.Dp(1)))
					paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect{Max: size}.Op())
					return layout.Dimensions{Size: size}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if step.inspector.allResolved {
						return a.layoutProofreadingInspectorCompletion(gtx, step)
					}
					return a.layoutProofreadingInspectorBody(gtx, step, comparison, connections)
				}),
			)
		})
	})
}

func (a *App) layoutProofreadingInspectorHeader(gtx layout.Context, step *proofreadingOCRStepState, total int) layout.Dimensions {
	height := gtx.Dp(52)
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))

	idx := step.inspector.diffIndex
	order := idx + 1
	for _, anchor := range step.badgeAnchors[step.inspector.pair] {
		if anchor.connectionIndex == idx {
			order = anchor.orderNumber
			break
		}
	}
	kindStr := "替换差异"
	kindBg := color.NRGBA{R: 253, G: 237, B: 237, A: 255}
	kindInk := color.NRGBA{R: 217, G: 48, B: 37, A: 255}
	if idx >= 0 && idx < len(step.inspector.connections) {
		k := step.inspector.connections[idx].kind
		if k == proofreadingOCRDiffAOnly {
			kindStr = "原件有·稿件缺"
			kindBg = color.NRGBA{R: 254, G: 243, B: 199, A: 255}
			kindInk = color.NRGBA{R: 180, G: 83, B: 9, A: 255}
		} else if k == proofreadingOCRDiffBOnly {
			kindStr = "稿件多余"
			kindBg = color.NRGBA{R: 224, G: 242, B: 254, A: 255}
			kindInk = color.NRGBA{R: 3, G: 105, B: 161, A: 255}
		}
	}

	return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			// Left: Badge + Title + Kind Pill
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						badgeR := gtx.Dp(4)
						badgeSize := image.Pt(gtx.Dp(24), gtx.Dp(24))
						badgeRect := image.Rect(0, 0, badgeSize.X, badgeSize.Y)
						paint.FillShape(gtx.Ops, kindInk, clip.UniformRRect(badgeRect, badgeR).Op(gtx.Ops))
						lbl := material.Label(a.theme, 11, fmt.Sprint(order))
						lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
						lbl.Font.Weight = font.Bold
						layoutProofreadingInspectorCenteredLabel(gtx, badgeSize, lbl)
						return layout.Dimensions{Size: badgeSize}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(10), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						title := material.Label(a.theme, 13, fmt.Sprintf("差异校对 (第 %d / %d 处)", order, total))
						title.Font.Weight = font.Bold
						title.Color = color.NRGBA{R: 30, G: 41, B: 59, A: 255}
						return layoutProofreadingOpticallyCenteredLabel(gtx, title)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(10), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(a.theme, 10, kindStr)
						lbl.Color = kindInk
						lbl.Font.Weight = font.Medium
						labelGtx := gtx
						labelGtx.Constraints.Min.Y = 0
						macro := op.Record(labelGtx.Ops)
						dims := lbl.Layout(labelGtx)
						call := macro.Stop()
						pillH := gtx.Dp(22)
						pillW := dims.Size.X + gtx.Dp(16)
						pillRect := image.Rect(0, 0, pillW, pillH)
						paint.FillShape(gtx.Ops, kindBg, clip.UniformRRect(pillRect, gtx.Dp(4)).Op(gtx.Ops))
						off := op.Offset(image.Pt((pillW-dims.Size.X)/2, (pillH-dims.Size.Y)/2)).Push(gtx.Ops)
						call.Add(gtx.Ops)
						off.Pop()
						return layout.Dimensions{Size: image.Pt(pillW, pillH)}
					}),
				)
			}),
			// Right: Nav Buttons + Close Button
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if step.inspector.prevBtn.Clicked(gtx) {
					a.advanceProofreadingDiffInspector(-1)
					gtx.Execute(op.InvalidateCmd{})
				}
				if step.inspector.nextBtn.Clicked(gtx) {
					a.advanceProofreadingDiffInspector(1)
					gtx.Execute(op.InvalidateCmd{})
				}
				if step.inspector.closeBtn.Clicked(gtx) {
					step.inspector.open = false
					gtx.Execute(op.InvalidateCmd{})
				}

				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return step.inspector.prevBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutInspectorHeaderIconButton(gtx, "excel", "chevron-left", "上一处")
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(4), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return step.inspector.nextBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutInspectorHeaderIconButton(gtx, "excel", "chevron-right", "下一处")
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(8), 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return step.inspector.closeBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutInspectorHeaderIconButton(gtx, "home", "x", "关闭差异校对")
						})
					}),
				)
			}),
		)
	})
}

func (a *App) layoutInspectorHeaderIconButton(gtx layout.Context, iconFamily, iconName, description string) layout.Dimensions {
	size := image.Pt(gtx.Dp(32), gtx.Dp(32))
	gtx.Constraints = layout.Exact(size)
	semantic.DescriptionOp(description).Add(gtx.Ops)
	rect := image.Rect(0, 0, size.X, size.Y)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 241, G: 245, B: 249, A: 255}, clip.UniformRRect(rect, gtx.Dp(5)).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Stroke{
		Path:  clip.UniformRRect(rect, gtx.Dp(5)).Path(gtx.Ops),
		Width: float32(max(1, gtx.Dp(1))),
	}.Op())
	iconSize := gtx.Dp(14)
	off := op.Offset(image.Pt((size.X-iconSize)/2, (size.Y-iconSize)/2)).Push(gtx.Ops)
	layoutProofreadingIcon(gtx, iconFamily, iconName, 14, color.NRGBA{R: 71, G: 85, B: 105, A: 255})
	off.Pop()
	return layout.Dimensions{Size: size}
}

func layoutProofreadingInspectorCenteredLabel(gtx layout.Context, size image.Point, label material.LabelStyle) layout.Dimensions {
	labelGtx := gtx
	labelGtx.Constraints = layout.Constraints{Max: size}
	macro := op.Record(labelGtx.Ops)
	dims := label.Layout(labelGtx)
	call := macro.Stop()
	offset := image.Pt(max(0, (size.X-dims.Size.X)/2), max(0, (size.Y-dims.Size.Y)/2))
	off := op.Offset(offset).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return layout.Dimensions{Size: size}
}

func (a *App) layoutProofreadingInspectorBody(gtx layout.Context, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, connections []proofreadingOCRConnection) layout.Dimensions {
	idx := step.inspector.diffIndex
	if idx < 0 || idx >= len(connections) {
		return layout.Dimensions{}
	}
	conn := connections[idx]
	frame := conn.frame

	return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(14), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
			// Left: PDF original side
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutInspectorPDFSide(gtx, step, comparison, frame)
			}),
			// Center: Symmetrical Action Rail
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutInspectorCenterRail(gtx, step)
			}),
			// Right: X draft side
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutInspectorDraftSide(gtx, step, comparison, frame)
			}),
		)
	})
}

func (a *App) layoutInspectorPDFSide(gtx layout.Context, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, frame proofreadingOCRDiffFrame) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			title := material.Label(a.theme, 11, "A: PDF 原件 (局部 2.5x 放大)")
			title.Font.Weight = font.Bold
			title.Color = color.NRGBA{R: 71, G: 85, B: 105, A: 255}
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, title.Layout)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// Zoomed preview window
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					r := gtx.Dp(6)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 252, A: 255}, clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), r).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Stroke{
						Path:  clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), r).Path(gtx.Ops),
						Width: float32(max(1, gtx.Dp(1))),
					}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Max}
				},
				func(gtx layout.Context) layout.Dimensions {
					return a.layoutInspectorPDFZoomedCrop(gtx, step, comparison, frame)
				},
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(0, gtx.Dp(8))}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			charStr := string(frame.AChar)
			if frame.Kind == proofreadingOCRDiffBOnly || frame.AChar == 0 {
				charStr = "（无对应字符）"
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(a.theme, 11, "OCR 识别字符:")
					lbl.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Dp(8), 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(a.theme, 13, charStr)
					lbl.Font.Weight = font.Bold
					lbl.Color = color.NRGBA{R: 220, G: 38, B: 38, A: 255}
					macro := op.Record(gtx.Ops)
					dims := lbl.Layout(gtx)
					call := macro.Stop()

					pillH := gtx.Dp(24)
					pillW := dims.Size.X + gtx.Dp(16)
					rect := image.Rect(0, 0, pillW, pillH)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 254, G: 226, B: 226, A: 255}, clip.UniformRRect(rect, gtx.Dp(4)).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 252, G: 165, B: 165, A: 255}, clip.Stroke{
						Path:  clip.UniformRRect(rect, gtx.Dp(4)).Path(gtx.Ops),
						Width: float32(max(1, gtx.Dp(1))),
					}.Op())
					off := op.Offset(image.Pt((pillW-dims.Size.X)/2, (pillH-dims.Size.Y)/2)).Push(gtx.Ops)
					call.Add(gtx.Ops)
					off.Pop()
					return layout.Dimensions{Size: rect.Size()}
				}),
			)
		}),
	)
}

func (a *App) layoutInspectorPDFZoomedCrop(gtx layout.Context, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, frame proofreadingOCRDiffFrame) layout.Dimensions {
	w, h := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	if w <= 0 || h <= 0 {
		return layout.Dimensions{Size: image.Pt(w, h)}
	}

	view := a.pdfView
	var imageOp paint.ImageOp
	if view != nil {
		if cache, ok := view.cache[comparison.APage]; ok && cache.imageOp.Size().X > 0 {
			imageOp = cache.imageOp
		}
	}

	if imageOp.Size().X == 0 {
		// Fallback for headless testing or before render is cached
		lbl := material.Label(a.theme, 28, string(frame.AChar))
		lbl.Font.Weight = font.Bold
		lbl.Color = color.NRGBA{R: 217, G: 48, B: 37, A: 255}
		return layout.Center.Layout(gtx, lbl.Layout)
	}

	imgSize := imageOp.Size()
	bounds, located := proofreadingOCRStepSourceCharacterBounds(step, comparison, frame, imgSize)
	if !located {
		anchor, locatedAnchor := proofreadingOCRStepSourceAnchor(step, comparison, frame.ABlockID, imgSize)
		if locatedAnchor {
			bounds = image.Rect(anchor.X-24, anchor.Y-24, anchor.X+24, anchor.Y+24)
		} else {
			bounds = image.Rect(imgSize.X/2-24, imgSize.Y/2-24, imgSize.X/2+24, imgSize.Y/2+24)
		}
	}

	centerX := (bounds.Min.X + bounds.Max.X) / 2
	centerY := (bounds.Min.Y + bounds.Max.Y) / 2
	zoom := float32(2.5)

	offsetX := float32(w)/2 - float32(centerX)*zoom
	offsetY := float32(h)/2 - float32(centerY)*zoom

	cropArea := clip.UniformRRect(image.Rect(0, 0, w, h), gtx.Dp(6)).Push(gtx.Ops)
	defer cropArea.Pop()

	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: image.Pt(w, h)}.Op())

	trans := op.Offset(image.Pt(int(offsetX), int(offsetY))).Push(gtx.Ops)
	scaleAff := op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(zoom, zoom))).Push(gtx.Ops)
	imageOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	scaleAff.Pop()
	trans.Pop()

	hlRect := image.Rect(
		int(offsetX+float32(bounds.Min.X)*zoom),
		int(offsetY+float32(bounds.Min.Y)*zoom),
		int(offsetX+float32(bounds.Max.X)*zoom),
		int(offsetY+float32(bounds.Max.Y)*zoom),
	)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 217, G: 48, B: 37, A: 35}, clip.Rect(hlRect).Op())
	paint.FillShape(gtx.Ops, color.NRGBA{R: 217, G: 48, B: 37, A: 220}, clip.Stroke{
		Path:  clip.Rect(hlRect).Path(),
		Width: float32(max(1, gtx.Dp(1.5))),
	}.Op())

	return layout.Dimensions{Size: image.Pt(w, h)}
}

func (a *App) layoutInspectorCenterRail(gtx layout.Context, step *proofreadingOCRStepState) layout.Dimensions {
	railW := gtx.Dp(164)
	gtx.Constraints = layout.Exact(image.Pt(railW, gtx.Constraints.Max.Y))

	if step.inspector.adoptPdfBtn.Clicked(gtx) {
		a.adoptProofreadingDiffInspector()
		gtx.Execute(op.InvalidateCmd{})
	}
	if step.inspector.keepCurrentBtn.Clicked(gtx) {
		a.keepProofreadingDiffInspector()
		gtx.Execute(op.InvalidateCmd{})
	}
	if step.inspector.ignoreBtn.Clicked(gtx) {
		a.advanceProofreadingDiffInspector(1)
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		// Vertical subtle divider line through the center
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			midX := railW / 2
			line := image.Rect(midX, gtx.Dp(6), midX+max(1, gtx.Dp(1)), gtx.Constraints.Max.Y-gtx.Dp(6))
			paint.FillShape(gtx.Ops, proofreadChromeLine, clip.Rect(line).Op())
			return layout.Dimensions{Size: image.Pt(railW, gtx.Constraints.Max.Y)}
		}),
		// Action buttons centered
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Adopt PDF button
					return step.inspector.adoptPdfBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						btnW, btnH := gtx.Dp(136), gtx.Dp(34)
						size := image.Pt(btnW, btnH)
						gtx.Constraints = layout.Exact(size)
						r := gtx.Dp(6)
						paint.FillShape(gtx.Ops, color.NRGBA{R: 26, G: 115, B: 232, A: 255}, clip.UniformRRect(image.Rect(0, 0, btnW, btnH), r).Op(gtx.Ops))
						lbl := material.Label(a.theme, 12, "采用原文 ➔")
						lbl.Font.Weight = font.Bold
						lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
						return layoutProofreadingInspectorCenteredLabel(gtx, size, lbl)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(10))}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Keep current button
					return step.inspector.keepCurrentBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						btnW, btnH := gtx.Dp(136), gtx.Dp(34)
						size := image.Pt(btnW, btnH)
						gtx.Constraints = layout.Exact(size)
						r := gtx.Dp(6)
						paint.FillShape(gtx.Ops, color.NRGBA{R: 240, G: 253, B: 244, A: 255}, clip.UniformRRect(image.Rect(0, 0, btnW, btnH), r).Op(gtx.Ops))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 22, G: 163, B: 74, A: 255}, clip.Stroke{
							Path:  clip.UniformRRect(image.Rect(0, 0, btnW, btnH), r).Path(gtx.Ops),
							Width: float32(max(1, gtx.Dp(1.5))),
						}.Op())
						lbl := material.Label(a.theme, 12, "✓ 维持当前")
						lbl.Font.Weight = font.Bold
						lbl.Color = color.NRGBA{R: 22, G: 163, B: 74, A: 255}
						return layoutProofreadingInspectorCenteredLabel(gtx, size, lbl)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(12))}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// Skip button
					return step.inspector.ignoreBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						btnW, btnH := gtx.Dp(96), gtx.Dp(26)
						size := image.Pt(btnW, btnH)
						gtx.Constraints = layout.Exact(size)
						rect := image.Rect(0, 0, btnW, btnH)
						paint.FillShape(gtx.Ops, color.NRGBA{R: 241, G: 245, B: 249, A: 255}, clip.UniformRRect(rect, gtx.Dp(4)).Op(gtx.Ops))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Stroke{
							Path:  clip.UniformRRect(rect, gtx.Dp(4)).Path(gtx.Ops),
							Width: float32(max(1, gtx.Dp(1))),
						}.Op())
						lbl := material.Label(a.theme, 11, "跳过此项")
						lbl.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
						return layoutProofreadingInspectorCenteredLabel(gtx, size, lbl)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(6))}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					hint := material.Label(a.theme, 9, "Esc 关闭 · Tab 跳过")
					hint.Color = color.NRGBA{R: 148, G: 163, B: 184, A: 255}
					return hint.Layout(gtx)
				}),
			)
		}),
	)
}

func (a *App) layoutInspectorDraftSide(gtx layout.Context, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, frame proofreadingOCRDiffFrame) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			title := material.Label(a.theme, 11, "X: 稿件文本 (可直接编辑)")
			title.Font.Weight = font.Bold
			title.Color = color.NRGBA{R: 71, G: 85, B: 105, A: 255}
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, title.Layout)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					r := gtx.Dp(6)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), r).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 203, G: 213, B: 225, A: 255}, clip.Stroke{
						Path:  clip.UniformRRect(image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y), r).Path(gtx.Ops),
						Width: float32(max(1, gtx.Dp(1))),
					}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Max}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(a.theme, &step.inspector.editor, "")
						ed.TextSize = 15
						ed.LineHeightScale = 1.35
						return ed.Layout(gtx)
					})
				},
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(0, gtx.Dp(8))}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			charStr := string(frame.BChar)
			if frame.Kind == proofreadingOCRDiffAOnly || frame.BChar == 0 {
				charStr = "（无对应字符）"
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(a.theme, 11, "当前稿件字符:")
					lbl.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Dp(8), 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(a.theme, 13, charStr)
					lbl.Font.Weight = font.Bold
					lbl.Color = color.NRGBA{R: 2, G: 132, B: 199, A: 255}
					macro := op.Record(gtx.Ops)
					dims := lbl.Layout(gtx)
					call := macro.Stop()

					pillH := gtx.Dp(24)
					pillW := dims.Size.X + gtx.Dp(16)
					rect := image.Rect(0, 0, pillW, pillH)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 224, G: 242, B: 254, A: 255}, clip.UniformRRect(rect, gtx.Dp(4)).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 147, G: 197, B: 253, A: 255}, clip.Stroke{
						Path:  clip.UniformRRect(rect, gtx.Dp(4)).Path(gtx.Ops),
						Width: float32(max(1, gtx.Dp(1))),
					}.Op())
					off := op.Offset(image.Pt((pillW-dims.Size.X)/2, (pillH-dims.Size.Y)/2)).Push(gtx.Ops)
					call.Add(gtx.Ops)
					off.Pop()
					return layout.Dimensions{Size: rect.Size()}
				}),
			)
		}),
	)
}

func (a *App) layoutProofreadingInspectorCompletion(gtx layout.Context, step *proofreadingOCRStepState) layout.Dimensions {
	if step.inspector.finishBtn.Clicked(gtx) {
		step.inspector.open = false
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				icon := material.Label(a.theme, 36, "🎉")
				return icon.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(0, gtx.Dp(12))}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				msg := material.Label(a.theme, 16, "本页所有差异已全部校对完成！")
				msg.Font.Weight = font.Bold
				msg.Color = color.NRGBA{R: 34, G: 134, B: 58, A: 255}
				return msg.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(0, gtx.Dp(16))}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return step.inspector.finishBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					btnW, btnH := gtx.Dp(120), gtx.Dp(36)
					gtx.Constraints = layout.Exact(image.Pt(btnW, btnH))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 26, G: 115, B: 232, A: 255}, clip.UniformRRect(image.Rect(0, 0, btnW, btnH), gtx.Dp(6)).Op(gtx.Ops))
					lbl := material.Label(a.theme, 13, "完成并返回")
					lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					return layoutProofreadingInspectorFinishLabel(gtx, image.Pt(btnW, btnH), lbl)
				})
			}),
		)
	})
}

// Center the visible glyphs, including the CJK fallback font's asymmetric
// ascent/descent, rather than its larger logical line rectangle.
func layoutProofreadingInspectorFinishLabel(gtx layout.Context, size image.Point, label material.LabelStyle) layout.Dimensions {
	label.MaxLines = 1
	label.Shaper.LayoutString(text.Parameters{
		Font: label.Font, PxPerEm: fixed.I(gtx.Sp(label.TextSize)),
		MaxWidth: size.X, MaxLines: 1, Locale: gtx.Locale,
	}, label.Text)
	var ink image.Rectangle
	for glyph, ok := label.Shaper.NextGlyph(); ok; glyph, ok = label.Shaper.NextGlyph() {
		bounds := image.Rect((glyph.X + glyph.Bounds.Min.X).Floor(), int(glyph.Y)+glyph.Bounds.Min.Y.Floor(), (glyph.X + glyph.Bounds.Max.X).Ceil(), int(glyph.Y)+glyph.Bounds.Max.Y.Ceil())
		ink = ink.Union(bounds)
	}
	if ink.Empty() {
		return layoutProofreadingInspectorCenteredLabel(gtx, size, label)
	}
	labelGtx := gtx
	labelGtx.Constraints = layout.Constraints{Max: size}
	macro := op.Record(gtx.Ops)
	label.Layout(labelGtx)
	call := macro.Stop()
	offset := image.Pt((size.X-ink.Dx())/2-ink.Min.X, (size.Y-ink.Dy())/2-ink.Min.Y)
	off := op.Offset(offset).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return layout.Dimensions{Size: size}
}
