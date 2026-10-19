package desktopui

import (
	"fmt"
	"glance/internal/document"
	"image/color"
	"strconv"
	"strings"
)

func selectedWordRun(blocks []document.WordBlock, id string) (document.WordTextRun, bool) {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == id {
				return *inline.Text, true
			}
		}
	}
	return document.WordTextRun{}, false
}

func selectedWordParagraph(blocks []document.WordBlock, id string) (document.WordParagraph, bool) {
	for _, paragraph := range allWordParagraphs(blocks) {
		if paragraph.ID == id {
			return paragraph, true
		}
	}
	return document.WordParagraph{}, false
}

func selectedWordParagraphForRun(blocks []document.WordBlock, runID string) (document.WordParagraph, bool) {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				return paragraph, true
			}
		}
	}
	return document.WordParagraph{}, false
}

func wordParagraphAfter(blocks []document.WordBlock, paragraphID string) (document.WordParagraph, bool) {
	paragraphs := allWordParagraphs(blocks)
	for index, paragraph := range paragraphs {
		if paragraph.ID == paragraphID && index+1 < len(paragraphs) {
			return paragraphs[index+1], true
		}
	}
	return document.WordParagraph{}, false
}

func selectedWordImage(blocks []document.WordBlock, id string) bool {
	return findWordImage(blocks, id) != nil
}

func findWordImage(blocks []document.WordBlock, id string) *document.WordImage {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == id {
				copy := *inline.Image
				return &copy
			}
		}
	}
	return nil
}

func allWordParagraphs(blocks []document.WordBlock) []document.WordParagraph {
	var paragraphs []document.WordParagraph
	for _, block := range blocks {
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, *block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
		}
	}
	return paragraphs
}

func parseHexColor(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBA{
		R: uint8(v >> 16),
		G: uint8((v >> 8) & 0xFF),
		B: uint8(v & 0xFF),
		A: 255,
	}, nil
}
