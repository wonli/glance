# Windows 内存采集

正式 Windows 包和 `cmd/glance` 共用诊断入口。默认关闭；监听地址由 `GLANCE_PPROF_ADDR` 显式指定。
完全退出旧 Glance 进程（包括托盘），再从 PowerShell 启动新构建：

```powershell
$env:GLANCE_PPROF_ADDR = "0.0.0.0:6060"
Start-Process ".\glance-windows-amd64-latest.exe"
```

浏览器打开 `http://127.0.0.1:6060/debug/pprof/`。局域网其他电脑使用 `http://<Windows电脑的局域网IP>:6060/debug/pprof/`，下面采集命令中的 `127.0.0.1` 也替换成该 IP。若无法连接，在 Windows 防火墙中允许该程序在专用网络访问，或添加仅允许本地子网的 TCP 6060 入站规则。pprof 没有身份验证，调试结束后关闭监听；不要把端口映射到公网。

端口占用或地址无效会写入 `%APPDATA%\Glance Office\startup.log`，应用仍正常启动。
不需要安装 Go 就能下载快照。以下命令在 PowerShell 中执行：

1. 空窗口稳定后采集基线，并记录任务管理器中的进程内存数值及列名。

```powershell
curl.exe -f -o empty.heap "http://127.0.0.1:6060/debug/pprof/heap?gc=1"
```

2. 打开目标 Word，等待分页结束、内存稳定，先记录进程内存并采集自然状态，再采集 GC 后状态；再次记录进程内存。

```powershell
curl.exe -f -o word-before-gc.heap "http://127.0.0.1:6060/debug/pprof/heap"
curl.exe -f -o word.heap "http://127.0.0.1:6060/debug/pprof/heap?gc=1"
```

3. 关闭文档后再采集，并记录进程内存。

```powershell
curl.exe -f -o closed.heap "http://127.0.0.1:6060/debug/pprof/heap?gc=1"
```

`gc=1` 会主动触发 GC，可能短暂影响交互，但不保证把空闲内存立即归还操作系统。
默认二进制 heap 是用于分析的文件，不是浏览器中的可视化图表。安装 Go 的机器可运行：

```powershell
go tool pprof -top -inuse_space word.heap
go tool pprof -top -inuse_space -base empty.heap word.heap
go tool pprof -http=127.0.0.1:8080 word.heap
```

交互图形视图可能需要 Graphviz；`-top` 不需要。没有 Go 时也可下载文本概要：

```powershell
curl.exe -f -o word-heap.txt "http://127.0.0.1:6060/debug/pprof/heap?debug=1"
```

优先分析 `inuse_space`（存活对象），而不是把累计分配量当常驻占用。
heap 仅覆盖 Go 堆，不能直接解释任务管理器中的全部进程占用，尤其是 GPU 和原生分配。
保留同次构建的可执行文件以供符号分析。诊断结束后退出应用、移除变量并重启：

```powershell
Remove-Item Env:GLANCE_PPROF_ADDR
```

## 2026-10-19 第一轮主页绘制优化

Windows 实测采样中，关闭 Word 后的存活 Go 堆约 119.7 MiB；继续滚动主页后，主动 GC 后仍约 119.7 MiB，但两次采集之间累计分配约 294.7 MiB，热点为 Gio 曲线描边和 `paint.NewImageOp` 的 RGBA 转换。这些是采样估算，不是进程常驻内存统计。

第一轮复用 SVG 图标的 `paint.ImageOp`（保留原始图像具体类型），并对主页/工作区的圆角边框保留不可变绘制指令。边框缓存最多 256 项，包含几何、描边宽度、颜色；窗口缩放和尺寸变化使用新键，旧尺寸按容量淘汰。

本机 Apple M1 / Metal 预热微基准：

