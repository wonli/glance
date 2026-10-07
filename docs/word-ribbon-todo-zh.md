# Word 与 Excel 功能区（Ribbon）图标与能力全景 TODO（中文）

更新日期：2026-10-05  
状态依据：当前仓库实现、独立 SVG 图标资产体系、`docs/excel_menu.json` 与 Pen 参考节点 `Clpgm`、`kiBim`、`FsQrk` 对照。

---

## 1. 目标与架构统一标准

1. **统一的矢量 SVG 图标资产体系**：
   - 彻底废除旧版位图/复合雪碧图（`toolbar-icons.png` / `toolbar-icons.svg` 裁切机制）。
   - 全面迁移为独立嵌入的矢量 SVG 资产体系（`assets/icons/word/` 与 `assets/icons/excel/`），运行时通过 `oksvg` 与 `rasterx` 在内存中矢量光栅化并动态着色（NRGBA Tinting），且以 `sync.RWMutex` 进行多规格缓存。
   - **Word 图标规范**：7 个常驻标签（99 个命令）已全部具备独立的 `assets/icons/word/<action.id>.svg` 专属图标；上下文及未定制操作自动回退到 `assets/icons/excel/<alias>.svg` 共享图集。
   - **Excel 图标规范**：8 个常驻标签（73 个命令）与图片上下文（3 个命令）统一通过 `excelIconAssetNames` 映射到 `assets/icons/excel/<assetName>.svg`，完全对齐 `docs/excel_menu.json`。
   - 图标渲染标准尺寸统一为 20 dp（按钮统一外框 36×36 dp，圆角 3 dp / 7 dp，交互态与置灰态严格统一）。

2. **功能接通口径（Definition of Done）**：
   - **已接通（`[x]`）**：要求图标有实际可用的 UI 操作路径与反馈。
     - **修改文档类命令**（如文字编辑、样式、单元格格式、行列增删、插入图片/分页符等）：必须具备完整的中立模型/适配器写入路径，且必须有原子替换、保存重开和未修改内容完好保留的回归证据。
     - **视图/会话/工作区类命令**（如模式切换、缩放、标尺、网格线、导航、历史撤销重做、剪贴板等）：必须具备真实 UI 交互与状态回归，不修改文档脏状态。
     - **文件导出/系统命令**（如 PDF 导出、系统原生打印、本地共享副本导出）：必须通过隔离的任务管道完成，校验文档版本防篡改，并有取消与异常保护。
   - **适配器具备，UI 未接通**：适配器/文档模型已实现能力并通过底层测试，但功能区尚未开放可用操作入口（如 Word 段前/段后间距）。单列统计，不计入已接通数。
   - **尚无可用实现（`[ ]`）**：图标已定义并绘制，但后端尚未具备能力。
     - **Word 策略**：通过能力判断置灰（`disabledToolbarText`），禁用 Clickable 事件，禁止留“看似可点实则无效”的按钮。
     - **Excel 策略**：未实现插槽统一拦截，点击时通过 `a.showSpreadsheetFeatureMissing(action.label)` 在底栏状态栏提示““%s”功能暂未实现”。

3. **近期重要架构与功能调整记录（2026-10-05 梳理）**：
   - **图标体系重构（Commit `63dc077`）**：将单一 Sprite Sheet 替换为两套独立 SVG 目录（Word 99 个 SVG、Excel 67 个通用 SVG），解耦图标资源。
   - **Excel 功能区多标签扩展**：Excel 从原先简略的 3 标签扩展为 8 个常驻标签（文件、开始、插入、页面布局、公式、数据、审阅、视图）及 1 个图片上下文标签，全面对接 `docs/excel_menu.json`。
   - **统一模态弹窗系统（Commit `f1e8930`）**：将分散的即席输入框统一为通用 `confirm_dialog.go`（`modalDialogState`），支持输入、确认、多字段表单（覆盖 Word 文档信息、Word 字体/缩放/图片调整、Excel 表格重命名、Excel 图片尺寸调整等）。
   - **双端 PDF 导出管道接通**：
     - Word：通过共享分页引擎与 HarfBuzz 矢量重放，原子写盘导出 PDF。
     - Excel：通过纯 Go `internal/format/pdf/spreadsheet.go` 导出网格、格式与图片至 PDF，已接通 Excel 文件 > 导出命令。
   - **应用壳重命名（Commit `edfed2d`）**：全局统一为 `glance` 桌面架构。

---

## 2. 统计总览

统计口径：统计单位为各功能区中的一个可点击命令图标插槽。

| 功能区分类 | 标签页 | 命令图标数 | 已接通 | 适配器/代码具备，UI未接通或待最终验收 | 尚无可用实现 |
|---|---|---:|---:|---:|---:|
| **Word 常驻功能区** | 文件 | 12 | 11 | 0 | 1 |
| | 开始 | 27 | 24 | 0 | 3 |
| | 插入 | 16 | 12 | 0 | 4 |
| | 设计 | 9 | 5 | 0 | 4 |
| | 布局 | 12 | 12 | 0 | 0 |
| | 审阅 | 11 | 9 | 0 | 2 |
| | 视图 | 12 | 11 | 0 | 1 |
| | **Word 常驻小计** | **99** | **84** | **0** | **15** |
| Word 上下文标签 | 图片（选图后出现） | 5 | 5 | 0 | 0 |
| **Excel 常驻功能区** | 文件 | 6 | 5 | 0 | 1 |
| | 开始 | 24 | 24 | 0 | 0 |
| | 插入 | 9 | 8 | 0 | 1 |
| | 页面布局 | 6 | 6 | 0 | 0 |
| | 公式 | 8 | 8 | 0 | 0 |
| | 数据 | 7 | 7 | 0 | 0 |
| | 审阅 | 6 | 6 | 0 | 0 |
| | 视图 | 7 | 7 | 0 | 0 |
| | **Excel 常驻小计** | **73** | **70** | **0** | **3** |
| Excel 上下文标签 | 图片（选图后出现） | 3 | 3 | 0 | 0 |
| **全应用功能区总计** | **全常驻标签合计** | **172** | **154** | **0** | **18** |

*(注：Word 另有快捷撤销/重做 `wordHistoryActions`；Excel 撤销/重做已包含在“开始”功能区中。)*

---

