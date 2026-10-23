package emojifont

import "github.com/go-text/typesetting/segmenter"

// TextRun identifies grapheme clusters that should use the bundled color face.
// Keeping the marker on whole clusters preserves variation selectors and ZWJ
// sequences when a renderer splits text by syntax color or typography.
type TextRun struct {
	Text       string
	ColorEmoji bool
}

// SplitText groups ordinary graphemes separately from emoji-presentation
// clusters. Renderers can keep their local family for ordinary text and route
// only marked clusters to Family, even in monospace runs that contain a text
// glyph for the same base character.
func SplitText(value string) []TextRun {
	if value == "" {
		return nil
	}
	hasEmoji := false
	for _, r := range value {
		if emojiPresentationCodePoint(r) {
			hasEmoji = true
			break
		}
	}
	if !hasEmoji {
		return []TextRun{{Text: value}}
	}
	var graphemes segmenter.Segmenter
	graphemes.Init([]rune(value))
	iterator := graphemes.GraphemeIterator()
	var runs []TextRun
	for iterator.Next() {
		cluster := string(iterator.Grapheme().Text)
		colorEmoji := colorEmojiCluster(iterator.Grapheme().Text)
		if len(runs) > 0 && runs[len(runs)-1].ColorEmoji == colorEmoji {
			runs[len(runs)-1].Text += cluster
			continue
		}
		runs = append(runs, TextRun{Text: cluster, ColorEmoji: colorEmoji})
	}
	return runs
}

func colorEmojiCluster(cluster []rune) bool {
	for _, r := range cluster {
		if emojiPresentationCodePoint(r) {
			return true
		}
	}
	return false
}

func emojiPresentationCodePoint(r rune) bool {
	if r == 0xfe0f || r == 0x200d || r == 0x20e3 || r >= 0x1f1e6 && r <= 0x1f1ff || r >= 0x1f3fb && r <= 0x1f3ff {
		return true
	}
	// Emoji_Presentation symbols in the BMP can also have monochrome glyphs
	// in code fonts. Select color glyphs for their default emoji presentation.
	if r >= 0x231a && r <= 0x231b || r >= 0x23e9 && r <= 0x23ec || r == 0x23f0 || r == 0x23f3 ||
		r >= 0x25fd && r <= 0x25fe || r >= 0x2614 && r <= 0x2615 || r >= 0x2648 && r <= 0x2653 ||
		r == 0x267f || r == 0x2693 || r == 0x26a1 || r >= 0x26aa && r <= 0x26ab || r >= 0x26bd && r <= 0x26be ||
		r >= 0x26c4 && r <= 0x26c5 || r == 0x26ce || r == 0x26d4 || r == 0x26ea || r >= 0x26f2 && r <= 0x26f3 ||
		r == 0x26f5 || r == 0x26fa || r == 0x26fd || r == 0x2705 || r >= 0x270a && r <= 0x270b || r == 0x2728 ||
		r == 0x274c || r == 0x274e || r >= 0x2753 && r <= 0x2755 || r == 0x2757 || r >= 0x2795 && r <= 0x2797 ||
		r == 0x27b0 || r == 0x27bf || r >= 0x2b1b && r <= 0x2b1c || r == 0x2b50 || r == 0x2b55 {
		return true
	}
	// Emoji pictographs outside the BMP use emoji presentation by default.
	return r >= 0x1f000 && r <= 0x1faff
}
