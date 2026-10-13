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
		"为全文正文和表格段落应用间距预设：\n\n"+
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
			if !a.runWordApplicationCommand("应用文档主题："+themeName, "word.paragraph.spacing_all.set", worddesignBeforePointsAfterPointsArguments{BeforePoints: float64(before) / 20, AfterPoints: float64(after) / 20}) {
				a.setStatus("无法启动 Word 文档主题操作")
			}
		},
		modalDialogActionSpec{ID: "theme_office", Label: "Office 默认"},
		modalDialogActionSpec{ID: "theme_business", Label: "商务经典"},
		modalDialogActionSpec{ID: "theme_academic", Label: "学术紧凑"},
		modalDialogActionSpec{ID: "theme_modern", Label: "现代出版"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

type wordPageBackgroundSetArguments struct {
	Color string `json:"color"`
}

func (a *App) showWordColorsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	_, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		a.setStatus("当前文档不支持主题配色切换")
		return
	}
	a.modal.Show(
		"主题配色",
		"设置文档页面底色：\n\n"+
			"· 经典白底（#FFFFFF，标准打印）\n"+
			"· 雅致米黄（#FAF8F5，羊皮纸护眼风格）\n"+
			"· 护眼薄荷（#E8F5E9，柔和阅读风格）\n"+
			"· 商务湛蓝（#E3F2FD，冷色调现代风格）",
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
			if !a.runWordApplicationCommand("应用主题配色："+schemeName, "word.page.background.set", wordPageBackgroundSetArguments{Color: colorHex}) {
				a.setStatus("无法启动 Word 主题配色操作")
			}
		},
		modalDialogActionSpec{ID: "scheme_white", Label: "经典白底"},
		modalDialogActionSpec{ID: "scheme_parchment", Label: "雅致米黄"},
		modalDialogActionSpec{ID: "scheme_mint", Label: "护眼薄荷"},
		modalDialogActionSpec{ID: "scheme_blue", Label: "商务湛蓝"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

type wordTextRunFontFamilySetArguments struct {
	RunId      string `json:"run_id"`
	FontFamily string `json:"font_family"`
}

func (a *App) showWordFontsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	_, ok := any(state.word).(document.WordTextFontFamilyEditor)
	if !ok {
		a.setStatus("当前文档不支持主题字体配置")
		return
	}
	a.modal.Show(
		"主题字体",
		"选择正文文本运行的字体（选中正文文本时应用于该文本；无选区时应用于首个可编辑正文文本）：\n\n"+
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
			if runID == "" {
				a.setStatus("未找到可设置字体的正文文本")
				return
			}
			if !a.runWordApplicationCommand("设置主题字体："+label, "word.text_run.font_family.set", wordTextRunFontFamilySetArguments{RunId: runID, FontFamily: fontName}) {
				a.setStatus("无法启动 Word 主题字体操作")
			}
		},
		modalDialogActionSpec{ID: "font_yahei", Label: "微软雅黑"},
		modalDialogActionSpec{ID: "font_song", Label: "宋体"},
		modalDialogActionSpec{ID: "font_fangsong", Label: "仿宋"},
		modalDialogActionSpec{ID: "font_calibri", Label: "Calibri"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

type wordThemeEffectsSetArguments struct {
	EffectProfile string `json:"effect_profile"`
}

func (a *App) showWordEffectsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.Show(
		"主题效果",
		"为使用主题效果引用的对象设置阴影强度。直接写入对象的效果不会改变；Glance 当前不预览这些主题阴影。\n\n"+
			"· 微妙：柔和、短距离阴影\n"+
			"· 适中：清晰的中等阴影\n"+
			"· 强烈：较深、较远的阴影",
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			if a.applicationSessionRegistered(state.documentID) {
				if !a.runWordApplicationCommand("设置主题效果", "word.theme.effects.set", wordThemeEffectsSetArguments{EffectProfile: action}) {
					a.setStatus("无法启动主题效果设置")
				}
				return
			}
			if !a.runWordThemeEffectsCommand("设置主题效果", action) {
				a.setStatus("当前文档不支持主题效果设置")
			}
		},
		modalDialogActionSpec{ID: "subtle", Label: "微妙（默认）"},
		modalDialogActionSpec{ID: "moderate", Label: "适中"},
		modalDialogActionSpec{ID: "intense", Label: "强烈"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

type worddesignBeforePointsAfterPointsArguments struct {
	BeforePoints float64 `json:"before_points"`
	AfterPoints  float64 `json:"after_points"`
}

func (a *App) showWordDesignerDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	stats := wordTextStatisticsForState(state)
	a.modal.Show(
		"设计器",
		fmt.Sprintf("当前文档包含 %d 个段落、%d 个字符。选择一个版式预设：\n\n"+
			"版式预设：\n"+
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
			if !a.runWordApplicationCommand("设计器应用版式："+schemeName, "word.paragraph.spacing_all.set", worddesignBeforePointsAfterPointsArguments{BeforePoints: float64(before) / 20, AfterPoints: float64(after) / 20}) {
				a.setStatus("无法启动 Word 设计器版式操作")
			}
		},
		modalDialogActionSpec{ID: "scheme_gov", Label: "应用公文规范"},
		modalDialogActionSpec{ID: "scheme_biz", Label: "应用商务企划"},
		modalDialogActionSpec{ID: "scheme_academic", Label: "应用精简学术"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
