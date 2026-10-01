package desktopui

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordTableCellImageFlowsThroughGioLayout(t *testing.T) {
	word := syntheticWordTableDocument(t)
	defer word.Close()
	state := newDocumentSessionState("fixture.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	var expectedIDs []string
	var imageID string
	for _, paragraph := range allWordParagraphs(state.wordBlocks) {
		for _, inline := range paragraph.Inlines {
			switch {
			case inline.Text != nil:
				expectedIDs = append(expectedIDs, inline.Text.ID)
			case inline.Image != nil:
				expectedIDs = append(expectedIDs, inline.Image.ID)
				imageID = inline.Image.ID
			}
		}
	}
	if len(expectedIDs) != 7 || imageID == "" {
		t.Fatalf("synthetic table content IDs = %#v; want seven text/image slots", expectedIDs)
	}
	imageBytes, err := word.WordImageBytes(imageID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeWordPreview(imageBytes)
	if err != nil {
		t.Fatal(err)
	}
	state.wordImageCache.Put(imageID, compactWordPreview(decoded))
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(960, 760)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	dimensions := app.layoutWord(gtx)
	if dimensions.Size != image.Pt(960, 760) {
		t.Fatalf("table fixture layout dimensions = %v", dimensions.Size)
	}
	for _, id := range expectedIDs[1:6] {
		isImage := findWordImage(state.wordBlocks, id) != nil
		if isImage && state.wordObjectButtons[id] == nil {
			t.Errorf("visible table text/image slot %q has no persistent selection widget", id)
		} else if !isImage && state.wordRunEditors[id] == nil {
			t.Errorf("visible table text/image slot %q has no persistent in-place editor", id)
		}
	}
	window, err := headless.NewWindow(960, 760)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(&ops); err != nil {
		t.Fatalf("render Word table and inline image: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, 960, 760))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture Word table and inline image: %v", err)
	}
	bluePixels, orangePixels := 0, 0
	for y := 0; y < screenshot.Bounds().Dy(); y++ {
		for x := 0; x < screenshot.Bounds().Dx(); x++ {
			pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
			if pixel == (color.NRGBA{R: 45, G: 110, B: 190, A: 255}) {
				bluePixels++
			}
			if pixel == (color.NRGBA{R: 240, G: 170, B: 55, A: 255}) {
				orangePixels++
			}
		}
	}
	if bluePixels == 0 || orangePixels == 0 {
		t.Fatalf("rendered inline image colors: blue=%d orange=%d; both image halves must be visible", bluePixels, orangePixels)
	}
	var cellParagraphID string
	for _, paragraph := range allWordParagraphs(state.wordBlocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == expectedIDs[2] {
				cellParagraphID = paragraph.ID
			}
		}
	}
	run, ok := selectedWordRun(state.wordBlocks, expectedIDs[2])
	if !ok || cellParagraphID == "" || !selectWordRun(state, cellParagraphID, run.ID) {
		t.Fatalf("table-cell run %q in paragraph %q could not hand off to the Word editor", expectedIDs[2], cellParagraphID)
	}
	if state.wordEditor.Text() != "inside cell" {
		t.Fatalf("table-cell editor text = %q", state.wordEditor.Text())
	}
}

func TestWriteWordTableImageFixtureForVisualReview(t *testing.T) {
	output := os.Getenv("GLANCE_WORD_TABLE_FIXTURE_OUT")
	if output == "" {
		t.Skip("set GLANCE_WORD_TABLE_FIXTURE_OUT to emit a disposable visual-review DOCX")
	}
	word := syntheticWordTableDocument(t)
	defer word.Close()
	data, err := word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWordSampleFlowBuildsVisibleTextAndImageWidgets(t *testing.T) {
	source := os.Getenv("GLANCE_OFFICE_TEST_DOCX")
	if source == "" {
		t.Skip("set GLANCE_OFFICE_TEST_DOCX to inspect the external Word sample layout")
	}
	word, err := docx.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState(source)
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(1280, 820)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	dimensions := app.layoutWord(gtx)
	if dimensions.Size != image.Pt(1280, 820) {
		t.Fatalf("sample flow dimensions = %v", dimensions.Size)
	}
	if len(state.wordBlocks) == 0 {
		t.Fatal("sample has no projected Word blocks")
	}
	visibleTextEditors := 0
	for _, paragraph := range allWordParagraphs(state.wordBlocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && state.wordRunEditors[inline.Text.ID] != nil {
				visibleTextEditors++
			}
		}
	}
	if visibleTextEditors < 2 {
		t.Fatalf("sample viewport rendered only %d editable text runs; later paragraphs must flow after the first", visibleTextEditors)
	}
	var textAfterFirstUnsupportedBlock string
	seenUnsupportedBlock := false
	for _, block := range state.wordBlocks {
		if block.Kind == document.WordUnsupportedBlock {
			seenUnsupportedBlock = true
		}
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.Text == "聊天窗口" {
				if seenUnsupportedBlock {
					textAfterFirstUnsupportedBlock = inline.Text.ID
				}
			}
		}
	}
	if textAfterFirstUnsupportedBlock == "" || state.wordRunEditors[textAfterFirstUnsupportedBlock] == nil {
		t.Fatal("sample text after the first unsupported-content block did not flow into the initial viewport")
	}
	var firstImage *document.WordImage
	var firstImageBlock int
	for index, block := range state.wordBlocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Image != nil {
				firstImage, firstImageBlock = inline.Image, index
				break
			}
		}
		if firstImage != nil {
			break
		}
	}
	if firstImage == nil {
		t.Fatal("sample contains no supported inline image")
	}
	imageBytes, err := word.WordImageBytes(firstImage.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeWordPreview(imageBytes)
	if err != nil {
		t.Fatal(err)
	}
	state.wordImageCache.Put(firstImage.ID, compactWordPreview(decoded))
	state.wordListState.Position = layout.Position{First: firstImageBlock}
	ops.Reset()
	if dimensions := app.layoutWord(gtx); dimensions.Size != image.Pt(1280, 820) {
		t.Fatalf("scrolled sample flow dimensions = %v", dimensions.Size)
	}
	if state.wordObjectButtons[firstImage.ID] == nil {
		t.Fatalf("inline image %q was not laid out at block %d", firstImage.ID, firstImageBlock)
	}
	if len(state.wordObjectButtons) == 0 {
		t.Fatalf("sample layout created no visible text/image widgets (blocks=%d, first=%d)", len(state.wordBlocks), state.wordListState.Position.First)
	}
}

