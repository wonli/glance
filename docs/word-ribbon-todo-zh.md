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
| 文件 | 12 | 1 | 11 |
| 开始 | 27 | 11 | 16 |
| 插入 | 16 | 1 | 15 |
| 设计 | 9 | 0 | 9 |
| 布局 | 12 | 0 | 12 |
| 审阅 | 11 | 0 | 11 |
| 视图 | 12 | 0 | 12 |
| **常驻功能区合计** | **99** | **13** | **86** |
| 图片上下文功能区（额外项，不在 Pen 的 99 项内） | 5 | 5 | 0 |

另有撤销/重做的 DOCX 历史能力和调用代码，但 `wordHistoryActions` 未进入当前可见的七个常驻标签的 action 列表。它们不计入上表；需要对照 `Clpgm` 最终确认目标入口，如果设计要求显示图标，再加入功能区并补视觉验收。

## 已完成项

### 主页面与布局

- [x] Word 主页面采用独立的 Gio 布局和文档会话状态。
- [x] 七个常驻功能区标签及其 Pen 命令顺序已建模：文件、开始、插入、设计、布局、审阅、视图。
- [x] Word 连续流正文支持段落、表格、格式文本和行内图片；编辑器支持已覆盖范围内的文字修改、段落拆分/合并与图片编辑。
- [x] `FsQrk` 底栏的 34 dp 区域、主要文字和图形已绘制。
- [x] 当前底栏字数由正文内容动态计算。
- [x] 标尺、纸张画布和底栏在当前实现中已有对应布局。

### 已接通的 13 个常驻功能区图标

- [x] 文件：保存。
- [x] 开始：粘贴、剪切、复制、加粗、斜体、下划线、字体颜色、左对齐、居中、右对齐、两端对齐。
- [x] 插入：图片。

### 已接通的 5 个图片上下文操作

- [x] 替换图片。
- [x] 删除图片。
- [x] 保持宽高比调整图片宽度。
- [x] 向左旋转 90°。
- [x] 向右旋转 90°。

这些图片操作经过 DOCX 图片能力接口调用，不应与七个常驻标签的 99 项混算。

## 按图标逐项 TODO

状态说明：`[x]` 表示该功能区图标已有用户可触发的操作路径；`[ ]` 表示图标虽已定义或绘制，但当前没有对应的 Word ribbon 行为。若现有其他位置提供了相近操作，会在条目中说明；这仍不算该图标已接通。

### 文件（12 项：已完成 1，待实现 11）

- [ ] 新建 `new`：接入现有空白 DOCX 草稿创建流程，并切换到新文档。
- [ ] 打开 `open`：接入工作区现有异步文件选择器和打开流程。
- [ ] 最近使用 `recent`：打开工作区最近文件列表或对应文档入口。
- [x] 保存 `save`：保存当前 DOCX；等待待提交文字草稿后执行。
- [ ] 另存为 `save-as`：接入工作区已有的 Save As 流程；取消时保留原文档与脏状态。
- [ ] 自动保存 `autosave`：定义开关状态、触发时机、失败提示和关闭行为后再接入后台保存。
- [ ] 打印 `print`：需要打印页面布局或 PDF 渲染路径，详见“当前技术边界”。
- [ ] 导出 PDF `export-pdf`：先选定渲染方案，不能把连续流截图伪称 Word 打印版。
- [ ] 共享 `share`：接入平台分享服务或安全的导出文件流程；当前无 Word 专属分享入口。
- [ ] 文档信息 `doc-info`：读取并编辑受支持的 DOCX 核心属性，未支持的元数据必须原样保留。
- [ ] 保护文档 `protect-document`：先确定保护模式范围；密码/权限保护需单独做安全与兼容性验证。
- [ ] 版本历史 `version-history`：定义持久化版本快照、恢复及与当前会话撤销栈的关系。

### 开始（27 项：已完成 11，待实现 16）

