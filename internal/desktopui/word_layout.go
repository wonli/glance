package desktopui

import (
	"encoding/hex"
	"image"
	"image/color"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

type wordImageInteraction struct {
	moveDrag       gesture.Drag
	resizeDrag     gesture.Drag
	preview        wordImageGesture
	moveStart      f32.Point
	resizeStartX   float32
	resizeStartEMU int64
	displayWidth   int
}

func wordContentOrder(blocks []document.WordBlock) []string {
	var order []string
	appendParagraph := func(paragraph document.WordParagraph) {
		for _, inline := range paragraph.Inlines {
			switch {
			case inline.Text != nil:
				order = append(order, inline.Text.ID)
			case inline.Image != nil:
				order = append(order, inline.Image.ID)
			case inline.Unsupported != nil:
				order = append(order, inline.Unsupported.ID)
			}
		}
	}
	for _, block := range blocks {
		switch block.Kind {
		case document.WordParagraphBlock:
			if block.Paragraph != nil {
				appendParagraph(*block.Paragraph)
			}
		case document.WordTableBlock:
			if block.Table == nil {
				continue
			}
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						appendParagraph(paragraph)
					}
				}
			}
		case document.WordUnsupportedBlock:
			if block.Unsupported != nil {
				order = append(order, block.Unsupported.ID)
			}
		}
	}
	return order
}

func wordBlocksForState(state *documentSessionState) []document.WordBlock {
	if state == nil {
		return nil
	}
	if state.wordBlocks != nil {
		return state.wordBlocks
	}
	if state.word != nil {
		state.wordBlocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		return state.wordBlocks
	}
	return nil
}

func selectWordRun(state *documentSessionState, paragraphID, runID string) bool {
	if state == nil || state.word == nil {
		return false
	}
	blocks := wordBlocksForState(state)
	paragraph, ok := selectedWordParagraph(blocks, paragraphID)
	if !ok {
		return false
	}
	run, ok := selectedWordRun(blocks, runID)
	if !ok || !run.Editable {
		return false
	}
	state.selectedWordRunID = runID
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordToolbarTab = wordTabHome
	state.wordEditor.SetText(run.Text)
	return true
}

func selectWordImage(state *documentSessionState, imageID string) bool {
	if state == nil || state.word == nil {
		return false
	}
	blocks := wordBlocksForState(state)
	if findWordImage(blocks, imageID) == nil {
		return false
	}
	state.selectedWordRunID = ""
	state.selectedWordImageID = imageID
	state.selectedBlock = -1
	state.selectedBlockEditable = false
	state.wordToolbarTab = wordTabPicture
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == imageID {
				state.selectedWordParagraphID = paragraph.ID
				return true
			}
		}
	}
	return false
}

func wordObjectButton(state *documentSessionState, id string) *widget.Clickable {
	if state.wordObjectButtons == nil {
		state.wordObjectButtons = make(map[string]*widget.Clickable)
	}
	button := state.wordObjectButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		state.wordObjectButtons[id] = button
	}
	return button
}

func wordImageInteractionFor(state *documentSessionState, id string) *wordImageInteraction {
	if state.wordImageInteractions == nil {
		state.wordImageInteractions = make(map[string]*wordImageInteraction)
	}
	interaction := state.wordImageInteractions[id]
	if interaction == nil {
		interaction = &wordImageInteraction{}
		state.wordImageInteractions[id] = interaction
	}
	return interaction
}

func (a *App) layoutWordFlow(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return layout.Dimensions{}
	}
	blocks := wordBlocksForState(state)
	state.wordListState.List.Axis = layout.Vertical
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutWordToolbar),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return material.List(a.theme, &state.wordListState).Layout(gtx, len(blocks), func(gtx layout.Context, index int) layout.Dimensions {
						return a.layoutWordBlock(gtx, blocks[index])
					})
				}),
			)
		}),
		layout.Stacked(a.layoutWordToolbarColorPicker),
	)
}

