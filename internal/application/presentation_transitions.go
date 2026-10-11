package application

import (
	"context"
	"encoding/json"
	"fmt"

	"glance/internal/document"
)

type presentationTransitionReader interface {
	GetPresentationTransition(context.Context, uint32) (document.PresentationTransition, error)
}

type presentationTransitionEditor interface {
	SetPresentationTransition(context.Context, uint32, document.PresentationTransitionPatch, bool) (document.PresentationTransition, []uint32, bool, error)
}

func registerPresentationTransitionCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.transition.get", Name: "读取幻灯片切换效果",
				Description: "按稳定 slide_id 读取该页切换类型、速度、方向、分割参数、through_black 和点击翻页设置。未设置切换效果时返回 type=none。库无法安全读取的扩展切换会明确报错。",
				Category:    CapabilityQuery, Formats: []Format{FormatPPTX}, Risk: RiskLow,
				RequiresDocument: true,
				InputSchema:      json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"}},"required":["slide_id"],"additionalProperties":false}`),
				OutputSchema:     presentationTransitionGetOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					SlideID uint64 `json:"slide_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				reader, ok := capability.Adapter.(presentationTransitionReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				transition, err := reader.GetPresentationTransition(ctx, uint32(input.SlideID))
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					SlideID    uint32                          `json:"slide_id"`
					Transition document.PresentationTransition `json:"transition"`
				}{uint32(input.SlideID), transition}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.transition.set", Name: "设置幻灯片切换效果",
				Description: "按稳定 slide_id 设置 PPTX 切换类型、速度、方向或点击翻页。可通过 apply_to_all 将当前参数应用到整份演示文稿；设为 true 且省略所有切换字段时，会复制源幻灯片当前效果。可通过 presentation.transition.get 读取现有设置后只修改指定字段；新类型取代旧类型时会重置类型专属参数。Library 不支持的扩展切换会拒绝修改。",
				Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"transition_type":{"type":"string","enum":["none","fade","push","wipe","split","cover","cut","dissolve"],"description":"切换类型；可省略以保留现有类型，只更新其它字段。"},"speed":{"type":"string","enum":["slow","med","fast"],"description":"切换速度档位，分别代表慢、中、快；可省略以保留现有值。"},"direction":{"type":"string","enum":["l","r","u","d"],"description":"推进、擦除或覆盖的方向：左、右、上、下；仅适用于 push/wipe/cover。"},"split_direction":{"type":"string","enum":["in","out"],"description":"分割效果向内或向外；仅适用于 split。"},"split_axis":{"type":"string","enum":["hor","vert"],"description":"分割效果的水平或垂直轴；仅适用于 split。"},"through_black":{"type":"boolean","description":"fade 或 cut 是否经过黑场。"},"advance_on_click":{"type":"boolean","description":"放映时是否允许点击推进到下一页。"},"apply_to_all":{"type":"boolean","description":"是否将合并后的设置应用于全部幻灯片。设为 true 且省略其它切换字段时，复制源幻灯片当前效果。"}},"required":["slide_id"],"additionalProperties":false}`),
				OutputSchema: presentationTransitionSetOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				input, err := decodePresentationTransitionSet(raw)
				if err != nil {
					return CapabilityResult{}, err
				}
				if pendingPresentationDraft(capability) {
					return CapabilityResult{}, ErrPendingEditorDraft
				}
				editor, ok := capability.Adapter.(presentationTransitionEditor)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				transition, affected, changed, err := editor.SetPresentationTransition(ctx, uint32(input.SlideID), input.Patch, input.ApplyToAll)
				if err != nil {
					return CapabilityResult{}, err
				}
				if affected == nil {
					affected = []uint32{}
				}
				return capabilityJSONResult(struct {
					SlideID          uint32                          `json:"slide_id"`
					AffectedSlideIDs []uint32                        `json:"affected_slide_ids"`
					Transition       document.PresentationTransition `json:"transition"`
					Changed          bool                            `json:"changed"`
				}{uint32(input.SlideID), affected, transition, changed}, changed)
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

