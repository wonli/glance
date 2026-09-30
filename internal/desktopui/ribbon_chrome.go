package desktopui

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

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

//go:embed assets/toolbar-icons.png
var toolbarIconSpritePNG []byte

var (
	toolbarIconSpriteOnce sync.Once
	toolbarIconSprite     *image.RGBA
	toolbarIconSpriteErr  error
	toolbarIconTintCache  = struct {
		sync.RWMutex
		icons map[toolbarIconTintKey]*image.NRGBA
	}{icons: make(map[toolbarIconTintKey]*image.NRGBA)}
)

type toolbarIconTintKey struct {
	name  string
	color color.NRGBA
}

func toolbarIconData(name string) (image.Image, bool) {
	cell, ok := toolbarIconCells[name]
	if !ok {
		return nil, false
	}
	toolbarIconSpriteOnce.Do(func() {
		decoded, err := png.Decode(bytes.NewReader(toolbarIconSpritePNG))
		if err != nil {
			toolbarIconSpriteErr = err
			return
		}
		toolbarIconSprite = image.NewRGBA(decoded.Bounds())
		draw.Draw(toolbarIconSprite, toolbarIconSprite.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	})
	if toolbarIconSpriteErr != nil || toolbarIconSprite == nil {
		return nil, false
	}
	const cellSize = 96
	return toolbarIconSprite.SubImage(image.Rect(cell.X*cellSize, cell.Y*cellSize, (cell.X+1)*cellSize, (cell.Y+1)*cellSize)), true
}

func tintedToolbarIcon(name string, tint color.NRGBA) (image.Image, bool) {
	key := toolbarIconTintKey{name: name, color: tint}
	toolbarIconTintCache.RLock()
	icon, ok := toolbarIconTintCache.icons[key]
	toolbarIconTintCache.RUnlock()
	if ok {
		return icon, true
	}
	mask, ok := toolbarIconData(name)
	if !ok {
		return nil, false
	}
	icon = tintToolbarIcon(mask, tint)
	toolbarIconTintCache.Lock()
	if cached, exists := toolbarIconTintCache.icons[key]; exists {
		icon = cached
	} else {
		toolbarIconTintCache.icons[key] = icon
	}
	toolbarIconTintCache.Unlock()
	return icon, true
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

var toolbarIconCells = map[string]image.Point{
	"cut": {X: 0, Y: 0}, "copy": {X: 1, Y: 0}, "paste": {X: 2, Y: 0}, "type": {X: 3, Y: 0},
	"minus": {X: 4, Y: 0}, "plus": {X: 5, Y: 0}, "bold": {X: 6, Y: 0}, "italic": {X: 7, Y: 0},
	"underline": {X: 0, Y: 1}, "align-left": {X: 1, Y: 1}, "align-center": {X: 2, Y: 1}, "align-right": {X: 3, Y: 1},
	"align-distributed": {X: 4, Y: 1}, "text-wrap": {X: 5, Y: 1}, "paintbrush": {X: 6, Y: 1}, "percent": {X: 7, Y: 1},
	"hash": {X: 0, Y: 2}, "square-plus": {X: 1, Y: 2}, "trash-2": {X: 2, Y: 2}, "settings-2": {X: 3, Y: 2},
	"undo": {X: 4, Y: 2}, "redo": {X: 5, Y: 2}, "save": {X: 6, Y: 2}, "clear": {X: 7, Y: 2},
	"row-insert": {X: 0, Y: 3}, "row-delete": {X: 1, Y: 3}, "column-insert": {X: 2, Y: 3}, "column-delete": {X: 3, Y: 3},
	"sheet-plus": {X: 4, Y: 3}, "sheet-edit": {X: 5, Y: 3}, "sheet-trash": {X: 6, Y: 3}, "sort-asc": {X: 7, Y: 3},
	"sort-desc": {X: 0, Y: 4}, "filter": {X: 1, Y: 4}, "house": {X: 2, Y: 4},
	"menu": {X: 0, Y: 5}, "align-center-vertical": {X: 1, Y: 5}, "list": {X: 2, Y: 5}, "grip-horizontal": {X: 3, Y: 5}, "sigma": {X: 4, Y: 5},
	"clock-3": {X: 0, Y: 6}, "star": {X: 1, Y: 6}, "folder": {X: 2, Y: 6}, "folder-open": {X: 3, Y: 6},
	"folder-plus": {X: 4, Y: 6}, "file-text": {X: 5, Y: 6}, "file-spreadsheet": {X: 6, Y: 6}, "presentation": {X: 7, Y: 6},
	"file-code": {X: 0, Y: 7}, "code": {X: 1, Y: 7}, "file": {X: 2, Y: 7}, "search": {X: 3, Y: 7},
	"list-filter": {X: 4, Y: 7}, "ellipsis": {X: 5, Y: 7}, "chevron-down": {X: 6, Y: 7}, "star-filled": {X: 7, Y: 7},
	"paint-bucket": {X: 6, Y: 1}, "grid": {X: 3, Y: 2},
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

func toolbarIconButton(gtx layout.Context, button *widget.Clickable, icon image.Image, label string, enabled, mixed bool, background color.NRGBA) layout.Dimensions {
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
				iconSize := gtx.Dp(unit.Dp(20))
				gtx.Constraints.Min = image.Pt(iconSize, iconSize)
				gtx.Constraints.Max = gtx.Constraints.Min
				return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 20.0 / 96.0}.Layout(gtx)
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
			return layout.Center.Layout(gtx, style.Layout)
		})
		if selected {
			lineY := height - gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, style.Color, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(dims.Size.X, height)}.Op())
		}
		dims.Size.Y = height
		return dims
	})
}
