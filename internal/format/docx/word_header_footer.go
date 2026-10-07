package docx

import (
	"archive/zip"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordHeaderFooterEditor = (*Document)(nil)

const (
	wordHeaderPath             = "word/header1.xml"
	wordFooterPath             = "word/footer1.xml"
	wordHeaderRelID            = "rIdHeader1"
	wordFooterRelID            = "rIdFooter1"
	wordHeaderRelType          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	wordFooterRelType          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	wordHeaderContentType      = "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"
	wordFooterContentType      = "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"
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

	pageNumberXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="%s"/></w:pPr><w:fldSimple w:instr="PAGE"><w:r><w:rPr><w:sz w:val="18"/><w:color w:val="666666"/></w:rPr><w:t>1</w:t></w:r></w:fldSimple></w:p></w:ftr>`, escapeXMLString(align))

	return d.applyHeaderFooterLocked(wordFooterPath, wordFooterRelID, wordFooterRelType, wordFooterContentType, "footerReference", []byte(pageNumberXML))
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
