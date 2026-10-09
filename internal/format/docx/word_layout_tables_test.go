package docx

import (
	"bytes"
	"glance/internal/document"
	"testing"
)

func TestWordLayoutProjectionReportsUnsupportedSectionsAndTableStructures(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:p><w:r><w:t>section one</w:t></w:r><w:pPr><w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:pPr></w:p>
	<w:tbl><w:tblGrid><w:gridCol w:w="4320"/><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/></w:tcPr><w:p><w:r><w:t>merged</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>nested</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:tc><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>vertical merge</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
	<w:sectPr><w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/><w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800"/><w:cols w:num="5"/></w:sectPr>
	</w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	data := requireWordLayoutReader(t, doc).WordLayoutData()
	if len(data.Sections) != 2 || data.Sections[0].StartBlock != 0 || data.Sections[1].StartBlock != 1 {
		t.Fatalf("section boundaries = %+v; want a second section beginning at block 1", data.Sections)
	}
	codes := make(map[string]string)
	for _, item := range data.Unsupported {
		codes[item.Code] = item.SourceID
	}
	for _, code := range []string{"multiple-sections", "columns", "table-grid-span"} {
		if codes[code] == "" {
			t.Errorf("missing located unsupported diagnostic %q: %+v", code, data.Unsupported)
		}
	}
	for _, code := range []string{"table-grid-span"} {
		if codes[code] != "b1" {
			t.Errorf("table diagnostic %q source = %q; want body block b1", code, codes[code])
		}
	}
	blocks := doc.WordBlocks()
	foundNested := false
	for _, row := range blocks[1].Table.Rows {
		for _, cell := range row.Cells {
			for _, paragraph := range cell.Paragraphs {
				for _, inline := range paragraph.Inlines {
					if inline.Text != nil && inline.Text.Text == "nested" {
						foundNested = true
						if inline.Text.Editable || paragraph.PreviewWarning == "" {
							t.Errorf("flattened nested-table run = %+v, paragraph warning=%q; want read-only preview text", inline.Text, paragraph.PreviewWarning)
						}
					}
				}
			}
		}
	}
	if !foundNested {
		t.Fatal("nested table text was not flattened into the parent cell")
	}
}

func TestWordLayoutProjectionProjectsMergedCells(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:tbl><w:tblGrid><w:gridCol w:w="4320"/><w:gridCol w:w="4320"/><w:gridCol w:w="4320"/></w:tblGrid>
	<w:tr>
		<w:tc><w:tcPr><w:tcW w:w="8640" w:type="dxa"/><w:gridSpan w:val="2"/><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>wide cell</w:t></w:r></w:p></w:tc>
		<w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>single cell</w:t></w:r></w:p></w:tc>
	</w:tr>
	<w:tr>
		<w:tc><w:tcPr><w:tcW w:w="8640" w:type="dxa"/><w:gridSpan w:val="2"/><w:vMerge/></w:tcPr><w:p/></w:tc>
		<w:tc><w:tcPr><w:vMerge w:val="continue"/></w:tcPr><w:p/></w:tc>
	</w:tr></w:tbl>
	</w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Table == nil || len(blocks[0].Table.Rows) != 2 {
		t.Fatalf("projected blocks = %+v; want one two-row table", blocks)
	}
	rows := blocks[0].Table.Rows
	if len(rows[0].Cells) != 2 || len(rows[1].Cells) != 2 {
		t.Fatalf("projected table rows = %+v; want two cells per row", rows)
	}
	if cell := rows[0].Cells[0]; cell.GridSpan != 2 || cell.VerticalMerge != document.WordTableVerticalMergeRestart {
		t.Errorf("first merged cell = %+v; want span 2 and vertical restart", cell)
	}
	if cell := rows[0].Cells[1]; cell.GridSpan != 0 || cell.VerticalMerge != document.WordTableVerticalMergeRestart {
		t.Errorf("second merged cell = %+v; want default span 1 and vertical restart", cell)
	}
	if cell := rows[1].Cells[0]; cell.GridSpan != 2 || cell.VerticalMerge != document.WordTableVerticalMergeContinue {
		t.Errorf("first continuation = %+v; want span 2 and vertical continuation from omitted val", cell)
	}
	if cell := rows[1].Cells[1]; cell.GridSpan != 0 || cell.VerticalMerge != document.WordTableVerticalMergeContinue {
		t.Errorf("second continuation = %+v; want default span 1 and explicit continuation", cell)
	}
	for _, item := range requireWordLayoutReader(t, doc).WordLayoutData().Unsupported {
		if item.Code == "table-grid-span" || item.Code == "table-vertical-merge" {
			t.Errorf("valid merged table produced unsupported diagnostic: %+v", item)
		}
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only merge projection changed the DOCX snapshot")
	}
}

func TestWordLayoutProjectionReadsCantSplitRows(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:tbl><w:tblGrid><w:gridCol w:w="4320"/></w:tblGrid>
		<w:tr><w:trPr><w:cantSplit/></w:trPr><w:tc><w:p><w:r><w:t>kept together</w:t></w:r></w:p></w:tc></w:tr>
		<w:tr><w:trPr><w:cantSplit w:val="false"/></w:trPr><w:tc><w:p><w:r><w:t>splittable</w:t></w:r></w:p></w:tc></w:tr>
		<w:tr><w:tc><w:p><w:r><w:t>default split behavior</w:t></w:r></w:p></w:tc></w:tr>
	</w:tbl></w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	blocks := doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Table == nil || len(blocks[0].Table.Rows) != 3 {
		t.Fatalf("projected table blocks = %+v; want a three-row table", blocks)
	}
	rows := blocks[0].Table.Rows
	if !rows[0].CantSplit || rows[1].CantSplit || rows[2].CantSplit {
		t.Fatalf("projected row split flags = %v/%v/%v; want true/false/false", rows[0].CantSplit, rows[1].CantSplit, rows[2].CantSplit)
	}
}

