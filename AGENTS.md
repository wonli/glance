# Repository Guidance

## Worktree Policy

- Do not create, add, move, or relocate a Git worktree unless the user explicitly authorizes that action for the task. Without explicit authorization, continue in the current authorized workspace.
- Before editing, inspect the current working directory and existing worktrees. If the user or task context specifies a worktree, perform all edits, builds, tests, and launches in that worktree.
- Never silently switch to another checkout, edit the primary checkout while working in a designated worktree, or copy, patch, restore, or clean changes across checkouts. Do not remove a worktree or alter its existing changes unless explicitly authorized.

## Architecture

- Follow [docs/architecture.md](docs/architecture.md): Gio UI -> session/controller -> neutral document model -> format adapters -> local files.
- Keep third-party Office library types behind format adapters. UI widgets consume neutral view data and send intents through the controller.
- Keep the Gio window loop responsive. Perform parsing and saves outside layout/frame code and publish results through synchronized application state.
- Keep source files focused; split layout, event handling, state, and format operations by responsibility as they grow.

## Gio UI

- Persistent widgets (`widget.Editor`, `widget.List`, `widget.Clickable`) belong in stable state, not as throwaway values constructed on every frame.
- Set the axis of every `widget.List` explicitly. Editors in vertical list rows use the full available width; reset inherited minimum constraints for small badges/chips.
- Keep layout/drawing separate from event handling and controller operations. Normalize pointer coordinates at the input boundary before using them in drag or selection math.
- For Excel row-height/column-width dragging, reuse `../mysql-ai/docs/drag-patterns.md`: keep preview geometry in grid coordinates, keep the resize cursor active over the preview, expand preview bounds past the old grid edge, rebase anchors when coordinate spaces change, and commit dimensions only on release. Convert pixels to file-format units through one tested helper; do not persist pixels directly.
- Dialog drafts must receive the latest editor text before confirm/close. Track committed document values separately from transient editor drafts.
- For spreadsheet editing, preserve geometry while entering edit mode, make no-op commit a no-op, support cancel, and avoid committing intermediate IME composition text. Cover empty and styled cells and Chinese input in interaction regressions.
- See [docs/architecture.md](docs/architecture.md) and the sibling reference `../mysql-ai/docs/gio-form-layout-gotchas.md` for rationale and examples.
- The approved XLSX virtual-grid design is [docs/superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md](docs/superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md).

### 设置页修复经验（2026-10-12）

本次涉及独立设置导航（设计节点 `ug773`）、AI 提供商输入框，以及 MCP 页底部本机会话提示（设计节点 `q8tkFY`）。以下规则适用于后续 Gio 菜单、表单和 SVG 图标修改。

#### 1. 菜单图文错位和样式不一致

- **现象：** 设置页标题、子菜单的图标和文字大小与主页不同；图标与文字的可见内容没有处于同一条水平中轴线上。
- **产生原因：** 设置页单独指定了更小的字号和图标尺寸，并使用独立的固定高度、居中包装。它没有复用主页的文字光学居中处理。
- **根因：** Gio 居中的是控件报告的布局矩形，而非字形实际可见像素。字体的 ascent、descent 和行高留白可能不对称，因此 `layout.Middle` / `layout.Center` 并不保证文字墨迹中心与图标中心一致。父级最小高度继续传给标签时，还会影响标签报告的尺寸。
- **修复方式：** 设置菜单使用与主页相同的主题字体、`homeTextLabel` 和 `homeIcon`；导航图标使用 20dp、文字使用 15sp。按设计恢复侧栏间距和菜单行高，并复用 `homeTextOpticalCenterTopInset` 对文字做光学居中。
- **下次注意：** 先对照同仓库中已经正确显示的主页导航，统一字体、字重、图标尺寸、颜色和间距。区分行容器的固定高度与子控件的自然高度；需要时清除子控件继承的 `Constraints.Min.Y`。光学偏移依赖字体和字号，不能将当前 6dp 偏移未经验证地套到所有字体或多行文本上。换行后也要重新检查图文对齐。

