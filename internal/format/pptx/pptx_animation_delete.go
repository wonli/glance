package pptx

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

// DeletePresentationAnimationByID removes one supported generated entrance
// animation group while preserving the rest of the slide timing tree.
func (d *Document) DeletePresentationAnimationByID(ctx context.Context, slideID uint32, elementID int64, animationIndex int, expectedRevision uint64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if slideID == 0 || elementID <= 0 || elementID > int64(^uint32(0)) || animationIndex < 1 || animationIndex > maxPresentationAnimations {
		return 0, fmt.Errorf("动画目标或序号无效")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, fmt.Errorf("演示文稿已关闭")
	}
	if expectedRevision == 0 || d.revision != expectedRevision {
		return 0, fmt.Errorf("文档修订已变化：当前为 %d，调用方基于 %d", d.revision, expectedRevision)
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	var target *gopptx.Slide
	for _, slide := range slides {
		if uint32(slide.ID()) == slideID {
			target = slide
			break
		}
	}
	if target == nil {
		return 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}
	shapes, err := target.Shapes()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片对象失败: %w", err)
	}
	shapeMatches := 0
	for _, shape := range shapes {
		if uint32(shape.ID()) == uint32(elementID) {
			shapeMatches++
		}
	}
	if shapeMatches != 1 {
		if shapeMatches == 0 {
			return 0, fmt.Errorf("幻灯片 %d 中不存在元素 ID %d", slideID, elementID)
		}
		return 0, fmt.Errorf("元素 ID %d 不唯一，拒绝删除动画", elementID)
	}
	raw, _, err := target.TimingTreeRaw()
	if err != nil {
		return 0, fmt.Errorf("读取动画时序失败: %w", err)
	}
	if override := d.slideAnimationChanges[slideID]; len(override) > 0 {
		raw = append([]byte(nil), override...)
	}
	updated, removedNodes, err := removePresentationAnimationGroup(raw, uint32(elementID), animationIndex)
	if err != nil {
		return 0, err
	}
	if d.slideAnimationChanges == nil {
		d.slideAnimationChanges = make(map[uint32][]byte)
	}
	d.slideAnimationChanges[slideID] = updated
	d.dirty = true
	d.revision++
	return removedNodes, nil
}

