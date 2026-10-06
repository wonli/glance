package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"glance/internal/document"
)

type wordXMLNode struct {
	name         xml.Name
	rawName      string
	attrs        []xml.Attr
	text         string
	children     []*wordXMLNode
	parent       *wordXMLNode
	start        int
	openEnd      int
	contentStart int
	contentEnd   int
	end          int
	selfClosing  bool
}

type wordRelationship struct {
	target    string
	image     bool
	numbering bool
	styles    bool
	external  bool
}

type wordNumberingLevel struct {
	kind  document.WordListKind
	text  string
	start int
}

type wordNumberingDefinition struct {
	levels map[int]wordNumberingLevel
}

type wordProjection struct {
	blocks           []document.WordBlock
	layout           document.WordLayoutData
	imageParts       map[string]int
	imageTargets     map[string]wordImageTarget
	inlineTargets    map[string]wordInlineTarget
	paragraphInlines map[string][]string
	textTargets      map[string]wordTextTarget
	paragraphTargets map[string]*wordXMLNode
}

type wordTextTarget struct {
	text        *wordXMLNode
	run         *wordXMLNode
	paragraph   *wordXMLNode
	paragraphID string
}

type wordInlineTarget struct {
	node        *wordXMLNode
	run         *wordXMLNode
	paragraphID string
}

type wordImageTarget struct {
	drawing        *wordXMLNode
	extent         *wordXMLNode
	blip           *wordXMLNode
	transform      *wordXMLNode
	paragraphID    string
	relationshipID string
}

func (d *Document) WordBlocks() []document.WordBlock {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	return cloneWordBlocks(d.wordBlocks)
}

func (d *Document) WordImageBytes(imageID string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	partIndex, ok := d.wordImages[imageID]
	if !ok {
		return nil, fmt.Errorf("image %q is not available in this DOCX document", imageID)
	}
	if partIndex < 0 || partIndex >= len(d.parts) {
		return nil, fmt.Errorf("image %q references a missing DOCX media part", imageID)
	}
	return append([]byte(nil), d.parts[partIndex].data...), nil
}

func parseWordXML(data []byte) (*wordXMLNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root *wordXMLNode
	var stack []*wordXMLNode
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			openEnd := int(decoder.InputOffset())
			start := bytes.LastIndex(data[:openEnd], []byte("<"))
			if start < 0 {
				return nil, errors.New("cannot locate DOCX XML start tag")
			}
			rawOpen := data[start:openEnd]
			rawName := rawElementName(rawOpen)
			if rawName == "" {
				return nil, errors.New("cannot read DOCX XML element name")
			}
			trimmedOpen := bytes.TrimSpace(rawOpen)
			selfClosing := len(trimmedOpen) >= 2 && trimmedOpen[len(trimmedOpen)-2] == '/'
			node := &wordXMLNode{name: value.Name, rawName: rawName, attrs: append([]xml.Attr(nil), value.Attr...), start: start, openEnd: openEnd, contentStart: openEnd, contentEnd: openEnd, end: openEnd, selfClosing: selfClosing}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("DOCX XML contains multiple root elements")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				node.parent = parent
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) != 0 {
				stack[len(stack)-1].text += string(value)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("unexpected DOCX XML closing element")
			}
			node := stack[len(stack)-1]
			end := int(decoder.InputOffset())
			if node.selfClosing && end == node.openEnd {
				node.contentEnd, node.end = node.openEnd, node.openEnd
			} else {
				endStart := bytes.LastIndex(data[:end], []byte("</"))
				if endStart < node.openEnd {
					return nil, errors.New("cannot locate DOCX XML closing tag")
				}
				node.contentEnd, node.end = endStart, end
			}
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil {
		return nil, errors.New("DOCX XML is empty")
	}
	if len(stack) != 0 {
		return nil, errors.New("DOCX XML has unclosed elements")
	}
	return root, nil
}