| 场景 | 直接创建 | 复用缓存 |
| --- | --- | --- |
| 40px SVG 图标 ImageOp | 6,600 B/op，3 allocs/op | 0 B/op，0 allocs/op |
| 单圆角边框真实 GPU 帧 | 约 91,244 B/op，210 allocs/op | 184 B/op，3 allocs/op |

1x/2x 圆角边框渲染与原实现逐像素一致。上述数字不是整页帧开销，也不保证 Windows 进程内存下降相同幅度。字体解析与常驻字体内存尚未在本轮修改。

复验命令：

```sh
go test ./internal/desktopui -run '^(TestHomeIconWarmFrames|TestRoundedBorderCache|TestCachedRoundedBorder)' -count=1 -v
go test ./internal/desktopui -run '^$' -bench '^(BenchmarkHomeIconImageOp|BenchmarkHomeIconWarmFrame|BenchmarkRoundedBorderFrame)$' -benchmem
```

真实 GPU 不可用导致的 SKIP 不是视觉或 GPU 分配验证通过。Windows 重新构建后，保持同一文档、窗口尺寸、缩放和滚动路径，重新采集打开、关闭、主页滚动的 heap，并记录进程内存后对比。

### 本轮验证状态

主页/图标针对性回归、真实 Metal 1x/2x 像素对比、`-race` 并发复用检查、Windows GUI 构建及 `git diff --check` 均通过。移除图标缓存的临时 Go overlay 会让零分配回归失败（3 allocs/frame），确认测试能捕获原问题。独立代码审查未发现需要修改的问题。

`go test ./...` 未通过，不能作为全量通过证据。桌面包运行约 471 秒后主动中断并抓取堆栈；当时位于 `TestWordViewSetRejectsRangeOutsideCurrentRunWithoutChangingSelection` 的应用 Runtime/命令目录初始化调用链，不是本次边框或图标缓存代码。中断前及其他包报告的失败如下（未在本轮修复）：

- `TestAgentHistorySearchAndDateGroups`
- `TestAgentRibbonTextFormsCenteredGroup`
- `TestParseCellRangeAddress`
- `TestGridResizeBoundaryDoubleClickAutoFitsColumn`
- `TestCellSelectionBoundsClampsToXLSXLimits`
- `TestMarkdownPageFocusKeepsTextDragSelection`
- `TestDesktopMCPStdioEditsTheLiveGioSession`
- `TestSettingsFieldBlankEdgesFocusEditor`
- `TestSettingsSavedKeyEdgeClickFocusesVisiblePreview`
- `TestSettingsAPIKeyEyeClickTogglesVisibility`
- `TestSettingsFieldsVerticallyCenterText`
- `TestSpreadsheetStatusBarShowsEndModeIndicator`
- `TestSpreadsheetZoomInteractionsAndClamping`
- `TestSpreadsheetRecalculateAndRefreshAll`
- `TestSpreadsheetToolbar_SpellCheckAndTranslate`
- `TestSpreadsheetToolbarActionsCoverHomeAndDataGroups`
- `TestExcelRibbonActionsUseBundledIconAssets`
- `TestSpreadsheetViewModesAndRuler`
- `TestHideMainWindowToTrayPreconditions`
- `TestWordShapeAndWordArtRibbonsUseRegisteredTextBoxCommand`
- `TestWordCanvasAndHorizontalRulerMatchPenClpgmGeometry`
- `TestWordHomeRibbonKeepsCommandGeometryAfterAgentEntry`
- `TestPrivateFontSceneExportKeepsLoadedResourceAfterRefresh`
- `TestMCPCLIRecognizesServeAndDocumentsStdoutContract`
- `TestLoadManifestLoadsCuratedCorpus`

其中四项设置页失败（`TestSettingsFieldBlankEdgesFocusEditor`、`TestSettingsSavedKeyEdgeClickFocusesVisiblePreview`、`TestSettingsAPIKeyEyeClickTogglesVisibility`、`TestSettingsFieldsVerticallyCenterText`）已在干净 HEAD 快照复现。`TestLoadManifestLoadsCuratedCorpus` 也在干净 HEAD 复现；其余失败的基线补查因磁盘空间不足导致链接器 `errno=28`，未完成，不能全部称为既有失败。

