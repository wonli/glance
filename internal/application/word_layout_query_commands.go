package application

import (
	"context"
	"encoding/json"
	"fmt"

	"glance/internal/document"
)

type WordLayoutDataReader interface {
	ReadWordLayoutData(context.Context) (document.WordLayoutData, error)
}

type wordPageSetupView struct {
	WidthPoints         float64  `json:"width_points"`
	HeightPoints        float64  `json:"height_points"`
	Orientation         string   `json:"orientation"`
	MarginTopPoints     float64  `json:"margin_top_points"`
	MarginRightPoints   float64  `json:"margin_right_points"`
	MarginBottomPoints  float64  `json:"margin_bottom_points"`
	MarginLeftPoints    float64  `json:"margin_left_points"`
	Columns             int      `json:"columns"`
	ColumnSpacingPoints float64  `json:"column_spacing_points"`
	ExplicitFields      []string `json:"explicit_fields"`
}

type wordPageSectionView struct {
	StartBlock int               `json:"start_block"`
	StartType  string            `json:"start_type"`
	PageSetup  wordPageSetupView `json:"page_setup"`
}

type wordLayoutUnsupportedView struct {
	SourceID    string `json:"source_id"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

type wordPageSetupResult struct {
	Sections    []wordPageSectionView       `json:"sections"`
	Unsupported []wordLayoutUnsupportedView `json:"unsupported"`
}

func registerWordLayoutQueryCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.page_setup.get", Name: "读取 Word 页面设置",
			Description: "读取明确 DOCX 文档各节的纸张尺寸、方向、页边距和分栏设置，长度单位为磅。explicit_fields 标出源文件中显式写入的设置；unsupported 列出当前中立视图无法完整表达的布局内容。结果最多包含 1000 节或诊断项。",
			Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"sections":{"type":"array","maxItems":1000,"items":{"type":"object","properties":{"start_block":{"type":"integer"},"start_type":{"type":"string"},"page_setup":{"type":"object","properties":{"width_points":{"type":"number"},"height_points":{"type":"number"},"orientation":{"type":"string"},"margin_top_points":{"type":"number"},"margin_right_points":{"type":"number"},"margin_bottom_points":{"type":"number"},"margin_left_points":{"type":"number"},"columns":{"type":"integer"},"column_spacing_points":{"type":"number"},"explicit_fields":{"type":"array","items":{"type":"string"}}},"required":["width_points","height_points","orientation","margin_top_points","margin_right_points","margin_bottom_points","margin_left_points","columns","column_spacing_points","explicit_fields"],"additionalProperties":false}},"required":["start_block","start_type","page_setup"],"additionalProperties":false}},"unsupported":{"type":"array","maxItems":1000,"items":{"type":"object","properties":{"source_id":{"type":"string"},"code":{"type":"string"},"description":{"type":"string"}},"required":["source_id","code","description"],"additionalProperties":false}}},"required":["sections","unsupported"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			reader, ok := capability.Adapter.(WordLayoutDataReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			layout, err := reader.ReadWordLayoutData(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(layout.Sections) > MaxQueryItems || len(layout.Unsupported) > MaxQueryItems {
				return CapabilityResult{}, ErrQueryResultTooLarge
			}
			result := wordPageSetupResult{
				Sections:    make([]wordPageSectionView, 0, len(layout.Sections)),
				Unsupported: make([]wordLayoutUnsupportedView, 0, len(layout.Unsupported)),
			}
			for _, unsupported := range layout.Unsupported {
				result.Unsupported = append(result.Unsupported, wordLayoutUnsupportedView{
					SourceID: unsupported.SourceID, Code: unsupported.Code, Description: unsupported.Description,
				})
			}
			for _, section := range layout.Sections {
				result.Sections = append(result.Sections, wordPageSectionView{
					StartBlock: section.StartBlock, StartType: section.StartType, PageSetup: toWordPageSetupView(section.PageSetup),
				})
			}
			capabilityResult, err := capabilityJSONResult(result, false)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(capabilityResult.Data) > MaxQueryResultBytes {
				return CapabilityResult{}, fmt.Errorf("%w: serialized Word page setup exceeds %d bytes", ErrQueryResultTooLarge, MaxQueryResultBytes)
			}
			return capabilityResult, nil
		},
	})
}

func toWordPageSetupView(setup document.WordPageSetup) wordPageSetupView {
	explicit := make([]string, 0, 9)
	add := func(mask document.WordPageSetupMask, name string, present bool) {
		if setup.Set&mask != 0 || present {
			explicit = append(explicit, name)
		}
	}
	add(document.WordPageWidthSet, "width_points", setup.WidthPT != 0)
	add(document.WordPageHeightSet, "height_points", setup.HeightPT != 0)
	add(document.WordPageOrientationSet, "orientation", setup.Orientation != "")
	add(document.WordPageMarginTopSet, "margin_top_points", setup.MarginTopPT != 0)
	add(document.WordPageMarginRightSet, "margin_right_points", setup.MarginRightPT != 0)
	add(document.WordPageMarginBottomSet, "margin_bottom_points", setup.MarginBottomPT != 0)
	add(document.WordPageMarginLeftSet, "margin_left_points", setup.MarginLeftPT != 0)
	add(document.WordPageColumnsSet, "columns", setup.Columns != 0)
	add(document.WordPageColumnSpacingSet, "column_spacing_points", setup.ColumnSpacingPT != 0)
	return wordPageSetupView{
		WidthPoints: setup.WidthPT, HeightPoints: setup.HeightPT, Orientation: setup.Orientation,
		MarginTopPoints: setup.MarginTopPT, MarginRightPoints: setup.MarginRightPT,
		MarginBottomPoints: setup.MarginBottomPT, MarginLeftPoints: setup.MarginLeftPT,
		Columns: setup.Columns, ColumnSpacingPoints: setup.ColumnSpacingPT, ExplicitFields: explicit,
	}
}
