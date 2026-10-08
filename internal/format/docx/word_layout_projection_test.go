package docx

import (
	"archive/zip"
	"bytes"
	"slices"
	"testing"

	"glance/internal/document"
)

func wordLayoutProjectionFixture(t *testing.T, documentXML, stylesXML string) *Document {
	t.Helper()
	parts := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`),
		"_rels/.rels":         []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`),
		"word/document.xml":   []byte(documentXML),
	}
	if stylesXML != "" {
		parts["word/_rels/document.xml.rels"] = []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`)
		parts["word/styles.xml"] = []byte(stylesXML)
	}
	var packageBytes bytes.Buffer
	writer := zip.NewWriter(&packageBytes)
	for name, data := range parts {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	doc, err := OpenSnapshot(packageBytes.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	return doc
}

func requireWordLayoutReader(t *testing.T, doc *Document) document.WordLayoutReader {
	t.Helper()
	reader, ok := any(doc).(document.WordLayoutReader)
	if !ok {
		t.Fatal("DOCX adapter does not expose neutral Word layout metadata")
	}
	return reader
}

func TestWordLayoutProjectionReadsSectionPageSetupAndStyleDefaults(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:p><w:pPr><w:pStyle w:val="Normal"/><w:spacing w:before="240" w:after="120" w:line="360" w:lineRule="exact"/><w:ind w:firstLine="360"/><w:keepNext/><w:keepLines/></w:pPr><w:r><w:rPr><w:b w:val="0"/><w:sz w:val="28"/></w:rPr><w:t>content</w:t></w:r></w:p>
	<w:tbl><w:tblGrid><w:gridCol w:w="4320"/><w:gridCol w:w="4320"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="4320" w:type="dxa"/></w:tcPr><w:p><w:pPr><w:ind w:hanging="240"/></w:pPr><w:r><w:t>cell</w:t></w:r></w:p></w:tc><w:tc><w:tcPr><w:tcW w:w="4320" w:type="dxa"/></w:tcPr><w:p><w:r><w:t>cell 2</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
	<w:sectPr><w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/><w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800" w:header="720" w:footer="720"/></w:sectPr>
	</w:body></w:document>`
	stylesXML := `<?xml version="1.0" encoding="UTF-8"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Arial" w:eastAsia="Noto Sans CJK SC"/><w:sz w:val="22"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="240" w:line="360" w:lineRule="exact"/></w:pPr></w:pPrDefault></w:docDefaults>
	<w:style w:type="paragraph" w:styleId="Base"><w:pPr><w:spacing w:before="120"/></w:pPr><w:rPr><w:color w:val="336699"/></w:rPr></w:style>
	<w:style w:type="paragraph" w:styleId="Normal"><w:basedOn w:val="Base"/><w:pPr><w:keepNext/></w:pPr><w:rPr><w:b/></w:rPr></w:style>
	</w:styles>`
	doc := wordLayoutProjectionFixture(t, documentXML, stylesXML)
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	data := requireWordLayoutReader(t, doc).WordLayoutData()
	if len(data.Sections) != 1 {
		t.Fatalf("section count = %d; want 1", len(data.Sections))
	}
	section := data.Sections[0]
	if section.StartBlock != 0 || section.PageSetup.WidthPT != 841.9 || section.PageSetup.HeightPT != 595.3 || section.PageSetup.Orientation != "landscape" {
		t.Fatalf("section geometry = %+v; want landscape 841.9×595.3pt starting at block 0", section)
	}
	pageSetupSet := document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageOrientationSet | document.WordPageMarginTopSet | document.WordPageMarginRightSet | document.WordPageMarginBottomSet | document.WordPageMarginLeftSet
	if section.PageSetup.Set&pageSetupSet != pageSetupSet {
		t.Fatalf("page setup presence mask = %b; want all geometry/margin values explicitly present", section.PageSetup.Set)
	}
	if section.PageSetup.MarginTopPT != 72 || section.PageSetup.MarginRightPT != 90 || section.PageSetup.MarginBottomPT != 72 || section.PageSetup.MarginLeftPT != 90 {
		t.Fatalf("section margins = %+v; want top/bottom 72pt and left/right 90pt", section.PageSetup)
	}
	if data.Defaults.Text.FontFamily != "Noto Sans CJK SC" || data.Defaults.Text.FontSize != 11 || data.Defaults.TextSet&document.WordTextFontFamilySet == 0 || data.Defaults.TextSet&document.WordTextFontSizeSet == 0 {
		t.Fatalf("docDefaults text = %+v mask=%b; want CJK 11pt and explicit masks", data.Defaults.Text, data.Defaults.TextSet)
	}
	if data.Defaults.Paragraph.AfterPT != 12 || data.Defaults.Paragraph.LineSpacingPT != 18 || data.Defaults.Paragraph.LineSpacingRule != "exact" {
		t.Fatalf("docDefaults paragraph = %+v; want after 12pt and exact 18pt line spacing", data.Defaults.Paragraph)
	}
	foundNormal := false
	for _, style := range data.Styles {
		if style.ID == "Normal" {
			foundNormal = true
			if style.BasedOn != "Base" || style.TextSet&document.WordTextBoldSet == 0 || !style.Text.Bold || style.Paragraph.Set&document.WordParagraphKeepWithNextSet == 0 || !style.Paragraph.KeepWithNext {
				t.Fatalf("Normal style = %+v; want Base inheritance and explicit bold/keepNext", style)
			}
		}
	}
	if !foundNormal {
		t.Fatal("style inheritance input Normal was not projected")
	}

	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph == nil || blocks[1].Table == nil {
		t.Fatalf("projected body blocks = %#v; want paragraph then table", blocks)
	}
	paragraph := blocks[0].Paragraph
	layout := paragraph.Layout
	if layout.BeforePT != 12 || layout.AfterPT != 6 || layout.LineSpacingPT != 18 || layout.FirstLineIndentEMU != 228600 || !layout.KeepWithNext || !layout.KeepTogether {
		t.Fatalf("paragraph layout = %+v; want direct spacing, 360-twip indent, and keep flags", layout)
	}
	run := paragraph.Inlines[0].Text
	if run == nil || run.Style.Bold || run.Style.FontSize != 14 || run.StyleSet&document.WordTextBoldSet == 0 || run.StyleSet&document.WordTextFontSizeSet == 0 {
		t.Fatalf("direct run style = %#v; explicit bold-off and 14pt masks must survive", run)
	}
	table := blocks[1].Table
	if !slices.Equal(table.Layout.GridColumnsEMU, []int64{2743200, 2743200}) || len(table.Rows) != 1 || table.Rows[0].Cells[0].WidthEMU != 2743200 {
		t.Fatalf("table geometry = %+v; want grid/cell widths converted from 4320 twips", table)
	}
	cellParagraph := table.Rows[0].Cells[0].Paragraphs[0].Layout
	if cellParagraph.HangingIndentEMU != 152400 || cellParagraph.Set&document.WordParagraphHangingIndentSet == 0 {
		t.Fatalf("table-cell hanging indent = %+v; want 240 twips preserved", cellParagraph)
	}
	if len(data.Unsupported) != 0 {
		t.Fatalf("supported one-section fixture produced unsupported diagnostics: %+v", data.Unsupported)
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || doc.Dirty() {
		t.Fatal("layout projection modified the package snapshot or dirty state")
	}
	data.Sections[0].PageSetup.WidthPT = 1
	data.Styles[0].ID = "mutated"
	table.Layout.GridColumnsEMU[0] = 1
	if reloaded := requireWordLayoutReader(t, doc).WordLayoutData(); reloaded.Sections[0].PageSetup.WidthPT != 841.9 || reloaded.Styles[0].ID == "mutated" {
		t.Fatalf("caller mutation leaked back into the DOCX layout projection: %+v", reloaded)
	}
	if reloaded := doc.WordBlocks(); reloaded[1].Table.Layout.GridColumnsEMU[0] != 2743200 {
		t.Fatalf("caller mutation leaked back into the DOCX table projection: %+v", reloaded[1].Table.Layout)
	}
}

func TestWordLayoutProjectionDoesNotRejectOmittedPageOrientation(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>content</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="13500" w:h="9000"/><w:pgMar w:top="1000" w:right="1350" w:bottom="1000" w:left="1350"/></w:sectPr></w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")

	section := requireWordLayoutReader(t, doc).WordLayoutData().Sections[0]
	if section.PageSetup.Orientation != "" || section.PageSetup.Set&document.WordPageOrientationSet != 0 {
		t.Fatalf("omitted orientation was projected as explicit: %+v", section.PageSetup)
	}
	if section.PageSetup.WidthPT != 675 || section.PageSetup.HeightPT != 450 {
		t.Fatalf("page size = %.2f×%.2fpt; want 675×450pt", section.PageSetup.WidthPT, section.PageSetup.HeightPT)
	}
	if data := requireWordLayoutReader(t, doc).WordLayoutData(); len(data.Unsupported) != 0 {
		t.Fatalf("valid page size with omitted orientation produced unsupported diagnostics: %+v", data.Unsupported)
	}
}

func TestWordLayoutProjectionReadsParagraphBottomBorder(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="8" w:space="5" w:color="4F81BD"/></w:pBdr></w:pPr><w:r><w:t>Title</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	blocks := doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph == nil {
		t.Fatalf("projected blocks = %#v; want one paragraph", blocks)
	}
	layout := blocks[0].Paragraph.Layout
	if layout.Set&document.WordParagraphBottomBorderSet == 0 || layout.BottomBorder != (document.WordParagraphBorder{Style: "single", SizePT: 1, SpacePT: 5, Color: "4F81BD"}) {
		t.Fatalf("paragraph bottom border = %+v mask=%b; want single 1pt blue line with 5pt spacing", layout.BottomBorder, layout.Set)
	}
}

func TestWordLayoutProjectionRejectsExplicitEmptyPageOrientation(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>content</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="12240" w:h="15840" w:orient=""/></w:sectPr></w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")

	for _, item := range requireWordLayoutReader(t, doc).WordLayoutData().Unsupported {
		if item.Code == "unknown-orientation" && item.SourceID == "section-0" {
			return
		}
	}
	t.Fatal("explicit empty orientation was not reported as unsupported")
}

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
	for _, code := range []string{"multiple-sections", "columns", "table-grid-span", "table-vertical-merge", "nested-table"} {
		if codes[code] == "" {
			t.Errorf("missing located unsupported diagnostic %q: %+v", code, data.Unsupported)
		}
	}
	for _, code := range []string{"table-grid-span", "table-vertical-merge", "nested-table"} {
		if codes[code] != "b1" {
			t.Errorf("table diagnostic %q source = %q; want body block b1", code, codes[code])
		}
	}
}

