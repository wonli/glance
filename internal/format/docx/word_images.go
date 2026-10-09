package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"math/big"
	"path"
	"strconv"
	"strings"

	"glance/internal/document"
)

const (
	maxWordImagePixels = 32_000_000
	maxWordImageEMU    = int64(100 * 914400)
	wordPixelToEMU     = int64(9525)
)

const drawingMLRotationUnitsPerDegree = int64(60000)

func normalizeWordImageRotation(degrees float64) float64 {
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}
	if math.Abs(degrees) < 1e-9 || math.Abs(degrees-360) < 1e-9 {
		return 0
	}
	return degrees
}

func (d *Document) SetInlineImageRotation(imageID string, degrees float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if math.IsNaN(degrees) || math.IsInf(degrees, 0) {
		return errors.New("Word image rotation must be a finite angle")
	}
	target, ok := d.wordImageTargets[imageID]
	if !ok || target.drawing == nil || target.transform == nil {
		return fmt.Errorf("Word image %q has no editable DrawingML transform", imageID)
	}
	normalized := normalizeWordImageRotation(degrees)
	rotationUnits := int64(math.Round(normalized * float64(drawingMLRotationUnitsPerDegree)))
	currentUnits, _ := strconv.ParseInt(attrValue(target.transform, "rot"), 10, 64)
	if currentUnits == rotationUnits {
		return nil
	}
	opening := d.parts[d.documentIdx].data[target.transform.start:target.transform.openEnd]
	patchedOpening, err := setRawXMLAttribute(opening, "", "rot", strconv.FormatInt(rotationUnits, 10))
	if err != nil {
		return err
	}
	patched, err := applyXMLReplacements(d.parts[d.documentIdx].data, []xmlReplacement{{start: target.transform.start, end: target.transform.openEnd, value: patchedOpening}})
	if err != nil {
		return err
	}
	return d.commitWordPackageChange(map[string]*part{"word/document.xml": d.partWithData("word/document.xml", patched)})
}

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
	const maxDefaultWidthEMU = int64(5.5 * 914400)  // 5.5 inches ≈ 5,029,200 EMU
	const maxDefaultHeightEMU = int64(7.5 * 914400) // 7.5 inches ≈ 6,858,000 EMU
	if widthEMU > maxDefaultWidthEMU && widthEMU > 0 {
		heightEMU = heightEMU * maxDefaultWidthEMU / widthEMU
		widthEMU = maxDefaultWidthEMU
	}
	if heightEMU > maxDefaultHeightEMU && heightEMU > 0 {
		widthEMU = widthEMU * maxDefaultHeightEMU / heightEMU
		heightEMU = maxDefaultHeightEMU
	}
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
	if err := d.commitWordPackageChange(changes); err != nil {
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
	return d.commitWordPackageChange(changes)
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
	return d.commitWordPackageChange(map[string]*part{"word/document.xml": d.partWithData("word/document.xml", patched)})
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
	return d.commitWordPackageChange(map[string]*part{"word/document.xml": d.partWithData("word/document.xml", patched)})
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
	return d.commitWordPackageChange(changes)
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

var _ document.WordImageRotator = (*Document)(nil)
