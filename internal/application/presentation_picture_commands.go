package application

import (
	"context"
	"encoding/json"

	"glance/internal/document"
)

type PresentationPictureReplaceExecutor interface {
	ReplacePresentationPicture(context.Context, uint32, int64, string, []byte) (bool, error)
}

type PresentationPictureCropExecutor interface {
	SetPresentationPictureCrop(context.Context, uint32, int64, document.PresentationCrop) (bool, error)
}

func registerPresentationPictureCapabilities(catalog *Catalog) error {
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.picture.replace", Name: "替换幻灯片图片",
			Description: "用 PNG/JPEG 数据替换明确的 PPTX 图片元素。先用 document.inspect 获取稳定 slide_id、element_id 和 expected_revision；图片通过 image_base64 传入，解码后最多 32 MiB。替换保留元素 ID 与几何框，并清除原裁剪。Ribbon 在明确选中图片后选择新图片，也复用此命令。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的可编辑图片元素 ID。"},"media_type":{"type":"string","enum":["image/png","image/jpeg"],"description":"新图片媒体类型，必须与实际数据匹配。"},"image_base64":{"type":"string","format":"byte","contentEncoding":"base64","maxLength":44739244,"description":"图片原始字节的 Base64 编码，解码后最多 32 MiB。"}},"required":["slide_id","element_id","media_type","image_base64"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"media_type":{"type":"string","enum":["image/png","image/jpeg"]},"changed":{"type":"boolean"}},"required":["slide_id","element_id","media_type","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if len(raw) > maxPresentationPictureCommandJSONBytes {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var input struct {
				SlideID   uint64 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				MediaType string `json:"media_type"`
				ImageData []byte `json:"image_base64"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) ||
				input.ElementID <= 0 || uint64(input.ElementID) > uint64(^uint32(0)) || len(input.ImageData) == 0 || len(input.ImageData) > maxPresentationPictureCommandBytes {
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
			executor, ok := capability.Adapter.(PresentationPictureReplaceExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := executor.ReplacePresentationPicture(ctx, uint32(input.SlideID), input.ElementID, input.MediaType, input.ImageData)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				MediaType string `json:"media_type"`
				Changed   bool   `json:"changed"`
			}{SlideID: uint32(input.SlideID), ElementID: input.ElementID, MediaType: input.MediaType, Changed: changed}, changed)
		},
	}); err != nil {
		return err
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.picture.crop.set", Name: "设置幻灯片图片裁剪",
			Description: "按稳定 slide_id 和 element_id 设置图片可见区域。left、top、right、bottom 使用万分比整数（0..100000）；每轴裁剪总和必须小于 100000，确保仍有可见内容。桌面 Ribbon 调整裁剪后点击“完成”时调用同一处理器。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的可编辑图片元素 ID。"},"left":{"type":"integer","minimum":0,"maximum":100000,"description":"左侧裁剪，万分比。"},"top":{"type":"integer","minimum":0,"maximum":100000,"description":"顶部裁剪，万分比。"},"right":{"type":"integer","minimum":0,"maximum":100000,"description":"右侧裁剪，万分比。"},"bottom":{"type":"integer","minimum":0,"maximum":100000,"description":"底部裁剪，万分比。"}},"required":["slide_id","element_id","left","top","right","bottom"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"crop":{"type":"object","properties":{"left":{"type":"integer","minimum":0,"maximum":100000},"top":{"type":"integer","minimum":0,"maximum":100000},"right":{"type":"integer","minimum":0,"maximum":100000},"bottom":{"type":"integer","minimum":0,"maximum":100000}},"required":["left","top","right","bottom"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["slide_id","element_id","crop","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				SlideID   uint64 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Left      int    `json:"left"`
				Top       int    `json:"top"`
				Right     int    `json:"right"`
				Bottom    int    `json:"bottom"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID <= 0 || uint64(input.ElementID) > uint64(^uint32(0)) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			crop := document.PresentationCrop{Left: input.Left, Top: input.Top, Right: input.Right, Bottom: input.Bottom}
			if crop.Left < 0 || crop.Left > 100000 || crop.Top < 0 || crop.Top > 100000 || crop.Right < 0 || crop.Right > 100000 || crop.Bottom < 0 || crop.Bottom > 100000 || crop.Left+crop.Right >= 100000 || crop.Top+crop.Bottom >= 100000 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationPictureCropExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := executor.SetPresentationPictureCrop(ctx, uint32(input.SlideID), input.ElementID, crop)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Crop      struct {
					Left   int `json:"left"`
					Top    int `json:"top"`
					Right  int `json:"right"`
					Bottom int `json:"bottom"`
				} `json:"crop"`
				Changed bool `json:"changed"`
			}{SlideID: uint32(input.SlideID), ElementID: input.ElementID, Crop: struct {
				Left   int `json:"left"`
				Top    int `json:"top"`
				Right  int `json:"right"`
				Bottom int `json:"bottom"`
			}{Left: crop.Left, Top: crop.Top, Right: crop.Right, Bottom: crop.Bottom}, Changed: changed}, changed)
		},
	})
}

var _ PresentationPictureReplaceExecutor = (*adapterFuncs)(nil)
var _ PresentationPictureCropExecutor = (*adapterFuncs)(nil)
