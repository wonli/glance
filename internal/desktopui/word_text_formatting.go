package desktopui

import (
	"fmt"
	"glance/internal/document"
	"math"
	"strings"
)

func (a *App) toggleWordTextStyle(property string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok {
		return
	}
	runID := run.ID
	patch := document.WordTextFormatPatch{}
	switch property {
	case "bold":
		value := !run.Style.Bold
		patch.Bold = &value
	case "italic":
		value := !run.Style.Italic
		patch.Italic = &value
	case "underline":
		value := !run.Style.Underline
		patch.Underline = &value
	default:
		return
	}
	a.runWordCommand("文字格式", func(editor document.WordTextEditor) error { return editor.SetTextRunStyle(runID, patch) })
}

func (a *App) setWordFontColor(runID, value string) {
	a.runWordCommand("文字颜色", func(editor document.WordTextEditor) error {
		return editor.SetTextRunStyle(runID, document.WordTextFormatPatch{FontColor: &value})
	})
}

func (a *App) chooseWordHighlightColor(index int) {
	state := a.documentSessionState
	if state == nil || state.word == nil || index < 0 || index >= len(wordHighlightColors) {
		return
	}
	runID := state.selectedWordRunID
	run, ok := selectedWordRun(wordBlocksForState(state), runID)
	if !ok || !run.Editable {
		state.wordToolbarHighlightPicker = false
		return
	}
	value := wordHighlightColors[index].value
	state.wordToolbarHighlightPicker = false
	a.setWordTextHighlight(runID, value)
}

func (a *App) setWordTextHighlight(runID, value string) {
	a.runWordCommand("文字高亮", func(editor document.WordTextEditor) error {
		return editor.SetTextRunStyle(runID, document.WordTextFormatPatch{Highlight: &value})
	})
}

func effectiveWordFontSize(points float64) float64 {
	if points <= 0 || math.IsNaN(points) || math.IsInf(points, 0) {
		return wordDefaultFontSizePt
	}
	return points
}

func (a *App) adjustWordFontSize(delta float64) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordTextFontSizeEditor); !ok {
		return
	}
	next := math.Max(wordMinFontSizePt, math.Min(wordMaxFontSizePt, effectiveWordFontSize(run.Style.FontSize)+delta))
	runID := run.ID
	a.runWordCommand("字号", func(editor document.WordTextEditor) error {
		fontSizeEditor, ok := any(editor).(document.WordTextFontSizeEditor)
		if !ok {
			return fmt.Errorf("DOCX adapter does not support font-size editing")
		}
		return fontSizeEditor.SetTextRunFontSize(runID, next)
	})
}

func (a *App) showWordFontFamilyDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordTextFontFamilyEditor); !ok {
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	runID := run.ID
	a.modal.ShowInput("设置字体", "输入字体名称", run.Style.FontFamily, "字体名称", func(action string) {
		value := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if value == "" {
			a.setStatus("字体名称不能为空")
			return
		}
		a.runWordCommand("字体", func(editor document.WordTextEditor) error {
			fontFamilyEditor, ok := any(editor).(document.WordTextFontFamilyEditor)
			if !ok {
				return fmt.Errorf("DOCX adapter does not support font-family editing")
			}
			return fontFamilyEditor.SetTextRunFontFamily(runID, value)
		})
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) toggleWordFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if state.wordFormatPainterActive {
		state.wordFormatPainterActive = false
		state.wordFormatPainterSourceRunID = ""
		state.wordFormatPainterTargetRunID = ""
		state.wordFormatPainterTargetParagraphID = ""
		a.setStatus("格式刷已取消")
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordFormatPainter); !ok {
		return
	}
	state.wordFormatPainterSourceRunID = run.ID
	state.wordFormatPainterActive = true
	a.setStatus("已启动格式刷；单击目标文字应用，再点格式刷取消")
}

func (a *App) queueWordFormatPainterTarget(paragraphID, runID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || !state.wordFormatPainterActive || runID == state.wordFormatPainterSourceRunID {
		return
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, runID)
	paragraph, paragraphOK := selectedWordParagraph(blocks, paragraphID)
	if !ok || !run.Editable || !paragraphOK || !wordParagraphContainsRun(paragraph, runID) {
		return
	}
	if _, ok := any(state.word).(document.WordFormatPainter); !ok {
		return
	}
	state.wordFormatPainterActive = false
	state.selectedWordParagraphID = paragraphID
	state.selectedWordRunID = runID
	state.wordFormatPainterTargetParagraphID = paragraphID
	state.wordFormatPainterTargetRunID = runID
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		state.wordDeferredToolbarAction = "apply-format-painter"
		return
	}
	a.startWordFormatPainter()
}

func (a *App) processWordFormatPainterTargetIntent() {
	state := a.documentSessionState
	if state == nil || state.wordFormatPainterPendingRunID == "" {
		return
	}
	paragraphID, runID := state.wordFormatPainterPendingParagraphID, state.wordFormatPainterPendingRunID
	state.wordFormatPainterPendingParagraphID = ""
	state.wordFormatPainterPendingRunID = ""
	a.queueWordFormatPainterTarget(paragraphID, runID)
}

func wordParagraphContainsRun(paragraph document.WordParagraph, runID string) bool {
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil && inline.Text.ID == runID {
			return true
		}
	}
	return false
}

func (a *App) startWordFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || state.wordFormatPainterSourceRunID == "" || state.wordFormatPainterTargetRunID == "" {
		return
	}
	painter, ok := any(state.word).(document.WordFormatPainter)
	if !ok {
		state.wordFormatPainterSourceRunID = ""
		state.wordFormatPainterTargetRunID = ""
		state.wordFormatPainterTargetParagraphID = ""
		return
	}
	sourceRunID, targetRunID := state.wordFormatPainterSourceRunID, state.wordFormatPainterTargetRunID
	state.wordFormatPainterSourceRunID = ""
	state.wordFormatPainterTargetRunID = ""
	state.wordFormatPainterTargetParagraphID = ""
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	a.setStatus("正在应用格式刷")
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return painter.ApplyWordFormatting(sourceRunID, targetRunID) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "格式刷", err: err, blocks: blocks})
	}()
}

func (a *App) setWordAlignment(actionID string) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	paragraphID := state.selectedWordParagraphID
	alignment := map[string]document.WordAlignment{"align-left": document.WordAlignLeft, "align-center": document.WordAlignCenter, "align-right": document.WordAlignRight, "align-justify": document.WordAlignJustified}[actionID]
	a.runWordCommand("段落对齐", func(editor document.WordTextEditor) error {
		return editor.SetParagraphAlignment(paragraphID, alignment)
	})
}

func (a *App) setWordList(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	if !ok {
		return
	}
	kind := document.WordListBullet
	if actionID == "numbered-list" {
		kind = document.WordListNumbered
	}
	if paragraph.ListKind == kind {
		kind = document.WordListNone
	}
	paragraphID := paragraph.ID
	a.runWordListCommand("段落列表", func(editor document.WordParagraphListEditor) error {
		return editor.SetParagraphList(paragraphID, kind)
	})
}
