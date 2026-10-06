package markdowndoc

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"glance/internal/document"
	"html"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func Parse(source string) (document.MarkdownSnapshot, error) {
	p := document.MarkdownSnapshot{Source: source}
	if !utf8.ValidString(source) {
		return p, fmt.Errorf("Markdown is not valid UTF-8")
	}
	if len(source) > 32<<20 {
		return p, fmt.Errorf("Markdown preview exceeds 32 MiB")
	}
	raw := []byte(strings.TrimPrefix(source, "\ufeff"))
	baseRune := 0
	if strings.HasPrefix(source, "\ufeff") {
		baseRune = 1
	}
	type lineOffset struct{ byteOffset, runeOffset int }
	lines := []lineOffset{{0, baseRune}}
	runeIndex := baseRune
	for i, c := range string(raw) {
		runeIndex++
		if c == '\n' {
			lines = append(lines, lineOffset{i + 1, runeIndex})
		}
	}
	tree := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote)).Parser().Parse(text.NewReader(raw))
	_ = ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := n.(*ast.RawHTML); entering && ok {
			p.Diagnostics = append(p.Diagnostics, "行内 HTML 以文本显示，不执行脚本")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	offset := func(n ast.Node) int {
		if n.Lines().Len() == 0 {
			return 0
		}
		b := n.Lines().At(0).Start
		index := sort.Search(len(lines), func(i int) bool { return lines[i].byteOffset > b }) - 1
		return lines[max(0, index)].runeOffset
	}
	byteRuneOffset := func(b int) int {
		index := sort.Search(len(lines), func(i int) bool { return lines[i].byteOffset > b }) - 1
		index = max(0, index)
		return lines[index].runeOffset + utf8.RuneCount(raw[lines[index].byteOffset:b])
	}
	var blocks func(ast.Node, int, string)
	blocks = func(parent ast.Node, depth int, prefix string) {
		for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
			b := document.MarkdownBlock{Offset: offset(n), Level: depth, Prefix: prefix}
			switch v := n.(type) {
			case *ast.Heading:
				b.Kind = "heading"
				b.Level = v.Level
				b.Spans = inline(n, raw, byteRuneOffset)
				b.Text = plain(b.Spans)
				p.Headings = append(p.Headings, document.MarkdownHeading{Title: b.Text, Level: v.Level, Offset: b.Offset, Block: len(p.Blocks)})
			case *ast.Paragraph, *ast.TextBlock:
				b.Kind = "paragraph"
				if prefix == "> " {
					b.Kind = "quote"
				} else if _, item := parent.(*ast.ListItem); item || prefix != "" {
					b.Kind = "list"
				}
				b.Spans = inline(n, raw, byteRuneOffset)
				b.Text = plain(b.Spans)
				value := string(n.Lines().Value(raw))
				trim := strings.TrimSpace(value)
				if strings.HasPrefix(trim, "$$") && strings.HasSuffix(trim, "$$") && len(trim) > 4 {
					b.Kind = "math"
					b.Text = strings.TrimSpace(trim[2 : len(trim)-2])
					b.Spans = nil
					leading := len(value) - len(strings.TrimLeft(value, " \t\r\n"))
					trailing := len(value) - len(strings.TrimRight(value, " \t\r\n"))
					b.SourceStart = byteRuneOffset(n.Lines().At(0).Start + leading)
					b.SourceEnd = byteRuneOffset(n.Lines().At(n.Lines().Len()-1).Stop - trailing)
				}
			case *ast.FencedCodeBlock:
				b.Kind = "code"
				b.Text = string(v.Lines().Value(raw))
				b.Language = string(v.Language(raw))
			case *ast.CodeBlock:
				b.Kind = "code"
				b.Text = string(v.Lines().Value(raw))
			case *ast.ThematicBreak:
				b.Kind = "rule"
			case *ast.List:
				number := v.Start
				for item := n.FirstChild(); item != nil; item = item.NextSibling() {
					marker := "• "
					if v.IsOrdered() {
						marker = fmt.Sprintf("%d. ", number)
						number++
					}
					blocks(item, depth+1, marker)
				}
				continue
			case *ast.Blockquote:
				blocks(n, depth+1, "> ")
				continue
			case *extast.Footnote:
				first := len(p.Blocks)
				blocks(n, 0, "")
				if first < len(p.Blocks) {
					p.Blocks[first].Anchor = fmt.Sprintf("footnote-%d", v.Index)
					p.Blocks[first].Prefix = fmt.Sprintf("[%d] ", v.Index)
				}
				continue
			case *extast.Table:
				b.Kind = "table"
				for _, alignment := range v.Alignments {
					value := "left"
					if alignment == extast.AlignCenter {
						value = "center"
					}
					if alignment == extast.AlignRight {
						value = "right"
					}
					b.TableAlignments = append(b.TableAlignments, value)
				}
				for row := n.FirstChild(); row != nil; row = row.NextSibling() {
					cells := []string{}
					cellSpans := [][]document.MarkdownSpan{}
					for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
						spans := inline(cell, raw, byteRuneOffset)
						cells = append(cells, plain(spans))
						cellSpans = append(cellSpans, spans)
					}
					b.Rows = append(b.Rows, cells)
					b.TableSpans = append(b.TableSpans, cellSpans)
				}
			case *ast.HTMLBlock:
				b.Kind = "code"
				b.Text = string(n.Lines().Value(raw))
				p.Diagnostics = append(p.Diagnostics, "HTML 以文本显示，不执行脚本")
			default:
				blocks(n, depth, prefix)
				continue
			}
			if b.Kind == "code" {
				b.CodeSpans = HighlightCode(b.Text, b.Language)
			}
			p.Blocks = append(p.Blocks, b)
			if _, item := parent.(*ast.ListItem); item {
				prefix = ""
			}
		}
	}
	blocks(tree, 0, "")
	p.Characters = utf8.RuneCountInString(strings.TrimPrefix(source, "\ufeff"))
	p.Words = len(strings.Fields(source))
	for _, r := range source {
		if unicode.Is(unicode.Han, r) {
			p.Words++
		}
	}
	p.ReadingMinutes = max(1, (p.Words+299)/300)
	return p, nil
}
func plain(spans []document.MarkdownSpan) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}
func inline(n ast.Node, source []byte, runeOffset func(int) int) []document.MarkdownSpan {
	out := []document.MarkdownSpan{}
	var walk func(ast.Node, bool, bool, bool, string)
	walk = func(parent ast.Node, bold, italic, strike bool, dest string) {
		for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.Text:
				out = append(out, document.MarkdownSpan{Text: string(v.Segment.Value(source)), SourceStart: runeOffset(v.Segment.Start), SourceEnd: runeOffset(v.Segment.Stop), Bold: bold, Italic: italic, Strike: strike, Destination: dest})
				if v.SoftLineBreak() || v.HardLineBreak() {
					out = append(out, document.MarkdownSpan{Text: "\n"})
				}
			case *ast.String:
				out = append(out, document.MarkdownSpan{Text: string(v.Value), Bold: bold, Italic: italic, Strike: strike, Destination: dest})
			case *ast.Emphasis:
				walk(c, bold || v.Level == 2, italic || v.Level == 1, strike, dest)
			case *extast.Strikethrough:
				walk(c, bold, italic, true, dest)
			case *ast.Link:
				walk(c, bold, italic, strike, markdownPlainText(string(v.Destination)))
			case *ast.AutoLink:
				out = append(out, document.MarkdownSpan{Text: string(v.Label(source)), Destination: string(v.URL(source))})
			case *ast.Image:
				out = append(out, document.MarkdownSpan{Kind: "image", Text: markdownPlainText(string(v.Text(source))), Destination: markdownPlainText(string(v.Destination)), LinkDestination: dest})
			case *ast.CodeSpan:
				out = append(out, document.MarkdownSpan{Kind: "code", Text: string(v.Text(source))})
			case *ast.RawHTML:
				var literal strings.Builder
				for i := 0; i < v.Segments.Len(); i++ {
					segment := v.Segments.At(i)
					literal.Write(segment.Value(source))
				}
				out = append(out, document.MarkdownSpan{Kind: "code", Text: literal.String()})
			case *extast.TaskCheckBox:
				label := "☐ "
				if v.IsChecked {
					label = "☑ "
				}
				out = append(out, document.MarkdownSpan{Kind: "task", Text: label, Checked: v.IsChecked, SourceStart: runeOffset(parent.Lines().At(0).Start) + 1, SourceEnd: runeOffset(parent.Lines().At(0).Start) + 2})
			case *extast.FootnoteLink:
				out = append(out, document.MarkdownSpan{Text: fmt.Sprintf("[%d]", v.Index), Destination: fmt.Sprintf("#footnote-%d", v.Index)})
			case *extast.FootnoteBacklink:
				// Backlinks are navigation chrome, not definition content.
			default:
				walk(c, bold, italic, strike, dest)
			}
		}
	}
	walk(n, false, false, false, "")
	// Merge adjacent text nodes before finding inline math (AST splits punctuation).
	merged := []document.MarkdownSpan{}
	for _, s := range out {
		if len(merged) > 0 && s.Kind == "" {
			last := &merged[len(merged)-1]
			if last.SourceEnd == s.SourceStart && last.Kind == s.Kind && last.Bold == s.Bold && last.Italic == s.Italic && last.Strike == s.Strike && last.Destination == s.Destination {
				last.Text += s.Text
				last.SourceEnd = s.SourceEnd
				continue
			}
		}
		merged = append(merged, s)
	}
	out = nil
	for _, s := range merged {
		if s.Kind != "" {
			out = append(out, s)
			continue
		}
		out = append(out, splitMarkdownMath(s)...)
	}
	return out
}

