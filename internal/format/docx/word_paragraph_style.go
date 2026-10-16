package docx

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"glance/internal/document"
)

func (d *Document) SetParagraphStyles(paragraphIDs []string, styleID string) error {
	styleID = strings.TrimSpace(styleID)
	if styleID == "" || len(styleID) > 255 || strings.IndexFunc(styleID, unicode.IsControl) >= 0 {
		return fmt.Errorf("Word paragraph style ID must contain 1–255 printable characters")
	}
	if len(paragraphIDs) == 0 {
		return fmt.Errorf("at least one Word paragraph must be selected")
	}

	type target struct {
		id        string
		paragraph *wordXMLNode
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(paragraphIDs))
	targets := make([]target, 0, len(paragraphIDs))
	for _, id := range paragraphIDs {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		paragraph := d.wordParagraphTargets[id]
		if paragraph == nil {
			return fmt.Errorf("Word paragraph %q is not editable", id)
		}
		targets = append(targets, target{id: id, paragraph: paragraph})
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].paragraph.start > targets[j].paragraph.start
	})

	data := d.parts[d.documentIdx].data
	patched := data
	for _, target := range targets {
		paragraph := target.paragraph
		properties := firstChild(paragraph, "pPr")
		style := firstChild(properties, "pStyle")
		if style != nil && attrValue(style, "val") == styleID {
			continue
		}

		var err error
		switch {
		case style != nil:
			opening := patched[style.start:style.openEnd]
			opening, err = setRawXMLAttribute(opening, elementPrefix(style.rawName), "val", styleID)
			if err == nil {
				patched, err = applyXMLReplacements(patched, []xmlReplacement{{start: style.start, end: style.openEnd, value: opening}})
			}
		case properties != nil:
			markup := makeWordPropertyMarkup(rawWordPrefix(properties.rawName), "pStyle", "val", styleID)
			patched, err = insertElementContentAtStart(patched, properties, markup)
		default:
			prefix := rawWordPrefix(paragraph.rawName)
			styleMarkup := makeWordPropertyMarkup(prefix, "pStyle", "val", styleID)
			propertiesMarkup := wrapWordElement(prefix, "pPr", styleMarkup)
			patched, err = insertElementContentAtStart(patched, paragraph, propertiesMarkup)
		}
		if err != nil {
			return fmt.Errorf("set Word paragraph %q style: %w", target.id, err)
		}
	}
	if bytes.Equal(data, patched) {
		return nil
	}
	return d.applyWordDocumentXMLLocked(patched)
}

var _ document.WordParagraphStyleEditor = (*Document)(nil)
