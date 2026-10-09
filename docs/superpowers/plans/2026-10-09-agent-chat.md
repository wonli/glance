# Glance 内置 Agent 聊天与模型配置实施计划

> 本计划只覆盖桌面内置 Agent Host 和聊天界面。它依赖[共享运行时与 MCP 能力计划](2026-10-09-agent-first-mcp.md)中的能力目录和 MCP Server；文档业务逻辑仍只在应用能力中实现。

**状态：** 待审阅
**设计稿：** [Agent、MCP 与共享文档运行时](../specs/2026-10-09-mcp-command-architecture-design.md)

## 目标

让用户在 Glance 配置一个兼容的模型服务地址、模型名和 API Key，然后通过聊天让 Agent 查询/编辑文档并生成日报、周报或新 Word 文档。Agent Host 使用进程内 MCP Client 调用与外部 MCP 相同的工具；聊天 UI 不直连格式适配器。

## 首期范围与约束

- 首期支持 OpenAI-compatible Chat Completions 接口，配置服务地址、模型名和 API Key；通过 `ModelProvider` 接口隔离厂商差异。
- 服务地址和模型名保存在普通用户配置；API Key 只保存在操作系统凭据库，不能落入 SQLite、聊天记录、普通配置或日志。
- 第一版聊天记录只保留在当前应用进程内；退出后不恢复。后续若需持久化另行设计。
- 请求只把用户输入和 Agent 明确调用工具取得的结果发给模型，不自动上传整个当前文档。
- 需要确认的操作由人通过 Gio 确认；模型不能批准自己的待执行操作。
- MCP Runtime/Server 不依赖 Agent Host；未配置模型时，外部 MCP 仍可独立工作。

## 代码文件职责地图

| 路径 | 职责 |
|---|---|
| `internal/agent/provider.go` | 模型 Provider 请求/事件接口和 OpenAI-compatible 实现 |
| `internal/agent/host.go` | 对话轮次、MCP 工具循环、取消、错误和调用上限 |
| `internal/agent/conversation.go` | 当前进程内的消息、工具状态和结果摘要 |
| `internal/agentsettings/settings.go` | 非机密设置读写、凭据库接口和密钥引用 |
| `internal/desktopui/agent_chat.go` | 聊天面板布局、发送/停止操作和持久 widget 状态 |
| `internal/desktopui/agent_settings.go` | Provider 地址、模型、API Key 的配置 UI |
| 桌面 composition root | 将 MCP Server、进程内 MCP Client、Agent Host 和 Gio 事件队列接线 |

## 高风险回归点

- API Key 在字段、错误、日志、配置文件和聊天消息中均不泄漏。
- 模型超时、断网、限流和无效工具参数能显示为可理解的错误，不能卡住 Gio 窗口循环。
- 同一 Agent 工具调用只通过 MCP 能力目录执行，不能绕过 Command Bus 修改文档。
- 取消对话会取消模型请求和当前未完成工具请求，不会留下未说明的后台写操作。
- 需要确认的工具结果在用户批准之前不会修改文档。

## 任务 1：模型设置和凭据库

**文件：** 新建 `internal/agentsettings/settings.go`、`credential_store.go` 和测试；新增 `internal/desktopui/agent_settings.go`；按需要修改配置装配代码和 `go.mod`。

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

- [ ] 为有效/无效 Base URL、模型名、空 Key、凭据读写失败和删除 Key 添加用例。
- [ ] 检查配置序列化不包含密钥；凭据库错误和日志不回显密钥。
- [ ] 实现可注入的凭据库接口，并在本机支持的操作系统上接入系统凭据库。
- [ ] 在 Gio 设置区域提供保存、更新、删除和连通性检查操作；网络检查在后台执行。

## 任务 2：实现模型适配器和 Agent Host

**文件：** 新建 `internal/agent/provider.go`、`openai_compatible.go`、`host.go`、`conversation.go` 和测试。

**接口：**

```go
type ModelProvider interface {
    Stream(context.Context, ModelRequest, func(ModelEvent) error) error
}
type AgentHost interface {
    Send(context.Context, ConversationID, string, ViewContext) error
    Cancel(ConversationID) error
    Subscribe(context.Context, ConversationID, func(AgentEvent)) (cancel func())
}
```

