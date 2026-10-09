package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

func (a *App) showWordEditorDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	stats := wordTextStatisticsForState(state)

	score := 100
	var suggestions []string
	for i, b := range blocks {
		if b.Paragraph != nil {
			for _, in := range b.Paragraph.Inlines {
				if in.Text != nil {
					t := in.Text.Text
					if strings.Contains(t, "，，") || strings.Contains(t, "。。") || strings.Contains(t, "、、") {
						suggestions = append(suggestions, fmt.Sprintf("段落 %d: 存在连续重复标点", i+1))
						score -= 5
					}
					if strings.Count(t, "（") != strings.Count(t, "）") || strings.Count(t, "“") != strings.Count(t, "”") {
						suggestions = append(suggestions, fmt.Sprintf("段落 %d: 括号或引号可能未成对闭合", i+1))
						score -= 5
					}
				}
			}
		}
	}
	if score < 60 {
		score = 60
	}

	estMin := stats.charactersWithoutWhitespace / 350
	if estMin < 1 {
		estMin = 1
	}

	report := fmt.Sprintf("【文档编辑器综合评分：%d / 100】\n\n"+
		"排版与统计概览：\n"+
		"· 总字符数：%d（不含空白：%d）\n"+
		"· 段落总数：%d\n"+
		"· 表格总数：%d\n"+
		"· 预估阅读耗时：约 %d 分钟\n\n",
		score,
		stats.charactersWithWhitespace,
		stats.charactersWithoutWhitespace,
		stats.paragraphs,
		stats.tables,
		estMin,
	)

	if len(suggestions) > 0 {
		report += "校对建议：\n"
		for i, s := range suggestions {
			if i >= 3 {
				report += fmt.Sprintf("· 更多 %d 条建议已省略...\n", len(suggestions)-3)
				break
			}
			report += "· " + s + "\n"
		}
	} else {
		report += "校对结果：未发现明显的拼写、标点或排版问题，文档规范良好。"
	}

	a.modal.Show("文档编辑器", report, func(action string) {
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
}

func (a *App) showWordTranslateDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	var activeRunID, originalText string
	if run, ok := selectedWordRun(blocks, state.selectedWordRunID); ok && run.Editable {
		activeRunID = run.ID
		originalText = run.Text
	} else {
		for _, b := range blocks {
			if b.Paragraph != nil {
				for _, in := range b.Paragraph.Inlines {
					if in.Text != nil && in.Text.Editable {
						activeRunID = in.Text.ID
						originalText = in.Text.Text
						break
					}
				}
			}
			if activeRunID != "" {
				break
			}
		}
	}
	if activeRunID == "" {
		a.setStatus("未找到可翻译的正文内容，请先选择包含文本的段落")
		return
	}

	previewText := originalText
	if previewText == "" {
		previewText = "（空段落）"
	} else {
		runes := []rune(previewText)
		if len(runes) > 25 {
			previewText = string(runes[:25]) + "..."
		}
	}

	a.modal.ShowInput(
		"翻译",
		fmt.Sprintf("原文内容：%s\n请输入翻译译文（确认后将替换选中文本）：", previewText),
		originalText,
		"输入翻译后的内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			translated := strings.TrimSpace(a.modal.Input.Text())
			if translated == "" {
				return
			}
			a.runWordCommand("翻译替换", func(editor document.WordTextEditor) error {
				return editor.ReplaceTextRun(activeRunID, translated)
			})
		},
		modalDialogActionSpec{ID: "confirm", Label: "替换入正文"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordCompareDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	dirty := sessionDirty(state)
	filePath := sessionPath(state)
	stats := wordTextStatisticsForState(state)

	msg := "文档版本对比：\n\n"
	if filePath == "" {
		msg += "当前为未保存的新建文档。\n"
	} else {
		msg += fmt.Sprintf("当前文件：%s\n", filePath)
		if dirty {
			msg += "状态：有未保存的编辑修改（当前内存版本与磁盘文件存在差异）。\n"
		} else {
			msg += "状态：已与磁盘文件完全同步（无差异）。\n"
		}
	}
	msg += fmt.Sprintf("\n当前版本指标：\n· 段落数：%d\n· 字符数：%d\n· 表格数：%d\n",
		stats.paragraphs, stats.charactersWithWhitespace, stats.tables)

	a.modal.Show("文档比较", msg, func(action string) {
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
}
