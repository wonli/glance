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
	"gioui.org/io/key"
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
	"golang.org/x/image/math/fixed"

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
	editor := wordRunEditor(state, run)
	if _, draft := state.wordRunDrafts[run.ID]; !draft {
		editor.SetText(run.Text)
		state.wordRunCommitted[run.ID] = run.Text
	}
	return true
}

func wordRunEditor(state *documentSessionState, run document.WordTextRun) *widget.Editor {
	if state.wordRunEditors == nil {
		state.wordRunEditors = make(map[string]*widget.Editor)
	}
	if state.wordRunCommitted == nil {
		state.wordRunCommitted = make(map[string]string)
	}
	if state.wordRunDrafts == nil {
		state.wordRunDrafts = make(map[string]string)
	}
	editor := state.wordRunEditors[run.ID]
	if editor == nil {
		editor = new(widget.Editor)
		state.wordRunEditors[run.ID] = editor
		editor.SetText(run.Text)
		state.wordRunCommitted[run.ID] = run.Text
	} else if _, draft := state.wordRunDrafts[run.ID]; !draft && state.wordRunCommitted[run.ID] != run.Text {
		editor.SetText(run.Text)
		state.wordRunCommitted[run.ID] = run.Text
	}
	editor.SingleLine = false
	editor.Submit = true
	return editor
}

func selectWordImage(state *documentSessionState, imageID string) bool {
	if state == nil || state.word == nil {
		return false
	}
	blocks := wordBlocksForState(state)
	if findWordImage(blocks, imageID) == nil {
		return false
	}
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush = true
		state.wordFlushAllDrafts = true
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
	state.wordLayoutFocusedRunID = ""
	dimensions := layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutWordToolbar),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordCanvas(gtx, state, blocks)
				}),
			)
		}),
		layout.Stacked(a.layoutWordToolbarColorPicker),
	)
	if focused := state.wordLayoutFocusedRunID; focused != "" {
		if wordHasPendingTextDrafts(state) && (focused != state.wordFocusedRunID || state.wordForceDraftFlush) {
			state.wordFlushRequested = true
			state.wordFlushAllDrafts = state.wordFlushAllDrafts || state.wordForceDraftFlush
		}
		state.wordFocusedRunID = focused
	} else {
		if wordHasPendingTextDrafts(state) {
			state.wordFlushRequested = true
			state.wordFlushAllDrafts = true
		}
		state.wordFocusedRunID = ""
	}
	state.wordForceDraftFlush = false
	if state.wordFlushRequested && !state.formatRunning {
		a.startNextWordTextCommit()
	}
	return dimensions
}

const wordPaperMaxWidthDP = 816

func wordPageGeometry(availableWidth, maxWidth int) (pageWidth, left int) {
	pageWidth = min(max(0, availableWidth), max(0, maxWidth))
	left = max(0, (availableWidth-pageWidth)/2)
	return pageWidth, left
}

func wordPageHorizontalMargin(pageWidth, preferred int) int {
	return min(max(0, preferred), max(0, (pageWidth-160)/2))
}

func (a *App) layoutWordCanvas(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				pageWidth, _ := wordPageGeometry(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(wordPaperMaxWidthDP)))
				gtx.Constraints.Min.X = pageWidth
				gtx.Constraints.Max.X = pageWidth
				margin := wordPageHorizontalMargin(pageWidth, gtx.Dp(unit.Dp(72)))
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
						return layout.Dimensions{Size: gtx.Constraints.Max}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(28), Bottom: unit.Dp(28), Left: unit.Dp(margin), Right: unit.Dp(margin)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutWordList(gtx, state, blocks)
						})
					},
				)
			})
		},
	)
}

var wordWorkspaceBackground = color.NRGBA{R: 238, G: 241, B: 245, A: 255}

func (a *App) layoutWordList(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) layout.Dimensions {
	state.wordListState.List.Axis = layout.Vertical
	const scrollLimit = 1 << 30
	positionDelta := state.wordBodyScroll.Update(
		gtx.Metric,
		gtx.Source,
		gtx.Now,
		gesture.Vertical,
		pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
		pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
	)
	if positionDelta != 0 {
		state.wordListState.Position.Offset += positionDelta
	}
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return material.List(a.theme, &state.wordListState).Layout(gtx, len(blocks)+1, func(gtx layout.Context, index int) layout.Dimensions {
				if index == len(blocks) {
					return a.layoutWordAppendTarget(gtx, state)
				}
				return a.layoutWordBlock(gtx, blocks[index])
			})
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			pass := pointer.PassOp{}.Push(gtx.Ops)
			state.wordBodyScroll.Add(gtx.Ops)
			pass.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)
}

