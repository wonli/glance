# Shared Font Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Word/Excel 共享字体目录、扫描缓存和手工刷新、已确认的轻量单列表，以及可为空的私有字体下载源与完整渲染接入。

**Architecture:** 应用运行时持有共享字体服务，独立目录包输出中性元数据及固定版本资源快照。扫描、下载与解析在后台执行，Gio 仅消费快照和提交意图；Word、Excel、PDF 各自适配同一资源集合，不重写排版引擎。

**Tech Stack:** Go 1.27、Gio v0.10.3、go-text/typesetting v0.3.5、现有工作区与应用运行时；不新增字体解析依赖。

**Spec:** [2026-10-15-shared-font-library-design.md](../specs/2026-10-15-shared-font-library-design.md)

## Global Constraints

- 下载字体只供 Glance 使用，正文存持久数据目录；可重建扫描索引存缓存目录。
- 不设本机、在线、文档分类切换；文档字体置顶，无在线数据不留入口、空分组或占位。
- 名称后显示小型“在线”标签，右侧保留下载按钮；下载后另行应用，不能迟到修改旧选区。
- 手工刷新必须绕过本次扫描的元数据及第三方缓存；失败保留有效目录，不删除已下载字体。
- 字体变更同步影响 Word、Excel 和 PDF，刷新本身不修改 dirty 状态或单元格尺寸。
- 不改写原文档字体名来隐藏缺失；不自动嵌入 DOCX/XLSX，不绕过 PDF 嵌入检查。
- 生产及测试文件按职责拆分，通常不超过约 500 行；遵守仓库 AGENTS.md 的后台任务及 Gio 约束。
- 每个任务先写行为测试、观察预期失败，再实现并运行检查；当前清单全部完成前不宣称目标完成。

## Review Focus

- 同大小、同修改时间的字体替换：手工刷新必须识别内容改变，任务 2 验证。
- 同名不同来源及集合多字面：匹配身份稳定，不按名字误合并资源，任务 1、4 验证。
- 刷新、下载注册、导出并发：旧结果不得覆盖新资源，单次导出固定快照，任务 3、4、5 验证。
- 原生中文输入和菜单命中：组合文本不提交字体，空白区域、下载按钮、行选取不串事件，任务 6 验证。
- 只有缓存而字体文件已消失：不可继续显示为可用或在失败时清空整个目录，任务 2、3 验证。

## Task 1: 中性字体目录和资源身份

**Files:** 新建 `internal/fontcatalog/{catalog.go,metadata.go,resolve.go,resources.go}` 及对应 `_test.go`。

**Interfaces:** 定义 `Source`（system/private/online）、`Family`、`Face`、`Diagnostic`、`Snapshot` 和 `ResourceID`。`Snapshot` 不暴露可变切片或 map；提供 `Families() []Family`、`Search(query string) []Family`、`Resolve(request Request) (Resolution, error)`、`OpenFace(id ResourceID) (io.ReadCloser, error)`。`Request` 包含家族、字重、斜体和待覆盖字符；`Resolution` 含请求名、实际资源 ID 和替代原因。

- [ ] 写 `TestFamilyAliasesAndAspects`、`TestSameNameDifferentContent`、`TestCollectionFaceIdentity`、`TestSnapshotIsolation`：以可再分发的合成/现有测试字体验证别名、字重、来源与 face index；修改返回切片不能污染快照。
- [ ] 运行 `go test ./internal/fontcatalog -run 'TestFamily|TestSameName|TestCollection|TestSnapshot' -count=1`，观察缺少能力的预期失败。
- [ ] 通过已锁定 go-text API 解析字体元数据和本地化 name table，建立稳定内容身份及规范名称；通用匹配不依赖 WordTextStyle。按需加载并限制字面数据缓存，不全量加载字形。
- [ ] 运行 `go test ./internal/fontcatalog -count=1`；通过后提交本任务。

## Task 2: 可刷新扫描器与原子磁盘缓存

**Files:** 新建 `internal/fontcatalog/{scanner.go,scanner_system.go,cache.go,cache_files.go}` 及对应测试；修改 `internal/wordfont/cache.go` 的扫描准备边界。

**Interfaces:** `Scanner.Scan(ctx context.Context, previous Snapshot, force bool) (ScanResult, error)`；`ScanResult` 含资源、诊断和各枚举范围是否完成。`LoadCache(path string) (Snapshot, error)`、`SaveCache(path string, snapshot Snapshot) error`。缓存 schema 初始值 1，带平台与解析器版本。

