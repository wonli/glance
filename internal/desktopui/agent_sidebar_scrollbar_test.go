package desktopui

import (
	"image"
	"runtime"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

func TestAgentSidebarScrollbarHeightRemainsStableWhileScrolling(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tallMarkdown := strings.Join([]string{
		"### 详细分析与执行方案",
		"以下是针对各阶段转化率与渠道表现的完整复盘报告：",
		"",
		"- **阶段一（线索获取）**：通过搜索关键词与内容矩阵引流，覆盖核心高意向人群。",
		"- **阶段二（激活留存）**：结合自动化触达与首单激励策略，缩短首次转化周期。",
		"- **阶段三（复购扩张）**：按周监控留存曲线与客单价分布，动态调整投放权重。",
		"",
		"```sql",
		"SELECT channel, COUNT(*) AS leads, AVG(conversion_rate) AS conv",
		"FROM campaign_metrics",
		"WHERE quarter = '2026-Q3'",
		"GROUP BY channel",
		"ORDER BY conv DESC;",
		"```",
		"",
		"| 渠道 | 曝光量 | 转化率 | 趋势 |",
		"| :--- | :---: | :---: | :---: |",
		"| 搜索广告 | 128,000 | 4.8% | 上升 |",
		"| 内容社区 | 94,500 | 3.6% | 稳定 |",
		"| 私域触达 | 42,300 | 7.2% | 上升 |",
	}, "\n")

	mediumMarkdown := strings.Join([]string{
		"我已经核对了表中的异常记录：",
		"1. 华东区周三数据存在 2 条重复导入记录",
		"2. 建议先执行去重后再汇总月度报表",
	}, "\n")

	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.agentUI.chatLines = []agentChatLine{
			{role: "user", text: "第一问：请帮我生成一份完整的季度渠道复盘分析。"},
			newAgentAssistantLine(tallMarkdown),
			{role: "user", text: "好的"},
			newAgentAssistantLine(mediumMarkdown),
			{role: "user", text: "继续展开第二阶段的详细SQL和表格"},
			newAgentAssistantLine(tallMarkdown + "\n\n" + tallMarkdown),
			{role: "user", text: "收到，谢谢！"},
			newAgentAssistantLine("不客气！如需导出结果，请随时告诉我。"),
		}

		var ops op.Ops
		var router input.Router
		size := image.Pt(int(250*scale), int(701*scale))

		renderFrame := func() (minY, maxY, height int) {
			ops.Reset()
			gtx := layout.Context{
				Ops:         &ops,
				Source:      router.Source(),
				Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
				Constraints: layout.Exact(size),
			}
			a.layoutAgentSidebar(gtx)
			router.Frame(&ops)

			img := renderPresentationTestOps(t, &ops, size)
			minY, maxY = -1, -1
			xStart := int(238 * scale)
			xEnd := int(246 * scale)
			yStart := int(70 * scale)
			yEnd := int(590 * scale)
			for y := yStart; y < yEnd; y++ {
				rowHasThumb := false
				for x := xStart; x < xEnd; x++ {
					px := img.RGBAAt(x, y)
					// Sidebar background is #F8FAFC (248, 250, 252).
					// The scrollbar indicator is a darker semi-transparent gray blended over #F8FAFC.
					if px.R < 210 && px.G < 215 && px.B < 220 {
						rowHasThumb = true
						break
					}
				}
				if rowHasThumb {
					if minY == -1 {
						minY = y
					}
					maxY = y
				}
			}
			if minY == -1 || maxY == -1 {
				t.Fatalf("scale=%v: expected visible scrollbar thumb in chat message list", scale)
			}
			return minY, maxY, maxY - minY + 1
		}

		// Settle initial bottom-aligned layout.
		_, _, _ = renderFrame()
		startMinY, startMaxY, baselineHeight := renderFrame()
		if baselineHeight < int(24*scale) {
			t.Fatalf("scale=%v: unexpected small scrollbar thumb height %dpx", scale, baselineHeight)
		}

		scrollPoint := f32.Pt(125*scale, 260*scale)
		minSeenY := startMinY
		maxSeenY := startMaxY

		// Scroll upward step-by-step across variable-height message groups.
		for step := 0; step < 30; step++ {
			router.Queue(pointer.Event{
				Kind:     pointer.Scroll,
				Source:   pointer.Mouse,
				Position: scrollPoint,
				Scroll:   f32.Pt(0, -90*scale),
			})
			minY, maxY, h := renderFrame()
			if h != baselineHeight {
				t.Fatalf("scale=%v upward step=%d: scrollbar thumb height changed from %dpx to %dpx (minY=%d maxY=%d, First=%d Offset=%d BeforeEnd=%v)",
					scale, step, baselineHeight, h, minY, maxY,
					a.agentUI.chatList.Position.First,
					a.agentUI.chatList.Position.Offset,
					a.agentUI.chatList.Position.BeforeEnd,
				)
			}
			if minY < minSeenY {
				minSeenY = minY
			}
		}

		if minSeenY >= startMinY-int(100*scale) {
			t.Fatalf("scale=%v: expected scrollbar thumb to move near top while scrolling up (startMinY=%d minSeenY=%d)", scale, startMinY, minSeenY)
		}

		// Scroll downward step-by-step back to the bottom.
		for step := 0; step < 30; step++ {
			router.Queue(pointer.Event{
				Kind:     pointer.Scroll,
				Source:   pointer.Mouse,
				Position: scrollPoint,
				Scroll:   f32.Pt(0, 90*scale),
			})
			minY, maxY, h := renderFrame()
			if h != baselineHeight {
				t.Fatalf("scale=%v downward step=%d: scrollbar thumb height changed from %dpx to %dpx (minY=%d maxY=%d, First=%d Offset=%d BeforeEnd=%v)",
					scale, step, baselineHeight, h, minY, maxY,
					a.agentUI.chatList.Position.First,
					a.agentUI.chatList.Position.Offset,
					a.agentUI.chatList.Position.BeforeEnd,
				)
			}
			if maxY > maxSeenY {
				maxSeenY = maxY
			}
		}

		// Also test clicking on the upper scrollbar track to jump via ScrollDistance().
		trackClickPt := f32.Pt(242*scale, 100*scale)
		router.Queue(
			pointer.Event{
				Kind:     pointer.Press,
				Source:   pointer.Mouse,
				Buttons:  pointer.ButtonPrimary,
				Position: trackClickPt,
			},
			pointer.Event{
				Kind:     pointer.Release,
				Source:   pointer.Mouse,
				Position: trackClickPt,
			},
		)
		_, _, _ = renderFrame()
		jumpMinY, _, jumpHeight := renderFrame()
		if jumpHeight != baselineHeight {
			t.Fatalf("scale=%v track click: scrollbar thumb height changed from %dpx to %dpx", scale, baselineHeight, jumpHeight)
		}
		if jumpMinY >= startMinY {
			t.Fatalf("scale=%v track click: expected scrollbar thumb to jump upward (jumpMinY=%d startMinY=%d)", scale, jumpMinY, startMinY)
		}
	}
}

