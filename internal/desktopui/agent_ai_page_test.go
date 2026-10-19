package desktopui

import (
	"context"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/agentsettings"
	"glance/internal/workspace"
)

func newTestAIPageApp(t *testing.T) *App {
	t.Helper()
	app := New(nil, "")
	t.Cleanup(func() {
		close(app.shutdown)
	})
	dir := t.TempDir()
	store, err := workspace.Open(filepath.Join(dir, "workspace.db"))
	if err != nil {
		t.Fatalf("open workspace store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	rt := app.ensureDesktopRuntime()
	rt.workspaceStore = store
	credStore := &desktopMemoryCredentials{items: make(map[string]string)}
	settingsStore, err := agentsettings.NewStore(filepath.Join(dir, "settings.json"), credStore)
	if err != nil {
		t.Fatalf("open settings store: %v", err)
	}
	rt.agentSettings = settingsStore
	return app
}

func layoutTestFrame(app *App, size image.Point) layout.Dimensions {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: size},
	}
	return app.layoutRecent(gtx)
}

func TestAIAssistantNavigation(t *testing.T) {
	app := newTestAIPageApp(t)
	size := image.Pt(1440, 900)

	// Initial view is Recent.
	layoutTestFrame(app, size)
	if app.homeSection != homeSectionRecent {
		t.Fatalf("initial section=%v, want %v", app.homeSection, homeSectionRecent)
	}

	// Click AI 助手 navigation item.
	app.homeNavButton("ai-assistant").Click()
	layoutTestFrame(app, size)

	if app.homeSection != homeSectionAIAssistant {
		t.Fatalf("section after clicking ai-assistant=%v, want %v", app.homeSection, homeSectionAIAssistant)
	}
	if app.agentUI.conversationDocumentID != GlobalAIAssistantDocumentID {
		t.Fatalf("conversationDocumentID=%q, want %q", app.agentUI.conversationDocumentID, GlobalAIAssistantDocumentID)
	}

	// Switch back to Recent.
	app.homeNavButton("recent").Click()
	layoutTestFrame(app, size)
	if app.homeSection != homeSectionRecent {
		t.Fatalf("section after clicking recent=%v, want %v", app.homeSection, homeSectionRecent)
	}

	// Switch back to AI Assistant: state is preserved.
	app.homeNavButton("ai-assistant").Click()
	layoutTestFrame(app, size)
	if app.homeSection != homeSectionAIAssistant {
		t.Fatalf("section after re-clicking ai-assistant=%v, want %v", app.homeSection, homeSectionAIAssistant)
	}
}

func TestAIAssistantEmptyStateSuggestions(t *testing.T) {
	app := newTestAIPageApp(t)
	size := image.Pt(1440, 900)

	app.homeSection = homeSectionAIAssistant
	app.ensureGlobalAIAssistant()
	layoutTestFrame(app, size)

	if len(app.agentUI.chatLines) != 0 {
		t.Fatalf("expected 0 chat lines in empty state, got %d", len(app.agentUI.chatLines))
	}

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(780, 600)},
	}
	emptyDims := app.layoutAIAssistantEmptyState(gtx)
	if emptyDims.Size.Y > 200 {
		t.Fatalf("empty state height=%d, expected compact height < 200 (suggestion buttons should not expand vertically)", emptyDims.Size.Y)
	}

	// Click suggestion chip 0 ("总结一份文档").
	app.aiSuggestionButtons[0].Click()
	layoutTestFrame(app, size)

	if text := app.agentUI.chatInput.Text(); text != "总结一份文档" {
		t.Fatalf("expected chatInput '总结一份文档', got %q", text)
	}

	// Click suggestion chip 1 ("梳理项目计划").
	app.aiSuggestionButtons[1].Click()
	layoutTestFrame(app, size)

	if text := app.agentUI.chatInput.Text(); text != "梳理项目计划" {
		t.Fatalf("expected chatInput '梳理项目计划', got %q", text)
	}
}

func TestAIAssistantActiveConversation(t *testing.T) {
	app := newTestAIPageApp(t)
	size := image.Pt(1440, 900)

	app.homeSection = homeSectionAIAssistant
	app.agentUI.conversationDocumentID = GlobalAIAssistantDocumentID
	app.agentUI.conversationTitle = "核心目标梳理"
	app.agentUI.chatLines = []agentChatLine{
		{role: "user", text: "结合这份策划书，帮我梳理核心目标。"},
		{role: "assistant", text: "核心目标可归纳为三项：01 品牌认知 02 渠道协同 03 复盘机制。"},
	}

	dims := layoutTestFrame(app, size)
	if dims.Size.X == 0 || dims.Size.Y == 0 {
		t.Fatalf("layout produced zero size: %v", dims.Size)
	}

	// Copy response button.
	app.agentUI.copyResponseButton.Click()
	layoutTestFrame(app, size)
	if app.statusText() != "已复制回答。" {
		t.Fatalf("status after copy=%q, want '已复制回答。'", app.statusText())
	}
}

