package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

func (a *App) showWordWordArtDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordTextBoxEditor); !ok {
		a.setStatus("当前文档不支持插入艺术字")
		return
	}
	a.modal.ShowInput(
		"插入艺术字",
		"请输入艺术字文本（选择样式后将插入艺术字排版块）：",
		"艺术字标题",
		"输入艺术字文本内容",
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				text = "艺术字标题"
			}
			var stylePrefix string
			switch action {
			case "style_gold":
				stylePrefix = "【艺术字·金辉立体】"
			case "style_neon":
				stylePrefix = "【艺术字·霓虹光效】"
			case "style_hollow":
				stylePrefix = "【艺术字·极简空心】"
			default:
				stylePrefix = "【艺术字】"
			}
			afterPID := state.selectedWordParagraphID
			content := fmt.Sprintf("%s %s", stylePrefix, text)
			if !a.runWordApplicationCommand("插入艺术字", "word.text_box.insert", map[string]any{
				"after_paragraph_id": afterPID,
				"text":               content,
			}) {
				a.setStatus("当前会话无法通过能力目录插入艺术字")
				return
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "默认样式"},
		modalDialogActionSpec{ID: "style_gold", Label: "金辉立体"},
		modalDialogActionSpec{ID: "style_neon", Label: "霓虹光效"},
		modalDialogActionSpec{ID: "style_hollow", Label: "极简空心"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordEquationDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.ShowInput(
		"插入公式",
		"输入公式表达式；当前将公式作为可编辑文本插入，不创建 Office 数学公式对象：",
		"x = (-b ± √(b² - 4ac)) / (2a)",
		"输入或编辑数学公式",
		func(action string) {
			a.modal.Hide()
			var formula string
			switch action {
			case "quadratic":
				formula = "x = (-b ± √(b² - 4ac)) / (2a)"
			case "euler":
				formula = "e^(iπ) + 1 = 0"
			case "pythagorean":
				formula = "a² + b² = c²"
			case "confirm":
				formula = strings.TrimSpace(a.modal.Input.Text())
			default:
				return
			}
			if formula == "" {
				return
			}
			run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
			arguments := map[string]any{}
			if hasRun && run.Editable {
				expression := formula
				if run.Text != "" {
					expression = run.Text + " " + formula
				}
				arguments = map[string]any{"target": "run", "run_id": run.ID, "expression": expression}
			} else if _, hasTextBox := any(state.word).(document.WordTextBoxEditor); hasTextBox {
				arguments = map[string]any{"target": "textbox", "after_paragraph_id": state.selectedWordParagraphID, "expression": "【公式】" + formula}
			} else {
				a.setStatus("请先选择可编辑的文字段落以插入公式")
				return
			}
			if !a.runWordApplicationCommand("插入公式文本", "word.equation.insert", arguments) {
				a.setStatus("当前会话无法通过能力目录插入公式文本")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "插入输入内容"},
		modalDialogActionSpec{ID: "quadratic", Label: "二次方程"},
		modalDialogActionSpec{ID: "euler", Label: "欧拉公式"},
		modalDialogActionSpec{ID: "pythagorean", Label: "勾股定理"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordShapesDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordTextBoxEditor); !ok {
		a.setStatus("当前文档不支持插入形状容器")
		return
	}
	a.modal.ShowInput(
		"插入形状",
		"请选择几何形状类型并输入形状内嵌文本：",
		"形状注释内容",
		"输入形状标注文本",
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				text = "说明文本"
			}
			var shapeType string
			switch action {
			case "rect":
				shapeType = "矩形框"
			case "round_rect":
				shapeType = "圆角矩形框"
			case "callout":
				shapeType = "标注提示框"
			case "flowchart":
				shapeType = "流程处理框"
			default:
				shapeType = "图形框"
			}
			afterPID := state.selectedWordParagraphID
			content := fmt.Sprintf("【形状·%s】%s", shapeType, text)
			if !a.runWordApplicationCommand("插入形状", "word.text_box.insert", map[string]any{
				"after_paragraph_id": afterPID,
				"text":               content,
			}) {
				a.setStatus("当前会话无法通过能力目录插入形状文本框")
				return
			}
		},
		modalDialogActionSpec{ID: "rect", Label: "矩形框"},
		modalDialogActionSpec{ID: "round_rect", Label: "圆角矩形"},
		modalDialogActionSpec{ID: "callout", Label: "标注框"},
		modalDialogActionSpec{ID: "flowchart", Label: "流程框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordChartDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	_, ok := any(state.word).(document.WordTableInsertEditor)
	if !ok {
		a.setStatus("当前文档不支持插入图表数据表")
		return
	}
	a.modal.Show(
		"插入图表",
		"选择图表类型并在文档中插入图表数据源结构：\n\n"+
			"· 簇状柱形图（3x3 类别/系列数据表）\n"+
			"· 折线趋势图（4x3 时间序列数据表）\n"+
			"· 饼状分布图（3x2 占比分布数据表）",
		func(action string) {
			a.modal.Hide()
			var rows, cols int
			var chartName string
			switch action {
			case "column":
				rows, cols, chartName = 3, 3, "柱形图"
			case "line":
				rows, cols, chartName = 4, 3, "折线图"
			case "pie":
				rows, cols, chartName = 3, 2, "饼状图"
			default:
				return
			}
			if !a.runWordApplicationCommand("插入图表基础数据表", "word.table.insert", map[string]any{
				"after_paragraph_id": state.selectedWordParagraphID,
				"rows":               rows,
				"columns":            cols,
			}) {
				a.setStatus("当前会话无法通过能力目录插入图表数据表")
				return
			}
			a.setStatus(fmt.Sprintf("正在插入 %s 基础数据表格（%d 行 × %d 列）", chartName, rows, cols))
		},
		modalDialogActionSpec{ID: "column", Label: "柱形图"},
		modalDialogActionSpec{ID: "line", Label: "折线图"},
		modalDialogActionSpec{ID: "pie", Label: "饼状图"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
