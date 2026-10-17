package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func registerSpreadsheetCellTimeCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.cell.time.insert", Name: "插入静态时间",
			Description: "将明确的 HH:mm 时间写入指定单元格；常规格式使用 Excel 内置 24 小时制时间格式，其他单元格格式保持不变。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255},"cell":{"type":"string","minLength":2,"maxLength":8},"time":{"type":"string","pattern":"^([01][0-9]|2[0-3]):[0-5][0-9]$"}},"required":["sheet","cell","time"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"time":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","cell","time","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet string `json:"sheet"`
				Cell  string `json:"cell"`
				Time  string `json:"time"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.cell.time.insert: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			input.Cell = strings.ToUpper(strings.TrimSpace(input.Cell))
			clock, err := time.Parse("15:04", input.Time)
			if input.Sheet == "" || !validSpreadsheetCellReference(input.Cell) || err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			setter, ok := capability.Adapter.(SpreadsheetCellTimeSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetSpreadsheetCellTime(ctx, input.Sheet, input.Cell, clock)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Sheet   string `json:"sheet"`
				Cell    string `json:"cell"`
				Time    string `json:"time"`
				Changed bool   `json:"changed"`
			}{input.Sheet, input.Cell, clock.Format("15:04"), changed}, changed)
		},
	})
}
