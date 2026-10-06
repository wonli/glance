package desktopui

import (
	"glance/internal/format/jsondoc"
)

type jsonToolResult struct {
	id, source, text string
	err              error
}

// Transform an immutable snapshot off the window loop. A newer source draft
// cancels installation, without reserving a session mutation while parsing.
func (a *App) startJSONTool(id string) {
	v := a.ensureJSONView()
	if v.toolPending {
		return
	}
	source := a.jsonDraftText()
	v.toolPending = true
	results, window := v.toolResults, a.window
	go func() {
		result := jsonToolResult{id: id, source: source}
		switch id {
		case "format":
			result.text, result.err = jsondoc.Format(source)
		case "minify":
			result.text, result.err = jsondoc.Minify(source)
		case "sort-keys":
			result.text, result.err = jsondoc.SortKeys(source)
		case "validate":
			result.err = jsondoc.Validate(source)
		}
		results <- result
		if window != nil {
			window.Invalidate()
		}
	}()
}

func (a *App) applyJSONToolResult(result jsonToolResult) {
	v := a.ensureJSONView()
	v.toolPending = false
	if a.jsonDraftText() != result.source || a.jsonDoc.Text() != result.source {
		return
	}
	label := map[string]string{"format": "格式化", "minify": "压缩", "sort-keys": "排序键", "validate": "验证"}[result.id]
	if result.err != nil {
		a.setStatus("JSON " + label + "失败：" + result.err.Error())
		v.toolMessage = a.status
		return
	}
	if result.id == "validate" {
		a.setStatus("JSON 格式正确")
		v.toolMessage = a.status
		return
	}
	if a.applyJSONTextMutationIfSource(result.text, "已"+label+" JSON", result.source) {
		v.toolMessage = "已" + label + " JSON"
	} else {
		v.toolMessage = a.status
	}
}

func (a *App) pollJSONToolResults() {
	v := a.ensureJSONView()
	select {
	case result := <-v.toolResults:
		a.applyJSONToolResult(result)
	default:
	}
}
