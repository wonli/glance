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
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
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
			reader, ok := capability.Adapter.(SpreadsheetGridWindowController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			sourceRange, ok := spreadsheetCellReferenceRange(input.SourceCell)
			if !ok {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			source, err := reader.ReadSpreadsheetGridWindow(ctx, input.Sheet, sourceRange)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(source.Cells) == 0 || len(source.Cells[0]) == 0 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			formatter, ok := capability.Adapter.(SpreadsheetRangeFormatter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			patch := spreadsheetFormatPatchFromStyle(source.Cells[0][0].Style)
			changed, err := formatter.SetSpreadsheetCellFormats(ctx, input.Sheet, input.TargetBounds, patch)
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

func spreadsheetFormatPatchFromStyle(style document.CellStyle) document.CellFormatPatch {
	bold, italic, underline, wrap := style.Bold, style.Italic, style.Underline, style.WrapText
	patch := document.CellFormatPatch{Bold: &bold, Italic: &italic, Underline: &underline, WrapText: &wrap}
	if style.FontColor != "" {
		color := style.FontColor
		patch.FontColor = &color
	}
	if style.FillColor != "" {
		fill := style.FillColor
		patch.FillColor = &fill
	}
	if style.HorizontalAlignment != "" {
		alignment := style.HorizontalAlignment
		patch.HorizontalAlignment = &alignment
	}
	if style.NumberFormat != "" {
		numberFormat := style.NumberFormat
		patch.NumberFormat = &numberFormat
	}
	if style.FontSize > 0 {
		fontSize := style.FontSize
		patch.FontSize = &fontSize
	}
	if style.FontFamily != "" {
		fontFamily := style.FontFamily
		patch.FontFamily = &fontFamily
	}
	for _, border := range style.Borders {
		if border.Style != 0 {
			preset := "all"
			patch.BorderPreset = &preset
			break
		}
	}
	return patch
}
