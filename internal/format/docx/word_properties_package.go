package docx

import (
	"archive/zip"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

func findWordCoreProperties(parts []part) (wordCorePackageLocation, error) {
	const relationshipPartName = "_rels/.rels"
	relationshipPart, exists := findPackagePart(parts, relationshipPartName)
	if !exists {
		return findUnlinkedWordCoreProperties(parts)
	}
	root, err := parseWordXML(relationshipPart.data)
	if err != nil {
		return wordCorePackageLocation{}, fmt.Errorf("parse DOCX package relationships: %w", err)
	}
	if root.name.Local != "Relationships" || root.name.Space != wordPackageRelationshipsNS {
		return wordCorePackageLocation{}, errors.New("DOCX package relationships have an unexpected root element")
	}
	var coreRelationship *wordXMLNode
	for _, child := range root.children {
		if child.name.Local != "Relationship" || child.name.Space != wordPackageRelationshipsNS || !isWordCorePropertiesRelationship(wordPackageAttribute(child, "Type")) {
			continue
		}
		if coreRelationship != nil {
			return wordCorePackageLocation{}, errors.New("DOCX package has multiple core-properties relationships")
		}
		coreRelationship = child
	}
	if coreRelationship == nil {
		return findUnlinkedWordCoreProperties(parts)
	}
	if strings.EqualFold(wordPackageAttribute(coreRelationship, "TargetMode"), "External") {
		return wordCorePackageLocation{}, errors.New("DOCX core-properties relationship cannot be external")
	}
	partName, err := resolvePackageRootTarget(wordPackageAttribute(coreRelationship, "Target"))
	if err != nil {
		return wordCorePackageLocation{}, fmt.Errorf("invalid DOCX core-properties target: %w", err)
	}
	corePart, exists := findPackagePart(parts, partName)
	if !exists {
		return wordCorePackageLocation{}, fmt.Errorf("DOCX core-properties target %q is missing", partName)
	}
	if err := validateWordCorePropertiesContentType(parts, partName, false); err != nil {
		return wordCorePackageLocation{}, err
	}
	if _, err := parseWordCorePropertiesRoot(corePart.data); err != nil {
		return wordCorePackageLocation{}, err
	}
	return wordCorePackageLocation{partName: partName, part: corePart, found: true}, nil
}

func findUnlinkedWordCoreProperties(parts []part) (wordCorePackageLocation, error) {
	const partName = "docProps/core.xml"
	corePart, exists := findPackagePart(parts, partName)
	if !exists {
		return wordCorePackageLocation{}, nil
	}
	if err := validateWordCorePropertiesContentType(parts, partName, false); err != nil {
		return wordCorePackageLocation{}, err
	}
	if _, err := parseWordCorePropertiesRoot(corePart.data); err != nil {
		return wordCorePackageLocation{}, err
	}
	return wordCorePackageLocation{partName: partName, part: corePart, found: true}, nil
}

func parseWordCorePropertiesRoot(data []byte) (*wordXMLNode, error) {
	root, err := parseWordXML(data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX core properties: %w", err)
	}
	if root.name.Local != "coreProperties" || root.name.Space != wordCorePropertiesNS {
		return nil, errors.New("DOCX core-properties part has an unexpected root element")
	}
	return root, nil
}

func readWordCorePropertyNodes(root *wordXMLNode) (map[string]*wordXMLNode, error) {
	result := make(map[string]*wordXMLNode, len(wordCorePropertySpecs))
	for _, child := range root.children {
		for _, spec := range wordCorePropertySpecs {
			if child.name.Local != spec.localName || child.name.Space != spec.namespace {
				continue
			}
			if result[spec.field] != nil {
				return nil, fmt.Errorf("DOCX core properties contain duplicate %s elements", spec.field)
			}
			result[spec.field] = child
		}
	}
	return result, nil
}

func ensureWordCorePropertiesRelationship(parts []part, partName string) (*part, string, error) {
	const name = "_rels/.rels"
	relationshipPart, exists := findPackagePart(parts, name)
	if !exists {
		relationshipPart = part{header: zip.FileHeader{Name: name, Method: zip.Deflate}, data: []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="` + wordPackageRelationshipsNS + `"/>`)}
	}
	root, err := parseWordXML(relationshipPart.data)
	if err != nil {
		return nil, "", fmt.Errorf("parse DOCX package relationships: %w", err)
	}
	if root.name.Local != "Relationships" || root.name.Space != wordPackageRelationshipsNS {
		return nil, "", errors.New("DOCX package relationships have an unexpected root element")
	}
	count := 0
	for _, child := range root.children {
		if child.name.Local == "Relationship" && child.name.Space == wordPackageRelationshipsNS && isWordCorePropertiesRelationship(wordPackageAttribute(child, "Type")) {
			count++
		}
	}
	if count > 1 {
		return nil, "", errors.New("DOCX package has multiple core-properties relationships")
	}
	if count == 1 {
		return nil, name, nil
	}
	id := nextPackageRelationshipID(root)
	target := escapeXMLString(partName)
	if !strings.Contains(partName, "/") {
		target = escapeXMLString(partName)
	}
	child := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, id, wordCorePropertiesRelationshipURI, target)
	patched, err := insertElementContent(relationshipPart.data, root, []byte(child))
	if err != nil {
		return nil, "", err
	}
	relationshipPart.data = patched
	return &relationshipPart, name, nil
}

func ensureWordCorePropertiesContentType(parts []part, partName string) (*part, error) {
	const name = "[Content_Types].xml"
	contentTypesPart, exists := findPackagePart(parts, name)
	if !exists {
		return nil, errors.New("DOCX package is missing [Content_Types].xml")
	}
	root, err := parseWordXML(contentTypesPart.data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX content types: %w", err)
	}
	if root.name.Local != "Types" || root.name.Space != wordContentTypesNS {
		return nil, errors.New("DOCX content-types part has an unexpected root element")
	}
	cleanName := path.Clean(strings.TrimPrefix(partName, "/"))
	var matching *wordXMLNode
	for _, child := range root.children {
		if child.name.Local != "Override" || child.name.Space != wordContentTypesNS {
			continue
		}
		overrideName := strings.TrimPrefix(wordPackageAttribute(child, "PartName"), "/")
		if path.Clean(overrideName) != cleanName {
			continue
		}
		if matching != nil {
			return nil, fmt.Errorf("DOCX content types contain duplicate overrides for %q", partName)
		}
		matching = child
	}
	if matching != nil {
		if wordPackageAttribute(matching, "ContentType") != wordCorePropertiesType {
			return nil, fmt.Errorf("DOCX core-properties part %q has a conflicting content type", partName)
		}
		return nil, nil
	}
	child := fmt.Sprintf(`<Override PartName="/%s" ContentType="%s"/>`, escapeXMLString(cleanName), wordCorePropertiesType)
	patched, err := insertElementContent(contentTypesPart.data, root, []byte(child))
	if err != nil {
		return nil, err
	}
	contentTypesPart.data = patched
	return &contentTypesPart, nil
}

func validateWordCorePropertiesContentType(parts []part, partName string, require bool) error {
	const name = "[Content_Types].xml"
	contentTypesPart, exists := findPackagePart(parts, name)
	if !exists {
		return errors.New("DOCX package is missing [Content_Types].xml")
	}
	root, err := parseWordXML(contentTypesPart.data)
	if err != nil {
		return fmt.Errorf("parse DOCX content types: %w", err)
	}
	if root.name.Local != "Types" || root.name.Space != wordContentTypesNS {
		return errors.New("DOCX content-types part has an unexpected root element")
	}
	cleanName := path.Clean(strings.TrimPrefix(partName, "/"))
	count := 0
	valid := false
	for _, child := range root.children {
		if child.name.Local != "Override" || child.name.Space != wordContentTypesNS {
			continue
		}
		if path.Clean(strings.TrimPrefix(wordPackageAttribute(child, "PartName"), "/")) != cleanName {
			continue
		}
		count++
		valid = wordPackageAttribute(child, "ContentType") == wordCorePropertiesType
	}
	if count > 1 {
		return fmt.Errorf("DOCX content types contain duplicate overrides for %q", partName)
	}
	if count == 1 && !valid {
		return fmt.Errorf("DOCX core-properties part %q has a conflicting content type", partName)
	}
	if require && count == 0 {
		return fmt.Errorf("DOCX core-properties part %q has no content-type override", partName)
	}
	return nil
}

func resolvePackageRootTarget(target string) (string, error) {
	if target == "" || strings.Contains(target, "\\") {
		return "", errors.New("empty or invalid package target")
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", errors.New("package target must be an internal part path")
	}
	decoded := parsed.Path
	if decoded == "" {
		return "", errors.New("package target has no part path")
	}
	decoded = strings.TrimPrefix(decoded, "/")
	clean := path.Clean(decoded)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", errors.New("package target escapes the package root")
	}
	return clean, nil
}

func isWordCorePropertiesRelationship(value string) bool {
	return value == wordCorePropertiesRelationshipURI || value == strings.Replace(wordCorePropertiesRelationshipURI, "http://", "https://", 1)
}

func nextPackageRelationshipID(root *wordXMLNode) string {
	used := make(map[string]bool)
	for _, child := range root.children {
		if child.name.Local == "Relationship" {
			used[wordPackageAttribute(child, "Id")] = true
		}
	}
	for index := 1; ; index++ {
		candidate := fmt.Sprintf("rId%d", index)
		if !used[candidate] {
			return candidate
		}
	}
}

func wordPackageAttribute(node *wordXMLNode, localName string) string {
	for _, attr := range node.attrs {
		if attr.Name.Space == "" && attr.Name.Local == localName {
			return attr.Value
		}
	}
	return ""
}

func xmlNamespacePrefixes(root *wordXMLNode) map[string]string {
	prefixes := make(map[string]string)
	for _, attr := range root.attrs {
		if attr.Name.Space == "xmlns" {
			prefixes[attr.Value] = attr.Name.Local
		} else if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
			prefixes[attr.Value] = ""
		}
	}
	return prefixes
}

func preferredCorePropertyPrefix(namespace string) string {
	if namespace == wordDCElementsNS {
		return "dc"
	}
	return "cp"
}
