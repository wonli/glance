# Windows 内存采集

正式 Windows 包和 `cmd/glance` 共用诊断入口。默认关闭；监听地址由 `GLANCE_PPROF_ADDR` 显式指定。
完全退出旧 Glance 进程（包括托盘），再从 PowerShell 启动新构建：

```powershell
$env:GLANCE_PPROF_ADDR = "0.0.0.0:6060"
Start-Process ".\glance-windows-amd64-latest.exe"
```

浏览器打开 `http://127.0.0.1:6060/debug/pprof/`。局域网其他电脑使用 `http://<Windows电脑的局域网IP>:6060/debug/pprof/`，下面采集命令中的 `127.0.0.1` 也替换成该 IP。若无法连接，在 Windows 防火墙中允许该程序在专用网络访问，或添加仅允许本地子网的 TCP 6060 入站规则。pprof 没有身份验证，调试结束后关闭监听；不要把端口映射到公网。

端口占用或地址无效会写入 `%APPDATA%\Glance\startup.log`，应用仍正常启动。
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
