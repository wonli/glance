# Proofreading Workbench and History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现可恢复的校对工作台、清晰页状态、逐页 Git 历史及手动远程推送。

**Architecture:** workspace SQLite 保存任务与当前恢复状态，独立任务目录保存原稿和 Git 成果。中立模型提供稳定页面与字符身份，application 协调后台文件、数据库和历史服务；Gio 只处理意图、稳定控件和结果展示。

**Tech Stack:** Go、Gio、现有 modernc SQLite、系统 Git/SSH、现有 CredentialStore。

**Spec:** `docs/superpowers/specs/2026-10-24-proofreading-workbench-history-design.md`

## Global Constraints

- 所有实现及验证在 `/Users/mima1234/project/glance-go`，不创建 worktree，不建立独立缓存或重复构建产物。
- 未安装 Git 时仍可确认已校对与自动保存，但无快照、不补造历史。
- Git 仅显式提交目标页文件；不修改用户全局配置，不自动推送，不强推。
- 原稿复制到任务目录；SQLite 恢复不依赖 Git HEAD；版本恢复不得破坏其他页。
- IO 在受管理后台任务中执行，保留完整 IME 合成语义；中英文文案齐全。
- 临时测试目录在授权仓库内创建并自动清理；GPU SKIP 不算视觉通过。

## Review Focus

- Git 未安装、身份未配置与提交失败均不能丢失已校对成果（任务 3、5）。
- 撤销后新增字符以及恢复旧页不能发生字符身份重用（任务 1、6）。
- 复制中断、源文件删除、旧草稿损坏不能形成虚假可恢复任务（任务 2、4）。
- 快速确认后继续编辑、切任务与退出不能由迟到结果覆盖新状态（任务 3、5）。
- 凭据含特殊字符、远程分歧与 SSH 未信任主机不能泄露秘密或挂起 UI（任务 7）。

---

### Task 1: 跨历史稳定字符身份

**Files:** Modify `internal/proofreading/model.go`, `internal/proofreading/operations.go`; Create `internal/proofreading/history_identity_test.go`.

**Interfaces:** 中立 Snapshot 新增可兼容旧版本的分配水位；`Model.EnsureAllocationWatermark(next CharacterID)` 只提升不回退。Capture/Snapshot 携带水位供持久化使用。

- [ ] 写测试：插入、撤销、再插入的新增 ID 不重用；恢复旧 Snapshot 后提升水位再插入不与历史冲突。
- [ ] 运行 `go test ./internal/proofreading -run 'TestHistoryIdentity' -count=1`，确认行为测试先失败。
- [ ] 实现单调分配与旧快照兼容，撤销内容但不回退 ID 水位。
- [ ] 重跑目标测试及 `go test ./internal/proofreading -count=1`。

### Task 2: SQLite 任务与文件生命周期

**Files:** Modify `internal/workspace/store.go`; Create `internal/workspace/proofreading.go`, `internal/workspace/proofreading_test.go`, `internal/application/proofreading_projects.go`, `internal/application/proofreading_projects_test.go`.

**Interfaces:** workspace `ProofreadingProject` 包含 ID、Title、Directory、SourceA/SourceB、源哈希、PageCount、UpdatedAt、ViewState、Payload、AllocationWatermark、RemoteURL、CredentialRef。Store 提供 `SaveProofreadingProject(p ProofreadingProject) error`, `LoadProofreadingProject(id string) (ProofreadingProject, bool, error)`, `ListProofreadingProjects() ([]ProofreadingProject, error)`。application `CreateProofreadingProject(ctx context.Context, root, sourceA, sourceB string) (workspace.ProofreadingProject, error)` 执行原稿复制并返回可持久化记录。

- [ ] 写迁移、二进制 Payload 往返与列表排序测试；写复制失败及成功后删除外部原稿仍可读测试。
- [ ] 运行 `go test ./internal/workspace ./internal/application -run 'TestProofreadingProject' -count=1` 确认失败。
- [ ] 在现有 schema 版本之后加事务迁移；任务源文件以临时文件写入、哈希核对后原子替换，成功后登记。失败只清理本次新建产物。
- [ ] 重跑目标测试，覆盖中文路径、同名稿件和取消复制。

### Task 3: 受限逐页 Git 服务

**Files:** Create `internal/proofhistory/{repository.go,page.go,history.go,repository_test.go}`.

**Interfaces:** `Identity{Name, Email string}`；`PageResult` 包含 schema、ProjectID、SourceHash、Page proofreading.Page、Runs []proofreading.Run、ConfirmedAt、Identity。`Repository{Directory string}` 提供 `Available(ctx context.Context) error`, `Confirm(ctx context.Context, page PageResult) (CommitResult,error)`, `List(ctx context.Context, pageID proofreading.PageID) ([]Entry,error)`, `ReadPage(ctx context.Context, commit string, pageID proofreading.PageID) (PageResult,error)`；pageID 为零表示全历史。`CommitResult` 返回 Hash、ContentHash、Created。

