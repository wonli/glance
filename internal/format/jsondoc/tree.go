package jsondoc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"glance/internal/document"
)

var arrayIndexStrings [65536]string

func init() {
	for i := 0; i < len(arrayIndexStrings); i++ {
		arrayIndexStrings[i] = strconv.Itoa(i)
	}
}

func arrayIndexStr(i int) string {
	if i < len(arrayIndexStrings) {
		return arrayIndexStrings[i]
	}
	return strconv.Itoa(i)
}

// ParseTree retains source order, duplicate keys and exact number spelling.
// Callers should parse outside the window loop and publish the immutable view.
func ParseTree(source string) (*document.JSONTree, error) {
	return ParseTreeWithCancel(source, nil)
}

// ParseTreeWithCancel parses the JSON tree, checking done periodically.
// If done is closed, parsing halts immediately and returns an error.
func ParseTreeWithCancel(source string, done <-chan struct{}) (*document.JSONTree, error) {
	select {
	case <-done:
		return nil, errors.New("JSON parsing cancelled")
	default:
	}
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, errors.New("empty JSON source")
	}
	large := len(source) > 512*1024
	estNodes := 64
	if large {
		estNodes = countJSONNodesWithCancel(source, done)
		select {
		case <-done:
			return nil, errors.New("JSON parsing cancelled")
		default:
		}
	}
	p := &fastParser{
		src:    source,
		len:    len(source),
		nodes:  make([]document.JSONNode, 0, estNodes),
		intern: make(map[string]string, 1024),
		done:   done,
	}
	if _, err := p.parseValue("root", -1, 0); err != nil {
		return nil, err
	}
	p.skipWhitespace()
	if p.pos < p.len {
		return nil, errors.New("multiple JSON values")
	}
	return &document.JSONTree{
		Source:   source,
		Nodes:    p.nodes,
		KeyCount: p.keyCount,
		MaxDepth: p.maxDepth,
	}, nil
}

type fastParser struct {
	src      string
	pos      int
	len      int
	nodes    []document.JSONNode
	keyCount int
	maxDepth int
	intern   map[string]string
	done     <-chan struct{}
	cancelN  int
}

func (p *fastParser) skipWhitespace() {
	for p.pos < p.len {
		b := p.src[p.pos]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			p.pos++
		} else {
			break
		}
	}
}

func (p *fastParser) internStr(s string) string {
	if val, ok := p.intern[s]; ok {
		return val
	}
	if len(p.intern) < 8192 {
		p.intern[s] = s
	}
	return s
}

func (p *fastParser) parseString() (display string, rawStart, rawEnd int, err error) {
	if p.pos >= p.len || p.src[p.pos] != '"' {
		return "", 0, 0, errors.New("expected '\"'")
	}
	rawStart = p.pos
	p.pos++
	start := p.pos
	hasEscape := false

	for p.pos < p.len {
		idx := strings.IndexAny(p.src[p.pos:], "\"\\")
		if idx < 0 {
			return "", 0, 0, errors.New("unterminated string")
		}
		for j := 0; j < idx; j++ {
			if p.src[p.pos+j] < 0x20 {
				return "", 0, 0, errors.New("unescaped control character in string")
			}
		}
		p.pos += idx
		if p.src[p.pos] == '"' {
			rawEnd = p.pos + 1
			content := p.src[start:p.pos]
			p.pos++
			if !hasEscape {
				display = content
			} else {
				display, err = unescapeJSONString(content)
				if err != nil {
					return "", 0, 0, err
				}
			}
			return display, rawStart, rawEnd, nil
		}
		hasEscape = true
		p.pos++
		if p.pos >= p.len {
			return "", 0, 0, errors.New("unexpected EOF in string escape")
		}
		p.pos++
	}
	return "", 0, 0, errors.New("unterminated string")
}

