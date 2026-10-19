package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/application"
	"glance/internal/document"
	"strings"
)

type wordTextInsertArguments struct {
	Target           string `json:"target"`
	AfterParagraphId string `json:"after_paragraph_id"`
	Text             string `json:"text"`
}

type wordTextInsertArgumentsTarget struct {
	Target string `json:"target"`
	RunId  string `json:"run_id"`
	Text   string `json:"text"`
}

func (a *App) showWordDictateDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	a.modal.ShowInputMessage("word.dictation", i18n.Message{ID: "word.dictation.dialog.title"}, i18n.Message{ID: "word.dictation.dialog.body"}, a.text(i18n.Message{ID: "word.dictation.default_text"}), i18n.Message{ID: "word.dictation.dialog.hint"},
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				return
			}
			if hasRun && run.Editable {
				if !a.runWordApplicationCommand("语音听写录入", "word.text.insert", wordTextInsertArgumentsTarget{Target: "run", RunId: run.ID, Text: text}) {
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_word_dictation_f02ce1"}, feedbackWarning, notificationToast)
				}
			} else if _, ok := any(state.word).(document.WordTextBoxEditor); ok {
				if !a.runWordApplicationCommand("语音听写录入", "word.text.insert", wordTextInsertArguments{Target: "textbox", AfterParagraphId: state.selectedWordParagraphID, Text: text}) {
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_word_dictation_f02ce1"}, feedbackWarning, notificationToast)
				}
			} else {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.select_a_text_paragraph_before_5de47c"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.dictation.action.insert"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type showWordAddInsDialogPayload struct {
}

func (a *App) showWordAddInsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.closed.Load() {
		return
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_word_agent_capability_catalog_99d6fd"}, feedbackWarning, notificationToast)
		return
	}
	arguments, _ := json.Marshal(showWordAddInsDialogPayload{})
	result, err := runtime.applicationRuntime.InvokeCapability(context.Background(), application.CapabilityRequest{
		RequestID:  fmt.Sprintf("word-capabilities-%s-%d", state.documentID, a.applicationRequestSeq.Add(1)),
		DocumentID: state.documentID, CapabilityID: "word.addins.list", ExpectedRevision: state.wordRevisionAtomic.Load(),
		Operator:  application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
		Arguments: arguments,
	})
	if err != nil {
		a.setWordCommandStatus(err)
		return
	}
	var catalog struct {
		Capabilities []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Category    string `json:"category"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(result.Data, &catalog); err != nil {
		a.setWordCommandStatus(application.ErrInvalidCommandResult)
		return
	}
	categoryCounts := map[string]int{}
	var rows strings.Builder
	for index, capability := range catalog.Capabilities {
		categoryCounts[capability.Category]++
		if index < 12 {
			fmt.Fprintf(&rows, "· %s（%s）\n", capability.Name, capability.ID)
		}
	}
	if len(catalog.Capabilities) > 12 {
		fmt.Fprintf(&rows, "· 另有 %d 项能力；完整目录和调用参数由 MCP 工具列表提供。\n", len(catalog.Capabilities)-12)
	}
	a.modal.ShowMessage("word.addins", i18n.Message{ID: "word.addins.dialog.title"}, i18n.Message{ID: "word.addins.dialog.body", Args: i18n.Args{"Total": len(catalog.Capabilities), "Queries": categoryCounts["query"], "Commands": categoryCounts["command"], "Workflows": categoryCounts["workflow"], "Rows": rows.String()}},
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}},
	)
}
