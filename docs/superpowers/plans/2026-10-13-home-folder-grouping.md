# 首页逻辑文件夹与文档归类 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在首页接通可持久化的层级逻辑文件夹，并允许用户通过拖放把最近文档归类到一个文件夹。

**Architecture:** `workspace` 包提供中立文件夹快照、树操作和 SQLite 事务 API；`desktopRuntime` 共享带修订号的快照，并通过现有写入队列异步持久化。`desktopui` 负责文件夹导航/对话框、文档筛选和 Gio 拖放；拖放只更新文档归属，不触碰本地文件。

**Tech Stack:** Go、SQLite (`modernc.org/sqlite`)、Gio v0.10.3、现有 workspace writer 和 pointer/gesture 输入路由。

**Spec:** `docs/superpowers/specs/2026-10-13-home-folder-grouping-design.md`

## Global Constraints

- 目录树、名称、登记路径和文档归属保存在工作区 SQLite；UI 选择与展开状态仅保存在窗口状态。
- “最近”继续显示所有最近打开文档；选中文件夹后右侧列表只显示该文件夹的直接文档归属。
- 一个文档最多归属一个文件夹；拖放不移动、复制、重命名或删除任何本地路径。
- 已登记的本地目录保存为一级文件夹来源路径；选中后在后台枚举一级支持格式文件并显示在其内容列表中，但不把它们加入“最近”或写成逻辑归属，不递归，也不创建磁盘子目录。
- 文件夹删除会解除其直接文档归属，并把子文件夹提升到其父级；本地文件和目录保持不变。
- 所有 SQLite 写入通过现有工作区队列；布局与事件处理不执行同步磁盘 I/O。
- Gio 控件保持稳定状态；所有 `widget.List` 明确使用纵向轴，文件行编辑/拖动不影响原有点击打开行为。
- 文件夹图标自身切换 `folder` / `folder-open` 并只负责展开/收起；文件夹名称独立负责选择和过滤，行内不再放第二个展开箭头。
- 文件夹内容区使用整页自然高度滚动，滚动条沿内容区右缘显示；按已选文件夹每次展示 30 项，接近当前列表底部时再扩展下一页。
- 文件夹管理菜单与文件夹行等宽，图标从 home SVG 资源绘制；每项保留 10dp 上下内边距、14sp 自然行高，不能把文字挤进固定行高而裁切。
- 文档拖动提供保留抓取偏移的悬浮行预览、阴影和文件夹落点高亮；拖动提交仍只修改逻辑归属。

## Review Focus

- 大小写不同的同级重名和不同父级同名：分别拒绝与允许，固定在 workspace 模型测试。
- 损坏的持久树（缺失父 ID、循环或无效归属）：加载失败并保留可读错误，固定在 workspace 加载测试。
- 连续跨窗口写入时较早的失败结果：不得回滚覆盖较新的修订，固定在 runtime 修订测试。
- 多窗口并发创建/改名/移除/归类：操作必须基于 writer 队列中的最新共享快照，固定在并发 mutation 测试。
- 异步写入回滚或其他窗口移除当前文件夹：选中页必须在下一帧恢复到有效导航状态，固定在窗口状态回归。
- 菜单锚点靠近短滚动视口底部：浮层不得受列表裁切，固定在短视口截图和位置回归。
- 目录选择取消、重复路径或不可访问路径：取消无变化，重复不新增，不可访问时给出错误，固定在目录登记测试。
- 拖动释放到无效区域、有效文件夹或原文件夹：分别不变、归类、无操作；拖动不应触发行点击打开，固定在 Gio 路由测试。

---

## 文件结构

- `internal/workspace/home_folders.go`：文件夹/归属中立类型、校验和纯快照变换。
- `internal/workspace/store.go`：SQLite schema v7 迁移。
- `internal/workspace/home_folders_test.go`：模型、迁移、事务、重载测试。
- `internal/desktopui/runtime.go`：运行时共享文件夹快照、修订和已提交快照。
- `internal/desktopui/home_folder_runtime.go`：快照复制、加载、修订更新和写入完成处理。
- `internal/desktopui/runtime_workspace_persistence.go`、`workspace_file_commands.go`、`workspace_async_results.go`、`workspace_sessions.go`：写队列、目录选择结果与启动加载。
- `internal/desktopui/home_folders.go`：窗口选择/展开状态、CRUD 意图、名称验证调用和文件夹视图筛选。
- `internal/desktopui/home_folder_layout.go`：层级树行、创建/改名/确认移除菜单布局。
- `internal/desktopui/home_folder_drag.go`：文档拖动、目标几何命中与归属提交。
- `internal/desktopui/home_layout.go`、`home_table.go`、`home_filters.go`、`app.go`：连接导航、稳定 Gio 控件、过滤后的表格和计数显示。
- `internal/desktopui/home_folder_interaction_test.go`、`home_folder_drag_test.go`、`home_folder_visual_test.go`：界面行为、拖放路由与截图验证。

