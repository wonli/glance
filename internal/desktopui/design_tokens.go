package desktopui

import (
	"image/color"

	"gioui.org/unit"
)

// Logo blue sampled from Pen node TRupw (叠页 / 蓝色前页).
var logoBlue = color.NRGBA{R: 0x24, G: 0x63, B: 0xEB, A: 255}

// ThemeMode specifies the theme appearance preference for the desktop shell.
type ThemeMode string

const (
	ThemeModeSystem ThemeMode = "system" // Follow operating system light/dark mode
	ThemeModeLight  ThemeMode = "light"  // Force light appearance
	ThemeModeDark   ThemeMode = "dark"   // Force dark appearance
)

// Typography defines semantic font-size tokens for the desktop shell UI.
// This decouples UI widgets from hardcoded font dimensions and establishes
// a centralized design system scale.
type Typography struct {
	// Display: 34sp - Top-level hero/home brand headings ("glance")
	Display unit.Sp

	// Heading: 17sp - Dialog titles, modal headings, settings group titles
	Heading unit.Sp

	// Nav: 15sp - Primary sidebar and settings navigation items
	Nav unit.Sp

	// Title: 14sp - Prominent badges, counts, preview titles
	Title unit.Sp

	// Body: 14sp - Default body text, form field labels, editors, tab labels, tree nodes
	Body unit.Sp

	// SettingsBody: 16sp - Primary text and fields on settings pages
	SettingsBody unit.Sp

	// SettingsSecondary: 14sp - Explanatory and supporting text on settings pages
	SettingsSecondary unit.Sp

	// Secondary: 12sp - Secondary body, agent chat messages, sheet tab labels, dialog buttons
	Secondary unit.Sp

	// Caption: 11sp - Captions, metadata, file timestamps, file paths, sub-labels
	Caption unit.Sp

	// Micro10: 10sp - Compact status bar, small timestamps, ribbon dropdowns
	Micro10 unit.Sp

	// Micro: 9sp - Extremely compact controls, ruler numbers, ribbon icon labels
	Micro unit.Sp
}

// DefaultTypography is the baseline typographic scale matching current production layout.
var DefaultTypography = Typography{
	Display:           unit.Sp(34),
	Heading:           unit.Sp(17),
	Nav:               unit.Sp(15),
	Title:             unit.Sp(14),
	Body:              unit.Sp(14),
	SettingsBody:      unit.Sp(16),
	SettingsSecondary: unit.Sp(14),
	Secondary:         unit.Sp(12),
	Caption:           unit.Sp(11),
	Micro10:           unit.Sp(10),
	Micro:             unit.Sp(9),
}

// TextColors defines semantic font colors for the desktop shell UI.
type TextColors struct {
	// Primary: Main titles, body text, active inputs
	Primary color.NRGBA

	// Secondary: Auxiliary text, status labels, secondary buttons
	Secondary color.NRGBA

	// Muted: Placeholder hints, file metadata, subtle captions
	Muted color.NRGBA

	// Disabled: Inactive/disabled actions, grayed out items
	Disabled color.NRGBA

	// Brand: Accent text, selected tab text, active links
	Brand color.NRGBA

	// Inverse: Text rendered over dark/contrast backgrounds
	Inverse color.NRGBA

	// PureBlack: High-contrast shell labels and icons (white in dark mode)
	PureBlack color.NRGBA
}

// SurfaceColors defines semantic background and structural colors for UI chrome.
type SurfaceColors struct {
	// Canvas: Base window background
	Canvas color.NRGBA

	// Chrome: Top workspace titlebar, ribbon background, tab strip
	Chrome color.NRGBA

	// Surface: Card backgrounds, dialog containers, editor backgrounds
	Surface color.NRGBA

	// Sidebar: Navigation sidebar background
	Sidebar color.NRGBA

	// Selected: Selected item highlight fill
	Selected color.NRGBA

	// Hover: Interactive hover highlight fill
	Hover color.NRGBA

	// Border: Standard boundary divider
	Border color.NRGBA

	// BorderMuted: Subtle internal line dividers
	BorderMuted color.NRGBA
}

