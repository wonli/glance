package docx

import (
	"archive/zip"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"glance/internal/document"
)

const wordNumberingContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"

func (d *Document) SetParagraphList(paragraphID string, kind document.WordListKind) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if kind != document.WordListNone && kind != document.WordListBullet && kind != document.WordListNumbered {
		return fmt.Errorf("unsupported Word paragraph list kind %q", kind)
	}
	paragraph, ok := d.wordParagraphTargets[paragraphID]
	if !ok || paragraph == nil {
		return fmt.Errorf("Word paragraph %q is not editable", paragraphID)
	}
	model := findWordParagraph(d.wordBlocks, paragraphID)
	if model == nil {
		return fmt.Errorf("Word paragraph %q is not present in the editable projection", paragraphID)
	}
	if model.ListKind == document.WordListUnsupported {
		return fmt.Errorf("Word paragraph %q uses unsupported numbering; it was left unchanged", paragraphID)
	}
	if model.ListKind == kind {
		return nil
	}

	changes := make(map[string]*part, 4)
	var numID string
	if kind != document.WordListNone {
		relationshipPart, numberingName, err := d.ensureWordNumberingRelationship()
		if err != nil {
			return err
		}
		changes["word/_rels/document.xml.rels"] = relationshipPart
		numberingData := []byte(`<?xml version="1.0" encoding="UTF-8"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`)
		if numberingPart, exists := findPackagePart(d.parts, numberingName); exists {
			numberingData = numberingPart.data
		}
		numID = d.wordListNumIDForNeighbor(paragraph, kind)
		if numID == "" {
			var abstractID string
			numberingData, abstractID, err = ensureWordNumberingDefinition(numberingData, kind)
			if err != nil {
				return fmt.Errorf("prepare DOCX numbering: %w", err)
			}
			numberingData, numID, err = appendWordNumberingInstance(numberingData, abstractID)
			if err != nil {
				return fmt.Errorf("allocate DOCX list number: %w", err)
			}
		}
		changes[numberingName] = d.partWithData(numberingName, numberingData)
		contentTypesPart, err := ensureWordNumberingContentType(d.parts, numberingName)
		if err != nil {
			return err
		}
		changes["[Content_Types].xml"] = contentTypesPart
	}

	documentData := d.parts[d.documentIdx].data
	patched, err := patchWordParagraphNumbering(documentData, paragraph, kind, numID)
	if err != nil {
		return fmt.Errorf("edit Word paragraph list: %w", err)
	}
	changes["word/document.xml"] = d.partWithData("word/document.xml", patched)
	for name, changed := range changes {
		if changed == nil || !strings.HasSuffix(strings.ToLower(name), ".xml") {
			continue
		}
		if _, err := parseWordXML(changed.data); err != nil {
			return fmt.Errorf("validate DOCX package part %s: %w", name, err)
		}
	}
	return d.commitWordPackageChange(changes)
}