func TestWordLayoutProjectionProjectsColumns(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:p><w:r><w:t>hello</w:t></w:r></w:p>
	<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:cols w:num="2" w:space="720"/></w:sectPr>
	</w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	data := requireWordLayoutReader(t, doc).WordLayoutData()
	if len(data.Sections) != 1 {
		t.Fatalf("sections count = %d", len(data.Sections))
	}
	setup := data.Sections[0].PageSetup
	if setup.Columns != 2 {
		t.Errorf("got columns %d, want 2", setup.Columns)
	}
	if setup.ColumnSpacingPT != 36.0 {
		t.Errorf("got column spacing %f, want 36.0", setup.ColumnSpacingPT)
	}
	for _, item := range data.Unsupported {
		if item.Code == "columns" {
			t.Errorf("unexpected columns unsupported diagnostic: %+v", item)
		}
	}
}

func TestWordLayoutProjectionAllowsSectionsWithEquivalentPageSetup(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
	<w:p><w:r><w:t>cover</w:t></w:r><w:pPr><w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800"/><w:cols w:num="1" w:space="720"/></w:sectPr></w:pPr></w:p>
	<w:p><w:r><w:t>contents</w:t></w:r><w:pPr><w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800"/><w:cols w:num="1" w:space="720"/></w:sectPr></w:pPr></w:p>
	<w:p><w:r><w:t>body</w:t></w:r></w:p>
	<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800"/><w:cols w:num="1" w:space="720"/></w:sectPr>
	</w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	data := requireWordLayoutReader(t, doc).WordLayoutData()
	if len(data.Sections) != 3 || data.Sections[0].StartBlock != 0 || data.Sections[1].StartBlock != 1 || data.Sections[2].StartBlock != 2 {
		t.Fatalf("section boundaries = %+v; want three sections at blocks 0, 1, and 2", data.Sections)
	}
	for _, item := range data.Unsupported {
		if item.Code == "multiple-sections" {
			t.Fatalf("equivalent page setups reported as unsupported: %+v", item)
		}
	}
}