#### 2. 输入框文字未垂直居中

- **现象：** API Key、API 地址和默认模型输入框中的文字、占位文字或掩码文字偏上。
- **产生原因：** 44dp 的输入框容器把相同的最小、最大高度继续强制传给内部 `material.Editor`，再用 `layout.Stack{Alignment: layout.W}` 包装。
- **根因：** 编辑器本身已经报告占满 44dp 的尺寸，外部 Stack 没有剩余空间可以移动它；编辑器内部仍按文本行的布局位置绘制字形。仅增加一个“居中”容器无法修复已经被固定高度撑满的文本控件。
- **修复方式：** 保留外部 44dp 输入框几何，清除内部编辑器的最小高度，让其按文本行自然高度布局；由水平 Flex 的 `layout.Middle` 配合主页文字光学偏移完成居中，同时保留横向可用宽度。
- **下次注意：** 沿容器 → Inset → Flex/Stack → Editor 检查约束传递，不要只修改最外层 Alignment。必须分别检查空值占位、已有英文、中文和密码掩码；文字居中不应改变输入框宽高、右侧按钮位置或编辑器持久状态。

#### 3. 居中修复引入的输入框点击区域缩小

- **现象：** 文字居中后，点击输入框中部可以编辑，但点击上下留白无法聚焦。这是本次修复过程中由独立审查发现的交互回归。
- **产生原因：** 清除编辑器最小高度后，它的布局尺寸缩小为文本行高度。
- **根因：** Gio `widget.Editor` 将输入事件区域裁剪到自身的可见尺寸。外部输入框虽然仍绘制为 44dp，但背景绘制不会自动扩大编辑器的命中区域；视觉边界与交互边界变得不一致。
- **修复方式：** 使用 `settingsFieldFocus` 为整个输入框注册指针区域，使用稳定的 editor 对象作为事件 tag，并通过 `key.FocusCmd` 将空白区域的点击转为编辑器聚焦。内部编辑器仍保留自然高度和原生选取、拖动处理；右侧密钥显示按钮保留自己的命中区域。
- **下次注意：** 缩小 Editor 的布局尺寸或增加居中、内边距时，必须同步核对命中区域。不要为此每帧创建新的 Clickable/Editor，也不要通过扩大文本高度重新引入文字偏移。验证上沿、中部、下沿点击后可以聚焦并输入中文，还要检查文本选取、拖动及右侧按钮是否被外层事件区域截获。

#### 4. 黑色图标发灰、选中图标颜色异常、Retina 描边过细

- **现象：** 图标虽传入黑色或正确的导航颜色，实际显示仍很浅；选中的子菜单图标和底部盾牌图标尤其明显。
- **产生原因：** 原先先在固定 96px 画布上栅格化 SVG，再缩小到菜单图标尺寸；使用的 `oksvg.SetTarget` 缩放路径几何，却没有同步缩放 `SVGPaths[i].LineWidth`。
- **根因：** 源 SVG 的 1.8 单位描边在 96px 栅格里仍只有约 1.8px，随后再被缩小，抗锯齿和采样让描边显得发灰。这是描边几何与像素密度不匹配，不是简单的前景色设置错误。仅按实际像素尺寸栅格化仍不完整：Retina 下路径变大而描边不变，逻辑描边依然会偏细。
- **修复方式：** `homeIcon` 按 `gtx.Dp(size)` 得到实际目标像素尺寸，调用 `svgIconTintAtSize`；`widget.Image.Scale` 按逻辑尺寸与实际图像宽度计算。`renderSVGIcon` 对 `home` 家族在绘制前将 `LineWidth` 乘以目标像素尺寸与 viewBox 尺寸的比例，使路径和描边一起缩放。
- **下次注意：** 图标颜色不对时，同时检查 tint、SVG 原始属性、栅格尺寸、描边缩放、透明度和图像显示比例；不要只换颜色或盲目加粗。dp 与物理像素必须分清，正常屏幕和 2 倍 Retina 缩放都要验证。缓存 key 必须区分尺寸和 tint。当前描边补偿仅作用于 `home` 家族；若扩展到其他图标家族，必须先检查它们的既有渲染和截图，不能未经验证全局套用。