func (d *Document) SetParagraphListLevel(paragraphID string, level int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if level < 0 || level > 8 {
		return fmt.Errorf("Word list level must be between 0 and 8")
	}
	paragraph, ok := d.wordParagraphTargets[paragraphID]
	if !ok || paragraph == nil {
		return fmt.Errorf("Word paragraph %q is not editable", paragraphID)
	}
	model := findWordParagraph(d.wordBlocks, paragraphID)
	if model == nil || (model.ListKind != document.WordListBullet && model.ListKind != document.WordListNumbered) {
		return fmt.Errorf("Word paragraph %q is not in a supported list", paragraphID)
	}
	if level > model.ListMaxLevel {
		return fmt.Errorf("Word list level %d is not defined for paragraph %q", level, paragraphID)
	}
	if level == model.ListLevel {
		return nil
	}
	properties := firstChild(paragraph, "pPr")
	numPr := firstChild(properties, "numPr")
	if numPr == nil {
		return fmt.Errorf("Word paragraph %q has no direct numbering properties", paragraphID)
	}
	data := d.parts[d.documentIdx].data
	var patched []byte
	var err error
	if ilvl := firstChild(numPr, "ilvl"); ilvl != nil {
		patched, err = patchElementAttribute(data, ilvl, "val", strconv.Itoa(level))
		if err != nil {
			return err
		}
	} else {
		prefix := rawWordPrefix(numPr.rawName)
		markup := makeWordPropertyMarkup(prefix, "ilvl", "val", strconv.Itoa(level))
		patched, err = applyXMLReplacements(data, []xmlReplacement{{start: numPr.openEnd, end: numPr.openEnd, value: markup}})
		if err != nil {
			return err
		}
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) wordListNumIDForNeighbor(paragraph *wordXMLNode, kind document.WordListKind) string {
	parent := paragraph.parent
	if parent == nil {
		return ""
	}
	index := -1
	for i, child := range parent.children {
		if child == paragraph {
			index = i
			break
		}
	}
	if index < 0 {
		return ""
	}
	var previous, next string
	if index > 0 {
		previous = d.wordListNeighborID(parent.children[index-1], kind)
	}
	if index+1 < len(parent.children) {
		next = d.wordListNeighborID(parent.children[index+1], kind)
	}
	if previous != "" && next != "" && previous == next {
		return previous
	}
	if previous != "" && next == "" {
		return previous
	}
	if next != "" && previous == "" {
		return next
	}
	return ""
}

func (d *Document) wordListNeighborID(paragraph *wordXMLNode, kind document.WordListKind) string {
	if paragraph == nil || paragraph.name.Local != "p" {
		return ""
	}
	numID, level := wordParagraphNumID(paragraph)
	if numID == "" {
		return ""
	}
	partIndexes := make(map[string]int, len(d.parts))
	for i, part := range d.parts {
		partIndexes[part.header.Name] = i
	}
	numbering := readWordNumbering(d.parts, partIndexes, readDocumentRelationships(d.parts, partIndexes))
	definition, ok := numbering[numID]
	if !ok || definition.levels[level].kind != kind {
		return ""
	}
	return numID
}

func (d *Document) ensureWordNumberingRelationship() (*part, string, error) {
	const relationshipName = "word/_rels/document.xml.rels"
	relationshipPart, exists := findPackagePart(d.parts, relationshipName)
	if !exists {
		relationshipPart = part{header: zip.FileHeader{Name: relationshipName, Method: zip.Deflate}, data: []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`)}
	}
	root, err := parseWordXML(relationshipPart.data)
	if err != nil {
		return nil, "", fmt.Errorf("parse DOCX relationships: %w", err)
	}
	for _, relationship := range root.children {
		if relationship.name.Local != "Relationship" || !strings.HasSuffix(strings.ToLower(attrValue(relationship, "Type")), "/numbering") || strings.EqualFold(attrValue(relationship, "TargetMode"), "External") {
			continue
		}
		target := resolveDocumentTarget(attrValue(relationship, "Target"))
		if target == "" {
			return nil, "", errors.New("DOCX numbering relationship has an invalid target")
		}
		if _, found := findPackagePart(d.parts, target); !found {
			return nil, "", fmt.Errorf("DOCX numbering relationship target %q is missing", target)
		}
		return &relationshipPart, target, nil
	}

	const numberingName = "word/numbering.xml"
	id := d.nextWordRelationshipID()
	child := fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>`, escapeXMLString(id))
	patched, err := insertElementContent(relationshipPart.data, root, []byte(child))
	if err != nil {
		return nil, "", err
	}
	relationshipPart.data = patched
	return &relationshipPart, numberingName, nil
}

func ensureWordNumberingContentType(parts []part, partName string) (*part, error) {
	const name = "[Content_Types].xml"
	contentTypesPart, exists := findPackagePart(parts, name)
	if !exists {
		return nil, errors.New("DOCX package is missing [Content_Types].xml")
	}
	root, err := parseWordXML(contentTypesPart.data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX content types: %w", err)
	}
	for _, override := range root.children {
		if override.name.Local != "Override" || path.Clean(strings.TrimPrefix(attrValue(override, "PartName"), "/")) != path.Clean(partName) {
			continue
		}
		if attrValue(override, "ContentType") == wordNumberingContentType {
			return &contentTypesPart, nil
		}
		patched, patchErr := patchElementAttribute(contentTypesPart.data, override, "ContentType", wordNumberingContentType)
		if patchErr != nil {
			return nil, patchErr
		}
		contentTypesPart.data = patched
		return &contentTypesPart, nil
	}
	child := fmt.Sprintf(`<Override PartName="/%s" ContentType="%s"/>`, escapeXMLString(partName), wordNumberingContentType)
	patched, err := insertElementContent(contentTypesPart.data, root, []byte(child))
	if err != nil {
		return nil, err
	}
	contentTypesPart.data = patched
	return &contentTypesPart, nil
}

func ensureWordNumberingDefinition(data []byte, kind document.WordListKind) ([]byte, string, error) {
	root, err := parseWordXML(data)
	if err != nil {
		return nil, "", err
	}
	prefix := rawWordPrefix(root.rawName)
	abstracts := make(map[string]bool)
	maximum := -1
	for _, child := range root.children {
		if child.name.Local != "abstractNum" {
			continue
		}
		id := attrValue(child, "abstractNumId")
		if parsed, parseErr := strconv.Atoi(id); parseErr == nil && parsed > maximum {
			maximum = parsed
		}
		abstracts[id] = true
		levels := make(map[int]*wordXMLNode)
		for _, levelNode := range child.children {
			if levelNode.name.Local != "lvl" {
				continue
			}
			if level, parseErr := strconv.Atoi(attrValue(levelNode, "ilvl")); parseErr == nil && level >= 0 && level <= 8 {
				levels[level] = levelNode
			}
		}
		levelZero := levels[0]
		if levelZero == nil || !wordNumberingLevelMatchesKind(levelZero, kind) {
			continue
		}
		complete := true
		for level := 0; level <= 8; level++ {
			levelNode := levels[level]
			if levelNode == nil || !wordNumberingLevelMatchesKind(levelNode, kind) {
				complete = false
				break
			}
		}
		if complete {
			return data, id, nil
		}
	}
	abstractID := strconv.Itoa(maximum + 1)
	for abstracts[abstractID] {
		maximum++
		abstractID = strconv.Itoa(maximum + 1)
	}
	markup := wordMultilevelAbstractMarkup(prefix, abstractID, kind)
	var patched []byte
	firstNum := (*wordXMLNode)(nil)
	for _, child := range root.children {
		if child.name.Local == "num" {
			firstNum = child
			break
		}
	}
	if firstNum != nil {
		patched, err = applyXMLReplacements(data, []xmlReplacement{{start: firstNum.start, end: firstNum.start, value: []byte(markup)}})
	} else {
		patched, err = insertElementContent(data, root, []byte(markup))
	}
	if err != nil {
		return nil, "", err
	}
	return patched, abstractID, nil
}

func wordNumberingLevelMatchesKind(level *wordXMLNode, kind document.WordListKind) bool {
	format := strings.ToLower(attrValue(firstChild(level, "numFmt"), "val"))
	text := attrValue(firstChild(level, "lvlText"), "val")
	if text == "" {
		return false
	}
	switch kind {
	case document.WordListBullet:
		return format == "bullet"
	case document.WordListNumbered:
		return format == "decimal"
	default:
		return false
	}
}

func wordMultilevelAbstractMarkup(prefix, abstractID string, kind document.WordListKind) string {
	tag := func(local string) string { return q(prefix, local) }
	markup := "<" + tag("abstractNum") + " " + tag("abstractNumId") + `="` + abstractID + `">` +
		"<" + tag("multiLevelType") + " " + tag("val") + `="multilevel"/>`
	for level := 0; level <= 8; level++ {
		format := "decimal"
		marker := make([]string, level+1)
		for index := range marker {
			marker[index] = "%" + strconv.Itoa(index+1)
		}
		markerText := strings.Join(marker, ".") + "."
		if kind == document.WordListBullet {
			format = "bullet"
			markerText = wordListBulletMarker(level)
		}
		left := strconv.Itoa(720 * (level + 1))
		markup += "<" + tag("lvl") + " " + tag("ilvl") + `="` + strconv.Itoa(level) + `">` +
			"<" + tag("start") + " " + tag("val") + `="1"/>` +
			"<" + tag("numFmt") + " " + tag("val") + `="` + format + `"/>` +
			"<" + tag("lvlText") + " " + tag("val") + `="` + escapeXMLString(markerText) + `"/>` +
			"<" + tag("lvlJc") + " " + tag("val") + `="left"/>` +
			"<" + tag("pPr") + "><" + tag("tabs") + "><" + tag("tab") + " " + tag("val") + `="num" ` + tag("pos") + `="` + left + `"/></` + tag("tabs") + ">" +
			"<" + tag("ind") + " " + tag("left") + `="` + left + `" ` + tag("hanging") + `="360"/></` + tag("pPr") + ">" +
			"</" + tag("lvl") + ">"
	}
	return markup + "</" + tag("abstractNum") + ">"
}

func appendWordNumberingInstance(data []byte, abstractID string) ([]byte, string, error) {
	root, err := parseWordXML(data)
	if err != nil {
		return nil, "", err
	}
	maximum := 0
	for _, child := range root.children {
		if child.name.Local != "num" {
			continue
		}
		if value, parseErr := strconv.Atoi(attrValue(child, "numId")); parseErr == nil && value > maximum {
			maximum = value
		}
	}
	numID := strconv.Itoa(maximum + 1)
	prefix := rawWordPrefix(root.rawName)
	markup := fmt.Sprintf(`<%s %s="%s"><%s %s="%s"/></%s>`, q(prefix, "num"), q(prefix, "numId"), numID, q(prefix, "abstractNumId"), q(prefix, "val"), abstractID, q(prefix, "num"))
	patched, err := insertElementContent(data, root, []byte(markup))
	if err != nil {
		return nil, "", err
	}
	return patched, numID, nil
}

func patchWordParagraphNumbering(data []byte, paragraph *wordXMLNode, kind document.WordListKind, numID string) ([]byte, error) {
	properties := firstChild(paragraph, "pPr")
	var current *wordXMLNode
	if properties != nil {
		current = firstChild(properties, "numPr")
	}
	var value []byte
	if kind != document.WordListNone {
		prefix := rawWordPrefix(paragraph.rawName)
		tag := func(local string) string { return q(prefix, local) }
		value = []byte("<" + tag("numPr") + "><" + tag("ilvl") + " " + tag("val") + `="0"/><` + tag("numId") + " " + tag("val") + `="` + escapeXMLString(numID) + `"/></` + tag("numPr") + ">")
	}
	if current != nil {
		return applyXMLReplacements(data, []xmlReplacement{{start: current.start, end: current.end, value: value}})
	}
	if kind == document.WordListNone {
		return data, nil
	}
	if properties == nil {
		markup := wrapWordElement(rawWordPrefix(paragraph.rawName), "pPr", value)
		if paragraph.selfClosing {
			return insertElementContent(data, paragraph, markup)
		}
		return applyXMLReplacements(data, []xmlReplacement{{start: paragraph.openEnd, end: paragraph.openEnd, value: markup}})
	}
	if style := firstChild(properties, "pStyle"); style != nil {
		return applyXMLReplacements(data, []xmlReplacement{{start: style.end, end: style.end, value: value}})
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: properties.openEnd, end: properties.openEnd, value: value}})
}

func rawWordPrefix(rawName string) string {
	index := strings.IndexByte(rawName, ':')
	if index < 0 {
		return ""
	}
	return rawName[:index]
}

func q(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

var _ document.WordParagraphListEditor = (*Document)(nil)
