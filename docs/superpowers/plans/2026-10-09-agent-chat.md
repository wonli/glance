# Glance 内置 Agent 聊天与模型配置实施计划

> 本计划只覆盖桌面内置 Agent Host 和聊天界面。它依赖[共享运行时与 MCP 能力计划](2026-10-09-agent-first-mcp.md)中的能力目录和 MCP Server；文档业务逻辑仍只在应用能力中实现。

**状态：** 执行中
**设计稿：** [Agent、MCP 与共享文档运行时](../specs/2026-10-09-mcp-command-architecture-design.md)

## 目标

让用户在 Glance 配置一个兼容的模型服务地址、模型名和 API Key，然后通过聊天让 Agent 查询/编辑文档并生成日报、周报或新 Word 文档。Agent Host 使用进程内 MCP Client 调用与外部 MCP 相同的工具；聊天 UI 不直连格式适配器。

## 首期范围与约束

- 直接迁移现有模型调用实现中可复用的 HTTP 请求、Chat Completions 消息结构、SSE 流解析和响应错误处理，目标放在 Glance 的 `internal/agent/provider/`。
- 首批 Provider 为 MiniMax、Kimi、DeepSeek 的 Chat Completions 接口和本地 Ollama；地址、模型名、API Key 从 Glance 设置注入。通过 `ModelProvider` 接口隔离差异。
- 补齐模型工具调用请求/响应字段，使模型可以调用 MCP 工具；现有基础聊天客户端没有完整的工具调用循环，Agent Host 在 Glance 实现。
- 服务地址和模型名保存在普通用户配置；API Key 只保存在操作系统凭据库，不能落入 SQLite、聊天记录、普通配置或日志。
- 直接复制并适配通用 Provider 协议代码和对应协议测试；业务领域专属 Prompt、DTO、应用消息信封、服务端队列/推送代码不复制。任何硬编码密钥都不迁移，统一从凭据库注入。
- 第一版聊天记录只保留在当前应用进程内；退出后不恢复。后续若需持久化另行设计。
- 请求只把用户输入和 Agent 明确调用工具取得的结果发给模型，不自动上传整个当前文档。
- 需要确认的操作由人通过 Gio 确认；模型不能批准自己的待执行操作。
- MCP Runtime/Server 不依赖 Agent Host；未配置模型时，外部 MCP 仍可独立工作。

## 代码文件职责地图

| 路径 | 职责 |
|---|---|
| `internal/agent/provider/provider.go` | 模型 Provider 请求/事件接口和统一消息类型 |
| `internal/agent/provider/chat_completions.go` | MiniMax、Kimi、DeepSeek 共用的 Chat Completions 请求、工具调用响应、HTTP 和 SSE 解析 |
| `internal/agent/provider/ollama.go` | 本地 Ollama 请求/NDJSON 流式响应适配 |
| `internal/agent/host.go` | 对话轮次、MCP 工具循环、取消、错误和调用上限 |
| `internal/agent/conversation.go` | 当前进程内的消息、工具状态和结果摘要 |
| `internal/agentsettings/settings.go` | 非机密设置读写、凭据库接口和密钥引用 |
| `internal/agentsettings/credentials.go` | 操作系统凭据库适配 |
| `internal/desktopui/agent_chat_ui.go` | 聊天面板布局、发送/停止操作和持久 widget 状态 |
| `internal/desktopui/agent_settings_ui.go` | Provider 地址、模型、API Key 的配置 UI |
| 桌面 composition root | 将 MCP Server、进程内 MCP Client、Agent Host 和 Gio 事件队列接线 |

## 高风险回归点

- API Key 在字段、错误、日志、配置文件和聊天消息中均不泄漏。
- 模型超时、断网、限流和无效工具参数能显示为可理解的错误，不能卡住 Gio 窗口循环。
- 同一 Agent 工具调用只通过 MCP 能力目录执行，不能绕过 Command Bus 修改文档。
- 取消对话会取消模型请求和当前未完成工具请求，不会留下未说明的后台写操作。
- 需要确认的工具结果在用户批准之前不会修改文档。

## 任务 1：模型设置和凭据库

**文件：** 新建 `internal/agentsettings/settings.go`、`credentials.go` 和测试；新增 `internal/desktopui/agent_settings_ui.go`；按需要修改配置装配代码和 `go.mod`。

**接口：**

```go
type ModelSettings struct {
    ProviderID string
    BaseURL string
    Model string
    CredentialRef string
}
type CredentialStore interface {
    Get(context.Context, string) (string, error)
    Put(context.Context, string, string) error
    Delete(context.Context, string) error
}
func Load(context.Context) (ModelSettings, error)
func Save(context.Context, ModelSettings) error
```

凭据实现使用系统凭据库；没有可用凭据服务时给出明确错误，不回退到明文文件。设置界面用密码字段输入 Key，保存后只显示已配置状态和脱敏标识。

