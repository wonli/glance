# Word 分页、打印与 PDF 设计

## 目标与约束

本设计推进 Word TODO 中的“打印”“导出 PDF”“打印布局”以及 `FsQrk` 底栏页数。Glance 是独立桌面软件：分页、页面预览和 PDF 输出在 Go 进程内完成；不得启动 LibreOffice 或其他 Office 程序。打印使用当前操作系统提供的原生打印服务，不要求安装外部 Office 软件。桌面入口保持 `go run ./cmd/desktop`，构建的应用显示名为 `desktop`。

目标分为两部分：

1. **Pen 界面复原**：保留 `Clpgm` 主页面与 `kiBim` 功能区尺寸、分组、图标和交互；按 `FsQrk` 显示真实页码、总页数与缩放状态。页码必须来自当前 DOCX 的布局结果，不能写死为“第 1 页，共 3 页”。
2. **支持范围内的页面输出**：屏幕打印布局、PDF 和打印使用同一份分页、换行、字体替代及图片位置结果。不得把连续视图截图切片当作 DOCX 分页，也不得把未支持内容静默丢弃或声称完全兼容 Microsoft Word。

当前实现依据：`internal/desktopui/word_layout.go` 在连续流中绘制 Word 段落、run、表格、行内图片和分页符；`internal/document/word.go` 没有页面设置或分页片段 DTO；`internal/format/docx/word_projection.go` 忽略 section properties；`Document.Snapshot()` 可取得不修改源文件的当前 DOCX 包快照。Gio 的页面布局没有可复用的打印分页输出。

## 方案比较与选型

### 方案 A：进程内共享分页引擎（采用）

增加独立 Word 布局服务，输入 neutral Word 文档、页面设置和字体测量服务，输出页面与定位片段。打印布局、页数、PDF 绘制及原生打印均使用该结果。Go PDF 生成库作为构建依赖随应用编译；打印通过平台 adapter 调用系统打印服务，不启动外部办公/阅读器应用。

这条路能满足独立运行和真实页数要求；实现成本最高，字体度量、Word 样式继承和表格分页需要通过样本文档逐步校准。它不承诺所有 DOCX 与 Microsoft Word 像素一致。

### 方案 B：把当前连续流页面栅格化导出

实现较快，但无法遵循 Word 的纸张尺寸、页边距和分页符语义，也不能可靠计算段落或表格跨页。它与用户要求的真实页数及 1:1 验收冲突，不采用。

### 方案 C：调用外部 Office/Web 渲染器

可以复用成熟的分页实现，但会要求用户安装、捆绑或运行外部渲染软件，违反独立软件及不依赖 LibreOffice 的约束，不采用。

## 用户行为

- “打印布局”切换到有纸张边界的多页编辑视图；连续视图仍保留为独立视图。打印布局中可以滚动浏览页面并继续编辑已支持的文字。
- 底栏的“第 N 页，共 M 页”根据焦点 run/字符位置映射到实际页面；无文本焦点时以首个可见页作为 N。M 来自同一分页结果。布局失败或文档含阻止输出的未支持结构时，底栏显示明确状态，不使用估算页数。
- 打印入口先提交当前编辑器草稿，再生成不可变快照并异步布局。成功后显示打印预览；用户再确认打开系统原生打印对话框，选择打印机和份数后才会打印。取消不改变文档或 dirty 状态。
- 导出 PDF 先提交草稿、异步布局并显示保存文件对话框。写入目标目录下的临时文件，完整关闭并校验成功后原子替换目标；失败保留既有目标文件及文档 dirty 状态。
- 每次输出固定使用触发操作时的文档版本、提交后的编辑和页面设置。若布局期间文档被继续编辑，旧结果不可覆盖新版本界面；用户可重新生成。

## 架构

```text
Gio Word UI / controller
    ├── continuous-flow view
    └── print-layout view + status page mapping
             ↓ committed edits + immutable snapshot
DOCX adapter → neutral Word content, styles and section geometry
             ↓
internal/wordlayout → pages, positioned line/object fragments and source mapping
       ├── Gio page painter
       ├── in-process Go PDF backend
       └── per-platform native print adapter
```

