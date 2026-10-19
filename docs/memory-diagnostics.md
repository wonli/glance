# 本机内存诊断

沿用显式开启的 pprof listener，默认关闭，不修改 Gio 依赖。
macOS 示例（先保存并完全退出旧进程）：

```sh
go build -o /tmp/glance-memory-diagnostics .
GLANCE_PPROF_ADDR=127.0.0.1:6060 /tmp/glance-memory-diagnostics
```

局域网调试仍可将监听地址设为 `0.0.0.0:6060`，沿用
[Windows 采集文档](windows-memory-profiling.md)中的网络访问范围要求。

- `GET /debug/memory`：时间、PID、Go 堆分类，以及 macOS RSS/physical footprint。
- `GET /debug/fonts`：应用共享字体解析清单、解析与缓存命中次数、来源文件/TTC 索引、使用方和当前存活状态。
- `POST /debug/memory/release`：记录 before，调用一次 `debug.FreeOSMemory`（自带 GC），再记录 after 和耗时。GET 不执行释放；并发释放返回 409。它运行在 HTTP worker，仍可能有 GC 停顿，只用于明确的诊断对照。

```sh
curl -f http://127.0.0.1:6060/debug/memory
curl -f http://127.0.0.1:6060/debug/fonts
curl -f -X POST http://127.0.0.1:6060/debug/memory/release
```

解释边界：

- `runtime_mapped_estimate = Sys - HeapReleased` 是 Go 管理内存的估计，不是 RSS，也不能与 footprint 相减就断言差额是 GPU。
- `heap_inuse_unused = HeapInuse - HeapAlloc`；`heap_idle_unreleased = HeapIdle - HeapReleased`。这些与 HeapAlloc、Sys 有包含关系，不能重复相加。
- macOS 原生计数通过 `proc_pid_rusage(RUSAGE_INFO_V2)` 获取。RSS 与 footprint 的记账不同，归还可复用页后 RSS 不保证同步下降。无 cgo 或其他平台暂返回明确的原生计数不可用信息，Go 分类和 pprof 仍可使用。
- 字体清单仅覆盖启用 profiling 后的应用共享解析，不含 Gio 私有字体缓存。`source_bytes` 是输入文件大小（TTC 可能含多个 face），不是该 face 的存活堆。使用弱引用，最多 256 项历史、每项最多 16 个来源/使用方；淘汰计数随清单输出，累计解析次数不因淘汰丢失。
- 文件名/字体路径只在显式启用的诊断 listener 暴露，无后台上传。

## 2026-10-19 macOS 单行 Word 对照

用户确认文档只有一行文字，无分页。诊断进程 PID 61692，以下数值来自同一进程。
各阶段先采自然状态，再做一次显式归还；自然样本与 before 相差数毫秒，表中采用 release 的 before/after。

| 阶段 | Go HeapAlloc | macOS footprint | RSS |
| --- | ---: | ---: | ---: |
| 主页，归还前 | 109.7 MiB | 190.8 MiB | 227.2 MiB |
| 主页，归还后 | 53.4 MiB | 168.5 MiB | 227.2 MiB |
| 打开单行 Word，归还前 | 142.8 MiB | 335.3 MiB | 408.6 MiB |
| 打开单行 Word，归还后 | 128.8 MiB | 264.6 MiB | 408.6 MiB |
| 关闭 Word，归还后 | 109.1 MiB | 229.6 MiB | 409.3 MiB |

字体清单显示 Word 侧只解析一次 PingFang.ttc 的 index 3，源 TTC 约 74.6 MiB；
pprof 将该共享解析的保留量估为约 14.66 MiB。关闭文档后 weak 存活状态为 false，
相应 Word 解析路径已从堆快照消失。

Gio `loadFromDisk` 路径的采样保留量：初始主页约 29.58 MiB，打开 Word 后约
75.58 MiB，关闭后仍约 75.58 MiB。展开调用路径后，约 46 MiB 的新增字体解析
归在 Word Ribbon 剪贴板命令的标签绘制路径 `layoutOfficeRibbonCommandContents`。
这是 UI 标签触发的字体成本，不能解释为单行正文复杂。

进一步隔离同一个平台 Typeface、新建 Shaper 的自然保留量：

| 标签 | 保留量（实际 HeapAlloc 差值） |
| --- | ---: |
| `Paste` | 约 13.9 MiB |
| `Paste ▾` | 约 59.9 MiB |
| `粘贴` | 约 13.9 MiB |
| `粘贴 ▾` | 约 59.9 MiB |
| 单独 `▾` | 约 46.2 MiB |