func syntheticWordTableDocument(t *testing.T) *docx.Document {
	t.Helper()
	var media bytes.Buffer
	pixels := image.NewNRGBA(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			c := color.NRGBA{R: 45, G: 110, B: 190, A: 255}
			if x > 60 {
				c = color.NRGBA{R: 240, G: 170, B: 55, A: 255}
			}
			pixels.SetNRGBA(x, y, c)
		}
	}
	if err := png.Encode(&media, pixels); err != nil {
		t.Fatal(err)
	}
	imageDrawing := `<w:r><w:drawing><wp:inline><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="Table image"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:blipFill><a:blip r:embed="rId1"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body><w:p><w:r><w:t>Document before table</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>first cell paragraph</w:t></w:r></w:p><w:p><w:r><w:t>inside cell</w:t></w:r>` + imageDrawing + `<w:r><w:t> end of cell paragraph</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>second cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:r><w:t>Document after table</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`
	relationships := `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/></Relationships>`
	contentTypes := `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
	var packageBytes bytes.Buffer
	writer := zip.NewWriter(&packageBytes)
	parts := map[string][]byte{
		"[Content_Types].xml":          []byte(contentTypes),
		"word/document.xml":            []byte(documentXML),
		"word/_rels/document.xml.rels": []byte(relationships),
		"word/media/image1.png":        media.Bytes(),
	}
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
	word, err := docx.OpenSnapshot(packageBytes.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return word
}