## 第二轮：共享不可变字体解析结果

- 按字体内容 SHA-256 和 TTC 字面索引共享 `font.Font`，每个 Word/Gio 排版器仍单独创建可变的 `font.Face`。索引最多 256 项，值为弱引用，不阻止未使用字体被回收。
- 资源读取仍校验目录快照的内容摘要；在字体加载路径直接使用已验证字节，减少 `OpenFace` 后再 `io.ReadAll` 的整文件复制。
- Gio 在已有后台字体准备任务中预载常用界面字体的正常/半粗字面，保留系统回退。缺失字重遵循原有 CSS 匹配方向。
- 文档指定字体先计费，可选 UI 预载随后计费；共用原有 128 MiB 唯一源文件预算。该预算不是字体解析后内存或进程总内存上限。

覆盖范围：Word 排版和其他 `wordfont` 消费者可共享同内容、同索引的解析结果；Markdown、表格、演示文稿的 PDF 导出也使用该路径。Gio 文本渲染可使用预载的共享字体。PDF 页面渲染后端、图片解码和 GPU 纹理不在本轮范围内；不能据此宣称所有格式占用同等下降。

小型 Go Regular 字体夹具的预热解析微基准（Apple M1）：直接解析约 387,328 B/op、2,795 allocs/op；共享命中为 0 B/op、0 allocs/op。该数字只比较解析阶段，不包含文件读取、摘要校验或新建 Face，不代表 Windows 中文字体或进程内存降幅。

字体目录/排版核心测试、弱引用回收、并发复用 `-race`、UI 字体来源/字重/预算、相关真实 GPU 回归、Word PDF 分页几何和 Markdown PDF 导出检查通过；表格/演示文稿 PDF 包编译通过。未重跑上一轮有失败的整个桌面套件。Windows 的实际占用需重新构建并采集 pprof 确认。

验证使用全局 `GOCACHE`（本机为 `/Users/mima1234/Library/Caches/go-build`），不在临时仓库内另建缓存，不复制系统字体集合；Windows 复测程序覆盖同一输出路径，避免累积副本。

## 第三轮：字体列表与缓存生命周期

本机同一进程的三阶段采样（文档内容不变）：基线 Go 堆约 178 MiB，字体列表展开并滚动后约 193 MiB，关闭并再次 GC 后约 177 MiB；RSS 则由约 572 MiB 到约 626 MiB。说明本次采样没有持续增加的存活 Go 堆，但有明显临时分配及进程驻留内存未回落，不能仅凭 RSS 断言字体对象泄漏，也不能将 Go 堆回落称为整个进程已恢复。

- 列表当前以统一 UI 字体显示名称和示例，开关列表不再改变公共 Shaper 的字体需求，也不再批量解析未被文档使用的私有字体。选择或应用字体后，正常的文档字体需求更新仍会触发后台准备。
- 构建新字体集合后，解析缓存只保留当前集合需要的条目，移除已不需要的文档字体强引用。旧 Shaper/在途帧独立持有字体，不因缓存清理失效；全局共享解析索引仍为弱引用。
- 关闭文档继续使用已有的视图/布局资源释放和防抖后台内存归还流程，没有增加定时强制 GC。原生/GPU 资源占用与操作系统驻留内存不由此保证立即下降。

新增回归先观察到开关列表触发重建、无文档字体需求时缓存仍保留字体两项失败，修复后通过。相关字体选择/中文输入/真实 GPU 检查、字体核心检查，以及各格式开关后主页空闲、Word 非活动布局引擎释放和关闭当帧交互回归通过。各格式内存测试只说明其现有断言通过，不代表每种格式回到相同基线。未重跑前述有已知失败的全量套件。