func TestAIAssistantHistoryDrawer(t *testing.T) {
	app := newTestAIPageApp(t)
	size := image.Pt(1440, 900)

	app.homeSection = homeSectionAIAssistant
	app.agentUI.conversationDocumentID = GlobalAIAssistantDocumentID
	now := time.Now()
	summary1 := agent.ConversationSummary{
		ID:        agent.ConversationID("conv-1"),
		Title:     "核心目标梳理",
		CreatedAt: now,
		UpdatedAt: now,
	}
	summary2 := agent.ConversationSummary{
		ID:        agent.ConversationID("conv-2"),
		Title:     "周报草稿润色",
		CreatedAt: now.Add(-2 * time.Hour),
		UpdatedAt: now.Add(-2 * time.Hour),
	}
	app.agentUI.conversationHistory = []agent.ConversationSummary{summary1, summary2}

	// Initially drawer is closed.
	layoutTestFrame(app, size)
	if app.aiHistoryDrawerOpen {
		t.Fatal("drawer should initially be closed")
	}

	// Click drawer toggle button.
	app.aiDrawerOpenBtn.Click()
	layoutTestFrame(app, size)
	if !app.aiHistoryDrawerOpen {
		t.Fatal("drawer should be open after clicking aiDrawerOpenBtn")
	}

	// Click drawer close button.
	app.aiHistoryDrawerCloseBtn.Click()
	layoutTestFrame(app, size)
	if app.aiHistoryDrawerOpen {
		t.Fatal("drawer should be closed after clicking aiHistoryDrawerCloseBtn")
	}

	// Open again, then click overlay to close.
	app.aiDrawerOpenBtn.Click()
	layoutTestFrame(app, size)
	if !app.aiHistoryDrawerOpen {
		t.Fatal("drawer should be open")
	}
	app.aiHistoryDrawerOverlay.Click()
	layoutTestFrame(app, size)
	if app.aiHistoryDrawerOpen {
		t.Fatal("drawer should be closed after clicking overlay")
	}
}

