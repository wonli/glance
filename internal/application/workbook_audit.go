package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"glance/internal/document"
)

const maxWorkbookAuditFindings = 100

type workbookAuditFinding struct {
	Code           string   `json:"code"`
	Message        string   `json:"message"`
	Cells          []string `json:"cells"`
	CellsTruncated bool     `json:"cells_truncated,omitempty"`
	Value          string   `json:"value,omitempty"`
}

type workbookAuditResult struct {
	Sheet           string                 `json:"sheet"`
	Range           document.CellRange     `json:"range"`
	CellsScanned    int                    `json:"cells_scanned"`
	NonEmptyCells   int                    `json:"non_empty_cells"`
	BlankCells      int                    `json:"blank_cells"`
	FormulaCells    int                    `json:"formula_cells"`
	FindingsTotal   int                    `json:"findings_total"`
	HasMoreFindings bool                   `json:"has_more_findings"`
	Findings        []workbookAuditFinding `json:"findings"`
}

func registerWorkbookAuditCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "workbook.audit", Name: "审计工作簿数据",
			Description: "对指定工作表的有界区域执行只读数据审计：统计空白/公式单元格，查找重复表头、缺失必需表头、重复键和以 # 开头的错误样式值。必须提供工作表和不超过 10000 个单元格的区域；header_row 省略或为 0 时使用区域首行，否则使用工作表绝对行号；key_columns 使用工作表绝对列号。表头按去除首尾空格并忽略大小写比较，键列有空值的行不参与重复键检查。只读取文档中的值和缓存结果，不重新计算公式。最多返回 100 条发现。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatXLSX, FormatXLS}, Risk: RiskLow, RequiresDocument: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1,"maxLength":255},"range":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"header_row":{"type":"integer","minimum":0,"maximum":1048576},"required_headers":{"type":"array","maxItems":64,"items":{"type":"string","minLength":1,"maxLength":128}},"key_columns":{"type":"array","maxItems":32,"items":{"type":"integer","minimum":1,"maximum":16384}}},"required":["sheet","range"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"range":{"type":"object","properties":{"start_row":{"type":"integer"},"start_column":{"type":"integer"},"end_row":{"type":"integer"},"end_column":{"type":"integer"}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"cells_scanned":{"type":"integer"},"non_empty_cells":{"type":"integer"},"blank_cells":{"type":"integer"},"formula_cells":{"type":"integer"},"findings_total":{"type":"integer"},"has_more_findings":{"type":"boolean"},"findings":{"type":"array","maxItems":100,"items":{"type":"object","properties":{"code":{"type":"string"},"message":{"type":"string"},"cells":{"type":"array","maxItems":1000,"items":{"type":"string"}},"cells_truncated":{"type":"boolean"},"value":{"type":"string"}},"required":["code","message","cells"],"additionalProperties":false}}},"required":["sheet","range","cells_scanned","non_empty_cells","blank_cells","formula_cells","findings_total","has_more_findings","findings"],"additionalProperties":false}`),
		},
		Handler: handleWorkbookAudit,
	})
}

