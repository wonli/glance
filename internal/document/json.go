package document

import (
	"encoding/json"
	"strings"
	"unicode"
)

// JSONTree is an immutable view of the original source. Byte ranges allow an
// adapter to edit a value without rewriting unrelated keys or number lexemes.
type JSONTree struct {
	Source   string
	Nodes    []JSONNode
	KeyCount int
	MaxDepth int
}

type JSONNode struct {
	Key, Kind, Raw, Display             string
	Pointer, JSONPath, DotPath          string
	Parent, Depth, Start, End           int
	FirstChild, NextSibling, ChildCount int
	Children                            []int
}


func (n JSONNode) Container() bool { return n.Kind == "object" || n.Kind == "array" }

func (t *JSONTree) ChildCount(id int) int {
	if t == nil || id < 0 || id >= len(t.Nodes) {
		return 0
	}
	n := t.Nodes[id]
	if n.ChildCount > 0 {
		return n.ChildCount
	}
	return len(n.Children)
}

func (t *JSONTree) Children(id int) []int {
	if t == nil || id < 0 || id >= len(t.Nodes) {
		return nil
	}
	n := t.Nodes[id]
	if len(n.Children) > 0 {
		return n.Children
	}
	if n.ChildCount == 0 {
		return nil
	}
	res := make([]int, 0, n.ChildCount)
	for c := n.FirstChild; c >= 0; c = t.Nodes[c].NextSibling {
		res = append(res, c)
	}
	return res
}

func (t *JSONTree) NodePointer(id int) string {
	if t == nil || id < 0 || id >= len(t.Nodes) {
		return ""
	}
	if t.Nodes[id].Pointer != "" {
		return t.Nodes[id].Pointer
	}
	var parts []string
	for curr := id; curr > 0 && curr < len(t.Nodes); curr = t.Nodes[curr].Parent {
		key := t.Nodes[curr].Key
		escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
		parts = append(parts, escaped)
	}
	var sb strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		sb.WriteByte('/')
		sb.WriteString(parts[i])
	}
	return sb.String()
}

func (t *JSONTree) NodeJSONPath(id int) string {
	if t == nil || id < 0 || id >= len(t.Nodes) {
		return ""
	}
	if t.Nodes[id].JSONPath != "" {
		return t.Nodes[id].JSONPath
	}
	var ancestors []int
	for curr := id; curr >= 0 && curr < len(t.Nodes); curr = t.Nodes[curr].Parent {
		ancestors = append(ancestors, curr)
	}
	var sb strings.Builder
	sb.WriteString("$")
	for i := len(ancestors) - 2; i >= 0; i-- {
		node := t.Nodes[ancestors[i]]
		parent := t.Nodes[node.Parent]
		if parent.Kind == "array" {
			sb.WriteString("[" + node.Key + "]")
		} else if isJSONIdent(node.Key) {
			sb.WriteString("." + node.Key)
		} else {
			quoted, _ := json.Marshal(node.Key)
			sb.WriteString("[" + string(quoted) + "]")
		}
	}
	return sb.String()
}

func (t *JSONTree) NodeDotPath(id int) string {
	if t == nil || id < 0 || id >= len(t.Nodes) {
		return ""
	}
	if t.Nodes[id].DotPath != "" {
		return t.Nodes[id].DotPath
	}
	var ancestors []int
	for curr := id; curr >= 0 && curr < len(t.Nodes); curr = t.Nodes[curr].Parent {
		ancestors = append(ancestors, curr)
	}
	var sb strings.Builder
	sb.WriteString("root")
	for i := len(ancestors) - 2; i >= 0; i-- {
		node := t.Nodes[ancestors[i]]
		parent := t.Nodes[node.Parent]
		if parent.Kind == "array" {
			sb.WriteString("[" + node.Key + "]")
		} else if isJSONIdent(node.Key) {
			sb.WriteString("." + node.Key)
		} else {
			quoted, _ := json.Marshal(node.Key)
			sb.WriteString("[" + string(quoted) + "]")
		}
	}
	return sb.String()
}

func (t *JSONTree) FindNodeByPointer(pointer string) int {
	if t == nil || len(t.Nodes) == 0 || pointer == "" {
		return 0
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	curr := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		found := -1
		for c := t.Nodes[curr].FirstChild; c >= 0; c = t.Nodes[c].NextSibling {
			if t.Nodes[c].Key == part {
				found = c
				break
			}
		}
		if found < 0 {
			for _, c := range t.Nodes[curr].Children {
				if t.Nodes[c].Key == part {
					found = c
					break
				}
			}
		}
		if found < 0 {
			return curr
		}
		curr = found
	}
	return curr
}

func isJSONIdent(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if r != '_' && r != '$' && !unicode.IsLetter(r) && !(i > 0 && unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

