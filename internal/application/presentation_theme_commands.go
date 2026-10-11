package application

import (
	"context"
	"encoding/json"
)

type presentationThemeAccess interface {
	SetPresentationTheme(context.Context, string) (bool, error)
	SetPresentationThemePalette(context.Context, string) (bool, error)
}

func registerPresentationThemeCapabilities(catalog *Catalog) error {
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.theme.set", Name: "设置演示文稿主题",
			Description: "将明确指定的 PPTX 主题预设应用到文档内所有主题部件。内置预设 glance-corporate、glance-forest、glance-warm 只调整主题颜色方案和主/正文西文字体方案；保留母版、版式、主题效果、背景素材和对象的直接格式。复杂或非标准主题结构会拒绝修改。该操作会改变整份文档的外观，风险高，MCP 调用须由桌面人类确认。Ribbon“设计/主题”复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"theme_id":{"type":"string","enum":["glance-corporate","glance-forest","glance-warm"],"description":"要应用的 Glance 内置主题。"}},"required":["theme_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"theme_id":{"type":"string"}},"required":["changed","theme_id"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				ThemeID string `json:"theme_id"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.ThemeID == "" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			access, ok := capability.Adapter.(presentationThemeAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := access.SetPresentationTheme(ctx, input.ThemeID)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool   `json:"changed"`
				ThemeID string `json:"theme_id"`
			}{changed, input.ThemeID}, changed)
		},
	}); err != nil {
		return err
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.theme.palette.set", Name: "设置演示文稿配色",
			Description: "将明确指定的配色方案应用到 PPTX 文档内所有主题部件，只修改主题 clrScheme，不改字体、效果、背景、版式或对象直接格式。内置配色：office、ocean、forest、warm。非标准配色 XML 会拒绝修改。风险高，MCP 调用须由桌面人类确认。Ribbon“设计/配色”复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"palette_id":{"type":"string","enum":["office","ocean","forest","warm"],"description":"要应用的内置配色方案。"}},"required":["palette_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"palette_id":{"type":"string"}},"required":["changed","palette_id"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				PaletteID string `json:"palette_id"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.PaletteID == "" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			access, ok := capability.Adapter.(presentationThemeAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := access.SetPresentationThemePalette(ctx, input.PaletteID)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed   bool   `json:"changed"`
				PaletteID string `json:"palette_id"`
			}{changed, input.PaletteID}, changed)
		},
	})
}
