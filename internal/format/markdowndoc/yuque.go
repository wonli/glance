package markdowndoc

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	yuqueDiagramRegex = regexp.MustCompile(`(?s)<!--\s*这是一个文本绘图[，,]源码为：(.*?)(?:-->\s*)?!\[[^\]]*\]\((https?://[^\s)]+yuque[^\s)]+)\)`)
	yuqueMathBlockRegex = regexp.MustCompile(`(?m)^\$\s*(\\(?:begin|bmatrix|align|matrix|cases|gather)[\s\S]*?\\(?:end|bmatrix|align|matrix|cases|gather)\s*(?:[^\$\n]*))\s*\$$`)
)

// normalizeYuque converts Yuque-exported proprietary diagram and math patterns
// into standard Markdown fenced blocks.
func normalizeYuque(source string) string {
	if strings.Contains(source, "这是一个文本绘图") {
		source = yuqueDiagramRegex.ReplaceAllStringFunc(source, func(match string) string {
			sub := yuqueDiagramRegex.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			code := strings.TrimSpace(sub[1])
			for strings.HasSuffix(code, "-->") {
				code = strings.TrimSpace(strings.TrimSuffix(code, "-->"))
			}
			if code == "" {
				return match
			}
			return fmt.Sprintf("\n```mermaid\n%s\n```\n", code)
		})
	}

	if strings.Contains(source, `\begin`) {
		source = yuqueMathBlockRegex.ReplaceAllStringFunc(source, func(match string) string {
			sub := yuqueMathBlockRegex.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			content := strings.TrimSpace(sub[1])
			return fmt.Sprintf("$$\n%s\n$$", content)
		})
	}

	return source
}
