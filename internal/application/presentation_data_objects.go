package application

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"glance/internal/document"
)

const (
	maxPresentationTableRows    = 50
	maxPresentationTableColumns = 20
	maxPresentationChartPoints  = 100
	maxPresentationChartSeries  = 10
)

type PresentationTableExecutor interface {
	AddPresentationTable(context.Context, uint32, document.PresentationRect, document.PresentationTableData) (int64, error)
}

type PresentationChartExecutor interface {
	AddPresentationChart(context.Context, uint32, document.PresentationRect, document.PresentationChartData) (int64, error)
}

func registerPresentationDataObjectCapabilities(catalog *Catalog) error {
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.table.create", Name: "在幻灯片上创建表格",
			Description: "在明确的 PPTX 幻灯片上按 point 坐标创建含明确单元格文本的表格。rows 为矩形字符串矩阵，最多 50 行、20 列和 1000 个单元格；返回稳定 element_id。Ribbon 提供 2×2、3×3、4×4 空白表格入口，并复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"x":{"type":"number","description":"表格左上角 X 坐标，单位 point。"},"y":{"type":"number","description":"表格左上角 Y 坐标，单位 point。"},"width":{"type":"number","minimum":8,"maximum":10000,"description":"表格宽度，单位 point。"},"height":{"type":"number","minimum":8,"maximum":10000,"description":"表格高度，单位 point。"},"rows":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"string","maxLength":1024}}},"first_row":{"type":"boolean","description":"是否使用内建首行样式标记。"},"band_rows":{"type":"boolean","description":"是否启用内建隔行样式标记。"}},"required":["slide_id","x","y","width","height","rows"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1},"row_count":{"type":"integer","minimum":1,"maximum":50},"column_count":{"type":"integer","minimum":1,"maximum":20},"bounds":{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number"},"height":{"type":"number"}},"required":["x","y","width","height"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["slide_id","element_id","row_count","column_count","bounds","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if len(raw) == 0 || len(raw) > MaxQueryResultBytes {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var input struct {
				SlideID  uint64     `json:"slide_id"`
				X        float64    `json:"x"`
				Y        float64    `json:"y"`
				Width    float64    `json:"width"`
				Height   float64    `json:"height"`
				Rows     [][]string `json:"rows"`
				FirstRow bool       `json:"first_row"`
				BandRows bool       `json:"band_rows"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || !validPresentationObjectBounds(input.X, input.Y, input.Width, input.Height) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if len(input.Rows) == 0 || len(input.Rows) > maxPresentationTableRows {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			columns := len(input.Rows[0])
			if columns == 0 || columns > maxPresentationTableColumns || len(input.Rows)*columns > 1000 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			for _, row := range input.Rows {
				if len(row) != columns {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				for _, value := range row {
					if len([]rune(value)) > 1024 {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				}
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationTableExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			bounds := document.PresentationRect{X: input.X, Y: input.Y, W: input.Width, H: input.Height}
			elementID, err := executor.AddPresentationTable(ctx, uint32(input.SlideID), bounds, document.PresentationTableData{
				Rows: input.Rows, FirstRow: input.FirstRow, BandRows: input.BandRows,
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			if elementID <= 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SlideID     uint32 `json:"slide_id"`
				ElementID   int64  `json:"element_id"`
				RowCount    int    `json:"row_count"`
				ColumnCount int    `json:"column_count"`
				Bounds      struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"bounds"`
				Changed bool `json:"changed"`
			}{SlideID: uint32(input.SlideID), ElementID: elementID, RowCount: len(input.Rows), ColumnCount: columns,
				Bounds: struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				}{input.X, input.Y, input.Width, input.Height}, Changed: true,
			}, true)
		},
	}); err != nil {
		return err
	}

	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.element.chart.create", Name: "在幻灯片上创建图表",
			Description: "在明确的 PPTX 幻灯片上按 point 坐标创建柱状、折线或饼图，并在嵌入式工作簿中写入明确类别和数值系列。至少一个类别和一个系列；最多 100 个类别、10 个系列及 1000 个数据点。返回稳定 element_id。Ribbon 插入入口使用明确标注的占位数据，并复用此能力。",
			Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"document.inspect 返回的稳定幻灯片 ID。"},"chart_type":{"type":"string","enum":["bar","line","pie"]},"title":{"type":"string","maxLength":256},"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number","minimum":8,"maximum":10000},"height":{"type":"number","minimum":8,"maximum":10000},"categories":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","minLength":1,"maxLength":256}},"series":{"type":"array","minItems":1,"maxItems":10,"items":{"type":"object","properties":{"name":{"type":"string","maxLength":128},"values":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"number"}}},"required":["name","values"],"additionalProperties":false}}},"required":["slide_id","chart_type","x","y","width","height","categories","series"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1},"chart_type":{"type":"string","enum":["bar","line","pie"]},"category_count":{"type":"integer","minimum":1,"maximum":100},"series_count":{"type":"integer","minimum":1,"maximum":10},"bounds":{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number"},"height":{"type":"number"}},"required":["x","y","width","height"],"additionalProperties":false},"changed":{"type":"boolean"}},"required":["slide_id","element_id","chart_type","category_count","series_count","bounds","changed"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if len(raw) == 0 || len(raw) > MaxQueryResultBytes {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var input struct {
				SlideID    uint64   `json:"slide_id"`
				ChartType  string   `json:"chart_type"`
				Title      string   `json:"title"`
				X          float64  `json:"x"`
				Y          float64  `json:"y"`
				Width      float64  `json:"width"`
				Height     float64  `json:"height"`
				Categories []string `json:"categories"`
				Series     []struct {
					Name   string    `json:"name"`
					Values []float64 `json:"values"`
				} `json:"series"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || !validPresentationObjectBounds(input.X, input.Y, input.Width, input.Height) || len([]rune(input.Title)) > 256 || len(input.Categories) == 0 || len(input.Categories) > maxPresentationChartPoints || len(input.Series) == 0 || len(input.Series) > maxPresentationChartSeries {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			switch input.ChartType {
			case "bar", "line", "pie":
			default:
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			for _, category := range input.Categories {
				if strings.TrimSpace(category) == "" || len([]rune(category)) > 256 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			}
			if len(input.Categories)*len(input.Series) > 1000 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			chart := document.PresentationChartData{
				Type: input.ChartType, Title: input.Title,
				Categories: append([]string(nil), input.Categories...),
				Series:     make([]document.PresentationChartSeries, len(input.Series)),
			}
			for index, item := range input.Series {
				if len([]rune(item.Name)) > 128 || len(item.Values) != len(input.Categories) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				values := append([]float64(nil), item.Values...)
				for _, value := range values {
					if math.IsNaN(value) || math.IsInf(value, 0) {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
				}
				chart.Series[index] = document.PresentationChartSeries{Name: item.Name, Values: values}
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			executor, ok := capability.Adapter.(PresentationChartExecutor)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			bounds := document.PresentationRect{X: input.X, Y: input.Y, W: input.Width, H: input.Height}
			elementID, err := executor.AddPresentationChart(ctx, uint32(input.SlideID), bounds, chart)
			if err != nil {
				return CapabilityResult{}, err
			}
			if elementID <= 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SlideID       uint32 `json:"slide_id"`
				ElementID     int64  `json:"element_id"`
				ChartType     string `json:"chart_type"`
				CategoryCount int    `json:"category_count"`
				SeriesCount   int    `json:"series_count"`
				Bounds        struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"bounds"`
				Changed bool `json:"changed"`
			}{SlideID: uint32(input.SlideID), ElementID: elementID, ChartType: input.ChartType,
				CategoryCount: len(input.Categories), SeriesCount: len(input.Series),
				Bounds: struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				}{input.X, input.Y, input.Width, input.Height}, Changed: true,
			}, true)
		},
	})
}

func validPresentationObjectBounds(x, y, width, height float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && !math.IsNaN(y) && !math.IsInf(y, 0) &&
		!math.IsNaN(width) && !math.IsInf(width, 0) && !math.IsNaN(height) && !math.IsInf(height, 0) &&
		width >= 8 && height >= 8 && width <= 10000 && height <= 10000 && math.Abs(x) <= 10000 && math.Abs(y) <= 10000
}