#### 5. MCP 本机会话提示前的图标完全缺失

- **现象：** `q8tkFY` 对应的本机会话提示文字前没有 `info` 图标。
- **产生原因：** `layoutMCPConnectionDetailsPage` 已调用 `homeIcon(..., "info", 16, settingsBlack)`，但 `assets/icons/home/info.svg` 不存在，备用工具栏图标路径也不支持这个名字。
- **根因：** 调用方引用的图标名与嵌入资源库不一致；`homeIcon` 在主资源和备用资源均失败后只返回预留尺寸，不绘制任何内容。因此布局仍然正常，缺失资源表现为空白，不会产生编译错误。
- **修复方式：** 补齐 `assets/icons/home/info.svg`，使用与设置页一致的 Lucide 轮廓和可着色描边，并将 `info` 纳入实际渲染检查。
- **下次注意：** 新增图标调用时，同时确认准确的 family/name、SVG 文件、`go:embed` 覆盖和备用路径。不能用“函数被调用”或“控件尺寸非零”证明图标存在；应验证有实际可见像素。设置页的 AI 提供商和 MCP 两个分支都要检查，避免只看默认页面而漏掉另一页的底部提示。

#### 6. 复制图标切换为“已复制”后文字偏上

- **现象：** MCP 地址或 Token 输入框尾部的复制图标居中，点击后替换为“已复制”文字却靠近输入框顶部。
- **产生原因：** `layoutMCPConnectionCopyButton` 的图标分支显式设置 32dp 高度并使用 `layout.Center`，文字分支提前返回，只设置左右内边距，继承了父输入框的 44dp 高度。
- **根因：** 同一控件的不同状态没有共享高度和居中约束。文字标签被父级最小高度撑满后，外层 `layout.Middle` 无法再移动其内部字形；默认图标状态的正确布局不能证明反馈文字状态也正确。
- **修复方式：** 在状态分支之前统一设置 32dp 高度；“已复制”分支在这一高度内使用自然文本尺寸、`layout.Center` 和经过渲染验证的光学偏移，保留左右内边距与持久的 Clickable。
- **下次注意：** 对图标 → 文字、加载中 → 成功、复制 → 已复制等状态变化，逐一检查尺寸、命中区域和可见内容中心。共享几何约束放在分支之前；不要只截默认状态。输入框尾部按钮和标题行按钮共用布局时，也要检查不同父级约束。

#### 7. 设置与主页双向切换时旧画面或混合布局滞留

- **现象：** 点击设置页返回按钮后感觉切换卡顿；返回主页后再次进入设置也会停顿或闪动。
- **产生原因：** `layoutRecent` 先根据旧的 `homeSection` 选择布局，再在侧栏中处理导航。返回时状态已变成主页，当帧却仍继续设置页布局；进入时状态变成设置，已经选定的主页布局仍绘制主页侧栏，而内容区读到新状态后绘制设置表单，出现混合画面。
- **根因：** 决定页面的导航状态在页面布局选择之后才被修改，造成“状态已切换，绘制仍使用旧布局树”的一帧不一致。处理也没有显式请求另一帧，旧画面可能等到后续输入触发绘制才更新；不是同一帧重复绘制主页。仅修复返回方向会遗漏再次进入设置的同类问题。
- **修复方式：** 在 `layoutRecent` 判断页面分支之前，根据当前页面调用 `processSettingsNavClicks` 或 `processHomeNavClicks`。两个方向都先处理点击，再选定完整布局树；两种侧栏不再消费导航事件。
- **下次注意：** 会改变顶层页面、活动标签或布局分支的事件，应在选择布局树之前处理。不要通过再调用一次整页布局或额外重绘旧页面掩盖顺序错误。必须测试“主页 → 设置 → 主页 → 设置”完整循环，不能仅验证一个方向。切页性能排查需区分事件处理、布局、GPU 提交和后台任务；只看状态变量改变无法证明用户已看到目标页面。测试中的布局耗时不能直接当作真实窗口的总响应时间，也不能凭这一修复声称所有卡顿原因都已排除。

