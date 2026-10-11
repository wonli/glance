package application

import (
	"context"
	"encoding/json"
	"fmt"
)

func wordViewCapabilityRegistrations() []CapabilityRegistration {
	registrations := []CapabilityRegistration{{
		Descriptor: CapabilityDescriptor{
			ID: "word.view.get", Name: "读取 Word 窗口视图",
			Description: "读取指定桌面窗口中的 Word 版式、阅读模式、标尺、网格线、导航窗格、分页布局和缩放。",
			Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: wordViewStateSchema(),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			return invokeWordViewIntent(ctx, capability, "word.view.get", json.RawMessage(`{}`))
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "word.view.set", Name: "设置 Word 窗口视图",
			Description: "设置 Word 窗口视图或正文选择；一次调用可组合设置版式、阅读模式、标尺、网格线、导航窗格、多页布局、缩放和 selection。selection.scope=document 选择全文；scope=range 通过 document.inspect 返回的稳定 run_id 与 Unicode 字符偏移选择范围；scope=clear 清除选择。只影响指定窗口，不修改文档内容。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatDocx}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"mode":{"type":"string","enum":["continuous","print","web"]},"reading_mode":{"type":"boolean"},"ruler_visible":{"type":"boolean"},"gridlines_visible":{"type":"boolean"},"navigation_visible":{"type":"boolean"},"multiple_pages":{"type":"boolean"},"zoom_percent":{"type":"integer","minimum":50,"maximum":150},"selection":{"type":"object","description":"设置当前桌面窗口中的正文选择。run_id 和 offset 必须来自当前修订的 document.inspect；offset 按 Unicode 字符计数。","properties":{"scope":{"type":"string","enum":["document","range","clear"]},"start_run_id":{"type":"string","minLength":1},"start_offset":{"type":"integer","minimum":0},"end_run_id":{"type":"string","minLength":1},"end_offset":{"type":"integer","minimum":0}},"required":["scope"],"additionalProperties":false}},"additionalProperties":false}`),
			OutputSchema: wordViewStateSchema(),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Mode              *string `json:"mode"`
				ReadingMode       *bool   `json:"reading_mode"`
				RulerVisible      *bool   `json:"ruler_visible"`
				GridlinesVisible  *bool   `json:"gridlines_visible"`
				NavigationVisible *bool   `json:"navigation_visible"`
				MultiplePages     *bool   `json:"multiple_pages"`
				ZoomPercent       *int    `json:"zoom_percent"`
				Selection         *struct {
					Scope       string  `json:"scope"`
					StartRunID  *string `json:"start_run_id"`
					StartOffset *int    `json:"start_offset"`
					EndRunID    *string `json:"end_run_id"`
					EndOffset   *int    `json:"end_offset"`
				} `json:"selection"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode word.view.set: %w", err)
			}
			if input.Mode == nil && input.ReadingMode == nil && input.RulerVisible == nil && input.GridlinesVisible == nil && input.NavigationVisible == nil && input.MultiplePages == nil && input.ZoomPercent == nil && input.Selection == nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if input.Selection != nil {
				switch input.Selection.Scope {
				case "document", "clear":
					if input.Selection.StartRunID != nil || input.Selection.StartOffset != nil || input.Selection.EndRunID != nil || input.Selection.EndOffset != nil {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				case "range":
					if input.Selection.StartRunID == nil || *input.Selection.StartRunID == "" || input.Selection.StartOffset == nil || *input.Selection.StartOffset < 0 || input.Selection.EndRunID == nil || *input.Selection.EndRunID == "" || input.Selection.EndOffset == nil || *input.Selection.EndOffset < 0 {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				default:
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			}
			if input.Mode != nil && *input.Mode != "continuous" && *input.Mode != "print" && *input.Mode != "web" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if input.ZoomPercent != nil && (*input.ZoomPercent < 50 || *input.ZoomPercent > 150) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if (input.MultiplePages != nil && input.Mode != nil && *input.Mode != "print") || (input.MultiplePages != nil && input.ZoomPercent != nil && *input.MultiplePages) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			return invokeWordViewIntent(ctx, capability, "word.view.set", append(json.RawMessage(nil), raw...))
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "word.zoom.dialog", Name: "打开 Word 缩放设置",
			Description: "在指定桌面窗口打开 Word 缩放输入框。Agent 也可以直接调用 word.view.set 设置 zoom_percent。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatDocx}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"opened":{"type":"boolean"}},"required":["window_id","opened"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			return invokeWordViewIntent(ctx, capability, "word.zoom.dialog", json.RawMessage(`{}`))
		},
	}}
	registrations = append(registrations, CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.autosave.set", Name: "设置 Word 自动保存",
			Description: "在指定桌面窗口启用或关闭当前 Word 会话的自动保存。MCP 调用需要桌面人类确认，因为启用后 Glance 会自动写回后续编辑。新建文档会保持启用并等待另存为。只影响桌面会话，不修改文档内容。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatDocx}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"enabled":{"type":"boolean","description":"是否启用自动保存。"}},"required":["enabled"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"enabled":{"type":"boolean"},"effective":{"type":"boolean"},"changed":{"type":"boolean"}},"required":["window_id","enabled","effective","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Enabled *bool `json:"enabled"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.Enabled == nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			return invokeWordViewIntent(ctx, capability, "word.autosave.set", append(json.RawMessage(nil), raw...))
		},
	}, CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.view.new_window", Name: "在新窗口打开 Word",
			Description: "在新的桌面窗口打开同一 Word 实时会话，并复制当前窗口视图。要求附着的桌面窗口和支持多窗口的平台。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatDocx}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"source_window_id":{"type":"integer","minimum":1},"document_id":{"type":"string"},"requested":{"type":"boolean"}},"required":["source_window_id","document_id","requested"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			return invokeWordViewIntent(ctx, capability, "word.view.new_window", json.RawMessage(`{}`))
		},
	}, CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.view.side_by_side", Name: "并排查看 Word",
			Description: "在已有多个窗口时保持并排查看；否则尝试打开同一文档的新窗口，不支持时切换到双页并排视图。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatDocx}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"strategy":{"type":"string","enum":["existing_windows","new_window","two_page"]}},"required":["window_id","strategy"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			return invokeWordViewIntent(ctx, capability, "word.view.side_by_side", json.RawMessage(`{}`))
		},
	})
	return registrations
}

