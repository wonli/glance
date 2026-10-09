package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
)

type wordXMLNode struct {
	name         xml.Name
	rawName      string
	attrs        []xml.Attr
	text         string
	children     []*wordXMLNode
	parent       *wordXMLNode
	start        int
	openEnd      int
	contentStart int
	contentEnd   int
	end          int
	selfClosing  bool
}

func parseWordXML(data []byte) (*wordXMLNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root *wordXMLNode
	var stack []*wordXMLNode
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			openEnd := int(decoder.InputOffset())
			start := bytes.LastIndex(data[:openEnd], []byte("<"))
			if start < 0 {
				return nil, errors.New("cannot locate DOCX XML start tag")
			}
			rawOpen := data[start:openEnd]
			rawName := rawElementName(rawOpen)
			if rawName == "" {
				return nil, errors.New("cannot read DOCX XML element name")
			}
			trimmedOpen := bytes.TrimSpace(rawOpen)
			selfClosing := len(trimmedOpen) >= 2 && trimmedOpen[len(trimmedOpen)-2] == '/'
			node := &wordXMLNode{name: value.Name, rawName: rawName, attrs: append([]xml.Attr(nil), value.Attr...), start: start, openEnd: openEnd, contentStart: openEnd, contentEnd: openEnd, end: openEnd, selfClosing: selfClosing}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("DOCX XML contains multiple root elements")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				node.parent = parent
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) != 0 {
				stack[len(stack)-1].text += string(value)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("unexpected DOCX XML closing element")
			}
			node := stack[len(stack)-1]
			end := int(decoder.InputOffset())
			if node.selfClosing && end == node.openEnd {
				node.contentEnd, node.end = node.openEnd, node.openEnd
			} else {
				endStart := bytes.LastIndex(data[:end], []byte("</"))
				if endStart < node.openEnd {
					return nil, errors.New("cannot locate DOCX XML closing tag")
				}
				node.contentEnd, node.end = endStart, end
			}
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil {
		return nil, errors.New("DOCX XML is empty")
	}
	if len(stack) != 0 {
		return nil, errors.New("DOCX XML has unclosed elements")
	}
	return root, nil
}

func firstChild(node *wordXMLNode, local string) *wordXMLNode {
	if node == nil {
		return nil
	}
	for _, child := range node.children {
		if child.name.Local == local {
			return child
		}
	}
	return nil
}

func findDescendant(node *wordXMLNode, local string) *wordXMLNode {
	for _, child := range node.children {
		if child.name.Local == local {
			return child
		}
		if found := findDescendant(child, local); found != nil {
			return found
		}
	}
	return nil
}

func containsNode(node *wordXMLNode, local string) bool { return findDescendant(node, local) != nil }

func attrValue(node *wordXMLNode, local string) string {
	value, _ := attrValuePresent(node, local)
	return value
}

func attrValuePresent(node *wordXMLNode, local string) (string, bool) {
	if node == nil {
		return "", false
	}
	for _, attr := range node.attrs {
		if attr.Name.Local == local {
			return attr.Value, true
		}
	}
	return "", false
}

func rawElementName(rawOpen []byte) string {
	if len(rawOpen) < 3 || rawOpen[0] != '<' {
		return ""
	}
	end := 1
	for end < len(rawOpen) {
		value := rawOpen[end]
		if value == '>' || value == '/' || value == ' ' || value == '\t' || value == '\r' || value == '\n' {
			break
		}
		end++
	}
	if end <= 1 {
		return ""
	}
	return string(rawOpen[1:end])
}
