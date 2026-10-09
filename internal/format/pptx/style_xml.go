package pptx

import (
	"fmt"
	"math"
	"strconv"

	"glance/internal/document"
)

type styleXMLNode struct {
	local              string
	qName              string
	parent             int
	start, startTagEnd int
	closeStart, end    int
	selfClose          bool
}

func patchParagraphStyleFragment(fragment []byte, patch document.PresentationElementStylePatch) ([]byte, error) {
	nodes, err := scanStyleXML(fragment)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 || nodes[0].local != "p" {
		return nil, fmt.Errorf("段落样式目标无效")
	}
	prefix := styleXMLPrefix(nodes[0].qName)
	var replacements []shapeXMLReplacement
	pPrIndex := -1
	for _, child := range styleNodeChildren(nodes, 0) {
		if nodes[child].local == "pPr" {
			pPrIndex = child
			break
		}
	}
	if patch.ParagraphAlignment != nil || patch.LineSpacing != nil || patch.SpaceBefore != nil || patch.SpaceAfter != nil {
		var pPr []byte
		if pPrIndex >= 0 {
			span := nodes[pPrIndex]
			pPr = append([]byte(nil), fragment[span.start:span.end]...)
		} else {
			pPr = []byte("<" + prefix + "pPr/>")
		}
		pPr, err = patchParagraphProperties(pPr, patch)
		if err != nil {
			return nil, err
		}
		if pPrIndex >= 0 {
			span := nodes[pPrIndex]
			replacements = append(replacements, shapeXMLReplacement{start: span.start, end: span.end, data: pPr})
		} else {
			replacements = append(replacements, shapeXMLReplacement{start: nodes[0].startTagEnd, end: nodes[0].startTagEnd, data: pPr})
		}
	}
	for _, child := range styleNodeChildren(nodes, 0) {
		if nodes[child].local != "r" && nodes[child].local != "endParaRPr" {
			continue
		}
		span := nodes[child]
		run := append([]byte(nil), fragment[span.start:span.end]...)
		if nodes[child].local == "r" {
			run, err = patchRunElement(run, patch)
		} else {
			run, err = patchRunProperties(run, patch)
		}
		if err != nil {
			return nil, err
		}
		replacements = append(replacements, shapeXMLReplacement{start: span.start, end: span.end, data: run})
	}
	return applyXMLReplacements(fragment, replacements), nil
}

func patchRunElement(fragment []byte, patch document.PresentationElementStylePatch) ([]byte, error) {
	nodes, err := scanStyleXML(fragment)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 || nodes[0].local != "r" {
		return nil, fmt.Errorf("文字运行样式目标无效")
	}
	for _, child := range styleNodeChildren(nodes, 0) {
		if nodes[child].local == "rPr" {
			span := nodes[child]
			props := append([]byte(nil), fragment[span.start:span.end]...)
			props, err = patchRunProperties(props, patch)
			if err != nil {
				return nil, err
			}
			return applyXMLReplacements(fragment, []shapeXMLReplacement{{start: span.start, end: span.end, data: props}}), nil
		}
	}
	prefix := styleXMLPrefix(nodes[0].qName)
	props, err := patchRunProperties([]byte("<"+prefix+"rPr/>"), patch)
	if err != nil {
		return nil, err
	}
	return applyXMLReplacements(fragment, []shapeXMLReplacement{{start: nodes[0].startTagEnd, end: nodes[0].startTagEnd, data: props}}), nil
}