func TestAIAssistantHistorySelection(t *testing.T) {
	app := newTestAIPageApp(t)
	size := image.Pt(1440, 900)

	app.homeSection = homeSectionAIAssistant
	app.agentUI.conversationDocumentID = GlobalAIAssistantDocumentID

	// Save a conversation in workspace store so it can be loaded.
	record := agent.ConversationRecord{
		ID:         agent.ConversationID("conv-target"),
		DocumentID: GlobalAIAssistantDocumentID,
		Title:      "目标会话内容",
		Messages: []provider.ModelMessage{
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "你好！有什么我可以帮你的吗？"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo := workspaceAgentConversationStore{runtime: app.ensureDesktopRuntime()}
	if err := repo.Save(context.Background(), record); err != nil {
		t.Fatalf("save conversation record: %v", err)
	}

	summary := agent.ConversationSummary{
		ID:        record.ID,
		Title:     record.Title,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
	app.agentUI.conversationHistory = []agent.ConversationSummary{summary}
	app.aiHistoryDrawerOpen = true

	// Click the conversation item in drawer.
	layoutTestFrame(app, size)
	app.agentHistoryItemButton(summary.ID).Click()
	layoutTestFrame(app, size)

	// Drawer should close upon selection.
	if app.aiHistoryDrawerOpen {
		t.Fatal("drawer should close when a conversation is clicked")
	}
}

func TestAIAssistantModelSelectButtonNavigatesToSettings(t *testing.T) {
	app := newTestAIPageApp(t)
	size := image.Pt(1440, 900)

	app.homeSection = homeSectionAIAssistant
	layoutTestFrame(app, size)

	app.aiModelSelectButton.Click()
	layoutTestFrame(app, size)

	if app.homeSection != homeSectionSettings {
		t.Fatalf("expected section %v, got %v", homeSectionSettings, app.homeSection)
	}
	if app.agentUI.settingsPageTab != "providers" {
		t.Fatalf("expected settings tab 'providers', got %q", app.agentUI.settingsPageTab)
	}
}

func TestAIAssistantNewConversationRetainsDraft(t *testing.T) {
	app := newTestAIPageApp(t)
	size := image.Pt(1440, 900)

	app.homeSection = homeSectionAIAssistant
	app.agentUI.conversationDocumentID = GlobalAIAssistantDocumentID
	app.agentUI.chatLines = []agentChatLine{
		{role: "user", text: "历史提问"},
		{role: "assistant", text: "历史回答"},
	}
	app.agentUI.chatInput.SetText("正在输入的草稿内容")

	layoutTestFrame(app, size)

	// Clicking AI 助手 in sidebar starts new conversation while retaining unsent draft.
	app.homeNavButton("ai-assistant").Click()
	layoutTestFrame(app, size)

	if len(app.agentUI.chatLines) != 0 {
		t.Fatalf("expected chatLines to be cleared for new conversation, got %d", len(app.agentUI.chatLines))
	}
	if draft := app.agentUI.chatInput.Text(); draft != "正在输入的草稿内容" {
		t.Fatalf("expected unsent draft to be retained, got %q", draft)
	}
}

func TestAIAssistantDrawerCentering(t *testing.T) {
	app := newTestAIPageApp(t)

	// Test 1: Search box with placeholder "搜索会话"
	{
		var ops op.Ops
		paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		gtx := layout.Context{
			Ops:         &ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(264, 0), Max: image.Pt(264, 36)},
		}
		app.layoutAIDrawerSearchBox(gtx)
		img := renderPresentationTestOps(t, &ops, image.Pt(264, 36))
		iconTop, iconBot, iconOk := darkInkVerticalBounds(img, image.Rect(8, 0, 30, 36))
		textTop, textBot, textOk := darkInkVerticalBounds(img, image.Rect(30, 0, 150, 36))
		if !iconOk || !textOk {
			t.Fatalf("search icon (ok=%v) or text (ok=%v) missing", iconOk, textOk)
		}
		iconCenter := float64(iconTop+iconBot) / 2.0
		textCenter := float64(textTop+textBot) / 2.0
		if math.Abs(iconCenter-textCenter) > 2.0 {
			t.Errorf("search placeholder: icon center %.1f != text center %.1f (diff %.1f > 2.0)", iconCenter, textCenter, math.Abs(iconCenter-textCenter))
		}
		if math.Abs(textCenter-18.0) > 2.5 {
			t.Errorf("search placeholder: text center %.1f not vertically centered in 36px box", textCenter)
		}
	}

	// Test 2: Search box with typed query "核心目标"
	{
		app.agentUI.historySearch.SetText("核心目标")
		var ops op.Ops
		paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		gtx := layout.Context{
			Ops:         &ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(264, 0), Max: image.Pt(264, 36)},
		}
		app.layoutAIDrawerSearchBox(gtx)
		img := renderPresentationTestOps(t, &ops, image.Pt(264, 36))
		iconTop, iconBot, iconOk := darkInkVerticalBounds(img, image.Rect(8, 0, 30, 36))
		textTop, textBot, textOk := darkInkVerticalBounds(img, image.Rect(30, 0, 150, 36))
		if !iconOk || !textOk {
			t.Fatalf("search icon (ok=%v) or typed text (ok=%v) missing", iconOk, textOk)
		}
		iconCenter := float64(iconTop+iconBot) / 2.0
		textCenter := float64(textTop+textBot) / 2.0
		if math.Abs(iconCenter-textCenter) > 2.0 {
			t.Errorf("search typed text: icon center %.1f != text center %.1f (diff %.1f > 2.0)", iconCenter, textCenter, math.Abs(iconCenter-textCenter))
		}
		if math.Abs(textCenter-18.0) > 2.5 {
			t.Errorf("search typed text: text center %.1f not vertically centered in 36px box", textCenter)
		}
	}

	// Test 3: History list item title and time vertically centered
	{
		app.agentUI.conversationID = agent.ConversationID("conv-1")
		app.agentUI.conversationHistory = []agent.ConversationSummary{
			{
				ID:        agent.ConversationID("conv-1"),
				Title:     "新会话",
				UpdatedAt: time.Date(2026, 10, 13, 12, 28, 0, 0, time.Local),
			},
		}
		var ops op.Ops
		paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		gtx := layout.Context{
			Ops:         &ops,
			Now:         time.Date(2026, 10, 13, 12, 30, 0, 0, time.Local),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Min: image.Pt(264, 0), Max: image.Pt(264, 300)},
		}
		app.layoutAIDrawerList(gtx)
		itemImg := renderPresentationTestOps(t, &ops, image.Pt(264, 300))
		btnTop, btnBot := -1, -1
		for y := 0; y < 200; y++ {
			r, g, _, _ := itemImg.At(100, y).RGBA()
			if r < 245*257 && g < 245*257 {
				if btnTop == -1 {
					btnTop = y
				}
				btnBot = y
			}
		}
		if btnTop == -1 || btnBot <= btnTop {
			t.Fatalf("could not locate history item button background (top=%d, bot=%d)", btnTop, btnBot)
		}
		buttonCenter := float64(btnTop+btnBot) / 2.0
		allTop, allBot, allOk := darkInkVerticalBounds(itemImg, image.Rect(16, btnTop, 200, btnBot))
		if !allOk {
			t.Fatalf("history item text ink not found within button [%d..%d]", btnTop, btnBot)
		}
		textCenter := float64(allTop+allBot) / 2.0
		if math.Abs(textCenter-buttonCenter) > 2.0 {
			t.Errorf("history item text center %.1f not vertically centered in button [%d..%d] (center=%.1f, diff=%.1f)",
				textCenter, btnTop, btnBot, buttonCenter, math.Abs(textCenter-buttonCenter))
		}
	}
}

