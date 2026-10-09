package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (a *App) layoutCreateMenuOpenActions(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(12))}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuOpenAction(gtx, &a.actionOpenFile, "打开本地文件", "folder-open", "⌘ O", false)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutCreateMenuOpenAction(gtx, &a.actionOpenFolder, "打开文件夹", "folder", "", true)
		}),
	)
}

func (a *App) layoutCreateMenuOpenAction(gtx layout.Context, click *widget.Clickable, label, icon, shortcut string, folder bool) layout.Dimensions {
	if click.Clicked(gtx) {
		a.showNewMenu = false
		if folder {
			a.startFolderPicker()
		} else {
			a.startOpenFilePicker()
		}
	}
	height := gtx.Dp(unit.Dp(42))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				fill := color.NRGBA{R: 242, G: 246, B: 251, A: 255}
				if click.Hovered() {
					fill = color.NRGBA{R: 235, G: 242, B: 250, A: 255}
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(8)))
				paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(unit.Dp(26))
								gtx.Constraints.Min = image.Pt(size, size)
								gtx.Constraints.Max = gtx.Constraints.Min
								return layout.Stack{Alignment: layout.Center}.Layout(gtx,
									layout.Expanded(func(gtx layout.Context) layout.Dimensions {
										shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6)))
										paint.FillShape(gtx.Ops, contentSurface, shape.Op(gtx.Ops))
										return layout.Dimensions{Size: gtx.Constraints.Max}
									}),
									layout.Stacked(func(gtx layout.Context) layout.Dimensions {
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeIcon(gtx, icon, 15, color.NRGBA{R: 52, G: 120, B: 209, A: 255})
										})
									}),
								)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeLabelStyle(a.theme, label, 12, color.NRGBA{R: 49, G: 95, B: 140, A: 255}, font.SemiBold).Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if shortcut == "" {
								return layout.Dimensions{}
							}
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeLabelStyle(a.theme, shortcut, 10, color.NRGBA{R: 122, G: 143, B: 168, A: 255}, font.SemiBold).Layout(gtx)
							})
						}),
					)
				})
			},
		)
	})
}

func (a *App) createActionItems() []createActionItem {
	buttons := []createActionItem{
		{click: &a.actionOpenFile, label: "打开本地文件", icon: "folder-open"},
		{click: &a.actionOpenFolder, label: "打开文件夹", icon: "folder"},
	}
	buttons = append(buttons,
		createActionItem{separator: true},
		createActionItem{click: &a.actionNewDOCX, label: "文档", icon: "file-text"},
		createActionItem{click: &a.actionNewXLSX, label: "表格", icon: "file-spreadsheet"},
		createActionItem{click: &a.actionNewJSON, label: "JSON 数据", icon: "file-code"},
		createActionItem{click: &a.actionNewMarkdown, label: "Markdown 文本", icon: "file-text"},
		createActionItem{click: &a.actionNewPDF, label: "PDF 文档", icon: "file"},
	)
	return buttons
}

func (a *App) createActionLabels() []string {
	items := a.createActionItems()
	labels := make([]string, 0, len(items))
	for _, item := range items {
		if !item.separator {
			labels = append(labels, item.label)
		}
	}
	return labels
}
