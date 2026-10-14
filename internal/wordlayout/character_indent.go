package wordlayout

import (
	"context"
	"glance/internal/document"
	"math"
)

func resolveCharacterIndentUnits(ctx context.Context, shaper TextShaper, cursor *layoutCursor, sourceID string, textStyle document.WordTextStyle, paragraph *document.WordParagraphLayout) error {
	firstLineChars := paragraph.Set&document.WordParagraphFirstLineIndentCharsSet != 0 || paragraph.FirstLineIndentChars != 0
	hangingChars := paragraph.Set&document.WordParagraphHangingIndentCharsSet != 0 || paragraph.HangingIndentChars != 0
	if !firstLineChars && !hangingChars {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	charWidthPT := 0.0
	if paragraph.FirstLineIndentChars != 0 || paragraph.HangingIndentChars != 0 {
		shaped, err := shaper.Shape("　", textStyle)
		if err == nil {
			for _, glyph := range shaped.Glyphs {
				charWidthPT += glyph.AdvanceXPT
			}
		}
	}
	if !finite(charWidthPT) || charWidthPT <= epsilonPT {
		charWidthPT = textStyle.FontSize
		if !finite(charWidthPT) || charWidthPT <= epsilonPT {
			charWidthPT = 11
		}
		if paragraph.FirstLineIndentChars != 0 || paragraph.HangingIndentChars != 0 {
			cursor.diagnostic(DiagnosticWarning, sourceID, "character-indent-width-fallback", "The ideographic-space font metric is unavailable; character-based indentation uses the paragraph font size as its character width.")
		}
	}
	if firstLineChars {
		paragraph.FirstLineIndentEMU = int64(math.Round(float64(paragraph.FirstLineIndentChars) * charWidthPT * emuPerPoint / 100))
		paragraph.FirstLineIndentChars = 0
		paragraph.Set &^= document.WordParagraphFirstLineIndentCharsSet
	}
	if hangingChars {
		paragraph.HangingIndentEMU = int64(math.Round(float64(paragraph.HangingIndentChars) * charWidthPT * emuPerPoint / 100))
		paragraph.HangingIndentChars = 0
		paragraph.Set &^= document.WordParagraphHangingIndentCharsSet
	}
	return nil
}
