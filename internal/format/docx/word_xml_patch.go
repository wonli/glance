package docx

import (
	"bytes"
	"errors"
	"strings"
)

type xmlReplacement struct {
	start int
	end   int
	value []byte
}

type rawXMLAttribute struct {
	name      string
	prefix    string
	local     string
	start     int
	end       int
	valueFrom int
	valueTo   int
	quote     byte
}

func wrapWordProperties(prefix string, children []byte) []byte {
	return wrapWordElement(prefix, "rPr", children)
}

func wrapWordElement(prefix, local string, children []byte) []byte {
	name := qualifiedWordName(prefix, local)
	result := make([]byte, 0, len(name)*2+5+len(children))
	result = append(result, '<')
	result = append(result, name...)
	result = append(result, '>')
	result = append(result, children...)
	result = append(result, []byte("</"+name+">")...)
	return result
}

func makeWordPropertyMarkup(prefix, element, attribute, value string) []byte {
	name := qualifiedWordName(prefix, element)
	result := make([]byte, 0, len(name)*2+len(attribute)+len(value)+8)
	result = append(result, '<')
	result = append(result, name...)
	if attribute != "" {
		result = append(result, ' ')
		result = append(result, qualifiedWordName(prefix, attribute)...)
		result = append(result, "=\""...)
		result = append(result, escapeXMLAttribute(value)...)
		result = append(result, '"')
	}
	result = append(result, "/>"...)
	return result
}

func qualifiedWordName(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

func elementPrefix(name string) string {
	if index := strings.IndexByte(name, ':'); index >= 0 {
		return name[:index]
	}
	return "w"
}

func patchElementAttribute(data []byte, node *wordXMLNode, attribute, value string) ([]byte, error) {
	opening := data[node.start:node.openEnd]
	patched, err := setRawXMLAttribute(opening, elementPrefix(node.rawName), attribute, value)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(opening, patched) {
		return data, nil
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: node.start, end: node.openEnd, value: patched}})
}

func insertElementContent(data []byte, node *wordXMLNode, value []byte) ([]byte, error) {
	if len(value) == 0 {
		return data, nil
	}
	if node.selfClosing {
		opening, err := expandSelfClosingTag(data[node.start:node.openEnd])
		if err != nil {
			return nil, err
		}
		replacement := append([]byte(nil), opening...)
		replacement = append(replacement, value...)
		replacement = append(replacement, []byte("</"+node.rawName+">")...)
		return applyXMLReplacements(data, []xmlReplacement{{start: node.start, end: node.end, value: replacement}})
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: node.contentEnd, end: node.contentEnd, value: value}})
}

func insertElementContentAtStart(data []byte, node *wordXMLNode, value []byte) ([]byte, error) {
	if len(value) == 0 {
		return data, nil
	}
	if node.selfClosing {
		opening, err := expandSelfClosingTag(data[node.start:node.openEnd])
		if err != nil {
			return nil, err
		}
		replacement := append([]byte(nil), opening...)
		replacement = append(replacement, value...)
		replacement = append(replacement, []byte("</"+node.rawName+">")...)
		return applyXMLReplacements(data, []xmlReplacement{{start: node.start, end: node.end, value: replacement}})
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: node.openEnd, end: node.openEnd, value: value}})
}

func expandSelfClosingTag(rawOpen []byte) ([]byte, error) {
	end := bytes.LastIndexByte(rawOpen, '>')
	if end < 0 {
		return nil, errors.New("invalid XML start tag")
	}
	slash := end - 1
	for slash >= 0 && (rawOpen[slash] == ' ' || rawOpen[slash] == '\t' || rawOpen[slash] == '\r' || rawOpen[slash] == '\n') {
		slash--
	}
	if slash < 0 || rawOpen[slash] != '/' {
		return nil, errors.New("XML element is not self-closing")
	}
	result := make([]byte, 0, len(rawOpen)-1)
	result = append(result, rawOpen[:slash]...)
	result = append(result, rawOpen[slash+1:]...)
	return result, nil
}

func setRawXMLAttribute(rawOpen []byte, prefix, local, value string) ([]byte, error) {
	attributes, nameEnd, tagEnd, err := scanRawXMLAttributes(rawOpen)
	if err != nil {
		return nil, err
	}
	escaped := escapeXMLAttribute(value)
	for _, attribute := range attributes {
		if attribute.prefix != prefix || attribute.local != local {
			continue
		}
		result := make([]byte, 0, len(rawOpen)-attribute.valueTo+attribute.valueFrom+len(escaped))
		result = append(result, rawOpen[:attribute.valueFrom]...)
		result = append(result, escaped...)
		result = append(result, rawOpen[attribute.valueTo:]...)
		return result, nil
	}
	name := qualifiedWordName(prefix, local)
	addition := append([]byte(" "+name+`="`), escaped...)
	addition = append(addition, '"')
	insertAt := tagEnd
	for insertAt > nameEnd && (rawOpen[insertAt-1] == ' ' || rawOpen[insertAt-1] == '\t' || rawOpen[insertAt-1] == '\r' || rawOpen[insertAt-1] == '\n') {
		insertAt--
	}
	if insertAt > nameEnd && rawOpen[insertAt-1] == '/' {
		insertAt--
	}
	result := make([]byte, 0, len(rawOpen)+len(addition))
	result = append(result, rawOpen[:insertAt]...)
	result = append(result, addition...)
	result = append(result, rawOpen[insertAt:]...)
	return result, nil
}

