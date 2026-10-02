# Word 主页面与功能区 TODO（中文）

更新日期：2026-10-02
状态依据：当前仓库实现与 Pen 参考节点 `Clpgm`、`kiBim`、`FsQrk` 对照。

## 目标与已确认方向

- 以 Pen `Clpgm` 的 Word 主页面和 `kiBim` 的功能区分组、文字标签、控件位置为布局参考。
- 以 Pen `FsQrk` 为底栏视觉参考。
- **Word 功能区图标采用 Excel 现有图标集与着色方案**，与 Excel 的图标语言统一。该决定优先于“功能区图标逐像素使用 Pen 原始图形”；布局、分组和图标语义仍按 Pen 对照。
- 所有功能只有在 UI 入口、文档模型/DOCX 适配器路径和保存重开证据具备后，才标记为“已完成”并启用。仅绘制了图标或定义了 action 不算功能完成。

## 统计口径与总览

统计单位是功能区中的一个可点击命令图标。七个常驻 Pen 功能区共有 99 个命令图标；不把标签页文字、分隔线、输入框、样式库、标尺及底栏图标计入这个数字。

| 功能区 | 命令图标数 | 已接通 | 待实现 |
|---|---:|---:|---:|
| 文件 | 12 | 7 | 5 |
| 开始 | 27 | 22 | 5 |
| 插入 | 16 | 2 | 14 |
| 设计 | 9 | 0 | 9 |
| 布局 | 12 | 0 | 12 |
| 审阅 | 11 | 1 | 10 |
| 视图 | 12 | 0 | 12 |
| **常驻功能区合计** | **99** | **32** | **67** |
| 图片上下文功能区（额外项，不在 Pen 的 99 项内） | 5 | 5 | 0 |

另有撤销/重做的 DOCX 历史能力和调用代码，但 `wordHistoryActions` 未进入当前可见的七个常驻标签的 action 列表。它们不计入上表；需要对照 `Clpgm` 最终确认目标入口，如果设计要求显示图标，再加入功能区并补视觉验收。

## 已完成项

### 主页面与布局

- [x] Word 主页面采用独立的 Gio 布局和文档会话状态。
- [x] 七个常驻功能区标签及其 Pen 命令顺序已建模：文件、开始、插入、设计、布局、审阅、视图。
- [x] Word 连续流正文支持段落、表格、格式文本和行内图片；编辑器支持已覆盖范围内的文字修改、段落拆分/合并与图片编辑。
- [x] 顶部文档标签栏匹配 `Clpgm`/Excel Pen 参考：标签条高 42 dp，文档标签 190×37 dp，Glance/文档/加号之间留 6 dp；两个文档标签时加号锚点为 x=526 dp。
- [x] `FsQrk` 底栏的 34 dp 区域、主要文字和图形已绘制。
- [x] 当前底栏字数由正文内容动态计算。
- [x] 标尺、纸张画布和底栏在当前实现中已有对应布局。

### 已接通的 32 个常驻功能区图标

- [x] 文件：新建、打开、最近使用、保存、另存为、自动保存、文档信息。
- [x] 开始：粘贴、剪切、复制、格式刷、字体族、减小字号、增大字号、加粗、斜体、下划线、高亮、字体颜色、项目符号、编号、左对齐、居中、右对齐、两端对齐、减少缩进、增加缩进、查找、替换。
- [x] 插入：图片、分页符。
- [x] 审阅：字数统计。

### 已接通的 5 个图片上下文操作

- [x] 替换图片。
- [x] 删除图片。
- [x] 保持宽高比调整图片宽度。
- [x] 向左旋转 90°。
- [x] 向右旋转 90°。

这些图片操作经过 DOCX 图片能力接口调用，不应与七个常驻标签的 99 项混算。

## 按图标逐项 TODO

状态说明：`[x]` 表示该功能区图标已有用户可触发的操作路径；`[ ]` 表示图标虽已定义或绘制，但当前没有对应的 Word ribbon 行为。若现有其他位置提供了相近操作，会在条目中说明；这仍不算该图标已接通。

### 文件（12 项：已完成 7，待实现 5）

- [x] 新建 `new`：创建空白 DOCX 草稿并切换到新文档标签；保留已有文档会话。
- [x] 打开 `open`：接入工作区异步 DOCX/XLSX/JSON 文件选择器和现有打开流程；取消不改变当前文档。
- [x] 最近使用 `recent`：切换至工作区“最近”页面；保留当前文档会话。
- [x] 保存 `save`：保存当前 DOCX；等待待提交文字草稿后执行。
- [x] 另存为 `save-as`：接入工作区已有的 DOCX Save As 流程；取消时保留原文档、路径和脏状态。
- [x] 自动保存 `autosave`：按文档保存开关状态；成功提交 Word 编辑后后台保存。无路径草稿先由工作区快照恢复保护，另存后自动保存源 DOCX；保存失败使用错误状态且保留脏状态，关闭仍按未保存文档流程确认。重启恢复开关状态有用例。
- [ ] 打印 `print`：需要打印页面布局或 PDF 渲染路径，详见“当前技术边界”。
- [ ] 导出 PDF `export-pdf`：先选定渲染方案，不能把连续流截图伪称 Word 打印版。
- [ ] 共享 `share`：接入平台分享服务或安全的导出文件流程；当前无 Word 专属分享入口。
- [x] 文档信息 `doc-info`：异步读取并编辑标题、主题、作者、关键词、说明五个 DOCX 核心属性；保存重开、XML 特殊字符、未知元数据保留、撤销/重做均有适配器回归。缺失核心属性时补齐部件、包根关系和内容类型；重复、嵌套或含非文本标记的目标属性拒绝写入，避免破坏无法安全解释的元数据。
- [ ] 保护文档 `protect-document`：先确定保护模式范围；密码/权限保护需单独做安全与兼容性验证。
- [ ] 版本历史 `version-history`：定义持久化版本快照、恢复及与当前会话撤销栈的关系。