- [ ] 写真实临时仓库测试：6→500→7 各提交仅含目标文件；同页修订两版本；无变化确认不新增提交；缺 Git、缺身份和 Git 失败返回可辨识错误。
- [ ] 运行 `go test ./internal/proofhistory -count=1` 确认失败。
- [ ] 使用 exec.CommandContext 参数数组，无 shell；固定分支 main，显式相对页路径，参数化 author/committer，禁止外部 hooks/signing 干扰，禁用终端提示。仅初始化本次独立任务仓库。
- [ ] 增加待提交日志与按页面内容哈希协调中断恢复，确保提交成功而 SQLite 未更新时不重复提交；测试中断重试与非目标页未跟踪文件不被提交。
- [ ] 重跑目标测试；测试目录自动清理。

### Task 4: 工作台与当前进度恢复

**Files:** Create `internal/desktopui/proofread_workbench_{state,layout,events}.go`, `internal/desktopui/proofread_project_persistence.go`, `internal/desktopui/proofread_workbench_test.go`; Modify `internal/desktopui/proofread_window.go`, `proofread_edit.go`, `proofread_scroll.go`, `proofread_chrome.go` 以及现有窗口请求分发位置。

**Interfaces:** UI `openProofreadingProject(id string)` 与 `queueProofreadingProjectPersistence()` 通过 managed background 调用任务 2，结果携带任务 ID 与 generation。持久化 DTO 扩充当前页、A/X 滚动、字号及模型；Git 页成果不进入自动保存路径。

- [ ] 写新建/关闭/重开、删除外部源文件、恢复当前页和滚动、旧草稿迁移失败不覆盖测试。
- [ ] 运行 `go test ./internal/desktopui -run 'TestProofreading(Project|Workbench)' -count=1` 确认失败。
- [ ] 按 kms7p 实现真实列表及新建/继续，采用既有文件选择流程；只打开选中的任务；数据库列表查询、复制和恢复在后台完成。
- [ ] 自动保存使用捕获模型并合并频繁写入，视图移动也能保存；关闭时排空必要写入。保留可验证旧草稿迁移路径。
- [ ] 重跑目标测试，增加快速切换任务丢弃迟到恢复结果的断言。

### Task 5: 身份设置、确认流程和页状态

**Files:** Create `internal/desktopui/proofread_identity_{state,layout,events}.go`, `proofread_confirmation.go`, `proofread_page_status.go`, `proofread_confirmation_test.go`, `proofread_page_status_test.go`; Modify `proofread_page_ownership.go`, `proofread_editor_events.go`, `proofread_page_menu.go`, `proofread_layout.go`, `proofread_chrome.go`, `internal/i18n/locales/{zh-CN,en-US}.json`.

**Interfaces:** `proofreadingPageStatus` 独立表达 Reviewed、ModifiedSinceConfirmation、SnapshotState、PushedCommit。身份以 preferences 中 `proofreading.identity` JSON 原子保存。确认请求捕获任务、目标页、身份、内容哈希与 generation。

- [ ] 写状态测试：无 Git 仍可已校对但无快照；无身份引导设置；提交失败仍保留成果；已确认页编辑显示待确认，撤销匹配后恢复；迟到提交不确认新内容。
- [ ] 运行 `go test ./internal/desktopui -run 'TestProofreading(Identity|Confirmation|PageStatus)' -count=1` 确认失败。
- [ ] 按 dKZjY 实现身份输入、保存、返回与持久化；使用稳定 Editor，整个输入框可聚焦，校验姓名邮箱及控制字符。
- [ ] 同步完成的编辑草稿后捕获目标页；IME 合成时阻止确认；任务串行提交并更新独立状态。已修改页继续保留跨页保护，不因旧 Reviewed 布尔值阻止确认修订。
- [ ] 每页页眉增加文字+图标徽标，已校对页轻量边框，当前页高亮独立；顶部显示已校对数量，状态不得只用颜色表示。
- [ ] 重跑状态测试，补足普通/Retina 像素及上中下点击输入检查；明确记录 GPU 跳过。

### Task 6: 单页和全任务历史及安全恢复

**Files:** Create `internal/application/proofreading_history.go`, `internal/application/proofreading_history_test.go`, `internal/desktopui/proofread_history_{state,layout,events}.go`, `internal/desktopui/proofread_history_test.go`.

**Interfaces:** application `ValidateProofreadingPageRestore(current proofreading.Snapshot, result proofhistory.PageResult) error` 检查任务/源身份及字符归属；恢复只替换目标页并标为待确认。UI 按页或全任务调用任务 3 的 List/ReadPage。

- [ ] 写 6→500→7 顺序、同页修订差异、恢复冲突拒绝且当前文档不变、历史已删除字符恢复及旧 ID 水位测试。
- [ ] 运行 `go test ./internal/application ./internal/desktopui -run 'TestProofreadingHistory' -count=1` 确认失败。
- [ ] 实现页面/全任务历史列表、作者时间与文本差异；后台读取历史；安全恢复为待确认修改，不执行仓库 reset/checkout。
- [ ] 重跑目标测试，确认其他页内容与顺序完全不变。