func TestWordLayoutProjectionIgnoresZeroCharacterIndent(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:ind w:firstLine="0" w:firstLineChars="0"/></w:pPr><w:r><w:t>text</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	data := requireWordLayoutReader(t, doc).WordLayoutData()
	for _, item := range data.Unsupported {
		if item.Code == "character-indent" {
			t.Fatalf("zero character indent reported as unsupported: %+v", item)
		}
	}
	paragraph := doc.WordBlocks()[0].Paragraph
	if paragraph.Layout.FirstLineIndentEMU != 0 || paragraph.Layout.Set&document.WordParagraphFirstLineIndentSet == 0 {
		t.Fatalf("zero first-line indent = %+v; want an explicitly set zero indent", paragraph.Layout)
	}
}

func TestWordLayoutProjectionReportsMalformedStylesPart(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>text</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`
	stylesXML := `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault></w:styles>`
	doc := wordLayoutProjectionFixture(t, documentXML, stylesXML)
	data := requireWordLayoutReader(t, doc).WordLayoutData()
	for _, item := range data.Unsupported {
		if item.Code == "invalid-styles" && item.SourceID == "word/styles.xml" {
			return
		}
	}
	t.Fatalf("malformed styles part has no located layout diagnostic: %+v", data.Unsupported)
}
