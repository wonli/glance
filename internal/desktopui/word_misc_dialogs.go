package desktopui

import (
	"context"
	"encoding/json"
	"fmt"

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
	a.modal.ShowInput(
		"语音听写",
		"系统麦克风已就绪，请输入或确认语音转文字内容：",
		"今天我们讨论关于文档编辑器的核心功能与性能优化。",
		"语音识别内容",
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
					a.setStatus("无法启动 Word 听写插入操作")
				}
			} else if _, ok := any(state.word).(document.WordTextBoxEditor); ok {
				if !a.runWordApplicationCommand("语音听写录入", "word.text.insert", wordTextInsertArguments{Target: "textbox", AfterParagraphId: state.selectedWordParagraphID, Text: text}) {
					a.setStatus("无法启动 Word 听写插入操作")
				}
			} else {
				a.setStatus("请先选择文字段落以插入听写内容")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "插入到正文"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
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
		a.setStatus("Word Agent 能力目录不可用")
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
		a.setStatus("读取 Word Agent 能力失败：" + err.Error())
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
		a.setStatus("读取 Word Agent 能力失败：" + application.ErrInvalidCommandResult.Error())
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
	a.modal.Show(
		"加载项与扩展",
		fmt.Sprintf("当前文档注册了 %d 项 Agent 能力（查询 %d、命令 %d、工作流 %d）。\n\n%s\n使用 MCP 工具列表可以读取完整用途说明和 JSON Schema。",
			len(catalog.Capabilities), categoryCounts["query"], categoryCounts["command"], categoryCounts["workflow"], rows.String()),
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}
