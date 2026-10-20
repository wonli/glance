package desktopui

func wordOfficeRibbonMenu(actionID string) officeRibbonMenuDefinition {
	option := func(label, command string, enabled bool) officeRibbonMenuOption {
		return officeRibbonMenuOption{label: label, command: command, enabled: enabled}
	}
	disabled := func(label string) officeRibbonMenuOption { return option(label, "", false) }
	switch actionID {
	case "print":
		return officeRibbonMenuDefinition{"打印", []officeRibbonMenuOption{option("打印…", "print", true), option("快速打印", "print", true), option("打印预览", "view-print-layout", true), option("页面设置…", "layout-margins", true)}}
	case "doc-info":
		return officeRibbonMenuDefinition{"文档信息", []officeRibbonMenuOption{option("属性…", "doc-info", true), disabled("检查文档"), option("版本历史", "version-history", true), option("保护文档…", "review-restrict", true)}}
	case "paste":
		return officeRibbonMenuDefinition{"粘贴选项", []officeRibbonMenuOption{disabled("保留源格式"), disabled("合并格式"), option("仅保留文本", "paste-text", true), disabled("选择性粘贴…")}}
	case "font-size":
		return officeRibbonMenuDefinition{"字号", []officeRibbonMenuOption{
			option("初号", "word-size-42", true),
			option("小初", "word-size-36", true),
			option("一号", "word-size-26", true),
			option("小一", "word-size-24", true),
			option("二号", "word-size-22", true),
			option("小二", "word-size-18", true),
			option("三号", "word-size-16", true),
			option("小三", "word-size-15", true),
			option("四号", "word-size-14", true),
			option("小四", "word-size-12", true),
			option("五号", "word-size-10.5", true),
			option("小五", "word-size-9", true),
			option("11", "word-size-11", true),
			option("12", "word-size-12", true),
			option("14", "word-size-14", true),
			option("16", "word-size-16", true),
			option("18", "word-size-18", true),
			option("24", "word-size-24", true),
			option("36", "word-size-36", true),
			option("自定义…", "word-size-custom", true),
		}}
	case "find":
		return officeRibbonMenuDefinition{"查找", []officeRibbonMenuOption{option("打开导航窗格…", "find", true), disabled("按标题浏览"), disabled("按页面浏览"), disabled("最近搜索")}}
	case "select":
		return officeRibbonMenuDefinition{"选择", []officeRibbonMenuOption{option("全选", "select-all", true), disabled("选择对象"), disabled("选择相同格式"), disabled("选择窗格…")}}
	case "cover":
		return officeRibbonMenuDefinition{"封面", []officeRibbonMenuOption{option("选择封面…", "cover", true), disabled("更多封面模板")}}
	case "insert-image":
		return officeRibbonMenuDefinition{"插入图片", []officeRibbonMenuOption{option("此设备…", "insert-image", true), disabled("最近使用"), disabled("图片库"), disabled("在线图片")}}
	case "insert-link":
		return officeRibbonMenuDefinition{"链接", []officeRibbonMenuOption{option("插入链接…", "insert-link", true), disabled("最近使用的链接")}}
	case "insert-header":
		return officeRibbonMenuDefinition{"页眉", []officeRibbonMenuOption{option("编辑页眉…", "insert-header", true), disabled("空白"), disabled("简洁"), disabled("横线样式")}}
	case "insert-textbox":
		return officeRibbonMenuDefinition{"文本框", []officeRibbonMenuOption{option("插入文本框…", "insert-textbox", true), disabled("简洁文本框"), disabled("侧边栏文本框"), disabled("绘制文本框")}}
	case "insert-equation":
		return officeRibbonMenuDefinition{"公式", []officeRibbonMenuOption{option("插入公式…", "insert-equation", true), disabled("分数"), disabled("上下标"), disabled("根式"), disabled("积分、求和与矩阵")}}
	case "references-toc":
		return officeRibbonMenuDefinition{"目录", []officeRibbonMenuOption{disabled("自动目录 1"), disabled("自动目录 2"), disabled("手动目录"), disabled("自定义目录…")}}
	case "references-footnotes":
		return officeRibbonMenuDefinition{"脚注和尾注", []officeRibbonMenuOption{disabled("插入脚注"), disabled("插入尾注"), disabled("下一条脚注"), disabled("脚注选项…")}}
	case "references-citations":
		return officeRibbonMenuDefinition{"引文", []officeRibbonMenuOption{disabled("最近使用的源"), disabled("添加新源…"), disabled("管理源"), disabled("引用样式")}}
	case "references-caption":
		return officeRibbonMenuDefinition{"题注", []officeRibbonMenuOption{disabled("插入题注…"), disabled("新建标签…"), disabled("题注位置")}}
	case "design-themes":
		return officeRibbonMenuDefinition{"主题", []officeRibbonMenuOption{option("主题设置…", "design-themes", true), disabled("Office"), disabled("简约"), disabled("专业")}}
	case "design-colors":
		return officeRibbonMenuDefinition{"主题颜色", []officeRibbonMenuOption{option("颜色方案…", "design-colors", true), disabled("自定义颜色")}}
	case "design-fonts":
		return officeRibbonMenuDefinition{"主题字体", []officeRibbonMenuOption{option("字体方案…", "design-fonts", true), disabled("自定义字体")}}
	case "design-effects":
		return officeRibbonMenuDefinition{"主题效果", []officeRibbonMenuOption{option("效果设置…", "design-effects", true), disabled("自定义效果")}}
	case "design-compact", "design-spacious":
		return officeRibbonMenuDefinition{"段落间距", []officeRibbonMenuOption{option("紧凑", "design-compact", true), disabled("默认"), option("宽松", "design-spacious", true), disabled("自定义间距…")}}
	case "design-watermark":
		return officeRibbonMenuDefinition{"水印", []officeRibbonMenuOption{option("水印设置…", "design-watermark", true), disabled("无水印"), disabled("机密"), disabled("草稿")}}
	case "layout-breaks":
		return officeRibbonMenuDefinition{"分隔符", []officeRibbonMenuOption{option("分页符", "page-break", true), option("分节符…", "layout-breaks", true), disabled("分栏符")}}
	case "layout-indent":
		return officeRibbonMenuDefinition{"缩进", []officeRibbonMenuOption{option("缩进设置…", "layout-indent", true), option("减少缩进", "decrease-indent", true), option("增加缩进", "increase-indent", true), disabled("特殊格式")}}
	case "layout-position":
		return officeRibbonMenuDefinition{"位置", []officeRibbonMenuOption{option("布局位置设置…", "layout-position", true), disabled("左上"), disabled("水平居中"), disabled("右下")}}
	case "review-new-comment":
		return officeRibbonMenuDefinition{"批注", []officeRibbonMenuOption{option("新建批注", "review-new-comment", true), disabled("新建并提及"), disabled("打开批注窗格"), option("删除当前批注", "review-delete-comment", true)}}
	case "review-compare":
		return officeRibbonMenuDefinition{"比较文档", []officeRibbonMenuOption{option("选择另一份文档…", "review-compare", true)}}
	case "view-reading":
		return officeRibbonMenuDefinition{"阅读视图", []officeRibbonMenuOption{option("阅读模式", "view-reading", true), option("打印布局", "view-print-layout", true), option("Web 版式", "view-web-layout", true), option("单页", "view-single-page", true), option("多页", "view-multiple-pages", true)}}
	case "view-ruler":
		return officeRibbonMenuDefinition{"显示", []officeRibbonMenuOption{option("水平标尺", "view-ruler", true), disabled("垂直标尺"), option("网格线", "view-gridlines", true), option("导航窗格", "view-navigation", true)}}
	case "view-new-window":
		return officeRibbonMenuDefinition{"窗口", []officeRibbonMenuOption{option("新建窗口", "view-new-window", true), disabled("全部重排"), disabled("拆分窗口"), option("并排查看", "view-side-by-side", true), disabled("同步滚动")}}
	default:
		return officeRibbonMenuDefinition{}
	}
}

