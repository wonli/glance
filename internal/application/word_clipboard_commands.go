package application

import (
	"context"
	"encoding/json"
	"strings"
)

// WordTextRangeReader reads an explicit range from one editable Word run.
type WordTextRangeReader interface {
	ReadWordTextRange(context.Context, string, int, int) (string, error)
}

// WordTextRangeCutter atomically returns and deletes an explicit Word text range.
type WordTextRangeCutter interface {
	CutWordTextRange(context.Context, string, int, int) (string, bool, error)
}

func registerWordClipboardCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text_range.copy", Name: "读取 Word 文本范围",
				Description: "读取一个可编辑文本运行中的明确 Unicode 字符范围并返回文本，不访问系统剪贴板，也不依赖当前 UI 选区。start 和 end 从 0 开始，end 不包含在范围内；run_id 和 expected_revision 应来自当前文档查询结果。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","minLength":1},"start":{"type":"integer","minimum":0},"end":{"type":"integer","minimum":0}},"required":["run_id","start","end"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"start":{"type":"integer"},"end":{"type":"integer"},"text":{"type":"string"}},"required":["run_id","start","end","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID string `json:"run_id"`
					Start int    `json:"start"`
					End   int    `json:"end"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.RunID) == "" || input.Start < 0 || input.End < input.Start {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				reader, ok := capability.Adapter.(WordTextRangeReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				text, err := reader.ReadWordTextRange(ctx, input.RunID, input.Start, input.End)
				if err != nil {
					return CapabilityResult{}, err
				}
				if len(text) > MaxQueryResultBytes {
					return CapabilityResult{}, ErrQueryResultTooLarge
				}
				return capabilityJSONResult(struct {
					RunID string `json:"run_id"`
					Start int    `json:"start"`
					End   int    `json:"end"`
					Text  string `json:"text"`
				}{input.RunID, input.Start, input.End, text}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text_range.cut", Name: "剪切 Word 文本范围",
				Description: "从一个可编辑文本运行中删除并返回明确的 Unicode 字符范围。start 和 end 从 0 开始，end 不包含在范围内；操作会修改文档并要求当前修订号。run_id 应来自当前文档查询结果。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","minLength":1},"start":{"type":"integer","minimum":0},"end":{"type":"integer","minimum":0}},"required":["run_id","start","end"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"start":{"type":"integer"},"end":{"type":"integer"},"text":{"type":"string"}},"required":["changed","run_id","start","end","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID string `json:"run_id"`
					Start int    `json:"start"`
					End   int    `json:"end"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.RunID) == "" || input.Start < 0 || input.End < input.Start {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				cutter, ok := capability.Adapter.(WordTextRangeCutter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				text, changed, err := cutter.CutWordTextRange(ctx, input.RunID, input.Start, input.End)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					RunID   string `json:"run_id"`
					Start   int    `json:"start"`
					End     int    `json:"end"`
					Text    string `json:"text"`
				}{changed, input.RunID, input.Start, input.End, text}, changed)
			},
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}
