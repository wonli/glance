package pagealign

import (
	"strings"
	"unicode"
)

// CleanOCRText converts OCR markup (such as LaTeX math wrappers or formatting
// commands output by multimodal document models like MinerU) into clean plain text.
// It is completely document-agnostic and uses no book-specific heuristics or regexes.
func CleanOCRText(raw string) string {
	if raw == "" {
		return ""
	}
	// Fast path: if there are no backslashes or dollar signs, return raw text directly.
	if !strings.ContainsRune(raw, '\\') && !strings.ContainsRune(raw, '$') {
		return raw
	}

	runes := []rune(raw)
	var b strings.Builder
	b.Grow(len(raw))

	i := 0
	n := len(runes)

	for i < n {
		r := runes[i]
		if r == '$' {
			// Skip LaTeX math delimiters.
			i++
			continue
		}
		if r == '\\' {
			// LaTeX command: \commandName{content} or \symbol
			i++
			cmdStart := i
			for i < n && unicode.IsLetter(runes[i]) {
				i++
			}
			// If followed by '{', we skip the command name and opening brace,
			// allowing the inner content to be processed as normal text.
			if i < n && runes[i] == '{' {
				i++ // skip '{'
			}
			// If not followed by '{' (e.g. \quad, \dots), command is just dropped.
			_ = cmdStart
			continue
		}
		if r == '}' {
			// Closing brace of LaTeX command wrapper.
			i++
			continue
		}
		b.WriteRune(r)
		i++
	}

	return b.String()
}

// TextSource represents a stripped, non-whitespace character stream with a mapping
// from each character's sequential index back to its rune offset in the original text.
type TextSource struct {
	Characters []rune
	Starts     []int
	Original   []rune
}

// NewTextSource converts text into a clean non-whitespace rune stream.
func NewTextSource(raw string) TextSource {
	runes := []rune(raw)
	src := TextSource{
		Characters: make([]rune, 0, len(runes)),
		Starts:     make([]int, 0, len(runes)),
		Original:   runes,
	}
	for i, r := range runes {
		if unicode.IsSpace(r) {
			continue
		}
		src.Characters = append(src.Characters, r)
		src.Starts = append(src.Starts, i)
	}
	return src
}