func (a *App) layoutWordBlock(gtx layout.Context, block document.WordBlock) layout.Dimensions {
	switch block.Kind {
	case document.WordParagraphBlock:
		if block.Paragraph != nil {
			return a.layoutWordParagraph(gtx, *block.Paragraph, false)
		}
	case document.WordTableBlock:
		if block.Table != nil {
			return a.layoutWordTable(gtx, *block.Table)
		}
	case document.WordUnsupportedBlock:
		if block.Unsupported != nil {
			return wordReadOnlyPlaceholder(gtx, a.theme, block.Unsupported.Description)
		}
	}
	return layout.Dimensions{}
}

func (a *App) layoutWordTable(gtx layout.Context, table document.WordTable) layout.Dimensions {
	rows := make([]layout.FlexChild, 0, len(table.Rows))
	for _, row := range table.Rows {
		row := row
		cells := make([]layout.FlexChild, 0, len(row.Cells))
		for _, cell := range row.Cells {
			cell := cell
			cells = append(cells, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordTableCell(gtx, cell)
			}))
		}
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, cells...)
			})
		}))
	}
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return wordMeasuredBackground(gtx, gridLine, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		})
	})
}

func (a *App) layoutWordTableCell(gtx layout.Context, cell document.WordTableCell) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(cell.Paragraphs))
	for _, paragraph := range cell.Paragraphs {
		paragraph := paragraph
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordParagraph(gtx, paragraph, true)
		}))
	}
	return wordMeasuredBackground(gtx, contentSurface, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

// wordMeasuredBackground paints a list item's measured bounds without claiming
// the full remaining viewport height. Returning viewport-sized fill dimensions
// here makes a vertical List believe its first paragraph consumes the screen.
func wordMeasuredBackground(gtx layout.Context, fill color.NRGBA, child func(layout.Context) layout.Dimensions) layout.Dimensions {
	recorder := op.Record(gtx.Ops)
	dimensions := child(gtx)
	call := recorder.Stop()
	paint.FillShape(gtx.Ops, fill, clip.Rect{Max: dimensions.Size}.Op())
	call.Add(gtx.Ops)
	return dimensions
}

func (a *App) layoutWordParagraph(gtx layout.Context, paragraph document.WordParagraph, inCell bool) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(paragraph.Inlines))
	for _, inline := range paragraph.Inlines {
		inline := inline
		switch {
		case inline.Text != nil:
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordTextRun(gtx, paragraph, *inline.Text)
			}))
		case inline.Image != nil:
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordImage(gtx, paragraph, *inline.Image)
			}))
		case inline.Unsupported != nil:
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return wordReadOnlyPlaceholder(gtx, a.theme, inline.Unsupported.Description)
			}))
		}
	}
	spacing := unit.Dp(6)
	if inCell {
		spacing = unit.Dp(3)
	}
	return layout.Inset{Top: unit.Dp(5), Bottom: spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (a *App) layoutWordTextRun(gtx layout.Context, paragraph document.WordParagraph, run document.WordTextRun) layout.Dimensions {
	state := a.documentSessionState
	button := wordObjectButton(state, run.ID)
	selected := state.selectedWordRunID == run.ID
	previewTarget := false
	for _, interaction := range state.wordImageInteractions {
		preview := interaction.preview.Preview()
		if preview.Active && interaction.preview.mode == wordImageGestureMove && preview.TargetParagraph == paragraph.ID && preview.AfterInlineID == run.ID {
			previewTarget = true
		}
	}
	if button.Clicked(gtx) {
		selectWordRun(state, paragraph.ID, run.ID)
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	for _, interaction := range state.wordImageInteractions {
		if interaction.preview.mode == wordImageGestureMove && interaction.preview.Preview().Active && button.Hovered() {
			interaction.preview.MovePreview(paragraph.ID, run.ID)
			state.wordImageDropTargetID = run.ID
			state.wordImageDropParagraphID = paragraph.ID
		}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		label := wordRunLabel(a.theme, paragraph, run)
		background := contentSurface
		if selected {
			background = selectedFill
		} else if previewTarget {
			background = toolbarActiveBackground
		}
		return wordMeasuredBackground(gtx, background, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := label.Layout(gtx)
				if run.Style.Underline && dims.Size.X > 0 && dims.Size.Y > 0 {
					y := dims.Size.Y - max(1, gtx.Dp(unit.Dp(1)))
					paint.FillShape(gtx.Ops, label.Color, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(dims.Size.X, y+max(1, gtx.Dp(unit.Dp(1))))}.Op())
				}
				return dims
			})
		})
	})
}

