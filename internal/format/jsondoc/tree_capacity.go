package jsondoc

// countJSONNodes is an allocation-free lexical pass, not a validator. Strings
// followed by a colon are keys; containers and other tokens are values. The
// parser still validates the source. Exact capacity avoids both large unused
// tails and transient copies of the node array while parsing large documents.
func countJSONNodes(source string) int {
	nodes := 0
	for i := 0; i < len(source); {
		switch source[i] {
		case ' ', '\t', '\r', '\n', ':', ',', '}', ']':
			i++
		case '{', '[':
			nodes++
			i++
		case '"':
			i++
			for i < len(source) {
				c := source[i]
				i++
				if c == '\\' {
					if i < len(source) {
						i++
					}
				} else if c == '"' {
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
			for i < len(source) && !jsonTokenBoundary(source[i]) {
				i++
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
