package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"strconv"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

const presentationGroupMinElements = 2

// GroupElementsByID creates a DrawingML group around adjacent top-level slide
// elements. Child geometry and z-order remain unchanged because the group
// transform maps the slide coordinate space to itself.
func (d *Document) GroupElementsByID(ctx context.Context, slideID uint32, elementIDs []uint32) (uint32, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if slideID == 0 || len(elementIDs) < presentationGroupMinElements || len(elementIDs) > 100 {
		return 0, false, fmt.Errorf("组合操作需要 2 到 100 个明确元素 ID")
	}
	selected := make(map[uint32]bool, len(elementIDs))
	for _, id := range elementIDs {
		if id == 0 || selected[id] {
			return 0, false, fmt.Errorf("组合元素 ID 缺失或重复")
		}
		selected[id] = true
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, false, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	slideIndex := -1
	var slide *gopptx.Slide
	for index, candidate := range slides {
		if uint32(candidate.ID()) == slideID {
			slideIndex, slide = index, candidate
			break
		}
	}
	if slideIndex < 0 || slide == nil {
		return 0, false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}
	if d.presentationComments != nil && d.presentationComments.LoadErr != nil {
		return 0, false, fmt.Errorf("演示文稿包含未能读取的批注状态，拒绝重载文档")
	}
	shapes, err := slide.Shapes()
	if err != nil {
		return 0, false, fmt.Errorf("读取幻灯片元素失败: %w", err)
	}
	projected := make(map[uint32]document.PresentationElement, len(shapes))
	for _, shape := range shapes {
		if _, isGroup := shape.(*gopptx.GroupShape); isGroup {
			continue
		}
		element := d.projectShape(shape, slideIndex)
		if element.ID > 0 && element.ID <= math.MaxUint32 {
			projected[uint32(element.ID)] = element
		}
	}
	for id := range selected {
		element, ok := projected[id]
		if !ok {
			return 0, false, fmt.Errorf("元素 %d 不是该页可组合的顶层对象", id)
		}
		if element.GroupID != 0 {
			return 0, false, fmt.Errorf("已组合对象暂不支持再次组合")
		}
		if !finitePresentationRect(element.Bounds) || element.Bounds.W < 0 || element.Bounds.H < 0 {
			return 0, false, fmt.Errorf("元素 %d 的几何信息不完整，拒绝组合", id)
		}
	}

	base, err := d.snapshotBytesLocked()
	if err != nil {
		return 0, false, err
	}
	reader, err := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		return 0, false, fmt.Errorf("读取 PPTX 暂存包失败: %w", err)
	}
	pkg := newPPTXPackage(reader)
	if slideIndex >= len(pkg.slideParts) {
		return 0, false, fmt.Errorf("无法将幻灯片 ID 映射到 PPTX 部件")
	}
	partName := pkg.slideParts[slideIndex]
	part := pkg.parts[partName]
	if part == nil {
		return 0, false, fmt.Errorf("幻灯片部件缺失: %s", partName)
	}
	source, err := readOPCPart(part)
	if err != nil {
		return 0, false, fmt.Errorf("读取幻灯片 XML 失败: %w", err)
	}
	bounds := presentationGroupBounds(selected, projected)
	groupID, updated, err := wrapPresentationShapeGroup(source, selected, bounds)
	if err != nil {
		return 0, false, err
	}
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	updatedPackage, err := replacePPTXPackageParts(ctx, base, map[string][]byte{partName: updated})
	if err != nil {
		return 0, false, err
	}
	if err := d.adoptPPTXSnapshotLocked(updatedPackage); err != nil {
		return 0, false, err
	}
	return groupID, true, nil
}

func finitePresentationRect(rect document.PresentationRect) bool {
	return !math.IsNaN(rect.X) && !math.IsNaN(rect.Y) && !math.IsNaN(rect.W) && !math.IsNaN(rect.H) &&
		!math.IsInf(rect.X, 0) && !math.IsInf(rect.Y, 0) && !math.IsInf(rect.W, 0) && !math.IsInf(rect.H, 0)
}

func presentationGroupBounds(selected map[uint32]bool, projected map[uint32]document.PresentationElement) document.PresentationRect {
	var result document.PresentationRect
	first := true
	for id := range selected {
		bounds := projected[id].Bounds
		if first {
			result, first = bounds, false
			continue
		}
		left := math.Min(result.X, bounds.X)
		top := math.Min(result.Y, bounds.Y)
		right := math.Max(result.X+result.W, bounds.X+bounds.W)
		bottom := math.Max(result.Y+result.H, bounds.Y+bounds.H)
		result = document.PresentationRect{X: left, Y: top, W: right - left, H: bottom - top}
	}
	return result
}

