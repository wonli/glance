package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type ribbonAction struct {
	slot         int
	id           string
	label        string
	displayLabel string
	group        string
	icon         string
	symbol       string
}

type spreadsheetToolbarAction = ribbonAction

func toolbarIconData(name string) (image.Image, bool) {
	assetName, ok := excelIconAssetName(name)
	if !ok {
		return nil, false
	}
	return svgIconOriginal("excel", assetName)
}

func tintedToolbarIcon(name string, tint color.NRGBA) (image.Image, bool) {
	return tintedToolbarIconAtSize(name, tint, 96)
}

func tintedToolbarIconAtSize(name string, tint color.NRGBA, size int) (image.Image, bool) {
	assetName, ok := excelIconAssetName(name)
	if !ok {
		return nil, false
	}
	return svgIconTintAtSize("excel", assetName, size, tint)
}

func excelIconAssetName(name string) (string, bool) {
	if assetName, ok := excelIconAssetNames[name]; ok {
		return assetName, true
	}
	if !svgIconNameToken.MatchString(name) {
		return "", false
	}
	if _, err := svgIconAssets.ReadFile("assets/icons/excel/" + name + ".svg"); err != nil {
		return "", false
	}
	return name, true
}

func tintToolbarIcon(mask image.Image, tint color.NRGBA) *image.NRGBA {
	bounds := mask.Bounds()
	tinted := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			pixel := color.NRGBAModel.Convert(mask.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			tinted.SetNRGBA(x, y, color.NRGBA{R: tint.R, G: tint.G, B: tint.B, A: pixel.A})
		}
	}
	return tinted
}

func toolbarIconColor(enabled bool) color.NRGBA {
	colors := defaultShellTokens.Colors.Toolbar
	if !enabled {
		return colors.DisabledText
	}
	return colors.Glyph
}

func toolbarActionIconColor(actionID string, enabled bool, colors ToolbarColors) color.NRGBA {
	if !enabled {
		return colors.DisabledText
	}
	switch actionID {
	case "insert-wordart":
		return colors.AccentWordArt
	case "paste", "clipboard-paste", "format-painter", "paintbrush":
		return colors.AccentClipboard
	case "text-highlight", "highlighter", "paint-bucket":
		return colors.AccentHighlight
	case "font-color", "baseline":
		return colors.AccentFontColor
	default:
		return colors.Glyph
	}
}

func (a *App) toolbarActionIconColor(actionID string, enabled bool) color.NRGBA {
	return toolbarActionIconColor(actionID, enabled, a.tokens().Colors.Toolbar)
}

func (a *App) toolbarIconColor(enabled bool) color.NRGBA {
	colors := a.tokens().Colors.Toolbar
	if !enabled {
		return colors.DisabledText
	}
	return colors.Glyph
}

func toolbarPickerPanelX(slot int, actions []spreadsheetToolbarAction, position layout.Position, normalWidth, groupWidth, viewportWidth, panelWidth int) int {
	if viewportWidth <= 0 {
		return 0
	}
	maxX := viewportWidth - panelWidth
	if maxX < 0 {
		maxX = 0
	}
	target := -1
	for index, action := range actions {
		if action.slot == slot {
			target = index
			break
		}
	}
	if target < 0 || target < position.First {
		return 0
	}
	x := -position.Offset
	for index := position.First; index < target; index++ {
		width := normalWidth
		if index > 0 && actions[index-1].group != actions[index].group {
			width = groupWidth
		}
		x += width
	}
	if x < 0 {
		return 0
	}
	if x > maxX {
		return maxX
	}
	return x
}

