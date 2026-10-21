package desktopui

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/agent"
	"glance/internal/i18n"
)

func collectSemanticLabels(router *input.Router) []string {
	var labels []string
	seen := map[input.SemanticID]bool{}
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			if n.Desc.Label != "" {
				labels = append(labels, n.Desc.Label)
			}
			visit(n.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	return labels
}

func TestAgentSidebarReasoningUsesCompactDisclosure(t *testing.T) {
	a := New(nil, "")
	a.agentUI.chatLines = []agentChatLine{newAgentReasoningLine("隐藏的思考内容", true)}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(210, 1000)}}
	dims := a.layoutAgentSidebarMessage(gtx, 0, false)
	if dims.Size.Y > 40 {
		t.Fatalf("collapsed reasoning height=%d, want compact disclosure", dims.Size.Y)
	}
}

func TestAgentToolLocalAccessNamesAndSummaries(t *testing.T) {
	a := New(nil, "")
	for _, tool := range []struct {
		name     string
		nameID   string
		result   string
		resultID string
		contains string
	}{
		{"local.request_directory_access", "agent.tool.name.local_request_directory_access", `{"data":{"scope_id":"opaque-scope","folder_name":"repo"}}`, "agent.tool.result.local_access_allowed", "repo"},
		{"local.list_directory", "agent.tool.name.local_list_directory", `{"data":{"entries":[{},{}]}}`, "agent.tool.result.local_entries_listed", "2"},
		{"local.read_text_file", "agent.tool.name.local_read_text_file", `{"data":{"relative_path":"docs/notes.md","content":"PRIVATE_FILE_TEXT"}}`, "agent.tool.result.local_file_read", "docs/notes.md"},
		{"local.list_git_commits", "agent.tool.name.local_list_git_commits", `{"data":{"commits":[{},{}]}}`, "agent.tool.result.local_commits_listed", "2"},
	} {
		if got := a.agentToolDisplayName(tool.name); got == tool.name || got == tool.nameID {
			t.Errorf("tool %q display name was not localized: %q", tool.name, got)
		}
		ref, summary := agentToolResultSummaryReference(tool.name, []byte(tool.result), false)
		if ref.ID != tool.resultID || summary != "" {
			t.Errorf("tool %q summary reference=%+v summary=%q, want %q", tool.name, ref, summary, tool.resultID)
		}
		rendered := a.text(ref)
		if !strings.Contains(rendered, tool.contains) {
			t.Errorf("tool %q rendered summary %q does not contain %q", tool.name, rendered, tool.contains)
		}
		if strings.Contains(rendered, "PRIVATE_FILE_TEXT") {
			t.Errorf("tool %q summary exposed local file content: %q", tool.name, rendered)
		}
	}
	ref, summary := agentToolResultSummaryReference("local.request_directory_access", []byte(`{"error":"directory_access_denied","message":"directory access was denied"}`), true)
	if ref.ID != "agent.tool.result.local_access_denied" || summary != "" {
		t.Fatalf("denied local access summary = %+v / %q", ref, summary)
	}
}

func TestAgentSidebarUnifiedProcessEntryFoldsReasoningAndToolsBeforeEachAnswer(t *testing.T) {
	a := New(nil, "")
	reasoning := newAgentReasoningLine("已阅读当前文档，归纳出品牌、渠道与复盘三个核心目标。", true)
	reasoning.duration = 3 * time.Second
	a.agentUI.chatLines = []agentChatLine{
		{role: "user", text: "帮我总结这份策划书的核心目标。"},
		reasoning,
		{role: "tool", toolName: "document.list", toolSummary: "已找到 1 份当前文档", text: "工具 document.list 已完成", collapsed: true},
		{role: "tool", toolName: "spreadsheet.range.copy", toolSummary: "已复制 A1:F12 区域", text: "工具 spreadsheet.range.copy 已完成", collapsed: true},
		{role: "tool", toolName: "获取表格图片", toolSummary: "已找到 2 张图片", text: "工具 获取表格图片 已完成", collapsed: true},
		{role: "assistant", text: "这份策划书围绕品牌与增长，明确了三个核心目标。"},
	}
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(250, 701))}
		a.layoutAgentSidebarMessages(gtx)
		router.Frame(&ops)
	}
	frame()
	labels := collectSemanticLabels(&router)
	processHeaders := 0
	for _, label := range labels {
		if strings.HasPrefix(label, "处理过程") {
			processHeaders++
			if label != "处理过程 · 3 秒 · 3 次调用" {
				t.Fatalf("process header = %q, want %q", label, "处理过程 · 3 秒 · 3 次调用")
			}
		}
		if strings.Contains(label, "已阅读当前文档") || strings.Contains(label, "获取文档列表") || strings.Contains(label, "已找到 1 份当前文档") {
			t.Fatalf("collapsed process entry leaked inner details: %q", label)
		}
	}
	if processHeaders != 1 {
		t.Fatalf("process headers = %d, want 1", processHeaders)
	}

	// Expand by clicking the unified process disclosure.
	a.agentUI.chatLines[1].toggle.Click()
	frame()
	labels = collectSemanticLabels(&router)
	joined := strings.Join(labels, "\n")
	for _, want := range []string{
		"处理过程 · 3 秒 · 3 次调用",
		"已阅读当前文档，归纳出品牌、渠道与复盘三个核心目标。",
		"获取文档列表",
		"已找到 1 份当前文档",
		"复制表格区域",
		"已复制 A1:F12 区域",
		"获取表格图片",
		"已找到 2 张图片",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expanded process entry missing %q in labels:\n%s", want, joined)
		}
	}

	// Collapse again.
	a.agentUI.chatLines[1].toggle.Click()
	frame()
	labels = collectSemanticLabels(&router)
	for _, label := range labels {
		if strings.Contains(label, "已阅读当前文档") || strings.Contains(label, "获取文档列表") {
			t.Fatalf("re-collapsed process entry still shows inner details: %q", label)
		}
	}
}

