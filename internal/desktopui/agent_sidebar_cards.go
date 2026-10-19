package desktopui

import (
	"encoding/json"
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

	"glance/internal/i18n"
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

func (a *App) agentProcessGroupHeaderTitle(lines []agentChatLine, indices []int, now time.Time) string {
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
			name := a.agentToolDisplayName(active.toolName)
			if name != "" {
				return a.textArgs("agent.process.running", i18n.Args{"Action": name})
			}
			return a.textID("agent.process.active_tool")
		}
		title := a.textID("agent.process.analyzing")
		if seconds > 0 {
			title += " · " + a.textArgs("agent.process.duration", i18n.Args{"Duration": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(seconds))})
		}
		if toolCount > 0 {
			title += " · " + a.textArgs("agent.process.tool_count", i18n.Args{"Count": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(toolCount))})
		}
		return title
	}
	title := a.textID("agent.process.group")
	if seconds > 0 {
		title += " · " + a.textArgs("agent.process.duration", i18n.Args{"Duration": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(seconds))})
	}
	if toolCount > 0 {
		title += " · " + a.textArgs("agent.process.tool_count", i18n.Args{"Count": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(toolCount))})
	}
	return title
}

func agentSidebarProcessDisclosure(gtx layout.Context, theme *material.Theme, button *widget.Clickable, title, description string, collapsed bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(20))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(description).Add(gtx.Ops)
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

func agentSidebarResumeButton(gtx layout.Context, app *App, button *widget.Clickable) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(42)), gtx.Dp(unit.Dp(30)))
	gtx.Constraints = layout.Exact(size)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(app.textID("agent.process.resume.description")).Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Label(app.theme, unit.Sp(11), app.textID("common.action.continue"))
			label.Color = agentSidebarText
			gtx.Constraints.Min = image.Point{}
			return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, label.Layout)
		})
	})
}

var agentKnownToolNames = map[string]string{
	"document.list":                "agent.tool.name.document_list",
	"document.inspect":             "agent.tool.name.document_inspect",
	"document.create":              "agent.tool.name.document_create",
	"document.open":                "agent.tool.name.document_open",
	"document.save":                "agent.tool.name.document_save",
	"document.save_as":             "agent.tool.name.document_save_as",
	"document.close":               "agent.tool.name.document_close",
	"confirmation.get":             "agent.tool.name.confirmation_get",
	"workbook.audit":               "agent.tool.name.workbook_audit",
	"spreadsheet.cell.set":         "agent.tool.name.spreadsheet_cell_set",
	"spreadsheet.range.copy":       "agent.tool.name.spreadsheet_range_copy",
	"spreadsheet.range.clear":      "agent.tool.name.spreadsheet_range_clear",
	"word.paragraph.append":        "agent.tool.name.word_paragraph_append",
	"word.text_range.replace":      "agent.tool.name.word_text_range_replace",
	"word.text_range.copy":         "agent.tool.name.word_text_range_copy",
	"word.text_range.cut":          "agent.tool.name.word_text_range_cut",
	"word.text.search":             "agent.tool.name.word_text_search",
	"word.text.statistics":         "agent.tool.name.word_text_statistics",
	"word.table.insert":            "agent.tool.name.word_table_insert",
	"word.image.insert":            "agent.tool.name.word_image_insert",
	"word.document.properties.get": "agent.tool.name.word_document_properties_get",
	"word.document.properties.set": "agent.tool.name.word_document_properties_set",
}

func (a *App) agentToolDisplayName(name string) string {
	trimmed := strings.TrimSpace(name)
	if messageID, ok := agentKnownToolNames[trimmed]; ok {
		return a.textID(messageID)
	}
	return trimmed
}

func (a *App) agentToolSummaryText(line agentChatLine) string {
	if line.toolSummaryRef.ID != "" {
		return a.text(line.toolSummaryRef)
	}
	return line.toolSummary
}

func (a *App) agentToolRecordDisplay(line agentChatLine) (string, string) {
	if line.toolName != "" {
		title := a.agentToolDisplayName(line.toolName)
		summary := strings.TrimSpace(a.agentToolSummaryText(line))
		if line.needsAuth && !line.isError {
			summary = a.textID("agent.tool.result.needs_confirmation")
		}
		if summary == "" {
			switch {
			case line.inProgress:
				summary = a.textID("agent.tool.result.running")
			case line.needsAuth:
				summary = a.textID("agent.tool.result.needs_confirmation")
			case line.isError:
				summary = a.textID("agent.tool.result.failed")
			default:
				summary = a.textID("agent.tool.result.completed")
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
		return a.agentToolDisplayName(rawName), a.textID("agent.tool.result.completed")
	}
	if strings.HasPrefix(text, "正在执行 ") {
		rawName := strings.TrimPrefix(text, "正在执行 ")
		return a.agentToolDisplayName(rawName), a.textID("agent.tool.result.running")
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

func agentToolResultSummaryReference(toolName string, raw []byte, isError bool) (i18n.Message, string) {
	if isError {
		if agentToolResultNeedsConfirmation(raw) {
			return i18n.Message{ID: "agent.tool.result.confirmation_required"}, ""
		}
		var payload struct {
			Message string   `json:"message"`
			Error   string   `json:"error"`
			Content []string `json:"content"`
		}
		if len(raw) > 0 && json.Unmarshal(raw, &payload) == nil {
			for _, detail := range []string{payload.Message, payload.Error} {
				if strings.TrimSpace(detail) != "" {
					return i18n.Message{}, strings.TrimSpace(detail)
				}
			}
			if len(payload.Content) > 0 && strings.TrimSpace(payload.Content[0]) != "" {
				return i18n.Message{}, strings.TrimSpace(payload.Content[0])
			}
		}
		return i18n.Message{ID: "agent.tool.result.execution_failed"}, ""
	}
	var payload struct {
		Data struct {
			CellsScanned int    `json:"cells_scanned"`
			Documents    []any  `json:"documents"`
			SourcePath   string `json:"source_path"`
		} `json:"data"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &payload) == nil {
		switch toolName {
		case "document.list":
			if len(payload.Data.Documents) > 0 {
				return i18n.Message{ID: "agent.tool.result.documents_found", Args: i18n.Args{"Count": len(payload.Data.Documents)}}, ""
			}
		case "workbook.audit":
			if payload.Data.CellsScanned > 0 {
				return i18n.Message{ID: "agent.tool.result.cells_scanned", Args: i18n.Args{"Count": payload.Data.CellsScanned}}, ""
			}
		case "document.save", "document.save_as":
			if payload.Data.SourcePath != "" {
				return i18n.Message{ID: "agent.tool.result.document_saved"}, ""
			}
		case "document.create":
			return i18n.Message{ID: "agent.tool.result.document_created"}, ""
		case "document.inspect":
			return i18n.Message{ID: "agent.tool.result.document_inspected"}, ""
		case "word.paragraph.append":
			return i18n.Message{ID: "agent.tool.result.paragraph_updated"}, ""
		}
	}
	return i18n.Message{ID: "agent.tool.result.completed"}, ""
}
