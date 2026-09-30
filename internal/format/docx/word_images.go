package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math/big"
	"path"
	"strconv"
	"strings"
)

const (
	maxWordImagePixels = 32_000_000
	maxWordImageEMU    = int64(100 * 914400)
	wordPixelToEMU     = int64(9525)
)

func (d *Document) InsertInlineImage(paragraphID, afterInlineID, mediaType string, data []byte) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", err
	}
	config, normalizedType, extension, err := validateWordImage(data, mediaType)
	if err != nil {
		return "", err
	}
	paragraph, ok := d.wordParagraphTargets[paragraphID]
	if !ok || paragraph == nil {
		return "", fmt.Errorf("Word paragraph %q is not editable", paragraphID)
	}
	offset, run, err := d.imageInsertionPoint(paragraphID, afterInlineID)
	if err != nil {
		return "", err
	}
	relationshipID := d.nextWordRelationshipID()
	mediaName := d.nextWordMediaName(extension)
	docPrID := d.nextWordDocPrID()
	imageID := "img-" + strconv.Itoa(docPrID)
	widthEMU, heightEMU := int64(config.Width)*wordPixelToEMU, int64(config.Height)*wordPixelToEMU
	drawing := wordDrawingMarkup(paragraph.rawName, docPrID, relationshipID, widthEMU, heightEMU, path.Base(mediaName))
	if run != nil {
		drawing, err = wordDrawingChildMarkup(drawing, elementPrefix(paragraph.rawName))
		if err != nil {
			return "", err
		}
	}
	documentData, err := applyXMLReplacements(d.parts[d.documentIdx].data, []xmlReplacement{{start: offset, end: offset, value: drawing}})
	if err != nil {
		return "", err
	}
	relationshipPart, err := d.addWordRelationship(relationshipID, path.Join("media", path.Base(mediaName)))
	if err != nil {
		return "", err
	}
	contentTypePart, err := d.addWordImageContentType(mediaName, normalizedType, extension)
	if err != nil {
		return "", err
	}
	changes := map[string]*part{
		"word/document.xml":            d.partWithData("word/document.xml", documentData),
		"word/_rels/document.xml.rels": relationshipPart,
		"[Content_Types].xml":          contentTypePart,
		mediaName:                      &part{header: zip.FileHeader{Name: mediaName, Method: zip.Deflate}, data: append([]byte(nil), data...)},
	}
	if err := d.commitWordImagePackageChange(changes); err != nil {
		return "", err
	}
	return imageID, nil
}

func (d *Document) ReplaceInlineImage(imageID, mediaType string, data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	_, normalizedType, extension, err := validateWordImage(data, mediaType)
	if err != nil {
		return err
	}
	target, ok := d.wordImageTargets[imageID]
	if !ok || target.blip == nil {
		return fmt.Errorf("Word image %q is not editable", imageID)
	}
	relationshipID := d.nextWordRelationshipID()
	mediaName := d.nextWordMediaName(extension)
	opening := d.parts[d.documentIdx].data[target.blip.start:target.blip.openEnd]
	patchedOpening, err := setRawXMLAttribute(opening, "r", "embed", relationshipID)
	if err != nil {
		return err
	}
	documentData, err := applyXMLReplacements(d.parts[d.documentIdx].data, []xmlReplacement{{start: target.blip.start, end: target.blip.openEnd, value: patchedOpening}})
	if err != nil {
		return err
	}
	relPart, err := d.addWordRelationship(relationshipID, path.Join("media", path.Base(mediaName)))
	if err != nil {
		return err
	}
	contentTypePart, err := d.addWordImageContentType(mediaName, normalizedType, extension)
	if err != nil {
		return err
	}
	changes := map[string]*part{
		"word/document.xml":            d.partWithData("word/document.xml", documentData),
		"word/_rels/document.xml.rels": relPart,
		"[Content_Types].xml":          contentTypePart,
		mediaName:                      &part{header: zip.FileHeader{Name: mediaName, Method: zip.Deflate}, data: append([]byte(nil), data...)},
	}
	if err := d.pruneUnusedImageRelationship(changes, target.relationshipID); err != nil {
		return err
	}
	return d.commitWordImagePackageChange(changes)
}

