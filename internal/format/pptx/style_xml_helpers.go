package pptx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

func scanStyleXML(data []byte) ([]styleXMLNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var nodes []styleXMLNode
	var stack []int
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析样式 XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := findNextXMLStart(data, before)
			if start < 0 {
				return nil, fmt.Errorf("无法定位样式 XML 起始标签")
			}
			end, err := xmlStartTagEnd(data, start)
			if err != nil {
				return nil, err
			}
			qName, err := xmlElementQName(data[start:end])
			if err != nil {
				return nil, err
			}
			parent := -1
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			selfClose := bytes.HasSuffix(bytes.TrimSpace(data[start:end-1]), []byte("/"))
			node := styleXMLNode{local: value.Name.Local, qName: qName, parent: parent, start: start, startTagEnd: end, selfClose: selfClose}
			nodeIndex := len(nodes)
			nodes = append(nodes, node)
			stack = append(stack, nodeIndex)
		case xml.EndElement:
			if len(stack) == 0 || nodes[stack[len(stack)-1]].local != value.Name.Local {
				return nil, fmt.Errorf("样式 XML 元素层级不匹配")
			}
			nodeIndex := stack[len(stack)-1]
			if !nodes[nodeIndex].selfClose {
				closeStart := findNextXMLStart(data, before)
				if closeStart < 0 {
					return nil, fmt.Errorf("无法定位样式 XML 结束标签")
				}
				nodes[nodeIndex].closeStart = closeStart
				nodes[nodeIndex].end = int(decoder.InputOffset())
			} else {
				nodes[nodeIndex].closeStart = nodes[nodeIndex].startTagEnd
				nodes[nodeIndex].end = nodes[nodeIndex].startTagEnd
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("样式 XML 元素未闭合")
	}
	return nodes, nil
}

func firstStyleNode(nodes []styleXMLNode, local string) int {
	for index := range nodes {
		if nodes[index].local == local {
			return index
		}
	}
	return -1
}

func styleNodeChildren(nodes []styleXMLNode, parent int) []int {
	var children []int
	for index := range nodes {
		if nodes[index].parent == parent {
			children = append(children, index)
		}
	}
	return children
}

func applyXMLReplacements(data []byte, replacements []shapeXMLReplacement) []byte {
	sort.Slice(replacements, func(i, j int) bool {
		if replacements[i].start == replacements[j].start {
			return replacements[i].end > replacements[j].end
		}
		return replacements[i].start > replacements[j].start
	})
	result := append([]byte(nil), data...)
	for _, replacement := range replacements {
		result = append(result[:replacement.start], append(replacement.data, result[replacement.end:]...)...)
	}
	return result
}

func styleXMLPrefix(qName string) string {
	if index := strings.LastIndexByte(qName, ':'); index >= 0 {
		return qName[:index+1]
	}
	return ""
}

func styleAttributeValue(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return strings.ReplaceAll(buffer.String(), `"`, "&quot;")
}

func patchRootXMLAttributes(data []byte, attributes map[string]string) ([]byte, error) {
	nodes, err := scanStyleXML(data)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("样式 XML 根节点缺失")
	}
	span := nodes[0]
	patched, err := setXMLStartTagAttributes(data[span.start:span.startTagEnd], attributes)
	if err != nil {
		return nil, err
	}
	return applyXMLReplacements(data, []shapeXMLReplacement{{start: span.start, end: span.startTagEnd, data: patched}}), nil
}

func upsertStyleChild(data []byte, childNames []string, childXML []byte, order []string) ([]byte, error) {
	nodes, err := scanStyleXML(data)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("样式 XML 父节点缺失")
	}
	parent := nodes[0]
	childIndices := styleNodeChildren(nodes, 0)
	nameSet := make(map[string]bool, len(childNames))
	for _, name := range childNames {
		nameSet[name] = true
	}
	for _, index := range childIndices {
		child := nodes[index]
		if nameSet[child.local] {
			return applyXMLReplacements(data, []shapeXMLReplacement{{start: child.start, end: child.end, data: childXML}}), nil
		}
	}
	if parent.selfClose {
		opening := append([]byte(nil), data[parent.start:parent.startTagEnd]...)
		opening = bytes.TrimSuffix(bytes.TrimSpace(opening), []byte("/>"))
		opening = append(opening, '>')
		closing := []byte("</" + parent.qName + ">")
		result := make([]byte, 0, len(data)+len(childXML)+len(closing))
		result = append(result, data[:parent.start]...)
		result = append(result, opening...)
		result = append(result, childXML...)
		result = append(result, closing...)
		result = append(result, data[parent.end:]...)
		return result, nil
	}
	orderIndex := func(name string) int {
		for index, candidate := range order {
			if candidate == name {
				return index
			}
		}
		return len(order)
	}
	insertAt := parent.closeStart
	for _, index := range childIndices {
		if orderIndex(nodes[index].local) > orderIndex(childNames[0]) {
			insertAt = nodes[index].start
			break
		}
	}
	return applyXMLReplacements(data, []shapeXMLReplacement{{start: insertAt, end: insertAt, data: childXML}}), nil
}
