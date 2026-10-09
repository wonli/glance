package docx

import (
	"strings"

	"glance/internal/document"
)

func (p *wordProjector) addParagraphFallback(paragraph *document.WordParagraph, id string, source *wordXMLNode, label, warning string) {
	text := strings.Join(wordElementParagraphs(source), "\n")
	if text == "" {
		text = label
	}
	p.appendFallbackText(paragraph, id, text)
	p.addParagraphWarning(paragraph, warning)
}

func (p *wordProjector) addParagraphWarning(paragraph *document.WordParagraph, warning string) {
	if paragraph == nil {
		return
	}
	paragraph.PreviewWarning = appendWordPreviewWarning(paragraph.PreviewWarning, warning)
}

func (p *wordProjector) appendAlternateContent(paragraph *document.WordParagraph, node *wordXMLNode, id string) {
	choice := firstChild(node, "Choice")
	fallback := firstChild(node, "Fallback")
	selected := choice
	if selected == nil {
		selected = fallback
	}
	text := wordAlternativeContentText(selected)
	if strings.TrimSpace(text) == "" && fallback != nil && fallback != selected {
		text = wordAlternativeContentText(fallback)
	}
	if strings.TrimSpace(text) == "" {
		text = "[Embedded grouped shape or chart]"
	}
	p.appendFallbackText(paragraph, id, text)
	p.addParagraphWarning(paragraph, "AlternateContent text is preserved; embedded grouped shapes and charts are shown as read-only fallback text.")
}

func wordAlternativeContentText(node *wordXMLNode) string {
	if node == nil {
		return ""
	}
	textBox := findDescendant(node, "txbxContent")
	if textBox == nil {
		textBox = node
	}
	paragraphs := wordElementParagraphs(textBox)
	if len(paragraphs) != 0 {
		return strings.Join(paragraphs, "\n")
	}
	return strings.TrimSpace(wordElementText(textBox))
}

func (p *wordProjector) makeParagraphReadOnly(paragraph *document.WordParagraph) {
	if paragraph == nil {
		return
	}
	for index := range paragraph.Inlines {
		inline := &paragraph.Inlines[index]
		if inline.Text != nil {
			inline.Text.Editable = false
			delete(p.textTargets, inline.Text.ID)
			delete(p.inlineTargets, inline.Text.ID)
		}
		if inline.Image != nil {
			inline.Image.Editable = false
			delete(p.imageTargets, inline.Image.ID)
		}
	}
}

func appendWordPreviewWarning(current, warning string) string {
	if warning == "" || strings.Contains(current, warning) {
		return current
	}
	if current == "" {
		return warning
	}
	return current + " " + warning
}

func (p *wordProjector) appendFallbackText(paragraph *document.WordParagraph, id, text string) {
	if paragraph == nil || text == "" {
		return
	}
	paragraph.Inlines = append(paragraph.Inlines, document.WordInline{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: id, Text: text, Editable: false},
	})
	p.paragraphInlines[paragraph.ID] = append(p.paragraphInlines[paragraph.ID], id)
}
