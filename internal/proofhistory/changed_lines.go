package proofhistory

import "strings"

// ChangedTextLines keeps only logical body lines that contain inserted or
// deleted characters. Inline spans remain intact, so a one-character edit
// retains its line's context without duplicating the rest of the page.
func ChangedTextLines(spans []TextChange) []TextChange {
	var result, line []TextChange
	changed, omitted := false, false
	flush := func() {
		if changed {
			if len(result) > 0 && omitted {
				result = append(result, TextChange{Kind: TextUnchanged, Text: "…\n"})
			}
			result = append(result, line...)
			omitted = false
		} else {
			omitted = true
		}
		line = nil
		changed = false
	}
	for _, span := range spans {
		text := span.Text
		for len(text) > 0 {
			end := strings.IndexByte(text, '\n')
			if end < 0 {
				line = append(line, TextChange{span.Kind, text})
				changed = changed || span.Kind != TextUnchanged
				break
			}
			end++
			line = append(line, TextChange{span.Kind, text[:end]})
			changed = changed || span.Kind != TextUnchanged
			text = text[end:]
			flush()
		}
	}
	if len(line) > 0 {
		flush()
	}
	return result
}