## 3. Word 功能区逐项扫描与 TODO 清单

### 3.1 文件（12 项：已接通 11，待实现 1）

- [x] **新建** `new`（图标：`new.svg`，组：文档）：创建空白 DOCX 草稿并切换到新文档标签；保留已有文档会话。
- [x] **打开** `open`（图标：`open.svg`，组：文档）：接入工作区异步 DOCX/XLSX/JSON 文件选择器；取消不改变当前文档。
- [x] **最近使用** `recent`（图标：`recent.svg`，组：文档）：切换至工作区“最近”页面；保留当前文档会话。
- [x] **保存** `save`（图标：`save.svg`，组：保存）：提交正文草稿后原子写入源 DOCX 文件；支持脏状态判定与保存重开验证。
- [x] **另存为** `save-as`（图标：`save-as.svg`，组：保存）：接入工作区 DOCX 另存为流程；取消时保留原文档、路径与脏状态。
- [x] **自动保存** `autosave`（图标：`autosave.svg`，组：保存）：按文档开关状态；成功提交 Word 编辑后后台异步存盘。
- [x] **打印** `print`（图标：`print.svg`，组：输出）：macOS 使用 PDFKit/AppKit 原生打印面板，复用共享分页场景生成的 PDF；Windows/Linux 暂置灰禁用。
- [x] **导出 PDF** `export-pdf`（图标：`export-pdf.svg`，组：输出）：触发系统保存对话框；重放共享分页场景中 HarfBuzz 字形矢量与行内图片，原子写盘，带隐藏文本层供搜索复制。
- [x] **共享** `share`（图标：`share.svg`，组：输出）：提交草稿后打开保存选择器，将当前一致版本导出为本地 DOCX 独立副本；禁止同源覆盖。
- [x] **文档信息** `doc-info`（图标：`doc-info.svg`，组：信息）：异步读取并编辑标题、主题、作者、关键词、说明 5 个核心属性；已接入统一 `ModalDialog` 表单，回归覆盖 XML 字符转义与未知元数据保留。
- [ ] **保护文档** `protect-document`（图标：`protect-document.svg`，组：信息）：**暂缓**。涉及 Office Agile AES 加密规范与 SQLite 恢复快照加密边界，在加解密方案评估完成前保持禁用。
- [x] **版本历史** `version-history`（图标：`version-history.svg`，组：信息）：本机 SQLite 历史时间线；支持基线记录、手动与自动保存快照、历史版本预览恢复与撤销重做。

### 3.2 开始（27 项：已接通 24，待实现 3）

- [x] **粘贴** `paste`（图标：`paste.svg`，组：剪贴板）：将纯文本异步写入当前光标或可编辑文本 run。
- [x] **剪切** `cut`（图标：`cut.svg`，组：剪贴板）：复制选中文本并删除对应 run 内容。
- [x] **复制** `copy`（图标：`copy.svg`，组：剪贴板）：将当前选中文本或 run 内容写入系统剪贴板。
- [x] **格式刷** `format-painter`（图标：`format-painter.svg`，组：剪贴板）：单次/粘性复制来源 run 与段落格式到目标 run。
- [x] **字体** `font-family`（图标：`font-family.svg`，组：字体）：弹出统一输入对话框修改字体族，覆盖 `ascii`、`hAnsi`、`eastAsia`、`cs` 属性。
- [x] **减小字号** `font-size-decrease`（图标：`font-size-decrease.svg`，组：字体）：按 1 pt 步进减小，范围 1–1638 pt。
- [x] **增大字号** `font-size-increase`（图标：`font-size-increase.svg`，组：字体）：按 1 pt 步进增大。
- [x] **加粗** `bold`（图标：`bold.svg`，组：字体）：切换选中文本加粗状态。
- [x] **斜体** `italic`（图标：`italic.svg`，组：字体）：切换选中文本斜体状态。
- [x] **下划线** `underline`（图标：`underline.svg`，组：字体）：切换选中文本单下划线状态。
- [x] **高亮** `text-highlight`（图标：`text-highlight.svg`，组：字体）：弹出 8 色 Word 标准命名高亮选择器及无高亮选项。
- [x] **字体颜色** `font-color`（图标：`font-color.svg`，组：字体）：弹出 8 色字体颜色面板。
- [x] **项目符号** `bullet-list`（图标：`bullet-list.svg`，组：段落）：切换多级项目符号（0–8 级，循环 `•`、`◦`、`▪`）。
- [x] **编号** `numbered-list`（图标：`numbered-list.svg`，组：段落）：切换多级十进制编号（0–8 级）。
- [x] **左对齐** `align-left`（图标：`align-left.svg`，组：段落）：修改段落为左对齐。
- [x] **居中** `align-center`（图标：`align-center.svg`，组：段落）：修改段落为居中对齐。
- [x] **右对齐** `align-right`（图标：`align-right.svg`，组：段落）：修改段落为右对齐。
- [x] **两端对齐** `align-justify`（图标：`align-justify.svg`，组：段落）：修改段落为两端对齐。
- [x] **减少缩进** `decrease-indent`（图标：`decrease-indent.svg`，组：段落）：列表降低层级或普通段落减少左缩进 0.5 英寸。
- [x] **增加缩进** `increase-indent`（图标：`increase-indent.svg`，组：段落）：列表提升层级或普通段落增加左缩进 0.5 英寸。
- [x] **查找** `find`（图标：`find.svg`，组：编辑）：打开查找对话框，按 run 匹配定位、滚动并高亮显示。
- [x] **替换** `replace`（图标：`replace.svg`，组：编辑）：打开双输入替换对话框，单 run 内安全替换文本。
- [x] **选择** `select`（图标：`select.svg`，组：编辑）：实现全文档及跨段落 run 文本范围全选模型，建立全局选区与 caret 聚焦高亮，支持一键全选并复制。
- [x] **编辑器** `editor`（图标：`editor.svg`，组：编辑器）：通过统一 `ModalDialog` 弹窗对文档进行综合语法与排版质量评分（0–100 分），统计字数、段落数、表格数并提供连续重复标点、未配对括号/引号检测与校对建议。
- [ ] **听写** `dictate`（图标：`dictate.svg`，组：编辑器）：待系统语音转文字（STT）服务接入。
- [ ] **加载项** `add-ins`（图标：`add-ins.svg`，组：编辑器）：待设计受限插件扩展接口。
- [ ] **设计器** `designer`（图标：`designer.svg`，组：编辑器）：待排版规则引擎或建议服务接入。

