package docx

import (
	"strconv"
	"strings"

	"glance/internal/document"
	"glance/internal/textintern"
)

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
		style.FontColor = textintern.String(strings.ToUpper(attrValue(color, "val")))
		if style.FontColor == "AUTO" {
			style.FontColor = ""
		}
	}
	if highlight := firstChild(properties, "highlight"); highlight != nil {
		style.Highlight = textintern.String(strings.TrimSpace(attrValue(highlight, "val")))
		if strings.EqualFold(style.Highlight, "none") {
			style.Highlight = ""
		}
	}
	if fonts := firstChild(properties, "rFonts"); fonts != nil {
		style.FontFamilyASCII = textintern.String(attrValue(fonts, "ascii"))
		style.FontFamilyHighANSI = textintern.String(attrValue(fonts, "hAnsi"))
		style.FontFamilyEastAsia = textintern.String(attrValue(fonts, "eastAsia"))
		style.FontFamilyComplexScript = textintern.String(attrValue(fonts, "cs"))
		for _, key := range []string{"eastAsia", "ascii", "hAnsi", "cs"} {
			if value := attrValue(fonts, key); value != "" {
				style.FontFamily = textintern.String(value)
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
