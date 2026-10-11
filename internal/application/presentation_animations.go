package application

import (
	"context"
	"encoding/json"

	"glance/internal/document"
)

type presentationAnimationReader interface {
	PresentationAnimations(context.Context, uint32) (document.PresentationAnimationSummary, error)
}

func registerPresentationAnimationCapabilities(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.animations.list", Name: "读取幻灯片动画",
			Description: "按 document.inspect 返回的稳定 slide_id 读取单页 PPTX 动画时序。animations 按 XML 顺序列出识别出的 OOXML 时序节点，不等于实际播放顺序；target_element_id 来自 spid，preset_id/preset_class 取最近的 cTn 元数据，duration 保留原始 OOXML 值（例如 500 或 indefinite）。无动画时返回空数组。最多解析 4 MiB XML、500 项和 100000 个 XML 令牌，超限时返回 truncated=true 与诊断。只读，不修改文档；结果含读取时的 revision。",
			Category:    CapabilityQuery, Formats: []Format{FormatPPTX}, Risk: RiskLow, RequiresDocument: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID；每次调用检查一张幻灯片。"}},"required":["slide_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"revision":{"type":"integer","minimum":0},"has_timing":{"type":"boolean"},"truncated":{"type":"boolean"},"animations":{"type":"array","maxItems":500,"items":{"type":"object","properties":{"index":{"type":"integer","minimum":1},"kind":{"type":"string","enum":["anim","animClr","animEffect","animMotion","animRot","animScale","cmd","set"]},"target_element_id":{"type":"integer","minimum":1,"maximum":4294967295},"preset_id":{"type":"string","maxLength":256},"preset_class":{"type":"string","maxLength":256},"transition":{"type":"string","maxLength":256},"effect":{"type":"string","maxLength":256},"duration":{"type":"string","maxLength":256}},"required":["index","kind"],"additionalProperties":false}},"diagnostics":{"type":"array","maxItems":8,"items":{"type":"string","maxLength":256}}},"required":["slide_id","revision","has_timing","truncated","animations","diagnostics"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) != 1 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if _, ok := fields["slide_id"]; !ok {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var input struct {
				SlideID uint64 `json:"slide_id"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			reader, ok := capability.Adapter.(presentationAnimationReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			summary, err := reader.PresentationAnimations(ctx, uint32(input.SlideID))
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(summary, false)
		},
	})
}
