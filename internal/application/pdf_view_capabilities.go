package application

import (
	"context"
	"encoding/json"
	"fmt"
)

func registerPDFViewCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{{
		Descriptor: CapabilityDescriptor{
			ID: "pdf.view.get", Name: "读取 PDF 窗口视图",
			Description: "读取指定桌面窗口中 PDF 的当前页、页面总数、缩放、适配方式、阅读模式、缩略图和搜索栏状态。该信息属于窗口视图，不修改 PDF 内容。",
			Category:    CapabilityQuery, Formats: []Format{FormatPDF}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: pdfViewStateSchema(),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			return invokePDFViewIntent(ctx, capability, "pdf.view.get", json.RawMessage(`{}`))
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "pdf.view.set", Name: "设置 PDF 窗口视图",
			Description: "在指定桌面窗口设置 PDF 当前页、缩放、页面适配、缩略图、搜索栏和阅读/批注/签名模式。只影响窗口视图，不修改文档内容；缩放范围为 25% 到 400%，页码从 1 开始。签名模式只提供手写墨迹，不是数字证书签名。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatPDF}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["set_page","previous","next","set_zoom","zoom_in","zoom_out","reset_zoom","fit_width","fit_page","set_thumbnails","set_search_visible","set_mode","set_tool"]},"page_number":{"type":"integer","minimum":1,"maximum":50000},"zoom_percent":{"type":"integer","minimum":25,"maximum":400},"enabled":{"type":"boolean"},"mode":{"type":"string","enum":["read","edit","annotate","signature"]},"tool":{"type":"string","enum":["","highlight","ink"]}},"required":["operation"],"additionalProperties":false}`),
			OutputSchema: pdfViewStateSchema(),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if err := ValidatePDFViewArguments(raw); err != nil {
				return CapabilityResult{}, err
			}
			return invokePDFViewIntent(ctx, capability, "pdf.view.set", raw)
		},
	}}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

func invokePDFViewIntent(ctx context.Context, capability CapabilityContext, action string, arguments json.RawMessage) (CapabilityResult, error) {
	result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
		WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
		Revision: capability.Document.Revision, Action: action, Arguments: arguments,
	})
	if err != nil {
		return CapabilityResult{}, err
	}
	return CapabilityResult{Data: result}, nil
}

// ValidatePDFViewArguments enforces the operation-specific PDF view arguments
// for both MCP invocations and the Ribbon's in-process dispatcher.
func ValidatePDFViewArguments(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return ErrInvalidCommandArguments
	}
	for key := range fields {
		switch key {
		case "operation", "page_number", "zoom_percent", "enabled", "mode":
		case "tool":
		default:
			return ErrInvalidCommandArguments
		}
	}
	var input struct {
		Operation   string `json:"operation"`
		PageNumber  *int   `json:"page_number"`
		ZoomPercent *int   `json:"zoom_percent"`
		Enabled     *bool  `json:"enabled"`
		Mode        string `json:"mode"`
		Tool        string `json:"tool"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return fmt.Errorf("decode pdf.view.set: %w", ErrInvalidCommandArguments)
	}
	noOtherArgs := func() bool {
		return input.PageNumber == nil && input.ZoomPercent == nil && input.Enabled == nil && input.Mode == "" && input.Tool == ""
	}
	switch input.Operation {
	case "set_page":
		if input.PageNumber == nil || *input.PageNumber < 1 || *input.PageNumber > 50000 || input.ZoomPercent != nil || input.Enabled != nil || input.Mode != "" || input.Tool != "" {
			return ErrInvalidCommandArguments
		}
	case "previous", "next", "zoom_in", "zoom_out", "reset_zoom", "fit_width", "fit_page":
		if !noOtherArgs() {
			return ErrInvalidCommandArguments
		}
	case "set_zoom":
		if input.ZoomPercent == nil || *input.ZoomPercent < 25 || *input.ZoomPercent > 400 || input.PageNumber != nil || input.Enabled != nil || input.Mode != "" || input.Tool != "" {
			return ErrInvalidCommandArguments
		}
	case "set_thumbnails":
		if input.Enabled == nil || input.PageNumber != nil || input.ZoomPercent != nil || input.Mode != "" || input.Tool != "" {
			return ErrInvalidCommandArguments
		}
	case "set_search_visible":
		if input.Enabled == nil || input.PageNumber != nil || input.ZoomPercent != nil || input.Mode != "" || input.Tool != "" {
			return ErrInvalidCommandArguments
		}
	case "set_mode":
		if input.Mode != "read" && input.Mode != "edit" && input.Mode != "annotate" && input.Mode != "signature" || input.PageNumber != nil || input.ZoomPercent != nil || input.Enabled != nil || input.Tool != "" {
			return ErrInvalidCommandArguments
		}
	case "set_tool":
		if input.Tool != "" && input.Tool != "highlight" && input.Tool != "ink" || input.PageNumber != nil || input.ZoomPercent != nil || input.Enabled != nil || input.Mode != "" {
			return ErrInvalidCommandArguments
		}
	default:
		return ErrInvalidCommandArguments
	}
	return nil
}

func pdfViewStateSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"page_number":{"type":"integer","minimum":1},"page_count":{"type":"integer","minimum":1},"zoom_percent":{"type":"integer"},"fit_mode":{"type":"string","enum":["custom","width","page"]},"mode":{"type":"string","enum":["read","edit","annotate","signature"]},"tool":{"type":"string","enum":["","highlight","ink"]},"thumbnails":{"type":"boolean"},"search_visible":{"type":"boolean"},"changed":{"type":"boolean"}},"required":["window_id","page_number","page_count","zoom_percent","fit_mode","mode","tool","thumbnails","search_visible","changed"],"additionalProperties":false}`)
}
