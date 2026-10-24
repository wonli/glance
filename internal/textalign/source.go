package textalign

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Source indexes visible source characters. Offsets are Unicode rune offsets,
// including supplementary-plane Han characters, never bytes or UTF-16 units.
type Source struct {
	Text         []rune
	starts, ends []int
	headings     []Range
}

func fold(r rune) rune {
	if r >= 0xff01 && r <= 0xff5e {
		r -= 0xfee0
	}
	return unicode.ToLower(r)
}
func keep(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }
func normalized(s string) []rune {
	var out []rune
	for _, r := range s {
		r = fold(r)
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}
func NewSource(markdown string) (*Source, error) {
	if !utf8.ValidString(markdown) || len(markdown) > 8<<20 {
		return nil, errors.New("textalign: Markdown must be valid UTF-8 and at most 8 MiB")
	}
	raw := []byte(markdown)
	positions := make([]int, len(raw)+1)
	offset := 0
	for i, r := range markdown {
		for j := 0; j < utf8.RuneLen(r); j++ {
			positions[i+j] = offset
		}
		offset++
	}
	positions[len(raw)] = offset
	s := &Source{}
	headingStarts := map[ast.Node]int{}
	tree := goldmark.New().Parser().Parse(text.NewReader(raw))
	_ = ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := n.(*ast.Heading); ok {
			if entering {
				headingStarts[n] = len(s.Text)
			} else if len(s.Text) > headingStarts[n] {
				s.headings = append(s.headings, Range{headingStarts[n], len(s.Text)})
			}
			return ast.WalkContinue, nil
		}
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.Image, *ast.RawHTML, *ast.HTMLBlock:
			return ast.WalkSkipChildren, nil
		}
		if node, ok := n.(*ast.Text); ok {
			value := string(node.Segment.Value(raw))
			literal := false
			if _, ok := node.Parent().(*ast.CodeSpan); ok {
				literal = true
			}
			for i := 0; i < len(value); {
				r, size := utf8.DecodeRuneInString(value[i:])
				decoded := string(r)
				at := node.Segment.Start + i
				if value[i] == '&' && !literal {
					escapes := 0
					for j := at - 1; j >= 0 && raw[j] == '\\'; j-- {
						escapes++
					}
					if escapes%2 != 0 {
						i += size
						continue
					}
					if end := strings.IndexByte(value[i:min(len(value), i+40)], ';'); end > 0 {
						entity := value[i : i+end+1]
						if replacement, ok := markdownEntity(entity); ok {
							decoded, size = replacement, end+1
						}
					}
				}
				for _, r := range decoded {
					r = fold(r)
					if !keep(r) {
						continue
					}
					s.Text = append(s.Text, r)
					s.starts = append(s.starts, positions[at])
					s.ends = append(s.ends, positions[at+size])
				}
				i += size
			}
		}
		return ast.WalkContinue, nil
	})
	return s, nil
}

func markdownEntity(entity string) (string, bool) {
	name := entity[1 : len(entity)-1]
	if strings.HasPrefix(name, "#") {
		digits := name[1:]
		hex := false
		if strings.HasPrefix(digits, "x") || strings.HasPrefix(digits, "X") {
			hex = true
			digits = digits[1:]
		}
		limit := 7
		if hex {
			limit = 6
		}
		if len(digits) == 0 || len(digits) > limit {
			return "", false
		}
		for _, c := range []byte(digits) {
			if hex {
				if !util.IsHexDecimal(c) {
					return "", false
				}
			} else if c < '0' || c > '9' {
				return "", false
			}
		}
		return string(util.ResolveNumericReferences([]byte(entity))), true
	}
	if value, ok := util.LookUpHTML5EntityByName(name); ok {
		return string(value.Characters), true
	}
	return "", false
}

// indivisible prevents splitting an entity that expands to multiple letters.
func (s *Source) indivisible(start, end int) bool {
	return !(start > 0 && s.starts[start] < s.ends[start-1]) && !(end < len(s.Text) && s.starts[end] < s.ends[end-1])
}

// OriginalRange returns the smallest original-source span containing the indexed
// characters. Excluded markup inside that span is preserved, not declared matched.
func (s *Source) OriginalRange(start, end int) Range {
	if start < 0 || end <= start || end > len(s.Text) {
		return Range{}
	}
	return Range{s.starts[start], s.ends[end-1]}
}
