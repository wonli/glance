package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"glance/internal/document"
)

func registerSpreadsheetPivotTableCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.pivot.create", Name: "创建数据透视表",
			Description: "基于明确工作表和源数据区域创建 XLSX 数据透视表。源区域至少包含标题行和一行数据，最多 1000000 个单元格。可提供行字段、列字段和值字段；省略字段时使用首个、第二个和最后一个标题作为默认值。源标题为空时会补写“字段N”名称。pivot_table_range 可用同一工作表上的 A1 或 A1:B2 形式指定输出位置，省略时由适配器选择默认位置。先确认源数据表头和目标区域。Ribbon 数据透视表按钮也复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255,"description":"源数据所在工作表的精确名称。"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false,"description":"包含一行表头和至少一行数据的源区域。"},"pivot_table_range":{"type":"string","maxLength":64,"description":"同一工作表上的目标锚点或区域，例如 F2 或 F2:J15。"},"rows":{"type":"array","maxItems":64,"items":{"type":"string","minLength":1,"maxLength":255},"description":"用作行分组的源表头名称。"},"columns":{"type":"array","maxItems":64,"items":{"type":"string","minLength":1,"maxLength":255},"description":"用作列分组的源表头名称。"},"data":{"type":"array","maxItems":64,"items":{"type":"string","minLength":1,"maxLength":255},"description":"用作汇总值的源表头名称；当前适配器按求和方式汇总。"}},"required":["sheet","bounds"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object","properties":{"start_row":{"type":"integer"},"start_column":{"type":"integer"},"end_row":{"type":"integer"},"end_column":{"type":"integer"}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"pivot_table_range":{"type":"string"},"rows":{"type":"array","items":{"type":"string"}},"columns":{"type":"array","items":{"type":"string"}},"data":{"type":"array","items":{"type":"string"}},"changed":{"type":"boolean"}},"required":["sheet","bounds","pivot_table_range","rows","columns","data","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Sheet           string             `json:"sheet"`
				Bounds          document.CellRange `json:"bounds"`
				PivotTableRange string             `json:"pivot_table_range"`
				Rows            []string           `json:"rows"`
				Columns         []string           `json:"columns"`
				Data            []string           `json:"data"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.pivot.create: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			input.PivotTableRange = strings.TrimSpace(input.PivotTableRange)
			input.Rows = normalizePivotFields(input.Rows)
			input.Columns = normalizePivotFields(input.Columns)
			input.Data = normalizePivotFields(input.Data)
			targetBounds, targetValid := parseSpreadsheetPivotRange(input.PivotTableRange)
			if input.Sheet == "" || len([]rune(input.Sheet)) > 255 || !validCellRange(input.Bounds, 1_000_000) || input.Bounds.EndRow <= input.Bounds.StartRow || len(input.PivotTableRange) > 64 || (input.PivotTableRange != "" && (!targetValid || cellRangesOverlap(input.Bounds, targetBounds))) || !validPivotFields(input.Rows) || !validPivotFields(input.Columns) || !validPivotFields(input.Data) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			reader, ok := capability.Adapter.(SpreadsheetRangeReaderCapability)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			headers, err := reader.ReadSpreadsheetRange(ctx, input.Sheet, document.CellRange{
				StartRow: input.Bounds.StartRow, EndRow: input.Bounds.StartRow,
				StartColumn: input.Bounds.StartColumn, EndColumn: input.Bounds.EndColumn,
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			fieldNames, err := spreadsheetPivotHeaderNames(headers, input.Bounds.EndColumn-input.Bounds.StartColumn+1)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(input.Rows) == 0 {
				input.Rows = []string{fieldNames[0]}
			}
			if len(input.Data) == 0 {
				input.Data = []string{fieldNames[len(fieldNames)-1]}
			}
			if len(input.Columns) == 0 && len(fieldNames) > 2 {
				input.Columns = []string{fieldNames[1]}
			}
			if input.Columns == nil {
				input.Columns = []string{}
			}
			if !pivotFieldsExist(input.Rows, fieldNames) || !pivotFieldsExist(input.Columns, fieldNames) || !pivotFieldsExist(input.Data, fieldNames) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			controller, ok := capability.Adapter.(SpreadsheetPivotTableController)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			spec := document.SpreadsheetPivotTableSpec{
				Sheet: input.Sheet, DataRange: input.Bounds, PivotTableRange: input.PivotTableRange,
				Rows: input.Rows, Columns: input.Columns, Data: input.Data,
			}
			if err := controller.CreateSpreadsheetPivotTable(ctx, spec); err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Sheet           string             `json:"sheet"`
				Bounds          document.CellRange `json:"bounds"`
				PivotTableRange string             `json:"pivot_table_range"`
				Rows            []string           `json:"rows"`
				Columns         []string           `json:"columns"`
				Data            []string           `json:"data"`
				Changed         bool               `json:"changed"`
			}{input.Sheet, input.Bounds, input.PivotTableRange, input.Rows, input.Columns, input.Data, true}, true)
		},
	})
}

func spreadsheetPivotHeaderNames(rows [][]document.Cell, expected int) ([]string, error) {
	if len(rows) == 0 || len(rows[0]) < expected || expected < 1 {
		return nil, ErrInvalidCommandArguments
	}
	fields := make([]string, expected)
	seen := make(map[string]struct{}, expected)
	for i := range fields {
		name := strings.TrimSpace(rows[0][i].Value)
		if name == "" {
			name = fmt.Sprintf("字段%d", i+1)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("pivot table source headers must be unique: %q", name)
		}
		seen[name] = struct{}{}
		fields[i] = name
	}
	return fields, nil
}

func pivotFieldsExist(requested, available []string) bool {
	set := make(map[string]struct{}, len(available))
	for _, field := range available {
		set[field] = struct{}{}
	}
	for _, field := range requested {
		if _, ok := set[field]; !ok {
			return false
		}
	}
	return true
}

func validPivotFields(fields []string) bool {
	if len(fields) > 64 {
		return false
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field == "" || len([]rune(field)) > 255 {
			return false
		}
		if _, exists := seen[field]; exists {
			return false
		}
		seen[field] = struct{}{}
	}
	return true
}

func normalizePivotFields(fields []string) []string {
	if len(fields) == 0 {
		return []string{}
	}
	normalized := make([]string, len(fields))
	for i, field := range fields {
		normalized[i] = strings.TrimSpace(field)
	}
	return normalized
}

func parseSpreadsheetPivotRange(value string) (document.CellRange, bool) {
	if value == "" {
		return document.CellRange{}, true
	}
	parts := strings.Split(value, ":")
	if len(parts) < 1 || len(parts) > 2 {
		return document.CellRange{}, false
	}
	startRow, startColumn, ok := parsePivotCell(strings.TrimSpace(parts[0]))
	if !ok {
		return document.CellRange{}, false
	}
	endRow, endColumn := startRow, startColumn
	if len(parts) == 2 {
		endRow, endColumn, ok = parsePivotCell(strings.TrimSpace(parts[1]))
		if !ok || endRow < startRow || endColumn < startColumn {
			return document.CellRange{}, false
		}
	}
	return document.CellRange{StartRow: startRow, StartColumn: startColumn, EndRow: endRow, EndColumn: endColumn}, true
}

func parsePivotCell(value string) (row, column int, ok bool) {
	index := 0
	for index < len(value) && value[index] >= 'A' && value[index] <= 'Z' {
		column = column*26 + int(value[index]-'A'+1)
		if column > 16_384 {
			return 0, 0, false
		}
		index++
	}
	if index == 0 || index == len(value) {
		return 0, 0, false
	}
	row, err := strconv.Atoi(value[index:])
	return row, column, err == nil && row >= 1 && row <= 1_048_576
}

func cellRangesOverlap(left, right document.CellRange) bool {
	return left.StartRow <= right.EndRow && right.StartRow <= left.EndRow && left.StartColumn <= right.EndColumn && right.StartColumn <= left.EndColumn
}
