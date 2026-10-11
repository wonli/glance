package application

import (
	"context"
	"encoding/json"
	"fmt"
)

func registerPresentationViewCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.view.get", Name: "读取演示文稿窗口视图",
				Description: "读取指定桌面窗口中的当前幻灯片、页面总数、缩放、适应窗口、备注面板、放映状态和排练计时状态；只读窗口视图。排练计时包含运行状态与累计毫秒数。",
				Category:    CapabilityQuery, Formats: []Format{FormatPPTX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true, RequiresView: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
				OutputSchema: presentationViewStateSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				if err := validateEmptyPresentationViewArguments(raw); err != nil {
					return CapabilityResult{}, err
				}
				return invokePresentationViewIntent(ctx, capability, "presentation.view.get", json.RawMessage(`{}`))
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.view.set", Name: "设置演示文稿窗口视图",
				Description: "在指定窗口导航到幻灯片、设置 25–400% 缩放、适应窗口、显示备注面板、切换标尺、普通/幻灯片浏览/放映/演讲者视图，控制计时，或按明确图片 ID 开始/重置/取消裁剪预览。演讲者视图在当前窗口显示当前页、备注、下一页摘要并自动开始计时，不提供双屏独立控制台。排练计时由 rehearsal_start 启动放映并累计时间，rehearsal_stop 暂停累计，rehearsal_reset 清零；退出放映会自动停止计时，presentation.view.get 可读取累计毫秒数。选择幻灯片浏览卡片后返回普通视图并定位该页。裁剪预览不会改动文档；用 presentation.element.picture.crop.set 提交裁剪。页码从 1 开始。",
				Category:    CapabilityWorkflow, Formats: []Format{FormatPPTX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true, RequiresView: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["set_slide","set_zoom","fit","set_notes_visible","set_ruler_visible","set_mode","rehearsal_start","rehearsal_stop","rehearsal_reset","picture_crop_mode"],"description":"set_slide 跳转到页码；set_zoom 设置百分比；fit 适应窗口；set_notes_visible 控制备注面板；set_ruler_visible 控制水平和垂直标尺；set_mode 切换普通、浏览、放映或演讲者模式；rehearsal_start 启动排练放映和计时；rehearsal_stop 暂停计时；rehearsal_reset 清零并停止；picture_crop_mode 管理图片裁剪预览。每种操作只接受对应参数。"},"page_number":{"type":"integer","minimum":1,"maximum":10000,"description":"目标幻灯片页码，从 1 开始；set_mode 可选地同时指定起始页。"},"zoom_percent":{"type":"integer","minimum":25,"maximum":400,"description":"窗口缩放百分比，仅 set_zoom 使用。"},"enabled":{"type":"boolean","description":"对应面板或标尺是否显示；set_notes_visible 与 set_ruler_visible 使用。"},"mode":{"type":"string","enum":["normal","sorter","slideshow","presenter"],"description":"目标窗口模式；presenter 显示当前页、备注、下一页摘要和计时。"},"crop_action":{"type":"string","enum":["begin","reset","cancel"],"description":"begin 开始目标图片的裁剪预览；reset 还原预览裁剪量；cancel 退出且不保存。三者都要求 slide_id 和 element_id。"},"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID；裁剪预览须为当前窗口可编辑的图片所在幻灯片。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定图片元素 ID。"}},"required":["operation"],"additionalProperties":false}`),
				OutputSchema: presentationViewStateSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				if err := ValidatePresentationViewArguments(raw); err != nil {
					return CapabilityResult{}, err
				}
				return invokePresentationViewIntent(ctx, capability, "presentation.view.set", raw)
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

func invokePresentationViewIntent(ctx context.Context, capability CapabilityContext, action string, arguments json.RawMessage) (CapabilityResult, error) {
	result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
		WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
		Revision: capability.Document.Revision, Action: action, Arguments: arguments,
	})
	if err != nil {
		return CapabilityResult{}, err
	}
	return CapabilityResult{Data: result}, nil
}

func validateEmptyPresentationViewArguments(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) != 0 {
		return ErrInvalidCommandArguments
	}
	return nil
}

func ValidatePresentationViewArguments(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return ErrInvalidCommandArguments
	}
	for key := range fields {
		switch key {
		case "operation", "page_number", "zoom_percent", "enabled", "mode", "crop_action", "slide_id", "element_id":
		default:
			return ErrInvalidCommandArguments
		}
	}
	var input struct {
		Operation   string  `json:"operation"`
		PageNumber  *int    `json:"page_number"`
		ZoomPercent *int    `json:"zoom_percent"`
		Enabled     *bool   `json:"enabled"`
		Mode        string  `json:"mode"`
		CropAction  string  `json:"crop_action"`
		SlideID     *uint64 `json:"slide_id"`
		ElementID   *int64  `json:"element_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return fmt.Errorf("decode presentation.view.set: %w", ErrInvalidCommandArguments)
	}
	noOtherArgs := func() bool {
		return input.PageNumber == nil && input.ZoomPercent == nil && input.Enabled == nil && input.Mode == ""
	}
	switch input.Operation {
	case "set_slide":
		if input.PageNumber == nil || *input.PageNumber < 1 || *input.PageNumber > 10000 || input.ZoomPercent != nil || input.Enabled != nil || input.Mode != "" || input.CropAction != "" || input.SlideID != nil || input.ElementID != nil {
			return ErrInvalidCommandArguments
		}
	case "set_zoom":
		if input.ZoomPercent == nil || *input.ZoomPercent < 25 || *input.ZoomPercent > 400 || input.PageNumber != nil || input.Enabled != nil || input.Mode != "" || input.CropAction != "" || input.SlideID != nil || input.ElementID != nil {
			return ErrInvalidCommandArguments
		}
	case "fit":
		if !noOtherArgs() || input.CropAction != "" || input.SlideID != nil || input.ElementID != nil {
			return ErrInvalidCommandArguments
		}
	case "set_notes_visible":
		if input.Enabled == nil || input.PageNumber != nil || input.ZoomPercent != nil || input.Mode != "" || input.CropAction != "" || input.SlideID != nil || input.ElementID != nil {
			return ErrInvalidCommandArguments
		}
	case "set_ruler_visible":
		if input.Enabled == nil || input.PageNumber != nil || input.ZoomPercent != nil || input.Mode != "" || input.CropAction != "" || input.SlideID != nil || input.ElementID != nil {
			return ErrInvalidCommandArguments
		}
	case "set_mode":
		if input.Mode != "normal" && input.Mode != "sorter" && input.Mode != "slideshow" && input.Mode != "presenter" || input.ZoomPercent != nil || input.Enabled != nil || input.PageNumber != nil && (*input.PageNumber < 1 || *input.PageNumber > 10000) || input.CropAction != "" || input.SlideID != nil || input.ElementID != nil {
			return ErrInvalidCommandArguments
		}
	case "rehearsal_start", "rehearsal_stop", "rehearsal_reset":
		if !noOtherArgs() || input.CropAction != "" || input.SlideID != nil || input.ElementID != nil {
			return ErrInvalidCommandArguments
		}
	case "picture_crop_mode":
		if input.CropAction != "begin" && input.CropAction != "cancel" && input.CropAction != "reset" || input.SlideID == nil || *input.SlideID == 0 || *input.SlideID > uint64(^uint32(0)) || input.ElementID == nil || *input.ElementID <= 0 || uint64(*input.ElementID) > uint64(^uint32(0)) || input.PageNumber != nil || input.ZoomPercent != nil || input.Enabled != nil || input.Mode != "" {
			return ErrInvalidCommandArguments
		}
	default:
		return ErrInvalidCommandArguments
	}
	return nil
}

func presentationViewStateSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"slide_number":{"type":"integer","minimum":1},"slide_count":{"type":"integer","minimum":1},"zoom_percent":{"type":"integer","minimum":25,"maximum":400},"fit_mode":{"type":"boolean"},"notes_visible":{"type":"boolean"},"ruler_visible":{"type":"boolean"},"mode":{"type":"string","enum":["normal","sorter","slideshow","presenter"]},"rehearsal_running":{"type":"boolean"},"rehearsal_elapsed_ms":{"type":"integer","minimum":0},"picture_crop_active":{"type":"boolean"},"picture_crop_slide_id":{"type":"integer","minimum":0},"picture_crop_element_id":{"type":"integer","minimum":0},"picture_crop_preview":{"type":"object","properties":{"left":{"type":"integer","minimum":0,"maximum":100000},"top":{"type":"integer","minimum":0,"maximum":100000},"right":{"type":"integer","minimum":0,"maximum":100000},"bottom":{"type":"integer","minimum":0,"maximum":100000}},"required":["left","top","right","bottom"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["window_id","slide_number","slide_count","zoom_percent","fit_mode","notes_visible","ruler_visible","mode","rehearsal_running","rehearsal_elapsed_ms","picture_crop_active","picture_crop_slide_id","picture_crop_element_id","picture_crop_preview","changed"],"additionalProperties":false}`)
}