func invokeWordViewIntent(ctx context.Context, capability CapabilityContext, action string, arguments json.RawMessage) (CapabilityResult, error) {
	result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
		WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
		Revision: capability.Document.Revision, Action: action, Arguments: arguments,
	})
	if err != nil {
		return CapabilityResult{}, err
	}
	return CapabilityResult{Data: result}, nil
}

func wordViewStateSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"mode":{"type":"string","enum":["continuous","print","web"]},"reading_mode":{"type":"boolean"},"ruler_visible":{"type":"boolean"},"gridlines_visible":{"type":"boolean"},"navigation_visible":{"type":"boolean"},"page_fit":{"type":"boolean"},"multiple_pages":{"type":"boolean"},"zoom_percent":{"type":"integer","minimum":50,"maximum":150},"changed":{"type":"boolean"},"selection":{"type":"object","properties":{"scope":{"type":"string","enum":["none","document","range"]},"start_run_id":{"type":"string"},"start_offset":{"type":"integer","minimum":0},"end_run_id":{"type":"string"},"end_offset":{"type":"integer","minimum":0},"selected_runes":{"type":"integer","minimum":0}},"required":["scope","start_run_id","start_offset","end_run_id","end_offset","selected_runes"],"additionalProperties":false}},"required":["window_id","mode","reading_mode","ruler_visible","gridlines_visible","navigation_visible","page_fit","multiple_pages","zoom_percent","changed","selection"],"additionalProperties":false}`)
}
