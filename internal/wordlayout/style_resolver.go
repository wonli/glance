package wordlayout

import (
	"glance/internal/document"
	"math"
)

type styleResolver struct {
	defaults                document.WordStyleDefaults
	styles                  map[string]document.WordStyle
	defaultParagraphStyleID string
	defaultTabStopPT        float64
	cursor                  *layoutCursor
}

func newStyleResolver(layout document.WordLayoutData, cursor *layoutCursor) *styleResolver {
	defaultTabStop := layout.DefaultTabStopPT
	if defaultTabStop <= 0 || math.IsNaN(defaultTabStop) || math.IsInf(defaultTabStop, 0) {
		defaultTabStop = 36
	}
	resolver := &styleResolver{defaults: layout.Defaults, styles: make(map[string]document.WordStyle, len(layout.Styles)), defaultTabStopPT: defaultTabStop, cursor: cursor}
	for _, style := range layout.Styles {
		if style.ID == "" {
			continue
		}
		if _, duplicate := resolver.styles[style.ID]; duplicate {
			cursor.diagnostic(DiagnosticError, style.ID, "duplicate-style", "The DOCX contains duplicate style identifiers.")
			continue
		}
		resolver.styles[style.ID] = style
		if style.DefaultParagraph && resolver.defaultParagraphStyleID == "" {
			resolver.defaultParagraphStyleID = style.ID
		}
	}
	return resolver
}

type resolvedParagraphStyle struct {
	text      document.WordTextStyle
	paragraph document.WordParagraphLayout
}

func (r *styleResolver) paragraphStyle(paragraph document.WordParagraph, sourceID string) resolvedParagraphStyle {
	out := resolvedParagraphStyle{}
	mergeTextStyle(&out.text, r.defaults.Text, r.defaults.TextSet)
	mergeParagraphLayout(&out.paragraph, r.defaults.Paragraph)
	paragraphStyleID := paragraph.Style
	if paragraphStyleID == "" {
		paragraphStyleID = r.defaultParagraphStyleID
	}
	if paragraphStyleID != "" {
		chain := make([]document.WordStyle, 0, 4)
		seen := map[string]bool{}
		current := paragraphStyleID
		for current != "" {
			if seen[current] {
				r.cursor.diagnostic(DiagnosticError, sourceID, "style-cycle", "A paragraph style inheritance cycle prevents deterministic layout.")
				break
			}
			seen[current] = true
			style, ok := r.styles[current]
			if !ok {
				r.cursor.diagnostic(DiagnosticError, sourceID, "missing-style", "The paragraph references a missing style: "+current)
				break
			}
			chain = append(chain, style)
			current = style.BasedOn
		}
		for index := len(chain) - 1; index >= 0; index-- {
			style := chain[index]
			mergeTextStyle(&out.text, style.Text, style.TextSet)
			mergeParagraphLayout(&out.paragraph, style.Paragraph)
		}
	}
	mergeParagraphLayout(&out.paragraph, paragraph.Layout)
	if paragraph.Layout.Set&document.WordParagraphAlignmentSet == 0 && paragraph.Alignment != document.WordAlignDefault {
		out.paragraph.Alignment = paragraph.Alignment
		out.paragraph.Set |= document.WordParagraphAlignmentSet
	}
	if paragraph.Layout.Set&document.WordParagraphLeftIndentSet == 0 && paragraph.LeftIndentEMU != 0 {
		out.paragraph.LeftIndentEMU = paragraph.LeftIndentEMU
		out.paragraph.Set |= document.WordParagraphLeftIndentSet
	}
	if paragraph.Layout.Set&document.WordParagraphRightIndentSet == 0 && paragraph.RightIndentEMU != 0 {
		out.paragraph.RightIndentEMU = paragraph.RightIndentEMU
		out.paragraph.Set |= document.WordParagraphRightIndentSet
	}
	if out.text.FontSize <= 0 {
		out.text.FontSize = 11
	}
	if out.text.FontFamily == "" {
		out.text.FontFamily = "sans-serif"
	}
	if out.text.FontColor == "" {
		out.text.FontColor = "000000"
	}
	if borderStyle := out.paragraph.BottomBorder.Style; borderStyle != "" && borderStyle != "single" && borderStyle != "none" && borderStyle != "nil" {
		r.cursor.diagnostic(DiagnosticError, sourceID, "paragraph-border-style", "The paragraph uses an unsupported bottom border style: "+borderStyle)
	}
	return out
}

func (r *styleResolver) applyCharacterStyle(target *document.WordTextStyle, styleID, sourceID string) {
	if styleID == "" {
		return
	}
	chain := make([]document.WordStyle, 0, 4)
	seen := map[string]bool{}
	current := styleID
	for current != "" {
		if seen[current] {
			r.cursor.diagnostic(DiagnosticError, sourceID, "style-cycle", "A character style inheritance cycle prevents deterministic layout.")
			break
		}
		seen[current] = true
		style, ok := r.styles[current]
		if !ok {
			r.cursor.diagnostic(DiagnosticError, sourceID, "missing-style", "A run references a missing character style: "+current)
			break
		}
		chain = append(chain, style)
		current = style.BasedOn
	}
	for index := len(chain) - 1; index >= 0; index-- {
		mergeTextStyle(target, chain[index].Text, chain[index].TextSet)
	}
}

