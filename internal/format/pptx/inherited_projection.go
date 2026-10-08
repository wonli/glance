package pptx

import (
	"encoding/xml"
	"strconv"
	"strings"

	"glance/internal/document"
)

type placeholderKey struct {
	typeName string
	index    uint32
}

type placeholderRecord struct {
	id      uint32
	key     placeholderKey
	bounds  document.PresentationRect
	hasXfrm bool
}

// inheritedElements projects supported, non-placeholder master and layout
// artwork into a read-only snapshot layer. Slide-local shapes stay in the
// adapter's normal editable element collection.
func (p *pptxPackage) inheritedElements(slidePart, layoutPart, masterPart string) []document.PresentationElement {
	if layoutPart == "" {
		return nil
	}
	slideRoot, slideOK := p.xmlPart(slidePart)
	layoutRoot, layoutOK := p.xmlPart(layoutPart)
	if !layoutOK {
		return nil
	}
	showMaster := (!slideOK || xmlBoolDefaultTrue(slideRoot.attr("showMasterSp"))) &&
		xmlBoolDefaultTrue(layoutRoot.attr("showMasterSp"))
	colorMap, theme := p.backgroundTheme(masterPart)
	var elements []document.PresentationElement
	if showMaster && masterPart != "" {
		elements = append(elements, p.projectInheritedPart(masterPart, 0, colorMap, theme)...)
	}
	elements = append(elements, p.projectInheritedPart(layoutPart, 1, colorMap, theme)...)
	return elements
}

// placeholderBounds returns geometry for slide placeholders that do not have
// their own transform. Matches use normalized type/index pairs and require a
// unique candidate at each inherited level.
func (p *pptxPackage) placeholderBounds(slidePart, layoutPart, masterPart string) map[uint32]document.PresentationRect {
	slideRoot, ok := p.xmlPart(slidePart)
	if !ok {
		return nil
	}
	slideTree := shapeTree(slideRoot)
	if slideTree == nil {
		return nil
	}
	layoutRoot, _ := p.xmlPart(layoutPart)
	masterRoot, _ := p.xmlPart(masterPart)
	layoutTree, masterTree := shapeTree(layoutRoot), shapeTree(masterRoot)
	result := make(map[uint32]document.PresentationRect)
	for _, shape := range slideTree.Children {
		if shape.XMLName.Local != "sp" {
			continue
		}
		record, ok := placeholderRecordForShape(shape)
		if !ok || record.id == 0 || record.hasXfrm {
			continue
		}
		bounds, matched, ambiguous := uniquePlaceholderBounds(layoutTree, record.key)
		if ambiguous {
			continue
		}
		if matched && validPresentationRect(bounds) {
			result[record.id] = bounds
			continue
		}
		bounds, matched, ambiguous = uniquePlaceholderBounds(masterTree, record.key)
		if !ambiguous && matched && validPresentationRect(bounds) {
			result[record.id] = bounds
		}
	}
	return result
}

func uniquePlaceholderBounds(tree *pptxXMLNode, key placeholderKey) (document.PresentationRect, bool, bool) {
	if tree == nil {
		return document.PresentationRect{}, false, false
	}
	var match *placeholderRecord
	count := 0
	for _, shape := range tree.Children {
		if shape.XMLName.Local != "sp" {
			continue
		}
		record, ok := placeholderRecordForShape(shape)
		if !ok || record.key != key {
			continue
		}
		count++
		if count == 1 {
			recordCopy := record
			match = &recordCopy
		}
	}
	if count > 1 {
		return document.PresentationRect{}, false, true
	}
	if match == nil || !match.hasXfrm {
		return document.PresentationRect{}, false, false
	}
	return match.bounds, true, false
}

func placeholderRecordForShape(shape pptxXMLNode) (placeholderRecord, bool) {
	nv := shape.child("nvSpPr")
	if nv == nil {
		return placeholderRecord{}, false
	}
	nvPr := nv.child("nvPr")
	if nvPr == nil {
		return placeholderRecord{}, false
	}
	ph := nvPr.child("ph")
	if ph == nil {
		return placeholderRecord{}, false
	}
	cNvPr := nv.child("cNvPr")
	if cNvPr == nil {
		return placeholderRecord{}, false
	}
	id, err := strconv.ParseUint(cNvPr.attr("id"), 10, 32)
	if err != nil {
		return placeholderRecord{}, false
	}
	key := placeholderKey{typeName: ph.attr("type"), index: 0}
	if key.typeName == "" {
		key.typeName = "obj"
	}
	if idx := ph.attr("idx"); idx != "" {
		index, parseErr := strconv.ParseUint(idx, 10, 32)
		if parseErr != nil {
			return placeholderRecord{}, false
		}
		key.index = uint32(index)
	}
	bounds, hasXfrm := shapeBounds(shape)
	return placeholderRecord{id: uint32(id), key: key, bounds: bounds, hasXfrm: hasXfrm}, true
}

