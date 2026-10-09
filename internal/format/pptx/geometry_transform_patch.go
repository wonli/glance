package pptx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type transformTagSpan struct {
	start, end int
	local      string
	parent     string
	selfClose  bool
}

type shapeTransformTags struct {
	spPr *transformTagSpan
	xfrm *transformTagSpan
	off  *transformTagSpan
	ext  *transformTagSpan
}

func inspectShapeTransformTags(fragment []byte) (shapeTransformTags, error) {
	decoder := xml.NewDecoder(bytes.NewReader(fragment))
	var stack []xml.Name
	var found shapeTransformTags
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return shapeTransformTags{}, fmt.Errorf("解析对象 XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := findNextXMLStart(fragment, before)
			end, err := xmlStartTagEnd(fragment, start)
			if err != nil {
				return shapeTransformTags{}, err
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1].Local
			}
			selfClose := bytes.HasSuffix(bytes.TrimSpace(fragment[start:end-1]), []byte("/"))
			span := transformTagSpan{start: start, end: end, local: value.Name.Local, parent: parent, selfClose: selfClose}
			switch {
			case value.Name.Local == "spPr" && found.spPr == nil:
				found.spPr = &span
			case value.Name.Local == "xfrm" && parent == "spPr" && found.xfrm == nil:
				found.xfrm = &span
			case value.Name.Local == "off" && parent == "xfrm" && found.off == nil:
				found.off = &span
			case value.Name.Local == "ext" && parent == "xfrm" && found.ext == nil:
				found.ext = &span
			}
			stack = append(stack, value.Name)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].Local != value.Name.Local {
				return shapeTransformTags{}, fmt.Errorf("对象 XML 元素层级不匹配")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if found.spPr == nil {
		return shapeTransformTags{}, fmt.Errorf("对象缺少 p:spPr")
	}
	return found, nil
}

func patchShapeTransformFragment(fragment []byte, slidePart string, slideIndex int, id uint32, pkg *pptxPackage, override presentationTransformOverride) ([]byte, error) {
	tags, err := inspectShapeTransformTags(fragment)
	if err != nil {
		return nil, err
	}
	if tags.xfrm == nil {
		inherited, err := inheritedPlaceholderTransform(pkg, slidePart, id)
		if err != nil {
			return nil, err
		}
		opening := append([]byte(nil), fragment[tags.spPr.start:tags.spPr.end]...)
		spPrName, err := xmlElementQName(opening)
		if err != nil {
			return nil, err
		}
		xfrm := materializedTransformXML(override, inherited)
		if tags.spPr.selfClose {
			opening = bytes.TrimSuffix(opening, []byte("/>"))
			opening = append(opening, '>')
			result := make([]byte, 0, len(fragment)+len(xfrm)+len(spPrName)+4)
			result = append(result, fragment[:tags.spPr.start]...)
			result = append(result, opening...)
			result = append(result, xfrm...)
			result = append(result, []byte("</"+spPrName+">")...)
			result = append(result, fragment[tags.spPr.end:]...)
			return result, nil
		}
		result := make([]byte, 0, len(fragment)+len(xfrm))
		result = append(result, fragment[:tags.spPr.end]...)
		result = append(result, xfrm...)
		result = append(result, fragment[tags.spPr.end:]...)
		return result, nil
	}
	if tags.off == nil || tags.ext == nil || tags.xfrm.selfClose {
		return nil, fmt.Errorf("对象的 a:xfrm 缺少 a:off 或 a:ext")
	}
	updates := []struct {
		span    *transformTagSpan
		changes map[string]string
	}{
		{span: tags.xfrm, changes: map[string]string{"rot": strconv.FormatInt(degreesToDrawingML(override.RotationDeg), 10)}},
		{span: tags.off, changes: map[string]string{
			"x": strconv.FormatInt(pointToEMU(override.Bounds.X), 10),
			"y": strconv.FormatInt(pointToEMU(override.Bounds.Y), 10),
		}},
		{span: tags.ext, changes: map[string]string{
			"cx": strconv.FormatInt(pointToEMU(override.Bounds.W), 10),
			"cy": strconv.FormatInt(pointToEMU(override.Bounds.H), 10),
		}},
	}
	for _, update := range updates {
		if update.span.selfClose && update.span.local == "xfrm" {
			return nil, fmt.Errorf("对象的 a:xfrm 缺少 a:off 或 a:ext")
		}
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].span.start > updates[j].span.start })
	result := append([]byte(nil), fragment...)
	for _, update := range updates {
		startTag := result[update.span.start:update.span.end]
		patchedTag, err := setXMLStartTagAttributes(startTag, update.changes)
		if err != nil {
			return nil, err
		}
		replacement := shapeXMLReplacement{start: update.span.start, end: update.span.end, data: patchedTag}
		result = append(result[:replacement.start], append(replacement.data, result[replacement.end:]...)...)
	}
	return result, nil
}

