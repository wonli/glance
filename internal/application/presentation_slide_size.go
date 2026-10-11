package application

import (
	"context"
	"encoding/json"
	"math"
)

type presentationSlideSizeEditor interface {
	SetPresentationSlideSize(context.Context, float64, float64) (bool, error)
}

func registerPresentationSlideSizeCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.slide.size.set", Name: "设置演示文稿页面尺寸",
			Description: "设置整个 PPTX 演示文稿的画布宽高，单位为 point（72 point = 1 inch），范围为 36–4032 point。先用 document.inspect 读取当前 slide_width_pt 与 slide_height_pt，再传入 expected_revision。修改画布尺寸不会缩放或移动现有对象；对象保持原绝对 point 坐标，可能超出新的画布边界。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"width_pt":{"type":"number","minimum":36,"maximum":4032,"description":"新画布宽度，单位 point。"},"height_pt":{"type":"number","minimum":36,"maximum":4032,"description":"新画布高度，单位 point。"}},"required":["width_pt","height_pt"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"width_pt":{"type":"number"},"height_pt":{"type":"number"}},"required":["changed","width_pt","height_pt"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				WidthPT  float64 `json:"width_pt"`
				HeightPT float64 `json:"height_pt"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.WidthPT < 36 || input.WidthPT > 4032 || input.HeightPT < 36 || input.HeightPT > 4032 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			input.WidthPT = math.Round(input.WidthPT*12700) / 12700
			input.HeightPT = math.Round(input.HeightPT*12700) / 12700
			editor, ok := capability.Adapter.(presentationSlideSizeEditor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := editor.SetPresentationSlideSize(ctx, input.WidthPT, input.HeightPT)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed  bool    `json:"changed"`
				WidthPT  float64 `json:"width_pt"`
				HeightPT float64 `json:"height_pt"`
			}{changed, input.WidthPT, input.HeightPT}, changed)
		},
	})
}