func (p *pptxPackage) projectInheritedPart(partPath string, partIndex int, colorMap, theme map[string]string) []document.PresentationElement {
	root, ok := p.xmlPart(partPath)
	if !ok {
		return nil
	}
	tree := shapeTree(root)
	if tree == nil {
		return nil
	}
	var result []document.PresentationElement
	for _, node := range tree.Children {
		var elem document.PresentationElement
		switch node.XMLName.Local {
		case "sp":
			if isPlaceholderShape(node) {
				continue
			}
			elem, ok = projectInheritedShape(node, colorMap, theme)
		case "pic":
			elem, ok = p.projectInheritedPicture(partPath, node)
		default:
			continue
		}
		if !ok || !validPresentationRect(elem.Bounds) {
			continue
		}
		// Separate deterministic ID ranges keep static snapshot IDs away from
		// slide-local IDs while preserving stable ordering across snapshots.
		if elem.ID > 0 {
			elem.ID = -int64(partIndex+1)*1_000_000 - elem.ID
		}
		result = append(result, elem)
	}
	return result
}

func projectInheritedShape(shape pptxXMLNode, colorMap, theme map[string]string) (document.PresentationElement, bool) {
	cNvPr := nonVisualProperties(shape).child("cNvPr")
	if cNvPr == nil {
		return document.PresentationElement{}, false
	}
	bounds, hasXfrm := shapeBounds(shape)
	if !hasXfrm {
		return document.PresentationElement{}, false
	}
	elem := document.PresentationElement{
		ID:        parseShapeID(cNvPr.attr("id")),
		Name:      cNvPr.attr("name"),
		Kind:      document.ElementKindShape,
		Bounds:    bounds,
		ShapeType: "rect",
		WrapText:  true,
	}
	spPr := shape.child("spPr")
	if spPr != nil {
		if geom := spPr.child("prstGeom"); geom != nil && geom.attr("prst") != "" {
			elem.ShapeType = geom.attr("prst")
		} else if geom := spPr.child("custGeom"); geom != nil {
			if paths, ok := projectInheritedCustomPaths(geom); ok {
				elem.Paths = paths
			} else {
				return document.PresentationElement{}, false
			}
		}
		if fill := spPr.child("solidFill"); fill != nil {
			elem.FillColorHex = resolveBackgroundColor(*fill, colorMap, theme)
		}
		if fill := spPr.child("gradFill"); fill != nil {
			if gradient, ok := projectInheritedGradient(fill, colorMap, theme); ok {
				elem.FillGradient = gradient
			}
		}
		if line := spPr.child("ln"); line != nil {
			if width, err := strconv.ParseFloat(line.attr("w"), 64); err == nil && width > 0 {
				elem.BorderWidth = width / emuPerPoint
			}
			if fill := line.child("solidFill"); fill != nil {
				elem.BorderColor = resolveBackgroundColor(*fill, colorMap, theme)
			}
		}
	}
	if paragraphs := projectInheritedText(shape.child("txBody"), colorMap, theme); len(paragraphs) > 0 {
		elem.Kind = document.ElementKindText
		elem.Paragraphs = paragraphs
	}
	return elem, true
}