探针仅测新 Shaper 存活时的堆差，不代表整个应用 RSS，也不确定 Gio 实际选中了哪个字体文件。
原始日志 `/tmp/glance-font-symbol-probe.log`。临时探针测试已移除，不保留依赖本机字体大小的脆弱内存阈值断言。

修复将保留在命令元数据中的尾部 ` ▾` 从送入字体排版的文本中移除，改用 dp 缩放的
6×4 矢量三角形绘制，颜色沿用标签。适用于 Word/Excel Ribbon 和 adaptive choice；
外层持久按钮、语义和命中区域仍由原控件负责。换行标签将箭头对齐末行，已检查中英文
1x/2x 实际渲染。未修改 Gio 或字体选择顺序。

新增回归通过真实字体 Cmap 观察确认正文标签仍被排版、U+25BE 不进入排版；旧代码先失败，
修复后通过。Ribbon Word/Excel 1x/2x 总览、品牌状态和箭头实际像素检查通过，无 SKIP。

同阶段外部 vmmap 的图形映射可随帧和采样时机变化；不要用其各行 resident 简单
相加代替进程 RSS/footprint，也不要将这里的约 46 MiB 直接许诺为修复后节省量。

实现验证：新增内存接口先见 404 后通过；字体清单覆盖共享计数、历史上限及弱引用回收。
诊断 handler/字体清单 race 检查、fontcatalog/wordfont 和针对性 UI 字体生命周期检查通过，
macOS 根入口和 Windows netgo/GUI 根入口编译通过。未重跑先前已有失败的整个桌面套件。

最终检查：菜单 pointer/keyboard/outside-click、Ribbon 几何/录制边界和字体生命周期
针对性回归通过（0.937s）；诊断与弱引用清单 race 通过；最终 macOS 和 Windows
根入口编译通过。独立审查无待处理问题。修复版输出覆盖同一路径，未重复复制缓存。
截至最后确认，listener 仍是修复前的 PID 61692，因此尚不能报告应用整体的修复后降幅；
独立字符探针的约 46 MiB 差值仅用于定位触发原因，需重启修复版做同文档对照。


### 修复版同文档复测（2026-10-19 19:20）

用户重启并打开同一份单行 Word；新进程 PID 64944，区别于原 PID 61692。
继续按自然状态 → 显式归还 → heap 的顺序采集。以下采用各次 release 的 after：

| 指标 | 修复前 | 修复后 |
| --- | ---: | ---: |
| 实际 Go HeapAlloc | 128.8 MiB | 79.9 MiB |
| macOS physical footprint | 264.6 MiB | 183.0 MiB |
| RSS | 408.6 MiB | 389.4 MiB |
| Gio/fontscan 字体解析路径（采样） | 75.58 MiB | 27.47 MiB |

自然状态的 footprint（release before）约 335.3 → 227.1 MiB。
新字体清单仍只有 Word 的 PingFang TTC index 3，一次解析；共享解析路径采样约
16.92 MiB（之前约 14.66 MiB，采样及执行过程会有波动）。

字体回退路径实测下降约 48.11 MiB，与隔离字符探针吻合，确认箭头修复在完整应用
中生效。RSS 仍高于 footprint，不能把所有变化都归于同一字体，也不能宣称整个
进程 RSS 同步下降了约 48 MiB。对照文件前缀 `/tmp/glance-mac-arrow-fixed-word*`。

### 箭头修复后继续拆查（2026-10-19 19:21–19:26）

用户确认已关闭 Word 回到主页，仍是 PID 64944。Word 解析清单 weak alive=false。
再次显式归还后的 HeapAlloc=66,244,192 B（约 63.2 MiB），footprint=188,616,320 B
（约 179.9 MiB），RSS=409,927,680 B（约 390.9 MiB）。

外部 `footprint --pid 64944 --wide --format bytes` 直接提供分类：

- `untagged (VM_ALLOCATE)` reclaimable=195,510,272 B（约 186.5 MiB）；全类别
  reclaimable=200,933,376 B（约 191.6 MiB）。这些页可以仍驻留，但系统可以回收；
  不能视为尚被业务对象引用的存活 Go 堆。采集时间不同，不能与另一次 RSS 机械相加/相减。
