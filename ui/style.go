// Package ui contains reusable Gio controls with application-supplied styles.
package ui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/unit"
)

// StateColors describes the colors a control can draw for its common states.
// A zero alpha color is transparent, which is useful for idle navigation rows.
type StateColors struct {
	Background         color.NRGBA
	Hover              color.NRGBA
	Selected           color.NRGBA
	DisabledBackground color.NRGBA
	Border             color.NRGBA
	HoverBorder        color.NRGBA
	SelectedBorder     color.NRGBA
	DisabledBorder     color.NRGBA
	Foreground         color.NRGBA
	HoverForeground    color.NRGBA
	SelectedForeground color.NRGBA
	DisabledForeground color.NRGBA
}

// TextStyle contains the typography shared by reusable controls.
type TextStyle struct {
	Size   unit.Sp
	Weight font.Weight
}

// IconTextButtonStyle describes the geometry and appearance of an icon/text button.
type IconTextButtonStyle struct {
	Width, Height unit.Dp
	IconSize      unit.Dp
	Gap           unit.Dp
	Radius        unit.Dp
	Text          TextStyle
	Colors        StateColors
	TextOffsetY   unit.Dp
	IconOffsetY   unit.Dp
}

// NavItemStyle describes a single selectable navigation row.
type NavItemStyle struct {
	Height          unit.Dp
	IconSize        unit.Dp
	LeftInset       unit.Dp
	RightInset      unit.Dp
	IconGap         unit.Dp
	TrailingGap     unit.Dp
	Radius          unit.Dp
	Text            TextStyle
	SelectedText    TextStyle
	TrailingText    TextStyle
	Colors          StateColors
	TrailingColor   color.NRGBA
	TextOffsetY     unit.Dp
	IconOffsetY     unit.Dp
	TrailingOffsetY unit.Dp
}

// TextFieldStyle describes the field frame and text placement.
type TextFieldStyle struct {
	Width, Height      unit.Dp
	LeftInset          unit.Dp
	RightInset         unit.Dp
	SuffixWidth        unit.Dp
	Radius             unit.Dp
	Text               TextStyle
	Fill               color.NRGBA
	Border             color.NRGBA
	Foreground         color.NRGBA
	Hint               color.NRGBA
	HoverBorder        color.NRGBA
	FocusBorder        color.NRGBA
	DisabledFill       color.NRGBA
	DisabledBorder     color.NRGBA
	DisabledForeground color.NRGBA
	DisabledHint       color.NRGBA
	TextOffsetY        unit.Dp
}
