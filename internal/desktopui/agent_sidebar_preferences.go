package desktopui

import (
	"gioui.org/unit"
	"math"
	"strconv"
)

const agentSidebarWidthPreference = "ai.sidebar.width_dp"

func (a *App) loadAgentSidebarPreference() {
	if a.workspaceStore == nil {
		return
	}
	value, found, err := a.workspaceStore.Preference(agentSidebarWidthPreference)
	if err != nil {
		a.setStatus("读取 AI 面板宽度失败：" + err.Error())
		return
	}
	if !found {
		return
	}
	width, err := strconv.ParseFloat(value, 32)
	if err == nil && !math.IsNaN(width) && !math.IsInf(width, 0) && width >= 180 && width <= 600 {
		a.agentUI.sidebarWidthDP = unit.Dp(width)
	}
}

func (a *App) saveAgentSidebarPreference() {
	a.ensureDesktopRuntime().queueWorkspacePreference(agentSidebarWidthPreference, strconv.FormatFloat(float64(max(unit.Dp(180), min(unit.Dp(600), a.agentUI.sidebarWidthDP))), 'f', -1, 32))
}
