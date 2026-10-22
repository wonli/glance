package desktopui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/i18n"
	"glance/internal/wordlayout"
)

type wordTableSelection struct {
	documentID   string
	tableID      string
	anchorCellID string
	focusCellID  string
}

type wordTableSelectionDeleteIntent struct {
	documentID string
	tableID    string
	cellIDs    []string
}

type wordTableCaretSnapshot struct {
	selectedRunID, selectedParagraphID   string
	focusedRunID, layoutFocusedRunID     string
	focusRunID                           string
	pendingFocusRunID, pendingCaretRunID string
	pendingCaretOffset                   int
	pendingCaretByX                      bool
	selectedBlock                        int
	selectedBlockEditable                bool
	caretStart, caretEnd                 int
	pageTextSelection                    *wordPageTextSelection
	focusWasSet                          bool
}

type wordTableSelectionDrag struct {
	pointerID    pointer.ID
	viewMode     wordViewMode
	pageIndex    int
	documentID   string
	tableID      string
	anchorCellID string
	startCellID  string
	origin       f32.Point
	startedRange bool
	caret        *wordTableCaretSnapshot
}

type wordTablePointerTag struct{}

type wordTableGridCell struct {
	id      string
	ownerID string
	row     int
	index   int
	column  int
	span    int
	endRow  int
}

type wordTableGrid struct {
	cells       []wordTableGridCell
	byID        map[string]int
	ownerByID   map[string]string
	columnCount int
	rowCount    int
}

type wordTableGridBounds struct {
	firstRow, lastRow       int
	firstColumn, lastColumn int
}

var wordTableSelectionColor = color.NRGBA{R: 62, G: 132, B: 222, A: 72}

func buildWordTableGrid(table document.WordTable) (wordTableGrid, error) {
	if table.HasLegacyHorizontalMerge {
		return wordTableGrid{}, fmt.Errorf("table uses unsupported legacy horizontal merges")
	}
	grid := wordTableGrid{
		byID:      make(map[string]int),
		ownerByID: make(map[string]string),
		rowCount:  len(table.Rows),
	}
	if len(table.Rows) == 0 {
		return wordTableGrid{}, fmt.Errorf("table has no rows")
	}
	active := make(map[int]wordTableGridCell)
	for rowIndex, row := range table.Rows {
		next := make(map[int]wordTableGridCell)
		column := 0
		for cellIndex, source := range row.Cells {
			if source.ID == "" {
				return wordTableGrid{}, fmt.Errorf("table row %d contains a cell without an ID", rowIndex)
			}
			if _, exists := grid.byID[source.ID]; exists {
				return wordTableGrid{}, fmt.Errorf("table contains duplicate cell ID %q", source.ID)
			}
			span := source.GridSpan
			if span == 0 {
				span = 1
			}
			if span < 1 || span > 4096 || (len(table.Layout.GridColumnsEMU) > 0 && (column > len(table.Layout.GridColumnsEMU) || span > len(table.Layout.GridColumnsEMU)-column)) {
				return wordTableGrid{}, fmt.Errorf("cell %q has invalid grid span", source.ID)
			}
			cell := wordTableGridCell{id: source.ID, ownerID: source.ID, row: rowIndex, index: cellIndex, column: column, span: span, endRow: rowIndex}
			switch source.VerticalMerge {
			case document.WordTableVerticalMergeNone:
			case document.WordTableVerticalMergeRestart:
				next[column] = cell
			case document.WordTableVerticalMergeContinue:
				owner, exists := active[column]
				if !exists || owner.span != span {
					return wordTableGrid{}, fmt.Errorf("cell %q has an unmatched vertical merge continuation", source.ID)
				}
				cell.ownerID = owner.ownerID
				owner.endRow = rowIndex
				next[column] = owner
			default:
				return wordTableGrid{}, fmt.Errorf("cell %q has an unknown vertical merge state", source.ID)
			}
			grid.byID[cell.id] = len(grid.cells)
			grid.ownerByID[cell.id] = cell.ownerID
			grid.cells = append(grid.cells, cell)
			column += span
		}
		if len(table.Layout.GridColumnsEMU) > 0 && column > len(table.Layout.GridColumnsEMU) {
			return wordTableGrid{}, fmt.Errorf("table row %d extends beyond its declared grid", rowIndex)
		}
		grid.columnCount = max(grid.columnCount, column)
		active = next
	}
	if len(table.Layout.GridColumnsEMU) > 0 {
		grid.columnCount = len(table.Layout.GridColumnsEMU)
	}
	// Propagate the final extent of each vertical-merge owner to every source
	// cell in the merged region, including the owner row.
	mergeEnds := make(map[string]int)
	for _, cell := range grid.cells {
		if cell.ownerID != cell.id {
			mergeEnds[cell.ownerID] = max(mergeEnds[cell.ownerID], cell.row)
		}
	}
	for index := range grid.cells {
		if end := mergeEnds[grid.cells[index].ownerID]; end > grid.cells[index].endRow {
			grid.cells[index].endRow = end
		}
	}
	return grid, nil
}

