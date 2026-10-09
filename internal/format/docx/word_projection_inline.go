package docx

import (
	"fmt"
	"glance/internal/document"
	"strconv"
	"strings"
)

func (p *wordProjector) appendRunInlines(paragraph *document.WordParagraph, paragraphNode, run *wordXMLNode, inlineIndex *int) {
	runProperties := firstChild(run, "rPr")
	style := parseWordTextStyle(runProperties)
	styleSet := parseWordTextStyleSet(runProperties)
	characterStyle := attrValue(firstChild(runProperties, "rStyle"), "val")
	for _, child := range run.children {
		id := fmt.Sprintf("%s.i%d", paragraph.ID, *inlineIndex)
		switch child.name.Local {
		case "rPr", "lastRenderedPageBreak":
			continue
		case "t":
			text := &document.WordTextRun{ID: id, Text: child.text, Style: style, StyleSet: styleSet, CharacterStyle: characterStyle, Editable: true}
			p.textTargets[id] = wordTextTarget{text: child, run: run, paragraph: paragraphNode, paragraphID: paragraph.ID}
			p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordTextInline, Text: text}, child, run)
			*inlineIndex++
		case "drawing":
			inline, projectedID := p.drawingInline(child, id, paragraph.ID)
			if inline.Unsupported != nil {
				p.addParagraphFallback(paragraph, projectedID, child, "[Drawing preview unavailable]", inline.Unsupported.Description)
			} else {
				p.appendProjectedInline(paragraph, projectedID, inline, child, run)
			}
			*inlineIndex++
		case "br":
			brType := attrValue(child, "type")
			if child.name.Space == wordprocessingMLNamespace && (strings.EqualFold(brType, "page") || strings.EqualFold(brType, "column")) {
				bt := document.WordBreakPage
				if strings.EqualFold(brType, "column") {
					bt = document.WordBreakColumn
				}
				pageBreak := &document.WordPageBreak{ID: id, Type: bt}
				p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordPageBreakInline, PageBreak: pageBreak}, child, run)
			} else if child.name.Space == wordprocessingMLNamespace && (brType == "" || strings.EqualFold(brType, "text") || strings.EqualFold(brType, "textWrapping")) {
				lineBreak := &document.WordTextRun{ID: id, Text: "\n", Style: style, StyleSet: styleSet, CharacterStyle: characterStyle}
				p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordTextInline, Text: lineBreak}, child, run)
			} else {
				p.appendProjectedInline(paragraph, id, unsupportedInline(id, "Unsupported run content: "+child.name.Local), child, run)
			}
			*inlineIndex++
		case "cr":
			lineBreak := &document.WordTextRun{ID: id, Text: "\n", Style: style, StyleSet: styleSet, CharacterStyle: characterStyle}
			p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordTextInline, Text: lineBreak}, child, run)
			*inlineIndex++
		case "softHyphen":
			// Soft hyphens are invisible until a line ends at that opportunity.
			// The preview shaper has no discretionary-glyph atom, so preserve the
			// source as an empty, non-editable projection without blocking the page.
			softHyphen := &document.WordTextRun{ID: id, Text: "", Style: style, StyleSet: styleSet, CharacterStyle: characterStyle}
			p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordTextInline, Text: softHyphen}, child, run)
			*inlineIndex++
		case "tab":
			tab := &document.WordTab{ID: id}
			p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordTabInline, Tab: tab}, child, run)
			*inlineIndex++
		case "fldChar", "instrText":
			value := child.text
			if child.name.Local == "fldChar" {
				value = attrValue(child, "fldCharType")
			}
			fieldCode := &document.WordFieldCode{ID: id, Element: child.name.Local, Value: value}
			p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordFieldCodeInline, FieldCode: fieldCode}, child, run)
			*inlineIndex++
		case "AlternateContent":
			p.appendAlternateContent(paragraph, child, id)
			*inlineIndex++
		case "footnoteReference", "endnoteReference":
			kind := "footnotes"
			if child.name.Local == "endnoteReference" {
				kind = "endnotes"
			}
			if !p.appendNoteReference(paragraph, id, kind, attrValue(child, "id")) {
				p.appendProjectedInline(paragraph, id, unsupportedInline(id, "Unsupported run content: "+child.name.Local), child, run)
			}
			*inlineIndex++
		case "pict":
			inline := p.vmlImageInline(child, id)
			if inline.Unsupported != nil {
				p.addParagraphFallback(paragraph, id, child, "[VML shape preview unavailable]", inline.Unsupported.Description)
			} else {
				p.appendProjectedInline(paragraph, id, inline, child, run)
			}
			*inlineIndex++
		case "object":
			inline := p.vmlImageInline(child, id)
			if inline.Image != nil {
				inline.Image.InlineFallback = false
				p.addParagraphWarning(paragraph, "Embedded OLE content is represented by its static preview image.")
				p.appendProjectedInline(paragraph, id, inline, child, run)
			} else {
				detail := "Embedded object has no usable static preview."
				if inline.Unsupported != nil {
					detail = inline.Unsupported.Description
				}
				p.addParagraphFallback(paragraph, id, child, "[Embedded object preview unavailable]", detail)
			}
			*inlineIndex++
		case "sym":
			p.appendProjectedInline(paragraph, id, unsupportedInline(id, "Unsupported run content: "+child.name.Local), child, run)
			*inlineIndex++
		default:
			p.appendProjectedInline(paragraph, id, unsupportedInline(id, "Unsupported run content: "+child.name.Local), child, run)
			*inlineIndex++
		}
	}
}

