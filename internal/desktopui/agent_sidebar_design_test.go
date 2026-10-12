package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/agent"
	"glance/internal/agent/provider"
	"image"
	"math"
	"runtime"
	"testing"
)

func TestAgentSidebarDesignHeader(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.agentUI.conversationTitle = "修复文字换行问题"
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(int(218*scale), int(701*scale))}}
		normalDims := a.layoutAgentSidebarHeader(gtx)
		if normalDims.Size.Y != int(30*scale) {
			t.Fatalf("normal header height=%d; design requires one 30dp conversation row", normalDims.Size.Y)
		}
		ops.Reset()
		historyDims := a.layoutAgentHistoryHeader(gtx)
		if historyDims.Size.Y != normalDims.Size.Y {
			t.Fatalf("history header height=%d != normal header height=%d", historyDims.Size.Y, normalDims.Size.Y)
		}

		sidebarSize := image.Pt(int(250*scale), int(200*scale))
		ops.Reset()
		gtx.Constraints = layout.Exact(sidebarSize)
		a.agentUI.conversationHistoryVisible = false
		a.layoutAgentSidebar(gtx)
		normalImg := renderPresentationTestOps(t, &ops, sidebarSize)

		ops.Reset()
		gtx.Constraints = layout.Exact(sidebarSize)
		a.agentUI.conversationHistoryVisible = true
		a.layoutAgentHistorySidebar(gtx)
		historyImg := renderPresentationTestOps(t, &ops, sidebarSize)

		normalTitleMinY, normalTitleMaxY, ok := darkInkVerticalBounds(normalImg, image.Rect(int(18*scale), int(13*scale), int(115*scale), int(43*scale)))
		if !ok {
			t.Fatalf("scale=%v: normal conversation title ink missing", scale)
		}
		normalChevronMinY, normalChevronMaxY, ok := darkInkVerticalBounds(normalImg, image.Rect(int(116*scale), int(13*scale), int(142*scale), int(43*scale)))
		if !ok {
			t.Fatalf("scale=%v: normal conversation chevron ink missing", scale)
		}
		normalPenMinY, normalPenMaxY, ok := darkInkVerticalBounds(normalImg, image.Rect(int(204*scale), int(13*scale), int(234*scale), int(43*scale)))
		if !ok {
			t.Fatalf("scale=%v: normal new-conversation icon ink missing", scale)
		}
		backMinY, backMaxY, ok := darkInkVerticalBounds(historyImg, image.Rect(int(12*scale), int(13*scale), int(42*scale), int(43*scale)))
		if !ok {
			t.Fatalf("scale=%v: history back arrow ink missing", scale)
		}
		historyTitleMinY, historyTitleMaxY, ok := darkInkVerticalBounds(historyImg, image.Rect(int(42*scale), int(13*scale), int(120*scale), int(43*scale)))
		if !ok {
			t.Fatalf("scale=%v: history title ink missing", scale)
		}
		historyPenMinY, historyPenMaxY, ok := darkInkVerticalBounds(historyImg, image.Rect(int(204*scale), int(13*scale), int(234*scale), int(43*scale)))
		if !ok {
			t.Fatalf("scale=%v: history new-conversation icon ink missing", scale)
		}

		backCenter := float64(backMinY+backMaxY) / 2
		historyTitleCenter := float64(historyTitleMinY+historyTitleMaxY) / 2
		normalTitleCenter := float64(normalTitleMinY+normalTitleMaxY) / 2
		normalChevronCenter := float64(normalChevronMinY+normalChevronMaxY) / 2
		normalPenCenter := float64(normalPenMinY+normalPenMaxY) / 2
		historyPenCenter := float64(historyPenMinY+historyPenMaxY) / 2

		if diff := math.Abs(historyTitleCenter - backCenter); diff > float64(scale) {
			t.Fatalf("scale=%v: history title center %.1f and back arrow center %.1f differ by %.1fpx (title=[%d,%d], arrow=[%d,%d])",
				scale, historyTitleCenter, backCenter, diff, historyTitleMinY, historyTitleMaxY, backMinY, backMaxY)
		}
		if diff := math.Abs(normalTitleCenter - normalChevronCenter); diff > float64(scale) {
			t.Fatalf("scale=%v: normal title center %.1f and chevron center %.1f differ by %.1fpx (title=[%d,%d], chevron=[%d,%d])",
				scale, normalTitleCenter, normalChevronCenter, diff, normalTitleMinY, normalTitleMaxY, normalChevronMinY, normalChevronMaxY)
		}
		if diff := math.Abs(historyPenCenter - normalPenCenter); diff > 0.5 {
			t.Fatalf("scale=%v: header title area vertical position shifted when toggling history (normal pen center=%.1f, history pen center=%.1f)",
				scale, normalPenCenter, historyPenCenter)
		}
		if diff := math.Abs(historyTitleCenter - normalTitleCenter); diff > float64(scale) {
			t.Fatalf("scale=%v: history title center %.1f and normal title center %.1f differ by %.1fpx",
				scale, historyTitleCenter, normalTitleCenter, diff)
		}
	}
}