func TestAgentSidebarProcessShowsLiveProgressAndSeparateFailureOrAuthReminders(t *testing.T) {
	r := newDesktopRuntime()
	t.Cleanup(r.shutdown)
	a := newAppWithRuntime(nil, r)
	a.agentUI.conversationID = "conv-progress"
	a.agentUI.conversationBusy = true

	var ops op.Ops
	var router input.Router
	frame := func() []string {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(250, 701))}
		a.layoutAgentSidebarMessages(gtx)
		router.Frame(&ops)
		return collectSemanticLabels(&router)
	}

	a.agentUI.chatEvents <- agent.AgentEvent{ConversationID: "conv-progress", Type: agent.EventReasoningDelta, Reasoning: "正在核对工作表结构"}
	a.processAgentChatEvents()
	joined := strings.Join(frame(), "\n")
	if !strings.Contains(joined, a.textID("agent.process.analyzing")) || !strings.Contains(joined, "正在核对工作表结构") {
		t.Fatalf("in-progress reasoning did not display current progress:\n%s", joined)
	}

	a.agentUI.chatEvents <- agent.AgentEvent{ConversationID: "conv-progress", Type: agent.EventToolStarted, ToolName: "workbook.audit"}
	a.processAgentChatEvents()
	joined = strings.Join(frame(), "\n")
	if !strings.Contains(joined, a.textArgs("agent.process.running", i18n.Args{"Action": a.textID("agent.tool.name.workbook_audit")})) {
		t.Fatalf("in-progress tool did not display current tool progress:\n%s", joined)
	}
	if strings.Contains(joined, "正在核对工作表结构") {
		t.Fatalf("completed reasoning was not folded when tool execution started:\n%s", joined)
	}

	a.agentUI.chatEvents <- agent.AgentEvent{
		ConversationID: "conv-progress",
		Type:           agent.EventToolCompleted,
		ToolName:       "workbook.audit",
		Result:         json.RawMessage(`{"error":"audit_failed","message":"无法读取合并单元格区域"}`),
		IsError:        true,
	}
	a.agentUI.chatEvents <- agent.AgentEvent{
		ConversationID: "conv-progress",
		Type:           agent.EventToolStarted,
		ToolName:       "document.save",
	}
	a.agentUI.chatEvents <- agent.AgentEvent{
		ConversationID: "conv-progress",
		Type:           agent.EventToolCompleted,
		ToolName:       "document.save",
		Result:         json.RawMessage(`{"status":"confirmation_required","confirmation_id":"conf-1"}`),
	}
	a.agentUI.chatEvents <- agent.AgentEvent{
		ConversationID: "conv-progress",
		Type:           agent.EventAssistantDelta,
		Text:           "审计遇到异常，保存操作正在等待您的确认。",
	}
	a.processAgentChatEvents()
	labels := frame()
	joined = strings.Join(labels, "\n")

	processCount := 0
	for _, label := range labels {
		if strings.HasPrefix(label, "处理过程") {
			processCount++
		}
	}
	if processCount != 1 {
		t.Fatalf("process headers = %d, want 1 in:\n%s", processCount, joined)
	}
	if strings.Contains(joined, "正在核对工作表结构") {
		t.Fatalf("collapsed process leaked thinking summary after answer started:\n%s", joined)
	}
	if !strings.Contains(joined, "无法读取合并单元格区域") {
		t.Fatalf("failed tool reminder was hidden while process was collapsed:\n%s", joined)
	}
	if !strings.Contains(joined, "等待人工授权确认") {
		t.Fatalf("authorization reminder was hidden while process was collapsed:\n%s", joined)
	}
}

func TestAgentSidebarResumeHasVisibleText(t *testing.T) {
	a := New(nil, "")
	a.agentUI.conversationInterrupted = true
	a.agentUI.conversationTitle = "很长很长的历史会话标题"
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(148, 100)}}
	a.layoutAgentSidebarHeader(gtx)
	router.Frame(&ops)
	found := false
	for _, n := range router.AppendSemantics(nil) {
		if n.Desc.Label == "继续" {
			found = true
		}
	}
	if !found {
		t.Fatal("interrupted conversation has no visible continue label")
	}
}

func TestAgentSidebarCardsVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.agentUI.conversationTitle = "修复文字换行问题"
		a.agentUI.conversationInterrupted = true
		a.agentUI.chatLines = []agentChatLine{{role: "user", text: "请检查 A12、A13 的文字换行。"}, newAgentReasoningLine("检查文档状态", true), {role: "assistant", text: "我先检查这两个单元格的内容与格式。"}, {role: "tool", text: "document.inspect 检查完成"}, newAgentReasoningLine("对照单元格布局", true), {role: "assistant", text: "已定位换行设置的问题，下一步调整单元格格式。"}}
		var ops op.Ops
		var router input.Router
		size := image.Pt(int(250*scale), int(701*scale))
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		a.layoutAgentSidebar(gtx)
		router.Frame(&ops)
		labels := collectSemanticLabels(&router)
		processCount := 0
		for _, label := range labels {
			if strings.HasPrefix(label, "处理过程") {
				processCount++
			}
			if label == "工具" || label == "思考过程" {
				t.Fatalf("legacy separate %q label rendered instead of unified process entry", label)
			}
		}
		if processCount != 2 {
			t.Fatalf("process entry count = %d, want 1 before each of the 2 formal answers", processCount)
		}
		img := renderPresentationTestOps(t, &ops, size)
		titleTop, titleBottom, titleOK := darkInkVerticalBounds(img, image.Rect(int(20*scale), int(13*scale), int(116*scale), int(43*scale)))
		arrowTop, arrowBottom, arrowOK := darkInkVerticalBounds(img, image.Rect(int(123*scale), int(13*scale), int(137*scale), int(43*scale)))
		delta := titleTop + titleBottom - arrowTop - arrowBottom
		if !titleOK || !arrowOK || delta > int(4*scale) || delta < -int(4*scale) {
			t.Fatalf("title/triangle visible centers differ at scale %v: title %d..%d triangle %d..%d", scale, titleTop, titleBottom, arrowTop, arrowBottom)
		}
		blue := 0
		for y := int(90 * scale); y < int(550*scale); y++ {
			for x := int(20 * scale); x < int(230*scale); x++ {
				p := img.RGBAAt(x, y)
				if int(p.B) > int(p.R)+40 && int(p.B) > int(p.G)+40 {
					blue++
				}
			}
		}
		if blue > 0 {
			t.Fatal("reasoning uses blue filled button")
		}
		if scale == 1 {
			f, err := os.Create("/private/tmp/glance-cards-preview.png")
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}

func TestAgentSidebarHistoryViewKeepsDocumentVisibleAndReturnsWithBack(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := New(nil, "")
	a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	a.documentID = "history-overlay-test"
	a.agentUI.conversationDocumentID = a.documentID
	a.agentUI.sidebarVisible = true
	a.agentUI.conversationHistoryLoaded = true
	a.agentUI.conversationHistoryVisible = true
	a.agentUI.conversationHistory = []agent.ConversationSummary{{ID: "one", Title: "上一段会话"}, {ID: "two", Title: "另一段会话"}}
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 701))}
		a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		router.Frame(&ops)
	}
	frame()
	img := renderPresentationTestOps(t, &ops, image.Pt(800, 701))
	p := img.RGBAAt(500, 300)
	if p.R < 245 || p.G < 245 || p.B < 245 {
		t.Fatal("history sidebar dims the document")
	}
	f, err := os.Create("/private/tmp/glance-history-preview.png")
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, img)
	f.Close()
	click := func(pos f32.Point) {
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 9, Buttons: pointer.ButtonPrimary, Position: pos})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 9, Position: pos})
		frame()
		frame()
	}
	click(f32.Pt(18, 60))
	if !a.agentUI.conversationHistoryVisible {
		t.Fatal("clicking popup padding dismissed history")
	}
	click(f32.Pt(500, 300))
	if !a.agentUI.conversationHistoryVisible {
		t.Fatal("document click unexpectedly closed history")
	}
	click(f32.Pt(25, 30))
	if a.agentUI.conversationHistoryVisible {
		t.Fatal("back click did not return to chat")
	}
}

func TestAgentSidebarTypedContinueUsesResume(t *testing.T) {
	a := New(nil, "")
	a.agentUI.conversationInterrupted = true
	a.agentUI.conversationID = "interrupted-test"
	a.agentUI.chatInput.SetText(a.textID("common.action.continue"))
	a.sendAgentChatMessage()
	if a.agentUI.chatInput.Text() != "" || len(a.agentUI.chatLines) != 0 {
		t.Fatal("continue was kept as draft or sent as duplicate user turn")
	}
	if a.text(a.agentUI.chatStatusRef) != a.textID("agent.chat.status.no_model") {
		t.Fatal("typed continue did not reach resume operation")
	}
}