- [x] 为有效/无效 Base URL、模型名、空 Key、凭据写入失败、Key 轮换和删除 Key 添加用例。
- [x] 检查配置序列化不包含密钥；凭据库写入错误会去除密钥内容。
- [x] 实现可注入的凭据库接口，并接入系统凭据库。
- [x] 在 Gio 设置区域提供保存、更新和删除 API Key 操作。
- [x] 增加由用户主动触发的模型连通性检查；只发送固定探测消息，不附带文档内容，并复用当前输入的密钥或同一服务商已保存的凭据。

## 任务 2：实现模型适配器和 Agent Host

**文件：** 新建 `internal/agent/provider/provider.go`、`internal/agent/provider/chat_completions.go`、`internal/agent/provider/ollama.go`、`internal/agent/host.go`、`internal/agent/conversation.go` 和测试。Provider 适配只使用 Go 标准库，不增加 HTTP/SSE 依赖。

**接口：**

```go
type ModelProvider interface {
    Stream(context.Context, ModelRequest, func(ModelEvent) error) error
}
type ModelRequest struct {
    Model string
    Messages []ModelMessage
    Tools []ModelTool
}
type ModelEvent struct {
    TextDelta string
    ToolCallDelta *ToolCallDelta
    Usage *TokenUsage
    Done bool
}
type ModelMessage struct {
    Role string
    Content string
    ToolCallID string
    ToolName string
    ToolCalls []ModelToolCall
}
type ModelTool struct {
    Type string
    Function ModelToolFunction
}
type ModelToolFunction struct {
    Name string
    Description string
    Parameters json.RawMessage
}
type ModelToolCall struct {
    ID string
    Type string
    Function ModelToolCallFunction
}
type ModelToolCallFunction struct {
    Name string
    Arguments string
}
type ToolCallDelta struct {
    Index int
    ID string
    Type string
    Name string
    ArgumentsDelta string
}
type TokenUsage struct {
    PromptTokens int
    CompletionTokens int
    TotalTokens int
}
type AgentHost interface {
    Send(context.Context, ConversationID, string, ViewContext) error
    Cancel(ConversationID) error
    Subscribe(context.Context, ConversationID, func(AgentEvent)) (cancel func())
}
```

Agent Host 接收 composition root 注入的 MCP `ClientSession`。每轮把模型请求、模型工具调用、MCP `tools/call` 结果和最终答复转换成 `AgentEvent`。限制每轮工具调用次数、单次参数大小和对话上下文大小；事件不得携带 API Key。

- [x] 将可复用的 Provider HTTP/SSE 请求、错误处理和消息结构直接复制并适配到 Glance；由构造函数注入地址、模型、密钥和 `http.Client`，协议适配用 Go 标准库实现。
- [x] 为 Provider 请求、流解析、工具调用 JSON 字段和多段 SSE 工具调用拼装添加回归用例。
- [x] 用 fake Provider 和 MCP 内存传输覆盖纯文本回复、单次及多次工具调用和工具结果回传。
- [x] 补充 MCP 工具调用错误和模型流错误后的恢复回归；MCP 错误会作为 tool result 回传，模型流失败会释放 busy 状态，同一对话可继续发送并获得正常回复。
- [x] 多个工具调用中途取消时，为尚未执行的调用补齐错误 tool result 和完成事件，保持对话消息配对，并验证同一对话下一轮仍可继续。
- [x] 覆盖用户停止/取消与模型流三分钟超时；补齐空回复后的同对话恢复、非对象/超长工具参数、超量工具调用释放 busy 状态，以及超限 JSON 工具结果替换成有界错误并回传模型。
- [x] Chat Completions 适配器支持流式文本、工具调用增量及 Usage；Ollama 使用单独的 NDJSON 适配；云端密钥仅放 HTTP Authorization Header。
- [x] 工具调用只接受 MCP Server 实际发布的工具名和 Schema；不做任意方法或 shell 执行。
- [x] 把模型错误映射为稳定的 UI 错误类别：未配置、认证失败、限流、超时、网络错误、无效请求和无效响应；事件另带稳定错误码。

## 任务 3：接入内置 MCP Client 和应用事件

**文件：** 修改桌面 composition root；新建 `internal/agent/mcp_client.go` 和测试；复用 `internal/mcpserver` 工具注册。桌面 Runtime 订阅文档打开/保存事件，并在 Gio 帧循环中同步工作区标签。

**行为：** 桌面启动时创建一对 MCP 进程内传输，将应用 MCP Server 接到内置 MCP Client，再把 ClientSession 注入 Agent Host。关闭顺序为 Agent Host 停止、MCP Client Session 关闭、应用 Runtime 关闭。应用事件通过 `On` 进入 Gio 事件队列，供聊天面板更新工具进度和文档变更摘要。