func (a *App) layoutWordAppendTarget(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	if state.wordAppendTarget.Clicked(gtx) && !state.wordInsertionRunning {
		state.wordForceDraftFlush = true
		state.wordInsertAfterDraftFlush = true
		state.wordFlushRequested = true
		state.wordFlushAllDrafts = true
	}
	return state.wordAppendTarget.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(560))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("单击此处继续输入").Add(gtx.Ops)
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				if !state.wordAppendTarget.Hovered() {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				}
				return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Caption(a.theme, "单击此处继续输入").Layout(gtx)
				})
			},
		)
	})
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
	children := make([]wordInlineLayoutChild, 0, len(paragraph.Inlines))
	maxWidth := max(1, gtx.Constraints.Max.X)
	for _, inline := range paragraph.Inlines {
		inline := inline
		switch {
		case inline.Text != nil:
			run := *inline.Text
			children = append(children, wordInlineLayoutChild{width: wordRunPreferredWidth(gtx, a.theme, paragraph, run), layout: func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordTextRun(gtx, paragraph, *inline.Text)
			}})
		case inline.Image != nil:
			item := *inline.Image
			children = append(children, wordInlineLayoutChild{width: a.wordImagePreferredWidth(gtx, item, maxWidth), layout: func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordImage(gtx, paragraph, *inline.Image)
			}})
		case inline.Unsupported != nil:
			children = append(children, wordInlineLayoutChild{width: min(maxWidth, gtx.Dp(unit.Dp(240))), layout: func(gtx layout.Context) layout.Dimensions {
				return wordReadOnlyPlaceholder(gtx, a.theme, inline.Unsupported.Description)
			}})
		}
	}
	spacing := unit.Dp(6)
	if inCell {
		spacing = unit.Dp(3)
	}
	return layout.Inset{Top: unit.Dp(5), Bottom: spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layoutWordInlineFlow(gtx, paragraph.Alignment, children)
	})
}

func (a *App) layoutWordTextRun(gtx layout.Context, paragraph document.WordParagraph, run document.WordTextRun) layout.Dimensions {
	state := a.documentSessionState
	editor := wordRunEditor(state, run)
	selected := state.selectedWordRunID == run.ID
	label := wordRunLabel(a.theme, paragraph, run)
	focusTargetPending := state.wordPendingFocusRunID != "" && state.wordPendingFocusRunID != run.ID
	paragraphSplitPending := state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == run.ID
	editor.ReadOnly = !run.Editable || a.saving || paragraphSplitPending || focusTargetPending
	if !editor.ReadOnly {
		for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
			if _, submit := event.(widget.SubmitEvent); submit {
				selectionStart, selectionEnd := editor.Selection()
				a.queueWordParagraphSplit(run.ID, editor.Text(), selectionStart, selectionEnd)
				if state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == run.ID {
					editor.ReadOnly = true
				}
			}
		}
	}
	editor.Alignment = wordParagraphAlignment(paragraph.Alignment)
	style := material.Editor(a.theme, editor, "")
	style.Font, style.TextSize, style.Color = label.Font, label.TextSize, label.Color
	background := contentSurface
	if selected {
		background = selectedFill
	}
	dimensions := wordMeasuredBackground(gtx, background, func(gtx layout.Context) layout.Dimensions {
		dims := layoutWordRunEditor(gtx, style, run.ID)
		if run.Style.Underline && dims.Size.X > 0 && dims.Size.Y > 0 {
			y := max(0, dims.Baseline)
			thickness := max(1, gtx.Dp(unit.Dp(1)))
			paint.FillShape(gtx.Ops, label.Color, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(dims.Size.X, y+thickness)}.Op())
		}
		return dims
	})
	if gtx.Focused(editor) && run.Editable && !focusTargetPending {
		state.selectedWordRunID = run.ID
		state.selectedWordParagraphID = paragraph.ID
		state.selectedWordImageID = ""
		state.selectedBlock = -1
		state.selectedBlockEditable = true
		state.wordToolbarTab = wordTabHome
		state.wordLayoutFocusedRunID = run.ID
	}
	if run.Editable {
		current := editor.Text()
		captureWordTextDraft(state, run.ID, current)
		if current != state.wordRunCommitted[run.ID] {
			state.selectedWordRunID = run.ID
			state.selectedWordParagraphID = paragraph.ID
		}
	}
	if state.wordPendingFocusRunID == run.ID {
		gtx.Execute(key.FocusCmd{Tag: editor})
		state.wordPendingFocusRunID = ""
	}
	return dimensions
}

func layoutWordRunEditor(gtx layout.Context, style material.EditorStyle, runID string) layout.Dimensions {
	recorder := op.Record(gtx.Ops)
	dimensions := style.Layout(gtx)
	call := recorder.Stop()
	clip := clip.Rect{Max: dimensions.Size}.Push(gtx.Ops)
	semantic.DescriptionOp("正文文字 " + runID).Add(gtx.Ops)
	call.Add(gtx.Ops)
	clip.Pop()
	return dimensions
}

type wordInlineLayoutChild struct {
	width  int
	layout layout.Widget
}

