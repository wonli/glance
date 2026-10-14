package wordlayout

import (
	"fmt"

	"glance/internal/document"
)

const (
	unsupportedListDetail    = "This paragraph uses a list definition that is outside the supported numbering subset."
	unsupportedListFallback  = "Paragraph text is shown; the unsupported list marker is omitted."
	imagePlaceholderFallback = "此处显示“图片无法显示”占位文本，文档其他内容保留。"
)

// Preflight reports support gaps visible in the neutral Word model before page
// scenes are built. The layout pass adds findings that depend on measured page
// geometry, fonts, or successful shaping.
func Preflight(input Input) SupportReport {
	report := SupportReport{}
	for _, unsupported := range input.Layout.Unsupported {
		detail := unsupported.Description
		if detail == "" {
			detail = "This document feature cannot be represented by the current page layout."
		}
		report.add(SupportFinding{
			SourceID: unsupported.SourceID,
			Location: unsupported.SourceID,
			Code:     unsupported.Code,
			Detail:   detail,
			Impact:   SupportImpactBlocksPreview,
			Fallback: "No safe page-layout fallback is available.",
		})
	}
	paragraphNumber, tableNumber := 0, 0
	for blockIndex, block := range input.Blocks {
		switch block.Kind {
		case document.WordParagraphBlock:
			paragraphNumber++
			paragraphLocation := fmt.Sprintf("正文第%d段", paragraphNumber)
			if block.Paragraph == nil {
				report.add(blockingFinding(block.ID, paragraphLocation, "invalid-paragraph", "The paragraph block has no paragraph data."))
				continue
			}
			preflightParagraph(&report, *block.Paragraph, block.ID, paragraphLocation)
		case document.WordTableBlock:
			tableNumber++
			tableLocation := fmt.Sprintf("正文第%d个表格", tableNumber)
			if block.Table == nil {
				report.add(blockingFinding(block.ID, tableLocation, "invalid-table", "The table block has no table data."))
				continue
			}
			for rowIndex, row := range block.Table.Rows {
				for cellIndex, cell := range row.Cells {
					for paragraphIndex, paragraph := range cell.Paragraphs {
						location := fmt.Sprintf("%s第%d行第%d列第%d段", tableLocation, rowIndex+1, cellIndex+1, paragraphIndex+1)
						preflightParagraph(&report, paragraph, paragraph.ID, location)
					}
				}
			}
		case document.WordUnsupportedBlock:
			detail, sourceID := "This body content is not supported by Word pagination.", block.ID
			if block.Unsupported != nil {
				if block.Unsupported.Description != "" {
					detail = block.Unsupported.Description
				}
				if block.Unsupported.ID != "" {
					sourceID = block.Unsupported.ID
				}
			}
			report.add(blockingFinding(sourceID, fmt.Sprintf("正文第%d项", blockIndex+1), "unsupported-block", detail))
		default:
			report.add(blockingFinding(block.ID, fmt.Sprintf("正文第%d项", blockIndex+1), "unknown-block-kind", "The document contains an unknown body block kind."))
		}
	}
	return report
}