func wordTableCellRange(table document.WordTable, anchorCellID, focusCellID string) ([]string, error) {
	grid, err := buildWordTableGrid(table)
	if err != nil {
		return nil, err
	}
	anchorIndex, anchorOK := grid.byID[anchorCellID]
	focusIndex, focusOK := grid.byID[focusCellID]
	if !anchorOK || !focusOK {
		return nil, fmt.Errorf("table selection references a stale cell")
	}
	anchor := grid.cells[anchorIndex]
	focus := grid.cells[focusIndex]
	bounds := wordTableGridBounds{
		firstRow: min(anchor.row, focus.row), lastRow: max(anchor.row, focus.row),
		firstColumn: min(anchor.column, focus.column), lastColumn: max(anchor.column+anchor.span-1, focus.column+focus.span-1),
	}
	for {
		previous := bounds
		for _, cell := range grid.cells {
			if !wordTableCellIntersectsBounds(cell, bounds) {
				continue
			}
			bounds.firstRow = min(bounds.firstRow, cell.row)
			bounds.lastRow = max(bounds.lastRow, cell.endRow)
			bounds.firstColumn = min(bounds.firstColumn, cell.column)
			bounds.lastColumn = max(bounds.lastColumn, cell.column+cell.span-1)
		}
		if bounds == previous {
			break
		}
	}
	return wordTableGridOwnersInBounds(grid, bounds), nil
}

func wordTableCellIntersectsBounds(cell wordTableGridCell, bounds wordTableGridBounds) bool {
	return cell.row <= bounds.lastRow && cell.endRow >= bounds.firstRow && cell.column <= bounds.lastColumn && cell.column+cell.span-1 >= bounds.firstColumn
}

func wordTableGridOwnersInBounds(grid wordTableGrid, bounds wordTableGridBounds) []string {
	owners := make([]string, 0)
	seen := make(map[string]struct{})
	for _, cell := range grid.cells {
		if !wordTableCellIntersectsBounds(cell, bounds) {
			continue
		}
		if _, exists := seen[cell.ownerID]; exists {
			continue
		}
		seen[cell.ownerID] = struct{}{}
		owners = append(owners, cell.ownerID)
	}
	return owners
}

func wordTableSelectionTSV(table document.WordTable, cellIDs []string, drafts map[string]string) (string, error) {
	grid, err := buildWordTableGrid(table)
	if err != nil {
		return "", err
	}
	selected := make(map[string]struct{}, len(cellIDs))
	var bounds wordTableGridBounds
	first := true
	for _, id := range cellIDs {
		index, ok := grid.byID[id]
		if !ok {
			return "", fmt.Errorf("table selection references stale cell %q", id)
		}
		ownerID := grid.cells[index].ownerID
		if _, duplicate := selected[ownerID]; duplicate {
			continue
		}
		selected[ownerID] = struct{}{}
		ownerIndex, ok := grid.byID[ownerID]
		if !ok {
			return "", fmt.Errorf("table selection references missing merge owner %q", ownerID)
		}
		owner := grid.cells[ownerIndex]
		if first {
			bounds = wordTableGridBounds{firstRow: owner.row, lastRow: owner.endRow, firstColumn: owner.column, lastColumn: owner.column + owner.span - 1}
			first = false
		} else {
			bounds.firstRow = min(bounds.firstRow, owner.row)
			bounds.lastRow = max(bounds.lastRow, owner.endRow)
			bounds.firstColumn = min(bounds.firstColumn, owner.column)
			bounds.lastColumn = max(bounds.lastColumn, owner.column+owner.span-1)
		}
	}
	if first {
		return "", fmt.Errorf("table selection is empty")
	}
	rows := make([]string, 0, bounds.lastRow-bounds.firstRow+1)
	cellAt := make(map[[2]int]wordTableGridCell, len(grid.cells))
	for _, cell := range grid.cells {
		for row := cell.row; row <= cell.endRow; row++ {
			for column := cell.column; column < cell.column+cell.span; column++ {
				cellAt[[2]int{row, column}] = cell
			}
		}
	}
	for row := bounds.firstRow; row <= bounds.lastRow; row++ {
		values := make([]string, 0, bounds.lastColumn-bounds.firstColumn+1)
		for column := bounds.firstColumn; column <= bounds.lastColumn; column++ {
			cell, exists := cellAt[[2]int{row, column}]
			if !exists || cell.row != row || cell.column != column || cell.ownerID != cell.id {
				values = append(values, "")
				continue
			}
			if _, isSelected := selected[cell.ownerID]; !isSelected {
				values = append(values, "")
				continue
			}
			values = append(values, wordTableCellClipboardText(table.Rows[cell.row].Cells[cell.index], drafts))
		}
		rows = append(rows, strings.Join(values, "\t"))
	}
	return strings.Join(rows, "\n"), nil
}

