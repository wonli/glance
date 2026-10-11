package application

import (
	"context"
	"encoding/json"
)

type WordProtectionReader interface {
	GetWordDocumentProtection(context.Context) (bool, error)
}

type WordProtectionSetter interface {
	SetWordDocumentProtection(context.Context, bool) (bool, error)
}

func registerWordProtectionCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.document.protection.get", Name: "读取 Word 编辑保护状态",
				Description: "读取明确 DOCX 文档的编辑保护标记。该标记会要求 Word 以只读方式打开；它不设置密码，也不加密文件。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"enabled":{"type":"boolean"}},"required":["enabled"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				reader, ok := capability.Adapter.(WordProtectionReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				enabled, err := reader.GetWordDocumentProtection(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Enabled bool `json:"enabled"`
				}{enabled}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.document.protection.set", Name: "设置 Word 编辑保护",
				Description: "为明确 DOCX 文档开启或关闭只读编辑保护标记。开启后 Word 会以只读方式打开；这不是密码保护或加密。该高影响操作需要桌面人类确认，使用 enabled 传入明确目标状态。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"enabled":{"type":"boolean","description":"true 开启只读编辑保护；false 移除该保护标记。"}},"required":["enabled"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"enabled":{"type":"boolean"}},"required":["changed","enabled"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Enabled *bool `json:"enabled"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.Enabled == nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordProtectionSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordDocumentProtection(ctx, *input.Enabled)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool `json:"changed"`
					Enabled bool `json:"enabled"`
				}{changed, *input.Enabled}, changed)
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
