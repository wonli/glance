package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/document"
	"glance/internal/format/sheetpdf"
)

func registerSpreadsheetPDFExportCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.document.export_pdf", Name: "导出工作表为 PDF",
			Description: "将指定 XLSX 工作表中的明确单元格范围导出为 PDF。范围最多 5000 行、100 列和 500000 个单元格；方向可选 portrait 或 landscape，默认 portrait。会使用当前工作表的中立网格、格式和锚定图片，校验生成结果并创建新的 .pdf 路径；不会覆盖已有文件，也不改变源工作簿。导出前请先读取目标数据并使用当前修订号。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"工作表的精确名称。"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false,"description":"要导出的明确单元格范围，最多 5000 行、100 列及 500000 个单元格。"},"target_path":{"type":"string","minLength":1,"description":"新的 PDF 输出路径，必须以 .pdf 结尾；目标已存在时拒绝覆盖。"},"orientation":{"type":"string","enum":["portrait","landscape"],"description":"页面方向，默认 portrait。"}},"required":["sheet","bounds","target_path"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"bytes_written":{"type":"integer","minimum":1},"page_count":{"type":"integer","minimum":1}},"required":["path","bytes_written","page_count"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet       string             `json:"sheet"`
				Bounds      document.CellRange `json:"bounds"`
				TargetPath  string             `json:"target_path"`
				Orientation string             `json:"orientation"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			path := canonicalPath(strings.TrimSpace(input.TargetPath))
			rows := input.Bounds.EndRow - input.Bounds.StartRow + 1
			columns := input.Bounds.EndColumn - input.Bounds.StartColumn + 1
			if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || path == "." || !strings.EqualFold(filepath.Ext(path), ".pdf") ||
				!validCellRange(input.Bounds, 500_000) || rows > 5000 || columns > 100 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if source := capability.Document.SourcePath; source != "" && canonicalPath(source) == path {
				return CapabilityResult{}, ErrTargetExists
			}
			if _, err := os.Lstat(path); err == nil {
				return CapabilityResult{}, ErrTargetExists
			} else if !errors.Is(err, os.ErrNotExist) {
				return CapabilityResult{}, err
			}
			controller, ok := capability.Adapter.(SpreadsheetPDFController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			window, err := controller.ReadSpreadsheetPDFWindow(ctx, input.Sheet, input.Bounds)
			if err != nil {
				return CapabilityResult{}, err
			}
			images := spreadsheetPDFImageSource{ctx: ctx, controller: controller}
			result, err := sheetpdf.ExportWindow(ctx, path, input.Sheet, window, input.Orientation, images, func() (func(), error) {
				return capability.Runtime.lockDocumentRevision(ctx, capability.Document.DocumentID, capability.Document.Revision)
			})
			if err != nil {
				if errors.Is(err, os.ErrExist) {
					return CapabilityResult{}, ErrTargetExists
				}
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(result, false)
		},
	})
}

type spreadsheetPDFImageSource struct {
	ctx        context.Context
	controller SpreadsheetPDFController
}

func (s spreadsheetPDFImageSource) SpreadsheetImages(sheet string) ([]document.SpreadsheetImage, error) {
	return s.controller.ListSpreadsheetImages(s.ctx, sheet)
}

func (s spreadsheetPDFImageSource) SpreadsheetImageBytes(imageID string) ([]byte, error) {
	return s.controller.ReadSpreadsheetImageBytes(s.ctx, imageID)
}
