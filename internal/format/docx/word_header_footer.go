package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordHeaderFooterEditor = (*Document)(nil)

const (
	wordHeaderPath        = "word/header1.xml"
	wordFooterPath        = "word/footer1.xml"
	wordHeaderRelID       = "rIdHeader1"
	wordFooterRelID       = "rIdFooter1"
	wordHeaderRelType     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	wordFooterRelType     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	wordHeaderContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"
	wordFooterContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"
)

// SetHeader configures the top page header text across the document sections.
func (d *Document) SetHeader(text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		text = "页眉"
	}

	headerXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:sz w:val="18"/><w:color w:val="666666"/></w:rPr><w:t>%s</w:t></w:r></w:p></w:hdr>`, escapeXMLString(text))
	if existing, ok := findPackagePart(d.parts, wordHeaderPath); ok && bytes.Equal(existing.data, []byte(headerXML)) {
		return nil
	}

	return d.applyHeaderFooterLocked(wordHeaderPath, wordHeaderRelID, wordHeaderRelType, wordHeaderContentType, "headerReference", []byte(headerXML))
}

// SetFooter configures the bottom page footer text across the document sections.
func (d *Document) SetFooter(text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		text = "页脚"
	}

	footerXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:sz w:val="18"/><w:color w:val="666666"/></w:rPr><w:t>%s</w:t></w:r></w:p></w:ftr>`, escapeXMLString(text))
	if existing, ok := findPackagePart(d.parts, wordFooterPath); ok && bytes.Equal(existing.data, []byte(footerXML)) {
		return nil
	}

	return d.applyHeaderFooterLocked(wordFooterPath, wordFooterRelID, wordFooterRelType, wordFooterContentType, "footerReference", []byte(footerXML))
}

// SetPageNumber inserts a dynamic page number field into the document footer.
func (d *Document) SetPageNumber(align string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}

	align = strings.TrimSpace(align)
	if align == "" {
		align = "center"
	}
	if align != "left" && align != "center" && align != "right" {
		return fmt.Errorf("page number alignment must be left, center, or right")
	}

	footerXML := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"></w:ftr>`)
	existingFooter, hasExistingFooter := findPackagePart(d.parts, wordFooterPath)
	if hasExistingFooter {
		footerXML = existingFooter.data
	}
	root, err := parseWordXML(footerXML)
	if err != nil {
		return fmt.Errorf("parse DOCX footer before inserting page number: %w", err)
	}
	if root.name.Local != "ftr" {
		return errors.New("DOCX footer part has an unexpected root element")
	}

	if paragraph := wordPageNumberParagraph(root, nil); paragraph != nil {
		footerXML, err = setWordParagraphXMLAlignment(footerXML, paragraph, align)
		if err != nil {
			return fmt.Errorf("align existing DOCX page number: %w", err)
		}
	} else {
		prefix := elementPrefix(root.rawName)
		if prefix == "" {
			prefix = "w"
		}
		pageNumberParagraph := fmt.Sprintf(`<%[1]s:p><%[1]s:pPr><%[1]s:jc %[1]s:val="%[2]s"/></%[1]s:pPr><%[1]s:fldSimple %[1]s:instr="PAGE"><%[1]s:r><%[1]s:rPr><%[1]s:sz %[1]s:val="18"/><%[1]s:color %[1]s:val="666666"/></%[1]s:rPr><%[1]s:t>1</%[1]s:t></%[1]s:r></%[1]s:fldSimple></%[1]s:p>`, prefix, escapeXMLString(align))
		footerXML, err = insertElementContent(footerXML, root, []byte(pageNumberParagraph))
		if err != nil {
			return fmt.Errorf("insert DOCX page number: %w", err)
		}
	}
	if hasExistingFooter && bytes.Equal(existingFooter.data, footerXML) {
		return nil
	}

	return d.applyHeaderFooterLocked(wordFooterPath, wordFooterRelID, wordFooterRelType, wordFooterContentType, "footerReference", footerXML)
}