## Task 1: Workspace 文件夹模型与 SQLite 持久化

**Files:**
- Create: `internal/workspace/home_folders.go`
- Create: `internal/workspace/home_folders_test.go`
- Modify: `internal/workspace/store.go`
- Modify: `internal/workspace/word_versions_test.go` (旧版本迁移期望当前版本由 6 更新为 7)
- Modify: `internal/workspace/store_test.go`, `internal/workspace/preferences_test.go` (旧降级夹具要移除 v7 文件夹表)

**Interfaces:**
- Produces:
  - `type HomeFolder struct { ID, ParentID, Name string; SortOrder int; SourcePath string }`
  - `type HomeFolderAssignment struct { Path, FolderID string }`
  - `type HomeFolderSnapshot struct { Folders []HomeFolder; Assignments []HomeFolderAssignment }`
  - `func ValidateHomeFolderSnapshot(HomeFolderSnapshot) error`
  - `func (HomeFolderSnapshot) AddFolder(HomeFolder) (HomeFolderSnapshot, error)`
  - `func (HomeFolderSnapshot) RenameFolder(id, name string) (HomeFolderSnapshot, error)`
  - `func (HomeFolderSnapshot) RemoveFolder(id string) (HomeFolderSnapshot, error)`
  - `func (HomeFolderSnapshot) AssignDocument(path, folderID string) (HomeFolderSnapshot, error)`
  - `func (*Store) LoadHomeFolderSnapshot() (HomeFolderSnapshot, error)`
  - `func (*Store) SaveHomeFolderSnapshot(HomeFolderSnapshot) error`
- `AddFolder` 将新文件夹追加到同级最大 `SortOrder` 之后；`RemoveFolder` 清除该文件夹的直接归属并把直接子文件夹提升一层；`AssignDocument` 对已有 path 归属执行重分组。
- 名称校验去首尾空白、拒绝空值/路径分隔符/控制字符、按大小写不敏感拒绝同级重名；允许不同父级重名。
- 快照校验拒绝重复 ID、缺失父项、父子循环、重复 source path、非法父级 source path、重复文档 path 和不存在的目标文件夹。

- [x] **Step 1: 编写迁移、模型与事务失败测试**

在 `home_folders_test.go` 新增：

```go
func TestHomeFoldersMigrateFromV6AndPreserveExistingState(t *testing.T)
func TestHomeFolderSnapshotRoundTripsAndReplaces(t *testing.T)
func TestAssignHomeDocumentReplacesExistingFolder(t *testing.T)
func TestHomeFolderSnapshotRejectsInvalidTreesAndNames(t *testing.T)
func TestLoadHomeFolderSnapshotRejectsCorruptRows(t *testing.T)
func TestRemoveHomeFolderPromotesChildrenAndUngroupsDirectDocuments(t *testing.T)
func TestHomeFolderSnapshotSaveIsTransactional(t *testing.T)
```

迁移测试从版本 7 测试库移除 v7 文件夹表并将 `user_version` 设回 6，重开后断言版本为 7、已有 recent/favorite/document 内容保留、文件夹快照为空。校验测试覆盖空名称、路径分隔符、控制字符、同级大小写重名、不同父级同名、缺失父项、循环、重复来源路径、子级登记路径和未知归属。保存失败测试用 SQLite abort trigger 拒绝插入，断言旧快照不变。

- [x] **Step 2: 运行 workspace 定向测试确认失败**

运行：`go test ./internal/workspace -run '^(TestHomeFoldersMigrateFromV6AndPreserveExistingState|TestHomeFolderSnapshotRoundTripsAndReplaces|TestAssignHomeDocumentReplacesExistingFolder|TestHomeFolderSnapshotRejectsInvalidTreesAndNames|TestLoadHomeFolderSnapshotRejectsCorruptRows|TestRemoveHomeFolderPromotesChildrenAndUngroupsDirectDocuments|TestHomeFolderSnapshotSaveIsTransactional)$' -count=1`

预期：FAIL，原因是文件夹快照 API/schema 尚不存在，且旧迁移仍停留在版本 6。

