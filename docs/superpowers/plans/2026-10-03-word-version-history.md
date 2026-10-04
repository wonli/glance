# Word 版本历史实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**目标：** 为本机 DOCX 增加按文件路径隔离的持久版本列表和完整包恢复，并让一次恢复成为可撤销、可重做的 Word 操作。

**架构：** 在 workspace SQLite 中保存按完整 DOCX 包 SHA-256 去重的快照；DOCX 适配器负责原子保存快照和整包撤销历史；desktop controller 在后台串联打开、保存、自动保存、另存为及恢复；Gio 文件功能区显示历史元数据并发起恢复。

**技术栈：** Go、Gio、现有 DOCX/OOXML 适配器、modernc SQLite、本地文件系统。不得依赖 LibreOffice 或其他外部 Office 程序。

**规格：** docs/superpowers/specs/2026-10-03-word-version-history-design.md

## 全局约束

- 当前 SQLite schema 为 user_version 4；版本历史迁移必须事务化并支持已有 1、2、3 版数据库。
- 每条历史记录保留完整 DOCX 包；每个规范化本地路径最多 50 条或 512 MiB，单个快照超过 512 MiB 时不录入。
- 内容去重键为路径键与完整快照 SHA-256；自动保存的不同内容版本最短间隔为 5 分钟，手动保存不受限频影响。
- 保存历史必须使用同一次成功磁盘写入的不可变包字节，不能在 UI 收到结果后重新 Snapshot 当前文档。
- 恢复前当前包必须可从 SQLite 读取；校验或保护失败时不应用目标包。恢复作为一次 Word undo/redo 历史操作。
- 所有快照和 SQLite I/O 都在 Gio 帧外执行；UI 只消费带会话和 revision 标记的异步结果。
- 保留现有保存失败、脏状态、未知 OOXML 部件和外部 Office 运行时边界；桌面启动命令为 go run ./cmd/desktop。

## 审查重点

- 从 user_version 1、2、3 升级时，既有 tabs、recents、favorites 与恢复数据均可读取；Task 1 覆盖迁移回归。
- DOCX 中适配器不理解的 XML、关系和媒体部件在恢复、撤销、重做后仍原样存在；Task 2 覆盖完整包回归。
- 保存磁盘写入后文档立即再次编辑时，历史仍对应已写入 revision；Task 3 覆盖并发编辑场景。
- 相同路径别名、另存为新路径和另存为已有路径时，历史只进入正确的规范路径时间线；Task 3 覆盖路径场景。
- SQLite 写失败、快照损坏、单包超限和清理时的保护版本失败都不能伪报成功或部分应用恢复；Task 1、3、4 分别覆盖数据库和恢复失败场景。

---

### Task 1：SQLite 版本记录与保留策略

**文件：**

- 修改：internal/workspace/store.go
- 新建：internal/workspace/word_versions.go
- 修改：internal/workspace/store_test.go
- 新建：internal/workspace/word_versions_test.go

**接口：**

- 在 internal/workspace/word_versions.go 定义 WordVersionSource、WordVersion、WordVersionInput。
- WordVersionInput 包含 PathKey、DisplayPath、Source、CapturedAt、Snapshot；WordVersion 返回 ID、PathKey、DisplayPath、Source、CreatedAt、SHA256、SizeBytes。
- Store 方法签名：AddWordVersion(WordVersionInput) (WordVersion, bool, error)、ListWordVersions(pathKey string) ([]WordVersion, error)、LoadWordVersion(pathKey string, id int64) (WordVersion, []byte, error)。
- 来源常量：WordVersionOpened、WordVersionManualSave、WordVersionAutoSave、WordVersionRestoreBefore。
- 默认配额常量为 50 条、512 MiB、单快照 512 MiB、自动保存间隔 5 分钟；validateWordVersionSize(size int64) error 检查单包边界。用 workspace 私有 wordVersionPolicy 覆盖测试配额，使保留策略测试使用小数据量而不创建数百 MiB 文件。
- 自动保存限频、SHA-256、单快照大小限制、事务插入、相同内容去重及按最旧优先清理均由 Store 实现。
- restore-before 写入触发保留清理时，将本次命中的版本 ID 排除在删除候选之外。