func patchRunProperties(fragment []byte, patch document.PresentationElementStylePatch) ([]byte, error) {
	nodes, err := scanStyleXML(fragment)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("字符属性目标缺失")
	}
	root := nodes[0]
	prefix := styleXMLPrefix(root.qName)
	attributes := make(map[string]string)
	if patch.Bold != nil {
		attributes["b"] = boolDrawingML(*patch.Bold)
	}
	if patch.Italic != nil {
		attributes["i"] = boolDrawingML(*patch.Italic)
	}
	if patch.Underline != nil {
		if *patch.Underline {
			attributes["u"] = "sng"
		} else {
			attributes["u"] = "none"
		}
	}
	if patch.FontSizePT != nil {
		attributes["sz"] = strconv.FormatInt(int64(math.Round(*patch.FontSizePT*100)), 10)
	}
	if len(attributes) > 0 {
		fragment, err = patchRootXMLAttributes(fragment, attributes)
		if err != nil {
			return nil, err
		}
	}
	if patch.TextColorHex != nil {
		fill := []byte("<" + prefix + "solidFill><" + prefix + "srgbClr val=\"" + (*patch.TextColorHex)[1:] + "\"/></" + prefix + "solidFill>")
		fragment, err = upsertStyleChild(fragment, []string{"noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill"}, fill,
			[]string{"ln", "solidFill", "gradFill", "effectLst", "effectDag", "highlight", "uFill", "uFillTx", "uLn", "uLnTx", "latin", "ea", "cs", "sym", "hlinkClick", "hlinkMouseOver", "rtl", "extLst"})
		if err != nil {
			return nil, err
		}
	}
	if patch.FontFamily != nil {
		font := styleAttributeValue(*patch.FontFamily)
		for _, childName := range []string{"latin", "ea"} {
			child := []byte("<" + prefix + childName + " typeface=\"" + font + "\"/>")
			fragment, err = upsertStyleChild(fragment, []string{childName}, child,
				[]string{"ln", "solidFill", "gradFill", "effectLst", "effectDag", "highlight", "uFill", "uFillTx", "uLn", "uLnTx", "latin", "ea", "cs", "sym", "hlinkClick", "hlinkMouseOver", "rtl", "extLst"})
			if err != nil {
				return nil, err
			}
		}
	}
	return fragment, nil
}

func boolDrawingML(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func patchParagraphProperties(fragment []byte, patch document.PresentationElementStylePatch) ([]byte, error) {
	nodes, err := scanStyleXML(fragment)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("段落属性目标缺失")
	}
	prefix := styleXMLPrefix(nodes[0].qName)
	if patch.ParagraphAlignment != nil {
		alignment := map[string]string{"left": "l", "center": "ctr", "right": "r", "justify": "just"}[*patch.ParagraphAlignment]
		fragment, err = patchRootXMLAttributes(fragment, map[string]string{"algn": alignment})
		if err != nil {
			return nil, err
		}
	}
	for _, item := range []struct {
		name    string
		spacing *document.PresentationSpacing
	}{
		{name: "lnSpc", spacing: patch.LineSpacing},
		{name: "spcBef", spacing: patch.SpaceBefore},
		{name: "spcAft", spacing: patch.SpaceAfter},
	} {
		if item.spacing == nil {
			continue
		}
		child := presentationSpacingXML(prefix, item.name, *item.spacing)
		order := []string{"lnSpc", "spcBef", "spcAft", "buClr", "buSz", "buFont", "buChar", "tabLst", "defRPr", "extLst"}
		fragment, err = upsertStyleChild(fragment, []string{item.name}, child, order)
		if err != nil {
			return nil, err
		}
	}
	return fragment, nil
}

func presentationSpacingXML(prefix, name string, spacing document.PresentationSpacing) []byte {
	valueName := "spcPts"
	value := spacing.Value * 100
	if spacing.Kind == "pct" {
		valueName = "spcPct"
		value = spacing.Value * 100000
	}
	return []byte(fmt.Sprintf("<%s%s><%s%s val=\"%d\"/></%s%s>", prefix, name, prefix, valueName, int64(math.Round(value)), prefix, name))
}

func patchShapePresentationStyle(fragment []byte, patch document.PresentationElementStylePatch) ([]byte, error) {
	if patch.FontFamily == nil && patch.FontSizePT == nil && patch.Bold == nil && patch.Italic == nil && patch.Underline == nil &&
		patch.TextColorHex == nil && patch.ParagraphAlignment == nil && patch.LineSpacing == nil && patch.SpaceBefore == nil && patch.SpaceAfter == nil {

		return patchShapeFillAndLine(fragment, patch)
	}
	nodes, err := scanStyleXML(fragment)
	if err != nil {
		return nil, err
	}
	var paragraphs []int
	for index := range nodes {
		if nodes[index].local == "p" && nodes[index].parent >= 0 && nodes[nodes[index].parent].local == "txBody" {
			paragraphs = append(paragraphs, index)
		}
	}
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("对象没有可编辑的文字段落")
	}
	var replacements []shapeXMLReplacement
	for _, index := range paragraphs {
		span := nodes[index]
		paragraph, err := patchParagraphStyleFragment(fragment[span.start:span.end], patch)
		if err != nil {
			return nil, err
		}
		replacements = append(replacements, shapeXMLReplacement{start: span.start, end: span.end, data: paragraph})
	}
	fragment = applyXMLReplacements(fragment, replacements)
	return patchShapeFillAndLine(fragment, patch)
}