func (d *Document) MoveInlineImage(imageID, targetParagraphID, afterInlineID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	target, ok := d.wordImageTargets[imageID]
	if !ok || target.drawing == nil {
		return fmt.Errorf("Word image %q is not editable", imageID)
	}
	paragraph, ok := d.wordParagraphTargets[targetParagraphID]
	if !ok || paragraph == nil {
		return fmt.Errorf("Word paragraph %q is not editable", targetParagraphID)
	}
	if afterInlineID == imageID {
		return nil
	}
	offset, targetRun, err := d.imageInsertionPoint(targetParagraphID, afterInlineID)
	if err != nil {
		return err
	}
	if d.imageMoveIsNoop(imageID, targetParagraphID, afterInlineID) {
		return nil
	}
	documentData := d.parts[d.documentIdx].data
	markup := append([]byte(nil), documentData[target.drawing.start:target.drawing.end]...)
	if targetRun == nil {
		prefix := elementPrefix(paragraph.rawName)
		markup = append(append([]byte("<"+prefix+":r>"), markup...), []byte("</"+prefix+":r>")...)
	}
	removeStart, removeEnd := target.drawing.start, target.drawing.end
	if target.paragraphID == targetParagraphID && offset > removeStart {
		offset -= removeEnd - removeStart
	}
	patched, err := applyXMLReplacements(documentData, []xmlReplacement{
		{start: removeStart, end: removeEnd},
		{start: offset, end: offset, value: markup},
	})
	if err != nil {
		return err
	}
	return d.commitWordImagePackageChange(map[string]*part{"word/document.xml": d.partWithData("word/document.xml", patched)})
}

func (d *Document) ResizeInlineImage(imageID string, widthEMU int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	target, ok := d.wordImageTargets[imageID]
	if !ok || target.extent == nil {
		return fmt.Errorf("Word image %q has no editable inline extent", imageID)
	}
	oldWidth, widthErr := strconv.ParseInt(attrValue(target.extent, "cx"), 10, 64)
	oldHeight, heightErr := strconv.ParseInt(attrValue(target.extent, "cy"), 10, 64)
	if widthErr != nil || heightErr != nil || oldWidth <= 0 || oldHeight <= 0 {
		return fmt.Errorf("Word image %q has an invalid aspect ratio", imageID)
	}
	if widthEMU <= 0 || widthEMU > maxWordImageEMU {
		return fmt.Errorf("Word image width must be between 1 and %d EMU", maxWordImageEMU)
	}
	newHeight, err := proportionalWordImageHeight(widthEMU, oldWidth, oldHeight)
	if err != nil || newHeight <= 0 || newHeight > maxWordImageEMU {
		return fmt.Errorf("resized Word image height exceeds the supported 100-inch limit")
	}
	if oldWidth == widthEMU && oldHeight == newHeight {
		return nil
	}
	data := d.parts[d.documentIdx].data
	opening := data[target.extent.start:target.extent.openEnd]
	patchedOpening, err := setRawXMLAttribute(opening, "", "cx", strconv.FormatInt(widthEMU, 10))
	if err != nil {
		return err
	}
	patchedOpening, err = setRawXMLAttribute(patchedOpening, "", "cy", strconv.FormatInt(newHeight, 10))
	if err != nil {
		return err
	}
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: target.extent.start, end: target.extent.openEnd, value: patchedOpening}})
	if err != nil {
		return err
	}
	return d.commitWordImagePackageChange(map[string]*part{"word/document.xml": d.partWithData("word/document.xml", patched)})
}

func (d *Document) DeleteInlineImage(imageID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	target, ok := d.wordImageTargets[imageID]
	if !ok || target.drawing == nil {
		return fmt.Errorf("Word image %q is not editable", imageID)
	}
	data := d.parts[d.documentIdx].data
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: target.drawing.start, end: target.drawing.end}})
	if err != nil {
		return err
	}
	changes := map[string]*part{"word/document.xml": d.partWithData("word/document.xml", patched)}
	if err := d.pruneUnusedImageRelationship(changes, target.relationshipID); err != nil {
		return err
	}
	return d.commitWordImagePackageChange(changes)
}