func mergeTextStyle(target *document.WordTextStyle, source document.WordTextStyle, mask document.WordTextStyleMask) {
	if mask == 0 {
		if source.Bold {
			mask |= document.WordTextBoldSet
		}
		if source.Italic {
			mask |= document.WordTextItalicSet
		}
		if source.Underline {
			mask |= document.WordTextUnderlineSet
		}
		if source.Strike {
			mask |= document.WordTextStrikeSet
		}
		if source.FontFamily != "" {
			mask |= document.WordTextFontFamilySet
		}
		if source.FontSize != 0 {
			mask |= document.WordTextFontSizeSet
		}
		if source.FontColor != "" {
			mask |= document.WordTextFontColorSet
		}
		if source.Highlight != "" {
			mask |= document.WordTextHighlightSet
		}
	}
	if mask&document.WordTextBoldSet != 0 {
		target.Bold = source.Bold
	}
	if mask&document.WordTextItalicSet != 0 {
		target.Italic = source.Italic
	}
	if mask&document.WordTextUnderlineSet != 0 {
		target.Underline = source.Underline
	}
	if mask&document.WordTextStrikeSet != 0 {
		target.Strike = source.Strike
	}
	if mask&document.WordTextFontFamilySet != 0 {
		target.FontFamily = source.FontFamily
	}
	if mask&document.WordTextFontSizeSet != 0 {
		target.FontSize = source.FontSize
	}
	if mask&document.WordTextFontColorSet != 0 {
		target.FontColor = source.FontColor
	}
	if mask&document.WordTextHighlightSet != 0 {
		target.Highlight = source.Highlight
	}
}

func mergeParagraphLayout(target *document.WordParagraphLayout, source document.WordParagraphLayout) {
	mask := source.Set
	if mask == 0 {
		if source.BeforePT != 0 {
			mask |= document.WordParagraphBeforeSet
		}
		if source.AfterPT != 0 {
			mask |= document.WordParagraphAfterSet
		}
		if source.LineSpacingPT != 0 || source.LineSpacingMultiple != 0 || source.LineSpacingRule != "" {
			mask |= document.WordParagraphLineSpacingSet
		}
		if source.FirstLineIndentEMU != 0 {
			mask |= document.WordParagraphFirstLineIndentSet
		}
		if source.HangingIndentEMU != 0 {
			mask |= document.WordParagraphHangingIndentSet
		}
		if source.Alignment != document.WordAlignDefault {
			mask |= document.WordParagraphAlignmentSet
		}
		if source.LeftIndentEMU != 0 {
			mask |= document.WordParagraphLeftIndentSet
		}
		if source.RightIndentEMU != 0 {
			mask |= document.WordParagraphRightIndentSet
		}
		if source.KeepWithNext {
			mask |= document.WordParagraphKeepWithNextSet
		}
		if source.KeepTogether {
			mask |= document.WordParagraphKeepTogetherSet
		}
		if source.PageBreakBefore {
			mask |= document.WordParagraphPageBreakBeforeSet
		}
		if source.BottomBorder != (document.WordParagraphBorder{}) {
			mask |= document.WordParagraphBottomBorderSet
		}
		if source.TabStops != nil {
			mask |= document.WordParagraphTabStopsSet
		}
	}
	if mask&document.WordParagraphBeforeSet != 0 {
		target.BeforePT = source.BeforePT
	}
	if mask&document.WordParagraphAfterSet != 0 {
		target.AfterPT = source.AfterPT
	}
	if mask&document.WordParagraphLineSpacingSet != 0 {
		target.LineSpacingPT = source.LineSpacingPT
		target.LineSpacingMultiple = source.LineSpacingMultiple
		target.LineSpacingRule = source.LineSpacingRule
	}
	if mask&document.WordParagraphFirstLineIndentSet != 0 {
		target.FirstLineIndentEMU = source.FirstLineIndentEMU
	}
	if mask&document.WordParagraphHangingIndentSet != 0 {
		target.HangingIndentEMU = source.HangingIndentEMU
	}
	if mask&document.WordParagraphAlignmentSet != 0 {
		target.Alignment = source.Alignment
	}
	if mask&document.WordParagraphLeftIndentSet != 0 {
		target.LeftIndentEMU = source.LeftIndentEMU
	}
	if mask&document.WordParagraphRightIndentSet != 0 {
		target.RightIndentEMU = source.RightIndentEMU
	}
	if mask&document.WordParagraphKeepWithNextSet != 0 {
		target.KeepWithNext = source.KeepWithNext
	}
	if mask&document.WordParagraphKeepTogetherSet != 0 {
		target.KeepTogether = source.KeepTogether
	}
	if mask&document.WordParagraphPageBreakBeforeSet != 0 {
		target.PageBreakBefore = source.PageBreakBefore
	}
	if mask&document.WordParagraphBottomBorderSet != 0 {
		target.BottomBorder = source.BottomBorder
	}
	if mask&document.WordParagraphTabStopsSet != 0 {
		target.TabStops = append([]document.WordTabStop(nil), source.TabStops...)
	}
	target.Set |= mask
}