func removePresentationAnimationGroup(raw []byte, elementID uint32, animationIndex int) ([]byte, int, error) {
	if len(raw) == 0 || len(raw) > maxPresentationTimingBytes {
		return nil, 0, fmt.Errorf("现有动画时序缺失或超过 4 MiB 上限")
	}
	if err := validateEditablePresentationTiming(raw); err != nil {
		return nil, 0, err
	}
	nodes, err := scanStyleXML(raw)
	if err != nil || len(nodes) == 0 || nodes[0].local != "timing" {
		return nil, 0, fmt.Errorf("现有动画时序结构无效")
	}
	animationNodes := make([]int, 0, 8)
	selected := -1
	for index, node := range nodes {
		if !animationKind(node.local) {
			continue
		}
		animationNodes = append(animationNodes, index)
		if len(animationNodes) == animationIndex {
			selected = index
		}
		if len(animationNodes) > maxPresentationAnimations {
			return nil, 0, fmt.Errorf("动画项超过 500 项修改上限")
		}
	}
	if selected < 0 {
		return nil, 0, fmt.Errorf("未找到动画序号 %d", animationIndex)
	}
	if nodes[selected].local != "set" && nodes[selected].local != "animEffect" {
		return nil, 0, fmt.Errorf("动画序号 %d 的类型 %q 不属于支持的简单进入动画", animationIndex, nodes[selected].local)
	}
	group := -1
	for ancestor := nodes[selected].parent; ancestor >= 0; ancestor = nodes[ancestor].parent {
		if nodes[ancestor].local != "par" {
			continue
		}
		children := styleNodeChildren(nodes, ancestor)
		if len(children) != 1 || nodes[children[0]].local != "cTn" {
			continue
		}
		attrs, attrErr := styleXMLAttributes(raw, nodes[children[0]])
		if attrErr != nil {
			return nil, 0, attrErr
		}
		if attrs["nodeType"] == "clickEffect" {
			group = ancestor
			break
		}
	}
	if group < 0 {
		return nil, 0, fmt.Errorf("动画序号 %d 不属于可安全删除的点击进入动画组", animationIndex)
	}
	eventWrapper, err := presentationAnimationEventWrapper(raw, nodes, group)
	if err != nil {
		return nil, 0, err
	}
	groupCtn := styleNodeChildren(nodes, group)[0]
	groupAttrs, err := styleXMLAttributes(raw, nodes[groupCtn])
	if err != nil || groupAttrs["presetClass"] != "entr" {
		return nil, 0, fmt.Errorf("动画组不是可识别的进入效果，拒绝删除")
	}
	groupLists := 0
	conditionLists := 0
	childList := -1
	for _, child := range styleNodeChildren(nodes, groupCtn) {
		switch nodes[child].local {
		case "childTnLst":
			groupLists++
			childList = child
		case "stCondLst":
			conditionLists++
			conditions := styleNodeChildren(nodes, child)
			if len(conditions) != 1 || nodes[conditions[0]].local != "cond" {
				return nil, 0, fmt.Errorf("动画组触发条件不受支持，拒绝删除")
			}
			conditionAttrs, attrErr := styleXMLAttributes(raw, nodes[conditions[0]])
			if attrErr != nil || conditionAttrs["delay"] != "0" {
				return nil, 0, fmt.Errorf("动画组触发条件不受支持，拒绝删除")
			}
		default:
			return nil, 0, fmt.Errorf("动画组包含未知节点 %q，拒绝删除", nodes[child].local)
		}
	}
	if groupLists != 1 || conditionLists != 1 || childList < 0 {
		return nil, 0, fmt.Errorf("动画组子时序结构不唯一，拒绝删除")
	}
	groupAnimations := styleNodeChildren(nodes, childList)
	if len(groupAnimations) < 1 || len(groupAnimations) > 2 {
		return nil, 0, fmt.Errorf("动画组包含不支持的子动画数量，拒绝删除")
	}
	descendantAnimations := 0
	for index, node := range nodes {
		if animationKind(node.local) && styleXMLDescendantOf(nodes, index, group) {
			descendantAnimations++
		}
		if node.local != "spTgt" || !styleXMLDescendantOf(nodes, index, group) {
			continue
		}
		inAnimationNode := false
		for _, animationNode := range groupAnimations {
			if styleXMLDescendantOf(nodes, index, animationNode) {
				inAnimationNode = true
				break
			}
		}
		if !inAnimationNode {
			return nil, 0, fmt.Errorf("动画组包含未绑定动画节点的形状目标，拒绝删除")
		}
	}
	if descendantAnimations != len(groupAnimations) {
		return nil, 0, fmt.Errorf("动画组包含嵌套动画节点，拒绝删除")
	}
	setCount, effectCount := 0, 0
	for _, animationNode := range groupAnimations {
		switch nodes[animationNode].local {
		case "set":
			setCount++
			if !presentationVisibilitySet(raw, nodes[animationNode]) {
				return nil, 0, fmt.Errorf("动画组包含非 Glance 可见性节点，拒绝删除")
			}
		case "animEffect":
			effectCount++
			effectAttrs, attrErr := styleXMLAttributes(raw, nodes[animationNode])
			if attrErr != nil || effectAttrs["transition"] != "in" || (effectAttrs["filter"] != "fade" && effectAttrs["filter"] != "slide(fromLeft)") {
				return nil, 0, fmt.Errorf("动画组包含不支持的进入效果，拒绝删除")
			}
		default:
			return nil, 0, fmt.Errorf("动画组包含类型 %q，拒绝删除", nodes[animationNode].local)
		}
		targetID, targetErr := presentationAnimationNodeTarget(raw, nodes, animationNode)
		if targetErr != nil || targetID != elementID {
			return nil, 0, fmt.Errorf("动画组目标与元素 ID %d 不一致，拒绝删除", elementID)
		}
	}
	if setCount != 1 || (effectCount != 0 && effectCount != 1) || len(groupAnimations) != setCount+effectCount {
		return nil, 0, fmt.Errorf("动画组节点组合不受支持，拒绝删除")
	}
	if effectCount == 0 && groupAttrs["presetID"] != "1" {
		return nil, 0, fmt.Errorf("即时进入动画类型不受支持，拒绝删除")
	}
	if effectCount == 1 {
		effectNode := nodes[groupAnimations[0]]
		if effectNode.local != "animEffect" {
			effectNode = nodes[groupAnimations[1]]
		}
		attrs, attrErr := styleXMLAttributes(raw, effectNode)
		preset := map[string]string{"fade": "10", "slide(fromLeft)": "2"}[attrs["filter"]]
		if attrErr != nil || preset == "" || groupAttrs["presetID"] != preset {
			return nil, 0, fmt.Errorf("动画效果与预设 ID 不匹配，拒绝删除")
		}
	}
	selectedInGroup := false
	for _, animationNode := range groupAnimations {
		if animationNode == selected {
			selectedInGroup = true
		}
	}
	if !selectedInGroup {
		return nil, 0, fmt.Errorf("动画序号 %d 不属于已验证动画组", animationIndex)
	}
	span := nodes[eventWrapper]
	updated := applyXMLReplacements(raw, []shapeXMLReplacement{{start: span.start, end: span.end}})
	if err := validateEditablePresentationTiming(updated); err != nil {
		return nil, 0, fmt.Errorf("删除后的动画时序校验失败: %w", err)
	}
	return updated, len(groupAnimations), nil
}