- [x] 粘贴 `paste`：将纯文本粘贴到当前可编辑文本 run。
- [x] 剪切 `cut`：复制选中文本并删除对应内容。
- [x] 复制 `copy`：将当前 run 或选中文本写入系统剪贴板。
- [ ] 格式刷 `format-painter`：复制支持的文字/段落格式并应用到目标范围；先支持单文档内操作。
- [ ] 字体 `font-family`：读取时模型已有字体族字段，但写入 patch 尚未覆盖；增加 DOCX run 属性写入和 UI 选项。
- [ ] 减小字号 `font-size-decrease`：增加字号写入能力，定义步进、上下限和混合字号状态。
- [ ] 增大字号 `font-size-increase`：同上，并保留没有显式字号的继承语义。
- [x] 加粗 `bold`：通过 DOCX 文本格式 patch 修改。
- [x] 斜体 `italic`：通过 DOCX 文本格式 patch 修改。
- [x] 下划线 `underline`：通过 DOCX 文本格式 patch 修改。
- [ ] 高亮 `text-highlight`：扩展 neutral style patch 和 DOCX run 属性读写；明确主题色与高亮色的支持边界。
- [x] 字体颜色 `font-color`：颜色选择器可修改选中文本颜色。
- [ ] 项目符号 `bullet-list`：增加段落编号/项目符号模型及 `numbering.xml` 关系的安全写入。
- [ ] 编号 `numbered-list`：同项目符号，需验证编号连续性、嵌套和保存重开。
- [x] 左对齐 `align-left`：修改所选段落对齐方式。
- [x] 居中 `align-center`：修改所选段落对齐方式。
- [x] 右对齐 `align-right`：修改所选段落对齐方式。
- [x] 两端对齐 `align-justify`：修改所选段落对齐方式。
- [ ] 减少缩进 `decrease-indent`：ruler 已能改左右缩进；把该图标接到明确的缩进步进，不要重置首行/悬挂缩进等未支持属性。
- [ ] 增加缩进 `increase-indent`：同上，完成后覆盖保存重开和既有段落属性保留。
- [ ] 查找 `find`：在投影的可见/可编辑文本中查找并导航到命中项。
- [ ] 替换 `replace`：支持选定命中范围的替换；跨 run 替换前须定义格式、图片和表格边界规则。
- [ ] 选择 `select`：定义选择全部、当前段落等具体命令；现有选择主要基于 run/段落，尚无完整跨 run 选择模型。
- [ ] 编辑器 `editor`：确认该图标要打开的具体能力；没有后端或真实编辑能力前保持不可用。
- [ ] 听写 `dictate`：需要系统语音识别或受控的语音服务接入，详见技术边界。
- [ ] 加载项 `add-ins`：当前无 Office Add-ins 运行时或插件宿主；需单独设计安全扩展接口。
- [ ] 设计器 `designer`：没有相应布局建议服务或本地规则引擎；明确产品范围后再接入。

### 插入（16 项：已完成 1，待实现 15）

- [ ] 封面 `cover`：提供受支持的本地封面模板，并按 DOCX 正文结构插入。
- [ ] 空白页 `blank-page`：实现明确的分页/分节语义；不能只插入空白段落后声称创建了新页。
- [ ] 分页 `page-break`：写入 DOCX 分页符并验证其投影、保存和 Word 重开表现。
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
- [ ] 字数统计 `review-word-count`：底栏目前只显示动态非空白字符数；增加明确的字数/字符数详情面板。
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

底栏的**区域与图形已绘制**，但绘制出来不代表控件已有功能。

- [x] 底栏位置、尺寸、字数区域及左右侧图形已画出。
- [ ] 修正静态页数文案：当前显示“第 1 页”，但应用没有 Word 分页器。删除该伪页数或改为“连续视图”等真实状态；在分页能力未完成前不显示页码/总页数。
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
- [ ] 使用 Computer Use 在 1440×900 基准尺寸逐标签对照 Word/Excel，并在窄窗口检查滚动和缩放；保存对照截图或记录视觉问题并迭代。当前 CUA 可连接 Pen，但 Glance Preview 窗口读取超时，临时构建实例也未能注册为可访问应用，尚无运行界面对照截图。

代码侧已完成图集迁移。待 CUA 能读取 Glance Office 窗口后，再完成基准尺寸及窄窗口的视觉验收。

此项在当前栈内可完成：Excel 已有 96×96 图集裁切、图标着色和缓存函数。风险主要是图标语义映射、Home 位图拆分后的命中区域和小尺寸下的视觉差异，不需要更换 UI 框架。

## 当前技术边界、原因与可选方案

