package pdf

import (
	"bytes"
	"context"
	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/wordlayout"
	"golang.org/x/image/font/gofont/goregular"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestMarkdownPDFProjection(t *testing.T) {
	snapshot := document.MarkdownSnapshot{Blocks: []document.MarkdownBlock{
		{Kind: "heading", Level: 1, Spans: []document.MarkdownSpan{{Text: "中文标题"}}},
		{Kind: "paragraph", Spans: []document.MarkdownSpan{{Text: "正文", Bold: true}, {Kind: "image", Destination: "a.png"}, {Kind: "math", Text: "x^2"}}},
		{Kind: "table", Rows: [][]string{{"名称", "值"}, {"中文", "42"}}},
	}}
	blocks, images, err := MarkdownWordBlocks(snapshot, map[string]image.Image{"image:a.png": image.NewRGBA(image.Rect(0, 0, 30, 20)), "math:x^2": image.NewRGBA(image.Rect(0, 0, 20, 10))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 3 || blocks[0].Paragraph.Inlines[0].Text.Text != "中文标题" || !blocks[1].Paragraph.Inlines[0].Text.Style.Bold || len(blocks[2].Table.Rows) != 2 {
		t.Fatalf("projection = %#v", blocks)
	}
	if len(images) != 2 {
		t.Fatalf("images=%d", len(images))
	}
	for key := range images {
		if data, err := images.ImageBytes(key); err != nil || len(data) == 0 {
			t.Fatalf("image %q: %v", key, err)
		}
	}
}

func TestMarkdownPDFSemantics(t *testing.T) {
	snapshot := document.MarkdownSnapshot{Blocks: []document.MarkdownBlock{
		{Kind: "paragraph", Spans: []document.MarkdownSpan{{Text: "旧内容", Strike: true}, {Kind: "math", Text: "x"}, {Kind: "image", Destination: "a.png", LinkDestination: "https://example.com"}}},
		{Kind: "math", Text: "x"},
		{Kind: "list", Level: 3, Prefix: "• ", Text: "嵌套列表"},
		{Kind: "quote", Level: 2, Prefix: "> ", Text: "引用"},
		{Kind: "table", Rows: [][]string{{"左", "中", "右"}}, TableAlignments: []string{"left", "center", "right"}},
		{Kind: "code", Text: "$x$ ~~literal~~", Spans: []document.MarkdownSpan{{Kind: "math", Text: "wrong"}}},
	}}
	resources := map[string]image.Image{"math:x": image.NewRGBA(image.Rect(0, 0, 100, 40)), "math:display:x": image.NewRGBA(image.Rect(0, 0, 140, 60)), "image:a.png": image.NewRGBA(image.Rect(0, 0, 100, 40))}
	blocks, _, err := MarkdownWordBlocks(snapshot, resources, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := blocks[0].Paragraph
	if !p.Inlines[0].Text.Style.Strike || p.Inlines[0].Text.StyleSet&document.WordTextStrikeSet == 0 {
		t.Fatal("strike lost")
	}
	if p.Inlines[1].Image.WidthEMU != 50*12700 || p.Inlines[2].Image.WidthEMU != 75*12700 {
		t.Fatal("math or image resolution scale incorrect")
	}
	if blocks[1].Paragraph.Alignment != document.WordAlignLeft || blocks[1].Paragraph.Inlines[0].Image.ID != "math:display:x" {
		t.Fatal("display formula lost")
	}
	if blocks[2].Paragraph.LeftIndentEMU <= blocks[3].Paragraph.LeftIndentEMU || blocks[3].Paragraph.LeftIndentEMU <= 0 {
		t.Fatal("nesting indentation lost")
	}
	for i, alignment := range []document.WordAlignment{document.WordAlignLeft, document.WordAlignCenter, document.WordAlignRight} {
		if blocks[4].Table.Rows[0].Cells[i].Paragraphs[0].Alignment != alignment {
			t.Fatal("table alignment lost")
		}
	}
	if run := blocks[5].Paragraph.Inlines[0]; run.Kind != document.WordTextInline || run.Text.Text != "$x$ ~~literal~~" {
		t.Fatal("code rich spans were interpreted")
	}
}

func TestMarkdownPDFStrikeDrawsAcrossText(t *testing.T) {
	resources := testResources{fonts: map[string][]byte{"font": goregular.TTF}}
	result := wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 100, HeightPT: 100}, Text: []wordlayout.TextFragment{{Text: "A", RuneEnd: 1, XPT: 20, BaselineYPT: 50, Style: document.WordTextStyle{FontSize: 20, FontColor: "FF0000", Strike: true}, Glyphs: []wordlayout.PositionedGlyph{{FontKey: "font", RuneEnd: 1, XPT: 20, YPT: 50, AdvanceXPT: 12}}}}}}}
	var output bytes.Buffer
	if err := WriteWordPDF(context.Background(), &output, result, resources, resources); err != nil {
		t.Fatal(err)
	}
	reader, err := gopdf.NewReader(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	rendered, _, err := reader.RenderPageDetail(0, gopdf.RenderOpts{DPI: 144, IncludeVector: true})
	if err != nil {
		t.Fatal(err)
	}
	pixel := color.NRGBAModel.Convert(rendered.At(50, 88)).(color.NRGBA)
	if pixel.R < 200 || pixel.G > 80 || pixel.B > 80 {
		t.Fatalf("strike pixel=%v", pixel)
	}
}

func TestMarkdownPDFProjectionRejectsMissingContent(t *testing.T) {
	for _, snapshot := range []document.MarkdownSnapshot{
		{Diagnostics: []string{"unsupported HTML"}},
		{Blocks: []document.MarkdownBlock{{Kind: "paragraph", Spans: []document.MarkdownSpan{{Kind: "math", Text: "missing"}}}}},
	} {
		if _, _, err := MarkdownWordBlocks(snapshot, nil, nil); err == nil {
			t.Fatal("expected incomplete export rejection")
		}
	}
}

func TestMarkdownPDFMissingImageRetainsPlaceholderAndFollowingContent(t *testing.T) {
	snapshot := document.MarkdownSnapshot{Blocks: []document.MarkdownBlock{
		{Kind: "paragraph", Spans: []document.MarkdownSpan{{Kind: "image", Text: "封面", Destination: "https://example.com/missing.png"}, {Text: "后续正文"}}},
		{Kind: "table", Rows: [][]string{{"图片"}}, TableSpans: [][][]document.MarkdownSpan{{{{Kind: "image", Text: "表格图", Destination: "missing.png"}}}}},
	}}
	blocks, _, err := MarkdownWordBlocks(snapshot, nil, map[string]string{"image:https://example.com/missing.png": "HTTP 400", "image:missing.png": "文件不存在"})
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, inline := range blocks[0].Paragraph.Inlines {
		if inline.Text != nil {
			text += inline.Text.Text
		}
	}
	for _, inline := range blocks[1].Table.Rows[0].Cells[0].Paragraphs[0].Inlines {
		if inline.Text != nil {
			text += inline.Text.Text
		}
	}
	for _, want := range []string{"图片无法加载", "封面", "https://example.com/missing.png", "HTTP 400", "后续正文", "表格图", "missing.png"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q from %q", want, text)
		}
	}
}

