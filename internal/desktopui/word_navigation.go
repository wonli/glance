package desktopui

import (
	"math"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/document"
)

type wordNavigationRun struct {
	run         document.WordTextRun
	blockIndex  int
	paragraphID string
}

type wordNavigationGeometry struct {
	x int
	y int
}

type wordCaretStop struct {
	offset int
	x      int
	y      int
}

type wordNavigationTarget struct {
	runID       string
	paragraphID string
	blockIndex  int
	offset      int
	x           int
	y           int
	byX         bool
}

func wordNavigationRuns(blocks []document.WordBlock) []wordNavigationRun {
	var runs []wordNavigationRun
	appendParagraph := func(paragraph document.WordParagraph, blockIndex int) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			runs = append(runs, wordNavigationRun{run: *inline.Text, paragraphID: paragraph.ID, blockIndex: blockIndex})
		}
	}
	for blockIndex, block := range blocks {
		switch block.Kind {
		case document.WordParagraphBlock:
			if block.Paragraph != nil {
				appendParagraph(*block.Paragraph, blockIndex)
			}
		case document.WordTableBlock:
			if block.Table == nil {
				continue
			}
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						appendParagraph(paragraph, blockIndex)
					}
				}
			}
		}
	}
	return runs
}

func wordNavigationRunOrder(runs []wordNavigationRun) map[string]int {
	order := make(map[string]int, len(runs))
	for index, run := range runs {
		order[run.run.ID] = index
	}
	return order
}

func (a *App) handleWordDocumentBoundaryShortcut(gtx layout.Context, state *documentSessionState, currentRunID string, editor *widget.Editor, toEnd, extendSelection, pageLayout bool) bool {
	if state == nil || editor == nil || !gtx.Focused(editor) || editor.ReadOnly || state.formatRunning.Load() || a.isSaving() {
		return true
	}
	runs := wordNavigationRuns(wordBlocksForState(state))
	if len(runs) == 0 {
		return true
	}
	currentIndex := -1
	for index, run := range runs {
		if run.run.ID == currentRunID {
			currentIndex = index
			break
		}
	}
	if currentIndex < 0 {
		return true
	}
	for pending, ok := editor.Update(gtx); ok; pending, ok = editor.Update(gtx) {
		if _, submit := pending.(widget.SubmitEvent); submit {
			start, end := editor.Selection()
			a.queueWordParagraphSplit(currentRunID, editor.Text(), start, end)
			if state.wordParagraphSplitIntent != nil && state.wordParagraphSplitIntent.runID == currentRunID {
				return true
			}
		}
	}
	if a.finishWordSelectionInput(state, currentRunID, editor) {
		return true
	}
	captureWordEditorDraft(state, currentRunID, editor.Text())

	targetIndex := 0
	targetOffset := 0
	if toEnd {
		targetIndex = len(runs) - 1
		targetOffset = wordRunEditor(state, runs[targetIndex].run).Len()
	}
	target := runs[targetIndex]
	var selection *wordPageTextSelection
	if extendSelection {
		var anchor wordPageCaretPosition
		if wordPageTextSelectionActive(state.wordPageTextSelection) {
			anchor = state.wordPageTextSelection.anchor
		} else {
			start, end := editor.Selection()
			offset := end
			if start != end {
				if toEnd {
					offset = min(start, end)
				} else {
					offset = max(start, end)
				}
			}
			anchor = wordPageCaretPosition{runID: currentRunID, runeOffset: offset}
		}
		selection = &wordPageTextSelection{
			scope:    "range",
			anchor:   anchor,
			focus:    wordPageCaretPosition{runID: target.run.ID, runeOffset: targetOffset},
			runOrder: wordNavigationRunOrder(runs),
		}
	}
	state.wordPageTextSelection = nil
	state.wordPageSelectionDrag = nil
	state.wordPreferredVerticalXSet = false
	state.wordPagePreferredVerticalXSet = false
	if pageLayout {
		a.focusWordPageRun(gtx, state, target.run.ID, targetOffset, targetOffset)
		requestWordPageScrollForKeyboardCaret(state, target.run.ID, targetOffset)
	} else {
		targetEditor := wordRunEditor(state, target.run)
		a.focusWordNavigationRun(gtx, state, target, targetEditor, targetOffset, false, 0)
	}
	state.wordPageTextSelection = selection
	if a.window != nil {
		a.window.Invalidate()
	}
	return true
}