### 开始（27 项：已完成 22，待实现 5）

- [x] 粘贴 `paste`：将纯文本粘贴到当前可编辑文本 run。
- [x] 剪切 `cut`：复制选中文本并删除对应内容。
- [x] 复制 `copy`：将当前 run 或选中文本写入系统剪贴板。
- [x] 格式刷 `format-painter`：单击来源 run 的格式刷后，再单击目标 run，将来源的支持文字属性以及段落样式、列表、对齐、缩进复制到目标完整 run/段落；同一 DOCX 中完成为一个可撤销操作。当前编辑器只写入整 run 样式，不能独立改写 run 内所选子串；遇到不支持的列表定义时拒绝修改。
- [x] 字体 `font-family`：输入字体名称后写入所选 run 的 `ascii`、`hAnsi`、`eastAsia`、`cs` 字体属性；保留其他 run 属性，移除冲突的主题字体属性，并验证保存重开。
- [x] 减小字号 `font-size-decrease`：选中可编辑文本后按 1 pt 减小；字号范围 1–1638 pt。
- [x] 增大字号 `font-size-increase`：选中可编辑文本后按 1 pt 增大；run 没有显式字号时暂以 11 pt 作为起始值。
- [x] 加粗 `bold`：通过 DOCX 文本格式 patch 修改。
- [x] 斜体 `italic`：通过 DOCX 文本格式 patch 修改。
- [x] 下划线 `underline`：通过 DOCX 文本格式 patch 修改。
- [x] 高亮 `text-highlight`：提供 Word 标准命名高亮色和“无高亮”选项；局部更新 `w:highlight`、连续流预览绘制文字宽度高亮，并覆盖保存重开及其他 run 属性保留。当前不支持主题色或任意 RGB 高亮。
- [x] 字体颜色 `font-color`：颜色选择器可修改选中文本颜色。
- [x] 项目符号 `bullet-list`：可切换当前段落的多级项目符号（层级 0–8）；按需创建 `numbering.xml`、关系和内容类型，保存重开、撤销/重做和未修改包部件保留均有回归用例。多级标记使用 `•`、`◦`、`▪` 循环。
- [x] 编号 `numbered-list`：可切换当前段落的多级十进制编号（层级 0–8）；相邻同类段落连续计数，父级变化后子级重新计数，断开的编号列表从 1 重新开始；保存重开与取消列表均有回归用例。
- [x] 左对齐 `align-left`：修改所选段落对齐方式。
- [x] 居中 `align-center`：修改所选段落对齐方式。
- [x] 右对齐 `align-right`：修改所选段落对齐方式。
- [x] 两端对齐 `align-justify`：修改所选段落对齐方式。
- [x] 减少缩进 `decrease-indent`：列表段落降低 `ilvl`，一级列表降级为普通段落；普通段落左缩进每次减少 0.5 英寸且最小为 0。保留右缩进及既有首行/悬挂缩进属性。
- [x] 增加缩进 `increase-indent`：列表段落提高 `ilvl`（仅到其定义的最大层级）；普通段落左缩进每次增加 0.5 英寸。保留右缩进及既有首行/悬挂缩进属性，并受 DOCX twips 范围限制。
- [x] 查找 `find`：大小写不敏感地遍历可编辑文字 run，支持当前命中后的下一项、循环回绕和未提交草稿；自动滚动到段落并选中命中范围。查询不跨 run，字段等只读文字不参与搜索。
- [x] 替换 `replace`：对当前选中的、不区分大小写匹配的文字范围进行替换；保留所在 run 格式，替换后自动定位下一项。查询和替换值在双输入对话框中编辑。替换限定在单个可编辑 run 内，不跨 run、只读字段、图片或段落边界。
- [ ] 选择 `select`：定义选择全部、当前段落等具体命令；现有选择主要基于 run/段落，尚无完整跨 run 选择模型。
- [ ] 编辑器 `editor`：确认该图标要打开的具体能力；没有后端或真实编辑能力前保持不可用。
- [ ] 听写 `dictate`：需要系统语音识别或受控的语音服务接入，详见技术边界。
- [ ] 加载项 `add-ins`：当前无 Office Add-ins 运行时或插件宿主；需单独设计安全扩展接口。
- [ ] 设计器 `designer`：没有相应布局建议服务或本地规则引擎；明确产品范围后再接入。

### 插入（16 项：已完成 2，待实现 14）