Windows 复测请完全退出旧进程，再启动新版并开启局域网 pprof。依次采集：同一文档稳定后、展开字体列表并滚动、关闭列表稳定后、关闭文档返回主页稳定后；各阶段同时记录任务管理器同一内存列。必要时重复两轮，区分一次性初始化与持续增长。

## 第四轮：撤掉未命中的系统 UI 字体预载

Windows 新版纯文字 Word 的 GC 后采样 `/tmp/glance-win-round3-current.heap`：
pprof 的 `inuse_space` 总计约 238.02 MiB，Gio/fontscan 的磁盘解析路径累计约
102.33 MiB，共享 `wordfont.ParseFont` 路径累计约 79.11 MiB，其中 UI 集合解析
约 38.18 MiB。路径累计值不能再与其子调用相加，也不能直接当作进程 RSS。

已在本机实际系统 Arial 上复现：Gio v0.10.3 `FontToDescription` 未设置
`Aspect.Stretch`，注册预载字体时为 0；系统扫描的正常宽度为 1。
fontscan 先筛选宽度，再处理用户字体优先级，因此同名系统字体胜出。
补齐正常宽度的独立探针会命中共享 face，原逻辑不会。

按用户要求不引入 Gio 副本、不更改依赖。本轮只跳过**自动系统 UI 字体预解析**，
让 Gio 继续正常选择系统字重/斜体及字符回退；保留私有/在线 UI 字体注册、
显式请求的文档字体加载及关闭后的缓存释放。共享解析机制仍服务 Word 和导出。
这消除额外 UI 预载的成本，但**没有修复 Gio 底层匹配错误**，也不保证清除
102.33 MiB 的系统解析。实际收益必须用相同文档重启 Windows 新进程后测量。

未采用全局内部别名方案：只预载常规/半粗体时，别名家族会先选中近似样式，
遮蔽原家族中真实的粗体/斜体。不能为减少内存而降低字体样式正确性。

新回归先观察到无文档时仍解析系统 UI 字体的失败；修复后检查无重复预解析、
显式文档请求仍可用、关闭后缓存清空，以及私有 UI 字体仍注册。

最终版本的字体/Word/表格/演示文稿针对性回归通过（4.372s）；预载及缓存生命周期
`-race` 通过（3.190s）；Ribbon Word/Excel 1x/2x 和设置导航真实渲染检查通过，
无 SKIP（1.311s）。独立审查无待处理问题。Windows 使用与 `make windows`
一致的根入口、netgo 和 GUI 链接参数编译通过，输出仍覆盖
`/tmp/glance-memory-optimized-windows.exe`。全程复用全局 Go 缓存，未再运行
前述有已知失败的全量套件；尚未测量本轮 Windows 运行后的实际降幅。

本机进程分类、字体弱引用清单和显式归还接口见 [本机内存诊断](memory-diagnostics.md)。macOS 原生计数已实现，Windows 当前仍使用 pprof/Go 分类。

## 窗口拖动与绘制耗时

启用同一个 `GLANCE_PPROF_ADDR` 后，CPU profile 带有 `window_phase`
标签（`config`、`event`、`layout`、`frame-submit`）。例如采集 20 秒，
期间反复拖动窗口：

```powershell
go tool pprof -http=127.0.0.1:8080 "http://127.0.0.1:6060/debug/pprof/profile?seconds=20"
```

CPU 样本不代表等待耗时。`window-latency` 是可直接读取的 pprof 格式，
提供 `calls/count` 和 `wall/nanoseconds` 两种 sample type；默认显示累计
实际耗时。采集前后两个快照，再求差值可隔离一次操作：

```powershell
curl.exe -o before.pb.gz http://127.0.0.1:6060/debug/pprof/window-latency
# 拖动窗口，然后立即采集第二份
curl.exe -o after.pb.gz http://127.0.0.1:6060/debug/pprof/window-latency
go tool pprof -base before.pb.gz -sample_index=wall -top after.pb.gz
go tool pprof -base before.pb.gz -sample_index=calls -top after.pb.gz
```