func TestAIAssistantModelDisplayReflectsConfiguredSettings(t *testing.T) {
	app := newTestAIPageApp(t)

	// 1. Initial/default state with no custom config: defaults to OpenAI gpt-4.1
	if label := app.agentModelDisplayLabel(); label != "OpenAI · gpt-4.1  ⌄" {
		t.Fatalf("default model label=%q, want %q", label, "OpenAI · gpt-4.1  ⌄")
	}

	// 2. User configured DeepSeek
	rt := app.ensureDesktopRuntime()
	err := rt.saveAgentSettings(context.Background(), agentsettings.ModelSettings{
		ProviderID: "deepseek",
		Model:      "deepseek-coder",
	}, "test-key")
	if err != nil {
		t.Fatalf("save deepseek settings: %v", err)
	}
	app.agentUI.settings = agentsettings.ModelSettings{} // reset cached UI settings to verify loading
	if label := app.agentModelDisplayLabel(); label != "DeepSeek · deepseek-coder  ⌄" {
		t.Fatalf("deepseek model label=%q, want %q", label, "DeepSeek · deepseek-coder  ⌄")
	}

	// 3. User configured MiniMax
	err = rt.saveAgentSettings(context.Background(), agentsettings.ModelSettings{
		ProviderID: "minimax",
		Model:      "abab6.5s-chat",
	}, "test-key-2")
	if err != nil {
		t.Fatalf("save minimax settings: %v", err)
	}
	app.agentUI.settings = agentsettings.ModelSettings{}
	if label := app.agentModelDisplayLabel(); label != "MiniMax · abab6.5s-chat  ⌄" {
		t.Fatalf("minimax model label=%q, want %q", label, "MiniMax · abab6.5s-chat  ⌄")
	}

	// 4. User configured custom openai-compatible endpoint
	err = rt.saveAgentSettings(context.Background(), agentsettings.ModelSettings{
		ProviderID: "openai-compatible",
		BaseURL:    "https://api.mycustomllm.com/v1",
		Model:      "my-custom-model",
	}, "test-key-3")
	if err != nil {
		t.Fatalf("save custom settings: %v", err)
	}
	app.agentUI.settings = agentsettings.ModelSettings{}
	if label := app.agentModelDisplayLabel(); label != "自定义 · my-custom-model  ⌄" {
		t.Fatalf("custom model label=%q, want %q", label, "自定义 · my-custom-model  ⌄")
	}

	// 5. Model name empty in settings: defaults to tab's default model
	app.agentUI.settings = agentsettings.ModelSettings{
		ProviderID: "kimi",
	}
	if label := app.agentModelDisplayLabel(); label != "Kimi · moonshot-v1-8k  ⌄" {
		t.Fatalf("kimi default model inference=%q, want %q", label, "Kimi · moonshot-v1-8k  ⌄")
	}
}

func TestAIAssistantChatListScrollbarFlushToEdge(t *testing.T) {
	app := newTestAIPageApp(t)
	app.homeSection = homeSectionAIAssistant
	app.ensureGlobalAIAssistant()

	// Add chat messages to trigger message list layout
	app.agentUI.chatLines = []agentChatLine{
		{role: "user", text: "你好，请帮我分析这份季度财报"},
		{role: "assistant", text: "没问题！请把文档发给我，我来帮你整理关键指标。"},
	}

	var ops op.Ops
	mainWidth := 1000
	mainHeight := 800
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(mainWidth, mainHeight), Max: image.Pt(mainWidth, mainHeight)},
	}

	// Layout AIAssistantMessages
	dims := app.layoutAIAssistantMessages(gtx)

	// AIAssistantMessages MUST span the full available width so the scrollbar attaches to the right edge.
	if dims.Size.X != mainWidth {
		t.Fatalf("layoutAIAssistantMessages width=%d, want full width %d so scrollbar is flush to edge", dims.Size.X, mainWidth)
	}

	// Also verify layoutAIAssistantMain occupies the full container
	mainDims := app.layoutAIAssistantMain(gtx)
	if mainDims.Size.X != mainWidth || mainDims.Size.Y != mainHeight {
		t.Fatalf("layoutAIAssistantMain dims=%v, want %v", mainDims.Size, image.Pt(mainWidth, mainHeight))
	}
}

