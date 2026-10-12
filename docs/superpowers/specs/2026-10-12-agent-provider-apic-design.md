# Agent Provider apic 统一设计

**状态：** 已实现；验证范围与本机网络限制见 `.superpowers/sdd/2026-10-12-agent-provider-unification/progress.md`

**日期：** 2026-10-12

## 目标

将 Glance 的大模型流式 Provider 统一为一套接口：所有服务商实现都通过 `github.com/wonli/apic/v2` 发送请求，并各自放在独立目录。保留 AgentHost 的工具循环、人工确认和 MCP 契约；每个文档会话可以有多个独立聊天会话，聊天上下文持久化到本地工作区数据库，确保更换 Provider/模型或重启应用后仍恢复到指定文档下的指定历史会话。Provider 实现统一的 `ModelProvider.Stream(ctx, request, emit)`，并把服务商流式响应转换为中立 `ModelEvent`。

支持范围与现有模型设置一致：MiniMax、Kimi、DeepSeek、OpenAI 兼容服务和 Ollama。API Key 仍由现有凭据流程注入，不写入普通配置、会话、请求错误或日志。

## 当前状态与问题

- `internal/agent/provider/provider.go` 定义中立的模型请求、消息、工具、事件和 Provider 接口。当前 `AgentHost` 把消息保存在自身的 `conversations` map；模型设置保存时 `installAgentHost` 会替换整个 Host，因此历史随 Host 一起丢失，需要将历史所有权迁到 Host 生命周期之外。
- 当前代码同时有根目录标准库协议适配和 `internal/agent/provider/{kimi,deepseek,ollama}` 下的 apic 适配草稿。根目录适配器已有装配调用点；本次会将这些调用改到新目录实现，但不保留旧构造入口或兼容包装。
- 新目录中的草稿目前只接收一条 system 和一条 user 文本，聚合回复后调用完成回调，尚未实现统一的 `ModelProvider`，也没有序列化工具定义或返回工具调用增量。
- 草稿适配器调用全局 `apic.Init()` 并启用 `Debug`。共享客户端 context 会跨请求共享；调试中间件会记录请求/响应和流内容。两者都不适合桌面聊天的并发和凭据边界。

## 架构

`internal/agent/provider` 根包是唯一的协议中立边界，根目录只保存 `ModelProvider`、`ModelRequest`、`ModelMessage`、`ModelInfo`、工具 DTO、`ModelEvent` 和稳定错误类型。`ModelProvider` 提供模型列表、流式对话和终止活动请求三个操作。AgentHost 不直接依赖 apic。所有服务商目录实现完整接口；本次直接删除或替换根目录的适配器草稿及对应旧构造入口，不保留别名、包装器或双轨实现。桌面装配代码改用各服务商目录的新实现。

```go
type ModelProvider interface {
    ListModels(context.Context) ([]ModelInfo, error)
    Stream(context.Context, ModelRequest, func(ModelEvent) error) error
    Stop(ctx context.Context, requestID string) error
}

type ModelInfo struct {
    ID   string
    Name string
}
```

文档与聊天会话是一对多关系。`DocumentID` 是文档作用域，`ConversationID` 是一条聊天会话的稳定唯一 ID；同一文档可创建和切换多条历史会话，不同文档不能读取或切换彼此的历史。会话 ID 不使用临时 `windowID`。文档关闭后历史仍保留，重新打开文档时按工作区稳定 `DocumentID` 加载该文档的会话列表。没有活动文档时不复用其他文档的聊天历史；新建会话和历史选择必须绑定当前活动文档。

会话仓库由 workspace/runtime 提供，存入现有本地 workspace SQLite 数据库。Agent 定义中立的会话仓库接口，桌面端通过 workspace store 适配该接口；workspace 数据库从当前版本 4 迁移到版本 5，新增独立的 Agent conversation 表，以 `ConversationID` 为主键，并为 `DocumentID` 建立索引，保存标题、标题生成状态、待提交的首条用户消息、中立消息上下文、UI transcript 和创建/更新时间，不覆盖文档快照。接口至少包含 `CreateConversation(documentID)`、`ListConversations(documentID)`、`LoadConversation(documentID, conversationID)`、`SaveConversation(record)`；列表按更新时间倒序，加载时同时校验会话属于所给文档。该接口由 runtime/UI 直接访问，不要求已配置模型或存在活动 AgentHost。打开/恢复文档后 UI 调用列表接口；切换活动文档后只显示该文档的会话；选择历史会话后通过加载接口恢复消息上下文和 transcript。Provider 不拥有会话缓存，也不以模型名或 Provider 名拆分同一聊天会话。

