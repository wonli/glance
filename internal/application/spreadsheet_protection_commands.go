package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func registerSpreadsheetProtectionCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.protection.get", Name: "读取工作表或工作簿保护状态",
				Description: "读取明确 XLSX 工作表或工作簿结构的保护状态。scope 为 sheet 时必须提供 sheet；scope 为 workbook 时省略 sheet。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","enum":["sheet","workbook"],"description":"读取工作表保护或工作簿结构保护。"},"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"scope 为 sheet 时的明确工作表名称。"}},"required":["scope"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","enum":["sheet","workbook"]},"sheet":{"type":"string"},"enabled":{"type":"boolean"}},"required":["scope","enabled"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Scope string `json:"scope"`
					Sheet string `json:"sheet"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.protection.get: %w", err)
				}
				input.Scope, input.Sheet = strings.TrimSpace(input.Scope), strings.TrimSpace(input.Sheet)
				if !validSpreadsheetProtectionTarget(input.Scope, input.Sheet) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetProtectionController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				enabled, err := controller.GetSpreadsheetProtection(ctx, input.Scope, input.Sheet)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Scope   string `json:"scope"`
					Sheet   string `json:"sheet,omitempty"`
					Enabled bool   `json:"enabled"`
				}{input.Scope, input.Sheet, enabled}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.protection.set", Name: "设置工作表或工作簿保护",
				Description: "按明确目标启用或移除 XLSX 工作表保护或工作簿结构保护。scope 为 sheet 时必须提供 sheet；scope 为 workbook 时省略 sheet。设置或移除保护都需要桌面人类确认。password 会在待确认请求中短暂保留于进程内存，确认后传给格式适配器；不会出现在确认摘要或返回结果中，也不会以明文写入应用设置。Agent 不应从文档推测或编造密码。保护标记不是文件加密。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","enum":["sheet","workbook"],"description":"设置工作表保护或工作簿结构保护。"},"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"scope 为 sheet 时的明确工作表名称。"},"enabled":{"type":"boolean","description":"true 启用保护，false 移除保护。"},"password":{"type":"string","maxLength":1024,"description":"可选保护密码；取消保护时应提供创建保护时使用的密码。待确认时短暂保留在进程内存，摘要和结果不会回显。"}},"required":["scope","enabled"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","enum":["sheet","workbook"]},"sheet":{"type":"string"},"enabled":{"type":"boolean"},"changed":{"type":"boolean"}},"required":["scope","enabled","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Scope    string `json:"scope"`
					Sheet    string `json:"sheet"`
					Enabled  *bool  `json:"enabled"`
					Password string `json:"password"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.Enabled == nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Scope, input.Sheet = strings.TrimSpace(input.Scope), strings.TrimSpace(input.Sheet)
				if !validSpreadsheetProtectionTarget(input.Scope, input.Sheet) || len(input.Password) > 1024 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetProtectionController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetProtection(ctx, input.Scope, input.Sheet, *input.Enabled, input.Password)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Scope   string `json:"scope"`
					Sheet   string `json:"sheet,omitempty"`
					Enabled bool   `json:"enabled"`
					Changed bool   `json:"changed"`
				}{input.Scope, input.Sheet, *input.Enabled, changed}, changed)
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

func validSpreadsheetProtectionTarget(scope, sheet string) bool {
	switch scope {
	case "sheet":
		return sheet != ""
	case "workbook":
		return sheet == ""
	default:
		return false
	}
}
