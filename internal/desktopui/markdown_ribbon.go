package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
)

var markdownToolbarActions = []ribbonAction{
	{id: "open", label: "打开", group: "file", icon: "open"}, {id: "save", label: "保存", group: "file", icon: "save"}, {id: "save-as", label: "另存为", group: "file", icon: "save-as"}, {id: "export-pdf", label: "导出 PDF", group: "file", icon: "export-pdf"},
	{id: "heading", label: "标题", group: "format", icon: "heading"}, {id: "bold", label: "加粗", group: "format", icon: "bold"}, {id: "italic", label: "斜体", group: "format", icon: "italic"}, {id: "quote", label: "引用", group: "format", icon: "quote"}, {id: "code", label: "代码", group: "format", icon: "code"},
	{id: "link", label: "链接", group: "insert", icon: "link"}, {id: "image", label: "图片", group: "insert", icon: "insert-image"}, {id: "list", label: "列表", group: "insert", icon: "bullet-list"}, {id: "todo", label: "待办", group: "insert", icon: "todo"}, {id: "math", label: "公式", group: "math", icon: "insert-equation"},
}

func markdownToolbarIcon(action ribbonAction, enabled bool) (image.Image, bool) {
	for _, family := range []string{"markdown", "word", "excel", "json"} {
		if img, ok := svgIconTintAtSize(family, action.icon, 192, toolbarIconColor(enabled)); ok {
			return img, true
		}
	}
	return nil, false
}
func (a *App) layoutMarkdownRibbon(gtx layout.Context) layout.Dimensions {
	v := a.ensureMarkdownView()
	gtx = jsonFixedHeight(gtx, 54)
	return jsonPanel(gtx, contentSurface, false, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 8, Right: 12, Top: 3, Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return v.toolbar.Layout(gtx, len(markdownToolbarActions), func(gtx layout.Context, index int) layout.Dimensions {
						action := markdownToolbarActions[index]
						enabled := !a.isSaving() && !v.exportPending && !v.editPending
						icon, _ := markdownToolbarIcon(action, enabled)
						return layoutToolbarActionItem(gtx, action, index, markdownToolbarActions, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight)))
							return toolbarIconButton(gtx, a.theme, v.button(action.id), icon, 18, action.label, action.label, enabled, false, contentSurface)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 8}.Layout(gtx, a.layoutMarkdownModes)
				}),
			)
		})
	})
}
func (a *App) layoutMarkdownModes(gtx layout.Context) layout.Dimensions {
	v := a.ensureMarkdownView()
	width := unit.Dp(202)
	if gtx.Constraints.Max.X < gtx.Dp(202) {
		width = unit.Dp(float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp)
	}
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(width), gtx.Dp(34)))
	paint.FillShape(gtx.Ops, chromeSurface, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(6)).Op(gtx.Ops))
	return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{}
		for i, it := range []struct{ id, label string }{{"mode-edit", "编辑"}, {"mode-preview", "预览"}, {"mode-split", "分屏"}} {
			i, it := i, it
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return v.button(it.id).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if i == v.mode {
						paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
					}
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, it.label, 11, jsonBlue, i == v.mode, false)
					})
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}
