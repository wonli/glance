package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/document"
)

func registerSpreadsheetCellTransferCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.cells.transfer", Name: "粘贴单元格区域",
			Description: "将已捕获的单元格矩阵粘贴到明确工作表位置。保留单元格公式、原始值类型和完整 XLSX 样式，并按目标位置调整相对公式引用；整块作为一个可撤销操作。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX, FormatCSV}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"source":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"target_row":{"type":"integer","minimum":1,"maximum":1048576},"target_column":{"type":"integer","minimum":1,"maximum":16384},"cells":{"type":"array","minItems":1,"maxItems":10000,"items":{"type":"array","minItems":1,"items":{"type":"object"}}}},"required":["sheet","source","target_row","target_column","cells"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"target_row":{"type":"integer"},"target_column":{"type":"integer"},"cell_count":{"type":"integer"}},"required":["changed","sheet","target_row","target_column","cell_count"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet        string             `json:"sheet"`
				Source       document.CellRange `json:"source"`
				TargetRow    int                `json:"target_row"`
				TargetColumn int                `json:"target_column"`
				Cells        [][]document.Cell  `json:"cells"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.cells.transfer: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			if input.Sheet == "" || !validCellRange(input.Source, 10_000) ||
				input.TargetRow < 1 || input.TargetRow > 1_048_576 ||
				input.TargetColumn < 1 || input.TargetColumn > 16_384 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			rows, columns := input.Source.EndRow-input.Source.StartRow+1, input.Source.EndColumn-input.Source.StartColumn+1
			if len(input.Cells) != rows || rows*columns > 10_000 ||
				input.TargetRow+rows-1 > 1_048_576 || input.TargetColumn+columns-1 > 16_384 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			for _, row := range input.Cells {
				if len(row) != columns {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			}
			setter, ok := capability.Adapter.(SpreadsheetCellTransferSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.TransferSpreadsheetCellRange(ctx, input.Sheet, input.Source, input.TargetRow, input.TargetColumn, input.Cells)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed      bool   `json:"changed"`
				Sheet        string `json:"sheet"`
				TargetRow    int    `json:"target_row"`
				TargetColumn int    `json:"target_column"`
				CellCount    int    `json:"cell_count"`
			}{changed, input.Sheet, input.TargetRow, input.TargetColumn, rows * columns}, changed)
		},
	})
}