数据从本次进程开启诊断起累计，所有窗口合并，只统计已完成的阶段。
profile 中的 `window.*` 节点是阶段汇总，不是采样调用栈；它不提供
P95/最大值，也不自动识别原生窗口拖动。`event` 是帧前应用事件处理，
不包含 `Window.Event()` 等待；`config` 是应用配置事件处理；
`layout` 包含页面布局内部的输入处理；`frame-submit` 是 `e.Frame()`
调用耗时，不能解释成 GPU 执行或 `Present` 的精确耗时。

需要时间线时，另行采集 trace，查看 `window.*` region 与 goroutine 调度：

```powershell
curl.exe -o window.trace "http://127.0.0.1:6060/debug/pprof/trace?seconds=10"
go tool trace window.trace
```

建议分别采集主页、Word 和 Excel，每次只操作一个窗口。阶段标签不包含
文件名、路径或文档内容；默认未开启诊断时不执行计时与标签操作。

## Word 鼠标选区错位诊断

启用同一个 pprof 服务后，会在内存中循环保留最近 512 条 Word 页面选区事件。
复现“拖动时高亮错位、松开后正常”后立即获取：

```powershell
curl.exe -o word-selection.json "http://127.0.0.1:6060/debug/pprof/word-selection"
```

这是 JSON 事件快照，和 heap、CPU、window-latency 文件一起提供即可，
不使用 `go tool pprof` 打开。也可通过 `/debug/word-selection` 获取同一快照。
GET 不会清空记录；继续操作时旧记录会被覆盖。

`input-Press/Drag/Release/Cancel` 记录收到事件时的局部、视口和页面坐标；
`hit-Drag/Release` 记录命中的文字 run 与字符偏移；`paint` 记录绘制高亮时
实际使用的选区起止、编辑器选区、页面原点、缩放、视图及阅读模式。
按时间对照这些阶段可以判断输入坐标、命中结果与绘制范围是否一致。
只记录几何数据、内部 ID 和字符偏移，不记录正文、文件名或路径。
未开启 pprof 时不采集。当前覆盖连续/打印页面视图，Web 编辑器不在此轨迹内。

## Windows 原生拖动消息桥接

Windows 默认启用应用层 `internal/windowmove` 桥接，Gio 仍是官方
`gioui.org v0.10.3`，没有本地替换或源码补丁。通过公开的
`app.Win32ViewEvent` 获取 HWND，再用 `Window.Run` 在该窗口所属线程
安装 `SetWindowSubclass`。安装失败记录错误并保留 Gio 原有行为。

桥接只在 `WM_ENTERSIZEMOVE` 到 `WM_EXITSIZEMOVE` 之间，且消息明确没有
尺寸、层叠顺序、边框或显示/隐藏变化时，才考虑拦截
`WM_WINDOWPOSCHANGED`。同时核对前后客户区尺寸、DPI、普通窗口状态。
查询失败、跨 DPI、最大化/最小化都转交原有消息链。
连续采样还捕获到移动手势内部的 `WM_PAINT -> draw(true) -> D3D11
CreateBuffer`，因此只拦截位置消息不足以消除停顿后的卡顿。纯移动且
几何、DPI、窗口状态保持不变时，桥接用 `ValidateRect` 消耗绘制区域，
延后该次绘制；退出拖动时重新失效整个窗口，展示期间累积的内容更新。
收到 `WM_SIZING` 或观察到尺寸、DPI、状态变化后，本次手势恢复正常绘制。
纯移动期间内容可能暂时保持旧画面；后台任务和输入不被暂停。
回调不调用 Gio，不解析文档，也不读写应用状态；原生销毁时移除桥接。

重启程序后生效。关闭桥接进行对照：

