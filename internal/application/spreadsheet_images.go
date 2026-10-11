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
	"strings"

	"glance/internal/document"
)

const (
	maxSpreadsheetImageCommandBytes = 32 << 20
	maxSpreadsheetImageCommandJSON  = 44739244 + 4096
	maxSpreadsheetImagesReturned    = 1000
)

// ErrSpreadsheetImageNotFound indicates that an image ID is not present on
// the explicitly named worksheet.
var ErrSpreadsheetImageNotFound = errors.New("spreadsheet image was not found on the requested worksheet")

type spreadsheetImageResult struct {
	ImageID         string `json:"image_id"`
	Sheet           string `json:"sheet"`
	Cell            string `json:"cell"`
	Row             int    `json:"row"`
	Column          int    `json:"column"`
	OffsetX         int    `json:"offset_x"`
	OffsetY         int    `json:"offset_y"`
	WidthPx         int    `json:"width_px"`
	HeightPx        int    `json:"height_px"`
	MediaType       string `json:"media_type"`
	Name            string `json:"name"`
	AltText         string `json:"alt_text"`
	MoveWithCells   bool   `json:"move_with_cells"`
	SizeWithCells   bool   `json:"size_with_cells"`
	LockAspectRatio bool   `json:"lock_aspect_ratio"`
	Editable        bool   `json:"editable"`
	ReadOnlyReason  string `json:"read_only_reason,omitempty"`
}

func registerSpreadsheetImageCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.images.list", Name: "列出工作表图片",
				Description: "列出明确工作表中的图片对象及其当前 image_id、锚点、尺寸和可编辑状态。最多返回 1000 张；image_id 在位置变化后可能改变，移动图片后请重新列出。此查询不依赖活动窗口或当前选区。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"要操作图片的精确工作表名称。"}},"required":["sheet"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"images":{"type":"array","items":{"type":"object","properties":{"image_id":{"type":"string"},"sheet":{"type":"string"},"cell":{"type":"string"},"row":{"type":"integer","minimum":1},"column":{"type":"integer","minimum":1},"offset_x":{"type":"integer","minimum":0},"offset_y":{"type":"integer","minimum":0},"width_px":{"type":"integer","minimum":1},"height_px":{"type":"integer","minimum":1},"media_type":{"type":"string"},"name":{"type":"string"},"alt_text":{"type":"string"},"move_with_cells":{"type":"boolean"},"size_with_cells":{"type":"boolean"},"lock_aspect_ratio":{"type":"boolean"},"editable":{"type":"boolean"},"read_only_reason":{"type":"string"}},"required":["image_id","sheet","cell","row","column","offset_x","offset_y","width_px","height_px","media_type","name","alt_text","move_with_cells","size_with_cells","lock_aspect_ratio","editable"],"additionalProperties":false}},"truncated":{"type":"boolean"}},"required":["sheet","images","truncated"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || len([]rune(input.Sheet)) > 255 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetImageController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				images, err := controller.ListSpreadsheetImages(ctx, input.Sheet)
				if err != nil {
					return CapabilityResult{}, err
				}
				truncated := len(images) > maxSpreadsheetImagesReturned
				if truncated {
					images = images[:maxSpreadsheetImagesReturned]
				}
				items := make([]spreadsheetImageResult, 0, len(images))
				for _, item := range images {
					items = append(items, spreadsheetImageResult{
						ImageID: item.ID, Sheet: item.Sheet, Cell: item.Cell,
						Row: item.Row, Column: item.Column, OffsetX: item.OffsetX, OffsetY: item.OffsetY,
						WidthPx: item.WidthPx, HeightPx: item.HeightPx, MediaType: item.MediaType,
						Name: item.Name, AltText: item.AltText, MoveWithCells: item.MoveWithCells,
						SizeWithCells: item.SizeWithCells, LockAspectRatio: item.LockAspectRatio,
						Editable: item.Editable, ReadOnlyReason: item.ReadOnlyReason,
					})
				}
				return capabilityJSONResult(struct {
					Sheet     string                   `json:"sheet"`
					Images    []spreadsheetImageResult `json:"images"`
					Truncated bool                     `json:"truncated"`
				}{Sheet: input.Sheet, Images: items, Truncated: truncated}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.image.insert", Name: "插入工作表图片",
				Description: "在明确工作表的单元格锚点插入 PNG/JPEG 图片。坐标使用从 1 开始的行列，偏移和显示尺寸使用 96 DPI 像素；图片数据 Base64 解码后最多 32 MiB。调用前先列出目标工作表图片并确认锚点未占用。返回的 image_id 用于当前锚点；Ribbon 选图插入也复用此命令。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"要操作图片的精确工作表名称。"},"row":{"type":"integer","minimum":1,"maximum":1048576,"description":"目标锚点所在行，从 1 开始。"},"column":{"type":"integer","minimum":1,"maximum":16384,"description":"目标锚点所在列，从 1 开始；A 列为 1。"},"offset_x":{"type":"integer","minimum":0,"maximum":1000000,"description":"相对锚点左边的水平偏移，单位为 96 DPI 像素。"},"offset_y":{"type":"integer","minimum":0,"maximum":1000000,"description":"相对锚点顶部的垂直偏移，单位为 96 DPI 像素。"},"width_px":{"type":"integer","minimum":1,"maximum":1000000,"description":"目标显示宽度，单位为 96 DPI 像素；高度按当前宽高比例计算。"},"height_px":{"type":"integer","minimum":1,"maximum":1000000,"description":"图片显示高度，单位为 96 DPI 像素。"},"media_type":{"type":"string","enum":["image/png","image/jpeg"],"description":"图片实际媒体类型，必须与编码数据一致。"},"name":{"type":"string","maxLength":256,"description":"可选的图片对象名称。"},"alt_text":{"type":"string","maxLength":2000,"description":"可选的无障碍替代文本。"},"image_base64":{"type":"string","format":"byte","contentEncoding":"base64","maxLength":44739244,"description":"PNG/JPEG 原始字节的 Base64 编码，解码后最多 32 MiB。"}},"required":["sheet","row","column","width_px","height_px","media_type","image_base64"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"},"sheet":{"type":"string"},"cell":{"type":"string"},"changed":{"type":"boolean"}},"required":["image_id","sheet","cell","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				if len(raw) > maxSpreadsheetImageCommandJSON {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				var input struct {
					Sheet      string `json:"sheet"`
					Row        int    `json:"row"`
					Column     int    `json:"column"`
					OffsetX    int    `json:"offset_x"`
					OffsetY    int    `json:"offset_y"`
					WidthPx    int    `json:"width_px"`
					HeightPx   int    `json:"height_px"`
					MediaType  string `json:"media_type"`
					Name       string `json:"name"`
					AltText    string `json:"alt_text"`
					ImageBytes []byte `json:"image_base64"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || input.Row < 1 || input.Row > 1_048_576 || input.Column < 1 || input.Column > 16_384 || input.OffsetX < 0 || input.OffsetY < 0 || input.OffsetX > 1_000_000 || input.OffsetY > 1_000_000 || input.WidthPx < 1 || input.HeightPx < 1 || input.WidthPx > 1_000_000 || input.HeightPx > 1_000_000 || len([]rune(input.Name)) > 256 || len([]rune(input.AltText)) > 2000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if err := validateSpreadsheetImageData(input.MediaType, input.ImageBytes); err != nil {
					return CapabilityResult{}, err
				}
				controller, ok := capability.Adapter.(SpreadsheetImageController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				placement := document.SpreadsheetImagePlacement{
					Sheet: input.Sheet, MediaType: input.MediaType, Name: input.Name, AltText: input.AltText,
					Row: input.Row, Column: input.Column, OffsetX: input.OffsetX, OffsetY: input.OffsetY,
					WidthPx: input.WidthPx, HeightPx: input.HeightPx,
				}
				imageID, err := controller.InsertSpreadsheetImage(ctx, placement, input.ImageBytes)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					ImageID string `json:"image_id"`
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Changed bool   `json:"changed"`
				}{ImageID: imageID, Sheet: input.Sheet, Cell: spreadsheetCellName(input.Row, input.Column), Changed: true}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.image.replace", Name: "替换工作表图片",
				Description: "用 PNG/JPEG 图片替换明确工作表上的图片，保留锚点和显示尺寸。image_base64 解码后最多 32 MiB；先调用 spreadsheet.images.list 获取 image_id 和当前修订。MCP 调用须经人工确认，Ribbon 操作也复用此命令。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"要操作图片的精确工作表名称。"},"image_id":{"type":"string","minLength":1,"maxLength":512,"description":"由 spreadsheet.images.list 在当前修订返回的图片 ID。移动后锚点变化时请重新列出。"},"media_type":{"type":"string","enum":["image/png","image/jpeg"],"description":"图片实际媒体类型，必须与编码数据一致。"},"image_base64":{"type":"string","format":"byte","contentEncoding":"base64","maxLength":44739244,"description":"PNG/JPEG 原始字节的 Base64 编码，解码后最多 32 MiB。"}},"required":["sheet","image_id","media_type","image_base64"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"},"sheet":{"type":"string"},"changed":{"type":"boolean"}},"required":["image_id","sheet","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				if len(raw) > maxSpreadsheetImageCommandJSON {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				var input struct {
					Sheet      string `json:"sheet"`
					ImageID    string `json:"image_id"`
					MediaType  string `json:"media_type"`
					ImageBytes []byte `json:"image_base64"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Sheet, input.ImageID = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.ImageID)
				if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || input.ImageID == "" || len(input.ImageID) > 512 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if err := validateSpreadsheetImageData(input.MediaType, input.ImageBytes); err != nil {
					return CapabilityResult{}, err
				}
				controller, ok := capability.Adapter.(SpreadsheetImageController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.ReplaceSpreadsheetImage(ctx, input.Sheet, input.ImageID, input.MediaType, input.ImageBytes)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					ImageID string `json:"image_id"`
					Sheet   string `json:"sheet"`
					Changed bool   `json:"changed"`
				}{input.ImageID, input.Sheet, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.image.move", Name: "移动工作表图片",
				Description: "将明确工作表上的图片移动到新的单元格锚点和像素偏移，保留图片尺寸。坐标使用从 1 开始的行列；目标锚点必须可用。先调用 spreadsheet.images.list 获取本次修订的 image_id；移动后锚点生成的 image_id 可能改变，请重新列出。Ribbon 拖动图片也复用此命令。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"要操作图片的精确工作表名称。"},"image_id":{"type":"string","minLength":1,"maxLength":512,"description":"由 spreadsheet.images.list 在当前修订返回的图片 ID。移动后锚点变化时请重新列出。"},"row":{"type":"integer","minimum":1,"maximum":1048576,"description":"目标锚点所在行，从 1 开始。"},"column":{"type":"integer","minimum":1,"maximum":16384,"description":"目标锚点所在列，从 1 开始；A 列为 1。"},"offset_x":{"type":"integer","minimum":0,"maximum":1000000,"description":"相对锚点左边的水平偏移，单位为 96 DPI 像素。"},"offset_y":{"type":"integer","minimum":0,"maximum":1000000,"description":"相对锚点顶部的垂直偏移，单位为 96 DPI 像素。"}},"required":["sheet","image_id","row","column","offset_x","offset_y"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"previous_image_id":{"type":"string"},"sheet":{"type":"string"},"row":{"type":"integer","minimum":1},"column":{"type":"integer","minimum":1},"offset_x":{"type":"integer","minimum":0},"offset_y":{"type":"integer","minimum":0},"changed":{"type":"boolean"}},"required":["previous_image_id","sheet","row","column","offset_x","offset_y","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet   string `json:"sheet"`
					ImageID string `json:"image_id"`
					Row     int    `json:"row"`
					Column  int    `json:"column"`
					OffsetX int    `json:"offset_x"`
					OffsetY int    `json:"offset_y"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Sheet, input.ImageID = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.ImageID)
				if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || input.ImageID == "" || len(input.ImageID) > 512 || input.Row < 1 || input.Row > 1_048_576 || input.Column < 1 || input.Column > 16_384 || input.OffsetX < 0 || input.OffsetY < 0 || input.OffsetX > 1_000_000 || input.OffsetY > 1_000_000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetImageController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.MoveSpreadsheetImage(ctx, input.Sheet, input.ImageID, input.Row, input.Column, input.OffsetX, input.OffsetY)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					ImageID string `json:"previous_image_id"`
					Sheet   string `json:"sheet"`
					Row     int    `json:"row"`
					Column  int    `json:"column"`
					OffsetX int    `json:"offset_x"`
					OffsetY int    `json:"offset_y"`
					Changed bool   `json:"changed"`
				}{input.ImageID, input.Sheet, input.Row, input.Column, input.OffsetX, input.OffsetY, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.image.resize", Name: "调整工作表图片宽度",
				Description: "将明确工作表上的图片宽度设为 1 至 1000000 像素，并保持当前宽高比例。先调用 spreadsheet.images.list 获取 image_id 和当前修订。Ribbon 设置宽度也复用此命令。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"要操作图片的精确工作表名称。"},"image_id":{"type":"string","minLength":1,"maxLength":512,"description":"由 spreadsheet.images.list 在当前修订返回的图片 ID。移动后锚点变化时请重新列出。"},"width_px":{"type":"integer","minimum":1,"maximum":1000000,"description":"目标显示宽度，单位为 96 DPI 像素；高度按当前宽高比例计算。"}},"required":["sheet","image_id","width_px"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"},"sheet":{"type":"string"},"width_px":{"type":"integer","minimum":1,"maximum":1000000},"changed":{"type":"boolean"}},"required":["image_id","sheet","width_px","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet   string `json:"sheet"`
					ImageID string `json:"image_id"`
					WidthPx int    `json:"width_px"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Sheet, input.ImageID = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.ImageID)
				if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || input.ImageID == "" || len(input.ImageID) > 512 || input.WidthPx < 1 || input.WidthPx > 1_000_000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetImageController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.ResizeSpreadsheetImage(ctx, input.Sheet, input.ImageID, input.WidthPx)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					ImageID string `json:"image_id"`
					Sheet   string `json:"sheet"`
					WidthPx int    `json:"width_px"`
					Changed bool   `json:"changed"`
				}{input.ImageID, input.Sheet, input.WidthPx, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.image.delete", Name: "删除工作表图片",
				Description: "删除明确工作表上的图片对象。先调用 spreadsheet.images.list 确认 image_id 和当前修订。MCP 调用须经人工确认；Ribbon 删除按钮也复用此命令。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"要操作图片的精确工作表名称。"},"image_id":{"type":"string","minLength":1,"maxLength":512,"description":"由 spreadsheet.images.list 在当前修订返回的图片 ID。移动后锚点变化时请重新列出。"}},"required":["sheet","image_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"image_id":{"type":"string"},"sheet":{"type":"string"},"changed":{"type":"boolean"}},"required":["image_id","sheet","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet   string `json:"sheet"`
					ImageID string `json:"image_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				input.Sheet, input.ImageID = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.ImageID)
				if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || input.ImageID == "" || len(input.ImageID) > 512 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetImageController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.DeleteSpreadsheetImage(ctx, input.Sheet, input.ImageID)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					ImageID string `json:"image_id"`
					Sheet   string `json:"sheet"`
					Changed bool   `json:"changed"`
				}{input.ImageID, input.Sheet, changed}, changed)
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

func validateSpreadsheetImageData(mediaType string, data []byte) error {
	if len(data) == 0 || len(data) > maxSpreadsheetImageCommandBytes {
		return ErrInvalidCommandArguments
	}
	if mediaType != "image/png" && mediaType != "image/jpeg" {
		return ErrInvalidCommandArguments
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode spreadsheet image: %w", err)
	}
	want := map[string]string{"image/png": "png", "image/jpeg": "jpeg"}[mediaType]
	if format != want {
		return ErrInvalidCommandArguments
	}
	return nil
}

func spreadsheetCellName(row, column int) string {
	var name []byte
	for column > 0 {
		column--
		name = append([]byte{byte('A' + column%26)}, name...)
		column /= 26
	}
	return fmt.Sprintf("%s%d", name, row)
}

func findSpreadsheetImage(ctx context.Context, reader document.SpreadsheetImageReader, sheet, imageID string) (document.SpreadsheetImage, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetImage{}, err
	}
	if reader == nil || strings.TrimSpace(sheet) == "" || strings.TrimSpace(imageID) == "" {
		return document.SpreadsheetImage{}, ErrInvalidCommandArguments
	}
	images, err := reader.SpreadsheetImages(sheet)
	if err != nil {
		return document.SpreadsheetImage{}, err
	}
	for _, item := range images {
		if item.ID == imageID {
			if !item.Editable {
				return document.SpreadsheetImage{}, fmt.Errorf("%w: %s", ErrCapabilityUnsupported, item.ReadOnlyReason)
			}
			return item, nil
		}
	}
	return document.SpreadsheetImage{}, ErrSpreadsheetImageNotFound
}

func ListSpreadsheetImageMetadata(ctx context.Context, reader document.SpreadsheetImageReader, sheet string) ([]document.SpreadsheetImage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reader == nil || strings.TrimSpace(sheet) == "" {
		return nil, ErrInvalidCommandArguments
	}
	images, err := reader.SpreadsheetImages(sheet)
	if err != nil {
		return nil, err
	}
	return append([]document.SpreadsheetImage(nil), images...), nil
}

func InsertSpreadsheetImage(ctx context.Context, editor document.SpreadsheetImageEditor, placement document.SpreadsheetImagePlacement, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if editor == nil {
		return "", ErrCapabilityUnsupported
	}
	if strings.TrimSpace(placement.Sheet) == "" {
		return "", ErrInvalidCommandArguments
	}
	imageID, err := editor.InsertSpreadsheetImage(placement, append([]byte(nil), data...))
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func ReplaceSpreadsheetImage(ctx context.Context, reader document.SpreadsheetImageReader, editor document.SpreadsheetImageEditor, sheet, imageID, mediaType string, data []byte) (bool, error) {
	item, err := findSpreadsheetImage(ctx, reader, sheet, imageID)
	if err != nil {
		return false, err
	}
	if editor == nil {
		return false, ErrCapabilityUnsupported
	}
	current, err := reader.SpreadsheetImageBytes(imageID)
	if err != nil {
		return false, err
	}
	if item.MediaType == mediaType && bytes.Equal(current, data) {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.ReplaceSpreadsheetImage(imageID, mediaType, append([]byte(nil), data...)); err != nil {
		return false, err
	}
	return true, nil
}

func MoveSpreadsheetImage(ctx context.Context, reader document.SpreadsheetImageReader, editor document.SpreadsheetImageEditor, sheet, imageID string, row, column, offsetX, offsetY int) (bool, error) {
	item, err := findSpreadsheetImage(ctx, reader, sheet, imageID)
	if err != nil {
		return false, err
	}
	if editor == nil {
		return false, ErrCapabilityUnsupported
	}
	if item.Row == row && item.Column == column && item.OffsetX == offsetX && item.OffsetY == offsetY {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.MoveSpreadsheetImage(imageID, row, column, offsetX, offsetY); err != nil {
		return false, err
	}
	return true, nil
}

func ResizeSpreadsheetImage(ctx context.Context, reader document.SpreadsheetImageReader, editor document.SpreadsheetImageEditor, sheet, imageID string, widthPx int) (bool, error) {
	item, err := findSpreadsheetImage(ctx, reader, sheet, imageID)
	if err != nil {
		return false, err
	}
	if editor == nil {
		return false, ErrCapabilityUnsupported
	}
	if item.WidthPx == widthPx {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.ResizeSpreadsheetImage(imageID, widthPx); err != nil {
		return false, err
	}
	return true, nil
}

func DeleteSpreadsheetImage(ctx context.Context, reader document.SpreadsheetImageReader, editor document.SpreadsheetImageEditor, sheet, imageID string) (bool, error) {
	if _, err := findSpreadsheetImage(ctx, reader, sheet, imageID); err != nil {
		return false, err
	}
	if editor == nil {
		return false, ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.DeleteSpreadsheetImage(imageID); err != nil {
		return false, err
	}
	return true, nil
}