func handleWorkbookAudit(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	var input struct {
		Sheet          string             `json:"sheet"`
		Range          document.CellRange `json:"range"`
		HeaderRow      int                `json:"header_row"`
		RequiredHeader []string           `json:"required_headers"`
		KeyColumns     []int              `json:"key_columns"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return CapabilityResult{}, fmt.Errorf("decode workbook.audit: %w", err)
	}
	input.Sheet = strings.TrimSpace(input.Sheet)
	if input.Sheet == "" || !validWorkbookAuditRange(input.Range) || len(input.RequiredHeader) > 64 || len(input.KeyColumns) > 32 {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	headerRow := input.HeaderRow
	if headerRow == 0 {
		headerRow = input.Range.StartRow
	}
	if headerRow < input.Range.StartRow || headerRow > input.Range.EndRow {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	for _, header := range input.RequiredHeader {
		if strings.TrimSpace(header) == "" || len([]rune(header)) > 128 {
			return CapabilityResult{}, ErrInvalidCommandArguments
		}
	}
	seenColumns := make(map[int]struct{}, len(input.KeyColumns))
	for _, column := range input.KeyColumns {
		if column < input.Range.StartColumn || column > input.Range.EndColumn {
			return CapabilityResult{}, ErrInvalidCommandArguments
		}
		if _, duplicate := seenColumns[column]; duplicate {
			return CapabilityResult{}, ErrInvalidCommandArguments
		}
		seenColumns[column] = struct{}{}
	}

	snapshot, err := capability.Runtime.Queries().InspectDocument(ctx, InspectRequest{
		DocumentID: capability.Document.DocumentID, Sheet: input.Sheet, Range: &input.Range, MaxBytes: MaxQueryResultBytes,
	})
	if err != nil {
		if err == ErrInvalidQuery || err == ErrQueryResultTooLarge {
			return CapabilityResult{}, ErrInvalidCommandArguments
		}
		return CapabilityResult{}, err
	}
	var inspected spreadsheetQueryResult
	if err := json.Unmarshal(snapshot.Content, &inspected); err != nil {
		return CapabilityResult{}, fmt.Errorf("decode workbook audit range: %w", err)
	}
	result := workbookAuditResult{
		Sheet: input.Sheet, Range: input.Range, Findings: make([]workbookAuditFinding, 0),
	}
	appendFinding := func(finding workbookAuditFinding) {
		if len(finding.Cells) > MaxQueryItems {
			finding.Cells = finding.Cells[:MaxQueryItems]
			finding.CellsTruncated = true
			result.HasMoreFindings = true
		}
		result.FindingsTotal++
		if len(result.Findings) < maxWorkbookAuditFindings {
			result.Findings = append(result.Findings, finding)
		} else {
			result.HasMoreFindings = true
		}
	}
	for rowIndex, row := range inspected.Rows {
		if err := ctx.Err(); err != nil {
			return CapabilityResult{}, err
		}
		absoluteRow := input.Range.StartRow + rowIndex
		result.CellsScanned += len(row)
		for columnIndex, cell := range row {
			if cell.Value != "" || cell.Formula != "" {
				result.NonEmptyCells++
			}
			if cell.Formula != "" {
				result.FormulaCells++
			}
			if strings.HasPrefix(strings.TrimSpace(cell.Value), "#") {
				address := workbookAuditCellAddress(absoluteRow, input.Range.StartColumn+columnIndex)
				appendFinding(workbookAuditFinding{
					Code: "error_like_value", Message: "单元格显示以 # 开头的值；本审计只读取显示或缓存值，不重新计算公式。",
					Cells: []string{address}, Value: cell.Value,
				})
			}
		}
	}
	result.BlankCells = result.CellsScanned - result.NonEmptyCells

	headerIndex := headerRow - input.Range.StartRow
	if headerIndex < 0 || headerIndex >= len(inspected.Rows) {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	headerCells := inspected.Rows[headerIndex]
	headers := make(map[string][]string)
	for index, cell := range headerCells {
		value := strings.TrimSpace(cell.Value)
		if value == "" {
			continue
		}
		normalized := strings.ToLower(value)
		address := workbookAuditCellAddress(headerRow, input.Range.StartColumn+index)
		headers[normalized] = append(headers[normalized], address)
	}
	headerKeys := make([]string, 0, len(headers))
	for normalized := range headers {
		headerKeys = append(headerKeys, normalized)
	}
	sort.Strings(headerKeys)
	for _, normalized := range headerKeys {
		cells := headers[normalized]
		if len(cells) > 1 {
			appendFinding(workbookAuditFinding{
				Code: "duplicate_header", Message: "表头名称重复。", Cells: cells, Value: normalized,
			})
		}
	}
	for _, required := range input.RequiredHeader {
		if len(headers[strings.ToLower(strings.TrimSpace(required))]) == 0 {
			appendFinding(workbookAuditFinding{
				Code: "missing_required_header", Message: "未找到指定的必需表头。", Cells: []string{}, Value: required,
			})
		}
	}
	if len(input.KeyColumns) > 0 {
		type duplicateKeyRows struct {
			value string
			cells []string
		}
		keys := make(map[string]*duplicateKeyRows)
		for rowIndex, row := range inspected.Rows {
			absoluteRow := input.Range.StartRow + rowIndex
			if absoluteRow <= headerRow {
				continue
			}
			parts := make([]string, len(input.KeyColumns))
			cells := make([]string, len(input.KeyColumns))
			complete := true
			for index, column := range input.KeyColumns {
				cellIndex := column - input.Range.StartColumn
				if cellIndex >= len(row) {
					complete = false
					break
				}
				parts[index] = strings.TrimSpace(row[cellIndex].Value)
				cells[index] = workbookAuditCellAddress(absoluteRow, column)
				if parts[index] == "" {
					complete = false
				}
			}
			if !complete {
				continue
			}
			key := strings.Join(parts, "\x00")
			if existing := keys[key]; existing != nil {
				existing.cells = append(existing.cells, cells...)
				continue
			}
			keys[key] = &duplicateKeyRows{value: strings.Join(parts, " | "), cells: cells}
		}
		duplicateKeys := make([]string, 0, len(keys))
		for key := range keys {
			duplicateKeys = append(duplicateKeys, key)
		}
		sort.Strings(duplicateKeys)
		for _, key := range duplicateKeys {
			duplicate := keys[key]
			if len(duplicate.cells) > len(input.KeyColumns) {
				appendFinding(workbookAuditFinding{
					Code: "duplicate_key", Message: "数据区域中存在重复的完整键值。",
					Cells: duplicate.cells, Value: duplicate.value,
				})
			}
		}
	}
	result.HasMoreFindings = result.HasMoreFindings || result.FindingsTotal > len(result.Findings)
	capabilityResult, err := capabilityJSONResult(result, false)
	if err != nil {
		return CapabilityResult{}, err
	}
	if len(capabilityResult.Data) > MaxQueryResultBytes {
		return CapabilityResult{}, ErrQueryResultTooLarge
	}
	return capabilityResult, nil
}

func validWorkbookAuditRange(bounds document.CellRange) bool {
	if bounds.StartRow < 1 || bounds.StartColumn < 1 || bounds.EndRow < bounds.StartRow || bounds.EndColumn < bounds.StartColumn || bounds.EndRow > 1_048_576 || bounds.EndColumn > 16_384 {
		return false
	}
	rows, columns := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	return rows > 0 && columns > 0 && rows <= MaxQueryCells/columns
}

func workbookAuditCellAddress(row, column int) string {
	name := ""
	for column > 0 {
		column--
		name = string(rune('A'+column%26)) + name
		column /= 26
	}
	return fmt.Sprintf("%s%d", name, row)
}