func (a *App) handleWordArrowNavigation(gtx layout.Context, paragraph document.WordParagraph, run document.WordTextRun, editor *widget.Editor) (handled bool) {
	state := a.documentSessionState
	if state == nil || editor == nil || !gtx.Focused(editor) || editor.ReadOnly || state.formatRunning.Load() || a.isSaving() {
		return false
	}
	start, end := editor.Selection()
	selectionActive := start != end
	if selectionActive {
		state.wordPreferredVerticalXSet = false
	}

	// Document-boundary shortcuts do not depend on measured caret geometry. Check
	// them before the flow-navigation geometry guard so they also work on the
	// first frame after focus moves to a run that has not been measured yet.
	if !editorHasActiveComposition(editor) {
		if ev, ok := gtx.Event(
			key.Filter{Focus: editor, Name: key.NameHome, Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Focus: editor, Name: key.NameEnd, Required: key.ModShortcut, Optional: key.ModShift},
		); ok {
			if keyEvent, ok := ev.(key.Event); ok && keyEvent.State == key.Press {
				return a.handleWordDocumentBoundaryShortcut(gtx, state, run.ID, editor,
					keyEvent.Name == key.NameEnd, keyEvent.Modifiers.Contain(key.ModShift), false)
			}
			return true
		}
	}

	if state.wordNavLayouts == nil {
		return false
	}
	if _, measured := state.wordNavLayouts[run.ID]; !measured {
		return false
	}
	filters := make([]event.Filter, 0, 6)
	if !selectionActive {
		filters = append(filters,
			key.Filter{Focus: editor, Name: key.NameLeftArrow},
			key.Filter{Focus: editor, Name: key.NameRightArrow},
			key.Filter{Focus: editor, Name: key.NameUpArrow},
			key.Filter{Focus: editor, Name: key.NameDownArrow},
		)
	}
	if len(filters) == 0 {
		return false
	}
	ev, ok := gtx.Event(filters...)
	if !ok {
		return false
	}
	keyEvent, ok := ev.(key.Event)
	if !ok {
		return true
	}
	beforeStart, beforeEnd, beforeLength := wordDebugEditorSelection(editor)
	defer func() {
		afterEditor := state.wordRunEditors[state.selectedWordRunID]
		afterStart, afterEnd, afterLength := wordDebugEditorSelection(afterEditor)
		logWordDebug("flow_key_navigation revision=%d key=%q state=%d modifiers=%s handled=%v paragraph=%q before_run=%q before_selection=%d:%d before_length=%d after_run=%q after_selection=%d:%d after_length=%d preferred_x=%d preferred_x_set=%v",
			state.wordRevisionAtomic.Load(), keyEvent.Name, keyEvent.State, keyEvent.Modifiers.String(), handled, paragraph.ID,
			run.ID, beforeStart, beforeEnd, beforeLength, state.selectedWordRunID, afterStart, afterEnd, afterLength,
			state.wordPreferredVerticalX, state.wordPreferredVerticalXSet)
	}()
	if keyEvent.State != key.Press {
		return true
	}
	if selectionActive || keyEvent.Modifiers != 0 {
		return true
	}

	blocks := wordBlocksForState(state)
	runs := wordNavigationRuns(blocks)
	currentIndex := -1
	for index, item := range runs {
		if item.run.ID == run.ID {
			currentIndex = index
			break
		}
	}
	if currentIndex < 0 {
		return true
	}
	current := runs[currentIndex]
	geometry := state.wordNavLayouts[run.ID]

	switch keyEvent.Name {
	case key.NameLeftArrow, key.NameRightArrow:
		state.wordPreferredVerticalXSet = false
		direction := 1
		if gtx.Locale.Direction.Progression() == system.TowardOrigin {
			direction = -1
		}
		if keyEvent.Name == key.NameLeftArrow {
			direction = -direction
		}
		if direction < 0 && start == 0 && currentIndex > 0 {
			targetEditor := wordRunEditor(state, runs[currentIndex-1].run)
			a.focusWordNavigationRun(gtx, state, runs[currentIndex-1], targetEditor, targetEditor.Len(), false, 0)
			return true
		}
		if direction > 0 && start == editor.Len() && currentIndex+1 < len(runs) {
			targetEditor := wordRunEditor(state, runs[currentIndex+1].run)
			a.focusWordNavigationRun(gtx, state, runs[currentIndex+1], targetEditor, 0, false, 0)
			return true
		}
		editor.MoveCaret(direction, direction)
		state.selectedWordRunID = run.ID
		state.selectedWordParagraphID = paragraph.ID
		state.selectedWordImageID = ""
		return true
	case key.NameUpArrow, key.NameDownArrow:
		state.wordNavigationVerticalHandledRunID = run.ID
		direction := -1
		if keyEvent.Name == key.NameDownArrow {
			direction = 1
		}
		caret := editor.CaretCoords()
		currentX := geometry.x + int(math.Round(float64(caret.X)))
		if !state.wordPreferredVerticalXSet {
			state.wordPreferredVerticalX = currentX
			state.wordPreferredVerticalXSet = true
		}
		target, found := findWordVerticalCaretTarget(state, runs, currentIndex, direction, state.wordPreferredVerticalX, gtx.Dp(unit.Dp(8)))
		if found {
			a.focusWordNavigationTarget(gtx, state, current, target)
		} else {
			state.selectedWordRunID = run.ID
			state.selectedWordParagraphID = paragraph.ID
		}
		return true
	}
	return true
}