func projectWordDocument(parts []part, documentIdx int) (wordProjection, error) {
	if documentIdx < 0 || documentIdx >= len(parts) {
		return wordProjection{}, errors.New("DOCX package is missing word/document.xml")
	}
	partIndexes := make(map[string]int, len(parts))
	for i := range parts {
		partIndexes[parts[i].header.Name] = i
	}
	relationships := readDocumentRelationships(parts, partIndexes)
	contentTypes := readContentTypes(parts, partIndexes)
	numbering := readWordNumbering(parts, partIndexes, relationships)
	documentRoot, err := parseWordXML(parts[documentIdx].data)
	if err != nil {
		return wordProjection{}, fmt.Errorf("parse word/document.xml: %w", err)
	}
	body := firstChild(documentRoot, "body")
	if body == nil {
		return wordProjection{}, errors.New("DOCX document has no body")
	}
	projector := wordProjector{
		parts: parts, partIndexes: partIndexes, relationships: relationships, contentTypes: contentTypes,
		numbering:  numbering,
		imageParts: make(map[string]int), imageTargets: make(map[string]wordImageTarget), inlineTargets: make(map[string]wordInlineTarget),
		paragraphInlines: make(map[string][]string), textTargets: make(map[string]wordTextTarget), paragraphTargets: make(map[string]*wordXMLNode), seenImageIDs: make(map[string]bool),
	}
	var blocks []document.WordBlock
	for i, child := range body.children {
		blockID := fmt.Sprintf("b%d", i)
		switch child.name.Local {
		case "p":
			paragraph := projector.paragraph(child, blockID+".p")
			blocks = append(blocks, document.WordBlock{ID: blockID, Kind: document.WordParagraphBlock, Paragraph: &paragraph})
		case "tbl":
			table := projector.table(child, blockID)
			blocks = append(blocks, document.WordBlock{ID: blockID, Kind: document.WordTableBlock, Table: &table})
		case "sectPr":
			// Section properties affect layout metadata, not body content.
		case "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd", "permStart", "permEnd":
			// Range markers are zero-width annotations. Word permits them
			// between body blocks; they do not represent unsupported content.
			continue
		default:
			unsupported := &document.WordUnsupported{ID: blockID + ".unsupported", Description: "Unsupported body content: " + child.name.Local}
			blocks = append(blocks, document.WordBlock{ID: blockID, Kind: document.WordUnsupportedBlock, Unsupported: unsupported})
		}
	}
	layout := projectWordLayoutData(parts, partIndexes, relationships, documentRoot)
	return wordProjection{blocks: blocks, layout: layout, imageParts: projector.imageParts, imageTargets: projector.imageTargets, inlineTargets: projector.inlineTargets, paragraphInlines: projector.paragraphInlines, textTargets: projector.textTargets, paragraphTargets: projector.paragraphTargets}, nil
}

type wordProjector struct {
	parts            []part
	partIndexes      map[string]int
	relationships    map[string]wordRelationship
	contentTypes     wordContentTypes
	numbering        map[string]wordNumberingDefinition
	imageParts       map[string]int
	imageTargets     map[string]wordImageTarget
	inlineTargets    map[string]wordInlineTarget
	paragraphInlines map[string][]string
	textTargets      map[string]wordTextTarget
	paragraphTargets map[string]*wordXMLNode
	seenImageIDs     map[string]bool
}

