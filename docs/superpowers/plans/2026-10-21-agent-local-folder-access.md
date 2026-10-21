# AI 按需读取本地目录 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为全局 AI 助手增加按需、只读、仅当前用户请求有效的本地目录授权，并支持受限文本和 Git 提交查询。

**Architecture:** `internal/localaccess` 持有短生命周期 scope、范围校验和只读文件/Git 操作；应用能力目录通过注入的 broker 暴露窄工具。AgentHost 通过 MCP 私有元数据传递可信 turn/window 标识，桌面 UI 用原生目录选择器和 Pen `yInjn` 风格的双按钮面板完成授权，不把 UI 逻辑放进 Runtime 或 Gio 帧布局。

**Tech Stack:** Go、Gio、现有 MCP Go SDK `github.com/modelcontextprotocol/go-sdk/mcp`、`os/exec` 固定 Git 命令、现有 `internal/i18n` JSON 目录。

**Spec:** [docs/superpowers/specs/2026-10-21-agent-local-folder-access-design.md](../specs/2026-10-21-agent-local-folder-access-design.md)

## Global Constraints

- 授权单位是当前全局 AI 助手的一次用户请求（turn），而非整个会话、工作区或应用生命周期。
- 只提供“拒绝”和“允许本次”两个操作，不提供“始终允许”或权限管理页。
- 授权只存在于内存中，并随当前请求结束、取消或应用退出而清除。
- 目录列表单次最多返回 500 项。
- 单个 UTF-8 文本文件最多读取 1 MiB。
- Git 提交查询每次最多返回 200 条。
- 写入、编辑、移动、删除、执行任意 shell 命令或打开任意 URL 不在能力范围内。
- Git 查询只返回短提交 ID、提交日期、作者显示名、标题和相对变更路径，不包含 diff、patch、作者邮箱或提交正文。
- 用户授权前，本地目录内容不进入模型提示词、Agent 消息、工具结果或诊断日志。
- 授权后，获准读取的文本/Git 元数据会进入模型工具结果并发送给当前 AI 提供商，也会按现有对话持久化机制保存在该对话记录中。
- 文件读取和 Git 查询运行在后台；Gio 窗口循环只负责展示、选择和发布结果。

## Review Focus

- 路径穿越、NUL、平台分隔符变体和指向根目录外的符号链接必须在读取前被拒绝。测试：`TestBrokerRejectsTraversalAndSymlinkEscape`（Task 1）。
- 二进制、无效 UTF-8、超过 1 MiB 的文本和非普通文件必须失败关闭。测试：`TestBrokerReadTextFileRejectsUnsupportedAndOversizedFiles`（Task 1）。
- 非仓库/非仓库根目录、无效日期范围、超时及超过提交/输出上限时，Git 工具必须返回结构化错误而不执行 diff 或任意命令。测试：`TestBrokerGitCommitsBoundsAndFailures`（Task 1）。
- 拒绝、选择器取消、停止生成和迟到决定都不能创建/复用 scope 或把读取结果写入后续 turn。测试：`TestBrokerDecisionAndTurnCleanup`（Task 1）、`TestAgentHostTurnMetadataIsTrustedAndReleased`（Task 2）、`TestAgentLocalAccessPromptCancelAndDeny`（Task 4）。
- 新 turn、多个已授权目录以及带伪造 MCP `_meta` 的外部调用必须保持 scope 隔离，外部调用不能触发桌面授权。测试：`TestBrokerScopesAreTurnAndFolderBound`（Task 1）、`TestMCPServerIgnoresLocalAccessMetadataForExternalAgent`（Task 2）。

---

### Task 1: 本地访问 Broker 与受限文件/Git 读取器

**Files:**
- Create: `internal/localaccess/broker.go`
- Create: `internal/localaccess/filesystem.go`
- Create: `internal/localaccess/git.go`
- Test: `internal/localaccess/broker_test.go`
- Test: `internal/localaccess/filesystem_test.go`
- Test: `internal/localaccess/git_test.go`