- [ ] 封面 `cover`：提供受支持的本地封面模板，并按 DOCX 正文结构插入。
- [ ] 空白页 `blank-page`：实现明确的分页/分节语义；不能只插入空白段落后声称创建了新页。
- [x] 分页 `page-break`：在选中的正文文字 run 的光标位置写入 DOCX 原生 `w:br w:type="page"`；选区会被分页符替换。连续流预览显示分页符标记但不声称完成分页。保留原 run 格式，提交未保存文字草稿后再原子插入；保存重开、undo/redo 和 UI 异步插入均有回归。表格段落、含额外非文本子节点的 run 暂不开放。
- [ ] 表格 `insert-table`：当前可预览已有表格；增加 neutral 表格编辑/插入能力及 DOCX 表格写入。
- [x] 图片 `insert-image`：从异步文件选择器插入受支持的行内 PNG/JPEG 图片。
- [ ] 形状 `insert-shapes`：增加 DrawingML 形状对象的 neutral 表示、写入和预览；当前未知 DrawingML 内容不能直接编辑。
- [ ] 图表 `insert-chart`：创建图表部件、关系与图表数据；需定义是否支持编辑数据及图表种类。
- [ ] 链接 `insert-link`：增加超链接 run/relationship 模型，支持创建、编辑和保留外部关系。
- [ ] 书签 `insert-bookmark`：实现文档内唯一书签 ID 的创建、定位和安全删除。
- [ ] 页眉 `insert-header`：扩展到 section 与 header part；需先补齐保存/重开能力。
- [ ] 页脚 `insert-footer`：扩展到 section 与 footer part；动态页码还依赖分页方案。
- [ ] 页码 `insert-page-number`：静态 PAGE 字段可写入，但正确显示页码/总页数需要分页渲染能力。
- [ ] 文本框 `insert-textbox`：增加 DrawingML 文本框结构和位置/环绕显示支持。
- [ ] 艺术字 `insert-wordart`：依赖 DrawingML 形状/文本框支持及字体效果渲染。
- [ ] 公式 `insert-equation`：增加 OMML 投影/插入能力或明确先保持只读。
- [ ] 符号 `insert-symbol`：提供字符面板并按当前 caret 插入 Unicode 字符；不得以替换整个 run 的方式实现。

### 设计（9 项：已完成 0，待实现 9）

- [ ] 主题 `design-themes`：解析并编辑文档主题；先限定支持的主题属性与回退行为。
- [ ] 颜色 `design-colors`：映射主题颜色到安全的 DOCX theme/style 更新。
- [ ] 字体 `design-fonts`：修改文档主题字体；需与单段字体设置区分。
- [ ] 效果 `design-effects`：明确作用对象；形状/文字效果依赖相应 DrawingML 支持。
- [ ] 紧凑 `design-compact`：通过受控的段落间距 patch 实现并保存验证。
- [ ] 宽松 `design-spacious`：同上，定义对选区还是全篇应用。
- [ ] 水印 `design-watermark`：一般需要页眉/背景 DrawingML 能力及 section 范围。
- [ ] 页面颜色 `design-page-color`：增加 section 页面属性写入并验证其他页面属性不丢失。
- [ ] 页面边框 `design-page-border`：增加 section page-border 属性写入及 Word 重开验证。

### 布局（12 项：已完成 0，待实现 12）

- [ ] 页边距 `layout-margins`：增加 section 页面设置读取/写入；当前纸张画布宽度调整不等同 DOCX 页边距编辑。
- [ ] 方向 `layout-orientation`：写入 section 方向和对应纸张尺寸，测试不同 section。
- [ ] 纸张大小 `layout-size`：增加受限纸张预设及 section 属性 patch。
- [ ] 分栏 `layout-columns`：增加 section 分栏属性和连续流预览规则。
- [ ] 分隔符 `layout-breaks`：区分页、栏、节分隔符并实现 DOCX 写入与显示。
- [ ] 行号 `layout-line-numbers`：需分页/行布局结果或明确只实现近似连续视图的行号。
- [ ] 缩进 `layout-indent`：已有 ruler 的左右缩进能力；需接通 ribbon 命令并说明它调整左缩进、右缩进还是特殊缩进。
- [ ] 段前 `layout-before`：增加段落 spacing patch，保留其他 `pPr` 属性。
- [ ] 段后 `layout-after`：同上。
- [ ] 位置 `layout-position`：当前主要支持行内图片；浮动对象位置需要 DrawingML anchor 能力。
- [ ] 环绕文字 `layout-wrap`：当前没有 Word 文本环绕引擎；浮动图文环绕无法通过行内图片模型实现。
- [ ] 对齐 `layout-align`：明确这是对象对齐还是段落对齐；目前段落对齐已有 Home 命令，浮动对象对齐未实现。

### 审阅（11 项：已完成 0，待实现 11）

- [ ] 编辑器 `review-editor`：若指校对编辑器，需定义与 Home“编辑器”的差别及校对引擎。
- [x] 字数统计 `review-word-count`：打开详情面板，列出含/不含空白字符数、段落数和表格数；统计包括尚未提交的文字草稿。当前统计的是 DOCX 正文文字，不含页数估算、脚注/尾注等未投影内容。
- [ ] 翻译 `review-translate`：需要离线词典或外部翻译服务；未配置服务前保持不可用。
- [ ] 语言 `review-language`：增加 run/段落语言属性写入；与 UI 显示语言分开处理。
- [ ] 新建批注 `review-new-comment`：增加 Word comment、comments part 和关系模型。
- [ ] 删除批注 `review-delete-comment`：在批注对象及其引用模型完成后实现安全删除。
- [ ] 修订 `review-track`：当前无 tracked changes 投影和写入能力，不能仅用普通文本差异模拟。
- [ ] 接受 `review-accept`：需根据修订类型应用插入/删除并保留未处理修订。
- [ ] 拒绝 `review-reject`：同上。
- [ ] 比较 `review-compare`：可先做 neutral 文本/段落差异；完整 Word 比较及修订生成需更广的 OOXML 支持。
- [ ] 限制编辑 `review-restrict`：增加保护设置模型和明确的防误操作边界；不等同加密或强安全权限控制。

