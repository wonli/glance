package application

import (
	"context"
	"encoding/json"
	"strings"
)

type WordImageWrapAccess interface {
	SetWordImageWrap(context.Context, string, string) (string, bool, error)
}

func registerWordImageWrapCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.image.wrap.set", Name: "设置 Word 图片文字环绕",
			Description: "将明确指定的 DrawingML 浮动图片转换为嵌入型文字环绕，使图片按行内字符参与正文排版。保留图片数据、稳定 ID、尺寸、替代文本和图片内容；移除绝对定位与浮动环绕规则，因此正文可能重新分页。仅接受 document.inspect 返回 wrap_editable=true 的图片；复杂或未知结构会拒绝修改。风险高，Agent 调用须由桌面人工确认。Ribbon“布局/环绕文字”复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string","minLength":1,"maxLength":256,"description":"document.inspect 返回的稳定图片 ID。"},"wrap_mode":{"type":"string","enum":["inline"],"description":"目标文字环绕；当前支持 inline，图片与正文按行内字符排版。"}},"required":["image_id","wrap_mode"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"image_id":{"type":"string"},"previous_wrap_mode":{"type":"string"},"wrap_mode":{"type":"string","enum":["inline"]}},"required":["changed","image_id","previous_wrap_mode","wrap_mode"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) != 2 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			for key := range fields {
				if key != "image_id" && key != "wrap_mode" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			}
			var input struct {
				ImageID  string `json:"image_id"`
				WrapMode string `json:"wrap_mode"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.ImageID) == "" || len(input.ImageID) > 256 || input.WrapMode != "inline" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			access, ok := capability.Adapter.(WordImageWrapAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			previous, changed, err := access.SetWordImageWrap(ctx, input.ImageID, input.WrapMode)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed          bool   `json:"changed"`
				ImageID          string `json:"image_id"`
				PreviousWrapMode string `json:"previous_wrap_mode"`
				WrapMode         string `json:"wrap_mode"`
			}{changed, input.ImageID, previous, input.WrapMode}, changed)
		},
	})
}
