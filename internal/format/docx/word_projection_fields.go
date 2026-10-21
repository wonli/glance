package docx

import (
	"strings"
	"unicode"
)

type wordFieldTextProjection struct {
	editable        bool
	hyperlinkTarget string
}

type wordFieldProjectionFrame struct {
	instruction strings.Builder
	inResult    bool
	target      string
}

// wordFieldTextProjections marks only cached results belonging to fields. A
// HYPERLINK result remains editable so the normal Word selection path can hit
// it; other field results remain read-only.
func wordFieldTextProjections(paragraph *wordXMLNode) map[*wordXMLNode]wordFieldTextProjection {
	projections := make(map[*wordXMLNode]wordFieldTextProjection)
	var fields []*wordFieldProjectionFrame
	var walk func(*wordXMLNode)
	walk = func(node *wordXMLNode) {
		if node == nil {
			return
		}
		if node.name.Local == "fldSimple" {
			target := wordHyperlinkFieldTarget(attrValue(node, "instr"))
			markSimpleWordFieldResults(node, target, projections)
			return
		}
		switch node.name.Local {
		case "fldChar":
			switch strings.ToLower(attrValue(node, "fldCharType")) {
			case "begin":
				fields = append(fields, &wordFieldProjectionFrame{})
			case "separate":
				if len(fields) > 0 {
					field := fields[len(fields)-1]
					field.target = wordHyperlinkFieldTarget(field.instruction.String())
					field.inResult = true
				}
			case "end":
				if len(fields) > 0 {
					fields = fields[:len(fields)-1]
				}
			}
			return
		case "instrText":
			if len(fields) > 0 && !fields[len(fields)-1].inResult {
				fields[len(fields)-1].instruction.WriteString(node.text)
			}
			return
		case "t":
			if len(fields) > 0 && fields[len(fields)-1].inResult {
				field := fields[len(fields)-1]
				projections[node] = wordFieldTextProjection{editable: field.target != "", hyperlinkTarget: field.target}
			}
			return
		}
		for _, child := range node.children {
			walk(child)
		}
	}
	walk(paragraph)
	return projections
}

func markSimpleWordFieldResults(node *wordXMLNode, target string, projections map[*wordXMLNode]wordFieldTextProjection) {
	var walk func(*wordXMLNode)
	walk = func(current *wordXMLNode) {
		if current == nil {
			return
		}
		if current.name.Local == "t" {
			projections[current] = wordFieldTextProjection{editable: target != "", hyperlinkTarget: target}
			return
		}
		for _, child := range current.children {
			walk(child)
		}
	}
	for _, child := range node.children {
		walk(child)
	}
}

func wordHyperlinkFieldTarget(instruction string) string {
	tokens := wordFieldInstructionTokens(instruction)
	if len(tokens) == 0 || !strings.EqualFold(tokens[0], "HYPERLINK") {
		return ""
	}
	target := ""
	anchor := ""
	for index := 1; index < len(tokens); index++ {
		token := tokens[index]
		if !strings.HasPrefix(token, "\\") {
			if target == "" {
				target = token
			}
			continue
		}
		if index+1 >= len(tokens) {
			continue
		}
		value := tokens[index+1]
		index++
		if strings.EqualFold(token, "\\l") {
			anchor = value
		}
	}
	if anchor != "" {
		if target == "" {
			return "#" + anchor
		}
		if strings.HasPrefix(target, "#") {
			return target
		}
		return target + "#" + anchor
	}
	return target
}

func wordFieldInstructionTokens(instruction string) []string {
	var tokens []string
	var current strings.Builder
	quoted := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	for _, r := range strings.TrimSpace(instruction) {
		switch {
		case r == '"':
			quoted = !quoted
		case unicode.IsSpace(r) && !quoted:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}
