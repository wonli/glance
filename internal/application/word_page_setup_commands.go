package application

import (
	"context"
	"encoding/json"
)

func registerWordPageSetupCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		wordPageSetupRegistration("word.page_setup.orientation.set", "设置 Word 页面方向", "将文档各节的页面方向设为 portrait（纵向）或 landscape（横向）。修改前先检查当前页面设置并使用当前修订号。", `{"type":"object","properties":{"orientation":{"type":"string","enum":["portrait","landscape"]}},"required":["orientation"],"additionalProperties":false}`, `{"type":"object","properties":{"changed":{"type":"boolean"},"orientation":{"type":"string"}},"required":["changed","orientation"],"additionalProperties":false}`, func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Orientation string `json:"orientation"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			setter, ok := capability.Adapter.(WordPageSetupSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetWordPageOrientation(ctx, input.Orientation)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed     bool   `json:"changed"`
				Orientation string `json:"orientation"`
			}{changed, input.Orientation}, changed)
		}),
		wordPageSetupRegistration("word.page_setup.margins.set", "设置 Word 页边距", "设置文档各节的上、右、下、左页边距，单位为磅；每个值必须在 0 至 1000 磅之间。", `{"type":"object","properties":{"top_points":{"type":"number","minimum":0,"maximum":1000},"right_points":{"type":"number","minimum":0,"maximum":1000},"bottom_points":{"type":"number","minimum":0,"maximum":1000},"left_points":{"type":"number","minimum":0,"maximum":1000}},"required":["top_points","right_points","bottom_points","left_points"],"additionalProperties":false}`, `{"type":"object","properties":{"changed":{"type":"boolean"},"top_points":{"type":"number"},"right_points":{"type":"number"},"bottom_points":{"type":"number"},"left_points":{"type":"number"}},"required":["changed","top_points","right_points","bottom_points","left_points"],"additionalProperties":false}`, func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Top    float64 `json:"top_points"`
				Right  float64 `json:"right_points"`
				Bottom float64 `json:"bottom_points"`
				Left   float64 `json:"left_points"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			setter, ok := capability.Adapter.(WordPageSetupSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetWordPageMargins(ctx, input.Top, input.Right, input.Bottom, input.Left)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool    `json:"changed"`
				Top     float64 `json:"top_points"`
				Right   float64 `json:"right_points"`
				Bottom  float64 `json:"bottom_points"`
				Left    float64 `json:"left_points"`
			}{changed, input.Top, input.Right, input.Bottom, input.Left}, changed)
		}),
		wordPageSetupRegistration("word.page_setup.size.set", "设置 Word 纸张大小", "设置文档各节的纸张宽度和高度，单位为磅，范围为 1 至 3000 磅。宽高方向由数值及当前页面方向决定。", `{"type":"object","properties":{"width_points":{"type":"number","minimum":1,"maximum":3000},"height_points":{"type":"number","minimum":1,"maximum":3000}},"required":["width_points","height_points"],"additionalProperties":false}`, `{"type":"object","properties":{"changed":{"type":"boolean"},"width_points":{"type":"number"},"height_points":{"type":"number"}},"required":["changed","width_points","height_points"],"additionalProperties":false}`, func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Width  float64 `json:"width_points"`
				Height float64 `json:"height_points"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			setter, ok := capability.Adapter.(WordPageSetupSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetWordPageSize(ctx, input.Width, input.Height)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool    `json:"changed"`
				Width   float64 `json:"width_points"`
				Height  float64 `json:"height_points"`
			}{changed, input.Width, input.Height}, changed)
		}),
		wordPageSetupRegistration("word.page_setup.columns.set", "设置 Word 页面分栏", "将文档各节设置为 1 至 4 栏。分栏会影响整节的文本排版，写入前请检查文档范围。", `{"type":"object","properties":{"columns":{"type":"integer","minimum":1,"maximum":4}},"required":["columns"],"additionalProperties":false}`, `{"type":"object","properties":{"changed":{"type":"boolean"},"columns":{"type":"integer"}},"required":["changed","columns"],"additionalProperties":false}`, func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Columns int `json:"columns"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			setter, ok := capability.Adapter.(WordPageSetupSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetWordPageColumns(ctx, input.Columns)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool `json:"changed"`
				Columns int  `json:"columns"`
			}{changed, input.Columns}, changed)
		}),
		wordPageSetupRegistration("word.page_setup.line_numbers.set", "设置 Word 行号", "设置文档各节的行号方式：continuous 连续编号，newPage 每页重新编号，none 清除行号。", `{"type":"object","properties":{"mode":{"type":"string","enum":["continuous","newPage","none"]}},"required":["mode"],"additionalProperties":false}`, `{"type":"object","properties":{"changed":{"type":"boolean"},"mode":{"type":"string"}},"required":["changed","mode"],"additionalProperties":false}`, func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Mode string `json:"mode"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			setter, ok := capability.Adapter.(WordPageSetupSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetWordPageLineNumbers(ctx, input.Mode)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool   `json:"changed"`
				Mode    string `json:"mode"`
			}{changed, input.Mode}, changed)
		}),
		wordPageSetupRegistration("word.page_setup.borders.set", "设置 Word 页面边框", "设置或移除文档各节的页面边框。border_type 支持 single、double、dashed、wave、box 和 none；box 是单线方框，none 移除边框。", `{"type":"object","properties":{"border_type":{"type":"string","enum":["single","double","dashed","wave","box","none"]}},"required":["border_type"],"additionalProperties":false}`, `{"type":"object","properties":{"changed":{"type":"boolean"},"border_type":{"type":"string"}},"required":["changed","border_type"],"additionalProperties":false}`, func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				BorderType string `json:"border_type"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			setter, ok := capability.Adapter.(WordPageSetupSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetWordPageBorders(ctx, input.BorderType)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed    bool   `json:"changed"`
				BorderType string `json:"border_type"`
			}{changed, input.BorderType}, changed)
		}),
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

func wordPageSetupRegistration(id, name, description, inputSchema, outputSchema string, handler CapabilityHandler) CapabilityRegistration {
	return CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: id, Name: name, Description: description, Category: CapabilityCommand,
			Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
			InputSchema: json.RawMessage(inputSchema), OutputSchema: json.RawMessage(outputSchema),
		},
		Handler: handler,
	}
}