// MenuColors defines colors used by dropdown and context menus.
type MenuColors struct {
	Surface      color.NRGBA // Dropdown and context-menu panel fill
	Hover        color.NRGBA // Pointer hover fill for enabled menu items
	Selected     color.NRGBA // Persistent selection fill for checked menu items
	DisabledText color.NRGBA // Labels for unavailable menu items
	Border       color.NRGBA // Menu outline
	Shadow       color.NRGBA // Menu drop shadow, including alpha
}

// ToolbarColors defines colors used by ribbon controls and their states.
type ToolbarColors struct {
	Background         color.NRGBA // Ribbon command-area background
	Glyph              color.NRGBA // Enabled icon and mixed-state indicator
	Label              color.NRGBA // Enabled command labels
	DisabledText       color.NRGBA // Disabled labels and icons
	DisabledBackground color.NRGBA // Disabled control fill where a fill is shown
	MixedBackground    color.NRGBA // Mixed-value control fill
	ActiveBackground   color.NRGBA // Selected or active control fill
	ActiveBorder       color.NRGBA // Selected control outline
	ActiveGlyph        color.NRGBA // Selected control icon
	HoverBackground    color.NRGBA // Pointer hover fill
	Divider            color.NRGBA // Ribbon group separators
	AccentAI           color.NRGBA // AI assistant icon and AI entry mark
	AccentAISecondary  color.NRGBA // Back page of the brand assistant mark
	AccentAIStar       color.NRGBA // Star on the front page
	AccentWordArt      color.NRGBA // WordArt icon, independent of the assistant identity
	AccentClipboard    color.NRGBA // Paste and format-painter icons
	AccentHighlight    color.NRGBA // Text highlighter icon
	AccentFontColor    color.NRGBA // Font-color icon
}

// GridColors controls spreadsheet interaction chrome independently of document styling.
type GridColors struct {
	SelectionBorder color.NRGBA // Active cell and selected range outline
}

// ColorTokens encapsulates text and surface colors for a complete visual theme.
type ColorTokens struct {
	Text    TextColors
	Surface SurfaceColors
	Menu    MenuColors
	Toolbar ToolbarColors
	Grid    GridColors
}