func (p *wordProjector) paragraph(node *wordXMLNode, id string) document.WordParagraph {
	paragraph := document.WordParagraph{ID: id, Alignment: document.WordAlignDefault}
	paragraph.ListKind, paragraph.ListLevel, paragraph.ListMarker = p.paragraphList(node)
	paragraph.ListMaxLevel = p.paragraphListMaxLevel(node, paragraph.ListKind)
	p.paragraphTargets[id] = node
	if properties := firstChild(node, "pPr"); properties != nil {
		paragraph.Layout = parseWordParagraphLayout(properties)
		if style := firstChild(properties, "pStyle"); style != nil {
			paragraph.Style = attrValue(style, "val")
		}
		if alignment := firstChild(properties, "jc"); alignment != nil {
			paragraph.Alignment = parseWordAlignment(attrValue(alignment, "val"))
		}
		if indent := firstChild(properties, "ind"); indent != nil {
			left := attrValue(indent, "left")
			if left == "" {
				left = attrValue(indent, "start")
			}
			right := attrValue(indent, "right")
			if right == "" {
				right = attrValue(indent, "end")
			}
			paragraph.LeftIndentEMU = parseWordIndentEMU(left)
			paragraph.RightIndentEMU = parseWordIndentEMU(right)
		}
	}
	inlineIndex := 0
	p.paragraphInlines[id] = nil
	for _, child := range node.children {
		switch child.name.Local {
		case "pPr", "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd", "permStart", "permEnd":
			continue
		case "r":
			p.appendRunInlines(&paragraph, node, child, &inlineIndex)
		case "hyperlink":
			for _, linked := range child.children {
				if linked.name.Local == "r" {
					p.appendRunInlines(&paragraph, node, linked, &inlineIndex)
				} else {
					inlineID := fmt.Sprintf("%s.i%d", id, inlineIndex)
					p.appendProjectedInline(&paragraph, inlineID, unsupportedInline(inlineID, "Unsupported hyperlink content: "+linked.name.Local), linked, nil)
					inlineIndex++
				}
			}
		default:
			inlineID := fmt.Sprintf("%s.i%d", id, inlineIndex)
			p.appendProjectedInline(&paragraph, inlineID, unsupportedInline(inlineID, "Unsupported paragraph content: "+child.name.Local), child, nil)
			inlineIndex++
		}
	}
	fieldContent := false
	for _, inline := range paragraph.Inlines {
		if inline.Unsupported == nil {
			continue
		}
		if strings.Contains(inline.Unsupported.Description, "fldChar") || strings.Contains(inline.Unsupported.Description, "instrText") {
			fieldContent = true
			break
		}
	}
	if fieldContent {
		for index := range paragraph.Inlines {
			text := paragraph.Inlines[index].Text
			if text == nil {
				continue
			}
			text.Editable = false
			delete(p.textTargets, text.ID)
			delete(p.inlineTargets, text.ID)
		}
	}
	return paragraph
}

func (p *wordProjector) paragraphList(node *wordXMLNode) (document.WordListKind, int, string) {
	properties := firstChild(node, "pPr")
	numPr := firstChild(properties, "numPr")
	if numPr == nil {
		return document.WordListNone, 0, ""
	}
	numID := attrValue(firstChild(numPr, "numId"), "val")
	if numID == "" {
		return document.WordListUnsupported, 0, ""
	}
	level := 0
	if value := attrValue(firstChild(numPr, "ilvl"), "val"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 || parsed > 8 {
			return document.WordListUnsupported, 0, ""
		}
		level = parsed
	}
	definition, ok := p.numbering[numID]
	if !ok {
		return document.WordListUnsupported, level, ""
	}
	numberingLevel, ok := definition.levels[level]
	if !ok || numberingLevel.kind == document.WordListNone {
		return document.WordListUnsupported, level, ""
	}
	for parentLevel := 0; parentLevel < level; parentLevel++ {
		parentDefinition, exists := definition.levels[parentLevel]
		if !exists || parentDefinition.kind != numberingLevel.kind {
			return document.WordListUnsupported, level, ""
		}
	}
	if numberingLevel.kind == document.WordListBullet {
		marker := strings.TrimSpace(numberingLevel.text)
		if marker == "" || strings.Contains(marker, "%") {
			marker = wordListBulletMarker(level)
		}
		return numberingLevel.kind, level, marker
	}
	if numberingLevel.kind != document.WordListNumbered {
		return document.WordListUnsupported, level, ""
	}
	marker := numberingLevel.text
	for markerLevel := 0; markerLevel <= level; markerLevel++ {
		levelDefinition, exists := definition.levels[markerLevel]
		if !exists {
			continue
		}
		value := wordNumberingValue(node, numID, markerLevel, level, levelDefinition.start)
		marker = strings.ReplaceAll(marker, "%"+strconv.Itoa(markerLevel+1), strconv.Itoa(value))
	}
	if marker == "" {
		marker = strconv.Itoa(wordNumberingValue(node, numID, level, level, numberingLevel.start)) + "."
	}
	return numberingLevel.kind, level, marker
}

