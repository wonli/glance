package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"glance/internal/document"
)

func registerSpreadsheetCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.flash_fill.apply", Name: "按已有模式填充空白单元格",
				Description: "在明确范围内按现有数字序列或文本模式填充空白单元格，不覆盖非空单元格。最多处理 10000 个单元格。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false}},"required":["sheet","bounds"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object"},"rows_filled":{"type":"integer","minimum":0}},"required":["sheet","bounds","rows_filled"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet  string             `json:"sheet"`
					Bounds document.CellRange `json:"bounds"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.flash_fill.apply: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || !validCellRange(input.Bounds, 10_000) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetFlashFillController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				result, err := controller.FlashFillSpreadsheet(ctx, input.Sheet, input.Bounds)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet      string             `json:"sheet"`
					Bounds     document.CellRange `json:"bounds"`
					RowsFilled int                `json:"rows_filled"`
				}{input.Sheet, input.Bounds, result.RowsFilled}, result.RowsFilled > 0)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.formulas.fill", Name: "向选区输入公式",
				Description: "将活动单元格输入的公式写入明确的单元格矩形，并按每个目标单元格相对于活动单元格的偏移调整相对 A1 引用；绝对引用保持固定。最多处理 10000 个单元格，整个矩形作为一个可撤销操作。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"active":{"type":"object","properties":{"row":{"type":"integer","minimum":1,"maximum":1048576},"column":{"type":"integer","minimum":1,"maximum":16384}},"required":["row","column"],"additionalProperties":false},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"formula":{"type":"string","minLength":1,"maxLength":8192}},"required":["sheet","active","bounds","formula"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"bounds":{"type":"object"}},"required":["changed","sheet","bounds"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet   string                  `json:"sheet"`
					Active  document.CellCoordinate `json:"active"`
					Bounds  document.CellRange      `json:"bounds"`
					Formula string                  `json:"formula"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.formulas.fill: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || !validCellRange(input.Bounds, 10_000) ||
					input.Active.Row < input.Bounds.StartRow || input.Active.Row > input.Bounds.EndRow ||
					input.Active.Column < input.Bounds.StartColumn || input.Active.Column > input.Bounds.EndColumn ||
					strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input.Formula), "=")) == "" ||
					!withinXLSXFormulaLimit(input.Formula) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(SpreadsheetFormulaRangeSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetSpreadsheetFormulaRange(ctx, input.Sheet, input.Active, input.Bounds, input.Formula)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool                `json:"changed"`
					Sheet   string              `json:"sheet"`
					Bounds  document.CellRange `json:"bounds"`
				}{changed, input.Sheet, input.Bounds}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.comment.add", Name: "新增或更新单元格批注",
				Description: "为明确单元格新增或更新批注；同一单元格已有批注时会替换原批注。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"cell":{"type":"string","minLength":2,"maxLength":8},"author":{"type":"string","maxLength":255,"description":"批注作者，最多 255 个 Unicode 字符。"},"text":{"type":"string","minLength":1,"maxLength":32767,"description":"批注内容，最多 32767 个 Unicode 字符。"}},"required":["sheet","cell","text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"author":{"type":"string"},"text":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","cell","author","text","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet  string `json:"sheet"`
					Cell   string `json:"cell"`
					Author string `json:"author"`
					Text   string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.comment.add: %w", err)
				}
				input.Sheet, input.Cell, input.Author = strings.TrimSpace(input.Sheet), strings.ToUpper(strings.TrimSpace(input.Cell)), strings.TrimSpace(input.Author)
				if input.Author == "" {
					input.Author = "Agent"
				}
				if input.Sheet == "" || !validSpreadsheetCellReference(input.Cell) || strings.TrimSpace(input.Text) == "" || utf8.RuneCountInString(input.Text) > 32767 || utf8.RuneCountInString(input.Author) > 255 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetCommentController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				comment := document.SpreadsheetComment{Author: input.Author, Cell: input.Cell, Text: input.Text}
				changed, err := controller.AddSpreadsheetComment(ctx, input.Sheet, comment)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Author  string `json:"author"`
					Text    string `json:"text"`
					Changed bool   `json:"changed"`
				}{input.Sheet, input.Cell, input.Author, input.Text, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.comment.delete", Name: "删除单元格批注",
				Description: "删除明确单元格中的批注。此操作会永久移除该批注；Agent 调用需人类确认。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"cell":{"type":"string","minLength":2,"maxLength":10}},"required":["sheet","cell"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","cell","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					Cell  string `json:"cell"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.comment.delete: %w", err)
				}
				input.Sheet, input.Cell = strings.TrimSpace(input.Sheet), strings.ToUpper(strings.TrimSpace(input.Cell))
				if input.Sheet == "" || !validSpreadsheetCellReference(input.Cell) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetCommentController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.DeleteSpreadsheetComment(ctx, input.Sheet, input.Cell)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Changed bool   `json:"changed"`
				}{input.Sheet, input.Cell, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.comments.get", Name: "读取工作表批注",
				Description: "读取明确工作表中所有批注的单元格、作者和文本。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1}},"required":["sheet"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"comments":{"type":"array","items":{"type":"object","properties":{"author":{"type":"string"},"cell":{"type":"string"},"text":{"type":"string"}},"required":["author","cell","text"],"additionalProperties":false}}},"required":["sheet","comments"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.comments.get: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetCommentController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				comments, err := controller.GetSpreadsheetComments(ctx, input.Sheet)
				if err != nil {
					return CapabilityResult{}, err
				}
				if comments == nil {
					comments = []document.SpreadsheetComment{}
				}
				return capabilityJSONResult(struct {
					Sheet    string                        `json:"sheet"`
					Comments []document.SpreadsheetComment `json:"comments"`
				}{input.Sheet, comments}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.hyperlink.get", Name: "读取单元格超链接",
				Description: "读取明确工作表和单元格的超链接目标及提示文本；没有链接时 exists 为 false。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"cell":{"type":"string","minLength":2,"maxLength":10}},"required":["sheet","cell"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"exists":{"type":"boolean"},"target":{"type":"string"},"tooltip":{"type":"string"}},"required":["sheet","cell","exists","target","tooltip"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					Cell  string `json:"cell"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.hyperlink.get: %w", err)
				}
				input.Sheet, input.Cell = strings.TrimSpace(input.Sheet), strings.ToUpper(strings.TrimSpace(input.Cell))
				if input.Sheet == "" || !validSpreadsheetCellReference(input.Cell) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetHyperlinkController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				link, exists, err := controller.GetSpreadsheetHyperlink(ctx, input.Sheet, input.Cell)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Exists  bool   `json:"exists"`
					Target  string `json:"target"`
					Tooltip string `json:"tooltip"`
				}{input.Sheet, input.Cell, exists, link.Target, link.Tooltip}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.hyperlink.remove", Name: "删除单元格超链接",
				Description: "删除明确工作表和单元格中的超链接。此操作会移除现有链接，Agent 调用需人类确认。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"cell":{"type":"string","minLength":2,"maxLength":8}},"required":["sheet","cell"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","cell","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					Cell  string `json:"cell"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.hyperlink.remove: %w", err)
				}
				input.Sheet, input.Cell = strings.TrimSpace(input.Sheet), strings.ToUpper(strings.TrimSpace(input.Cell))
				if input.Sheet == "" || !validSpreadsheetCellReference(input.Cell) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetHyperlinkController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetHyperlink(ctx, input.Sheet, input.Cell, "", "", true)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Changed bool   `json:"changed"`
				}{input.Sheet, input.Cell, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.hyperlink.set", Name: "设置单元格超链接",
				Description: "为明确工作表和单元格设置外部 URL 或工作簿内位置链接，并可指定悬停提示；不会自动打开目标地址。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"cell":{"type":"string","minLength":2,"maxLength":10},"target":{"type":"string","minLength":1,"maxLength":2048},"tooltip":{"type":"string","maxLength":255}},"required":["sheet","cell","target"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"target":{"type":"string"},"tooltip":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","cell","target","tooltip","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Target  string `json:"target"`
					Tooltip string `json:"tooltip"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.hyperlink.set: %w", err)
				}
				input.Sheet, input.Cell, input.Target = strings.TrimSpace(input.Sheet), strings.ToUpper(strings.TrimSpace(input.Cell)), strings.TrimSpace(input.Target)
				input.Tooltip = strings.TrimSpace(input.Tooltip)
				if input.Sheet == "" || !validSpreadsheetCellReference(input.Cell) || input.Target == "" || utf8.RuneCountInString(input.Target) > 2048 || utf8.RuneCountInString(input.Tooltip) > 255 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetHyperlinkController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetHyperlink(ctx, input.Sheet, input.Cell, input.Target, input.Tooltip, false)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Target  string `json:"target"`
					Tooltip string `json:"tooltip"`
					Changed bool   `json:"changed"`
				}{input.Sheet, input.Cell, input.Target, input.Tooltip, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.duplicates.remove", Name: "删除范围内重复行",
				Description: "按指定键列或整行内容查找重复项。此格式适配器会删除工作表整行，可能影响选择区域之外的列；Agent 调用需人类确认。范围最多 100000 个单元格。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"key_column":{"type":"integer","minimum":0,"maximum":16384}},"required":["sheet","bounds","key_column"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object"},"key_column":{"type":"integer"},"removed_count":{"type":"integer","minimum":0},"remaining_count":{"type":"integer","minimum":0}},"required":["sheet","bounds","key_column","removed_count","remaining_count"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					KeyColumn int                `json:"key_column"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.duplicates.remove: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || !validCellRange(input.Bounds, 100_000) || input.KeyColumn < 0 || (input.KeyColumn > 0 && (input.KeyColumn < input.Bounds.StartColumn || input.KeyColumn > input.Bounds.EndColumn)) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetDuplicatesController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				result, err := controller.RemoveSpreadsheetDuplicates(ctx, input.Sheet, input.Bounds, input.KeyColumn)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet          string             `json:"sheet"`
					Bounds         document.CellRange `json:"bounds"`
					KeyColumn      int                `json:"key_column"`
					RemovedCount   int                `json:"removed_count"`
					RemainingCount int                `json:"remaining_count"`
				}{input.Sheet, input.Bounds, input.KeyColumn, result.RemovedCount, result.RemainingCount}, result.RemovedCount > 0)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.panes.get", Name: "读取工作表冻结窗格",
				Description: "读取明确工作表当前冻结状态、冻结行数和列数。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1}},"required":["sheet"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"frozen":{"type":"boolean"},"x_split":{"type":"integer","minimum":0},"y_split":{"type":"integer","minimum":0}},"required":["sheet","frozen","x_split","y_split"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.panes.get: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetPanesController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				panes, err := controller.GetSpreadsheetPanes(ctx, input.Sheet)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet  string `json:"sheet"`
					Frozen bool   `json:"frozen"`
					XSplit int    `json:"x_split"`
					YSplit int    `json:"y_split"`
				}{input.Sheet, panes.Frozen, panes.XSplit, panes.YSplit}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.panes.set", Name: "设置工作表冻结窗格",
				Description: "按明确行列冻结位置调整或取消冻结窗格。freeze_rows 冻结顶端行，freeze_columns 冻结左侧列，freeze_at 按行列数冻结，unfreeze 清除冻结。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"operation":{"type":"string","enum":["freeze_rows","freeze_columns","freeze_at","unfreeze"]},"rows":{"type":"integer","minimum":0,"maximum":1048575},"columns":{"type":"integer","minimum":0,"maximum":16383}},"required":["sheet","operation"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"operation":{"type":"string"},"frozen":{"type":"boolean"},"x_split":{"type":"integer","minimum":0},"y_split":{"type":"integer","minimum":0},"changed":{"type":"boolean"}},"required":["sheet","operation","frozen","x_split","y_split","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet     string `json:"sheet"`
					Operation string `json:"operation"`
					Rows      int    `json:"rows"`
					Columns   int    `json:"columns"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.panes.set: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				panes := document.SpreadsheetPaneFreeze{}
				switch input.Operation {
				case "freeze_rows":
					if input.Rows < 1 || input.Columns != 0 {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					panes = document.SpreadsheetPaneFreeze{Frozen: true, YSplit: input.Rows}
				case "freeze_columns":
					if input.Columns < 1 || input.Rows != 0 {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					panes = document.SpreadsheetPaneFreeze{Frozen: true, XSplit: input.Columns}
				case "freeze_at":
					if input.Rows < 0 || input.Columns < 0 || (input.Rows == 0 && input.Columns == 0) {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					panes = document.SpreadsheetPaneFreeze{Frozen: true, XSplit: input.Columns, YSplit: input.Rows}
				case "unfreeze":
					if input.Rows != 0 || input.Columns != 0 {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				default:
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetPanesController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetPanes(ctx, input.Sheet, panes)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet     string `json:"sheet"`
					Operation string `json:"operation"`
					Frozen    bool   `json:"frozen"`
					XSplit    int    `json:"x_split"`
					YSplit    int    `json:"y_split"`
					Changed   bool   `json:"changed"`
				}{input.Sheet, input.Operation, panes.Frozen, panes.XSplit, panes.YSplit, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.view.get", Name: "读取工作表视图",
				Description: "读取明确工作表保存的显示模式、标尺和缩放比例。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1}},"required":["sheet"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"view_mode":{"type":"string","enum":["normal","pageBreakPreview","pageLayout"]},"show_ruler":{"type":"boolean"},"zoom_percent":{"type":"integer","minimum":10,"maximum":400}},"required":["sheet","view_mode","show_ruler","zoom_percent"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.view.get: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetViewController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				view, err := controller.GetSpreadsheetView(ctx, input.Sheet)
				if err != nil {
					return CapabilityResult{}, err
				}
				if view.ViewMode == "" {
					view.ViewMode = "normal"
				}
				zoomPercent := view.ZoomPercent
				if zoomPercent < 10 || zoomPercent > 400 {
					zoomPercent = 100
				}
				return capabilityJSONResult(struct {
					Sheet       string `json:"sheet"`
					ViewMode    string `json:"view_mode"`
					ShowRuler   bool   `json:"show_ruler"`
					ZoomPercent int    `json:"zoom_percent"`
				}{input.Sheet, view.ViewMode, view.ShowRuler, zoomPercent}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.view.mode.set", Name: "设置工作表显示模式",
				Description: "将明确工作表设为普通视图或分页预览，并保存该工作表的视图设置；不会修改单元格内容。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"mode":{"type":"string","enum":["normal","pageBreakPreview"]}},"required":["sheet","mode"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"mode":{"type":"string","enum":["normal","pageBreakPreview"]},"changed":{"type":"boolean"}},"required":["sheet","mode","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					Mode  string `json:"mode"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.view.mode.set: %w", err)
				}
				input.Sheet, input.Mode = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.Mode)
				if input.Sheet == "" || (input.Mode != "normal" && input.Mode != "pageBreakPreview") {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetViewController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetViewMode(ctx, input.Sheet, input.Mode)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string `json:"sheet"`
					Mode    string `json:"mode"`
					Changed bool   `json:"changed"`
				}{input.Sheet, input.Mode, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.view.ruler.set", Name: "设置工作表标尺",
				Description: "显示或隐藏明确工作表的标尺，并保存该工作表的视图设置；不会修改单元格内容。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"show_ruler":{"type":"boolean"}},"required":["sheet","show_ruler"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"show_ruler":{"type":"boolean"},"changed":{"type":"boolean"}},"required":["sheet","show_ruler","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet     string `json:"sheet"`
					ShowRuler bool   `json:"show_ruler"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.view.ruler.set: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetViewController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetViewRuler(ctx, input.Sheet, input.ShowRuler)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet     string `json:"sheet"`
					ShowRuler bool   `json:"show_ruler"`
					Changed   bool   `json:"changed"`
				}{input.Sheet, input.ShowRuler, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.text_to_columns.split", Name: "按分隔符拆分单元格内容",
				Description: "按明确分隔符拆分指定列中的值，并从源列向右写入结果。相邻列内容可能被覆盖；Agent 调用需人类确认。最多处理 100000 个单元格。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"key_column":{"type":"integer","minimum":1,"maximum":16384},"delimiter":{"type":"string","minLength":1,"maxLength":256,"description":"拆分分隔符，最多 256 个 Unicode 字符。"}},"required":["sheet","bounds","key_column","delimiter"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object"},"key_column":{"type":"integer"},"rows_split":{"type":"integer","minimum":0},"max_columns":{"type":"integer","minimum":0}},"required":["sheet","bounds","key_column","rows_split","max_columns"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					KeyColumn int                `json:"key_column"`
					Delimiter string             `json:"delimiter"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.text_to_columns.split: %w", err)
				}
				input.Sheet, input.Delimiter = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.Delimiter)
				if input.Sheet == "" || !validCellRange(input.Bounds, 100_000) || input.KeyColumn < input.Bounds.StartColumn || input.KeyColumn > input.Bounds.EndColumn || input.Delimiter == "" || utf8.RuneCountInString(input.Delimiter) > 256 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetTextToColumnsController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				result, err := controller.SplitSpreadsheetTextToColumns(ctx, input.Sheet, input.Bounds, input.KeyColumn, input.Delimiter)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					KeyColumn int                `json:"key_column"`
					RowsSplit int                `json:"rows_split"`
					MaxCols   int                `json:"max_columns"`
				}{input.Sheet, input.Bounds, input.KeyColumn, result.RowsSplit, result.MaxCols}, result.RowsSplit > 0)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.conditional_format.set", Name: "设置或清除区域条件格式",
				Description: "为明确工作表范围设置数据条、三色色阶、重复值或高于平均值规则；operation=clear 时清除此范围的条件格式。范围最多 1000000 个单元格。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"operation":{"type":"string","enum":["set","clear"]},"type":{"type":"string","enum":["data_bar","3_color_scale","duplicate","above_average"]}},"required":["sheet","bounds","operation"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object"},"operation":{"type":"string"},"type":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","bounds","operation","type","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					Operation string             `json:"operation"`
					Type      string             `json:"type"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.conditional_format.set: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || !validCellRange(input.Bounds, 1_000_000) || (input.Operation != "set" && input.Operation != "clear") {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				clear := input.Operation == "clear"
				validTypes := map[string]document.SpreadsheetConditionalFormatType{
					"data_bar":      document.ConditionalFormatDataBar,
					"3_color_scale": document.ConditionalFormat3ColorScale,
					"duplicate":     document.ConditionalFormatDuplicate,
					"above_average": document.ConditionalFormatAboveAvg,
				}
				ruleType, typeOK := validTypes[input.Type]
				if (clear && input.Type != "") || (!clear && !typeOK) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetConditionalFormatController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetConditionalFormat(ctx, document.SpreadsheetConditionalFormatSpec{Sheet: input.Sheet, Range: input.Bounds, Type: ruleType}, clear)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					Operation string             `json:"operation"`
					Type      string             `json:"type"`
					Changed   bool               `json:"changed"`
				}{input.Sheet, input.Bounds, input.Operation, input.Type, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.data_validation.set", Name: "设置或清除单元格数据验证",
				Description: "为明确工作表范围添加下拉列表或数字范围验证，或清除验证。下拉列表最多 100 项；范围最多 1000000 个单元格。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"operation":{"type":"string","enum":["list","number","clear"]},"items":{"type":"array","maxItems":100,"items":{"type":"string","minLength":1,"maxLength":255}},"minimum":{"type":"number"},"maximum":{"type":"number"}},"required":["sheet","bounds","operation"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object"},"operation":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","bounds","operation","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					Operation string             `json:"operation"`
					Items     []string           `json:"items"`
					Minimum   *float64           `json:"minimum"`
					Maximum   *float64           `json:"maximum"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.data_validation.set: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || !validCellRange(input.Bounds, 1_000_000) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				spec := document.SpreadsheetDataValidationSpec{Sheet: input.Sheet, Range: input.Bounds}
				clear := input.Operation == "clear"
				switch input.Operation {
				case "list":
					if len(input.Items) == 0 || len(input.Items) > 100 || input.Minimum != nil || input.Maximum != nil {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					for _, item := range input.Items {
						if strings.TrimSpace(item) == "" || len(item) > 255 {
							return CapabilityResult{}, ErrInvalidCommandArguments
						}
					}
					spec.Type, spec.DropList = document.DataValidationTypeList, input.Items
				case "number":
					if len(input.Items) != 0 || (input.Minimum == nil && input.Maximum == nil) || (input.Minimum != nil && input.Maximum != nil && *input.Minimum > *input.Maximum) {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					spec.Type, spec.MinNumber, spec.MaxNumber = document.DataValidationTypeNumber, input.Minimum, input.Maximum
				case "clear":
					if len(input.Items) != 0 || input.Minimum != nil || input.Maximum != nil {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				default:
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetDataValidationController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetDataValidation(ctx, spec, clear)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					Operation string             `json:"operation"`
					Changed   bool               `json:"changed"`
				}{input.Sheet, input.Bounds, input.Operation, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.table.create", Name: "创建结构化表格",
				Description: "在明确工作表范围创建带筛选器和行列条纹的表格，至少包含标题行和一行数据，最多 1000000 个单元格。空白或重复的标题单元格会被自动改成唯一名称。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"name":{"type":"string","maxLength":255},"style_name":{"type":"string","enum":["TableStyleLight1","TableStyleMedium2","TableStyleDark1"]}},"required":["sheet","bounds"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object"},"name":{"type":"string"},"style_name":{"type":"string"},"changed":{"type":"boolean"}},"required":["sheet","bounds","name","style_name","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					Name      string             `json:"name"`
					StyleName string             `json:"style_name"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.table.create: %w", err)
				}
				input.Sheet, input.Name = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.Name)
				if input.Sheet == "" || !validCellRange(input.Bounds, 1_000_000) || input.Bounds.EndRow <= input.Bounds.StartRow || len(input.Name) > 255 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.Name != "" && !validSpreadsheetTableName(input.Name) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.StyleName != "" && input.StyleName != "TableStyleLight1" && input.StyleName != "TableStyleMedium2" && input.StyleName != "TableStyleDark1" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetTableController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				spec := document.SpreadsheetTableSpec{Sheet: input.Sheet, Range: input.Bounds, Name: input.Name, StyleName: input.StyleName}
				if err := controller.CreateSpreadsheetTable(ctx, spec); err != nil {
					return CapabilityResult{}, err
				}
				if spec.StyleName == "" {
					spec.StyleName = "TableStyleMedium2"
				}
				return capabilityJSONResult(struct {
					Sheet     string             `json:"sheet"`
					Bounds    document.CellRange `json:"bounds"`
					Name      string             `json:"name"`
					StyleName string             `json:"style_name"`
					Changed   bool               `json:"changed"`
				}{spec.Sheet, spec.Range, spec.Name, spec.StyleName, true}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.defined_names.get", Name: "读取工作簿定义名称",
				Description: "列出工作簿中的已定义名称、引用位置、作用范围和说明。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object"}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"names":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"ref":{"type":"string"},"scope":{"type":"string"},"comment":{"type":"string"}},"required":["name","ref","scope","comment"],"additionalProperties":false}}},"required":["names"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				controller, ok := capability.Adapter.(SpreadsheetDefinedNameController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				names, err := controller.GetSpreadsheetDefinedNames(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				if names == nil {
					names = []document.SpreadsheetDefinedName{}
				}
				return capabilityJSONResult(struct {
					Names []document.SpreadsheetDefinedName `json:"names"`
				}{names}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.defined_names.set", Name: "新增或更新工作簿定义名称",
				Description: "新增或更新工作簿定义名称。name、scope 和 comment 各最多 255 个 Unicode 字符，ref 最多 4096 个 Unicode 字符且必须指向明确的单元格或范围；scope 留空表示工作簿级名称。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","minLength":1,"maxLength":255,"description":"定义名称，最多 255 个 Unicode 字符。"},"ref":{"type":"string","minLength":1,"maxLength":4096,"description":"单元格或范围引用，最多 4096 个 Unicode 字符。"},"scope":{"type":"string","maxLength":255,"description":"可选工作表名称，最多 255 个 Unicode 字符；留空表示工作簿级名称。"},"comment":{"type":"string","maxLength":255,"description":"可选说明，最多 255 个 Unicode 字符。"}},"required":["name","ref"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"ref":{"type":"string"},"scope":{"type":"string"},"comment":{"type":"string"},"changed":{"type":"boolean"}},"required":["name","ref","scope","comment","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var name document.SpreadsheetDefinedName
				if err := json.Unmarshal(raw, &name); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.defined_names.set: %w", err)
				}
				name.Name, name.Ref, name.Scope = strings.TrimSpace(name.Name), strings.TrimSpace(name.Ref), strings.TrimSpace(name.Scope)
				if !validDefinedName(name.Name) || name.Ref == "" || utf8.RuneCountInString(name.Ref) > 4096 || utf8.RuneCountInString(name.Scope) > 255 || utf8.RuneCountInString(name.Comment) > 255 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetDefinedNameController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetDefinedName(ctx, name)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					document.SpreadsheetDefinedName
					Changed bool `json:"changed"`
				}{name, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.defined_names.delete", Name: "删除工作簿定义名称",
				Description: "删除明确名称和作用范围的定义名称。删除后，引用该名称的公式可能失效；执行前应检查其引用关系。Agent 调用需人类确认。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","minLength":1,"maxLength":255},"scope":{"type":"string","maxLength":255}},"required":["name"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"scope":{"type":"string"},"changed":{"type":"boolean"}},"required":["name","scope","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Name  string `json:"name"`
					Scope string `json:"scope"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.defined_names.delete: %w", err)
				}
				input.Name, input.Scope = strings.TrimSpace(input.Name), strings.TrimSpace(input.Scope)
				if !validDefinedName(input.Name) || utf8.RuneCountInString(input.Scope) > 255 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetDefinedNameController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.DeleteSpreadsheetDefinedName(ctx, input.Name, input.Scope)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Name    string `json:"name"`
					Scope   string `json:"scope"`
					Changed bool   `json:"changed"`
				}{input.Name, input.Scope, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.gridlines.get", Name: "读取工作表网格线设置",
				Description: "读取指定工作表是否显示网格线。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1}},"required":["sheet"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"show":{"type":"boolean"}},"required":["sheet","show"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.gridlines.get: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				reader, ok := capability.Adapter.(SpreadsheetGridlinesController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				show, err := reader.GetSpreadsheetGridlines(ctx, input.Sheet)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet string `json:"sheet"`
					Show  bool   `json:"show"`
				}{input.Sheet, show}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.page_layout.get", Name: "读取工作表页面布局",
				Description: "读取指定工作表的打印方向、纸张、页边距、打印区域和标题行设置。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1}},"required":["sheet"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"layout":{"type":"object","properties":{"orientation":{"type":"string"},"paper_size":{"type":"integer"},"margin_top":{"type":"number"},"margin_right":{"type":"number"},"margin_bottom":{"type":"number"},"margin_left":{"type":"number"},"print_area":{"type":"string"},"print_titles":{"type":"string"}},"required":["orientation","paper_size","margin_top","margin_right","margin_bottom","margin_left","print_area","print_titles"],"additionalProperties":false}},"required":["sheet","layout"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.page_layout.get: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				reader, ok := capability.Adapter.(SpreadsheetPageLayoutController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				layout, err := reader.GetSpreadsheetPageLayout(ctx, input.Sheet)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet  string                         `json:"sheet"`
					Layout document.SpreadsheetPageLayout `json:"layout"`
				}{input.Sheet, layout}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.page_layout.set", Name: "设置工作表页面布局",
				Description: "设置一个明确的工作表打印属性。operation 支持 orientation、margins、paper_size、print_area、print_titles；清除打印区域或标题行时传入空字符串。页边距单位为英寸。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"operation":{"type":"string","enum":["orientation","margins","paper_size","print_area","print_titles"]},"orientation":{"type":"string","enum":["portrait","landscape"]},"paper_size":{"type":"integer","enum":[1,5,8,9]},"margin_top":{"type":"number","minimum":0,"maximum":20},"margin_right":{"type":"number","minimum":0,"maximum":20},"margin_bottom":{"type":"number","minimum":0,"maximum":20},"margin_left":{"type":"number","minimum":0,"maximum":20},"print_area":{"type":"string","maxLength":512},"print_titles":{"type":"string","maxLength":512}},"required":["sheet","operation"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"changed":{"type":"boolean"},"layout":{"type":"object","properties":{"orientation":{"type":"string"},"paper_size":{"type":"integer"},"margin_top":{"type":"number"},"margin_right":{"type":"number"},"margin_bottom":{"type":"number"},"margin_left":{"type":"number"},"print_area":{"type":"string"},"print_titles":{"type":"string"}},"required":["orientation","paper_size","margin_top","margin_right","margin_bottom","margin_left","print_area","print_titles"],"additionalProperties":false}},"required":["sheet","changed","layout"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					SpreadsheetPageLayoutPatch
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.page_layout.set: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetPageLayoutController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				layout, changed, err := controller.SetSpreadsheetPageLayout(ctx, input.Sheet, input.SpreadsheetPageLayoutPatch)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string                         `json:"sheet"`
					Changed bool                           `json:"changed"`
					Layout  document.SpreadsheetPageLayout `json:"layout"`
				}{input.Sheet, changed, layout}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.gridlines.set", Name: "设置工作表网格线显示",
				Description: "为明确指定的工作表显示或隐藏网格线。show 为 true 时显示，false 时隐藏。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"show":{"type":"boolean"}},"required":["sheet","show"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"show":{"type":"boolean"},"changed":{"type":"boolean"}},"required":["sheet","show","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					Show  *bool  `json:"show"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.gridlines.set: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || input.Show == nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetGridlinesController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.SetSpreadsheetGridlines(ctx, input.Sheet, *input.Show)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Sheet   string `json:"sheet"`
					Show    bool   `json:"show"`
					Changed bool   `json:"changed"`
				}{input.Sheet, *input.Show, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.range.copy", Name: "复制单元格区域内容",
				Description: "读取明确工作表区域中的单元格内容，矩阵最多 10000 个单元格。返回兼容 TSV 的显示值，以及供同一工作簿内复制/粘贴保留的公式、格式和类型信息。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false}},"required":["sheet","bounds"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"bounds":{"type":"object"},"rows":{"type":"array","items":{"type":"array","items":{"type":"string"}}},"cells":{"type":"array","items":{"type":"array","items":{"type":"object"}}}},"required":["sheet","bounds","rows","cells"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet  string             `json:"sheet"`
					Bounds document.CellRange `json:"bounds"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.range.copy: %w", err)
				}
				if strings.TrimSpace(input.Sheet) == "" || !validCellRange(input.Bounds, 10_000) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				reader, ok := capability.Adapter.(SpreadsheetRangeReaderCapability)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				cells, err := reader.ReadSpreadsheetRange(ctx, input.Sheet, input.Bounds)
				if err != nil {
					return CapabilityResult{}, err
				}
				rows := make([][]string, len(cells))
				for rowIndex, row := range cells {
					rows[rowIndex] = make([]string, len(row))
					for columnIndex, cell := range row {
						rows[rowIndex][columnIndex] = cell.Value
					}
				}
				return capabilityJSONResult(struct {
					Sheet  string             `json:"sheet"`
					Bounds document.CellRange `json:"bounds"`
					Rows   [][]string         `json:"rows"`
					Cells  [][]document.Cell `json:"cells"`
				}{input.Sheet, input.Bounds, rows, cells}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.structure.edit", Name: "调整工作表结构",
				Description: "创建、重命名或删除工作表，或在明确位置插入/删除行列。所有结构修改均需人类确认；Agent 必须提供操作类型和目标，行列操作还需提供起始索引与数量。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskHigh,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["create_sheet","rename_sheet","delete_sheet","insert_rows","delete_rows","insert_columns","delete_columns"]},"sheet":{"type":"string"},"new_name":{"type":"string","maxLength":31},"index":{"type":"integer","minimum":1,"maximum":1048576},"count":{"type":"integer","minimum":1,"maximum":1048576}},"required":["operation"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"operation":{"type":"string"},"sheet":{"type":"string"},"new_name":{"type":"string"},"index":{"type":"integer"},"count":{"type":"integer"}},"required":["changed","operation","sheet","new_name","index","count"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Operation string `json:"operation"`
					Sheet     string `json:"sheet"`
					NewName   string `json:"new_name"`
					Index     int    `json:"index"`
					Count     int    `json:"count"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.structure.edit: %w", err)
				}
				kinds := map[string]document.StructureEditKind{
					"insert_rows": document.StructureInsertRows, "delete_rows": document.StructureDeleteRows,
					"insert_columns": document.StructureInsertColumns, "delete_columns": document.StructureDeleteColumns,
					"create_sheet": document.StructureCreateSheet, "rename_sheet": document.StructureRenameSheet,
					"delete_sheet": document.StructureDeleteSheet,
				}
				kind, ok := kinds[input.Operation]
				if !ok || (kind != document.StructureCreateSheet && strings.TrimSpace(input.Sheet) == "") {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if kind == document.StructureInsertRows || kind == document.StructureDeleteRows || kind == document.StructureInsertColumns || kind == document.StructureDeleteColumns {
					if input.Index < 1 || input.Count < 1 || input.Index > 1_048_576 || input.Count > 1_048_576 {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				}
				controller, ok := capability.Adapter.(SpreadsheetStructureController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				edit := document.StructureEdit{Kind: kind, Sheet: input.Sheet, NewName: input.NewName, Index: input.Index, Count: input.Count}
				changed, err := controller.ApplySpreadsheetStructure(ctx, edit)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					Operation string `json:"operation"`
					Sheet     string `json:"sheet"`
					NewName   string `json:"new_name"`
					Index     int    `json:"index"`
					Count     int    `json:"count"`
				}{changed, input.Operation, input.Sheet, input.NewName, input.Index, input.Count}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.range.search", Name: "查找工作表内容",
				Description: "在明确指定的工作表中查找文本，忽略大小写，最多返回 10000 个单元格匹配并标记结果是否截断。可指定 max_results 限制返回数量。",
				Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"query":{"type":"string","minLength":1,"maxLength":256,"description":"要查找的文本，最多 256 个 Unicode 字符。"},"max_results":{"type":"integer","minimum":1,"maximum":10000}},"required":["sheet","query"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"matches":{"type":"array","items":{"type":"object","properties":{"row":{"type":"integer"},"column":{"type":"integer"},"value":{"type":"string"}},"required":["row","column","value"],"additionalProperties":false}},"truncated":{"type":"boolean"}},"required":["matches","truncated"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet      string `json:"sheet"`
					Query      string `json:"query"`
					MaxResults int    `json:"max_results"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.range.search: %w", err)
				}
				input.Sheet, input.Query = strings.TrimSpace(input.Sheet), strings.TrimSpace(input.Query)
				if input.Sheet == "" || input.Query == "" || utf8.RuneCountInString(input.Query) > 256 || input.MaxResults < 0 || input.MaxResults > 10000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.MaxResults == 0 {
					input.MaxResults = 100
				}
				searcher, ok := capability.Adapter.(SpreadsheetSearchCapability)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				matches, truncated, err := searcher.FindSpreadsheetCells(ctx, input.Sheet, input.Query, input.MaxResults)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Matches   []document.CellMatch `json:"matches"`
					Truncated bool                 `json:"truncated"`
				}{matches, truncated}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.formula.set", Name: "设置单元格公式",
				Description: "为明确指定的工作表和单元格设置公式。formula 可带或不带开头的等号，含可选等号最多 8192 个 UTF-16 单元（补充平面字符按 2 个单元计）；此操作只写入公式，不承诺公式引擎已完成重算。请先查询目标单元格并使用当前文档修订号。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"cell":{"type":"string","minLength":1},"formula":{"type":"string","minLength":1,"maxLength":8192,"description":"公式字符串含可选开头等号最多 8192 个 UTF-16 单元；补充平面字符按 2 个单元计。"}},"required":["sheet","cell","formula"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"cell":{"type":"string"},"formula":{"type":"string"}},"required":["changed","sheet","cell","formula"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Formula string `json:"formula"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.formula.set: %w", err)
				}
				if strings.TrimSpace(input.Sheet) == "" || strings.TrimSpace(input.Cell) == "" || strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input.Formula), "=")) == "" || !withinXLSXFormulaLimit(input.Formula) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(SpreadsheetCellFormulaSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetSpreadsheetCellFormula(ctx, input.Sheet, input.Cell, input.Formula)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					Sheet   string `json:"sheet"`
					Cell    string `json:"cell"`
					Formula string `json:"formula"`
				}{changed, input.Sheet, input.Cell, input.Formula}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.formulas.set", Name: "批量设置单元格公式",
				Description: "在同一工作表中一次写入 1 至 10000 个明确坐标的公式；每条公式含可选开头等号最多 8192 个 UTF-16 单元。整个批次是一个可撤销操作。此操作只写入公式，不承诺本地公式引擎已完成重算。请先查询目标单元格并使用当前文档修订号。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"edits":{"type":"array","minItems":1,"maxItems":10000,"items":{"type":"object","properties":{"row":{"type":"integer","minimum":1,"maximum":1048576},"column":{"type":"integer","minimum":1,"maximum":16384},"formula":{"type":"string","minLength":1,"maxLength":8192,"description":"公式字符串含可选开头等号最多 8192 个 UTF-16 单元。"}},"required":["row","column","formula"],"additionalProperties":false}}},"required":["sheet","edits"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"requested_count":{"type":"integer","minimum":0}},"required":["changed","sheet","requested_count"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string                     `json:"sheet"`
					Edits []document.CellFormulaEdit `json:"edits"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.formulas.set: %w", err)
				}
				input.Sheet = strings.TrimSpace(input.Sheet)
				if input.Sheet == "" || len(input.Edits) == 0 || len(input.Edits) > 10_000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				seen := make(map[[2]int]struct{}, len(input.Edits))
				for _, edit := range input.Edits {
					if edit.Row < 1 || edit.Row > 1_048_576 || edit.Column < 1 || edit.Column > 16_384 || strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(edit.Formula), "=")) == "" || !withinXLSXFormulaLimit(edit.Formula) {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					key := [2]int{edit.Row, edit.Column}
					if _, duplicate := seen[key]; duplicate {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					seen[key] = struct{}{}
				}
				setter, ok := capability.Adapter.(SpreadsheetFormulaBatchSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetSpreadsheetFormulas(ctx, input.Sheet, input.Edits)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed        bool   `json:"changed"`
					Sheet          string `json:"sheet"`
					RequestedCount int    `json:"requested_count"`
				}{changed, input.Sheet, len(input.Edits)}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.cells.set", Name: "批量设置单元格内容",
				Description: "在一个工作表中一次写入 1 至 10000 个明确坐标的单元格值，支持文本、数字和布尔值；字符串按文本原样保存，例如 \"00123\" 保持前导零。单个公式调用 spreadsheet.formula.set，公式批次调用 spreadsheet.formulas.set；不接受重复坐标。适合 Agent 批量整理数据，调用前先查询并使用当前文档修订号。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"edits":{"type":"array","minItems":1,"maxItems":10000,"items":{"type":"object","properties":{"row":{"type":"integer","minimum":1,"maximum":1048576},"column":{"type":"integer","minimum":1,"maximum":16384},"value":{"anyOf":[{"type":"string","maxLength":32767},{"type":"number"},{"type":"boolean"}]}},"required":["row","column","value"],"additionalProperties":false}}},"required":["sheet","edits"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"requested_count":{"type":"integer","minimum":0}},"required":["changed","sheet","requested_count"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string `json:"sheet"`
					Edits []struct {
						Row    int             `json:"row"`
						Column int             `json:"column"`
						Value  json.RawMessage `json:"value"`
					} `json:"edits"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.cells.set: %w", err)
				}
				if strings.TrimSpace(input.Sheet) == "" || len(input.Edits) == 0 || len(input.Edits) > 10000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				seen := make(map[[2]int]struct{}, len(input.Edits))
				edits := make([]document.CellEdit, 0, len(input.Edits))
				for _, inputEdit := range input.Edits {
					value, err := decodeSpreadsheetCellValue(inputEdit.Value)
					if err != nil || inputEdit.Row < 1 || inputEdit.Row > 1_048_576 || inputEdit.Column < 1 || inputEdit.Column > 16_384 || (value.Kind == document.CellValueText && strings.HasPrefix(strings.TrimSpace(value.Text), "=")) {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					coordinate := [2]int{inputEdit.Row, inputEdit.Column}
					if _, duplicate := seen[coordinate]; duplicate {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					seen[coordinate] = struct{}{}
					edits = append(edits, document.CellEdit{Row: inputEdit.Row, Column: inputEdit.Column, Value: value.Text, TypedValue: &value})
				}
				setter, ok := capability.Adapter.(SpreadsheetCellsSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetSpreadsheetCells(ctx, input.Sheet, edits)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed        bool   `json:"changed"`
					Sheet          string `json:"sheet"`
					RequestedCount int    `json:"requested_count"`
				}{changed, input.Sheet, len(input.Edits)}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.range.format.set", Name: "设置单元格区域格式",
				Description: "为明确指定的工作表区域设置受支持的字体、字号步进、颜色、水平和垂直对齐、数字格式、小数位、边框或换行属性。普通区域最多影响 10000 个单元格；另支持整行、整列和全表格式。未提供的属性保持不变。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"patch":{"type":"object","properties":{"bold":{"type":"boolean"},"italic":{"type":"boolean"},"underline":{"type":"boolean"},"font_color":{"type":"string"},"fill_color":{"type":"string"},"horizontal_alignment":{"type":"string","enum":["general","left","center","right"]},"vertical_alignment":{"type":"string","enum":["top","center","bottom"]},"number_format":{"type":"string","enum":["General","0","0.00","0%","0.00%","yyyy-mm-dd","\"¥\"#,##0.00"]},"decimal_places_delta":{"type":"integer","enum":[-1,1]},"border_preset":{"type":"string","enum":["all","outside","bottom","none"]},"wrap_text":{"type":"boolean"},"font_size":{"type":"number","exclusiveMinimum":0,"maximum":409},"font_size_step":{"type":"integer","enum":[-1,1]},"font_family":{"type":"string"},"clear_format":{"type":"boolean"}},"additionalProperties":false}},"required":["sheet","bounds","patch"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"bounds":{"type":"object"}},"required":["changed","sheet","bounds"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet  string                   `json:"sheet"`
					Bounds document.CellRange       `json:"bounds"`
					Patch  document.CellFormatPatch `json:"patch"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.range.format.set: %w", err)
				}
				if !validSpreadsheetFormatRange(input.Bounds) || strings.TrimSpace(input.Sheet) == "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				formatter, ok := capability.Adapter.(SpreadsheetRangeFormatter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := formatter.SetSpreadsheetCellFormats(ctx, input.Sheet, input.Bounds, input.Patch)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool               `json:"changed"`
					Sheet   string             `json:"sheet"`
					Bounds  document.CellRange `json:"bounds"`
				}{changed, input.Sheet, input.Bounds}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.range.sort", Name: "排序单元格区域",
				Description: "按指定区域内的键列稳定排序，并随行移动区域中的单元格值和格式。区域最多 10000 个单元格；当前实现拒绝包含公式或合并单元格的区域。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"key_column":{"type":"integer","minimum":1,"maximum":16384},"descending":{"type":"boolean"}},"required":["sheet","bounds","key_column","descending"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"bounds":{"type":"object"},"key_column":{"type":"integer"},"descending":{"type":"boolean"}},"required":["changed","sheet","bounds","key_column","descending"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var request document.SortRequest
				if err := json.Unmarshal(raw, &request); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.range.sort: %w", err)
				}
				if strings.TrimSpace(request.Sheet) == "" || !validCellRange(request.Bounds, 10_000) || request.KeyColumn < request.Bounds.StartColumn || request.KeyColumn > request.Bounds.EndColumn {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				sorter, ok := capability.Adapter.(SpreadsheetRangeSorter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := sorter.SortSpreadsheetRange(ctx, request)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed    bool               `json:"changed"`
					Sheet      string             `json:"sheet"`
					Bounds     document.CellRange `json:"bounds"`
					KeyColumn  int                `json:"key_column"`
					Descending bool               `json:"descending"`
				}{changed, request.Sheet, request.Bounds, request.KeyColumn, request.Descending}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.history.apply", Name: "撤销或重做表格修改",
				Description: "对明确文档会话执行一次撤销或重做。只恢复该工作簿历史中记录的编辑；调用前查询当前文档修订并传入 action。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["undo","redo"]}},"required":["action"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"action":{"type":"string","enum":["undo","redo"]}},"required":["changed","action"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Action string `json:"action"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.Action != "undo" && input.Action != "redo" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(SpreadsheetHistoryController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := controller.ApplySpreadsheetHistory(ctx, input.Action == "undo")
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					Action  string `json:"action"`
				}{changed, input.Action}, changed)
			},
		},
	}
	registrations = append(registrations, spreadsheetFormatPainterRegistration())
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

