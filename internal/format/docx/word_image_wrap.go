package docx

import (
	"bytes"
	"fmt"

	"glance/internal/document"
)

const (
	wordDrawingNamespace     = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
	drawingMLNamespace       = "http://schemas.openxmlformats.org/drawingml/2006/main"
	word2010DrawingNamespace = "http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing"
	xmlnsNamespace           = "http://www.w3.org/2000/xmlns/"
)

func drawingWrapMode(anchor *wordXMLNode) string {
	if anchor == nil || anchor.name.Local != "anchor" {
		return "inline"
	}
	for _, child := range anchor.children {
		if child.name.Space != wordDrawingNamespace {
			continue
		}
		switch child.name.Local {
		case "wrapNone":
			return "none"
		case "wrapSquare":
			return "square"
		case "wrapTight":
			return "tight"
		case "wrapThrough":
			return "through"
		case "wrapTopAndBottom":
			return "top_and_bottom"
		}
	}
	return "unknown"
}

func wordAnchorCanConvertToInline(anchor *wordXMLNode) bool {
	if anchor == nil || anchor.name.Space != wordDrawingNamespace || anchor.name.Local != "anchor" || anchor.selfClosing {
		return false
	}
	for _, attr := range anchor.attrs {
		if attr.Name.Space == "xmlns" || attr.Name.Space == xmlnsNamespace || attr.Name.Local == "xmlns" {
			continue
		}
		switch attr.Name.Space {
		case "", wordDrawingNamespace:
			switch attr.Name.Local {
			case "distT", "distB", "distL", "distR", "simplePos", "relativeHeight", "behindDoc", "locked", "layoutInCell", "allowOverlap":
			default:
				return false
			}
		case word2010DrawingNamespace:
			if attr.Name.Local != "anchorId" && attr.Name.Local != "editId" {
				return false
			}
		default:
			return false
		}
	}
	seen := make(map[string]bool)
	for _, child := range anchor.children {
		if child.name.Local == "graphic" && child.name.Space == drawingMLNamespace {
			if seen["graphic"] {
				return false
			}
			seen["graphic"] = true
			continue
		}
		if child.name.Space != wordDrawingNamespace {
			return false
		}
		switch child.name.Local {
		case "simplePos", "positionH", "positionV", "wrapNone", "wrapSquare", "wrapTight", "wrapThrough", "wrapTopAndBottom":
			if seen[child.name.Local] {
				return false
			}
			seen[child.name.Local] = true
			continue
		case "extent", "docPr", "effectExtent", "cNvGraphicFramePr":
			if seen[child.name.Local] {
				return false
			}
			seen[child.name.Local] = true
		default:
			return false
		}
	}
	return seen["extent"] && seen["docPr"] && seen["graphic"]
}

func (d *Document) SetWordImageWrap(imageID, wrapMode string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if wrapMode != "inline" {
		return fmt.Errorf("当前仅支持将 Word 图片设为嵌入型环绕")
	}
	image := findProjectedWordImage(d.wordBlocks, imageID)
	if image == nil {
		return fmt.Errorf("未找到 Word 图片 %q", imageID)
	}
	if !image.WrapEditable {
		return fmt.Errorf("Word 图片 %q 的环绕结构不支持安全修改", imageID)
	}
	if image.WrapMode == "inline" {
		return nil
	}
	documentXML := d.parts[d.documentIdx].data
	root, err := parseWordXML(documentXML)
	if err != nil {
		return fmt.Errorf("解析 Word 图片失败: %w", err)
	}
	var target *wordXMLNode
	matches := 0
	walkWordXML(root, func(node *wordXMLNode) {
		if node.name.Space != wordDrawingNamespace || node.name.Local != "anchor" {
			return
		}
		docPr := firstChild(node, "docPr")
		if docPr == nil || "img-"+attrValue(docPr, "id") != imageID {
			return
		}
		target = node
		matches++
	})
	if matches != 1 || !wordAnchorCanConvertToInline(target) {
		return fmt.Errorf("Word 图片 %q 的浮动结构不唯一或不支持安全转换", imageID)
	}
	inline, err := drawingAnchorAsInline(documentXML, target)
	if err != nil {
		return err
	}
	updated, err := applyXMLReplacements(documentXML, []xmlReplacement{{start: target.start, end: target.end, value: inline}})
	if err != nil {
		return err
	}
	return d.commitWordPackageChange(map[string]*part{"word/document.xml": d.partWithData("word/document.xml", updated)})
}