func (p *wordProjector) paragraphListMaxLevel(node *wordXMLNode, kind document.WordListKind) int {
	if kind != document.WordListBullet && kind != document.WordListNumbered {
		return 0
	}
	numPr := firstChild(firstChild(node, "pPr"), "numPr")
	numID := attrValue(firstChild(numPr, "numId"), "val")
	numberingDefinition, ok := p.numbering[numID]
	if !ok {
		return 0
	}
	maximum := -1
	for level := 0; level <= 8; level++ {
		levelDefinition, exists := numberingDefinition.levels[level]
		if !exists || levelDefinition.kind != kind {
			break
		}
		maximum = level
	}
	return max(0, maximum)
}

func wordListBulletMarker(level int) string {
	markers := [...]string{"•", "◦", "▪"}
	if level < 0 {
		level = 0
	}
	return markers[level%len(markers)]
}

func wordNumberingValue(node *wordXMLNode, numID string, level, currentLevel, start int) int {
	value := start
	if node == nil || node.parent == nil {
		return value
	}
	for _, sibling := range node.parent.children {
		if sibling == node {
			break
		}
		if sibling.name.Local != "p" {
			continue
		}
		siblingNumID, siblingLevel := wordParagraphNumID(sibling)
		if siblingNumID != numID {
			continue
		}
		if siblingLevel == level {
			value++
		} else if siblingLevel >= 0 && siblingLevel < level {
			value = start
		}
	}
	if currentLevel > level && value > start {
		value--
	}
	return value
}

func wordParagraphNumID(paragraph *wordXMLNode) (string, int) {
	numPr := firstChild(firstChild(paragraph, "pPr"), "numPr")
	if numPr == nil {
		return "", -1
	}
	numID := attrValue(firstChild(numPr, "numId"), "val")
	level := 0
	if value := attrValue(firstChild(numPr, "ilvl"), "val"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return numID, -1
		}
		level = parsed
	}
	return numID, level
}

func parseWordIndentEMU(value string) int64 {
	if value == "" {
		return 0
	}
	twips, err := strconv.ParseInt(value, 10, 64)
	const maxTwips = int64(1<<63-1) / 635
	const minTwips = -maxTwips - 1
	if err != nil || twips > maxTwips || twips < minTwips {
		return 0
	}
	return twips * 635
}