### 3.3 插入（16 项：已接通 12，待实现 4）

- [x] **封面** `cover`（图标：`cover.svg`，组：页面）：通过统一 `ModalDialog` 弹窗向 DOCX 文档首页插入标准封面页（标题、副标题、作者及分页符），驱动连续排版与分页重排，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **空白页** `blank-page`（图标：`blank-page.svg`，组：页面）：在当前 caret 插入双原生分页符与空白编辑 run，自动定位光标至空白页，驱动连续排版与分页重排，支持保存重开与撤销重做。
- [x] **分页** `page-break`（图标：`page-break.svg`，组：页面）：在当前 caret 插入原生 `w:br w:type="page"`，支持保存重开与分页重排。
- [x] **表格** `insert-table`（图标：`insert-table.svg`，组：表格）：通过统一 `ModalDialog` 弹窗支持标准尺寸预设（2x2、3x3、4x4、5x3）与自定义行列输入（1–100 行，1–50 列），向 DOCX 写入规范 `<w:tbl>` 网格结构并在当前光标段落下插入，直接对接 Gio UI 真实表格渲染并受撤销重做（Undo/Redo）与历史快照保护。
- [x] **图片** `insert-image`（图标：`insert-image.svg`，组：插图）：异步文件选择器插入 PNG/JPEG 行内图片。
- [ ] **形状** `insert-shapes`（图标：`insert-shapes.svg`，组：插图）：待 DrawingML 形状中立模型与 XML 写入。
- [ ] **图表** `insert-chart`（图标：`insert-chart.svg`，组：插图）：待图表部件与数据模型接入。
- [x] **链接** `insert-link`（图标：`insert-link.svg`，组：链接）：通过统一 `ModalDialog` 双输入弹窗向当前选区/Run 插入外部 Web 超链接（写入 `word/_rels/document.xml.rels` 外部关系）或内部书签超链接（`w:anchor`），带下划线与标准超链接蓝色着色，受完整撤销重做（Undo/Redo）与历史快照保护。
- [x] **书签** `insert-bookmark`（图标：`insert-bookmark.svg`，组：链接）：通过统一 `ModalDialog` 弹窗在当前选区/Run 插入命名书签（`<w:bookmarkStart>` 与 `<w:bookmarkEnd>`，自动分配唯一 ID 与转义名称），受完整撤销重做（Undo/Redo）与历史快照保护。
- [x] **页眉** `insert-header`（图标：`insert-header.svg`，组：页眉页脚）：通过统一 `ModalDialog` 弹窗设置或清空文档页眉文本，写入 `word/header1.xml`、建立 relationships 引用并在 `<w:sectPr>` 写入 `<w:headerReference>`，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **页脚** `insert-footer`（图标：`insert-footer.svg`，组：页眉页脚）：通过统一 `ModalDialog` 弹窗设置或清空文档页脚文本，写入 `word/footer1.xml`、建立 relationships 引用并在 `<w:sectPr>` 写入 `<w:footerReference>`，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **页码** `insert-page-number`（图标：`insert-page-number.svg`，组：页眉页脚）：通过统一 `ModalDialog` 弹窗在文档页脚中插入动态页码字段（`<w:fldSimple w:instr="PAGE"/>`），支持前后附带说明文字，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **文本框** `insert-textbox`（图标：`insert-textbox.svg`，组：文本）：通过统一 `ModalDialog` 弹窗在当前光标段落下插入标准 Callout 文本框（1x1 结构化排版框、左侧强调蓝边与柔和底色），直接对接 Gio UI 原生表格渲染并受撤销重做（Undo/Redo）保护。
- [ ] **艺术字** `insert-wordart`（图标：`insert-wordart.svg`，组：文本）：待文本特效渲染。
- [ ] **公式** `insert-equation`（图标：`insert-equation.svg`，组：符号）：待 OMML 数学公式模型。
- [x] **符号** `insert-symbol`（图标：`insert-symbol.svg`，组：符号）：弹出 24 常用标点/货币/数学/箭头符号面板并按字符插入。

### 3.4 设计（9 项：已接通 5，待实现 4）

- [ ] **主题** `design-themes`（图标：`design-themes.svg`，组：文档格式）：待主题 XML 解析与切换。
- [ ] **颜色** `design-colors`（图标：`design-colors.svg`，组：文档格式）：待文档主题配色方案。
- [ ] **字体** `design-fonts`（图标：`design-fonts.svg`，组：文档格式）：待主题主要/次要字体配置。
- [ ] **效果** `design-effects`（图标：`design-effects.svg`，组：文档格式）：待图形特效集。
- [x] **紧凑** `design-compact`（图标：`design-compact.svg`，组：段落间距）：通过 `SetAllParagraphsSpacing(0, 80)` 将全文档所有段落统一设置为紧凑间距（段前 0 pt，段后 4 pt），批量应用 XML patch 并受撤销重做与历史快照保护。
- [x] **宽松** `design-spacious`（图标：`design-spacious.svg`，组：段落间距）：通过 `SetAllParagraphsSpacing(0, 240)` 将全文档所有段落统一设置为宽松间距（段前 0 pt，段后 12 pt），批量应用 XML patch 并受撤销重做与历史快照保护。
- [x] **水印** `design-watermark`（图标：`design-watermark.svg`，组：页面背景）：通过统一 `ModalDialog` 弹窗支持设置文字水印（如“机密”、“绝密”、“草稿”或自定义输入）或一键清除水印，底层向 DOCX `word/header1.xml` 写入标准 Word 兼容的水印形状部件与 `<w:headerReference>` 引用，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **页面颜色** `design-page-color`（图标：`design-page-color.svg`，组：页面背景）：通过统一 `ModalDialog` 弹窗支持标准预设（羊皮纸米黄 #FAF8F5、护眼绿 #E8F5E9、柔和淡蓝 #E3F2FD、深色夜间 #263238、纯白）及任意自定义 16 进制颜色输入，向 DOCX `word/document.xml` 写入或清除 `<w:background w:color="HEX"/>`，实时驱动页面表面动态着色与重排，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **页面边框** `design-page-border`（图标：`design-page-border.svg`，组：页面背景）：通过统一 `ModalDialog` 弹窗支持单线方框（box/single）、双线方框（double）、虚线方框（dashed）预设及无边框清除，向 DOCX `<w:sectPr><w:pgBorders>` 写入标准四方页面边框，受撤销重做（Undo/Redo）与历史快照保护。

