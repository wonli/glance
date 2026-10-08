package pptx

import (
	"bytes"
	"encoding/xml"
	"image"
	"strconv"
	"strings"
)

const (
	pptxSlideLayoutRelationshipType = pptxOfficeRelationshipNS + "/slideLayout"
	pptxSlideMasterRelationshipType = pptxOfficeRelationshipNS + "/slideMaster"
	pptxThemeRelationshipType       = pptxOfficeRelationshipNS + "/theme"
)

type pptxBackground struct {
	ColorHex string
	Image    image.Image
	present  bool
}

type pptxXMLNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr    `xml:",any,attr"`
	Children []pptxXMLNode `xml:",any"`
	Text     string        `xml:",chardata"`
}

func (n pptxXMLNode) child(name string) *pptxXMLNode {
	for i := range n.Children {
		if n.Children[i].XMLName.Local == name {
			return &n.Children[i]
		}
	}
	return nil
}

func (n pptxXMLNode) descendant(name string) *pptxXMLNode {
	if n.XMLName.Local == name {
		return &n
	}
	for i := range n.Children {
		if found := n.Children[i].descendant(name); found != nil {
			return found
		}
	}
	return nil
}

func (n pptxXMLNode) attr(name string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (p *pptxPackage) resolveBackground(slidePart, layoutPart, masterPart string) pptxBackground {
	colorMap, theme := p.backgroundTheme(masterPart)
	for _, part := range []string{slidePart, layoutPart, masterPart} {
		if part == "" {
			continue
		}
		background, found := p.backgroundForPart(part, masterPart, colorMap, theme)
		if found {
			if background.ColorHex == "" && background.Image == nil {
				background.ColorHex = "#FFFFFF"
			}
			return background
		}
	}
	return pptxBackground{ColorHex: "#FFFFFF"}
}

func (p *pptxPackage) backgroundForPart(ownerPart, masterPart string, colorMap, theme map[string]string) (pptxBackground, bool) {
	part := p.parts[ownerPart]
	if part == nil {
		return pptxBackground{}, false
	}
	data, err := readOPCPart(part)
	if err != nil {
		return pptxBackground{}, false
	}
	var root pptxXMLNode
	if err := xml.Unmarshal(data, &root); err != nil {
		return pptxBackground{}, false
	}
	cSld := root.child("cSld")
	if cSld == nil {
		return pptxBackground{}, false
	}
	bg := cSld.child("bg")
	if bg == nil {
		return pptxBackground{}, false
	}
	background := pptxBackground{present: true}
	if bgPr := bg.child("bgPr"); bgPr != nil {
		if blipFill := bgPr.child("blipFill"); blipFill != nil {
			if blip := blipFill.child("blip"); blip != nil {
				if target, ok := p.imagePartForRelationship(ownerPart, blip.attr("embed")); ok {
					background.Image = p.image(target)
				}
			}
			return background, true
		}
		if solidFill := bgPr.child("solidFill"); solidFill != nil {
			background.ColorHex = resolveBackgroundColor(*solidFill, colorMap, theme)
			return background, true
		}
		return background, true
	}
	if bgRef := bg.child("bgRef"); bgRef != nil {
		if styled, found := p.backgroundReferenceStyle(masterPart, bgRef, colorMap, theme); found {
			styled.present = true
			return styled, true
		}
		background.ColorHex = resolveBackgroundColor(*bgRef, colorMap, theme)
		return background, true
	}
	return background, true
}

func (p *pptxPackage) backgroundTheme(masterPart string) (map[string]string, map[string]string) {
	colorMap := make(map[string]string)
	if master := p.parts[masterPart]; master != nil {
		if data, err := readOPCPart(master); err == nil {
			var root pptxXMLNode
			if xml.Unmarshal(data, &root) == nil {
				if mapping := root.child("clrMap"); mapping != nil {
					for _, key := range []string{"bg1", "tx1", "bg2", "tx2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"} {
						if value := mapping.attr(key); value != "" {
							colorMap[key] = value
						}
					}
				}
			}
		}
	}
	themeColors := make(map[string]string)
	if themePart, ok := p.relatedPart(masterPart, pptxThemeRelationshipType); ok {
		if theme := p.parts[themePart]; theme != nil {
			if data, err := readOPCPart(theme); err == nil {
				var root pptxXMLNode
				if xml.Unmarshal(data, &root) == nil {
					if scheme := root.descendant("clrScheme"); scheme != nil {
						for _, entry := range scheme.Children {
							if color := colorFromThemeEntry(entry); color != "" {
								themeColors[entry.XMLName.Local] = color
							}
						}
					}
				}
			}
		}
	}
	return colorMap, themeColors
}

func colorFromThemeEntry(entry pptxXMLNode) string {
	if color := entry.child("srgbClr"); color != nil {
		return normalizedRGB(color.attr("val"))
	}
	if color := entry.child("sysClr"); color != nil {
		return normalizedRGB(color.attr("lastClr"))
	}
	return ""
}

func resolveBackgroundColor(node pptxXMLNode, colorMap, themeColors map[string]string) string {
	if color := resolveDrawingColorNode(node, colorMap, themeColors); color != "" {
		return color
	}
	for _, child := range node.Children {
		if color := resolveDrawingColorNode(child, colorMap, themeColors); color != "" {
			return color
		}
	}
	return ""
}

func resolveDrawingColorNode(node pptxXMLNode, colorMap, themeColors map[string]string) string {
	switch node.XMLName.Local {
	case "srgbClr":
		return normalizedRGB(node.attr("val"))
	case "sysClr":
		return normalizedRGB(node.attr("lastClr"))
	case "schemeClr":
		scheme := node.attr("val")
		if mapped := colorMap[scheme]; mapped != "" {
			scheme = mapped
		}
		return themeColors[scheme]
	}
	return ""
}

func (p *pptxPackage) backgroundReferenceStyle(masterPart string, bgRef *pptxXMLNode, colorMap, themeColors map[string]string) (pptxBackground, bool) {
	idx, err := strconv.Atoi(bgRef.attr("idx"))
	if err != nil || idx < 0 {
		return pptxBackground{}, false
	}
	if idx == 0 || idx == 1000 {
		return pptxBackground{ColorHex: "#FFFFFF", present: true}, true
	}
	themePart, ok := p.relatedPart(masterPart, pptxThemeRelationshipType)
	if !ok || p.parts[themePart] == nil {
		return pptxBackground{}, false
	}
	data, err := readOPCPart(p.parts[themePart])
	if err != nil {
		return pptxBackground{}, false
	}
	var root pptxXMLNode
	if xml.Unmarshal(data, &root) != nil {
		return pptxBackground{}, false
	}
	themeElements := root.child("themeElements")
	if themeElements == nil {
		return pptxBackground{}, false
	}
	formatScheme := themeElements.child("fmtScheme")
	if formatScheme == nil {
		return pptxBackground{}, false
	}
	styleListName := "fillStyleLst"
	styleIndex := idx - 1
	if idx >= 1001 {
		styleListName = "bgFillStyleLst"
		styleIndex = idx - 1001
	}
	styleList := formatScheme.child(styleListName)
	if styleList == nil {
		return pptxBackground{}, false
	}
	if styleIndex < 0 || styleIndex >= len(styleList.Children) {
		return pptxBackground{}, false
	}
	fill := styleList.Children[styleIndex]
	background := pptxBackground{present: true}
	if solidFill := fillForStyle(fill, "solidFill"); solidFill != nil {
		if scheme := solidFill.child("schemeClr"); scheme != nil && scheme.attr("val") == "phClr" {
			background.ColorHex = resolveBackgroundColor(*bgRef, colorMap, themeColors)
		} else {
			background.ColorHex = resolveBackgroundColor(*solidFill, colorMap, themeColors)
		}
		return background, true
	}
	if blipFill := fillForStyle(fill, "blipFill"); blipFill != nil {
		if blip := blipFill.child("blip"); blip != nil {
			if target, ok := p.imagePartForRelationship(themePart, blip.attr("embed")); ok {
				background.Image = p.image(target)
			}
		}
		return background, true
	}
	return background, true
}

func fillForStyle(style pptxXMLNode, childName string) *pptxXMLNode {
	if style.XMLName.Local == childName {
		return &style
	}
	return style.child(childName)
}

func normalizedRGB(value string) string {
	value = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(value)), "#")
	if len(value) != 6 {
		return ""
	}
	if _, err := strconv.ParseUint(value, 16, 24); err != nil {
		return ""
	}
	return "#" + value
}

func (p *pptxPackage) image(partPath string) image.Image {
	if p.imageCache != nil {
		if cached := p.imageCache[partPath]; cached != nil {
			return cached
		}
	}
	part := p.parts[partPath]
	if part == nil {
		return nil
	}
	data, err := readOPCPart(part)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	if p.imageCache != nil {
		p.imageCache[partPath] = img
	}
	return img
}
