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

	watermarkXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
       xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
       xmlns:v="urn:schemas-microsoft-com:vml"
       xmlns:o="urn:schemas-microsoft-com:office:office">
  <w:p>
    <w:pPr>
      <w:pStyle w:val="Header"/>
    </w:pPr>
    <w:r>
      <w:pict>
        <v:shapetype id="_x0000_t136" coordsize="21600,21600" o:spt="136" adj="10800" path="m@7,l@8,m@5,21600l@6,21600e">
          <v:path textpathok="t" o:connecttype="rect"/>
          <v:textpath on="t" fitshape="t"/>
        </v:shapetype>
        <v:shape id="%s" style="position:absolute;margin-left:0;margin-top:0;width:468pt;height:117pt;rotation:315;z-index:-251656704;mso-position-horizontal:center;mso-position-horizontal-relative:margin;mso-position-vertical:center;mso-position-vertical-relative:margin" fillcolor="#d8d8d8" stroked="f" type="#_x0000_t136">
          <v:fill opacity=".5"/>
          <v:textpath style="font-family:'SimSun';font-size:1pt" string="%s"/>
        </v:shape>
      </w:pict>
    </w:r>
  </w:p>
</w:hdr>`, watermarkShapeID, escapeXMLString(cleanText))

	return d.applyHeaderFooterLocked(wordHeaderPath, wordHeaderRelID, wordHeaderRelType, wordHeaderContentType, "headerReference", []byte(watermarkXML))
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

	current := findWatermarkString(root)
	if current == "" {
		return nil
	}

	emptyHeader := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`)

	changes := map[string]*part{wordHeaderPath: d.partWithData(wordHeaderPath, emptyHeader)}
	return d.commitWordPackageChange(changes)
}

func findWatermarkString(node *wordXMLNode) string {
	if node == nil {
		return ""
	}
	if node.name.Local == "textpath" || node.name.Local == "textPath" {
		str := attrValue(node, "string")
		if str != "" {
			return str
		}
	}
	for _, child := range node.children {
		if res := findWatermarkString(child); res != "" {
			return res
		}
	}
	return ""
}