func validCellRange(bounds document.CellRange, maxCells int) bool {
	if bounds.StartRow < 1 || bounds.StartColumn < 1 || bounds.EndRow < bounds.StartRow || bounds.EndColumn < bounds.StartColumn || bounds.EndRow > 1_048_576 || bounds.EndColumn > 16_384 {
		return false
	}
	rows, columns := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	return rows <= maxCells && columns <= maxCells && rows*columns <= maxCells
}

func validSpreadsheetTableName(name string) bool {
	if name == "" {
		return false
	}
	for index, char := range name {
		if index == 0 {
			if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_' || char == '\\') {
				return false
			}
			continue
		}
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '.' || char == '\\') {
			return false
		}
	}
	return true
}

func validDefinedName(name string) bool {
	if name == "" {
		return false
	}
	for index, char := range name {
		if index == 0 {
			if !(unicode.IsLetter(char) || char == '_' || char == '\\') {
				return false
			}
			continue
		}
		if !(unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' || char == '.' || char == '\\') {
			return false
		}
	}
	return true
}

func validSpreadsheetCellReference(cell string) bool {
	if cell == "" {
		return false
	}
	index := 0
	column := 0
	for index < len(cell) && cell[index] >= 'A' && cell[index] <= 'Z' {
		column = column*26 + int(cell[index]-'A'+1)
		if column > 16_384 {
			return false
		}
		index++
	}
	if index == 0 || index == len(cell) {
		return false
	}
	row, err := strconv.Atoi(cell[index:])
	return err == nil && row >= 1 && row <= 1_048_576
}

func withinXLSXFormulaLimit(formula string) bool {
	const maxUTF16Units = 8192
	units := 0
	for _, char := range formula {
		units++
		if char > 0xFFFF {
			units++
		}
		if units > maxUTF16Units {
			return false
		}
	}
	return true
}
