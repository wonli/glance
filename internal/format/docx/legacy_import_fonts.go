package docx

import (
	"bytes"
	"fmt"
	"glance/internal/document"
)

func legacyParagraphFontReplacements(data []byte, p *wordXMLNode, prefix string, fonts document.WordTextStyle) ([]xmlReplacement, error) {
	slots := []struct{ name, theme, value string }{{"ascii", "asciiTheme", fonts.FontFamilyASCII}, {"hAnsi", "hAnsiTheme", fonts.FontFamilyHighANSI}, {"eastAsia", "eastAsiaTheme", fonts.FontFamilyEastAsia}, {"cs", "cstheme", fonts.FontFamilyComplexScript}}
	present := false
	for _, slot := range slots {
		present = present || slot.value != ""
	}
	if !present {
		return nil, nil
	}
	var replacements []xmlReplacement
	var visit func(*wordXMLNode) error
	visit = func(n *wordXMLNode) error {
		if n.name.Local != "r" {
			for _, c := range n.children {
				if c.name.Local != "pPr" {
					if err := visit(c); err != nil {
						return err
					}
				}
			}
			return nil
		}
		props := firstChild(n, "rPr")
		old := firstChild(props, "rFonts")
		opening := []byte("<" + prefix + ":rFonts/>")
		fontPrefix := prefix
		if old != nil {
			opening = bytes.Clone(data[old.start:old.openEnd])
			fontPrefix = elementPrefix(old.rawName)
		}
		var err error
		for _, slot := range slots {
			if slot.value == "" {
				continue
			}
			opening, err = setRawXMLAttribute(opening, fontPrefix, slot.name, slot.value)
			if err != nil {
				return err
			}
			opening, err = removeRawXMLAttribute(opening, fontPrefix, slot.theme)
			if err != nil {
				return err
			}
			if slot.name == "cs" {
				opening, err = removeRawXMLAttribute(opening, fontPrefix, "csTheme")
				if err != nil {
					return err
				}
			}
		}
		if old != nil {
			markup := append(opening, data[old.openEnd:old.end]...)
			replacements = append(replacements, xmlReplacement{start: old.start, end: old.end, value: markup})
			return nil
		}
		if props == nil {
			markup := []byte(fmt.Sprintf("<%s:rPr>%s</%s:rPr>", prefix, opening, prefix))
			replacements = append(replacements, xmlReplacement{start: n.contentStart, end: n.contentStart, value: markup})
			return nil
		}
		if props.selfClosing {
			raw := bytes.TrimSpace(data[props.start:props.openEnd])
			markup := append(bytes.Clone(raw[:len(raw)-2]), '>')
			markup = append(markup, opening...)
			markup = append(markup, []byte("</"+props.rawName+">")...)
			replacements = append(replacements, xmlReplacement{start: props.start, end: props.end, value: markup})
			return nil
		}
		at := props.contentStart
		if style := firstChild(props, "rStyle"); style != nil {
			at = style.end
		}
		replacements = append(replacements, xmlReplacement{start: at, end: at, value: opening})
		return nil
	}
	if err := visit(p); err != nil {
		return nil, err
	}
	return replacements, nil
}