### 视图（12 项：已完成 0，待实现 12）

- [ ] 阅读模式 `view-reading`：定义简化视图状态及恢复路径。
- [ ] 打印布局 `view-print-layout`：依赖分页/打印布局渲染，当前连续流模式不能冒充该视图。
- [ ] Web 版式 `view-web-layout`：定义无纸张边界的连续排版规则。
- [ ] 标尺 `view-ruler`：横竖标尺当前已显示；图标尚未接入显示/隐藏开关。
- [ ] 网格线 `view-gridlines`：增加 Word 画布网格线状态和绘制，不修改 DOCX 内容。
- [ ] 导航窗格 `view-navigation`：增加标题/查找结果索引；当前 DOCX 投影尚无完整标题导航索引。
- [ ] 缩放 `view-zoom`：底栏当前绘有静态滑块及 100% 文本；实现真正缩放前需定义缩放对纸张、字体、光标及拖拽坐标的统一换算。
- [ ] 单页 `view-single-page`：依赖分页模型。
- [ ] 多页 `view-multiple-pages`：依赖分页模型和多页 viewport。
- [ ] 新建窗口 `view-new-window`：接入 workspace/window 生命周期并明确是否复用当前文档会话。
- [ ] 并排查看 `view-side-by-side`：需要双视口、独立滚动/选择状态和窗口宽度策略。
- [ ] 宏 `view-macros`：当前无 VBA 宏读取/编辑/执行能力；禁止在应用内不受控执行宏。

## 主页面底部 `FsQrk` 的未完成状态

Pen `Clpgm` 的底栏内容已核对：左侧依次显示“第 1 页，共 3 页”、`582 个字`、`中文(中国)`、辅助功能图标和“辅助功能：正常”；右侧依次是焦点模式、打印布局、Web 版式、缩小、缩放滑块、放大和 `100%`。底栏的**区域与图形已绘制**，但绘制出来不代表控件已有功能。

- [x] 底栏位置、尺寸、字数区域及左右侧图形已画出。
- [x] 修正静态页数文案：当前显示“第 1 页”，但应用没有 Word 分页器。现显示“连续视图”；在分页能力未完成前不显示页码/总页数。
- [ ] 视图/缩放图标、滑块和 `100%` 当前为静态绘制；要么接入真实状态与点击/拖拽行为，要么按设计明确绘制为不可交互状态。
- [ ] 辅助功能图标当前是静态状态图形；若设计要求打开辅助功能面板，再增加有实际内容的操作。

底栏的 6 个图形图标和缩放滑块不计入上文的 99 个功能区命令图标。底栏继续按 `FsQrk` 使用 Pen 资源；本 TODO 中“改用 Excel 图标方案”的范围仅指 ribbon 命令图标。

## Excel 共享图标方案 TODO（代码已实现，CUA 视觉验收待完成）

- [x] 对 Word ribbon 的每个操作使用 Excel 共享 `toolbarIconData` / `tintedToolbarIcon` 图集和配色。
- [x] Home 当前由 Pen 单张整行图像 `VSCUo` 提供图标；已改为 27 个独立图标绘制，保留当前按钮位置、命中区域、分组、无障碍标签和分隔线。
- [x] 其他 Pen 标签已改用 Excel 图集；保留每个 action 的原始标签和语义描述。Word 专属 glyph 使用 `internal/desktopui/word_ribbon_icons.go` 中的 Excel 图集映射。
- [x] 对 Excel 图集缺少的 glyph 使用已有近似图形映射；Word ribbon 不再混用 Pen PNG 和 Excel sprite。
- [x] 沿用 Excel 的正常态和禁用态着色；保留原有 hover/active 命中与背景行为，图标替换没有改变 action 可用性。
- [x] 检查 Pen PNG 引用后保留状态栏仍使用的资源；移除了 ribbon 专用的 Pen action 图标映射。
- [ ] 使用 Computer Use 在 1440×900 基准尺寸逐标签对照 Word/Excel，并在窄窗口检查滚动和缩放；保存对照截图或记录视觉问题并迭代。Pen 参考画布可访问，`getState()` 能看到 Glance Office 正在运行；但 `getApp("Glance Office")`、bundle ID `local.codex.glanceoffice` 和本地构建路径 `/private/tmp/glance-go-desktop` 均返回 `Invalid app`。直接启动该二进制报告 LaunchServices/Hiservices 连接错误；将它放进最小 `.app` 后 `open -a` 返回 `kLSNoExecutableErr`，Computer Use 仍未枚举出 QA 窗口。后续需通过完整且可被 LaunchServices 注册的 `.app` bundle 启动，并验证 CUA 可绑定；或修复 CUA 对该本地窗口的绑定，再做视觉验收。

代码侧已完成图集迁移。待 CUA 能绑定 Glance Office 窗口后，再完成基准尺寸及窄窗口的视觉验收。

此项在当前栈内可完成：Excel 已有 96×96 图集裁切、图标着色和缓存函数。风险主要是图标语义映射、Home 位图拆分后的命中区域和小尺寸下的视觉差异，不需要更换 UI 框架。

## Word 分页与输出 TODO（2026-10-02）

