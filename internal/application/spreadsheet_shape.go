package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/document"
)

func registerSpreadsheetShapeCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.shape.create", Name: "插入工作表形状或文本框",
			Description: "在明确 XLSX 工作表的单元格锚点插入几何形状或文本框。支持 rect、roundRect、ellipse、rightArrow、star5；宽高以 96 DPI 像素提供，可设置内部文本。Ribbon 的形状和文本框插入按钮也复用此命令。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"形状所在工作表的精确名称。"},"cell":{"type":"string","maxLength":16,"description":"形状左上角锚点单元格，例如 B2。"},"type":{"type":"string","enum":["rect","roundRect","ellipse","rightArrow","star5"],"description":"形状类型；文本框使用 rect 并提供 text。"},"width_px":{"type":"integer","minimum":1,"maximum":1000000,"description":"形状宽度，单位为 96 DPI 像素。"},"height_px":{"type":"integer","minimum":1,"maximum":1000000,"description":"形状高度，单位为 96 DPI 像素。"},"text":{"type":"string","maxLength":32767,"description":"可选的形状内部文本。"}},"required":["sheet","cell","type"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"type":{"type":"string","enum":["rect","roundRect","ellipse","rightArrow","star5"]},"width_px":{"type":"integer","minimum":1},"height_px":{"type":"integer","minimum":1},"text":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","cell","type","width_px","height_px","text","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet  string `json:"sheet"`
				Cell   string `json:"cell"`
				Type   string `json:"type"`
				Width  uint   `json:"width_px"`
				Height uint   `json:"height_px"`
				Text   string `json:"text"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.shape.create: %w", err)
			}
			input.Sheet, input.Cell = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.Cell)
			if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || !validSpreadsheetCellReference(input.Cell) || len([]rune(input.Text)) > 32767 || input.Width > 1_000_000 || input.Height > 1_000_000 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			switch input.Type {
			case "rect", "roundRect", "ellipse", "rightArrow", "star5":
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if input.Width == 0 {
				input.Width = 160
			}
			if input.Height == 0 {
				input.Height = 90
			}
			controller, ok := capability.Adapter.(SpreadsheetShapeController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			spec := document.SpreadsheetShapeSpec{Sheet: input.Sheet, Cell: input.Cell, Type: input.Type, Width: input.Width, Height: input.Height, Text: input.Text}
			if err := controller.CreateSpreadsheetShape(ctx, spec); err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Sheet   string `json:"sheet"`
				Cell    string `json:"cell"`
				Type    string `json:"type"`
				Width   uint   `json:"width_px"`
				Height  uint   `json:"height_px"`
				Text    string `json:"text"`
				Changed bool   `json:"changed"`
			}{input.Sheet, input.Cell, input.Type, input.Width, input.Height, input.Text, true}, true)
		},
	})
}
