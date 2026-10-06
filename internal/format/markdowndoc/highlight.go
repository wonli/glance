package markdowndoc

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"glance/internal/document"
	"strings"
	"time"
	"unicode/utf8"
)

const maxCodeHighlightBytes = 1 << 20
const maxCodeHighlightTokens = 100000
const codeHighlightBudget = 250 * time.Millisecond

// HighlightCode produces neutral, literal code spans for background parsing.
// Unknown languages and work-limit failures retain the exact original source.
// Chroma's regex lexer separately bounds each regex match to 250 ms.
func HighlightCode(source, language string) (spans []document.MarkdownSpan) {
	plain := []document.MarkdownSpan{{Kind: "code", Text: source}}
	spans = plain
	defer func() {
		if recover() != nil {
			spans = plain
		}
	}()
	if source == "" || len(source) > maxCodeHighlightBytes || !utf8.ValidString(source) {
		return plain
	}
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "c#" {
		language = "csharp"
	}
	if language == "" {
		return plain
	}
	lexer := lexers.Get(language)
	if lexer == nil {
		return plain
	}
	deadline := time.Now().Add(codeHighlightBudget)
	// Nested avoids the lexer's implicit final newline. EnsureLF remains false:
	// CRLF, isolated CR, tabs and BOM are part of the user's literal code.
	iterator, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", Nested: true}, source)
	if err != nil {
		return plain
	}
	result := make([]document.MarkdownSpan, 0, 32)
	var run strings.Builder
	colour := ""
	flush := func() {
		if run.Len() > 0 {
			result = append(result, document.MarkdownSpan{Kind: "code", Text: run.String(), CodeColor: colour})
			run.Reset()
		}
	}
	consumed := 0
	for count := 0; ; count++ {
		if time.Now().After(deadline) {
			return plain
		}
		token := iterator()
		if token == chroma.EOF {
			break
		}
		if count >= maxCodeHighlightTokens || !strings.HasPrefix(source[consumed:], token.Value) {
			return plain
		}
		consumed += len(token.Value)
		if token.Value == "" {
			continue
		}
		nextColour := codeTokenColour(token.Type)
		if nextColour != colour {
			flush()
			colour = nextColour
		}
		run.WriteString(token.Value)
	}
	flush()
	if consumed != len(source) || len(result) == 0 {
		return plain
	}
	return result
}

func codeTokenColour(token chroma.TokenType) string {
	switch {
	case token.InCategory(chroma.Comment):
		return "64748b"
	case token.InCategory(chroma.Keyword):
		return "7c3aed"
	case token.InSubCategory(chroma.LiteralString):
		return "15803d"
	case token.InSubCategory(chroma.LiteralNumber):
		return "b45309"
	case token.InSubCategory(chroma.NameFunction), token.InSubCategory(chroma.NameBuiltin), token == chroma.NameClass:
		return "2563eb"
	case token.InSubCategory(chroma.NameVariable), token == chroma.NameAttribute, token == chroma.NameProperty, token == chroma.NameTag:
		return "9f1239"
	default:
		return "334155"
	}
}
