package docx

import (
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordPageBackgroundEditor = (*Document)(nil)

// GetPageBackgroundColor inspects word/document.xml for a <w:background> color.
func (d *Document) GetPageBackgroundColor() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return "", errors.New("document part missing")
	}
	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		return "", fmt.Errorf("parse word/document.xml: %w", err)
	}
	bg := firstChild(root, "background")
	if bg == nil {
		return "", nil
	}
	color := attrValue(bg, "color")
	return strings.ToUpper(strings.TrimSpace(color)), nil
}

// SetPageBackgroundColor updates or removes the page background color in word/document.xml.
func (d *Document) SetPageBackgroundColor(hexColor string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("document part missing")
	}
	docData := d.parts[d.documentIdx].data
	root, err := parseWordXML(docData)
	if err != nil {
		return fmt.Errorf("parse word/document.xml: %w", err)
	}

	cleanColor := strings.TrimSpace(hexColor)
	cleanColor = strings.TrimPrefix(cleanColor, "#")

	bg := firstChild(root, "background")

	var updated []byte
	if cleanColor == "" || strings.EqualFold(cleanColor, "none") || strings.EqualFold(cleanColor, "auto") {
		if bg == nil {
			return nil
		}
		updated, err = applyXMLReplacements(docData, []xmlReplacement{{start: bg.start, end: bg.end, value: nil}})
		if err != nil {
			return err
		}
	} else {
		cleanColor = strings.ToUpper(cleanColor)
		replacement := []byte(fmt.Sprintf(`<w:background w:color="%s"/>`, cleanColor))
		if bg != nil {
			if strings.EqualFold(attrValue(bg, "color"), cleanColor) {
				return nil
			}
			updated, err = applyXMLReplacements(docData, []xmlReplacement{{start: bg.start, end: bg.end, value: replacement}})
			if err != nil {
				return err
			}
		} else {
			body := firstChild(root, "body")
			if body == nil {
				return errors.New("DOCX document has no body")
			}
			updated, err = applyXMLReplacements(docData, []xmlReplacement{{start: body.start, end: body.start, value: replacement}})
			if err != nil {
				return err
			}
		}
	}

	changes := map[string]*part{d.parts[d.documentIdx].header.Name: d.partWithData(d.parts[d.documentIdx].header.Name, updated)}
	return d.commitWordPackageChange(changes)
}
