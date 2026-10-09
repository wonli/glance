package desktopui

import (
	"glance/internal/document"
	"image"
	"image/color"
)

// pptxToolbarIcon resolves and renders an icon strictly from the PPTX icon family (assets/icons/pptx).
func pptxToolbarIcon(name string, tint color.NRGBA) (image.Image, bool) {
	return pptxToolbarIconAtSize(name, tint, 96)
}

func pptxToolbarIconAtSize(name string, tint color.NRGBA, size int) (image.Image, bool) {
	return svgIconTintAtSize("pptx", name, size, tint)
}

// PPTX ribbon actions configured per tab, strictly matching Pen design frame iRLmn.
var pptxHomeActions = []ribbonAction{
	{id: "home_paste", label: "粘贴", group: "剪贴板", icon: "clipboard-paste"},
	{id: "home_copy", label: "复制", group: "剪贴板", icon: "copy"},
	{id: "home_new_slide", label: "新建幻灯片", group: "幻灯片", icon: "gallery-horizontal-end"},
	{id: "home_layout", label: "版式", group: "幻灯片", icon: "panels-top-left"},
	{id: "home_font", label: "字体", group: "字体", icon: "type"},
	{id: "home_font_size", label: "字号", group: "字体", icon: "type"},
	{id: "home_bold", label: "加粗", group: "字体", icon: "bold"},
	{id: "home_italic", label: "斜体", group: "字体", icon: "italic"},
	{id: "home_underline", label: "下划线", group: "字体", icon: "underline"},
	{id: "home_font_color", label: "字体颜色", group: "字体", icon: "baseline"},
	{id: "home_bullets", label: "项目符号", group: "段落", icon: "list"},
	{id: "home_align_left", label: "左对齐", group: "段落", icon: "format_align_left"},
	{id: "home_align_center", label: "居中", group: "段落", icon: "format_align_center"},
	{id: "home_align_right", label: "右对齐", group: "段落", icon: "align-right"},
	{id: "home_align_justify", label: "两端对齐", group: "段落", icon: "align-justify"},
	{id: "home_line_spacing", label: "行距", group: "段落", icon: "list"},
	{id: "home_space_before", label: "段前距", group: "段落", icon: "move-up"},
	{id: "home_space_after", label: "段后距", group: "段落", icon: "move-down"},
	{id: "home_shapes", label: "形状", group: "绘图", icon: "shapes"},
	{id: "home_fill", label: "填充颜色", group: "绘图", icon: "paint-bucket"},
	{id: "home_align", label: "对齐", group: "排列", icon: "align-start-vertical"},
	{id: "home_group", label: "组合", group: "排列", icon: "group"},
}

var pptxFileActions = []ribbonAction{
	{id: "file_new", label: "新建", group: "新建与打开", icon: "file-plus"},
	{id: "file_open", label: "打开", group: "新建与打开", icon: "folder-open"},
	{id: "file_save", label: "保存", group: "保存", icon: "save"},
	{id: "file_saveas", label: "另存为", group: "保存", icon: "save-all"},
	{id: "file_export", label: "导出 PDF", group: "输出", icon: "file-down"},
	{id: "file_print", label: "打印", group: "输出", icon: "printer"},
}

var pptxInsertActions = []ribbonAction{
	{id: "insert_table", label: "表格", group: "页面", icon: "table"},
	{id: "insert_picture", label: "图片", group: "图像", icon: "image"},
	{id: "insert_shapes", label: "形状", group: "插图", icon: "shapes"},
	{id: "insert_chart", label: "图表", group: "插图", icon: "chart-column"},
	{id: "insert_textbox", label: "文本框", group: "文本", icon: "text-cursor-input"},
	{id: "insert_video", label: "视频", group: "媒体", icon: "video"},
	{id: "insert_audio", label: "音频", group: "媒体", icon: "volume-2"},
}

var pptxDesignActions = []ribbonAction{
	{id: "design_theme", label: "主题", group: "主题", icon: "palette"},
	{id: "design_colors", label: "配色", group: "主题", icon: "paint-bucket"},
	{id: "design_bg", label: "背景格式", group: "背景", icon: "settings-2"},
	{id: "design_size", label: "幻灯片大小", group: "页面设置", icon: "ratio"},
}

var pptxTransitionsActions = []ribbonAction{
	{id: "trans_fade", label: "淡化", group: "切换效果", icon: "blend"},
	{id: "trans_push", label: "推进", group: "切换效果", icon: "move-right"},
	{id: "trans_options", label: "效果选项", group: "效果设置", icon: "sliders-horizontal"},
	{id: "trans_duration", label: "持续时间", group: "效果设置", icon: "timer"},
	{id: "trans_apply_all", label: "全部应用", group: "换片方式", icon: "copy-check"},
}

var pptxAnimationsActions = []ribbonAction{
	{id: "anim_appear", label: "出现", group: "动画效果", icon: "circle-plus"},
	{id: "anim_fadein", label: "淡入", group: "动画效果", icon: "blend"},
	{id: "anim_flyin", label: "飞入", group: "动画效果", icon: "move-up"},
	{id: "anim_add", label: "添加动画", group: "动画管理", icon: "plus"},
	{id: "anim_pane", label: "动画窗格", group: "动画管理", icon: "panel-right"},
	{id: "anim_duration", label: "持续时间", group: "计时", icon: "timer"},
}

