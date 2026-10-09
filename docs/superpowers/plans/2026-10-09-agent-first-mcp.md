# Agent 优先的 MCP 运行时与能力实施计划

> 本计划实施设计稿中的“共享运行时与 MCP 能力”部分。内置聊天单独列在[内置 Agent 聊天计划](2026-10-09-agent-chat.md)，依赖本计划的 MCP 工具接口。

**状态：** 待审阅
**设计稿：** [Agent、MCP 与共享文档运行时](../specs/2026-10-09-mcp-command-architecture-design.md)

## 目标

让 Gio、无窗口 MCP 和内置 Agent 使用同一个无 UI 应用运行时。通过 MCP 暴露完整工作区生命周期、文档查询、每个 Ribbon 操作及 Ribbon 之外的 Agent 工作流。外部 Agent 连接桌面 HTTP 端点时共享未保存会话；通过 stdio 启动时操作独立无窗口会话。

## 技术与约束

- Go 1.27、Gio v0.10.2、现有中立文档模型、格式适配器和 SQLite 工作区。
- MCP 使用官方 [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)，计划固定 v1.7.0；SDK 提供 stdio、Streamable HTTP 和进程内传输。
- 当前只实现 MCP，不实现 WebSocket、mDNS、局域网协作、通用插件或通用 REST API。
- Agent 能力严格大于 Ribbon；不要求 Agent 模拟 Gio 对话框或鼠标点击。
- `layout.Context` 不能离开 Gio 帧。视图上下文只能用明确、可序列化的数据表达。
- Office 第三方库类型留在格式适配器后；保存/覆盖能力仍需满足 round-trip 安全门槛。
- 计划中的测试用于后续实现验收；开始实施后按任务执行，不在当前文档阶段运行测试。

## 代码文件职责地图

| 路径 | 职责 |
|---|---|
| `internal/application/` | Runtime、实时会话、查询、能力目录、Command Bus、事件和 Agent 工作流 |
| `internal/mcpserver/` | MCP 工具注册、无窗口 stdio 和桌面 Streamable HTTP |
| `cmd/glance/` | 默认桌面启动和 `glance mcp serve --stdio` CLI 分发 |
| `internal/desktopui/` | Gio 输入、窗口视图状态和运行时事件消费；不持有共享业务命令实现 |
| `internal/document/`、`internal/format/*` | 中立 DTO 与格式专属读取、编辑、保存 |
| `docs/compatibility/` | Ribbon 能力覆盖清单和已验证支持范围 |

## 高风险回归点

- 未支持或只读格式不能广告编辑/保存能力；必须有 `TestOpenDocumentCapabilityByFormat`。
- 同一规范化路径不能产生多个可写会话；必须有 `TestOpenDocumentReusesCanonicalSession`。
- 未保存会话只能通过桌面 MCP 连接共享；独立无窗口运行时不能读取 SQLite 快照并谎称是实时状态。
- 过期文档版本和重复幂等键不能重复修改；必须有 `TestCommandBusRejectsStaleAndDuplicateRequests`。
- stdio 协议输出只写 stdout，日志和诊断写 stderr。
- 未确认的高影响操作不能因断连、超时或 MCP 客户端不支持确认而执行；必须有 `TestMCPConfirmationRequiresHumanResponse`。

## 任务 1：建立 Ribbon 到 Agent 能力的完整清单

**文件：** 新建 `docs/compatibility/agent-command-coverage.json`、`docs/compatibility/agent-command-coverage.md`、`internal/desktopui/agent_command_coverage_test.go`；盘点当前 Ribbon 分发文件。

**接口与结果：** 每条可触发的 Ribbon 操作有稳定 Action ID、格式、Agent 能力 ID、明确参数、目标类型、状态影响和风险类别。对话框操作改写成直接参数；视图动作注明所需窗口/文档上下文。清单还登记 Ribbon 之外的 Agent 查询和工作流。

- [ ] 添加 `TestRibbonActionCoverageManifest`，对比清单与当前 Ribbon Action ID，检查缺项、重复和孤立项。
- [ ] 按 Word、表格、演示文稿、JSON、Markdown、PDF 和工作区逐项盘点；标注现有能力限制。
- [ ] 审阅清单中的目标、参数、格式能力和视图依赖，确保不把“打开 UI”当成 Agent 操作。
- [ ] 完成清单与覆盖检查。

## 任务 2：提取 UI 无关的 Runtime 和实时文档会话

**文件：** 新建 `internal/application/runtime.go`、`session.go`、对应测试；修改 `internal/desktopui/runtime.go`、`document_session.go`、`session_state.go`、`workspace_sessions.go` 和启动代码。

**接口：**

```go
type RuntimeOptions struct { WorkspaceDBPath string }
func NewRuntime(context.Context, RuntimeOptions) (*Runtime, error)
func (r *Runtime) Close(context.Context) error
func (r *Runtime) OpenDocument(context.Context, string) (DocumentInfo, error)
func (r *Runtime) CreateDocument(context.Context, Format) (DocumentInfo, error)
func (r *Runtime) ListDocuments(context.Context) ([]DocumentInfo, error)
func (r *Runtime) GetDocument(context.Context, string) (DocumentInfo, error)
func (r *Runtime) SaveDocument(context.Context, string) (DocumentInfo, error)
func (r *Runtime) SaveDocumentAs(context.Context, string, string) (DocumentInfo, error)
func (r *Runtime) CloseDocument(context.Context, string) error
```