func TestWordLayoutProjectionReadsStructuredDocumentContentReadOnly(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:sdt><w:sdtPr/><w:sdtContent>
		<w:p><w:r><w:t>controlled paragraph</w:t></w:r></w:p>
		<w:tbl><w:tblGrid><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:p><w:r><w:t>controlled cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
	</w:sdtContent></w:sdt>
	</w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Kind != document.WordParagraphBlock || blocks[1].Kind != document.WordTableBlock {
		t.Fatalf("structured document content blocks = %+v; want paragraph and table blocks", blocks)
	}
	if blocks[0].Paragraph.Inlines[0].Text.Editable {
		t.Fatal("paragraph inside a content control was projected as editable")
	}
	cellParagraph := blocks[1].Table.Rows[0].Cells[0].Paragraphs[0]
	if cellParagraph.Inlines[0].Text.Editable {
		t.Fatal("table-cell text inside a content control was projected as editable")
	}
	for _, block := range blocks {
		if block.Kind == document.WordUnsupportedBlock {
			t.Fatalf("ordinary content-control children became unsupported: %+v", block.Unsupported)
		}
	}
	if unsupported := requireWordLayoutReader(t, doc).WordLayoutData().Unsupported; len(unsupported) != 0 {
		t.Fatalf("ordinary content-control children produced layout blockers: %+v", unsupported)
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only content-control projection changed the DOCX snapshot")
	}
}

func TestWordLayoutProjectionReadsTabStopsAndPreservesFieldControls(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:p><w:pPr><w:tabs><w:tab w:val="right" w:leader="dot" w:pos="8296"/></w:tabs></w:pPr><w:r><w:t>Entry</w:t><w:tab/></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>7</w:t><w:fldChar w:fldCharType="end"/></w:r></w:p>
	</w:body></w:document>`
	settingsXML := `<?xml version="1.0" encoding="UTF-8"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:defaultTabStop w:val="480"/></w:settings>`
	doc := wordLayoutProjectionFixtureWithSettings(t, documentXML, "", settingsXML)
	blocks := doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph == nil {
		t.Fatalf("projected blocks = %+v; want one paragraph", blocks)
	}
	paragraph := blocks[0].Paragraph
	if len(paragraph.Inlines) != 7 || paragraph.Inlines[1].Kind != document.WordTabInline || paragraph.Inlines[1].Tab == nil {
		t.Fatalf("projected tab and field controls = %+v; want tab and retained field syntax", paragraph.Inlines)
	}
	fieldControls := 0
	for _, inline := range paragraph.Inlines {
		if inline.Kind == document.WordFieldCodeInline && inline.FieldCode != nil {
			fieldControls++
		}
	}
	if fieldControls != 4 {
		t.Fatalf("field controls projected = %d; want all begin/instruction/separator/end syntax", fieldControls)
	}
	layout := requireWordLayoutReader(t, doc).WordLayoutData()
	if layout.DefaultTabStopPT != 24 || len(paragraph.Layout.TabStops) != 1 || paragraph.Layout.TabStops[0] != (document.WordTabStop{PositionPT: 414.8, Alignment: "right", Leader: "dot"}) {
		t.Fatalf("tab settings = default %.2fpt, paragraph %+v; want default 24pt and right stop at 414.8pt", layout.DefaultTabStopPT, paragraph.Layout.TabStops)
	}
	if len(layout.Unsupported) != 0 {
		t.Fatalf("field controls or tab stops produced blockers: %+v", layout.Unsupported)
	}
	if paragraph.Inlines[5].Text == nil || paragraph.Inlines[5].Text.Editable {
		t.Fatalf("cached field result = %+v; want read-only text", paragraph.Inlines[5])
	}
}

func TestWordLayoutProjectionRejectsMalformedMergedCells(t *testing.T) {
	tests := []struct {
		name  string
		table string
		code  string
	}{
		{
			name:  "zero span",
			table: `<w:tbl><w:tblGrid><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:gridSpan w:val="0"/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`,
			code:  "table-grid-span",
		},
		{
			name:  "span outside grid",
			table: `<w:tbl><w:tblGrid><w:gridCol w:w="4320"/><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:gridSpan w:val="3"/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`,
			code:  "table-grid-span",
		},
		{
			name:  "orphan continuation",
			table: `<w:tbl><w:tblGrid><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`,
			code:  "table-vertical-merge",
		},
		{
			name:  "mismatched continuation span",
			table: `<w:tbl><w:tblGrid><w:gridCol w:w="4320"/><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p/></w:tc><w:tc><w:p/></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/><w:vMerge w:val="continue"/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`,
			code:  "table-vertical-merge",
		},
		{
			name:  "nonempty continuation",
			table: `<w:tbl><w:tblGrid><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>owner</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge w:val="continue"/></w:tcPr><w:p><w:r><w:t>duplicate</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`,
			code:  "table-vertical-merge",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + test.table + `</w:body></w:document>`
			doc := wordLayoutProjectionFixture(t, documentXML, "")
			for _, item := range requireWordLayoutReader(t, doc).WordLayoutData().Unsupported {
				if item.Code == test.code {
					if item.SourceID != "b0" {
						t.Errorf("diagnostic source = %q; want table block b0", item.SourceID)
					}
					return
				}
			}
			t.Fatalf("missing located %q diagnostic for malformed merge structure", test.code)
		})
	}
}
