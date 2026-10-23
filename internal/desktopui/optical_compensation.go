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
// resolved offsets. Every optical correction is zero for the current baseline
// comparison; layout spacing remains owned by the individual controls.
var shellOpticalProfiles = map[string]shellOpticalProfile{
	"darwin":  {},
	"windows": {},
	"linux":   {},
	"default": {},
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
