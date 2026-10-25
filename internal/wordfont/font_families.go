package wordfont

import (
	"github.com/go-text/typesetting/font"
	"glance/internal/document"
	"glance/internal/fontmanager/fontfallback"
	"unicode"
)

var defaultEastAsianFamily = FallbackFamilies()[0]

// FamilyForRune reports the preferred script-specific family, before resolving
// installed aliases and font coverage. Generic family requests remain usable.
func FamilyForRune(style document.WordTextStyle, r rune) string { return fontFamilyForRune(style, r) }

func fontFamilyForRune(style document.WordTextStyle, r rune) string {
	family := style.FontFamilyHighANSI
	if r <= 0x7f {
		family = style.FontFamilyASCII
	} else if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || r >= 0x3000 && r <= 0x303f || r >= 0xff00 && r <= 0xffef {
		family = style.FontFamilyEastAsia
		if family == "" && (style.FontFamilyASCII != "" || style.FontFamilyHighANSI != "" || style.FontFamilyComplexScript != "") {
			// OOXML's missing eastAsia attribute uses an application CJK default;
			// ascii/hAnsi/cs do not specify the font of Han text.
			return defaultEastAsianFamily
		}
	} else if isComplexScriptRune(r) {
		family = style.FontFamilyComplexScript
	}
	if family == "" {
		family = style.FontFamily
	}
	return family
}

// Exact English/localized aliases precede platform substitutes. A substitute
// supplies outlines; it does not rewrite the source font names in the package.
func wordFontFamilyCandidates(value string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, family := range splitWordFontFamilies(value) {
		for _, candidate := range fontfallback.FamilyCandidates(family) {
			normalized := font.NormalizeFamily(candidate)
			if !seen[normalized] {
				seen[normalized] = true
				out = append(out, candidate)
			}
		}
	}
	return out
}

func (m *wordTextFontMap) missingBiWidthFamily(family string) bool {
	normalized := font.NormalizeFamily(family)
	// SimHei's Latin repertoire is monospaced at half an em. Do not impose
	// this on arbitrary unavailable families or proportional platform fonts.
	if normalized != "simhei" && normalized != "黑体" {
		return false
	}
	if m.biWidthMissing == nil {
		m.biWidthMissing = map[string]bool{}
	}
	if missing, ok := m.biWidthMissing[normalized]; ok {
		return missing
	}
	missing := true
	for _, f := range m.owner.fonts {
		if f.Family == "simhei" || f.Family == "黑体" {
			missing = false
			break
		}
	}
	m.biWidthMissing[normalized] = missing
	return missing
}

func asciiCluster(runes []rune) bool {
	for _, r := range runes {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return len(runes) > 0
}

func isComplexScriptRune(r rune) bool {
	return unicode.IsOneOf([]*unicode.RangeTable{unicode.Arabic, unicode.Hebrew, unicode.Syriac, unicode.Thaana, unicode.Nko, unicode.Devanagari, unicode.Bengali, unicode.Gurmukhi, unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu, unicode.Kannada, unicode.Malayalam, unicode.Sinhala, unicode.Thai, unicode.Lao, unicode.Tibetan, unicode.Myanmar, unicode.Khmer}, r)
}
