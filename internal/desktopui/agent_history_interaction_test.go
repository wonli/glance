package desktopui

import (
	"context"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/agent"
	"glance/internal/workspace"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAgentHistoryHoverDeleteDoesNotSelectConversation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.agentUI.conversationDocumentID = "doc"
		a.agentUI.conversationID = "current"
		item := agent.ConversationSummary{ID: "other", Title: "优化推广计划措辞", UpdatedAt: time.Now()}
		var router input.Router
		var ops op.Ops
		size := image.Pt(int(226*scale), int(56*scale))
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			a.layoutAgentHistoryRow(gtx, item, time.Now())
			router.Frame(&ops)
		}
		frame()
		normal := renderPresentationTestOps(t, &ops, size)
		pos := f32.Pt(203*scale, 28*scale)
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: pos})
		frame()
		frame()
		hovered := renderPresentationTestOps(t, &ops, size)
		changed := 0
		for y := int(15 * scale); y < int(41*scale); y++ {
			for x := int(190 * scale); x < int(216*scale); x++ {
				if normal.RGBAAt(x, y) != hovered.RGBAAt(x, y) {
					changed++
				}
			}
		}
		if changed < int(40*scale*scale) {
			t.Fatal("hover delete button is not visible")
		}
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		frame()
		frame()
		if !a.modal.Visible || a.modal.Title != "删除这条会话？" {
			t.Fatal("hover delete click did not show confirmation")
		}
		if a.agentUI.conversationID != "current" || a.agentUI.conversationHistoryLoading {
			t.Fatal("delete click selected or deleted conversation before confirmation")
		}
		a.modal.OnAction("cancel")
		frame()
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(240*scale, 80*scale)})
		frame()
		frame()
		if a.agentUI.conversationHistoryButtons[item.ID].Hovered() || a.agentUI.conversationHistoryDeleteButtons[item.ID].Hovered() {
			t.Fatal("delete hover stuck after pointer left row")
		}
	}
}

func TestAgentHistoryConfirmedDeleteRemovesStoredCurrentConversation(t *testing.T) {
	r := newDesktopRuntime()
	t.Cleanup(r.shutdown)
	s, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	r.workspaceStore = s
	a := newAppWithRuntime(nil, r)
	t.Cleanup(func() { a.shutdownOnce.Do(func() { close(a.shutdown) }); a.stopAndWaitManagedBackground() })
	a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	a.documentID = "doc"
	a.agentUI.conversationDocumentID = "doc"
	a.agentUI.conversationID = "current"
	a.agentUI.chatLines = []agentChatLine{{role: "user", text: "旧消息"}}
	repo := workspaceAgentConversationStore{runtime: r}
	for _, id := range []agent.ConversationID{"current", "keep"} {
		if err := repo.Save(context.Background(), agent.ConversationRecord{ID: id, DocumentID: "doc", Title: string(id)}); err != nil {
			t.Fatal(err)
		}
	}
	a.agentUI.conversationHistory = []agent.ConversationSummary{{ID: "current", Title: "当前会话"}, {ID: "keep", Title: "保留会话"}}
	a.confirmAgentConversationDelete(a.agentUI.conversationHistory[0])
	a.modal.OnAction("delete")
	a.cancelAgentChatHistoryRequest() // unrelated history refresh must not cancel confirmed deletion
	deadline := time.Now().Add(3 * time.Second)
	for len(a.agentUI.conversationDeletes) > 0 && time.Now().Before(deadline) {
		a.processAgentChatHistoryResults()
		time.Sleep(time.Millisecond)
	}
	if len(a.agentUI.conversationDeletes) > 0 || a.agentUI.conversationHistoryError != "" {
		t.Fatalf("delete not completed: %s", a.agentUI.conversationHistoryError)
	}
	if _, found, err := repo.Load(context.Background(), "doc", "current"); err != nil || found {
		t.Fatal("confirmed conversation remains stored")
	}
	if _, found, err := repo.Load(context.Background(), "doc", "keep"); err != nil || !found {
		t.Fatal("unrelated conversation deleted")
	}
	if a.agentUI.conversationID != "" || len(a.agentUI.chatLines) != 0 {
		t.Fatal("deleted current transcript remains visible")
	}
	if len(a.agentUI.conversationHistory) != 1 || a.agentUI.conversationHistory[0].ID != "keep" {
		t.Fatal("history list did not remove only the deleted item")
	}
	a.agentUI.conversationHistoryResults <- agentChatHistoryResult{kind: "list", generation: a.agentUI.conversationHistoryGeneration, documentID: "doc", summaries: []agent.ConversationSummary{{ID: "current"}, {ID: "keep"}}}
	a.processAgentChatHistoryResults()
	if len(a.agentUI.conversationHistory) != 1 || a.agentUI.conversationHistory[0].ID != "keep" {
		t.Fatal("stale list restored deleted conversation")
	}

}

func TestAgentHistoryAndDeleteDialogVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.agentUI.conversationHistoryVisible = true
		a.agentUI.conversationDocumentID = "doc"
		a.agentUI.conversationID = "active"
		now := time.Date(2026, 10, 12, 10, 42, 0, 0, time.Local)
		for i, title := range []string{"核心目标梳理", "优化推广计划措辞", "生成渠道执行清单", "渠道预算与资源分配", "策划书结构调整"} {
			id := agent.ConversationID(title)
			if i == 0 {
				id = "active"
			}
			a.agentUI.conversationHistory = append(a.agentUI.conversationHistory, agent.ConversationSummary{ID: id, Title: title, UpdatedAt: now.Add(-time.Duration(i) * 12 * time.Hour)})
		}
		var ops op.Ops
		size := image.Pt(int(250*scale), int(701*scale))
		gtx := layout.Context{Ops: &ops, Now: now, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		a.layoutAgentHistorySidebar(gtx)
		img := renderPresentationTestOps(t, &ops, size)
		if _, _, ok := darkInkVerticalBounds(img, image.Rect(int(22*scale), int(140*scale), int(90*scale), int(168*scale))); !ok {
			t.Fatal("history title does not start at the left content inset")
		}
		if scale == 1 {
			saveAgentHistoryScreenshot(t, "/private/tmp/glance-history-sidebar.png", img)
		}
		a.confirmAgentConversationDelete(a.agentUI.conversationHistory[1])
		ops.Reset()
		size = image.Pt(int(800*scale), int(600*scale))
		gtx.Constraints = layout.Exact(size)
		dims := a.layoutAgentDeleteDialog(gtx)
		if dims.Size.X != int(440*scale) || dims.Size.Y > int(240*scale) {
			t.Fatalf("delete card size %v does not match rFL4D unified confirm dialog geometry", dims.Size)
		}
		ops.Reset()
		paint.FillShape(gtx.Ops, agentSidebarFill, clip.Rect{Max: size}.Op())
		a.layoutModalDialog(gtx)
		img = renderPresentationTestOps(t, &ops, size)
		hasColor := func(want color.RGBA) int {
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
		if hasColor(color.RGBA{199, 68, 68, 255}) < int(500*scale*scale) {
			t.Fatal("rFL4D danger button fill #C74444 missing")
		}
		if hasColor(color.RGBA{253, 238, 238, 255}) < int(200*scale*scale) {
			t.Fatal("rFL4D danger icon badge fill #FDEEEE missing")
		}
		if scale == 1 {
			saveAgentHistoryScreenshot(t, "/private/tmp/glance-history-delete.png", img)
		}
	}
}

func TestAgentDeleteDialogButtonsClickableOverSpreadsheetWorkspace(t *testing.T) {
	a := New(nil, "")
	t.Cleanup(func() {
		a.shutdownOnce.Do(func() { close(a.shutdown) })
		a.stopAndWaitManagedBackground()
		if a.desktopRuntime != nil {
			a.desktopRuntime.shutdown()
		}
	})
	a.documentSessionState = newDocumentSessionState("")
	a.book = &toolbarTestBook{editable: true}
	a.documentID = "doc"
	a.activeTabID = "doc"
	a.sheet = "Sheet1"
	a.agentUI.sidebarVisible = true
	a.agentUI.conversationHistoryVisible = true
	a.agentUI.conversationDocumentID = "doc"
	item := agent.ConversationSummary{ID: "to-delete", Title: "优化推广计划措辞", UpdatedAt: time.Now()}
	a.agentUI.conversationHistory = []agent.ConversationSummary{item}
	a.selectedCell = "A1"

	var router input.Router
	var ops op.Ops
	size := image.Pt(800, 600)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size),
		}
		a.layout(gtx)
		router.Frame(&ops)
	}

	a.confirmAgentConversationDelete(item)
	frame()
	if !a.modal.Visible {
		t.Fatal("delete dialog should be visible")
	}

	// Click "取消" button inside the centered 440px rFL4D card (Cancel button is around x=440, y=365).
	cancelPos := f32.Pt(440, 365)
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: cancelPos},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: cancelPos},
	)
	frame()
	if a.modal.Visible {
		t.Fatal("clicking Cancel button did not close delete modal")
	}
	if a.selectedCell != "A1" {
		t.Fatalf("clicking Cancel button leaked through to spreadsheet grid and selected %s", a.selectedCell)
	}

	// Re-open and click "删除" button (Delete button is around x=540, y=365).
	a.confirmAgentConversationDelete(item)
	frame()
	deletePos := f32.Pt(540, 365)
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: deletePos},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: deletePos},
	)
	frame()
	if a.modal.Visible {
		t.Fatal("clicking Delete button did not close delete modal")
	}
	if a.selectedCell != "A1" {
		t.Fatalf("clicking Delete button leaked through to spreadsheet grid and selected %s", a.selectedCell)
	}
}
func saveAgentHistoryScreenshot(t *testing.T, path string, img *image.RGBA) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