```powershell
$env:GLANCE_WINDOWS_MOVE_REDRAW = "0"
# 启动程序，采集相同页面、相近时长的拖动快照差值
```

恢复默认开启：

```powershell
Remove-Item Env:GLANCE_WINDOWS_MOVE_REDRAW -ErrorAction SilentlyContinue
```

`false`、`off` 同样关闭，值不区分大小写。此开关独立于 pprof 开关。
检查退出旧进程、重新构建和启动后，再比较 `config`、`frame-submit`
次数与耗时。仍可能有 hover、动画等正常帧，不能要求完全零帧。
还须实际检查缩放、贴靠、最大化/还原、最小化/还原、多窗口、关闭窗口、
跨不同 DPI 显示器拖动和拖动期间后台更新。

测试：`go test ./internal/windowmove`。Windows 上还会运行真实 HWND 的
消息路由、线程限制和销毁清理测试，无需 GPU。其他平台只运行可移植
判断逻辑与开关测试；交叉编译不等于 Windows 实机性能或交互验证通过。

### Word 拖动坐标应用层修正

Windows 轨迹曾显示 Press/Release 命中当前段落，而 Drag 坐标突然向左上跳、
命中更早段落，导致拖动时出现大片反向高亮。应用层对捕获中的鼠标 Drag 使用
`GetCursorPos` + `ScreenToClient`，结合上一完整帧的 Gio 公共语义树正文视口
边界计算视口坐标；不修改或替换 Gio。只读取每帧最新光标位置，合并到一帧
的多个 Drag 使用同一位置；Press/Release 保留事件时间对应的位置。
触屏和其他平台沿用原事件路径。原生查询失败或窗口尺寸、DPI、会话变化时
回退到原路径。`native_normalized=true` 标记实际使用修正的输入记录，
可对照原始 `local_x/local_y` 与修正后的 `viewport_x/viewport_y`。

本机使用录到的左上偏移回放已验证修正前 GPU 高亮错误、修正后双向拖动
逐帧正确；Windows 交叉构建通过。真实 Windows 更新进程后的选区、跨行/页、
窗口移动/缩放和触屏仍需验证，不能以回放代替实机结论。

实机复测发现上一版 `native_normalized=true` 时仍然错位。坐标查询原先在普通
Go 线程执行，但上游 Gio 只在 HWND 原生事件线程设置 per-monitor DPI 感知；
普通 Go 线程的 Win32 光标 API 可能返回虚拟化坐标。查询现改为通过公开的
`Window.Run` 在窗口线程执行，并核对当前线程与 HWND 所属线程一致。
新的轨迹含 `native_thread/window_thread`、`native_client_x/y` 和
`viewport_origin_x/y`，用于实机确认查询上下文及坐标转换；不能仅凭
`native_normalized=true` 判定问题已修复。

2026-10-19 用户确认窗口线程查询版 Word 选取恢复正常，同时报告 Excel 网格
和单元格内文字拖选有相同现象。网格输入现复用同一窗口线程快照；原生
`widget.Editor` 无法在应用层替换其内部 Drag，因此另在 HWND 线程通过公共
Win32 `SetWindowSubclass` 接入：仅对 `WM_POINTERUPDATE` 中按住主键的鼠标，
用同线程 `GetCursorPos` 的屏幕坐标重新编码 lParam，再交给原 Gio 窗口过程。
按下、释放、触屏、笔及普通悬停不改写；销毁时移除 subclass。此桥与窗口移动
subclass 使用不同 ID，继续使用上游 `gioui.org v0.10.3`，没有本地替换。

Excel 回归模拟错误拖动坐标并核对实时正反向单元格选择、固定锚点与释放状态；
Word GPU 高亮及滚动条回归继续通过。Windows 原生编辑器路径已交叉构建，
单元格内中文/英文、多行、横向滚动文字拖选仍待真实 Windows 验证。