func unescapeJSONString(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s) {
			return "", errors.New("incomplete escape")
		}
		switch s[i] {
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case '/':
			b.WriteByte('/')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			if i+4 >= len(s) {
				return "", errors.New("incomplete unicode escape")
			}
			r, err := strconv.ParseUint(s[i+1:i+5], 16, 16)
			if err != nil {
				return "", err
			}
			i += 4
			runeVal := rune(r)
			if utf16.IsSurrogate(runeVal) {
				if i+6 < len(s) && s[i+1] == '\\' && s[i+2] == 'u' {
					r2, err2 := strconv.ParseUint(s[i+3:i+7], 16, 16)
					if err2 == nil {
						combined := utf16.DecodeRune(runeVal, rune(r2))
						if combined != utf8.RuneError {
							i += 6
							b.WriteRune(combined)
							continue
						}
					}
				}
			}
			b.WriteRune(runeVal)
		default:
			return "", fmt.Errorf("invalid escape character '\\%c'", s[i])
		}
	}
	return b.String(), nil
}

func (p *fastParser) parseNumber() (display string, rawStart, rawEnd int, err error) {
	rawStart = p.pos
	if p.pos < p.len && p.src[p.pos] == '-' {
		p.pos++
	}
	if p.pos >= p.len {
		return "", 0, 0, errors.New("expected digit after '-'")
	}
	if p.src[p.pos] == '0' {
		p.pos++
	} else if p.src[p.pos] >= '1' && p.src[p.pos] <= '9' {
		for p.pos < p.len && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
	} else {
		return "", 0, 0, errors.New("expected digit")
	}
	if p.pos < p.len && p.src[p.pos] == '.' {
		p.pos++
		if p.pos >= p.len || p.src[p.pos] < '0' || p.src[p.pos] > '9' {
			return "", 0, 0, errors.New("expected digit after '.'")
		}
		for p.pos < p.len && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < p.len && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
		p.pos++
		if p.pos < p.len && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
			p.pos++
		}
		if p.pos >= p.len || p.src[p.pos] < '0' || p.src[p.pos] > '9' {
			return "", 0, 0, errors.New("expected digit in exponent")
		}
		for p.pos < p.len && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
	}
	rawEnd = p.pos
	return p.src[rawStart:rawEnd], rawStart, rawEnd, nil
}

func (p *fastParser) parseLiteral(lit string) error {
	if p.pos+len(lit) <= p.len && p.src[p.pos:p.pos+len(lit)] == lit {
		p.pos += len(lit)
		return nil
	}
	end := p.pos + 10
	if end > p.len {
		end = p.len
	}
	return fmt.Errorf("unexpected token near %q", p.src[p.pos:end])
}