func findWordVerticalCaretTarget(state *documentSessionState, runs []wordNavigationRun, currentIndex, direction, preferredX, rowTolerance int) (wordNavigationTarget, bool) {
	if state == nil || currentIndex < 0 || currentIndex >= len(runs) {
		return wordNavigationTarget{}, false
	}
	current := runs[currentIndex]
	currentGeometry, ok := state.wordNavLayouts[current.run.ID]
	currentEditor := state.wordRunEditors[current.run.ID]
	if !ok || currentEditor == nil {
		return wordNavigationTarget{}, false
	}
	currentCaret := currentEditor.CaretCoords()
	currentY := currentGeometry.y + int(math.Round(float64(currentCaret.Y)))
	if stop, found := wordNearestVisualLineStop(currentEditor, direction, preferredX-currentGeometry.x); found {
		return wordNavigationTarget{runID: current.run.ID, paragraphID: current.paragraphID, blockIndex: current.blockIndex, offset: stop.offset}, true
	}

	best := wordNavigationTarget{}
	bestVerticalDistance := math.MaxInt
	bestHorizontalDistance := math.MaxInt
	for _, item := range runs {
		if item.paragraphID != current.paragraphID || item.run.ID == current.run.ID {
			continue
		}
		geometry, measured := state.wordNavLayouts[item.run.ID]
		editor := state.wordRunEditors[item.run.ID]
		if !measured || editor == nil {
			continue
		}
		for _, stop := range wordEditorCaretStops(editor) {
			x, y := geometry.x+stop.x, geometry.y+stop.y
			verticalDistance := currentY - y
			if direction > 0 {
				verticalDistance = y - currentY
			}
			if verticalDistance < rowTolerance {
				continue
			}
			horizontalDistance := absWordInt(x - preferredX)
			if verticalDistance < bestVerticalDistance || (verticalDistance == bestVerticalDistance && horizontalDistance < bestHorizontalDistance) {
				bestVerticalDistance = verticalDistance
				bestHorizontalDistance = horizontalDistance
				best = wordNavigationTarget{runID: item.run.ID, paragraphID: item.paragraphID, blockIndex: item.blockIndex, offset: stop.offset}
			}
		}
	}
	if best.runID != "" {
		return best, true
	}

	paragraphIDs := make([]string, 0, len(runs))
	for _, item := range runs {
		if len(paragraphIDs) == 0 || paragraphIDs[len(paragraphIDs)-1] != item.paragraphID {
			paragraphIDs = append(paragraphIDs, item.paragraphID)
		}
	}
	paragraphIndex := -1
	for index, paragraphID := range paragraphIDs {
		if paragraphID == current.paragraphID {
			paragraphIndex = index
			break
		}
	}
	paragraphIndex += direction
	if paragraphIndex < 0 || paragraphIndex >= len(paragraphIDs) {
		return wordNavigationTarget{}, false
	}
	targetParagraphID := paragraphIDs[paragraphIndex]
	var paragraphRuns []wordNavigationRun
	for _, item := range runs {
		if item.paragraphID == targetParagraphID {
			paragraphRuns = append(paragraphRuns, item)
		}
	}
	if len(paragraphRuns) == 0 {
		return wordNavigationTarget{}, false
	}

	var edgeY int
	edgeSet := false
	var candidates []wordNavigationTarget
	for _, item := range paragraphRuns {
		geometry, measured := state.wordNavLayouts[item.run.ID]
		editor := state.wordRunEditors[item.run.ID]
		if !measured || editor == nil {
			continue
		}
		for _, stop := range wordEditorCaretStops(editor) {
			y := geometry.y + stop.y
			if !edgeSet || (direction < 0 && y > edgeY) || (direction > 0 && y < edgeY) {
				edgeY, edgeSet = y, true
			}
			candidates = append(candidates, wordNavigationTarget{runID: item.run.ID, paragraphID: item.paragraphID, blockIndex: item.blockIndex, offset: stop.offset, x: geometry.x + stop.x, y: y})
		}
	}
	if edgeSet {
		best := wordNavigationTarget{}
		bestDistance := math.MaxInt
		for _, candidate := range candidates {
			if absWordInt(candidate.y-edgeY) > rowTolerance {
				continue
			}
			distance := absWordInt(candidate.x - preferredX)
			if distance < bestDistance {
				bestDistance = distance
				best = candidate
			}
		}
		if best.runID != "" {
			return best, true
		}
	}

	item := paragraphRuns[0]
	offset := 0
	if direction < 0 {
		item = paragraphRuns[len(paragraphRuns)-1]
		if editor := state.wordRunEditors[item.run.ID]; editor != nil {
			offset = editor.Len()
		} else {
			offset = len([]rune(item.run.Text))
		}
	}
	return wordNavigationTarget{runID: item.run.ID, paragraphID: item.paragraphID, blockIndex: item.blockIndex, offset: offset, x: preferredX, byX: true}, true
}