#### 8. 保存 API Key 后眼睛按钮无法显示真实密钥

- **现象：** 保存后输入框显示圆点，点击眼睛图标虽切换了图标/掩码状态，仍看不到已保存的密钥。
- **产生原因：** 保存成功后 `apiKeyInput` 被清空，圆点是 `material.Editor` 的静态 Hint，不是编辑器中真实密钥的掩码；原按钮只修改 `Editor.Mask`，没有读取系统凭据库。
- **根因：** 把未保存的编辑草稿、保存后的占位提示、已保存密钥的临时查看混为同一个显示状态。改变空编辑器的 Mask 无法改变静态占位内容；若直接把已保存密钥填回待保存草稿，又会让单纯查看被当作输入新 Key，导致保存时无谓轮换凭据。
- **修复方式：** 仅在用户点击查看且服务商/接口匹配已保存配置时，使用受管理的后台任务读取凭据；结果通过带 generation 和配置快照的通道回到 UI。完整密钥显示在持久的独立只读 Editor 中，待保存的 `apiKeyInput` 保持为空。读取失败退出加载状态并恢复掩码；隐藏、离页、切换服务商、保存或输入新草稿时清除临时预览，丢弃迟到结果。
- **下次注意：** 系统凭据读取不能在 layout/frame 内同步执行；测试必须使用合成密钥与内存凭据库。真实密钥不应出现在日志或测试截图中。临时查看不能改写配置或轮换凭据，也不能将旧服务商的异步结果显示到新接口配置中。
- **长文本与交互：** 不要把真实密钥只放进 Hint：Hint 不参与 Editor 的文本滚动范围，长密钥会被截断而无法查看尾部。只读预览必须保留完整内容并允许水平滚动、选取。外层整框聚焦与内部绘制必须使用同一个当前显示的 Editor；否则点击上下留白会聚焦不可见的草稿，End/选取操作无法作用于预览。
- **按钮命中区域：** 眼睛的 18dp 图形不应决定整个按钮的可点击高度；按钮覆盖输入框的 44dp 高度，图形在其中居中，并验证图形上下留白在正常/Retina 缩放下都能切换。

#### 验证方式与现有回归证据

- `internal/desktopui/settings_visual_test.go` 检查输入框文字墨迹中心、导航图标与文字中心、黑色图标可见描边、设置页截图，以及正常/Retina 缩放下的描边重量。视觉断言应针对渲染结果，不能只断言源代码中存在颜色或 Alignment 常量。
- `internal/desktopui/settings_field_interaction_test.go` 检查普通输入框和 API Key 输入框的上沿、中部、下沿点击，确认聚焦后中文输入不会丢失。
- `internal/desktopui/mcp_copy_feedback_test.go` 在真实输入框布局中依次渲染复制图标和“已复制”文字，检查正常及 2 倍 Retina 缩放下两种状态均垂直居中。
- `internal/desktopui/settings_navigation_test.go` 检查返回和完整循环后的再次进入，对比点击当帧与后续稳定目标页面帧的实际像素，防止旧页面滞留或主页侧栏与设置内容混合。
- `settings_saved_key_preview_test.go` 检查保存后的完整长密钥显示、滚动到尾部、隐藏，以及查看不产生编辑草稿或改写凭据；`settings_key_preview_state_test.go` 检查失败和迟到结果；`settings_key_preview_interaction_test.go` 检查上下留白聚焦只读预览、End 导航和禁止编辑；`settings_key_visibility_test.go` 检查眼睛按钮实际点击切换。
- 使用 `go test ./internal/desktopui -run '^TestSettings' -count=1 -v` 运行针对性回归。需要截图时可设置 `GLANCE_SETTINGS_SCREENSHOT`；截图使用合成测试数据，不应包含用户真实密钥。
- macOS 视觉测试依赖可用的 Metal GPU。测试报告中的 `SKIP` 不是视觉验证通过：检查详细输出，必要时在具备 GPU 访问权限的环境运行。本次实际观察过修复前失败和修复后通过。
- 修改共享的 `homeIcon` / SVG 绘制路径时，还要检查主页等调用方，并运行适当的全量回归。源码和截图检查之外，使用 `git diff --check` 检查补丁格式；文档记录本身不需要新增源码文本匹配测试。