- [x] **Step 3: 实现中立快照操作和 schema v7**

在 `home_folders.go` 实现上述类型、函数和纯快照变换。`store.go` 将最高 schema 版本提升为 7，新增 `home_folders`、`home_document_folders` 表和合适索引；Store 加载/保存 API 在事务中校验快照、按父级顺序插入并整体替换。更新版本迁移测试期望为 7。

- [x] **Step 4: 运行 workspace 定向测试**

运行 Step 2 的同一条 `go test` 命令。

预期：PASS；失败插入后的读取仍与初始快照完全一致。

- [x] **Step 5: 完成 workspace 存储任务并保留待提交改动**

确认 Step 4 定向回归通过后继续 Task 2；按用户要求，所有文件夹功能与计划文件在 Task 5 一次提交。

## Task 2: Runtime 共享状态与异步写入

**Files:**
- Modify: `internal/desktopui/runtime.go`
- Create: `internal/desktopui/home_folder_runtime.go`
- Modify: `internal/desktopui/runtime_workspace_persistence.go`
- Modify: `internal/desktopui/workspace_file_commands.go`
- Modify: `internal/desktopui/workspace_sessions.go`
- Modify: `internal/desktopui/workspace_async_results.go`
- Create: `internal/desktopui/home_folder_runtime_test.go`

**Interfaces:**
- Consumes Task 1 的 `workspace.HomeFolderSnapshot` 和 `Store.LoadHomeFolderSnapshot` / `SaveHomeFolderSnapshot`。
- Produces:
  - `func (*desktopRuntime) loadHomeFolderState() error`：从 Store 加载并校验一次；保存加载错误以阻止后续覆写损坏状态。
  - `func (*desktopRuntime) homeFolderSnapshot() (workspace.HomeFolderSnapshot, uint64)`：返回深拷贝快照和当前修订。
  - `func (*desktopRuntime) updateHomeFolderSnapshot(next workspace.HomeFolderSnapshot, result chan homeFolderWriteResult) (uint64, error)`：校验并发布新修订，将快照和修订排入异步写队列；队列不可用时返回错误并不发布。
  - `type homeFolderWriteResult struct { revision uint64; snapshot workspace.HomeFolderSnapshot; err error }`
  - `func (*desktopRuntime) homeFolderStateError() error`：返回已记录的加载错误。
  - `func (*desktopRuntime) finishHomeFolderWrite(result homeFolderWriteResult) bool`：成功时推进已提交快照；失败且仍是最新修订时回滚；返回是否需重绘。
- `App.initializeWorkspace` 只在 runtime 尚未加载文件夹状态时从 Store 加载一次；多个窗口读取 runtime 的同一快照。
- `App` 持有容量为 1 的 `homeFolderWriteResults chan homeFolderWriteResult`；`workspaceWriteJob` 携带完整快照、修订号和该结果通道。
- 唯一 writer 按队列顺序执行 `SaveHomeFolderSnapshot`，紧接着调用 `finishHomeFolderWrite` 再向发起窗口送结果；这样最新写入失败时，runtime 已拥有真实的最后一次成功快照，不依赖窗口事件循环消费较早成功结果的时序。writer 调用 `invalidateWindows()` 唤醒全部窗口。
- 加载校验失败时保留错误并显示工作区文件夹不可用；不得静默将损坏快照当作空树后覆盖数据库。

- [x] **Step 1: 编写共享、成功提交与新旧修订失败测试**

新增：

```go
func TestHomeFolderRuntimeSharesSnapshotAcrossWindows(t *testing.T)
func TestHomeFolderWriterCommitsSnapshotAsynchronously(t *testing.T)
func TestHomeFolderWriteFailureRollsBackOnlyLatestRevision(t *testing.T)
func TestHomeFolderLoadFailureBlocksOverwrite(t *testing.T)
```

断言两个窗口读到同一份快照；成功写入后 Store 重载一致；即使发起窗口延迟消费较早成功结果，旧修订失败也不能覆盖更晚的工作快照，最新修订失败回到最后一次已提交快照。

- [x] **Step 2: 运行 runtime 定向测试确认失败**

运行：`GLANCE_MCP_PORT=0 go test ./internal/desktopui -run '^TestHomeFolder(RuntimeSharesSnapshotAcrossWindows|WriterCommitsSnapshotAsynchronously|WriteFailureRollsBackOnlyLatestRevision|LoadFailureBlocksOverwrite)$' -count=1`

预期：FAIL，原因是 runtime 尚无共享快照和修订处理。

