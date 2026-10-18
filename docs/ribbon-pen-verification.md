# Pen Ribbon 还原与图标核对（2026-10-15）

参考：Pen `kiBim`（Word）和 `nmShb`（Excel）。本次基于工作树中已有的界面修改继续修正，没有提交 Git commit。

## 尺寸与排列

- 命令区 88dp，分类栏 34dp，总高 122dp。普通命令为 80×68dp、4dp 间隔；开始页分组高 72dp、上下 8dp。
- Word 分类显示顺序为文件、开始、插入、引用、设计、布局、审阅、视图。非开始页不再插入额外的 AI 入口；开始页的 AI、剪贴板、字体、段落、样式和编辑分组按设计定位。
- Excel 开始页保留经典双行分组，其余分类按设计排列大按钮；修正导出/打印、打印标题/打印网格线的顺序。
- 字体下拉框保留完整按钮区域，文字按自然高度居中，使用实际 chevron-down 矢量。修正数字区重复文字、字符按钮顺序和 2x 下像素误当 dp 引起的紧凑按钮溢出。
- 修正短命令行被 Background 居中的问题，菜单锚点随当前按钮尺寸和滚动位置计算。

## 图标完整清单

`ribbon-pen-icons.json` 记录 Word 的 109 个和 Excel 的 86 个图标实例，含分类、命令、来源图标库、名称、颜色和尺寸。共 195 个实例、123 个不同图形。

通过 Pen 工具导出这两个节点为 HTML，从导出结果提取实际 SVG 路径到 `internal/desktopui/assets/icons/ribbon`，没有凭名称重画近似图标。资产含 Lucide、Phosphor 与 Material Symbols Rounded 的原始填充轮廓；不对填充轮廓重复做描边补偿。`office_ribbon_icon_mapping.go` 将 Word action 和 Excel slot 映射到准确图形；数据页 slot 18 单独使用 arrow-up-down，开始页同槽使用 list-filter。

重点替换：Word 打印、另存为、自动保存、导出 PDF、目录、脚注、尾注、引文、题注、页码、修订、限制编辑、阅读视图及宏；Excel 条件格式、单元格样式、插入/删除、格式、日期时间、打印标题与数字区图标。下拉箭头与字号增减也使用 Pen 实际图形。

## 验证证据

- 新回归先复现附件中的 `stack push and pop must not cross macro boundary`，修正 PassOp 的 Push/Pop 录制边界后通过；Word/Excel 两种菜单都覆盖。
- 几何回归先复现文件页错误的宽度、左边距和总高度；修正后覆盖 1x/2x 的按钮边界与菜单左边距。
- `TestPenRibbonEveryReferenceGlyphHasVisiblePixels` 检查全部参考图标在正常/Retina 尺寸的非空可见像素、实际不透明像素和纯色着色。Material 缩进图形沿用设计较细的真实轮廓，不因 1x 不透明像素较少而擅自加粗。
- 22 项相关测试通过，包括图标资产实际渲染、菜单、横向滚动、符号按钮点击、主页分组几何、画布与工具栏接缝、命令覆盖清单。
- `TestPenRibbonOverviewVisual` 的 Word/Excel × 1x/2x 四个子测试实际使用 Metal GPU 通过，没有以 SKIP 计为视觉通过。截图分别为 `/private/tmp/glance-ribbon-screenshots/word-1x.png`、`word-2x.png`、`excel-1x.png`、`excel-2x.png`。使用合成文档与测试工作簿生成。
- 原生窗口验证使用当前源码构建的 `/private/tmp/GlanceRibbonQA.app`。每轮退出旧进程，再构建并冷启动；最终窗口中实际展开 Word 新建与 Excel 打开菜单，确认无崩溃且锚点贴合按钮。
- `go build ./cmd/glance` 与 `git diff --check` 通过。相关详细输出保存在 `/private/tmp/glance-scope-final.log`。

## 范围与限制

实际能力仍决定启用状态：不可用命令显示禁用灰色，不能通过视觉还原将其伪装成可执行功能。引用页新增的更新目录、尾注、书目与交叉引用是禁用的设计占位；不宣称已实现其文档修改能力。该页原有四个入口仍使用既有菜单定义。已有浮层选项不属于这两个功能区总览的像素参考，未宣称这些浮层与另一个设计节点逐像素一致。