func removeRawXMLAttribute(rawOpen []byte, prefix, local string) ([]byte, error) {
	attributes, _, _, err := scanRawXMLAttributes(rawOpen)
	if err != nil {
		return nil, err
	}
	for _, attribute := range attributes {
		if attribute.prefix == prefix && attribute.local == local {
			start := attribute.start
			for start > 0 && xmlSpace(rawOpen[start-1]) {
				start--
			}
			return append(append([]byte(nil), rawOpen[:start]...), rawOpen[attribute.end:]...), nil
		}
	}
	return rawOpen, nil
}

func rawAttributeEquals(rawOpen []byte, prefix, local, value string) bool {
	attributes, _, _, err := scanRawXMLAttributes(rawOpen)
	if err != nil {
		return false
	}
	for _, attribute := range attributes {
		if attribute.prefix == prefix && attribute.local == local {
			return string(rawOpen[attribute.valueFrom:attribute.valueTo]) == value
		}
	}
	return false
}

func scanRawXMLAttributes(rawOpen []byte) ([]rawXMLAttribute, int, int, error) {
	end := bytes.LastIndexByte(rawOpen, '>')
	if end < 0 || len(rawOpen) == 0 || rawOpen[0] != '<' {
		return nil, 0, 0, errors.New("invalid XML start tag")
	}
	nameEnd := 1
	for nameEnd < end && !xmlSpace(rawOpen[nameEnd]) && rawOpen[nameEnd] != '/' {
		nameEnd++
	}
	attributes := make([]rawXMLAttribute, 0)
	for index := nameEnd; index < end; {
		for index < end && (xmlSpace(rawOpen[index]) || rawOpen[index] == '/') {
			index++
		}
		if index >= end {
			break
		}
		nameStart := index
		for index < end && !xmlSpace(rawOpen[index]) && rawOpen[index] != '=' && rawOpen[index] != '/' {
			index++
		}
		if nameStart == index {
			return nil, 0, 0, errors.New("invalid XML attribute name")
		}
		name := string(rawOpen[nameStart:index])
		for index < end && xmlSpace(rawOpen[index]) {
			index++
		}
		if index >= end || rawOpen[index] != '=' {
			return nil, 0, 0, errors.New("invalid XML attribute assignment")
		}
		index++
		for index < end && xmlSpace(rawOpen[index]) {
			index++
		}
		if index >= end || (rawOpen[index] != '\'' && rawOpen[index] != '"') {
			return nil, 0, 0, errors.New("invalid XML attribute quote")
		}
		quote := rawOpen[index]
		index++
		valueFrom := index
		for index < end && rawOpen[index] != quote {
			index++
		}
		if index >= end {
			return nil, 0, 0, errors.New("unterminated XML attribute")
		}
		valueTo := index
		index++
		prefix, local := splitQualifiedName(name)
		attributes = append(attributes, rawXMLAttribute{name: name, prefix: prefix, local: local, start: nameStart, end: index, valueFrom: valueFrom, valueTo: valueTo, quote: quote})
	}
	return attributes, nameEnd, end, nil
}

func splitQualifiedName(name string) (string, string) {
	if index := strings.IndexByte(name, ':'); index >= 0 {
		return name[:index], name[index+1:]
	}
	return "", name
}

func xmlSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func escapeXMLAttribute(value string) []byte {
	return []byte(strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value))
}

func applyXMLReplacements(data []byte, replacements []xmlReplacement) ([]byte, error) {
	if len(replacements) == 0 {
		return data, nil
	}
	sortXMLReplacements(replacements)
	result := append([]byte(nil), data...)
	previousStart := len(data) + 1
	for _, replacement := range replacements {
		if replacement.start < 0 || replacement.end < replacement.start || replacement.end > len(data) || replacement.end > previousStart {
			return nil, errors.New("overlapping or out-of-range DOCX XML patch")
		}
		patched := make([]byte, 0, len(result)-(replacement.end-replacement.start)+len(replacement.value))
		patched = append(patched, result[:replacement.start]...)
		patched = append(patched, replacement.value...)
		patched = append(patched, result[replacement.end:]...)
		result = patched
		previousStart = replacement.start
	}
	return result, nil
}

func sortXMLReplacements(replacements []xmlReplacement) {
	for i := 1; i < len(replacements); i++ {
		for j := i; j > 0 && replacements[j].start > replacements[j-1].start; j-- {
			replacements[j], replacements[j-1] = replacements[j-1], replacements[j]
		}
	}
}