- [x] **步骤 1：先写迁移与存储回归**

  新增 TestWordVersionMigrateV1PreservesWorkspace、TestWordVersionMigrateV2PreservesWorkspace、TestWordVersionMigrateV3PreservesWorkspace，逐一确认旧 tabs、recents、favorites 和 documents 可读取。新增 TestWordVersionAddDeduplicatesByPathAndHash，确认同一路径同 SHA-256 只返回一个 ID、另一条路径允许相同包。新增 TestWordVersionListOmitsPayload，确认列表对象没有 BLOB。新增 TestWordVersionAutoSaveThrottle，使用固定 CapturedAt 验证 5 分钟内不同内容自动版本跳过、5 分钟后可记录、manual-save 不限频。用私有 wordVersionPolicy 的小型测试配额验证 TestWordVersionRetentionPrunesOldestAndProtectsRestoreBefore；单包大小通过 validateWordVersionSize 的 512 MiB 和 512 MiB+1 边界断言；TestWordVersionAddRollsBackOnFailure 用触发清理失败的 SQLite trigger 验证事务不留下新增记录并保留原有记录。

- [x] **步骤 2：运行 workspace 定向回归确认新断言失败**

  运行：go test ./internal/workspace -run 'Test(WordVersion|WorkspaceMigration)' -count=1
  预期：尚无版本 API 或 schema v4 时新测试失败。

- [x] **步骤 3：在 Store 中迁移到 user_version 4**

  建立 word_versions 表，列为 id、path_key、display_path、source、created_at、sha256、size_bytes、payload；约束 path_key 与 sha256 唯一并为 path_key/created_at/id 建立列表索引。在单个迁移事务中更新 user_version。拒绝高于 4 的数据库版本。

- [x] **步骤 4：实现版本读写和配额清理**

  AddWordVersion 在事务内校验大小、计算哈希、先处理重复与自动保存限频、插入新记录，再按 path_key 统计数量和总字节并清理最旧未保护记录。列表查询只 SELECT 元数据列。

- [x] **步骤 5：运行 workspace 定向回归确认通过**

  运行：go test ./internal/workspace -run 'Test(WordVersion|WorkspaceMigration)' -count=1
  预期：迁移、路径隔离、去重、限频、限额与事务原子性均通过。

### Task 2：DOCX 完整包快照保存与原子恢复

**文件：**

- 修改：internal/document/word.go
- 修改：internal/format/docx/document.go
- 修改：internal/format/docx/word_history.go
- 新建或扩展：internal/format/docx/word_version_history_test.go

**接口：**

- 在 internal/document/word.go 声明 WordPackageSnapshotter（Snapshot() ([]byte, error)）、WordPackageSnapshotSaver（SaveWithSnapshot() ([]byte, error)、SaveAsDocumentWithSnapshot(path string) ([]byte, error)）和 WordPackageSnapshotRestorer（RestoreSnapshot(snapshot []byte) error）。
- DOCX 的 SaveWithSnapshot() ([]byte, error) 和 SaveAsDocumentWithSnapshot(path string) ([]byte, error) 在一次 package 序列化中同时写文件并返回同一字节切片。
- RestoreSnapshot(snapshot []byte) error 先校验并解析完整包，再把整包替换记作一个 Word history entry；失败时恢复原 parts、投影、dirty 和 undo/redo 状态。

- [x] **步骤 1：先写实际写入字节和整包撤销回归**

  新增 TestSaveWithSnapshotReturnsExactWrittenPackage，断言返回字节等于磁盘文件。新增 TestSaveAsDocumentWithSnapshotAdoptsPathOnlyAfterSuccess，断言写入失败不更新源路径。新增 TestRestoreSnapshotPreservesUnknownParts，验证未知 XML/关系/媒体部件完整保留。新增 TestRestoreSnapshotIsSingleUndoRedoEntry，断言一次 undo/redo 分别恢复整包前态/目标态。新增 TestRestoreSnapshotInvalidPackageLeavesDocumentUntouched，覆盖损坏包和 reparse 失败回滚。新增 TestRestoreSnapshotIdenticalPackageStillCreatesDirtyUndoableEntry，锁定相同包恢复后的 dirty 与撤销语义。

- [x] **步骤 2：运行 DOCX 定向回归确认新断言失败**

  运行：go test ./internal/format/docx -run 'Test(SaveWithSnapshot|SaveAsDocumentWithSnapshot|RestoreSnapshot)' -count=1
  预期：新增接口和 restore history 行为尚未实现。

- [x] **步骤 3：实现同一序列化结果写盘并返回**

  调整 DOCX save 内部函数只生成一次完整 ZIP 字节；原子替换成功后由新方法返回这些字节。原 Save 与 SaveAsDocument 保持原有公开行为。