type presentationTransitionSetInput struct {
	SlideID    uint64
	ApplyToAll bool
	Patch      document.PresentationTransitionPatch
}

func decodePresentationTransitionSet(raw json.RawMessage) (presentationTransitionSetInput, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return presentationTransitionSetInput{}, ErrInvalidCommandArguments
	}
	for key := range fields {
		switch key {
		case "slide_id", "transition_type", "speed", "direction", "split_direction", "split_axis", "through_black", "advance_on_click", "apply_to_all":
		default:
			return presentationTransitionSetInput{}, ErrInvalidCommandArguments
		}
	}
	var input struct {
		SlideID        uint64  `json:"slide_id"`
		TransitionType *string `json:"transition_type"`
		Speed          *string `json:"speed"`
		Direction      *string `json:"direction"`
		SplitDirection *string `json:"split_direction"`
		SplitAxis      *string `json:"split_axis"`
		ThroughBlack   *bool   `json:"through_black"`
		AdvanceOnClick *bool   `json:"advance_on_click"`
		ApplyToAll     bool    `json:"apply_to_all"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) {
		return presentationTransitionSetInput{}, ErrInvalidCommandArguments
	}
	patch := document.PresentationTransitionPatch{
		Type: input.TransitionType, Speed: input.Speed, Direction: input.Direction,
		SplitDirection: input.SplitDirection, SplitAxis: input.SplitAxis,
		ThroughBlack: input.ThroughBlack, AdvanceOnClick: input.AdvanceOnClick,
	}
	if presentationTransitionPatchEmpty(patch) && !input.ApplyToAll {
		return presentationTransitionSetInput{}, fmt.Errorf("%w: 至少指定一个切换字段，或设置 apply_to_all=true", ErrInvalidCommandArguments)
	}
	if patch.Speed != nil && *patch.Speed != "slow" && *patch.Speed != "med" && *patch.Speed != "fast" {
		return presentationTransitionSetInput{}, ErrInvalidCommandArguments
	}
	if patch.Type != nil {
		switch *patch.Type {
		case "none", "fade", "push", "wipe", "split", "cover", "cut", "dissolve":
		default:
			return presentationTransitionSetInput{}, ErrInvalidCommandArguments
		}
	}
	return presentationTransitionSetInput{SlideID: input.SlideID, ApplyToAll: input.ApplyToAll, Patch: patch}, nil
}

func presentationTransitionPatchEmpty(patch document.PresentationTransitionPatch) bool {
	return patch.Type == nil && patch.Speed == nil && patch.Direction == nil && patch.SplitDirection == nil &&
		patch.SplitAxis == nil && patch.ThroughBlack == nil && patch.AdvanceOnClick == nil
}

func presentationTransitionGetOutputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"transition":{"type":"object","properties":{"type":{"type":"string","enum":["none","fade","push","wipe","split","cover","cut","dissolve"]},"speed":{"type":"string","enum":["slow","med","fast"]},"direction":{"type":"string","enum":["l","r","u","d"]},"split_direction":{"type":"string","enum":["in","out"]},"split_axis":{"type":"string","enum":["hor","vert"]},"through_black":{"type":"boolean"},"advance_on_click":{"type":"boolean"}},"required":["type","speed","through_black","advance_on_click"],"additionalProperties":false}},"required":["slide_id","transition"],"additionalProperties":false}`)
}

func presentationTransitionSetOutputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"affected_slide_ids":{"type":"array","items":{"type":"integer","minimum":1,"maximum":4294967295}},"transition":{"type":"object","properties":{"type":{"type":"string","enum":["none","fade","push","wipe","split","cover","cut","dissolve"]},"speed":{"type":"string","enum":["slow","med","fast"]},"direction":{"type":"string","enum":["l","r","u","d"]},"split_direction":{"type":"string","enum":["in","out"]},"split_axis":{"type":"string","enum":["hor","vert"]},"through_black":{"type":"boolean"},"advance_on_click":{"type":"boolean"}},"required":["type","speed","through_black","advance_on_click"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["slide_id","affected_slide_ids","transition","changed"],"additionalProperties":false}`)
}
