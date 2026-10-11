package application

import (
	"context"
	"encoding/json"
	"strings"
)

type presentationSlideBackgroundEditor interface {
	SetPresentationSlideBackground(context.Context, uint32, string) (bool, error)
}

func registerPresentationBackgroundCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.slide.background.set", Name: "设置幻灯片背景颜色",
			Description: "将指定 PPTX 幻灯片的背景填充设为纯色 RGB。slide_id 使用 document.inspect 返回的稳定 ID；color_hex 使用 #RRGGBB。此操作替换该页现有背景填充，保留背景属性中的其他效果；不修改主题、版式或母版。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"color_hex":{"type":"string","pattern":"^#[0-9A-Fa-f]{6}$","description":"纯色背景，格式为 #RRGGBB。"}},"required":["slide_id","color_hex"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"color_hex":{"type":"string","pattern":"^#[0-9A-F]{6}$"}},"required":["changed","slide_id","color_hex"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID  uint64 `json:"slide_id"`
				ColorHex string `json:"color_hex"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			input.ColorHex = strings.TrimSpace(input.ColorHex)
			if len(input.ColorHex) != 7 || input.ColorHex[0] != '#' {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			for _, digit := range input.ColorHex[1:] {
				if !strings.ContainsRune("0123456789abcdefABCDEF", digit) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			editor, ok := capability.Adapter.(presentationSlideBackgroundEditor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			colorHex := "#" + strings.ToUpper(input.ColorHex[1:])
			changed, err := editor.SetPresentationSlideBackground(ctx, uint32(input.SlideID), colorHex)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed  bool   `json:"changed"`
				SlideID  uint32 `json:"slide_id"`
				ColorHex string `json:"color_hex"`
			}{changed, uint32(input.SlideID), colorHex}, changed)
		},
	})
}
