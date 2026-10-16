package desktopui

import (
	"runtime"
	"strings"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
)

type formulaReferenceSpan struct {
	start int
	end   int
}

// cycleFormulaReferenceAtCaret cycles the A1 reference under the insertion
// point through A1, $A$1, A$1, and $A1. It ignores quoted strings, function
// names, and text that is part of a larger identifier or structured ref.
func cycleFormulaReferenceAtCaret(formula string, selectionStart, selectionEnd int) (string, int, int, bool) {
	runes := []rune(formula)
	selectionStart, selectionEnd = clampEditorRange(selectionStart, selectionEnd, len(runes))
	if selectionStart > selectionEnd {
		selectionStart, selectionEnd = selectionEnd, selectionStart
	}
	span, ok := formulaReferenceAtSelection(formula, selectionStart, selectionEnd)
	if !ok {
		return formula, selectionStart, selectionEnd, false
	}
	replacement := cycleAbsoluteFormulaReference(formula[span.start:span.end])
	if replacement == formula[span.start:span.end] {
		return formula, selectionStart, selectionEnd, false
	}
	updated := formula[:span.start] + replacement + formula[span.end:]
	spanStartRune := utf8.RuneCountInString(formula[:span.start])
	replacementRunes := utf8.RuneCountInString(replacement)
	if selectionStart != selectionEnd {
		return updated, spanStartRune, spanStartRune + replacementRunes, true
	}
	caret := spanStartRune + min(max(0, selectionStart-spanStartRune), replacementRunes)
	return updated, caret, caret, true
}

func formulaReferenceAtSelection(formula string, selectionStart, selectionEnd int) (formulaReferenceSpan, bool) {
	for index := 0; index < len(formula); {
		switch formula[index] {
		case '"':
			index = skipFormulaString(formula, index)
			continue
		case '\'':
			if _, end, ok := quotedFormulaSheet(formula, index); ok {
				index = end
				continue
			}
		}
		if !isFormulaReferenceStart(formula, index) ||
			(index > 0 && (isFormulaIdentifierByte(formula[index-1]) || formula[index-1] == '[')) {
			index++
			continue
		}
		match := formulaCellReferenceAtStart.FindString(formula[index:])
		if match == "" {
			index++
			continue
		}
		end := index + len(match)
		if _, parsedEnd, ok := parseFormulaCellReference(formula, index); !ok || parsedEnd != end || hasFormulaTokenBoundary(formula, index, end) {
			index = end
			continue
		}
		after := skipFormulaSpaces(formula, end)
		if after < len(formula) && formula[after] == '(' {
			index = end
			continue
		}
		startRune := utf8.RuneCountInString(formula[:index])
		endRune := utf8.RuneCountInString(formula[:end])
		if selectionStart == selectionEnd {
			if selectionStart >= startRune && selectionStart <= endRune {
				return formulaReferenceSpan{start: index, end: end}, true
			}
		} else if selectionStart < endRune && selectionEnd > startRune {
			return formulaReferenceSpan{start: index, end: end}, true
		}
		index = end
	}
	return formulaReferenceSpan{}, false
}

func cycleAbsoluteFormulaReference(reference string) string {
	columnAbsolute := strings.HasPrefix(reference, "$")
	body := strings.TrimPrefix(reference, "$")
	columnEnd := 0
	for columnEnd < len(body) && (body[columnEnd] >= 'A' && body[columnEnd] <= 'Z' || body[columnEnd] >= 'a' && body[columnEnd] <= 'z') {
		columnEnd++
	}
	if columnEnd == 0 || columnEnd >= len(body) {
		return reference
	}
	rowAndAbsolute := body[columnEnd:]
	rowAbsolute := strings.HasPrefix(rowAndAbsolute, "$")
	row := strings.TrimPrefix(rowAndAbsolute, "$")
	column := body[:columnEnd]

	switch {
	case !columnAbsolute && !rowAbsolute:
		return "$" + column + "$" + row
	case columnAbsolute && rowAbsolute:
		return column + "$" + row
	case !columnAbsolute && rowAbsolute:
		return "$" + column + row
	default:
		return column + row
	}
}

func (a *App) handleSpreadsheetFormulaReferenceCycle(gtx layout.Context, v *gridViewport, editor *widget.Editor) bool {
	state := a.documentSessionState
	if state == nil || editor == nil || editorHasActiveComposition(editor) ||
		!strings.HasPrefix(strings.TrimSpace(editor.Text()), "=") || state.formulaReference.Dragging {
		return false
	}
	filters := []event.Filter{key.Filter{Focus: editor, Name: key.NameF4}}
	if runtime.GOOS == "darwin" {
		filters = append(filters, key.Filter{Focus: editor, Name: key.Name("T"), Required: key.ModShortcut})
	}
	for event, ok := gtx.Event(filters...); ok; event, ok = gtx.Event(filters...) {
		pressed, ok := event.(key.Event)
		if !ok || pressed.State != key.Press {
			continue
		}
		text := editor.Text()
		start, end := editor.Selection()
		updated, nextStart, nextEnd, changed := cycleFormulaReferenceAtCaret(text, start, end)
		if !changed {
			return true
		}
		editor.SetText(updated)
		editor.SetCaret(nextStart, nextEnd)
		if editor == &a.cellEditor {
			a.syncSpreadsheetGridTextDraft()
			a.updateSpreadsheetGridInputSnippet(gtx, v)
			updateGridCellRowPreview(gtx, a, v)
		}
		if a.window != nil {
			a.window.Invalidate()
		}
		return true
	}
	return false
}
