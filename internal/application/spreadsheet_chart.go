package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/document"
)

func registerSpreadsheetChartCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.chart.create", Name: "创建工作表图表",
			Description: "基于明确 XLSX 工作表区域创建柱状图、折线图或饼图。源区域至少包含标题行和一行数据，最多 1000000 个单元格。可明确设置图表锚点单元格和标题；省略锚点时放在源区域右侧两列。锚点不能落在源数据区域内。Ribbon 三种图表按钮也调用此命令。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"源数据所在工作表的精确名称。"},"kind":{"type":"string","enum":["col","line","pie"],"description":"图表类型：柱状图 col、折线图 line 或饼图 pie。"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false,"description":"包含标题行和至少一行数据的源区域。"},"cell":{"type":"string","maxLength":16,"description":"图表左上锚点单元格，例如 E2；省略时放在源区域右侧两列。"},"title":{"type":"string","maxLength":255,"description":"图表标题；省略或空字符串时使用对应图表类型的默认标题。"}},"required":["sheet","kind","bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"kind":{"type":"string","enum":["col","line","pie"]},"bounds":{"type":"object","properties":{"start_row":{"type":"integer"},"start_column":{"type":"integer"},"end_row":{"type":"integer"},"end_column":{"type":"integer"}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"cell":{"type":"string"},"title":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","kind","bounds","cell","title","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet  string             `json:"sheet"`
				Kind   string             `json:"kind"`
				Bounds document.CellRange `json:"bounds"`
				Cell   string             `json:"cell"`
				Title  string             `json:"title"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.chart.create: %w", err)
			}
			input.Sheet, input.Cell, input.Title = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.Cell), strings.TrimSpace(input.Title)
			if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || len([]rune(input.Title)) > 255 || !validCellRange(input.Bounds, 1_000_000) || input.Bounds.EndRow <= input.Bounds.StartRow {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			kind := document.SpreadsheetChartKind(input.Kind)
			switch kind {
			case document.SpreadsheetChartColumn, document.SpreadsheetChartLine, document.SpreadsheetChartPie:
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if input.Title == "" {
				switch kind {
				case document.SpreadsheetChartLine:
					input.Title = "折线图"
				case document.SpreadsheetChartPie:
					input.Title = "饼图"
				default:
					input.Title = "柱状图"
				}
			}
			if input.Cell == "" {
				input.Cell = spreadsheetCellName(input.Bounds.StartRow, input.Bounds.EndColumn+2)
			}
			row, column, ok := parsePivotCell(input.Cell)
			if !ok || row >= input.Bounds.StartRow && row <= input.Bounds.EndRow && column >= input.Bounds.StartColumn && column <= input.Bounds.EndColumn {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			controller, ok := capability.Adapter.(SpreadsheetChartController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			spec := document.SpreadsheetChartSpec{Sheet: input.Sheet, Cell: input.Cell, Kind: kind, Title: input.Title, Range: input.Bounds}
			if err := controller.CreateSpreadsheetChart(ctx, spec); err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Sheet   string             `json:"sheet"`
				Kind    string             `json:"kind"`
				Bounds  document.CellRange `json:"bounds"`
				Cell    string             `json:"cell"`
				Title   string             `json:"title"`
				Changed bool               `json:"changed"`
			}{input.Sheet, input.Kind, input.Bounds, input.Cell, input.Title, true}, true)
		},
	})
}