func presentationAnimationEventWrapper(raw []byte, nodes []styleXMLNode, group int) (int, error) {
	innerList := nodes[group].parent
	if innerList < 0 || nodes[innerList].local != "childTnLst" || !presentationAnimationChildren(nodes, innerList, "par") {
		return -1, fmt.Errorf("动画组不在独立点击时序中，拒绝删除")
	}
	// Older Glance versions put a simple clickEffect directly under tmRoot.
	// Keep those groups editable while new animations use Office's mainSeq form.
	legacyRoot := nodes[innerList].parent
	if legacyRoot >= 0 && nodes[legacyRoot].local == "cTn" {
		attrs, err := styleXMLAttributes(raw, nodes[legacyRoot])
		if err != nil {
			return -1, err
		}
		if attrs["nodeType"] == "tmRoot" {
			legacyRootPar := nodes[legacyRoot].parent
			if legacyRootPar < 0 || nodes[legacyRootPar].local != "par" || !presentationAnimationChildren(nodes, legacyRootPar, "cTn") {
				return -1, fmt.Errorf("旧版动画组缺少根时序包装，拒绝删除")
			}
			return group, nil
		}
	}
	innerCtn := nodes[innerList].parent
	if innerCtn < 0 || nodes[innerCtn].local != "cTn" || !presentationAnimationChildren(nodes, innerCtn, "stCondLst", "childTnLst") {
		return -1, fmt.Errorf("动画组缺少内部点击时序包装，拒绝删除")
	}
	if !presentationAnimationWrapperCtn(raw, nodes[innerCtn]) || !presentationAnimationWrapperCondition(raw, nodes, styleNodeChildren(nodes, innerCtn)[0], "0") {
		return -1, fmt.Errorf("动画组内部点击条件不受支持，拒绝删除")
	}
	innerPar := nodes[innerCtn].parent
	if innerPar < 0 || nodes[innerPar].local != "par" || !presentationAnimationChildren(nodes, innerPar, "cTn") {
		return -1, fmt.Errorf("动画组内部并行节点不受支持，拒绝删除")
	}
	outerList := nodes[innerPar].parent
	if outerList < 0 || nodes[outerList].local != "childTnLst" {
		return -1, fmt.Errorf("动画组缺少外部点击时序列表，拒绝删除")
	}
	outerCtn := nodes[outerList].parent
	if outerCtn < 0 || nodes[outerCtn].local != "cTn" || !presentationAnimationChildren(nodes, outerCtn, "stCondLst", "childTnLst") {
		return -1, fmt.Errorf("动画组缺少外部点击时序包装，拒绝删除")
	}
	if !presentationAnimationWrapperCtn(raw, nodes[outerCtn]) || !presentationAnimationWrapperCondition(raw, nodes, styleNodeChildren(nodes, outerCtn)[0], "indefinite") {
		return -1, fmt.Errorf("动画组外部点击条件不受支持，拒绝删除")
	}
	eventWrapper := nodes[outerCtn].parent
	if eventWrapper < 0 || nodes[eventWrapper].local != "par" || !presentationAnimationChildren(nodes, eventWrapper, "cTn") {
		return -1, fmt.Errorf("动画组点击包装不受支持，拒绝删除")
	}
	mainList := nodes[eventWrapper].parent
	if mainList < 0 || nodes[mainList].local != "childTnLst" {
		return -1, fmt.Errorf("动画组不在主动画序列列表中，拒绝删除")
	}
	mainSequence := nodes[mainList].parent
	if mainSequence < 0 || nodes[mainSequence].local != "cTn" {
		return -1, fmt.Errorf("动画组缺少主动画序列，拒绝删除")
	}
	mainAttrs, err := styleXMLAttributes(raw, nodes[mainSequence])
	if err != nil || mainAttrs["nodeType"] != "mainSeq" || mainAttrs["dur"] != "indefinite" {
		return -1, fmt.Errorf("动画组不属于主动画序列，拒绝删除")
	}
	sequence := nodes[mainSequence].parent
	if sequence < 0 || nodes[sequence].local != "seq" || !presentationAnimationChildren(nodes, sequence, "cTn", "prevCondLst", "nextCondLst") {
		return -1, fmt.Errorf("主动画序列结构不受支持，拒绝删除")
	}
	rootList := nodes[sequence].parent
	if rootList < 0 || nodes[rootList].local != "childTnLst" {
		return -1, fmt.Errorf("主动画序列不在根时序列表中，拒绝删除")
	}
	rootCtn := nodes[rootList].parent
	if rootCtn < 0 || nodes[rootCtn].local != "cTn" {
		return -1, fmt.Errorf("主动画序列缺少根时序节点，拒绝删除")
	}
	rootAttrs, err := styleXMLAttributes(raw, nodes[rootCtn])
	if err != nil || rootAttrs["nodeType"] != "tmRoot" {
		return -1, fmt.Errorf("动画组不属于根时序节点，拒绝删除")
	}
	return eventWrapper, nil
}

