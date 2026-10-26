package desktopui

import "gioui.org/layout"

func (a *App) openAgentSettingsPage() {
	if a.homeSection != homeSectionSettings {
		a.settingsReturnSection = a.homeSection
	}
	a.resetHomeContentScroll()
	a.prepareAgentSettingsPage()
	a.agentUI.settingsPageTab = "basic"
	a.agentUI.languageMenuOpen = false
	a.homeSection = homeSectionSettings
}

// Resolve navigation before choosing the page layout, so a back click cannot
// leave the settings page drawn with the application already on the home page.
func (a *App) processSettingsNavClicks(gtx layout.Context) {
	for _, id := range []string{"basic", "providers", "mcp"} {
		if a.settingsPageTabButton(id).Clicked(gtx) {
			a.selectSettingsPageTab(id)
		}
	}
	if a.homeNavButton("settings-back").Clicked(gtx) {
		a.cancelAgentModelLookup(true)
		a.captureEditedAgentSettingsKeyPreview()
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
		a.resetHomeContentScroll()
		switch a.settingsReturnSection {
		case homeSectionRecent, homeSectionFavorites, homeSectionAIAssistant, homeSectionTools:
			a.homeSection = a.settingsReturnSection
		default:
			a.homeSection = homeSectionRecent
		}
	}
}