这里的“当前无法完成”指当前实现和现有能力接口无法可靠交付，不表示 Gio/Go 永远做不到。需要扩展接口、格式适配器或引入外部组件的项目，在相应能力通过保存重开验证前都应保持禁用。

| 能力 | 当前边界与原因 | 可行方案 | 建议当前处理 |
|---|---|---|---|
| Word 精确分页、打印布局、单页/多页、总页数、动态页码 | UI 使用 Gio 连续流布局；neutral Word model 没有 Word 页面布局结果，也没有分页引擎。字体、表格、分节等因素会影响分页。 | 两条路：实现限定 DOCX 子集的自有分页器，接受与 Word 不完全一致；或接入 LibreOffice 等外部 DOCX→PDF 渲染器，再定义跨平台打包、调用超时、字体、临时文件和编辑坐标同步。若要求 Microsoft Word 级完全一致，需可用且可合法分发的同等布局引擎。 | 保持连续视图；隐藏页数/分页视图入口，直到选定并验证分页方案。PDF/打印列为单独技术决策，不伪造页数。 |
| 页眉、页脚、页码、节设置 | 当前投影/编辑能力主要针对正文段落、表格、run 与行内图片；没有 section/header/footer part 的 neutral 编辑接口。页码字段的实际页数依赖分页。 | 扩展 section 与 header/footer DTO、DOCX relationship/part 读写和保存重开用例。先可支持固定内容页眉页脚；动态页码与总页数等待分页引擎。 | 固定页眉页脚可作为 DOCX 适配器扩展；动态页码归分页技术决策。 |
| 字体、字号、颜色高亮、段距、主题等样式写入 | 部分样式可读或已投影，但 `WordTextFormatPatch` 目前只写粗体、斜体、下划线、字体颜色；字体族/字号没有写入 patch。 | 为每种样式增加 neutral patch 和 `word/document.xml` / styles/theme 的局部更新；保留未改 OOXML 属性，使用临时副本做保存重开与 untouched-content 检查。 | 属于当前 DOCX adapter 可扩展范围，不是 Gio 的硬限制；按属性分项实现。 |
| 表格、链接、书签、列表写入 | 表格可读取/显示，但尚无通用结构创建接口；超链接、书签、编号列表需要关系、ID 或 `numbering.xml` 管理。 | 扩展 neutral Word 编辑能力与 DOCX XML/关系写入；从小范围结构开始，加唯一 ID、失效关系和保存重开验证。 | 可实现，但需逐项验证；在 adapter 完成前保持图标禁用。 |
| 形状、图表、文本框、艺术字、公式 | 当前编辑模型主要支持文字和行内栅格图片；未知 DrawingML/OMML 内容不能作为可编辑对象安全投影。 | 增加对象 DTO、DrawingML/OMML 读取写入、部件关系、预览和兼容性矩阵；优先选择一个对象类型做端到端切片。 | 不是简单的图标接线工作；单独立项，未完成前保持只读/禁用。 |
| 批注、修订、接受/拒绝 | 缺少 comment、revision 的对象模型和包内关系；直接改 XML 易丢作者、时间、范围或修订语义。 | 建立 comments/revisions neutral 模型和受限操作；先选定支持的 DOCX 子集，验证 Word 重开及未处理修订保留。 | 高兼容性风险；作为独立 DOCX 适配器项目，不能仅用普通文本差异模拟。 |
| 语音听写、翻译 | Gio/Go 当前没有语音识别或翻译服务。 | 增加平台语音适配器或有明确隐私/网络授权的服务接口；需定义离线/失败反馈和语言选择。 | 作为可选外部能力，不作为当前 ribbon 的内建能力；未接入前禁用。 |
| 加载项、设计器、宏 | 当前没有 Office Add-ins 宿主、Designer 服务或 VBA 宏安全沙箱。宏执行会越出普通文档编辑的安全边界。 | 加载项需设计受限插件协议；Designer 可定义本地规则或服务；宏优先只做安全识别/提示，执行只能经独立、明确授权的隔离方案。 | 不在当前 MVP 内；宏执行保持禁用。 |
| 文件打印、PDF、系统分享 | 普通本地打开/保存流程已有工作区能力，但 Word ribbon 这些入口未接线；PDF/打印需要渲染与平台服务，分享需要平台集成。 | 新建文件操作复用现有 workspace API；打印/PDF 复用获选定的渲染器；分享走操作系统分享面板或导出文件。 | Open/Save As 等优先复用现有功能；Print/PDF 先解决渲染器决策。 |

