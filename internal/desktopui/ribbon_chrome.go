package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type ribbonAction struct {
	slot  int
	id    string
	label string
	group string
	icon  string
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
	if !enabled {
		return disabledToolbarText
	}
	return toolbarGlyphText
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
	"paint-bucket": "paintbrush", "grid": "grid-3x3",
}

var (
	disabledToolbarBackground = color.NRGBA{R: 232, G: 235, B: 240, A: 255}
	disabledToolbarText       = color.NRGBA{R: 150, G: 156, B: 165, A: 255}
	mixedToolbarBackground    = color.NRGBA{R: 239, G: 241, B: 243, A: 255}
	toolbarActiveBackground   = color.NRGBA{R: 241, G: 243, B: 245, A: 255}
	toolbarHoverBackground    = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	toolbarGlyphText          = color.NRGBA{R: 75, G: 83, B: 94, A: 255}
	ribbonDividerColor        = color.NRGBA{R: 225, G: 228, B: 232, A: 255}
)

func layoutRibbonCategoryTabs(gtx layout.Context, theme *material.Theme, labels []string, buttons []widget.Clickable, selected int) layout.Dimensions {
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	children := make([]layout.FlexChild, 0, len(labels))
	for index, label := range labels {
		index, label := index, label
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return toolbarTabButton(gtx, theme, &buttons[index], label, index == selected)
			})
		}))
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
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

func layoutToolbarActionItem(gtx layout.Context, action ribbonAction, index int, actions []ribbonAction, child layout.Widget) layout.Dimensions {
	groupStart := index == 0 || actions[index-1].group != action.group
	groupEnd := index == len(actions)-1 || actions[index+1].group != action.group
	children := make([]layout.FlexChild, 0, 5)
	if groupStart && index > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
		}))
	}
	children = append(children, layout.Rigid(child))
	gap := unit.Dp(6)
	if groupEnd {
		gap = unit.Dp(12)
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Dp(gap), 0)}
	}))
	if groupEnd && index < len(actions)-1 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(32)))
			gtx.Constraints.Max = gtx.Constraints.Min
			shift := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
			paint.FillShape(gtx.Ops, ribbonDividerColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
			shift.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func toolbarIconButton(gtx layout.Context, button *widget.Clickable, icon image.Image, iconSize unit.Dp, label string, enabled, mixed bool, background color.NRGBA) layout.Dimensions {
	const buttonSize = unit.Dp(36)
	size := gtx.Dp(buttonSize)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	draw := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(label).Add(gtx.Ops)
		buttonBackground := background
		if button != nil && enabled && (button.Hovered() || gtx.Focused(button)) && !mixed {
			buttonBackground = toolbarHoverBackground
		}
		paint.FillShape(gtx.Ops, buttonBackground, clip.UniformRRect(image.Rectangle{Max: image.Pt(size, size)}, gtx.Dp(unit.Dp(7))).Op(gtx.Ops))
		if icon != nil {
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				iconSizePx := gtx.Dp(iconSize)
				gtx.Constraints.Min = image.Pt(iconSizePx, iconSizePx)
				gtx.Constraints.Max = gtx.Constraints.Min
				return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(iconSize) / 96.0}.Layout(gtx)
			})
		}
		if mixed {
			dotSize, inset := gtx.Dp(unit.Dp(5)), gtx.Dp(unit.Dp(5))
			paint.FillShape(gtx.Ops, toolbarGlyphText, clip.Ellipse(image.Rect(size-dotSize-inset, inset, size-inset, dotSize+inset)).Op(gtx.Ops))
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	}
	if button == nil {
		return draw(gtx)
	}
	return button.Layout(gtx, draw)
}

func toolbarTabButton(gtx layout.Context, theme *material.Theme, button *widget.Clickable, label string, selected bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(30))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		style := material.Body2(theme, label)
		style.TextSize = unit.Sp(13)
		style.Color = color.NRGBA{R: 89, G: 98, B: 109, A: 255}
		if selected {
			style.Color = color.NRGBA{R: 53, G: 95, B: 150, A: 255}
		}
		dims := layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			shift := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(3)))).Push(gtx.Ops)
			dims := layout.Center.Layout(gtx, style.Layout)
			shift.Pop()
			return dims
		})
		if selected {
			lineY := height - gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, style.Color, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(dims.Size.X, height)}.Op())
		}
		dims.Size.Y = height
		return dims
	})
}