### Task 7: 认证配置与项目手动推送

**Files:** Create `internal/proofhistory/{remote.go,remote_test.go}`, `internal/application/proofreading_credentials.go`, `internal/application/proofreading_credentials_test.go`, `internal/desktopui/proofread_remote_{state,layout,events}.go`; Extend dKZjY 设置及任务入口文案。

**Interfaces:** `RemoteConfig{URL, Branch, CredentialRef string}`；`Repository.Push(ctx context.Context, config RemoteConfig, auth TransportAuth) (string,error)` 返回成功推送的提交 ID。application 凭据服务复用 agentsettings.CredentialStore 接口，SQLite 只存引用及认证用户名；SSH 私钥受限存本机。

- [ ] 写合成凭据存取/删除/替换失败清理与错误脱敏测试；测试远程 URL 拒绝内嵌密码和不允许协议。
- [ ] 运行 `go test ./internal/proofhistory ./internal/application -run 'TestProofreading(Remote|Credentials)' -count=1` 确认失败。
- [ ] 实现 SSH 已有密钥选择及 ssh-keygen 生成专用密钥，显示公钥；实现 HTTPS 用户名+令牌写入系统凭据库。传输不把令牌放命令行，使用受限进程间凭据传递，不继承交互提示。
- [ ] 添加项目远程地址与认证选择、手动推送；SSH 新主机显示指纹确认并固定信任，不禁用校验；仅普通推送 main，无强推、自动拉取或合并。
- [ ] 用仓库内临时 bare remote 测首次/重复推送、分歧拒绝、取消/超时、推送期间新提交仍待推送；不连接真实用户远程。
- [ ] 重跑目标测试，检查日志/数据库/URL/提交中均无合成令牌。

### Task 8: 完整交互验证与文档

**Files:** Modify `docs/architecture.md`, `docs/office-support.md`; Create `internal/desktopui/proofread_workbench_visual_test.go`.

- [ ] 增加 1x/2x 工作台、身份、每页三种主要校对状态的真实渲染断言；输入合成中文并检查确认前草稿同步。
- [ ] 运行上述新增目标回归、`go test ./internal/proofreading ./internal/workspace ./internal/proofhistory -count=1` 和 `git diff --check`；共享 UI 改动再运行相关现有校对回归。
- [ ] 退出旧 Glance，当前目录执行 `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance`，验证新建→跳页确认→关闭重开→同页修订→历史查看，并检查没有 Git 的降级状态。不得把编译当实际界面通过。
- [ ] 更新架构和已实现边界，明确凭据、Git 可用性、恢复冲突和远程分歧行为；清理本次验证产物，报告未验证部分。

## Execution and integration

建议当前会话直接执行，避免跨任务传递恢复与提交协议时产生偏差。代码提交只包含本任务明确文件；不操作已有其他 worktree。不在实施完成前推送或更改真实用户远程。完成后进行一次独立代码审查，再按发现补充必要验证。

## Execution record

- Implemented inline in the authorized directory; user explicitly requested one final commit, so no per-task commits were made.
- Task creation retains the existing source-hash-based project identity and copies the adapter's immutable PDF snapshot in the persistence worker. The additional unused random-ID creation scaffold was removed; `TestProofreadingProjectSQLiteAndSourceRecovery` proves source retention through the actual production persistence path.
- Navigation was corrected per user feedback: primary permanent first tab is 文档工作台, with no new sidebar/toolbar entry; the proofreading first tab and existing history entry are wired directly. Default list/settings geometry follows kms7p/dKZjY.
- Targeted proofreading, SQLite, Git, localization and application regressions passed; Metal 1x/2x tests passed without SKIP. Added regression evidence for single writable task window, atomic remote-field update, stale restore rejection, undoable restore, and preserving local scroll when widget count is unchanged.
- Independent review findings were fixed: duplicate task windows, remote/autosave lost updates, author identity capture, and page-version comparison.
- Expanded application suite had four non-proofreading PDF/export/print failures, recorded in office-support.md. Real external credentials/push were not exercised; transport tests use local disposable bare repositories. go-run startup was exercised, but desktop automation could not bind its unpackaged window, so full native-window interaction remains unverified.

## 校对模块多标签修正

恢复应用原主页和 Glance 首标签。工作台是独立校对模块窗口中的固定首标签，稿件在该模块内以同级标签打开；复用主页标签栏的布局与稳定控件，不将校对列表替换为应用主入口。移除任务各开一窗的路由，改为已有模块窗口内打开/定位稿件标签。覆盖独立页码/滚动、同任务去重、关闭隔离、切页前中文草稿，以及主页与模块相互隔离的实际 GPU 渲染。1x/2x Metal 回归与现有加号菜单、文档标签、Ribbon 接缝检查通过；未将离屏渲染表述为完整原生窗口交互验证。
