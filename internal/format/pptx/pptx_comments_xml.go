package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func escapePPTXXMLText(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

func escapePPTXXMLAttribute(value string) string {
	return html.EscapeString(value)
}

func nextPPTXRelationshipID(data []byte) string {
	root, err := scanPPTXXML(data)
	if err != nil {
		return "rId1"
	}
	used := make(map[uint64]bool)
	for _, child := range root.Children {
		if child.Local != "Relationship" {
			continue
		}
		id := pptxXMLAttribute(child, "Id")
		if strings.HasPrefix(id, "rId") {
			if n, err := strconv.ParseUint(strings.TrimPrefix(id, "rId"), 10, 64); err == nil {
				used[n] = true
			}
		}
	}
	for i := uint64(1); ; i++ {
		if !used[i] {
			return "rId" + strconv.FormatUint(i, 10)
		}
	}
}

func appendPPTXOPCRelationship(data []byte, relationshipXML string) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="` + pptxPackageRelationshipNS + `"></Relationships>`)
	}
	root, err := scanPPTXXML(data)
	if err != nil || root.Local != "Relationships" {
		return nil, errors.New("PPTX OPC 关系部件无效")
	}
	for _, child := range root.Children {
		if child.Local != "Relationship" {
			continue
		}
		if pptxXMLAttribute(child, "Id") == "" || pptxXMLAttribute(child, "Type") == "" || pptxXMLAttribute(child, "Target") == "" {
			return nil, errors.New("PPTX OPC 关系包含无效项")
		}
	}
	return appendPPTXXMLChild(data, "Relationships", relationshipXML)
}

func setPPTXCommentAuthorLastIndex(data []byte, authorID, index uint32) ([]byte, error) {
	root, err := scanPPTXXML(data)
	if err != nil || root.Local != "cmAuthorLst" {
		return nil, errors.New("PPTX 批注作者部件无效")
	}
	for _, child := range root.Children {
		if child.Local == "cmAuthor" && pptxXMLAttribute(child, "id") == strconv.FormatUint(uint64(authorID), 10) {
			return setPPTXXMLAttribute(data, child, "lastIdx", strconv.FormatUint(uint64(index), 10))
		}
	}
	return nil, fmt.Errorf("未找到 PPTX 批注作者 ID %d", authorID)
}

func removePPTXComment(data []byte, authorID, index uint32) ([]byte, error) {
	root, err := scanPPTXXML(data)
	if err != nil || root.Local != "cmLst" {
		return nil, errors.New("PPTX 批注部件无效")
	}
	for _, child := range root.Children {
		if child.Local == "cm" && pptxXMLAttribute(child, "authorId") == strconv.FormatUint(uint64(authorID), 10) && pptxXMLAttribute(child, "idx") == strconv.FormatUint(uint64(index), 10) {
			return applyPPTXXMLRanges(data, []pptxXMLRange{{Start: child.Start, End: child.End}})
		}
	}
	return nil, fmt.Errorf("未找到 PPTX 批注 %s", pptxCommentID(authorID, index))
}

func appendPPTXXMLChild(data []byte, rootLocal, childXML string) ([]byte, error) {
	root, err := scanPPTXXML(data)
	if err != nil || root.Local != rootLocal {
		return nil, fmt.Errorf("PPTX XML 根元素 %q 无效", rootLocal)
	}
	if root.SelfClosing {
		openTag := append([]byte(nil), data[root.Start:root.StartTagEnd]...)
		close := bytes.LastIndexByte(openTag, '>')
		if close < 0 {
			return nil, errors.New("PPTX XML 根元素标签无效")
		}
		beforeClose := bytes.TrimRight(openTag[:close], " \t\r\n")
		if len(beforeClose) == 0 || beforeClose[len(beforeClose)-1] != '/' {
			return nil, errors.New("PPTX XML 自闭合根元素无效")
		}
		open := append([]byte(nil), beforeClose[:len(beforeClose)-1]...)
		open = append(open, '>')
		open = append(open, childXML...)
		open = append(open, []byte("</"+root.RawName+">")...)
		return applyPPTXXMLRanges(data, []pptxXMLRange{{Start: root.Start, End: root.End, Data: open}})
	}
	return applyPPTXXMLRanges(data, []pptxXMLRange{{Start: root.EndTagStart, End: root.EndTagStart, Data: []byte(childXML)}})
}

type pptxXMLSpan struct {
	Local       string
	RawName     string
	Start       int
	StartTagEnd int
	EndTagStart int
	End         int
	SelfClosing bool
	Attrs       []xml.Attr
	Children    []*pptxXMLSpan
}

func scanPPTXXML(data []byte) (*pptxXMLSpan, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	stack := make([]*pptxXMLSpan, 0, 16)
	var root *pptxXMLSpan
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch item := token.(type) {
		case xml.StartElement:
			end := int(decoder.InputOffset())
			start := bytes.LastIndex(data[:end], []byte("<"))
			if start < 0 {
				return nil, errors.New("PPTX XML 起始标签无效")
			}
			endTag := bytes.LastIndex(data[start:end], []byte(">"))
			if endTag < 0 {
				return nil, errors.New("PPTX XML 起始标签未闭合")
			}
			startTagEnd := start + endTag + 1
			rawName := pptxXMLRawName(data[start:startTagEnd])
			if rawName == "" {
				return nil, errors.New("PPTX XML 元素名称为空")
			}
			trimmed := bytes.TrimSpace(data[start+1 : startTagEnd-1])
			span := &pptxXMLSpan{Local: item.Name.Local, RawName: rawName, Start: start, StartTagEnd: startTagEnd, SelfClosing: len(trimmed) > 0 && trimmed[len(trimmed)-1] == '/', Attrs: append([]xml.Attr(nil), item.Attr...)}
			if len(stack) > 0 {
				stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, span)
			} else if root == nil {
				root = span
			} else {
				return nil, errors.New("PPTX XML 包含多个根元素")
			}
			stack = append(stack, span)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("PPTX XML 结束标签无对应起始标签")
			}
			span := stack[len(stack)-1]
			end := int(decoder.InputOffset())
			if span.SelfClosing {
				span.EndTagStart = span.StartTagEnd
				span.End = span.StartTagEnd
			} else {
				closeStart := bytes.LastIndex(data[:end], []byte("</"))
				if closeStart < 0 {
					return nil, errors.New("PPTX XML 结束标签无效")
				}
				span.EndTagStart = closeStart
				span.End = end
			}
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, errors.New("PPTX XML 根元素或标签结构不完整")
	}
	return root, nil
}

func pptxXMLRawName(startTag []byte) string {
	if len(startTag) < 3 || startTag[0] != '<' {
		return ""
	}
	start := 1
	for start < len(startTag) && (startTag[start] == '/' || startTag[start] == '?' || startTag[start] == '!') {
		start++
	}
	end := start
	for end < len(startTag) && startTag[end] != ' ' && startTag[end] != '\t' && startTag[end] != '\r' && startTag[end] != '\n' && startTag[end] != '/' && startTag[end] != '>' {
		end++
	}
	return string(startTag[start:end])
}

func pptxXMLAttribute(node *pptxXMLSpan, name string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func setPPTXXMLAttribute(data []byte, node *pptxXMLSpan, name, value string) ([]byte, error) {
	if node == nil || node.Start < 0 || node.StartTagEnd > len(data) || node.Start >= node.StartTagEnd {
		return nil, errors.New("PPTX XML 属性节点无效")
	}
	tag := append([]byte(nil), data[node.Start:node.StartTagEnd]...)
	attrPattern := regexp.MustCompile(`(^|[\t\r\n ])` + regexp.QuoteMeta(name) + `[\t\r\n ]*=[\t\r\n ]*(["'])[^"']*(["'])`)
	loc := attrPattern.FindSubmatchIndex(tag)
	quoted := escapePPTXXMLAttribute(value)
	if loc != nil {
		replacement := append([]byte(nil), tag[:loc[5]]...)
		replacement = append(replacement, quoted...)
		replacement = append(replacement, tag[loc[6]:]...)
		tag = replacement
	} else {
		close := bytes.LastIndexByte(tag, '>')
		if close < 0 {
			return nil, errors.New("PPTX XML 起始标签无效")
		}
		insertAt := close
		for insertAt > 0 && (tag[insertAt-1] == '/' || tag[insertAt-1] == ' ' || tag[insertAt-1] == '\t' || tag[insertAt-1] == '\r' || tag[insertAt-1] == '\n') {
			insertAt--
		}
		tag = append(tag[:insertAt], append([]byte(" "+name+`="`+quoted+`"`), tag[insertAt:]...)...)
	}
	return applyPPTXXMLRanges(data, []pptxXMLRange{{Start: node.Start, End: node.StartTagEnd, Data: tag}})
}

func applyPPTXXMLRanges(data []byte, ranges []pptxXMLRange) ([]byte, error) {
	if len(ranges) == 0 {
		return append([]byte(nil), data...), nil
	}
	ranges = append([]pptxXMLRange(nil), ranges...)
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start > ranges[j].Start })
	result := append([]byte(nil), data...)
	previousStart := len(data) + 1
	for _, current := range ranges {
		if current.Start < 0 || current.End < current.Start || current.End > len(result) || current.End > previousStart {
			return nil, errors.New("PPTX XML 修改范围重叠或越界")
		}
		updated := make([]byte, 0, len(result)-(current.End-current.Start)+len(current.Data))
		updated = append(updated, result[:current.Start]...)
		updated = append(updated, current.Data...)
		updated = append(updated, result[current.End:]...)
		result = updated
		previousStart = current.Start
	}
	if _, err := scanPPTXXML(result); err != nil {
		return nil, fmt.Errorf("PPTX XML 补丁产生无效 XML: %w", err)
	}
	return result, nil
}

type pptxXMLRange struct {
	Start int
	End   int
	Data  []byte
}

func validatePPTXCommentEdits(pathName string, edits map[string][]byte) error {
	if len(edits) == 0 {
		return nil
	}
	file, err := os.Open(pathName)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	zr, err := zip.NewReader(file, info.Size())
	if err != nil {
		_ = file.Close()
		return err
	}
	parts := make(map[string]*zip.File, len(zr.File))
	for _, part := range zr.File {
		parts[strings.TrimPrefix(part.Name, "/")] = part
	}
	for name, expected := range edits {
		part := parts[strings.TrimPrefix(name, "/")]
		if part == nil {
			_ = file.Close()
			return fmt.Errorf("保存后的 PPTX 缺少批注部件 %s", name)
		}
		reader, openErr := part.Open()
		if openErr != nil {
			_ = file.Close()
			return openErr
		}
		actual, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			_ = file.Close()
			return readErr
		}
		if closeErr != nil {
			_ = file.Close()
			return closeErr
		}
		if !bytes.Equal(actual, expected) {
			_ = file.Close()
			return fmt.Errorf("保存后的 PPTX 批注部件 %s 与预期不一致", name)
		}
	}
	return file.Close()
}