func projectInheritedCustomPaths(geometry *pptxXMLNode) ([]document.PresentationPath, bool) {
	pathList := geometry.child("pathLst")
	if pathList == nil {
		return nil, false
	}
	var paths []document.PresentationPath
	for _, node := range pathList.Children {
		if node.XMLName.Local != "path" {
			continue
		}
		width, widthErr := strconv.ParseFloat(node.attr("w"), 64)
		height, heightErr := strconv.ParseFloat(node.attr("h"), 64)
		if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
			return nil, false
		}
		path := document.PresentationPath{
			Width:  width,
			Height: height,
			Fill:   node.attr("fill"),
			Stroke: node.attr("stroke"),
		}
		hasSegment := false
		for _, commandNode := range node.Children {
			command := document.PresentationPathCommand{Kind: commandNode.XMLName.Local}
			switch command.Kind {
			case "moveTo", "lnTo", "cubicBezTo", "quadBezTo":
				for _, pointNode := range commandNode.Children {
					if pointNode.XMLName.Local != "pt" {
						continue
					}
					x, xErr := strconv.ParseFloat(pointNode.attr("x"), 64)
					y, yErr := strconv.ParseFloat(pointNode.attr("y"), 64)
					if xErr != nil || yErr != nil {
						return nil, false
					}
					command.Points = append(command.Points, document.PresentationPoint{X: x, Y: y})
				}
				wantPoints := 0
				switch command.Kind {
				case "moveTo", "lnTo":
					wantPoints = 1
				case "cubicBezTo":
					wantPoints = 3
				case "quadBezTo":
					wantPoints = 2
				}
				if len(command.Points) != wantPoints {
					return nil, false
				}
				if command.Kind != "moveTo" {
					hasSegment = true
				}
			case "close":
			default:
				return nil, false
			}
			path.Commands = append(path.Commands, command)
		}
		if !hasSegment {
			return nil, false
		}
		paths = append(paths, path)
	}
	return paths, len(paths) > 0
}

func projectInheritedGradient(fill *pptxXMLNode, colorMap, theme map[string]string) (*document.PresentationGradient, bool) {
	linear := fill.child("lin")
	stopList := fill.child("gsLst")
	if linear == nil || stopList == nil {
		return nil, false
	}
	angle := 0.0
	if rawAngle := linear.attr("ang"); rawAngle != "" {
		parsedAngle, err := strconv.ParseFloat(rawAngle, 64)
		if err != nil {
			return nil, false
		}
		angle = parsedAngle
	}
	gradient := &document.PresentationGradient{AngleDeg: angle / 60000}
	for _, stopNode := range stopList.Children {
		if stopNode.XMLName.Local != "gs" {
			continue
		}
		position, err := strconv.ParseFloat(stopNode.attr("pos"), 64)
		if err != nil {
			return nil, false
		}
		var colorNode *pptxXMLNode
		for childIndex := range stopNode.Children {
			candidate := &stopNode.Children[childIndex]
			switch candidate.XMLName.Local {
			case "srgbClr", "sysClr", "schemeClr":
				colorNode = candidate
			}
			if colorNode != nil {
				break
			}
		}
		if colorNode == nil {
			return nil, false
		}
		colorHex := resolveBackgroundColor(*colorNode, colorMap, theme)
		if colorHex == "" {
			return nil, false
		}
		alpha := 1.0
		if alphaNode := colorNode.child("alpha"); alphaNode != nil {
			alpha, err = strconv.ParseFloat(alphaNode.attr("val"), 64)
			if err != nil {
				return nil, false
			}
			alpha /= 100000
		}
		gradient.Stops = append(gradient.Stops, document.PresentationGradientStop{
			Position: position,
			ColorHex: colorHex,
			Alpha:    alpha,
		})
	}
	if len(gradient.Stops) < 2 {
		return nil, false
	}
	return gradient, true
}

func (p *pptxPackage) projectInheritedPicture(ownerPart string, picture pptxXMLNode) (document.PresentationElement, bool) {
	nv := picture.child("nvPicPr")
	if nv == nil {
		return document.PresentationElement{}, false
	}
	cNvPr := nv.child("cNvPr")
	if cNvPr == nil {
		return document.PresentationElement{}, false
	}
	bounds, hasXfrm := shapeBounds(picture)
	if !hasXfrm {
		return document.PresentationElement{}, false
	}
	blipFill := picture.child("blipFill")
	if blipFill == nil || blipFill.child("blip") == nil {
		return document.PresentationElement{}, false
	}
	target, ok := p.imagePartForRelationship(ownerPart, blipFill.child("blip").attr("embed"))
	if !ok {
		return document.PresentationElement{}, false
	}
	img := p.image(target)
	if img == nil {
		return document.PresentationElement{}, false
	}
	return document.PresentationElement{
		ID:      parseShapeID(cNvPr.attr("id")),
		Name:    cNvPr.attr("name"),
		Kind:    document.ElementKindPicture,
		Bounds:  bounds,
		Image:   img,
		AltText: cNvPr.attr("descr"),
	}, true
}