func wordPageNumberParagraph(node, paragraph *wordXMLNode) *wordXMLNode {
	if node == nil {
		return nil
	}
	if node.name.Local == "p" {
		paragraph = node
	}
	if node.name.Local == "fldSimple" && strings.EqualFold(strings.TrimSpace(attrValue(node, "instr")), "PAGE") {
		return paragraph
	}
	for _, child := range node.children {
		if found := wordPageNumberParagraph(child, paragraph); found != nil {
			return found
		}
	}
	return nil
}

func setWordParagraphXMLAlignment(data []byte, paragraph *wordXMLNode, alignment string) ([]byte, error) {
	if paragraph == nil || paragraph.name.Local != "p" {
		return nil, errors.New("page number field is not inside a paragraph")
	}
	pPr := firstChild(paragraph, "pPr")
	if pPr == nil {
		prefix := elementPrefix(paragraph.rawName)
		if prefix == "" {
			prefix = "w"
		}
		return insertElementContentAtStart(data, paragraph, []byte(fmt.Sprintf(`<%[1]s:pPr><%[1]s:jc %[1]s:val="%[2]s"/></%[1]s:pPr>`, prefix, escapeXMLString(alignment))))
	}
	if jc := firstChild(pPr, "jc"); jc != nil {
		return patchElementAttribute(data, jc, "val", alignment)
	}

	propertyOrder := map[string]int{
		"pStyle": 1, "keepNext": 2, "keepLines": 3, "pageBreakBefore": 4, "framePr": 5,
		"widowControl": 6, "numPr": 7, "suppressLineNumbers": 8, "pBdr": 9, "shd": 10,
		"tabs": 11, "suppressAutoHyphens": 12, "kinsoku": 13, "wordWrap": 14,
		"overflowPunct": 15, "topLinePunct": 16, "autoSpaceDE": 17, "autoSpaceDN": 18,
		"bidi": 19, "adjustRightInd": 20, "snapToGrid": 21, "spacing": 22, "ind": 23,
		"contextualSpacing": 24, "mirrorIndents": 25, "suppressOverlap": 26, "jc": 27,
		"textDirection": 28, "textAlignment": 29, "textboxTightWrap": 30, "outlineLvl": 31,
		"divId": 32, "cnfStyle": 33, "rPr": 34, "sectPr": 35,
	}
	prefix := elementPrefix(pPr.rawName)
	if prefix == "" {
		prefix = "w"
	}
	markup := []byte(fmt.Sprintf(`<%[1]s:jc %[1]s:val="%[2]s"/>`, prefix, escapeXMLString(alignment)))
	insertAt := pPr.contentEnd
	for _, child := range pPr.children {
		if order, known := propertyOrder[child.name.Local]; known && order > propertyOrder["jc"] {
			insertAt = child.start
			break
		}
	}
	if pPr.selfClosing {
		return insertElementContent(data, pPr, markup)
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: insertAt, end: insertAt, value: markup}})
}

