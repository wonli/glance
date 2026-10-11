package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"glance/internal/document"
	"image"
	"strconv"
	"strings"
)

// Document wraps a PPTX presentation behind a safe format adapter.
// Third-party library types are kept strictly internal.
type shapeBodyProps struct {
	wrap         string
	autoFit      bool
	fillColorHex string
	fillGradient *document.PresentationGradient
}

// inspectZipPackage extracts presentation dimensions and preloads media.
func (d *Document) inspectZipPackage(zr *zip.Reader) {
	pkg := newPPTXPackage(zr)
	pkg.imageCache = d.mediaImages
	if len(pkg.layoutParts) > 0 {
		defaultLayout := pkg.layoutParts[0]
		defaultMaster := pkg.layoutMasterParts[defaultLayout]
		d.defaultSlideBackground = pkg.resolveBackground("", defaultLayout, defaultMaster)
		d.defaultSlideInherited = pkg.inheritedElements("", defaultLayout, defaultMaster)
	}
	files := pkg.parts
	for _, f := range zr.File {
		// Preload images from ppt/media/
		if strings.HasPrefix(f.Name, "ppt/media/") {
			imgBytes, err := readOPCPart(f)
			if err != nil {
				continue
			}
			d.mediaBytes[f.Name] = imgBytes
			if d.mediaImages[f.Name] == nil {
				if img, _, err := image.Decode(bytes.NewReader(imgBytes)); err == nil {
					d.mediaImages[f.Name] = img
				}
			}
		}
	}

	// Read presentation.xml to detect slide size and ordered slide IDs
	if presFile, ok := files["ppt/presentation.xml"]; ok {
		if data, err := readOPCPart(presFile); err == nil {
			d.parsePresentationXML(data)
		}
	}
	orderedSlideFiles := pkg.slideParts

	// Parse slide XMLs for shape text body properties (wrap, autoFit)
	if len(orderedSlideFiles) > 0 {
		d.slideBackgrounds = make([]pptxBackground, len(orderedSlideFiles))
		d.slideBackgroundChanges = make([]presentationBackgroundOverride, len(orderedSlideFiles))
		d.slideInherited = make([][]document.PresentationElement, len(orderedSlideFiles))
		d.slidePlaceholderBounds = make([]map[uint32]document.PresentationRect, len(orderedSlideFiles))
		d.slidePlaceholderTypes = make([]map[uint32]string, len(orderedSlideFiles))
		d.slidePlaceholderChanges = make([]map[uint32]string, len(orderedSlideFiles))
		d.slideShapeTransforms = make([]map[uint32]presentationTransformMetadata, len(orderedSlideFiles))
		d.slidePlaceholderXfrms = make([]map[uint32]presentationTransformMetadata, len(orderedSlideFiles))
		d.slideTransformChanges = make([]map[uint32]presentationTransformOverride, len(orderedSlideFiles))
		d.slideStyleChanges = make([]map[uint32]document.PresentationElementStylePatch, len(orderedSlideFiles))
		d.slidePictureCropStates = make([]map[uint32]presentationPictureCropState, len(orderedSlideFiles))
		d.slidePictureCropChanges = make([]map[uint32]document.PresentationCrop, len(orderedSlideFiles))
		d.slidePictureImageChanges = make([]map[uint32]presentationPictureImageOverride, len(orderedSlideFiles))
		for idx, slidePath := range orderedSlideFiles {
			layoutPart, _ := pkg.relatedPart(slidePath, pptxSlideLayoutRelationshipType)
			masterPart, _ := pkg.relatedPart(layoutPart, pptxSlideMasterRelationshipType)
			d.slideBackgrounds[idx] = pkg.resolveBackground(slidePath, layoutPart, masterPart)
			d.slideInherited[idx] = pkg.inheritedElements(slidePath, layoutPart, masterPart)
			d.slidePlaceholderBounds[idx] = pkg.placeholderBounds(slidePath, layoutPart, masterPart)
			d.slidePlaceholderTypes[idx] = pkg.placeholderTypes(slidePath)
			d.slidePlaceholderChanges[idx] = make(map[uint32]string)
			if sf, ok := files[slidePath]; ok {
				if data, err := readOPCPart(sf); err == nil {
					d.slideBodyProps[idx] = parseSlideXML(data)
					d.slideShapeTransforms[idx] = parseSlideTransformMetadata(data)
					d.slidePictureCropStates[idx] = parseSlidePictureCropStates(data)
					d.loadSlidePictureImages(pkg, idx, slidePath, data)
				}
			}
			d.slidePlaceholderXfrms[idx] = pkg.placeholderTransforms(slidePath, layoutPart, masterPart)
		}
	} else {
		// Fallback: parse all slide files matching ppt/slides/slide*.xml
		slideIdx := 0
		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
				if data, err := readOPCPart(f); err == nil {
					d.slideBodyProps[slideIdx] = parseSlideXML(data)
					d.slideShapeTransforms = append(d.slideShapeTransforms, parseSlideTransformMetadata(data))
					d.slidePictureCropStates = append(d.slidePictureCropStates, parseSlidePictureCropStates(data))
					d.loadSlidePictureImages(pkg, slideIdx, f.Name, data)
					layoutPart, _ := pkg.relatedPart(f.Name, pptxSlideLayoutRelationshipType)
					masterPart, _ := pkg.relatedPart(layoutPart, pptxSlideMasterRelationshipType)
					d.slideInherited = append(d.slideInherited, pkg.inheritedElements(f.Name, layoutPart, masterPart))
					d.slidePlaceholderBounds = append(d.slidePlaceholderBounds, pkg.placeholderBounds(f.Name, layoutPart, masterPart))
					d.slidePlaceholderTypes = append(d.slidePlaceholderTypes, pkg.placeholderTypes(f.Name))
					d.slidePlaceholderChanges = append(d.slidePlaceholderChanges, make(map[uint32]string))
					d.slidePlaceholderXfrms = append(d.slidePlaceholderXfrms, pkg.placeholderTransforms(f.Name, layoutPart, masterPart))
					d.slideTransformChanges = append(d.slideTransformChanges, make(map[uint32]presentationTransformOverride))
					d.slideStyleChanges = append(d.slideStyleChanges, make(map[uint32]document.PresentationElementStylePatch))
					d.slidePictureCropChanges = append(d.slidePictureCropChanges, make(map[uint32]document.PresentationCrop))
					d.slidePictureImageChanges = append(d.slidePictureImageChanges, make(map[uint32]presentationPictureImageOverride))
					if len(d.slideBackgrounds) <= slideIdx {
						d.slideBackgrounds = append(d.slideBackgrounds, pptxBackground{})
					}
					d.slideBackgrounds[slideIdx] = pkg.resolveBackground(f.Name, layoutPart, masterPart)
					d.slideBackgroundChanges = append(d.slideBackgroundChanges, presentationBackgroundOverride{})
					slideIdx++
				}
			}
		}
	}
}