func wordNearestVisualLineStop(editor *widget.Editor, direction, preferredX int) (wordCaretStop, bool) {
	if editor == nil {
		return wordCaretStop{}, false
	}
	start, end := editor.Selection()
	startLine, _ := editor.CaretPos()
	defer editor.SetCaret(start, end)
	step := 1
	if direction < 0 {
		step = -1
	}
	targetLine := -1
	best := wordCaretStop{}
	bestDistance := math.MaxInt
	editor.SetCaret(start, end)
	for count := 0; count <= editor.Len()+1; count++ {
		previousStart, previousEnd := editor.Selection()
		editor.MoveCaret(step, step)
		caretStart, caretEnd := editor.Selection()
		if caretStart == previousStart && caretEnd == previousEnd {
			break
		}
		line, _ := editor.CaretPos()
		if targetLine < 0 {
			if direction < 0 && line < startLine || direction > 0 && line > startLine {
				targetLine = line
			}
		}
		if targetLine >= 0 {
			if line != targetLine {
				if direction < 0 && line < targetLine || direction > 0 && line > targetLine {
					break
				}
			} else {
				coords := editor.CaretCoords()
				stop := wordCaretStop{offset: caretStart, x: int(math.Round(float64(coords.X))), y: int(math.Round(float64(coords.Y)))}
				distance := absWordInt(stop.x - preferredX)
				if distance < bestDistance {
					best, bestDistance = stop, distance
				}
			}
		}
	}
	if bestDistance == math.MaxInt {
		return wordCaretStop{}, false
	}
	return best, true
}