产品约束：独立桌面软件，不启动或依赖 LibreOffice/外部 Office。启动验收命令为 `go run ./cmd/desktop`。目标是应用屏幕、导出的 PDF 和实际打印复用同一套页面几何；不重新换行或重新分页。任何不支持的 DOCX 结构都要显示具体诊断，不能悄悄近似后仍标为“完全兼容”。

| 阶段 | 状态 | 已完成 | 未完成与验收项 |
|---|---|---|---|
| 1. DOCX 分页输入投影 | ✅ 完成 | 以中立 DTO 读取页面尺寸/方向/边距、样式继承输入、段落间距/行高/缩进/分页控制、表格网格和列宽；只读投影不改 DOCX 包；已覆盖多节、分栏、特殊缩进、合并/嵌套表格等定位诊断。 | 后续只在具体布局规则开始支持时补保存重开与 Word 样例证据；投影本身不等于算页数。 |
| 2. 字形 shaping 与字体 | ✅ 完成（接口与探针） | 接入 Go-text HarfBuzz shaping 和系统字体 fallback，输出实际 glyph ID、字体键、Unicode cluster 与点单位 advance/offset；混合简中/拉丁缺字回归通过。选择并固定纯 Go PDF 矢量/图片后端版本，完成简中可见、ToUnicode/复制提取、透明 PNG 和 Poppler 打开/渲染探针。 | 还没有生产 PDF 导出。正式输出需按具体字体的 `OS/2.fsType` 检查嵌入许可，并在 Task 5 验证系统回退字体和独立文本提取。 |
| 3. 共享分页引擎 | ⬜ 未开始 | — | 依据 Task 1 元数据、WordBlock 和 Task 2 shaper 实现点单位分页、run 格式、段落/列表/图片、显式分页符、常见表格分页和 source map；覆盖页边界、超高表格行、取消、页数及定位诊断。 |
| 4. 打印布局界面 | ⬜ 未开始 | — | 后台按文档 ID/版本/页面设置键计算布局；页面视图按原始 PageScene 画字形/图片/表格；页数只显示真实结果；补缩放、状态栏、导航、脏版本防过期结果和 Pen Node `Clpgm`/`FsQrk` 对照。 |
| 5. PDF 导出 | ⬜ 未开始 | 已确定可用技术路径：页面矢量对象按共享场景落点绘制；整形 glyph 轮廓绘为 PDF vector path；Unicode 隐形文字层只负责搜索/复制，不参与可见排版。 | 实现后台导出、准确页框/对象矩形/字体 fallback、字体许可检查、Unicode 独立提取、PDF reopen、取消/失败原子替换；导出不得改变 DOCX dirty state。 |
| 6. 原生打印 | ⬜ 未开始 | — | 以 Task 5 已生成的内存 PDF 调系统原生打印面板；macOS 评估 PDFKit/AppKit 主线程桥接；Windows/Linux 各有独立 adapter。验证取消/确认、页数/范围/份数与 Gio 窗口响应；禁止启动外部 Office/PDF/打印 CLI。 |
| 7. 统一验收 | ⬜ 未开始 | — | 运行目标回归和 `go run ./cmd/desktop`；用 Computer Use 在 1440×900 按 Pen `Clpgm`（正文）、`kiBim`（主菜单）和 `FsQrk`（底栏）截图逐项对照；比较屏幕/PDF/打印尺寸、页数和对象坐标，记录 fixture、字体、诊断、截图、打印设备及残留差异。未达到像素级证据前不标记 1:1 通过。 |

### 当前技术栈边界与解决方案

| 边界 | 原因 | 解决方案与当前状态 |
|---|---|---|
| PDF 库的普通文字 API 不接受 HarfBuzz glyph ID/精确定位 | 它会按 Unicode 和自身 kerning 编码；若把分页好的文字再交给它排版，打印版就会与屏幕不一致。 | 已探针验证将 Go-text glyph outline 转成矢量 path，按共享页面场景原点绘制；再加 alpha=0 的 Unicode/ToUnicode 文本层供提取。屏幕和 PDF 必须重放同一 outline 几何。集成尚未实现。 |
| 任意 DOCX 无法承诺 Microsoft Word 像素级一致 | 当前适配器只读取定义的分页输入；分节、分栏、浮动对象、域、修订、字体度量和复杂表格等语义并未全部建模。 | 逐个扩展 neutral 模型并用 Word/LibreOffice 不作运行时依赖的离线样例进行对照；未支持且影响页数的情况给出源位置错误并阻止 PDF/打印。当前多节/分栏及复杂表格边界已报告，完整分页未实现。 |
| 可见字形路径和可搜索文字层依赖实际字体 | 系统 fallback 字体各平台不同；PDF 嵌入许可由字体文件决定，不能假设所有系统字体均可嵌入。 | 不随程序捆绑系统字体；导出前读取字体嵌入标志，嵌入被禁止时报告诊断或以后实现不嵌入字体的 `ActualText` 文字层。当前只测得系统 Arial Unicode 样例 `fsType=0x0008`，其他 fallback 需逐一验证。 |
| 原生打印不是 Go UI 的通用 API | 面板、主线程要求和打印对象在 macOS、Windows、Linux 不同。 | 每个平台独立适配器消费同一份 PDF 字节；没通过面板、取消、确认及虚拟打印机测试的平台保持 disabled。禁止调用外部 LibreOffice/PDF reader。 |
| Linux GUI 跨平台编译暂不能在当前 macOS 环境确认 | Gio 的 Vulkan/CGo 源文件在本机 `CGO_ENABLED=0` Linux cross compile 中不参与构建。 | 纯 Go shaper/PDF 路径已在 macOS、Windows、Linux 三目标交叉编译；Gio Linux 桌面包需后续在 Linux 原生 runner/toolchain 验证，不能用纯 Go 探针结果替代。 |

