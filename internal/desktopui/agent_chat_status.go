package desktopui

import "glance/internal/i18n"

func (a *App) setAgentChatStatus(message i18n.Message) {
	a.agentUI.chatStatusRef = message
	if message.ID == "" {
		a.agentUI.chatStatus = ""
		return
	}
	a.agentUI.chatStatus = a.text(message)
}

func (a *App) setAgentChatStatusText(text string) {
	a.agentUI.chatStatusRef = i18n.Message{}
	a.agentUI.chatStatus = text
}

func (a *App) clearAgentChatStatus() {
	a.setAgentChatStatus(i18n.Message{})
}

func (a *App) agentChatStatusText() string {
	if a.agentUI.chatStatusRef.ID != "" {
		return a.text(a.agentUI.chatStatusRef)
	}
	return a.agentUI.chatStatus
}