`DocumentInfo` 提供稳定文档 ID、格式、源路径、修订版本、脏状态、只读状态和支持的操作。具体格式适配器由 Runtime 内部注册；外部只看中立类型。Gio 每个窗口的选区、滚动和布局缓存留在 `desktopui`。

- [ ] 先为 Runtime 生命周期、格式工厂和会话 ID 写应用层测试。
- [ ] 抽出 `sharedDocumentSession` 中的业务文档状态；保留 Gio 视图状态和 widget 所有权。
- [ ] 让桌面启动创建 Runtime，文档打开/保存/关闭调用 Runtime。
- [ ] 覆盖路径规范化、重复打开、脏文档关闭和恢复快照行为。

## 任务 3：建立中立查询与有界结果

**文件：** 新建 `internal/application/query.go` 和测试；仅在缺少中立读取 DTO 时修改 `internal/document/*`。

**接口：**

```go
type InspectRequest struct {
    DocumentID string
    Sheet string
    Range *document.CellRange
    Start, Limit, MaxBytes int
}
type DocumentSnapshot struct {
    Info DocumentInfo
    Content json.RawMessage
    Diagnostics []Diagnostic
}
func (r *Runtime) Queries() *QueryService
func (q *QueryService) InspectDocument(context.Context, InspectRequest) (DocumentSnapshot, error)
```

所有查询须携带文档 ID；表格范围、页数、段落/元素数量和序列化大小都有上限。返回内容只使用中立 DTO，不暴露 Office 库对象。

- [ ] 为支持格式建立有界内容读取夹具，覆盖未知文档 ID 和不适用的筛选参数。
- [ ] 为超大范围、负数边界和超出字节限制添加拒绝用例。
- [ ] 让查询读取 Runtime 的实时会话，而不是工作区恢复快照。

## 任务 4：实现能力目录、Command Bus、事件订阅和 Ribbon 共用处理器

**文件：** 新建 `internal/application/catalog.go`、`command_bus.go`、`events.go` 和测试；修改 Runtime wiring。

**接口：**

```go
type CapabilityDescriptor struct {
    ID, Name, Description string
    Category CapabilityCategory
    InputSchema, OutputSchema json.RawMessage
    Formats []Format
    Risk RiskClass
}
type CommandRequest struct {
    RequestID, IdempotencyKey, DocumentID, CommandID string
    ExpectedRevision uint64
    Operator Operator
    Arguments json.RawMessage
}
type CommandResult struct {
    RequestID, DocumentID string
    Revision uint64
    Data json.RawMessage
}
func (b *CommandBus) Execute(context.Context, CommandRequest) (CommandResult, error)
func (q *QueryService) Execute(context.Context, QueryRequest) (QueryResult, error)
func (r *Runtime) On(context.Context, EventFilter, func(Event)) (cancel func())
```

注册项包含描述、Schema、格式/能力限制、参数校验和 query/command/workflow 处理器。Command Bus 校验文档版本和格式能力；相同幂等键与载荷重放原结果，不同载荷返回冲突。只在成功修改后发布有类型的 Event。入口只能调用已注册操作，不能任意发布成功事件。

- [ ] 覆盖重复/缺失能力 ID、无处理器、无效 Schema、只读格式和不支持功能。
- [ ] 覆盖版本冲突、幂等重放、不同载荷复用幂等键和事件元数据。
- [ ] 先实现 `spreadsheet.cell.set` 与 `word.text_run.replace` 两个纵向切片。
- [ ] 让 Gio 对应按钮提交相同目标 ID、参数和命令 ID，确认 UI 与 MCP 最终经过同一处理器。
- [ ] 保留现有撤销、脏状态、草稿和格式适配器保存语义。

## 任务 5：提供无窗口 MCP stdio 服务

**文件：** 新建 `internal/mcpserver/server.go`、`tools.go`、测试和 `cmd/glance/mcp.go`；修改 `cmd/glance/main.go`、`go.mod`、`go.sum`。

**接口与行为：** `mcpserver.New(runtime *application.Runtime)` 创建由能力目录注册工具的 MCP Server；`RunStdio(ctx)` 在 stdio 上服务。CLI 入口为 `glance mcp serve --stdio`；没有子命令时继续启动桌面。无窗口启动创建自己的 Runtime，处理显式指定的文件路径，不假设有“当前标签页”。

- [ ] 用 SDK 内存传输编写协议集成用例：连接、列举工具、参数校验、结构化结果、取消和关闭。
- [ ] 添加 `TestMCPToolsExactlyMatchCatalog`，保证所有注册能力都映射到 MCP 工具，且 MCP 没有旁路工具。
- [ ] 验证无窗口模式可以新建 Word 文件、写入内容、保存到新路径并关闭。
- [ ] 确保协议 JSON 只写 stdout，日志写 stderr。