## 当前技术边界、原因与可选方案

这里的“当前无法完成”指当前实现和现有能力接口无法可靠交付，不表示 Gio/Go 永远做不到。需要扩展接口、格式适配器或引入外部组件的项目，在相应能力通过保存重开验证前都应保持禁用。

| 能力 | 当前边界与原因 | 可行方案 | 建议当前处理 |
|---|---|---|---|
| Word 分页、打印布局、单页/多页与总页数 | 正文仍使用 Gio 连续流，尚未分页或绘制 PageScene。Stage 1 已投影页面/样式/表格输入；Stage 2 已实现点单位 Go-text 字形 shaping 与 CJK fallback。产品约束：不得依赖 LibreOffice 或外部 Office。 | 由应用内 Go 布局服务生成共享矢量页面场景；UI、PDF、打印复用 glyph outline、页面坐标和对象矩形。已选纯 Go `SalvioniDigitalSolutions/gopdf` 做路径/图像输出；普通 Text API 会自行编码/kerning，不能用于可见文字。方案和阶段进度见 `docs/superpowers/plans/2026-10-02-word-pagination-pdf-print.md` 与本 TODO；完整导出与原生打印尚未实现。 | 分页引擎完成前不猜页数，不用连续流截图冒充打印版。未支持且影响页数的内容须定位并阻止 PDF/打印；字体嵌入许可必须检查。该方案不承诺任意 DOCX 与 Microsoft Word 像素级一致。 |
| 页眉、页脚、页码、节设置 | 当前投影/编辑能力主要针对正文段落、表格、run 与行内图片；没有 section/header/footer part 的 neutral 编辑接口。页码字段的实际页数依赖分页。 | 扩展 section 与 header/footer DTO、DOCX relationship/part 读写和保存重开用例。先可支持固定内容页眉页脚；动态页码与总页数等待分页引擎。 | 固定页眉页脚可作为 DOCX 适配器扩展；动态页码归分页技术决策。 |
| 字体、字号、颜色高亮、段距、主题等样式写入 | 字号、字体族和高亮已通过 run 级格式能力局部写入；高亮支持 Word 标准命名颜色，颜色选择面板可清除高亮，但不支持主题色或任意 RGB 高亮。保存重开与未修改 OOXML 部件保留有用例。无显式字号时目前没有完整的 Word style/theme 继承解析，ribbon 以 11 pt 作为增减起点。 | 为段距、主题等剩余属性扩展 neutral patch 和 `word/document.xml` / styles/theme 的局部更新；后续可加入 style/theme 继承解析。保留未改 OOXML 属性，使用临时副本做保存重开与 untouched-content 检查。 | 字体族、字号增减和标准高亮色已可用；继承字号解析和其他样式属性继续分项实现。 |
| 表格、链接、书签、列表写入 | 表格可读取/显示，但尚无通用结构创建接口；超链接、书签需要关系或唯一 ID。项目符号、十进制编号和嵌套层级 0–8 已支持 `numbering.xml` 局部写入。其他编号格式、`lvlOverride` 自定义 restart，以及样式继承编号仍只读。 | 表格、链接、书签继续扩展 neutral Word 编辑能力与 DOCX XML/关系写入。后续可加入字母/罗马数字格式和覆盖重启解析，再验证 Word 重开及连续性。 | 生成或识别出的常见项目符号/十进制层级可用；无法安全理解的编号定义不允许改写。 |
| 段落缩进与列表层级 | Home 增加/减少缩进对列表修改 `ilvl`，对普通段落按 0.5 英寸步进调整左缩进；右缩进保持原值，普通段落减少时不低于 0，已有 `firstLine`/`hanging` 属性保留。列表按钮只允许移动到 `numbering.xml` 中已定义的同类层级。 | 扩展特殊缩进和复杂编号层级仍需模型及 XML patch；继续用保存重开验证右缩进、`firstLine`/`hanging` 与编号连续性。 | 常见多级列表和普通左缩进已可用；未识别的编号格式及自定义 restart 保持只读。 |
| 查找/替换范围 | 查找和替换都按可编辑文本 run 逐项处理；Replace 使用 Unicode 字符偏移，只支持同一 run 内当前选中的匹配项。neutral 模型把 run、图片和未识别内容作为有序独立 inline。 | 若产品要求跨 run，先定义混合格式替换的继承规则（建议使用匹配范围首字符的 run 格式），实现 DOCX 适配器的多 run 拆分/合并及 neutral range 操作；图片、只读字段、段落/单元格边界继续作为硬边界。对复杂字段和原始 OOXML 属性增加保存重开及 untouched-part 用例。 | 当前可用范围保持启用；跨 run 替换不是 Gio 限制，可通过扩展 neutral 接口和 DOCX 格式适配器完成，策略确定前不启用。 |
| 形状、图表、文本框、艺术字、公式 | 当前编辑模型主要支持文字和行内栅格图片；未知 DrawingML/OMML 内容不能作为可编辑对象安全投影。 | 增加对象 DTO、DrawingML/OMML 读取写入、部件关系、预览和兼容性矩阵；优先选择一个对象类型做端到端切片。 | 不是简单的图标接线工作；单独立项，未完成前保持只读/禁用。 |
| 批注、修订、接受/拒绝 | 缺少 comment、revision 的对象模型和包内关系；直接改 XML 易丢作者、时间、范围或修订语义。 | 建立 comments/revisions neutral 模型和受限操作；先选定支持的 DOCX 子集，验证 Word 重开及未处理修订保留。 | 高兼容性风险；作为独立 DOCX 适配器项目，不能仅用普通文本差异模拟。 |
| 语音听写、翻译 | Gio/Go 当前没有语音识别或翻译服务。 | 增加平台语音适配器或有明确隐私/网络授权的服务接口；需定义离线/失败反馈和语言选择。 | 作为可选外部能力，不作为当前 ribbon 的内建能力；未接入前禁用。 |
| 加载项、设计器、宏 | 当前没有 Office Add-ins 宿主、Designer 服务或 VBA 宏安全沙箱。宏执行会越出普通文档编辑的安全边界。 | 加载项需设计受限插件协议；Designer 可定义本地规则或服务；宏优先只做安全识别/提示，执行只能经独立、明确授权的隔离方案。 | 不在当前 MVP 内；宏执行保持禁用。 |
| 文件打印、PDF、系统分享 | 新建、打开、最近、保存、另存为和按编辑自动保存已接入工作区；PDF/打印需要应用内分页和平台打印服务，分享需要平台集成。 | 打印/PDF 复用应用内 Go 分页结果；打印调用系统原生 API，不启动外部 Office/PDF 应用。分享走操作系统分享面板或导出文件。 | Open/Save As 等已复用工作区功能；进程内 Go PDF 矢量输出路径已探针通过，但分页、生产导出和各平台原生打印适配器尚未实现。 |
| 文档信息与核心属性 | 当前 neutral 能力只开放标题、主题、作者、关键词、说明五个通用核心属性；自定义属性、扩展命名空间属性及带嵌套内容的目标属性没有编辑模型，直接覆盖可能破坏语义。 | 扩展 neutral 属性模型和 DOCX 属性映射，并定义未知/多值字段的编辑策略；每种字段以最小 XML patch 验证保存重开和未知属性保留。 | 五个常用字段已在当前栈实现；其余属性保持原样，不在表单内改写。 |