var pptxSlideShowActions = []ribbonAction{
	{id: "show_from_start", label: "从头开始", group: "开始放映", icon: "presentation"},
	{id: "show_from_current", label: "从当前页", group: "开始放映", icon: "play"},
	{id: "show_rehearse", label: "排练计时", group: "放映设置", icon: "timer"},
	{id: "show_presenter", label: "演讲者视图", group: "演示工具", icon: "panels-top-left"},
}

var pptxReviewActions = []ribbonAction{
	{id: "review_spell", label: "拼写检查", group: "校对", icon: "spell-check"},
	{id: "review_new_comment", label: "新建批注", group: "批注", icon: "message-square-plus"},
	{id: "review_del_comment", label: "删除批注", group: "批注", icon: "message-square-x"},
	{id: "review_access", label: "辅助功能", group: "检查", icon: "accessibility"},
}

var pptxViewActions = []ribbonAction{
	{id: "view_normal", label: "普通视图", group: "演示视图", icon: "panels-top-left"},
	{id: "view_sorter", label: "幻灯片浏览", group: "演示视图", icon: "layout-grid"},
	{id: "view_notes", label: "备注页", group: "演示视图", icon: "sticky-note"},
	{id: "view_ruler", label: "标尺", group: "显示", icon: "ruler"},
	{id: "view_fit", label: "适应窗口", group: "缩放", icon: "maximize"},
}

var pptxPictureActions = []ribbonAction{
	{id: "picture_replace", label: "替换图片", group: "图片", icon: "image"},
	{id: "picture_crop", label: "裁剪", group: "图片", icon: "ratio"},
	{id: "picture_outline", label: "图片轮廓", group: "图片样式", icon: "paint-bucket"},
	{id: "picture_line_width", label: "轮廓宽度", group: "图片样式", icon: "sliders-horizontal"},
}

var pptxShapeActions = []ribbonAction{
	{id: "shape_fill", label: "形状填充", group: "形状样式", icon: "paint-bucket"},
	{id: "shape_no_fill", label: "无填充", group: "形状样式", icon: "x"},
	{id: "shape_outline", label: "形状轮廓", group: "形状样式", icon: "baseline"},
	{id: "shape_line_width", label: "轮廓宽度", group: "形状样式", icon: "sliders-horizontal"},
}

var pptxPictureCropActions = []ribbonAction{
	{id: "picture_crop_done", label: "完成裁剪", group: "裁剪", icon: "check"},
	{id: "picture_crop_cancel", label: "取消裁剪", group: "裁剪", icon: "x"},
	{id: "picture_crop_reset", label: "重置裁剪", group: "裁剪", icon: "rotate-ccw"},
}

type presentationRibbonTab struct {
	id    string
	label string
}

func presentationRibbonTabs(v *presentationViewState) []presentationRibbonTab {
	tabs := []presentationRibbonTab{
		{id: "file", label: "文件"},
		{id: "home", label: "开始"},
		{id: "insert", label: "插入"},
		{id: "design", label: "设计"},
		{id: "transitions", label: "切换"},
		{id: "animations", label: "动画"},
		{id: "slideshow", label: "幻灯片放映"},
		{id: "review", label: "审阅"},
		{id: "view", label: "视图"},
	}
	if _, _, ok := presentationSelectedEditablePicture(v); ok {
		tabs = append(tabs, presentationRibbonTab{id: "picture", label: "图片格式"})
	} else if _, elem, ok := presentationSelectedStyleElement(v); ok && elem.Kind != document.ElementKindPicture {
		tabs = append(tabs, presentationRibbonTab{id: "shape", label: "形状格式"})
	}
	return tabs
}

func presentationSelectedEditablePicture(v *presentationViewState) (int, document.PresentationElement, bool) {
	if v == nil || v.selectedShape == 0 {
		return 0, document.PresentationElement{}, false
	}
	slide := v.currentSlide()
	if slide == nil {
		return 0, document.PresentationElement{}, false
	}
	for _, elem := range slide.Elements {
		if elem.ID == v.selectedShape && elem.Kind == document.ElementKindPicture && elem.CanEditPicture {
			return v.activeSlide, elem, true
		}
	}
	return 0, document.PresentationElement{}, false
}

func pptxActionsForTab(tab string) []ribbonAction {
	switch tab {
	case "file":
		return pptxFileActions
	case "insert":
		return pptxInsertActions
	case "design":
		return pptxDesignActions
	case "transitions":
		return pptxTransitionsActions
	case "animations":
		return pptxAnimationsActions
	case "slideshow":
		return pptxSlideShowActions
	case "review":
		return pptxReviewActions
	case "view":
		return pptxViewActions
	case "picture":
		return pptxPictureActions
	case "shape":
		return pptxShapeActions
	default:
		return pptxHomeActions
	}
}

func presentationActionsForView(v *presentationViewState) []ribbonAction {
	if v != nil && v.activeRibbonTab == "picture" && v.pictureCrop.active {
		return pptxPictureCropActions
	}
	if v == nil {
		return nil
	}
	return pptxActionsForTab(v.activeRibbonTab)
}