### 浮动菜单经验（2026-10-13）

文件夹右键菜单与最近文件行尾菜单使用 Gio 自绘浮层时，沿用以下规则：

- **控件状态要稳定：** 菜单锚点和各项 `widget.Clickable` 保存在 `App` 或稳定的行状态中。菜单需要的动作按钮、键盘焦点 tag 不能在每帧临时创建；多个菜单共用时，明确唯一活动菜单，并在切换时关闭另一菜单。
- **浮层脱离滚动行布局：** 在列表行中只记录锚点的窗口根坐标；菜单绘制在页面根级 `layout.Stack` / overlay 中，避免被短行或滚动列表裁剪。单独给菜单设精确宽高，不能沿用列表回调继承的大 `Constraints.Max.Y`，否则可能留下空白菜单或错误命中区域。
- **每帧重算位置：** 锚点、菜单 bounds、点击外部区域都使用同一个根坐标系。默认贴锚点右边并留设计间距；底部空间不足时翻到上方，再将菜单限制在可视 viewport 内。滚动后重新计算锚点；锚点离开 viewport 时关闭菜单，不能继续使用上一帧的旧位置。
- **关闭时透传输入：** 用稳定的全屏 dismiss 区域和 `pointer.PassOp` 检测菜单及锚点之外的点击，让底层按钮仍能收到点击。Escape 关闭菜单并把焦点交还锚点；方向键更新当前项，Enter 执行该项。菜单项点击必须与列表行打开事件隔离。
- **布局按子控件自然尺寸检查：** 固定菜单行高只约束行容器；给 SVG 和文本设置明确尺寸、间隔与 hover 背景，并清除不需要继承的 `Constraints.Min.Y`。对照 Pen 检查图标、文字、分隔线、圆角、阴影和菜单相对锚点的位置。新增 home 图标时必须补齐 `assets/icons/home/*.svg` 并实际验证可见像素，不能只依赖预留布局尺寸。
- **异步动作和撤销：** OS 文件管理器操作、工作区数据库写入放后台执行，结果回到 UI 线程更新状态。错误要保留可重试入口。最近列表“移除”只删除最近记录，不删除源文件；撤销应恢复原始记录、时间和顺序，并在持久化成功后显示撤销操作。
- **回归验证：** 覆盖唯一活动菜单、锚点右对齐/上下翻转/viewport 限界、点击外部透传、Escape/方向键/Enter、动作不触发行点击、失败重试与撤销。视觉检查应覆盖 1x 和 2x 的真实渲染像素；Metal/GPU 不可用导致的 `SKIP` 不是视觉通过，须明确记录限制或在可用环境重跑。

### Ribbon 还原经验（2026-10-15）

