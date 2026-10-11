package application

import (
	"context"
	"encoding/json"
	"math"

	"glance/internal/document"
)

type WordAllParagraphSpacingSetter interface {
	SetWordAllParagraphSpacing(context.Context, int64, int64) (int, bool, error)
}

func registerWordParagraphBulkCapabilities(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.paragraph.spacing_all.set", Name: "设置全部 Word 段落间距",
			Description: "将 DOCX 所有正文和表格段落的段前、段后间距设为明确值，单位为 points，范围为 0 至 10000。Agent 可使用任意数值；Ribbon 的紧凑和宽松预设分别对应段后 4 和 12 points。",
			Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"before_points":{"type":"number","minimum":0,"maximum":10000},"after_points":{"type":"number","minimum":0,"maximum":10000}},"required":["before_points","after_points"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"before_points":{"type":"number"},"after_points":{"type":"number"},"paragraph_count":{"type":"integer","minimum":0}},"required":["changed","before_points","after_points","paragraph_count"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Before float64 `json:"before_points"`
				After  float64 `json:"after_points"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			if math.IsNaN(input.Before) || math.IsInf(input.Before, 0) || math.IsNaN(input.After) || math.IsInf(input.After, 0) || input.Before < 0 || input.Before > 10000 || input.After < 0 || input.After > 10000 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			setter, ok := capability.Adapter.(WordAllParagraphSpacingSetter)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			beforeTwips, afterTwips := int64(math.Round(input.Before*20)), int64(math.Round(input.After*20))
			paragraphCount, changed, err := setter.SetWordAllParagraphSpacing(ctx, beforeTwips, afterTwips)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Changed        bool    `json:"changed"`
				BeforePoints   float64 `json:"before_points"`
				AfterPoints    float64 `json:"after_points"`
				ParagraphCount int     `json:"paragraph_count"`
			}{changed, float64(beforeTwips) / 20, float64(afterTwips) / 20, paragraphCount}, changed)
		},
	})
}

func countWordParagraphs(blocks []document.WordBlock) int {
	count := 0
	for _, block := range blocks {
		if block.Paragraph != nil {
			count++
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					count += len(cell.Paragraphs)
				}
			}
		}
	}
	return count
}