- [ ] 写 `TestCacheRestartReuse`、`TestForceRefreshSameStatDifferentBytes`、`TestDeletedResourceUnavailable`、`TestBrokenCacheRebuild`、`TestUnreadableScopeRetainsDiagnostics`、`TestCacheWriteFailureKeepsMemoryCatalog`。临时目录和注入的字体目录枚举器隔离用户字体。
- [ ] 运行 `go test ./internal/fontcatalog -run 'TestCache|TestForceRefresh|TestDeleted|TestBroken|TestUnreadable' -count=1`，观察预期失败。
- [ ] 用 `fontscan.DefaultFontDirectories` 发现平台目录，独立枚举和解析，复用自己管理的索引。不能以 `fontscan.SystemFonts` 作为可刷新目录：当前依赖在进程内使用 `sync.Once`，换缓存路径也不能强制重新扫描。
- [ ] 增量扫描比较元信息，强制扫描重新解析并核验内容指纹；索引写到同目录暂存文件并原子替换。枚举失败保留范围诊断，失败扫描不以空目录覆盖有效缓存。
- [ ] 运行 `go test ./internal/fontcatalog -count=1`；通过后提交本任务。

## Task 3: 应用共享服务和有序发布

**Files:** 新建 `internal/application/{fonts.go,fonts_refresh.go,fonts_test.go}`；修改 `internal/application/runtime.go` 及其关闭生命周期；在 `internal/desktopui/runtime.go` 接入，新增 `internal/desktopui/font_service.go`。

**Interfaces:** `FontService.Snapshot() Snapshot`、`Refresh(ctx context.Context, force bool) error`、`Register(ctx context.Context, resource PrivateResource) error`、`Subscribe() (<-chan FontEvent, func())`、`Close() error`。`FontEvent` 含版本、刷新状态、错误及缓存持久化诊断。运行时拥有服务，headless 与桌面均可注入测试实例。

- [ ] 写 `TestFontRefreshCoalesces`、`TestFontRefreshFailureRetainsSnapshot`、`TestFontRefreshCannotOverwriteRegistration`、`TestFontUnchangedRefreshKeepsVersion`、`TestFontServiceCloseDrainsWorkers`，用屏障通道控制真实竞态顺序。
- [ ] 运行 `go test ./internal/application -run '^TestFont' -count=1`，观察预期失败。
- [ ] 启动在后台先加载缓存再增量核验；共享刷新合并，有序处理注册及刷新结果，无变化不增加目录版本。窗口只订阅事件，关闭浮层不停止服务；运行时关闭取消并等待工作线程。
- [ ] 运行 `go test ./internal/application -run '^TestFont' -count=1` 和 `go test -race ./internal/fontcatalog ./internal/application -run 'TestFont|TestCache|TestForceRefresh' -count=1`；通过后提交本任务。

## Task 4: Word、Excel 及 PDF 使用统一资源快照

**Files:** 修改 `internal/wordfont/{system.go,font_families.go}`，新建 `internal/wordfont/catalog.go`；新建 `internal/desktopui/font_rendering.go`，修改 `word_text_shaper.go`、`word_pagination.go`、`word_pdf_export.go`、`grid_paint.go`、`app.go`；修改 `internal/format/{wordpdf,sheetpdf,markdownpdf,presentationpdf}/export.go` 的字体构造接缝。

**Interfaces:** `wordfont.NewWithSnapshot(snapshot fontcatalog.Snapshot) (*Shaper, error)`。保留现有系统构造函数供兼容调用，但导出入口允许注入固定快照；不得只给全局旧 shaper 加一个可变字段。Gio 接入以快照构造内容字体集合，保持 UI 字体不变。

- [ ] 写 `TestCatalogShaperUsesPrivateFace`、`TestFontRefreshInvalidatesWordLayout`、`TestFontRefreshInvalidatesGridText`、`TestExportPinsFontSnapshot`。断言实际资源身份和字形尺寸，刷新不改 dirty/行列存储尺寸；覆盖同名字面冲突及中英混排。
- [ ] 运行相应包中上述命名测试，观察旧系统目录不能解析新资源或未更新缓存的失败。
- [ ] Word shaper 用快照元数据取代内部系统扫描来源，保留脚本槽和兼容度量。Word 布局键、Excel 内容/测量缓存带字体版本；Gio 内容 shaper 在后台准备后安全发布，私有字体加入预览及 Excel 内容集合。
- [ ] PDF、打印、Markdown/PPTX 共享调用链接收同一任务快照及资源租约，任务结束前资源不释放；仍检查既有导出修订及字体嵌入限制。
- [ ] 运行 `go test ./internal/wordfont ./internal/wordlayout ./internal/format/wordpdf ./internal/format/sheetpdf ./internal/format/markdownpdf ./internal/format/presentationpdf -count=1` 及新 desktopui 定向测试；通过后提交本任务。

