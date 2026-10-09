package docx

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

type wordNote struct {
	paragraphs []string
}

type wordNotePart struct {
	items map[string]wordNote
	order []string
}

func readWordNoteParts(parts []part, partIndexes map[string]int, relationships map[string]wordRelationship) map[string]wordNotePart {
	result := make(map[string]wordNotePart)
	for _, definition := range []struct {
		key     string
		related func(wordRelationship) bool
		element string
	}{
		{key: "footnotes", related: func(r wordRelationship) bool { return r.footnotes }, element: "footnote"},
		{key: "endnotes", related: func(r wordRelationship) bool { return r.endnotes }, element: "endnote"},
	} {
		partName := ""
		for _, relationship := range relationships {
			if definition.related(relationship) && !relationship.external {
				partName = relationship.target
				break
			}
		}
		index, ok := partIndexes[partName]
		if partName == "" || !ok {
			continue
		}
		root, err := parseWordXML(parts[index].data)
		if err != nil {
			continue
		}
		part := wordNotePart{items: make(map[string]wordNote)}
		for _, node := range root.children {
			if node.name.Local != definition.element || attrValue(node, "type") == "separator" || attrValue(node, "type") == "continuationSeparator" {
				continue
			}
			id := attrValue(node, "id")
			if id == "" {
				continue
			}
			paragraphs := wordElementParagraphs(node)
			if len(paragraphs) == 0 {
				continue
			}
			part.items[id] = wordNote{paragraphs: paragraphs}
			part.order = append(part.order, id)
		}
		if len(part.items) != 0 {
			result[definition.key] = part
		}
	}
	return result
}

func wordElementParagraphs(note *wordXMLNode) []string {
	var paragraphs []string
	var collect func(*wordXMLNode)
	collect = func(node *wordXMLNode) {
		for _, child := range node.children {
			if child.name.Local == "p" {
				if text := wordElementText(child); strings.TrimSpace(text) != "" {
					paragraphs = append(paragraphs, text)
				}
				continue
			}
			collect(child)
		}
	}
	collect(note)
	if len(paragraphs) == 0 {
		if text := wordElementText(note); strings.TrimSpace(text) != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	return paragraphs
}

func wordElementText(node *wordXMLNode) string {
	var text strings.Builder
	var collect func(*wordXMLNode)
	collect = func(current *wordXMLNode) {
		switch current.name.Local {
		case "t":
			text.WriteString(current.text)
		case "tab":
			text.WriteByte('\t')
		case "br", "cr":
			text.WriteByte('\n')
		case "instrText", "delText":
			return
		default:
			for _, child := range current.children {
				collect(child)
			}
		}
	}
	collect(node)
	return text.String()
}

func (p *wordProjector) appendNoteReference(paragraph *document.WordParagraph, id, kind, noteID string) bool {
	part, ok := p.noteParts[kind]
	if !ok {
		return false
	}
	if _, ok := part.items[noteID]; !ok {
		return false
	}
	if p.usedNotes[kind] == nil {
		p.usedNotes[kind] = make(map[string]bool)
	}
	p.usedNotes[kind][noteID] = true
	paragraph.Inlines = append(paragraph.Inlines, document.WordInline{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: id, Text: noteID, Editable: false},
	})
	p.paragraphInlines[paragraph.ID] = append(p.paragraphInlines[paragraph.ID], id)
	return true
}

func (p *wordProjector) appendReferencedNotes(blocks *[]document.WordBlock) {
	for _, kind := range []string{"footnotes", "endnotes"} {
		part, ok := p.noteParts[kind]
		if !ok {
			continue
		}
		for _, noteID := range part.order {
			if !p.usedNotes[kind][noteID] {
				continue
			}
			note := part.items[noteID]
			for paragraphIndex, noteText := range note.paragraphs {
				blockID := fmt.Sprintf("notes-%s-%s-p%d", kind, noteID, paragraphIndex)
				if paragraphIndex == 0 {
					noteText = "[" + noteID + "] " + noteText
				}
				paragraph := document.WordParagraph{ID: blockID + ".paragraph", Alignment: document.WordAlignDefault}
				paragraph.Inlines = []document.WordInline{{
					Kind: document.WordTextInline,
					Text: &document.WordTextRun{ID: blockID + ".text", Text: noteText, Editable: false},
				}}
				*blocks = append(*blocks, document.WordBlock{ID: blockID, Kind: document.WordParagraphBlock, Paragraph: &paragraph})
			}
		}
	}
}
