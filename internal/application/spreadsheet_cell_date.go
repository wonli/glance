package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func registerSpreadsheetCellDateCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.cell.date.insert", Name: "插入静态日期",
			Description: "将明确的 YYYY-MM-DD 日期写入指定单元格；常规格式使用 Excel 默认日期格式，其他单元格格式保持不变。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255},"cell":{"type":"string","minLength":2,"maxLength":8},"date":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"}},"required":["sheet","cell","date"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"date":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","cell","date","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet string `json:"sheet"`
				Cell  string `json:"cell"`
				Date  string `json:"date"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.cell.date.insert: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			input.Cell = strings.ToUpper(strings.TrimSpace(input.Cell))
			date, err := time.Parse("2006-01-02", input.Date)
			if input.Sheet == "" || !validSpreadsheetCellReference(input.Cell) || err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			setter, ok := capability.Adapter.(SpreadsheetCellDateSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			changed, err := setter.SetSpreadsheetCellDate(ctx, input.Sheet, input.Cell, date)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Sheet   string `json:"sheet"`
				Cell    string `json:"cell"`
				Date    string `json:"date"`
				Changed bool   `json:"changed"`
			}{input.Sheet, input.Cell, date.Format("2006-01-02"), changed}, changed)
		},
	})
}