- IOSurface dirty=51,412,992 B（约 49.0 MiB）；IOAccelerator graphics dirty=
  31,457,280 B（30 MiB）。约 79 MiB 是图形相关映射，不应全算字体或原生泄漏；
  呈现缓冲、纹理各占多少还需进一步针对性验证，不能承诺全部可省。
- 系统 malloc 区约 10 MiB 量级，没有观察到足以单独解释额外两百 MiB 的 malloc 膨胀。

本机 Go runtime `mem_darwin.go` 的 sysUnusedOS 实际调用 `MADV_FREE_REUSABLE`，
符合归还后 RSS 暂时维持、footprint 先下降的现象。没有给 macOS 套用 Linux 专用的
madvdontneed 调整，也没有更改 Go runtime。

主页静置十秒的 allocs delta 未采到样本，CPU profile 共 60ms（约 0.6%），主要还含
诊断请求本身的开销。只说明这次窗口没有检测到持续业务分配/重绘，不代表零分配的严格证明。
原始文件 `/tmp/glance-mac-idle-allocs.pb.gz`、`/tmp/glance-mac-idle-cpu.pb.gz`。

主页存活堆剩余主要路径（pprof 采样）为：UI fontscan 约 27.47 MiB；
Agent.NewHost→loadTools→ClientSession.ListTools 约 8 MiB；image.NewNRGBA 约 7 MiB
（SVG 约 4.45 MiB、PNG 约 2.55 MiB）；i18n.compileEntries 约 6 MiB；
Gio CPU 顶点缓存约 4.12 MiB。它们是路径累计/采样量，不与其子调用再重复相加。

源码也确认配置了模型后 restoreAgentHost/installAgentHost 会初始化内置 MCP client，
NewHost 立即 ListTools，SDK 新协议路径缓存列表；尚未调用 AI 也可保留该目录。
语言目录目前为每条普通文本也创建 template。两者是后续降低空窗口基线的候选，
尚未改为延迟初始化或纯文本快路径，不能报告节省量。

### 降低空闲基线：语言目录与内置 AI 工具清单

普通翻译原来即便没有参数也构建/执行 text/template。现在不含 `{{` 的形式直接
保留字符串并返回，真正的模板仍解析并校验，语言间参数一致性与复数/回退规则保留。
回归先观察到纯文本查询每次 5 次分配，修复后为 0。中文目录构建微基准（3 次，M1）：
总分配从约 19,011,024 B/op、172,173 allocs/op，降到 7,640,416 B/op、72,943 allocs/op。
这是构建阶段分配，不能直接当作进程常驻降幅。

桌面创建 AgentHost 时显式启用 LazyTools：模型配置和 MCP 连接继续初始化，
`tools/list` 推迟到首次实际对话的 runTurn；普通 NewHost 调用默认仍进行原来的
立即读取/校验。并发请求合并首次成功加载，等待方可独立取消；失败不永久缓存，
下次可重试；关闭仍取消并等待进行中的 turn。首次读取不进入 Gio frame/layout。
因此只推迟了工具清单/SDK 列表缓存，不宣称卸载全部 AI 功能或释放所有工具描述。

实际内存收益待重启采样。全量 agent、i18n 回归和 race 通过（Agent race 43.043s），
桌面 AI 配置/订阅/关闭/聊天和相关菜单针对性回归通过（1.411s）；独立审查无待处理问题。
macOS 根入口和 Windows netgo/GUI 根入口均编译通过，覆盖原临时输出路径。
未重新运行已有失败的整个桌面套件。审查误建的临时 Go 缓存已清理，最终验证复用全局缓存。


### 语言/工具目录优化后主页复测（2026-10-19 19:40）

用户重启并停在主页、不使用 AI，新进程 PID 67446。归还后 HeapAlloc=
47,169,656 B（约 45.0 MiB），physical footprint=148,606,528 B（约 141.7 MiB），
RSS=223,330,304 B（约 213.0 MiB）。归还前自然状态分别约 63.7、180.4、213.0 MiB。

pprof 中 Agent.loadTools 路径已消失（上次主页约 8 MiB）；i18n.compileEntries
约 1.50 MiB（上次约 6 MiB）；fontscan.loadFromDisk 约 27.18 MiB，量级未变。
两项针对性变化均与预期相符。原始快照前缀 `/tmp/glance-mac-lazy-home*`。

上一进程关闭 Word 返回主页后为约 63.2 MiB Go 堆、179.9 MiB footprint，
但它已经历文档打开/关闭，而这次是新启动主页，不能把两者的全部差额归于本轮优化。
首次使用 AI 后工具目录会正常加载，该部分节省针对未使用 AI 时的空闲基线。

