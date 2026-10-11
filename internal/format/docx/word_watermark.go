package docx

import (
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordWatermarkEditor = (*Document)(nil)

const (
	watermarkShapeID = "PowerPlusWaterMarkObject"
)

// GetWatermark inspects the document headers for a watermark string.
func (d *Document) GetWatermark() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", err
	}
	headerPart, exists := findPackagePart(d.parts, wordHeaderPath)
	if !exists {
		return "", nil
	}

	root, err := parseWordXML(headerPart.data)
	if err != nil {
		return "", fmt.Errorf("parse header XML: %w", err)
	}

	text := findWatermarkString(root)
	return text, nil
}

// SetWatermark adds or updates a diagonal text watermark in the document header.
func (d *Document) SetWatermark(text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part missing")
	}

	cleanText := strings.TrimSpace(text)
	if cleanText == "" {
		return d.clearWatermarkLocked()
	}

	var headerXML []byte
	if headerPart, exists := findPackagePart(d.parts, wordHeaderPath); exists {
		root, err := parseWordXML(headerPart.data)
		if err != nil {
			return fmt.Errorf("parse header XML: %w", err)
		}
		if root.name.Local != "hdr" {
			return errors.New("DOCX header part has an unexpected root element")
		}
		textPaths := findWatermarkTextPaths(root)
		if len(textPaths) > 0 {
			current := attrValue(textPaths[0], "string")
			if current == cleanText {
				return nil
			}
			replacements := make([]xmlReplacement, 0, len(textPaths))
			for _, textPath := range textPaths {
				opening := headerPart.data[textPath.start:textPath.openEnd]
				patched, err := setRawXMLAttribute(opening, "", "string", cleanText)
				if err != nil {
					return fmt.Errorf("update DOCX watermark text: %w", err)
				}
				replacements = append(replacements, xmlReplacement{start: textPath.start, end: textPath.openEnd, value: patched})
			}
			headerXML, err = applyXMLReplacements(headerPart.data, replacements)
			if err != nil {
				return fmt.Errorf("update DOCX watermark text: %w", err)
			}
		} else {
			headerXML, err = insertElementContent(headerPart.data, root, watermarkParagraphXML(cleanText))
			if err != nil {
				return fmt.Errorf("insert DOCX watermark: %w", err)
			}
		}
	} else {
		headerXML = []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
		headerXML = append(headerXML, watermarkParagraphXML(cleanText)...)
		headerXML = append(headerXML, []byte(`</w:hdr>`)...)
	}
	return d.applyHeaderFooterLocked(wordHeaderPath, wordHeaderRelID, wordHeaderRelType, wordHeaderContentType, "headerReference", headerXML)
}

// ClearWatermark removes any existing watermark from the document.
func (d *Document) ClearWatermark() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	return d.clearWatermarkLocked()
}

func (d *Document) clearWatermarkLocked() error {
	headerPart, exists := findPackagePart(d.parts, wordHeaderPath)
	if !exists {
		return nil
	}

	root, err := parseWordXML(headerPart.data)
	if err != nil {
		return fmt.Errorf("parse header XML: %w", err)
	}

	picts := findWatermarkPicts(root)
	if len(picts) == 0 {
		return nil
	}
	replacements := make([]xmlReplacement, 0, len(picts))
	for _, pict := range picts {
		replacements = append(replacements, xmlReplacement{start: pict.start, end: pict.end})
	}
	updatedHeader, err := applyXMLReplacements(headerPart.data, replacements)
	if err != nil {
		return fmt.Errorf("remove DOCX watermark: %w", err)
	}
	changes := map[string]*part{wordHeaderPath: d.partWithData(wordHeaderPath, updatedHeader)}
	return d.commitWordPackageChange(changes)
}

func findWatermarkString(node *wordXMLNode) string {
	for _, textPath := range findWatermarkTextPaths(node) {
		if value := attrValue(textPath, "string"); value != "" {
			return value
		}
	}
	return ""
}

func findWatermarkTextPaths(node *wordXMLNode) []*wordXMLNode {
	var matches []*wordXMLNode
	var walk func(*wordXMLNode, bool)
	walk = func(current *wordXMLNode, inWatermarkShape bool) {
		if current == nil {
			return
		}
		if current.name.Local == "shape" && attrValue(current, "id") == watermarkShapeID {
			inWatermarkShape = true
		}
		if inWatermarkShape && (current.name.Local == "textpath" || current.name.Local == "textPath") {
			matches = append(matches, current)
		}
		for _, child := range current.children {
			walk(child, inWatermarkShape)
		}
	}
	walk(node, false)
	return matches
}

func findWatermarkPicts(node *wordXMLNode) []*wordXMLNode {
	var matches []*wordXMLNode
	var walk func(*wordXMLNode)
	walk = func(current *wordXMLNode) {
		if current == nil {
			return
		}
		if current.name.Local == "pict" && containsWatermarkShape(current) {
			matches = append(matches, current)
			return
		}
		for _, child := range current.children {
			walk(child)
		}
	}
	walk(node)
	return matches
}

func containsWatermarkShape(node *wordXMLNode) bool {
	if node == nil {
		return false
	}
	if node.name.Local == "shape" && attrValue(node, "id") == watermarkShapeID {
		return true
	}
	for _, child := range node.children {
		if containsWatermarkShape(child) {
			return true
		}
	}
	return false
}

func watermarkParagraphXML(text string) []byte {
	markup := fmt.Sprintf(`<w:p xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office"><w:pPr><w:pStyle w:val="Header"/></w:pPr><w:r><w:pict><v:shapetype id="_x0000_t136" coordsize="21600,21600" o:spt="136" adj="10800" path="m@7,l@8,m@5,21600l@6,21600e"><v:path textpathok="t" o:connecttype="rect"/><v:textpath on="t" fitshape="t"/></v:shapetype><v:shape id="%s" style="position:absolute;margin-left:0;margin-top:0;width:468pt;height:117pt;rotation:315;z-index:-251656704;mso-position-horizontal:center;mso-position-horizontal-relative:margin;mso-position-vertical:center;mso-position-vertical-relative:margin" fillcolor="#d8d8d8" stroked="f" type="#_x0000_t136"><v:fill opacity=".5"/><v:textpath style="font-family:'SimSun';font-size:1pt" string="%s"/></v:shape></w:pict></w:r></w:p>`, watermarkShapeID, escapeXMLString(text))
	return []byte(markup)
}