- [x] **步骤 4：实现完整包单步恢复**

  从目标快照构造可解析的临时 Document；锁住当前 Document 后保存整包前态、替换 parts/order 并 reparse。成功后记录一个历史项并标记 dirty；失败时回滚全部适配器状态。

- [x] **步骤 5：运行 DOCX 定向回归确认通过**

  运行：go test ./internal/format/docx -run 'Test(SaveWithSnapshot|SaveAsDocumentWithSnapshot|RestoreSnapshot)' -count=1
  预期：写盘字节一致，未知部件和 undo/redo 整包一致，失败路径不改变当前文档。

### Task 3：打开、保存、自动保存与另存为记录版本

**文件：**

- 修改：internal/desktopui/app.go
- 修改：internal/desktopui/layout.go
- 修改：internal/desktopui/runtime.go
- 修改：internal/desktopui/word_toolbar.go
- 修改：internal/desktopui/workspace.go
- 新建：internal/desktopui/word_version_history_test.go

**接口：**

- saveResult 新增单独的 historyErr，区分源文件保存结果和历史入库结果。
- 保存 worker 在 session adapter 写锁内完成 DOCX 保存并取得该次写盘字节与 operation token revision；锁外按目标路径把这份不可变快照写入 workspace。
- desktopui 定义 wordSaveReason、wordSaveManual、wordSaveAuto 和 startSaveForReason(reason wordSaveReason)；原 startSave() 保持手动保存入口。maybeAutoSaveWord 明确传 wordSaveAuto，延迟草稿保存由 documentSessionState.wordSaveAfterDraftsReason 保留来源。
- 打开本地 DOCX 成功后记录 opened 基线；失败只提示历史暂不可用，不阻断文档打开。
- Save As 使用目标路径路径键；常规保存使用保存开始时捕获的活动路径。路径键复用 canonicalSessionPath 规范化。

- [x] **步骤 1：先写保存/打开生命周期回归**

  新增 TestWordVersionOpenAddsAndDeduplicatesBaseline，断言首次打开插入 opened、重开同一包不重复、磁盘包变化后插入新基线；TestWordVersionOpenDatabaseFailureDoesNotBlockOpen，断言 Store 不可用仍可读取 DOCX。新增 TestWordVersionSaveFailureKeepsDirtyWithoutHistory，断言磁盘写入失败保持 dirty 且无记录。新增 TestWordVersionHistoryFailureKeepsSuccessfulDiskSaveClean，断言磁盘保存成功、历史写失败时 DOCX clean 且状态区分两者。新增 TestWordVersionSaveCapturesWrittenRevision，在历史入库被阻塞时再编辑文档，断言历史 BLOB 等于磁盘文件且不等于后续编辑态。新增 TestWordVersionSaveAsUsesTargetPathTimeline，分别断言新目标路径独立建线和已有目标沿用原记录。新增 TestWordVersionAutoSaveUsesThrottledSource，断言手动/自动来源不同且限频只作用于自动历史记录。

- [x] **步骤 2：运行 desktopui 定向回归确认新断言失败**

  运行：go test ./internal/desktopui -run 'TestWordVersion(Open|Save|AutoSave|SaveAs)' -count=1
  预期：当前保存结果没有快照或 historyErr，生命周期断言失败。

- [x] **步骤 3：贯通打开基线和保存快照**

  在后台打开与保存流程调用 workspace Store；Save、AutoSave 与 Save As 共用快照记录路径，但传入各自的 source。源文件保存成功才开始手动/自动版本记录。

- [x] **步骤 4：区分源文件与历史写入结果**

  UI 消费 saveResult 时先按源文件结果处理 dirty 和关闭流程；源文件写入失败时保留 dirty 且不记录历史；文件写入成功而版本记录失败时显示“文件已保存，但历史版本未记录”。若保存后文档已再次修改，状态同时指出当前文档仍有未保存修改。

- [x] **步骤 5：运行 desktopui 定向回归确认通过**

  运行：go test ./internal/desktopui -run 'TestWordVersion(Open|Save|AutoSave|SaveAs)' -count=1
  预期：每个路径的历史只记录实际写入包，自动保存限频不改变原自动保存与 dirty 行为。

### Task 4：恢复事务与 Word undo/redo 同步

**文件：**

- 修改：internal/document/word.go
- 修改：internal/desktopui/app.go
- 修改：internal/desktopui/runtime.go
- 新建或扩展：internal/desktopui/word_version_history.go
- 新建或扩展：internal/desktopui/word_version_history_test.go

**接口：**