func projectInheritedText(textBody *pptxXMLNode, colorMap, theme map[string]string) []document.PresentationParagraph {
	if textBody == nil {
		return nil
	}
	var paragraphs []document.PresentationParagraph
	for _, paragraphNode := range textBody.Children {
		if paragraphNode.XMLName.Local != "p" {
			continue
		}
		paragraph := document.PresentationParagraph{}
		if props := paragraphNode.child("pPr"); props != nil {
			switch props.attr("algn") {
			case "ctr":
				paragraph.Align = "center"
			case "r":
				paragraph.Align = "right"
			case "just":
				paragraph.Align = "justify"
			default:
				paragraph.Align = "left"
			}
		}
		for _, child := range paragraphNode.Children {
			if child.XMLName.Local != "r" && child.XMLName.Local != "fld" {
				continue
			}
			text := child.child("t")
			if text == nil || text.Text == "" {
				continue
			}
			run := document.PresentationTextRun{Text: text.Text, SizePT: 18, ColorHex: "#1F2937"}
			if props := child.child("rPr"); props != nil {
				if fontSize, err := strconv.ParseFloat(props.attr("sz"), 64); err == nil && fontSize > 0 {
					run.SizePT = fontSize / 100
				}
				run.Bold = props.attr("b") == "1"
				run.Italic = props.attr("i") == "1"
				if latin := props.child("latin"); latin != nil {
					run.Font = latin.attr("typeface")
				}
				if fill := props.child("solidFill"); fill != nil {
					if value := resolveBackgroundColor(*fill, colorMap, theme); value != "" {
						run.ColorHex = value
					}
				}
			}
			paragraph.Runs = append(paragraph.Runs, run)
		}
		if len(paragraph.Runs) > 0 {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	return paragraphs
}

func (p *pptxPackage) xmlPart(partPath string) (pptxXMLNode, bool) {
	part := p.parts[strings.TrimPrefix(partPath, "/")]
	if part == nil {
		return pptxXMLNode{}, false
	}
	data, err := readOPCPart(part)
	if err != nil {
		return pptxXMLNode{}, false
	}
	var root pptxXMLNode
	if xml.Unmarshal(data, &root) != nil {
		return pptxXMLNode{}, false
	}
	return root, true
}

func shapeTree(root pptxXMLNode) *pptxXMLNode {
	cSld := root.child("cSld")
	if cSld == nil {
		return nil
	}
	return cSld.child("spTree")
}

func isPlaceholderShape(shape pptxXMLNode) bool {
	nv := shape.child("nvSpPr")
	if nv == nil {
		return false
	}
	nvPr := nv.child("nvPr")
	return nvPr != nil && nvPr.child("ph") != nil
}

func nonVisualProperties(shape pptxXMLNode) pptxXMLNode {
	if nv := shape.child("nvSpPr"); nv != nil {
		return *nv
	}
	return pptxXMLNode{}
}

func shapeBounds(shape pptxXMLNode) (document.PresentationRect, bool) {
	properties := shape.child("spPr")
	if properties == nil {
		return document.PresentationRect{}, false
	}
	xfrm := properties.child("xfrm")
	if xfrm == nil {
		return document.PresentationRect{}, false
	}
	off, ext := xfrm.child("off"), xfrm.child("ext")
	if off == nil || ext == nil {
		return document.PresentationRect{}, true
	}
	x, xErr := strconv.ParseFloat(off.attr("x"), 64)
	y, yErr := strconv.ParseFloat(off.attr("y"), 64)
	w, wErr := strconv.ParseFloat(ext.attr("cx"), 64)
	h, hErr := strconv.ParseFloat(ext.attr("cy"), 64)
	if xErr != nil || yErr != nil || wErr != nil || hErr != nil {
		return document.PresentationRect{}, true
	}
	return document.PresentationRect{X: x / emuPerPoint, Y: y / emuPerPoint, W: w / emuPerPoint, H: h / emuPerPoint}, true
}

func validPresentationRect(bounds document.PresentationRect) bool {
	return bounds.W > 0 && bounds.H > 0 && bounds.X > -1_000_000 && bounds.Y > -1_000_000 && bounds.W < 1_000_000 && bounds.H < 1_000_000
}

func parseShapeID(value string) int64 {
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
}

func xmlBoolDefaultTrue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "off":
		return false
	default:
		return true
	}
}