func findProjectedWordImage(blocks []document.WordBlock, imageID string) *document.WordImage {
	for _, block := range blocks {
		if block.Paragraph != nil {
			for _, inline := range block.Paragraph.Inlines {
				if inline.Image != nil && inline.Image.ID == imageID {
					return inline.Image
				}
			}
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						for _, inline := range paragraph.Inlines {
							if inline.Image != nil && inline.Image.ID == imageID {
								return inline.Image
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func drawingAnchorAsInline(data []byte, anchor *wordXMLNode) ([]byte, error) {
	if !wordAnchorCanConvertToInline(anchor) || anchor.start < 0 || anchor.end > len(data) || anchor.end <= anchor.start {
		return nil, fmt.Errorf("Word 浮动图片结构无效，拒绝转换")
	}
	opening := data[anchor.start:anchor.openEnd]
	attrs, _, _, err := scanRawXMLAttributes(opening)
	if err != nil {
		return nil, fmt.Errorf("读取 Word 图片定位属性失败: %w", err)
	}
	qualifiedName := "inline"
	if prefix := elementPrefix(anchor.rawName); prefix != "" {
		qualifiedName = prefix + ":inline"
	}
	var result bytes.Buffer
	result.Grow(anchor.end - anchor.start)
	result.WriteByte('<')
	result.WriteString(qualifiedName)
	for _, attr := range attrs {
		keep := attr.name == "xmlns" || attr.prefix == "xmlns"
		keep = keep || attr.prefix == "" && (attr.local == "distT" || attr.local == "distB" || attr.local == "distL" || attr.local == "distR")
		keep = keep || attr.prefix == "wp14" && (attr.local == "anchorId" || attr.local == "editId")
		if keep {
			result.WriteByte(' ')
			result.Write(opening[attr.start:attr.end])
		}
	}
	result.WriteByte('>')
	allowedOrder := map[string]int{"extent": 1, "effectExtent": 2, "docPr": 3, "cNvGraphicFramePr": 4, "graphic": 5}
	lastOrder := 0
	var removed []xmlReplacement
	for _, child := range anchor.children {
		order := 0
		if child.name.Space == wordDrawingNamespace {
			order = allowedOrder[child.name.Local]
		} else if child.name.Space == drawingMLNamespace && child.name.Local == "graphic" {
			order = allowedOrder["graphic"]
		}
		if order == 0 {
			removed = append(removed, xmlReplacement{start: child.start - anchor.contentStart, end: child.end - anchor.contentStart})
			continue
		}
		if order < lastOrder {
			return nil, fmt.Errorf("Word 浮动图片子节点顺序不受支持，拒绝转换")
		}
		lastOrder = order
	}
	inner, err := applyXMLReplacements(data[anchor.contentStart:anchor.contentEnd], removed)
	if err != nil {
		return nil, fmt.Errorf("移除 Word 图片浮动定位失败: %w", err)
	}
	result.Write(inner)
	result.WriteString("</" + qualifiedName + ">")
	return result.Bytes(), nil
}

func walkWordXML(node *wordXMLNode, visit func(*wordXMLNode)) {
	if node == nil {
		return
	}
	visit(node)
	for _, child := range node.children {
		walkWordXML(child, visit)
	}
}

var _ document.WordImageWrapEditor = (*Document)(nil)