Agent 的模型提示词统一放在 `internal/agent/prompts/` 目录，以 Go `text/template` 模板文件维护，例如 Agent system 指令和新会话标题生成指令。模板使用 `{{.Field}}` 标签插入当前文档视图、首条用户输入等动态值；由 prompts 包集中解析和渲染，模板数据使用明确的结构体，模板通过 `embed.FS` 随程序打包，不依赖运行时工作目录。AgentHost 和标题生成流程不得再内嵌或拼接 prompt 指令字面量；Provider 只接收已构造好的中立消息，不拥有提示词模板。模板缺少变量或渲染失败时，在发送模型请求前返回明确错误，不能静默回退到硬编码提示词。用户消息作为模板数据插入，不作为模板源码二次解析。

用户点击 Pen 设计节点 `L7EDq` 的“新会话”入口后，在当前活动文档下创建新的 `ConversationID`；顶部当前标题入口展示该会话标题并打开同文档历史列表，列表选择切换至对应会话。新会话第一次发送消息时先把首条输入持久化为待提交消息，再通过当前所选 Provider/模型发起独立、无工具的标题请求；请求只包含首条用户消息，不把标题指令拼进会话历史。要求模型返回简短标题，成功后校验并持久化标题，同时将待提交消息提升为正式上下文，再启动正常聊天请求。若标题生成失败，保留持久化的待提交输入并显示重试状态；标题生成成功前不发送正常聊天请求，也不以本地截断或固定文案代替模型标题。应用重启后仍可重试该首条消息的标题生成，不重复追加用户消息。

消息持久化不等待模型请求结束：用户消息在发起流之前保存；助手工具调用在执行前保存；每个工具结果追加后立即保存；完整助手最终消息在流成功后保存。存储失败不能被忽略：用户消息未成功落盘时不发起模型请求；工具调用意图未成功落盘时不执行工具；后续保存失败时停止本轮继续调用模型并向用户报告。未完成的流式回答不写为一条完整助手上下文，取消或失败时保留用户输入、此前历史及已完成工具往返，界面恢复时标示该轮中断并允许继续。推理/思考 transcript 与最终文本分开保存供界面还原，但思考内容不进入发送给另一个 Provider 的提示词历史。

恢复通过 `ResumeConversation` 继续已保存的最后一轮，不再追加用户消息。标题失败时它重试首条消息的模型标题，然后运行普通回复；已命名会话的回复中断时从保存的上下文继续。若恢复时发现已保存的 assistant 工具调用缺少工具结果，为避免重复产生文档副作用，Agent 将保存一个“结果未确认、不会自动重放”的工具结果，再让模型继续处理。

每个服务商单独实现一个根包子目录中的 Provider：

| 目录 | 服务商 / 协议 |
|---|---|
| `internal/agent/provider/minimax` | MiniMax native `/v1/text/chatcompletion_v2`; OpenAI-compatible `/v1/models` catalog |
| `internal/agent/provider/kimi` | Kimi Chat Completions |
| `internal/agent/provider/deepseek` | DeepSeek Chat Completions |
| `internal/agent/provider/openai` | OpenAI 及自定义 Chat Completions 兼容服务 |
| `internal/agent/provider/ollama` | Ollama `/api/chat` NDJSON |

每个服务商目录独立实现 apic 请求、回调注册和响应解析。即使多个服务商使用相似的 SSE/Chat Completions 协议，也不通过共享协议解析器抹平字段差异；各自适配后统一映射为根 provider 包中的中立 DTO。Ollama 使用自己的 NDJSON 回调解析。桌面 composition root 按设置选择 Provider，并注入 URL、模型、API Key 和可选 HTTP client。

所有服务商目录都必须实现统一的 `ListModels(ctx)` 并返回中立 `ModelInfo`（至少包含稳定模型 ID 和展示名称）。目录内部自行处理对应服务商的模型列表 endpoint、认证、响应解析和错误分类；Chat Completions 兼容服务与 Ollama 的路径/响应差异不泄漏到 UI。列表能力由 provider 设置/模型选择流程调用；失败时返回稳定错误，不静默退回到硬编码模型列表。

