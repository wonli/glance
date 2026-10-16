package docx

import (
	"bytes"
	"fmt"

	"glance/internal/document"
)

const maxWordTextRunReplacements = 10000

var _ document.WordTextBatchEditor = (*Document)(nil)

func (d *Document) ReplaceTextRuns(replacements []document.WordTextRunReplacement) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	if len(replacements) == 0 {
		return false, nil
	}
	if len(replacements) > maxWordTextRunReplacements {
		return false, fmt.Errorf("Word text replacement exceeds %d runs", maxWordTextRunReplacements)
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return false, fmt.Errorf("Word document part is unavailable")
	}

	data := d.parts[d.documentIdx].data
	seen := make(map[string]struct{}, len(replacements))
	changes := make([]xmlReplacement, 0, len(replacements))
	for _, replacement := range replacements {
		if replacement.RunID == "" {
			return false, fmt.Errorf("Word text replacement has an empty run ID")
		}
		if _, duplicate := seen[replacement.RunID]; duplicate {
			return false, fmt.Errorf("duplicate Word text replacement for run %q", replacement.RunID)
		}
		seen[replacement.RunID] = struct{}{}

		target, ok := d.wordTextTargets[replacement.RunID]
		if !ok || target.text == nil || target.run == nil {
			return false, fmt.Errorf("Word text run %q is not editable", replacement.RunID)
		}
		if target.text.text != replacement.ExpectedText {
			return false, fmt.Errorf("Word text run %q changed before replacement", replacement.RunID)
		}
		if target.text.text == replacement.Replacement {
			continue
		}

		patched, err := replaceWordTextNode(data, target.text, replacement.Replacement)
		if err != nil {
			return false, fmt.Errorf("prepare Word text run %q: %w", replacement.RunID, err)
		}
		end := target.text.end + len(patched) - len(data)
		if target.text.start < 0 || end < target.text.start || end > len(patched) {
			return false, fmt.Errorf("prepared Word text run %q has invalid XML bounds", replacement.RunID)
		}
		changes = append(changes, xmlReplacement{
			start: target.text.start,
			end:   target.text.end,
			value: append([]byte(nil), patched[target.text.start:end]...),
		})
	}
	if len(changes) == 0 {
		return false, nil
	}
	patched, err := applyXMLReplacements(data, changes)
	if err != nil {
		return false, err
	}
	if bytes.Equal(data, patched) {
		return false, nil
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return false, err
	}
	return true, nil
}