### 3.5 布局（12 项：全部已接通，待实现 0）

- [x] **页边距** `layout-margins`（图标：`layout-margins.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗支持预设（普通 72pt、窄 36pt）与上/下/左/右自定义输入（0–1000 pt），转换为 twips 写入 DOCX `<w:sectPr><w:pgMar>`，支持保存重开与撤销重做。
- [x] **方向** `layout-orientation`（图标：`layout-orientation.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗切换纵向（Portrait）与横向（Landscape），自动适配纸张宽高互换与 `<w:pgSz w:orient>` 写入，驱动共享分页引擎与 PDF 重排，支持撤销重做。
- [x] **纸张大小** `layout-size`（图标：`layout-size.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗支持标准尺寸预设（A4 210×297mm、Letter 8.5×11in）与自定义宽高输入（1–3000 pt），写入 DOCX `<w:sectPr><w:pgSz>`，支持撤销重做。
- [x] **分栏** `layout-columns`（图标：`layout-columns.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗支持单栏、两栏、三栏设置（1–4 栏），向 DOCX `<w:sectPr><w:cols>` 写入分栏结构并规范排列顺序，支持保存重开、撤销重做与历史快照保护。
- [x] **分隔符** `layout-breaks`（图标：`layout-breaks.svg`，组：分隔）：通过统一 `ModalDialog` 弹窗支持插入分页符（`w:br w:type="page"`）与分栏符（`w:br w:type="column"`），切分正文 run 并自动定位光标，驱动连续排版与分页重排，支持保存重开与撤销重做。
- [x] **行号** `layout-line-numbers`（图标：`layout-line-numbers.svg`，组：分隔）：通过统一 `ModalDialog` 弹窗支持开启连续行号（continuous）、每页重置（newPage）或一键清除行号设置，底层写入 DOCX `<w:sectPr><w:lnNumType>`，受撤销重做与历史快照保护。
- [x] **缩进** `layout-indent`（图标：`layout-indent.svg`，组：段落）：通过统一 `ModalDialog` 弹窗支持左侧/右侧自定义缩进输入（0–1000 磅）或一键清除缩进，转换为 EMU 写入 DOCX 段落 XML，受撤销重做与历史快照保护。
- [x] **段前** `layout-before`（图标：`layout-before.svg`，组：段落）：通过统一 `ModalDialog` 弹窗输入磅值（0–1584 pt），由 `math.Round(pt * 20.0)` 转换为 twips 写入 DOCX 段落 XML，支持撤销重做。
- [x] **段后** `layout-after`（图标：`layout-after.svg`，组：段落）：通过统一 `ModalDialog` 弹窗输入磅值（0–1584 pt），转换为 twips 写入 DOCX 段落 XML，支持撤销重做。
- [x] **位置** `layout-position`（图标：`layout-position.svg`，组：排列）：通过统一 `ModalDialog` 弹窗展示与配置文档对象排版位置属性（嵌入式流式排版或页边距对齐规范）。
- [x] **环绕文字** `layout-wrap`（图标：`layout-wrap.svg`，组：排列）：通过统一 `ModalDialog` 弹窗展示与配置对象文字环绕模式（嵌入型 Inline、四周型 Square 等）。
- [x] **对齐** `layout-align`（图标：`layout-align.svg`，组：排列）：通过统一 `ModalDialog` 弹窗支持对选中段落或对象进行左对齐、居中、右对齐与两端对齐设置，受撤销重做（Undo/Redo）与历史快照保护。

### 3.6 审阅（11 项：已接通 9，待实现 2）

- [x] **编辑器** `review-editor`（图标：`review-editor.svg`，组：校对）：通过统一 `ModalDialog` 弹窗对文档进行综合语法与排版质量评分，统计字符数、段落数并提供标点闭合与重复标点校对建议。
- [x] **字数统计** `review-word-count`（图标：`review-word-count.svg`，组：校对）：打开统一模态面板展示含/不含空白字数、段落数、表格数。
- [x] **翻译** `review-translate`（图标：`review-translate.svg`，组：语言）：通过统一 `ModalDialog` 弹窗读取当前选区或正文段落内容，输入翻译译文一键替换入正文，受完整撤销重做与历史快照保护。
- [x] **语言** `review-language`（图标：`review-language.svg`，组：语言）：通过统一 `ModalDialog` 弹窗支持标准校对语言预设（简体中文 zh-CN、英语 en-US、日语 ja-JP）与自定义语言代码输入，底层向 DOCX 写入 `<w:lang>` 属性，受撤销重做（Undo/Redo）与历史快照保护。
- [ ] **新建批注** `review-new-comment`（图标：`review-new-comment.svg`，组：批注）：待 comment 部件与锚点模型。
- [ ] **删除批注** `review-delete-comment`（图标：`review-delete-comment.svg`，组：批注）：待批注删除。
- [x] **修订** `review-track`（图标：`review-track.svg`，组：修订）：一键开启或关闭文档修订跟踪模式，底层向 DOCX `word/settings.xml` 写入或移除 `<w:trackRevisions/>`，按钮高亮与状态栏实时反馈，受撤销重做（Undo/Redo）保护。
- [x] **接受** `review-accept`（图标：`review-accept.svg`，组：修订）：通过统一 `ModalDialog` 弹窗确认后一次性接受文档中所有插入修订并永久清除删除标记，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **拒绝** `review-reject`（图标：`review-reject.svg`，组：修订）：通过统一 `ModalDialog` 弹窗确认后一次性放弃所有插入修订并恢复所有已删除文本，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **比较** `review-compare`（图标：`review-compare.svg`，组：保护）：通过统一 `ModalDialog` 弹窗对比当前编辑会话与磁盘保存版本差异，展示同步状态与段落、字符、表格结构指标。
- [x] **限制编辑** `review-restrict`（图标：`review-restrict.svg`，组：保护）：通过统一 `ModalDialog` 弹窗支持开启或解除只读编辑保护，底层向 DOCX `word/settings.xml` 写入或移除 `<w:documentProtection w:edit="readOnly" w:enforcement="1"/>`，高亮激活态与底栏状态实时反馈，受撤销重做（Undo/Redo）与历史快照保护。