func presentationAnimationWrapperCtn(raw []byte, node styleXMLNode) bool {
	attrs, err := styleXMLAttributes(raw, node)
	if err != nil || attrs["fill"] != "hold" || len(attrs) != 2 {
		return false
	}
	_, err = strconv.ParseUint(attrs["id"], 10, 32)
	return err == nil
}

func presentationAnimationWrapperCondition(raw []byte, nodes []styleXMLNode, list int, wantDelay string) bool {
	if !presentationAnimationChildren(nodes, list, "cond") {
		return false
	}
	condition := styleNodeChildren(nodes, list)[0]
	attrs, err := styleXMLAttributes(raw, nodes[condition])
	return err == nil && len(attrs) == 1 && attrs["delay"] == wantDelay && nodes[condition].selfClose
}

func presentationAnimationNodeTarget(raw []byte, nodes []styleXMLNode, animationNode int) (uint32, error) {
	var target uint32
	targets := 0
	for index, node := range nodes {
		if node.local != "spTgt" || !styleXMLDescendantOf(nodes, index, animationNode) {
			continue
		}
		attrs, err := styleXMLAttributes(raw, node)
		if err != nil {
			return 0, err
		}
		id, err := strconv.ParseUint(attrs["spid"], 10, 32)
		if err != nil || id == 0 {
			return 0, fmt.Errorf("动画目标 ID 无效")
		}
		targets++
		target = uint32(id)
	}
	if targets != 1 {
		return 0, fmt.Errorf("动画必须只有一个明确形状目标")
	}
	return target, nil
}

func presentationVisibilitySet(raw []byte, node styleXMLNode) bool {
	if node.start < 0 || node.end <= node.start || node.end > len(raw) {
		return false
	}
	fragment := raw[node.start:node.end]
	nodes, err := scanStyleXML(fragment)
	if err != nil || len(nodes) != 9 || nodes[0].local != "set" {
		return false
	}
	if !presentationAnimationChildren(nodes, 0, "cBhvr", "to") {
		return false
	}
	behavior := styleNodeChildren(nodes, 0)[0]
	if !presentationAnimationChildren(nodes, behavior, "cTn", "tgtEl", "attrNameLst") {
		return false
	}
	behaviorChildren := styleNodeChildren(nodes, behavior)
	durationAttrs, err := styleXMLAttributes(fragment, nodes[behaviorChildren[0]])
	if err != nil || durationAttrs["dur"] != "1" || durationAttrs["fill"] != "hold" || !nodes[behaviorChildren[0]].selfClose {
		return false
	}
	if !presentationAnimationChildren(nodes, behaviorChildren[1], "spTgt") || !presentationAnimationChildren(nodes, behaviorChildren[2], "attrName") {
		return false
	}
	to := styleNodeChildren(nodes, 0)[1]
	if !presentationAnimationChildren(nodes, to, "strVal") {
		return false
	}
	strVal := styleNodeChildren(nodes, to)[0]
	valueAttrs, err := styleXMLAttributes(fragment, nodes[strVal])
	if err != nil || valueAttrs["val"] != "visible" {
		return false
	}
	attrName := styleNodeChildren(nodes, behaviorChildren[2])[0]
	text, ok := presentationXMLText(fragment[nodes[attrName].start:nodes[attrName].end])
	return ok && strings.TrimSpace(text) == "style.visibility"
}

func presentationAnimationChildren(nodes []styleXMLNode, parent int, want ...string) bool {
	children := styleNodeChildren(nodes, parent)
	if len(children) != len(want) {
		return false
	}
	for index, child := range children {
		if nodes[child].local != want[index] {
			return false
		}
	}
	return true
}

func presentationXMLText(fragment []byte) (string, bool) {
	decoder := xml.NewDecoder(bytes.NewReader(fragment))
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		switch value := token.(type) {
		case xml.CharData:
			text.Write(value)
		}
	}
	return text.String(), true
}