## 建议执行顺序

### P0：让界面真实且图标统一

- [ ] 修正底栏静态“第 1 页”，并区分状态显示和不可交互图形。
- [ ] 完成 Word ribbon 到 Excel sprite/tint 的迁移和图标语义映射。
- [ ] 完成 Computer Use 下的 Word/Excel 同尺寸对照；确认 Word 布局与 Pen 分组仍对齐。
- [ ] 复核所有未实现图标保持不可操作，图标色不暗示功能可用。

### P1：优先补齐普通编辑高频功能

- [ ] 字体族/字号/高亮。
- [ ] 项目符号/编号/缩进、段前段后。
- [ ] 查找/替换/选择、字数详情。
- [ ] 表格、链接、书签、分页符、空白页、符号。
- [ ] 新建/打开/最近/另存为等入口接入现有 workspace 流程。

### P2：DOCX 页面属性与设计

- [ ] 页边距、方向、纸张大小、分栏、页面颜色/边框、水印、主题与段落间距。
- [ ] 在每个功能通过 DOCX 保存、重开和未修改内容保留验证后再启用。

### P3：复杂 OOXML 与外部能力

- [ ] 形状/图表/文本框/艺术字/公式。
- [ ] 批注、修订、比较、保护编辑。
- [ ] 语音、翻译、加载项、Designer。
- [ ] 单独决定是否引入分页/打印/PDF 渲染引擎；该决定会影响页眉页脚和多个 View 功能。

## 验收标准

- [ ] 99 个 Pen 常驻功能区命令逐项有明确状态：已接通并经过验证，或保持不可操作；不留“看起来可点、实际无效”的按钮。
- [ ] Word ribbon icon 使用 Excel 的共享图集和着色方式；正常、禁用、hover/active 状态与 Excel 一致。
- [ ] Pen 的功能区标签、组名、分组顺序、图标语义和控件位置在 1440×900 对照尺寸保持；窄窗口可滚动且控件命中位置正确。
- [ ] 保存类编辑先在临时 DOCX 上验证保存/重开；失败不丢 dirty 状态，未支持的 OOXML 内容不得静默丢弃。
- [ ] 底栏 `FsQrk` 结构与位置保留；页面/缩放状态仅显示真实数据。没有分页引擎前不显示静态页码或总页数。
- [ ] 最终使用 Computer Use 实际并排检查 Pen 参考、Word 页面和 Excel ribbon；截图或问题清单记录在验收结果中。

## 代码与状态依据

- `internal/desktopui/word_toolbar.go`：功能区 action 清单、能力启用判断、事件处理。
- `internal/desktopui/word_ribbon_layout.go`：Word 功能区绘制、Pen 参考位置/分组及交互区域；命令 glyph 使用 Excel 图集。
- `internal/desktopui/word_ribbon_icons.go`：Word action 到 Excel 共享图集的 glyph 映射及着色调用。
- `internal/desktopui/word_pen_assets.go`：状态栏仍使用的 Pen PNG 嵌入资源。
- `internal/desktopui/ribbon_chrome.go`：Excel 96×96 图标图集裁切、着色和共享图标按钮。
- `internal/document/word.go`：Word neutral model 和文本/段落/图片/历史能力接口。
- `internal/format/docx/word_text.go`、`internal/format/docx/word_images.go`：当前 DOCX 文本样式与图片写入能力。
- `internal/desktopui/word_status_bar.go`：`FsQrk` 状态栏；当前“第 1 页”、视图图标、缩放滑块和百分比是静态绘制。
- `docs/office-support.md`：DOCX 当前支持范围、连续流边界和保存验证状态。

最近一次图标迁移验证：Word ribbon 专项测试通过，桌面构建通过；`go test ./...` 仍有 4 个既有的字体与网格几何测试失败（`TestCellTypefaceKeepsWorkbookFamilyAndAddsCJKFallbacks`、`TestGridResizeHandleUsesFixedHeaderCoordinates`、`TestSelectionRectUsesVisibleGridGeometry`、`TestGridViewportScalesGeometryToDevicePixels`）。CUA 视觉验收因 Glance Preview 窗口不可访问而未完成。
