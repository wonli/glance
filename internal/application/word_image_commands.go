package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/document"
)

const maxWordImageFileBytes = 128 << 20

type WordImageAccess interface {
	InsertWordImage(context.Context, string, string, string, []byte) (string, error)
	MoveWordImage(context.Context, string, string, string) (bool, error)
	ReplaceWordImage(context.Context, string, string, []byte) (bool, error)
	DeleteWordImage(context.Context, string) (bool, error)
	ResizeWordImage(context.Context, string, int64) (bool, error)
	RotateWordImage(context.Context, string, float64) (float64, bool, error)
}

func registerWordImageCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.image.move", Name: "调整 Word 图片行内位置",
				Description: "将明确指定的可编辑行内图片移动到目标 paragraph_id 中，并放在可选 after_inline_id 所指的文本运行或图片之后；省略锚点时移到段落开头。浮动图片和不支持安全编辑的对象会明确拒绝。先用 document.inspect 获取稳定图片/段落/行内 ID 和当前修订。Ribbon“布局/位置”把所选图片移动到当前光标段落及行内位置。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string","minLength":1,"maxLength":256,"description":"来自 document.inspect 的稳定可编辑行内图片 ID。"},"paragraph_id":{"type":"string","minLength":1,"maxLength":512,"description":"来自 document.inspect 的目标段落 ID。"},"after_inline_id":{"type":"string","maxLength":512,"description":"可选的目标段落文本运行或图片 ID；省略或传空字符串时将图片放在段落开头。"}},"required":["image_id","paragraph_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"image_id":{"type":"string"},"paragraph_id":{"type":"string"},"after_inline_id":{"type":"string"}},"required":["changed","image_id","paragraph_id","after_inline_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ImageID       string `json:"image_id"`
					ParagraphID   string `json:"paragraph_id"`
					AfterInlineID string `json:"after_inline_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.ImageID) == "" || strings.TrimSpace(input.ParagraphID) == "" || len(input.ImageID) > 256 || len(input.ParagraphID) > 512 || len(input.AfterInlineID) > 512 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(WordImageAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := access.MoveWordImage(ctx, input.ImageID, input.ParagraphID, input.AfterInlineID)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed       bool   `json:"changed"`
					ImageID       string `json:"image_id"`
					ParagraphID   string `json:"paragraph_id"`
					AfterInlineID string `json:"after_inline_id"`
				}{changed, input.ImageID, input.ParagraphID, input.AfterInlineID}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.image.insert", Name: "插入 Word 图片",
				Description: "从 Glance 进程可访问的本地 PNG/JPEG 路径，在明确段落和可选行内位置插入图片。文件上限 128 MiB；先用 document.inspect 获取 paragraph_id 和当前修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"paragraph_id":{"type":"string"},"after_inline_id":{"type":"string","maxLength":256},"path":{"type":"string","minLength":1,"maxLength":4096}},"required":["paragraph_id","path"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"image_id":{"type":"string"},"paragraph_id":{"type":"string"},"media_type":{"type":"string","enum":["image/png","image/jpeg"]}},"required":["changed","image_id","paragraph_id","media_type"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ParagraphID   string `json:"paragraph_id"`
					AfterInlineID string `json:"after_inline_id"`
					Path          string `json:"path"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ParagraphID) == "" || strings.TrimSpace(input.Path) == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				mediaType, data, err := loadWordImageFile(input.Path)
				if err != nil {
					return CapabilityResult{}, err
				}
				access, ok := capability.Adapter.(WordImageAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				imageID, err := access.InsertWordImage(ctx, input.ParagraphID, input.AfterInlineID, mediaType, data)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool   `json:"changed"`
					ImageID     string `json:"image_id"`
					ParagraphID string `json:"paragraph_id"`
					MediaType   string `json:"media_type"`
				}{true, imageID, input.ParagraphID, mediaType}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.image.replace", Name: "替换 Word 图片",
				Description: "用 Glance 进程可访问的本地 PNG/JPEG 文件替换明确的可编辑行内图片。文件上限 128 MiB；Agent 调用需要人工确认，先从 document.inspect 获取 image_id 和当前修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"},"path":{"type":"string","minLength":1,"maxLength":4096}},"required":["image_id","path"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"image_id":{"type":"string"},"media_type":{"type":"string","enum":["image/png","image/jpeg"]}},"required":["changed","image_id","media_type"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ImageID string `json:"image_id"`
					Path    string `json:"path"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ImageID) == "" || strings.TrimSpace(input.Path) == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				mediaType, data, err := loadWordImageFile(input.Path)
				if err != nil {
					return CapabilityResult{}, err
				}
				access, ok := capability.Adapter.(WordImageAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := access.ReplaceWordImage(ctx, input.ImageID, mediaType, data)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					ImageID   string `json:"image_id"`
					MediaType string `json:"media_type"`
				}{changed, input.ImageID, mediaType}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.image.delete", Name: "删除 Word 图片",
				Description: "删除明确的可编辑行内图片。Agent 调用需要人工确认；先通过 document.inspect 确认 image_id 和当前修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"}},"required":["image_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"image_id":{"type":"string"}},"required":["changed","image_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ImageID string `json:"image_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ImageID) == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(WordImageAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := access.DeleteWordImage(ctx, input.ImageID)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					ImageID string `json:"image_id"`
				}{changed, input.ImageID}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.image.resize", Name: "调整 Word 图片大小",
				Description: "将明确的可编辑行内图片宽度设为 0.01 至 7200 points，并按原比例计算高度。先从 document.inspect 获取 image_id 和当前修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"},"width_points":{"type":"number","minimum":0.01,"maximum":7200}},"required":["image_id","width_points"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"image_id":{"type":"string"},"width_points":{"type":"number"}},"required":["changed","image_id","width_points"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ImageID     string  `json:"image_id"`
					WidthPoints float64 `json:"width_points"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ImageID) == "" || math.IsNaN(input.WidthPoints) || math.IsInf(input.WidthPoints, 0) || input.WidthPoints < 0.01 || input.WidthPoints > 7200 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(WordImageAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				widthEMU := int64(math.Round(input.WidthPoints * 12700))
				changed, err := access.ResizeWordImage(ctx, input.ImageID, widthEMU)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool    `json:"changed"`
					ImageID     string  `json:"image_id"`
					WidthPoints float64 `json:"width_points"`
				}{changed, input.ImageID, input.WidthPoints}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.image.rotate", Name: "旋转 Word 图片",
				Description: "将明确的可编辑行内图片按指定角度增量旋转。Ribbon 的左转和右转对应 -90 和 90 度；Agent 可提供任意有限角度增量，范围为 -3600 至 3600 度。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"},"delta_degrees":{"type":"number","minimum":-3600,"maximum":3600}},"required":["image_id","delta_degrees"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"image_id":{"type":"string"},"rotation_degrees":{"type":"number"}},"required":["changed","image_id","rotation_degrees"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					ImageID      string  `json:"image_id"`
					DeltaDegrees float64 `json:"delta_degrees"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if strings.TrimSpace(input.ImageID) == "" || math.IsNaN(input.DeltaDegrees) || math.IsInf(input.DeltaDegrees, 0) || math.Abs(input.DeltaDegrees) > 3600 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				access, ok := capability.Adapter.(WordImageAccess)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				rotation, changed, err := access.RotateWordImage(ctx, input.ImageID, input.DeltaDegrees)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed         bool    `json:"changed"`
					ImageID         string  `json:"image_id"`
					RotationDegrees float64 `json:"rotation_degrees"`
				}{changed, input.ImageID, rotation}, changed)
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