## 建议执行顺序

### P0：让界面真实且图标统一

- [x] 修正底栏静态“第 1 页”，并区分状态显示和不可交互图形。
- [x] Word ribbon 当前绘制路径使用 Excel 共享 sprite/tint；Word action 到图集的映射覆盖由 `TestWordToolbarExposesPenReferenceCategories` 检查。
- [x] 代码检查确认未实现操作由 capability allowlist 禁用，绘制时以 disabled 色渲染且没有 Clickable 事件入口。
- [ ] 在 Computer Use 视觉对照中复核 Excel alias 图标的具体语义是否仍易于辨认。
- [ ] 完成 Computer Use 下的 Word/Excel 同尺寸对照；确认 Word 布局与 Pen 分组仍对齐。

### P1：优先补齐普通编辑高频功能

- [x] 格式刷：复制同一 DOCX 来源 run 的支持文字属性及段落样式、列表、对齐和缩进到下一目标 run/段落；作为一个撤销操作写入，保留目标的未支持 OOXML 属性。
- [x] 字号增减：1 pt 步进，1–1638 pt；没有显式字号时暂从 11 pt 起步。
- [x] 字体族：对所选 run 统一设置西文、东亚及复杂文字字体，支持带 XML 特殊字符的字体名称。
- [x] 高亮：支持 Word 标准命名色，连续流预览根据每行文字宽度绘制背景；不支持主题色和任意 RGB 高亮。
- [x] 增加/减少缩进：普通段落左缩进按 0.5 英寸步进；列表段落通过安全的 `ilvl` 层级增减，层级 0–8。
- [x] 查找：在单个可编辑 run 内不区分大小写地查找，滚动并选中命中项；不跨 run 和只读字段。
- [ ] 选择；字数详情已接通。其他编号格式、自定义 restart/override、段前段后仍待实现。
- [x] 分页符：在当前正文 caret 插入原生 DOCX 分页符；连续流视图仅显示标记，分页布局仍依赖分页引擎。
- [ ] 表格、链接、书签、空白页、符号。
- [x] 新建/打开/最近/另存为接入现有 workspace 流程；自动保存按文档开关并在提交编辑后异步写源文件；另存为取消及最近页切换均保留当前会话。

### P2：DOCX 页面属性与设计

- [ ] 页边距、方向、纸张大小、分栏、页面颜色/边框、水印、主题与段落间距。
- [ ] 在每个功能通过 DOCX 保存、重开和未修改内容保留验证后再启用。

### P3：复杂 OOXML 与外部能力

- [ ] 形状/图表/文本框/艺术字/公式。
- [ ] 批注、修订、比较、保护编辑。
- [ ] 语音、翻译、加载项、Designer。
- [ ] 实现应用内 Go 分页、PDF 和原生打印路径；不得引入 LibreOffice/外部 Office 运行时。该能力会影响页眉页脚和多个 View 功能。

## 验收标准

