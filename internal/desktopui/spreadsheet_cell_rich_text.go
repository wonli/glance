package desktopui

import (
	"glance/internal/i18n"
	"strings"

	"gioui.org/widget"

	"glance/internal/document"
)

func (a *App) spreadsheetCellTextSelectionActive() bool {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return false
	}
	if state.cellEdit.Editing {
		start, end := state.cellEditor.Selection()
		if start != end {
			return true
		}
	}
	fStart, fEnd := a.formulaEditor.Selection()
	if fStart != fEnd && a.selectedCell != "" {
		text := a.formulaEditor.Text()
		if !strings.HasPrefix(strings.TrimSpace(text), "=") {
			return true
		}
	}
	return false
}

func (a *App) spreadsheetActiveCellEditor() (editor *widget.Editor, start, end int, cell string, ok bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return nil, 0, 0, "", false
	}
	if state.cellEdit.Editing {
		start, end := state.cellEditor.Selection()
		if start != end {
			if start > end {
				start, end = end, start
			}
			return &state.cellEditor, start, end, state.cellEdit.Address, true
		}
	}
	fStart, fEnd := a.formulaEditor.Selection()
	if fStart != fEnd && a.selectedCell != "" {
		text := a.formulaEditor.Text()
		if !strings.HasPrefix(strings.TrimSpace(text), "=") {
			if fStart > fEnd {
				fStart, fEnd = fEnd, fStart
			}
			cell = a.selectedCell
			if state.cellEdit.Editing && state.cellEdit.Address != "" {
				cell = state.cellEdit.Address
			}
			return &a.formulaEditor, fStart, fEnd, cell, true
		}
	}
	return nil, 0, 0, "", false
}

// spreadsheetTextSelectionStyle resolves the selected run rather than the cell's
// default font. Mixed selections stay inactive and toggle on consistently.
func (a *App) spreadsheetTextSelectionStyle(slot int) (document.CellStyle, bool, bool) {
	editor, start, end, cell, ok := a.spreadsheetActiveCellEditor()
	if !ok || slot < 4 || slot > 6 {
		return document.CellStyle{}, false, false
	}
	base := a.selectedCellStyle()
	rich, ok := a.book.(document.SpreadsheetCellRichTextEditor)
	if !ok {
		return base, false, true
	}
	runs, err := rich.GetCellRichText(a.sheet, cell)
	var text strings.Builder
	for _, run := range runs {
		text.WriteString(run.Text)
	}
	if err != nil || text.String() != editor.Text() {
		return base, false, true
	}
	offset, found, mixed := 0, false, false
	var selected document.CellStyle
	for _, run := range runs {
		next := offset + len([]rune(run.Text))
		if offset < end && next > start {
			current := base
			current.Bold, current.Italic, current.Underline = run.Bold, run.Italic, run.Underline
			if !found {
				selected, found = current, true
			} else if formatStyleActive(slot, current) != formatStyleActive(slot, selected) {
				mixed = true
			}
		}
		offset = next
	}
	if !found {
		return base, false, true
	}
	return selected, mixed, true
}

func (a *App) applySpreadsheetCellTextFormat(slot int) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, start, end, cell, hasSelection := a.spreadsheetActiveCellEditor()
	if !hasSelection || cell == "" {
		return
	}
	richEditor, ok := state.book.(document.SpreadsheetCellRichTextEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	if sheet == "" {
		return
	}
	text := editor.Text()
	if text == "" {
		return
	}

	current, mixed, _ := a.spreadsheetTextSelectionStyle(slot)
	currentBold, currentItalic, currentUnderline := current.Bold && !mixed, current.Italic && !mixed, current.Underline && !mixed

	patch := document.CellFormatPatch{}
	switch slot {
	case 4: // Bold
		v := !currentBold
		patch.Bold = &v
	case 5: // Italic
		v := !currentItalic
		patch.Italic = &v
	case 6: // Underline
		v := !currentUnderline
		patch.Underline = &v
	default:
		return
	}

	if err := richEditor.SetCellTextSelectionFormat(sheet, cell, text, start, end, patch); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		return
	}

	// Synchronize draft and original so a subsequent commit won't overwrite rich text with plain text.
	if state.cellEdit.Address == cell || state.cellEdit.Address == "" {
		state.cellEdit.Original = text
		state.cellEdit.Draft = text
		state.cellEdit.Address = cell
	}

	// Keep both editors in sync with text and selection.
	if editor == &state.cellEditor {
		a.formulaEditor.SetText(text)
		a.formulaEditor.SetCaret(start, end)
	} else {
		state.cellEditor.SetText(text)
		state.cellEditor.SetCaret(start, end)
	}
	editor.SetCaret(start, end)

	if state.virtualGrid != nil {
		if row, col, ok := parseCellAddress(cell); ok {
			runs, _ := richEditor.GetCellRichText(sheet, cell)
			state.virtualGrid.updateCellRichText(row, col, text, runs)
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
