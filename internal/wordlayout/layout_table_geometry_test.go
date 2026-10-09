package wordlayout

import (
	"context"
	"glance/internal/document"
	"math"
	"strings"
	"testing"
)

func TestLayoutGridSpanUsesCoveredColumnWidths(t *testing.T) {
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{127000, 0, 127000}}}
	table.Rows = []document.WordTableRow{{Cells: []document.WordTableCell{
		{ID: "wide", WidthEMU: 254000, GridSpan: 2},
		{ID: "last", WidthEMU: 127000},
	}}}
	input := layoutTestInput(100, 80, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview {
		t.Fatalf("valid spanned table blocked layout: %+v", result.Diagnostics)
	}
	if len(result.Pages[0].TableCells) != 2 {
		t.Fatalf("table placements = %+v; want two cells", result.Pages[0].TableCells)
	}
	wide, last := result.Pages[0].TableCells[0], result.Pages[0].TableCells[1]
	if wide.CellID != "wide" || math.Abs(wide.Bounds.XPT-5) > 0.01 || math.Abs(wide.Bounds.WidthPT-60) > 0.01 {
		t.Fatalf("spanned cell bounds = %+v; want x=5pt width=60pt", wide)
	}
	if last.CellID != "last" || math.Abs(last.Bounds.XPT-65) > 0.01 || math.Abs(last.Bounds.WidthPT-30) > 0.01 {
		t.Fatalf("following cell bounds = %+v; want x=65pt width=30pt", last)
	}
}

func TestLayoutVerticalMergeAggregatesRowsAndSuppressesInternalBorders(t *testing.T) {
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{127000, 127000}}}
	table.Rows = []document.WordTableRow{
		{Cells: []document.WordTableCell{
			layoutTableCell("owner", "owner", 10, 0, document.WordTableVerticalMergeRestart),
			layoutTableCell("peer0", "a", 10, 0, document.WordTableVerticalMergeNone),
		}},
		{Cells: []document.WordTableCell{
			layoutTableCell("continuation", "", 10, 0, document.WordTableVerticalMergeContinue),
			layoutTableCell("peer1", "b\nb", 10, 0, document.WordTableVerticalMergeNone),
		}},
	}
	input := layoutTestInput(100, 100, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) != 1 {
		t.Fatalf("vertical merge layout = pages %d, diagnostics %+v", len(result.Pages), result.Diagnostics)
	}
	placements := result.Pages[0].TableCells
	if len(placements) != 3 {
		t.Fatalf("placements = %+v; want restart plus two peer cells and no continuation cell", placements)
	}
	var merged, row0Peer, row1Peer TableCellPlacement
	for _, placement := range placements {
		switch placement.CellID {
		case "owner":
			merged = placement
		case "peer0":
			row0Peer = placement
		case "peer1":
			row1Peer = placement
		case "continuation":
			t.Fatal("vertical continuation emitted a duplicate cell placement")
		}
	}
	if math.Abs(merged.Bounds.HeightPT-row0Peer.Bounds.HeightPT-row1Peer.Bounds.HeightPT) > 0.01 {
		t.Fatalf("merged bounds height %.2f does not aggregate row heights %.2f + %.2f", merged.Bounds.HeightPT, row0Peer.Bounds.HeightPT, row1Peer.Bounds.HeightPT)
	}
	if strings.Count(joinPageText(result.Pages[0]), "owner") != 1 {
		t.Fatalf("restart content was duplicated: %q", joinPageText(result.Pages[0]))
	}
	if !merged.DrawTopBorder || !merged.DrawBottomBorder || !merged.DrawLeftBorder || !merged.DrawRightBorder {
		t.Fatalf("merged outer edges = %+v; want visible top, bottom, left, and right", merged)
	}
}

func TestLayoutVerticalMergeCreatesVisiblePageSegments(t *testing.T) {
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{127000, 127000}}}
	table.Rows = []document.WordTableRow{
		{Cells: []document.WordTableCell{
			layoutTableCell("owner", "owner", 10, 0, document.WordTableVerticalMergeRestart),
			layoutTableCell("peer0", "a", 15, 0, document.WordTableVerticalMergeNone),
		}},
		{Cells: []document.WordTableCell{
			layoutTableCell("continuation", "", 10, 0, document.WordTableVerticalMergeContinue),
			layoutTableCell("peer1", "b", 15, 0, document.WordTableVerticalMergeNone),
		}},
	}
	input := layoutTestInput(100, 40, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) != 2 {
		t.Fatalf("vertical merge page count = %d, diagnostics %+v; want two pages", len(result.Pages), result.Diagnostics)
	}
	for pageIndex, page := range result.Pages {
		var segment *TableCellPlacement
		for cellIndex := range page.TableCells {
			if page.TableCells[cellIndex].CellID == "owner" {
				segment = &page.TableCells[cellIndex]
			}
		}
		if segment == nil {
			t.Fatalf("page %d has no visible segment for the merged cell: %+v", pageIndex, page.TableCells)
		}
		if !segment.DrawTopBorder || !segment.DrawBottomBorder || !segment.DrawLeftBorder || !segment.DrawRightBorder {
			t.Errorf("page %d merge segment edges = %+v; want visible page-segment boundaries", pageIndex, segment)
		}
		if pageIndex == 0 && !strings.Contains(joinPageText(page), "owner") {
			t.Errorf("restart content is missing from first page: %q", joinPageText(page))
		}
		if pageIndex == 1 && strings.Contains(joinPageText(page), "owner") {
			t.Errorf("restart content was duplicated on continuation page: %q", joinPageText(page))
		}
	}
}