Agent Host 接收 composition root 注入的 MCP `ClientSession`。每轮把模型请求、模型工具调用、MCP `tools/call` 结果和最终答复转换成 `AgentEvent`。限制每轮工具调用次数、单次参数大小和对话上下文大小；事件不得携带 API Key。

- [ ] 用 fake Provider 和 MCP 内存传输覆盖纯文本回复、单次工具调用、多次工具调用、MCP 错误和模型错误。
- [ ] 覆盖停止/取消、超时、空回复、无效工具参数、过量工具调用和 JSON 结果上限。
- [ ] OpenAI-compatible Provider 支持流式文本与工具调用增量；密钥仅放 HTTP Authorization Header。
- [ ] 工具调用只接受 MCP Server 实际发布的工具名和 Schema；不做任意方法或 shell 执行。
- [ ] 把模型错误映射为稳定的 UI 错误类别：未配置、认证失败、限流、超时、网络错误和无效响应。

## 任务 3：接入内置 MCP Client 和应用事件

**文件：** 修改桌面 composition root；新建 `internal/agent/mcp_client.go` 和测试；复用 `internal/mcpserver` 工具注册。

**行为：** 桌面启动时创建一对 MCP 进程内传输，将应用 MCP Server 接到内置 MCP Client，再把 ClientSession 注入 Agent Host。关闭顺序为 Agent Host 停止、MCP Client Session 关闭、应用 Runtime 关闭。应用事件通过 `On` 进入 Gio 事件队列，供聊天面板更新工具进度和文档变更摘要。

- [ ] 验证内置 Client `tools/list` 与外部 MCP Server 的工具列表完全相同。
- [ ] 验证内置 Agent 修改文档时 Gio 在事件循环内更新状态，不跨帧保留 `layout.Context`。
- [ ] 验证应用 Runtime 关闭时会话和订阅均能退出，不留 goroutine。
- [ ] 未配置 Provider 时仍可启动桌面、编辑文档并提供外部 MCP 服务。

## 任务 4：加入 Gio 聊天面板和当前视图上下文

**文件：** 新建 `internal/desktopui/agent_chat.go` 和聊天状态测试；修改主窗口布局与命令路由。

**行为：** 提供消息历史、输入框、发送、停止、流式回复、工具调用名称/状态/摘要、失败说明。持久 widget 放在稳定的界面状态中。窗口循环只消费事件和布局，不执行模型网络请求或文档解析。

Agent 可获得由 UI 组装的 `ViewContext`：活动窗口 ID、文档 ID、文档修订和选中对象 ID。上下文作为普通数据注入 Agent Host；没有可用选区时，工具仍要求明确目标，不能猜测当前标签。

- [ ] 覆盖空对话、长回复滚动、发送/停止、工具进度和错误展示。
- [ ] 覆盖活动文档切换、过期选区版本和没有附着窗口的情况。
- [ ] 覆盖中文输入、IME 组合期间发送行为和窗口循环响应。
- [ ] 让应用事件按文档 ID 关联到聊天中的工具操作，不把其他窗口事件误显示为当前任务结果。

## 任务 5：人工确认、隐私说明和端到端验收

**文件：** 修改 `internal/desktopui/agent_chat.go`、确认事件处理器和帮助/设置说明；增加 Agent Host 集成用例。

- [ ] 工具返回 `confirmation_required` 时显示操作对象、影响摘要、文档版本和批准/拒绝按钮。
- [ ] 只有明确的人类按钮动作能提交一次性批准；模型消息和工具参数不能批准。
- [ ] 用户批准后执行绑定原修订的待处理命令；文档版本变化时要求重新生成计划。
- [ ] 在设置和聊天中说明用户输入及 Agent 明确取得的文档片段会发送到所配置的模型服务。
- [ ] 端到端模拟“检查工作簿数据→整理结果→生成日报 Word→另存新路径”，并断言新文件由应用能力创建、聊天给出保存路径、桌面以文档事件显示新标签。
- [ ] 端到端验证模型未配置时外部 Codex MCP 仍可独立创建 Word 文件。

## 执行顺序

先完成共享 MCP 计划中的任务 1 至 5，再开始本计划任务 1 至 3；本计划任务 4、5 等待应用事件和确认接口稳定后实施。两个计划可以分别验收：先发布无窗口/桌面 MCP 工具能力，再发布可配置模型的桌面聊天。