func inheritedPlaceholderTransform(pkg *pptxPackage, slidePart string, id uint32) (presentationTransformMetadata, error) {
	slideRoot, ok := pkg.xmlPart(slidePart)
	if !ok {
		return presentationTransformMetadata{}, fmt.Errorf("无法读取幻灯片 XML")
	}
	tree := shapeTree(slideRoot)
	if tree == nil {
		return presentationTransformMetadata{}, fmt.Errorf("幻灯片缺少形状树")
	}
	var target *pptxXMLNode
	for index := range tree.Children {
		shape := &tree.Children[index]
		if shape.XMLName.Local == "sp" && shapeIDFromXML(*shape) == id {
			if target != nil {
				return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 不唯一", id)
			}
			target = shape
		}
	}
	if target == nil {
		return presentationTransformMetadata{}, fmt.Errorf("未找到幻灯片形状 ID %d", id)
	}
	slideRecord, ok := placeholderRecordForShape(*target)
	if !ok || slideRecord.hasXfrm {
		return presentationTransformMetadata{}, fmt.Errorf("形状 ID %d 没有可继承的占位符几何", id)
	}
	layoutPart, _ := pkg.relatedPart(slidePart, pptxSlideLayoutRelationshipType)
	masterPart, _ := pkg.relatedPart(layoutPart, pptxSlideMasterRelationshipType)
	layoutRoot, _ := pkg.xmlPart(layoutPart)
	if record, found, ambiguous := uniquePlaceholderRecord(shapeTree(layoutRoot), slideRecord.key); ambiguous {
		return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 的版式几何不唯一", id)
	} else if found && record.hasXfrm && validPresentationRect(record.bounds) {
		metadata := record.transform
		metadata.Bounds = record.bounds
		return metadata, nil
	}
	masterRoot, _ := pkg.xmlPart(masterPart)
	if record, found, ambiguous := uniquePlaceholderRecord(shapeTree(masterRoot), slideRecord.key); ambiguous {
		return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 的母版几何不唯一", id)
	} else if found && record.hasXfrm && validPresentationRect(record.bounds) {
		metadata := record.transform
		metadata.Bounds = record.bounds
		return metadata, nil
	}
	return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 没有有效的继承几何", id)
}

func materializedTransformXML(override presentationTransformOverride, inherited presentationTransformMetadata) []byte {
	attrs := []string{
		`xmlns="` + drawingMLNamespace + `"`,
		`rot="` + strconv.FormatInt(degreesToDrawingML(override.RotationDeg), 10) + `"`,
	}
	if inherited.FlipH {
		attrs = append(attrs, `flipH="1"`)
	}
	if inherited.FlipV {
		attrs = append(attrs, `flipV="1"`)
	}
	return []byte(fmt.Sprintf(`<xfrm %s><off x="%d" y="%d"/><ext cx="%d" cy="%d"/></xfrm>`, strings.Join(attrs, " "),
		pointToEMU(override.Bounds.X), pointToEMU(override.Bounds.Y), pointToEMU(override.Bounds.W), pointToEMU(override.Bounds.H)))
}
