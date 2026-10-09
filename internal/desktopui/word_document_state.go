package desktopui

import (
	"glance/internal/document"
	"image"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/widget"
)

type wordImageInteraction struct {
	moveDrag          gesture.Drag
	resizeDrags       [8]gesture.Drag
	rotationDrag      gesture.Drag
	preview           wordImageGesture
	committingPreview wordImagePreview
	committingMode    wordImageGestureMode
	commitPending     bool
	moveStart         f32.Point
	resizeStart       f32.Point
	resizeStartFrame  image.Point
	resizeStartEMU    int64
	rotationStart     f32.Point
	rotationCenter    f32.Point
	rotationStartDeg  float64
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
	state.wordBlocksMu.RLock()
	if state.wordBlocks != nil {
		blocks := cloneWordBlockSnapshot(state.wordBlocks)
		state.wordBlocksMu.RUnlock()
		return blocks
	}
	state.wordBlocksMu.RUnlock()
	if state.word != nil {
		unlock := lockSessionAdapterRead(state)
		blocks := cloneWordBlockSnapshot(state.word.WordBlocks())
		unlock()
		state.wordBlocksMu.Lock()
		if state.wordBlocks == nil {
			state.wordBlocks = blocks
		}
		blocks = cloneWordBlockSnapshot(state.wordBlocks)
		state.wordBlocksMu.Unlock()
		return blocks
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
	state.wordPreferredVerticalXSet = false
	state.selectedWordImageID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordEditor.SetText(run.Text)
	editor := wordRunEditor(state, run)
	if draft, exists := state.wordDraft(run.ID); exists {
		editor.SetText(draft)
	} else {
		committed, exists := state.committedWordText(run.ID)
		if !exists || committed != run.Text {
			state.setCommittedWordText(run.ID, run.Text)
		}
		editor.SetText(run.Text)
	}
	return true
}

func wordRunEditor(state *documentSessionState, run document.WordTextRun) *widget.Editor {
	if state.wordRunEditors == nil {
		state.wordRunEditors = make(map[string]*widget.Editor)
	}
	editor := state.wordRunEditors[run.ID]
	if editor == nil {
		editor = new(widget.Editor)
		state.wordRunEditors[run.ID] = editor
		if draft, exists := state.wordDraft(run.ID); exists {
			editor.SetText(draft)
		} else {
			editor.SetText(run.Text)
			state.setCommittedWordText(run.ID, run.Text)
		}
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
		state.wordForceDraftFlush.Store(true)
		state.wordFlushAllDrafts.Store(true)
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

func clearWordImageSelection(state *documentSessionState) {
	if state == nil {
		return
	}
	state.selectedWordImageID = ""
	state.selectedWordRunID = ""
	state.selectedWordParagraphID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = false
	if state.wordToolbarTab == wordTabPicture {
		state.wordToolbarTab = wordTabHome
	}
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

func wordImageLayoutPreview(interaction *wordImageInteraction) (wordImagePreview, wordImageGestureMode) {
	if interaction == nil {
		return wordImagePreview{}, wordImageGestureNone
	}
	preview := interaction.preview.Preview()
	mode := interaction.preview.mode
	if !preview.Active && interaction.commitPending {
		return interaction.committingPreview, interaction.committingMode
	}
	return preview, mode
}

func finishWordImagePreviewCommand(state *documentSessionState, imageID string) {
	if state == nil || imageID == "" {
		return
	}
	interaction := wordImageInteractionFor(state, imageID)
	interaction.committingPreview = wordImagePreview{}
	interaction.committingMode = wordImageGestureNone
	interaction.commitPending = false
}