- App 增加 wordVersionHistoryResults、wordVersionRestoreResults 两个异步结果通道；result 携带 sessionOperationToken、documentGeneration、路径键以及版本元数据、Word blocks 或错误。
- startRestoreWordVersion(state *documentSessionState, versionID int64) 启动后台恢复并返回；完成结果只在 token 和 document generation 匹配时发布。
- 操作顺序为读取版本 BLOB、校验存储哈希、由 DOCX adapter 解析校验目标包、捕获并写入当前包的 restore-before 保护版本、最后调用 RestoreSnapshot。
- 任一步失败不应用目标包；restore-before 入库失败时拒绝恢复。恢复成功后由 controller 发布 blocks、dirty、revision 并触发分页更新。

- [x] **步骤 1：先写恢复成功和失败回归**

  新增 TestWordVersionRestorePersistsProtectionBeforeApply，确认 RestoreSnapshot 执行前 restore-before 已可从 Store 读取。新增 TestWordVersionRestoreIsSingleUndoRedoOperation，断言 restore 后 dirty、一次 undo 回到恢复前、redo 回到所选版本。新增 TestWordVersionRestoreRejectsCorruptPayload，分别覆盖 SHA-256 不匹配和 DOCX 解析失败并断言当前包字节不变。新增 TestWordVersionRestoreProtectionWriteFailureRejects，断言保护版本入库失败时不调用适配器恢复。新增 TestWordVersionRestoreBeforeSurvivesPruning，确认配额清理不能删除本次保护快照。

- [x] **步骤 2：运行 desktopui 恢复定向回归确认新断言失败**

  运行：go test ./internal/desktopui -run 'TestWordVersionRestore' -count=1
  预期：尚无异步恢复入口或保护版本流程。

- [x] **步骤 3：实现后台恢复操作**

  使用 session mutation operation 与 adapter 写锁串行化恢复；读取 BLOB 和校验在帧外完成。先持久化当前完整包，随后调用 DOCX RestoreSnapshot；只有成功结果进入 UI 状态。

- [x] **步骤 4：发布恢复结果**

  检查 token 和 document generation，刷新 Word blocks、撤销/重做能力、脏状态、revision 与分页请求；遇到错误显示可读状态并保持视图内容。

- [x] **步骤 5：运行恢复定向回归确认通过**

  运行：go test ./internal/desktopui -run 'TestWordVersionRestore' -count=1
  预期：恢复、undo/redo、失败原子性和保护版本配额行为均通过。

### Task 5：版本历史列表、交互与最终验收

**文件：**

- 修改：internal/desktopui/app.go
- 修改：internal/desktopui/layout.go
- 修改：internal/desktopui/word_toolbar.go
- 新建：internal/desktopui/word_version_history.go
- 新建或扩展：internal/desktopui/word_version_history_test.go
- 修改：docs/word-ribbon-todo-zh.md
- 修改：docs/office-support.md

**接口：**

- File ribbon 中已有的 version-history action 打开本地路径的历史列表。
- 版本列表显示时间、来源和大小；列表只读元数据；选中版本后可确认恢复。
- 未命名 Word 文档禁用 action，并在语义描述中解释需要先保存；历史列表、点击状态和异步状态保存在每个 App 的稳定 view state，保持多窗口各自独立。
- 历史列表读取、BLOB 加载和恢复不在 Gio layout/frame 函数中进行。

- [x] **步骤 1：先写 ribbon 与历史列表交互回归**

  新增 TestWordVersionHistoryActionEnabledOnlyForSavedDOCX，断言有路径启用、草稿禁用。新增 TestWordVersionHistorySemanticDescriptionRequiresSave，断言禁用原因包含“请先保存文档”。新增 TestWordVersionHistoryListShowsMetadataAndOrder，断言列表时间倒序且每项显示来源、时间、字节大小。新增 TestWordVersionHistoryLoadFailureIsVisible，断言查询错误可见。新增 TestWordVersionRestoreRunsAsyncAndLocksConflictingOperations，断言恢复在帧外运行，恢复期间正文修改与保存入口禁用。

- [x] **步骤 2：运行 desktopui UI 定向回归确认新断言失败**

  运行：go test ./internal/desktopui -run 'TestWordVersionHistory(UI|List|Ribbon)' -count=1
  预期：version-history 当前只有 ribbon 占位 action，无列表交互。

- [x] **步骤 3：实现稳定的异步历史视图**

  创建独立 Gio 历史列表视图；将 list/clickable 状态保存在 App；版本元数据按路径加载。复用现有 ribbon enable/semantic.DescriptionOp、异步结果和状态提示模式。

