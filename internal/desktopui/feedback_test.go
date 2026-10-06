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