### 3.7 视图（12 项：已接通 11，待实现 1）

- [x] **阅读模式** `view-reading`（图标：`view-reading.svg`，组：视图）：沉浸式隐藏功能区与标尺，正文只读，支持一键退出。
- [x] **打印布局** `view-print-layout`（图标：`view-print-layout.svg`，组：视图）：切换至基于 `internal/wordlayout` 计算的真实 `PageScene` 分页页面视图。
- [x] **Web版式** `view-web-layout`（图标：`view-web-layout.svg`，组：视图）：切换至无边界连续排版画布，自适应窗口宽度。
- [x] **标尺** `view-ruler`（图标：`view-ruler.svg`，组：显示）：切换顶部与左侧标尺显隐，恢复后缩进拖拽保持可用。
- [x] **网格线** `view-gridlines`（图标：`view-gridlines.svg`，组：显示）：屏幕上叠加 24 dp 辅助网格线，不影响文档内容与导出。
- [x] **导航窗格** `view-navigation`（图标：`view-navigation.svg`，组：显示）：打开左侧 300 dp 面板，支持内置标题大纲树与全文搜索定位。
- [x] **缩放** `view-zoom`（图标：`view-zoom.svg`，组：缩放）：打开统一弹窗输入 50%–150% 百分比，驱动会话缩放。
- [x] **单页** `view-single-page`（图标：`view-single-page.svg`，组：缩放）：打印布局自动适配视口完整显示单页。
- [x] **多页** `view-multiple-pages`（图标：`view-multiple-pages.svg`，组：缩放）：打印布局双页并排横向平铺。
- [x] **新建窗口** `view-new-window`（图标：`view-new-window.svg`，组：窗口）：基于共享会话派生新桌面窗口，保持独立视口、选区与草稿双向同步，已建立端到端集成测试验证。
- [x] **并排查看** `view-side-by-side`（图标：`view-side-by-side.svg`，组：窗口）：支持多桌面窗口运行时平铺并排排列，或切换双页横向并排模式，状态栏与按钮实时高亮反馈。
- [ ] **宏** `view-macros`（图标：`view-macros.svg`，组：宏）：安全沙箱未建立前保持禁用。

### 3.8 Word 图片上下文功能区（5 项：全部已接通）

- [x] **替换图片** `replace-image`（回退图标：`excel/copy.svg`）：文件选择器选择新图片并局部替换原行内图片。
- [x] **删除图片** `delete-image`（回退图标：`excel/trash-2.svg`）：安全删除选中的行内图片，同步清除选择态。
- [x] **调整大小** `resize-image`（回退图标：`excel/settings-2.svg`）：通过统一 `ModalDialog` 弹窗输入图片英寸宽度并等比缩放。
- [x] **向左旋转 90°** `rotate-left`（回退图标：`excel/undo-2.svg`）：修改 DrawingML 图片旋转角 -90°。
- [x] **向右旋转 90°** `rotate-right`（回退图标：`excel/redo-2.svg`）：修改 DrawingML 图片旋转角 +90°。

---

## 4. Excel 功能区逐项扫描与 TODO 清单

### 4.1 文件（6 项：已接通 5，未实现 1）

- [x] **打开**（Slot 78，图标：`folder-open.svg`，组：文件）：异步文件选择器打开 XLSX/DOCX/JSON。
- [x] **保存**（Slot 17，图标：`save.svg`，组：文件）：提交单元格草稿后异步保存 XLSX 工作簿。
- [x] **另存为**（Slot 79，图标：`file-down.svg`，组：文件）：异步文件选择器另存为新 XLSX。
- [ ] **打印**（Slot 80，图标：`printer.svg`，组：文件）：点击底栏提示““打印”功能暂未实现”。
- [x] **导出**（Slot 81，图标：`file-output.svg`，组：文件）：触发系统保存对话框，将当前活动 Sheet 网格渲染为 PDF 并原子写盘。
- [x] **共享**（Slot 82，图标：`share-2.svg`，组：文件）：提交单元格草稿后打开文件选择器导出独立 XLSX 副本，带原子替换与防篡改 Guard 校验，不改变当前文档会话、路径与脏状态。

### 4.2 开始（24 项：已接通 21，未实现 3）

