package desktopui

import (
	"runtime"

	"gioui.org/unit"
)

type shellOpticalProfile struct {
	homeNav, settingsNav, settingsField, confirmationButton unit.Dp
	homeFolderTextInset, homeFileListTextInset              unit.Dp
	homeFilterTextInset, homeSearchTextInset                unit.Dp
	homeTabIconOffset                                       unit.Dp
}

type shellOpticalTextOffsetComponent uint8

const (
	opticalHomeNav shellOpticalTextOffsetComponent = iota
	opticalSettingsNav
	opticalSettingsField
	opticalConfirmationButton
)

type shellOpticalTextInsetComponent uint8

const (
	opticalHomeFolderText shellOpticalTextInsetComponent = iota
	opticalHomeFileListText
	opticalHomeFilterText
	opticalHomeSearchText
)

type shellOpticalIconOffsetComponent uint8

const (
	opticalHomeTabIcon shellOpticalIconOffsetComponent = iota
)

// Keep platform selection in the app adapter; reusable ui controls receive
// resolved offsets. Windows uses no optical correction for home navigation,
// folders, file rows, filter chips, or the search field.
var shellOpticalProfiles = map[string]shellOpticalProfile{
	"darwin":  {homeNav: 3, settingsNav: 3, settingsField: 3, confirmationButton: 3, homeFolderTextInset: 6, homeFileListTextInset: 6, homeFilterTextInset: 6, homeSearchTextInset: 6, homeTabIconOffset: -3},
	"windows": {homeNav: 0, settingsNav: 0, settingsField: 3, confirmationButton: 3, homeFolderTextInset: 0, homeFileListTextInset: 0, homeFilterTextInset: 0, homeSearchTextInset: 0, homeTabIconOffset: 0},
	"linux":   {homeNav: 3, settingsNav: 3, settingsField: 3, confirmationButton: 3, homeFolderTextInset: 6, homeFileListTextInset: 6, homeFilterTextInset: 6, homeSearchTextInset: 6, homeTabIconOffset: -3},
	"default": {homeNav: 3, settingsNav: 3, settingsField: 3, confirmationButton: 3, homeFolderTextInset: 6, homeFileListTextInset: 6, homeFilterTextInset: 6, homeSearchTextInset: 6, homeTabIconOffset: -3},
}

func currentShellOpticalProfile() shellOpticalProfile {
	profile, ok := shellOpticalProfiles[runtime.GOOS]
	if !ok {
		profile = shellOpticalProfiles["default"]
	}
	return profile
}

func shellOpticalTextOffset(component shellOpticalTextOffsetComponent) unit.Dp {
	profile := currentShellOpticalProfile()
	switch component {
	case opticalHomeNav:
		return profile.homeNav
	case opticalSettingsNav:
		return profile.settingsNav
	case opticalSettingsField:
		return profile.settingsField
	case opticalConfirmationButton:
		return profile.confirmationButton
	default:
		return 0
	}
}

func shellOpticalTextTopInset(component shellOpticalTextInsetComponent) unit.Dp {
	profile := currentShellOpticalProfile()
	switch component {
	case opticalHomeFolderText:
		return profile.homeFolderTextInset
	case opticalHomeFileListText:
		return profile.homeFileListTextInset
	case opticalHomeFilterText:
		return profile.homeFilterTextInset
	case opticalHomeSearchText:
		return profile.homeSearchTextInset
	default:
		return 0
	}
}

func shellOpticalIconOffset(component shellOpticalIconOffsetComponent) unit.Dp {
	profile := currentShellOpticalProfile()
	switch component {
	case opticalHomeTabIcon:
		return profile.homeTabIconOffset
	default:
		return 0
	}
}
