# PPTX 共用对象操作框与富文本编辑组件实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**状态：** 后续实施方案，本次提交不执行。当前编辑排版问题仍未解决，不能将此文档或既有局部回归视为修复证据。

**Goal:** 文字、图片、形状共用对象操作框；文字预览和编辑共用排版结果，进入编辑后字体、字号、间距、换行、位置保持不变。

**Architecture:** 外层 `PresentationObjectFrame` 负责对象选择、八点缩放、移动和旋转。内层 `PresentationTextBox` 负责富文本排版、只读显示、输入与光标。Gio 组件只消费中立模型并发出意图；controller 管理草稿与提交，PPTX adapter 负责结构保留和保存。

**Tech Stack:** Go 1.27、Gio 0.10.2、go-pptx v2.0.3、现有中立文档模型与定点 DrawingML 修补路径。

**Spec:** 本文的“交互约定”“排版与数据约定”为本轮后续设计；结合 `docs/architecture.md`、`2026-10-08-pptx-native-object-editing.md`、`../specs/2026-10-08-pptx-text-shape-style-editing-design.md` 阅读。多段落支持扩展已有单段落安全编辑范围，需要新增保存证据。

## 1. 已有工作与未解决的问题

已有工作可复用：对象几何计算、八个缩放点、旋转、文本编辑时保留操作框、图片替换/裁剪、对象级样式覆盖、定点 XML 写回与保存校验。本次整理提交保留这些工作。

尚未完成：统一文本排版、多段落安全编辑、段内换行投影、混合字号/字体的编辑显示、统一光标及选区定位、完整的输入法回归。既有 `widget.Editor` 只提供一个整体字体与行高，不能直接承载 PPTX 富文本。

实际反馈文件：`/Users/mima1234/Downloads/slidetemplate.pptx`，第 1 页第二块文字 `Subtitle 3`，shape ID `4`，边界为 `{X:108, Y:306, W:504, H:138}` pt。源 XML 包含：

- 第一个 `a:p`：`Dean’ Budget Retreat`。
- 第二个 `a:p`：`School of Engineering`，一个 `a:br`，然后 `February 3, 2017`。
- 当前 adapter 投影中的第二个段落丢失 `a:br`，两个 run 文本直接拼接。当前文本保存方法拒绝多段落或特殊换行。

当前代码路径问题：

1. `presentation_canvas.go` 预览分别布局段落并额外加 2 dp 间隔；编辑将整个对象 `PlainText()` 交给单个 Editor。段落边界的距离与段内换行距离不是同一套结果。
2. 预览 Label/富文本与 Editor 使用不同换行宽度；预览有按填充/边框决定垂直居中的启发式，编辑没有。
3. 富文本预览保留多个 run 字体/字号，Editor 使用首个可见 run 的字体/字号，无法表达混合格式。
4. 光标定位额外重排一遍文本，没有直接复用最终屏幕布局。
5. 当前基线距离回归覆盖单段落富文本路径，未证明实际反馈文件的多段落路径正确。

## 2. 交互约定

- 单击对象选中；二次点击或 Enter 进入文字编辑；空占位符提示进入编辑后消失，提示不写入源文本。
- 选中与编辑的对象框使用同一份几何、同一条边框、同一组八点缩放柄和旋转柄。
- 四角与四边中点均可拖拽；图片的显式比例锁仍受尊重；旋转对象沿本地坐标缩放。
- 编辑时文本区域用于定位光标/选字；外框边缘移动对象；缩放柄与旋转柄优先命中。拖动对象不结束草稿，也不把 pointer 当作文字点击。
- 预览阶段不修改 document revision；释放提交一次；取消/no-op 不提交。保留现有 4 dp 移动阈值和 8 pt 最小尺寸。
- 缩放、滚动、Retina、旋转均在输入边界统一转换坐标。源文件存 pt/角度，不存屏幕像素。

## 3. 排版与数据约定

**“同一个组件”意味着同一份实际布局结果，不是让两个组件调用一个行距 helper。**

