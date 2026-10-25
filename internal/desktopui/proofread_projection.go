package desktopui

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/proofreading"
)

func (a *App) layoutProofreadingProjectionPages(gtx layout.Context, status string, statusIsError bool) layout.Dimensions {
	if a.pdfDoc == nil {
		return a.layoutProofreadingPageMessageOnCanvas(gtx, a.textID("proofread.loading.source"), false, proofreadXCanvas)
	}
	state := a.proofreading
	a.validateProofreadingOCRLink()
	if state.xPageAdjustmentActive {
		i := state.xPageAdjustmentPage
		if i < 0 || i >= len(state.pageOwnership) || !state.pageOwnership[i].Locked || state.pageOwnership[i].Reviewed {
			state.xPageAdjustmentActive = false
		}
	}
	view := a.ensurePDFView()
	recordNativePinchSurface(gtx)
	pageCount := max(0, view.info.Pages)
	pages := a.proofreadingPageIndices(pageCount)
	view.pageMap = pages
	if a.syncPDFPageFromCanvas(view) {
		gtx.Execute(op.InvalidateCmd{})
	}
	state.xCanvas.Axis = layout.Vertical
	if !a.validateProofreadingOCRLink() {
		state.xCanvas.Position = view.canvas.Position
	}
	paint.FillShape(gtx.Ops, proofreadXCanvas, clip.Rect{Max: gtx.Constraints.Max}.Op())
	if pageCount == 0 {
		return a.layoutProofreadingPageMessageOnCanvas(gtx, a.textID("proofread.page_frame.error"), true, proofreadXCanvas)
	}
	if len(pages) == 0 {
		state.visibleEditorPages = []int{}
		state.xPageAdjustmentActive = false
		return a.layoutProofreadingPageMessageOnCanvas(gtx, a.textID("proofread.filter.empty"), false, proofreadXCanvas)
	}
	return a.layoutProofreadingXNavigatorCanvas(gtx, view, pageCount, status, statusIsError)
}