func wordTableCellClipboardText(cell document.WordTableCell, drafts map[string]string) string {
	paragraphs := make([]string, 0, len(cell.Paragraphs))
	for _, paragraph := range cell.Paragraphs {
		var text strings.Builder
		for _, inline := range paragraph.Inlines {
			switch inline.Kind {
			case document.WordTextInline:
				if inline.Text == nil {
					continue
				}
				value := inline.Text.Text
				if draft, ok := drafts[inline.Text.ID]; ok {
					value = draft
				}
				text.WriteString(value)
			case document.WordImageInline:
				if inline.Image != nil {
					text.WriteString("[图片]")
				}
			case document.WordTabInline:
				text.WriteByte('\t')
			case document.WordPageBreakInline:
				text.WriteByte('\n')
			case document.WordFieldCodeInline:
				if inline.FieldCode != nil {
					text.WriteString(inline.FieldCode.Value)
				}
			}
		}
		paragraphs = append(paragraphs, text.String())
	}
	return strings.Join(paragraphs, "\n")
}

func wordTableForID(state *documentSessionState, tableID string) (document.WordTable, bool) {
	if state == nil || tableID == "" {
		return document.WordTable{}, false
	}
	for _, block := range wordBlocksForState(state) {
		if block.ID == tableID && block.Kind == document.WordTableBlock && block.Table != nil {
			return *block.Table, true
		}
	}
	return document.WordTable{}, false
}

func setWordTableSelection(state *documentSessionState, tableID, anchorCellID, focusCellID string) bool {
	if state == nil {
		return false
	}
	table, ok := wordTableForID(state, tableID)
	if !ok {
		state.wordTableSelection = nil
		return false
	}
	if _, err := wordTableCellRange(table, anchorCellID, focusCellID); err != nil {
		state.wordTableSelection = nil
		return false
	}
	state.wordTableSelection = &wordTableSelection{
		documentID: state.documentID, tableID: tableID, anchorCellID: anchorCellID, focusCellID: focusCellID,
	}
	return true
}

func captureWordTableCaretSnapshot(gtx layout.Context, state *documentSessionState) *wordTableCaretSnapshot {
	if state == nil {
		return nil
	}
	snapshot := &wordTableCaretSnapshot{
		selectedRunID: state.selectedWordRunID, selectedParagraphID: state.selectedWordParagraphID,
		focusedRunID: state.wordFocusedRunID, layoutFocusedRunID: state.wordLayoutFocusedRunID,
		pendingFocusRunID: state.wordPendingFocusRunID, pendingCaretRunID: state.wordPendingCaretRunID,
		pendingCaretOffset: state.wordPendingCaretOffset, pendingCaretByX: state.wordPendingCaretByX,
		selectedBlock: state.selectedBlock, selectedBlockEditable: state.selectedBlockEditable,
		pageTextSelection: state.wordPageTextSelection,
	}
	if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
		snapshot.caretStart, snapshot.caretEnd = editor.Selection()
	}
	for runID, editor := range state.wordRunEditors {
		if gtx.Focused(editor) {
			snapshot.focusRunID = runID
			snapshot.focusWasSet = true
			break
		}
	}
	return snapshot
}

