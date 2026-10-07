package docx

import (
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordLanguageEditor = (*Document)(nil)

// SetLanguage sets the proofing/language tag on the specified run, or on all text runs
// if runID is empty.
func (d *Document) SetLanguage(runID, langCode string) error {
	langCode = strings.TrimSpace(langCode)
	if langCode == "" {
		langCode = "zh-CN"
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return fmt.Errorf("DOCX document part is unavailable")
	}

	data := d.parts[d.documentIdx].data

	var targetRuns []*wordXMLNode
	if runID != "" {
		target, ok := d.wordTextTargets[runID]
		if !ok || target.run == nil {
			return fmt.Errorf("Word text run %q is not editable", runID)
		}
		targetRuns = append(targetRuns, target.run)
	} else {
		for _, target := range d.wordTextTargets {
			if target.run != nil {
				targetRuns = append(targetRuns, target.run)
			}
		}
	}
	if len(targetRuns) == 0 {
		return nil
	}

	var replacements []xmlReplacement
	for _, runNode := range targetRuns {
		prefix := elementPrefix(runNode.rawName)
		properties := firstChild(runNode, "rPr")
		langNode := (*wordXMLNode)(nil)
		if properties != nil {
			langNode = firstChild(properties, "lang")
		}
		if langNode == nil {
			markup := makeWordPropertyMarkup(prefix, "lang", "val", langCode)
			var err error
			for _, attr := range []string{"eastAsia", "bidi"} {
				markup, err = setRawXMLAttribute(markup, prefix, attr, langCode)
				if err != nil {
					return err
				}
			}
			if properties == nil {
				replacements = append(replacements, xmlReplacement{start: runNode.openEnd, end: runNode.openEnd, value: wrapWordProperties(prefix, markup)})
			} else {
				replacements = append(replacements, xmlReplacement{start: properties.contentStart, end: properties.contentStart, value: markup})
			}
		} else {
			opening := data[langNode.start:langNode.openEnd]
			var err error
			for _, attr := range []string{"val", "eastAsia", "bidi"} {
				opening, err = setRawXMLAttribute(opening, elementPrefix(langNode.rawName), attr, langCode)
				if err != nil {
					return err
				}
			}
			replacements = append(replacements, xmlReplacement{start: langNode.start, end: langNode.openEnd, value: opening})
		}
	}

	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}