### 图形/图标轮：复用上传标识，缓存单份预乘像素

源码确认 Ribbon 等多处仍对不变的 SVG icon 每帧调用 paint.NewImageOp。对 NRGBA
输入，Gio 每次新建 RGBA 像素副本，并为纹理缓存分配新的 handle。生产布局回归先观察到：
单个 192×192 图标的预热命令控件每帧分配 147,528 B、3 allocs；优化后为 0 B、0 allocs
（M1，100 次微基准）。这只测一个空标签图标控件，不是整页所有工作都零分配。

- renderSVGIcon 仍按原方式栅格化 NRGBA，再采用与 Gio 原 NewImageOp 相同的
  image.NewRGBA + draw.Draw(Src) 转换一次，只将转换后的 RGBA 放入缓存；原 NRGBA
  不再强引用保留。一个栅格只保留一份 CPU 像素，避免“源像素 + 上传副本”双份缓存。
- 已登记的不可变 SVG RGBA 通过 iconImageOp/sync.Once 复用上传标识。覆盖 Ribbon、
  状态栏、共用图标按钮、创建菜单、确认/反馈和 JSON 图标。未登记图片继续获得新标识，
  不把可能变化的正文图片纳入静态缓存。
- 不修改 Gio 依赖、栅格尺寸、描边、颜色、控件几何或输入处理。原有 size/tint 缓存
  key 和保留策略不变；没有声称可清空全部约 79 MiB 图形映射。

着色断言改为比较预乘 GPU 颜色，避免低 alpha 像素反预乘时放大 8-bit 量化误差。
以旧 svg_icons.go 的临时 Go overlay 渲染基准，与新版实际 GPU 输出对照：Word/Excel
总览、品牌及选中态共 10 张 1x/2x PNG 文件逐字节一致。原图目录
`/tmp/glance-graphics-before-shots` 与 `/tmp/glance-graphics-after-shots`，日志
`/tmp/glance-graphics-before-visual.log`、`/tmp/glance-graphics-after-visual.log`。
旧着色断言失败源于表示改变，不能据此忽略真正的显示差异；这里用实际 GPU 图像相同
作为显示不变的证据。

SVG 全资源、设置导航/描边和实际渲染回归通过（2.204s，无 SKIP）；图标并发和未登记
图片新标识 race 检查通过（2.810s）；主页实际渲染、菜单交互、Ribbon 几何及录制边界
检查通过（1.176s）。独立审查无待处理问题。macOS/Windows 根入口均编译通过。
继续复用全局 Go 缓存，输出覆盖原临时二进制；未重跑有已知失败的整个桌面套件。

#### 本机单行 Word 复测（2026-10-19 19:53）

新进程 PID 68933，用户确认打开同一单行 Word、不使用 AI。归还后 HeapAlloc=
71,024,144 B（约 67.7 MiB），physical footprint=210,718,464 B（约 201.0 MiB），
RSS=388,268,032 B（约 370.3 MiB）。字体清单仍是一次 Word PingFang index 3 解析。

采样中 SVG 图像约 3.44 MiB，均落在 renderSVGIcon→image.NewRGBA；PNG 约 1.87 MiB。
原先 paint.NewImageOp→image.NewRGBA 的像素复制路径未出现；此前同文档旧快照该路径
约 2.22 MiB。后续外部 footprint 的图形分类为 IOSurface 约 49.0 MiB、
IOAccelerator graphics 约 39.4 MiB，与先前 Word 阶段约 40 MiB 的量级一致。

本轮验证了重复复制/上传身份抖动的消除，但未取得严格控制窗口/历史操作的整进程
footprint 下降结论。上个可用 Word 对照还包含本轮之前的 AI/语言目录变更，不能将
Go 堆约 80→68 MiB 全归因于本轮图标优化，更不能声称已节省约 79 MiB 图形资源。
原始快照前缀 `/tmp/glance-mac-graphics-word*`。

倒三角补充复查：当前使用 U+25BE 的界面入口均通过 layoutDropdownLabel 绘制矢量
指示器，覆盖 Word/Excel Ribbon 和 adaptive choice。Excel field 模式只用于字体、
字号和数字格式，当前传值会覆盖 display label，不携带字符标记；PPT/PDF/Markdown
工具栏没有 U+25BE 使用点。本结论针对界面控件，不替换文档正文字符。