func restoreWordTableCaretSnapshot(gtx layout.Context, state *documentSessionState, snapshot *wordTableCaretSnapshot) {
	if state == nil || snapshot == nil {
		return
	}
	state.selectedWordRunID = snapshot.selectedRunID
	state.selectedWordParagraphID = snapshot.selectedParagraphID
	state.wordFocusedRunID = snapshot.focusedRunID
	state.wordLayoutFocusedRunID = snapshot.layoutFocusedRunID
	state.wordPendingFocusRunID = snapshot.pendingFocusRunID
	state.wordPendingCaretRunID = snapshot.pendingCaretRunID
	state.wordPendingCaretOffset = snapshot.pendingCaretOffset
	state.wordPendingCaretByX = snapshot.pendingCaretByX
	state.selectedBlock = snapshot.selectedBlock
	state.selectedBlockEditable = snapshot.selectedBlockEditable
	state.wordPageTextSelection = snapshot.pageTextSelection
	if editor := state.wordRunEditors[snapshot.selectedRunID]; editor != nil {
		start := max(0, min(snapshot.caretStart, editor.Len()))
		end := max(0, min(snapshot.caretEnd, editor.Len()))
		editor.SetCaret(start, end)
	}
	if snapshot.focusWasSet {
		if editor := state.wordRunEditors[snapshot.focusRunID]; editor != nil {
			gtx.Execute(key.FocusCmd{Tag: editor})
		}
	}
}

func wordTableSelectionCellIDs(state *documentSessionState) ([]string, document.WordTable, bool) {
	if state == nil {
		return nil, document.WordTable{}, false
	}
	selection := state.wordTableSelection
	if selection == nil || selection.documentID != state.documentID {
		return nil, document.WordTable{}, false
	}
	table, ok := wordTableForID(state, selection.tableID)
	if !ok {
		state.wordTableSelection = nil
		return nil, document.WordTable{}, false
	}
	ids, err := wordTableCellRange(table, selection.anchorCellID, selection.focusCellID)
	if err != nil {
		state.wordTableSelection = nil
		return nil, document.WordTable{}, false
	}
	return ids, table, len(ids) > 0
}

func wordTableCellAtPagePoint(page wordlayout.PageScene, point f32.Point) (string, string, bool) {
	for index := len(page.TableCells) - 1; index >= 0; index-- {
		cell := page.TableCells[index]
		bounds := cell.Bounds
		if float64(point.X) >= bounds.XPT && float64(point.X) <= bounds.XPT+bounds.WidthPT && float64(point.Y) >= bounds.YPT && float64(point.Y) <= bounds.YPT+bounds.HeightPT {
			return cell.TableID, cell.CellID, cell.TableID != "" && cell.CellID != ""
		}
	}
	return "", "", false
}

func wordTableCellAtViewportPoint(state *documentSessionState, point f32.Point) (string, string, int, bool) {
	if state == nil || state.wordLayoutResult == nil {
		return "", "", -1, false
	}
	for _, interaction := range state.wordPageVisibleInteractions {
		if !pointInWordPageRect(point, interaction.PageRect) || interaction.PageIndex < 0 || interaction.PageIndex >= len(state.wordLayoutResult.Pages) {
			continue
		}
		pagePoint := interaction.Transform.ScreenToPage(point)
		tableID, cellID, ok := wordTableCellAtPagePoint(state.wordLayoutResult.Pages[interaction.PageIndex], pagePoint)
		if ok {
			return tableID, cellID, interaction.PageIndex, true
		}
	}
	return "", "", -1, false
}

func wordTablePointerTagForCell(state *documentSessionState, tableID, cellID string) *wordTablePointerTag {
	if state.wordTablePointerTags == nil {
		state.wordTablePointerTags = make(map[string]map[string]*wordTablePointerTag)
	}
	tags := state.wordTablePointerTags[tableID]
	if tags == nil {
		tags = make(map[string]*wordTablePointerTag)
		state.wordTablePointerTags[tableID] = tags
	}
	if tag := tags[cellID]; tag != nil {
		return tag
	}
	tag := new(wordTablePointerTag)
	tags[cellID] = tag
	return tag
}

func wordImageGestureActive(state *documentSessionState) bool {
	if state == nil {
		return false
	}
	for _, interaction := range state.wordImageInteractions {
		if interaction != nil && interaction.preview.mode != wordImageGestureNone {
			return true
		}
	}
	return false
}