func preflightParagraph(report *SupportReport, paragraph document.WordParagraph, fallbackSourceID, location string) {
	sourceID := fallbackSourceID
	if sourceID == "" {
		sourceID = paragraph.ID
	}
	if paragraph.PreviewWarning != "" {
		report.add(SupportFinding{
			SourceID: sourceID,
			Location: location,
			Code:     "content-preview-fallback",
			Detail:   paragraph.PreviewWarning,
			Impact:   SupportImpactApproximation,
			Fallback: "The retained text is shown with a known preview approximation.",
		})
	}
	if paragraph.ListKind == document.WordListUnsupported {
		report.add(SupportFinding{
			SourceID: sourceID,
			Location: location,
			Code:     "unsupported-list",
			Detail:   unsupportedListDetail,
			Impact:   SupportImpactApproximation,
			Fallback: unsupportedListFallback,
		})
	}
	for _, inline := range paragraph.Inlines {
		if inline.Kind == document.WordImageInline {
			imageSourceID := sourceID
			code, detail := "", ""
			if inline.Image == nil {
				code = "invalid-image-inline"
				detail = "An image inline has no image data."
			} else {
				if inline.Image.ID != "" {
					imageSourceID = inline.Image.ID
				}
				if !inline.Image.Resolved {
					code = "unresolved-image"
					detail = "The inline image bytes are unavailable."
				} else if inline.Image.WidthEMU <= 0 || inline.Image.HeightEMU <= 0 {
					code = "invalid-image-size"
					detail = "The inline image has missing or invalid dimensions."
				}
			}
			if code != "" {
				report.add(SupportFinding{
					SourceID: imageSourceID,
					Location: location,
					Code:     code,
					Detail:   detail,
					Impact:   SupportImpactApproximation,
					Fallback: imagePlaceholderFallback,
				})
			}
			continue
		}
		if inline.Kind != document.WordUnsupportedInline {
			continue
		}
		inlineSourceID := sourceID
		detail := "This paragraph contains unsupported inline content."
		code := ""
		impact := SupportImpactBlocksPreview
		fallback := "No safe page-layout fallback is available."
		if inline.Unsupported != nil {
			if inline.Unsupported.ID != "" {
				inlineSourceID = inline.Unsupported.ID
			}
			if inline.Unsupported.Description != "" {
				detail = inline.Unsupported.Description
			}
			if inline.Unsupported.ImagePreview != nil {
				impact = SupportImpactApproximation
				code = "missing-image-media"
				fallback = "A placeholder is shown in place of the unsupported image object."
			}
		}
		if code == "" {
			code = "unsupported-inline"
		}
		report.add(SupportFinding{SourceID: inlineSourceID, Location: location, Code: code, Detail: detail, Impact: impact, Fallback: fallback})
	}
}

func blockingFinding(sourceID, location, code, detail string) SupportFinding {
	return SupportFinding{
		SourceID: sourceID,
		Location: location,
		Code:     code,
		Detail:   detail,
		Impact:   SupportImpactBlocksPreview,
		Fallback: "No safe page-layout fallback is available.",
	}
}

func (r *SupportReport) add(finding SupportFinding) {
	if r == nil || finding.Code == "" {
		return
	}
	for index := range r.Findings {
		if r.Findings[index].SourceID == finding.SourceID && r.Findings[index].Code == finding.Code {
			if r.Findings[index].Location == "" {
				r.Findings[index].Location = finding.Location
			}
			if r.Findings[index].Fallback == "" {
				r.Findings[index].Fallback = finding.Fallback
			}
			return
		}
	}
	r.Findings = append(r.Findings, finding)
}

func reportWithDiagnostics(preflight SupportReport, diagnostics []Diagnostic) SupportReport {
	report := SupportReport{Findings: append([]SupportFinding(nil), preflight.Findings...)}
	for _, diagnostic := range diagnostics {
		impact := SupportImpactBlocksPreview
		if diagnostic.Severity == DiagnosticWarning {
			impact = SupportImpactApproximation
		}
		fallback := ""
		switch diagnostic.Code {
		case "unsupported-list":
			fallback = unsupportedListFallback
		case "unsupported-block":
			fallback = "The unsupported body block is omitted from the page scene."
		case "unsupported-inline":
			fallback = "The unsupported inline item is omitted from the page scene."
		case "missing-image-media":
			fallback = "A placeholder is shown, or the image is omitted if it has no usable extent."
		case "invalid-image-inline", "unresolved-image", "invalid-image-size":
			fallback = imagePlaceholderFallback
		}
		report.add(SupportFinding{
			SourceID: diagnostic.SourceID,
			Location: diagnostic.SourceID,
			Code:     diagnostic.Code,
			Detail:   diagnostic.Detail,
			Impact:   impact,
			Fallback: fallback,
		})
	}
	return report
}
