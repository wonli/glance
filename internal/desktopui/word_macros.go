package desktopui

import (
	"encoding/json"
)

type wordMacrosInspectionResult struct {
	state      *documentSessionState
	generation uint64
	request    uint64
	revision   uint64
	data       []byte
	err        error
}

func (a *App) showWordMacrosDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		a.modal.Show("宏与安全性", "Word 应用能力目录当前不可用，无法检查宏部件。", func(string) { a.modal.Hide() }, modalDialogActionSpec{ID: "close", Label: "关闭"})
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatus("Word 文档正在处理其他操作")
		return
	}
	a.wordMacrosInspectionRequest++
	request := a.wordMacrosInspectionRequest
	generation := a.documentGeneration
	window, shutdown, results := a.window, a.shutdown, a.wordMacrosInspectionResults
	a.modal.Show("宏与安全性", "正在检查 DOCX 中是否包含 VBA 宏部件…", func(string) {
		if a.wordMacrosInspectionRequest == request {
			a.wordMacrosInspectionRequest++
		}
		a.modal.Hide()
		if window != nil {
			window.Invalidate()
		}
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, "word.macros.inspect", "word_document", state.documentID, "", map[string]any{})
		state.finishOperation(token)
		result := wordMacrosInspectionResult{
			state: state, generation: generation, request: request,
			revision: token.revision, data: command.Data, err: err,
		}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
}

func (a *App) applyWordMacrosInspectionResult(result wordMacrosInspectionResult) {
	if result.request != a.wordMacrosInspectionRequest || !a.modal.Visible || a.modal.Title != "宏与安全性" {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.generation != a.documentGeneration {
		a.modal.Message = "当前文档已切换，宏检查结果已丢弃。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.state.wordRevisionAtomic.Load() != result.revision {
		a.modal.Message = "文档在宏检查期间发生变化，结果已过期。请重新检查。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.err != nil {
		a.modal.Message = "宏部件检查失败：" + result.err.Error()
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	var inspection struct {
		MacroPayloadDetected bool   `json:"macro_payload_detected"`
		ExecutionSupported   bool   `json:"execution_supported"`
		Summary              string `json:"summary"`
	}
	if err := json.Unmarshal(result.data, &inspection); err != nil {
		a.modal.Message = "无法读取宏检查结果：" + err.Error()
	} else if inspection.Summary == "" || inspection.ExecutionSupported {
		a.modal.Message = "宏检查结果格式无效。"
	} else {
		a.modal.Message = inspection.Summary
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