- [x] **步骤 4：接入确认恢复并更新支持文档**

  从所选版本发起 Task 4 的恢复操作；确认后关闭列表或显示结果状态。更新 office-support 与中文 TODO 的实现边界、保留策略、错误提示及未支持范围。

- [x] **步骤 5：运行功能定向回归**

  运行：go test ./internal/workspace ./internal/format/docx ./internal/desktopui -run 'Test(WordVersion|RestoreSnapshot|SaveWithSnapshot|SaveAsDocumentWithSnapshot)' -count=1
  预期：持久化、DOCX 整包恢复、controller 生命周期及 UI 入口回归通过。

- [x] **步骤 6：使用 Computer Use 做桌面验收**

  使用 `go run ./cmd/desktop` 验证主启动路径，并用同一工作树构建的临时 macOS app 绑定 CUA（CUA 无法按 `go run` 进程绑定）。临时 DOCX 中确认 File > 版本历史显示手动保存与打开基线的来源、时间、完整包大小；恢复基线后正文从 29 字变为 23 字，`⌘Z` 撤销恢复回到 29 字，`⌘⇧Z` 重做回到 23 字。退出并重启 QA app 后再次打开该文档，历史列表仍有三条快照，证明本地 SQLite 历史跨进程保留。Pencil MCP 截取 `Clpgm`、`kiBim`、`FsQrk`；CUA 确认历史面板覆盖正文区，不改变功能区、标尺、正文视口和底栏的既有几何。完整页面仍未通过 1:1 验收：当前窗口为 1280×820 dp，Pen 画板为 1440×900，等价样稿的正文布局也仍有已记录差异；不将此功能验收误报为整页像素验收通过。

- [x] **步骤 7：补上未提交文字草稿与聚焦编辑器同步回归**

  最终代码审查指出：历史恢复可能在未提交正文文字草稿时捕获保护快照；恢复结果也可能让已聚焦的 Gio 编辑器保留旧文字。新增 `TestWordVersionRestoreFlushesPendingTextDraftBeforeProtectionSnapshot`、`TestWordVersionRestoreStopsWhenPendingTextDraftCannotCommit` 和 `TestWordTextResetForcesFocusedEditorsToSync`。验证成功提交后才恢复、保护快照包含草稿且单步撤销回到草稿、提交失败不改文档也不生成保护版本；恢复与撤销/重做后重建共享文字缓存，并通过 reset epoch 强制同步每窗编辑器。二次审查再发现撤销/重做 revision 与缓存发布时序及切标签后的失败提示问题，现用 `setWordBlocksAndTextState` 在同一同步边界提交投影与 epoch，并为每个显示该文档的窗口派发有锁保护的恢复失败通知；新增 `TestWordHistoryPublishesSharedTextBeforeRevisionToOtherWindows` 和 `TestWordVersionRestoreDraftFailureSurvivesTabSwitch`。最后审查发现两个窗口可能同时完成共享文字刷新，造成重复恢复；刷新标志改用原子布尔值，待恢复版本由共享锁单次领取，worker 完成后释放 claim，发起窗口关闭或不再处理私有结果不会锁住幸存窗口；新增 `TestWordVersionRestoreAfterDraftFlushHasSingleWindowOwner` 和 `TestWordRestoreClaimReleasesWhenOriginWindowStopsBeforeResultHandling`。恢复 worker 错误也会在投递发起窗口的私有结果前持久到共享会话并唤醒当前显示文档的其他窗口；新增 `TestWordRestoreFailureNoticeSurvivesOriginWindowShutdown` 覆盖发起窗口先关闭、幸存窗口仍展示恢复失败。相关定向回归及 `-race` 检查通过。CUA 尝试复验真实编辑器时先看到旧编辑器文字仍显示，因此增加强制焦点同步；最终修复版临时 app 再启动时 CUA 返回 `timeoutReached`，这次修复后的视觉复验待恢复 CUA 连接后补做。

## 规格覆盖自审

- SQLite schema、路径隔离、去重、限频、配额和迁移：Task 1。
- 完整包快照、未知部件、精确保存字节和单步撤销/重做：Task 2。
- 打开基线、手动/自动保存、保存错误差异和 Save As：Task 3。
- 校验、保护版本、恢复原子性、dirty 与 undo/redo：Task 4。
- 文件 ribbon 列表、错误呈现、局部几何和 office-support 文档：Task 5。
- 全局 Review Focus 五种高风险输入均绑定到相关任务的回归步骤；数据 I/O 均被限定在 UI 帧外。
