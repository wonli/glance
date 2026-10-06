package jsondoc

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"glance/internal/document"
)

// ParseTree retains source order, duplicate keys and exact number spelling.
// Callers should parse outside the window loop and publish the immutable view.
func ParseTree(source string) (*document.JSONTree, error) {
	d := json.NewDecoder(strings.NewReader(source))
	d.UseNumber()
	tree := &document.JSONTree{Source: source}
	var parse func(string, int, int, string, string, string) (int, error)
	parse = func(key string, parent, depth int, pointer, path, dot string) (int, error) {
		start := int(d.InputOffset())
		for start < len(source) && strings.ContainsRune(" \t\r\n,:", rune(source[start])) {
			start++
		}
		token, err := d.Token()
		if err != nil {
			return 0, err
		}
		n := document.JSONNode{Key: key, Parent: parent, Depth: depth, Start: start, Pointer: pointer, JSONPath: path, DotPath: dot}
		switch value := token.(type) {
		case json.Delim:
			if value == '{' {
				n.Kind = "object"
			} else if value == '[' {
				n.Kind = "array"
			} else {
				return 0, fmt.Errorf("unexpected delimiter %q", value)
			}
		case string:
			n.Kind, n.Display = "string", value
		case json.Number:
			n.Kind, n.Display = "number", value.String()
		case bool:
			n.Kind, n.Display = "boolean", strconv.FormatBool(value)
		case nil:
			n.Kind, n.Display = "null", "null"
		}
		id := len(tree.Nodes)
		tree.Nodes = append(tree.Nodes, n)
		tree.MaxDepth = max(tree.MaxDepth, depth)
		if n.Container() {
			for index := 0; d.More(); index++ {
				childKey := strconv.Itoa(index)
				childPath, childDot := path+"["+childKey+"]", dot+"["+childKey+"]"
				if n.Kind == "object" {
					t, err := d.Token()
					if err != nil {
						return 0, err
					}
					var ok bool
					childKey, ok = t.(string)
					if !ok {
						return 0, fmt.Errorf("expected object key")
					}
					tree.KeyCount++
					if jsonPathIdentifier(childKey) {
						childPath, childDot = path+"."+childKey, dot+"."+childKey
					} else {
						quoted, _ := json.Marshal(childKey)
						childPath, childDot = path+"["+string(quoted)+"]", dot+"["+string(quoted)+"]"
					}
				}
				escaped := strings.ReplaceAll(strings.ReplaceAll(childKey, "~", "~0"), "/", "~1")
				child, err := parse(childKey, id, depth+1, pointer+"/"+escaped, childPath, childDot)
				if err != nil {
					return 0, err
				}
				tree.Nodes[id].Children = append(tree.Nodes[id].Children, child)
			}
			if _, err := d.Token(); err != nil {
				return 0, err
			}
		}
		tree.Nodes[id].End = int(d.InputOffset())
		tree.Nodes[id].Raw = source[start:tree.Nodes[id].End]
		return id, nil
	}
	if _, err := parse("root", -1, 0, "", "$", "root"); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return tree, nil
}

func jsonPathIdentifier(key string) bool {
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

func ReplaceValue(tree *document.JSONTree, id int, value string) (string, error) {
	if tree == nil || id < 0 || id >= len(tree.Nodes) {
		return "", fmt.Errorf("JSON node is unavailable")
	}
	if !json.Valid([]byte(value)) {
		return "", fmt.Errorf("请输入一个有效的 JSON 值")
	}
	n := tree.Nodes[id]
	return tree.Source[:n.Start] + value + tree.Source[n.End:], nil
}
