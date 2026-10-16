package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/document"
)

func registerViewCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{{
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.zoom.get", Name: "读取工作表缩放",
			Description: "读取指定已附着桌面窗口中工作表的当前缩放比例。没有匹配窗口时返回 no_attached_view。",
			Category:    CapabilityQuery, Formats: []Format{FormatXLSX}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"zoom_percent":{"type":"integer","minimum":50,"maximum":200},"changed":{"type":"boolean"}},"required":["window_id","zoom_percent","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
				WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
				Revision: capability.Document.Revision, Action: "spreadsheet.zoom.get",
				Arguments: json.RawMessage(`{}`),
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			return CapabilityResult{Data: result}, nil
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.zoom.apply", Name: "调整工作表缩放",
			Description: "在指定的已附着桌面窗口中放大、恢复 100% 或设置工作表缩放比例。set 使用 50 到 200 的百分比。该操作改变窗口视图，不修改单元格内容；工作区快照与后续 XLSX 保存会保留每张表的比例。没有匹配窗口时返回 no_attached_view。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatXLSX}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["zoom_in","reset","set"]},"zoom_percent":{"type":"integer","minimum":50,"maximum":200}},"required":["operation"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"zoom_percent":{"type":"integer","minimum":50,"maximum":200},"changed":{"type":"boolean"}},"required":["window_id","zoom_percent","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Operation   string `json:"operation"`
				ZoomPercent int    `json:"zoom_percent"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.zoom.apply: %w", err)
			}
			switch input.Operation {
			case "zoom_in", "reset":
				if input.ZoomPercent != 0 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			case "set":
				if input.ZoomPercent < 50 || input.ZoomPercent > 200 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
				WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
				Revision: capability.Document.Revision, Action: "spreadsheet.zoom.apply",
				Arguments: append(json.RawMessage(nil), raw...),
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			return CapabilityResult{Data: result}, nil
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "spreadsheet.filter.apply", Name: "筛选工作表行",
			Description: "在指定的已附着桌面窗口中按单元格值隐藏不匹配的行，或清除该窗口当前筛选。范围最多 10000 个单元格；筛选只影响窗口视图，不修改工作簿数据。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatXLSX}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["set_value","set_from_row","clear"]},"sheet":{"type":"string","minLength":1},"bounds":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1,"maximum":1048576},"start_column":{"type":"integer","minimum":1,"maximum":16384},"end_row":{"type":"integer","minimum":1,"maximum":1048576},"end_column":{"type":"integer","minimum":1,"maximum":16384}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"column":{"type":"integer","minimum":1,"maximum":16384},"value":{"type":"string","maxLength":32767},"source_row":{"type":"integer","minimum":1,"maximum":1048576}},"required":["operation","sheet"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"operation":{"type":"string","enum":["set","clear"]},"sheet":{"type":"string"},"rows_visible":{"type":"integer","minimum":0},"changed":{"type":"boolean"}},"required":["window_id","operation","sheet","rows_visible","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Operation string              `json:"operation"`
				Sheet     string              `json:"sheet"`
				Bounds    *document.CellRange `json:"bounds"`
				Column    int                 `json:"column"`
				Value     *string             `json:"value"`
				SourceRow *int                `json:"source_row"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode spreadsheet.filter.apply: %w", err)
			}
			input.Sheet = strings.TrimSpace(input.Sheet)
			if input.Sheet == "" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var visibleRows []int
			var rowHeights []float64
			filterValue := ""
			var bounds document.CellRange
			switch input.Operation {
			case "clear":
				if input.Bounds != nil || input.Column != 0 || input.Value != nil || input.SourceRow != nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			case "set_value", "set_from_row":
				if input.Bounds == nil || !validCellRange(*input.Bounds, 10_000) || input.Column < input.Bounds.StartColumn || input.Column > input.Bounds.EndColumn {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.Operation == "set_value" && (input.Value == nil || input.SourceRow != nil || len([]rune(*input.Value)) > 32767) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.Operation == "set_from_row" && (input.Value != nil || input.SourceRow == nil || *input.SourceRow <= input.Bounds.StartRow || *input.SourceRow > input.Bounds.EndRow) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				bounds = *input.Bounds
				controller, ok := capability.Adapter.(SpreadsheetGridWindowController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				window, err := controller.ReadSpreadsheetGridWindow(ctx, input.Sheet, bounds)
				if err != nil {
					return CapabilityResult{}, err
				}
				columnOffset := input.Column - bounds.StartColumn
				if input.Operation == "set_value" {
					filterValue = *input.Value
				} else {
					rowOffset := *input.SourceRow - bounds.StartRow
					if rowOffset < 0 || rowOffset >= len(window.Cells) || columnOffset >= len(window.Cells[rowOffset]) {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					filterValue = window.Cells[rowOffset][columnOffset].Value
				}
				for offset := 1; offset < len(window.Cells); offset++ {
					if columnOffset >= 0 && columnOffset < len(window.Cells[offset]) && window.Cells[offset][columnOffset].Value == filterValue {
						visibleRows = append(visibleRows, bounds.StartRow+offset)
					}
				}
				rowHeights = append([]float64(nil), window.RowHeightsPoints...)
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			arguments, err := json.Marshal(struct {
				Operation        string             `json:"operation"`
				Sheet            string             `json:"sheet"`
				Bounds           document.CellRange `json:"bounds,omitempty"`
				Column           int                `json:"column,omitempty"`
				Value            string             `json:"value,omitempty"`
				VisibleRows      []int              `json:"visible_rows,omitempty"`
				RowHeightsPoints []float64          `json:"row_heights_points,omitempty"`
			}{
				Operation: "set", Sheet: input.Sheet, Bounds: bounds, Column: input.Column,
				Value: filterValue, VisibleRows: visibleRows, RowHeightsPoints: rowHeights,
			})
			if input.Operation == "clear" {
				arguments, err = json.Marshal(struct {
					Operation string `json:"operation"`
					Sheet     string `json:"sheet"`
				}{Operation: "clear", Sheet: input.Sheet})
			}
			if err != nil {
				return CapabilityResult{}, err
			}
			result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
				WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
				Revision: capability.Document.Revision, Action: "spreadsheet.filter.apply", Arguments: arguments,
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			return CapabilityResult{Data: result}, nil
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "json.view.set", Name: "设置 JSON 树视图",
			Description: "在明确附着的 Gio 窗口中全部展开、全部折叠或设置 JSON 树的展开层级。只改变窗口展示状态，不修改文档；无匹配窗口时返回 no_attached_view。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatJSON}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["expand_all","collapse_all","expand_level"]},"level":{"type":"integer","minimum":1,"maximum":3}},"required":["operation"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"operation":{"type":"string","enum":["expand_all","collapse_all","expand_level"]},"level":{"type":"integer","minimum":-1,"maximum":3},"changed":{"type":"boolean"}},"required":["window_id","operation","level","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Operation string `json:"operation"`
				Level     int    `json:"level"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			switch input.Operation {
			case "expand_all", "collapse_all":
				if input.Level != 0 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			case "expand_level":
				if input.Level < 1 || input.Level > 3 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
				WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
				Revision: capability.Document.Revision, Action: "json.view.set",
				Arguments: append(json.RawMessage(nil), raw...),
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			return CapabilityResult{Data: result}, nil
		},
	}, {
		Descriptor: CapabilityDescriptor{
			ID: "markdown.view.set", Name: "切换 Markdown 编辑视图",
			Description: "在明确附着的 Gio 窗口中切换 Markdown 编辑、预览或分屏模式。只改变窗口展示状态，不修改文档；无匹配窗口时返回 no_attached_view。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatMarkdown}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true, RequiresView: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"mode":{"type":"string","enum":["edit","preview","split"]}},"required":["mode"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"window_id":{"type":"integer","minimum":1},"mode":{"type":"string","enum":["edit","preview","split"]},"changed":{"type":"boolean"}},"required":["window_id","mode","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Mode string `json:"mode"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			switch input.Mode {
			case "edit", "preview", "split":
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			result, err := capability.Runtime.ApplyViewIntent(ctx, ViewIntent{
				WindowID: capability.View.WindowID, DocumentID: capability.Document.DocumentID,
				Revision: capability.Document.Revision, Action: "markdown.view.set",
				Arguments: append(json.RawMessage(nil), raw...),
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			return CapabilityResult{Data: result}, nil
		},
	}}
	registrations = append(registrations, wordViewCapabilityRegistrations()...)
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}
