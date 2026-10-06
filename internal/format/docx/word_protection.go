package docx

import (
	"archive/zip"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

const (
	wordSettingsRelationshipType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings"
	wordSettingsContentType      = "application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"
	wordSettingsPath             = "word/settings.xml"
)

var _ document.WordDocumentProtectionEditor = (*Document)(nil)

// GetWordDocumentProtection checks whether editing protection is enforced in word/settings.xml.
func (d *Document) GetWordDocumentProtection() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	settingsPart, exists := findPackagePart(d.parts, wordSettingsPath)
	if !exists {
		return false, nil
	}
	root, err := parseWordXML(settingsPart.data)
	if err != nil {
		return false, fmt.Errorf("parse DOCX settings: %w", err)
	}
	prot := firstChild(root, "documentProtection")
	if prot == nil {
		return false, nil
	}
	enf := attrValue(prot, "enforcement")
	return enf == "1" || strings.EqualFold(enf, "true") || strings.EqualFold(enf, "on"), nil
}

// SetWordDocumentProtection updates or toggles document protection in word/settings.xml.
func (d *Document) SetWordDocumentProtection(readOnly bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}

	settingsPart, exists := findPackagePart(d.parts, wordSettingsPath)
	if !exists {
		if !readOnly {
			return nil
		}
		// Create new word/settings.xml
		initialData := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
			`<w:documentProtection w:edit="readOnly" w:enforcement="1"/>` +
			`</w:settings>`)
		newPart := &part{
			header: zip.FileHeader{Name: wordSettingsPath, Method: zip.Deflate},
			data:   initialData,
		}
		changes := map[string]*part{wordSettingsPath: newPart}

		relPart, err := ensureWordSettingsRelationship(d.parts)
		if err != nil {
			return err
		}
		if relPart != nil {
			changes[relPart.header.Name] = relPart
		}

		ctPart, err := ensureWordSettingsContentType(d.parts)
		if err != nil {
			return err
		}
		if ctPart != nil {
			changes[ctPart.header.Name] = ctPart
		}

		return d.commitWordPackageChange(changes)
	}

	root, err := parseWordXML(settingsPart.data)
	if err != nil {
		return fmt.Errorf("parse DOCX settings: %w", err)
	}
	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}
	prot := firstChild(root, "documentProtection")

	var updated []byte
	if readOnly {
		tag := fmt.Sprintf(`<%s:documentProtection %s:edit="readOnly" %s:enforcement="1"/>`, prefix, prefix, prefix)
		if prot != nil {
			enf := attrValue(prot, "enforcement")
			edit := attrValue(prot, "edit")
			if enf == "1" && (edit == "readOnly" || edit == "") {
				return nil
			}
			updated, err = applyXMLReplacements(settingsPart.data, []xmlReplacement{{start: prot.start, end: prot.end, value: []byte(tag)}})
			if err != nil {
				return err
			}
		} else {
			updated, err = insertElementContent(settingsPart.data, root, []byte(tag))
			if err != nil {
				return err
			}
		}
	} else {
		if prot == nil {
			return nil
		}
		updated, err = applyXMLReplacements(settingsPart.data, []xmlReplacement{{start: prot.start, end: prot.end, value: nil}})
		if err != nil {
			return err
		}
	}

	changes := map[string]*part{wordSettingsPath: d.partWithData(wordSettingsPath, updated)}
	relPart, err := ensureWordSettingsRelationship(d.parts)
	if err != nil {
		return err
	}
	if relPart != nil {
		changes[relPart.header.Name] = relPart
	}

	ctPart, err := ensureWordSettingsContentType(d.parts)
	if err != nil {
		return err
	}
	if ctPart != nil {
		changes[ctPart.header.Name] = ctPart
	}

	return d.commitWordPackageChange(changes)
}

func ensureWordSettingsRelationship(parts []part) (*part, error) {
	const relPath = "word/_rels/document.xml.rels"
	relPart, exists := findPackagePart(parts, relPath)
	if !exists {
		relPart = part{
			header: zip.FileHeader{Name: relPath, Method: zip.Deflate},
			data:   []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="` + wordPackageRelationshipsNS + `"/>`),
		}
	}
	root, err := parseWordXML(relPart.data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX document relationships: %w", err)
	}
	for _, child := range root.children {
		if child.name.Local == "Relationship" && isWordSettingsRelationship(wordPackageAttribute(child, "Type")) {
			return nil, nil
		}
	}
	id := nextPackageRelationshipID(root)
	child := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="settings.xml"/>`, id, wordSettingsRelationshipType)
	patched, err := insertElementContent(relPart.data, root, []byte(child))
	if err != nil {
		return nil, err
	}
	relPart.data = patched
	return &relPart, nil
}

func isWordSettingsRelationship(relType string) bool {
	return strings.EqualFold(relType, wordSettingsRelationshipType)
}

func ensureWordSettingsContentType(parts []part) (*part, error) {
	const name = "[Content_Types].xml"
	contentTypesPart, exists := findPackagePart(parts, name)
	if !exists {
		return nil, errors.New("DOCX package is missing [Content_Types].xml")
	}
	root, err := parseWordXML(contentTypesPart.data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX content types: %w", err)
	}
	for _, child := range root.children {
		if child.name.Local == "Override" {
			partName := strings.TrimPrefix(wordPackageAttribute(child, "PartName"), "/")
			if strings.EqualFold(partName, wordSettingsPath) {
				return nil, nil
			}
		}
	}
	child := fmt.Sprintf(`<Override PartName="/%s" ContentType="%s"/>`, wordSettingsPath, wordSettingsContentType)
	patched, err := insertElementContent(contentTypesPart.data, root, []byte(child))
	if err != nil {
		return nil, err
	}
	contentTypesPart.data = patched
	return &contentTypesPart, nil
}