func TestAgentSidebarAssistantLabelOncePerTurn(t *testing.T) {
	a := New(nil, "")
	a.agentUI.chatLines = []agentChatLine{{role: "user", text: "问题"}, {role: "assistant", text: "先检查"}, {role: "tool", text: "检查完成"}, {role: "assistant", text: "继续处理"}, {role: "reasoning", text: "思考"}, {role: "assistant", text: "完成"}, {role: "user", text: "下个问题"}, {role: "assistant", text: "第二轮"}}
	var router input.Router
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(218, 1000)}}
	layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{}
		for i := range a.agentUI.chatLines {
			index := i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutAgentSidebarMessage(gtx, index, false) }))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}))
	router.Frame(&ops)
	count := 0
	seen := map[input.SemanticID]bool{}
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			if n.Desc.Label == "AI 助手" {
				count++
			}
			visit(n.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if count != 2 {
		t.Fatalf("assistant labels=%d, want one per user turn (2)", count)
	}
}

func TestAgentSidebarShortConversationStartsAtTop(t *testing.T) {
	a := New(nil, "")
	a.agentUI.chatLines = []agentChatLine{{role: "assistant", text: "简短回答"}}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(210, 450))}
	a.layoutAgentSidebarMessages(gtx)
	if a.agentUI.chatList.Position.Offset != 0 {
		t.Fatalf("short conversation bottom aligned: offset=%d", a.agentUI.chatList.Position.Offset)
	}
}

func TestAgentSidebarLongConversationFollowsEndAndPreservesHistory(t *testing.T) {
	a := New(nil, "")
	for i := 0; i < 30; i++ {
		a.agentUI.chatLines = append(a.agentUI.chatLines, agentChatLine{role: "assistant", text: "这是一段回复内容。"})
	}
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(210, 180))}
		a.layoutAgentSidebarMessages(gtx)
	}
	frame()
	if a.agentUI.chatList.Position.BeforeEnd {
		t.Fatal("long conversation does not follow final reply")
	}
	a.agentUI.chatLines = append(a.agentUI.chatLines, agentChatLine{role: "assistant", text: "新回复"})
	frame()
	if a.agentUI.chatList.Position.BeforeEnd {
		t.Fatal("new reply not followed")
	}
	a.agentUI.chatList.Position = layout.Position{First: 0, BeforeEnd: true}
	frame()
	first := a.agentUI.chatList.Position.First
	a.agentUI.chatLines = append(a.agentUI.chatLines, agentChatLine{role: "assistant", text: "新的流式回复"})
	frame()
	if a.agentUI.chatList.Position.First != first || !a.agentUI.chatList.Position.BeforeEnd {
		t.Fatal("new reply pulls user away from history")
	}
}

func TestAgentSidebarMouseScrollCanLeaveEnd(t *testing.T) {
	a := New(nil, "")
	for i := 0; i < 40; i++ {
		a.agentUI.chatLines = append(a.agentUI.chatLines, agentChatLine{role: "assistant", text: "这是一段回复内容。"})
	}
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(210, 180))}
		a.layoutAgentSidebarMessages(gtx)
		router.Frame(&ops)
	}
	frame()
	frame()
	a.agentUI.chatLines = append(a.agentUI.chatLines, agentChatLine{role: "assistant", text: "流式追加回复"})
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100, 80), Scroll: f32.Pt(0, -100)})
	frame()
	if !a.agentUI.chatList.Position.BeforeEnd {
		t.Fatal("mouse scroll up cannot leave the end")
	}
	first := a.agentUI.chatList.Position.First
	a.agentUI.chatLines = append(a.agentUI.chatLines, agentChatLine{role: "assistant", text: "新回复"})
	frame()
	if a.agentUI.chatList.Position.First != first {
		t.Fatal("new reply resets mouse scroll position")
	}
}

func TestAgentSidebarKeepsEndAfterReplyActionsAndWidthChange(t *testing.T) {
	a := New(nil, "")
	a.agentUI.conversationBusy = true
	for i := 0; i < 20; i++ {
		a.agentUI.chatLines = append(a.agentUI.chatLines, agentChatLine{role: "assistant", text: "这是一段足够长的回复，在较窄的侧栏中应当换行并保持最后内容可见。"})
	}
	var ops op.Ops
	frame := func(width int) {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, 180))}
		a.layoutAgentSidebarMessages(gtx)
	}
	frame(250)
	frame(250)
	a.agentUI.conversationBusy = false
	frame(250)
	if a.agentUI.chatList.Position.BeforeEnd {
		t.Fatal("response actions became hidden after completion")
	}
	frame(180)
	if a.agentUI.chatList.Position.BeforeEnd {
		t.Fatal("narrowing sidebar lost last reply")
	}
}

func TestAgentSidebarLoadedShortHistoryDiscardsOldScroll(t *testing.T) {
	a := New(nil, "")
	a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	a.documentID = "history-scroll"
	a.agentUI.conversationDocumentID = a.documentID
	a.agentUI.chatList.Position = layout.Position{Length: 2000, First: 20, BeforeEnd: true}
	a.agentUI.conversationHistoryResults = make(chan agentChatHistoryResult, 1)
	a.agentUI.conversationHistoryResults <- agentChatHistoryResult{kind: "load", found: true, documentID: a.documentID, snapshot: agent.ConversationSnapshot{ID: "short-history", Messages: []provider.ModelMessage{{Role: "assistant", Content: "短历史回答"}}}}
	a.processAgentChatHistoryResults()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(210, 450))}
	a.layoutAgentSidebarMessages(gtx)
	if a.agentUI.chatList.Position.Offset != 0 {
		t.Fatal("first frame of short history inherits old bottom alignment")
	}
}
