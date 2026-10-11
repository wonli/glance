package application

import (
	"context"
	"encoding/json"
)

// PresentationCommandExecutor keeps the presentation library behind the
// application adapter boundary.
type PresentationCommandExecutor interface {
	AddPresentationSlide(context.Context) (int, error)
	DeletePresentationSlide(context.Context, uint32) (int, error)
}

type PresentationSlideDuplicateExecutor interface {
	DuplicatePresentationSlide(context.Context, uint32) (uint32, int, error)
}

func registerPresentationCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.slide.add", Name: "添加演示文稿幻灯片",
			Description: "在明确的 PPTX 文档末尾追加一张默认版式幻灯片，并创建标题和正文占位内容。页码从 1 开始；操作要求 expected_revision。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_number":{"type":"integer","minimum":1},"slide_count":{"type":"integer","minimum":1}},"required":["slide_number","slide_count"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationCommandExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			count, err := executor.AddPresentationSlide(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			if count < 1 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SlideNumber int `json:"slide_number"`
				SlideCount  int `json:"slide_count"`
			}{SlideNumber: count, SlideCount: count}, true)
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "presentation.slide.delete", Name: "删除演示文稿幻灯片",
			Description: "按 document.inspect 返回的稳定 slide_id 删除明确的 PPTX 幻灯片；不能删除最后一页。此操作会永久移除该页及其内容，Agent 调用必须经过桌面人类确认；Ribbon 人工操作复用同一处理器。调用要求 expected_revision，并会拒绝桌面未提交的演示文稿草稿。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID，不是当前页码。"}},"required":["slide_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"deleted_slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"slide_count":{"type":"integer","minimum":1}},"required":["deleted_slide_id","slide_count"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID uint64 `json:"slide_id"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationCommandExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			count, err := executor.DeletePresentationSlide(ctx, uint32(input.SlideID))
			if err != nil {
				return CapabilityResult{}, err
			}
			if count < 1 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				DeletedSlideID uint32 `json:"deleted_slide_id"`
				SlideCount     int    `json:"slide_count"`
			}{DeletedSlideID: uint32(input.SlideID), SlideCount: count}, true)
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "presentation.slide.duplicate", Name: "复制演示文稿幻灯片",
			Description: "按 document.inspect 返回的稳定 slide_id 将整张 PPTX 幻灯片复制到文稿末尾。复制文本、形状、图片、表格、图表、音视频、演讲者备注、转场和动画计时树；图表数据独立复制，媒体默认共享。若页面包含无法安全重映射的未知内部关系，格式适配器会拒绝操作。此 Agent 工作流不依赖桌面当前选区，要求 expected_revision，并会拒绝未提交的桌面文本草稿。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定源幻灯片 ID，不是当前页码。"}},"required":["slide_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"source_slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"新建副本的稳定幻灯片 ID。"},"slide_number":{"type":"integer","minimum":1},"slide_count":{"type":"integer","minimum":1}},"required":["source_slide_id","slide_id","slide_number","slide_count"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID uint64 `json:"slide_id"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationSlideDuplicateExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			duplicateID, slideCount, err := executor.DuplicatePresentationSlide(ctx, uint32(input.SlideID))
			if err != nil {
				return CapabilityResult{}, err
			}
			if duplicateID == 0 || slideCount < 1 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SourceSlideID uint32 `json:"source_slide_id"`
				SlideID       uint32 `json:"slide_id"`
				SlideNumber   int    `json:"slide_number"`
				SlideCount    int    `json:"slide_count"`
			}{uint32(input.SlideID), duplicateID, slideCount, slideCount}, true)
		},
	}}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}