func (d *Document) applyHeaderFooterLocked(partPath, relID, relType, contentType, refTag string, content []byte) error {
	// 1. Prepare target XML part
	targetPart := &part{
		header: zip.FileHeader{Name: partPath, Method: zip.Deflate},
		data:   content,
	}

	// 2. Prepare relationship in word/_rels/document.xml.rels
	const relName = "word/_rels/document.xml.rels"
	relPartVal, exists := findPackagePart(d.parts, relName)
	if !exists {
		relPartVal = part{header: zip.FileHeader{Name: relName, Method: zip.Deflate}, data: []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="` + wordPackageRelationshipsNS + `"/>`)}
	}
	updatedRelPart, err := ensureDocumentRelPart(&relPartVal, relID, relType, strings.TrimPrefix(partPath, "word/"))
	if err != nil {
		return err
	}

	// 3. Prepare Content Type override in [Content_Types].xml
	const ctName = "[Content_Types].xml"
	ctPartVal, exists := findPackagePart(d.parts, ctName)
	if !exists {
		return errors.New("DOCX package missing [Content_Types].xml")
	}
	updatedCtPart, err := ensureContentTypePart(&ctPartVal, "/"+partPath, contentType)
	if err != nil {
		return err
	}

	// 4. Update word/document.xml to reference the header/footer in <w:sectPr>
	docData := d.parts[d.documentIdx].data
	root, err := parseWordXML(docData)
	if err != nil {
		return fmt.Errorf("parse DOCX document.xml: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return errors.New("DOCX document has no body element")
	}

	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}

	var sectPrNodes []*wordXMLNode
	for _, child := range body.children {
		if child.name.Local == "p" {
			if pPr := firstChild(child, "pPr"); pPr != nil {
				if sp := firstChild(pPr, "sectPr"); sp != nil {
					sectPrNodes = append(sectPrNodes, sp)
				}
			}
		} else if child.name.Local == "sectPr" {
			sectPrNodes = append(sectPrNodes, child)
		}
	}

	refXML := fmt.Sprintf("<%s:%s %s:type=\"default\" r:id=\"%s\"/>", prefix, refTag, prefix, escapeXMLString(relID))

	var replacements []xmlReplacement
	if len(sectPrNodes) == 0 {
		// Insert trailing sectPr at end of body
		newSect := fmt.Sprintf("<%s:sectPr>%s</%s:sectPr>", prefix, refXML, prefix)
		replacements = append(replacements, xmlReplacement{
			start: body.contentEnd,
			end:   body.contentEnd,
			value: []byte(newSect),
		})
	} else {
		for _, sp := range sectPrNodes {
			existingRef := firstChild(sp, refTag)
			if existingRef != nil {
				replacements = append(replacements, xmlReplacement{
					start: existingRef.start,
					end:   existingRef.end,
					value: []byte(refXML),
				})
			} else {
				// Insert at beginning of sectPr (conforming to wordSectionPropertyOrder)
				replacements = append(replacements, xmlReplacement{
					start: sp.contentStart,
					end:   sp.contentStart,
					value: []byte(refXML),
				})
			}
		}
	}

	patchedDoc, err := applyXMLReplacements(docData, replacements)
	if err != nil {
		return fmt.Errorf("patch document.xml with %s: %w", refTag, err)
	}

	// 5. Atomic multi-part commit
	changes := map[string]*part{
		partPath:            targetPart,
		relName:             updatedRelPart,
		ctName:              updatedCtPart,
		"word/document.xml": d.partWithData("word/document.xml", patchedDoc),
	}

	return d.commitWordPackageChange(changes)
}

func ensureDocumentRelPart(relPart *part, relID, relType, target string) (*part, error) {
	root, err := parseWordXML(relPart.data)
	if err != nil {
		return nil, fmt.Errorf("parse relationships: %w", err)
	}
	for _, child := range root.children {
		if child.name.Local == "Relationship" && attrValue(child, "Id") == relID {
			return relPart, nil
		}
	}
	entry := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, escapeXMLString(relID), escapeXMLString(relType), escapeXMLString(target))
	patched, err := insertElementContent(relPart.data, root, []byte(entry))
	if err != nil {
		return nil, err
	}
	relPart.data = patched
	return relPart, nil
}

func ensureContentTypePart(ctPart *part, partName, contentType string) (*part, error) {
	root, err := parseWordXML(ctPart.data)
	if err != nil {
		return nil, fmt.Errorf("parse content types: %w", err)
	}
	for _, child := range root.children {
		if child.name.Local == "Override" && strings.EqualFold(attrValue(child, "PartName"), partName) {
			return ctPart, nil
		}
	}
	entry := fmt.Sprintf(`<Override PartName="%s" ContentType="%s"/>`, escapeXMLString(partName), escapeXMLString(contentType))
	patched, err := insertElementContent(ctPart.data, root, []byte(entry))
	if err != nil {
		return nil, err
	}
	ctPart.data = patched
	return ctPart, nil
}
