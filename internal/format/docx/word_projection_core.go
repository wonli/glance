package docx

import (
	"errors"
	"fmt"
	"glance/internal/document"
)

type wordProjection struct {
	blocks           []document.WordBlock
	layout           document.WordLayoutData
	imageParts       map[string]int
	imageTargets     map[string]wordImageTarget
	noteParts        map[string]wordNotePart
	usedNotes        map[string]map[string]bool
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

func (d *Document) WordBlockWindow(start, limit int) ([]document.WordBlock, int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return nil, 0, err
	}
	if start < 0 || limit < 0 {
		return nil, 0, errors.New("Word block window bounds must be non-negative")
	}
	total := len(d.wordBlocks)
	start = min(start, total)
	end := min(start+limit, total)
	return cloneWordBlocks(d.wordBlocks[start:end]), total, nil
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
	noteParts := readWordNoteParts(parts, partIndexes, relationships)
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
		numbering: numbering, noteParts: noteParts, usedNotes: make(map[string]map[string]bool),
		imageParts: make(map[string]int), imageTargets: make(map[string]wordImageTarget), inlineTargets: make(map[string]wordInlineTarget),
		paragraphInlines: make(map[string][]string), textTargets: make(map[string]wordTextTarget), paragraphTargets: make(map[string]*wordXMLNode), seenImageIDs: make(map[string]bool),
	}
	var blocks []document.WordBlock
	contentBlock := 0
	for _, child := range body.children {
		blockID := fmt.Sprintf("b%d", contentBlock)
		switch child.name.Local {
		case "p":
			blocks = append(blocks, projector.projectBodyBlock(child, blockID))
			contentBlock++
		case "tbl":
			blocks = append(blocks, projector.projectBodyBlock(child, blockID))
			contentBlock++
		case "sdt":
			content := firstChild(child, "sdtContent")
			if content == nil {
				blocks = append(blocks, projector.projectBodyBlock(child, blockID))
				contentBlock++
				continue
			}
			for contentIndex, contentChild := range content.children {
				if isWordRangeMarker(contentChild.name.Local) {
					continue
				}
				contentBlockID := fmt.Sprintf("%s.c%d", blockID, contentIndex)
				block := projector.projectBodyBlock(contentChild, contentBlockID)
				makeWordBlockReadOnly(&block)
				blocks = append(blocks, block)
				contentBlock++
			}
		case "sectPr":
			// Section properties affect layout metadata, not body content.
		case "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd", "permStart", "permEnd":
			// Range markers are zero-width annotations. Word permits them
			// between body blocks; they do not represent unsupported content.
			continue
		default:
			unsupported := &document.WordUnsupported{ID: blockID + ".unsupported", Description: "Unsupported body content: " + child.name.Local}
			blocks = append(blocks, document.WordBlock{ID: blockID, Kind: document.WordUnsupportedBlock, Unsupported: unsupported})
			contentBlock++
		}
	}
	projector.appendReferencedNotes(&blocks)
	layout := projectWordLayoutData(parts, partIndexes, relationships, documentRoot)
	return wordProjection{blocks: blocks, layout: layout, imageParts: projector.imageParts, imageTargets: projector.imageTargets, inlineTargets: projector.inlineTargets, paragraphInlines: projector.paragraphInlines, textTargets: projector.textTargets, paragraphTargets: projector.paragraphTargets}, nil
}

func (p *wordProjector) projectBodyBlock(node *wordXMLNode, id string) document.WordBlock {
	switch node.name.Local {
	case "p":
		paragraph := p.paragraph(node, id+".p")
		return document.WordBlock{ID: id, Kind: document.WordParagraphBlock, Paragraph: &paragraph}
	case "tbl":
		table := p.table(node, id)
		return document.WordBlock{ID: id, Kind: document.WordTableBlock, Table: &table}
	default:
		return document.WordBlock{
			ID: id, Kind: document.WordUnsupportedBlock,
			Unsupported: &document.WordUnsupported{ID: id + ".unsupported", Description: "Unsupported body content: " + node.name.Local},
		}
	}
}

func isWordRangeMarker(name string) bool {
	switch name {
	case "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd", "permStart", "permEnd":
		return true
	default:
		return false
	}
}

func makeWordBlockReadOnly(block *document.WordBlock) {
	if block == nil {
		return
	}
	makeParagraphReadOnly := func(paragraph *document.WordParagraph) {
		if paragraph == nil {
			return
		}
		for inlineIndex := range paragraph.Inlines {
			inline := &paragraph.Inlines[inlineIndex]
			if inline.Text != nil {
				inline.Text.Editable = false
			}
			if inline.Image != nil {
				inline.Image.Editable = false
			}
		}
	}
	if block.Kind == document.WordParagraphBlock {
		makeParagraphReadOnly(block.Paragraph)
		return
	}
	if block.Kind == document.WordTableBlock && block.Table != nil {
		for rowIndex := range block.Table.Rows {
			for cellIndex := range block.Table.Rows[rowIndex].Cells {
				cell := &block.Table.Rows[rowIndex].Cells[cellIndex]
				for paragraphIndex := range cell.Paragraphs {
					makeParagraphReadOnly(&cell.Paragraphs[paragraphIndex])
				}
			}
		}
	}
}

type wordProjector struct {
	parts            []part
	partIndexes      map[string]int
	relationships    map[string]wordRelationship
	contentTypes     wordContentTypes
	numbering        map[string]wordNumberingDefinition
	noteParts        map[string]wordNotePart
	usedNotes        map[string]map[string]bool
	imageParts       map[string]int
	imageTargets     map[string]wordImageTarget
	inlineTargets    map[string]wordInlineTarget
	paragraphInlines map[string][]string
	textTargets      map[string]wordTextTarget
	paragraphTargets map[string]*wordXMLNode
	seenImageIDs     map[string]bool
}
