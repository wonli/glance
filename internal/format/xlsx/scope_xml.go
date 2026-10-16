package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// scopeNode retains the exact lexical XML for untouched worksheet content.
// Only edited start tags and explicitly edited child elements are rewritten.
type scopeNode struct {
	name        string
	open, close string
	parts       []scopePart
	attrs       map[string]string
	remove      bool
}
type scopePart struct {
	raw  string
	node *scopeNode
}

func parseScopeXML(data []byte) (*scopeNode, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	root := &scopeNode{}
	stack := []*scopeNode{root}
	for {
		before := int(d.InputOffset())
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		raw := string(data[before:int(d.InputOffset())])
		parent := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			n := &scopeNode{name: t.Name.Local, open: raw, attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[a.Name.Local] = a.Value
			}
			parent.parts = append(parent.parts, scopePart{node: n})
			stack = append(stack, n)
		case xml.EndElement:
			parent.close = raw
			stack = stack[:len(stack)-1]
		default:
			parent.parts = append(parent.parts, scopePart{raw: raw})
		}
	}
	return root, nil
}
func (n *scopeNode) children(name string) []*scopeNode {
	var out []*scopeNode
	for _, p := range n.parts {
		if p.node != nil && p.node.name == name && !p.node.remove {
			out = append(out, p.node)
		}
	}
	return out
}
func (n *scopeNode) child(name string) *scopeNode {
	v := n.children(name)
	if len(v) > 0 {
		return v[0]
	}
	return nil
}
func (n *scopeNode) integer(name string) int { v, _ := strconv.Atoi(n.attrs[name]); return v }
func (n *scopeNode) set(name, value string) {
	if n.attrs[name] == value {
		return
	}
	n.attrs[name] = value
	re := regexp.MustCompile(`\s` + regexp.QuoteMeta(name) + `\s*=\s*(?:"[^"]*"|'[^']*')`)
	attr := " " + name + `="` + value + `"`
	if re.MatchString(n.open) {
		n.open = re.ReplaceAllString(n.open, attr)
	} else {
		idx := strings.LastIndex(n.open, ">")
		if idx > 0 && n.open[idx-1] == '/' {
			idx--
		}
		n.open = n.open[:idx] + attr + n.open[idx:]
	}
}
func newScopeNode(name string) *scopeNode {
	return &scopeNode{name: name, open: "<" + name + ">", close: "</" + name + ">", attrs: map[string]string{}}
}
func (n *scopeNode) qualifiedName() string {
	name := strings.TrimPrefix(n.open, "<")
	if end := strings.IndexAny(name, " \t\r\n/>"); end >= 0 {
		name = name[:end]
	}
	return name
}

// New worksheet children reuse their parent's namespace prefix; no default
// namespace is assumed for imported prefixed XML.
func newScopeChild(parent *scopeNode, name string) *scopeNode {
	n := newScopeNode(name)
	if qualified := parent.qualifiedName(); strings.Contains(qualified, ":") {
		prefix := qualified[:strings.IndexByte(qualified, ':')+1]
		n.open = "<" + prefix + name + ">"
		n.close = "</" + prefix + name + ">"
	}
	return n
}

// Insert among existing numeric row/cell records without moving whitespace or
// unrelated XML nodes. Sorting parts directly is invalid around raw XML parts.
func insertScopeChild(parent, child *scopeNode, key func(*scopeNode) int) {
	index := len(parent.parts)
	for i, part := range parent.parts {
		if part.node == nil || part.node.name != child.name {
			continue
		}
		index = i + 1
		if key(part.node) > key(child) {
			index = i
			break
		}
	}
	parent.parts = append(parent.parts, scopePart{})
	copy(parent.parts[index+1:], parent.parts[index:])
	parent.parts[index] = scopePart{node: child}
}

func (n *scopeNode) render(b *strings.Builder) {
	if n.remove {
		return
	}
	open := n.open
	if len(n.parts) > 0 && strings.HasSuffix(open, "/>") {
		open = strings.TrimSuffix(open, "/>") + ">"
	}
	b.WriteString(open)
	for _, p := range n.parts {
		b.WriteString(p.raw)
		if p.node != nil {
			p.node.render(b)
		}
	}
	if n.close != "" {
		b.WriteString(n.close)
	} else if len(n.parts) > 0 && n.name != "" {
		b.WriteString("</" + n.qualifiedName() + ">")
	}
}
func (n *scopeNode) bytes() []byte { var b strings.Builder; n.render(&b); return []byte(b.String()) }

func scopePackage(data []byte, sheet string) (map[string][]byte, string, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", err
	}
	files := map[string][]byte{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			return nil, "", err
		}
		v, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return nil, "", err
		}
		files[f.Name] = v
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err = xml.Unmarshal(files["xl/workbook.xml"], &wb); err != nil {
		return nil, "", err
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err = xml.Unmarshal(files["xl/_rels/workbook.xml.rels"], &rels); err != nil {
		return nil, "", err
	}
	for _, s := range wb.Sheets {
		if s.Name == sheet {
			for _, r := range rels.Items {
				if r.ID == s.ID {
					target := strings.TrimPrefix(r.Target, "/")
					if !strings.HasPrefix(target, "xl/") {
						target = path.Join("xl", target)
					}
					return files, target, nil
				}
			}
		}
	}
	return nil, "", fmt.Errorf("worksheet %q missing package relationship", sheet)
}
func replaceScopePackage(snapshot []byte, files map[string][]byte) ([]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(snapshot), int64(len(snapshot)))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	out := zip.NewWriter(&b)
	for _, f := range z.File {
		payload, ok := files[f.Name]
		if !ok {
			continue
		}
		header := f.FileHeader
		header.CRC32 = 0
		header.CompressedSize64 = 0
		header.UncompressedSize64 = 0
		dst, err := out.CreateHeader(&header)
		if err != nil {
			return nil, err
		}
		if _, err = dst.Write(payload); err != nil {
			return nil, err
		}
	}
	if err = out.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