func wrapPresentationShapeGroup(data []byte, selected map[uint32]bool, bounds document.PresentationRect) (uint32, []byte, error) {
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, nil, fmt.Errorf("组合对象范围无效: %w", err)
	}
	nodes, err := scanStyleXML(data)
	if err != nil {
		return 0, nil, err
	}
	treeIndex := firstStyleNode(nodes, "spTree")
	if treeIndex < 0 || nodes[treeIndex].selfClose {
		return 0, nil, fmt.Errorf("幻灯片不含可编辑的 p:spTree")
	}
	tree := nodes[treeIndex]
	visualNodes := make([]int, 0)
	maxID := uint32(0)
	seenIDs := make(map[uint32]bool)
	for index := range nodes {
		node := nodes[index]
		if node.local == "cNvPr" {
			id, err := parseShapeIDAttribute(data[node.start:node.end])
			if err != nil {
				return 0, nil, err
			}
			if id > maxID {
				maxID = id
			}
		}
		if node.parent != treeIndex || !isPresentationVisualShape(node.local) {
			continue
		}
		id, err := presentationShapeIDFromFragment(data[node.start:node.end])
		if err != nil {
			return 0, nil, err
		}
		if id == 0 || seenIDs[id] {
			return 0, nil, fmt.Errorf("幻灯片存在缺失或重复的形状 ID，拒绝组合")
		}
		seenIDs[id] = true
		visualNodes = append(visualNodes, index)
	}
	if maxID == ^uint32(0) {
		return 0, nil, fmt.Errorf("幻灯片形状 ID 已耗尽，无法创建组合")
	}
	groupID := maxID + 1
	startIndex, endIndex := -1, -1
	selectedCount := 0
	for position, nodeIndex := range visualNodes {
		id, err := presentationShapeIDFromFragment(data[nodes[nodeIndex].start:nodes[nodeIndex].end])
		if err != nil {
			return 0, nil, err
		}
		if !selected[id] {
			continue
		}
		if !hasExplicitPresentationTransform(data[nodes[nodeIndex].start:nodes[nodeIndex].end], nodes[nodeIndex].local) {
			return 0, nil, fmt.Errorf("元素 %d 使用继承几何或未知变换，拒绝组合", id)
		}
		if startIndex < 0 {
			startIndex = position
		}
		endIndex = position
		selectedCount++
	}
	if selectedCount != len(selected) {
		return 0, nil, fmt.Errorf("一个或多个元素不在幻灯片顶层")
	}
	if selectedCount < presentationGroupMinElements || endIndex-startIndex+1 != selectedCount {
		return 0, nil, fmt.Errorf("为保持对象前后层级不变，只能组合相邻的幻灯片对象")
	}
	firstNode, lastNode := nodes[visualNodes[startIndex]], nodes[visualNodes[endIndex]]
	for _, nodeIndex := range styleNodeChildren(nodes, treeIndex) {
		if nodeIndex <= visualNodes[startIndex] || nodeIndex >= visualNodes[endIndex] {
			continue
		}
		if !isPresentationVisualShape(nodes[nodeIndex].local) {
			return 0, nil, fmt.Errorf("所选对象之间含有未知 spTree 结构，拒绝组合")
		}
	}
	childXML := string(data[firstNode.start:lastNode.end])
	groupXML := presentationGroupXML(styleXMLPrefix(tree.qName), groupID, bounds, childXML)
	updated := applyXMLReplacements(data, []shapeXMLReplacement{{start: firstNode.start, end: lastNode.end, data: []byte(groupXML)}})
	return groupID, updated, nil
}

func isPresentationVisualShape(local string) bool {
	switch local {
	case "sp", "pic", "graphicFrame", "cxnSp", "grpSp":
		return true
	default:
		return false
	}
}

func presentationShapeIDFromFragment(fragment []byte) (uint32, error) {
	var node pptxXMLNode
	if err := xml.Unmarshal(fragment, &node); err != nil {
		return 0, fmt.Errorf("解析幻灯片形状失败: %w", err)
	}
	id := shapeIDFromXML(node)
	if id == 0 {
		return 0, fmt.Errorf("幻灯片形状缺少稳定 ID")
	}
	return id, nil
}

func parseShapeIDAttribute(startTag []byte) (uint32, error) {
	var node pptxXMLNode
	if err := xml.Unmarshal(startTag, &node); err == nil {
		if value := node.attr("id"); value != "" {
			parsed, err := strconv.ParseUint(value, 10, 32)
			if err == nil {
				return uint32(parsed), nil
			}
		}
	}
	return 0, fmt.Errorf("幻灯片 cNvPr 包含无效形状 ID")
}

func hasExplicitPresentationTransform(fragment []byte, shapeKind string) bool {
	var root pptxXMLNode
	if xml.Unmarshal(fragment, &root) != nil {
		return false
	}
	var properties *pptxXMLNode
	switch shapeKind {
	case "sp", "pic", "cxnSp":
		properties = root.child("spPr")
	case "grpSp":
		properties = root.child("grpSpPr")
	case "graphicFrame":
		properties = &root
	}
	return properties != nil && properties.child("xfrm") != nil
}

func presentationGroupXML(prefix string, groupID uint32, bounds document.PresentationRect, children string) string {
	group := prefix + "grpSp"
	nv := prefix + "nvGrpSpPr"
	name := fmt.Sprintf("Glance 组合 %d", groupID)
	x, y := pointToEMU(bounds.X), pointToEMU(bounds.Y)
	w, h := pointToEMU(bounds.W), pointToEMU(bounds.H)
	return fmt.Sprintf("<%s><%s><%scNvPr id=\"%d\" name=\"%s\"/><%scNvGrpSpPr/><%snvPr/></%s><%sgrpSpPr><a:xfrm xmlns:a=\"%s\"><a:off x=\"%d\" y=\"%d\"/><a:ext cx=\"%d\" cy=\"%d\"/><a:chOff x=\"%d\" y=\"%d\"/><a:chExt cx=\"%d\" cy=\"%d\"/></a:xfrm></%sgrpSpPr>%s</%s>",
		group, nv, prefix, groupID, name, prefix, prefix, nv, prefix, drawingMLNamespace,
		x, y, w, h, x, y, w, h, prefix, children, group)
}