func loadWordImageFile(path string) (string, []byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil, ErrInvalidCommandArguments
	}
	mediaType, expectedFormat := "", ""
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mediaType, expectedFormat = "image/png", "png"
	case ".jpg", ".jpeg":
		mediaType, expectedFormat = "image/jpeg", "jpeg"
	default:
		return "", nil, fmt.Errorf("Word images must use PNG or JPEG format")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", nil, fmt.Errorf("open Word image: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("inspect Word image: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxWordImageFileBytes {
		return "", nil, fmt.Errorf("Word image must be a regular file between 1 byte and %d MiB", maxWordImageFileBytes>>20)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxWordImageFileBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("read Word image: %w", err)
	}
	if len(data) == 0 || int64(len(data)) > maxWordImageFileBytes {
		return "", nil, fmt.Errorf("Word image exceeds the %d MiB file limit", maxWordImageFileBytes>>20)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", nil, fmt.Errorf("decode Word image header: %w", err)
	}
	if format != expectedFormat {
		return "", nil, fmt.Errorf("Word image extension does not match its %s content", format)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 32_000_000/config.Height {
		return "", nil, errors.New("Word image exceeds the 32000000-pixel decode limit")
	}
	return mediaType, data, nil
}

func findWordImage(blocks []document.WordBlock, imageID string) *document.WordImage {
	for _, block := range blocks {
		var paragraphs []document.WordParagraph
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, *block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
		}
		for _, paragraph := range paragraphs {
			for _, inline := range paragraph.Inlines {
				if inline.Image != nil && inline.Image.ID == imageID {
					return inline.Image
				}
			}
		}
	}
	return nil
}

func wordImageMoveIsNoop(blocks []document.WordBlock, imageID, paragraphID, afterInlineID string) bool {
	for _, block := range blocks {
		paragraphs := make([]document.WordParagraph, 0, 1)
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, *block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
		}
		for _, paragraph := range paragraphs {
			imageIndex, anchorIndex := -1, -1
			for index, inline := range paragraph.Inlines {
				id := wordInlineStableID(inline)
				if id == imageID {
					imageIndex = index
				}
				if paragraph.ID == paragraphID && id == afterInlineID && (inline.Text != nil || inline.Image != nil) {
					anchorIndex = index
				}
			}
			if imageIndex < 0 {
				continue
			}
			if paragraph.ID != paragraphID {
				return false
			}
			if afterInlineID == "" {
				return imageIndex == 0
			}
			return anchorIndex >= 0 && imageIndex == anchorIndex+1
		}
	}
	return false
}

func wordInlineStableID(inline document.WordInline) string {
	switch {
	case inline.Text != nil:
		return inline.Text.ID
	case inline.Image != nil:
		return inline.Image.ID
	case inline.Unsupported != nil:
		return inline.Unsupported.ID
	case inline.PageBreak != nil:
		return inline.PageBreak.ID
	case inline.Tab != nil:
		return inline.Tab.ID
	case inline.FieldCode != nil:
		return inline.FieldCode.ID
	default:
		return ""
	}
}

func normalizeWordImageRotation(degrees float64) float64 {
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}
	if math.Abs(degrees) < 1e-9 || math.Abs(degrees-360) < 1e-9 {
		return 0
	}
	return degrees
}