- [x] **剪切**（Slot 3，图标：`scissors.svg`，组：剪贴板）：复制选区至剪贴板并清空单元格。
- [x] **复制**（Slot 2，图标：`copy.svg`，组：剪贴板）：将选区单元格内容制表符格式化写入系统剪贴板。
- [x] **粘贴**（Slot 21，图标：`clipboard-paste.svg`，组：剪贴板）：读取系统剪贴板并批量填入网格区域。
- [x] **格式刷**（Slot 29，图标：`paintbrush.svg`，组：剪贴板）：点击提取当前选中单元格全量样式（字体、字号、粗斜体、颜色、对齐、换行、数字格式、边框等），切换光标为准星态并高亮按钮；用户在网格中点击或拖拽选择目标区域释放后，原子应用格式，记录撤销重做快照；支持 Esc 或二次点击取消。
- [x] **字体**（Slot 30，图标：`type.svg`，组：字体）：弹出统一模态输入框修改选中单元格字体族名称，实时应用并受撤销重做保护。
- [x] **加粗**（Slot 4，图标：`bold.svg`，组：字体）：切换选中单元格加粗样式。
- [x] **斜体**（Slot 5，图标：`italic.svg`，组：字体）：切换选中单元格斜体样式。
- [x] **下划线**（Slot 6，图标：`underline.svg`，组：字体）：切换选中单元格下划线样式。
- [x] **左对齐**（Slot 31，图标：`format_align_left.svg`，组：对齐）：直接将选中单元格设为左对齐（若已为左对齐则切换回常规对齐），支持高亮态与混合态。
- [x] **居中**（Slot 9，图标：`format_align_center.svg`，组：对齐）：弹出对齐选择器（常规/左对齐/居中/右对齐）并应用。
- [x] **右对齐**（Slot 32，图标：`format_align_right.svg`，组：对齐）：直接将选中单元格设为右对齐（若已为右对齐则切换回常规对齐），支持高亮态与混合态。
- [x] **自动换行**（Slot 33，图标：`text-wrap.svg`，组：对齐）：切换选中单元格自动换行（`WrapText`）属性，底层写入 Excelize Alignment 并支持撤销重做。
- [x] **百分比**（Slot 34，图标：`percent.svg`，组：格式）：应用百分比格式（`0.00%`），若已为百分比则切换回常规格式，底层写入 Excelize NumFmt 并支持撤销重做。
- [x] **数字格式**（Slot 10，图标：`hash.svg`，组：格式）：弹出格式选择器（常规/整数/两位小数/日期）并应用。
- [x] **插入**（Slot 12，图标：`square-plus.svg`，组：单元格）：在当前活动行前插入 1 行。
- [x] **删除**（Slot 13，图标：`trash-2.svg`，组：单元格）：删除当前活动行。
- [x] **条件格式**（Slot 35，图标：`sparkles.svg`，组：单元格）：通过统一 `ModalDialog` 弹窗支持为选区配置预设条件格式规则（渐变数据条 data_bar、三色色阶 3_color_scale、高亮重复值 duplicate、高于平均值 above_average）或一键清除规则，底层调用 Excelize 写入并受完整撤销重做（Undo/Redo）与历史快照保护。
- [x] **表格格式**（Slot 36，图标：`table.svg`，组：单元格）：通过统一 `ModalDialog` 弹窗选择中等风格、浅色风格或深色风格，在当前选区自动创建格式化表格并规范首行标题，受撤销重做（Undo/Redo）与历史快照保护。
- [x] **求和**（Slot 37，图标：`sigma.svg`，组：编辑）：对选区内连续数值进行自动求和（支持多列底部填入、单行右侧填入以及单单元格向上/向左探查），结果自动批量写入并受撤销重做保护。
- [x] **排序**（Slot 18，图标：`arrow-up-down.svg`，组：编辑）：按当前选区活动列升序排序。
- [x] **筛选**（Slot 20，图标：`list-filter.svg`，组：编辑）：切换当前活动列值筛选下拉框。
- [x] **查找**（Slot 38，图标：`search.svg`，组：编辑）：弹出统一模态输入框，在当前工作表中全文不区分大小写查找文字，循环定位并高亮滚动至目标单元格。
- [x] **撤销**（Slot 0，图标：`undo-2.svg`，组：历史）：撤销上一步网格编辑或格式修改。
- [x] **重做**（Slot 1，图标：`redo-2.svg`，组：历史）：重做下一步操作。

### 4.3 插入（9 项：已接通 8，未实现 1）

- [x] **表格**（Slot 39，图标：`table.svg`，组：插入）：通过统一 `ModalDialog` 弹窗在当前选区插入结构化表格，支持多套预设表格样式，受撤销重做（Undo/Redo）与历史快照保护。
- [ ] **数据透视表**（Slot 40，图标：`layout-grid.svg`，组：插入）：未实现。
- [x] **图片**（Slot 25，图标：`image.svg`，组：插入）：文件选择器选择图片，作为浮动对象插入工作表。
- [x] **形状**（Slot 41，图标：`shapes.svg`，组：插入）：通过统一 `ModalDialog` 弹窗支持在指定单元格位置插入标准几何形状（圆角矩形 roundRect、矩形 rect、椭圆 ellipse、箭头 rightArrow、五角星 star5），底层调用 Excelize AddShape 写入并受撤销重做与历史快照保护。
- [x] **柱状图**（Slot 42，图标：`chart-column.svg`，组：图表）：通过统一 `ModalDialog` 弹窗基于选区范围自动解析分类轴与数据序列，在工作表指定位置插入标准柱状图（`excelize.Col`），支持图表标题自定义，受撤销重做与历史快照保护。
- [x] **折线图**（Slot 43，图标：`route.svg`，组：图表）：通过统一 `ModalDialog` 弹窗基于选区范围插入标准趋势折线图（`excelize.Line`），支持图表标题自定义，受撤销重做与历史快照保护。
- [x] **饼图**（Slot 44，图标：`chart-pie.svg`，组：图表）：通过统一 `ModalDialog` 弹窗基于选区范围插入标准占比饼图（`excelize.Pie`），支持图表标题自定义，受撤销重做与历史快照保护。
- [x] **链接**（Slot 45，图标：`link.svg`，组：链接）：通过统一 `ModalDialog` 双输入弹窗为当前选中单元格设置超链接（外部 URL 或跨工作表单元格引用）及提示文本；若单元格为空自动填充链接文本，支持移除现有超链接与撤销重做。
- [x] **文本框**（Slot 46，图标：`text-cursor-input.svg`，组：文本）：通过统一 `ModalDialog` 弹窗在工作表指定单元格位置插入带有富文本段落的矩形文本框，底层调用 Excelize AddShape 写入并受撤销重做与历史快照保护。

### 4.4 页面布局（6 项：全部已接通，未实现 0）

- [x] **页边距**（Slot 47，图标：`panel-left.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗支持标准预设（普通 0.75/0.7"、窄 0.75/0.25"、宽 1.0"）与自定义边距输入（0–20 英寸），写入工作表页面边距，受撤销重做与历史快照保护。
- [x] **纸张方向**（Slot 48，图标：`rotate-cw.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗选择纵向（Portrait）或横向（Landscape），更新工作表打印页面属性，支持撤销重做。
- [x] **纸张大小**（Slot 49，图标：`file.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗选择标准纸张规格（A4、Letter、A3），更新工作表纸张编号，受撤销重做保护。
- [x] **打印区域**（Slot 50，图标：`scan.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗支持将当前选区设为打印区域（如 $A$1:$D$10）或一键清除打印区域，底层通过 excelize 定义名称 `_xlnm.Print_Area`（带工作表作用域）写入，受撤销重做与历史快照保护。
- [x] **网格线**（Slot 51，图标：`printer-check.svg`，组：页面设置）：切换工作表网格线显隐与打印网格线属性，按钮高亮与状态栏实时反馈；通过 excelize SetSheetView 同步工作表属性并受撤销重做保护；动态刷新单元格绘制缓存。
- [x] **标题行**（Slot 52，图标：`heading.svg`，组：页面设置）：通过统一 `ModalDialog` 弹窗支持将当前选中行设为打印顶端重复标题行（如 $1:$10）或一键清除现有标题行，底层通过 excelize 定义名称 `_xlnm.Print_Titles`（带工作表作用域）写入，受撤销重做与历史快照保护。