var toolbarIconNames = map[string]struct{}{
	"cut":                   {},
	"copy":                  {},
	"paste":                 {},
	"type":                  {},
	"minus":                 {},
	"plus":                  {},
	"bold":                  {},
	"italic":                {},
	"underline":             {},
	"align-left":            {},
	"align-center":          {},
	"align-right":           {},
	"align-distributed":     {},
	"text-wrap":             {},
	"paintbrush":            {},
	"percent":               {},
	"hash":                  {},
	"square-plus":           {},
	"trash-2":               {},
	"settings-2":            {},
	"undo":                  {},
	"redo":                  {},
	"save":                  {},
	"clear":                 {},
	"row-insert":            {},
	"row-delete":            {},
	"column-insert":         {},
	"column-delete":         {},
	"sheet-plus":            {},
	"sheet-edit":            {},
	"sheet-trash":           {},
	"sort-asc":              {},
	"sort-desc":             {},
	"filter":                {},
	"house":                 {},
	"menu":                  {},
	"align-center-vertical": {},
	"list":                  {},
	"grip-horizontal":       {},
	"sigma":                 {},
	"clock-3":               {},
	"star":                  {},
	"folder":                {},
	"folder-open":           {},
	"folder-plus":           {},
	"file-text":             {},
	"file-spreadsheet":      {},
	"presentation":          {},
	"file-code":             {},
	"code":                  {},
	"file":                  {},
	"search":                {},
	"list-filter":           {},
	"ellipsis":              {},
	"chevron-down":          {},
	"star-filled":           {},
	"paint-bucket":          {},
	"grid":                  {},
}

var excelIconAssetNames = map[string]string{
	"cut": "scissors", "copy": "copy", "paste": "clipboard-paste", "type": "type",
	"minus": "minus", "plus": "plus", "bold": "bold", "italic": "italic",
	"underline": "underline", "align-left": "format_align_left", "align-center": "align-center-vertical", "align-right": "format_align_right",
	"align-distributed": "git-compare-arrows", "text-wrap": "text-wrap", "paintbrush": "paintbrush", "percent": "percent",
	"hash": "hash", "square-plus": "square-plus", "trash-2": "trash-2", "settings-2": "settings-2",
	"undo": "undo-2", "redo": "redo-2", "save": "save", "clear": "eraser",
	"row-insert": "table", "row-delete": "trash-2", "column-insert": "table-2", "column-delete": "table-2",
	"sheet-plus": "square-plus", "sheet-edit": "table", "sheet-trash": "trash-2", "sort-asc": "arrow-up-down",
	"sort-desc": "arrow-up-down-2", "filter": "list-filter", "house": "house",
	"menu": "menu", "align-center-vertical": "align-center-vertical", "list": "list", "grip-horizontal": "grip-horizontal", "sigma": "sigma",
	"clock-3": "clock-3", "star": "sparkles", "folder": "folder-open", "folder-open": "folder-open",
	"folder-plus": "square-plus", "file-text": "file-down", "file-spreadsheet": "table-2", "presentation": "chart-column",
	"file-code": "scan", "code": "scan-2", "file": "file-output", "search": "search",
	"list-filter": "list-filter", "ellipsis": "message-square-plus", "chevron-down": "arrow-up-down", "star-filled": "star-filled",
	"paint-bucket": "paintbrush", "grid": "grid-3x3", "pencil": "pencil",
}

var (
	disabledToolbarBackground = defaultShellTokens.Colors.Toolbar.DisabledBackground
	disabledToolbarText       = defaultShellTokens.Colors.Toolbar.DisabledText
	mixedToolbarBackground    = defaultShellTokens.Colors.Toolbar.MixedBackground
	toolbarActiveBackground   = defaultShellTokens.Colors.Toolbar.ActiveBackground
	toolbarHoverBackground    = defaultShellTokens.Colors.Toolbar.HoverBackground
	toolbarGlyphText          = defaultShellTokens.Colors.Toolbar.Glyph
	ribbonDividerColor        = defaultShellTokens.Colors.Toolbar.Divider
)

func layoutRibbonCategoryTabs(gtx layout.Context, theme *material.Theme, labels []string, buttons []widget.Clickable, selected int, colors ColorTokens) layout.Dimensions {
	buttonPointers := make([]*widget.Clickable, len(buttons))
	for index := range buttons {
		buttonPointers[index] = &buttons[index]
	}
	return layoutRibbonCategoryTabsWithButtons(gtx, theme, labels, buttonPointers, selected, colors)
}