**Interfaces:**
- Produces: `type Prompt interface { SelectDirectory(context.Context, SelectionRequest) (string, error); ConfirmDirectory(context.Context, ConsentRequest) (bool, error) }`。
- Produces: `SelectionRequest{TurnID string, WindowID uint64, Purpose string, UserRequestExcerpt string}` 和 `ConsentRequest{TurnID string, WindowID uint64, Purpose string, UserRequestExcerpt string, FolderName string, FolderPath string}`。
- Produces: `func NewBroker(prompt Prompt) *Broker`。
- Produces: `func (b *Broker) RequestDirectory(ctx context.Context, turnID string, windowID uint64, purpose, userRequestExcerpt string) (ScopeSummary, error)`；`ScopeSummary` 只含不透明 `ID`、目录显示名和允许的操作，不含绝对路径。
- Produces: `Entry{Name string, RelativePath string, IsDirectory bool, Size int64}`、`TextFile{RelativePath string, Content string, Size int64}`、`Commit{ID string, Date time.Time, Author string, Subject string, Paths []string}`、`CommitPage{Commits []Commit, HasMore bool}`。
- Produces: `func (b *Broker) ListDirectory(ctx context.Context, turnID, scopeID, relativePath string) ([]Entry, error)`。
- Produces: `func (b *Broker) ReadTextFile(ctx context.Context, turnID, scopeID, relativePath string) (TextFile, error)`。
- Produces: `func (b *Broker) ListGitCommits(ctx context.Context, turnID, scopeID string, since, until time.Time, limit int) (CommitPage, error)`。
- Produces: `func (b *Broker) ReleaseTurn(turnID string)` 和 `func (b *Broker) Close()`。
- `scope_id` 是绑定 turn 的不可重放句柄；它可能进入 Agent 已有的本地工具对话记录，但 Broker 的目录根和授权映射仅驻留内存。

- [x] **Step 1: 写 Broker 生命周期测试**：在 `broker_test.go` 增加 `TestBrokerScopesAreTurnAndFolderBound`（同 turn 同根复用、另一个根重新确认、不同 turn 隔离、拒绝不重弹、picker cancel 不授权）和 `TestBrokerDecisionAndTurnCleanup`（`ReleaseTurn` 后 scope 立即失效且迟到决定不能建立新 scope）。
- [x] **Step 2: 运行 Broker 测试确认失败**：运行 `go test ./internal/localaccess -run '^TestBroker(ScopesAreTurnAndFolderBound|DecisionAndTurnCleanup)$' -count=1`；预期包或测试尚不存在而失败。
- [x] **Step 3: 实现 `broker.go`**：定义本节 Interfaces 列出的 DTO 与方法、稳定错误值、turn map、已授权/已拒绝根目录记录、picker 取消标记、加密随机 scope ID、turn 校验和 `ReleaseTurn`/`Close`。先选择并规范化目录；同 turn 已授权根直接复用 scope；新根才确认；拒绝和取消不建立 scope。
- [x] **Step 4: 运行 Broker 测试确认通过**：再次运行 `go test ./internal/localaccess -run '^TestBroker(ScopesAreTurnAndFolderBound|DecisionAndTurnCleanup)$' -count=1`；预期 PASS。
- [x] **Step 5: 写文件系统测试**：在 `filesystem_test.go` 增加 `TestBrokerListsOnlyDirectChildrenAndSkipsGit`、`TestBrokerRejectsTraversalAndSymlinkEscape`、`TestBrokerReadTextFileRejectsUnsupportedAndOversizedFiles`，覆盖 500 项、路径穿越/外链、特殊文件、常见凭据文件名、二进制、无效 UTF-8 和 1 MiB 上限。
- [x] **Step 6: 运行文件系统测试确认失败**：运行 `go test ./internal/localaccess -run '^(TestBrokerListsOnlyDirectChildrenAndSkipsGit|TestBrokerRejectsTraversalAndSymlinkEscape|TestBrokerReadTextFileRejectsUnsupportedAndOversizedFiles)$' -count=1`；预期新增的读取行为失败。
- [x] **Step 7: 实现 `filesystem.go`**：所有工具参数只接受规范化相对路径；`ListDirectory` 仅接受 `.` 作为授权根；以 `filepath.Rel` 和 `EvalSymlinks` 验证最终对象仍在规范根内；只处理普通文件/目录；按名称和常见格式拒绝凭据/密钥文件；读取前检查文件大小，UTF-8 验证通过后再返回完整文本；输出目录项排序并限制 500 项。
- [x] **Step 8: 运行文件系统测试确认通过**：再次运行文件系统测试命令；预期 PASS。
- [x] **Step 9: 写 Git 查询测试**：在 `git_test.go` 增加 `TestBrokerGitCommitsBoundsAndFailures`，用临时 Git 仓库覆盖最多 200 条/`HasMore`、日期范围、授权根校验、非仓库、无效日期、超时、有界输出，断言不返回邮箱/正文/diff。
- [x] **Step 10: 运行 Git 测试确认失败**：运行 `go test ./internal/localaccess -run '^TestBrokerGitCommitsBoundsAndFailures$' -count=1`；预期 Git 查询行为失败。
- [x] **Step 11: 实现 `git.go`**：以 `exec.CommandContext` 参数数组调用固定的 `git`；校验 `--show-toplevel` 与授权根一致；设置 `GIT_TERMINAL_PROMPT=0`、`GIT_PAGER=cat`、`GIT_OPTIONAL_LOCKS=0`、`GIT_CONFIG_NOSYSTEM=1` 和 `GIT_CONFIG_GLOBAL=os.DevNull`；对 `since`/`until` 支持 RFC3339 或日期格式并验证范围；请求 `limit+1` 条以生成 `HasMore`；设置进程超时和有界输出缓冲，使用固定 NUL 分隔格式解析，不调用 shell/diff/外部 diff。
- [x] **Step 12: 运行 Git 查询测试确认通过**：再次运行 `go test ./internal/localaccess -run '^TestBrokerGitCommitsBoundsAndFailures$' -count=1`；预期 PASS。
- [x] **Step 13: 运行本包完整测试并整理工作区变更**：运行 `go test ./internal/localaccess -count=1`，预期 PASS；本功能的变更集中留在工作区供审阅，不按 task 分批提交。

