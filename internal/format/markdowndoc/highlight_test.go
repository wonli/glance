package markdowndoc

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestHighlightCodePreservesLiteralSourceAndColoursLanguages(t *testing.T) {
	for _, tc := range []struct{ language, source, keyword string }{
		{"c#", "\ufeffusing System;\r\nclass Example { string text = \"中文\"; }\r\n", "class"},
		{"css", "/* 中文 */\r\nbody { color: #ff0000; }\r\n", "color"},
		{"javascript", "// $x$ [link](url) ![image](url)\r\nconst text = \"中文\";\r\n", "const"},
		{"php", "namespace App;\r\nclass Example { public function run() { echo \"中文\"; } }\r\n", "class"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			spans := HighlightCode(tc.source, tc.language)
			var text strings.Builder
			colouredKeyword := false
			for _, span := range spans {
				text.WriteString(span.Text)
				if span.Kind != "code" || span.Destination != "" || span.LinkDestination != "" {
					t.Fatalf("code interpreted as Markdown: %+v", span)
				}
				if span.CodeColor != "" && span.CodeColor != "334155" && strings.Contains(span.Text, tc.keyword) {
					colouredKeyword = true
				}
				if _, err := hex.DecodeString(span.CodeColor); err != nil || len(span.CodeColor) != 6 {
					t.Fatalf("invalid neutral colour: %q", span.CodeColor)
				}
			}
			if text.String() != tc.source {
				t.Fatalf("source changed: %q", text.String())
			}
			if !colouredKeyword {
				t.Fatalf("%s code was not highlighted: %+v", tc.language, spans)
			}
		})
	}
}

func TestHighlightCodeFallsBackWithoutChangingSource(t *testing.T) {
	for _, tc := range []struct{ source, language string }{
		{"\ufeff\t中文\r\n  $x$ [link](url)\r\n", "unknown-language"},
		{"``` **literal**\r\n", ""},
		{"", "javascript"},
		{"\xff\r\n", "javascript"},
		{strings.Repeat("a", 1<<20+1), "javascript"},
		{strings.Repeat("+ ", 100001), "javascript"},
	} {
		spans := HighlightCode(tc.source, tc.language)
		if len(spans) != 1 || spans[0].Text != tc.source || spans[0].Kind != "code" || spans[0].CodeColor != "" {
			t.Fatalf("fallback changed source or retained partial colours: %d spans", len(spans))
		}
	}
}

func TestHighlightCodeExternalAcceptanceFile(t *testing.T) {
	path := os.Getenv("GLANCE_MARKDOWN_ACCEPTANCE_SOURCE")
	if path == "" {
		t.Skip("optional user acceptance file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, block := range p.Blocks {
		if block.Kind != "code" || block.Language == "" {
			continue
		}
		spans := HighlightCode(block.Text, block.Language)
		var text strings.Builder
		colours := map[string]bool{}
		for _, s := range spans {
			text.WriteString(s.Text)
			colours[s.CodeColor] = true
		}
		if text.String() != block.Text {
			t.Fatalf("%s lost literal code", block.Language)
		}
		if len(colours) < 2 {
			t.Fatalf("%s was not highlighted", block.Language)
		}
		count++
	}
	if count != 4 {
		t.Fatalf("acceptance highlighted blocks=%d want 4", count)
	}
}