- [x] **Step 3: 实现共享快照和 writer 消息**

在 `desktopRuntime` 增加受互斥保护的工作快照、已提交快照、修订号和加载状态。扩展 `workspaceWriteJob` 与 writer，在 `workspaceDBMu` 保护下调用 Store 事务 API；writer 串行完成持久化和 `finishHomeFolderWrite` 后，再向发起窗口结果通道发送结果。

- [x] **Step 4: 接入工作区初始化和异步结果**

在 `initializeWorkspace` 首次加载并保留 Store 快照错误；writer 在发结果前已完成 runtime 提交/回滚，`processAsyncResults` 消费发起窗口的结果并更新保存错误状态；runtime 发生回滚时唤醒所有窗口。

- [x] **Step 5: 运行 runtime 定向测试**

运行 Step 2 的同一条 `go test` 命令。

预期：PASS；测试使用临时 SQLite 文件，不依赖桌面窗口。

- [x] **Step 6: 完成 runtime 任务并保留待提交改动**

确认 Step 5 定向回归通过后继续 Task 3；不单独提交。

## Task 3: 首页文件夹树、管理对话框与目录登记

**Files:**
- Create: `internal/desktopui/home_folders.go`
- Create: `internal/desktopui/home_folder_layout.go`
- Create: `internal/desktopui/home_folder_interaction_test.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/home_layout.go`
- Modify: `internal/desktopui/home_table.go`
- Modify: `internal/desktopui/home_filters.go`
- Modify: `internal/desktopui/workspace_file_commands.go`
- Modify: `internal/desktopui/workspace_async_results.go`

**Interfaces:**
- Consumes Task 1 的快照变换和 Task 2 的 runtime 快照/更新接口。
- Produces:
  - `func (a *App) homeFolderVisibleRecents(folderID string) []workspace.Recent`：合并目标文件夹直接归属的全部工作区最近文档和已登记目录的一级支持文件，按规范路径去重，应用类型、搜索、排序，不应用最近页五行上限。
  - `func (a *App) selectHomeFolder(folderID string)`：清除收藏视图、选择文件夹并重置内容滚动。
  - `func (a *App) createHomeFolder(parentID, name string) error`
  - `func (a *App) renameHomeFolder(folderID, name string) error`
  - `func (a *App) removeHomeFolder(folderID string) error`
  - `func (a *App) registerHomeFolderPath(path string) error`
- 定义 `type homeFolderRowWidgets struct { selectButton, expandButton, menuButton widget.Clickable; menuItems map[string]*widget.Clickable }`。`homeFolderRows map[string]*homeFolderRowWidgets`、`homeSelectedFolderID string`、展开 ID 集合、双击时间戳和弹窗/菜单控件保持稳定状态。
- 使用已有 `modalDialog.ShowInput` / 确认弹窗；目录登记使用新的异步 `commandRegisterHomeFolder`。选中已登记目录后在后台复用 `scanOfficeFolder` 枚举一级支持文件，只在该文件夹视图显示，不写工作区 Recents。

- [x] **Step 1: 编写文件夹过滤、创建/改名/移除和路径登记测试**

新增：

```go
func TestHomeFolderVisibleRecentsFiltersDirectMembershipAndExistingFilters(t *testing.T)
func TestHomeFolderTreeExpandDoesNotSelectAndDoubleClickToggles(t *testing.T)
func TestHomeFolderCreateUsesSelectedParentAndSelectsResult(t *testing.T)
func TestHomeFolderRenameAndRemoveKeepLogicalSemantics(t *testing.T)
func TestHomeFolderMenuDismissesOnBlankClickAndEscape(t *testing.T)
func TestHomeFolderEmptyStateAndRemoveCancel(t *testing.T)
func TestHomeFolderDirectoryRegistrationDeduplicatesAndShowsContentsWithoutAddingRecents(t *testing.T)
func TestHomeFolderDirectoryPickerCancelLeavesSnapshotUnchanged(t *testing.T)
```

验证最近页仍包含已分组文件；点击文件夹名称按已选文件夹过滤，逻辑文件夹仅显示直接成员，已登记目录显示一级支持文件；目录扫描不改变 Recents；筛选后保留全部匹配文档；箭头只展开、双击名称切换展开；创建弹窗展示完整父级位置且创建后选中；改名弹窗初值、无改动不提交；空白点击/Esc 关闭菜单；移除清除直接归属、提升子文件夹，正在查看已移除项时回到“最近”；目录登记取消/重名不写入。

