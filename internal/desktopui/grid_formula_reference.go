package desktopui

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func (a *App) restoreFormulaReferenceTarget() {
	state := a.documentSessionState
	if state == nil || !state.formulaReference.Active {
		return
	}
	fr := state.formulaReference
	state.formulaReference = formulaReferenceState{}
	state.selectionDragging = false
	state.cellEdit.cancel()
	if state.virtualGrid != nil {
		state.virtualGrid.cancelCellEditRowPreview()
	}
	state.cellEditor.SetText(state.cellEdit.Original)
	state.formulaEditor.SetText(state.cellEdit.Original)
	state.sheet = fr.TargetSheet
	state.selectedCell = fr.TargetCell
	if row, column, ok := parseCellAddress(fr.TargetCell); ok {
		state.selection = CellSelection{Anchor: CellPosition{Row: row - 1, Column: column - 1}, Active: CellPosition{Row: row - 1, Column: column - 1}}
		state.selectionSet = true
		if state.virtualGrid != nil {
			state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = row, column
			state.virtualGrid.PendingSelectionLoad = true
		}
	}
	state.nameBox.SetText(fr.TargetCell)
	if state.virtualGrid != nil && state.virtualGrid.WindowSheet != fr.TargetSheet {
		state.virtualGrid.resetSheetWindow()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func cellEditorText(cell document.Cell) string {
	if cell.Formula != "" {
		return "=" + cell.Formula
	}
	return cell.Value
}

type formulaReferenceState struct {
	Active       bool
	InCell       bool
	Dragging     bool
	TargetSheet  string
	TargetCell   string
	BaseText     string
	ReplaceStart int
	ReplaceEnd   int
	LastText     string
	LastCaret    int
	Anchor       CellPosition
}

func (a *App) syncFormulaReferenceInput(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	if a.cellEdit.Editing && gtx.Focused(&a.cellEditor) {
		a.activateFormulaReferenceInput(true)
		return
	}
	if gtx.Focused(&a.formulaEditor) {
		a.activateFormulaReferenceInput(false)
		return
	}
	if state.formulaReference.Active {
		editor := a.formulaReferenceEditor()
		if editor == nil || !strings.HasPrefix(editor.Text(), "=") {
			state.formulaReference = formulaReferenceState{}
		}
	}
}

func (a *App) activateFormulaReferenceInput(inCell bool) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	editor := &a.formulaEditor
	if inCell {
		editor = &a.cellEditor
	}
	if !strings.HasPrefix(editor.Text(), "=") {
		if state.formulaReference.Active && state.formulaReference.InCell == inCell {
			state.formulaReference = formulaReferenceState{}
		}
		return
	}
	if state.formulaReference.Active {
		state.formulaReference.InCell = inCell
		return
	}
	targetCell := a.cellEdit.Address
	if targetCell == "" {
		targetCell = a.selectedCell
	}
	if targetCell == "" || a.sheet == "" {
		return
	}
	state.formulaReference = formulaReferenceState{
		Active:      true,
		InCell:      inCell,
		TargetSheet: a.sheet,
		TargetCell:  targetCell,
	}
}

func (a *App) formulaReferenceEditor() *widget.Editor {
	state := a.documentSessionState
	if state == nil || !state.formulaReference.Active {
		return nil
	}
	if state.formulaReference.InCell {
		return &a.cellEditor
	}
	return &a.formulaEditor
}

func (a *App) beginFormulaReferenceDrag(position CellPosition, extend bool) bool {
	state := a.documentSessionState
	if state == nil || !state.formulaReference.Active {
		return false
	}
	fr := &state.formulaReference
	editor := a.formulaReferenceEditor()
	if editor == nil || !strings.HasPrefix(editor.Text(), "=") {
		state.formulaReference = formulaReferenceState{}
		return false
	}
	start, end := editor.Selection()
	if start > end {
		start, end = end, start
	}
	text := []rune(editor.Text())
	start = max(0, min(start, len(text)))
	end = max(start, min(end, len(text)))
	reuseReference := fr.LastText != "" && fr.LastText == editor.Text() &&
		(extend || start == end && start == fr.LastCaret)
	seedAnchor, hasSeedAnchor := CellPosition{}, false
	if !reuseReference {
		fr.BaseText = string(text)
		fr.ReplaceStart, fr.ReplaceEnd = start, end
		if start == end {
			if seedStart, seedEnd, anchor, ok := unfinishedFormulaRangeAnchor(text, end); ok {
				fr.ReplaceStart, fr.ReplaceEnd = seedStart, seedEnd
				seedAnchor, hasSeedAnchor = anchor, true
			}
		}
	}
	fr.Anchor = position
	fr.Dragging = true
	if extend {
		if hasSeedAnchor {
			fr.Anchor = seedAnchor
		} else {
			fr.Anchor = state.selection.Anchor
		}
	}
	a.updateFormulaReferenceSelection(position)
	a.updateFormulaReferenceText(position)
	return true
}

func unfinishedFormulaRangeAnchor(text []rune, caret int) (int, int, CellPosition, bool) {
	caret = max(0, min(caret, len(text)))
	prefix := strings.TrimRight(string(text[:caret]), " \t\r\n")
	if len(prefix) == 0 || prefix[len(prefix)-1] != ':' {
		return 0, 0, CellPosition{}, false
	}
	left := strings.TrimRight(prefix[:len(prefix)-1], " \t\r\n")
	tokenStart := len(left)
	for tokenStart > 0 && isFormulaIdentifierByte(left[tokenStart-1]) {
		tokenStart--
	}
	token := left[tokenStart:]
	if token == "" || formulaCellReferenceAtStart.FindString(token) != token {
		return 0, 0, CellPosition{}, false
	}
	anchor, end, ok := parseFormulaCellReference(left, tokenStart)
	if !ok || end != len(left) {
		return 0, 0, CellPosition{}, false
	}
	replaceStart := tokenStart
	if qualifier := formulaSheetQualifierAtEnd.FindStringIndex(left[:tokenStart]); qualifier != nil {
		replaceStart = qualifier[0]
	}
	return len([]rune(left[:replaceStart])), caret, anchor, true
}

func (a *App) extendFormulaReferenceDrag(position CellPosition) bool {
	state := a.documentSessionState
	if state == nil || !state.formulaReference.Active || !state.formulaReference.Dragging {
		return false
	}
	a.updateFormulaReferenceSelection(position)
	a.updateFormulaReferenceText(position)
	return true
}

func (a *App) updateFormulaReferenceSelection(position CellPosition) {
	state := a.documentSessionState
	fr := &state.formulaReference
	position = clampCellPosition(position)
	state.selection = CellSelection{Anchor: fr.Anchor, Active: position}
	state.selectionSet = true
	if a.virtualGrid != nil {
		a.virtualGrid.SelectedRow = position.Row + 1
		a.virtualGrid.SelectedColumn = position.Column + 1
	}
	setSelectedGridPicture(state, "")
	state.gridPictureGesture.cancel()
}

func (a *App) updateFormulaReferenceText(position CellPosition) {
	state := a.documentSessionState
	fr := &state.formulaReference
	if fr.BaseText == "" {
		return
	}
	position = clampCellPosition(position)
	ref := formulaReferenceText(fr.TargetSheet, a.sheet, fr.Anchor, position)
	base := []rune(fr.BaseText)
	start := max(0, min(fr.ReplaceStart, len(base)))
	end := max(start, min(fr.ReplaceEnd, len(base)))
	replacement := make([]rune, 0, len(base)-(end-start)+len([]rune(ref)))
	replacement = append(replacement, base[:start]...)
	replacement = append(replacement, []rune(ref)...)
	replacement = append(replacement, base[end:]...)
	text := string(replacement)
	caret := start + len([]rune(ref))
	editor := a.formulaReferenceEditor()
	if editor == nil {
		return
	}
	editor.SetText(text)
	editor.SetCaret(caret, caret)
	fr.LastText, fr.LastCaret = text, caret
	a.cellEdit.setDraft(text)
	if fr.InCell {
		a.formulaEditor.SetText(text)
		a.formulaEditor.SetCaret(caret, caret)
	}
}

func formulaReferenceText(targetSheet, referenceSheet string, anchor, active CellPosition) string {
	startRow, endRow := min(anchor.Row, active.Row)+1, max(anchor.Row, active.Row)+1
	startColumn, endColumn := min(anchor.Column, active.Column)+1, max(anchor.Column, active.Column)+1
	start := xlsx.CellName(startColumn, startRow)
	end := xlsx.CellName(endColumn, endRow)
	ref := start
	if start != end {
		ref += ":" + end
	}
	if referenceSheet == targetSheet {
		return ref
	}
	return "'" + strings.ReplaceAll(referenceSheet, "'", "''") + "'!" + ref
}

func (a *App) finishFormulaReferenceInput() {
	if a.documentSessionState != nil {
		a.documentSessionState.formulaReference = formulaReferenceState{}
	}
}