func patchShapeFillAndLine(fragment []byte, patch document.PresentationElementStylePatch) ([]byte, error) {
	nodes, err := scanStyleXML(fragment)
	if err != nil {
		return nil, err
	}
	spPrIndex := -1
	for index := range nodes {
		if nodes[index].local == "spPr" {
			spPrIndex = index
			break
		}
	}
	if spPrIndex < 0 {
		if patch.FillColorHex != nil || patch.BorderColor != nil || patch.BorderWidthPT != nil {
			return nil, fmt.Errorf("对象缺少形状属性")
		}
		return fragment, nil
	}
	spPrSpan := nodes[spPrIndex]
	spPr := append([]byte(nil), fragment[spPrSpan.start:spPrSpan.end]...)

	prefix := "a:"
	if patch.FillColorHex != nil {
		fill := []byte("<" + prefix + "noFill/>")
		if *patch.FillColorHex != "" {
			fill = []byte("<" + prefix + "solidFill><" + prefix + "srgbClr val=\"" + (*patch.FillColorHex)[1:] + "\"/></" + prefix + "solidFill>")
		}
		spPr, err = upsertStyleChild(spPr, []string{"noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill"}, fill,
			[]string{"xfrm", "prstGeom", "custGeom", "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill", "ln", "effectLst", "effectDag", "scene3d", "sp3d", "extLst"})
		if err != nil {
			return nil, err
		}
	}
	if patch.BorderColor != nil || patch.BorderWidthPT != nil {
		spPrNodes, scanErr := scanStyleXML(spPr)
		if scanErr != nil {
			return nil, scanErr
		}
		lineIndex := -1
		for _, child := range styleNodeChildren(spPrNodes, 0) {
			if spPrNodes[child].local == "ln" {
				lineIndex = child
				break
			}
		}
		var line []byte
		if lineIndex >= 0 {
			span := spPrNodes[lineIndex]
			line = append([]byte(nil), spPr[span.start:span.end]...)
		} else {
			linePrefix := prefix
			line = []byte("<" + linePrefix + "ln/>")
		}
		lineNodes, scanErr := scanStyleXML(line)
		if scanErr != nil {
			return nil, scanErr
		}
		lineAttributes := make(map[string]string)
		if patch.BorderWidthPT != nil {
			lineAttributes["w"] = strconv.FormatInt(pointToEMU(*patch.BorderWidthPT), 10)
		} else if lineIndex < 0 && patch.BorderColor != nil && *patch.BorderColor != "" {
			lineAttributes["w"] = strconv.FormatInt(pointToEMU(1), 10)
		}
		if len(lineAttributes) > 0 {
			line, err = patchRootXMLAttributes(line, lineAttributes)
			if err != nil {
				return nil, err
			}
		}
		if patch.BorderColor != nil {
			linePrefix := styleXMLPrefix(lineNodes[0].qName)
			fill := []byte("<" + linePrefix + "noFill/>")
			if *patch.BorderColor != "" {
				fill = []byte("<" + linePrefix + "solidFill><" + linePrefix + "srgbClr val=\"" + (*patch.BorderColor)[1:] + "\"/></" + linePrefix + "solidFill>")
			}
			line, err = upsertStyleChild(line, []string{"noFill", "solidFill", "gradFill", "pattFill"}, fill,
				[]string{"noFill", "solidFill", "gradFill", "pattFill", "prstDash", "custDash", "round", "bevel", "miter", "headEnd", "tailEnd", "extLst"})
			if err != nil {
				return nil, err
			}
		}
		if lineIndex >= 0 {
			span := spPrNodes[lineIndex]
			spPr = applyXMLReplacements(spPr, []shapeXMLReplacement{{start: span.start, end: span.end, data: line}})
		} else {
			spPr, err = upsertStyleChild(spPr, []string{"ln"}, line,
				[]string{"xfrm", "prstGeom", "custGeom", "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill", "ln", "effectLst", "effectDag", "scene3d", "sp3d", "extLst"})
			if err != nil {
				return nil, err
			}
		}
	}
	return applyXMLReplacements(fragment, []shapeXMLReplacement{{start: spPrSpan.start, end: spPrSpan.end, data: spPr}}), nil
}
