package application

import (
	"context"
	"encoding/json"
)

type presentationGroupAccess interface {
	GroupPresentationElements(context.Context, uint32, []uint32) (uint32, bool, error)
}

func registerPresentationGroupCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.group.set", Name: "组合演示文稿元素",
			Description: "将同一张幻灯片上 2 到 100 个明确指定的顶层元素组合。element_ids 必须来自 document.inspect；元素必须相邻并具有显式几何，避免改变 z-order 或占位符继承。组合以 DrawingML p:grpSp 保存，子元素 ID、外观、顺序和坐标保持不变；元素仍可单独由 Agent 查询和编辑。复杂或未知结构会拒绝修改。风险高，MCP 调用须由桌面人类确认。Ribbon“开始/组合”在使用 Shift 或 Command/Ctrl 多选后复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"element_ids":{"type":"array","minItems":2,"maxItems":100,"uniqueItems":true,"items":{"type":"integer","minimum":1,"maximum":4294967295},"description":"要组合的相邻顶层元素 ID。"}},"required":["slide_id","element_ids"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"group_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_ids":{"type":"array","items":{"type":"integer","minimum":1,"maximum":4294967295}}},"required":["changed","slide_id","group_id","element_ids"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID    uint64   `json:"slide_id"`
				ElementIDs []uint64 `json:"element_ids"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || len(input.ElementIDs) < 2 || len(input.ElementIDs) > 100 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			ids := make([]uint32, len(input.ElementIDs))
			seen := make(map[uint32]bool, len(ids))
			for index, id := range input.ElementIDs {
				if id == 0 || id > uint64(^uint32(0)) || seen[uint32(id)] {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				ids[index] = uint32(id)
				seen[uint32(id)] = true
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			access, ok := capability.Adapter.(presentationGroupAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			groupID, changed, err := access.GroupPresentationElements(ctx, uint32(input.SlideID), ids)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed    bool     `json:"changed"`
				SlideID    uint32   `json:"slide_id"`
				GroupID    uint32   `json:"group_id"`
				ElementIDs []uint32 `json:"element_ids"`
			}{changed, uint32(input.SlideID), groupID, ids}, changed)
		},
	})
}
