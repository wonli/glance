package desktopui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	agentProcessHeaderTextColor = color.NRGBA{R: 122, G: 135, B: 151, A: 255}
	agentProcessHeaderIconColor = color.NRGBA{R: 152, G: 161, B: 173, A: 255}
	agentProcessLeftBarColor    = color.NRGBA{R: 205, G: 213, B: 223, A: 255}
	agentProcessPrimaryColor    = color.NRGBA{R: 133, G: 145, B: 159, A: 255}
	agentProcessSecondaryColor  = color.NRGBA{R: 146, G: 156, B: 170, A: 255}
	agentProcessErrorColor      = color.NRGBA{R: 178, G: 54, B: 54, A: 255}
	agentProcessErrorBarColor   = color.NRGBA{R: 214, G: 108, B: 108, A: 255}
	agentProcessAuthColor       = color.NRGBA{R: 153, G: 105, B: 34, A: 255}
	agentProcessAuthBarColor    = color.NRGBA{R: 224, G: 180, B: 95, A: 255}
)

func agentSidebarMessageGroups(lines []agentChatLine) [][2]int {
	groups := make([][2]int, 0, len(lines))
	for i := 0; i < len(lines); {
		end := i + 1
		if lines[i].role != "user" {
			for end < len(lines) && lines[end].role != "user" {
				end++
			}
		}
		groups = append(groups, [2]int{i, end})
		i = end
	}
	return groups
}

func isAgentProcessLine(line agentChatLine) bool {
	return line.role == "reasoning" || line.role == "tool"
}

func agentSubTurnBounds(lines []agentChatLine, index int) (int, int) {
	start := index
	for start > 0 {
		prev := lines[start-1]
		if prev.role == "user" || (prev.role == "assistant" && strings.TrimSpace(prev.text) != "") {
			break
		}
		start--
	}
	end := index + 1
	for end < len(lines) {
		curr := lines[end]
		if curr.role == "user" || (curr.role == "assistant" && strings.TrimSpace(curr.text) != "") {
			break
		}
		end++
	}
	return start, end
}

func collectSubTurnProcessLines(lines []agentChatLine, index int) []int {
	if index < 0 || index >= len(lines) || !isAgentProcessLine(lines[index]) {
		return nil
	}
	start, end := agentSubTurnBounds(lines, index)
	indices := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		if isAgentProcessLine(lines[i]) {
			indices = append(indices, i)
		}
	}
	return indices
}

func subTurnHasConfirmationLine(lines []agentChatLine, index int) bool {
	start, end := agentSubTurnBounds(lines, index)
	for i := start; i < end; i++ {
		if lines[i].role == "confirmation" {
			return true
		}
	}
	return false
}

func agentProcessGroupCollapsed(lines []agentChatLine, indices []int) bool {
	for _, idx := range indices {
		line := lines[idx]
		if (line.toggle != nil || line.role == "reasoning") && !line.collapsed {
			return false
		}
	}
	return true
}

func agentProcessGroupHeaderTitle(lines []agentChatLine, indices []int, now time.Time) string {
	toolCount := 0
	var totalDuration time.Duration
	lastInProgress := -1
	for _, idx := range indices {
		line := lines[idx]
		if line.role == "tool" {
			toolCount++
		}
		if line.duration > 0 {
			totalDuration += line.duration
		} else if line.inProgress && !line.startedAt.IsZero() && !now.IsZero() && now.After(line.startedAt) {
			totalDuration += now.Sub(line.startedAt)
		}
		if line.inProgress {
			lastInProgress = idx
		}
	}
	seconds := int((totalDuration + 500*time.Millisecond) / time.Second)
	if lastInProgress >= 0 {
		active := lines[lastInProgress]
		if active.role == "tool" {
			name := active.toolName
			if name == "" {
				name = strings.TrimSpace(strings.TrimPrefix(active.text, "正在执行"))
			}
			if name != "" {
				return "处理过程 · 正在执行 " + agentToolDisplayName(name)
			}
			return "处理过程 · 正在调用工具…"
		}
		title := "处理过程 · 正在分析…"
		if seconds > 0 {
			title += fmt.Sprintf(" · %d 秒", seconds)
		}
		if toolCount > 0 {
			title += fmt.Sprintf(" · %d 次调用", toolCount)
		}
		return title
	}
	title := "处理过程"
	if seconds > 0 {
		title += fmt.Sprintf(" · %d 秒", seconds)
	}
	if toolCount > 0 {
		title += fmt.Sprintf(" · %d 次调用", toolCount)
	}
	return title
}

func agentSidebarProcessDisclosure(gtx layout.Context, theme *material.Theme, button *widget.Clickable, title string, collapsed bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(20))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("展开或折叠处理过程").Add(gtx.Ops)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				label := material.Label(theme, unit.Sp(11), title)
				label.Color = agentProcessHeaderTextColor
				label.MaxLines = 1
				return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, label.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				icon := "triangle-up"
				if collapsed {
					icon = "triangle-down"
				}
				return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return homeIcon(gtx, icon, 12, agentProcessHeaderIconColor)
				})
			}),
		)
	})
}

func agentProcessLeftBar(gtx layout.Context, barColor color.NRGBA, child layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		width := max(1, gtx.Dp(unit.Dp(2)))
		paint.FillShape(gtx.Ops, barColor, clip.Rect{Max: image.Pt(width, size.Y)}.Op())
		return layout.Dimensions{Size: size}
	}, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, child)
	})
}

func agentSidebarResumeButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(42)), gtx.Dp(unit.Dp(30)))
	gtx.Constraints = layout.Exact(size)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("继续尚未完成的会话").Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Label(theme, unit.Sp(11), "继续")
			label.Color = agentSidebarText
			gtx.Constraints.Min = image.Point{}
			return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, label.Layout)
		})
	})
}

var agentKnownToolNames = map[string]string{
	"document.list":                "获取文档列表",
	"document.inspect":             "读取文档内容",
	"document.create":              "创建新文档",
	"document.open":                "打开文档",
	"document.save":                "保存文档",
	"document.save_as":             "另存为文档",
	"document.close":               "关闭文档",
	"confirmation.get":             "查询确认状态",
	"workbook.audit":               "检查工作簿",
	"spreadsheet.cell.set":         "修改单元格内容",
	"spreadsheet.range.copy":       "复制表格区域",
	"spreadsheet.range.clear":      "清空表格区域",
	"word.paragraph.append":        "追加文档段落",
	"word.text_range.replace":      "替换文档文本",
	"word.text_range.copy":         "复制文档文本",
	"word.text_range.cut":          "剪切文档文本",
	"word.text.search":             "搜索文档文本",
	"word.text.statistics":         "统计文档字数",
	"word.table.insert":            "插入文档表格",
	"word.image.insert":            "插入文档图片",
	"word.document.properties.get": "获取文档属性",
	"word.document.properties.set": "设置文档属性",
}

func agentToolDisplayName(name string) string {
	trimmed := strings.TrimSpace(name)
	if label, ok := agentKnownToolNames[trimmed]; ok {
		return label
	}
	return trimmed
}

func agentToolRecordDisplay(line agentChatLine) (string, string) {
	if line.toolName != "" {
		title := agentToolDisplayName(line.toolName)
		summary := strings.TrimSpace(line.toolSummary)
		if summary == "" {
			switch {
			case line.inProgress:
				summary = "正在执行…"
			case line.needsAuth:
				summary = "等待人工授权确认"
			case line.isError:
				summary = "执行失败"
			default:
				summary = "已完成"
			}
		}
		return title, summary
	}
	text := strings.TrimSpace(line.text)
	if parts := strings.SplitN(text, "\n", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if strings.HasPrefix(text, "工具 ") && strings.HasSuffix(text, " 已完成") {
		rawName := strings.TrimSuffix(strings.TrimPrefix(text, "工具 "), " 已完成")
		return agentToolDisplayName(rawName), "已完成"
	}
	if strings.HasPrefix(text, "正在执行 ") {
		rawName := strings.TrimPrefix(text, "正在执行 ")
		return agentToolDisplayName(rawName), "正在执行…"
	}
	return text, strings.TrimSpace(line.toolSummary)
}

func agentToolResultHasError(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var payload struct {
		Error   string `json:"error"`
		IsError bool   `json:"is_error"`
	}
	if json.Unmarshal(raw, &payload) == nil {
		return payload.Error != "" || payload.IsError
	}
	return false
}

func agentToolResultNeedsConfirmation(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) == nil {
		if envelope.Status == "confirmation_required" || envelope.Data.Status == "confirmation_required" {
			return true
		}
	}
	return strings.Contains(string(raw), `"confirmation_required"`)
}

func summarizeAgentToolResult(toolName string, raw []byte, isError bool) string {
	if agentToolResultNeedsConfirmation(raw) {
		return "等待人工授权确认"
	}
	if isError {
		var errPayload struct {
			Message string   `json:"message"`
			Error   string   `json:"error"`
			Content []string `json:"content"`
		}
		if len(raw) > 0 && json.Unmarshal(raw, &errPayload) == nil {
			if strings.TrimSpace(errPayload.Message) != "" {
				return strings.TrimSpace(errPayload.Message)
			}
			if len(errPayload.Content) > 0 && strings.TrimSpace(errPayload.Content[0]) != "" {
				return strings.TrimSpace(errPayload.Content[0])
			}
		}
		return "执行失败，Agent 将根据错误继续处理"
	}
	if len(raw) > 0 {
		var payload struct {
			Data struct {
				CellsScanned  int             `json:"cells_scanned"`
				FindingsTotal int             `json:"findings_total"`
				Documents     []any           `json:"documents"`
				SourcePath    string          `json:"source_path"`
				Format        string          `json:"format"`
				Text          string          `json:"text"`
				Content       json.RawMessage `json:"content"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &payload) == nil {
			switch toolName {
			case "document.list":
				if len(payload.Data.Documents) > 0 {
					return fmt.Sprintf("已找到 %d 份当前文档", len(payload.Data.Documents))
				}
			case "workbook.audit":
				if payload.Data.CellsScanned > 0 {
					return fmt.Sprintf("已检查 %d 个单元格", payload.Data.CellsScanned)
				}
			case "document.save", "document.save_as":
				if payload.Data.SourcePath != "" {
					return "已保存文档"
				}
			case "document.create":
				return "已创建新文档"
			case "document.inspect":
				return "已读取当前文档内容"
			case "word.paragraph.append":
				return "已更新文档段落"
			}
		}
	}
	return "已完成"
}