- [x] **Step 2: 运行首页文件夹定向测试确认失败**

运行：`GLANCE_MCP_PORT=0 go test ./internal/desktopui -run '^TestHomeFolder(VisibleRecents|Tree|Create|Rename|Menu|EmptyState|DirectoryRegistration|DirectoryPicker)' -count=1`

预期：FAIL，原因是首页文件夹模型和视图尚不存在。

- [x] **Step 3: 实现首页操作与筛选 helper**

在 `home_folders.go` 实现 Interfaces 中的操作；新 ID 使用 `newDraftSessionID("home-folder")` 的加密随机 ID 生成方式。筛选以现有 `canonicalDocumentPath` 规范 path 匹配归属，并保留 `homeTypeFilter`、`homeSearch` 与打开时间排序；将 `homeVisibleRecents` 拆成共享过滤/排序 helper，让“最近”保留所有文档及其现有五行上限、“文件夹”视图不设五行上限；切换“最近”或“收藏”时清除文件夹选择。已登记目录只在后台读取一级支持文件，缓存于窗口视图状态，不更新 Recents 或逻辑归属。

- [x] **Step 4: 实现树布局与稳定交互控件**

在 `home_folder_layout.go` 用显式纵向轴的持久 `widget.List` 布局文件夹树、箭头、选中行和子级缩进，记录当前 frame 中可见行的几何；接入右键/悬停更多菜单、新建/改名输入和移除确认。`home_layout.go` 用树布局替换固定 Glance 占位项。箭头展开不改变选中项；名称选择文件夹，连续两次名称点击切换展开状态；移除当前选择后返回“最近”。

- [x] **Step 5: 接入右侧内容过滤和目录选择器**

在 `home_table.go` 根据 `homeSelectedFolderID` 取过滤数据并显示“暂无文件”；`home_filters.go` 按当前视图显示对应计数。给文件命令增加 `commandRegisterHomeFolder` 分支，在后台选择目录，用现有 `canonicalDocumentPath` 规范路径并通过 `os.Stat` 确认路径仍可访问且为目录，再由 UI 注册逻辑根目录。重复路径给出状态提示，取消不改变状态；选中来源目录时在后台扫描其一级支持文件，仅展示在文件夹视图，不递归、不写入“最近”或逻辑归属。

- [x] **Step 6: 运行首页文件夹定向测试**

运行 Step 2 的同一条 `go test` 命令。

预期：PASS；目录 picker 使用注入的 runner，不打开真实系统对话框。

- [x] **Step 7: 完成首页树任务并保留待提交改动**

确认 Step 6 定向回归通过后继续 Task 4；不单独提交。

## Task 4: 文档行拖放与视觉回归