func wordRunLabel(theme *material.Theme, paragraph document.WordParagraph, run document.WordTextRun) material.LabelStyle {
	label := material.Body1(theme, run.Text)
	label.Alignment = wordParagraphAlignment(paragraph.Alignment)
	if run.Style.Bold {
		label.Font.Weight = font.Bold
	}
	if run.Style.Italic {
		label.Font.Style |= font.Italic
	}
	if run.Style.FontFamily != "" {
		label.Font.Typeface = font.Typeface(run.Style.FontFamily)
	}
	if run.Style.FontSize > 0 {
		label.TextSize = unit.Sp(run.Style.FontSize)
	}
	if fontColor, ok := parseWordRunColor(run.Style.FontColor); ok {
		label.Color = fontColor
	}
	switch strings.ToLower(paragraph.Style) {
	case "heading1":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(20)
	case "heading2":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(18)
	case "heading3":
		label.Font.Weight = font.Bold
		label.TextSize = unit.Sp(16)
	}
	return label
}

func wordParagraphAlignment(alignment document.WordAlignment) text.Alignment {
	switch alignment {
	case document.WordAlignCenter:
		return text.Middle
	case document.WordAlignRight:
		return text.End
	case document.WordAlignJustified:
		return text.Start
	default:
		return text.Start
	}
}

func parseWordRunColor(value string) (color.NRGBA, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return color.NRGBA{}, false
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: data[0], G: data[1], B: data[2], A: 255}, true
}

