package application

import (
	"context"
	"encoding/json"
	"strings"
)

type WordThemeEffectsAccess interface {
	SetWordThemeEffects(context.Context, string) (string, bool, error)
}

func registerWordThemeEffectsCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.theme.effects.set", Name: "设置 Word 主题效果",
			Description: "为 DOCX 主题设置微妙、适中或强烈的三档阴影效果。只改变主题 fmtScheme 的标准三项 effectStyle；使用主题效果引用的对象会受影响，具有直接效果格式的对象保持原样。调用前应确认目标文档包含可编辑的三项主题效果列表；复杂或扩展结构会拒绝修改。风险高，Agent 调用须由桌面人工确认。Ribbon“设计/效果”复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"effect_profile":{"type":"string","enum":["subtle","moderate","intense"],"description":"主题阴影强度：subtle 微妙、moderate 适中、intense 强烈。"}},"required":["effect_profile"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"previous_profile":{"type":"string"},"effect_profile":{"type":"string","enum":["subtle","moderate","intense"]}},"required":["changed","previous_profile","effect_profile"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) != 1 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if _, ok := fields["effect_profile"]; !ok {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var input struct {
				Profile string `json:"effect_profile"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			input.Profile = strings.TrimSpace(input.Profile)
			if input.Profile != "subtle" && input.Profile != "moderate" && input.Profile != "intense" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if drafts, ok := capability.Adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			access, ok := capability.Adapter.(WordThemeEffectsAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			previous, changed, err := access.SetWordThemeEffects(ctx, input.Profile)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed  bool   `json:"changed"`
				Previous string `json:"previous_profile"`
				Profile  string `json:"effect_profile"`
			}{changed, previous, input.Profile}, changed)
		},
	})
}
