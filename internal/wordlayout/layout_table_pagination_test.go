package wordlayout

import (
	"context"
	"glance/internal/document"
	"math"
	"strings"
	"testing"
)

func TestLayoutOrdinaryTableSplitsBetweenRows(t *testing.T) {
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{635000, 635000}}}
	for rowIndex := 0; rowIndex < 3; rowIndex++ {
		for cellIndex := 0; cellIndex < 2; cellIndex++ {
			id := strings.Join([]string{"t0", "r", string(rune('0' + rowIndex)), "c", string(rune('0' + cellIndex))}, "")
			paragraph := document.WordParagraph{ID: id + ".p0", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".run", Text: "row", Style: document.WordTextStyle{FontSize: 10}, Editable: true}}}}
			tableRow := document.WordTableRow{Cells: []document.WordTableCell{{ID: id, WidthEMU: 635000, Paragraphs: []document.WordParagraph{paragraph}}}}
			if cellIndex == 0 {
				table.Rows = append(table.Rows, tableRow)
			} else {
				table.Rows[rowIndex].Cells = append(table.Rows[rowIndex].Cells, tableRow.Cells[0])
			}
		}
	}
	input := layoutTestInput(120, 38, 4, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("page count = %d; want 2", len(result.Pages))
	}
	if len(result.Pages[0].TableCells) != 4 || len(result.Pages[1].TableCells) != 2 {
		t.Fatalf("table cells per page = %d/%d; want two rows then one row split by cells", len(result.Pages[0].TableCells), len(result.Pages[1].TableCells))
	}
	if result.Pages[0].TableCells[2].RowIndex != 1 || result.Pages[1].TableCells[0].RowIndex != 2 {
		t.Fatalf("table row order across pages = %+v / %+v", result.Pages[0].TableCells, result.Pages[1].TableCells)
	}
}

func TestLayoutTableGreedilyFillsRemainingPageWithCellLines(t *testing.T) {
	paragraph := document.WordParagraph{
		ID:        "cell.p0",
		Alignment: document.WordAlignDefault,
		Inlines: []document.WordInline{{
			Kind: document.WordTextInline,
			Text: &document.WordTextRun{ID: "cell.run", Text: "a\nb\nc\nd", Style: document.WordTextStyle{FontSize: 10}, Editable: true},
		}},
	}
	table := document.WordTable{
		Layout: document.WordTableLayout{GridColumnsEMU: []int64{635000}},
		Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{
			ID: "cell", Paragraphs: []document.WordParagraph{paragraph},
		}}}},
	}
	input := layoutTestInput(100, 60, 5,
		paragraphBlock("before", "before", document.WordTextStyle{FontSize: 10}),
		document.WordBlock{ID: "table", Kind: document.WordTableBlock, Table: &table},
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("page count = %d; want 2", len(result.Pages))
	}
	if len(result.Pages[0].TableCells) != 1 {
		t.Fatalf("first page table fragments = %+v; want the row to use remaining page space", result.Pages[0].TableCells)
	}
	if !result.Pages[0].TableCells[0].DrawTopBorder || !result.Pages[0].TableCells[0].DrawBottomBorder ||
		!result.Pages[1].TableCells[0].DrawTopBorder || !result.Pages[1].TableCells[0].DrawBottomBorder {
		t.Fatalf("split table row fragment edges = %+v / %+v; want each visible page segment enclosed", result.Pages[0].TableCells[0], result.Pages[1].TableCells[0])
	}
	firstPageText := joinPageText(result.Pages[0])
	if !strings.Contains(firstPageText, "a") || !strings.Contains(firstPageText, "b") || !strings.Contains(firstPageText, "c") || strings.Contains(firstPageText, "d") {
		t.Fatalf("first page cell continuation = %q; want lines a-c only", firstPageText)
	}
	if !strings.Contains(joinPageText(result.Pages[1]), "d") {
		t.Fatalf("second page is missing the remaining cell line: %q", joinPageText(result.Pages[1]))
	}
}

