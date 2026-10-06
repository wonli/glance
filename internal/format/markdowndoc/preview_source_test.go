package markdowndoc

import (
	"strings"
	"testing"
)

func TestMarkdownInteractiveSourceRanges(t *testing.T) {
	for _, source := range []string{"\ufeff中文 $x^2$ 后文", "前文\r\n后行 $y_1$", "前文  \r\n后行 $y_1$", "前文\\\n后行 $y_1$", "**前文** $y_1$", "> - [ ] 子项\r\n", "中文\n\n$$\r\nx^2\r\n$$\r\n"} {
		p, err := Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		r := []rune(source)
		for _, b := range p.Blocks {
			if b.Kind == "math" {
				raw := string(r[b.SourceStart:b.SourceEnd])
				if !strings.HasPrefix(raw, "$$") || !strings.HasSuffix(raw, "$$") || strings.TrimSpace(raw[2:len(raw)-2]) != b.Text {
					t.Errorf("block range %q in %q", raw, source)
				}
			}
			for _, s := range b.Spans {
				if s.Kind == "math" {
					raw := string(r[s.SourceStart:s.SourceEnd])
					if raw != "$"+s.Text+"$" {
						t.Errorf("math range %q wanted %q in %q", raw, s.Text, source)
					}
				}
				if s.Kind == "task" {
					if r[s.SourceStart-1] != '[' || r[s.SourceEnd] != ']' {
						t.Errorf("task range in %q", source)
					}
				}
			}
		}
	}
}