全量 `go test ./...` 已在允许 Metal、本地 HTTP 和 Unix socket 的环境运行，未通过。其日志在 `/private/tmp/glance-all-tests-final.log`。命令清单中新加映射导致的 `TestEveryManifestCapabilityIsRegisteredAndCallable` 已修正并单独重跑通过；其余仍失败的测试如下。本次没有将这些失败隐去或计为通过，也未据此声称整个应用已达到像素一致或全量测试通过。

- `TestDesktopMCPStdioEditsTheLiveGioSession`
- `TestSettingsFieldBlankEdgesFocusEditor`
- `TestSettingsSavedKeyEdgeClickFocusesVisiblePreview`
- `TestSettingsAPIKeyEyeClickTogglesVisibility`
- `TestSettingsFieldsVerticallyCenterText`
- `TestSpreadsheetRecalculateAndRefreshAll`
- `TestSpreadsheetToolbar_SpellCheckAndTranslate`
- `TestSpreadsheetViewModesAndRuler`
- `TestHideMainWindowToTrayPreconditions`
- `TestWordUpDownKeepsPreferredVisualColumnAcrossVisualLines`
- `TestWordToolbar_WatermarkTrackRevisionsEditorAndLayout`
- `TestWordToolbar_RemainingActionsBatch`
- `TestExportSnapshotUsesSameParagraphGeometryAsDesktopPages`
- `TestMCPCLIRecognizesServeAndDocumentsStdoutContract`
- `TestLoadManifestLoadsCuratedCorpus`

## 最新开始页同步（2026-10-15）

重新读取 `kiBim`、`nmShb`：两处 AI 入口引用的 20dp 叠页星芒来自组件 `DwuIp`。`assets/icons/brand/ai-{back,front,star}.svg` 保留 Pen 导出的路径和变换，各层分别读取 `Toolbar.AccentAISecondary`（`#A8C3FA`）、`AccentAI`（`#2463EB`）、`AccentAIStar`（白色），不对多色图形做单色 tint。

开始页选中按钮使用 `Toolbar.ActiveBackground = #E0EBFF`、`ActiveBorder = #A8C3FA`、`ActiveGlyph = #2463EB`，圆角 4dp、边框 1dp；悬停保留选中反馈。主页导航、工作区标签和单元格选择配色未在本轮调整。

退出旧 Glance 进程后构建并重新启动，实际核对 Word 和 Excel 开始页。针对性 8 项测试通过，其中总览 GPU 渲染覆盖两种格式的 1x/2x；`TestPenRibbonBrandAndActiveStateVisual` 的 6 个子项检查两种格式的 1x/2x 默认样式及 2x 自定义 token。无 GPU SKIP。截图在 `/private/tmp/glance-latest-ribbon/`，详细日志在 `/private/tmp/glance-latest-ribbon-tests.log`。本轮未重跑全量测试；之前的全量失败记录仍见上文。

### 粘贴、格式刷和单元格边框补充

再次核对开始页：粘贴、格式刷的默认图标为 `#000000`，已同步至 `Toolbar.AccentClipboard`（暗色主题使用与普通图标一致的浅色）。用户建议调整 Excel 选中单元格边框，本轮将活动单元格、选区和单元格编辑时的边框统一接入 `Grid.SelectionBorder = #2463EB`，保留原有线宽与选择几何。退出旧进程并重新启动后实际核对 Word/Excel 图标、单个单元格及拖选 A1:C3 的边框；相关 1x/2x Ribbon 渲染、选择与格式刷测试通过。日志：`/private/tmp/glance-clipboard-grid-tests.log`。未重跑全量测试。

### Word 与 Excel 字号 A+ / A− 图标（2026-10-18）

Word 与 Excel 的字号按钮统一使用 Material Symbols Outlined 的 `text_increase` / `text_decrease`；Pen 图标清单同步记录为 A+ / A−。项目资源位于 `internal/desktopui/assets/icons/ribbon/text_increase.svg`、`text_decrease.svg`，两个 Ribbon 都映射到这两个资源名。Word 与 Excel 的字体、字号下拉框共用相同的背景、文字字号、内边距和下拉箭头布局。原始 SVG 来自 Google 的 Material Design Icons 仓库（Apache-2.0）：

- [text_increase](https://github.com/google/material-design-icons/blob/master/symbols/web/text_increase/materialsymbolsoutlined/text_increase_24px.svg)
- [text_decrease](https://github.com/google/material-design-icons/blob/master/symbols/web/text_decrease/materialsymbolsoutlined/text_decrease_24px.svg)

这两份 SVG 保留官方路径，将负起点 viewBox 等价转换为零起点 viewBox 加平移，并显式设置黑色 fill，以兼容现有栅格化和 tint 路径。
