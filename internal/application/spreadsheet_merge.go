package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/document"
)

func registerSpreadsheetMergeCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.cells.merge", Name: "合并并居中单元格",
			Description: "合并明确选区并将左上角单元格水平居中；非空的其他单元格会先显示内容丢失确认，取消合并可通过再次执行此操作完成。最多处理 10000 个单元格。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"confirm_discard_token":{"type":"string","maxLength":64}},"required":["sheet","bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"confirmation_required":{"type":"boolean"},"discarded_cell_count":{"type":"integer","minimum":0},"confirm_discard_token":{"type":"string"},"unmerged":{"type":"boolean"}},"required":["changed","confirmation_required","discarded_cell_count","unmerged"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet               string             `json:"sheet"`
				Bounds              document.CellRange `json:"bounds"`
				ConfirmDiscardToken string             `json:"confirm_discard_token"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.cells.merge: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			if input.Sheet == "" || len(input.ConfirmDiscardToken) > 64 || !validCellRange(input.Bounds, 10_000) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			setter, ok := capability.Adapter.(SpreadsheetMergeCellsSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			result, err := setter.MergeCellsSpreadsheet(ctx, input.Sheet, input.Bounds, input.ConfirmDiscardToken)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(result, result.Changed)
		},
	})
}