func (a *App) layoutProofreadingXPageList(gtx layout.Context, view *pdfViewState, pageCount int, status string, statusIsError bool) layout.Dimensions {
	state := a.proofreading
	pages := a.proofreadingPageIndices(pageCount)
	if view != nil {
		view.pageMap = pages
		if a.syncPDFPageFromCanvas(view) {
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	state.visibleEditorPages = state.visibleEditorPages[:0]
	if len(pages) == 0 {
		state.visibleEditorPages = []int{}
		state.xPageAdjustmentActive = false
		return a.layoutProofreadingPageMessageOnCanvas(gtx, a.textID("proofread.filter.empty"), false, proofreadXCanvas)
	}
	state.xViewport = gtx.Constraints.Max
	a.handleProofreadingXCanvasScroll(gtx, view)
	var snapshot proofreading.Snapshot

	dimensions := state.xCanvas.Layout(gtx, len(pages), func(gtx layout.Context, row int) layout.Dimensions {
		pageIndex, ok := proofreadingPDFPageForRow(state, pages, row)
		if !ok {
			return layout.Dimensions{}
		}
		if state.controller != nil && pageIndex < len(state.xPages) {
			if !a.proofreadingPageLayoutCurrent(pageIndex) {
				state.modelRequestedPages = append(state.modelRequestedPages, proofreading.PageID(pageIndex+1))
				gtx.Execute(op.InvalidateCmd{})
			}
			pageSnapshot := state.controller.Model.PageSnapshot(proofreading.PageID(pageIndex + 1))
			a.syncProofreadingEditor(pageIndex, pageSnapshot)
			a.applyPendingProofreadingOCRLinkScroll(pageIndex)
			snapshot.Runs = append(snapshot.Runs, pageSnapshot.Runs...)
			state.visibleEditorPages = append(state.visibleEditorPages, pageIndex)
		}
		// A list row must occupy the pane's width; otherwise Center shrink-wraps
		// the paper at the left inset while wheel/menu math assumes it is centered.
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		gtx.Constraints.Min.Y = 0
		var page *proofreadingXPage
		if pageIndex < len(state.xPages) {
			page = &state.xPages[pageIndex]
		}
		pageStatus, isError := "", false
		if page != nil && page.FrameUnavailable {
			pageStatus, isError = a.textID("proofread.page_frame.page_unavailable"), true
		}
		if pageStatus == "" && pageIndex == view.page && status != "" {
			pageStatus, isError = status, statusIsError
		}
		return a.layoutProofreadingProjectionPage(gtx, view, pageIndex, page, pageStatus, isError)
	})

	state.xEditorsSnapshot = snapshot
	a.registerProofreadingXCanvasScrollPointer(gtx)
	if a.syncProofreadingXPositionToA(view) {
		gtx.Execute(op.InvalidateCmd{})
	}

	if state.xPageMenuOpen {
		a.layoutProofreadingXPageMenuOverlay(gtx, view)
	}
	return dimensions
}

func (a *App) layoutProofreadingProjectionPage(gtx layout.Context, view *pdfViewState, pageIndex int, page *proofreadingXPage, notice string, noticeIsError bool) layout.Dimensions {
	pageWidth, pageHeight := pdfCachedPageSize(view, pageIndex)
	if page != nil && page.Width > 0 && page.Height > 0 {
		pageWidth, pageHeight = page.Width, page.Height
	}
	pageWidth = max(1, pageWidth)
	pageHeight = max(1, pageHeight)
	scale := view.scale
	if scale <= 0 || scale != scale {
		availableW := max(1, gtx.Constraints.Max.X-gtx.Dp(48))
		availableH := max(1, gtx.Constraints.Max.Y-gtx.Dp(48))
		scale = min(float64(availableW)/pageWidth, float64(availableH)/pageHeight)
	}
	pageSize := image.Pt(max(1, int(pageWidth*scale)), max(1, int(pageHeight*scale)))
	return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(pageSize)
			shadowOffset := gtx.Dp(2)
			shadowRect := image.Rect(-shadowOffset, -shadowOffset, pageSize.X+shadowOffset, pageSize.Y+shadowOffset)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 20}, clip.UniformRRect(shadowRect, gtx.Dp(3)).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: pageSize}.Op())
			if a.proofreading.xPageAdjustmentActive && a.proofreading.xPageAdjustmentPage == pageIndex {
				drawProofreadingXPageScrollHighlight(gtx, pageSize, 255)
			} else if alpha := a.proofreadingXScrollHighlightAlpha(gtx, pageIndex); alpha > 0 {
				drawProofreadingXPageScrollHighlight(gtx, pageSize, alpha)
			} else {
				a.drawProofreadingXPageStatus(gtx, pageSize, pageIndex)
			}
			if page != nil {
				// Let the page click observer and text editor receive the same click.
				pass := pointer.PassOp{}.Push(gtx.Ops)
				a.handleProofreadingXPagePointer(gtx, view, pageIndex, *page)
				a.registerProofreadingXPagePointer(gtx, pageIndex, pageSize)
				a.drawProofreadingXContent(gtx, *page, pageSize)
				a.layoutProofreadingXPageScrollbar(gtx, *page, pageSize)
				pass.Pop()
			}
			a.layoutProofreadingOCRMissingPageBadge(gtx, pageSize, pageIndex)
			if notice != "" && pageIndex == view.page {
				a.drawProofreadingProjectionNotice(gtx, pageSize, notice, noticeIsError)
			}
			return layout.Dimensions{Size: pageSize}
		})
	})
}

type proofreadingContentRange struct {
	Start int
	End   int
}

type proofreadingFlowBlock struct {
	Kind  proofreadingUnitKind
	Level int
	Start int
	End   int
}

