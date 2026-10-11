package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

func registerWordHeaderFooterCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.header.set", Name: "设置 Word 页眉",
				Description: "将 Word 文档的默认页眉设置为给定文本，作用于文档各节。该操作会替换现有页眉内容；需要保留原有内容时，请先查询并将需保留文字合并到 text。文本长度为 1 至 16000 个 Unicode 字符。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","minLength":1,"maxLength":16000,"description":"页眉文本，最多 16000 个 Unicode 字符。"}},"required":["text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"text":{"type":"string"}},"required":["changed","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.Text) == "" || utf8.RuneCountInString(input.Text) > 16000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordHeaderFooterSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordHeader(ctx, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					Text    string `json:"text"`
				}{changed, input.Text}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.footer.set", Name: "设置 Word 页脚",
				Description: "将 Word 文档的默认页脚设置为给定文本，作用于文档各节。该操作会替换现有页脚内容；需要保留原有内容时，请先查询并将需保留文字合并到 text。文本长度为 1 至 16000 个 Unicode 字符。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","minLength":1,"maxLength":16000,"description":"页脚文本，最多 16000 个 Unicode 字符。"}},"required":["text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"text":{"type":"string"}},"required":["changed","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.Text) == "" || utf8.RuneCountInString(input.Text) > 16000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordHeaderFooterSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordFooter(ctx, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					Text    string `json:"text"`
				}{changed, input.Text}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.page_number.set", Name: "设置 Word 页码",
				Description: "在 Word 页脚中添加动态页码字段；若已有页码字段，则只更新其对齐方式，并保留页脚中的其他段落。alignment 可为 left、center 或 right。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"alignment":{"type":"string","enum":["left","center","right"]}},"required":["alignment"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"alignment":{"type":"string"}},"required":["changed","alignment"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Alignment string `json:"alignment"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				setter, ok := capability.Adapter.(WordHeaderFooterSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordPageNumber(ctx, input.Alignment)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					Alignment string `json:"alignment"`
				}{changed, input.Alignment}, changed)
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