func (d *Document) resolveSlideBackground(pkg *pptxPackage, slidePart string) pptxBackground {
	layoutPart, _ := pkg.relatedPart(slidePart, pptxSlideLayoutRelationshipType)
	masterPart, _ := pkg.relatedPart(layoutPart, pptxSlideMasterRelationshipType)
	return pkg.resolveBackground(slidePart, layoutPart, masterPart)
}

func (d *Document) loadSlidePictureImages(pkg *pptxPackage, slideIndex int, slidePart string, data []byte) {
	for shapeID, relID := range parseSlidePictureRelIDs(data) {
		target, ok := pkg.imagePartForRelationship(slidePart, relID)
		if !ok {
			continue
		}
		if d.slidePictureTargets[slideIndex] == nil {
			d.slidePictureTargets[slideIndex] = make(map[uint32]string)
		}
		d.slidePictureTargets[slideIndex][shapeID] = target
		_ = pkg.image(target)
	}
}

// parsePresentationXML parses <p:sldSz cx="..." cy="..."/> from presentation.xml
func (d *Document) parsePresentationXML(data []byte) {
	type sldSzXML struct {
		XMLName xml.Name `xml:"sldSz"`
		CX      int64    `xml:"cx,attr"`
		CY      int64    `xml:"cy,attr"`
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "sldSz" {
			var sz sldSzXML
			if err := decoder.DecodeElement(&sz, &se); err == nil {
				if sz.CX > 0 && sz.CY > 0 {
					d.widthPT = float64(sz.CX) / emuPerPoint
					d.heightPT = float64(sz.CY) / emuPerPoint
				}
			}
			break
		}
	}
}

// parsePresentationRels maps relationship IDs to target paths.
func parsePresentationRels(data []byte) map[string]string {
	rels := make(map[string]string)
	for id, rel := range parseOPCRelationships(data) {
		if rel.Target != "" {
			rels[id] = rel.Target
		}
	}
	return rels
}

// parsePresentationSlideFiles extracts ordered slide file paths from presentation.xml.
func parsePresentationSlideFiles(data []byte, rels map[string]string) []string {
	var files []string
	for _, relID := range presentationSlideRelationshipIDs(data) {
		if target, ok := rels[relID]; ok {
			if part, ok := resolveOPCTarget("ppt/presentation.xml", target); ok {
				files = append(files, part)
			}
		}
	}
	return files
}