func layoutRibbonCategoryTabsWithButtons(gtx layout.Context, theme *material.Theme, labels []string, buttons []*widget.Clickable, selected int, colors ColorTokens) layout.Dimensions {
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	children := make([]layout.FlexChild, 0, len(labels))
	for index, label := range labels {
		index, label := index, label
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if index >= len(buttons) || buttons[index] == nil {
				return layout.Dimensions{}
			}
			return layout.Inset{Right: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return toolbarTabButtonWithColors(gtx, theme, buttons[index], label, index == selected, colors)
			})
		}))
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, colors.Surface.Surface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				gtx.Constraints.Min.Y = 0
				return layout.Inset{Left: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
			})
		},
	)
}

const (
	documentRibbonHeightDP          = 68
	ribbonCategoryUnderlineHeightDP = 2
	toolbarButtonWidth              = unit.Dp(44)
	toolbarButtonHeight             = unit.Dp(48)
	toolbarButtonCornerRadius       = unit.Dp(5)
	toolbarIconSize                 = unit.Dp(18)
	toolbarItemGap                  = unit.Dp(6)

	excelToolbarButtonWidth   = unit.Dp(48)
	excelToolbarButtonHeight  = unit.Dp(48)
	excelToolbarItemGap       = unit.Dp(3)
	excelToolbarDividerHeight = unit.Dp(24)

	wordRibbonButtonWidth   = unit.Dp(44)
	wordRibbonButtonHeight  = unit.Dp(48)
	wordRibbonItemGap       = unit.Dp(6)
	wordRibbonDividerHeight = unit.Dp(32)
	wordRibbonDividerMargin = 12
)

var (
	toolbarLabelText = defaultShellTokens.Colors.Text.PureBlack
)

func ribbonLabelTextSize(label string) unit.Sp {
	return ribbonLabelTextSizeWithTypography(defaultShellTokens.Typography, label)
}

func layoutToolbarActionItem(gtx layout.Context, action ribbonAction, index int, actions []ribbonAction, child layout.Widget) layout.Dimensions {
	return layoutToolbarActionItemWithDivider(gtx, action, index, actions, defaultShellTokens.Colors.Toolbar.Divider, child)
}

func layoutToolbarActionItemWithDivider(gtx layout.Context, action ribbonAction, index int, actions []ribbonAction, dividerColor color.NRGBA, child layout.Widget) layout.Dimensions {
	groupStart := index == 0 || actions[index-1].group != action.group
	groupEnd := index == len(actions)-1 || actions[index+1].group != action.group
	children := make([]layout.FlexChild, 0, 5)
	if groupStart && index > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(excelToolbarItemGap), 0)}
		}))
	}
	children = append(children, layout.Rigid(child))
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Dp(excelToolbarItemGap), 0)}
	}))
	if groupEnd && index < len(actions)-1 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(excelToolbarDividerHeight))
			gtx.Constraints.Max = gtx.Constraints.Min
			paint.FillShape(gtx.Ops, dividerColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func toolbarIconButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, icon image.Image, iconSize unit.Dp, label, description string, enabled, mixed bool, background color.NRGBA) layout.Dimensions {
	return toolbarIconButtonWithColors(gtx, theme, button, icon, iconSize, label, description, enabled, mixed, background, defaultShellTokens.Colors)
}