func wordEditorCaretStops(editor *widget.Editor) []wordCaretStop {
	if editor == nil {
		return nil
	}
	start, end := editor.Selection()
	defer editor.SetCaret(start, end)
	editor.SetCaret(0, 0)
	var stops []wordCaretStop
	lastStart, lastEnd := -1, -1
	for count := 0; count <= editor.Len()+1; count++ {
		caretStart, caretEnd := editor.Selection()
		if caretStart == lastStart && caretEnd == lastEnd {
			break
		}
		coords := editor.CaretCoords()
		stops = append(stops, wordCaretStop{offset: caretStart, x: int(math.Round(float64(coords.X))), y: int(math.Round(float64(coords.Y)))})
		lastStart, lastEnd = caretStart, caretEnd
		if caretStart >= editor.Len() {
			break
		}
		editor.MoveCaret(1, 1)
	}
	return stops
}

func (a *App) focusWordNavigationRun(gtx layout.Context, state *documentSessionState, target wordNavigationRun, editor *widget.Editor, offset int, byX bool, x int) {
	if editor == nil {
		editor = wordRunEditor(state, target.run)
	}
	state.selectedWordRunID = target.run.ID
	state.selectedWordParagraphID = target.paragraphID
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordPendingFocusRunID = target.run.ID
	state.wordPendingCaretRunID = target.run.ID
	state.wordPendingCaretOffset = offset
	state.wordPendingCaretByX = byX
	state.wordPendingCaretX = x
	state.wordListState.List.ScrollTo(target.blockIndex)
	gtx.Execute(op.InvalidateCmd{})
	if _, visible := state.wordNavLayouts[target.run.ID]; visible {
		if byX {
			localX := x - state.wordNavLayouts[target.run.ID].x
			offset := wordClosestCaretOffsetToX(editor, localX)
			editor.SetCaret(offset, offset)
		} else {
			editor.SetCaret(offset, offset)
		}
		state.wordPendingCaretRunID = ""
		state.wordPendingCaretByX = false
		gtx.Execute(key.FocusCmd{Tag: editor})
	}
}

func (a *App) focusWordNavigationTarget(gtx layout.Context, state *documentSessionState, current wordNavigationRun, target wordNavigationTarget) {
	if target.runID == current.run.ID {
		if target.byX {
			editor := state.wordRunEditors[target.runID]
			if editor != nil {
				offset := wordClosestCaretOffsetToX(editor, target.x-state.wordNavLayouts[target.runID].x)
				editor.SetCaret(offset, offset)
			}
		} else if editor := state.wordRunEditors[target.runID]; editor != nil {
			editor.SetCaret(target.offset, target.offset)
		}
		return
	}
	var targetRun wordNavigationRun
	for _, item := range wordNavigationRuns(wordBlocksForState(state)) {
		if item.run.ID == target.runID {
			targetRun = item
			break
		}
	}
	if targetRun.run.ID == "" {
		return
	}
	a.focusWordNavigationRun(gtx, state, targetRun, state.wordRunEditors[target.runID], target.offset, target.byX, target.x)
}

func wordClosestCaretOffsetToX(editor *widget.Editor, targetX int) int {
	bestOffset, bestDistance := 0, math.MaxInt
	for _, stop := range wordEditorCaretStops(editor) {
		distance := absWordInt(stop.x - targetX)
		if distance < bestDistance {
			bestOffset, bestDistance = stop.offset, distance
		}
	}
	return bestOffset
}

func absWordInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