func (p *fastParser) parseValue(key string, parent, depth int) (int, error) {
	if p.done != nil {
		p.cancelN++
		if p.cancelN&2047 == 0 {
			select {
			case <-p.done:
				return 0, errors.New("JSON parsing cancelled")
			default:
			}
		}
	}
	p.skipWhitespace()
	if p.pos >= p.len {
		return 0, io.EOF
	}
	b := p.src[p.pos]
	start := p.pos

	id := len(p.nodes)
	if depth > p.maxDepth {
		p.maxDepth = depth
	}

	switch b {
	case '{':
		p.pos++
		p.nodes = append(p.nodes, document.JSONNode{
			Key: key, Kind: "object", Parent: parent, Depth: depth, Start: start,
			FirstChild: -1, NextSibling: -1,
		})
		lastChild := -1
		count := 0
		for {
			p.skipWhitespace()
			if p.pos >= p.len {
				return 0, errors.New("unterminated object")
			}
			if p.src[p.pos] == '}' {
				p.pos++
				break
			}
			childKey, _, _, err := p.parseString()
			if err != nil {
				return 0, err
			}
			childKey = p.internStr(childKey)
			p.keyCount++

			p.skipWhitespace()
			if p.pos >= p.len || p.src[p.pos] != ':' {
				return 0, errors.New("expected ':' after object key")
			}
			p.pos++

			childID, err := p.parseValue(childKey, id, depth+1)
			if err != nil {
				return 0, err
			}
			if lastChild < 0 {
				p.nodes[id].FirstChild = childID
			} else {
				p.nodes[lastChild].NextSibling = childID
			}
			lastChild = childID
			count++

			p.skipWhitespace()
			if p.pos >= p.len {
				return 0, errors.New("unterminated object")
			}
			if p.src[p.pos] == ',' {
				p.pos++
				continue
			} else if p.src[p.pos] == '}' {
				p.pos++
				break
			} else {
				return 0, errors.New("expected ',' or '}' in object")
			}
		}
		p.nodes[id].ChildCount = count
		p.nodes[id].End = p.pos
		p.nodes[id].Raw = p.src[start:p.pos]
		return id, nil

	case '[':
		p.pos++
		p.nodes = append(p.nodes, document.JSONNode{
			Key: key, Kind: "array", Parent: parent, Depth: depth, Start: start,
			FirstChild: -1, NextSibling: -1,
		})
		lastChild := -1
		count := 0
		index := 0
		for {
			p.skipWhitespace()
			if p.pos >= p.len {
				return 0, errors.New("unterminated array")
			}
			if p.src[p.pos] == ']' {
				p.pos++
				break
			}
			childKey := arrayIndexStr(index)
			childID, err := p.parseValue(childKey, id, depth+1)
			if err != nil {
				return 0, err
			}
			if lastChild < 0 {
				p.nodes[id].FirstChild = childID
			} else {
				p.nodes[lastChild].NextSibling = childID
			}
			lastChild = childID
			count++
			index++

			p.skipWhitespace()
			if p.pos >= p.len {
				return 0, errors.New("unterminated array")
			}
			if p.src[p.pos] == ',' {
				p.pos++
				continue
			} else if p.src[p.pos] == ']' {
				p.pos++
				break
			} else {
				return 0, errors.New("expected ',' or ']' in array")
			}
		}
		p.nodes[id].ChildCount = count
		p.nodes[id].End = p.pos
		p.nodes[id].Raw = p.src[start:p.pos]
		return id, nil

	case '"':
		disp, rawStart, rawEnd, err := p.parseString()
		if err != nil {
			return 0, err
		}
		if len(disp) <= 64 {
			disp = p.internStr(disp)
		}
		p.nodes = append(p.nodes, document.JSONNode{
			Key: key, Kind: "string", Raw: p.src[rawStart:rawEnd], Display: disp,
			Parent: parent, Depth: depth, Start: rawStart, End: rawEnd,
			FirstChild: -1, NextSibling: -1,
		})
		return id, nil

	case 't':
		if err := p.parseLiteral("true"); err != nil {
			return 0, err
		}
		p.nodes = append(p.nodes, document.JSONNode{
			Key: key, Kind: "boolean", Raw: "true", Display: "true",
			Parent: parent, Depth: depth, Start: start, End: p.pos,
			FirstChild: -1, NextSibling: -1,
		})
		return id, nil

	case 'f':
		if err := p.parseLiteral("false"); err != nil {
			return 0, err
		}
		p.nodes = append(p.nodes, document.JSONNode{
			Key: key, Kind: "boolean", Raw: "false", Display: "false",
			Parent: parent, Depth: depth, Start: start, End: p.pos,
			FirstChild: -1, NextSibling: -1,
		})
		return id, nil

	case 'n':
		if err := p.parseLiteral("null"); err != nil {
			return 0, err
		}
		p.nodes = append(p.nodes, document.JSONNode{
			Key: key, Kind: "null", Raw: "null", Display: "null",
			Parent: parent, Depth: depth, Start: start, End: p.pos,
			FirstChild: -1, NextSibling: -1,
		})
		return id, nil

	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		disp, rawStart, rawEnd, err := p.parseNumber()
		if err != nil {
			return 0, err
		}
		p.nodes = append(p.nodes, document.JSONNode{
			Key: key, Kind: "number", Raw: disp, Display: disp,
			Parent: parent, Depth: depth, Start: rawStart, End: rawEnd,
			FirstChild: -1, NextSibling: -1,
		})
		return id, nil

	default:
		return 0, fmt.Errorf("unexpected character '%c'", b)
	}
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
