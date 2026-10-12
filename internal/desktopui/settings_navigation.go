package desktopui

import "gioui.org/layout"

// Resolve navigation before choosing the page layout, so a back click cannot
// leave the settings page drawn with the application already on the home page.
func (a *App) processSettingsNavClicks(gtx layout.Context) {
	for _, id := range []string{"providers", "mcp"} {
		if a.settingsPageTabButton(id).Clicked(gtx) {
			a.selectSettingsPageTab(id)
		}
	}
	if a.homeNavButton("settings-back").Clicked(gtx) {
		a.clearAgentSettingsKeyPreview()
		a.agentUI.apiKeyInput.Mask = '•'
		a.resetHomeContentScroll()
		a.homeSection = homeSectionRecent
	}
}