// Decode ordinary text after recognizing math in the source. Entity-encoded
// and escaped dollar signs remain literal; code nodes bypass this conversion.
func markdownPlainText(value string) string {
	var encoded bytes.Buffer
	writer := bufio.NewWriter(&encoded)
	// Goldmark's writer applies CommonMark escape/entity ordering. Decode its
	// escaped output to neutral text; no HTML is interpreted or executed.
	gmhtml.DefaultWriter.Write(writer, []byte(value))
	_ = writer.Flush()
	return html.UnescapeString(encoded.String())
}

func escapedMarkdownDollar(value string, index int) bool {
	backslashes := 0
	for index > 0 && value[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 != 0
}

func splitMarkdownMath(span document.MarkdownSpan) []document.MarkdownSpan {
	value := span.Text
	out := []document.MarkdownSpan{}
	plainStart := 0
	for i := 0; i < len(value); i++ {
		if value[i] != '$' || escapedMarkdownDollar(value, i) {
			continue
		}
		if i+1 >= len(value) || value[i+1] == '$' {
			continue
		}
		next, _ := utf8.DecodeRuneInString(value[i+1:])
		if unicode.IsSpace(next) {
			continue
		}
		end := -1
		for j := i + 1; j < len(value); j++ {
			if value[j] == '\n' {
				break
			}
			if value[j] != '$' || escapedMarkdownDollar(value, j) {
				continue
			}
			previous, _ := utf8.DecodeLastRuneInString(value[i+1 : j])
			if unicode.IsSpace(previous) || j == i+1 {
				break
			}
			// "$5 and $10" is currency, not a mathematical expression.
			if j+1 < len(value) && value[j+1] >= '0' && value[j+1] <= '9' {
				break
			}
			end = j
			break
		}
		if end < 0 {
			continue
		}
		if i > plainStart {
			s := span
			s.Text = markdownPlainText(value[plainStart:i])
			out = append(out, s)
		}
		s := span
		s.Kind, s.Text = "math", value[i+1:end]
		s.SourceStart = span.SourceStart + utf8.RuneCountInString(value[:i])
		s.SourceEnd = span.SourceStart + utf8.RuneCountInString(value[:end+1])
		out = append(out, s)
		i, plainStart = end, end+1
	}
	if plainStart < len(value) {
		span.Text = markdownPlainText(value[plainStart:])
		out = append(out, span)
	}
	return out
}
