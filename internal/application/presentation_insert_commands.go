package application

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"glance/internal/document"
)

type PresentationTextBoxExecutor interface {
	AddPresentationTextBox(context.Context, uint32, document.PresentationRect, string) (int64, error)
}

type PresentationShapeExecutor interface {
	AddPresentationShape(context.Context, uint32, document.PresentationRect, string, string) (int64, error)
}

type PresentationPictureExecutor interface {
	AddPresentationPicture(context.Context, uint32, document.PresentationRect, string, []byte, string, string, bool) (int64, document.PresentationRect, error)
}

const maxPresentationPictureCommandBytes = 32 << 20
const maxPresentationPictureCommandJSONBytes = 44739244 + 4096

func registerPresentationInsertCapabilities(catalog *Catalog) error {
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.textbox.create", Name: "在幻灯片上新建文本框",
			Description: "在明确的 PPTX 幻灯片上按文档坐标（point）创建单段落文本框。先用 document.inspect 获取稳定 slide_id 和当前 expected_revision；位置和尺寸由调用方明确提供，宽高至少 8 point，文本最多 32767 个字符且不得换行。返回新元素的稳定 element_id，可继续用于文本、样式和几何命令。桌面 Ribbon 的文本框操作也调用同一处理器。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},"x":{"type":"number","description":"文本框左上角 X 坐标，单位 point。"},"y":{"type":"number","description":"文本框左上角 Y 坐标，单位 point。"},"width":{"type":"number","minimum":8,"description":"文本框宽度，单位 point，至少 8。"},"height":{"type":"number","minimum":8,"description":"文本框高度，单位 point，至少 8。"},"text":{"type":"string","maxLength":32767,"description":"可选初始单段落文本；不得包含换行。"}},"required":["slide_id","x","y","width","height"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"bounds":{"type":"object","description":"插入后的实际形状几何；contain 会在所给图片框内缩小并居中。","properties":{"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number"},"height":{"type":"number"}},"required":["x","y","width","height"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["slide_id","element_id","bounds","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID uint64  `json:"slide_id"`
				X       float64 `json:"x"`
				Y       float64 `json:"y"`
				Width   float64 `json:"width"`
				Height  float64 `json:"height"`
				Text    string  `json:"text"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.Width < 8 || input.Height < 8 || len([]rune(input.Text)) > 32767 || strings.ContainsAny(input.Text, "\r\n") {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationTextBoxExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			bounds := document.PresentationRect{X: input.X, Y: input.Y, W: input.Width, H: input.Height}
			elementID, err := executor.AddPresentationTextBox(ctx, uint32(input.SlideID), bounds, input.Text)
			if err != nil {
				return CapabilityResult{}, err
			}
			if elementID <= 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Bounds    struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"bounds"`
				Changed bool `json:"changed"`
			}{
				SlideID: uint32(input.SlideID), ElementID: elementID,
				Bounds: struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				}{X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H},
				Changed: true,
			}, true)
		},
	}); err != nil {
		return err
	}
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.shape.create", Name: "在幻灯片上新建自选图形",
			Description: "在明确的 PPTX 幻灯片上按文档坐标（point）创建矩形、圆角矩形或椭圆自选图形。先用 document.inspect 获取稳定 slide_id 和当前 expected_revision；宽高至少 8 point，文本最多 32767 个字符且不得换行。返回稳定 element_id，可继续调用文本、样式和几何命令。桌面 Ribbon 的形状插入操作创建矩形并复用同一处理器。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},"geometry":{"type":"string","enum":["rect","roundRect","ellipse"],"description":"自选图形几何类型。"},"x":{"type":"number","description":"图形左上角 X 坐标，单位 point。"},"y":{"type":"number","description":"图形左上角 Y 坐标，单位 point。"},"width":{"type":"number","minimum":8,"description":"图形宽度，单位 point，至少 8。"},"height":{"type":"number","minimum":8,"description":"图形高度，单位 point，至少 8。"},"text":{"type":"string","maxLength":32767,"description":"可选初始单段落文本；不得包含换行。"}},"required":["slide_id","geometry","x","y","width","height"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"geometry":{"type":"string","enum":["rect","roundRect","ellipse"]},"bounds":{"type":"object","description":"插入后的实际形状几何；contain 会在所给图片框内缩小并居中。","properties":{"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number"},"height":{"type":"number"}},"required":["x","y","width","height"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["slide_id","element_id","geometry","bounds","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID  uint64  `json:"slide_id"`
				Geometry string  `json:"geometry"`
				X        float64 `json:"x"`
				Y        float64 `json:"y"`
				Width    float64 `json:"width"`
				Height   float64 `json:"height"`
				Text     string  `json:"text"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.Width < 8 || input.Height < 8 || len([]rune(input.Text)) > 32767 || strings.ContainsAny(input.Text, "\r\n") {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			switch input.Geometry {
			case "rect", "roundRect", "ellipse":
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationShapeExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			bounds := document.PresentationRect{X: input.X, Y: input.Y, W: input.Width, H: input.Height}
			elementID, err := executor.AddPresentationShape(ctx, uint32(input.SlideID), bounds, input.Geometry, input.Text)
			if err != nil {
				return CapabilityResult{}, err
			}
			if elementID <= 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Geometry  string `json:"geometry"`
				Bounds    struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"bounds"`
				Changed bool `json:"changed"`
			}{
				SlideID: uint32(input.SlideID), ElementID: elementID, Geometry: input.Geometry,
				Bounds: struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				}{X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H},
				Changed: true,
			}, true)
		},
	}); err != nil {
		return err
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.picture.create", Name: "在幻灯片上插入图片",
			Description: "在明确的 PPTX 幻灯片上插入 PNG/JPEG 图片。image_base64 是图片原始字节的 Base64 编码，最多 32 MiB；位置和尺寸使用 point，fit 可选 stretch、contain、cover。先用 document.inspect 获取稳定 slide_id 和 expected_revision。返回 element_id，可继续使用元素样式、几何及图片裁剪能力。桌面 Ribbon 选图后点击画布，也调用此处理器。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},"x":{"type":"number","description":"图片左上角 X 坐标，单位 point。"},"y":{"type":"number","description":"图片左上角 Y 坐标，单位 point。"},"width":{"type":"number","minimum":8,"description":"图片框宽度，单位 point，至少 8。"},"height":{"type":"number","minimum":8,"description":"图片框高度，单位 point，至少 8。"},"fit":{"type":"string","enum":["stretch","contain","cover"],"description":"将图片放入图片框的方式；contain 保留完整图片，cover 裁切填满，stretch 拉伸。"},"media_type":{"type":"string","enum":["image/png","image/jpeg"],"description":"图片实际媒体类型，必须与图片数据匹配。"},"image_base64":{"type":"string","format":"byte","contentEncoding":"base64","maxLength":44739244,"description":"PNG/JPEG 原始字节的 Base64 编码，解码后最多 32 MiB。"},"alt_text":{"type":"string","maxLength":1000,"description":"可选的无障碍替代文本；装饰性图片不得同时设置此字段。"},"decorative":{"type":"boolean","description":"图片是否仅作装饰；为 true 时不提供 alt_text。"}},"required":["slide_id","x","y","width","height","fit","media_type","image_base64"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"media_type":{"type":"string","enum":["image/png","image/jpeg"]},"bounds":{"type":"object","description":"插入后的实际形状几何；contain 会在所给图片框内缩小并居中。","properties":{"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number"},"height":{"type":"number"}},"required":["x","y","width","height"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["slide_id","element_id","media_type","bounds","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if len(raw) > maxPresentationPictureCommandJSONBytes {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var input struct {
				SlideID    uint64  `json:"slide_id"`
				X          float64 `json:"x"`
				Y          float64 `json:"y"`
				Width      float64 `json:"width"`
				Height     float64 `json:"height"`
				Fit        string  `json:"fit"`
				MediaType  string  `json:"media_type"`
				ImageData  []byte  `json:"image_base64"`
				AltText    string  `json:"alt_text"`
				Decorative bool    `json:"decorative"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) ||
				math.IsNaN(input.X) || math.IsInf(input.X, 0) || math.IsNaN(input.Y) || math.IsInf(input.Y, 0) ||
				math.IsNaN(input.Width) || math.IsInf(input.Width, 0) || math.IsNaN(input.Height) || math.IsInf(input.Height, 0) ||
				input.Width < 8 || input.Height < 8 || len(input.ImageData) == 0 || len(input.ImageData) > maxPresentationPictureCommandBytes ||
				len([]rune(input.AltText)) > 1000 || (input.Decorative && input.AltText != "") {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			switch input.Fit {
			case "stretch", "contain", "cover":
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			switch input.MediaType {
			case "image/png", "image/jpeg":
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationPictureExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			bounds := document.PresentationRect{X: input.X, Y: input.Y, W: input.Width, H: input.Height}
			elementID, actualBounds, err := executor.AddPresentationPicture(ctx, uint32(input.SlideID), bounds, input.MediaType, input.ImageData, input.Fit, input.AltText, input.Decorative)
			if err != nil {
				return CapabilityResult{}, err
			}
			if elementID <= 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				MediaType string `json:"media_type"`
				Bounds    struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"bounds"`
				Changed bool `json:"changed"`
			}{
				SlideID: uint32(input.SlideID), ElementID: elementID, MediaType: input.MediaType,
				Bounds: struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				}{X: actualBounds.X, Y: actualBounds.Y, Width: actualBounds.W, Height: actualBounds.H}, Changed: true,
			}, true)
		},
	})
}