func validateWordImage(data []byte, mediaType string) (image.Config, string, string, error) {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	var extension string
	switch mediaType {
	case "image/png":
		extension = "png"
	case "image/jpeg":
		extension = "jpg"
	default:
		return image.Config{}, "", "", fmt.Errorf("unsupported inline image media type %q", mediaType)
	}
	if len(data) == 0 {
		return image.Config{}, "", "", errors.New("inline image data is empty")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Config{}, "", "", fmt.Errorf("decode inline image: %w", err)
	}
	wantFormat := map[string]string{"image/png": "png", "image/jpeg": "jpeg"}[mediaType]
	if format != wantFormat {
		return image.Config{}, "", "", fmt.Errorf("inline image bytes are %q, not %q", format, wantFormat)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxWordImagePixels/config.Height {
		return image.Config{}, "", "", fmt.Errorf("inline image exceeds the %d-pixel decode limit", maxWordImagePixels)
	}
	return config, mediaType, extension, nil
}

func proportionalWordImageHeight(width, oldWidth, oldHeight int64) (int64, error) {
	numerator := new(big.Int).Mul(big.NewInt(width), big.NewInt(oldHeight))
	denominator := big.NewInt(oldWidth)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, errors.New("resized Word image height overflows")
	}
	return quotient.Int64(), nil
}

func (d *Document) imageInsertionPoint(paragraphID, afterInlineID string) (int, *wordXMLNode, error) {
	paragraph := d.wordParagraphTargets[paragraphID]
	if paragraph == nil {
		return 0, nil, fmt.Errorf("Word paragraph %q is not editable", paragraphID)
	}
	if afterInlineID == "" {
		if pPr := firstChild(paragraph, "pPr"); pPr != nil {
			return pPr.end, nil, nil
		}
		return paragraph.contentStart, nil, nil
	}
	anchor, ok := d.wordInlineTargets[afterInlineID]
	if !ok || anchor.node == nil || anchor.paragraphID != paragraphID {
		return 0, nil, fmt.Errorf("inline anchor %q is not in Word paragraph %q", afterInlineID, paragraphID)
	}
	if anchor.run == nil {
		return 0, nil, fmt.Errorf("inline anchor %q cannot accept an image", afterInlineID)
	}
	return anchor.node.end, anchor.run, nil
}