AI 服务商配置页在用户新增或修改 API Key 后自动调用当前 Provider 的 `ListModels`，不要求用户先保存配置或点击“测试连接”。为避免逐字符发送无效请求，Key 输入短暂停止后触发一次；一次性粘贴后也应很快发起。调用使用当前 provider、BaseURL 和 Key 草稿创建临时 Provider，仅在本次请求内使用 Key，不提前写入系统凭据库。请求在线程/布局之外执行，使用可取消 context；Key、Provider 或 BaseURL 再次变化时取消旧请求并用 generation/config 快照丢弃迟到结果，旧服务商的模型不能覆盖当前选项。加载时显示“正在获取模型”，成功后用结果填充模型选择 options 并显示连接可用；保留仍在列表中的当前模型选择，不自动替用户切换模型。失败时显示稳定、脱敏后的连接错误，清除与旧凭据/地址关联的模型列表并允许编辑后自动重试。只有用户显式保存配置时才持久化 Key 和设置。Ollama 不使用 API Key；切换到 Ollama 或更改其 BaseURL 时直接调用模型列表来填充选项并检查本机服务。现有“测试连接”仍作为单独的聊天补全探测，用于检查模型生成路径；模型列表请求本身不发送文档或聊天内容。

每个 `ModelRequest` 携带由 AgentHost 为一次用户轮次生成的唯一 `RequestID`（仅供本地关联，不能序列化进发往服务商的请求体），同一轮中因工具调用产生的后续模型请求复用该 ID。所有 Provider 都实现 `Stop(ctx, requestID)`：`Stream` 开始时按该 ID 登记本地取消函数及服务商返回的请求标识（如果协议提供），完成、失败或 context 取消时清理登记。调用 `Stop` 时，若该 Provider 有正式支持的远程终止接口且已取得所需标识，应优先发起该远程终止，同时立即取消本地 apic HTTP 流并关闭连接；如果没有远程接口或缺少上游请求标识，则直接取消本地流。远程调用使用传入的独立、有时限 context；远程终止失败不能阻止本地取消，错误按稳定错误类型返回，不能暴露凭据或未筛选的服务端响应。对已结束或未知 ID 的调用可安全重复。AgentHost 取消整轮时仍要终止工具等非模型工作，并调用当前 Provider 的 `Stop`；传给 `Stop` 的 context 不得继承已取消的对话 context。provider 目录只在服务商正式支持时实现远程终止，不为标准 Chat Completions SSE 或 Ollama NDJSON 臆造 endpoint。Provider 内部用于跟踪活动请求的状态必须并发安全，不能跨请求共用 context。

## 统一响应结构与 thinking

Provider 根包定义单一 `ModelEvent` 内容结构，普通回答统一进入 `TextDelta`，模型思考/分析内容统一进入独立的 `ReasoningDelta`；工具调用、Usage、FinishReason、Done 继续使用同一结构。Provider 内部把服务商字段映射到这两个 channel，例如将 `reasoning_content`、`message.thinking` 等映射为 `ReasoningDelta`。不支持或未返回 thinking 的模型只产生 `TextDelta`，其 `ReasoningDelta` 为空。

同一响应块同时含 thinking 和最终文本时，适配器必须同时保留两者，不能用互斥分支丢弃最终文本。AgentHost 把 thinking 作为独立的标准事件转发给 UI，最终文本仍作为 assistant 正文事件；聊天窗口根据统一事件类型分别呈现思考内容和回答，不按 Provider ID 写分支。思考内容不拼入 `TextDelta`，也不作为后续跨模型请求的助手正文；跨 Provider 的上下文使用用户消息、助手最终正文及工具往返记录。

每条助手消息在 UI 中有独立的思考区和正文区。收到 `ReasoningDelta` 时只追加到思考区，并在生成期间展开显示增量内容；收到首个 `TextDelta` 时，将思考区标为完成并自动折叠，再开始显示正文。若一个 apic 回调数据块同时解析出两种增量，Provider/事件转发先发出 `ReasoningDelta`、再发出 `TextDelta`，保证这一帧先完成思考区状态切换，再呈现正文。正文开始后才到达的迟到思考增量仍追加到思考区且保持折叠，不能拼到正文或丢弃。没有 thinking 的模型直接显示正文，不显示空思考区。已完成历史消息默认折叠思考区，用户可主动展开查看；思考文本与正文分别持久化，切换 Provider 或重启后仍可分别还原。