func TestLayoutVerticalMergeTextContinuesAcrossPageSegments(t *testing.T) {
	owner := document.WordTableCell{
		ID: "owner", VerticalMerge: document.WordTableVerticalMergeRestart,
		Paragraphs: []document.WordParagraph{{
			ID: "owner.p0", Alignment: document.WordAlignDefault,
			Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{
				ID: "owner.run", Text: "a\nb\nc\nd", Style: document.WordTextStyle{FontSize: 10}, Editable: true,
			}}},
		}},
	}
	table := document.WordTable{
		Layout: document.WordTableLayout{GridColumnsEMU: []int64{635000, 635000}},
		Rows: []document.WordTableRow{
			{Cells: []document.WordTableCell{owner, layoutTableCell("peer0", "0", 10, 0, document.WordTableVerticalMergeNone)}},
			{Cells: []document.WordTableCell{{ID: "continue1", VerticalMerge: document.WordTableVerticalMergeContinue}, layoutTableCell("peer1", "1", 10, 0, document.WordTableVerticalMergeNone)}},
		},
	}
	input := layoutTestInput(120, 45, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) != 2 {
		t.Fatalf("merged cell page layout = %d pages, diagnostics %+v; want two visible page segments", len(result.Pages), result.Diagnostics)
	}
	continuedPages := 0
	lineCount := 0
	for _, page := range result.Pages {
		pageHasOwnerText := false
		for _, fragment := range page.Text {
			if fragment.RunID != "owner.run" {
				continue
			}
			pageHasOwnerText = true
			lineCount++
			if fragment.BaselineYPT > page.Content.YPT+page.Content.HeightPT+epsilonPT {
				t.Errorf("merged cell line escaped page %d printable area: baseline %.2f, bottom %.2f", page.Index, fragment.BaselineYPT, page.Content.YPT+page.Content.HeightPT)
			}
		}
		if pageHasOwnerText {
			continuedPages++
		}
	}
	if continuedPages != 2 || lineCount != 4 {
		t.Fatalf("merged owner text occupied %d pages with %d lines; want four lines continued across both pages", continuedPages, lineCount)
	}
	lastLine, ok := result.Locations[SourcePosition{RunID: "owner.run", RuneOffset: 6}]
	if !ok || lastLine.PageIndex != 1 {
		t.Fatalf("merged-cell continuation position = %+v found=%v; want the final rune on page 1", lastLine, ok)
	}
}

func TestLayoutRejectsMalformedMergeGeometry(t *testing.T) {
	tests := []struct {
		name  string
		table document.WordTable
		code  string
	}{
		{
			name:  "span outside grid",
			table: document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{127000, 127000}}, Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "bad-span", GridSpan: 3}}}}},
			code:  "table-grid-span",
		},
		{
			name:  "orphan continuation",
			table: document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{127000}}, Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "orphan", VerticalMerge: document.WordTableVerticalMergeContinue}}}}},
			code:  "table-vertical-merge",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := layoutTestInput(100, 80, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &test.table})
			result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if result.CanPreview {
				t.Fatalf("malformed table unexpectedly allowed preview: %+v", result.Pages)
			}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Code == test.code && diagnostic.Severity == DiagnosticError {
					if diagnostic.SourceID == "" {
						t.Fatal("malformed table diagnostic has no source location")
					}
					return
				}
			}
			t.Fatalf("missing located %q diagnostic: %+v", test.code, result.Diagnostics)
		})
	}
}

func TestLayoutFillsMissingTableGridColumnWithoutExceedingPageWidth(t *testing.T) {
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{1270000, 0}}}
	row := document.WordTableRow{}
	for index := 0; index < 2; index++ {
		paragraph := document.WordParagraph{ID: "cell.p" + itoa(index), Alignment: document.WordAlignDefault}
		row.Cells = append(row.Cells, document.WordTableCell{ID: "cell" + itoa(index), Paragraphs: []document.WordParagraph{paragraph}})
	}
	table.Rows = []document.WordTableRow{row}
	input := layoutTestInput(100, 80, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	cells := result.Pages[0].TableCells
	if len(cells) != 2 || cells[0].Bounds.XPT != 5 || cells[0].Bounds.WidthPT != 45 || cells[1].Bounds.XPT != 50 || cells[1].Bounds.WidthPT != 45 || cells[1].Bounds.XPT+cells[1].Bounds.WidthPT != 95 {
		t.Fatalf("table grid cell bounds = %+v; want adjacent 45pt columns contained in the 90pt page width", cells)
	}
	foundWarning := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == DiagnosticWarning && diagnostic.SourceID == "t0" && diagnostic.Code == "table-grid-column-width-inferred" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("diagnostics = %+v; want a located warning for the inferred grid column", result.Diagnostics)
	}
}