func (a *App) handleWordTableCellPointerEvents(gtx layout.Context, state *documentSessionState, tableID, cellID string, tag *wordTablePointerTag) {
	if state == nil || state.wordReadingMode || state.wordProtected || state.wordInsertionRunning || state.formatRunning.Load() || a.isSaving() {
		return
	}
	kinds := pointer.Press | pointer.Enter | pointer.Move | pointer.Drag | pointer.Release | pointer.Cancel
	for value, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: kinds}); ok; value, ok = gtx.Event(pointer.Filter{Target: tag, Kinds: kinds}) {
		eventValue, ok := value.(pointer.Event)
		if !ok {
			continue
		}
		drag := state.wordTableSelectionDrag
		switch eventValue.Kind {
		case pointer.Press:
			if eventValue.Buttons&pointer.ButtonPrimary == 0 {
				continue
			}
			if eventValue.Source == pointer.Mouse && wordHyperlinkActivationShortcut(eventValue.Modifiers) {
				continue
			}
			state.wordCanvasContentClickHandled = true
			if eventValue.Modifiers.Contain(key.ModShift) {
				state.wordTableCaretRestore = captureWordTableCaretSnapshot(gtx, state)
				anchor := cellID
				if current := state.wordTableSelection; current != nil && current.documentID == state.documentID && current.tableID == tableID {
					anchor = current.anchorCellID
				}
				state.wordTableSelectionDrag = nil
				setWordTableSelection(state, tableID, anchor, cellID)
				continue
			}
			state.wordTableSelection = nil
			state.wordTableSelectionDrag = &wordTableSelectionDrag{
				pointerID: eventValue.PointerID, viewMode: state.wordViewMode, documentID: state.documentID,
				tableID: tableID, anchorCellID: cellID, startCellID: cellID,
				origin: eventValue.Position, caret: captureWordTableCaretSnapshot(gtx, state),
			}
		case pointer.Enter, pointer.Move, pointer.Drag:
			if drag == nil || drag.pointerID != eventValue.PointerID || drag.documentID != state.documentID || drag.tableID != tableID {
				continue
			}
			if eventValue.Buttons&pointer.ButtonPrimary == 0 {
				state.wordTableSelectionDrag = nil
				continue
			}
			if wordImageGestureActive(state) {
				continue
			}
			if drag.startCellID == cellID {
				if drag.startedRange {
					state.wordTableSelection = nil
					state.wordCanvasContentClickHandled = true
				}
				continue
			}
			if setWordTableSelection(state, tableID, drag.anchorCellID, cellID) {
				drag.startedRange = true
				state.wordTableCaretRestore = drag.caret
				drag.caret = nil
				state.wordCanvasContentClickHandled = true
			}
		case pointer.Release:
			if drag != nil && drag.pointerID == eventValue.PointerID {
				if wordImageGestureActive(state) {
					state.wordTableSelectionDrag = nil
					continue
				}
				if drag.tableID == tableID && drag.documentID == state.documentID && drag.startCellID == cellID && drag.startedRange {
					state.wordTableSelection = nil
				} else if drag.tableID == tableID && drag.documentID == state.documentID && drag.startCellID != cellID {
					if setWordTableSelection(state, tableID, drag.anchorCellID, cellID) {
						drag.startedRange = true
						state.wordTableCaretRestore = drag.caret
						drag.caret = nil
						state.wordCanvasContentClickHandled = true
					}
				}
				state.wordTableSelectionDrag = nil
			}
		case pointer.Cancel:
			if drag != nil && drag.pointerID == eventValue.PointerID {
				state.wordTableSelectionDrag = nil
			}
		}
	}
}

func (a *App) queueWordTableSelectionDelete(state *documentSessionState) bool {
	if state == nil || state.word == nil || state.wordTableSelection == nil {
		return false
	}
	if state.wordReadingMode || state.wordProtected || state.wordInsertionRunning || state.formatRunning.Load() || a.isSaving() ||
		state.wordTableSelectionDeleteIntent != nil || state.wordSelectionEditIntent != nil || state.wordParagraphSplitIntent != nil ||
		state.wordParagraphJoinIntent != nil || state.wordPageBreakIntent != nil || state.wordFlushRequested.Load() {
		return false
	}
	_, ok := any(state.word).(document.WordTableSelectionEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.table_cell_deletion_is_not_supported_8d2263"}, feedbackWarning, notificationToast)
		return true
	}
	cellIDs, _, ok := wordTableSelectionCellIDs(state)
	if !ok {
		return true
	}
	for _, runEditor := range state.wordRunEditors {
		if editorHasActiveComposition(runEditor) {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.finish_chinese_input_composition_first_b4284e"}, feedbackWarning, notificationToast)
			return true
		}
	}
	selection := state.wordTableSelection
	state.wordTableSelectionDeleteIntent = &wordTableSelectionDeleteIntent{
		documentID: state.documentID, tableID: selection.tableID, cellIDs: append([]string(nil), cellIDs...),
	}
	for runID, runEditor := range state.wordRunEditors {
		captureWordEditorDraft(state, runID, runEditor.Text())
	}
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	a.startNextWordTextCommitFor(state)
	return true
}

