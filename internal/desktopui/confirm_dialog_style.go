package desktopui

import (
	"image"
	"image/color"
)

func defaultConfirmStyleForTitle(title string) (modalDialogConfirmStyle, bool) {
	switch title {
	case "保存更改？", "保存失败", "关闭文档":
		return modalDialogSaveConfirmStyle, true
	case "恢复历史版本？":
		return modalDialogRestoreConfirmStyle, true
	case "字数统计":
		return modalDialogInfoConfirmStyle, true
	case "替换":
		return modalDialogConfirmStyle{
			Icon:           "replace",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "查找":
		return modalDialogConfirmStyle{
			Icon:           "find",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "设置字体":
		return modalDialogConfirmStyle{
			Icon:           "font-family",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "调整图片大小", "设置图片宽度":
		return modalDialogConfirmStyle{
			Icon:           "insert-image",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "缩放":
		return modalDialogConfirmStyle{
			Icon:           "view-zoom",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "文档信息":
		return modalDialogConfirmStyle{
			Icon:           "doc-info",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "重命名工作表":
		return modalDialogConfirmStyle{
			Icon:           "file-spreadsheet",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "导出 PDF 预览":
		return modalDialogConfirmStyle{
			Icon:           "file-down",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "页面方向":
		return modalDialogConfirmStyle{
			Icon:           "layout-orientation",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "页边距设置":
		return modalDialogConfirmStyle{
			Icon:           "layout-margins",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "纸张大小设置":
		return modalDialogConfirmStyle{
			Icon:           "layout-size",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "段落缩进":
		return modalDialogConfirmStyle{
			Icon:           "layout-indent",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "打印区域设置":
		return modalDialogConfirmStyle{
			Icon:           "scan",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "插入链接":
		return modalDialogConfirmStyle{
			Icon:           "link",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "打印标题设置":
		return modalDialogConfirmStyle{
			Icon:           "heading",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "分栏设置":
		return modalDialogConfirmStyle{
			Icon:           "columns-2",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "冻结窗格":
		return modalDialogConfirmStyle{
			Icon:           "panels-top-left",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "删除重复项":
		return modalDialogConfirmStyle{
			Icon:           "trash-2",
			IconBackground: color.NRGBA{R: 253, G: 238, B: 238, A: 255},
			IconForeground: color.NRGBA{R: 199, G: 68, B: 68, A: 255},
		}, true
	case "插入分隔符":
		return modalDialogConfirmStyle{
			Icon:           "layout-breaks",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "保护工作表", "撤销保护工作表":
		return modalDialogConfirmStyle{
			Icon:           "lock-keyhole",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "行号设置":
		return modalDialogConfirmStyle{
			Icon:           "list-ordered",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "文本分列", "自定义分列":
		return modalDialogConfirmStyle{
			Icon:           "columns-3",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "创建表格":
		return modalDialogConfirmStyle{
			Icon:           "table",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "限制编辑":
		return modalDialogConfirmStyle{
			Icon:           "shield-check",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "页面颜色":
		return modalDialogConfirmStyle{
			Icon:           "palette",
			IconBackground: color.NRGBA{R: 243, G: 232, B: 255, A: 255},
			IconForeground: color.NRGBA{R: 147, G: 51, B: 234, A: 255},
		}, true
	case "插入函数", "最近使用的函数", "逻辑函数", "文本函数", "日期时间函数":
		return modalDialogConfirmStyle{
			Icon:           "sigma",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	case "名称管理器":
		return modalDialogConfirmStyle{
			Icon:           "tag",
			IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
			IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
		}, true
	default:
		return modalDialogConfirmStyle{}, false
	}
}

func findDialogIcon(family, name string, size int, tint color.NRGBA) (image.Image, bool) {
	if name == "" {
		return nil, false
	}
	if family != "" {
		if icon, ok := svgIconTintAtSize(family, name, size, tint); ok {
			return icon, true
		}
	}
	for _, fam := range []string{"confirm", "word", "excel", "pen"} {
		if fam == family {
			continue
		}
		if icon, ok := svgIconTintAtSize(fam, name, size, tint); ok {
			return icon, true
		}
	}
	return nil, false
}
