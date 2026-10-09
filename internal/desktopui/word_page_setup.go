package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strconv"
	"strings"
)

func (a *App) showWordOrientationDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	currentOrient := "portrait"
	if data, ok := wordLayoutDataForState(state); ok && len(data.Sections) > 0 {
		if o := strings.ToLower(data.Sections[0].PageSetup.Orientation); o != "" {
			currentOrient = o
		}
	}
	label := "纵向"
	if currentOrient == "landscape" {
		label = "横向"
	}
	message := fmt.Sprintf("当前页面方向：%s\n请选择目标方向：", label)
	a.modal.Show("页面方向", message, func(action string) {
		a.modal.Hide()
		if action == "portrait" || action == "landscape" {
			a.runWordPageSetupCommand("修改页面方向", func(editor document.WordPageSetupEditor) error {
				return editor.SetPageOrientation(action)
			})
		}
	},
		modalDialogActionSpec{ID: "portrait", Label: "纵向"},
		modalDialogActionSpec{ID: "landscape", Label: "横向"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordMarginsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	topPT, rightPT, bottomPT, leftPT := 72.0, 72.0, 72.0, 72.0
	if data, ok := wordLayoutDataForState(state); ok && len(data.Sections) > 0 {
		s := data.Sections[0].PageSetup
		if s.Set&document.WordPageMarginTopSet != 0 || s.MarginTopPT > 0 {
			topPT = s.MarginTopPT
		}
		if s.Set&document.WordPageMarginRightSet != 0 || s.MarginRightPT > 0 {
			rightPT = s.MarginRightPT
		}
		if s.Set&document.WordPageMarginBottomSet != 0 || s.MarginBottomPT > 0 {
			bottomPT = s.MarginBottomPT
		}
		if s.Set&document.WordPageMarginLeftSet != 0 || s.MarginLeftPT > 0 {
			leftPT = s.MarginLeftPT
		}
	}
	labels := []string{"上边距（磅）", "右边距（磅）", "下边距（磅）", "左边距（磅）"}
	hints := []string{"72", "72", "72", "72"}
	initial := []string{
		strconv.FormatFloat(topPT, 'f', -1, 64),
		strconv.FormatFloat(rightPT, 'f', -1, 64),
		strconv.FormatFloat(bottomPT, 'f', -1, 64),
		strconv.FormatFloat(leftPT, 'f', -1, 64),
	}
	a.modal.ShowForm("页边距设置", "可选择快速预设或输入自定义页边距（磅，1 厘米 ≈ 28.35 磅）：",
		labels, hints, initial,
		func(action string) {
			switch action {
			case "normal":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置普通页边距", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageMargins(72, 72, 72, 72)
				})
			case "narrow":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置窄页边距", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageMargins(36, 36, 36, 36)
				})
			case "confirm":
				top, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				right, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				bottom, err3 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[2].Text()), 64)
				left, err4 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[3].Text()), 64)
				if err1 != nil || err2 != nil || err3 != nil || err4 != nil ||
					top < 0 || right < 0 || bottom < 0 || left < 0 ||
					top > 1000 || right > 1000 || bottom > 1000 || left > 1000 {
					a.setStatus("页边距必须是 0 到 1000 磅之间的数值")
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				a.runWordPageSetupCommand("设置页边距", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageMargins(top, right, bottom, left)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "normal", Label: "普通 (72pt)"},
		modalDialogActionSpec{ID: "narrow", Label: "窄边距 (36pt)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordPageSizeDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	wPT, hPT := 595.3, 841.9
	if data, ok := wordLayoutDataForState(state); ok && len(data.Sections) > 0 {
		s := data.Sections[0].PageSetup
		if s.WidthPT > 0 {
			wPT = s.WidthPT
		}
		if s.HeightPT > 0 {
			hPT = s.HeightPT
		}
	}
	labels := []string{"宽度（磅）", "高度（磅）"}
	hints := []string{"A4 约为 595.3", "A4 约为 841.9"}
	initial := []string{
		strconv.FormatFloat(wPT, 'f', 1, 64),
		strconv.FormatFloat(hPT, 'f', 1, 64),
	}
	a.modal.ShowForm("纸张大小设置", "可选择标准纸张规格或输入自定义尺寸（磅）：",
		labels, hints, initial,
		func(action string) {
			switch action {
			case "a4":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置 A4 纸张大小", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageSize(595.3, 841.9)
				})
			case "letter":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置 Letter 纸张大小", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageSize(612, 792)
				})
			case "confirm":
				w, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				h, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				if err1 != nil || err2 != nil || w <= 0 || h <= 0 || w > 3000 || h > 3000 {
					a.setStatus("纸张宽度和高度必须在 1 到 3000 磅之间")
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				a.runWordPageSetupCommand("设置纸张大小", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageSize(w, h)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "a4", Label: "A4 (210×297mm)"},
		modalDialogActionSpec{ID: "letter", Label: "Letter (8.5×11in)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordColumnsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	message := "选择页面分栏排版布局："
	a.modal.Show("分栏设置", message, func(action string) {
		a.modal.Hide()
		var cols int
		switch action {
		case "col1":
			cols = 1
		case "col2":
			cols = 2
		case "col3":
			cols = 3
		default:
			return
		}
		a.runWordPageSetupCommand(fmt.Sprintf("设置页面分栏为 %d 栏", cols), func(editor document.WordPageSetupEditor) error {
			return editor.SetPageColumns(cols)
		})
	},
		modalDialogActionSpec{ID: "col1", Label: "一栏 (单栏常规)"},
		modalDialogActionSpec{ID: "col2", Label: "两栏 (双栏排版)"},
		modalDialogActionSpec{ID: "col3", Label: "三栏 (三栏报刊)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordLineNumbersDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	message := "请选择正文行号编排方式："
	a.modal.Show("行号设置", message, func(action string) {
		a.modal.Hide()
		var mode string
		var label string
		switch action {
		case "continuous":
			mode = "continuous"
			label = "连续行号"
		case "newPage":
			mode = "newPage"
			label = "每页重置行号"
		case "none":
			mode = "none"
			label = "清除行号"
		default:
			return
		}
		a.runWordPageSetupCommand("设置"+label, func(editor document.WordPageSetupEditor) error {
			return editor.SetPageLineNumbers(mode)
		})
	},
		modalDialogActionSpec{ID: "continuous", Label: "连续编号"},
		modalDialogActionSpec{ID: "newPage", Label: "每页重置"},
		modalDialogActionSpec{ID: "none", Label: "无 (清除行号)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordBreaksDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageBreakEditor); !ok {
		return
	}
	message := "选择在当前光标位置插入的分隔符类型："
	a.modal.Show("插入分隔符", message, func(action string) {
		a.modal.Hide()
		switch action {
		case "page":
			a.queueWordBreak(document.WordBreakPage)
		case "column":
			a.queueWordBreak(document.WordBreakColumn)
		}
	},
		modalDialogActionSpec{ID: "page", Label: "分页符 (进入下一页)"},
		modalDialogActionSpec{ID: "column", Label: "分栏符 (进入下一栏)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) runWordPageSetupCommand(operation string, edit func(document.WordPageSetupEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordPageSetupEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		if err == nil {
			state.invalidateWordLayoutAfterBlocksChange()
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) showWordCountDetails() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		return
	}
	statistics := wordTextStatisticsForState(state)
	message := fmt.Sprintf(
		"字符数（不含空白）：%d\n字符数（含空白）：%d\n段落数：%d\n表格数：%d",
		statistics.charactersWithoutWhitespace,
		statistics.charactersWithWhitespace,
		statistics.paragraphs,
		statistics.tables,
	)
	a.modal.Show("字数统计", message, func(action string) {
		if action == "close" {
			a.modal.Hide()
		}
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
}