func (a *App) startWordTableSelectionDelete(state *documentSessionState) {
	if state == nil || state.word == nil || state.wordTableSelectionDeleteIntent == nil || a.isSaving() {
		return
	}
	if state.formatRunning.Load() {
		state.wordTableSelectionDeleteIntent = nil
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_is_busy_try_again_shortly_66cff7"}, feedbackWarning, notificationSuppressed)
		return
	}
	intent := *state.wordTableSelectionDeleteIntent
	state.wordTableSelectionDeleteIntent = nil
	if intent.documentID != state.documentID {
		return
	}
	editor, ok := any(state.word).(document.WordTableSelectionEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.table_cell_deletion_is_not_supported_8d2263"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_is_busy_try_again_shortly_66cff7"}, feedbackWarning, notificationSuppressed)
		return
	}
	generation := a.documentGeneration
	started := a.launchManagedBackground(func() {
		outcome := document.WordTableSelectionNoChange
		blocks, err := runWordMutation(state, func() error {
			var deleteErr error
			outcome, deleteErr = editor.DeleteWordTableSelection(intent.tableID, intent.cellIDs)
			return deleteErr
		})
		noChange := err == nil && outcome == document.WordTableSelectionNoChange
		if noChange {
			blocks = nil
		}
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, generation: generation, operation: "删除表格单元格内容",
			err: err, blocks: blocks, tableSelectionEdit: true, tableSelectionEditOutcome: outcome,
			tableSelectionTableID: intent.tableID, noDocumentChange: noChange,
		})
	})
	if !started {
		state.finishOperation(token)
		state.wordTableSelectionDeleteIntent = &intent
	}
}

func wordTableParagraphIDInTable(state *documentSessionState, paragraphID string) bool {
	if paragraphID == "" {
		return false
	}
	for _, block := range wordBlocksForState(state) {
		if block.Table == nil {
			continue
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for _, paragraph := range cell.Paragraphs {
					if paragraph.ID == paragraphID {
						return true
					}
				}
			}
		}
	}
	return false
}

func pruneWordTableEditViewState(state *documentSessionState) {
	if state == nil {
		return
	}
	validRuns := make(map[string]struct{})
	validParagraphs := make(map[string]struct{})
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		validParagraphs[paragraph.ID] = struct{}{}
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID != "" {
				validRuns[inline.Text.ID] = struct{}{}
			}
		}
	}
	for runID := range state.wordRunEditors {
		if _, ok := validRuns[runID]; !ok {
			delete(state.wordRunEditors, runID)
		}
	}
	for paragraphID := range state.wordParagraphSelections {
		if _, ok := validParagraphs[paragraphID]; !ok {
			delete(state.wordParagraphSelections, paragraphID)
		}
	}
	if _, ok := validRuns[state.wordFocusedRunID]; !ok {
		state.wordFocusedRunID = ""
	}
	if _, ok := validRuns[state.wordPendingFocusRunID]; !ok {
		state.wordPendingFocusRunID = ""
	}
	if _, ok := validRuns[state.wordPendingCaretRunID]; !ok {
		state.wordPendingCaretRunID = ""
		state.wordPendingCaretOffset = 0
		state.wordPendingCaretByX = false
	}
	if _, ok := validRuns[state.wordLayoutFocusedRunID]; !ok {
		state.wordLayoutFocusedRunID = ""
	}
	if _, ok := validRuns[state.wordPagePendingScrollRunID]; !ok {
		state.wordPagePendingScrollRunID = ""
	}
	if _, ok := validRuns[state.wordPageCaretBlinkRunID]; !ok {
		state.wordPageCaretBlinkRunID = ""
		state.wordPageCaretBlinkStart = time.Time{}
	}
	if _, ok := validRuns[state.wordPageFocusedMetaRunID]; !ok {
		state.wordPageFocusedMetaRunID = ""
		state.wordPageFocusedMetaRevision = 0
	}
}
