# 相邻盒子收退验证（2026-10-23）

用户要求：原始 B 不变，预分配只是估计；只调整当前盒子与紧邻下一盒子的分界，已校验盒子的文字由本盒独立拥有。用户 UE 尚未验收。

## 当前行为

- `K8IiR0` 普通滚动累计 16 逻辑 dp 调整一个扩展字素簇，组合字符与 emoji 不拆分；Shift 使用实际排版行。
- 收取下一盒开头，或把当前盒末尾退给下一盒。原预分配可以退回，不再仅允许退回刚收进的文字。不跳过空的下一盒，不自动改动第三盒。
- 当前／下一盒已校验时停止转移。已校验盒子增删只影响本盒，保存重开后也不会从 B 恢复已删除内容。未校验页新输入的尾部仍保留本地保护。
- A 滚动联动 A/X 并确定当前盒子；顶部页码与导航条使用同一目标。X 页内滚动仅浏览。
- 初次导入后，编辑、调字及字号变化不再触发全书归属重分配。相邻收退只发布两盒变化并请求局部排版，每个阈值仍独立撤销。

## 卡顿原因与修改

原路径每帧同步所有页编辑器，模型调整还反复查找页边界。大稿件因此在同步路径扫描大量文字；调整后再走全书分配，又会改动用户未操作的盒子。

现在仅同步可见编辑器，页索引缓存且边界统计单遍完成；单帧最多处理四步或约 6ms 后让出后续步骤。后台排版使用既有归属，只更新请求页面；按行预览同时验证下一盒内容，避免使用邻盒编辑前的旧几何。

892 页、约 180 万字符合成数据的定向基准：

| 路径 | 修改前 | 修改后 |
| --- | --- | --- |
| 同步滚动帧路径 | 约 54ms／49MB 分配 | 约 0.40ms／153KB 分配 |
| 模型逐字调整 | 约 2.01s | 约 4.38ms |

这是合成基准中的指定代码路径，不代表真实窗口总帧耗时或用户设备上的滚轮手感。日志：`/private/tmp/proofread-scroll-before.log`、`/private/tmp/proofread-adjust-before.log`、`/private/tmp/proofread-final-perf.log`。

## 验证结果

- `go test ./internal/proofreading ./internal/application -count=1`：通过。覆盖双向相邻调整、原分配退回、空邻盒、已校验边界、独立增删保存恢复及过期按行预览。
- `go test ./internal/desktopui -run '^TestProofreading' -count=1 -v`：30 项通过，无 FAIL／SKIP；包含 1x／2x 实际 Metal GPU 渲染、IME、可见编辑器和大稿件路径。日志 `/private/tmp/proofread-adjacent-gpu.log`。
- `go build ./...`、`git diff --check`：通过；链接器仍有既有重复 `-lobjc` 警告。
- 独立只读代码复核后无剩余可操作发现。
- 已打开独立合成试用窗口“Glance 相邻盒子试用 · 892 页”，构建 `/private/tmp/GlanceAdjacent892.app`，不接工作区持久化。观察到目标页与导航条一致及 `+35字` 反馈；未将这一观察当作用户原稿或真实鼠标 UE 验收。用户原运行进程不代表已加载此构建。

## 全量测试限制

`go test ./...` 未通过，日志 `/private/tmp/proofread-adjacent-full.log`。本机端口／Unix socket bind 被沙箱拒绝，Markdown 测试因此 panic，中断桌面包；其他失败也未在本次范围内解决。不能据专项通过声称全仓通过。

失败项：`TestAgentHistorySearchAndDateGroups`、`TestParseCellRangeAddress`、`TestSpreadsheetWindowsDragTracksNativeCursor`、`TestGridResizeBoundaryDoubleClickAutoFitsColumn`、`TestCellSelectionBoundsClampsToXLSXLimits`、`TestMarkdownParsePublishesTextBeforeImagesAndCancelsStaleFetch`、`TestPrivateFontSceneExportKeepsLoadedResourceAfterRefresh`、`TestBrokerListsOnlyDirectChildrenAndSkipsGit`、`TestMCPCLIRecognizesServeAndDocumentsStdoutContract`、`TestMCPCLIUsesStableEndpointRegardlessOfLegacyTarget`、`TestDesktopMCPServesOneStableEndpoint`、`TestDesktopMCPDoesNotCreateRegistryDirectory`、`TestDesktopMCPIPCUsesPrivateSocketAndCleansUp`、`TestDesktopMCPIPCServesExistingCapabilityCatalog`、`TestDesktopMCPInstanceIDIsNotUsedForRouting`、`TestMCPStdioSubprocessTargetsStableDesktopRuntime`、`TestMCPStdioSubprocessIgnoresStaleLegacyID`、`TestLoadManifestLoadsCuratedCorpus`。
