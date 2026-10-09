package docx

import (
	"fmt"
	"strconv"
	"strings"
)

// SetPageColumns updates or sets the number of columns for the document sections (1 to 4).
func (d *Document) SetPageColumns(columns int) error {
	if columns < 1 || columns > 4 {
		return fmt.Errorf("unsupported column count %d: must be between 1 and 4", columns)
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
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX before setting columns: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return fmt.Errorf("DOCX document has no body")
	}

	prefix := elementPrefix(body.rawName)
	sections := findWordSectionNodes(body)
	if len(sections) == 0 {
		colsMarkup := makeWordPropertyMarkup(prefix, "cols", "num", strconv.Itoa(columns))
		colsMarkup, _ = setRawXMLAttribute(colsMarkup, prefix, "space", "720")
		sectPrMarkup := wrapWordElement(prefix, "sectPr", colsMarkup)
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: body.contentEnd, end: body.contentEnd, value: sectPrMarkup}})
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	var replacements []xmlReplacement
	for _, sectPr := range sections {
		cols := firstChild(sectPr, "cols")
		if cols != nil {
			opening := data[cols.start:cols.openEnd]
			patchedOpening := opening
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "num", strconv.Itoa(columns))
			if err != nil {
				return err
			}
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "space", "720")
			if err != nil {
				return err
			}
			replacements = append(replacements, xmlReplacement{start: cols.start, end: cols.openEnd, value: patchedOpening})
		} else {
			colsMarkup := makeWordPropertyMarkup(prefix, "cols", "num", strconv.Itoa(columns))
			colsMarkup, _ = setRawXMLAttribute(colsMarkup, prefix, "space", "720")
			rep, err := insertWordSectionProperty(data, sectPr, prefix, "cols", colsMarkup)
			if err != nil {
				return err
			}
			replacements = append(replacements, rep)
		}
	}

	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

// SetPageLineNumbers sets or clears line numbering for the document.
// restart can be "continuous", "newPage", or "none" (or empty to disable).
func (d *Document) SetPageLineNumbers(restart string) error {
	mode := strings.TrimSpace(restart)
	if mode != "" && mode != "none" && mode != "continuous" && mode != "newPage" {
		return fmt.Errorf("unsupported line numbers mode %q: must be continuous, newPage, or none", restart)
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
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX document: %w", err)
	}
	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}
	body := firstChild(root, "body")
	if body == nil {
		return fmt.Errorf("DOCX document has no body element")
	}

	sections := findWordSectionNodes(body)
	if len(sections) == 0 {
		if mode == "" || mode == "none" {
			return nil
		}
		lnNumMarkup := makeWordPropertyMarkup(prefix, "lnNumType", "countBy", "1")
		lnNumMarkup, _ = setRawXMLAttribute(lnNumMarkup, prefix, "restart", mode)
		sectPrMarkup := wrapWordElement(prefix, "sectPr", lnNumMarkup)
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: body.contentEnd, end: body.contentEnd, value: sectPrMarkup}})
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	var replacements []xmlReplacement
	for _, sectPr := range sections {
		lnNum := firstChild(sectPr, "lnNumType")
		if mode == "" || mode == "none" {
			if lnNum != nil {
				replacements = append(replacements, xmlReplacement{start: lnNum.start, end: lnNum.end, value: nil})
			}
		} else {
			if lnNum != nil {
				opening := data[lnNum.start:lnNum.openEnd]
				patchedOpening := opening
				patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "countBy", "1")
				if err != nil {
					return err
				}
				patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "restart", mode)
				if err != nil {
					return err
				}
				replacements = append(replacements, xmlReplacement{start: lnNum.start, end: lnNum.openEnd, value: patchedOpening})
			} else {
				lnNumMarkup := makeWordPropertyMarkup(prefix, "lnNumType", "countBy", "1")
				lnNumMarkup, _ = setRawXMLAttribute(lnNumMarkup, prefix, "restart", mode)
				rep, err := insertWordSectionProperty(data, sectPr, prefix, "lnNumType", lnNumMarkup)
				if err != nil {
					return err
				}
				replacements = append(replacements, rep)
			}
		}
	}

	if len(replacements) == 0 {
		return nil
	}

	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

// SetPageBorders sets or clears borders for all pages in the document.
// borderType can be "single", "box", "double", "dashed", "wave", "none", or empty.
func (d *Document) SetPageBorders(borderType string) error {
	mode := strings.ToLower(strings.TrimSpace(borderType))
	if mode == "box" {
		mode = "single"
	}
	if mode != "" && mode != "none" && mode != "single" && mode != "double" && mode != "dashed" && mode != "wave" {
		return fmt.Errorf("unsupported page border type %q", borderType)
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
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX document: %w", err)
	}
	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}
	body := firstChild(root, "body")
	if body == nil {
		return fmt.Errorf("DOCX document has no body element")
	}

	makeBordersMarkup := func() []byte {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("<%s:pgBorders %s:offsetFrom=\"page\">", prefix, prefix))
		for _, side := range []string{"top", "left", "bottom", "right"} {
			b.WriteString(fmt.Sprintf("<%s:%s %s:val=\"%s\" %s:sz=\"4\" %s:space=\"24\" %s:color=\"auto\"/>", prefix, side, prefix, mode, prefix, prefix, prefix))
		}
		b.WriteString(fmt.Sprintf("</%s:pgBorders>", prefix))
		return []byte(b.String())
	}

	sections := findWordSectionNodes(body)
	if len(sections) == 0 {
		if mode == "" || mode == "none" {
			return nil
		}
		sectPrMarkup := wrapWordElement(prefix, "sectPr", makeBordersMarkup())
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: body.contentEnd, end: body.contentEnd, value: sectPrMarkup}})
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	var replacements []xmlReplacement
	for _, sectPr := range sections {
		pgBorders := firstChild(sectPr, "pgBorders")
		if mode == "" || mode == "none" {
			if pgBorders != nil {
				replacements = append(replacements, xmlReplacement{start: pgBorders.start, end: pgBorders.end, value: nil})
			}
		} else {
			bordersMarkup := makeBordersMarkup()
			if pgBorders != nil {
				replacements = append(replacements, xmlReplacement{start: pgBorders.start, end: pgBorders.end, value: bordersMarkup})
			} else {
				rep, err := insertWordSectionProperty(data, sectPr, prefix, "pgBorders", bordersMarkup)
				if err != nil {
					return err
				}
				replacements = append(replacements, rep)
			}
		}
	}

	if len(replacements) == 0 {
		return nil
	}

	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}