func (d *Document) imageMoveIsNoop(imageID, paragraphID, afterInlineID string) bool {
	target := d.wordImageTargets[imageID]
	if target.paragraphID != paragraphID {
		return false
	}
	ids := d.wordParagraphInlines[paragraphID]
	index := -1
	for i, id := range ids {
		if id == imageID {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	if afterInlineID == "" {
		return index == 0
	}
	for i, id := range ids {
		if id == afterInlineID {
			return index == i+1
		}
	}
	return false
}

func (d *Document) nextWordRelationshipID() string {
	index := packagePartIndex(d.parts, "word/_rels/document.xml.rels")
	used := map[string]bool{}
	if index >= 0 {
		if root, err := parseWordXML(d.parts[index].data); err == nil {
			for _, child := range root.children {
				if child.name.Local == "Relationship" {
					used[attrValue(child, "Id")] = true
				}
			}
		}
	}
	for n := 1; ; n++ {
		id := "rId" + strconv.Itoa(n)
		if !used[id] {
			return id
		}
	}
}

func (d *Document) nextWordMediaName(extension string) string {
	used := make(map[string]bool, len(d.parts))
	for _, value := range d.parts {
		used[value.header.Name] = true
	}
	for n := 1; ; n++ {
		name := fmt.Sprintf("word/media/glance-image-%d.%s", n, extension)
		if !used[name] {
			return name
		}
	}
}

func (d *Document) nextWordDocPrID() int {
	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		return 1
	}
	maximum := 0
	var visit func(*wordXMLNode)
	visit = func(node *wordXMLNode) {
		if node.name.Local == "docPr" {
			if value, parseErr := strconv.Atoi(attrValue(node, "id")); parseErr == nil && value > maximum {
				maximum = value
			}
		}
		for _, child := range node.children {
			visit(child)
		}
	}
	visit(root)
	return maximum + 1
}

func wordDrawingMarkup(paragraphRawName string, docPrID int, relationshipID string, widthEMU, heightEMU int64, name string) []byte {
	w := elementPrefix(paragraphRawName)
	markup := fmt.Sprintf(`<%s:r><%s:drawing xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:docPr id="%d" name="%s" descr=""/><wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></%s:drawing></%s:r>`, w, w, widthEMU, heightEMU, docPrID, escapeXMLString(name), escapeXMLString(name), escapeXMLString(relationshipID), widthEMU, heightEMU, w, w)
	return []byte(markup)
}

func wordDrawingChildMarkup(fragment []byte, prefix string) ([]byte, error) {
	start := bytes.Index(fragment, []byte("<"+prefix+":drawing"))
	endTag := []byte("</" + prefix + ":drawing>")
	if start < 0 {
		return nil, errors.New("generated Word image has no drawing element")
	}
	end := bytes.Index(fragment[start:], endTag)
	if end < 0 {
		return nil, errors.New("generated Word image drawing is incomplete")
	}
	end += start + len(endTag)
	return append([]byte(nil), fragment[start:end]...), nil
}

func escapeXMLString(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

func (d *Document) addWordRelationship(id, target string) (*part, error) {
	const name = "word/_rels/document.xml.rels"
	partValue, exists := findPackagePart(d.parts, name)
	if !exists {
		data := []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`)
		partValue = part{header: zip.FileHeader{Name: name, Method: zip.Deflate}, data: data}
	}
	root, err := parseWordXML(partValue.data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX relationships: %w", err)
	}
	child := fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="%s"/>`, escapeXMLString(id), escapeXMLString(target))
	patched, err := insertElementContent(partValue.data, root, []byte(child))
	if err != nil {
		return nil, err
	}
	partValue.data = patched
	return &partValue, nil
}

func (d *Document) addWordImageContentType(mediaName, mediaType, extension string) (*part, error) {
	const name = "[Content_Types].xml"
	partValue, exists := findPackagePart(d.parts, name)
	if !exists {
		return nil, errors.New("DOCX package is missing [Content_Types].xml")
	}
	root, err := parseWordXML(partValue.data)
	if err != nil {
		return nil, fmt.Errorf("parse DOCX content types: %w", err)
	}
	types := readContentTypes(d.parts, map[string]int{name: packagePartIndex(d.parts, name)})
	if current := types.defaults[extension]; current == mediaType {
		return &partValue, nil
	}
	var child string
	var patched []byte
	if current := types.defaults[extension]; current != "" {
		child = fmt.Sprintf(`<Override PartName="/%s" ContentType="%s"/>`, escapeXMLString(mediaName), escapeXMLString(mediaType))
		patched, err = insertElementContent(partValue.data, root, []byte(child))
	} else {
		child = fmt.Sprintf(`<Default Extension="%s" ContentType="%s"/>`, escapeXMLString(extension), escapeXMLString(mediaType))
		firstOverride := (*wordXMLNode)(nil)
		for _, node := range root.children {
			if node.name.Local == "Override" {
				firstOverride = node
				break
			}
		}
		if firstOverride != nil {
			patched, err = applyXMLReplacements(partValue.data, []xmlReplacement{{start: firstOverride.start, end: firstOverride.start, value: []byte(child)}})
		} else {
			patched, err = insertElementContent(partValue.data, root, []byte(child))
		}
	}
	if err != nil {
		return nil, err
	}
	partValue.data = patched
	return &partValue, nil
}

func (d *Document) partWithData(name string, data []byte) *part {
	value, exists := findPackagePart(d.parts, name)
	if !exists {
		return &part{header: zip.FileHeader{Name: name, Method: zip.Deflate}, data: append([]byte(nil), data...)}
	}
	value.data = append([]byte(nil), data...)
	return &value
}

func (d *Document) commitWordImagePackageChange(changes map[string]*part) error {
	names := make([]string, 0, len(changes))
	for name := range changes {
		names = append(names, name)
	}
	before := d.captureWordHistoryLocked(names...)
	updated := make([]part, 0, len(d.parts)+len(changes))
	seen := make(map[string]bool, len(changes))
	for _, current := range d.parts {
		name := current.header.Name
		if changed, ok := changes[name]; ok {
			seen[name] = true
			if changed != nil {
				updated = append(updated, clonePackagePart(*changed))
			}
			continue
		}
		updated = append(updated, current)
	}
	for name, changed := range changes {
		if !seen[name] && changed != nil {
			updated = append(updated, clonePackagePart(*changed))
		}
	}
	previousParts, previousIndex := d.parts, d.documentIdx
	d.parts = updated
	d.documentIdx = packagePartIndex(updated, "word/document.xml")
	if d.documentIdx < 0 {
		d.parts, d.documentIdx = previousParts, previousIndex
		return errors.New("DOCX image operation removed word/document.xml")
	}
	if err := d.reparse(); err != nil {
		d.parts, d.documentIdx = previousParts, previousIndex
		if restoreErr := d.reparse(); restoreErr != nil {
			return fmt.Errorf("image edit failed (%v), and old document reparse failed (%v)", err, restoreErr)
		}
		return fmt.Errorf("reparse DOCX image edit: %w", err)
	}
	d.recordWordHistoryLocked(before)
	return nil
}

func (d *Document) pruneUnusedImageRelationship(changes map[string]*part, relationshipID string) error {
	if relationshipID == "" {
		return nil
	}
	virtual := wordPackageWithChanges(d.parts, changes)
	documentIndex := packagePartIndex(virtual, "word/document.xml")
	relationshipIndex := packagePartIndex(virtual, "word/_rels/document.xml.rels")
	if documentIndex < 0 || relationshipIndex < 0 {
		return nil
	}
	documentRoot, err := parseWordXML(virtual[documentIndex].data)
	if err != nil {
		return err
	}
	if documentReferencesRelationship(documentRoot, relationshipID) {
		return nil
	}
	relationshipRoot, err := parseWordXML(virtual[relationshipIndex].data)
	if err != nil {
		return err
	}
	var mediaTarget string
	var remove *wordXMLNode
	for _, node := range relationshipRoot.children {
		if node.name.Local == "Relationship" && attrValue(node, "Id") == relationshipID {
			mediaTarget = resolvePartRelationshipTarget("word/document.xml", attrValue(node, "Target"))
			remove = node
			break
		}
	}
	if remove == nil {
		return nil
	}
	relData := virtual[relationshipIndex].data
	patchedRel, err := applyXMLReplacements(relData, []xmlReplacement{{start: remove.start, end: remove.end}})
	if err != nil {
		return err
	}
	changes["word/_rels/document.xml.rels"] = d.partWithData("word/_rels/document.xml.rels", patchedRel)
	virtual = wordPackageWithChanges(d.parts, changes)
	if mediaTarget != "" && !wordMediaReferenced(virtual, mediaTarget) {
		changes[mediaTarget] = nil
	}
	return nil
}

func wordPackageWithChanges(parts []part, changes map[string]*part) []part {
	result := make([]part, 0, len(parts)+len(changes))
	seen := make(map[string]bool, len(changes))
	for _, current := range parts {
		if changed, ok := changes[current.header.Name]; ok {
			seen[current.header.Name] = true
			if changed != nil {
				result = append(result, *changed)
			}
			continue
		}
		result = append(result, current)
	}
	for name, changed := range changes {
		if !seen[name] && changed != nil {
			result = append(result, *changed)
		}
	}
	return result
}

func documentReferencesRelationship(root *wordXMLNode, relationshipID string) bool {
	var visit func(*wordXMLNode) bool
	visit = func(node *wordXMLNode) bool {
		for _, attr := range node.attrs {
			if attr.Value == relationshipID {
				return true
			}
		}
		for _, child := range node.children {
			if visit(child) {
				return true
			}
		}
		return false
	}
	return visit(root)
}

func wordMediaReferenced(parts []part, target string) bool {
	for _, value := range parts {
		if !strings.HasSuffix(strings.ToLower(value.header.Name), ".rels") {
			continue
		}
		root, err := parseWordXML(value.data)
		if err != nil {
			continue
		}
		source := relationshipSourcePart(value.header.Name)
		for _, node := range root.children {
			if node.name.Local != "Relationship" || strings.EqualFold(attrValue(node, "TargetMode"), "External") {
				continue
			}
			if resolvePartRelationshipTarget(source, attrValue(node, "Target")) == target {
				return true
			}
		}
	}
	return false
}

func relationshipSourcePart(relName string) string {
	directory, file := path.Split(relName)
	directory = strings.TrimSuffix(directory, "_rels/")
	file = strings.TrimSuffix(file, ".rels")
	if file == "." || file == "" {
		return ""
	}
	return path.Clean(path.Join(directory, file))
}

func resolvePartRelationshipTarget(source, target string) string {
	if target == "" {
		return ""
	}
	if strings.HasPrefix(target, "/") {
		return path.Clean(strings.TrimPrefix(target, "/"))
	}
	return path.Clean(path.Join(path.Dir(source), target))
}
