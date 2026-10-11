package application

import (
	"context"
	"encoding/json"
	"fmt"
)

type SpreadsheetRecalculateController interface {
	RecalculateSpreadsheet(context.Context) error
}

func registerSpreadsheetRecalculateCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.workbook.recalculate", Name: "触发工作簿公式重算",
			Description: "使 XLSX 公式缓存失效；Glance 后续读取公式单元格时会按需使用当前计算引擎支持的函数重新计算。不会遍历并预计算整个工作簿，也不会刷新外部数据连接。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"formula_cache_invalidated":{"type":"boolean"},"external_data_refreshed":{"type":"boolean"}},"required":["changed","formula_cache_invalidated","external_data_refreshed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			recalculator, ok := capability.Adapter.(SpreadsheetRecalculateController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			if err := recalculator.RecalculateSpreadsheet(ctx); err != nil {
				return CapabilityResult{}, fmt.Errorf("recalculate XLSX formula cache: %w", err)
			}
			return capabilityJSONResult(struct {
				Changed                 bool `json:"changed"`
				FormulaCacheInvalidated bool `json:"formula_cache_invalidated"`
				ExternalDataRefreshed   bool `json:"external_data_refreshed"`
			}{true, true, false}, true)
		},
	})
}