### Task 2: Agent turn 身份经 MCP 可信传递与结束钩子

**Files:**
- Modify: `internal/agent/host.go`
- Modify: `internal/agent/host_test.go`
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/server_test.go`
- Modify: `internal/application/capability_runtime.go`

**Interfaces:**
- Consumes: Task 1 的 `localaccess.Broker.ReleaseTurn(turnID string)`（后续由 Task 3 接入）。
- Produces: `CapabilityRequest.AgentTurnID string`, `CapabilityRequest.AgentWindowID uint64`, `CapabilityRequest.UserRequestExcerpt string`，全部来自 MCP `_meta`，不来自模型工具参数。
- Produces: 同名 `CapabilityContext` 字段，供本地能力 handler 使用。
- Produces: `HostOptions.OnTurnEnded func(turnID string)`，由 `AgentHost.completeTurn` defer 调用，覆盖成功、失败、取消和恢复轮次。

- [x] **Step 1: 写 AgentHost turn metadata 测试**：增加 `TestAgentHostTurnMetadataIsTrustedAndReleased`，使用 MCP 测试服务断言 `_meta` 带本 turn ID、可信 `ViewContext.WindowID` 和最多 240 rune 的当前用户请求摘要；`Send`/`ResumeConversation` 使用不同 turn ID，成功/失败/取消均调用一次结束钩子。
- [x] **Step 2: 运行 AgentHost 测试确认失败**：运行 `go test ./internal/agent -run '^TestAgentHostTurnMetadataIsTrustedAndReleased$' -count=1`；预期测试尚不存在而失败。
- [x] **Step 3: 实现 AgentHost turn metadata**：在 `runTurn`→`executeToolCall`→`invokeTool` 链路传递现有可信 `requestID`；扩展 `newCallToolParams` 为 `_meta` 写入 `glance/agent_turn_id`、`glance/window_id`、`glance/user_request_excerpt`；用户请求摘要仅从 conversation state 当前用户消息截取前 240 rune；在 `completeTurn` 用 defer 执行可选 `OnTurnEnded`。
- [x] **Step 4: 运行 AgentHost 测试确认通过**：再次运行 `go test ./internal/agent -run '^TestAgentHostTurnMetadataIsTrustedAndReleased$' -count=1`；预期 PASS。
- [x] **Step 5: 写 MCP 元数据信任边界测试**：增加 `TestMCPServerIgnoresLocalAccessMetadataForExternalAgent` 与 `TestMCPServerPassesLocalAccessMetadataForBuiltInAgent`，断言 built-in 服务将三个字段写入 `CapabilityRequest`，external operator 即使提交伪造 `_meta` 也会丢弃字段。
- [x] **Step 6: 运行 MCP 测试确认失败**：运行 `go test ./internal/mcpserver -run '^(TestMCPServerIgnoresLocalAccessMetadataForExternalAgent|TestMCPServerPassesLocalAccessMetadataForBuiltInAgent)$' -count=1`；预期新断言失败。
- [x] **Step 7: 实现 MCP 到 CapabilityContext 的传递**：在 `internal/mcpserver/server.go` 定义上述三个 metadata key；仅 `OperatorBuiltInAgent` 分支解析并写入 `CapabilityRequest`，external 分支忽略；将字段复制到 `CapabilityContext`。
- [x] **Step 8: 运行 MCP 与应用 metadata 测试确认通过**：运行 `go test ./internal/agent ./internal/mcpserver ./internal/application -run '^(TestAgentHostTurnMetadataIsTrustedAndReleased|TestMCPServerIgnoresLocalAccessMetadataForExternalAgent|TestMCPServerPassesLocalAccessMetadataForBuiltInAgent)$' -count=1`；预期 PASS。
- [x] **Step 9: 汇总 turn context 改动**：将列出的 Agent/MCP/application 文件纳入统一审阅变更集；按用户要求不创建 task 级或最终提交。

### Task 3: Runtime 注册本地访问能力并管理 Broker 生命周期

**Files:**
- Modify: `internal/application/runtime.go`
- Modify: `internal/desktopui/runtime.go`
- Create: `internal/application/local_access_capabilities.go`
- Create: `internal/application/local_access_capabilities_test.go`
- Modify: `internal/mcpserver/server.go`

**Interfaces:**
- Consumes: Task 1 的 `*localaccess.Broker` 和 Task 2 的可信 `CapabilityContext.AgentTurnID` / `AgentWindowID`。
- Produces: `RuntimeOptions.LocalAccessBroker *localaccess.Broker`，由 `NewRuntime` 保存并用于注册 handler。
- Produces: `func (r *Runtime) ReleaseAgentTurn(turnID string)`，向 Broker 释放该 turn scope；`Runtime.Close` 关闭 Broker。
- Produces: 四个能力及其参数：`local.request_directory_access({purpose})`（`purpose` 必填，最多 240 rune）；`local.list_directory({scope_id, relative_path})`（相对路径为 `.` 时表示授权根）；`local.read_text_file({scope_id, relative_path})`；`local.list_git_commits({scope_id, since, until, limit})`（日期为 ISO/RFC3339，`limit` 为 1–200，默认 200）。每个能力都使用固定输入/输出 JSON schema 和 `CapabilityContext` 内的可信 turn/window 元数据。
- Produces: 结构化输出：请求目录返回 `{scope_id, folder_name, operations}`；列目录返回 `{entries:[{name, relative_path, is_directory, size}]}`；读文本返回 `{relative_path, content, size}`；Git 返回 `{commits:[{id, date, author, subject, paths}], has_more}`。绝对根路径不在任何模型可见输出中。

- [x] **Step 1: 写能力目录与访问控制测试**：`TestLocalAccessCapabilitiesRegisteredWithBoundedSchemas` 断言四个 capability 的类别/描述及 schema 限制 `purpose`、相对路径、RFC3339/日期和 `limit <= 200`；`TestLocalAccessCapabilitiesRequireBuiltInTurnAndWindow` 断言 external operator、缺失 broker、缺失 turn ID 或 window ID 都返回 `interactive_authorization_unavailable` 且不调用 Prompt。
- [x] **Step 2: 运行新测试确认失败**：运行 `go test ./internal/application -run '^TestLocalAccessCapabilities' -count=1`；预期测试尚不存在而失败。
- [x] **Step 3: 实现 capability registrations**：在 `local_access_capabilities.go` 注册请求目录 `CapabilityWorkflow`（`RiskLow`，由 Broker 等待专用授权）和列目录/读文本/列 Git 提交 `CapabilityQuery`；使用本节定义的字段与输出 shape；handler 解码参数、校验范围，并把可信 turn/window 与 `UserRequestExcerpt` 传给 `localaccess.Broker`；只有 built-in operator 能执行，未注入 broker 时明确失败。读取结果编码后的 JSON 不得超过 AgentHost 1 MiB 工具结果预算。
- [x] **Step 4: 接入 Runtime 创建与清理**：在 `RuntimeOptions`、`Runtime` 加 Broker 字段；`NewRuntime` 注册能力；实现 `ReleaseAgentTurn`；`Runtime.Close` 调用 `Broker.Close`。能力 handler 不把任何绝对路径返回给模型。
- [x] **Step 5: 接入 AgentHost 生命周期**：在 `internal/desktopui/runtime.go` 的 `desktopRuntime.installAgentHost` 设置 `OnTurnEnded: applicationRuntime.ReleaseAgentTurn`。
- [x] **Step 6: 补稳定 MCP 错误码**：在 `mcpserver.errorCode` 将 localaccess 的 UI 不可用、拒绝、取消、超限和 Git 错误映射为稳定代码。
- [x] **Step 7: 运行应用与 MCP 能力测试确认通过**：运行 `go test ./internal/application ./internal/mcpserver -run '^(TestLocalAccessCapabilities|TestMCPServer.*LocalAccessMetadata)$' -count=1`；预期 PASS，并确认 external MCP 仍无法获得授权工具操作。
- [x] **Step 8: 汇总 Runtime capability 改动**：将列出的 application/mcpserver/desktopui 文件纳入统一审阅变更集；按用户要求不创建 task 级或最终提交。

### Task 4: 桌面目录选择、授权面板与异步请求桥接

**Files:**
- Modify: `internal/desktopui/runtime.go`
- Create: `internal/desktopui/runtime_local_access.go`
- Modify: `internal/desktopui/agent_ui_state.go`
- Modify: `internal/desktopui/workspace_async_results.go`
- Create: `internal/desktopui/agent_local_access_modal.go`
- Modify: `internal/desktopui/agent_confirmation_modal.go`
- Modify: `internal/desktopui/agent_confirmation_modal_test.go`
- Modify: `internal/desktopui/agent_chat_ui.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Test: `internal/desktopui/agent_local_access_ui_test.go`
- Test: `internal/desktopui/agent_local_access_ui_test.go`