### 4.5 公式（8 项：全部已接通，未实现 0）

- [x] **插入函数**（Slot 53，图标：`sigma.svg`，组：函数库）：通过统一 `ModalDialog` 弹窗支持标准函数模板（SUM、AVERAGE、COUNT、MAX、MIN、IF）智能识别选区坐标并插入公式，驱动网格重新计算与脏状态跟踪，受完整撤销重做（Undo/Redo）与历史快照保护。
- [x] **自动求和**（Slot 37，图标：`sigma.svg`，组：函数库）：与“开始”标签中的求和逻辑共用，智能计算并写入求和结果。
- [x] **最近使用**（Slot 54，图标：`clock-3.svg`，组：函数库）：弹出常用统计/查找函数插入弹窗并自动填入当前选区引用。
- [x] **逻辑函数**（Slot 55，图标：`git-compare-arrows.svg`，组：函数库）：弹出逻辑函数快捷插入弹窗（IF、AND、OR 等）。
- [x] **文本函数**（Slot 56，图标：`type.svg`，组：函数库）：弹出文本函数快捷插入弹窗（CONCATENATE、LEFT、RIGHT 等）。
- [x] **日期时间**（Slot 57，图标：`clock-3.svg`，组：函数库）：弹出日期时间函数快捷插入弹窗（TODAY、NOW 等）。
- [x] **名称管理器**（Slot 58，图标：`tag.svg`，组：定义名称）：通过统一 `ModalDialog` 弹窗展示已有定义名称数量并为当前选区创建或修改命名区域（Defined Name），底层通过 excelize 定义名称管理，受撤销重做与历史快照保护。
- [x] **重新计算**（Slot 59，图标：`rotate-cw.svg`，组：计算）：一键重新计算全工作簿公式链与关联数值，驱动网格视口与绘制缓存局部刷新。

### 4.6 数据（7 项：全部已接通，未实现 0）

- [x] **刷新全部**（Slot 60，图标：`rotate-cw.svg`，组：数据工具）：重新评估全部外部关联与计算结果，重置绘制缓存并刷新网格。
- [x] **排序**（Slot 18，图标：`arrow-up-down.svg`，组：排序和筛选）：按当前列排序。
- [x] **筛选**（Slot 20，图标：`list-filter.svg`，组：排序和筛选）：切换筛选框。
- [x] **分列**（Slot 61，图标：`columns-3.svg`，组：数据工具）：通过统一 `ModalDialog` 弹窗支持逗号、分号、空格、制表符预设及任意自定义分隔符向右拆分数据，写入单元格并动态反馈处理行数与扩展列数，受完整撤销重做（Undo/Redo）与历史快照保护。
- [x] **快速填充**（Slot 62，图标：`wand-sparkles.svg`，组：数据工具）：智能识别数字序列递增步长、带编号文本（如“Item 1”）及首行数据规律，一键向下推断并快速填充至选区空白行，受完整撤销重做（Undo/Redo）与历史快照保护。
- [x] **删除重复项**（Slot 63，图标：`trash-2.svg`，组：数据工具）：通过统一 `ModalDialog` 弹窗支持按活动列或整行全部列快速排重，倒序移除重复行防止坐标偏移，受完整撤销重做（Undo/Redo）与历史快照保护。
- [x] **数据验证**（Slot 64，图标：`circle-check.svg`，组：数据工具）：通过统一 `ModalDialog` 弹窗支持为选区配置下拉列表约束（用逗号分隔备选值）或留空一键清除验证，通过 excelize AddDataValidation / DeleteDataValidation 写入并受撤销重做与历史快照保护。

### 4.7 审阅（6 项：全部已接通，未实现 0）

- [x] **拼写检查**（Slot 65，图标：`spell-check-2.svg`，组：校对）：通过统一 `ModalDialog` 弹窗扫描选区或工作表单元格，校验拼写、特殊字符与格式规范并报告检查结果。
- [x] **翻译**（Slot 66，图标：`languages.svg`，组：校对）：通过统一 `ModalDialog` 弹窗支持查看当前单元格内容并快速输入翻译译文，一键应用至活动单元格并受撤销重做与历史快照保护。
- [x] **新建批注**（Slot 67，图标：`message-square-plus.svg`，组：批注）：通过统一 `ModalDialog` 弹窗为活动单元格添加或修改富文本批注，通过 excelize AddComment 写入并受撤销重做与历史快照保护。
- [x] **删除批注**（Slot 68，图标：`message-square-x.svg`，组：批注）：一键删除活动单元格的批注，通过 excelize DeleteComment 移除并受撤销重做与历史快照保护。
- [x] **保护工作表**（Slot 69，图标：`lock-keyhole.svg`，组：保护）：通过统一 `ModalDialog` 弹窗支持设置保护密码或无密码保护工作表，并支持已有保护工作表的安全撤销验证，通过 excelize ProtectSheet / UnprotectSheet 写入并受撤销重做与历史快照保护。
- [x] **保护工作簿**（Slot 70，图标：`shield-check.svg`，组：保护）：通过统一 `ModalDialog` 弹窗支持设置保护密码或无密码保护工作簿结构，并支持已有保护工作簿的安全撤销验证，通过 excelize ProtectWorkbook / UnprotectWorkbook 写入并受撤销重做与历史快照保护。

### 4.8 视图（7 项：全部已接通，未实现 0）

