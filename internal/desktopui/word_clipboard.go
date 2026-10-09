package desktopui

import (
	"glance/internal/document"
	"io"

	"gioui.org/io/transfer"
	"gioui.org/layout"
)

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
			a.setStatus("读取剪贴板失败：" + err.Error())
			continue
		}
		runID := state.wordClipboardRunID
		state.wordClipboardRunID = ""
		if state.formatRunning.Load() || wordHasPendingTextDrafts(state) {
			state.wordPendingPasteRunID = runID
			state.wordPendingPasteText = string(value)
			if wordHasPendingTextDrafts(state) {
				state.wordForceDraftFlush.Store(true)
				state.wordFlushRequested.Store(true)
				state.wordFlushAllDrafts.Store(true)
			}
			continue
		}
		a.runWordCommand("粘贴文字", func(editor document.WordTextEditor) error { return editor.ReplaceTextRun(runID, string(value)) })
	}
}
