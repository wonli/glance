package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"glance/internal/document"
)

const (
	wordPackageRelationshipsNS = "http://schemas.openxmlformats.org/package/2006/relationships"
	wordContentTypesNS         = "http://schemas.openxmlformats.org/package/2006/content-types"
	wordCorePropertiesNS       = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	wordDCElementsNS           = "http://purl.org/dc/elements/1.1/"
	wordCorePropertiesType     = "application/vnd.openxmlformats-package.core-properties+xml"
)

type wordCorePropertySpec struct {
	field     string
	localName string
	namespace string
}

var wordCorePropertySpecs = []wordCorePropertySpec{
	{field: "Title", localName: "title", namespace: wordDCElementsNS},
	{field: "Subject", localName: "subject", namespace: wordDCElementsNS},
	{field: "Creator", localName: "creator", namespace: wordDCElementsNS},
	{field: "Keywords", localName: "keywords", namespace: wordCorePropertiesNS},
	{field: "Description", localName: "description", namespace: wordDCElementsNS},
}

type wordCorePackageLocation struct {
	partName string
	part     part
	found    bool
}

func (d *Document) GetWordDocumentProperties() (document.WordDocumentProperties, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return document.WordDocumentProperties{}, err
	}
	location, err := findWordCoreProperties(d.parts)
	if err != nil {
		return document.WordDocumentProperties{}, err
	}
	if !location.found {
		return document.WordDocumentProperties{}, nil
	}
	root, err := parseWordCorePropertiesRoot(location.part.data)
	if err != nil {
		return document.WordDocumentProperties{}, err
	}
	values, err := readWordCorePropertyNodes(root)
	if err != nil {
		return document.WordDocumentProperties{}, err
	}
	var result document.WordDocumentProperties
	for _, spec := range wordCorePropertySpecs {
		if node := values[spec.field]; node != nil {
			if len(node.children) != 0 {
				return document.WordDocumentProperties{}, fmt.Errorf("DOCX core property %s contains nested markup", spec.field)
			}
			setWordCorePropertyValue(&result, spec.field, node.text)
		}
	}
	return result, nil
}

