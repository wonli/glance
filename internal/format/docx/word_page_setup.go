package docx

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"

	"glance/internal/document"
)

var wordSectionPropertyOrder = []string{
	"headerReference",
	"footerReference",
	"footnotePr",
	"endnotePr",
	"type",
	"pgSz",
	"pgMar",
	"paperSrc",
	"pgBorders",
	"lnNumType",
	"pgNumType",
	"cols",
	"formProt",
	"vAlign",
	"noEndnote",
	"titlePg",
	"textDirection",
	"bidi",
	"rtlGutter",
	"docGrid",
	"printerSettings",
	"sectPrChange",
}

func findWordSectionNodes(body *wordXMLNode) []*wordXMLNode {
	var sections []*wordXMLNode
	for _, child := range body.children {
		switch child.name.Local {
		case "p":
			if properties := firstChild(child, "pPr"); properties != nil {
				if sectPr := firstChild(properties, "sectPr"); sectPr != nil {
					sections = append(sections, sectPr)
				}
			}
		case "sectPr":
			sections = append(sections, child)
		}
	}
	return sections
}

func insertWordSectionProperty(data []byte, sectPr *wordXMLNode, prefix, elementName string, propertyMarkup []byte) (xmlReplacement, error) {
	if sectPr.selfClosing {
		open, err := expandSelfClosingTag(data[sectPr.start:sectPr.openEnd])
		if err != nil {
			return xmlReplacement{}, err
		}
		var content bytes.Buffer
		content.Write(open)
		content.Write(propertyMarkup)
		content.WriteString("</" + sectPr.rawName + ">")
		return xmlReplacement{start: sectPr.start, end: sectPr.openEnd, value: content.Bytes()}, nil
	}
	ranks := make(map[string]int, len(wordSectionPropertyOrder))
	for index, name := range wordSectionPropertyOrder {
		ranks[name] = index
	}
	targetRank := ranks[elementName]
	insertAt := sectPr.contentEnd
	for _, child := range sectPr.children {
		if rank, known := ranks[child.name.Local]; known && rank > targetRank {
			insertAt = child.start
			break
		}
	}
	return xmlReplacement{start: insertAt, end: insertAt, value: propertyMarkup}, nil
}