- `kiBim` / `nmShb` 是命令区总览：命令区 88dp，常规命令按钮 80×68dp、间隔 4dp；开始页分组高 72dp，上下各留 8dp。不要继续套用旧的 68dp 命令区或 44/48dp 通用按钮尺寸。
- `pointer.PassOp` 的 Push/Pop 必须在同一个 Gio 录制边界内。不能在 `Clickable.Layout` 外 Push、在其回调内 Pop；这会在打开浮层时触发 `stack push and pop must not cross macro boundary`。需要透传时将 Push 和 defer Pop 都放在 `Clickable.Layout` 外。
- `layout.Background` 会居中尺寸小于背景的内容。短 Ribbon 的横向列表必须报告完整视口宽度；开始页列表需要先扣除上下 8dp，再布局 72dp 分组，不能依靠外层 Middle 修复已经被列表撑满的交叉轴。
- `Constraints.Max.X` 是物理像素，传给接收 dp 的紧凑按钮函数前必须通过统一转换。2x 下直接把像素再当 dp 会使按钮命中区域越过分组边界。
- 图标按设计节点的实际 library/name 和矢量路径核对。`assets/icons/ribbon` 来自两个节点的 HTML 导出，包含 Lucide、Phosphor 和 Material 图形；对应清单在 `docs/ribbon-pen-icons.json`。不能用打印→文档、另存为→复制等近似图标掩盖映射缺失，也不能将填充轮廓资产当描边 SVG 再全局加粗。
- 同一命令槽在不同分类可能使用不同图形，例如 Excel 开始页“排序和筛选”为 list-filter，数据页“排序”为 arrow-up-down。显示与菜单锚点必须共用当前按钮顺序、80dp 宽度和 4dp 间隔，并扣除当前列表滚动位置。
- 每轮实际窗口验证前退出旧进程，再构建、启动新进程；同一个 App 窗口的截图不能证明进程已加载新代码。通过新进程确认 Word/Excel 菜单可展开，再检查 1x 和 2x 的真实 GPU 渲染。完整记录见 `docs/ribbon-pen-verification.md`。

## Document Safety

- A format library's API surface is not a compatibility guarantee. Record selected dependencies and evidence in [docs/office-library-evaluation.md](docs/office-library-evaluation.md).
- Keep the implemented boundary and known gaps current in [docs/office-support.md](docs/office-support.md), especially when adding legacy formats, style rendering, or resize interactions.
- Do not enable overwrite for a file class until save/reopen tests show edited values and representative untouched content survive. Use a temporary sibling file and replace the source only after a successful write.
- Preserve dirty state on save failure; never silently drop unsupported content or claim formula recalculation/Word pagination without evidence.
- Maintain an evidence-based compatibility matrix as features land, following `../tools/docs/compatibility/office-feature-matrix.md`.

## Reference Implementations

- `../tools` is the reference for neutral document models, format adapters, session lifecycle, compatibility matrices, and round-trip save gates.
- `../mysql-ai` is the reference for Gio event/layout practices, persistent widget state, dialogs, and coordinate-space lessons. Reuse proven patterns; avoid copying its unrelated domain structure or growing a monolithic UI file.

### Ribbon 品牌图标与选中态（2026-10-15）

- `kiBim`、`nmShb` 的开始页 AI 入口使用 `DwuIp` 叠页星芒图标。多色图形按 Pen 原始路径分层导出，分别通过 `Toolbar.AccentAISecondary`、`AccentAI`、`AccentAIStar` 着色；不要用单一 tint 把背页和白色星芒一起覆盖。
- Ribbon 选中背景、描边、图标分别使用 `Toolbar.ActiveBackground`、`ActiveBorder`、`ActiveGlyph`，选中反馈优先于 hover。不要为局部 Ribbon 设计改动连带替换主页导航、工作区标签或文档内容颜色。
- Excel 纯图标按钮先在完整按钮尺寸中居中，再对子级设置图标尺寸；提前缩小父约束会让图标靠在按钮左上角。图像 Scale 使用逻辑尺寸除以栅格像素尺寸，分别检查 1x 和 2x。
- `TestPenRibbonBrandAndActiveStateVisual` 使用合成数据检查实际像素与 token 覆盖；小星芒在 1x 下可能全部是抗锯齿像素，不应强制断言存在完全不透明的前景色。自定义多色 token 的精确覆盖在 2x 检查。
