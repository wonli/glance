package desktopui

import (
	"encoding/json"
	"glance/internal/i18n"
)

type wordMacrosInspectionResult struct {
	state      *documentSessionState
	generation uint64
	request    uint64
	revision   uint64
	data       []byte
	err        error
}
type wordMacrosInspectArguments struct {
}

func (a *App) showWordMacrosDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		a.modal.ShowMessage(dialogIDWordMacros, i18n.Message{ID: "word.macros.title"}, i18n.Message{ID: "word.macros.error.unavailable"}, func(string) { a.modal.Hide() }, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.operation_busy"}, feedbackWarning, notificationToast)
		return
	}
	a.wordMacrosInspectionRequest++
	request := a.wordMacrosInspectionRequest
	generation := a.documentGeneration
	window, shutdown, results := a.window, a.shutdown, a.wordMacrosInspectionResults
	a.modal.ShowMessage(dialogIDWordMacros, i18n.Message{ID: "word.macros.title"}, i18n.Message{ID: "word.macros.scanning"}, func(string) {
		if a.wordMacrosInspectionRequest == request {
			a.wordMacrosInspectionRequest++
		}
		a.modal.Hide()
		if window != nil {
			window.Invalidate()
		}
	}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, "word.macros.inspect", "word_document", state.documentID, "", wordMacrosInspectArguments{})
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
	if result.request != a.wordMacrosInspectionRequest || !a.modal.Visible || a.modal.DialogID != dialogIDWordMacros {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.generation != a.documentGeneration {
		a.modal.SetMessageRef(i18n.Message{ID: "word.macros.error.switched"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.state.wordRevisionAtomic.Load() != result.revision {
		a.modal.SetMessageRef(i18n.Message{ID: "word.macros.error.stale"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.err != nil {
		a.modal.SetMessageRef(i18n.Message{ID: "word.macros.error.failed", Args: i18n.Args{"Details": result.err.Error()}})
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
		a.modal.SetMessageRef(i18n.Message{ID: "word.macros.error.read", Args: i18n.Args{"Details": err.Error()}})
	} else if inspection.Summary == "" || inspection.ExecutionSupported {
		a.modal.SetMessageRef(i18n.Message{ID: "word.macros.error.invalid_result"})
	} else {
		a.modal.SetMessageText(inspection.Summary)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