该交互参考 `/Users/mima1234/project/mysql-ai` 的会话处理：生成期间将流式内容实时放入独立的 AI trace 展示；生成完成后，trace 仍可通过“思考过程”入口查看，而最终生成内容独立呈现。Glance 将其落实为聊天消息内的思考区和正文区，并在正文开始前自动折叠思考区；不复用 mysql-ai 的 SQL 结果区或单独 trace 弹窗作为聊天正文。

没有最终文本且没有工具调用的 thinking-only 响应仍视为无有效模型回复；有思考增量的正常回复以最终文本或工具调用判定完成。流式解析、字段识别及“同块双 channel”转换都在 Provider 子包中完成，UI 和 AgentHost 不解析服务商原始 payload。

## apic 调用与流式事件

每次 `Stream` 都创建新的 `apic.ApiId` 和 `apic.ApiClients`，将 `Stream` 设为 true，给 API 对象注册 `OnEvent` 回调，再调用：

```go
apiClient := apic.NewApiClient().WithContext(ctx).WithHTTPClient(config.HTTPClient)
err := apiClient.Call(apiID, options)
```

不使用 `apic.Init()` 全局实例，也不使用没有 context 注入入口的 `ApiId.Call()` 便捷方法。API Key 只进入 Authorization Header。Provider 和 `Options` 均保持 Debug 关闭。

apic 对每个流式响应行调用 API 对象的 `ReceiveEvent`；嵌入的 `apic.Apic` 会把它转给已注册的 `OnEvent` 回调。回调解码 SSE 或 NDJSON 数据，并通过 `emit` 立即发布中立事件：

- 文本增量 -> `ModelEvent.TextDelta`
- 思考/分析增量 -> `ModelEvent.ReasoningDelta`
- 工具调用增量 -> `ModelEvent.ToolCallDelta`
- Token 统计 -> `ModelEvent.Usage`
- 结束原因和完成标记 -> `ModelEvent.FinishReason`、`ModelEvent.Done`

回调类型没有 `error` 返回值。若解码或 `emit` 失败，Provider 保存首个原始错误并取消本次请求 context；`Call` 返回后优先返回保存的回调错误，避免把它误报为取消。正常用户取消则透传 `context.Canceled`，由现有 Agent 错误映射显示“请求已停止”。

apic 当前的流式分支会填充 `ApiId.Response.HttpStatus`，但不调用 `OnHttpStatusError`，也不把非 2xx 响应体写入 `Response.Data`。Provider 必须在 `Call` 后检查状态码，并转换为现有 `HTTPStatusError`，使 401/403、429 和服务端错误继续进入稳定错误分类；不得把原始认证响应或请求头写入日志/用户错误。

## 请求与工具调用

Chat Completions 请求保留协议支持的 role/content/tool 字段，继续发送 `Tools`、`Temperature` 和流式 Usage 选项。Provider 子包将中立持久化消息转换成各自的 wire DTO；例如通用上下文中的工具名称可供 Ollama 使用，但 OpenAI Chat Completions 工具结果仅发送 `role`、`tool_call_id`、`content`，不泄漏内部 `tool_name`。SSE parser 按 index 合并工具调用片段，并分别发出 ID、名称和 arguments 增量，供 AgentHost 的现有累积逻辑使用。

思考内容保存在独立的 `ReasoningBlock`，不写入普通消息。AgentHost 将它按对应 assistant 消息索引放入本地 `ReasoningByMessage` 元数据；DeepSeek 适配器按其 thinking + tools 协议要求映射为 `reasoning_content`。其他 Provider 的请求体忽略此元数据，因此切换到另一个服务商时不会收到前一个服务商的思考内容。

Ollama 适配器接收同一完整消息历史和工具定义，将 NDJSON 的文本、思考、工具调用、Usage、done 字段转换成相同事件。不同服务商的特殊字段（例如 Kimi/DeepSeek 的 thinking 参数）留在各自目录。所有 Provider 既解析服务商专属 thinking 字段，也把最终文本和 thinking 分别映射到统一 channel；若同一流块同时出现两类文本，两者都必须发出。

## 错误、安全和生命周期