func proofreadingProjectionContent(units []proofreadingContentUnit) (string, []proofreadingFlowBlock) {
	var content strings.Builder
	blocks := make([]proofreadingFlowBlock, 0, len(units))
	contentRunes := 0
	for _, unit := range units {
		value := strings.ReplaceAll(unit.Text, "\r\n", "\n")
		value = strings.ReplaceAll(value, "\r", "\n")
		if unit.Kind == proofreadingUnitTableRow {
			value = strings.ReplaceAll(value, "\t", "│")
		} else {
			value = strings.ReplaceAll(value, "\t", " ")
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if contentRunes > 0 {
			content.WriteByte('\n')
			contentRunes++
		}
		start := contentRunes
		content.WriteString(value)
		contentRunes += len([]rune(value))
		blocks = append(blocks, proofreadingFlowBlock{Kind: unit.Kind, Level: unit.Level, Start: start, End: contentRunes})
	}
	return content.String(), blocks
}

// Keep the existing blue selection/scroll highlight above review decoration.
// The label sits in the existing top gutter, outside the editable paper.
func (a *App) drawProofreadingXPageStatus(gtx layout.Context, size image.Point, page int) {
	label, _, border, ink := proofreadingThumbnailStatus(a.proofreadingPageStatusKey(page))
	proofreadRoundedBorder(gtx, size, 2, border, max(1, gtx.Dp(1)))
	offset := op.Offset(image.Pt(0, -gtx.Dp(12))).Push(gtx.Ops)
	defer offset.Pop()
	gtx.Constraints = layout.Constraints{Max: image.Pt(size.X, gtx.Dp(12))}
	style := material.Label(a.theme, unit.Sp(9), a.textID(label))
	style.Color = ink
	style.Alignment = text.End
	style.MaxLines = 1
	style.Layout(gtx)
}

func proofreadingPageBorder(gtx layout.Context, size image.Point) {
	line := color.NRGBA{R: 213, G: 217, B: 222, A: 255}
	proofreadRoundedBorder(gtx, size, 2, line, max(1, gtx.Dp(1)))
}

func (a *App) proofreadingXScrollHighlightAlpha(gtx layout.Context, pageIndex int) uint8 {
	state := a.proofreading
	if state == nil || pageIndex != state.xScrollHighlightPage || state.xScrollHighlightUntil.IsZero() {
		return 0
	}
	if gtx.Now.Before(state.xScrollHighlightUntil) {
		return 255
	}
	const fadeDuration = 420 * time.Millisecond
	remaining := state.xScrollHighlightUntil.Add(fadeDuration).Sub(gtx.Now)
	if remaining <= 0 {
		return 0
	}
	nextFrame := 16 * time.Millisecond
	if remaining < nextFrame {
		nextFrame = remaining
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(nextFrame)})
	return uint8(255 * float64(remaining) / float64(fadeDuration))
}

func drawProofreadingXPageScrollHighlight(gtx layout.Context, size image.Point, alpha uint8) {
	if size.X <= 0 || size.Y <= 0 || alpha == 0 {
		return
	}
	thickness := min(gtx.Dp(28), min(size.X, size.Y)/8)
	if thickness < 1 {
		return
	}
	accent := color.NRGBA{R: 48, G: 128, B: 220, A: alpha}
	paintPageEdgeFade := func(bounds image.Rectangle, start, end f32.Point, opacity uint8) {
		if bounds.Empty() || opacity == 0 {
			return
		}
		shape := clip.Rect(bounds).Push(gtx.Ops)
		clear := color.NRGBA{R: accent.R, G: accent.G, B: accent.B, A: 0}
		paint.LinearGradientOp{
			Stop1: start, Color1: color.NRGBA{R: accent.R, G: accent.G, B: accent.B, A: opacity},
			Stop2: end, Color2: clear,
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		shape.Pop()
	}

	fadeAlpha := uint8(float64(alpha) * 0.28)
	paintPageEdgeFade(image.Rect(0, 0, size.X, thickness), f32.Pt(0, 0), f32.Pt(0, float32(thickness)), fadeAlpha)
	paintPageEdgeFade(image.Rect(0, size.Y-thickness, size.X, size.Y), f32.Pt(0, float32(size.Y)), f32.Pt(0, float32(size.Y-thickness)), fadeAlpha)
	paintPageEdgeFade(image.Rect(0, 0, thickness, size.Y), f32.Pt(0, 0), f32.Pt(float32(thickness), 0), uint8(float64(fadeAlpha)*0.72))
	paintPageEdgeFade(image.Rect(size.X-thickness, 0, size.X, size.Y), f32.Pt(float32(size.X), 0), f32.Pt(float32(size.X-thickness), 0), uint8(float64(fadeAlpha)*0.72))
	proofreadRoundedBorder(gtx, size, 2, accent, max(1, gtx.Dp(2)))
}

func (a *App) drawProofreadingProjectionNotice(gtx layout.Context, pageSize image.Point, message string, isError bool) {
	margin := gtx.Dp(10)
	height := gtx.Dp(24)
	width := min(pageSize.X-2*margin, gtx.Dp(420))
	if width <= 0 || pageSize.Y <= 2*margin {
		return
	}
	if height > pageSize.Y-2*margin {
		height = pageSize.Y - 2*margin
	}
	offset := op.Offset(image.Pt((pageSize.X-width)/2, margin)).Push(gtx.Ops)
	defer offset.Pop()
	background := color.NRGBA{R: 232, G: 242, B: 252, A: 255}
	textColor := proofreadBlue
	if isError {
		background = color.NRGBA{R: 253, G: 239, B: 237, A: 255}
		textColor = color.NRGBA{R: 157, G: 61, B: 54, A: 255}
	}
	paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rect(0, 0, width, height), gtx.Dp(4)).Op(gtx.Ops))
	child := gtx
	child.Constraints = layout.Exact(image.Pt(width, height))
	label := material.Label(a.theme, 10, message)
	label.Color = textColor
	label.MaxLines = 1
	layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(child, label.Layout)
}

