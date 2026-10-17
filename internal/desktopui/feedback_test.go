package desktopui

import (
	"testing"
	"time"
)

func TestFeedbackTracksNewStatusesWithoutRestartingExpiry(t *testing.T) {
	var state feedbackState
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	state.observe("打开文件开始预览", now)
	if state.visible(now) {
		t.Fatal("startup hint should not become a toast")
	}
	state.observe("已保存 data.json", now)
	if state.kind != feedbackSuccess || !state.visible(now) {
		t.Fatal("save feedback missing")
	}
	until := state.until
	state.observe("已保存 data.json", now.Add(time.Second))
	if state.until != until {
		t.Fatal("frame restarted toast timer")
	}
	if state.visible(until) {
		t.Fatal("toast did not expire")
	}
	state.observe("保存失败：文件权限不足", until)
	if state.kind != feedbackError || !state.visible(until) {
		t.Fatal("error feedback missing")
	}
	state.dismiss()
	if state.visible(until) {
		t.Fatal("dismissed toast remains visible")
	}
}

func TestFeedbackSeverity(t *testing.T) {
	for _, tc := range []struct {
		message string
		kind    feedbackKind
	}{
		{"JSON 格式正确", feedbackSuccess},
		{"已对 JSON 键排序", feedbackSuccess},
		{"路径已复制", feedbackSuccess},
		{"文件名已复制", feedbackSuccess},
		{"JSON 内容已经改变，已取消操作", feedbackWarning},
		{"当前文件只读", feedbackWarning},
		{"格式化失败：invalid JSON", feedbackError},
		{"正在打开 data.json", feedbackInfo},
	} {
		if got := feedbackSeverity(tc.message); got != tc.kind {
			t.Errorf("%s: %v want %v", tc.message, got, tc.kind)
		}
	}
}

func TestFeedbackRepeatsCompletedOperationAfterDismissal(t *testing.T) {
	a := New(nil, "")
	now := time.Now()
	a.setStatus("JSON 格式正确")
	a.feedback.observe(a.status, now)
	a.feedback.dismiss()
	a.setStatus("JSON 格式正确")
	a.feedback.observe(a.status, now.Add(time.Second))
	if !a.feedback.visible(now.Add(time.Second)) {
		t.Fatal("repeated operation completion was mistaken for repeated frame")
	}
}

func TestFeedbackOnlyTriggersOnSideEffectOperations(t *testing.T) {
	nonSideEffects := []string{
		"缩放：100%",
		"缩放：125%",
		"已全选文档正文",
		"已开启并排查看模式",
		"已开启双页并排查看",
		"已打开新窗口并排查看",
		"已复制选中文字",
		"已复制选中单元格",
		"已复制 JSON",
		"选择要打开的文件…",
		"选择要浏览的文件夹…",
		"选择另存为位置…",
		"选择 PDF 保存位置…",
		"选择要插入的 PNG/JPEG 图片…",
		"最近打开的本地文档",
		"正在打开 data.json",
		"打开文件开始预览",
		"就绪",
		"已取消选择图片",
		"已取消操作",
		"已启动格式刷；单击或拖选目标文字应用，再点格式刷取消",
		"字数统计：共 120 字",
		"拼写检查完成：已校验 20 个单元格",
	}

	sideEffects := []string{
		"路径已复制",
		"文件名已复制",
		"已保存 data.json",
		"文件已保存，但历史版本未记录",
		"工作区已保存",
		"保存失败：文件权限不足",
		"PDF 已导出，共 2 页",
		"已导出共享副本",
		"已发送至系统打印队列",
		"已恢复历史版本",
		"已成功插入 3行3列 表格",
		"已插入形状：flowchart",
		"已插入公式",
		"已成功插入艺术字：标题",
		"已成功插入文本框",
		"已成功插入书签",
		"已成功插入超链接",
		"已成功设置页眉",
		"已成功设置页脚",
		"已成功在页脚插入页码",
		"已设置页面边框",
		"已清除页面边框",
		"已设置页面背景色为 #FAF8F5",
		"已成功将全文档语言设置为 en-US",
		"已应用主题配色：薄荷绿",
		"已应用主题字体方案：微软雅黑",
		"已应用主题效果集",
		"设计器已成功应用：政府公文",
		"已为选区添加审阅批注",
		"已清除批注与高亮标记",
		"已接受全部修订（处理 3 处）",
		"已拒绝全部修订（处理 3 处）",
		"已开启修订跟踪",
		"已关闭修订跟踪",
		"已开启编辑限制",
		"已解除编辑限制",
		"已设置宏安全策略：提示并拦截所有未签名脚本",
		"已创建数据透视表",
		"已设置条件格式",
		"已设置数据验证",
		"已保护工作表",
		"已撤销工作表保护",
		"已删除 2 个重复项，保留 10 个唯一项",
		"已完成分列：处理 10 行，最多拆分为 3 列",
		"已对 JSON 键排序",
		"已格式化 JSON",
		"JSON 格式正确",
		"当前文件只读",
		"“打印”功能暂未实现",
		"格式化失败：invalid JSON",
	}

	for _, msg := range nonSideEffects {
		var state feedbackState
		state.initialized = true
		now := time.Now()
		state.observe(msg, now)
		if state.visible(now) {
			t.Errorf("non-side-effect message %q should not trigger toast feedback", msg)
		}
	}

	for _, msg := range sideEffects {
		var state feedbackState
		state.initialized = true
		now := time.Now()
		state.observe(msg, now)
		if !state.visible(now) {
			t.Errorf("side-effect message %q should trigger toast feedback", msg)
		}
	}
}

func TestIsSideEffectStatus(t *testing.T) {
	if isSideEffectStatus("") {
		t.Error("empty status should not be side effect")
	}
	if isSideEffectStatus("   ") {
		t.Error("whitespace status should not be side effect")
	}
	if isSideEffectStatus("缩放：100%") {
		t.Error("zoom should not be side effect")
	}
	if !isSideEffectStatus("已保存 test.docx") {
		t.Error("save should be side effect")
	}
}