// SetPageOrientation updates or sets the page orientation (portrait or landscape) for the document.
func (d *Document) SetPageOrientation(orientation string) error {
	orient := strings.ToLower(strings.TrimSpace(orientation))
	if orient != "portrait" && orient != "landscape" {
		return fmt.Errorf("unsupported page orientation %q: must be portrait or landscape", orientation)
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
		return fmt.Errorf("parse DOCX before setting orientation: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return fmt.Errorf("DOCX document has no body")
	}

	prefix := elementPrefix(body.rawName)
	sections := findWordSectionNodes(body)
	if len(sections) == 0 {
		w := "11906"
		h := "16838"
		if orient == "landscape" {
			w, h = h, w
		}
		pgSz := makeWordPropertyMarkup(prefix, "pgSz", "w", w)
		pgSz, _ = setRawXMLAttribute(pgSz, prefix, "h", h)
		pgSz, _ = setRawXMLAttribute(pgSz, prefix, "orient", orient)

		pgMar := makeWordPropertyMarkup(prefix, "pgMar", "top", "1440")
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "right", "1440")
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "bottom", "1440")
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "left", "1440")

		sectPrContent := append(pgSz, pgMar...)
		sectPrMarkup := wrapWordElement(prefix, "sectPr", sectPrContent)
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: body.contentEnd, end: body.contentEnd, value: sectPrMarkup}})
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	var replacements []xmlReplacement
	for _, sectPr := range sections {
		pgSz := firstChild(sectPr, "pgSz")
		if pgSz != nil {
			opening := data[pgSz.start:pgSz.openEnd]
			patchedOpening := opening
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "orient", orient)
			if err != nil {
				return err
			}
			wAttr := attrValue(pgSz, "w")
			hAttr := attrValue(pgSz, "h")
			if wAttr != "" && hAttr != "" {
				wVal, errW := strconv.ParseInt(wAttr, 10, 64)
				hVal, errH := strconv.ParseInt(hAttr, 10, 64)
				if errW == nil && errH == nil {
					swapped := false
					if orient == "landscape" && wVal < hVal {
						wVal, hVal = hVal, wVal
						swapped = true
					} else if orient == "portrait" && wVal > hVal {
						wVal, hVal = hVal, wVal
						swapped = true
					}
					if swapped {
						patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "w", strconv.FormatInt(wVal, 10))
						if err != nil {
							return err
						}
						patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "h", strconv.FormatInt(hVal, 10))
						if err != nil {
							return err
						}
					}
				}
			}
			replacements = append(replacements, xmlReplacement{start: pgSz.start, end: pgSz.openEnd, value: patchedOpening})
		} else {
			w := "11906"
			h := "16838"
			if orient == "landscape" {
				w, h = h, w
			}
			pgSzMarkup := makeWordPropertyMarkup(prefix, "pgSz", "w", w)
			pgSzMarkup, _ = setRawXMLAttribute(pgSzMarkup, prefix, "h", h)
			pgSzMarkup, _ = setRawXMLAttribute(pgSzMarkup, prefix, "orient", orient)

			rep, err := insertWordSectionProperty(data, sectPr, prefix, "pgSz", pgSzMarkup)
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

// SetPageMargins updates or sets the top, right, bottom, and left page margins in points.
func (d *Document) SetPageMargins(topPT, rightPT, bottomPT, leftPT float64) error {
	if topPT < 0 || rightPT < 0 || bottomPT < 0 || leftPT < 0 {
		return fmt.Errorf("margins must be non-negative points")
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
		return fmt.Errorf("parse DOCX before setting margins: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return fmt.Errorf("DOCX document has no body")
	}

	topTwips := strconv.FormatInt(int64(math.Round(topPT*20.0)), 10)
	rightTwips := strconv.FormatInt(int64(math.Round(rightPT*20.0)), 10)
	bottomTwips := strconv.FormatInt(int64(math.Round(bottomPT*20.0)), 10)
	leftTwips := strconv.FormatInt(int64(math.Round(leftPT*20.0)), 10)

	prefix := elementPrefix(body.rawName)
	sections := findWordSectionNodes(body)
	if len(sections) == 0 {
		pgSz := makeWordPropertyMarkup(prefix, "pgSz", "w", "11906")
		pgSz, _ = setRawXMLAttribute(pgSz, prefix, "h", "16838")
		pgSz, _ = setRawXMLAttribute(pgSz, prefix, "orient", "portrait")

		pgMar := makeWordPropertyMarkup(prefix, "pgMar", "top", topTwips)
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "right", rightTwips)
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "bottom", bottomTwips)
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "left", leftTwips)

		sectPrContent := append(pgSz, pgMar...)
		sectPrMarkup := wrapWordElement(prefix, "sectPr", sectPrContent)
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: body.contentEnd, end: body.contentEnd, value: sectPrMarkup}})
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	var replacements []xmlReplacement
	for _, sectPr := range sections {
		pgMar := firstChild(sectPr, "pgMar")
		if pgMar != nil {
			opening := data[pgMar.start:pgMar.openEnd]
			patchedOpening := opening
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "top", topTwips)
			if err != nil {
				return err
			}
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "right", rightTwips)
			if err != nil {
				return err
			}
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "bottom", bottomTwips)
			if err != nil {
				return err
			}
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "left", leftTwips)
			if err != nil {
				return err
			}
			replacements = append(replacements, xmlReplacement{start: pgMar.start, end: pgMar.openEnd, value: patchedOpening})
		} else {
			pgMarMarkup := makeWordPropertyMarkup(prefix, "pgMar", "top", topTwips)
			pgMarMarkup, _ = setRawXMLAttribute(pgMarMarkup, prefix, "right", rightTwips)
			pgMarMarkup, _ = setRawXMLAttribute(pgMarMarkup, prefix, "bottom", bottomTwips)
			pgMarMarkup, _ = setRawXMLAttribute(pgMarMarkup, prefix, "left", leftTwips)

			rep, err := insertWordSectionProperty(data, sectPr, prefix, "pgMar", pgMarMarkup)
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

// SetPageSize updates or sets the physical page dimensions in points.
func (d *Document) SetPageSize(widthPT, heightPT float64) error {
	if widthPT <= 0 || heightPT <= 0 {
		return fmt.Errorf("page width and height must be positive points")
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
		return fmt.Errorf("parse DOCX before setting page size: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return fmt.Errorf("DOCX document has no body")
	}

	wTwips := int64(math.Round(widthPT * 20.0))
	hTwips := int64(math.Round(heightPT * 20.0))

	prefix := elementPrefix(body.rawName)
	sections := findWordSectionNodes(body)
	if len(sections) == 0 {
		orient := "portrait"
		if wTwips > hTwips {
			orient = "landscape"
		}
		pgSz := makeWordPropertyMarkup(prefix, "pgSz", "w", strconv.FormatInt(wTwips, 10))
		pgSz, _ = setRawXMLAttribute(pgSz, prefix, "h", strconv.FormatInt(hTwips, 10))
		pgSz, _ = setRawXMLAttribute(pgSz, prefix, "orient", orient)

		pgMar := makeWordPropertyMarkup(prefix, "pgMar", "top", "1440")
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "right", "1440")
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "bottom", "1440")
		pgMar, _ = setRawXMLAttribute(pgMar, prefix, "left", "1440")

		sectPrContent := append(pgSz, pgMar...)
		sectPrMarkup := wrapWordElement(prefix, "sectPr", sectPrContent)
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: body.contentEnd, end: body.contentEnd, value: sectPrMarkup}})
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	var replacements []xmlReplacement
	for _, sectPr := range sections {
		pgSz := firstChild(sectPr, "pgSz")
		curWTwips := wTwips
		curHTwips := hTwips
		if pgSz != nil {
			orient := strings.ToLower(attrValue(pgSz, "orient"))
			if orient == "landscape" && curWTwips < curHTwips {
				curWTwips, curHTwips = curHTwips, curWTwips
			} else if orient == "portrait" && curWTwips > curHTwips {
				curWTwips, curHTwips = curHTwips, curWTwips
			}
			opening := data[pgSz.start:pgSz.openEnd]
			patchedOpening := opening
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "w", strconv.FormatInt(curWTwips, 10))
			if err != nil {
				return err
			}
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, "h", strconv.FormatInt(curHTwips, 10))
			if err != nil {
				return err
			}
			replacements = append(replacements, xmlReplacement{start: pgSz.start, end: pgSz.openEnd, value: patchedOpening})
		} else {
			orient := "portrait"
			if curWTwips > curHTwips {
				orient = "landscape"
			}
			pgSzMarkup := makeWordPropertyMarkup(prefix, "pgSz", "w", strconv.FormatInt(curWTwips, 10))
			pgSzMarkup, _ = setRawXMLAttribute(pgSzMarkup, prefix, "h", strconv.FormatInt(curHTwips, 10))
			pgSzMarkup, _ = setRawXMLAttribute(pgSzMarkup, prefix, "orient", orient)

			rep, err := insertWordSectionProperty(data, sectPr, prefix, "pgSz", pgSzMarkup)
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

var _ document.WordPageSetupEditor = (*Document)(nil)



