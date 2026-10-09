package docx

import "glance/internal/document"

func cloneWordBlocks(blocks []document.WordBlock) []document.WordBlock {
	clone := make([]document.WordBlock, len(blocks))
	for i, block := range blocks {
		clone[i] = block
		if block.Unsupported != nil {
			unsupported := *block.Unsupported
			unsupported.ImagePreview = cloneWordImage(unsupported.ImagePreview)
			clone[i].Unsupported = &unsupported
		}
		if block.Paragraph != nil {
			paragraph := cloneWordParagraph(*block.Paragraph)
			clone[i].Paragraph = &paragraph
		}
		if block.Table != nil {
			table := document.WordTable{Layout: block.Table.Layout, Rows: make([]document.WordTableRow, len(block.Table.Rows))}
			table.Layout.GridColumnsEMU = append([]int64(nil), block.Table.Layout.GridColumnsEMU...)
			for rowIndex, row := range block.Table.Rows {
				table.Rows[rowIndex].CantSplit = row.CantSplit
				table.Rows[rowIndex].Cells = make([]document.WordTableCell, len(row.Cells))
				for cellIndex, cell := range row.Cells {
					table.Rows[rowIndex].Cells[cellIndex] = document.WordTableCell{
						ID: cell.ID, WidthEMU: cell.WidthEMU, WidthFallback: cell.WidthFallback, GridSpan: cell.GridSpan, VerticalMerge: cell.VerticalMerge,
						FillColor: cell.FillColor, Paragraphs: make([]document.WordParagraph, len(cell.Paragraphs)),
					}
					for paragraphIndex, paragraph := range cell.Paragraphs {
						table.Rows[rowIndex].Cells[cellIndex].Paragraphs[paragraphIndex] = cloneWordParagraph(paragraph)
					}
				}
			}
			clone[i].Table = &table
		}
	}
	return clone
}

func cloneWordParagraph(paragraph document.WordParagraph) document.WordParagraph {
	clone := paragraph
	clone.Inlines = make([]document.WordInline, len(paragraph.Inlines))
	for i, inline := range paragraph.Inlines {
		clone.Inlines[i] = inline
		if inline.Text != nil {
			text := *inline.Text
			clone.Inlines[i].Text = &text
		}
		if inline.Image != nil {
			clone.Inlines[i].Image = cloneWordImage(inline.Image)
		}
		if inline.Unsupported != nil {
			unsupported := *inline.Unsupported
			unsupported.ImagePreview = cloneWordImage(unsupported.ImagePreview)
			clone.Inlines[i].Unsupported = &unsupported
		}
		if inline.PageBreak != nil {
			pageBreak := *inline.PageBreak
			clone.Inlines[i].PageBreak = &pageBreak
		}
		if inline.Tab != nil {
			tab := *inline.Tab
			clone.Inlines[i].Tab = &tab
		}
		if inline.FieldCode != nil {
			fieldCode := *inline.FieldCode
			clone.Inlines[i].FieldCode = &fieldCode
		}
	}
	clone.Layout.TabStops = append([]document.WordTabStop(nil), paragraph.Layout.TabStops...)
	return clone
}

func cloneWordImage(source *document.WordImage) *document.WordImage {
	if source == nil {
		return nil
	}
	clone := *source
	if source.Floating != nil {
		floating := *source.Floating
		floating.WrapPolygon = append([]document.WordImageWrapPoint(nil), source.Floating.WrapPolygon...)
		clone.Floating = &floating
	}
	return &clone
}