func spreadsheetOfficeRibbonMenu(slot int) officeRibbonMenuDefinition {
	option := func(label, command string, enabled bool) officeRibbonMenuOption {
		return officeRibbonMenuOption{label: label, command: command, enabled: enabled}
	}
	disabled := func(label string) officeRibbonMenuOption { return option(label, "", false) }
	switch slot {
	case toolbarActionOpenFile:
		return officeRibbonMenuDefinition{"打开工作簿", []officeRibbonMenuOption{option("打开…", "excel-open", true), option("最近使用", "excel-recent", true), option("浏览…", "excel-open", true), disabled("恢复未保存的工作簿")}}
	case 21:
		return officeRibbonMenuDefinition{"粘贴选项", []officeRibbonMenuOption{disabled("保留源格式"), disabled("合并格式"), option("仅保留文本", "excel-paste-text", true), disabled("选择性粘贴…")}}
	case toolbarActionFontFamily:
		return officeRibbonMenuDefinition{"字体", []officeRibbonMenuOption{option("Aptos Narrow", "excel-font-aptos-narrow", true), option("等线", "excel-font-dengxian", true), option("宋体", "excel-font-simsun", true), option("微软雅黑", "excel-font-yahei", true), option("Arial", "excel-font-arial", true), option("更多字体…", "excel-font-custom", true)}}
	case toolbarActionFontSize:
		return officeRibbonMenuDefinition{"字号", []officeRibbonMenuOption{option("10", "excel-size-10", true), option("11", "excel-size-11", true), option("12", "excel-size-12", true), option("14", "excel-size-14", true), option("16", "excel-size-16", true), option("自定义…", "excel-size-custom", true)}}
	case toolbarActionCellStyles:
		return officeRibbonMenuDefinition{"单元格样式", []officeRibbonMenuOption{option("常规", "excel-style-normal", true), option("强调", "excel-style-emphasis", true), option("标题", "excel-style-title", true), disabled("其他样式")}}
	case 13:
		return officeRibbonMenuDefinition{"删除", []officeRibbonMenuOption{disabled("删除单元格并上移"), disabled("删除单元格并左移"), option("删除整行", "excel-delete-row", true), option("删除整列", "excel-delete-column", true), option("删除工作表", "excel-delete-sheet", true)}}
	case toolbarActionFormat:
		return officeRibbonMenuDefinition{"单元格格式", []officeRibbonMenuOption{
			option("设置单元格格式…", "excel-format-cells", true),
			option("行高…", "excel-row-height", true),
			option("自动调整行高", "excel-autofit-row", true),
			option("列宽…", "excel-column-width", true),
			option("自动调整列宽", "excel-autofit-column", true),
		}}
	case toolbarActionFreezePanes:
		return officeRibbonMenuDefinition{"冻结窗格菜单", []officeRibbonMenuOption{
			option("冻结窗格", "excel-freeze-selection", true),
			option("冻结首行", "excel-freeze-first-row", true),
			option("冻结首列", "excel-freeze-first-column", true),
			option("取消冻结窗格", "excel-unfreeze-panes", true),
		}}
	case toolbarActionSum:
		return officeRibbonMenuDefinition{"自动求和", []officeRibbonMenuOption{
			option("求和", "excel-sum", true),
			option("平均值", "excel-average", true),
			option("计数", "excel-count", true),
			option("最大值", "excel-max", true),
			option("最小值", "excel-min", true),
		}}
	case 18:
		return officeRibbonMenuDefinition{"排序和筛选", []officeRibbonMenuOption{option("升序", "excel-sort-asc", true), option("降序", "excel-sort-desc", true), disabled("自定义排序…"), option("筛选", "excel-filter", true), option("清除筛选", "excel-clear-filter", true), disabled("重新应用")}}
	case toolbarActionFind:
		return officeRibbonMenuDefinition{"查找和选择", []officeRibbonMenuOption{option("查找…", "excel-find", true), option("替换…", "excel-replace", true), disabled("定位…"), disabled("定位条件…"), disabled("选择对象")}}
	case toolbarActionInsertTable:
		return officeRibbonMenuDefinition{"插入表格", []officeRibbonMenuOption{option("设置表格范围…", "excel-insert-table", true), disabled("表格大小选择网格")}}
	case toolbarActionLayoutMargins:
		return officeRibbonMenuDefinition{"页边距", []officeRibbonMenuOption{option("普通", "excel-margin-normal", true), option("窄", "excel-margin-narrow", true), option("宽", "excel-margin-wide", true), disabled("上次自定义"), option("自定义…", "excel-margin-custom", true)}}
	case toolbarActionRefreshAll:
		return officeRibbonMenuDefinition{"刷新", []officeRibbonMenuOption{disabled("刷新全部"), disabled("刷新当前连接"), disabled("连接属性…"), disabled("查询状态")}}
	case toolbarActionSpellCheck:
		return officeRibbonMenuDefinition{"校对", []officeRibbonMenuOption{disabled("建议"), disabled("忽略一次"), disabled("更改语言…"), disabled("自动更正选项…")}}
	case toolbarActionNormalView:
		return officeRibbonMenuDefinition{"工作簿视图", []officeRibbonMenuOption{option("普通", "excel-view-normal", true), option("分页预览", "excel-view-page-break", true), disabled("页面布局"), disabled("自定义视图")}}
	default:
		return officeRibbonMenuDefinition{}
	}
}