## Task 5: 私有字体下载与可为空的 Provider

**Files:** 新建 `internal/fontdownload/{provider.go,manifest.go,download.go,store.go}` 及对应测试；新增 `internal/application/fonts_download.go`。

**Interfaces:** `Provider.List(ctx context.Context) ([]Entry, error)`；`Entry` 含固定版本、HTTPS URL、SHA-256、大小上限、授权文本和预期身份。`Downloader.Fetch(ctx context.Context, entry Entry, progress func(Progress)) (PrivateResource, error)`；结果通过任务 3 注册，不直接改 UI 或文档。

- [ ] 写 `TestEmptyProviderDoesNoNetwork`、`TestDownloadVerifyAndRestart`、`TestDownloadCancelAndRetry`、`TestConcurrentDownloadDeduplicates`、`TestInvalidDigestOrFaceNotPublished`、`TestDownloadSizeAndRedirectLimits`。仅用本地测试 HTTP 服务及测试字体；HTTPS 校验策略通过注入测试 transport 保留生产校验。
- [ ] 运行 `go test ./internal/fontdownload -count=1`，观察预期失败。
- [ ] 下载正文暂存后验证再发布到 `fonts/objects/<sha256>`；授权和资源登记原子写入持久数据目录，扫描缓存可从登记重建。限制最多 2 个并发下载、单资源 64 MiB、HTTP 客户端 120 秒，目录条目可进一步收紧大小上限；重定向只允许清单信任的 HTTPS 主机。
- [ ] 使用空发布清单起步；不虚构在线资源。支持取消、进度、失败重试、相同资源去重与启动清理暂存；注册成功即可供后续绘制/导出任务采用，不自动应用字体。
- [ ] 运行 `go test ./internal/fontdownload ./internal/application -run 'TestDownload|TestEmptyProvider|TestFont' -count=1`；通过后提交本任务。

## Task 6: Pen 单列表组件和安全应用意图

**Files:** 新建 `internal/desktopui/font_picker_{state,events,layout,rows,document_fonts}.go` 及行为/交互/视觉测试；修改 `word_text_formatting.go`、`spreadsheet_formatting.go`、`office_ribbon_dropdown.go`、`office_ribbon_menu_definitions.go`、`word_layout.go`、`spreadsheet_toolbar_layout.go` 及稳定会话状态定义。

**Interfaces:** `App.openFontPicker(target fontPickerTarget)`、`App.processFontPickerEvents(gtx layout.Context)`、`App.layoutFontPicker(gtx layout.Context) layout.Dimensions`。`fontPickerTarget` 固定 session、修订、Word run 或 Excel sheet/range 身份；`fontPickerState` 持有 Editor、纵向 List、稳定按钮及根坐标锚点。

- [ ] 写 `TestFontPickerWordAndExcelApply`、`TestFontPickerDocumentFontsFirst`、`TestFontPickerEmptyProviderNoPlaceholder`、`TestFontPickerSearchIMEAndClear`、`TestFontPickerRejectsStaleSelection`、`TestFontPickerRefreshInteraction`、`TestFontPickerDownloadClickDoesNotApply`；断言一次应用、无操作不 dirty、下载不触发行选择、刷新保留搜索与滚动。
- [ ] 运行 `go test ./internal/desktopui -run '^TestFontPicker' -count=1 -v`，观察预期失败。
- [ ] 按 Pen `BICaw` / `Em1rj` 实现宽 388dp 的紧凑单列表，受 viewport 限界；行高 44dp，搜索框 36dp，标题栏 38dp，底部栏 34dp。只有当前文档使用、本机及可选更多字体分组；本机短预览、在线标签及下载按钮，没有分类切换及大预览卡片。
- [ ] 文档字体在后台从修订快照汇总，Excel 遍历文档中性可读取范围/格式摘要，不能只报告当前视口。数量较多时同样虚拟化；提供加载状态而不阻塞 UI。字体选择通过现有 Word/Excel 命令执行，验证捕获目标仍有效。
- [ ] “刷新字体 / 刷新中… / 重试刷新”接任务 3；搜索无结果可清空。列表处理键盘、Escape、外部点击与焦点恢复；复用现有 Ribbon 锚点/滚动计算，保持同一录制边界内的 pointer PassOp Push/Pop。
- [ ] 增加合成数据 1x/2x GPU 渲染断言与截图，对比字体名、短预览、行内标签、下载与刷新按钮；更新旧手填对话框测试为新用户流程。
- [ ] 运行 `go test ./internal/desktopui -run '^TestFontPicker|^TestWordFontFamily|^TestOfficeRibbonDropdownRecordingBoundary' -count=1 -v`；通过后提交本任务。