- [ ] 99 个 Pen 常驻功能区命令逐项有明确状态：当前 32 项已接通，67 项待实现；已接通项须经过验证，未实现项保持不可操作，不留“看起来可点、实际无效”的按钮。
- [ ] Word ribbon icon 使用 Excel 的共享图集和着色方式；正常、禁用、hover/active 状态与 Excel 一致。
- [ ] Pen 的功能区标签、组名、分组顺序、图标语义和控件位置在 1440×900 对照尺寸保持；窄窗口可滚动且控件命中位置正确。
- [ ] 保存类编辑先在临时 DOCX 上验证保存/重开；失败不丢 dirty 状态，未支持的 OOXML 内容不得静默丢弃。
- [ ] 底栏 `FsQrk` 结构与位置保留；页面/缩放状态仅显示真实数据。没有分页引擎前不显示静态页码或总页数。
- [ ] 最终使用 Computer Use 实际并排检查 Pen 参考、Word 页面和 Excel ribbon；截图或问题清单记录在验收结果中。

## 代码与状态依据

- `internal/desktopui/word_toolbar.go`：功能区 action 清单、能力启用判断、事件处理、查找与替换。
- `internal/desktopui/dialog.go`：单/双输入模态对话框。
- `internal/desktopui/word_ribbon_layout.go`：Word 功能区绘制、Pen 参考位置/分组及交互区域；命令 glyph 使用 Excel 图集。
- `internal/desktopui/word_ribbon_icons.go`：Word action 到 Excel 共享图集的 glyph 映射及着色调用。
- `internal/desktopui/word_pen_assets.go`：状态栏仍使用的 Pen PNG 嵌入资源。
- `internal/desktopui/ribbon_chrome.go`：Excel 96×96 图标图集裁切、着色和共享图标按钮。
- `internal/document/word.go`：Word neutral model 和文本/文本范围/段落/图片/历史能力接口。
- `internal/format/docx/word_properties.go`、`internal/desktopui/word_document_info.go`：五个核心属性的 DOCX 局部读写、包关系/内容类型处理及异步文档信息表单。
- `internal/format/docx/word_text.go`、`internal/format/docx/word_list.go`、`internal/format/docx/word_images.go`：当前 DOCX 文本样式、单 run 文本范围、列表和图片写入能力。
- `internal/format/docx/word_format_painter.go`：格式刷对所支持 run/paragraph 属性的单次、可撤销 XML patch。
- `internal/desktopui/word_status_bar.go`：`FsQrk` 状态栏；静态页码已改为真实的“连续视图”状态，视图图标、缩放滑块和百分比仍为静态绘制。
- `docs/office-support.md`：DOCX 当前支持范围、连续流边界和保存验证状态。

当前轮静态验收：Pen 中已放大检查 `Word Document Editor` 1440×900 画板及 `FsQrk` 底栏。底栏参考为“第 1 页，共 3 页”、582 个字、中文(中国)、辅助功能状态；右侧含焦点/打印/Web 视图和缩放滑块。主尺寸与实现常量一致（1440×900 画布，42/34 dp 标签条，90 dp ribbon，33 dp 标尺，900×600 dp 纸张和 34 dp 底栏）；修正了顶部文档标签由 224 dp 改为 190 dp、标签间距由 12 dp 改为 6 dp，并修正加号菜单锚点。几何用例覆盖标签尺寸、槽位间距和菜单 x=526 dp。另从 Pen ribbon 自动布局计算了七个常驻标签的命令图标横坐标与组分割线，并用 `TestWordRibbonActionSlotsMatchPenNodeClpgm` 锁定这些坐标。DOCX 全包测试、Word 查找/替换、格式刷、列表、嵌套层级、缩进工具栏、文档信息、分页符及标记布局定向测试通过；桌面构建通过。分页符用例覆盖 Unicode caret/选区替换、原 run 格式保留、native `w:br` 投影、保存重开、撤销/重做及工具栏草稿提交和 caret 回位；表格段落不开放。文档信息用例覆盖五个核心属性的转义、保存重开、未知元数据与其他包部件保留、缺失/孤立部件关系修复、撤销/重做，以及重复或嵌套目标值拒绝写入；UI 用例验证 Ribbon 异步打开表单并提交。替换用例验证大小写不敏感地替换当前选中范围、保留 run 格式、跳转下一命中项、保持对话框打开及保存重开。替换范围以 Unicode 字符为单位，错误边界不会修改文档。格式刷用例验证支持的 run/paragraph 属性复制、目标文本及未支持属性保留、一个 Undo/Redo 步骤和 DOCX 保存重开。列表用例覆盖多级项目符号/编号、父级变更后的子级重启、撤销/重做、保存重开、包内其他文件字节保留，以及自定义 restart 只读保护。缩进用例验证普通段落 0.5 英寸步进、列表 `ilvl` 变化、右缩进保留和保存重开；查找用例验证草稿、大小写不敏感、跨 run 遍历、命中选区和循环回绕。字数统计面板用例覆盖未提交文字草稿、表格文字和段落/表格计数；文件 ribbon 用例覆盖新建草稿、Open/Save As 取消、最近页切换保留会话，以及自动保存的 DOCX 重开与开关恢复。完整 `internal/desktopui` 测试仍失败于 4 个 Excel/Grid 用例：单元格字体回退、固定表头缩放手柄、可见网格选择框几何、设备像素缩放。Computer Use 当前可访问 Pen，但 Glance Office 绑定持续返回 `Invalid app`；为继续 1:1 对照已请求用户确认把仓库构建的 QA 包安装到 `~/Applications` 并启动。