func (a *App) drawProofreadingXContent(gtx layout.Context, page proofreadingXPage, pageSize image.Point) {
	state := a.proofreading
	if state == nil || page.PageIndex < 0 || page.PageIndex >= len(state.xPageEditors) {
		return
	}
	scale := 1.0
	if page.Width > 0 && page.Height > 0 {
		scale = min(float64(pageSize.X)/page.Width, float64(pageSize.Y)/page.Height)
	}
	marginDocX, marginDocY := proofreadingTextMargins(page.Width, page.Height)
	marginX := int(math.Round(marginDocX * scale))
	marginY := int(math.Round(marginDocY * scale))
	contentRect := image.Rect(marginX, marginY, max(marginX, pageSize.X-marginX), max(marginY, pageSize.Y-marginY))
	if contentRect.Empty() {
		return
	}
	// Shape in document coordinates, then scale the recorded editor operations.
	// Re-shaping at the display's pixel size changes rounded baselines and can
	// change wrapping, invalidating the worker's page/window line boundaries.
	fontSize := unit.Sp(12 * proofreadingXTextScale(state.xTextSizeStep))
	offset := op.Offset(contentRect.Min).Push(gtx.Ops)
	defer offset.Pop()
	child := gtx
	child.Metric = unit.Metric{PxPerDp: 1, PxPerSp: 1}
	child.Constraints = layout.Exact(image.Pt(max(1, int(math.Round(page.Width-2*marginDocX))), max(1, int(math.Round(page.Height-2*marginDocY)))))
	area := clip.Rect{Max: contentRect.Size()}.Push(gtx.Ops)
	defer area.Pop()
	editor := &state.xPageEditors[page.PageIndex]
	editor.SingleLine = false
	editor.ReadOnly = !state.modelInitialAssigned
	editor.LineHeightScale = 1.28
	editor.WrapPolicy = text.WrapWords
	style := material.Editor(a.theme, editor, a.textID("proofread.x_editor.empty"))
	style.TextSize = fontSize
	style.Color = proofreadInk
	style.LineHeightScale = 1.28
	style.SelectionColor = color.NRGBA{R: 187, G: 215, B: 244, A: 160}
	window := state.xEditorWindows[page.PageIndex]
	state.xPageRenderScales[page.PageIndex] = scale
	state.xPageViewportHeights[page.PageIndex] = float64(contentRect.Dy()) / scale
	transform := op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(float32(scale), float32(scale)))).Push(gtx.Ops)
	defer transform.Pop()
	shiftY := int(math.Round(window.Top - state.xPageScrollOffsets[page.PageIndex]))
	shift := op.Offset(image.Pt(0, shiftY)).Push(gtx.Ops)
	row := child
	row.Constraints.Min.Y = 0
	// Only a bounded text window is present in this editor. Give it its natural
	// height so its private scroll-to-caret cannot fight the page scrollbar.
	row.Constraints.Max.Y = 1 << 20
	style.Layout(row)
	a.drawProofreadingOCRMatchHighlight(gtx, editor, page, window)
	// Mark the last character without changing native selection or keyboard focus.
	n := &state.navigator
	if n.initialized && n.page == page.PageIndex && !n.feedbackAt.IsZero() && gtx.Now.Sub(n.feedbackAt) < 700*time.Millisecond && n.highlightStart >= window.Start && n.highlightEnd <= window.End && n.highlightEnd > n.highlightStart {
		for _, region := range editor.Regions(n.highlightStart-window.Start, n.highlightEnd-window.Start, nil) {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 64, G: 139, B: 225, A: 55}, clip.Rect(region.Bounds).Op())
		}
		gtx.Execute(op.InvalidateCmd{At: n.feedbackAt.Add(700 * time.Millisecond)})
	}
	shift.Pop()
}