### Neutral Word 布局输入

- 在 `internal/document` 增加只包含格式中立值的 section/page setup 和表格布局信息。长度在模型中使用明确单位（DOCX 页面属性以 twips/EMU 读写；布局输出使用 point），避免保存屏幕像素。
- 输入包括有序段落/run/分页符、表格网格与单元格、行内图片引用、列表标记、支持的直接 run/paragraph 属性、由 `styles.xml` 与 `docDefaults` 解析出的有效样式，以及每个 section 的纸张尺寸、方向和页边距。
- DTO 不暴露 XML/ZIP、Gio widget 或 PDF 库类型。unknown OOXML 数据仍留在 DOCX 包中；布局过程只读取快照，不写回文档。
- 无显式页面属性时使用产品内确定的默认纸张与页边距，并在 layout diagnostics 标明采用了默认值。诊断分为可继续的 warning 与阻止输出的 error：字体 fallback 和可定位对象占位是 warning，需在预览中说明并由用户确认后才可输出；未知页面几何、多节分页等会造成错误分页的情况是 error，显示具体缺口并禁用打印/PDF，不生成看似完整的占位 PDF。

### `internal/wordlayout`

- 纯布局服务消费 neutral 数据和一项文本整形/度量接口，不依赖 Gio、DOCX adapter 或 PDF 包。具体文本实现从应用已使用的 Go/Gio 字体与 shaping 能力取得字形、宽度、基线和 CJK 换行机会；应用运行时不调用外部排版程序。
- 结果包含有序页面、页面矩形、段落行、定位文字片段、图片/表格片段，以及 `runID + Unicode 字符偏移 → 页面坐标`、对象 ID 到页面坐标的映射。PDF、打印和页面视图不得再次独立决定换行或分页。
- 页面算法处理：页边距、方向和纸张尺寸；段落对齐、已支持缩进/列表/段前段后与行高；直接字符格式和基础样式继承；中文/西文混排换行；inline 图片；普通表格按行布局和跨页；显式分页符与支持的段落分页控制。
- 页面分页和文本 shaping 在后台工作线程执行。布局输入有版本号与取消信号；UI 只在匹配当前 document ID、revision 和 page setup 时应用结果。页面视口按可见页绘制，不在每帧重算整篇文档。
- 布局返回结构化诊断（severity、对象/section 定位、默认值、替代字体、未知内容、溢出、未支持 section）；调用方按 severity 决定允许预览、需用户确认或阻止输出，不得只检查错误字符串来推断成功。

### PDF 与系统打印

- PDF writer 使用随 Go 可执行文件构建的库；库的选型须先在 `docs/office-library-evaluation.md` 记录许可证、CJK 字形/ToUnicode、字体嵌入、字形定位、透明图像、页数和跨平台构建证据。库不得要求用户安装单独程序或在运行时下载组件。
- PDF 逐页重放布局服务给出的几何与字形/对象位置，使用支持的字体子集嵌入；字体授权不允许嵌入时，明确提示字体替代并使用兼容 fallback。PDF 页数、文字换行、图片矩形必须与打印布局结果一致。
- 平台打印 adapter 只调用操作系统打印 API 和系统打印服务。禁止通过 `exec` 启动 LibreOffice、浏览器、系统 PDF 阅读器或命令行打印程序。原生打印能力逐平台实现并验证；平台 adapter 未通过验证的平台上禁用打印入口，PDF 导出仍可独立提供。
- 新增平台代码按文件/构建标签隔离；Go 主 UI、neutral model 和 layout 核心不直接调用平台 API。

## 初版支持范围与明确边界

### 初版需支持

- 单 section 的常见 DOCX 页面尺寸、方向和页边距；常用 Normal/继承样式及直接字符/段落格式。
- 当前 neutral model 支持的段落、中文和西文 run、常见项目符号/十进制列表、分页符、行内 PNG/JPEG 图片。
- 普通表格网格、单元格段落、换行和按整行分页；不能放入页面的超高表格行作为布局诊断并在预览中显示溢出提示。
- 页面定位、真实页数、打印布局编辑视图、原子 PDF 导出和经平台 adapter 的用户确认式打印。

