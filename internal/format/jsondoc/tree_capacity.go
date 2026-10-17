package jsondoc

import "strings"

// countJSONNodes is an allocation-free lexical pass, not a validator. Strings
// followed by a colon are keys; containers and other tokens are values. The
// parser still validates the source. Exact capacity avoids both large unused
// tails and transient copies of the node array while parsing large documents.
func countJSONNodes(source string) int {
	return countJSONNodesWithCancel(source, nil)
}

func countJSONNodesWithCancel(source string, done <-chan struct{}) int {
	nodes := 0
	lastCheck := 0
	for i := 0; i < len(source); {
		if done != nil && i-lastCheck > 65536 {
			lastCheck = i
			select {
			case <-done:
				return 0
			default:
			}
		}
		switch source[i] {
		case ' ', '\t', '\r', '\n', ':', ',', '}', ']':
			i++
		case '{', '[':
			nodes++
			i++
		case '"':
			i++
			for i < len(source) {
				idx := strings.IndexAny(source[i:], "\"\\")
				if idx < 0 {
					i = len(source)
					break
				}
				i += idx
				if source[i] == '\\' {
					i += 2
				} else {
					i++
					break
				}
			}
			for i < len(source) && jsonWhitespace(source[i]) {
				i++
			}
			if i == len(source) || source[i] != ':' {
				nodes++
			}
		default:
			nodes++
			idx := strings.IndexAny(source[i:], " \t\r\n,:{}[]\"")
			if idx < 0 {
				i = len(source)
			} else {
				i += idx
			}
		}
	}
	return nodes
}

func jsonWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func jsonTokenBoundary(c byte) bool {
	return jsonWhitespace(c) || c == ',' || c == ':' || c == '{' || c == '}' || c == '[' || c == ']' || c == '"'
}
