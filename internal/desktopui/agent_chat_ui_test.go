package desktopui

import (
	"context"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/agent"
	"glance/internal/agent/provider"
)

func TestAgentChatEventsKeepLongRepliesAndRenderToolAndErrorStatus(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	if len(app.agentUI.chatLines) != 0 {
		t.Fatalf("new chat is not empty: %+v", app.agentUI.chatLines)
	}
	if !app.agentUI.chatList.ScrollToEnd {
		t.Fatal("chat list does not follow the newest message")
	}

	longReply := strings.Repeat("工资汇总已核对。", 1500)
	for _, event := range []agent.AgentEvent{
		{ConversationID: "window-1", Type: agent.EventAssistantDelta, Text: longReply[:len(longReply)/2]},
		{ConversationID: "window-1", Type: agent.EventAssistantDelta, Text: longReply[len(longReply)/2:]},
		{ConversationID: "window-1", Type: agent.EventToolStarted, ToolName: "workbook.audit"},
		{ConversationID: "window-1", Type: agent.EventToolCompleted, ToolName: "workbook.audit"},
		{ConversationID: "window-1", Type: agent.EventTurnFailed, ErrorCode: "network", Error: "模型连接中断，请检查网络。"},
	} {
		app.agentUI.chatEvents <- event
	}
	app.agentUI.conversationBusy = true
	app.processAgentChatEvents()

	if len(app.agentUI.chatLines) != 3 {
		t.Fatalf("chat lines = %+v; want assistant, tool, and error", app.agentUI.chatLines)
	}
	if app.agentUI.chatLines[0].role != "assistant" || app.agentUI.chatLines[0].text != longReply {
		t.Fatalf("streamed assistant reply was split or truncated: role=%q bytes=%d", app.agentUI.chatLines[0].role, len(app.agentUI.chatLines[0].text))
	}
	if app.agentUI.chatLines[1].role != "tool" || app.agentUI.chatLines[1].text != "工具 workbook.audit 已完成" {
		t.Fatalf("tool progress line = %+v", app.agentUI.chatLines[1])
	}
	if app.agentUI.chatLines[2].role != "error" || app.agentUI.chatLines[2].text != "模型连接中断，请检查网络。" {
		t.Fatalf("model error line = %+v", app.agentUI.chatLines[2])
	}
	if app.agentUI.conversationBusy || app.agentUI.chatStatus != "" {
		t.Fatalf("failed turn state busy=%v status=%q", app.agentUI.conversationBusy, app.agentUI.chatStatus)
	}
}

func TestAgentChatSendAndStopUseBackgroundHostTurn(t *testing.T) {
	ctx := context.Background()
	desktopRuntime := newDesktopRuntime()
	t.Cleanup(desktopRuntime.shutdown)
	app := newAppWithRuntime(nil, desktopRuntime)
	desktopRuntime.attachWindow(1, app)

	client, err := agent.ConnectMCP(ctx, desktopRuntime.applicationRuntime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	model := &chatBlockingProvider{started: make(chan struct{})}
	host, err := agent.NewHost(ctx, model, client.Session, agent.HostOptions{Model: "chat-ui-test"})
	if err != nil {
		_ = client.Close()
		t.Fatalf("NewHost: %v", err)
	}
	desktopRuntime.agentMu.Lock()
	desktopRuntime.agentClient, desktopRuntime.agentHost = client, host
	desktopRuntime.agentMu.Unlock()
	t.Cleanup(func() {
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		app.closeAgentSubscription()
		app.stopAndWaitManagedBackground()
		app.closeWorkspace()
	})

	app.agentUI.chatInput.SetText("  请生成工资日报  ")
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(820, 500))}
	app.agentUI.sendButton.Click()
	app.layoutAgentChat(gtx)
	if !app.agentUI.conversationBusy || app.agentUI.chatInput.Text() != "" || len(app.agentUI.chatLines) != 1 ||
		app.agentUI.chatLines[0].role != "user" || app.agentUI.chatLines[0].text != "请生成工资日报" {
		t.Fatalf("send did not establish a clean active turn: busy=%v input=%q lines=%+v", app.agentUI.conversationBusy, app.agentUI.chatInput.Text(), app.agentUI.chatLines)
	}
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("chat send did not reach the model provider in a background task")
	}

	app.agentUI.stopButton.Click()
	app.layoutAgentChat(gtx)
	if app.agentUI.chatStatus != "正在停止…" {
		t.Fatalf("stop status = %q", app.agentUI.chatStatus)
	}
	deadline := time.After(2 * time.Second)
	for app.agentUI.conversationBusy {
		app.processAgentChatEvents()
		if !app.agentUI.conversationBusy {
			break
		}
		select {
		case <-time.After(time.Millisecond):
		case <-deadline:
			t.Fatal("Agent chat remained busy after stop")
		}
	}
	app.processAgentChatEvents()
	if app.agentUI.conversationBusy || app.agentUI.chatStatus != "" {
		t.Fatalf("stopped turn state busy=%v status=%q", app.agentUI.conversationBusy, app.agentUI.chatStatus)
	}
	var stoppedMessage bool
	for _, line := range app.agentUI.chatLines {
		stoppedMessage = stoppedMessage || line.role == "error" && line.text == "请求已停止。"
	}
	if !stoppedMessage {
		t.Fatalf("chat did not report the stopped turn: %+v", app.agentUI.chatLines)
	}
}

func TestAgentChatDoesNotSubmitChineseIMEComposition(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(820, 500))}
		app.layoutAgentChat(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Source().Execute(key.FocusCmd{Tag: &app.agentUI.chatInput})
	frame()
	router.Queue(
		key.CompositionEvent{Start: 0, End: 1},
		key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "中"},
		key.Event{Name: key.NameEnter, State: key.Press},
	)
	frame()
	if !editorHasActiveComposition(&app.agentUI.chatInput) {
		t.Fatal("Gio editor did not retain the active Chinese composition")
	}
	if len(app.agentUI.chatLines) != 0 || app.agentUI.chatInput.Text() != "中" {
		t.Fatalf("Enter submitted or cleared intermediate IME text: lines=%+v input=%q", app.agentUI.chatLines, app.agentUI.chatInput.Text())
	}
	app.sendAgentChatMessage()
	if len(app.agentUI.chatLines) != 0 || app.agentUI.chatInput.Text() != "中" {
		t.Fatalf("send action submitted or cleared active IME text: lines=%+v input=%q", app.agentUI.chatLines, app.agentUI.chatInput.Text())
	}

	router.Queue(key.CompositionEvent{Start: -1, End: -1})
	frame()
	if editorHasActiveComposition(&app.agentUI.chatInput) {
		t.Fatal("Gio editor remained in composition after the commit event")
	}
	app.sendAgentChatMessage()
	if app.agentUI.chatInput.Text() != "" || len(app.agentUI.chatLines) != 1 || app.agentUI.chatLines[0].role != "error" {
		t.Fatalf("committed Chinese text was not handled as an ordinary send: lines=%+v input=%q", app.agentUI.chatLines, app.agentUI.chatInput.Text())
	}
}

type chatBlockingProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *chatBlockingProvider) Stream(ctx context.Context, _ provider.ModelRequest, _ func(provider.ModelEvent) error) error {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return ctx.Err()
}