func (a *App) requestWordImageDecode(state *documentSessionState, imageID string) {
	if state == nil || state.word == nil || imageID == "" {
		return
	}
	if state.wordImagePending == nil {
		state.wordImagePending = make(map[string]uint64)
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	if state.wordImageErrors == nil {
		state.wordImageErrors = make(map[string]string)
	}
	version := state.wordImageVersions[imageID]
	if pendingVersion, pending := state.wordImagePending[imageID]; pending && pendingVersion == version {
		return
	}
	if a.wordImageResults == nil {
		a.wordImageResults = make(chan wordImageDecodeResult, 64)
	}
	state.wordImagePending[imageID] = version
	shutdown := a.shutdown
	results := a.wordImageResults
	window := a.window
	go func() {
		data, err := state.word.WordImageBytes(imageID)
		var decoded image.Image
		if err == nil {
			decoded, err = decodeWordPreview(data)
		}
		if err == nil {
			decoded = compactWordPreview(decoded)
		}
		result := wordImageDecodeResult{state: state, id: imageID, version: version, image: decoded, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func invalidateWordImagePreview(state *documentSessionState, imageID string) {
	if state == nil || imageID == "" {
		return
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	state.wordImageVersions[imageID]++
	state.wordImageCache.Delete(imageID)
	delete(state.wordImageErrors, imageID)
}

func invalidateAllWordImagePreviews(state *documentSessionState) {
	if state == nil {
		return
	}
	if state.wordImageCache != nil {
		state.wordImageCache.Clear()
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	for id := range state.wordImagePending {
		state.wordImageVersions[id]++
	}
	if state.word != nil {
		for _, block := range wordBlocksForState(state) {
			for _, paragraph := range wordParagraphsInBlock(block) {
				for _, inline := range paragraph.Inlines {
					if inline.Image != nil {
						state.wordImageVersions[inline.Image.ID]++
						delete(state.wordImageErrors, inline.Image.ID)
					}
				}
			}
		}
	}
}

func wordParagraphsInBlock(block document.WordBlock) []document.WordParagraph {
	switch block.Kind {
	case document.WordParagraphBlock:
		if block.Paragraph != nil {
			return []document.WordParagraph{*block.Paragraph}
		}
	case document.WordTableBlock:
		if block.Table != nil {
			var paragraphs []document.WordParagraph
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
			return paragraphs
		}
	}
	return nil
}

func reconcileWordSelection(state *documentSessionState, syncDraft bool) {
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	if state.selectedWordRunID != "" {
		if run, ok := selectedWordRun(blocks, state.selectedWordRunID); ok {
			if syncDraft {
				state.wordEditor.SetText(run.Text)
			}
		} else {
			state.selectedWordRunID = ""
			state.selectedWordParagraphID = ""
			state.wordEditor.SetText("")
		}
	}
	if state.selectedWordImageID != "" && findWordImage(blocks, state.selectedWordImageID) == nil {
		state.selectedWordImageID = ""
	}
}

func (a *App) layoutWordImage(gtx layout.Context, paragraph document.WordParagraph, item document.WordImage) layout.Dimensions {
	state := a.documentSessionState
	interaction := wordImageInteractionFor(state, item.ID)
	a.updateWordImageMoveGesture(gtx, interaction, item, paragraph.ID)
	button := wordObjectButton(state, item.ID)
	selected := state.selectedWordImageID == item.ID
	decoded, cached := state.wordImageCache.Get(item.ID)
	if !cached && state.wordImageErrors[item.ID] == "" {
		a.requestWordImageDecode(state, item.ID)
	}
	preview := interaction.preview.Preview()
	widthEMU, heightEMU := item.WidthEMU, item.HeightEMU
	if preview.Active && interaction.preview.mode == wordImageGestureResize {
		widthEMU, heightEMU = preview.WidthEMU, preview.HeightEMU
	}
	maxWidth := max(1, gtx.Constraints.Max.X-gtx.Dp(unit.Dp(32)))
	maxHeight := gtx.Dp(unit.Dp(360))
	var visualSize image.Point
	if decoded != nil {
		if widthEMU <= 0 || heightEMU <= 0 {
			visualSize = fitWordImageSize(decoded.Bounds().Dx(), decoded.Bounds().Dy(), maxWidth, maxHeight)
		} else {
			visualWidth := wordImageEMUToPixels(widthEMU, gtx.Metric.PxPerDp)
			visualHeight := wordImageEMUToPixels(heightEMU, gtx.Metric.PxPerDp)
			visualSize = fitWordImageSize(visualWidth, visualHeight, maxWidth, maxHeight)
		}
		interaction.displayWidth = visualSize.X
	}
	if button.Clicked(gtx) {
		selectWordImage(state, item.ID)
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		background := contentSurface
		if selected {
			background = selectedFill
		}
		return wordMeasuredBackground(gtx, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						interaction.moveDrag.Add(gtx.Ops)
						if decoded == nil {
							message := state.wordImageErrors[item.ID]
							if message == "" {
								message = "正在载入图片…"
							} else {
								message = "图片无法预览：" + message
							}
							return wordReadOnlyPlaceholder(gtx, a.theme, message)
						}
						gtx.Constraints.Min = visualSize
						gtx.Constraints.Max = visualSize
						semantic.DescriptionOp("内嵌图片 " + item.ID).Add(gtx.Ops)
						return widget.Image{Src: paint.NewImageOp(decoded), Fit: widget.Contain, Position: layout.Center, Scale: float32(visualSize.X) / float32(max(1, decoded.Bounds().Dx())) / max32(1, gtx.Metric.PxPerDp)}.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !selected {
						return layout.Dimensions{}
					}
					return a.layoutWordResizeHandle(gtx, interaction, item)
				}),
			)
		})
	})
}

func (a *App) updateWordImageMoveGesture(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage, paragraphID string) {
	for event, ok := interaction.moveDrag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = interaction.moveDrag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		switch event.Kind {
		case pointer.Press:
			interaction.moveStart = event.Position
			interaction.preview.BeginMove(item.ID, paragraphID)
		case pointer.Release:
			interaction.preview.ReleaseMove()
			preview := interaction.preview.Preview()
			if interaction.preview.CommitMove() {
				a.runWordImageCommand("移动图片", item.ID, func(editor document.WordImageEditor) error {
					return editor.MoveInlineImage(item.ID, preview.TargetParagraph, preview.AfterInlineID)
				}, false)
			}
			interaction.preview.Cancel()
		case pointer.Cancel:
			interaction.preview.Cancel()
		}
	}
}