**Interfaces:**
- Consumes: Task 1 `localaccess.Prompt`；Task 3 `RuntimeOptions.LocalAccessBroker`。
- Produces: `desktopRuntime` 的 `SelectDirectory` / `ConfirmDirectory` 实现，通过可信 `WindowID` 找到 `App`、提交请求后等待对应 UI 答复；等待通过 `ctx.Done()` 取消。
- Produces: 稳定的 `agentUIState.localAccess` 状态，承载 picker 请求/结果、当前授权对话框和持久 Clickable；不在 Gio layout 中启动 picker 或读文件。

- [x] **Step 1: 写 picker/确认桥接测试**：`TestAgentLocalAccessPromptCancelAndDeny` 断言选择器取消不会出现授权面板，拒绝会返回 denial 且不产生 scope，失效 window ID 会失败关闭，ctx cancel 能解除等待。
- [x] **Step 2: 运行 UI 桥接测试确认失败**：运行 `go test ./internal/desktopui -run '^TestAgentLocalAccessPromptCancelAndDeny$' -count=1`；预期测试尚不存在而失败。
- [x] **Step 3: 实现 Runtime 到 App 的请求路由**：创建 `runtime_local_access.go`；不持有 `desktopRuntime.mu` 等待 UI；按可信 WindowID 取得 App，将目录选择/授权请求送入 App channel，并在窗口关闭、turn 取消或 channel 关闭时返回错误。
- [x] **Step 4: 实现 App 原生 picker 异步流程**：在 `agent_local_access_ui.go` 消费选择请求，用 `launchManagedBackground` 调用现有 `chooseOfficeFolder`，picker 标题使用 `agent.local_access.picker.title`；通过结果 channel 回到 UI，空路径表示取消。picker/IO 不在 Gio frame callback 运行。
- [x] **Step 5: 运行 picker/桥接测试确认通过**：运行 `go test ./internal/desktopui -run '^TestAgentLocalAccessPromptCancelAndDeny$' -count=1`；预期 PASS。
- [x] **Step 6: 写授权面板交互测试**：增加允许、拒绝、取消、失效窗口、turn 取消、遮罩拒绝及 1×/2× 实际渲染测试；断言 scope 与当前请求绑定、拒绝不创建 scope，授权卡包含完整路径/用途/当前请求/只读与数据流说明，且只提供“拒绝”“允许本次”。更新旧权限面板回归，验证已删除“始终允许”。
- [x] **Step 7: 运行面板测试确认失败**：运行 `go test ./internal/desktopui -run '^(TestAgentLocalAccessModal|TestAgentPermissionConfirmationModal)' -count=1`；预期新断言因面板仍显示始终允许或缺少本地面板而失败。
- [x] **Step 8: 实现 Pen `yInjn` 风格面板与双按钮规则**：新建 `agent_local_access_modal.go` 使用 560dp 居中卡片与现有权限色板；显示目录名称/完整路径、只读范围、用户请求摘要、AI 用途说明和“内容发送给当前 AI 提供商并保存进本次对话记录”的提示；使用稳定 Clickable；接入 `layoutAgentConfirmationModal` 根 overlay；Esc/遮罩按拒绝处理。将现有通用文件夹权限确认收敛到“拒绝/允许本次”，删除 `alwaysAllow`、`permissionGrants` 和持久授权提示，改为说明授权在当前请求结束后失效，并更新旧确认状态。
- [x] **Step 9: 清理取消与迟到交互**：在 `agent_chat_ui.go` 的 EventTurnCompleted/EventTurnFailed 与 App shutdown 路径清理 pending prompt；picker/consent 结果回到 UI 时校验 turn/request ID，丢弃迟到结果。
- [x] **Step 10: 补齐本地授权中英文案并运行测试**：在两个 locale 增加完整 `agent.local_access.*` 消息（picker、标题、范围、传输/存储说明、按钮、取消/错误）。运行 `go test ./internal/desktopui -run '^(TestAgentLocalAccess|TestAgentPermissionConfirmationModal)' -count=1` 与 `go test ./internal/i18n -count=1`；预期 PASS。
- [x] **Step 11: 汇总 UI 改动**：将列出的 desktopui/i18n 文件纳入统一审阅变更集；按用户要求不创建 task 级或最终提交。