func TestLayoutCantSplitTableRowMovesWholeToNextPage(t *testing.T) {
	paragraph := document.WordParagraph{
		ID:        "cell.p0",
		Alignment: document.WordAlignDefault,
		Inlines: []document.WordInline{{
			Kind: document.WordTextInline,
			Text: &document.WordTextRun{ID: "cell.run", Text: "a\nb\nc\nd", Style: document.WordTextStyle{FontSize: 10}, Editable: true},
		}},
	}
	table := document.WordTable{
		Layout: document.WordTableLayout{GridColumnsEMU: []int64{635000}},
		Rows: []document.WordTableRow{{CantSplit: true, Cells: []document.WordTableCell{{
			ID: "cell", Paragraphs: []document.WordParagraph{paragraph},
		}}}},
	}
	input := layoutTestInput(100, 60, 5,
		paragraphBlock("before", "before", document.WordTextStyle{FontSize: 10}),
		document.WordBlock{ID: "table", Kind: document.WordTableBlock, Table: &table},
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 || len(result.Pages[0].TableCells) != 0 || len(result.Pages[1].TableCells) != 1 {
		t.Fatalf("cantSplit table page placement = pages %d, cells %d/%d; want the whole row on page 2", len(result.Pages), len(result.Pages[0].TableCells), len(result.Pages[1].TableCells))
	}
	if !strings.Contains(joinPageText(result.Pages[1]), "a") || !strings.Contains(joinPageText(result.Pages[1]), "b") || !strings.Contains(joinPageText(result.Pages[1]), "c") || !strings.Contains(joinPageText(result.Pages[1]), "d") {
		t.Fatalf("cantSplit row text = %q; want every line on page 2", joinPageText(result.Pages[1]))
	}
}

func TestLayoutKeepTogetherCellParagraphMovesTogetherToNextPage(t *testing.T) {
	paragraph := document.WordParagraph{
		ID: "cell.p0", Alignment: document.WordAlignDefault,
		Layout: document.WordParagraphLayout{KeepTogether: true, Set: document.WordParagraphKeepTogetherSet},
		Inlines: []document.WordInline{{
			Kind: document.WordTextInline,
			Text: &document.WordTextRun{ID: "cell.run", Text: "a\nb", Style: document.WordTextStyle{FontSize: 10}, Editable: true},
		}},
	}
	table := document.WordTable{
		Layout: document.WordTableLayout{GridColumnsEMU: []int64{635000}},
		Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{
			ID: "cell", Paragraphs: []document.WordParagraph{paragraph},
		}}}},
	}
	input := layoutTestInput(100, 60, 5,
		paragraphBlock("before", "top\nmiddle\nbottom", document.WordTextStyle{FontSize: 10}),
		document.WordBlock{ID: "table", Kind: document.WordTableBlock, Table: &table},
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 || len(result.Pages[0].TableCells) != 0 || len(result.Pages[1].TableCells) != 1 {
		t.Fatalf("keepTogether table placement = pages %d, cells %d/%d; want the paragraph together on page 2", len(result.Pages), len(result.Pages[0].TableCells), len(result.Pages[1].TableCells))
	}
	if got := joinPageText(result.Pages[1]); !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("keepTogether paragraph text = %q; want both lines on page 2", got)
	}
}

func TestLayoutOversizedTableRowReportsOverflowAndTerminates(t *testing.T) {
	paragraph := document.WordParagraph{ID: "cell.p0", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "abcdefghijklmnopqrstuvwx", Style: document.WordTextStyle{FontSize: 10}, Editable: true}}}}
	table := document.WordTable{Rows: []document.WordTableRow{{CantSplit: true, Cells: []document.WordTableCell{{ID: "cell0", WidthEMU: 254000, Paragraphs: []document.WordParagraph{paragraph}}}}}}
	input := layoutTestInput(40, 30, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.CanPreview {
		t.Fatal("oversized row must block output")
	}
	count := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "table-row-overflow" && diagnostic.SourceID == "t0.r0" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("overflow diagnostics = %+v; want one located t0.r0 diagnostic", result.Diagnostics)
	}
	if len(result.Pages) != 1 {
		t.Fatalf("oversized row caused retry/blank pages: %d", len(result.Pages))
	}
	for _, page := range result.Pages {
		for _, cell := range page.TableCells {
			if cell.Bounds.XPT < 0 || cell.Bounds.YPT < 0 || cell.Bounds.WidthPT < 0 || cell.Bounds.HeightPT < 0 || math.IsNaN(cell.Bounds.HeightPT) || math.IsInf(cell.Bounds.HeightPT, 0) {
				t.Fatalf("invalid overflow geometry: %+v", cell.Bounds)
			}
		}
	}
}

func TestLayoutSplittableTableRowTallerThanPageContinuesAcrossPages(t *testing.T) {
	paragraph := document.WordParagraph{ID: "cell.p0", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "abcdefghijklmnopqrstuvwx", Style: document.WordTextStyle{FontSize: 10}, Editable: true}}}}
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{254000}}, Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "cell0", WidthEMU: 254000, Paragraphs: []document.WordParagraph{paragraph}}}}}}
	input := layoutTestInput(40, 30, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) < 2 {
		t.Fatalf("splittable row layout = %d pages, diagnostics %+v; want preview across multiple pages", len(result.Pages), result.Diagnostics)
	}
	var content strings.Builder
	for _, page := range result.Pages {
		for _, fragment := range page.Text {
			content.WriteString(fragment.Text)
			if fragment.BaselineYPT > page.Content.YPT+page.Content.HeightPT+epsilonPT {
				t.Errorf("table text escaped page %d printable area: baseline %.2f, bottom %.2f", page.Index, fragment.BaselineYPT, page.Content.YPT+page.Content.HeightPT)
			}
		}
	}
	if got := content.String(); got != "abcdefghijklmnopqrstuvwx" {
		t.Fatalf("continued row text = %q; want every rune once in source order", got)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "table-row-overflow" {
			t.Fatalf("splittable row reported an overflow: %+v", diagnostic)
		}
	}
}
