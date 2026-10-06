package markdowndoc

import (
	"fmt"
	"regexp"
	"strings"
)

type EditArgs struct {
	Label, Destination string
	Display            bool
}
type EditResult struct {
	Text       string
	Start, End int
}

// ApplyEdit uses rune positions, as Gio editors do, never UTF-8 byte offsets.
func ApplyEdit(source, action string, start, end int, args EditArgs) (EditResult, error) {
	r := []rune(source)
	if start > end {
		start, end = end, start
	}
	if start < 0 || end > len(r) {
		return EditResult{}, fmt.Errorf("selection outside document")
	}
	selected := string(r[start:end])
	before, after := string(r[:start]), string(r[end:])
	left, right := "", ""
	switch action {
	case "bold":
		left, right = "**", "**"
	case "italic":
		left, right = "*", "*"
	case "code":
		if strings.Contains(selected, "\n") {
			fence := "```"
			for strings.Contains(selected, fence) {
				fence += "`"
			}
			left, right = fence+"\n", "\n"+fence
			if before != "" && !strings.HasSuffix(before, "\n") {
				left = "\n" + left
			}
			if after != "" && !strings.HasPrefix(after, "\n") {
				right += "\n"
			}
		} else {
			left, right = "`", "`"
			for strings.Contains(selected, left) {
				left += "`"
				right = left
			}
			if strings.HasPrefix(selected, "`") || strings.HasSuffix(selected, "`") {
				left += " "
				right = " " + right
			}
		}
	case "math":
		if args.Display {
			left, right = "$$\n", "\n$$"
		} else {
			left, right = "$", "$"
		}
	case "link", "image":
		if strings.ContainsAny(args.Destination, "\r\n") {
			return EditResult{}, fmt.Errorf("destination contains newline")
		}
		if args.Label != "" {
			selected = args.Label
		}
		selected = strings.ReplaceAll(strings.ReplaceAll(selected, "[", `\[`), "]", `\]`)
		left = "["
		if action == "image" {
			left = "!["
		}
		destination := args.Destination
		if len(destination) >= 3 && destination[1] == ':' && (destination[2] == '\\' || destination[2] == '/') {
			destination = "file:///" + strings.ReplaceAll(destination, "\\", "/")
		}
		if strings.ContainsAny(destination, " \t") {
			destination = "<" + strings.ReplaceAll(strings.ReplaceAll(destination, "<", "%3C"), ">", "%3E") + ">"
		} else {
			destination = strings.ReplaceAll(strings.ReplaceAll(destination, "(", `\(`), ")", `\)`)
		}
		right = "](" + destination + ")"
	case "heading", "quote", "list", "todo":
		// Expand to complete selected lines, excluding a following line selected only at its start.
		for start > 0 && r[start-1] != '\n' {
			start--
		}
		if start == 0 && len(r) > 0 && r[0] == '\ufeff' {
			start = 1
		}
		if end < start {
			end = start
		}
		if end > start && r[end-1] == '\n' {
			end--
		}
		for end < len(r) && r[end] != '\n' {
			end++
		}
		lines := strings.Split(string(r[start:end]), "\n")
		prefix := map[string]string{"heading": "# ", "quote": "> ", "list": "- ", "todo": "- [ ] "}[action]
		for i, line := range lines {
			if action == "todo" {
				if match := taskMarker.FindStringSubmatchIndex(line); match != nil {
					at := match[2]
					state := "x"
					if line[at] == 'x' || line[at] == 'X' {
						state = " "
					}
					lines[i] = line[:at] + state + line[at+1:]
					continue
				}
			}

			if action == "heading" {
				n := 0
				for n < len(line) && line[n] == '#' {
					n++
				}
				if n > 0 && n <= 6 && strings.HasPrefix(line[n:], " ") {
					level := n%6 + 1
					line = strings.TrimPrefix(line[n:], " ")
					lines[i] = strings.Repeat("#", level) + " " + line
					continue
				}
			}
			lines[i] = prefix + line
		}
		text := strings.Join(lines, "\n")
		return EditResult{Text: string(r[:start]) + text + string(r[end:]), Start: start, End: start + len([]rune(text))}, nil
	default:
		return EditResult{}, fmt.Errorf("unknown Markdown action %q", action)
	}
	return EditResult{Text: before + left + selected + right + after, Start: start + len([]rune(left)), End: start + len([]rune(left+selected))}, nil
}

var taskMarker = regexp.MustCompile(`^(?:[ \t]*>[ \t]*)*[ \t]*(?:[-+*]|[0-9]+[.)])[ \t]+\[([ xX])\]`)

// ToggleTask changes just the checked marker at a parser-provided rune offset.
func ToggleTask(source string, offset int) (string, error) {
	r := []rune(source)
	if offset < 1 || offset+1 >= len(r) || r[offset-1] != '[' || r[offset+1] != ']' {
		return "", fmt.Errorf("待办位置已改变")
	}
	switch r[offset] {
	case ' ':
		r[offset] = 'x'
	case 'x', 'X':
		r[offset] = ' '
	default:
		return "", fmt.Errorf("无效待办状态")
	}
	return string(r), nil
}