### Task 5: 全局 AI 使用说明与端到端回归

**Files:**
- Modify: `internal/agent/prompts/system.tmpl`
- Modify: `internal/agent/prompts/prompts_test.go`
- Modify: `internal/desktopui/agent_sidebar_cards.go`
- Modify: `internal/desktopui/agent_sidebar_cards_test.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Create: `internal/agent/local_access_e2e_test.go`

**Interfaces:**
- Consumes: Task 1 Broker API、Task 2 turn metadata、Task 3 capability schemas、Task 4 UI authorization flow。
- Produces: 本地工具的中英文显示名称/摘要；Agent 在确实需要本地资料时才请求目录，拒绝/取消后继续在无本地内容的上下文回答。

- [x] **Step 1: 写 Agent、Prompt 和工具摘要测试**：在 `internal/agent/local_access_e2e_test.go` 增加 `TestAgentLocalAccessRequiresApprovalBeforeReading`，使用临时目录、fake Prompt、脚本化 Provider 和 ConversationStore；断言用户 prompt 首先进入 provider、拒绝时模型未收到文件正文、允许时只收到获准文本/Git 元数据、工具结果和不透明 `scope_id` 按现有机制进入对话记录、turn 完成后旧 ID 不能用于新 turn。在 `prompts_test.go` 增加 `TestSystemPromptExplainsLocalAccessAuthorization`；在 `agent_sidebar_cards_test.go` 增加 `TestAgentToolLocalAccessNamesAndSummaries`。
- [x] **Step 2: 运行新测试确认失败**：运行 `go test ./internal/agent/prompts -run '^TestSystemPromptExplainsLocalAccessAuthorization$' -count=1` 和 `go test ./internal/desktopui -run '^TestAgentToolLocalAccessNamesAndSummaries$' -count=1`；预期因缺少本地访问文案和工具摘要而失败。端到端用例在后续步骤运行，不要求它在本任务开始时失败。
- [x] **Step 3: 实现 Agent 本地访问指令**：在 `system.tmpl` 说明本地数据只有在工具请求并获得用户授权后才可读取；不得把用户拒绝视为可重试授权。
- [x] **Step 4: 实现工具显示名/摘要**：在 `agent_sidebar_cards.go` 与中英文 locale 为四个 `local.*` 工具配置成功/失败摘要。
- [x] **Step 5: 运行 Prompt 与工具摘要测试确认通过**：运行 `go test ./internal/agent/prompts -run '^TestSystemPromptExplainsLocalAccessAuthorization$' -count=1` 和 `go test ./internal/desktopui -run '^TestAgentToolLocalAccessNamesAndSummaries$' -count=1`；预期 PASS。
- [x] **Step 6: 运行 Agent 端到端测试确认通过**：运行 `go test ./internal/agent -run '^TestAgentLocalAccessRequiresApprovalBeforeReading$' -count=1`；预期 PASS，并确认工具结果只包含用户已授权的文件/Git 数据。
- [x] **Step 7: 汇总集成回归和文案改动**：将列出的 prompt、Agent UI、locale 和测试文件纳入统一审阅变更集；按用户要求不创建 task 级或最终提交。

### Task 6: 最终证据核对与完整回归

**Files:**
- Verify: `docs/superpowers/specs/2026-10-21-agent-local-folder-access-design.md`
- Verify: `docs/superpowers/plans/2026-10-21-agent-local-folder-access.md`
- Verify: `internal/localaccess`, `internal/application`, `internal/agent`, `internal/mcpserver`, `internal/desktopui`, `internal/i18n`

**Interfaces:**
- Consumes: Tasks 1–5 的交付物。
- Produces: 全量检查结果、1x/2x 对话框实际渲染证据，以及无本地内容提前发送/无 headless 授权的回归证据。

- [x] **Step 1: 运行聚焦回归**：运行所有受影响包的测试；新功能专项与完整 `internal/application` 测试通过。完整 MCP/UI 包仍报告基线中已存在的无关失败，记录在最终结果；检查 `SKIP`，不能把 GPU 验证跳过记作通过。
- [x] **Step 2: 检查权限边界与文档一致性**：逐项对照设计文档验收标准，确认没有持久化授权、权限页、写操作、任意 shell 或 diff 能力；执行 `git diff --check`。
- [x] **Step 3: 在可用 GPU 环境核对面板像素**：已在当前 Metal 环境检查 1×/2× 实际渲染并查看 1× 截图；路径、说明及两个按钮可见。Linux 交叉构建受宿主 CGo 工具链限制，不影响当前 macOS GPU 验证。
- [x] **Step 4: 输出实施结果证据**：记录已通过的命令、视觉检查结果和任何平台限制；本任务不重写用户文档，不触碰实现范围外的工作区文件。

## 执行结果（2026-10-21）

- 本地访问、应用能力、Agent turn metadata/授权端到端、Prompt、MCP 信任边界、桌面授权 UI/工具摘要和 i18n 专项测试均通过；`go test ./internal/application -count=1` 通过。
- 完整 `internal/desktopui` 与 `internal/mcpserver` 包测试仍包含基线中已有的无关失败；基线见 `.superpowers/sdd/2026-10-21-agent-local-folder-access/baseline.log`。新功能相关的 UI 测试通过。
- 授权卡片在可用 Metal GPU 上完成 1×/2× 渲染检查并查看截图；路径、用途、当前请求、只读说明、数据去向和两个决策按钮均可见。
- Windows desktop UI 交叉编译通过。Linux 交叉编译因 macOS 宿主缺少 Linux CGo/Vulkan 工具链而受阻。
- `git diff --check` 通过。按用户要求不进行 task 级或最终提交；功能改动集中保留在工作区供审阅。
- 最终独立审查发现 Git 签名 helper 执行、授权目录被替换、敏感路径别名、私钥格式、turn ID 重用、空提交路径数组、超大目录枚举和路径错误泄漏；已在一次修正中补上 fail-closed 处理、有界结果和稳定错误。审查指出系统文件选择器未绑定触发窗口；当前按窗口路由结果与授权卡并在运行时串行化选择器，原生窗口 owner 仍是已知体验限制。
- 修正后 `gofmt`、`git diff --check`、macOS 相关包 `go build`、Windows 相关包交叉构建，以及 Linux 上 localaccess/agent/application/mcpserver 核心包交叉构建通过。Linux desktopui 仍受宿主 CGo/Vulkan 工具链限制。专项测试是在最终审查修正前通过的；修正后未重跑测试。
- 原生文件选择器尚未绑定调用窗口的 native owner；结果与授权卡仍路由到触发请求的 App，并通过运行时 gate 避免多窗口同时弹出选择器。该体验问题作为次要项延后。
- 本地目录身份在读写前后复核；极端并发进程若在两次检查之间替换并恢复同路径，仍存在窄 TOCTOU 风险，当前不使用平台专属文件描述符路径调用 Git。
