package application

import (
	"context"
	"encoding/json"
	"fmt"
)

type presentationElementAnimationEditor interface {
	SetPresentationElementAnimation(context.Context, uint32, int64, string, int, uint64) (bool, error)
}

type presentationAnimationDurationEditor interface {
	SetPresentationAnimationDuration(context.Context, uint32, int64, int, int, uint64) (bool, error)
}

type presentationAnimationDeleteEditor interface {
	DeletePresentationAnimation(context.Context, uint32, int64, int, uint64) (int, error)
}

func registerPresentationAnimationEditCapability(catalog *Catalog) error {
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.animation.add", Name: "为幻灯片元素添加进入动画",
			Description: "为指定 PPTX 幻灯片上的顶层文本框、形状、图片或连接符追加一个点击触发的进入动画。slide_id 和 element_id 必须来自 document.inspect；effect 支持 appear、fade、fly_in（从左侧进入），duration_ms 为 1–60000 毫秒，省略时为 500。该工具保留已有时序 XML 并追加新动画；遇到无法安全扩展的时序结构会拒绝修改。Command Bus 的 expected_revision 必须与最近查询结果一致。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的幻灯片顶层元素 ID。"},"effect":{"type":"string","enum":["appear","fade","fly_in"],"description":"进入效果；fly_in 从左侧进入。"},"duration_ms":{"type":"integer","minimum":1,"maximum":60000,"default":500,"description":"动画时长，单位毫秒。appear 是即时效果，时长固定为 1 毫秒。"}},"required":["slide_id","element_id","effect"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"effect":{"type":"string","enum":["appear","fade","fly_in"]},"duration_ms":{"type":"integer","minimum":1,"maximum":60000}},"required":["changed","slide_id","element_id","effect","duration_ms"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			input, err := decodePresentationAnimationEdit(raw)
			if err != nil {
				return CapabilityResult{}, err
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			editor, ok := capability.Adapter.(presentationElementAnimationEditor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := editor.SetPresentationElementAnimation(ctx, uint32(input.SlideID), int64(input.ElementID), input.Effect, input.DurationMS, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed    bool   `json:"changed"`
				SlideID    uint32 `json:"slide_id"`
				ElementID  uint32 `json:"element_id"`
				Effect     string `json:"effect"`
				DurationMS int    `json:"duration_ms"`
			}{changed, uint32(input.SlideID), uint32(input.ElementID), input.Effect, input.DurationMS}, changed)
		},
	}); err != nil {
		return err
	}
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.animation.duration.set", Name: "设置幻灯片动画时长",
			Description: "修改指定 PPTX 幻灯片上某个顶层元素的一条动画时长。先调用 presentation.animations.list，根据其 XML 顺序的 index、kind、target_element_id 选中 kind=animEffect 或 animMotion 且 target_element_id 与 element_id 相同的条目；animation_index 是该列表中的一基序号。持续时间为 1–60000 毫秒。appear 的即时可见性 set 节点没有可调时长。修改保留其余动画 XML；Command Bus 的 expected_revision 必须与最近查询结果一致。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"动画目标的稳定顶层元素 ID。"},"animation_index":{"type":"integer","minimum":1,"maximum":500,"description":"presentation.animations.list 返回的一基 XML 顺序序号；目标必须是该元素的 animEffect 或 animMotion。"},"duration_ms":{"type":"integer","minimum":1,"maximum":60000,"description":"新的动画持续时间，单位毫秒。"}},"required":["slide_id","element_id","animation_index","duration_ms"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"animation_index":{"type":"integer","minimum":1,"maximum":500},"duration_ms":{"type":"integer","minimum":1,"maximum":60000}},"required":["changed","slide_id","element_id","animation_index","duration_ms"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			input, err := decodePresentationAnimationDuration(raw)
			if err != nil {
				return CapabilityResult{}, err
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			editor, ok := capability.Adapter.(presentationAnimationDurationEditor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := editor.SetPresentationAnimationDuration(ctx, uint32(input.SlideID), int64(input.ElementID), input.AnimationIndex, input.DurationMS, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed        bool   `json:"changed"`
				SlideID        uint32 `json:"slide_id"`
				ElementID      uint32 `json:"element_id"`
				AnimationIndex int    `json:"animation_index"`
				DurationMS     int    `json:"duration_ms"`
			}{changed, uint32(input.SlideID), uint32(input.ElementID), input.AnimationIndex, input.DurationMS}, changed)
		},
	}); err != nil {
		return err
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.animation.delete", Name: "删除幻灯片动画",
			Description: "先调用 presentation.animations.list，并使用同一修订号的 slide_id、element_id 和 animation_index。仅支持 Glance 生成的简单进入动画组：appear 选择 set 项，fade/fly_in 选择属于目标元素的 animEffect 或配套 set 项；会删除整个动画组及其配套可见性节点。复杂、共享目标或无法明确归属的时序会拒绝修改，其余动画保持不变。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"动画目标的稳定顶层元素 ID。"},"animation_index":{"type":"integer","minimum":1,"maximum":500,"description":"presentation.animations.list 返回的一基 XML 顺序序号；目标必须是该元素所属简单动画组中的 set 或 animEffect。"}},"required":["slide_id","element_id","animation_index"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"animation_index":{"type":"integer","minimum":1,"maximum":500},"removed_animation_nodes":{"type":"integer","minimum":1,"maximum":2}},"required":["changed","slide_id","element_id","animation_index","removed_animation_nodes"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			input, err := decodePresentationAnimationDelete(raw)
			if err != nil {
				return CapabilityResult{}, err
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			editor, ok := capability.Adapter.(presentationAnimationDeleteEditor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			removed, err := editor.DeletePresentationAnimation(ctx, uint32(input.SlideID), int64(input.ElementID), input.AnimationIndex, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed               bool   `json:"changed"`
				SlideID               uint32 `json:"slide_id"`
				ElementID             uint32 `json:"element_id"`
				AnimationIndex        int    `json:"animation_index"`
				RemovedAnimationNodes int    `json:"removed_animation_nodes"`
			}{true, uint32(input.SlideID), uint32(input.ElementID), input.AnimationIndex, removed}, true)
		},
	})
}