func TestMarkdownPDFTablePreservesResourcesAndStyles(t *testing.T) {
	snapshot := document.MarkdownSnapshot{Blocks: []document.MarkdownBlock{{Kind: "table", Rows: [][]string{{"图片", "公式"}}, TableSpans: [][][]document.MarkdownSpan{{{{Kind: "image", Destination: "a.png"}}, {{Text: "中文", Bold: true}, {Kind: "math", Text: "x^2"}}}}}}}
	resources := map[string]image.Image{"image:a.png": image.NewRGBA(image.Rect(0, 0, 1000, 200)), "math:x^2": image.NewRGBA(image.Rect(0, 0, 30, 20))}
	blocks, images, err := MarkdownWordBlocks(snapshot, resources, nil)
	if err != nil {
		t.Fatal(err)
	}
	cells := blocks[0].Table.Rows[0].Cells
	if len(images) != 2 || cells[0].Paragraphs[0].Inlines[0].Kind != document.WordImageInline || cells[1].Paragraphs[0].Inlines[1].Kind != document.WordImageInline || !cells[1].Paragraphs[0].Inlines[0].Text.Style.Bold {
		t.Fatal("table content flattened")
	}
	if cells[0].Paragraphs[0].Inlines[0].Image.WidthEMU > cells[0].WidthEMU {
		t.Fatal("table image exceeds cell width")
	}
	if _, _, err := MarkdownWordBlocks(snapshot, nil, nil); err == nil {
		t.Fatal("table image silently dropped")
	}
	if _, _, err := MarkdownWordBlocks(snapshot, resources, map[string]string{"math:x^2": "unsupported formula"}); err == nil {
		t.Fatal("table formula silently dropped")
	}
}

func TestMarkdownPDFRetainsLiteralHTMLAndCodeColors(t *testing.T) {
	snapshot, err := markdowndoc.Parse("正文 <b>原文</b>\n\n```javascript\nconst answer = 42;\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	blocks, _, err := MarkdownWordBlocks(snapshot, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var content string
	colored := false
	for _, b := range blocks {
		if b.Paragraph == nil {
			continue
		}
		for _, s := range b.Paragraph.Inlines {
			if s.Text != nil {
				content += s.Text.Text
				if s.Text.Style.FontColor != "" {
					colored = true
				}
			}
		}
	}
	if !strings.Contains(content, "<b>原文</b>") || !strings.Contains(content, "const answer = 42;") || !colored {
		t.Fatalf("HTML/code loss or no color: %q", content)
	}
}