// LightColorTokens provides the baseline light palette matching production UI.
var LightColorTokens = ColorTokens{
	Grid: GridColors{SelectionBorder: logoBlue},
	Text: TextColors{
		Primary:   color.NRGBA{R: 52, G: 59, B: 69, A: 255},    // #343B45
		Secondary: color.NRGBA{R: 89, G: 98, B: 109, A: 255},   // #59626D
		Muted:     color.NRGBA{R: 138, G: 146, B: 157, A: 255}, // #8A929D
		Disabled:  color.NRGBA{R: 165, G: 173, B: 182, A: 255}, // #A5ADB6
		Brand:     color.NRGBA{R: 52, G: 95, B: 150, A: 255},   // #345F96
		Inverse:   color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		PureBlack: color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	},
	Surface: SurfaceColors{
		Canvas:      color.NRGBA{R: 247, G: 248, B: 250, A: 255}, // #F7F8FA
		Chrome:      color.NRGBA{R: 241, G: 243, B: 245, A: 255}, // #F1F3F5
		Surface:     color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // #FFFFFF
		Sidebar:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		Selected:    color.NRGBA{R: 241, G: 242, B: 244, A: 255}, // #F1F2F4
		Hover:       color.NRGBA{R: 247, G: 248, B: 249, A: 255}, // #F7F8F9
		Border:      color.NRGBA{R: 225, G: 229, B: 234, A: 255}, // #E1E5EA
		BorderMuted: color.NRGBA{R: 238, G: 240, B: 242, A: 255}, // #EEF0F2
	},
	Menu: MenuColors{
		Surface:      color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // #FFFFFF
		Hover:        color.NRGBA{R: 234, G: 242, B: 255, A: 255}, // #EAF2FF
		Selected:     color.NRGBA{R: 234, G: 242, B: 255, A: 255}, // #EAF2FF
		DisabledText: color.NRGBA{R: 165, G: 173, B: 182, A: 255}, // #A5ADB6
		Border:       color.NRGBA{R: 225, G: 229, B: 234, A: 255}, // #E1E5EA
		Shadow:       color.NRGBA{R: 28, G: 38, B: 54, A: 28},
	},
	Toolbar: ToolbarColors{
		Background:         color.NRGBA{R: 248, G: 249, B: 251, A: 255}, // #F8F9FB
		Glyph:              color.NRGBA{R: 0, G: 0, B: 0, A: 255},       // #000000
		Label:              color.NRGBA{R: 0, G: 0, B: 0, A: 255},       // #000000
		DisabledText:       color.NRGBA{R: 150, G: 156, B: 165, A: 255},
		DisabledBackground: color.NRGBA{R: 232, G: 235, B: 240, A: 255},
		MixedBackground:    color.NRGBA{R: 239, G: 241, B: 243, A: 255},
		ActiveBackground:   color.NRGBA{R: 0xE0, G: 0xEB, B: 0xFF, A: 255}, // #E0EBFF
		ActiveBorder:       color.NRGBA{R: 0xA8, G: 0xC3, B: 0xFA, A: 255}, // #A8C3FA
		ActiveGlyph:        logoBlue,
		HoverBackground:    color.NRGBA{R: 247, G: 248, B: 250, A: 255},
		Divider:            color.NRGBA{R: 221, G: 225, B: 231, A: 255}, // #DDE1E7
		AccentAI:           logoBlue,
		AccentAISecondary:  color.NRGBA{R: 0xA8, G: 0xC3, B: 0xFA, A: 255},
		AccentAIStar:       color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		AccentWordArt:      color.NRGBA{R: 33, G: 99, B: 65, A: 255},  // #216341
		AccentClipboard:    color.NRGBA{R: 0, G: 0, B: 0, A: 255},     // #000000
		AccentHighlight:    color.NRGBA{R: 177, G: 139, B: 0, A: 255}, // #B18B00
		AccentFontColor:    color.NRGBA{R: 195, G: 67, B: 67, A: 255}, // #C34343
	},
}