type presentationAnimationDurationInput struct {
	SlideID, ElementID uint64
	AnimationIndex     int
	DurationMS         int
}

func decodePresentationAnimationDuration(raw json.RawMessage) (presentationAnimationDurationInput, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return presentationAnimationDurationInput{}, ErrInvalidCommandArguments
	}
	for key := range fields {
		switch key {
		case "slide_id", "element_id", "animation_index", "duration_ms":
		default:
			return presentationAnimationDurationInput{}, ErrInvalidCommandArguments
		}
	}
	var input struct {
		SlideID        uint64 `json:"slide_id"`
		ElementID      uint64 `json:"element_id"`
		AnimationIndex int    `json:"animation_index"`
		DurationMS     int    `json:"duration_ms"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID == 0 || input.ElementID > uint64(^uint32(0)) || input.AnimationIndex < 1 || input.AnimationIndex > 500 || input.DurationMS < 1 || input.DurationMS > 60000 {
		return presentationAnimationDurationInput{}, ErrInvalidCommandArguments
	}
	return presentationAnimationDurationInput{SlideID: input.SlideID, ElementID: input.ElementID, AnimationIndex: input.AnimationIndex, DurationMS: input.DurationMS}, nil
}

type presentationAnimationDeleteInput struct {
	SlideID, ElementID uint64
	AnimationIndex     int
}

func decodePresentationAnimationDelete(raw json.RawMessage) (presentationAnimationDeleteInput, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return presentationAnimationDeleteInput{}, ErrInvalidCommandArguments
	}
	for key := range fields {
		switch key {
		case "slide_id", "element_id", "animation_index":
		default:
			return presentationAnimationDeleteInput{}, ErrInvalidCommandArguments
		}
	}
	var input struct {
		SlideID        uint64 `json:"slide_id"`
		ElementID      uint64 `json:"element_id"`
		AnimationIndex int    `json:"animation_index"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID == 0 || input.ElementID > uint64(^uint32(0)) || input.AnimationIndex < 1 || input.AnimationIndex > 500 {
		return presentationAnimationDeleteInput{}, ErrInvalidCommandArguments
	}
	return presentationAnimationDeleteInput{SlideID: input.SlideID, ElementID: input.ElementID, AnimationIndex: input.AnimationIndex}, nil
}

type presentationAnimationEditInput struct {
	SlideID    uint64
	ElementID  uint64
	Effect     string
	DurationMS int
}

func decodePresentationAnimationEdit(raw json.RawMessage) (presentationAnimationEditInput, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return presentationAnimationEditInput{}, ErrInvalidCommandArguments
	}
	for key := range fields {
		switch key {
		case "slide_id", "element_id", "effect", "duration_ms":
		default:
			return presentationAnimationEditInput{}, ErrInvalidCommandArguments
		}
	}
	var input struct {
		SlideID    uint64 `json:"slide_id"`
		ElementID  uint64 `json:"element_id"`
		Effect     string `json:"effect"`
		DurationMS *int   `json:"duration_ms"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID == 0 || input.ElementID > uint64(^uint32(0)) {
		return presentationAnimationEditInput{}, ErrInvalidCommandArguments
	}
	duration := 500
	if input.DurationMS != nil {
		duration = *input.DurationMS
	}
	if duration < 1 || duration > 60000 {
		return presentationAnimationEditInput{}, ErrInvalidCommandArguments
	}
	if input.Effect != "appear" && input.Effect != "fade" && input.Effect != "fly_in" {
		return presentationAnimationEditInput{}, fmt.Errorf("%w: effect 必须为 appear、fade 或 fly_in", ErrInvalidCommandArguments)
	}
	if input.Effect == "appear" {
		duration = 1
	}
	return presentationAnimationEditInput{SlideID: input.SlideID, ElementID: input.ElementID, Effect: input.Effect, DurationMS: duration}, nil
}
