package desktopui

import (
	"fmt"
	"math"
	"strings"

	"github.com/go-text/typesetting/segmenter"
	"glance/internal/document"
	"glance/internal/fontmanager/emojifont"
	wordpdf "glance/internal/format/pdf"
	"glance/internal/wordlayout"
)

// markdownPDFFallbackBitmapGlyphs preserves Markdown emoji and symbols when
// the system's vector font scanner cannot provide an outline for them. Gio has
// a separate color emoji path; PDF needs those glyphs as ordinary image runs.
func markdownPDFFallbackBitmapGlyphs(blocks []document.WordBlock, images wordpdf.MarkdownImages, shaper *wordTextShaper) error {
	for blockIndex := range blocks {
		block := &blocks[blockIndex]
		if block.Paragraph != nil {
			if err := markdownPDFFallbackParagraph(block.Paragraph, images, shaper); err != nil {
				return err
			}
		}
		if block.Table == nil {
			continue
		}
		for rowIndex := range block.Table.Rows {
			for cellIndex := range block.Table.Rows[rowIndex].Cells {
				cell := &block.Table.Rows[rowIndex].Cells[cellIndex]
				for paragraphIndex := range cell.Paragraphs {
					if err := markdownPDFFallbackParagraph(&cell.Paragraphs[paragraphIndex], images, shaper); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func markdownPDFFallbackParagraph(paragraph *document.WordParagraph, images wordpdf.MarkdownImages, shaper *wordTextShaper) error {
	if paragraph == nil {
		return nil
	}
	var rewritten []document.WordInline
	for inlineIndex, inline := range paragraph.Inlines {
		if inline.Kind != document.WordTextInline || inline.Text == nil || inline.Text.Text == "" {
			if rewritten != nil {
				rewritten = append(rewritten, inline)
			}
			continue
		}
		needsBitmapFallback := markdownPDFRunMayNeedBitmapFallback(inline.Text.Text)
		_, shapeErr := shaper.Shape(inline.Text.Text, inline.Text.Style)
		if !needsBitmapFallback && shapeErr == nil {
			if rewritten != nil {
				rewritten = append(rewritten, inline)
			}
			continue
		}
		replacements, changed, err := markdownPDFFallbackRun(inline.Text, images, shaper, shapeErr != nil)
		if err != nil {
			return err
		}
		if !changed {
			if rewritten != nil {
				rewritten = append(rewritten, inline)
			}
			continue
		}
		if rewritten == nil {
			rewritten = append([]document.WordInline(nil), paragraph.Inlines[:inlineIndex]...)
		}
		rewritten = append(rewritten, replacements...)
	}
	if rewritten != nil {
		paragraph.Inlines = rewritten
	}
	return nil
}

func markdownPDFFallbackRun(run *document.WordTextRun, images wordpdf.MarkdownImages, shaper *wordTextShaper, inspectUnsupported bool) ([]document.WordInline, bool, error) {
	runes := []rune(run.Text)
	var graphemes segmenter.Segmenter
	graphemes.Init(runes)
	iterator := graphemes.GraphemeIterator()
	var out []document.WordInline
	var text strings.Builder
	part := 0
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		copy := *run
		copy.ID = fmt.Sprintf("%s-pdf-%d", run.ID, part)
		copy.Text = text.String()
		part++
		out = append(out, document.WordInline{Kind: document.WordTextInline, Text: &copy})
		text.Reset()
	}
	changed := false
	for iterator.Next() {
		cluster := iterator.Grapheme().Text
		value := string(cluster)
		if !markdownPDFClusterMayNeedBitmapFallback(cluster) {
			if !inspectUnsupported {
				text.WriteString(value)
				continue
			}
			if _, err := shaper.Shape(value, run.Style); err == nil {
				text.WriteString(value)
				continue
			}
		}
		if markdownPDFClusterMayNeedBitmapFallback(cluster) {
			bitmapText := value
			if bitmapText == "✓" {
				bitmapText = "✔"
			}
			if data, width, height, ok := emojifont.BitmapPNGForText(bitmapText); ok {
				changed = true
				flushText()
				id := markdownPDFEmojiResourceID(cluster)
				images[id] = data
				out = append(out, markdownPDFBitmapInline(run.Style, id, width, height))
				continue
			}
		}
		var imagesForCluster []document.WordInline
		var remaining strings.Builder
		for _, r := range cluster {
			if markdownPDFEmojiIgnorable(r) {
				continue
			}
			if !markdownPDFMayBeBitmapGlyph(r) {
				remaining.WriteRune(r)
				continue
			}
			glyphRune := markdownPDFBitmapGlyphRune(r)
			data, width, height, ok := emojifont.BitmapPNG(glyphRune)
			if !ok {
				remaining.WriteRune(r)
				continue
			}
			id := fmt.Sprintf("markdown-emoji-%X", r)
			images[id] = data
			imagesForCluster = append(imagesForCluster, markdownPDFBitmapInline(run.Style, id, width, height))
		}
		if len(imagesForCluster) == 0 {
			visible := markdownPDFVisibleFallbackText(cluster)
			changed = changed || visible != value
			text.WriteString(visible)
			continue
		}
		changed = true
		flushText()
		if remaining.Len() > 0 {
			text.WriteString(remaining.String())
			flushText()
		}
		out = append(out, imagesForCluster...)
	}
	flushText()
	if !changed {
		return nil, false, nil
	}
	return out, true, nil
}

func markdownPDFBitmapInline(style document.WordTextStyle, id string, width, height int) document.WordInline {
	fontSize := style.FontSize
	if fontSize <= 0 {
		fontSize = wordlayout.DefaultFontSizePT
	}
	heightPT := fontSize
	widthPT := heightPT * float64(width) / float64(height)
	return document.WordInline{Kind: document.WordImageInline, Image: &document.WordImage{
		ID: id, MediaType: "image/png", WidthEMU: int64(math.Round(widthPT * 12700)), HeightEMU: int64(math.Round(heightPT * 12700)), Resolved: true,
	}}
}

func markdownPDFEmojiResourceID(cluster []rune) string {
	var id strings.Builder
	id.WriteString("markdown-emoji")
	for _, r := range cluster {
		fmt.Fprintf(&id, "-%X", r)
	}
	return id.String()
}

func markdownPDFBitmapGlyphRune(r rune) rune {
	// The common text-presentation check mark has the same visual intent as
	// the bundled color check mark, which has an embeddable bitmap.
	if r == '✓' {
		return '✔'
	}
	return r
}

func markdownPDFRunMayNeedBitmapFallback(value string) bool {
	for _, r := range value {
		if markdownPDFEmojiIgnorable(r) {
			return true
		}
		if markdownPDFMayBeBitmapGlyph(r) && (markdownPDFBitmapGlyphRune(r) != r || markdownPDFHasBitmapGlyph(r)) {
			return true
		}
	}
	return false
}

func markdownPDFClusterMayNeedBitmapFallback(cluster []rune) bool {
	for _, r := range cluster {
		if markdownPDFEmojiIgnorable(r) {
			return true
		}
		if markdownPDFMayBeBitmapGlyph(r) && (markdownPDFBitmapGlyphRune(r) != r || markdownPDFHasBitmapGlyph(r)) {
			return true
		}
	}
	return false
}

func markdownPDFHasBitmapGlyph(r rune) bool {
	_, _, _, ok := emojifont.BitmapPNG(r)
	return ok
}

func markdownPDFMayBeBitmapGlyph(r rune) bool {
	return r == '✓' || r >= 0x2300 && r <= 0x27ff || r >= 0x1f000 && r <= 0x1faff
}

func markdownPDFEmojiIgnorable(r rune) bool {
	return r == 0x200d || r == 0xfe0e || r == 0xfe0f || r == 0x20e3 || r >= 0x1f3fb && r <= 0x1f3ff
}

func markdownPDFVisibleFallbackText(cluster []rune) string {
	var visible strings.Builder
	for _, r := range cluster {
		if markdownPDFEmojiIgnorable(r) {
			continue
		}
		if r >= 0x1f1e6 && r <= 0x1f1ff {
			visible.WriteRune('A' + (r - 0x1f1e6))
			continue
		}
		visible.WriteRune(r)
	}
	return visible.String()
}
