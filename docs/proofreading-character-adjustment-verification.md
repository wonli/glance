# 逐字收退导航条试用记录（2026-10-22）

用户批准先写代码试用，UE 尚未验收。Pen `K8IiR0` 保留右侧 54dp 文字导航条外形，操作改为专用收退区域。

## 本轮行为

- A 区滚动联动 A/X；以 A 视口上方三分之一阅读线经过的页面选目标，导航条显示目标页。X 文字增减不改变外层页面位置或目标。
- 导航条普通滚动每累计 16 逻辑 dp 收退一个扩展字素簇；小位移累计、反向时清除旧余量，不加速。Shift 按实际排版行粗调；部分平台 Shift 产生水平滚动量时也能处理。
- 每个阈值单位各有一个共享撤销步骤。按行输入等待修订号匹配的排版；A 切页、撤销／重做、分页、状态切换或加载另一稿件时清掉待处理调节输入。
- X 页内滚轮／滚动条只浏览；X 页外只联动 A/X，不再改变文字归属。菜单取消“锁定当前页”。
- 首次有效收字自动建立 `Manual` 人工边界，保护原有和新输入文字；只有收进的连续尾部能退回。边界随版本 2 副本的可选 `manual` 字段保存，旧 `Locked` 副本继续兼容。已校对和独立页仍为屏障。
- 末尾自动进入可见范围，短暂标出最后字符，导航条显示本次净收退数、字符样例（换行 ↵、空格 ␣）和到达边界提示。保护边界落在旧副本组合字符内部时，保留整个组合字符。

## 回归与复核

先观察无锁定调整失败，再实现自动人工边界；先观察跨页收字后重排导致反向归属顺序错误，再修复历史退回页；先观察累积三步只生成一次撤销，再拆为逐步历史。另覆盖组合字符与 emoji、空格／换行、已校对屏障、保存恢复、排版等待、切页丢弃旧目标输入、中文 IME 未提交草稿、缩放一致性和后台排版后露出末尾。

独立只读代码复核发现上述重排退回与撤销问题；修正后复核未发现其他可操作问题。

- `go test ./internal/proofreading ./internal/application -count=1`：通过。
- `go test ./internal/desktopui -run '^TestProofreading' -count=1 -v`：28 项通过，包含 1x／2x 真实 Metal GPU 渲染，无 SKIP。GPU 检查在有 Metal 访问权限的进程运行。
- `go build ./...`、`git diff --check`：通过；链接器存在既有重复 `-lobjc` 警告。
- 真实窗口：独立无工作区持久化的 QA 构建，合成 4 页 PRODUCT REPORT PDF 与编号中文／英文 B。观察到小位移累计后收进一个字符并显示 +1 字、A 区滚动后 A/X 联动且导航条切到第 2 页。Shift 按行及 IME 的证据来自输入路由回归，不宣称已做真实鼠标硬件的 UE 验收。
- 截图：`/private/tmp/glance-proofread-navigator-captures/navigator-1x.png` 与 `navigator-2x.png`。定向完整日志：`/private/tmp/glance-proofread-navigator-final-all.log`。

## 全量测试限制

本轮尝试 `go test ./...`，未通过。完整日志 `/private/tmp/glance-proofread-navigator-full.log`。其中本机端口／Unix socket 的 bind 被沙箱拒绝，Markdown 异步测试因此 panic，未能跑完桌面包；另有其他模块失败，未把全量运行称为通过。失败名称如下：

- `TestAgentHistorySearchAndDateGroups`
- `TestParseCellRangeAddress`
- `TestSpreadsheetWindowsDragTracksNativeCursor`
- `TestGridResizeBoundaryDoubleClickAutoFitsColumn`
- `TestCellSelectionBoundsClampsToXLSXLimits`
- `TestMarkdownParsePublishesTextBeforeImagesAndCancelsStaleFetch`
- `TestPrivateFontSceneExportKeepsLoadedResourceAfterRefresh`
- `TestBrokerListsOnlyDirectChildrenAndSkipsGit`
- `TestMCPCLIRecognizesServeAndDocumentsStdoutContract`
- `TestMCPCLIUsesStableEndpointRegardlessOfLegacyTarget`
- `TestDesktopMCPServesOneStableEndpoint`
- `TestDesktopMCPDoesNotCreateRegistryDirectory`
- `TestDesktopMCPIPCUsesPrivateSocketAndCleansUp`
- `TestDesktopMCPIPCServesExistingCapabilityCatalog`
- `TestDesktopMCPInstanceIDIsNotUsedForRouting`
- `TestMCPStdioSubprocessTargetsStableDesktopRuntime`
- `TestMCPStdioSubprocessIgnoresStaleLegacyID`
- `TestLoadManifestLoadsCuratedCorpus`

## 待 UE 试用

16dp 阈值为第一版固定手感，尚未做不同鼠标／触控板的硬件标定。右侧刻度目前是全稿字符量与当前页结束边界的概览，不是逐段真实缩略文本，也不提供拖动跳转。本轮保留“原有文字不退回”的保护规则；是否符合实际校对节奏，由用户试用后决定。
