package desktopui

import (
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestAgentBubblesMatchPenColorsAndSeparateReasoning(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.agentUI.conversationStartedAt = time.Date(2026, 10, 12, 10, 42, 0, 0, time.Local)
		a.agentUI.chatLines = []agentChatLine{
			{role: "user", text: "帮我总结这份策划书的核心目标。"},
			{role: "reasoning", text: "先检查文档，再梳理关键目标。", collapsed: false, duration: 3 * time.Second},
			{role: "tool", text: "工具 word.list_paragraphs 已完成", toolName: "word.list_paragraphs", toolSummary: "已读取 12 段正文与 3 级标题结构", collapsed: false},
			{role: "assistant", text: "这份策划书围绕品牌与增长，明确了三个核心目标。"},
		}
		var ops op.Ops
		size := image.Pt(int(250*scale), int(500*scale))
		gtx := layout.Context{Ops: &ops, Now: time.Date(2026, 10, 12, 12, 0, 0, 0, time.Local), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		paint.FillShape(gtx.Ops, agentSidebarFill, clip.Rect{Max: size}.Op())
		a.layoutAgentSidebarMessages(gtx)
		img := renderPresentationTestOps(t, &ops, size)
		if _, _, ok := darkInkVerticalBounds(img, image.Rect(int(90*scale), int(12*scale), int(160*scale), int(32*scale))); !ok {
			t.Fatal("conversation time divider is not centered")
		}
		count := func(want color.RGBA) int {
			n := 0
			for y := 0; y < size.Y; y++ {
				for x := 0; x < size.X; x++ {
					if img.RGBAAt(x, y) == want {
						n++
					}
				}
			}
			return n
		}
		if count(color.RGBA{222, 230, 239, 255}) < int(1000*scale*scale) {
			t.Fatal("Pen user bubble fill #DEE6EF missing")
		}
		if count(color.RGBA{250, 251, 252, 255}) < int(1000*scale*scale) {
			t.Fatal("Pen AI body bubble fill #FAFBFC missing")
		}
		if count(color.RGBA{205, 213, 223, 255}) < int(20*scale*scale) {
			t.Fatal("Pen process left bar fill #CDD5DF missing")
		}
		if count(color.RGBA{236, 238, 241, 255}) < int(1000*scale*scale) {
			t.Fatal("reasoning and actions need sidebar background outside the AI bubble")
		}
		if scale == 1 {
			f, err := os.Create("/private/tmp/glance-chat-bubbles.png")
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

func TestAgentChatShowsConversationTimeDivider(t *testing.T) {
	a := New(nil, "")
	a.agentUI.conversationStartedAt = time.Date(2026, 10, 12, 10, 42, 0, 0, time.Local)
	a.agentUI.chatLines = []agentChatLine{{role: "user", text: "总结文档"}}
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Date(2026, 10, 12, 12, 0, 0, 0, time.Local), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(250, 500))}
	a.layoutAgentSidebarMessages(gtx)
	router.Frame(&ops)
	found := false
	for _, n := range router.AppendSemantics(nil) {
		if n.Desc.Label == "今天 10:42" {
			found = true
		}
	}
	if !found {
		t.Fatal("conversation time divider is missing")
	}
}