- `PresentationTextBox` 在只读/编辑两种模式下共用段落、run shaping、换行、基线和字符簇布局；模式只改变输入、光标与选区显示。
- 中立模型区分段落结束、段内软换行、普通 run 与字段；禁止将软换行直接当段落分隔符或丢掉。
- 段落保存对齐、行距、段前后距；run 保存字体、字号、颜色、粗斜体、下划线和 baseline。补充文本框源内边距、垂直锚定和必要的自动适配属性，移除按填充/边框猜测居中的逻辑。
- 固定 pt 行距与百分比行距统一计算；用最终字体布局决定 glyph 的 ascent/descent。不得以单行控件高度代替相邻行基线距离。
- `PresentationTextLayout` 保存行、字符簇、run 样式、baseline、字符到几何映射；绘制、HitTest、CaretRect 和选区均消费该结果。
- 缓存键包含对象文本/样式版本、可用宽度、缩放、字体环境和像素密度。光标移动不重新解析 PPTX；变换预览不写文件。
- 不能用多个独立 paragraph Editor 作为最终实现：跨段落选区、退格合并、换行、输入法和剪贴板必须有统一文档与选区状态。
- 优先复用现有字体 shaper 和输入设施。先调查 Gio Editor 的扩展接口；若不能注入富文本布局，就做独立的文本编辑状态与排版适配层。禁止靠透明普通 Editor 覆盖富文本来伪装，光标与 IME 候选位置必须准确。
- 未支持的字段、链接或 DrawingML 结构明确只读；失败保留草稿和 dirty state。不能先开放编辑再在保存时才提示不支持。

## 4. 文件职责与接口

以下为拟议接口，落地时按仓库类型命名；不得把 go-pptx 类型传入 UI。

| 文件 | 职责 |
| --- | --- |
| `internal/desktopui/presentation_object_frame.go` | `PresentationObjectFrame` 的绘制、命中注册和 persistent gesture state |
| `internal/desktopui/presentation_transform.go` | 继续复用纯几何算法 |
| `internal/desktopui/presentation_interaction.go` | 事件规范化与 transform intent，拆出组件职责 |
| `internal/desktopui/presentation_text_box.go` | `PresentationTextBox.Layout(gtx, view, mode)`，模式切换不换排版 |
| `internal/desktopui/presentation_text_layout.go` | `PresentationTextLayout`、`HitTest(point) TextPosition`、`CaretRect(position)`、选区几何 |
| `internal/desktopui/presentation_text_input.go` | 键盘、IME、剪贴板、跨段落选区及草稿命令 |
| `internal/document/presentation.go` | 软换行/段落/run 中立结构、`PresentationTextPosition` 与编辑 patch |
| `internal/desktopui/presentation_editing.go` | controller 校验、异步提交、草稿生命周期和 stale-result 防护 |
| `internal/format/pptx/text.go` | 文本结构投影、能力判断、定点文本 patch 与保存期望 |

操作框接收对象 ID、pt 边界、角度、比例锁、选中/编辑状态及内容 layout；输出 move/resize/rotate intents。文本组件输出内容 edit intents，不负责对象几何写回。组件状态存于每个 session 的稳定 view state，切页、删对象和关 tab 时释放。

## 5. Global Constraints

- 遵守 Gio → controller/session → neutral model → adapter → files；文件解析、保存和重新打开校验在工作线程执行。
- 本轮后续目标是单对象编辑；多对象选择/组合、图表/表格内部编辑不在首阶段范围。
- 不变更未请求的 XML、媒体、关系和未知属性；源文件只有通过临时文件写入与 reopen gate 才替换。
- 本地样本仅用于诊断，加入仓库前确认可以共享；可创建保留“两段落 + 段内换行”结构的匿名最小 fixture。
- 不以编译通过、字段 helper 测试或截图近似代替真实切换与保存验收。

## 6. Review Focus