// parseSlideXML parses text body properties, solid RGB fills, and linear RGB gradients.
func parseSlideXML(data []byte) map[uint32]shapeBodyProps {
	result := make(map[uint32]shapeBodyProps)
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var currentID uint32
	var currentWrap string
	var currentAutoFit bool
	var currentFillColorHex string
	var currentGradient *document.PresentationGradient
	gradientStopIndex := -1
	inShape := false
	inShapeProps := false
	inShapeLine := false
	inShapeSolidFill := false
	inShapeGradFill := false
	parseRGBHex := func(value string) (string, bool) {
		if len(value) != 6 {
			return "", false
		}
		if _, err := strconv.ParseUint(value, 16, 24); err != nil {
			return "", false
		}
		return "#" + strings.ToUpper(value), true
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sp":
				inShape = true
				currentID = 0
				currentWrap = ""
				currentAutoFit = false
				currentFillColorHex = ""
				currentGradient = nil
				gradientStopIndex = -1
				inShapeProps = false
				inShapeLine = false
				inShapeSolidFill = false
				inShapeGradFill = false
			case "cNvPr":
				if inShape {
					for _, a := range t.Attr {
						if a.Name.Local == "id" {
							var id uint32
							if _, err := fmt.Sscanf(a.Value, "%d", &id); err == nil {
								currentID = id
							}
						}
					}
				}
			case "spPr":
				if inShape {
					inShapeProps = true
				}
			case "ln":
				if inShapeProps {
					inShapeLine = true
				}
			case "solidFill":
				if inShapeProps && !inShapeLine && !inShapeGradFill {
					inShapeSolidFill = true
				}
			case "gradFill":
				if inShapeProps && !inShapeLine {
					currentGradient = &document.PresentationGradient{}
					inShapeGradFill = true
				}
			case "gs":
				if inShapeGradFill && currentGradient != nil {
					stop := document.PresentationGradientStop{Position: -1, Alpha: 1}
					for _, attr := range t.Attr {
						if attr.Name.Local == "pos" {
							if position, err := strconv.ParseFloat(attr.Value, 64); err == nil {
								stop.Position = position
							}
						}
					}
					currentGradient.Stops = append(currentGradient.Stops, stop)
					gradientStopIndex = len(currentGradient.Stops) - 1
				}
			case "lin":
				if inShapeGradFill && currentGradient != nil {
					for _, attr := range t.Attr {
						if attr.Name.Local == "ang" {
							if angle, err := strconv.ParseFloat(attr.Value, 64); err == nil {
								currentGradient.AngleDeg = angle / 60000
							}
						}
					}
				}
			case "path":
				if inShapeGradFill && currentGradient != nil {
					for _, attr := range t.Attr {
						if attr.Name.Local == "path" {
							currentGradient.PathType = attr.Value
						}
					}
				}
			case "srgbClr":
				for _, attr := range t.Attr {
					if attr.Name.Local != "val" {
						continue
					}
					if inShapeGradFill && currentGradient != nil && gradientStopIndex >= 0 {
						if hex, ok := parseRGBHex(attr.Value); ok {
							currentGradient.Stops[gradientStopIndex].ColorHex = hex
						}
					} else if inShapeSolidFill {
						if hex, ok := parseRGBHex(attr.Value); ok {
							currentFillColorHex = hex
						}
					}
				}
			case "alpha":
				if inShapeGradFill && currentGradient != nil && gradientStopIndex >= 0 {
					for _, attr := range t.Attr {
						if attr.Name.Local == "val" {
							if alpha, err := strconv.ParseFloat(attr.Value, 64); err == nil {
								currentGradient.Stops[gradientStopIndex].Alpha = min(1, max(0, alpha/100000))
							}
						}
					}
				}
			case "bodyPr":
				if inShape {
					for _, a := range t.Attr {
						if a.Name.Local == "wrap" {
							currentWrap = a.Value
						}
					}
				}
			case "spAutoFit", "normAutofit":
				if inShape {
					currentAutoFit = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "solidFill":
				inShapeSolidFill = false
			case "gs":
				gradientStopIndex = -1
			case "gradFill":
				inShapeGradFill = false
			case "ln":
				inShapeLine = false
			case "spPr":
				inShapeProps = false
			case "sp":
				if currentID > 0 {
					result[currentID] = shapeBodyProps{
						wrap:         currentWrap,
						autoFit:      currentAutoFit,
						fillColorHex: currentFillColorHex,
						fillGradient: currentGradient,
					}
				}
				inShape = false
			}
		}
	}
	return result
}
