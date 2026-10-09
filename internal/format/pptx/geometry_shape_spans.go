package pptx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
)

type slideShapeSpan struct {
	ID         uint32
	Kind       string
	Start, End int
}

type shapeXMLReplacement struct {
	start, end int
	data       []byte
}

type xmlStackFrame struct {
	name      xml.Name
	shapeSpan int
}

func slideShapeSpans(data []byte) ([]slideShapeSpan, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []xmlStackFrame
	var shapes []slideShapeSpan
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析 slide XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := findNextXMLStart(data, before)
			if start < 0 {
				return nil, fmt.Errorf("无法定位 slide XML 起始标签")
			}
			parentIsTree := len(stack) > 0 && stack[len(stack)-1].name.Local == "spTree"
			spanIndex := -1
			if parentIsTree && (value.Name.Local == "sp" || value.Name.Local == "pic") {
				spanIndex = len(shapes)
				shapes = append(shapes, slideShapeSpan{Kind: value.Name.Local, Start: start})
			} else if len(stack) > 0 {
				spanIndex = stack[len(stack)-1].shapeSpan
			}
			if spanIndex >= 0 && value.Name.Local == "cNvPr" {
				for _, attr := range value.Attr {
					if attr.Name.Local == "id" {
						parsed, parseErr := strconv.ParseUint(attr.Value, 10, 32)
						if parseErr != nil {
							return nil, fmt.Errorf("对象 cNvPr ID 无效: %q", attr.Value)
						}
						shapes[spanIndex].ID = uint32(parsed)
						break
					}
				}
			}
			stack = append(stack, xmlStackFrame{name: value.Name, shapeSpan: spanIndex})
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("slide XML 元素层级不匹配")
			}
			frame := stack[len(stack)-1]
			if frame.name.Local != value.Name.Local {
				return nil, fmt.Errorf("slide XML 元素层级不匹配: %s / %s", frame.name.Local, value.Name.Local)
			}
			if frame.shapeSpan >= 0 && (value.Name.Local == "sp" || value.Name.Local == "pic") {
				shapes[frame.shapeSpan].End = int(decoder.InputOffset())
			}
			stack = stack[:len(stack)-1]
		}
	}
	for _, shape := range shapes {
		if shape.ID == 0 || shape.End <= shape.Start {
			return nil, fmt.Errorf("slide XML 中存在缺少 ID 或闭合标签的对象")
		}
	}
	return shapes, nil
}

func findNextXMLStart(data []byte, from int) int {
	if from < 0 {
		return -1
	}
	for index := from; index < len(data); index++ {
		if data[index] == '<' {
			return index
		}
	}
	return -1
}

func xmlStartTagEnd(data []byte, start int) (int, error) {
	if start < 0 || start >= len(data) || data[start] != '<' {
		return 0, fmt.Errorf("XML 标签起始位置无效")
	}
	var quote byte
	for index := start + 1; index < len(data); index++ {
		current := data[index]
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == '>' {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("XML 起始标签未闭合")
}