func TestAIAssistantMessageAndInputVisualAlignment(t *testing.T) {
	app := newTestAIPageApp(t)
	app.homeSection = homeSectionAIAssistant
	app.ensureGlobalAIAssistant()

	app.agentUI.chatLines = []agentChatLine{
		{role: "user", text: "hi"},
		{role: "assistant", text: "Hello! How can I help you today?"},
	}

	size := image.Pt(1440, 900)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: size, Max: size},
	}
	app.layoutRecent(gtx)
	img := renderPresentationTestOps(t, &ops, size)

	// Expected column bounds:
	// Sidebar is 250px. Main area is 1440 - 250 = 1190px.
	// Centered 780px column:
	// left = 250 + (1190 - 780)/2 = 455px
	// right = 455 + 780 = 1235px.
	expectedColLeft := 455
	expectedColRight := 1235

	// 1. Scan for the user bubble ("hi") which uses aiUserBubbleBg (R: 228, G: 236, B: 247).
	userBubbleRight := -1
	for y := 0; y < 900; y++ {
		for x := 0; x < 1440; x++ {
			c := img.RGBAAt(x, y)
			if c.R >= 220 && c.R <= 224 && c.G >= 228 && c.G <= 232 && c.B >= 237 && c.B <= 241 {
				if x > userBubbleRight {
					userBubbleRight = x
				}
			}
		}
	}
	if userBubbleRight == -1 {
		t.Fatalf("could not locate user bubble in rendered frame")
	}
	if math.Abs(float64(userBubbleRight-expectedColRight)) > 2 {
		t.Errorf("user bubble right edge = %d, want right-aligned to message column at %d (diff = %d)",
			userBubbleRight, expectedColRight, userBubbleRight-expectedColRight)
	}

	// 2. Scan for input card bounds at the bottom (y around 820).
	// Input card is white (255, 255, 255) on homeCanvas (247, 248, 250).
	inputLeft, inputRight := -1, -1
	for x := 250; x < 1440; x++ {
		c := img.RGBAAt(x, 820)
		if c.R == 255 && c.G == 255 && c.B == 255 {
			if inputLeft == -1 {
				inputLeft = x
			}
			inputRight = x
		}
	}
	if inputLeft == -1 || inputRight == -1 {
		t.Fatalf("could not locate input card at y=820")
	}
	if math.Abs(float64(inputLeft-expectedColLeft)) > 2 {
		t.Errorf("input card left edge = %d, want %d", inputLeft, expectedColLeft)
	}
	if math.Abs(float64(inputRight-expectedColRight)) > 2 {
		t.Errorf("input card right edge = %d, want %d", inputRight, expectedColRight)
	}

	// 3. Scan for assistant card bounds (y around 260).
	// Assistant card is white (255, 255, 255) on homeCanvas (247, 248, 250).
	asstLeft, asstRight := -1, -1
	for x := 300; x < 1400; x++ {
		c := img.RGBAAt(x, 260)
		if c.R == 255 && c.G == 255 && c.B == 255 {
			if asstLeft == -1 {
				asstLeft = x
			}
			asstRight = x
		}
	}
	if asstLeft == -1 || asstRight == -1 {
		t.Fatalf("could not locate assistant card at y=260")
	}
	if math.Abs(float64(asstLeft-expectedColLeft)) > 2 {
		t.Errorf("assistant card left edge = %d, want %d", asstLeft, expectedColLeft)
	}
	if math.Abs(float64(asstRight-expectedColRight)) > 2 {
		t.Errorf("assistant card right edge = %d, want %d", asstRight, expectedColRight)
	}

	// 4. Assert visual alignment between assistant card and input card
	if math.Abs(float64(asstLeft-inputLeft)) > 2 {
		t.Errorf("left alignment mismatch: assistant left = %d, input left = %d", asstLeft, inputLeft)
	}
	if math.Abs(float64(asstRight-inputRight)) > 2 {
		t.Errorf("right alignment mismatch: assistant right = %d, input right = %d", asstRight, inputRight)
	}
}
