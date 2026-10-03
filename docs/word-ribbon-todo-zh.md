# Word 主页面与功能区 TODO（中文）

更新日期：2026-10-03
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
| 文件 | 12 | 10 | 2 |
| 开始 | 27 | 22 | 5 |
| 插入 | 16 | 2 | 14 |
| 设计 | 9 | 0 | 9 |
| 布局 | 12 | 0 | 12 |
| 审阅 | 11 | 1 | 10 |
| 视图 | 12 | 9 | 3 |
| **常驻功能区合计** | **99** | **44** | **55** |
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

### 已接通的 44 个常驻功能区图标

- [x] 文件：新建、打开、最近使用、保存、另存为、自动保存、打印、导出 PDF、共享副本导出、文档信息。
- [x] 开始：粘贴、剪切、复制、格式刷、字体族、减小字号、增大字号、加粗、斜体、下划线、高亮、字体颜色、项目符号、编号、左对齐、居中、右对齐、两端对齐、减少缩进、增加缩进、查找、替换。
- [x] 插入：图片、分页符。
- [x] 审阅：字数统计。
- [x] 视图：阅读模式、打印布局、Web 版式、缩放、标尺、网格线、导航窗格、单页、多页。

### 已接通的 5 个图片上下文操作

- [x] 替换图片。
- [x] 删除图片。
- [x] 保持宽高比调整图片宽度。
- [x] 向左旋转 90°。
- [x] 向右旋转 90°。

这些图片操作经过 DOCX 图片能力接口调用，不应与七个常驻标签的 99 项混算。

## 按图标逐项 TODO

状态说明：`[x]` 表示该功能区图标已有用户可触发的操作路径；`[ ]` 表示图标虽已定义或绘制，但当前没有对应的 Word ribbon 行为。若现有其他位置提供了相近操作，会在条目中说明；这仍不算该图标已接通。

### 文件（12 项：已完成 10，待实现 2）