func (d *Document) SetWordDocumentProperties(properties document.WordDocumentProperties) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	location, err := findWordCoreProperties(d.parts)
	if err != nil {
		return err
	}
	if !location.found && wordCorePropertiesEmpty(properties) {
		return nil
	}
	if !location.found {
		location.partName = "docProps/core.xml"
	}

	coreData := append([]byte(nil), location.part.data...)
	if !location.found {
		coreData = []byte(`<?xml version="1.0" encoding="UTF-8"?><cp:coreProperties xmlns:cp="` + wordCorePropertiesNS + `" xmlns:dc="` + wordDCElementsNS + `"></cp:coreProperties>`)
	}
	root, err := parseWordCorePropertiesRoot(coreData)
	if err != nil {
		return err
	}
	values, err := readWordCorePropertyNodes(root)
	if err != nil {
		return err
	}

	updated := coreData
	var replacements []xmlReplacement
	var additions []byte
	changed := false
	namespaces := xmlNamespacePrefixes(root)
	rootOpening := append([]byte(nil), coreData[root.start:root.openEnd]...)
	for _, spec := range wordCorePropertySpecs {
		value := wordCorePropertyValue(properties, spec.field)
		node := values[spec.field]
		if node != nil {
			if bytes.Contains(coreData[node.contentStart:node.contentEnd], []byte("<")) {
				return fmt.Errorf("DOCX core property %s contains non-text XML and cannot be safely edited", spec.field)
			}
			if node.text == value {
				continue
			}
			var escaped bytes.Buffer
			if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
				return fmt.Errorf("escape DOCX core property %s: %w", spec.field, err)
			}
			rawOpen := append([]byte(nil), coreData[node.start:node.openEnd]...)
			if needsWordSpacePreserve(value) && !rawAttributeEquals(rawOpen, "xml", "space", "preserve") {
				rawOpen, err = setRawXMLAttribute(rawOpen, "xml", "space", "preserve")
				if err != nil {
					return err
				}
			}
			if node.selfClosing {
				opening, expandErr := expandSelfClosingTag(rawOpen)
				if expandErr != nil {
					return expandErr
				}
				replacement := append([]byte(nil), opening...)
				replacement = append(replacement, escaped.Bytes()...)
				replacement = append(replacement, []byte("</"+node.rawName+">")...)
				replacements = append(replacements, xmlReplacement{start: node.start, end: node.end, value: replacement})
				changed = true
			} else if !bytes.Equal(escaped.Bytes(), coreData[node.contentStart:node.contentEnd]) || !bytes.Equal(rawOpen, coreData[node.start:node.openEnd]) {
				replacements = append(replacements, xmlReplacement{start: node.contentStart, end: node.contentEnd, value: escaped.Bytes()})
				if !bytes.Equal(rawOpen, coreData[node.start:node.openEnd]) {
					replacements = append(replacements, xmlReplacement{start: node.start, end: node.openEnd, value: rawOpen})
				}
				changed = true
			}
			continue
		}
		if value == "" {
			continue
		}
		prefix := namespaces[spec.namespace]
		if prefix == "" {
			prefix = preferredCorePropertyPrefix(spec.namespace)
			rootOpening, err = setRawXMLAttribute(rootOpening, "xmlns", prefix, spec.namespace)
			if err != nil {
				return err
			}
			namespaces[spec.namespace] = prefix
			changed = true
		}
		var escaped bytes.Buffer
		if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
			return fmt.Errorf("escape DOCX core property %s: %w", spec.field, err)
		}
		space := ""
		if needsWordSpacePreserve(value) {
			space = ` xml:space="preserve"`
		}
		additions = append(additions, []byte(fmt.Sprintf("<%s:%s%s>%s</%s:%s>", prefix, spec.localName, space, escaped.String(), prefix, spec.localName))...)
		changed = true
	}
	if root.selfClosing && len(additions) > 0 {
		opening, expandErr := expandSelfClosingTag(rootOpening)
		if expandErr != nil {
			return expandErr
		}
		replacement := append([]byte(nil), opening...)
		replacement = append(replacement, additions...)
		replacement = append(replacement, []byte("</"+root.rawName+">")...)
		replacements = append(replacements, xmlReplacement{start: root.start, end: root.end, value: replacement})
	} else {
		if !bytes.Equal(rootOpening, coreData[root.start:root.openEnd]) {
			replacements = append(replacements, xmlReplacement{start: root.start, end: root.openEnd, value: rootOpening})
		}
		if len(additions) > 0 {
			replacements = append(replacements, xmlReplacement{start: root.contentEnd, end: root.contentEnd, value: additions})
		}
	}
	if !changed {
		return nil
	}
	updated, err = applyXMLReplacements(coreData, replacements)
	if err != nil {
		return err
	}

	changes := map[string]*part{location.partName: d.partWithData(location.partName, updated)}
	rootRelationships, relName, err := ensureWordCorePropertiesRelationship(d.parts, location.partName)
	if err != nil {
		return err
	}
	if rootRelationships != nil {
		changes[relName] = rootRelationships
	}
	contentTypes, err := ensureWordCorePropertiesContentType(d.parts, location.partName)
	if err != nil {
		return err
	}
	if contentTypes != nil {
		changes["[Content_Types].xml"] = contentTypes
	}
	return d.commitWordPackageChange(changes)
}

const wordCorePropertiesRelationshipURI = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"

func wordCorePropertiesEmpty(properties document.WordDocumentProperties) bool {
	for _, spec := range wordCorePropertySpecs {
		if wordCorePropertyValue(properties, spec.field) != "" {
			return false
		}
	}
	return true
}

func wordCorePropertyValue(properties document.WordDocumentProperties, field string) string {
	switch field {
	case "Title":
		return properties.Title
	case "Subject":
		return properties.Subject
	case "Creator":
		return properties.Creator
	case "Keywords":
		return properties.Keywords
	case "Description":
		return properties.Description
	default:
		return ""
	}
}

func setWordCorePropertyValue(properties *document.WordDocumentProperties, field, value string) {
	switch field {
	case "Title":
		properties.Title = value
	case "Subject":
		properties.Subject = value
	case "Creator":
		properties.Creator = value
	case "Keywords":
		properties.Keywords = value
	case "Description":
		properties.Description = value
	}
}

var _ document.WordDocumentPropertiesEditor = (*Document)(nil)