### 初版明确不宣称支持

- 多节不同页面设置、分栏、页眉/页脚、动态 PAGE/NUMPAGES 字段、脚注/尾注。
- 浮动 DrawingML 对象及环绕、文本框/形状/图表、OMML 公式、批注与修订。
- 复杂表格合并/嵌套和重复标题行；无法完整解析的内容须给出明确诊断/占位，不能静默漏绘。
- 与所有 Microsoft Word 版本在任意字体、打印机驱动和主题下完全一致。UI 的 1:1 指 Pen 设计尺寸/位置/图标/状态；文档排版一致性仅按声明支持范围和样本文档验收。

## 失败处理与文档安全

- 打印、PDF 和分页只消费已经 flush 的编辑草稿及 `Document.Snapshot()` 得到的内存包数据，不写入源 DOCX，也不改变撤销栈或保存状态。
- snapshot、字体解析、布局、PDF 写入或原生打印任一失败时保留源文档和 dirty 状态，显示具体失败阶段。不能悄悄改为截图输出或静态页数。
- 超出支持范围的内容要能定位到对象/section。可安全定位的对象用带说明的占位并要求用户确认后再输出；若缺口会漏掉实质内容或错误分页，则阻止输出并指出需要移除/转换的对象。
- 暂存文件限定在目标同目录，使用原子替换；打印的临时内容在成功/取消/错误后删除。
- 保持 Office 包未知部件字节和原路径不受布局操作影响。保存功能启用门槛仍按现有 round-trip 安全策略执行。

## 验收与证据

### 自动化布局/格式验证

- 表格驱动的布局测试覆盖：精确页边界、段落跨页、分页符、页边距/方向/纸张尺寸、左/居中/右/两端对齐、缩进与段距、字号/粗体/斜体变化、中文/西文混排、软换行、inline 图片、常见列表、普通表格跨页和超高行诊断。
- 对同一 neutral 输入断言页面数、片段坐标、来源 run 字符范围、图片尺寸和段落顺序；禁止负尺寸、页面外无诊断的内容、丢失/重复字符和重叠片段。
- PDF 回归校验页数、嵌入字体/ToUnicode、每页文字提取、边界文本和图片矩形；生成的 PDF 用独立渲染器做像素/几何回归，不将该渲染器作为产品运行依赖。
- DOCX adapter fixtures 覆盖 section/page setup/style 默认解析；布局前后对 snapshot 做字节/包部件比较，证明无文档变更。
- 平台打印验收验证取消、确认、页数、份数、打印失败提示和临时文件清理；无真实打印机时使用操作系统提供的虚拟打印目标。

### Computer Use 与 Pen 验收

- 用指定命令 `go run ./cmd/desktop` 启动当前仓库构建；不安装或调用 LibreOffice。CUA 并排对照 Pen 节点 `Clpgm`、`kiBim`、`FsQrk` 与 Glance 主窗口。
- 在 `Clpgm` 主尺寸和窄窗口核对功能区/页面边距/多页纵向滚动/打印布局切换；核对 `kiBim` 图标沿用 Excel sprite 与状态色；核对 `FsQrk` 左右项、真实焦点页码/总页数、字数和缩放控制位置。
- 对照截图使用相同窗口尺寸、系统缩放、文档内容及可用字体。Pen 中的“第 1 页，共 3 页”和“582 个字”作为参考数据；实现不得写死这些字符串，需由相同源文档重新计算后比对。
- 验收结果记录实际命令、页面数、字体替代/unsupported diagnostics、截图位置和偏差；未达到时保留功能状态为未完成并更新 TODO，不以应用能启动代替 1:1 视觉通过。

## 文档维护

实现时逐阶段更新 `docs/word-ribbon-todo-zh.md`、`docs/office-support.md`、`docs/office-library-evaluation.md` 与兼容性矩阵。Print、PDF、打印布局、真实页数只有在各自 UI 路径、布局/输出、错误处理、对应回归与 CUA 可视验收具备证据后才改为已完成。