- 混合字号/字体、空 run、软换行和空段落：切换模式不改变文本结构或布局。
- 缩放/Retina/滚动后旋转对象：光标和所有操作柄命中正确，不跳动。
- 跨段落删除/粘贴与输入法组合：选区和候选位置正确，不提交中间组合文本。
- 保存失败、关页或异步结果过期：保留草稿，旧结果不能覆盖新对象。
- 字段/链接/未知 XML：不能被普通文本化；不支持编辑时预先只读。

## 7. 实施顺序与验收

### Task 1：封装共用对象操作框（工作量中等）

- [ ] 为文字、图片、形状及编辑状态建立统一的八柄、旋转、边缘移动回归；覆盖旋转与 1×/2×密度。
- [ ] 抽取 `PresentationObjectFrame`，复用现有 pure transform 算法；移除 canvas 内重复 frame/gesture 注册。
- [ ] 接入三类对象；验证 click/drag 不串到文字输入，cancel/no-op 与 release revision 语义一致。
- [ ] 在支持语料上 save/reopen 核对几何和 untouched 部分，独立提交。

### Task 2：中立文本结构与安全写回（工作量较大）

- [ ] 将反馈样本结构做成匿名 fixture；先证明投影缺少软换行、多段落修改被拒绝的现状。
- [ ] 为段内换行和空段落增加中立表示；保存编辑能力单独投影，不能以“对象可选中”代替“文本可安全编辑”。
- [ ] 提供结构化文本 patch，保留未触及 run 格式与未知 XML；支持多段落、软换行和 no-op，而非全对象扁平字符串替换。
- [ ] 覆盖新增/删除段落、跨段落改字、特殊字段拒绝、SaveAs/SnapshotBytes 一致、失败保留源文件，独立提交。

### Task 3：共用布局结果（工作量较大）

- [ ] 调查 Gio 扩展边界并记录方案选择；确认 rich shaping 和 IME 能使用同一字符坐标。
- [ ] 实现 TextLayout，绘制、caret、selection 与 hit-test 共用；去除另一套 click 重排。
- [ ] 接入 TextBox 只读模式与现有预览、缩略图；按源文本框内边距和 anchor 布局。
- [ ] 为反馈 fixture 比较全部行的 baseline、run 字体/字号、换行和边界；普通/混合字体、软换行、空段落均覆盖，独立提交。

### Task 4：开启同组件输入（工作量最大）

- [ ] 在同一 TextBox 开启输入模式；进入/退出时布局缓存不变，无输入不产生 revision。
- [ ] 实现字符簇与跨段落光标/选区、删除、粘贴、undo/redo；定义新输入继承相邻 run 样式的规则。
- [ ] 接入 IME；中文组合期间文本为 transient，候选窗口锚点复用 CaretRect。
- [ ] 验证编辑中移动/八点缩放/旋转保留草稿和选区，异步提交按 session generation 防止过期写入，独立提交。

### Task 5：实际反馈验收与能力文档

- [ ] 打开 `slidetemplate.pptx` 第 1 页第二块文字：进入编辑前后全部文本、换行、对齐、字号、间距和位置一致；原文件字节不变。
- [ ] 在三行不同字符位置点入，caret 落在目标字符；中文输入、选区、软换行与段落修改可用。
- [ ] 修改副本、移动/缩放/旋转并保存重开；核对文本结构、格式、其他对象及 ZIP parts，故障写入不损坏原文件。
- [ ] 经授权执行 focused UI/adapter regressions 与全仓库 suite，记录原生交互证据；更新 `docs/office-support.md` 和兼容矩阵。
- [ ] 审核后独立提交，按用户要求合并。

## 8. 工作量与收益

对象框主要整理已有逻辑，风险和工作量中等；富文本组件涉及数据、排版、输入和安全保存，工作量较大，不能承诺一次小补丁达到原生 PPT 全部行为。先完成操作框，再完成结构/布局，最后接输入，阶段结果均可独立验收。

这项封装值得做：文字、图片、形状会持续复用操作框；共用文本布局能从结构上消除编辑切换漂移。阶段验收以真实反馈文件为门槛，避免继续累积只覆盖 helper 的局部修补。
