package desktopui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestAgentAssistantReplyRendersMarkdownBlocksAndInlineStyles(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	markdownReply := strings.Join([]string{
		"这份策划书围绕品牌与增长，明确了三个核心目标：",
		"",
		"### 01 提升品牌认知",
		"- 提升 **品牌搜索量** 与 `关键词` 覆盖率",
		"- 完成重点城市的渠道联合推广",
		"",
		"> 建议按周复盘核心指标。",
		"",
		"```sql",
		"SELECT brand, growth FROM metrics;",
		"```",
		"",
		"| 指标 | 目标 |",
		"| :--- | :---: |",
		"| 搜索量 | +30% |",
	}, "\n")

	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.agentUI.chatLines = []agentChatLine{
			{role: "user", text: "帮我总结这份策划书的核心目标。"},
			newAgentAssistantLine(markdownReply),
		}

		blocks := a.agentUI.chatLines[1].ensureMarkdownBlocks()
		if len(blocks) < 6 {
			t.Fatalf("expected parsed markdown blocks (paragraph, heading, list items, quote, code, table), got %d blocks", len(blocks))
		}

		var ops op.Ops
		var router input.Router
		size := image.Pt(int(250*scale), int(701*scale))
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(size),
		}
		a.layoutAgentSidebar(gtx)
		router.Frame(&ops)

		labels := collectSemanticLabels(&router)
		joined := strings.Join(labels, "\n")
		for _, unexpectedRaw := range []string{"### 01", "**品牌搜索量**", "```sql"} {
			if strings.Contains(joined, unexpectedRaw) {
				t.Fatalf("scale=%v: assistant reply leaked raw markdown syntax %q in semantic labels:\n%s", scale, unexpectedRaw, joined)
			}
		}
		for _, wantParsed := range []string{
			"这份策划书围绕品牌与增长，明确了三个核心目标：",
			"01 提升品牌认知",
			"提升 品牌搜索量 与 关键词 覆盖率",
			"建议按周复盘核心指标。",
			"SELECT brand, growth FROM metrics;",
			"搜索量",
		} {
			if !strings.Contains(joined, wantParsed) {
				t.Fatalf("scale=%v: rendered markdown missing parsed content %q in semantic labels:\n%s", scale, wantParsed, joined)
			}
		}

		img := renderPresentationTestOps(t, &ops, size)
		countColor := func(want color.RGBA) int {
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
		// Code block / table header background (#F1F4F8)
		if countColor(color.RGBA{241, 244, 248, 255}) < int(200*scale*scale) {
			t.Fatalf("scale=%v: expected markdown code block / table header fill #F1F4F8 to be rendered", scale)
		}
		// Inline code pill background (#ECF0F5)
		if countColor(color.RGBA{236, 240, 245, 255}) < int(20*scale*scale) {
			t.Fatalf("scale=%v: expected markdown inline code pill fill #ECF0F5 to be rendered", scale)
		}
		// Blockquote left bar (#CDD5DF)
		if countColor(color.RGBA{205, 213, 223, 255}) < int(15*scale*scale) {
			t.Fatalf("scale=%v: expected markdown blockquote left bar #CDD5DF to be rendered", scale)
		}

		if scale == 1 {
			f, err := os.Create("/private/tmp/glance-agent-markdown.png")
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
