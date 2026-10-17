package application

import (
	"context"
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"strings"
)

type SpreadsheetRangeContentClearer interface {
	ClearSpreadsheetCellRange(context.Context, string, document.CellRange) (bool, error)
}

func validSpreadsheetFormatRange(b document.CellRange) bool {
	if !validCellRange(b, 1_048_576*16_384) {
		return false
	}
	return (b.StartRow == 1 && b.EndRow == 1_048_576) || (b.StartColumn == 1 && b.EndColumn == 16_384) || validCellRange(b, 10_000)
}

func registerSpreadsheetRangeClearCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.range.clear", Name: "清除单元格区域内容", Description: "清除指定区域已有单元格的值和公式，保留格式与行列尺寸；支持整行、整列和全表。", Category: CapabilityCommand, Formats: []Format{FormatXLSX, FormatCSV}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","end_row","start_column","end_column"],"additionalProperties":false}},"required":["sheet","bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"}},"required":["changed"],"additionalProperties":false}`),
		}, Handler: func(ctx context.Context, c CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet  string             `json:"sheet"`
				Bounds document.CellRange `json:"bounds"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.range.clear: %w", err)
			}
			if strings.TrimSpace(input.Sheet) == "" || !validCellRange(input.Bounds, 1_048_576*16_384) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			editor, ok := c.Adapter.(SpreadsheetRangeContentClearer)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := editor.ClearSpreadsheetCellRange(ctx, input.Sheet, input.Bounds)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool `json:"changed"`
			}{changed}, changed)
		}})
}

func (a *adapterFuncs) ClearSpreadsheetCellRange(ctx context.Context, sheet string, bounds document.CellRange) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.clearCellRange == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.clearCellRange(ctx, sheet, bounds)
}
