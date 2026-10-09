package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strconv"
	"strings"
)

func (a *App) showWordInsertTableDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordTableInsertEditor)
	if !ok {
		a.setStatus("当前文档不支持插入表格")
		return
	}
	prompt := "请输入表格尺寸（格式：行数x列数，例如 3x3）："
	a.modal.ShowInput("插入表格", prompt, "3x3", "行数x列数（如 3x3, 4x2）", func(action string) {
		a.modal.Hide()
		var rows, cols int
		switch action {
		case "confirm":
			val := strings.TrimSpace(a.modal.Input.Text())
			val = strings.ReplaceAll(val, "*", "x")
			val = strings.ReplaceAll(val, "X", "x")
			val = strings.ReplaceAll(val, "×", "x")
			val = strings.ReplaceAll(val, ",", "x")
			val = strings.ReplaceAll(val, "，", "x")
			parts := strings.Split(val, "x")
			if len(parts) != 2 {
				a.setStatus("表格尺寸格式错误，请输入如 3x3")
				return
			}
			r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			c, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil || r < 1 || c < 1 || r > 100 || c > 50 {
				a.setStatus("行数需在 1–100 之间，列数需在 1–50 之间")
				return
			}
			rows, cols = r, c
		case "2x2":
			rows, cols = 2, 2
		case "3x3":
			rows, cols = 3, 3
		case "4x4":
			rows, cols = 4, 4
		case "5x3":
			rows, cols = 5, 3
		default:
			return
		}
		a.insertWordTable(editor, rows, cols)
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入自定义"},
		modalDialogActionSpec{ID: "3x3", Label: "3行3列"},
		modalDialogActionSpec{ID: "4x4", Label: "4行4列"},
		modalDialogActionSpec{ID: "5x3", Label: "5行3列"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) insertWordTable(editor document.WordTableInsertEditor, rows, cols int) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	afterParagraphID := state.selectedWordParagraphID
	tableID, err := editor.InsertTable(afterParagraphID, rows, cols)
	if err != nil {
		a.setStatus("插入表格失败：" + err.Error())
		return
	}
	if state.word != nil {
		state.setWordBlocks(state.word.WordBlocks())
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	a.setStatus(fmt.Sprintf("已成功插入 %d行%d列 表格", rows, cols))
	_ = tableID
	if a.window != nil {
		a.window.Invalidate()
	}
}
