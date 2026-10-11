package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

const (
	maxPresentationTimingBytes   = 4 << 20
	maxPresentationTimingTokens  = 100000
	maxPresentationTimingDepth   = 128
	maxPresentationAnimations    = 500
	maxPresentationAnimationText = 256
)

// PresentationAnimationsByID reads the animation timing metadata for one
// stable slide ID. It never exposes library types and never modifies the file.
func (d *Document) PresentationAnimationsByID(ctx context.Context, slideID uint32) (document.PresentationAnimationSummary, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationAnimationSummary{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.pres == nil {
		return document.PresentationAnimationSummary{}, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return document.PresentationAnimationSummary{}, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	var target = -1
	for index, slide := range slides {
		if uint32(slide.ID()) == slideID {
			target = index
			break
		}
	}
	if target < 0 {
		return document.PresentationAnimationSummary{}, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}
	slide := slides[target]
	result := document.PresentationAnimationSummary{
		SlideID: slideID, Revision: d.revision,
		Animations:  make([]document.PresentationAnimation, 0),
		Diagnostics: make([]string, 0),
	}
	raw, diagnostics, err := slide.TimingTreeRaw()
	if err != nil {
		return document.PresentationAnimationSummary{}, fmt.Errorf("读取动画时序失败: %w", err)
	}
	if override := d.slideAnimationChanges[slideID]; len(override) > 0 {
		raw = append([]byte(nil), override...)
		diagnostics = nil
	}
	for _, diagnostic := range diagnostics {
		if !appendAnimationDiagnostic(&result, diagnostic.Message) {
			result.Truncated = true
			break
		}
	}
	result.HasTiming = raw != nil
	if !result.HasTiming || len(raw) == 0 {
		return result, nil
	}
	if len(raw) > maxPresentationTimingBytes {
		result.Truncated = true
		appendAnimationDiagnostic(&result, "动画时序 XML 超过 4 MiB 上限，未解析")
		return result, nil
	}
	if err := parsePresentationAnimations(ctx, raw, &result); err != nil {
		return document.PresentationAnimationSummary{}, err
	}
	return result, nil
}

// SetPresentationElementAnimationByID appends a supported entrance effect to
// one top-level shape. The generated timing XML is kept as an overlay so the
// original package remains untouched until the normal atomic save path.
func (d *Document) SetPresentationElementAnimationByID(ctx context.Context, slideID uint32, elementID int64, effect string, durationMS int, expectedRevision uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if slideID == 0 || elementID <= 0 || elementID > int64(^uint32(0)) || durationMS < 1 || durationMS > 60000 {
		return false, fmt.Errorf("动画目标或持续时间无效")
	}
	switch effect {
	case "appear", "fade", "fly_in":
	default:
		return false, fmt.Errorf("不支持的进入动画类型 %q", effect)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	if expectedRevision == 0 || d.revision != expectedRevision {
		return false, fmt.Errorf("文档修订已变化：当前为 %d，调用方基于 %d", d.revision, expectedRevision)
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	var target *gopptx.Slide
	for _, slide := range slides {
		if uint32(slide.ID()) == slideID {
			target = slide
			break
		}
	}
	if target == nil {
		return false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}
	shapes, err := target.Shapes()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片对象失败: %w", err)
	}
	shapeMatches := 0
	for _, shape := range shapes {
		if int64(shape.ID()) != elementID {
			continue
		}
		shapeMatches++
		if shape.Kind() != gopptx.ShapeTextBox && shape.Kind() != gopptx.ShapeAutoShape && shape.Kind() != gopptx.ShapePicture && shape.Kind() != gopptx.ShapeConnector {
			return false, fmt.Errorf("元素 ID %d 的类型 %q 不支持动画", elementID, shape.Kind())
		}
	}
	if shapeMatches != 1 {
		if shapeMatches == 0 {
			return false, fmt.Errorf("幻灯片 %d 中不存在元素 ID %d", slideID, elementID)
		}
		return false, fmt.Errorf("元素 ID %d 不唯一，拒绝添加动画", elementID)
	}
	raw, _, err := target.TimingTreeRaw()
	if err != nil {
		return false, fmt.Errorf("读取动画时序失败: %w", err)
	}
	if override := d.slideAnimationChanges[slideID]; len(override) > 0 {
		raw = append([]byte(nil), override...)
	}
	updated, err := appendPresentationEntranceAnimation(raw, uint32(elementID), effect, durationMS)
	if err != nil {
		return false, err
	}
	if d.slideAnimationChanges == nil {
		d.slideAnimationChanges = make(map[uint32][]byte)
	}
	d.slideAnimationChanges[slideID] = updated
	d.dirty = true
	d.revision++
	return true, nil
}

// SetPresentationAnimationDurationByID changes one explicitly identified
// animation node while preserving the remainder of its timing tree.
func (d *Document) SetPresentationAnimationDurationByID(ctx context.Context, slideID uint32, elementID int64, animationIndex int, durationMS int, expectedRevision uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if slideID == 0 || elementID <= 0 || elementID > int64(^uint32(0)) || animationIndex < 1 || animationIndex > maxPresentationAnimations || durationMS < 1 || durationMS > 60000 {
		return false, fmt.Errorf("动画目标、序号或持续时间无效")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	if expectedRevision == 0 || d.revision != expectedRevision {
		return false, fmt.Errorf("文档修订已变化：当前为 %d，调用方基于 %d", d.revision, expectedRevision)
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	var target *gopptx.Slide
	for _, slide := range slides {
		if uint32(slide.ID()) == slideID {
			target = slide
			break
		}
	}
	if target == nil {
		return false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}
	shapes, err := target.Shapes()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片对象失败: %w", err)
	}
	matches := 0
	for _, shape := range shapes {
		if int64(shape.ID()) == elementID {
			matches++
		}
	}
	if matches != 1 {
		if matches == 0 {
			return false, fmt.Errorf("幻灯片 %d 中不存在元素 ID %d", slideID, elementID)
		}
		return false, fmt.Errorf("元素 ID %d 不唯一，拒绝设置动画时长", elementID)
	}
	raw, _, err := target.TimingTreeRaw()
	if err != nil {
		return false, fmt.Errorf("读取动画时序失败: %w", err)
	}
	if override := d.slideAnimationChanges[slideID]; len(override) > 0 {
		raw = append([]byte(nil), override...)
	}
	updated, changed, err := patchPresentationAnimationDuration(raw, uint32(elementID), animationIndex, durationMS)
	if err != nil || !changed {
		return false, err
	}
	if d.slideAnimationChanges == nil {
		d.slideAnimationChanges = make(map[uint32][]byte)
	}
	d.slideAnimationChanges[slideID] = updated
	d.dirty = true
	d.revision++
	return true, nil
}

func patchPresentationAnimationDuration(raw []byte, elementID uint32, animationIndex, durationMS int) ([]byte, bool, error) {
	if len(raw) == 0 || len(raw) > maxPresentationTimingBytes {
		return nil, false, fmt.Errorf("现有动画时序缺失或超过 4 MiB 上限")
	}
	if err := validateEditablePresentationTiming(raw); err != nil {
		return nil, false, err
	}
	nodes, err := scanStyleXML(raw)
	if err != nil || len(nodes) == 0 || nodes[0].local != "timing" {
		return nil, false, fmt.Errorf("现有动画时序结构无效")
	}
	currentIndex := 0
	selected := -1
	for index, node := range nodes {
		if !animationKind(node.local) {
			continue
		}
		currentIndex++
		if currentIndex == animationIndex {
			selected = index
			break
		}
	}
	if selected < 0 {
		return nil, false, fmt.Errorf("未找到动画序号 %d", animationIndex)
	}
	if nodes[selected].local != "animEffect" && nodes[selected].local != "animMotion" {
		return nil, false, fmt.Errorf("动画序号 %d 的类型 %q 没有可编辑的时长", animationIndex, nodes[selected].local)
	}
	foundTarget := false
	durationNode := -1
	for index, node := range nodes {
		if index == selected || !styleXMLDescendantOf(nodes, index, selected) {
			continue
		}
		if node.local == "spTgt" {
			attrs, attrErr := styleXMLAttributes(raw, node)
			if attrErr != nil {
				return nil, false, attrErr
			}
			id, parseErr := strconv.ParseUint(attrs["spid"], 10, 32)
			if parseErr == nil && uint32(id) == elementID {
				foundTarget = true
			}
		}
		if durationNode < 0 && node.local == "cTn" {
			durationNode = index
		}
	}
	if !foundTarget {
		return nil, false, fmt.Errorf("动画序号 %d 不属于元素 ID %d", animationIndex, elementID)
	}
	if durationNode < 0 {
		return nil, false, fmt.Errorf("动画序号 %d 缺少可编辑时长节点", animationIndex)
	}
	attrs, err := styleXMLAttributes(raw, nodes[durationNode])
	if err != nil {
		return nil, false, err
	}
	newDuration := strconv.Itoa(durationMS)
	if attrs["dur"] == newDuration {
		return raw, false, nil
	}
	span := nodes[durationNode]
	patchedNode, err := patchRootXMLAttributes(raw[span.start:span.end], map[string]string{"dur": newDuration})
	if err != nil {
		return nil, false, err
	}
	return applyXMLReplacements(raw, []shapeXMLReplacement{{start: span.start, end: span.end, data: patchedNode}}), true, nil
}

func validateEditablePresentationTiming(raw []byte) error {
	if len(raw) == 0 || len(raw) > maxPresentationTimingBytes {
		return fmt.Errorf("动画时序长度无效，允许范围为 1–4 MiB")
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.Strict = true
	depth := 0
	tokens := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("动画时序 XML 无效: %w", err)
		}
		tokens++
		if tokens > maxPresentationTimingTokens {
			return fmt.Errorf("动画时序节点超过修改上限")
		}
		switch token.(type) {
		case xml.StartElement:
			depth++
			if depth > maxPresentationTimingDepth {
				return fmt.Errorf("动画时序嵌套超过修改上限")
			}
		case xml.EndElement:
			depth--
		}
	}
}

func styleXMLDescendantOf(nodes []styleXMLNode, child, ancestor int) bool {
	for child >= 0 && child < len(nodes) {
		child = nodes[child].parent
		if child == ancestor {
			return true
		}
	}
	return false
}

func appendPresentationEntranceAnimation(raw []byte, elementID uint32, effect string, durationMS int) ([]byte, error) {
	if len(raw) == 0 {
		animationID, setID, effectID := uint32(5), uint32(6), uint32(7)
		fragment := presentationEntranceAnimationFragment("p:", animationID, setID, effectID, elementID, effect, durationMS)
		event := presentationClickEventFragment("p:", 3, 4, fragment)
		sequence := presentationMainSequenceFragment("p:", 2, event)
		result := []byte(`<p:timing><p:tnLst><p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst>`)
		result = append(result, sequence...)
		result = append(result, []byte(`</p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`)...)
		return result, nil
	}
	if len(raw) > maxPresentationTimingBytes {
		return nil, fmt.Errorf("现有动画时序超过 4 MiB 上限，拒绝修改")
	}
	if err := validateEditablePresentationTiming(raw); err != nil {
		return nil, err
	}
	nodes, err := scanStyleXML(raw)
	if err != nil || len(nodes) == 0 || nodes[0].local != "timing" {
		return nil, fmt.Errorf("现有动画时序结构无效，拒绝修改")
	}
	prefix := styleXMLPrefix(nodes[0].qName)
	if prefix == "" {
		return nil, fmt.Errorf("动画时序缺少 PresentationML 命名空间前缀，拒绝修改")
	}
	maxID := uint32(0)
	rootIndex := -1
	for index, node := range nodes {
		if node.local != "cTn" {
			continue
		}
		attrs, attrErr := styleXMLAttributes(raw, node)
		if attrErr != nil {
			return nil, attrErr
		}
		id, parseErr := strconv.ParseUint(attrs["id"], 10, 32)
		if parseErr != nil {
			return nil, fmt.Errorf("现有动画时序含有无效 cTn ID，拒绝修改")
		}
		if uint32(id) > maxID {
			maxID = uint32(id)
		}
		if attrs["nodeType"] == "tmRoot" {
			if rootIndex >= 0 {
				return nil, fmt.Errorf("现有动画包含多个根时序节点，拒绝修改")
			}
			rootIndex = index
		}
	}
	if rootIndex < 0 {
		return nil, fmt.Errorf("现有动画时序缺少可安全扩展的根节点或 ID 空间，拒绝修改")
	}
	root := nodes[rootIndex]
	if root.selfClose {
		return nil, fmt.Errorf("现有动画根节点为空元素，拒绝修改")
	}
	childList := -1
	for _, child := range styleNodeChildren(nodes, rootIndex) {
		if nodes[child].local == "childTnLst" {
			if childList >= 0 {
				return nil, fmt.Errorf("动画根节点包含多个子时序列表，拒绝修改")
			}
			childList = child
		}
	}
	mainSequenceList := -1
	if childList >= 0 && !nodes[childList].selfClose {
		mainSequenceList, err = presentationMainSequenceChildList(raw, nodes, childList)
		if err != nil {
			return nil, err
		}
	}
	newMainSequence := mainSequenceList < 0
	neededIDs := uint64(4) // event wrapper, click wrapper, group, and visibility set
	if effect != "appear" {
		neededIDs++
	}
	if newMainSequence {
		neededIDs++
	}
	if uint64(maxID)+neededIDs > uint64(^uint32(0)) {
		return nil, fmt.Errorf("现有动画时序 ID 空间不足，拒绝修改")
	}
	nextID := maxID + 1
	mainSequenceID := uint32(0)
	if newMainSequence {
		mainSequenceID = nextID
		nextID++
	}
	outerID, innerID, animationID, setID := nextID, nextID+1, nextID+2, nextID+3
	nextID += 4
	effectID := uint32(0)
	if effect != "appear" {
		effectID = nextID
	}
	animation := presentationEntranceAnimationFragment(prefix, animationID, setID, effectID, elementID, effect, durationMS)
	event := presentationClickEventFragment(prefix, outerID, innerID, animation)
	if newMainSequence {
		sequence := presentationMainSequenceFragment(prefix, mainSequenceID, event)
		if childList < 0 {
			data := []byte("<" + prefix + "childTnLst>" + string(sequence) + "</" + prefix + "childTnLst>")
			return applyXMLReplacements(raw, []shapeXMLReplacement{{start: root.closeStart, end: root.closeStart, data: data}}), nil
		}
		list := nodes[childList]
		if list.selfClose {
			replacement := []byte("<" + prefix + "childTnLst>" + string(sequence) + "</" + prefix + "childTnLst>")
			return applyXMLReplacements(raw, []shapeXMLReplacement{{start: list.start, end: list.end, data: replacement}}), nil
		}
		return applyXMLReplacements(raw, []shapeXMLReplacement{{start: list.closeStart, end: list.closeStart, data: sequence}}), nil
	}
	list := nodes[mainSequenceList]
	if list.selfClose {
		replacement := []byte("<" + prefix + "childTnLst>" + string(event) + "</" + prefix + "childTnLst>")
		return applyXMLReplacements(raw, []shapeXMLReplacement{{start: list.start, end: list.end, data: replacement}}), nil
	}
	return applyXMLReplacements(raw, []shapeXMLReplacement{{start: list.closeStart, end: list.closeStart, data: event}}), nil
}

func presentationMainSequenceChildList(raw []byte, nodes []styleXMLNode, rootChildList int) (int, error) {
	mainSequence := -1
	for _, sequence := range styleNodeChildren(nodes, rootChildList) {
		if nodes[sequence].local != "seq" {
			continue
		}
		for _, child := range styleNodeChildren(nodes, sequence) {
			if nodes[child].local != "cTn" {
				continue
			}
			attrs, err := styleXMLAttributes(raw, nodes[child])
			if err != nil {
				return -1, err
			}
			if attrs["nodeType"] != "mainSeq" {
				continue
			}
			if mainSequence >= 0 {
				return -1, fmt.Errorf("根时序包含多个主序列，拒绝修改")
			}
			mainSequence = child
		}
	}
	if mainSequence < 0 {
		return -1, nil
	}
	if nodes[mainSequence].selfClose {
		return -1, fmt.Errorf("主动画序列为空元素，拒绝修改")
	}
	childList := -1
	for _, child := range styleNodeChildren(nodes, mainSequence) {
		if nodes[child].local != "childTnLst" {
			continue
		}
		if childList >= 0 {
			return -1, fmt.Errorf("主动画序列包含多个子时序列表，拒绝修改")
		}
		childList = child
	}
	if childList < 0 {
		return -1, fmt.Errorf("主动画序列缺少子时序列表，拒绝修改")
	}
	return childList, nil
}

func presentationMainSequenceFragment(prefix string, sequenceID uint32, event []byte) []byte {
	var fragment bytes.Buffer
	fmt.Fprintf(&fragment, `<%sseq><%scTn id="%d" dur="indefinite" nodeType="mainSeq"><%schildTnLst>`, prefix, prefix, sequenceID, prefix)
	fragment.Write(event)
	fmt.Fprintf(&fragment, `</%schildTnLst></%scTn>`, prefix, prefix)
	for _, eventName := range []string{"onPrev", "onNext"} {
		listName := "prevCondLst"
		if eventName == "onNext" {
			listName = "nextCondLst"
		}
		fmt.Fprintf(&fragment, `<%s%s><%scond evt="%s"><%stgtEl><%ssldTgt/></%stgtEl></%scond></%s%s>`, prefix, listName, prefix, eventName, prefix, prefix, prefix, prefix, prefix, listName)
	}
	fmt.Fprintf(&fragment, `</%sseq>`, prefix)
	return fragment.Bytes()
}

func presentationClickEventFragment(prefix string, outerID, innerID uint32, animation []byte) []byte {
	var fragment bytes.Buffer
	fmt.Fprintf(&fragment, `<%spar><%scTn id="%d" fill="hold">`, prefix, prefix, outerID)
	fmt.Fprintf(&fragment, `<%sstCondLst><%scond delay="indefinite"/></%sstCondLst><%schildTnLst>`, prefix, prefix, prefix, prefix)
	fmt.Fprintf(&fragment, `<%spar><%scTn id="%d" fill="hold">`, prefix, prefix, innerID)
	fmt.Fprintf(&fragment, `<%sstCondLst><%scond delay="0"/></%sstCondLst><%schildTnLst>`, prefix, prefix, prefix, prefix)
	fragment.Write(animation)
	fmt.Fprintf(&fragment, `</%schildTnLst></%scTn></%spar></%schildTnLst></%scTn></%spar>`, prefix, prefix, prefix, prefix, prefix, prefix)
	return fragment.Bytes()
}

func presentationEntranceAnimationFragment(prefix string, animationID, setID, effectID, elementID uint32, effect string, durationMS int) []byte {
	presetID, filter := "1", ""
	switch effect {
	case "fade":
		presetID, filter = "10", "fade"
	case "fly_in":
		presetID, filter = "2", "slide(fromLeft)"
	}
	var fragment bytes.Buffer
	fmt.Fprintf(&fragment, `<%spar><%scTn id="%d" presetID="%s" presetClass="entr" presetSubtype="0" fill="hold" grpId="0" nodeType="clickEffect"><%sstCondLst><%scond delay="0"/></%sstCondLst><%schildTnLst>`, prefix, prefix, animationID, presetID, prefix, prefix, prefix, prefix)
	fmt.Fprintf(&fragment, `<%sset><%scBhvr><%scTn id="%d" dur="1" fill="hold"/><%stgtEl><%sspTgt spid="%d"/></%stgtEl><%sattrNameLst><%sattrName>style.visibility</%sattrName></%sattrNameLst></%scBhvr><%sto><%sstrVal val="visible"/></%sto></%sset>`, prefix, prefix, prefix, setID, prefix, prefix, elementID, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix)
	if filter != "" {
		fmt.Fprintf(&fragment, `<%sanimEffect transition="in" filter="%s"><%scBhvr><%scTn id="%d" dur="%d"/><%stgtEl><%sspTgt spid="%d"/></%stgtEl></%scBhvr></%sanimEffect>`, prefix, filter, prefix, prefix, effectID, durationMS, prefix, prefix, elementID, prefix, prefix, prefix)
	}
	fmt.Fprintf(&fragment, `</%schildTnLst></%scTn></%spar>`, prefix, prefix, prefix)
	return fragment.Bytes()
}

func styleXMLAttributes(data []byte, node styleXMLNode) (map[string]string, error) {
	if node.start < 0 || node.startTagEnd <= node.start || node.startTagEnd > len(data) {
		return nil, fmt.Errorf("动画 XML 属性范围无效")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data[node.start:node.startTagEnd]))
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("解析动画 XML 属性失败: %w", err)
	}
	start, ok := token.(xml.StartElement)
	if !ok {
		return nil, fmt.Errorf("动画 XML 起始标签无效")
	}
	attrs := make(map[string]string, len(start.Attr))
	for _, attr := range start.Attr {
		attrs[attr.Name.Local] = attr.Value
	}
	return attrs, nil
}

func patchSlideTiming(slideXML, timing []byte) ([]byte, error) {
	if len(timing) == 0 || len(timing) > maxPresentationTimingBytes {
		return nil, fmt.Errorf("动画时序长度无效")
	}
	timingNodes, err := scanStyleXML(timing)
	if err != nil || len(timingNodes) == 0 || timingNodes[0].local != "timing" {
		return nil, fmt.Errorf("动画时序 XML 无效")
	}
	nodes, err := scanStyleXML(slideXML)
	if err != nil || len(nodes) == 0 || nodes[0].local != "sld" || nodes[0].selfClose {
		return nil, fmt.Errorf("幻灯片 XML 根节点无效")
	}
	var current int = -1
	var extension int = -1
	for _, child := range styleNodeChildren(nodes, 0) {
		switch nodes[child].local {
		case "timing":
			if current >= 0 {
				return nil, fmt.Errorf("幻灯片包含多个动画时序，拒绝覆盖")
			}
			current = child
		case "extLst":
			extension = child
		}
	}
	if current >= 0 {
		span := nodes[current]
		return applyXMLReplacements(slideXML, []shapeXMLReplacement{{start: span.start, end: span.end, data: timing}}), nil
	}
	position := nodes[0].closeStart
	if extension >= 0 {
		position = nodes[extension].start
	}
	return applyXMLReplacements(slideXML, []shapeXMLReplacement{{start: position, end: position, data: timing}}), nil
}

func (d *Document) presentationAnimationOverridesByIndexLocked() (map[int][]byte, error) {
	if len(d.slideAnimationChanges) == 0 {
		return nil, nil
	}
	if d.pres == nil {
		return nil, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return nil, fmt.Errorf("读取动画目标幻灯片失败: %w", err)
	}
	byIndex := make(map[int][]byte, len(d.slideAnimationChanges))
	for index, slide := range slides {
		if timing := d.slideAnimationChanges[uint32(slide.ID())]; len(timing) > 0 {
			byIndex[index] = append([]byte(nil), timing...)
		}
	}
	if len(byIndex) != len(d.slideAnimationChanges) {
		return nil, fmt.Errorf("动画补丁引用了已删除的幻灯片")
	}
	return byIndex, nil
}

func validatePPTXAnimationEdits(path string, expected map[int][]byte) error {
	if len(expected) == 0 {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开动画时序校验包失败: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("读取动画时序校验包信息失败: %w", err)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return fmt.Errorf("读取动画时序校验包失败: %w", err)
	}
	pkg := newPPTXPackage(reader)
	for index, want := range expected {
		if index < 0 || index >= len(pkg.slideParts) {
			return fmt.Errorf("动画时序校验幻灯片序号无效: %d", index)
		}
		partName := pkg.slideParts[index]
		part := pkg.parts[partName]
		if part == nil {
			return fmt.Errorf("动画时序校验缺少幻灯片部件 %s", partName)
		}
		slideXML, err := readOPCPart(part)
		if err != nil {
			return fmt.Errorf("读取动画时序校验幻灯片失败: %w", err)
		}
		nodes, err := scanStyleXML(slideXML)
		if err != nil || len(nodes) == 0 || nodes[0].local != "sld" {
			return fmt.Errorf("动画时序校验幻灯片 XML 无效: %s", partName)
		}
		found := 0
		for _, child := range styleNodeChildren(nodes, 0) {
			if nodes[child].local != "timing" {
				continue
			}
			found++
			span := nodes[child]
			if !bytes.Equal(slideXML[span.start:span.end], want) {
				return fmt.Errorf("幻灯片 %d 的动画时序未按预期保存", index+1)
			}
		}
		if found != 1 {
			return fmt.Errorf("幻灯片 %d 保存后包含 %d 个动画时序，预期 1 个", index+1, found)
		}
	}
	return nil
}

type presentationAnimationFrame struct {
	previous    int
	presetID    string
	presetClass string
}

func parsePresentationAnimations(ctx context.Context, raw []byte, result *document.PresentationAnimationSummary) error {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.Strict = true
	stack := make([]presentationAnimationFrame, 0, 32)
	activeAnimation := -1
	tokens := 0
	for {
		tokens++
		if tokens > maxPresentationTimingTokens {
			result.Truncated = true
			appendAnimationDiagnostic(result, "动画时序节点超过解析上限")
			return nil
		}
		if tokens&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("解析动画时序失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if len(stack) >= maxPresentationTimingDepth {
				result.Truncated = true
				appendAnimationDiagnostic(result, "动画时序嵌套超过解析上限")
				return nil
			}
			previous := activeAnimation
			presetID, presetClass := "", ""
			if len(stack) > 0 {
				presetID = stack[len(stack)-1].presetID
				presetClass = stack[len(stack)-1].presetClass
			}
			for _, attr := range value.Attr {
				switch attr.Name.Local {
				case "presetID":
					presetID = boundedAnimationText(attr.Value)
				case "presetClass":
					presetClass = boundedAnimationText(attr.Value)
				}
			}
			if value.Name.Local == "cTn" && activeAnimation >= 0 {
				if presetID != "" {
					result.Animations[activeAnimation].PresetID = presetID
				}
				if presetClass != "" {
					result.Animations[activeAnimation].PresetClass = presetClass
				}
				for _, attr := range value.Attr {
					if attr.Name.Local == "dur" {
						result.Animations[activeAnimation].Duration = boundedAnimationText(attr.Value)
					}
				}
			}
			if animationKind(value.Name.Local) {
				if len(result.Animations) >= maxPresentationAnimations {
					result.Truncated = true
					appendAnimationDiagnostic(result, "动画项超过 500 项上限")
					return nil
				}
				entry := document.PresentationAnimation{
					Index: len(result.Animations) + 1, Kind: value.Name.Local,
					PresetID: presetID, PresetClass: presetClass,
				}
				for _, attr := range value.Attr {
					text := boundedAnimationText(attr.Value)
					switch attr.Name.Local {
					case "presetID":
						entry.PresetID = text
					case "presetClass":
						entry.PresetClass = text
					case "filter":
						entry.Effect = text
					case "transition":
						entry.Transition = text
					case "dur":
						entry.Duration = text
					}
				}
				result.Animations = append(result.Animations, entry)
				activeAnimation = len(result.Animations) - 1
			} else if value.Name.Local == "spTgt" && activeAnimation >= 0 {
				for _, attr := range value.Attr {
					if attr.Name.Local == "spid" {
						id, err := strconv.ParseUint(attr.Value, 10, 32)
						if err == nil && id > 0 {
							result.Animations[activeAnimation].TargetElementID = uint32(id)
						}
						break
					}
				}
			}
			stack = append(stack, presentationAnimationFrame{previous: previous, presetID: presetID, presetClass: presetClass})
		case xml.EndElement:
			if len(stack) == 0 {
				return fmt.Errorf("解析动画时序失败: XML 元素栈不匹配")
			}
			activeAnimation = stack[len(stack)-1].previous
			stack = stack[:len(stack)-1]
		}
	}
}

func animationKind(name string) bool {
	switch name {
	case "anim", "animClr", "animEffect", "animMotion", "animRot", "animScale", "cmd", "set":
		return true
	default:
		return false
	}
}

func boundedAnimationText(value string) string {
	if len(value) <= maxPresentationAnimationText {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxPresentationAnimationText])
}

func appendAnimationDiagnostic(result *document.PresentationAnimationSummary, message string) bool {
	if len(result.Diagnostics) >= 8 {
		return false
	}
	result.Diagnostics = append(result.Diagnostics, boundedAnimationText(message))
	return true
}
