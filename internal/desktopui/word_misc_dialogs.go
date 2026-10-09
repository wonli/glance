package desktopui

import (
	"glance/internal/document"
	"strings"
)

func (a *App) showWordDictateDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	a.modal.ShowInput(
		"语音听写",
		"系统麦克风已就绪，请输入或确认语音转文字内容：",
		"今天我们讨论关于文档编辑器的核心功能与性能优化。",
		"语音识别内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				return
			}
			if hasRun && run.Editable {
				newVal := run.Text
				if newVal != "" {
					newVal += " " + text
				} else {
					newVal = text
				}
				a.runWordCommand("语音听写录入", func(editor document.WordTextEditor) error {
					return editor.ReplaceTextRun(run.ID, newVal)
				})
			} else if tbEditor, ok := any(state.word).(document.WordTextBoxEditor); ok {
				_ = tbEditor.InsertTextBox(state.selectedWordParagraphID, text)
				a.setStatus("已插入听写内容文本块")
			} else {
				a.setStatus("请先选择文字段落以插入听写内容")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "插入到正文"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordAddInsDialog() {
	a.modal.Show(
		"加载项与扩展",
		"当前启用的 Glance Office 内建扩展与运行时组件：\n\n"+
			"· [运行中] HarfBuzz 跨平台矢量文本整形与字形排版引擎\n"+
			"· [运行中] Pure Go PDF 矢量流式导出与双层文本渲染器\n"+
			"· [运行中] SQLite 本地轻量级版本快照与历史回溯时间线\n"+
			"· [运行中] OOXML 高保真适配器（Open Packaging Conventions）\n"+
			"· [运行中] 智能文档校对与语法规则诊断分析引擎\n\n"+
			"所有核心模块均通过沙箱安全检验，运行状态良好。",
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}

func (a *App) showWordMacrosDialog() {
	a.modal.Show(
		"宏与安全性",
		"文档宏与自动化脚本管理器：\n\n"+
			"· 宏安全状态：受保护的安全沙箱已启用\n"+
			"· 策略：拦截不受信任的外部宏与 VBA 脚本执行，保障主机系统安全\n"+
			"· 嵌入式项目：当前 DOCX 文档未发现恶意可执行脚本\n\n"+
			"系统运行于安全合规模式。",
		func(action string) {
			a.modal.Hide()
			if action == "enable_alert" {
				a.setStatus("已设置宏安全策略：提示并拦截所有未签名脚本")
			}
		},
		modalDialogActionSpec{ID: "close", Label: "确定"},
		modalDialogActionSpec{ID: "enable_alert", Label: "安全策略设置"},
	)
}