- 持续使用 AgentHost 的请求上限、工具回合上限、取消和错误分类；不改变 MCP 能力校验及人工确认流程。
- apic 客户端和 context 按请求隔离，支持窗口关闭和用户停止取消网络请求。
- HTTP 状态、超时、网络、非法请求、非法响应和 emit 回调错误映射成稳定错误，不保留密钥或未经筛选的服务端响应内容。
- 禁止 apic Debug 日志，且不添加 API Key 环境变量回退；凭据仅由现有配置/凭据库装配逻辑传入 Provider。
- 不维护独立 `internal/ai` 包；统一接口与各服务商目录是唯一 Provider 实现。本次不保留草稿 API 的兼容构造器或过渡适配层。

## 验收依据

- 对各 Provider 使用本地 HTTP 测试服务检查请求 URL、Authorization Header、完整 messages/tools 请求体，以及 SSE/NDJSON 流的文本、工具调用、Usage 和完成事件。
- 确认每个 Provider 都能通过自身服务商接口列出模型并返回统一 `ModelInfo`；模型列表请求使用正确的 endpoint、认证和响应映射。
- 验证设置页输入/粘贴 API Key 后会自动触发模型列表请求，成功时验证状态和模型选项更新；失败、连续编辑、切换 Provider/BaseURL 及迟到响应均不会保存草稿 Key 或污染当前模型列表。Ollama 不依赖 Key，切换服务/地址时也能加载模型。
- 启动多个并行模型流后按 `RequestID` 分别调用 `Stop`；确认支持远程终止的 Provider 使用正确 endpoint/上游请求标识，远程失败时仍会关闭本地目标流；其他 Provider 通过取消本地流停止。两类情况都要保证只终止目标流、连接及时关闭、错误安全分类、完成/未知请求的重复 Stop 安全，并清理请求登记。
- 覆盖无 thinking、只有 thinking 增量、同一响应块同时含 thinking 与最终文本的 Provider 流；确认统一事件结构分别保留两种 channel，聊天窗口不按 Provider ID 分支处理。
- 用聊天事件序列验证思考增量先在展开的独立区域显示，首个正文增量到达时思考区自动折叠且正文开始显示；同块双 channel 按思考后正文处理，迟到思考仍单独保留；无 thinking 的回复不显示空思考区。
- 持久化并重新加载一条同时有思考和正文的历史消息；确认正文内容不含思考文本，历史思考默认折叠且可展开查看。
- 确认 Agent system 指令和会话标题指令都由 `internal/agent/prompts/` 下的 Go 模板渲染；动态变量正确替换，缺失变量/模板错误会在请求发出前返回，用户输入不会被作为模板源码重新解析。
- 首次发起新会话时确认先持久化首条输入、再调用模型生成标题；标题失败时输入和待重试状态在重启后仍保留，不会静默生成本地标题或开始正常回复；标题成功后只追加一条用户消息并出现在该文档历史列表。
- 同一 DocumentID 下创建多条会话并通过 `ListConversations` / `LoadConversation` 切换；确认每条上下文独立，列表仅包含该文档记录，按更新时间倒序。
- 先由 Provider A 在文档 D 完成至少一轮 user/assistant 或工具往返，再关闭并重开应用、恢复 D 的历史并换为 Provider B；确认 B 的首个请求仍包含所选会话的中立消息历史。
- 打开两个不同 DocumentID 的文档分别创建多条会话、切换文档并重启应用；确认两边 transcript 和上下文分别恢复，且切换文档不串会话。
- 检查取消 context 会结束流；`emit` 或响应解析错误会返回原始错误；非 2xx 状态被正确分类；并发请求不会共享 context 或事件状态。
- 保留并运行 AgentHost 工具调用、取消、失败恢复、跨重启/跨文档历史隔离及现有模型设置测试。
- 使用 `git diff --check` 检查补丁格式。任何真实模型连通性检查都必须由用户明确选择已配置的服务商后单独执行，不在自动化测试中读取或打印真实凭据。

## 不在本次范围

- 重写 AgentHost 工具循环、MCP 工具目录或人工确认流程；不重做聊天布局，仅为统一 reasoning 事件增加必要的聊天流处理。
- 增加图片、多模态、非 Chat Completions 的云端协议或新的用户设置项。
- 更改 API Key 的存储位置、模型设置文件格式或自动上传文档上下文的行为。
- 将 thinking 作为跨 Provider 的提示词上下文发送；对话上下文只携带用户消息、助手最终正文及工具调用/结果。