func toolbarIconButtonWithColors(gtx layout.Context, theme *material.Theme, button *widget.Clickable, icon image.Image, iconSize unit.Dp, label, description string, enabled, mixed bool, background color.NRGBA, colors ColorTokens) layout.Dimensions {
	if theme == nil {
		theme = material.NewTheme()
	}
	if description == "" {
		description = label
	}
	widthDP := toolbarButtonWidth
	if gtx.Constraints.Min.X > 0 && gtx.Metric.PxPerDp > 0 {
		widthDP = unit.Dp(float32(gtx.Constraints.Min.X) / gtx.Metric.PxPerDp)
	} else if runes := len([]rune(label)); runes >= 4 && (gtx.Constraints.Max.X == 0 || gtx.Constraints.Max.X > gtx.Dp(toolbarButtonWidth)) {
		widthDP = unit.Dp(48)
	}
	width := gtx.Dp(widthDP)
	height := gtx.Dp(toolbarButtonHeight)
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = image.Pt(width, height)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(description).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()

		buttonBackground := background
		active := background == colors.Toolbar.ActiveBackground
		if button != nil && enabled && (button.Hovered() || gtx.Focused(button)) && !mixed && !active {
			buttonBackground = colors.Toolbar.HoverBackground
		}
		activeBounds := image.Rectangle{Max: image.Pt(width, height)}
		radius := gtx.Dp(toolbarButtonCornerRadius)
		if active {
			paint.FillShape(gtx.Ops, colors.Toolbar.ActiveBorder, clip.UniformRRect(activeBounds, radius).Op(gtx.Ops))
			activeBounds = activeBounds.Inset(gtx.Dp(unit.Dp(1)))
			radius = gtx.Dp(unit.Dp(4))
		}
		paint.FillShape(gtx.Ops, buttonBackground, clip.UniformRRect(activeBounds, radius).Op(gtx.Ops))
		layout.Flex{
			Axis:      layout.Vertical,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if icon == nil {
						return layout.Dimensions{Size: image.Pt(width, gtx.Dp(iconSize))}
					}
					iconSizePx := gtx.Dp(iconSize)
					iconGtx := gtx
					iconGtx.Constraints.Min = image.Pt(iconSizePx, iconSizePx)
					iconGtx.Constraints.Max = iconGtx.Constraints.Min
					scale := float32(iconSize) / 96.0
					if b := icon.Bounds(); b.Dx() > 0 {
						scale = float32(iconSize) / float32(b.Dx())
					}
					return layout.Center.Layout(iconGtx, func(gtx layout.Context) layout.Dimensions {
						return widget.Image{
							Src:      iconImageOp(icon),
							Fit:      widget.Contain,
							Position: layout.Center,
							Scale:    scale,
						}.Layout(gtx)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if label == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Bottom: unit.Dp(4), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Point{}
					caption := material.Caption(theme, label)
					caption.Alignment = text.Middle
					caption.TextSize = ribbonLabelTextSize(label)
					caption.Color = colors.Text.PureBlack
					caption.MaxLines = 1
					return layout.Center.Layout(gtx, caption.Layout)
				})
			}),
		)
		if mixed {
			dotSize, inset := gtx.Dp(unit.Dp(5)), gtx.Dp(unit.Dp(5))
			paint.FillShape(gtx.Ops, colors.Toolbar.Glyph, clip.Ellipse(image.Rect(width-dotSize-inset, inset, width-inset, dotSize+inset)).Op(gtx.Ops))
		}
		return layout.Dimensions{Size: image.Pt(width, height)}
	}
	if button != nil && enabled {
		return button.Layout(gtx, draw)
	}
	return draw(gtx)
}

func toolbarTabButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, selected bool) layout.Dimensions {
	return toolbarTabButtonWithColors(gtx, theme, button, label, selected, defaultShellTokens.Colors)
}

func toolbarTabButtonWithColors(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, selected bool, colors ColorTokens) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(30))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		style := material.Body2(theme, label)
		style.TextSize = unit.Sp(13)
		style.Color = colors.Text.PureBlack
		if selected {
			style.Color = colors.Text.Brand
		}
		dims := layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, style.Layout)
		})
		if selected {
			lineHeight := gtx.Dp(unit.Dp(ribbonCategoryUnderlineHeightDP))
			lineY := height - lineHeight
			paint.FillShape(gtx.Ops, colors.Text.Brand, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(dims.Size.X, height)}.Op())
		}
		dims.Size.Y = height
		return dims
	})
}
