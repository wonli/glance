package pptx

import (
	"fmt"
	"sort"
	"strings"
)

func xmlElementQName(startTag []byte) (string, error) {
	if len(startTag) < 3 || startTag[0] != '<' {
		return "", fmt.Errorf("XML 元素起始标签无效")
	}
	end := 1
	for end < len(startTag) && !isXMLSpace(startTag[end]) && startTag[end] != '/' && startTag[end] != '>' {
		end++
	}
	if end == 1 {
		return "", fmt.Errorf("XML 元素名称为空")
	}
	return string(startTag[1:end]), nil
}

func isXMLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func setXMLStartTagAttributes(startTag []byte, changes map[string]string) ([]byte, error) {
	if len(startTag) < 3 || startTag[0] != '<' || startTag[len(startTag)-1] != '>' {
		return nil, fmt.Errorf("XML 属性起始标签无效")
	}
	gt := len(startTag) - 1
	attributeLimit := gt
	for attributeLimit > 0 && isXMLSpace(startTag[attributeLimit-1]) {
		attributeLimit--
	}
	selfClose := attributeLimit > 0 && startTag[attributeLimit-1] == '/'
	if selfClose {
		attributeLimit--
	}
	nameEnd := 1
	for nameEnd < attributeLimit && !isXMLSpace(startTag[nameEnd]) && startTag[nameEnd] != '/' {
		nameEnd++
	}
	result := append([]byte(nil), startTag[:nameEnd]...)
	found := make(map[string]bool, len(changes))
	index := nameEnd
	for index < attributeLimit {
		spaceStart := index
		for index < attributeLimit && isXMLSpace(startTag[index]) {
			index++
		}
		result = append(result, startTag[spaceStart:index]...)
		if index >= attributeLimit {
			break
		}
		attributeStart := index
		for index < attributeLimit && !isXMLSpace(startTag[index]) && startTag[index] != '=' {
			index++
		}
		if attributeStart == index {
			return nil, fmt.Errorf("无法解析 XML 属性")
		}
		qualifiedName := string(startTag[attributeStart:index])
		for index < attributeLimit && isXMLSpace(startTag[index]) {
			index++
		}
		if index >= attributeLimit || startTag[index] != '=' {
			return nil, fmt.Errorf("XML 属性 %s 缺少等号", qualifiedName)
		}
		index++
		for index < attributeLimit && isXMLSpace(startTag[index]) {
			index++
		}
		if index >= attributeLimit || (startTag[index] != '\'' && startTag[index] != '"') {
			return nil, fmt.Errorf("XML 属性 %s 缺少引号", qualifiedName)
		}
		quote := startTag[index]
		valueStart := index + 1
		index = valueStart
		for index < attributeLimit && startTag[index] != quote {
			index++
		}
		if index >= attributeLimit {
			return nil, fmt.Errorf("XML 属性 %s 引号未闭合", qualifiedName)
		}
		localName := qualifiedName
		if colon := strings.LastIndexByte(localName, ':'); colon >= 0 {
			localName = localName[colon+1:]
		}
		if value, ok := changes[localName]; ok {
			result = append(result, startTag[attributeStart:valueStart]...)
			result = append(result, value...)
			result = append(result, quote)
			found[localName] = true
		} else {
			result = append(result, startTag[attributeStart:index+1]...)
		}
		index++
	}
	missing := make([]string, 0, len(changes)-len(found))
	for name := range changes {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		value := changes[name]
		result = append(result, ' ')
		result = append(result, name...)
		result = append(result, `="`...)
		result = append(result, value...)
		result = append(result, '"')
	}
	result = append(result, startTag[attributeLimit:gt]...)
	result = append(result, '>')
	return result, nil
}