func wordInlineGeometry(maxWidth int, sizes []image.Point, alignment document.WordAlignment) ([]image.Point, int) {
	if len(sizes) == 0 || maxWidth <= 0 {
		return make([]image.Point, len(sizes)), 0
	}
	positions := make([]image.Point, len(sizes))
	lineWidths := []int{0}
	lineHeights := []int{0}
	lineForItem := make([]int, len(sizes))
	line := 0
	for index, size := range sizes {
		width := min(maxWidth, max(0, size.X))
		if lineWidths[line] > 0 && lineWidths[line]+width > maxWidth {
			line++
			lineWidths = append(lineWidths, 0)
			lineHeights = append(lineHeights, 0)
		}
		positions[index].X = lineWidths[line]
		lineWidths[line] += width
		lineHeights[line] = max(lineHeights[line], max(0, size.Y))
		lineForItem[index] = line
	}
	lineTops := make([]int, len(lineHeights))
	for index := 1; index < len(lineTops); index++ {
		lineTops[index] = lineTops[index-1] + lineHeights[index-1]
	}
	for index, line := range lineForItem {
		positions[index].Y = lineTops[line]
		alignOffset := 0
		switch alignment {
		case document.WordAlignCenter:
			alignOffset = (maxWidth - lineWidths[line]) / 2
		case document.WordAlignRight:
			alignOffset = maxWidth - lineWidths[line]
		}
		positions[index].X += max(0, alignOffset)
	}
	totalHeight := lineTops[len(lineTops)-1] + lineHeights[len(lineHeights)-1]
	return positions, totalHeight
}

func layoutWordInlineFlow(gtx layout.Context, alignment document.WordAlignment, children []wordInlineLayoutChild) layout.Dimensions {
	maxWidth := max(1, gtx.Constraints.Max.X)
	type measuredChild struct {
		call op.CallOp
		size image.Point
	}
	measured := make([]measuredChild, len(children))
	sizes := make([]image.Point, len(children))
	for index, child := range children {
		width := min(maxWidth, max(1, child.width))
		childGtx := gtx
		childGtx.Constraints.Min.X = width
		childGtx.Constraints.Max.X = width
		recorder := op.Record(gtx.Ops)
		dims := child.layout(childGtx)
		call := recorder.Stop()
		measured[index] = measuredChild{call: call, size: dims.Size}
		sizes[index] = dims.Size
	}
	positions, height := wordInlineGeometry(maxWidth, sizes, alignment)
	for index, child := range measured {
		transform := op.Offset(positions[index]).Push(gtx.Ops)
		child.call.Add(gtx.Ops)
		transform.Pop()
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, height))}
}

func wordRunPreferredWidth(gtx layout.Context, theme *material.Theme, paragraph document.WordParagraph, run document.WordTextRun) int {
	if run.Text == "" {
		return max(1, gtx.Dp(unit.Dp(12)))
	}
	label := wordRunLabel(theme, paragraph, run)
	theme.Shaper.LayoutString(text.Parameters{Font: label.Font, PxPerEm: fixed.I(gtx.Sp(label.TextSize)), MaxWidth: 1 << 20, Locale: gtx.Locale}, run.Text)
	var width fixed.Int26_6
	for glyph, ok := theme.Shaper.NextGlyph(); ok; glyph, ok = theme.Shaper.NextGlyph() {
		right := glyph.X + glyph.Advance
		if right > width {
			width = right
		}
	}
	pixels := (int(width) + 63) / 64
	return max(1, pixels+gtx.Dp(unit.Dp(4)))
}

func (a *App) wordImagePreferredWidth(gtx layout.Context, item document.WordImage, maxWidth int) int {
	if decoded, ok := a.documentSessionState.wordImageCache.Get(item.ID); ok {
		width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
		if item.WidthEMU > 0 && item.HeightEMU > 0 {
			width = wordImageEMUToPixels(item.WidthEMU, gtx.Metric.PxPerDp)
			height = wordImageEMUToPixels(item.HeightEMU, gtx.Metric.PxPerDp)
		}
		return max(1, fitWordImageSize(width, height, maxWidth, gtx.Dp(unit.Dp(360))).X)
	}
	width, height := wordImageEMUToPixels(item.WidthEMU, gtx.Metric.PxPerDp), wordImageEMUToPixels(item.HeightEMU, gtx.Metric.PxPerDp)
	if width <= 0 || height <= 0 {
		return min(maxWidth, gtx.Dp(unit.Dp(240)))
	}
	return max(1, fitWordImageSize(width, height, maxWidth, gtx.Dp(unit.Dp(360))).X)
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
			if editor := state.wordRunEditors[run.ID]; editor != nil {
				if _, draft := state.wordRunDrafts[run.ID]; !draft {
					if editor.Text() != run.Text {
						editor.SetText(run.Text)
					}
					state.wordRunCommitted[run.ID] = run.Text
				}
			}
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
	maxWidth := max(1, gtx.Constraints.Max.X)
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