func (p *wordProjector) drawingInline(drawing *wordXMLNode, fallbackID, paragraphID string) (document.WordInline, string) {
	id := p.wordImageID(drawing, fallbackID)
	floating := containsNode(drawing, "anchor")
	container := findDescendant(drawing, "inline")
	if container == nil && floating {
		container = findDescendant(drawing, "anchor")
	}
	if container == nil {
		return unsupportedInline(id, "Drawing has no supported inline object"), id
	}
	extent := firstChild(container, "extent")
	var width, height int64
	if extent != nil {
		width, _ = strconv.ParseInt(attrValue(extent, "cx"), 10, 64)
		height, _ = strconv.ParseInt(attrValue(extent, "cy"), 10, 64)
	}
	blip := findDescendant(container, "blip")
	if blip == nil {
		return unsupportedInline(id, "Inline drawing has no image reference"), id
	}
	relID := attrValue(blip, "embed")
	if relID == "" {
		return unsupportedInline(id, "Linked or missing image relationship is read-only"), id
	}
	relationship, ok := p.relationships[relID]
	if !ok || relationship.external {
		return unsupportedInline(id, "Image relationship "+relID+" is missing or external"), id
	}
	if !relationship.image {
		return unsupportedInline(id, "Relationship "+relID+" is not an image"), id
	}
	partIndex, exists := p.partIndexes[relationship.target]
	if !exists {
		missing := unsupportedInline(id, "Image media part is missing: "+relationship.target)
		missing.Unsupported.ImagePreview = &document.WordImage{ID: id, WidthEMU: width, HeightEMU: height}
		return missing, id
	}
	mediaType := p.contentTypes.mediaType(relationship.target)
	if !supportedWordImageMedia(mediaType) {
		return unsupportedInline(id, "Unsupported image media type: "+mediaType), id
	}
	transform := findDescendant(container, "xfrm")
	rotation := 0.0
	if transform != nil {
		if rawRotation := attrValue(transform, "rot"); rawRotation != "" {
			if value, err := strconv.ParseInt(rawRotation, 10, 64); err == nil {
				rotation = normalizeWordImageRotation(float64(value) / 60000)
			}
		}
	}
	image := &document.WordImage{ID: id, MediaType: mediaType, WidthEMU: width, HeightEMU: height, RotationDegrees: rotation, Resolved: true, Editable: !floating, InlineFallback: floating}
	p.imageParts[id] = partIndex
	if !floating {
		p.imageTargets[id] = wordImageTarget{drawing: drawing, extent: extent, blip: blip, transform: transform, paragraphID: paragraphID, relationshipID: relID}
	}
	return document.WordInline{Kind: document.WordImageInline, Image: image}, id
}

func (p *wordProjector) appendProjectedInline(paragraph *document.WordParagraph, id string, inline document.WordInline, node, run *wordXMLNode) {
	paragraph.Inlines = append(paragraph.Inlines, inline)
	p.paragraphInlines[paragraph.ID] = append(p.paragraphInlines[paragraph.ID], id)
	if node != nil {
		p.inlineTargets[id] = wordInlineTarget{node: node, run: run, paragraphID: paragraph.ID}
	}
}

func (p *wordProjector) wordImageID(drawing *wordXMLNode, fallback string) string {
	docPr := findDescendant(drawing, "docPr")
	value := ""
	if docPr != nil {
		value = attrValue(docPr, "id")
	}
	if value == "" {
		return fallback
	}
	id := "img-" + value
	if p.seenImageIDs[id] {
		return fallback
	}
	p.seenImageIDs[id] = true
	return id
}

func unsupportedInline(id, description string) document.WordInline {
	return document.WordInline{Kind: document.WordUnsupportedInline, Unsupported: &document.WordUnsupported{ID: id, Description: description}}
}

func supportedWordImageMedia(mediaType string) bool {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/png", "image/jpeg", "image/gif":
		return true
	default:
		return false
	}
}