func (a *App) layoutWordResizeHandle(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage) layout.Dimensions {
	button := wordResizeHandleButton(a.documentSessionState, item.ID)
	for event, ok := interaction.resizeDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal); ok; event, ok = interaction.resizeDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal) {
		switch event.Kind {
		case pointer.Press:
			interaction.resizeStartX = event.Position.X
			interaction.resizeStartEMU = item.WidthEMU
			interaction.preview.BeginResize(item.ID, item.WidthEMU, item.HeightEMU)
		case pointer.Drag:
			displayWidth := max(1, interaction.displayWidth)
			deltaEMU := resizeWordImageWidthEMU(float64(event.Position.X-interaction.resizeStartX), item.WidthEMU, displayWidth)
			interaction.preview.ResizePreview(interaction.resizeStartEMU + deltaEMU)
		case pointer.Release:
			interaction.preview.ReleaseResize()
			preview := interaction.preview.Preview()
			if interaction.preview.CommitResize() {
				a.runWordImageCommand("调整图片", item.ID, func(editor document.WordImageEditor) error {
					return editor.ResizeInlineImage(item.ID, preview.WidthEMU)
				}, false)
			}
		case pointer.Cancel:
			interaction.preview.Cancel()
		}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		interaction.resizeDrag.Add(gtx.Ops)
		size := gtx.Dp(unit.Dp(22))
		gtx.Constraints.Min = image.Pt(size, size)
		gtx.Constraints.Max = gtx.Constraints.Min
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("拖动调整图片大小").Add(gtx.Ops)
		paint.FillShape(gtx.Ops, toolbarActiveBackground, clip.UniformRRect(image.Rectangle{Max: image.Pt(size, size)}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
		return layout.Center.Layout(gtx, material.Caption(a.theme, "↘").Layout)
	})
}

func wordImageEMUToPixels(widthEMU int64, pxPerDp float32) int {
	if widthEMU <= 0 || pxPerDp <= 0 {
		return 0
	}
	pixels := float64(widthEMU) * 96 * float64(pxPerDp) / 914400
	if pixels >= float64(math.MaxInt) {
		return math.MaxInt
	}
	return max(1, int(math.Round(pixels)))
}

func resizeWordImageWidthEMU(deltaScreenPx float64, currentWidthEMU int64, displayedWidthPx int) int64 {
	if currentWidthEMU <= 0 || displayedWidthPx <= 0 || math.IsNaN(deltaScreenPx) || math.IsInf(deltaScreenPx, 0) {
		return currentWidthEMU
	}
	delta := int64(math.Round(deltaScreenPx * float64(currentWidthEMU) / float64(displayedWidthPx)))
	if delta > 0 && currentWidthEMU > math.MaxInt64-delta {
		return math.MaxInt64
	}
	if delta < 0 && currentWidthEMU < math.MinInt64-delta {
		return 1
	}
	return max(int64(1), currentWidthEMU+delta)
}

func wordResizeHandleButton(state *documentSessionState, id string) *widget.Clickable {
	if state.wordResizeHandleButtons == nil {
		state.wordResizeHandleButtons = make(map[string]*widget.Clickable)
	}
	button := state.wordResizeHandleButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		state.wordResizeHandleButtons[id] = button
	}
	return button
}

func wordReadOnlyPlaceholder(gtx layout.Context, theme *material.Theme, message string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, gridHeaderFill, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				label := material.Body2(theme, message)
				label.Color = mutedText
				return layout.UniformInset(unit.Dp(7)).Layout(gtx, label.Layout)
			},
		)
	})
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
