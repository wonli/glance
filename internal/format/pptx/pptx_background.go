package pptx

import (
	"fmt"
	"strings"
)

type presentationBackgroundOverride struct {
	Set      bool
	ColorHex string
}

// SetSlideBackgroundByID writes a solid RGB background to a slide identified
// by its stable slide ID. Unsupported background effects and package parts
// remain untouched; only the slide's fill choice is replaced.
func (d *Document) SetSlideBackgroundByID(slideID uint32, colorHex string) (bool, error) {
	colorHex = normalizedRGB(colorHex)
	if colorHex == "" {
		return false, fmt.Errorf("幻灯片背景颜色必须为 #RRGGBB")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		current := d.slideBackgrounds[index]
		if normalizedRGB(current.ColorHex) == colorHex && current.Image == nil {
			return false, nil
		}
		d.slideBackgrounds[index] = pptxBackground{ColorHex: colorHex, present: true}
		d.slideBackgroundChanges[index] = presentationBackgroundOverride{Set: true, ColorHex: colorHex}
		d.dirty = true
		d.revision++
		return true, nil
	}
	return false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func patchSlideBackground(data []byte, colorHex string) ([]byte, error) {
	colorHex = normalizedRGB(colorHex)
	if colorHex == "" {
		return nil, fmt.Errorf("幻灯片背景颜色必须为 #RRGGBB")
	}
	nodes, err := scanStyleXML(data)
	if err != nil {
		return nil, err
	}
	cSldIndex := firstStyleNode(nodes, "cSld")
	if cSldIndex < 0 {
		return nil, fmt.Errorf("幻灯片缺少 p:cSld")
	}
	cSld := nodes[cSldIndex]
	if cSld.selfClose {
		return nil, fmt.Errorf("幻灯片 p:cSld 结构不受支持")
	}
	cSldData := data[cSld.start:cSld.end]
	backgroundXML := newSolidBackgroundXML(styleXMLPrefix(cSld.qName), colorHex)
	for _, childIndex := range styleNodeChildren(nodes, cSldIndex) {
		child := nodes[childIndex]
		if child.local != "bg" {
			continue
		}
		backgroundData := data[child.start:child.end]
		patched, err := patchBackgroundElement(backgroundData, backgroundXML, colorHex)
		if err != nil {
			return nil, err
		}
		cSldData = applyXMLReplacements(cSldData, []shapeXMLReplacement{{start: child.start - cSld.start, end: child.end - cSld.start, data: patched}})
		return applyXMLReplacements(data, []shapeXMLReplacement{{start: cSld.start, end: cSld.end, data: cSldData}}), nil
	}

	patched, err := upsertStyleChild(cSldData, []string{"bg"}, backgroundXML,
		[]string{"bg", "spTree", "clrMapOvr", "extLst"})
	if err != nil {
		return nil, err
	}
	return applyXMLReplacements(data, []shapeXMLReplacement{{start: cSld.start, end: cSld.end, data: patched}}), nil
}

func newSolidBackgroundXML(presentationPrefix, colorHex string) []byte {
	color := strings.TrimPrefix(colorHex, "#")
	return []byte(fmt.Sprintf(
		"<%sbg><%sbgPr><a:solidFill xmlns:a=\"%s\"><a:srgbClr val=\"%s\"/></a:solidFill></%sbgPr></%sbg>",
		presentationPrefix, presentationPrefix, drawingMLNamespace, color, presentationPrefix, presentationPrefix,
	))
}

func patchBackgroundElement(background, newBackground []byte, colorHex string) ([]byte, error) {
	nodes, err := scanStyleXML(background)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 || nodes[0].local != "bg" {
		return nil, fmt.Errorf("幻灯片背景 XML 无效")
	}
	var bgPrIndex = -1
	for _, childIndex := range styleNodeChildren(nodes, 0) {
		if nodes[childIndex].local == "bgPr" {
			bgPrIndex = childIndex
			break
		}
	}
	colorHex = strings.TrimPrefix(normalizedRGB(colorHex), "#")
	if colorHex == "" {
		return nil, fmt.Errorf("幻灯片背景颜色必须为 #RRGGBB")
	}
	newNodes, err := scanStyleXML(newBackground)
	if err != nil || len(newNodes) == 0 {
		return nil, fmt.Errorf("创建幻灯片背景 XML 失败")
	}
	newBGPrIndex := -1
	for _, childIndex := range styleNodeChildren(newNodes, 0) {
		if newNodes[childIndex].local == "bgPr" {
			newBGPrIndex = childIndex
			break
		}
	}
	if newBGPrIndex < 0 {
		return nil, fmt.Errorf("创建幻灯片背景属性失败")
	}
	newBGPr := newBackground[newNodes[newBGPrIndex].start:newNodes[newBGPrIndex].end]
	fill := []byte(fmt.Sprintf("<a:solidFill xmlns:a=\"%s\"><a:srgbClr val=\"%s\"/></a:solidFill>", drawingMLNamespace, colorHex))
	if bgPrIndex >= 0 {
		span := nodes[bgPrIndex]
		bgPr := background[span.start:span.end]
		patched, err := upsertStyleChild(bgPr,
			[]string{"noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill"}, fill,
			[]string{"noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill", "effectLst", "effectDag", "extLst"})
		if err != nil {
			return nil, err
		}
		return applyXMLReplacements(background, []shapeXMLReplacement{{start: span.start, end: span.end, data: patched}}), nil
	}
	patched, err := upsertStyleChild(background, []string{"bgRef"}, newBGPr,
		[]string{"bgPr", "bgRef", "extLst"})
	if err != nil {
		return nil, err
	}
	return patched, nil
}