// DarkColorTokens provides an accessible dark palette for dark-mode shell chrome.
var DarkColorTokens = ColorTokens{
	Grid: GridColors{SelectionBorder: logoBlue},
	Text: TextColors{
		Primary:   color.NRGBA{R: 235, G: 240, B: 245, A: 255}, // #EBF0F5
		Secondary: color.NRGBA{R: 178, G: 186, B: 196, A: 255}, // #B2BAC4
		Muted:     color.NRGBA{R: 125, G: 133, B: 144, A: 255}, // #7D8590
		Disabled:  color.NRGBA{R: 85, G: 92, B: 100, A: 255},   // #555C64
		Brand:     color.NRGBA{R: 90, G: 155, B: 240, A: 255},  // #5A9BF0
		Inverse:   color.NRGBA{R: 20, G: 24, B: 30, A: 255},
		PureBlack: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // In dark mode, pure contrast icons become white
	},
	Surface: SurfaceColors{
		Canvas:      color.NRGBA{R: 20, G: 22, B: 26, A: 255}, // #14161A
		Chrome:      color.NRGBA{R: 28, G: 31, B: 36, A: 255}, // #1C1F24
		Surface:     color.NRGBA{R: 34, G: 38, B: 44, A: 255}, // #22262C
		Sidebar:     color.NRGBA{R: 25, G: 28, B: 33, A: 255}, // #191C21
		Selected:    color.NRGBA{R: 45, G: 52, B: 62, A: 255}, // #2D343E
		Hover:       color.NRGBA{R: 40, G: 45, B: 53, A: 255}, // #282D35
		Border:      color.NRGBA{R: 52, G: 58, B: 66, A: 255}, // #343A42
		BorderMuted: color.NRGBA{R: 42, G: 47, B: 54, A: 255}, // #2A2F36
	},
	Menu: MenuColors{
		Surface:      color.NRGBA{R: 34, G: 38, B: 44, A: 255},
		Hover:        color.NRGBA{R: 43, G: 55, B: 71, A: 255},
		Selected:     color.NRGBA{R: 49, G: 72, B: 99, A: 255},
		DisabledText: color.NRGBA{R: 112, G: 120, B: 131, A: 255},
		Border:       color.NRGBA{R: 52, G: 58, B: 66, A: 255},
		Shadow:       color.NRGBA{R: 0, G: 0, B: 0, A: 96},
	},
	Toolbar: ToolbarColors{
		Background:         color.NRGBA{R: 28, G: 31, B: 36, A: 255},
		Glyph:              color.NRGBA{R: 235, G: 240, B: 245, A: 255},
		Label:              color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		DisabledText:       color.NRGBA{R: 125, G: 133, B: 144, A: 255},
		DisabledBackground: color.NRGBA{R: 43, G: 47, B: 54, A: 255},
		MixedBackground:    color.NRGBA{R: 54, G: 62, B: 73, A: 255},
		ActiveBackground:   color.NRGBA{R: 45, G: 52, B: 62, A: 255},
		ActiveBorder:       color.NRGBA{R: 83, G: 120, B: 171, A: 255},
		ActiveGlyph:        color.NRGBA{R: 168, G: 195, B: 250, A: 255},
		HoverBackground:    color.NRGBA{R: 40, G: 45, B: 53, A: 255},
		Divider:            color.NRGBA{R: 52, G: 58, B: 66, A: 255},
		AccentAI:           logoBlue,
		AccentAISecondary:  color.NRGBA{R: 0xA8, G: 0xC3, B: 0xFA, A: 255},
		AccentAIStar:       color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		AccentWordArt:      color.NRGBA{R: 100, G: 183, B: 139, A: 255},
		AccentClipboard:    color.NRGBA{R: 235, G: 240, B: 245, A: 255},
		AccentHighlight:    color.NRGBA{R: 218, G: 183, B: 47, A: 255},
		AccentFontColor:    color.NRGBA{R: 230, G: 112, B: 112, A: 255},
	},
}

// DesignTokens holds the unified active design system configuration.
type DesignTokens struct {
	Mode       ThemeMode
	IsDark     bool
	Typography Typography
	Colors     ColorTokens
}

// ResolveDesignTokens resolves the effective design tokens given user preference and typography.
func ResolveDesignTokens(mode ThemeMode, typo Typography) DesignTokens {
	if mode == "" {
		mode = ThemeModeSystem
	}
	isDark := false
	switch mode {
	case ThemeModeDark:
		isDark = true
	case ThemeModeLight:
		isDark = false
	case ThemeModeSystem:
		isDark = detectSystemDarkMode()
	}

	colors := LightColorTokens
	if isDark {
		colors = DarkColorTokens
	}

	if typo.Body == 0 {
		typo = DefaultTypography
	}
	if typo.SettingsBody == 0 {
		typo.SettingsBody = DefaultTypography.SettingsBody
	}
	if typo.SettingsSecondary == 0 {
		typo.SettingsSecondary = DefaultTypography.SettingsSecondary
	}

	return DesignTokens{
		Mode:       mode,
		IsDark:     isDark,
		Typography: typo,
		Colors:     colors,
	}
}

// defaultShellTokens is the default active tokens instance used by standalone UI components.
var defaultShellTokens = ResolveDesignTokens(ThemeModeSystem, DefaultTypography)

// ribbonLabelTextSizeWithTypography calculates Ribbon button label size with character-count compression.
func ribbonLabelTextSizeWithTypography(typo Typography, label string) unit.Sp {
	runes := len([]rune(label))
	switch {
	case runes <= 2:
		return typo.Micro10
	case runes == 3:
		return typo.Micro10 - unit.Sp(0.5)
	case runes == 4:
		return typo.Micro
	default:
		return typo.Micro - unit.Sp(0.5)
	}
}