- [x] **普通视图**（Slot 71，图标：`layout-grid.svg`，组：工作簿视图）：一键切换当前工作表为常规网格视图（`normal`），按钮高亮与状态栏实时反馈，通过 excelize SetSheetView 写入并受撤销重做保护。
- [x] **分页预览**（Slot 72，图标：`file-spreadsheet.svg`，组：工作簿视图）：一键切换当前工作表为分页预览视图（`pageBreakPreview`），按钮高亮与状态栏实时反馈，通过 excelize SetSheetView 写入并受撤销重做保护。
- [x] **标尺**（Slot 73，图标：`ruler.svg`，组：显示）：一键切换当前工作表标尺显示状态，按钮高亮与状态栏实时反馈，通过 excelize SetSheetView 写入并受撤销重做保护。
- [x] **网格线**（Slot 74，图标：`grid-3x3.svg`，组：显示）：切换工作表网格线显隐，按钮高亮与状态栏实时反馈；通过 excelize SetSheetView 同步工作表属性并受撤销重做保护；动态刷新单元格绘制缓存。
- [x] **放大**（Slot 75，图标：`zoom-in.svg`，组：缩放）：网格视口缩放比例递增 15%（上限 250%），自适应重排并于状态栏实时反馈当前百分比。
- [x] **100%显示**（Slot 76，图标：`maximize.svg`，组：缩放）：将网格视口缩放比例一键还原至 100% 原始尺寸。
- [x] **冻结窗格**（Slot 77，图标：`panels-top-left.svg`，组：窗口）：通过统一 `ModalDialog` 弹窗支持冻结首行、冻结首列、冻结至当前选区或取消冻结，通过 excelize SetPanes 同步工作表属性并受撤销重做保护。

### 4.9 Excel 图片上下文功能区（3 项：全部已接通）

- [x] **替换图片**（Slot 26，图标：`file-output.svg`，组：图片）：重新选择图片文件并替换当前选中的浮动图片。
- [x] **删除图片**（Slot 27，图标：`trash-2.svg`，组：图片）：删除当前选中的浮动图片。
- [x] **设置宽度**（Slot 28，图标：`settings-2.svg`，组：尺寸）：弹出统一 `ModalDialog` 输入 1–10000 像素宽度等比缩放。

---

## 5. 后续任务推进标准与路线图（Roadmap）

为了保证功能推进质量、避免功能“回退”或“仅有 UI 假按钮”，后续所有 Ribbon 开发任务均按以下标准执行：

### 5.1 验收基准（Acceptance Criteria）
1. **图标资产合规**：
   - 必须通过 `TestEveryWordToolbarActionHasSVGAsset`、`TestEveryExcelToolbarGlyphHasSVGAsset` 和 `TestEveryBundledSVGIconRendersInGioImageFormat` 测试。
   - 所有新增或调整的图标必须是独立 SVG，严禁硬编码依赖外部或已被删除的雪碧图。
2. **状态感知与保护**：
   - 会修改文件的操作在未就绪时必须处于禁用态（Word 置灰且无点击事件，Excel 给出明确未实现反馈）。
   - 包含文本输入的编辑操作必须在执行前 flush 暂存草稿，防止丢失用户正在输入的内容。
3. **安全保存闭环**：
   - 所有写入操作必须通过“临时文件原子写入 -> 保存重开验证 -> 确认无误替换原文件”的安全通道，确保异常断电或格式错误时不损坏原文档。

### 5.2 优先级规划

#### P0：高频编辑体验与核心闭环补齐
- **Word 端**：
  - [x] 接通 `段前`（`layout-before`）与 `段后`（`layout-after`）间距的 Ribbon 弹窗输入，连接已有 DOCX 适配器并支持撤销重做（已完成）。
  - [x] 完成 `view-new-window`（新建窗口）的多桌面窗口原生端对端自动化验收（已完成）。
- **Excel 端**：
  - [x] 接通“开始”标签中的高频缺失项：`左对齐`、`右对齐`、`自动换行`（Text Wrap）、`百分比格式`（已完成）。
  - [x] 接通“编辑”分组中的 `自动求和`（SUM 选区连续数值计算填入）与 `查找`（工作表文字匹配与单元格滚动定位）（已完成）。
  - [ ] 接通格式刷（`paintbrush`）与单元格边框独立按钮直达。

#### P1：结构与插入能力补齐
- **Word 端**：
  - 增加基础表格插入与行列增删能力（从只读展示进阶到可插入）。
  - 增加超链接（`insert-link`）插入与展示。
- **Excel 端**：
  - 接通“插入”标签中的基础形状或超链接。
  - 接通“数据”标签中的常用数据工具（如简单的 `删除重复项`）。

#### P2：版面与样式系统深化
- **Word 端**：
  - 接通“布局”标签的 `纸张方向` 与 `纸张大小`，与分页引擎贯通联动。
  - 扩展主题与整体段落间距调整。
- **Excel 端**：
  - 接通“页面布局”中的 `网格线显示开关` 与 `纸张方向`。
  - 接通“视图”中的 `冻结窗格`（首行/首列冻结）与 `网格线开关`。

#### P3：高级功能与协同/复杂格式
- Word/Excel 批注（Comments）、保护编辑（Restrict Editing）、公式库深入解析。

---

## 6. 代码与状态依据索引

- **图标与资产**：
  - `internal/desktopui/assets/icons/word/*.svg`（99 个 Word SVG）
  - `internal/desktopui/assets/icons/excel/*.svg`（67 个 Excel SVG）
  - `internal/desktopui/svg_icons.go`（矢量光栅化、着色与缓存核心）
  - `internal/desktopui/svg_icon_assets_test.go`（图标资产覆盖率与渲染回归）
  - `internal/desktopui/word_ribbon_icons.go`（Word 图标与回退逻辑）
  - `internal/desktopui/ribbon_chrome.go`（Excel 图标映射与工具栏组件）
- **Word Ribbon 实现**：
  - `internal/desktopui/word_toolbar.go`（Word action 定义、启用逻辑与事件分发）
  - `internal/desktopui/word_ribbon_layout.go`（Word 标签栏与命令按钮 Gio 布局）
  - `internal/desktopui/word_page_interaction.go` / `word_page_presentation.go`（统一查看与编辑渲染）
  - `internal/desktopui/word_new_window.go`（多窗口会话分发）
  - `internal/desktopui/confirm_dialog.go`（统一模态弹窗系统）
- **Excel Ribbon 实现**：
  - `internal/desktopui/layout_spreadsheet_toolbar.go`（Excel 8 常驻标签、插槽定义与事件处理）
  - `internal/desktopui/spreadsheet_commands.go`（Excel 核心格式/排序/筛选/行列命令）
  - `internal/desktopui/spreadsheet_pdf_export.go`（Excel PDF 导出调度）
  - `internal/format/pdf/spreadsheet.go`（Excel PDF 纯 Go 矢量渲染导出）
  - `docs/excel_menu.json`（Excel 菜单与按钮标准参考）
