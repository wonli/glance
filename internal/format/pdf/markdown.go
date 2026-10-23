package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"sort"
	"strings"

	"glance/internal/document"
	"glance/internal/format/markdowndoc"
)

// MarkdownImages owns frozen PNG bytes for the derived PDF scene.
type MarkdownImages map[string][]byte

func (images MarkdownImages) ImageBytes(id string) ([]byte, error) {
	data, ok := images[id]
	if !ok {
		return nil, fmt.Errorf("Markdown image %q is missing", id)
	}
	return data, nil
}

// MarkdownWordBlocks projects neutral Markdown into the shared pagination model.
// Unavailable images are explicit placeholders; formulas and unsafe failures
// remain errors. No source content is silently omitted.
func MarkdownWordBlocks(snapshot document.MarkdownSnapshot, resources map[string]image.Image, diagnostics map[string]string) ([]document.WordBlock, MarkdownImages, error) {
	for _, diagnostic := range snapshot.Diagnostics {
		switch diagnostic {
		case "HTML 以文本显示，不执行脚本", "行内 HTML 以文本显示，不执行脚本":
			// Literal HTML is already represented in neutral code/text spans.
		default:
			return nil, nil, fmt.Errorf("Markdown: %s", diagnostic)
		}
	}
	if len(diagnostics) > 0 {
		keys := make([]string, 0, len(diagnostics))
		for k := range diagnostics {
			if !strings.HasPrefix(k, "image:") && !strings.HasPrefix(k, "diagram:") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			return nil, nil, fmt.Errorf("公式或资源无法渲染：%s", diagnostics[keys[0]])
		}
	}
	images := MarkdownImages{}
	blocks := make([]document.WordBlock, 0, len(snapshot.Blocks))
	textInline := func(id, text string, style document.WordTextStyle) document.WordInline {
		return document.WordInline{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id, Text: text, Style: style, StyleSet: document.WordTextFontSizeSet | document.WordTextBoldSet | document.WordTextItalicSet | document.WordTextUnderlineSet | document.WordTextStrikeSet | document.WordTextFontFamilySet | document.WordTextFontColorSet}}
	}
	convertSpans := func(id string, spans []document.MarkdownSpan, base document.WordTextStyle, maxWidth float64) ([]document.WordInline, error) {
		inlines := make([]document.WordInline, 0, len(spans))
		for index, span := range spans {
			runID := fmt.Sprintf("%s-%d", id, index)
			if span.Kind == "task" {
				marker := "[ ] "
				if span.Checked {
					marker = "[x] "
				}
				inlines = append(inlines, textInline(runID, marker, base))
				continue
			}
			if span.Kind == "image" || span.Kind == "math" || span.Kind == "diagram" {
				key := markdowndoc.ResourceKey(span)
				img := resources[key]
				if img == nil {
					if span.Kind == "image" || span.Kind == "diagram" {
						reason := diagnostics[key]
						if reason == "" {
							reason = "图片未加载"
						}
						label := span.Text
						if label == "" {
							label = "未命名图片"
						}
						style := base
						style.FontColor = "805b24"
						placeholder := fmt.Sprintf("[图片无法加载：%s]\n%s\n原因：%s\n", label, span.Destination, reason)
						inlines = append(inlines, textInline(runID, placeholder, style))
						continue
					}
					return nil, fmt.Errorf("Markdown resource %q is missing", key)
				}
				logical := document.ResourceLogicalSize(img)
				pointsPerPixel := 0.75
				if span.Kind == "math" {
					pointsPerPixel = 0.5
				}
				w, h := float64(logical.X)*pointsPerPixel, float64(logical.Y)*pointsPerPixel
				if w <= 0 || h <= 0 {
					return nil, fmt.Errorf("Markdown resource %q has empty bounds", key)
				}
				scale := math.Min(1, math.Min(maxWidth/w, 650/h))
				w *= scale
				h *= scale
				if _, ok := images[key]; !ok {
					var data bytes.Buffer
					if err := png.Encode(&data, img); err != nil {
						return nil, err
					}
					images[key] = data.Bytes()
				}
				inlines = append(inlines, document.WordInline{Kind: document.WordImageInline, Image: &document.WordImage{ID: key, MediaType: "image/png", WidthEMU: int64(math.Round(w * 12700)), HeightEMU: int64(math.Round(h * 12700)), Resolved: true}})
				continue
			}
			style := base
			style.Bold = style.Bold || span.Bold
			style.Italic = style.Italic || span.Italic
			style.Strike = style.Strike || span.Strike
			if span.Kind == "code" {
				style.FontFamily = "monospace"
				if span.CodeColor != "" {
					style.FontColor = span.CodeColor
				}
			}
			if span.Destination != "" || span.LinkDestination != "" {
				style.Underline = true
				style.FontColor = "245db0"
			}
			inlines = append(inlines, textInline(runID, span.Text, style))
		}
		return inlines, nil
	}
	for index, b := range snapshot.Blocks {
		id := fmt.Sprintf("markdown-%d", index)
		if b.Kind == "table" {
			table := &document.WordTable{}
			columns := 0
			for _, row := range b.Rows {
				columns = max(columns, len(row))
			}
			for _, row := range b.TableSpans {
				columns = max(columns, len(row))
			}
			if columns == 0 {
				continue
			}
			width := int64(451.28 * 12700 / float64(columns))
			for range columns {
				table.Layout.GridColumnsEMU = append(table.Layout.GridColumnsEMU, width)
			}
			for r := range max(len(b.Rows), len(b.TableSpans)) {
				cells := document.WordTableRow{}
				for c := range columns {
					value := ""
					if r < len(b.Rows) && c < len(b.Rows[r]) {
						value = b.Rows[r][c]
					}
					cellID := fmt.Sprintf("%s-%d-%d", id, r, c)
					spans := []document.MarkdownSpan{{Text: value}}
					if r < len(b.TableSpans) && c < len(b.TableSpans[r]) {
						spans = b.TableSpans[r][c]
					}
					inlines, err := convertSpans(cellID, spans, document.WordTextStyle{FontSize: 11, Bold: r == 0}, math.Max(1, float64(width)/12700-12))
					if err != nil {
						return nil, nil, err
					}
					alignment := document.WordAlignLeft
					if c < len(b.TableAlignments) {
						switch b.TableAlignments[c] {
						case "center":
							alignment = document.WordAlignCenter
						case "right":
							alignment = document.WordAlignRight
						}
					}
					cells.Cells = append(cells.Cells, document.WordTableCell{ID: cellID, WidthEMU: width, Paragraphs: []document.WordParagraph{{ID: cellID, Alignment: alignment, Inlines: inlines}}})
				}
				table.Rows = append(table.Rows, cells)
			}
			blocks = append(blocks, document.WordBlock{ID: id, Kind: document.WordTableBlock, Table: table})
			continue
		}
		p := &document.WordParagraph{ID: id, Alignment: document.WordAlignLeft, Layout: document.WordParagraphLayout{AfterPT: 8, Set: document.WordParagraphAfterSet}}
		if b.Kind == "list" || b.Kind == "quote" || (b.Kind == "paragraph" && b.Level > 0) {
			p.LeftIndentEMU = int64(max(0, b.Level)) * 18 * 12700
		}
		size := 11.0
		switch b.Kind {
		case "heading":
			size = float64(26 - min(6, max(1, b.Level))*2)
			p.Layout.KeepWithNext = true
			p.Layout.Set |= document.WordParagraphKeepWithNextSet
		case "paragraph", "list", "quote", "code", "math", "diagram":
		case "rule":
			p.Layout.BottomBorder = document.WordParagraphBorder{Style: "single", SizePT: 0.5, Color: "cccccc"}
			p.Layout.Set |= document.WordParagraphBottomBorderSet
		default:
			return nil, nil, fmt.Errorf("unsupported Markdown block %q", b.Kind)
		}
		spans := b.Spans
		if b.Kind == "diagram" {
			spans = []document.MarkdownSpan{markdowndoc.DiagramSpan(b)}
		} else if b.Kind == "math" {
			spans = []document.MarkdownSpan{{Kind: "math", Text: b.Text, DisplayMath: true}}
		} else if b.Kind == "code" {
			spans = []document.MarkdownSpan{{Text: b.Text}}
			if len(b.CodeSpans) > 0 {
				spans = b.CodeSpans
			}
		} else if len(spans) == 0 && b.Text != "" {
			spans = []document.MarkdownSpan{{Text: b.Text}}
		}
		if b.Prefix != "" {
			p.Inlines = append(p.Inlines, textInline(id+"-prefix", b.Prefix, document.WordTextStyle{FontSize: size}))
		}
		style := document.WordTextStyle{FontSize: size, Bold: b.Kind == "heading"}
		if b.Kind == "code" {
			style.FontFamily = "monospace"
		}
		inlines, err := convertSpans(id, spans, style, math.Max(1, 451.28-float64(p.LeftIndentEMU)/12700))
		if err != nil {
			return nil, nil, err
		}
		p.Inlines = append(p.Inlines, inlines...)
		blocks = append(blocks, document.WordBlock{ID: id, Kind: document.WordParagraphBlock, Paragraph: p})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, document.WordBlock{ID: "markdown-empty", Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{ID: "markdown-empty"}})
	}
	return blocks, images, nil
}
