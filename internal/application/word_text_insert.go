package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

type WordTextRunAppender interface {
	AppendWordTextRun(context.Context, string, string) (bool, error)
}

func registerWordTextInsertCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.text.insert", Name: "插入 Word 正文文本",
			Description: "将调用方提供的文字插入明确的 Word 目标。文本最多 16000 个 Unicode 字符。target=run 时按 run_id 追加到可编辑文本运行末尾，若原文非空则自动加一个空格；target=textbox 时在正文末尾或 after_paragraph_id 后插入文本框。可用于写入 Agent 生成或听写识别的内容；不会调用语音识别服务。先查询当前文档并传入当前修订。",
			Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"target":{"type":"string","enum":["run","textbox"]},"run_id":{"type":"string","minLength":1},"after_paragraph_id":{"type":"string"},"text":{"type":"string","minLength":1,"maxLength":16000,"description":"最多 16000 个 Unicode 字符。"}},"required":["target","text"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"target":{"type":"string","enum":["run","textbox"]},"run_id":{"type":"string"},"after_paragraph_id":{"type":"string"},"text":{"type":"string"}},"required":["changed","target","text"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Target           string `json:"target"`
				RunID            string `json:"run_id"`
				AfterParagraphID string `json:"after_paragraph_id"`
				Text             string `json:"text"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if strings.TrimSpace(input.Text) == "" || utf8.RuneCountInString(input.Text) > 16000 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			result := struct {
				Changed          bool   `json:"changed"`
				Target           string `json:"target"`
				RunID            string `json:"run_id,omitempty"`
				AfterParagraphID string `json:"after_paragraph_id,omitempty"`
				Text             string `json:"text"`
			}{Target: input.Target, RunID: input.RunID, AfterParagraphID: input.AfterParagraphID, Text: input.Text}
			switch input.Target {
			case "run":
				if input.RunID == "" || input.AfterParagraphID != "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				appender, ok := capability.Adapter.(WordTextRunAppender)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := appender.AppendWordTextRun(ctx, input.RunID, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				result.Changed = changed
			case "textbox":
				if input.RunID != "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordTextBoxInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				if err := inserter.InsertWordTextBox(ctx, input.AfterParagraphID, input.Text); err != nil {
					return CapabilityResult{}, err
				}
				result.Changed = true
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			return capabilityJSONResult(result, result.Changed)
		},
	})
}
