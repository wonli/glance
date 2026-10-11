package application

import (
	"context"
	"encoding/json"
)

type wordCapabilitySummary struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Category     CapabilityCategory `json:"category"`
	Risk         RiskClass          `json:"risk"`
	InputSchema  json.RawMessage    `json:"input_schema"`
	OutputSchema json.RawMessage    `json:"output_schema"`
}

func registerWordCapabilityCatalog(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.addins.list", Name: "列出 Word Agent 能力",
			Description: "列出当前运行时可用于 Word 文档的已注册 Agent 能力，包含稳定 ID、用途说明、风险级别和 JSON 输入/输出 Schema。可按 query、command 或 workflow 分类筛选；此目录描述 Glance 内建功能，不会安装或启用第三方加载项。",
			Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"category":{"type":"string","enum":["query","command","workflow"],"description":"可选能力类别筛选。"}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"document_id":{"type":"string"},"revision":{"type":"integer","minimum":0},"capabilities":{"type":"array","maxItems":256,"items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},"description":{"type":"string"},"category":{"type":"string","enum":["query","command","workflow"]},"risk":{"type":"string","enum":["low","medium","high","critical"]},"input_schema":{"type":"object"},"output_schema":{"type":"object"}},"required":["id","name","description","category","risk","input_schema","output_schema"],"additionalProperties":false}}},"required":["document_id","revision","capabilities"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if err := ctx.Err(); err != nil {
				return CapabilityResult{}, err
			}
			var input struct {
				Category CapabilityCategory `json:"category"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			capabilities := make([]wordCapabilitySummary, 0, 128)
			for _, descriptor := range capability.Runtime.Catalog().Descriptors() {
				if descriptor.ID == "word.addins.list" || !capabilitySupportsWord(descriptor) ||
					input.Category != "" && descriptor.Category != input.Category {
					continue
				}
				capabilities = append(capabilities, wordCapabilitySummary{
					ID: descriptor.ID, Name: descriptor.Name, Description: descriptor.Description,
					Category: descriptor.Category, Risk: descriptor.Risk,
					InputSchema:  append(json.RawMessage(nil), descriptor.InputSchema...),
					OutputSchema: append(json.RawMessage(nil), descriptor.OutputSchema...),
				})
				if len(capabilities) > 256 {
					return CapabilityResult{}, ErrQueryResultTooLarge
				}
			}
			return capabilityJSONResult(struct {
				DocumentID   string                  `json:"document_id"`
				Revision     uint64                  `json:"revision"`
				Capabilities []wordCapabilitySummary `json:"capabilities"`
			}{capability.Document.DocumentID, capability.Document.Revision, capabilities}, false)
		},
	})
}

func capabilitySupportsWord(descriptor CapabilityDescriptor) bool {
	if len(descriptor.Formats) == 0 {
		return true
	}
	for _, format := range descriptor.Formats {
		if format == FormatDocx {
			return true
		}
	}
	return false
}
