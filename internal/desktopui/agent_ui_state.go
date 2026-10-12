package desktopui

import (
	"context"
	"time"

	"gioui.org/gesture"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/agentsettings"
	"glance/internal/application"
	"glance/internal/document"
)

type agentChatLine struct {
	role           string
	text           string
	markdownSource string
	markdownBlocks []document.MarkdownBlock
	toolName       string
	toolSummary    string
	confirmationID string
	collapsed      bool
	inProgress     bool
	isError        bool
	needsAuth      bool
	startedAt      time.Time
	duration       time.Duration
	toggle         *widget.Clickable
}

type agentChatHistoryResult struct {
	deletedID    agent.ConversationID
	kind         string
	generation   uint64
	documentID   string
	selectLatest bool
	summaries    []agent.ConversationSummary
	snapshot     agent.ConversationSnapshot
	found        bool
	err          error
}

type agentSettingsResult struct {
	operation string
	settings  agentsettings.ModelSettings
	err       error
}

type agentModelLookupState struct {
	generation    uint64
	fingerprint   string
	loading       bool
	models        []provider.ModelInfo
	modelsByID    map[string]provider.ModelInfo
	err           string
	cancel        context.CancelFunc
	results       chan agentModelLookupResult
	modelIDs      []string
	optionButtons map[string]*widget.Clickable
	optionsList   layout.List
}

type agentConfirmationCard struct {
	info    application.ConfirmationInfo
	approve widget.Clickable
	reject  widget.Clickable
}

type agentChatGroupHeightEntry struct {
	width     int
	pxPerDp   float32
	pxPerSp   float32
	signature uint64
	height    int
}

type agentUIState struct {
	providerInput      widget.Editor
	baseURLInput       widget.Editor
	modelInput         widget.Editor
	apiKeyInput        widget.Editor
	settingsInfo       string
	settingsError      string
	settings           agentsettings.ModelSettings
	settingsPageTab    string
	settingsPageTabs   map[string]*widget.Clickable
	settingsTab        string
	settingsTabButtons map[string]*widget.Clickable
	settingsKeyLink    widget.Clickable
	saving             bool
	returnToChat       bool
	settingsDone       chan agentSettingsResult
	settingsTest       widget.Clickable
	settingsSave       widget.Clickable
	settingsKeyToggle  widget.Clickable
	settingsKeyPreview agentSettingsKeyPreview
	modelLookup        agentModelLookupState
	applicationEvents  chan application.Event
	confirmationList   widget.List
	confirmations      map[string]*agentConfirmationCard
	confirmationOrder  []string

	chatInput                        widget.Editor
	chatList                         widget.List
	chatGroupHeights                 [][2]agentChatGroupHeightEntry
	chatLines                        []agentChatLine
	chatEvents                       chan agent.AgentEvent
	chatStatus                       string
	sidebarVisible                   bool
	sidebarWidthDP                   unit.Dp
	sidebarResizeDrag                gesture.Drag
	sidebarResizing                  bool
	sidebarResizeStartX              float32
	sidebarResizeStartWidthDP        unit.Dp
	sidebarResizePreviewDP           unit.Dp
	ribbonEntryButton                widget.Clickable
	sidebarCloseButton               widget.Clickable
	copyResponseButton               widget.Clickable
	regenerateResponseButton         widget.Clickable
	insertResponseButton             widget.Clickable
	conversationID                   agent.ConversationID
	conversationDocumentID           string
	conversationTitle                string
	conversationStartedAt            time.Time
	conversationHistory              []agent.ConversationSummary
	conversationHistoryButtons       map[agent.ConversationID]*widget.Clickable
	conversationHistoryDeleteButtons map[agent.ConversationID]*widget.Clickable
	conversationDeletedIDs           map[agent.ConversationID]struct{}
	conversationDeletes              map[agent.ConversationID]string
	conversationDeleteResults        chan agentChatHistoryResult
	historySearch                    widget.Editor
	historyBackButton                widget.Clickable
	conversationHistoryList          widget.List
	conversationHistoryVisible       bool
	conversationHistoryLoading       bool
	conversationHistoryLoaded        bool
	conversationHistoryError         string
	conversationHistoryGeneration    uint64
	conversationHistoryCancel        context.CancelFunc
	conversationHistoryResults       chan agentChatHistoryResult
	conversationInterrupted          bool
	newConversationButton            widget.Clickable
	conversationTitleButton          widget.Clickable
	resumeConversationButton         widget.Clickable
	conversationBusy                 bool
	activeReasoningLine              int
	activeAnswerStarted              bool
	sendButton                       widget.Clickable
	stopButton                       widget.Clickable
	settingsButton                   widget.Clickable
	unsubscribe                      func()
	subscriptionStop                 context.CancelFunc
	subscriptionDone                 chan struct{}
	subscribedHost                   *agent.AgentHost
}

func newAgentUIState() agentUIState {
	state := agentUIState{
		settingsDone:                     make(chan agentSettingsResult, 1),
		modelLookup:                      agentModelLookupState{results: make(chan agentModelLookupResult, 1), optionsList: layout.List{Axis: layout.Horizontal}},
		activeReasoningLine:              -1,
		settingsPageTabs:                 make(map[string]*widget.Clickable),
		settingsTabButtons:               make(map[string]*widget.Clickable),
		applicationEvents:                make(chan application.Event, 128),
		confirmationList:                 widget.List{List: layout.List{Axis: layout.Vertical}},
		confirmations:                    make(map[string]*agentConfirmationCard),
		chatEvents:                       make(chan agent.AgentEvent, 512),
		conversationHistoryButtons:       make(map[agent.ConversationID]*widget.Clickable),
		conversationHistoryDeleteButtons: make(map[agent.ConversationID]*widget.Clickable),
		conversationDeletes:              make(map[agent.ConversationID]string),
		conversationDeleteResults:        make(chan agentChatHistoryResult, 16),
		conversationHistoryList:          widget.List{List: layout.List{Axis: layout.Vertical}},
		conversationHistoryResults:       make(chan agentChatHistoryResult, 8),
		chatList:                         widget.List{List: layout.List{Axis: layout.Vertical, ScrollToEnd: true}},
	}
	state.historySearch.SingleLine = true
	state.apiKeyInput.Mask = '•'
	state.apiKeyInput.SingleLine = true
	state.providerInput.SingleLine = true
	state.baseURLInput.SingleLine = true
	state.modelInput.SingleLine = true
	// Handle Enter at the sidebar input boundary; Editor.Submit also treats
	// embedded newlines in pasted text as submission and truncates the draft.
	state.chatInput.Submit = false
	state.chatInput.SingleLine = false
	return state
}
