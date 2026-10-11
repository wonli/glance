package application

import (
	"context"
	"encoding/json"

	"glance/internal/document"
)

const maxPresentationLayoutOptions = 500

type presentationLayoutReader interface {
	PresentationLayouts(context.Context) (document.PresentationLayoutSummary, error)
}

type presentationSlideLayoutEditor interface {
	SetPresentationSlideLayout(context.Context, uint32, int, uint64) (document.PresentationLayoutOption, bool, error)
}

func registerPresentationLayoutCapabilities(catalog *Catalog) error {
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.layouts.list", Name: "列出演示文稿版式",
			Description: "读取当前 PPTX 文档可用的版式目录，按文档内顺序返回零基 layout_index、名称及读取时 revision。layout_index 只在该 revision 下有效；执行后续写操作前应重新读取并使用对应修订。最多返回 500 项；截断时会报告 total 和 truncated，超过 256 个 Unicode 字符的版式名称会附带 name_truncated=true。此查询不修改文档。",
			Category:    CapabilityQuery, Formats: []Format{FormatPPTX}, Risk: RiskLow, RequiresDocument: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"revision":{"type":"integer","minimum":0},"total":{"type":"integer","minimum":0},"truncated":{"type":"boolean"},"layouts":{"type":"array","maxItems":500,"items":{"type":"object","properties":{"index":{"type":"integer","minimum":0,"maximum":499},"name":{"type":"string","maxLength":256},"name_truncated":{"type":"boolean"}},"required":["index","name"],"additionalProperties":false}}},"required":["revision","total","truncated","layouts"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input map[string]json.RawMessage
			if err := json.Unmarshal(raw, &input); err != nil || input == nil || len(input) != 0 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			reader, ok := capability.Adapter.(presentationLayoutReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			result, err := reader.PresentationLayouts(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(result, false)
		},
	}); err != nil {
		return err
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.slide.layout.set", Name: "设置幻灯片版式",
			Description: "为指定 PPTX 幻灯片应用 presentation.layouts.list 返回的文档内版式。slide_id 必须来自 document.inspect；layout_index 为零基索引，并且必须来自当前文档修订的 presentation.layouts.list。通过 Command Bus 的 expected_revision 防止目录过期。切换版式会更新继承的母版/版式元素和占位符位置，保留幻灯片自己的对象及明确背景设置。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"layout_index":{"type":"integer","minimum":0,"maximum":499,"description":"presentation.layouts.list 在同一 expected_revision 下返回的零基版式索引。"}},"required":["slide_id","layout_index"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"layout_index":{"type":"integer","minimum":0,"maximum":499},"layout_name":{"type":"string","maxLength":256},"name_truncated":{"type":"boolean"}},"required":["changed","slide_id","layout_index","layout_name"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID     uint64 `json:"slide_id"`
				LayoutIndex int    `json:"layout_index"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.LayoutIndex < 0 || input.LayoutIndex >= maxPresentationLayoutOptions {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			editor, ok := capability.Adapter.(presentationSlideLayoutEditor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			option, changed, err := editor.SetPresentationSlideLayout(ctx, uint32(input.SlideID), input.LayoutIndex, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed       bool   `json:"changed"`
				SlideID       uint32 `json:"slide_id"`
				LayoutIndex   int    `json:"layout_index"`
				LayoutName    string `json:"layout_name"`
				NameTruncated bool   `json:"name_truncated,omitempty"`
			}{changed, uint32(input.SlideID), option.Index, option.Name, option.NameTruncated}, changed)
		},
	})
}