func (a *App) layoutProofreadingXBlock(gtx layout.Context, block proofreadingXBlock, baseSize unit.Sp) layout.Dimensions {
	textValue := strings.TrimSpace(block.Text)
	if textValue == "" {
		return layout.Dimensions{}
	}
	if block.Kind == proofreadingUnitRule {
		height := max(1, gtx.Dp(1))
		paint.FillShape(gtx.Ops, proofreadLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(4))}
	}
	textSize := proofreadingXBlockTextSize(baseSize, block)
	textColor := proofreadInk
	labelInset := layout.Inset{}
	if block.Kind == proofreadingUnitUnsupported {
		textColor = proofreadMuted
	}
	if block.Kind == proofreadingUnitList {
		labelInset.Left = unit.Dp(12)
	}
	if block.Kind == proofreadingUnitQuote {
		barWidth := max(1, gtx.Dp(2))
		barColor := color.NRGBA{R: 173, G: 190, B: 210, A: 255}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, barColor, clip.Rect{Max: image.Pt(barWidth, gtx.Dp(22))}.Op())
				return layout.Dimensions{Size: image.Pt(barWidth, gtx.Dp(22))}
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutProofreadingXLabel(gtx, textValue, textSize, proofreadMuted, false)
				})
			}),
		)
	}
	if block.Kind == proofreadingUnitTableRow {
		labelInset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}
	} else if block.Kind == proofreadingUnitCode {
		labelInset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}
		textColor = proofreadMuted
	}
	return labelInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.layoutProofreadingXLabel(gtx, textValue, textSize, textColor, block.Kind == proofreadingUnitHeading)
	})
}

func proofreadingXBlockTextSize(baseSize unit.Sp, block proofreadingXBlock) unit.Sp {
	if block.Kind != proofreadingUnitHeading {
		return baseSize
	}
	factor := float32(1.28)
	if block.Level > 1 {
		factor = 1.22 - float32(min(block.Level-2, 3))*0.06
	}
	return baseSize * unit.Sp(factor)
}

func proofreadingXBlockTextInsets(gtx layout.Context, block proofreadingXBlock) (left, top, right, bottom int) {
	switch block.Kind {
	case proofreadingUnitList:
		left = gtx.Dp(12)
	case proofreadingUnitQuote:
		left = max(1, gtx.Dp(2)) + gtx.Dp(9)
	case proofreadingUnitTableRow:
		left, right, top, bottom = gtx.Dp(6), gtx.Dp(6), gtx.Dp(3), gtx.Dp(3)
	case proofreadingUnitCode:
		left, right, top, bottom = gtx.Dp(8), gtx.Dp(8), gtx.Dp(3), gtx.Dp(3)
	}
	return
}

func (a *App) layoutProofreadingXLabel(gtx layout.Context, value string, size unit.Sp, color color.NRGBA, heading bool) layout.Dimensions {
	label := material.Label(a.theme, size, value)
	label.Color = color
	label.WrapPolicy = text.WrapGraphemes
	label.LineHeightScale = 1.28
	if heading {
		label.Font.Weight = font.SemiBold
	}
	return label.Layout(gtx)
}