func (p *wordProjector) appendRunInlines(paragraph *document.WordParagraph, paragraphNode, run *wordXMLNode, inlineIndex *int) {
	runProperties := firstChild(run, "rPr")
	style := parseWordTextStyle(runProperties)
	styleSet := parseWordTextStyleSet(runProperties)
	characterStyle := attrValue(firstChild(runProperties, "rStyle"), "val")
	for _, child := range run.children {
		id := fmt.Sprintf("%s.i%d", paragraph.ID, *inlineIndex)
		switch child.name.Local {
		case "rPr":
			continue
		case "t":
			text := &document.WordTextRun{ID: id, Text: child.text, Style: style, StyleSet: styleSet, CharacterStyle: characterStyle, Editable: true}
			p.textTargets[id] = wordTextTarget{text: child, run: run, paragraph: paragraphNode, paragraphID: paragraph.ID}
			p.appendProjectedInline(paragraph, id, document.WordInline{Kind: document.WordTextInline, Text: text}, child, run)
			*inlineIndex++
		case "drawing":
			inline, projectedID := p.drawingInline(child, id, paragraph.ID)
			p.appendProjectedInline(paragraph, projectedID, inline, child, run)
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
			} else {
				p.appendProjectedInline(paragraph, id, unsupportedInline(id, "Unsupported run content: "+child.name.Local), child, run)
			}
			*inlineIndex++
		case "tab", "cr", "pict", "object", "fldChar", "instrText", "sym", "footnoteReference", "endnoteReference":
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
	if containsNode(drawing, "anchor") {
		return unsupportedInline(id, "Floating drawing (wp:anchor) is read-only"), id
	}
	inline := findDescendant(drawing, "inline")
	if inline == nil {
		return unsupportedInline(id, "Drawing has no supported inline object"), id
	}
	extent := firstChild(inline, "extent")
	var width, height int64
	if extent != nil {
		width, _ = strconv.ParseInt(attrValue(extent, "cx"), 10, 64)
		height, _ = strconv.ParseInt(attrValue(extent, "cy"), 10, 64)
	}
	blip := findDescendant(inline, "blip")
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
	transform := findDescendant(drawing, "xfrm")
	rotation := 0.0
	if transform != nil {
		if rawRotation := attrValue(transform, "rot"); rawRotation != "" {
			if value, err := strconv.ParseInt(rawRotation, 10, 64); err == nil {
				rotation = normalizeWordImageRotation(float64(value) / 60000)
			}
		}
	}
	image := &document.WordImage{ID: id, MediaType: mediaType, WidthEMU: width, HeightEMU: height, RotationDegrees: rotation, Resolved: true, Editable: true}
	p.imageParts[id] = partIndex
	p.imageTargets[id] = wordImageTarget{drawing: drawing, extent: extent, blip: blip, transform: transform, paragraphID: paragraphID, relationshipID: relID}
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

func (p *wordProjector) table(node *wordXMLNode, id string) document.WordTable {
	table := document.WordTable{}
	table.Layout.FillColor = wordShadingFill(firstChild(node, "tblPr"))
	rowIndex := 0
	for _, child := range node.children {
		if child.name.Local == "tblGrid" {
			for _, gridColumn := range child.children {
				if gridColumn.name.Local == "gridCol" {
					table.Layout.GridColumnsEMU = append(table.Layout.GridColumnsEMU, parseWordIndentEMU(attrValue(gridColumn, "w")))
				}
			}
			continue
		}
		if child.name.Local != "tr" {
			continue
		}
		row := document.WordTableRow{}
		cellIndex := 0
		for _, cellNode := range child.children {
			if cellNode.name.Local != "tc" {
				continue
			}
			cellID := fmt.Sprintf("%s.r%d.c%d", id, rowIndex, cellIndex)
			cellProperties := firstChild(cellNode, "tcPr")
			cell := document.WordTableCell{
				ID:        cellID,
				WidthEMU:  wordCellWidthEMU(firstChild(cellProperties, "tcW")),
				FillColor: wordShadingFill(cellProperties),
			}
			if cell.FillColor == "" {
				cell.FillColor = table.Layout.FillColor
			}
			paragraphIndex := 0
			for _, content := range cellNode.children {
				switch content.name.Local {
				case "tcPr":
					continue
				case "p":
					cell.Paragraphs = append(cell.Paragraphs, p.paragraph(content, fmt.Sprintf("%s.p%d", cellID, paragraphIndex)))
					paragraphIndex++
				case "tbl":
					paragraph := document.WordParagraph{ID: fmt.Sprintf("%s.p%d", cellID, paragraphIndex), Alignment: document.WordAlignDefault}
					paragraph.Inlines = append(paragraph.Inlines, unsupportedInline(paragraph.ID+".i0", "Nested table is retained but not projected"))
					cell.Paragraphs = append(cell.Paragraphs, paragraph)
					paragraphIndex++
				default:
					paragraph := document.WordParagraph{ID: fmt.Sprintf("%s.p%d", cellID, paragraphIndex), Alignment: document.WordAlignDefault}
					paragraph.Inlines = append(paragraph.Inlines, unsupportedInline(paragraph.ID+".i0", "Unsupported table-cell content: "+content.name.Local))
					cell.Paragraphs = append(cell.Paragraphs, paragraph)
					paragraphIndex++
				}
			}
			row.Cells = append(row.Cells, cell)
			cellIndex++
		}
		table.Rows = append(table.Rows, row)
		rowIndex++
	}
	return table
}

func wordShadingFill(properties *wordXMLNode) string {
	shading := firstChild(properties, "shd")
	if shading == nil {
		return ""
	}
	if value := strings.ToLower(attrValue(shading, "val")); value == "nil" || value == "none" {
		return ""
	}
	fill := strings.TrimSpace(attrValue(shading, "fill"))
	if (fill == "" || strings.EqualFold(fill, "auto")) && strings.EqualFold(attrValue(shading, "val"), "solid") {
		fill = strings.TrimSpace(attrValue(shading, "color"))
	}
	if len(fill) != 6 {
		return ""
	}
	for _, digit := range fill {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f') || (digit >= 'A' && digit <= 'F')) {
			return ""
		}
	}
	return strings.ToUpper(fill)
}

func parseWordTextStyle(properties *wordXMLNode) document.WordTextStyle {
	var style document.WordTextStyle
	if properties == nil {
		return style
	}
	style.Bold = toggleProperty(firstChild(properties, "b"))
	style.Italic = toggleProperty(firstChild(properties, "i"))
	underline := firstChild(properties, "u")
	style.Underline = underline != nil && !strings.EqualFold(attrValue(underline, "val"), "none")
	if color := firstChild(properties, "color"); color != nil {
		style.FontColor = strings.ToUpper(attrValue(color, "val"))
		if style.FontColor == "AUTO" {
			style.FontColor = ""
		}
	}
	if highlight := firstChild(properties, "highlight"); highlight != nil {
		style.Highlight = strings.TrimSpace(attrValue(highlight, "val"))
		if strings.EqualFold(style.Highlight, "none") {
			style.Highlight = ""
		}
	}
	if fonts := firstChild(properties, "rFonts"); fonts != nil {
		for _, key := range []string{"eastAsia", "ascii", "hAnsi", "cs"} {
			if value := attrValue(fonts, key); value != "" {
				style.FontFamily = value
				break
			}
		}
	}
	if size := firstChild(properties, "sz"); size != nil {
		if halfPoints, err := strconv.ParseFloat(attrValue(size, "val"), 64); err == nil {
			style.FontSize = halfPoints / 2
		}
	}
	return style
}

func toggleProperty(node *wordXMLNode) bool {
	if node == nil {
		return false
	}
	value := strings.ToLower(attrValue(node, "val"))
	switch value {
	case "0", "false", "off", "none":
		return false
	default:
		return true
	}
}

func parseWordAlignment(value string) document.WordAlignment {
	switch strings.ToLower(value) {
	case "left", "start":
		return document.WordAlignLeft
	case "center":
		return document.WordAlignCenter
	case "right", "end":
		return document.WordAlignRight
	case "both", "justify":
		return document.WordAlignJustified
	default:
		return document.WordAlignDefault
	}
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

func readDocumentRelationships(parts []part, partIndexes map[string]int) map[string]wordRelationship {
	result := make(map[string]wordRelationship)
	index, ok := partIndexes["word/_rels/document.xml.rels"]
	if !ok {
		return result
	}
	root, err := parseWordXML(parts[index].data)
	if err != nil {
		return result
	}
	for _, node := range root.children {
		if node.name.Local != "Relationship" {
			continue
		}
		id := attrValue(node, "Id")
		target := attrValue(node, "Target")
		relationType := strings.ToLower(attrValue(node, "Type"))
		if id == "" {
			continue
		}
		external := strings.EqualFold(attrValue(node, "TargetMode"), "External")
		result[id] = wordRelationship{
			target:    resolveDocumentTarget(target),
			image:     strings.HasSuffix(relationType, "/image"),
			numbering: strings.HasSuffix(relationType, "/numbering"),
			styles:    strings.HasSuffix(relationType, "/styles"),
			external:  external,
		}
	}
	return result
}

func readWordNumbering(parts []part, partIndexes map[string]int, relationships map[string]wordRelationship) map[string]wordNumberingDefinition {
	result := make(map[string]wordNumberingDefinition)
	partName := ""
	for _, relationship := range relationships {
		if relationship.numbering && !relationship.external {
			partName = relationship.target
			break
		}
	}
	if partName == "" {
		return result
	}
	index, ok := partIndexes[partName]
	if !ok {
		return result
	}
	root, err := parseWordXML(parts[index].data)
	if err != nil {
		return result
	}
	abstracts := make(map[string]map[int]wordNumberingLevel)
	for _, node := range root.children {
		if node.name.Local != "abstractNum" {
			continue
		}
		abstractID := attrValue(node, "abstractNumId")
		levels := make(map[int]wordNumberingLevel)
		for _, levelNode := range node.children {
			if levelNode.name.Local != "lvl" {
				continue
			}
			level, parseErr := strconv.Atoi(attrValue(levelNode, "ilvl"))
			if parseErr != nil || level < 0 || level > 8 {
				continue
			}
			numFmt := strings.ToLower(attrValue(firstChild(levelNode, "numFmt"), "val"))
			lvlText := attrValue(firstChild(levelNode, "lvlText"), "val")
			start := 1
			if value := attrValue(firstChild(levelNode, "start"), "val"); value != "" {
				if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed > 0 {
					start = parsed
				}
			}
			kind := document.WordListUnsupported
			switch numFmt {
			case "bullet":
				kind = document.WordListBullet
			case "decimal":
				kind = document.WordListNumbered
			}
			for _, property := range levelNode.children {
				if property.name.Local == "lvlRestart" || property.name.Local == "lvlPicBulletId" {
					kind = document.WordListUnsupported
					break
				}
			}
			levels[level] = wordNumberingLevel{kind: kind, text: lvlText, start: start}
		}
		abstracts[abstractID] = levels
	}
	for _, node := range root.children {
		if node.name.Local != "num" {
			continue
		}
		numID := attrValue(node, "numId")
		abstractID := attrValue(firstChild(node, "abstractNumId"), "val")
		if numID == "" || abstractID == "" {
			continue
		}
		levels, ok := abstracts[abstractID]
		if !ok {
			continue
		}
		hasOverrides := false
		for _, child := range node.children {
			if child.name.Local == "lvlOverride" || child.name.Local == "startOverride" {
				hasOverrides = true
				break
			}
		}
		if hasOverrides {
			levels = make(map[int]wordNumberingLevel, len(levels))
			for level, definition := range abstracts[abstractID] {
				definition.kind = document.WordListUnsupported
				levels[level] = definition
			}
		}
		result[numID] = wordNumberingDefinition{levels: levels}
	}
	return result
}

func resolveDocumentTarget(target string) string {
	if target == "" {
		return ""
	}
	clean := strings.TrimPrefix(target, "/")
	if !strings.HasPrefix(target, "/") {
		clean = path.Join("word", target)
	}
	clean = path.Clean(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

type wordContentTypes struct {
	defaults  map[string]string
	overrides map[string]string
}

func readContentTypes(parts []part, partIndexes map[string]int) wordContentTypes {
	result := wordContentTypes{defaults: make(map[string]string), overrides: make(map[string]string)}
	index, ok := partIndexes["[Content_Types].xml"]
	if !ok {
		return result
	}
	root, err := parseWordXML(parts[index].data)
	if err != nil {
		return result
	}
	for _, node := range root.children {
		switch node.name.Local {
		case "Default":
			extension := strings.ToLower(strings.TrimPrefix(attrValue(node, "Extension"), "."))
			if extension != "" {
				result.defaults[extension] = attrValue(node, "ContentType")
			}
		case "Override":
			partName := strings.TrimPrefix(attrValue(node, "PartName"), "/")
			if partName != "" {
				result.overrides[path.Clean(partName)] = attrValue(node, "ContentType")
			}
		}
	}
	return result
}

func (types wordContentTypes) mediaType(partName string) string {
	if mediaType := types.overrides[path.Clean(partName)]; mediaType != "" {
		return mediaType
	}
	return types.defaults[strings.TrimPrefix(strings.ToLower(path.Ext(partName)), ".")]
}

func firstChild(node *wordXMLNode, local string) *wordXMLNode {
	if node == nil {
		return nil
	}
	for _, child := range node.children {
		if child.name.Local == local {
			return child
		}
	}
	return nil
}

func findDescendant(node *wordXMLNode, local string) *wordXMLNode {
	for _, child := range node.children {
		if child.name.Local == local {
			return child
		}
		if found := findDescendant(child, local); found != nil {
			return found
		}
	}
	return nil
}

func containsNode(node *wordXMLNode, local string) bool { return findDescendant(node, local) != nil }

func attrValue(node *wordXMLNode, local string) string {
	value, _ := attrValuePresent(node, local)
	return value
}

func attrValuePresent(node *wordXMLNode, local string) (string, bool) {
	if node == nil {
		return "", false
	}
	for _, attr := range node.attrs {
		if attr.Name.Local == local {
			return attr.Value, true
		}
	}
	return "", false
}

func rawElementName(rawOpen []byte) string {
	if len(rawOpen) < 3 || rawOpen[0] != '<' {
		return ""
	}
	end := 1
	for end < len(rawOpen) {
		value := rawOpen[end]
		if value == '>' || value == '/' || value == ' ' || value == '\t' || value == '\r' || value == '\n' {
			break
		}
		end++
	}
	if end <= 1 {
		return ""
	}
	return string(rawOpen[1:end])
}

func cloneWordBlocks(blocks []document.WordBlock) []document.WordBlock {
	clone := make([]document.WordBlock, len(blocks))
	for i, block := range blocks {
		clone[i] = block
		if block.Unsupported != nil {
			unsupported := *block.Unsupported
			unsupported.ImagePreview = cloneWordImage(unsupported.ImagePreview)
			clone[i].Unsupported = &unsupported
		}
		if block.Paragraph != nil {
			paragraph := cloneWordParagraph(*block.Paragraph)
			clone[i].Paragraph = &paragraph
		}
		if block.Table != nil {
			table := document.WordTable{Layout: block.Table.Layout, Rows: make([]document.WordTableRow, len(block.Table.Rows))}
			table.Layout.GridColumnsEMU = append([]int64(nil), block.Table.Layout.GridColumnsEMU...)
			for rowIndex, row := range block.Table.Rows {
				table.Rows[rowIndex].Cells = make([]document.WordTableCell, len(row.Cells))
				for cellIndex, cell := range row.Cells {
					table.Rows[rowIndex].Cells[cellIndex] = document.WordTableCell{ID: cell.ID, WidthEMU: cell.WidthEMU, FillColor: cell.FillColor, Paragraphs: make([]document.WordParagraph, len(cell.Paragraphs))}
					for paragraphIndex, paragraph := range cell.Paragraphs {
						table.Rows[rowIndex].Cells[cellIndex].Paragraphs[paragraphIndex] = cloneWordParagraph(paragraph)
					}
				}
			}
			clone[i].Table = &table
		}
	}
	return clone
}

func cloneWordParagraph(paragraph document.WordParagraph) document.WordParagraph {
	clone := paragraph
	clone.Inlines = make([]document.WordInline, len(paragraph.Inlines))
	for i, inline := range paragraph.Inlines {
		clone.Inlines[i] = inline
		if inline.Text != nil {
			text := *inline.Text
			clone.Inlines[i].Text = &text
		}
		if inline.Image != nil {
			image := *inline.Image
			clone.Inlines[i].Image = &image
		}
		if inline.Unsupported != nil {
			unsupported := *inline.Unsupported
			unsupported.ImagePreview = cloneWordImage(unsupported.ImagePreview)
			clone.Inlines[i].Unsupported = &unsupported
		}
		if inline.PageBreak != nil {
			pageBreak := *inline.PageBreak
			clone.Inlines[i].PageBreak = &pageBreak
		}
	}
	return clone
}

func cloneWordImage(source *document.WordImage) *document.WordImage {
	if source == nil {
		return nil
	}
	clone := *source
	return &clone
}