**Files:**
- Create: `internal/desktopui/home_folder_drag.go`
- Create: `internal/desktopui/home_folder_drag_test.go`
- Modify: `internal/desktopui/home_folder_drag_test.go` (includes 1x/2x visual state checks)
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/home_table.go`
- Modify: `internal/desktopui/home_folder_layout.go`

**Interfaces:**
- Consumes Task 2 的异步 runtime 更新接口和 Task 3 的可见文件夹树/文档过滤。
- Produces:
  - `type homeFolderDropTarget struct { FolderID string; Bounds image.Rectangle }`
  - `func homeFolderDropTargetAt(point image.Point, targets []homeFolderDropTarget) string`
  - `type homeDocumentDragState struct { drag gesture.Drag; origin image.Point }` 和 `App.homeDocumentDrags map[string]*homeDocumentDragState`；仅在 release 落到可见有效文件夹目标时生成 `HomeFolderSnapshot.AssignDocument` 更新。
- 使用 Gio `gesture.Drag` 的 3dp 手势阈值；拖动超过阈值时其 Grab 取消行点击，普通点击仍由原 `widget.Clickable` 打开文档。
- 目标边界和源行 origin 统一使用首页根坐标；列表滚动偏移计入几何。只有当前展开且可见的文件夹行是 drop target。

- [x] **Step 1: 编写纯目标命中与真实路由测试**

新增：

```go
func TestHomeFolderDropTargetAtUsesRootCoordinates(t *testing.T)
func TestHomeFolderDropTargetsExcludeCollapsedAndOffscreenRows(t *testing.T)
func TestHomeRecentRowDragDropAssignsWithoutOpening(t *testing.T)
func TestHomeRecentRowDragCancelPreservesAssignment(t *testing.T)
func TestHomeRecentRowDropOnSameFolderIsNoop(t *testing.T)
func TestHomeRecentRowClickStillOpensAfterDragSupport(t *testing.T)
func TestHomeFolderDragVisualStatesAtOneAndTwoScale(t *testing.T)
```

用 `input.Router` 排队 Press/Drag/Release，断言目标文件夹命中、折叠/屏幕外行不能命中、归属变化、路径原样保留、拖动不调用 `startOpen`、取消/同目标/有效区域外投放无变化、普通点击仍打开。视觉测试使用合成路径和名称。

- [x] **Step 2: 运行拖放定向测试确认失败**

运行：`GLANCE_MCP_PORT=0 go test ./internal/desktopui -run '^(TestHomeFolderDropTargetAtUsesRootCoordinates|TestHomeFolderDropTargetsExcludeCollapsedAndOffscreenRows|TestHomeRecentRowDragDropAssignsWithoutOpening|TestHomeRecentRowDragCancelPreservesAssignment|TestHomeRecentRowDropOnSameFolderIsNoop|TestHomeRecentRowClickStillOpensAfterDragSupport|TestHomeFolderDragVisualStatesAtOneAndTwoScale)$' -count=1 -v`

预期：FAIL，原因是行没有拖动手势或目标命中实现。

- [x] **Step 3: 实现根坐标目标记录和命中 helper**

在 `home_folder_drag.go` 定义拖放状态和目标类型；`homeFolderDropTargetAt` 对当前 frame 中已展开且可见目标 bounds 做半开矩形命中，无效点返回空 ID。文件夹布局按树行高度、`widget.List.Position.First/Offset` 和父级布局 origin 记录根坐标边界；文件行记录相同坐标系下的 origin。

- [x] **Step 4: 实现源行 gesture 与目标反馈**

在 `home_table.go` 为每条稳定文件行绑定 `gesture.Drag`，把事件位置转换到首页根坐标并命中当前 drop target；目标高亮、释放提交、取消清理都走独立 helper。普通 click 仍走 `homeOpenButtons[path]`；拖动超过阈值由 Gio Grab 取消 Clickable。

- [x] **Step 5: 运行拖放定向与视觉测试**

运行 Step 2 的同一条 `go test` 命令。

预期：交互测试 PASS；视觉测试按 Pen `GoW5E` 检查主/子级、选中/展开、空状态、拖动目标反馈和图文对齐，均无布局裁剪且 1x/2x 下正确。若环境报告 `SKIP`，记录为未完成视觉验证，不计作通过。

- [x] **Step 6: 完成拖放任务并保留待提交改动**

确认 Step 5 定向与视觉回归通过后继续 Task 5；不单独提交。

## Task 5: 集成回归与兼容检查

**Files:**
- Modify only if integration fixes are needed: `internal/workspace/*` and `internal/desktopui/*` files above.

**Interfaces:**
- Consumes Tasks 1–4 的全部模型、runtime 与 UI 行为；不新增对外接口。

- [x] **Step 1: 运行 workspace 与 desktopui 全包回归**

运行：`GLANCE_MCP_PORT=0 go test ./internal/workspace ./internal/desktopui`

预期：两个包均 PASS；设置 `GLANCE_MCP_PORT=0` 避免当前主机被占用的固定 MCP 端口；macOS 视觉测试必须查看详细输出，`SKIP` 不算通过。

- [x] **Step 2: 检查 patch 和文件状态**

运行：`git diff --check`

预期：无空白错误；确认 diff 只含文件夹分组实现及其测试。

- [ ] **Step 3: 一次提交全部文件夹功能**

```bash
git add <文件夹功能实现、测试与计划文件；对包含其他未提交工作的共享文件分块暂存>
git commit -m "feat: add logical home folders"
```

### 后续设计反馈（2026-10-13）

- `IQgJN`：文件夹图标显示展开/收起状态并独立切换；名称按钮负责选择。
- `W2kEhH`：右键菜单改为 SVG 图标、整行等宽，并补齐子项 hover 反馈；菜单项增至 42dp 以容纳实测 22dp 的 14sp 自然行高及 10dp 上下内边距。
- 文件夹文档列表不设固定高度，接入整页自然滚动和右缘滚动条；靠近底部时按 30 项加载。
- 拖动时显示 DOM 风格悬浮行，位置保留按下点偏移。
- 回归覆盖菜单文字行高、短视口菜单、整页滚动加载、图标展开交互、拖动预览与文件夹扫描失败清缓存。相关改动完成后作为后续单独提交，不重写已提交的基础实现。
