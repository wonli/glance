package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxJSONSearchNodes = 200_000
	maxJSONSearchDepth = 128
	maxJSONSearchHits  = 500
)

type jsonSearchInput struct {
	Query         string `json:"query"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type jsonSearchMatch struct {
	Pointer   string `json:"pointer"`
	Kind      string `json:"kind"`
	MatchedIn string `json:"matched_in"`
	Preview   string `json:"preview"`
}

type jsonSearchOutput struct {
	Matches      []jsonSearchMatch `json:"matches"`
	VisitedNodes int               `json:"visited_nodes"`
	Truncated    bool              `json:"truncated"`
}

type jsonSearchWalker struct {
	ctx           context.Context
	decoder       *json.Decoder
	query         string
	caseSensitive bool
	limit         int
	visited       int
	matches       []jsonSearchMatch
	truncated     bool
}

func registerJSONSearchCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "json.document.search", Name: "搜索 JSON 文档",
			Description: "在明确 JSON 文档的键名和值中查找文本，并返回 RFC 6901 JSON Pointer。搜索限制为 8 MiB、最多遍历 200000 个值、嵌套深度 128、最多返回 500 项；达到任一上限时 truncated=true。结果适合直接传给后续 JSON 节点读取或编辑能力。",
			Category:    CapabilityQuery, Formats: []Format{FormatJSON}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":512,"description":"要在 JSON 键名和值中查找的文本。"},"case_sensitive":{"type":"boolean","description":"是否区分大小写，默认 false。"},"limit":{"type":"integer","minimum":1,"maximum":500,"description":"最多返回的匹配项数，默认 100。"}},"required":["query"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"matches":{"type":"array","maxItems":500,"items":{"type":"object","properties":{"pointer":{"type":"string","description":"指向命中属性或值的 RFC 6901 JSON Pointer；根节点为空字符串。"},"kind":{"type":"string","enum":["property_name","string","number","boolean","null"]},"matched_in":{"type":"string","enum":["key","value"]},"preview":{"type":"string","maxLength":256}},"required":["pointer","kind","matched_in","preview"],"additionalProperties":false}},"visited_nodes":{"type":"integer","minimum":0,"maximum":200000},"truncated":{"type":"boolean"}},"required":["matches","visited_nodes","truncated"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input jsonSearchInput
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			input.Query = strings.TrimSpace(input.Query)
			if input.Query == "" || len([]rune(input.Query)) > 512 || input.Limit < 0 || input.Limit > maxJSONSearchHits {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if input.Limit == 0 {
				input.Limit = 100
			}
			access, ok := capability.Adapter.(JSONTextAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			source, err := access.ReadJSONText(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(source) > maxJSONCapabilityBytes {
				return CapabilityResult{}, ErrQueryResultTooLarge
			}
			output, err := searchJSONText(ctx, source, input)
			if err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(output, false)
		},
	})
}

func registerJSONReadTextCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "json.document.read_text", Name: "读取 JSON 原文",
			Description: "返回明确 JSON 文档的完整源文本，包括格式空白和无效内容。最多读取 8 MiB；需要按结构访问时优先使用 document.inspect 或 json.document.search。",
			Category:    CapabilityQuery, Formats: []Format{FormatJSON}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","maxLength":8388608},"content_bytes":{"type":"integer","minimum":0,"maximum":8388608}},"required":["text","content_bytes"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			access, ok := capability.Adapter.(JSONTextAccess)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			text, err := access.ReadJSONText(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(text) > maxJSONCapabilityBytes {
				return CapabilityResult{}, ErrQueryResultTooLarge
			}
			return capabilityJSONResult(struct {
				Text         string `json:"text"`
				ContentBytes int    `json:"content_bytes"`
			}{Text: text, ContentBytes: len(text)}, false)
		},
	})
}

func searchJSONText(ctx context.Context, source string, input jsonSearchInput) (jsonSearchOutput, error) {
	decoder := json.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()
	query := input.Query
	if !input.CaseSensitive {
		query = strings.ToLower(query)
	}
	walker := jsonSearchWalker{
		ctx: ctx, decoder: decoder, query: query, caseSensitive: input.CaseSensitive,
		limit: input.Limit, matches: make([]jsonSearchMatch, 0, min(input.Limit, 32)),
	}
	err := walker.walkValue(nil, 0)
	if err != nil && err != errJSONSearchStopped {
		return jsonSearchOutput{}, fmt.Errorf("invalid JSON document: %w", err)
	}
	if err == nil {
		var trailing json.RawMessage
		if decodeErr := decoder.Decode(&trailing); !errors.Is(decodeErr, io.EOF) {
			if decodeErr == nil {
				return jsonSearchOutput{}, fmt.Errorf("invalid JSON document: multiple top-level values")
			}
			return jsonSearchOutput{}, fmt.Errorf("invalid JSON document: %w", decodeErr)
		}
	}
	return jsonSearchOutput{Matches: walker.matches, VisitedNodes: walker.visited, Truncated: walker.truncated}, nil
}

var errJSONSearchStopped = fmt.Errorf("JSON search reached a safety limit")

func (w *jsonSearchWalker) walkValue(path []string, depth int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if depth > maxJSONSearchDepth || w.visited >= maxJSONSearchNodes {
		w.truncated = true
		return errJSONSearchStopped
	}
	w.visited++
	token, err := w.decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			for w.decoder.More() {
				keyToken, err := w.decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				childPath := appendPath(path, key)
				if w.contains(key) {
					if err := w.add(jsonSearchMatch{Pointer: jsonPointer(childPath), Kind: "property_name", MatchedIn: "key", Preview: limitJSONString(key)}); err != nil {
						return err
					}
				}
				if err := w.walkValue(childPath, depth+1); err != nil {
					return err
				}
			}
			_, err = w.decoder.Token()
			return err
		case '[':
			index := 0
			for w.decoder.More() {
				if err := w.walkValue(appendPath(path, fmt.Sprint(index)), depth+1); err != nil {
					return err
				}
				index++
			}
			_, err = w.decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
	}
	text, kind, ok := jsonSearchScalar(token)
	if ok && w.contains(text) {
		return w.add(jsonSearchMatch{Pointer: jsonPointer(path), Kind: kind, MatchedIn: "value", Preview: limitJSONString(text)})
	}
	return nil
}

func (w *jsonSearchWalker) contains(candidate string) bool {
	if !w.caseSensitive {
		candidate = strings.ToLower(candidate)
	}
	return strings.Contains(candidate, w.query)
}

func (w *jsonSearchWalker) add(match jsonSearchMatch) error {
	if len(w.matches) >= w.limit {
		w.truncated = true
		return errJSONSearchStopped
	}
	w.matches = append(w.matches, match)
	return nil
}

func jsonSearchScalar(token any) (text, kind string, ok bool) {
	switch value := token.(type) {
	case string:
		return value, "string", true
	case json.Number:
		return value.String(), "number", true
	case bool:
		return fmt.Sprint(value), "boolean", true
	case nil:
		return "null", "null", true
	default:
		return "", "", false
	}
}

func appendPath(path []string, part string) []string {
	child := make([]string, len(path)+1)
	copy(child, path)
	child[len(path)] = part
	return child
}

func jsonPointer(path []string) string {
	var builder strings.Builder
	for _, part := range path {
		builder.WriteByte('/')
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
		builder.WriteString(part)
	}
	return builder.String()
}

func limitJSONString(value string) string {
	runes := []rune(value)
	if len(runes) > 256 {
		return string(runes[:256])
	}
	return value
}
