package desktopui

import (
	"image"
	"image/png"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestAgentSidebarControlsReserveReplySpace(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		app := New(nil, "")
		for _, text := range []string{"", "中文问题", strings.Repeat("长问题需要滚动\n", 40)} {
			app.agentUI.chatInput.SetText(text)
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Min: image.Pt(int(218*scale), 0), Max: image.Pt(int(218*scale), int(600*scale))}}
			composer := app.layoutAgentSidebarComposer(gtx)
			if composer.Size.Y != int(99*scale) {
				t.Errorf("composer height = %d, want %d for draft length %d", composer.Size.Y, int(99*scale), len(text))
			}
		}
	}
}

func TestAgentSidebarReplyVisible(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		app := New(nil, "")
		app.agentUI.chatLines = []agentChatLine{{role: "assistant", text: "这份策划书围绕品牌与增长，明确了三个核心目标。"}}
		var ops op.Ops
		size := image.Pt(int(250*scale), int(701*scale))
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		app.layoutAgentSidebar(gtx)
		img := renderPresentationTestOps(t, &ops, size)
		app.agentUI.chatLines = nil
		var emptyOps op.Ops
		emptyGtx := gtx
		emptyGtx.Ops = &emptyOps
		app.layoutAgentSidebar(emptyGtx)
		empty := renderPresentationTestOps(t, &emptyOps, size)
		changed := 0
		for y := int(60 * scale); y < int(520*scale); y++ {
			for x := int(16 * scale); x < int(235*scale); x++ {
				if img.RGBAAt(x, y) != empty.RGBAAt(x, y) {
					changed++
				}
			}
		}
		if changed < 50 {
			t.Fatal("assistant reply is invisible in the message viewport")
		}
		assertPresentationPixel(t, img, image.Pt(int(30*scale), int(600*scale)), agentSidebarInputFill, "composer interior")
		assertPresentationPixel(t, img, image.Pt(int(14*scale), int(600*scale)), agentSidebarFill, "outside composer left edge")
		assertPresentationPixel(t, img, image.Pt(int(18*scale), int(600*scale)), agentSidebarInputFill, "inside composer left edge (16dp inset)")
		assertPresentationPixel(t, img, image.Pt(int(232*scale), int(600*scale)), agentSidebarInputFill, "inside composer right edge (16dp inset)")
		assertPresentationPixel(t, img, image.Pt(int(236*scale), int(600*scale)), agentSidebarFill, "outside composer right edge")
		assertPresentationPixel(t, img, image.Pt(int(100*scale), int(685*scale)), agentSidebarFill, "composer bottom margin")

		horizontalSpan := func(y int) (int, int) {
			left, right := size.X, -1
			bg := empty.RGBAAt(int(5*scale), y)
			for x := 0; x < int(245*scale); x++ {
				if img.RGBAAt(x, y) != bg {
					if x < left {
						left = x
					}
					if x > right {
						right = x
					}
				}
			}
			return left, right
		}
		replyLeft, replyRight := horizontalSpan(int(115 * scale))
		composerLeft, composerRight := horizontalSpan(int(610 * scale))
		if replyLeft != composerLeft || replyRight != composerRight {
			t.Fatalf("scale=%v: composer horizontal span [%d..%d] (width=%d) does not match AI reply span [%d..%d] (width=%d)",
				scale, composerLeft, composerRight, composerRight-composerLeft+1, replyLeft, replyRight, replyRight-replyLeft+1)
		}
		if path := os.Getenv("GLANCE_AGENT_SCREENSHOT"); path != "" && scale == 1 {
			f, err := os.Create(path)
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

func TestAgentSidebarUserBubbleFitsContent(t *testing.T) {
	app := New(nil, "")
	app.agentUI.chatLines = []agentChatLine{{role: "user", text: "请总结当前文档。"}}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(218, 0), Max: image.Pt(218, 10000)}}
	dims := app.layoutAgentSidebarMessage(gtx, 0, false)
	if dims.Size.Y > 120 {
		t.Fatalf("short user message fills scroll viewport: height %d", dims.Size.Y)
	}
}

func TestAgentRibbonTextFormsCenteredGroup(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		app := New(nil, "")
		var ops op.Ops
		size := image.Pt(int(40*scale), int(56*scale))
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		app.layoutAgentRibbonEntry(gtx)
		img := renderPresentationTestOps(t, &ops, size)
		_, bottom, ok := darkInkVerticalBounds(img, image.Rect(int(5*scale), 0, int(34*scale), int(29*scale)))
		top, _, ok2 := darkInkVerticalBounds(img, image.Rect(int(5*scale), int(29*scale), int(34*scale), int(56*scale)))
		if !ok || !ok2 || top-bottom > int(12*scale) {
			t.Fatalf("ribbon text is spread apart at scale %v: AI bottom %d, subtitle top %d", scale, bottom, top)
		}
	}
}

func TestAgentSidebarComposerBlankEdgesFocusChineseInput(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, y := range []float32{2, 45, 95} {
			app := New(nil, "")
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Min: image.Pt(int(218*scale), 0), Max: image.Pt(int(218*scale), int(600*scale))}}
				app.layoutAgentSidebarComposer(gtx)
				router.Frame(&ops)
			}
			frame()
			pos := f32.Pt(100*scale, y*scale)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: pos, Buttons: pointer.ButtonPrimary})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: pos})
			frame()
			frame()
			if !router.Source().Focused(&app.agentUI.chatInput) {
				t.Fatalf("click at y=%v scale=%v did not focus draft", y, scale)
			}
			router.Queue(key.EditEvent{Text: "中文"})
			frame()
			if app.agentUI.chatInput.Text() != "中文" {
				t.Fatal("Chinese input was lost")
			}
			router.Queue(key.Event{Name: key.NameEnter, State: key.Press, Modifiers: key.ModShift})
			frame()
			router.Queue(key.EditEvent{Text: "下一行"})
			frame()
			if !strings.Contains(app.agentUI.chatInput.Text(), "\n") {
				t.Fatal("Shift+Enter must preserve a newline in the bounded draft")
			}
			app.agentUI.chatInput.SetText("")
			router.Queue(key.EditEvent{Text: "第一行\n第二行"})
			frame()
			if app.agentUI.chatInput.Text() != "第一行\n第二行" {
				t.Fatal("multiline paste was truncated or submitted")
			}
			app.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
			app.documentSessionState.documentID = "composer-enter-test"
			app.agentUI.conversationDocumentID = "composer-enter-test"
			app.agentUI.conversationID = "conversation-enter-test"
			router.Queue(key.Event{Name: key.NameEnter, State: key.Press})
			frame()
			if app.agentUI.chatInput.Text() != "" || len(app.agentUI.chatLines) != 1 {
				t.Fatal("plain Enter did not send the committed draft exactly once")
			}
		}
	}
}

func TestAgentSidebarCloseUsesFullWorkspaceImmediately(t *testing.T) {
	app := New(nil, "")
	app.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	app.documentSessionState.documentID = "close-sidebar-test"
	app.agentUI.conversationDocumentID = "close-sidebar-test"
	app.agentUI.sidebarVisible = true
	app.agentUI.sidebarCloseButton.Click()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(900, 701))}
	width := 0
	app.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
		width = gtx.Constraints.Max.X
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if width != 900 || app.agentUI.sidebarVisible {
		t.Fatalf("close frame retains sidebar: content width=%d visible=%v", width, app.agentUI.sidebarVisible)
	}
}