- [x] 新建 `new`：创建空白 DOCX 草稿并切换到新文档标签；保留已有文档会话。
- [x] 打开 `open`：接入工作区异步 DOCX/XLSX/JSON 文件选择器和现有打开流程；取消不改变当前文档。
- [x] 最近使用 `recent`：切换至工作区“最近”页面；保留当前文档会话。
- [x] 保存 `save`：保存当前 DOCX；等待待提交文字草稿后执行。
- [x] 另存为 `save-as`：接入工作区已有的 DOCX Save As 流程；取消时保留原文档、路径和脏状态。
- [x] 自动保存 `autosave`：按文档保存开关状态；成功提交 Word 编辑后后台保存。无路径草稿先由工作区快照恢复保护，另存后自动保存源 DOCX；保存失败使用错误状态且保留脏状态，关闭仍按未保存文档流程确认。重启恢复开关状态有用例。
- [x] 打印 `print`：macOS 使用 PDFKit/AppKit 原生打印面板，复用共享分页场景生成的 PDF；Computer Use 已验证 1 页/1 份和取消返回。当前没有选中打印机，物理输出仍未验证；Windows/Linux/no-cgo 禁用。没有调用外部 Office、PDF 阅读器或打印命令。
- [x] 导出 PDF `export-pdf`：Word File ribbon 可触发原生保存对话框；后台重放共享分页场景并原子写入 PDF，页面内矢量字形和图片沿用同一坐标，隐藏 Unicode 层供搜索/复制。
- [x] 共享 `share`：提交所有正文草稿后打开 DOCX 保存位置选择器，将当前一致版本导出为本地副本；同源路径拒绝覆盖，临时文件完整写入并在替换目标前复核文档版本。保留活动文档路径、脏状态、自动保存开关和未识别 DOCX 包部件。保存失败或选择期间文档过期时不替换目标。此实现不调用云端或平台分享服务。
- [x] 文档信息 `doc-info`：异步读取并编辑标题、主题、作者、关键词、说明五个 DOCX 核心属性；保存重开、XML 特殊字符、未知元数据保留、撤销/重做均有适配器回归。缺失核心属性时补齐部件、包根关系和内容类型；重复、嵌套或含非文本标记的目标属性拒绝写入，避免破坏无法安全解释的元数据。
- [ ] 保护文档 `protect-document`（已确认范围：打开密码/文件级加密；本轮暂缓）：当前 DOCX 适配器和会话只处理普通 ZIP OOXML。Excelize v2.11.0 的 `Encrypt` 写 Standard encryption（AES-ECB、SHA-1 派生、无包级完整性 HMAC）；其 Agile `Decrypt` 路径没有校验 Agile 密码 verifier 或 `dataIntegrity` HMAC，不能用于安全的打开/保存链路。另发现纯 Go 候选 `github.com/mgilbir/spine` v0.3.0，声称写 AES-256-CBC/SHA-512 Agile 并严格验证 HMAC；它仍为 pre-1.0，尚未完成独立代码审查和 Office 互操作验证，暂不引入。更关键的是，当前工作区恢复快照把 DOCX 明文包存入 SQLite；只给磁盘文件加密会把明文留在恢复数据库。完整实现需让打开、保存/另存为、自动保存、共享副本和工作区快照统一保留加密，工作区重启恢复时重新要求口令，并以 tamper/wrong-password/save-reopen/恢复测试验证。在加密适配器与恢复生命周期方案完成评估前保持命令不可用。`w:documentProtection` 是另一项“审阅 > 限制编辑”能力，只用于防误改，不构成保密边界。依据：[MS-OFFCRYPTO 概览](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-offcrypto/2d1c4f2c-7a2a-4133-998f-f3373f00d542)、[Agile 加密](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-offcrypto/9e61da63-8ddb-4c0a-b25d-f85d990f44c8)、[Agile HMAC 完整性](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-offcrypto/63d9c262-82b9-4fa3-a06d-d087b93e3b00)、[Excelize v2.11.0 加密实现](https://github.com/qax-os/excelize/blob/v2.11.0/crypt.go)、[Spine 加密接口](https://pkg.go.dev/github.com/mgilbir/spine@v0.3.0/common/crypto)。
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

### 审阅（11 项：已完成 1，待实现 10）

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

### 视图（12 项：已完成 9，待实现 3）

- [x] 阅读模式 `view-reading`：保留进入前的连续/打印模式、滚动位置、缩放与状态栏；隐藏 Ribbon、标尺和网格线，将正文/图片编辑交互设为只读；顶部“退出阅读模式”按钮恢复原视图状态，不写入 DOCX。
- [x] 打印布局 `view-print-layout`：Word ribbon 和底栏开关均可切换到共享 `PageScene` 页面视图；遇到阻断诊断时明确显示错误，不回退成连续流假装打印版。页面内文字仍为只读场景，需切回连续视图编辑正文。
- [x] Web 版式 `view-web-layout`：Ribbon 与 `FsQrk` 底栏按钮切换同一会话状态；白色连续画布铺满窗口，正文列随窗口宽度重排并在宽屏时居中限制为 1200 dp。隐藏纸张边缘、分页阴影、纸张宽度拖拽柄和页面标尺。再次点击回到普通连续视图；切换不修改 DOCX 脏状态，打印布局仍进入共享分页页面场景。
- [x] 标尺 `view-ruler`：默认显示横竖标尺；Ribbon 开关可同时隐藏/恢复两者，恢复后标尺拖动与缩进功能继续可用。
- [x] 网格线 `view-gridlines`：默认隐藏；启用后在连续流纸张和打印布局页面绘制淡色 24 dp 网格，仅影响屏幕视图且不修改 DOCX 内容或导出/打印的 PageScene。
- [x] 导航窗格 `view-navigation`：在正文左侧打开 300 dp 面板；自动识别内置 Heading 1–9 及通过 `basedOn` 继承标题级别的自定义样式，显示标题层级；实时搜索正文和表格中可编辑文字 run（含未提交草稿），显示全部命中数量和摘要，最多列出前 500 项，点击标题/结果可滚动定位；在连续/Web 版式中会选择对应正文范围，在打印布局中会定位到对应页。索引按文档版本、查询和草稿 epoch 复用，缓存命中不遍历编辑器。关闭或打开窗格不修改 DOCX。搜索按单个可编辑 run 匹配，不跨 run/段落，也跳过只读域结果；嵌套表格不在当前正文投影范围内。标题目前依据段落样式链，尚未投影独立的 outline-level 元数据。大文档每次查询变化仍需在 UI 线程扫描并计数全部命中；如实测有卡顿，可将查询移入可取消的后台任务，并用文档版本、草稿 epoch 与 query 丢弃过期结果。
- [x] 单页 `view-single-page`：切入共享 `PageScene` 打印布局，按当前页尺寸和扣除标尺/面板后的可用视口动态缩放，尽量完整显示一页；窗口尺寸或当前页改变时重新适配。手动调整底栏缩放会退出适配模式并沿用 50%–150% 控件范围。仅影响视图状态，不修改 DOCX；如果视口要求低于 50%，页面仍可滚动。
- [x] 多页 `view-multiple-pages`：切入共享 `PageScene` 打印布局，将页面按两页一行并排显示并纵向滚动；自动缩放时同时考虑两页宽度与视口高度，奇数页末尾保留单页居中。切换到手动缩放时返回普通单页滚动。仅影响视图状态，不修改 DOCX；缩放下限为 50%。
- [x] 缩放 `view-zoom`：打开百分比对话框，接受 50–150 的整数百分比，并更新打印布局页面场景共用的会话缩放值；拒绝越界/非整数输入并允许在对话框内修正。底栏缩小/放大按 10% 调整，滑块可拖动，三种入口共同使用 50%–150% 范围。
- [ ] 新建窗口 `view-new-window`：代码与自动化回归已实现。多个窗口共享同一 Word 文档会话、编辑和脏状态；每窗独立维护光标、滚动、缩放和视图，主窗口退出后由最早创建的存活窗口接管工作区持久化。单元/生命周期/`-race` 回归通过。2026-10-03 macOS CUA 打开本地 DOCX 并切到“视图”功能区后，点击“新建窗口”后的窗口枚举/后续交互返回 `noWindowsAvailable`，未能确认第二窗口内容、跨窗编辑和关窗行为；macOS/Windows/Linux 原生路径均暂不标记已验收，所以视图仍按 9/12、常驻功能区仍按 44/99 统计。规格见 `docs/superpowers/specs/2026-10-03-word-new-window-design.md`，计划见 `docs/superpowers/plans/2026-10-03-word-new-window.md`。
- [ ] 并排查看 `view-side-by-side`：需要双视口、独立滚动/选择状态和窗口宽度策略。
- [ ] 宏 `view-macros`：当前无 VBA 宏读取/编辑/执行能力；禁止在应用内不受控执行宏。

## 主页面底部 `FsQrk` 的未完成状态

Pen `Clpgm` 的底栏内容已核对：左侧依次显示“第 1 页，共 3 页”、`582 个字`、`中文(中国)`、辅助功能图标和“辅助功能：正常”；右侧依次是焦点模式、打印布局、Web 版式、缩小、缩放滑块、放大和 `100%`。底栏的**区域与图形已绘制**，但绘制出来不代表控件已有功能。

- [x] 底栏位置、尺寸、字数区域及左右侧图形已画出。
- [x] 移除静态页数文案；当前底栏使用 rune source map / 首个可见页面和接受的分页结果显示真实页码/总页数，未完成时显示“正在计算页数…”或“页数未计算”，失败/不支持时明确显示状态。
- [x] 打印布局和 Web 版式图标、缩小/放大可交互；缩放百分比和滑块位置读取会话缩放值。
- [x] 底栏缩放滑块可按指针位置拖动，同一会话缩放值在 50%–150% 范围夹紧并立即驱动页面场景；± 按钮仍每次调整 10%。回归覆盖 1×/2× 像素密度、窄视口右锚和端点。
- [x] 单页和双页并排视图已接通；更高列数布局尚未提供。
- [x] Ribbon 缩放百分比对话框已接通；它与底栏滑块/±按钮更新同一会话缩放值。
- [ ] 辅助功能图标当前是静态状态图形；若设计要求打开辅助功能面板，再增加有实际内容的操作。

底栏的 6 个图形图标和缩放滑块不计入上文的 99 个功能区命令图标。底栏继续按 `FsQrk` 使用 Pen 资源；本 TODO 中“改用 Excel 图标方案”的范围仅指 ribbon 命令图标。

## Excel 共享图标方案 TODO（代码已实现，基准几何已用 CUA 对照，最终像素验收待完成）

- [x] 对 Word ribbon 的每个操作使用 Excel 共享 `toolbarIconData` / `tintedToolbarIcon` 图集和配色。
- [x] Home 当前由 Pen 单张整行图像 `VSCUo` 提供图标；已改为 27 个独立图标绘制，保留当前按钮位置、命中区域、分组、无障碍标签和分隔线。
- [x] 其他 Pen 标签已改用 Excel 图集；保留每个 action 的原始标签和语义描述。Word 专属 glyph 使用 `internal/desktopui/word_ribbon_icons.go` 中的 Excel 图集映射。
- [x] 对 Excel 图集缺少的 glyph 使用已有近似图形映射；Word ribbon 不再混用 Pen PNG 和 Excel sprite。
- [x] 沿用 Excel 的正常态和禁用态着色；保留原有 hover/active 命中与背景行为，图标替换没有改变 action 可用性。
- [x] 检查 Pen PNG 引用后保留状态栏仍使用的资源；移除了 ribbon 专用的 Pen action 图标映射。
- [x] 用 Computer Use 绑定当前分支的 Go 构建并查看 Word ribbon、纸张画布及 `FsQrk` 底栏。原始 `go run ./cmd/desktop` 进程启动成功但 CUA 无法按进程名绑定；将同一源码构建包装为临时 macOS app 后，CUA 绑定到窗口标题 `desktop`。另外用 Pencil MCP 截取 `Clpgm`、`kiBim`、`FsQrk` 节点。截图和节点树已在本轮对话中检查，没有写入仓库图片文件。
- [x] 已对照响应式几何：Pen `Clpgm` 为 1440×900 dp，顶栏/标签/功能区/标尺分别为 42/34/56/33 dp，纸张为 x=270、y=181、900×600 dp，底栏 y=866、高 34 dp。CUA 实际屏幕逻辑视口为 1280×820 dp，页面仍居中且宽 900 dp（x≈190），顶部固定高度与底栏锚点一致；垂直空间不足时页面缩至约 570 dp。对应 1440×900 坐标已有 `TestWordCanvasAndRulersMatchPenClpgmGeometry`。
- [ ] 最终 1:1 视觉验收仍未完成：从 Pen `Clpgm` 节点读取的可见正文已重建为临时 DOCX，并通过同分支 desktop 二进制的文件参数载入 CUA。修复省略 `w:orient` 被误报后，打印布局实际显示第 1 页/共 3 页、573 个字；Pen 参考是 3 页/582 个字。临时文件还含有补页文字，不能当作原始或等价 DOCX。CUA 视口为 1280×820 dp，Pen 画板为 1440×900 dp；尚无匹配视口配对截图。当前样稿第一屏的标题下蓝线、段间距、表头文字样式也未对齐，窄窗口滚动/缩放仍未逐标签检查。

代码侧已完成图集迁移。Pencil MCP 已读取并截图 `Clpgm`、`kiBim`、`FsQrk`，CUA 已实际载入重建的 DOCX 并显示三页分页结果。最终验收仍需等价样稿、匹配逻辑视口的配对截图，以及正文位置、样式和窄窗口 ribbon 滚动/缩放检查。

此项在当前栈内可完成：Excel 已有 96×96 图集裁切、图标着色和缓存函数。风险主要是图标语义映射、Home 位图拆分后的命中区域和小尺寸下的视觉差异，不需要更换 UI 框架。

## Word 分页与输出 TODO（2026-10-02）

产品约束：独立桌面软件，不启动或依赖 LibreOffice/外部 Office。启动验收命令为 `go run ./cmd/desktop`。目标是应用屏幕、导出的 PDF 和实际打印复用同一套页面几何；不重新换行或重新分页。任何不支持的 DOCX 结构都要显示具体诊断，不能悄悄近似后仍标为“完全兼容”。

| 阶段 | 状态 | 已完成 | 未完成与验收项 |
|---|---|---|---|
| 1. DOCX 分页输入投影 | ✅ 完成 | 以中立 DTO 读取页面尺寸/方向/边距、样式继承输入、段落间距/行高/缩进/分页控制、表格网格和列宽；只读投影不改 DOCX 包；已覆盖多节、分栏、特殊缩进、合并/嵌套表格等定位诊断。 | 后续只在具体布局规则开始支持时补保存重开与 Word 样例证据；投影本身不等于算页数。 |
| 2. 字形 shaping 与字体 | ✅ 完成（接口、探针和 PDF 集成） | 接入 Go-text HarfBuzz shaping 和系统字体 fallback，输出实际 glyph ID、字体键、Unicode cluster 与点单位 advance/offset；应用创建 Gio shaper 前先用可写缓存初始化进程级字体扫描器，避免默认缓存不可写时扫描器被过早失败锁定；混合简中/拉丁缺字和不可写缓存启动回归通过。纯 Go `gopdf` 后端已集成；PDF 可见字形使用页面场景中的矢量轮廓，隐藏 Unicode 层只负责搜索/复制；检查 `OS/2.fsType`、字形 cmap 和字体面索引。PDF reader 用例验证 CJK/Latin 文本提取（宿主有 Arial Unicode 时）与页面渲染。 | 当前系统字体许可不允许或缺失 `OS/2` 时会拒绝导出；TrueType 禁止子集、位图专用以及非首个 TTC 字体面也拒绝。未实现不嵌字字体的 `ActualText` 提取替代路径；其他操作系统字体 fallback 仍需各自验证。 |
| 3. 共享分页引擎 | ✅ 完成（引擎） | 新增纯 Go `internal/wordlayout`；以点为单位输出页框、正文框和页面 clip、定位文字片段/glyph 矢量轮廓、行内图片矩形、表格单元格和 run/Unicode 字符偏移→页面坐标映射。支持首个 section 几何、默认 A4、段落和字符样式继承、基础 run/段落格式、常见换行/列表/图片/分页符、段落分页控制和普通表格按行跨页。超宽行、超高表格行、未支持内容会产生定位错误并阻止输出；默认纸张/边距和估算列宽等 warning 要求后续预览确认。回归覆盖精确页边界、取消、source map、轮廓缓存、缺失表格列宽和 keep-with-next。 | 保守边界：只布局单 section；RTL、分栏、多节、合并/嵌套表格、表格内分页符会阻止输出；keep-with-next 只对连续段落分组，目标不是段落时给 warning；缺失列宽按已知列均值补齐并归一到正文宽度，同时给 warning，避免无提示地猜列宽。 |
| 4. 打印布局界面 | 🟨 已接通，视觉验收未完成 | 后台按文档 ID/版本/页面设置键启动分页；取消旧版本任务，仅接受同文档、同版本、同页面设置结果。虚拟化 Gio 页面列表重放原始 PageScene 的字形矢量轮廓、图片位置/旋转和表格边线；打印布局可从 Ribbon 或 `FsQrk` 切换；单页适配视口，多页按双页一行并支持纵向滚动；导航结果映射到实际页面，状态栏跟随当前可见页。页缩放 50%–150% 可通过 Ribbon 百分比对话框、± 按钮（每次 10%）及拖动滑块调整，三者和页面场景共用会话值。 | 页面视图里的正文目前只读，编辑请切回连续视图；页内 caret/editor overlay 与打印页内选区绘制尚未实现。图片解码完成前显示占位框。已核对 Pencil 几何并做过一次有限的 CUA 检查，但当前阶段临时 QA 应用的使用请求被 Computer Use 审批门拒绝；尚无同 DOCX、同 1440×900 视口的配对截图验收。阶段测试：分页状态/取消失效结果/模式切换、单页与多页视图、导航页定位、缩放对话框/滑块范围和坐标及 FsQrk 几何用例通过；完整 UI 包仍有 5 个桌面 UI 失败需另行处理。 |
| 5. PDF 导出 | ✅ 已接通（有限版；视觉验收未完成） | File ribbon 触发 PDF 保存框；先 flush 正文草稿、分页未就绪时后台等待对应 revision；warning 需要用户确认，blocking diagnostic 禁止输出。`internal/format/pdf` 重放页尺寸、glyph outlines、字体颜色/高亮/下划线、表格边线和 PNG/JPEG 图片矩形/旋转；隐藏文字层只供提取。字体按 OS/2 嵌入许可与 cmap 覆盖校验。导出用同目录临时文件，reopen 校验页数/页面尺寸后原子替换；revision 与草稿 epoch 在替换前再检查。回归覆盖页面/矢量可见性、CJK/Latin 提取、图片矩形、阻断诊断、取消/失败/旧版本保护，以及源 DOCX snapshot/dirty state 不变。 | `gopdf.Reader` 用于 reopen、页数/尺寸、渲染与文本提取验证；尚无独立 Poppler/PDFKit 文本提取交叉验证。macOS 原生打印面板现已接通并通过取消/页数显示检查，但本机无打印机，未提交物理输出。Pen 与 PDF 的 Computer Use 像素对照未做；字体许可拒绝、超出支持的 DOCX 结构、透明/复杂图片细节及平台 fallback 差异可能阻止输出或需要继续验证。 |
| 6. 原生打印 | 🟨 macOS 已接通；物理输出验收待完成 | `internal/printservice` 的 `Printer` 接口使用内存 PDF；macOS PDFKit/AppKit 通过 Cocoa 主队列显示原生打印面板，按原页面尺寸（不缩放、不自动旋转）输出；Word File > 打印先 flush 正文草稿、复用同 revision/page setup 的分页场景，提示布局 warning，生成与打印布局相同的 PDF 页并要求第二次确认。macOS 面板已用 Computer Use 打开，显示 1 页/1 份；取消后回到文档，DOCX dirty state 保留。Windows/Linux 与无 cgo 平台的打印入口禁用。 | 当前主机没有选中的打印机，实际 spool/纸面结果、多份和页码范围尚未验证；跨平台 adapter 未实现；Pen 节点 `Clpgm`/`kiBim`/`FsQrk` 的整体视觉 1:1 对照仍待阶段 7。禁止启动 LibreOffice、PDF 阅读器或打印 CLI。 |
| 7. 统一验收 | 🟨 CUA 已载入重建样稿；1:1 视觉验收未完成 | 通过 Pencil MCP 核对 `Clpgm`、`kiBim`、`FsQrk`，并让 CUA 载入按节点可见正文重建的临时 DOCX。发现 DOCX 合法省略 `w:orient` 时适配器曾错误阻断分页；修复后同分支构建显示第 1 页/共 3 页、573 个字。Pen 显示 3 页/582 个字。 | 重建文件含补页文字且 word count 与 Pen 相差 9，不能视作原稿。CUA 视口为 1280×820 dp，Pen 为 1440×900 dp；两边尚无等价文档和同尺寸配对截图。第一屏标题蓝线、段间距和表头样式仍有差异；需要等价样稿后再核正文/页边距、`kiBim` 图标语义、窄窗口滚动/缩放及多页滚动。 |

### 当前技术栈边界与解决方案

| 边界 | 原因 | 解决方案与当前状态 |
|---|---|---|
| PDF 库的普通文字 API 不接受 HarfBuzz glyph ID/精确定位 | 它会按 Unicode 和自身 kerning 编码；若把分页好的文字再交给它排版，打印版就会与屏幕不一致。 | 已实现 Go-text glyph outline 到 PDF vector path 的重放，并加 alpha=0 的 Unicode/ToUnicode 文字层供提取。可见页面不由 PDF Text API 再排版；writer 与 Gio 重放同一 PageScene outline、字形原点和对象框。 |
| 任意 DOCX 无法承诺 Microsoft Word 像素级一致 | 当前适配器只读取定义的分页输入；分节、分栏、浮动对象、域、修订、字体度量和复杂表格等语义并未全部建模。 | 逐个扩展 neutral 模型并用离线样例对照（LibreOffice 仅可作开发时参考，绝不作为运行时依赖）；未支持且影响页数的情况给出源位置错误并阻止 PDF/打印。当前多节/分栏及复杂表格边界已报告，完整兼容和 Pen 1:1 验收未完成。 |
| 可见字形路径和可搜索文字层依赖实际字体 | 系统 fallback 字体各平台不同；PDF 嵌入许可由字体文件决定，不能假设所有系统字体均可嵌入。 | 不随程序捆绑系统字体。导出前解析 `OS/2.fsType` 并验证选中 face 与 Unicode cmap；当前平台 Arial Unicode `fsType=0x0008` 的 mixed CJK/Latin 提取用例通过。无 OS/2、restricted、bitmap-only、TrueType no-subsetting 和 TTC 非首 face 明确失败关闭。各平台实际 fallback 字体需逐一验证；不嵌字体的 `ActualText` 仍是后续方案。 |
| 原生打印不是 Go UI 的通用 API | 面板、主线程要求和打印对象在 macOS、Windows、Linux 不同。 | macOS 已用 PDFKit/AppKit 从进程内 PDF 字节启动系统面板，并用 Computer Use 验证页数显示和取消；当前没有配置打印机，成功 spool、份数/范围和纸面尺寸还要在有虚拟/实体打印机的环境复核。Windows/Linux/no-cgo 继续 disabled，直到各自系统 adapter 验证通过。禁止调用外部 LibreOffice/PDF reader/CLI。 |
| Linux GUI 跨平台编译暂不能在当前 macOS 环境确认 | Gio 的 Vulkan/CGo 源文件在本机 `CGO_ENABLED=0` Linux cross compile 中不参与构建。 | 纯 Go shaper/PDF 路径已在 macOS、Windows、Linux 三目标交叉编译；Gio Linux 桌面包需后续在 Linux 原生 runner/toolchain 验证，不能用纯 Go 探针结果替代。 |

## 当前技术边界、原因与可选方案

这里的“当前无法完成”指当前实现和现有能力接口无法可靠交付，不表示 Gio/Go 永远做不到。需要扩展接口、格式适配器或引入外部组件的项目，在相应能力通过保存重开验证前都应保持禁用。

| 能力 | 当前边界与原因 | 可行方案 | 建议当前处理 |
|---|---|---|---|
| Word 分页、打印布局、单页/多页与总页数 | `internal/wordlayout` 为支持输入计算真实分页。Task 4 已将页面场景画到 Gio 并接入异步 revision-safe 结果及 `FsQrk` 页码；页面视图中的正文当前只读，窗口内 Pen CUA 对照未验收。Task 5 PDF writer/export 已接通；Task 6 macOS 原生打印面板已接通。不得依赖 LibreOffice 或外部 Office 运行时。 | 屏幕、PDF 和打印共用 PageScene glyph outline、clip、坐标和对象矩形；PDF 使用纯 Go `SalvioniDigitalSolutions/gopdf`，普通 Text API 不参与可见排版；macOS PDFKit/AppKit 消费同一份进程内 PDF。详情见 `docs/superpowers/plans/2026-10-02-word-pagination-pdf-print.md`。 | 支持输入可显示分页实算状态、导出 PDF 并进入 macOS 系统打印面板；blocking 内容定位后禁止输出，默认页参数/估算列宽等 warning 需确认。页内正文编辑、物理打印机输出、独立 PDF 解析器验证和 Pen 视觉对照未完成，不标记 1:1。 |
| 页眉、页脚、页码、节设置 | 当前投影/编辑能力主要针对正文段落、表格、run 与行内图片；没有 section/header/footer part 的 neutral 编辑接口。页码字段的实际页数依赖分页。 | 扩展 section 与 header/footer DTO、DOCX relationship/part 读写和保存重开用例。先可支持固定内容页眉页脚；动态页码与总页数等待分页引擎。 | 固定页眉页脚可作为 DOCX 适配器扩展；动态页码归分页技术决策。 |
| 字体、字号、颜色高亮、段距、主题等样式写入 | 字号、字体族和高亮已通过 run 级格式能力局部写入；高亮支持 Word 标准命名颜色，颜色选择面板可清除高亮，但不支持主题色或任意 RGB 高亮。保存重开与未修改 OOXML 部件保留有用例。无显式字号时目前没有完整的 Word style/theme 继承解析，ribbon 以 11 pt 作为增减起点。 | 为段距、主题等剩余属性扩展 neutral patch 和 `word/document.xml` / styles/theme 的局部更新；后续可加入 style/theme 继承解析。保留未改 OOXML 属性，使用临时副本做保存重开与 untouched-content 检查。 | 字体族、字号增减和标准高亮色已可用；继承字号解析和其他样式属性继续分项实现。 |
| 表格、链接、书签、列表写入 | 表格可读取/显示，但尚无通用结构创建接口；超链接、书签需要关系或唯一 ID。项目符号、十进制编号和嵌套层级 0–8 已支持 `numbering.xml` 局部写入。其他编号格式、`lvlOverride` 自定义 restart，以及样式继承编号仍只读。 | 表格、链接、书签继续扩展 neutral Word 编辑能力与 DOCX XML/关系写入。后续可加入字母/罗马数字格式和覆盖重启解析，再验证 Word 重开及连续性。 | 生成或识别出的常见项目符号/十进制层级可用；无法安全理解的编号定义不允许改写。 |
| 段落缩进与列表层级 | Home 增加/减少缩进对列表修改 `ilvl`，对普通段落按 0.5 英寸步进调整左缩进；右缩进保持原值，普通段落减少时不低于 0，已有 `firstLine`/`hanging` 属性保留。列表按钮只允许移动到 `numbering.xml` 中已定义的同类层级。 | 扩展特殊缩进和复杂编号层级仍需模型及 XML patch；继续用保存重开验证右缩进、`firstLine`/`hanging` 与编号连续性。 | 常见多级列表和普通左缩进已可用；未识别的编号格式及自定义 restart 保持只读。 |
| 查找/替换范围 | 查找和替换都按可编辑文本 run 逐项处理；Replace 使用 Unicode 字符偏移，只支持同一 run 内当前选中的匹配项。neutral 模型把 run、图片和未识别内容作为有序独立 inline。 | 若产品要求跨 run，先定义混合格式替换的继承规则（建议使用匹配范围首字符的 run 格式），实现 DOCX 适配器的多 run 拆分/合并及 neutral range 操作；图片、只读字段、段落/单元格边界继续作为硬边界。对复杂字段和原始 OOXML 属性增加保存重开及 untouched-part 用例。 | 当前可用范围保持启用；跨 run 替换不是 Gio 限制，可通过扩展 neutral 接口和 DOCX 格式适配器完成，策略确定前不启用。 |
| 形状、图表、文本框、艺术字、公式 | 当前编辑模型主要支持文字和行内栅格图片；未知 DrawingML/OMML 内容不能作为可编辑对象安全投影。 | 增加对象 DTO、DrawingML/OMML 读取写入、部件关系、预览和兼容性矩阵；优先选择一个对象类型做端到端切片。 | 不是简单的图标接线工作；单独立项，未完成前保持只读/禁用。 |
| 批注、修订、接受/拒绝 | 缺少 comment、revision 的对象模型和包内关系；直接改 XML 易丢作者、时间、范围或修订语义。 | 建立 comments/revisions neutral 模型和受限操作；先选定支持的 DOCX 子集，验证 Word 重开及未处理修订保留。 | 高兼容性风险；作为独立 DOCX 适配器项目，不能仅用普通文本差异模拟。 |
| 语音听写、翻译 | Gio/Go 当前没有语音识别或翻译服务。 | 增加平台语音适配器或有明确隐私/网络授权的服务接口；需定义离线/失败反馈和语言选择。 | 作为可选外部能力，不作为当前 ribbon 的内建能力；未接入前禁用。 |
| 加载项、设计器、宏 | 当前没有 Office Add-ins 宿主、Designer 服务或 VBA 宏安全沙箱。宏执行会越出普通文档编辑的安全边界。 | 加载项需设计受限插件协议；Designer 可定义本地规则或服务；宏优先只做安全识别/提示，执行只能经独立、明确授权的隔离方案。 | 不在当前 MVP 内；宏执行保持禁用。 |
| 文件打印、PDF、共享副本 | 新建、打开、最近、保存、另存为和按编辑自动保存已接入工作区；真实分页/只读打印布局、revision-safe PDF 导出、macOS 原生打印面板和本地 DOCX 共享副本导出已接入。Windows/Linux 打印及云端/平台分享服务仍未实现。 | PDF 复用应用内 Go PageScene，先确认 warning 再原子替换目标文件；macOS 打印 adapter 消费同一 PDF 字节并调用 PDFKit/AppKit，不启动外部 Office/PDF 应用。共享将当前 DOCX 快照复制到用户选择的本地路径，并在原子替换目标前检查版本。 | PDF 对声明范围可用；Computer Use 已核对 macOS 打印面板页数/取消，但当前没有选择打印机，物理输出未验证；独立解析器交叉验证与 Pen 视觉检查未做。系统分享面板和云端分享不在本地副本实现范围内。 |
| Word 密码保护与限制编辑 | 当前 DOCX 适配器处理普通 ZIP/OOXML；Word 打开密码使用 Office 文件加密容器，尚无加解密和密码输入流程。仓库依赖的 Excelize `Encrypt` 输出 Standard 模式，而 MS-OFFCRYPTO 说明该模式没有 CBC 和损坏检测，因此不能直接当作安全的文件加密能力。`w:documentProtection` 属于编辑限制，不加密文档，也不是安全功能。 | 文件级打开密码需实现/引入经评估的 Office Agile AES 加密/解密、口令派生及数据完整性验证，并覆盖新建/打开/另存/自动保存/错误口令；编辑限制需另建 neutral 保护状态，在 Gio 与所有适配器编辑路径中一致执行，验证 Word 互操作。参考 Microsoft [MS-OFFCRYPTO](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-offcrypto/2d1c4f2c-7a2a-4133-998f-f3373f00d542)、[Agile package encryption](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-offcrypto/9e61da63-8ddb-4c0a-b25d-f85d990f44c8) 和 [`documentProtection` 定义](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.documentprotection?view=openxml-3.0.1)。 | 范围确认前保持 File > Protect Document 和 Review > Restrict Editing 禁用；建议前者表示文件加密，后者表示防误编辑限制，不把限制编辑称为数据保密。 |
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
- [x] 实现应用内 Go 分页、PDF 和原生打印路径；macOS 系统打印面板已由 CUA 检查页数并取消，Windows/Linux 仍 disabled；没有配置打印机，物理输出未验证。不得引入 LibreOffice/外部 Office 运行时。该能力会影响页眉页脚和多个 View 功能。

## 验收标准

- [ ] 99 个 Pen 常驻功能区命令逐项有明确状态：当前 44 项已接通，55 项待实现；已接通项须经过验证，未实现项保持不可操作，不留“看起来可点、实际无效”的按钮。
- [ ] Word ribbon icon 使用 Excel 的共享图集和着色方式；正常、禁用、hover/active 状态与 Excel 一致。
- [ ] Pen 的功能区标签、组名、分组顺序、图标语义和控件位置在 1440×900 对照尺寸保持；窄窗口可滚动且控件命中位置正确。
- [ ] 保存类编辑先在临时 DOCX 上验证保存/重开；失败不丢 dirty 状态，未支持的 OOXML 内容不得静默丢弃。
- [x] 底栏 `FsQrk` 保持图标几何；页码/页数使用真实分页结果，未完成时显示计算/错误状态；缩小、放大和打印布局切换已接线。
- [ ] Computer Use 确认打印布局的实际页面和 Pen `Clpgm` 完全对齐，且 PDF 与原生打印重放相同场景。
- [ ] 最终使用 Computer Use 实际并排检查 Pen 参考、Word 页面和 Excel ribbon；截图或问题清单记录在验收结果中。

## 代码与状态依据

- `internal/desktopui/word_toolbar.go`：功能区 action 清单、能力启用判断、事件处理、查找与替换。
- `internal/desktopui/dialog.go`：单/双输入模态对话框。
- `internal/desktopui/word_ribbon_layout.go`：Word 功能区绘制、Pen 参考位置/分组及交互区域；命令 glyph 使用 Excel 图集。
- `internal/desktopui/word_ribbon_icons.go`：Word action 到 Excel 共享图集的 glyph 映射及着色调用。
- `internal/desktopui/word_pen_assets.go`：状态栏仍使用的 Pen PNG 嵌入资源。
- `internal/desktopui/ribbon_chrome.go`：Excel 96×96 图标图集裁切、着色和共享图标按钮。
- `internal/desktopui/word_share_copy.go`：Word Share 草稿 flush、选择状态校验、DOCX 快照副本导出及原子提交保护。
- `internal/format/docx/document.go`：DOCX 临时文件写入；`SaveAsWithCommitGuard` 在原子替换前允许版本校验。
- `internal/document/word.go`：Word neutral model 和文本/文本范围/段落/图片/历史能力接口。
- `internal/format/docx/word_properties.go`、`internal/desktopui/word_document_info.go`：五个核心属性的 DOCX 局部读写、包关系/内容类型处理及异步文档信息表单。
- `internal/format/docx/word_text.go`、`internal/format/docx/word_list.go`、`internal/format/docx/word_images.go`：当前 DOCX 文本样式、单 run 文本范围、列表和图片写入能力。
- `internal/format/docx/word_format_painter.go`：格式刷对所支持 run/paragraph 属性的单次、可撤销 XML patch。
- `internal/desktopui/word_status_bar.go`：`FsQrk` 状态栏；页码/总页数接真实结果，打印布局切换、缩小/放大和滑块拖动共用真实会话缩放状态。
- `docs/office-support.md`：DOCX 当前支持范围、连续流边界和保存验证状态。

静态尺寸验收记录：Pen `Word Document Editor` 参考画板为 1440×900，显示“第 1 页，共 3 页”、582 个字、中文(中国)、辅助功能状态，以及 42/34 dp 标签条、90 dp ribbon、33 dp 标尺、900×600 dp 纸张和 34 dp 底栏。文档标签 190×37 dp、6 dp 间距和加号菜单 x=526 dp 有几何用例；七个常驻标签的命令图标横坐标与组分割线由 `TestWordRibbonActionSlotsMatchPenNodeClpgm` 锁定。2026-10-03 用 Pencil MCP 实际读取并截图 `Clpgm`/`kiBim`/`FsQrk`，并用 CUA 检查同分支 Go 构建的 desktop 窗口。1280×820 dp 下顶栏和标尺高度匹配，900 dp 纸张居中于视口，底栏 34 dp 并右锚；按可见节点文字重建的临时样稿在修复省略 `w:orient` 误报后显示 3 页/573 字。Pen 是 3 页/582 字。重建 DOCX 含额外补页文字，第一屏标题线、段间距和表头样式不等价；页面坐标目前 x≈190 dp，而 Pen 为 x=270 dp。故尚无等价文档、匹配 1440×900 dp 视口配对截图，不能判定 1:1。截图只作为本轮 CUA/Pencil 内联结果，没有存放成可复用 PNG。macOS 打印面板曾通过 CUA 打开并取消，面板显示 1 页/1 份；这不代表 Pen 样稿或实际打印输出。DOCX、Word 编辑、分页/PDF/打印定向测试通过；最近的 `go test ./internal/desktopui -count=1` 暴露四个 Grid/typeface/geometry 失败和 `TestHomeFilterBarMatchesPenDimensions` 空夹具 panic；单独复跑还确认 `TestWordUpDownCrossesParagraphsAtPreferredVisualColumn` 仍失败。全量 UI 问题共 6 项，详情见 SDD ledger。
