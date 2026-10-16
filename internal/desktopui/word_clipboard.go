package desktopui

import (
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/transfer"
	"gioui.org/layout"
)

type wordClipboardSelectionTarget struct {
	start      wordPageCaretPosition
	end        wordPageCaretPosition
	localRunID string
	localText  string
	localStart int
	localEnd   int
}

type wordPendingSelectionPaste struct {
	target wordClipboardSelectionTarget
	text   string
}

func (a *App) requestWordClipboardRead(gtx layout.Context, state *documentSessionState) {
	if state == nil {
		return
	}
	if state.wordClipboardRunID != "" || state.wordClipboardSelection != nil {
		a.setStatus("已有一项粘贴正在读取剪贴板")
		return
	}
	state.wordClipboardRunID = state.selectedWordRunID
	state.wordClipboardSelection = nil
	if start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection); ok {
		state.wordClipboardSelection = &wordClipboardSelectionTarget{start: start, end: end}
	} else {
		runID := state.wordFocusedRunID
		run, ok := selectedWordRun(wordBlocksForState(state), runID)
		if !ok || !run.Editable {
			runID = state.selectedWordRunID
			run, ok = selectedWordRun(wordBlocksForState(state), runID)
		}
		if ok && run.Editable {
			editor := wordRunEditor(state, run)
			start, end := editor.Selection()
			state.wordClipboardRunID = runID
			state.wordClipboardSelection = &wordClipboardSelectionTarget{
				localRunID: runID, localText: editor.Text(), localStart: start, localEnd: end,
			}
		}
	}
	if state.wordClipboardSelection == nil {
		state.wordClipboardRunID = ""
		a.setStatus("请先将光标放到 Word 正文中再粘贴")
		return
	}
	gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
}

func (a *App) pasteWordLocalEditorSelection(gtx layout.Context, state *documentSessionState, target wordClipboardSelectionTarget, text string) {
	if state == nil || state.wordReadingMode || state.formatRunning.Load() || a.isSaving() {
		a.setStatus("当前 Word 文档不可编辑")
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), target.localRunID)
	if !ok || !run.Editable {
		a.setStatus("Word 粘贴位置已失效，请重新选择光标位置")
		return
	}
	editor := wordRunEditor(state, run)
	if editorHasActiveComposition(editor) {
		a.setStatus("请先完成当前中文输入，再粘贴文字")
		return
	}
	current := []rune(editor.Text())
	if editor.Text() != target.localText {
		a.setStatus("正文在剪贴板数据返回前已变化，请重新粘贴")
		return
	}
	start, end := target.localStart, target.localEnd
	if start < 0 || end < start || end > len(current) {
		a.setStatus("Word 粘贴光标已失效，请重新选择光标位置")
		return
	}
	editor.SetCaret(start, end)
	inserted := editor.Insert(text)
	value := editor.Text()
	caret := start + inserted
	state.wordPageTextSelection = nil
	state.wordPageSelectionDrag = nil
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = wordParagraphIDForRun(wordBlocksForState(state), run.ID)
	state.selectedWordImageID = ""
	state.wordFocusedRunID = run.ID
	state.wordLayoutFocusedRunID = run.ID
	state.wordPendingCaretRunID = run.ID
	state.wordPendingCaretOffset = caret
	state.wordPendingCaretByX = false
	state.wordPendingFocusRunID = run.ID
	captureWordTextDraft(state, run.ID, value)
	gtx.Execute(key.FocusCmd{Tag: editor})
	a.setStatus("正在编辑 Word 正文")
}

func (a *App) cutSelectedWordText(gtx layout.Context, state *documentSessionState) bool {
	if state == nil || !wordPageTextSelectionActive(state.wordPageTextSelection) {
		return false
	}
	text, selected := wordPageSelectedText(state)
	start, end, ordered := wordPageOrderedSelection(state.wordPageTextSelection)
	if !selected || !ordered {
		return true
	}
	if !a.queueWordSelectionDeletion(state, start, end) {
		return true
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
	return true
}

func (a *App) handleWordClipboardEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &state.wordClipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		data, ok := ev.(transfer.DataEvent)
		if !ok {
			continue
		}
		reader := data.Open()
		value, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			state.wordClipboardRunID = ""
			state.wordClipboardSelection = nil
			a.setStatus("读取剪贴板失败：" + err.Error())
			continue
		}
		state.wordClipboardRunID = ""
		selection := state.wordClipboardSelection
		state.wordClipboardSelection = nil
		if selection == nil {
			a.setStatus("Word 粘贴位置已失效，请重新选择光标位置")
			continue
		}
		if selection.localRunID != "" {
			a.pasteWordLocalEditorSelection(gtx, state, *selection, string(value))
			continue
		}
		if state.wordPendingPasteSelection != nil {
			a.setStatus("已有一项 Word 粘贴正在等待完成")
			continue
		}
		state.wordPendingPasteSelection = &wordPendingSelectionPaste{target: *selection, text: string(value)}
		if draft := state.wordSelectionInputDraft; draft != nil {
			if editor := state.wordRunEditors[draft.runID]; editor != nil {
				if editorHasActiveComposition(editor) {
					state.wordPendingPasteSelection = nil
					a.setStatus("请先完成当前中文输入，再粘贴选区文字")
					continue
				}
				if editor.Text() != draft.text {
					// Commit the user's already typed replacement first. The pending
					// paste will be rebased to the resulting caret on completion.
					a.finishWordSelectionInput(state, draft.runID, editor)
					continue
				}
				state.wordSelectionInputDraft = nil
			}
		}
		composing := false
		for _, editor := range state.wordRunEditors {
			if editorHasActiveComposition(editor) {
				composing = true
				break
			}
		}
		if composing {
			state.wordPendingPasteSelection = nil
			a.setStatus("请先完成当前中文输入，再粘贴选区文字")
			continue
		}
		if wordSelectionPasteTargetHasPendingDraft(state, *selection) {
			state.wordPendingPasteSelection = nil
			a.setStatus("选区内有未提交的文字修改，请完成编辑后重新粘贴")
			continue
		}
		for id, editor := range state.wordRunEditors {
			captureWordTextDraft(state, id, editor.Text())
		}
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		if !state.formatRunning.Load() {
			a.startNextWordTextCommitFor(state)
		}
		continue
	}
}