## Task 7: 端到端与兼容性验证

**Files:** 补充 `internal/format/docx/`、`internal/format/xlsx/` 字体保存重开测试；更新 `docs/architecture.md`、`docs/office-support.md` 和字体验证记录 `docs/font-library-verification.md`。

**Interfaces:** 使用前述公开目录、服务及下载接口，不另建测试专用生产分支。

- [ ] 写 DOCX/XLSX 字体修改保存重开用例，核对新字体名与代表性未修改样式、公式、图片和包结构。写缺失字体文档在注册资源前后渲染、导出一致性的集成用例。
- [ ] 运行新增测试确认失败指向缺少集成保障，再补齐边界；不可把只断言字体名称作为渲染验证。
- [ ] 运行 `go test ./...`、`go test -race ./internal/fontcatalog ./internal/fontdownload ./internal/wordfont ./internal/application`、`git diff --check`。逐项记录失败和 SKIP；GPU SKIP 不算视觉通过。
- [ ] 退出旧进程，构建并启动新进程，以合成文档核验 Word/Excel 打开选择器、中文搜索、刷新、下载与再次应用、字体实际绘制和 PDF；核验 1x/2x。真实字体源未启用时明确记录为空，不宣称已能下载公网字体。
- [ ] 更新支持文档，提交集成证据。完成独立代码审查并修复有效问题；若没有可用审查工具则明确记录审查限制。最终报告改动、测试、风险和所有未完成项。

## 执行前证据（历史）

- 当前仓库在 `main`，工作区干净，未创建新 worktree。
- 已检查 Gio 字体集合 API 和 go-text 扫描源码；`fontscan.SystemFonts` 的进程级 `sync.Once` 已确认，任务 2 禁止靠换缓存目录假装刷新。
- `go test ./internal/wordfont ./internal/wordlayout -count=1` 已全量通过；字体选择及 Ribbon 录制边界的 desktopui 定向回归通过。desktopui 链接器报告既有重复 `-lobjc` 警告，命令退出成功；尚未运行全仓测试或新 UI 的 GPU 验证。
- 以上为计划创建时的状态。用户随后批准当前会话原地执行；未新建 worktree，未推送远端。


## 执行结果（2026-10-15）

下表为实际交付状态。前面的逐条清单保留原始测试名称和预期步骤，不将没有单独留存 RED 记录的步骤追记为已验证。实际测试名称、命令和限制以永久验证记录为准。

- [x] Task 1：中性目录、内容身份、本地化别名、来源冲突及只读快照；提交 `81e174a`。
- [x] Task 2：独立扫描、增量索引、强制哈希、删除/失败范围、原子缓存；提交 `26a1f34`，后续补充成功检查时间及大字体元数据扫描。
- [x] Task 3：运行时共享服务、订阅、刷新合并、注册与发布排序、取消/关闭；提交 `538cebd`，后续修复关闭与偏好边界。
- [x] Task 4：Word/Gio/导出快照接入，字体版本失效，同名来源选择；实际字形度量、已加载资源删除后 PDF 仍可导出、刷新不取消打印/导出均有回归。
- [x] Task 5：空生产清单、私有对象和授权登记、HTTPS/大小/摘要/静态 TTF/家族校验、取消重试、并发去重及重定向限制；测试不依赖公网。
- [x] Task 6：Word/Excel 共用单列表、文档置顶、中文搜索、缺失/空搜索状态、在线行与独立下载按钮、刷新状态、失效选区拒绝、键盘导航和关闭恢复焦点；1x/2x GPU 验证通过。
- [x] Task 7：DOCX/XLSX 保存重开、私有字体 PDF、完整后端竞态检测、构建、原生 Word/Excel 窗口核验、支持文档和独立只读代码审查。
- [ ] 全仓测试完全通过：实现前已有失败，完整结果和对照见 [验证记录](../../font-library-verification.md)。没有为本次字体任务修改无关设置、MCP CLI 或语料预期。
- [ ] 真实中文输入法候选组合阶段、物理打印机、Windows/Linux 原生窗口验证；当前只有中文已提交文本事件与 macOS 原生中文搜索/Retina 窗口证据。
- [ ] 公网字体源运营清单：按批准范围保持为空，属于后续配置，下载通路已实现并以合成 HTTPS 服务验证。

范围裁定：不预加载所有系统字形；任务持有已加载字节，首次读取前的外部变更报错。来源偏好存 Glance 索引，不新增 Office 内嵌资源绑定。Excel 摘要读取字体声明表，包含屏外样式，完整富文本枚举另列支持限制。