func TestAgentSidebarProcessToggleKeepsHeaderPositionAndAllowsImmediateScroll(t *testing.T) {
	a := New(nil, "")
	longReasoning := strings.Repeat("深入分析各区域销售增长与库存周转表现，对比环比趋势并核验明细数据。\n", 18)
	a.agentUI.chatLines = []agentChatLine{
		{role: "user", text: "帮我检查并总结这份销售报表。"},
		{role: "reasoning", text: longReasoning, collapsed: true, duration: 2_000_000_000, toggle: new(widget.Clickable)},
		{role: "tool", toolName: "document.inspect", toolSummary: "已读取 Sheet1 (A1:F40)", collapsed: true, toggle: new(widget.Clickable)},
		{role: "tool", toolName: "workbook.audit", toolSummary: "已完成公式与异常值检查", collapsed: true, toggle: new(widget.Clickable)},
		newAgentAssistantLine("报表核对完成：整体营收环比增长 **14.2%**，未发现公式错误。"),
	}

	var ops op.Ops
	var router input.Router
	size := image.Pt(250, 500)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size),
		}
		a.layoutAgentSidebar(gtx)
		router.Frame(&ops)
	}

	findDisclosureBounds := func() (image.Rectangle, bool) {
		var found image.Rectangle
		var ok bool
		seen := map[input.SemanticID]bool{}
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, n := range nodes {
				if seen[n.ID] {
					continue
				}
				seen[n.ID] = true
				if n.Desc.Description == "展开或折叠处理过程" {
					found = n.Desc.Bounds
					ok = true
					return
				}
				visit(n.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		return found, ok
	}

	// Frame 1: initial short conversation (collapsed process group).
	frame()
	beforeBounds, ok := findDisclosureBounds()
	if !ok {
		t.Fatal("expected process disclosure button in semantic tree")
	}
	if a.agentUI.chatList.Position.First != 0 || a.agentUI.chatList.Position.Offset != 0 {
		t.Fatalf("expected initial short conversation to start at top, got First=%d Offset=%d",
			a.agentUI.chatList.Position.First, a.agentUI.chatList.Position.Offset)
	}

	// Click disclosure button to expand process details.
	clickPt := f32.Pt(
		float32(beforeBounds.Min.X+beforeBounds.Dx()/2),
		float32(beforeBounds.Min.Y+beforeBounds.Dy()/2),
	)
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: clickPt},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: clickPt},
	)
	frame()

	if a.agentUI.chatLines[1].collapsed {
		t.Fatal("expected process group to expand on click frame")
	}
	afterExpandBounds, ok := findDisclosureBounds()
	if !ok {
		t.Fatal("expected process disclosure button to remain visible after expanding")
	}
	if afterExpandBounds.Min.Y != beforeBounds.Min.Y {
		t.Fatalf("expected process disclosure header Y to stay anchored at %d on expand, got %d",
			beforeBounds.Min.Y, afterExpandBounds.Min.Y)
	}
	if !a.agentUI.chatList.Position.BeforeEnd {
		t.Fatal("expected expanded tall conversation to have BeforeEnd=true so user can scroll down")
	}

	// Immediately scroll downward on the very next frame without any extra warm-up frame.
	router.Queue(pointer.Event{
		Kind:     pointer.Scroll,
		Source:   pointer.Mouse,
		Position: f32.Pt(120, 260),
		Scroll:   f32.Pt(0, 140),
	})
	frame()
	if a.agentUI.chatList.Position.First == 0 && a.agentUI.chatList.Position.Offset == 0 {
		t.Fatalf("expected immediate downward scroll after expanding process group, got First=%d Offset=%d",
			a.agentUI.chatList.Position.First, a.agentUI.chatList.Position.Offset)
	}

	// Scroll back up and click disclosure button to collapse; verify it stays top-aligned on the same frame.
	router.Queue(pointer.Event{
		Kind:     pointer.Scroll,
		Source:   pointer.Mouse,
		Position: f32.Pt(120, 260),
		Scroll:   f32.Pt(0, -200),
	})
	frame()
	collapseBounds, ok := findDisclosureBounds()
	if !ok {
		t.Fatal("expected process disclosure button visible after scrolling back up")
	}
	collapsePt := f32.Pt(
		float32(collapseBounds.Min.X+collapseBounds.Dx()/2),
		float32(collapseBounds.Min.Y+collapseBounds.Dy()/2),
	)
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: collapsePt},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: collapsePt},
	)
	frame()
	if !a.agentUI.chatLines[1].collapsed {
		t.Fatal("expected process group to collapse on click frame")
	}
	if a.agentUI.chatList.Position.First != 0 || a.agentUI.chatList.Position.Offset != 0 {
		t.Fatalf("expected collapsed short conversation to stay top-aligned on click frame, got First=%d Offset=%d",
			a.agentUI.chatList.Position.First, a.agentUI.chatList.Position.Offset)
	}
}