## 任务 6：让外部 MCP 通过桌面端点共享实时会话

**文件：** 新建 `internal/mcpserver/desktop_endpoint.go` 和测试；修改桌面 Runtime 生命周期与设置界面，增加 MCP 端点信息输出。

**行为：** 桌面 Runtime 启动 MCP Streamable HTTP 服务，只绑定回环地址并要求 Bearer 凭据；关闭时撤销端点。提供当前端点和 MCP 客户端配置指引，凭据安全保存。外部客户端直接连接桌面 MCP Server，不需要 stdio 到 HTTP 的协议代理。`glance mcp serve --stdio` 始终启动独立无窗口 Runtime，避免把两份会话混为一谈。

- [ ] 覆盖非回环绑定、缺失/无效凭据、桌面退出、CORS/Origin 和重复端点启动。
- [ ] 使用桌面 HTTP MCP 客户端调用工具，验证修改桌面实时会话并触发 Gio 标签/状态更新。
- [ ] 使用 stdio MCP 启动独立 Runtime，验证它不会读取或覆盖桌面未保存会话。
- [ ] 确认桌面关闭和客户端断连能释放 MCP 会话与 goroutine。

## 任务 7：迁移 Word、表格和演示文稿 Ribbon 操作

**文件：** 按任务 1 清单分组新增 `internal/application/capabilities_word.go`、`capabilities_spreadsheet.go`、`capabilities_presentation.go` 及测试；修改对应 Ribbon 事件处理文件。

- [ ] 为每项能力定义直接参数和结构化结果，包含明确文档、段落/运行、工作表/区域、幻灯片/形状 ID。
- [ ] Word 覆盖文本、段落、页面、图片和表格等当前按钮能力；不支持结构返回诊断，不静默重写。
- [ ] 表格覆盖当前单元格、格式、结构、行列和工作表按钮；范围有界，公式计算能力不虚报，`.xls` 只公布已支持项。
- [ ] 演示文稿覆盖当前幻灯片和形状操作；中立几何参数通过格式适配器处理。
- [ ] 每项命令验证成功、无效目标、版本冲突、无操作提交、脏状态和格式兼容性。
- [ ] 按现有交互回归保护 Gio 选择、IME、拖动、对话框草稿和无窗口执行行为。

## 任务 8：迁移 JSON、Markdown、PDF 与工作区操作

**文件：** 新建 `internal/application/capabilities_text.go`、`capabilities_pdf.go`、`capabilities_workspace.go` 和测试；修改清单中对应 UI 事件处理器。

- [ ] 为每项清单能力提供 MCP 可调用的直接参数和明确文档 ID。
- [ ] 对 JSON/Markdown 内容、PDF 页数/范围和工作区列表设置大小上限。
- [ ] 将文件选择器改为显式路径参数；无窗口模式不得试图创建 Gio 文件对话框。
- [ ] 视图动作使用 `ViewIntent`，无附着窗口时返回 `no_attached_view`。
- [ ] 添加 `TestEveryManifestCapabilityIsRegisteredAndCallable` 和 `TestEveryRibbonActionHasMCPEquivalent`。

## 任务 9：加入 Ribbon 之外的 Agent 工作流与人工确认

**文件：** 新建 `internal/application/workflows.go`、`confirmation.go` 和测试；修改 MCP 工具与桌面确认事件。

**首批工作流：** `workbook.audit`、`document.compare`、`document.plan.validate`、`document.plan.preview`、`document.plan.apply`。修改计划包含文档 ID、基准修订、命令 ID、参数和计划 ID；执行前校验目标/能力，执行时拒绝过期修订并报告部分失败。

- [ ] 证明只读审计和比较不修改文档，计划预览不修改文档。
- [ ] 确认请求绑定一次性 ID、目标、影响摘要和修订版本；Agent 传 `confirmed: true` 无效。
- [ ] 对覆盖现有文件或删除等高影响操作建立待确认结果；无确认能力的客户端不能执行。
- [ ] 验证新建路径保存不覆盖源文件，并可通过 MCP 无窗口创建新的 Word 文档。

## 任务 10：最终覆盖与安全验收

**文件：** 更新 `docs/architecture.md`、`docs/office-support.md` 和 `docs/compatibility/agent-command-coverage.md`；保留实现中新增的测试。

- [ ] 执行全量 Go 测试和格式保存/重开测试；记录与本功能无关的基线失败。
- [ ] 端到端验证：无窗口 MCP 列出/打开、检查、修改、另存、关闭。
- [ ] 端到端验证：桌面 MCP、Gio 和内置 MCP Client 看到同一实时文档会话。
- [ ] 验证每个 Ribbon 操作都有 MCP 能力，且目录还含有额外 Agent 工作流。
- [ ] 只记录已由测试和保存/重开证据验证的格式能力。

## 执行顺序

任务 1 后，任务 2 至 6 先建立共享基础；任务 7 至 9 可按格式分组推进，但依赖任务 4 的接口和任务 6 的 MCP 入口；任务 10 最后收口。内置聊天计划在任务 5 后可以开始 Agent Host，但 UI/确认接入要等任务 9 的能力策略稳定。
