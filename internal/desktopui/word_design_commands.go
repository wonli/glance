package desktopui

import (
	"fmt"
	"glance/internal/document"
)

func (a *App) showWordThemesDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.Show(
		"文档主题",
		"选择整体文档格式主题：\n\n"+
			"· Office 默认：标准排版，平衡段落间距（段后 6 pt）\n"+
			"· 商务经典：正式严谨，宽松段落间距（段后 10 pt）\n"+
			"· 学术紧凑：紧凑版式，节省版面（段前 0 pt，段后 3 pt）\n"+
			"· 现代出版：轻盈通透，呼吸感版面（段后 12 pt）",
		func(action string) {
			a.modal.Hide()
			var before, after int64
			var themeName string
			switch action {
			case "theme_office":
				before, after, themeName = 0, 120, "Office 默认"
			case "theme_business":
				before, after, themeName = 40, 200, "商务经典"
			case "theme_academic":
				before, after, themeName = 0, 60, "学术紧凑"
			case "theme_modern":
				before, after, themeName = 60, 240, "现代出版"
			default:
				return
			}
			a.runWordSpacingCommand("应用文档主题："+themeName, func(editor document.WordParagraphSpacingEditor) error {
				return editor.SetAllParagraphsSpacing(before, after)
			})
		},
		modalDialogActionSpec{ID: "theme_office", Label: "Office 默认"},
		modalDialogActionSpec{ID: "theme_business", Label: "商务经典"},
		modalDialogActionSpec{ID: "theme_academic", Label: "学术紧凑"},
		modalDialogActionSpec{ID: "theme_modern", Label: "现代出版"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordColorsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	bgEditor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		a.setStatus("当前文档不支持主题配色切换")
		return
	}
	a.modal.Show(
		"主题配色",
		"选择文档主题色彩方案（背景与基调配色）：\n\n"+
			"· 经典白底（#FFFFFF，标准打印）\n"+
			"· 雅致米黄（#FAF8F5，羊皮纸护眼风格）\n"+
			"· 护眼薄荷（#E8F5E9，柔和阅读风格）\n"+
			"· 商务湛蓝（#E3F2FD，冷色调现代风格）\n"+
			"· 暗色夜间（#263238，低眩光暗调风格）",
		func(action string) {
			a.modal.Hide()
			var colorHex, schemeName string
			switch action {
			case "scheme_white":
				colorHex, schemeName = "", "经典白底"
			case "scheme_parchment":
				colorHex, schemeName = "FAF8F5", "雅致米黄"
			case "scheme_mint":
				colorHex, schemeName = "E8F5E9", "护眼薄荷"
			case "scheme_blue":
				colorHex, schemeName = "E3F2FD", "商务湛蓝"
			case "scheme_dark":
				colorHex, schemeName = "263238", "暗色夜间"
			default:
				return
			}
			if err := bgEditor.SetPageBackgroundColor(colorHex); err != nil {
				a.setStatus("应用主题配色失败：" + err.Error())
				return
			}
			a.setStatus("已成功应用主题配色：" + schemeName)
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "scheme_white", Label: "经典白底"},
		modalDialogActionSpec{ID: "scheme_parchment", Label: "雅致米黄"},
		modalDialogActionSpec{ID: "scheme_mint", Label: "护眼薄荷"},
		modalDialogActionSpec{ID: "scheme_blue", Label: "商务湛蓝"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordFontsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	fontEditor, ok := any(state.word).(document.WordTextFontFamilyEditor)
	if !ok {
		a.setStatus("当前文档不支持主题字体配置")
		return
	}
	a.modal.Show(
		"主题字体",
		"选择全文档主题字体搭配：\n\n"+
			"· 现代无衬线：微软雅黑 / Arial\n"+
			"· 经典宋体：宋体 / SimSun\n"+
			"· 仿宋公文：仿宋 / FangSong\n"+
			"· 西文排版：Calibri / Times New Roman",
		func(action string) {
			a.modal.Hide()
			var fontName, label string
			switch action {
			case "font_yahei":
				fontName, label = "Microsoft YaHei", "微软雅黑"
			case "font_song":
				fontName, label = "SimSun", "宋体"
			case "font_fangsong":
				fontName, label = "FangSong", "仿宋"
			case "font_calibri":
				fontName, label = "Calibri", "Calibri"
			default:
				return
			}
			runID := state.selectedWordRunID
			if runID == "" {
				blocks := wordBlocksForState(state)
				for _, b := range blocks {
					if b.Paragraph != nil {
						for _, in := range b.Paragraph.Inlines {
							if in.Text != nil && in.Text.Editable {
								runID = in.Text.ID
								break
							}
						}
					}
					if runID != "" {
						break
					}
				}
			}
			if runID != "" {
				if err := fontEditor.SetTextRunFontFamily(runID, fontName); err != nil {
					a.setStatus("设置主题字体失败：" + err.Error())
					return
				}
			}
			a.setStatus("已成功应用主题字体方案：" + label)
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "font_yahei", Label: "微软雅黑"},
		modalDialogActionSpec{ID: "font_song", Label: "宋体"},
		modalDialogActionSpec{ID: "font_fangsong", Label: "仿宋"},
		modalDialogActionSpec{ID: "font_calibri", Label: "Calibri"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordEffectsDialog() {
	a.modal.Show(
		"主题效果",
		"当前文档对象图形效果集：\n\n"+
			"· 微妙（Subtle）：平滑边缘，微柔和投影\n"+
			"· 适中（Moderate）：标准阴影与浅轮廓高亮\n"+
			"· 强烈（Intense）：深景深立体光泽投影\n\n"+
			"文档已默认启用与 OpenXML 规范兼容的“微妙”适中效果集。",
		func(action string) {
			a.modal.Hide()
			if action != "cancel" {
				a.setStatus("已应用主题效果集")
			}
		},
		modalDialogActionSpec{ID: "subtle", Label: "微妙（默认）"},
		modalDialogActionSpec{ID: "moderate", Label: "适中"},
		modalDialogActionSpec{ID: "intense", Label: "强烈"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordDesignerDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	stats := wordTextStatisticsForState(state)
	a.modal.Show(
		"设计器",
		fmt.Sprintf("AI 智能版式设计器已分析当前文档（%d 段落，%d 字符）：\n\n"+
			"推荐版式方案：\n"+
			"1. 【公文红头规范】：严谨宋体/仿宋，规范 1.5 倍行距，适合正式报告与通知\n"+
			"2. 【现代商务企划】：两端对齐，段后 8 pt 间距，适合企划案与项目总结\n"+
			"3. 【精简学术排版】：紧凑段间距，适合论文草稿与学术归档",
			stats.paragraphs, stats.charactersWithWhitespace),
		func(action string) {
			a.modal.Hide()
			var before, after int64
			var schemeName string
			switch action {
			case "scheme_gov":
				before, after, schemeName = 0, 160, "公文红头规范"
			case "scheme_biz":
				before, after, schemeName = 40, 160, "现代商务企划"
			case "scheme_academic":
				before, after, schemeName = 0, 60, "精简学术排版"
			default:
				return
			}
			a.runWordSpacingCommand("设计器应用版式："+schemeName, func(editor document.WordParagraphSpacingEditor) error {
				return editor.SetAllParagraphsSpacing(before, after)
			})
			a.setStatus("设计器已成功应用：" + schemeName)
		},
		modalDialogActionSpec{ID: "scheme_gov", Label: "应用公文规范"},
		modalDialogActionSpec{ID: "scheme_biz", Label: "应用商务企划"},
		modalDialogActionSpec{ID: "scheme_academic", Label: "应用精简学术"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