- [x] 验证内置 Client `tools/list` 与 MCP 能力目录完全相同，并把该清单提供给模型。
- [x] 验证应用事件进入 Gio 事件队列；视图上下文用可序列化数据构造，不跨帧保留 `layout.Context`。
- [x] 验证桌面窗口的 Agent 订阅在 Host 替换、Agent 禁用和窗口关闭时都会取消；修复替换 Host 后 watcher 一直等到窗口退出的问题。
- [x] 建立 Agent Host 轮次、Agent 订阅 watcher、应用事件总线 `On` watcher 和桌面 Agent watcher 的可等待关闭边界；定向关闭回归及 race 检查通过。
- [x] 将事件队列溢出转发、Agent 模型发送/设置任务和启动时恢复模型 Host 纳入 App 生命周期；关闭时发出取消并等待任务退出。
- [x] 覆盖完整桌面 Runtime 关闭时 App 所属异步任务都已退出：将 144 个 App 方法启动的后台任务统一纳入 `launchManagedBackground`，窗口销毁先关闭 shutdown 通道并等待任务，再关闭会话；增加 AST 回归，防止 App 方法新增未托管的闭包 goroutine。Runtime 工作区写入器和应用事件订阅保留各自的完成/取消边界。关闭、订阅、窗口和任务管理定向测试及 race 检查通过。
- [x] 未配置 Provider 时仍可启动桌面、编辑文档并提供外部 MCP 服务。

## 任务 4：加入 Gio 聊天面板和当前视图上下文

**文件：** 新建 `internal/desktopui/agent_chat.go` 和聊天状态测试；修改主窗口布局与命令路由。

**行为：** 提供消息历史、输入框、发送、停止、流式回复、工具调用名称/状态/摘要、失败说明。持久 widget 放在稳定的界面状态中。窗口循环只消费事件和布局，不执行模型网络请求或文档解析。已接入首页 AI 助手入口、窗口级会话、模型设置入口、流式结果和工具进度。

Agent 可获得由 UI 组装的 `ViewContext`：活动窗口 ID、文档 ID、文档修订和选中对象 ID。上下文作为普通数据注入 Agent Host；没有可用选区时，工具仍要求明确目标，不能猜测当前标签。

- [x] `TestAgentChatEventsKeepLongRepliesAndRenderToolAndErrorStatus` 覆盖空对话、长流式回复、工具进度、错误提示与滚动到最新消息；`TestAgentChatSendAndStopUseBackgroundHostTurn` 通过 Gio 按钮验证后台发送期间窗口仍可处理停止。
- [x] 覆盖活动文档切换、过期选区版本和没有附着窗口的情况：`TestAgentViewContextTracksActiveDocumentAndRevision` 验证活动文档/选区/revision 随视图切换刷新；`TestAgentHostRejectsWriteWhenCapturedViewRevisionBecomesStale` 验证 Agent 调用期间人类先修改文档时旧 revision 写入被拒绝；`TestMCPViewCapabilityWithoutAttachedWindowReturnsNoAttachedView` 验证纯 MCP 调用不会伪造桌面窗口。三项定向测试及 race 检查通过。
- [x] `TestAgentChatDoesNotSubmitChineseIMEComposition` 验证中文 IME 组合期间 Enter 和发送动作都保留未提交文本，收到组合结束事件后可正常发送；后台模型请求阻塞时的停止/窗口响应由 `TestAgentChatSendAndStopUseBackgroundHostTurn` 验证。
- [x] 让内置 Agent 工具事件按 conversation ID 过滤；确认事件携带来源 `window_id` 和文档 ID，定向更新来源聊天，不将其他窗口的确认状态写入当前聊天。

## 任务 5：人工确认、隐私说明和端到端验收

**文件：** 修改 `internal/desktopui/agent_chat.go`、确认事件处理器和帮助/设置说明；增加 Agent Host 集成用例。

- [x] 工具返回 `confirmation_required` 时显示操作对象、影响摘要、文档版本和批准/拒绝按钮。
- [x] 只有明确的人类按钮动作能提交一次性批准；模型消息和工具参数不能批准。
- [x] 用户批准后执行绑定原修订的待处理命令；文档版本变化时要求重新生成计划。
- [x] 在设置和聊天中说明用户输入及 Agent 明确取得的文档片段会发送到所配置的模型服务。
- [x] 端到端模拟“检查工作簿数据→整理结果→生成日报 Word→另存新路径”，并断言新文件由应用能力创建、聊天给出保存路径、桌面以文档事件显示新标签。
- [x] 端到端验证模型未配置时外部 stdio MCP 仍可在独立 Runtime 创建并保存 Word 文件；它不依赖桌面模型 Provider 配置，也不会改动桌面未保存会话。

## 执行顺序

先完成共享 MCP 计划中的任务 1 至 5，再开始本计划任务 1 至 3；本计划任务 4、5 等待应用事件和确认接口稳定后实施。两个计划可以分别验收：先发布无窗口/桌面 MCP 工具能力，再发布可配置模型的桌面聊天。
