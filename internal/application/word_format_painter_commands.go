package application

import (
	"context"
	"encoding/json"
	"strings"
)

func registerWordFormatPainterCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.text_format.paint", Name: "复制 Word 文本格式",
			Description: "将源文本运行的受支持字符格式和段落格式应用到目标文本运行，保留目标文字内容。source_run_id 和 target_run_id 必须来自当前文档查询；不支持的段落编号会返回错误。",
			Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"source_run_id":{"type":"string","minLength":1},"target_run_id":{"type":"string","minLength":1}},"required":["source_run_id","target_run_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"source_run_id":{"type":"string"},"target_run_id":{"type":"string"}},"required":["changed","source_run_id","target_run_id"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SourceRunID string `json:"source_run_id"`
				TargetRunID string `json:"target_run_id"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			if strings.TrimSpace(input.SourceRunID) == "" || strings.TrimSpace(input.TargetRunID) == "" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			painter, ok := capability.Adapter.(WordFormatPainterCapability)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := painter.ApplyWordFormatting(ctx, input.SourceRunID, input.TargetRunID)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed     bool   `json:"changed"`
				SourceRunID string `json:"source_run_id"`
				TargetRunID string `json:"target_run_id"`
			}{changed, input.SourceRunID, input.TargetRunID}, changed)
		},
	})
}
