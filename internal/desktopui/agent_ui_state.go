package desktopui

import (
	"context"

	"gioui.org/layout"
	"gioui.org/widget"

	"glance/internal/agent"
	"glance/internal/agentsettings"
	"glance/internal/application"
)

type agentChatLine struct {
	role           string
	text           string
	confirmationID string
}

type agentSettingsResult struct {
	operation string
	settings  agentsettings.ModelSettings
	err       error
}

type agentConfirmationCard struct {
	info    application.ConfirmationInfo
	approve widget.Clickable
	reject  widget.Clickable
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
	applicationEvents  chan application.Event
	confirmationList   widget.List
	confirmations      map[string]*agentConfirmationCard
	confirmationOrder  []string

	chatInput        widget.Editor
	chatList         widget.List
	chatLines        []agentChatLine
	chatEvents       chan agent.AgentEvent
	chatStatus       string
	conversationID   agent.ConversationID
	conversationBusy bool
	sendButton       widget.Clickable
	stopButton       widget.Clickable
	settingsButton   widget.Clickable
	unsubscribe      func()
	subscriptionStop context.CancelFunc
	subscriptionDone chan struct{}
	subscribedHost   *agent.AgentHost
}

func newAgentUIState() agentUIState {
	state := agentUIState{
		settingsDone:       make(chan agentSettingsResult, 1),
		settingsPageTabs:   make(map[string]*widget.Clickable),
		settingsTabButtons: make(map[string]*widget.Clickable),
		applicationEvents:  make(chan application.Event, 128),
		confirmationList:   widget.List{List: layout.List{Axis: layout.Vertical}},
		confirmations:      make(map[string]*agentConfirmationCard),
		chatEvents:         make(chan agent.AgentEvent, 512),
		chatList:           widget.List{List: layout.List{Axis: layout.Vertical, ScrollToEnd: true}},
	}
	state.apiKeyInput.Mask = '•'
	state.apiKeyInput.SingleLine = true
	state.providerInput.SingleLine = true
	state.baseURLInput.SingleLine = true
	state.modelInput.SingleLine = true
	state.chatInput.Submit = true
	state.chatInput.SingleLine = true
	return state
}
