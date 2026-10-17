package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/document"
)

func registerSpreadsheetFillRightCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.range.fill_right", Name: "向右填充单元格",
			Description: "将明确区域最左列的值、公式和单元格格式填充到右侧各列；相对公式引用按列调整。整个区域作为一个可撤销操作，最多处理 10000 个单元格。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false}},"required":["sheet","bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"bounds":{"type":"object"}},"required":["changed","sheet","bounds"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet  string             `json:"sheet"`
				Bounds document.CellRange `json:"bounds"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.range.fill_right: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			if input.Sheet == "" || input.Bounds.EndColumn <= input.Bounds.StartColumn || !validCellRange(input.Bounds, 10_000) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			setter, ok := capability.Adapter.(SpreadsheetFillRightSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.FillRightSpreadsheet(ctx, input.Sheet, input.Bounds)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed bool               `json:"changed"`
				Sheet   string             `json:"sheet"`
				Bounds  document.CellRange `json:"bounds"`
			}{changed, input.Sheet, input.Bounds}, changed)
		},
	})
}
