package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"glance/internal/document"
)

func spreadsheetFormatPainterRegistration() CapabilityRegistration {
	return CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.format.paint", Name: "复制单元格格式",
			Description: "读取指定源单元格的格式，并将当前中立格式模型支持的属性应用到同一工作表的目标区域。最多处理 10000 个目标单元格；这是一次显式范围操作，不依赖当前选区。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX, FormatCSV}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"source_cell":{"type":"string","minLength":2,"maxLength":10},"target_bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false}},"required":["sheet","source_cell","target_bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"source_cell":{"type":"string"},"target_bounds":{"type":"object"},"changed":{"type":"boolean"}},"required":["sheet","source_cell","target_bounds","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet        string             `json:"sheet"`
				SourceCell   string             `json:"source_cell"`
				TargetBounds document.CellRange `json:"target_bounds"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.format.paint: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			input.SourceCell = strings.ToUpper(strings.TrimSpace(input.SourceCell))
			if input.Sheet == "" || !validSpreadsheetCellReference(input.SourceCell) || !validCellRange(input.TargetBounds, 10_000) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			painter, ok := capability.Adapter.(SpreadsheetFormatPainterController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			if _, ok := spreadsheetCellReferenceRange(input.SourceCell); !ok {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			changed, err := painter.PaintSpreadsheetCellFormat(ctx, input.Sheet, input.SourceCell, input.TargetBounds)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Sheet        string             `json:"sheet"`
				SourceCell   string             `json:"source_cell"`
				TargetBounds document.CellRange `json:"target_bounds"`
				Changed      bool               `json:"changed"`
			}{input.Sheet, input.SourceCell, input.TargetBounds, changed}, changed)
		},
	}
}

func spreadsheetCellReferenceRange(cell string) (document.CellRange, bool) {
	if !validSpreadsheetCellReference(cell) {
		return document.CellRange{}, false
	}
	column, index := 0, 0
	for index < len(cell) && cell[index] >= 'A' && cell[index] <= 'Z' {
		column = column*26 + int(cell[index]-'A'+1)
		index++
	}
	row, err := strconv.Atoi(cell[index:])
	if err != nil || row < 1 || column < 1 {
		return document.CellRange{}, false
	}
	return document.CellRange{StartRow: row, StartColumn: column, EndRow: row, EndColumn: column}, true
}
