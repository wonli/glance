# Agent 优先的 MCP 运行时与能力实施计划

> 本计划实施设计稿中的“共享运行时与 MCP 能力”部分。内置聊天单独列在[内置 Agent 聊天计划](2026-10-09-agent-chat.md)，依赖本计划的 MCP 工具接口。

**状态：** 执行中
**设计稿：** [Agent、MCP 与共享文档运行时](../specs/2026-10-09-mcp-command-architecture-design.md)

## 目标

让 Gio、无窗口 MCP 和内置 Agent 使用同一个无 UI 应用运行时。通过 MCP 暴露完整工作区生命周期、文档查询、每个 Ribbon 操作及 Ribbon 之外的 Agent 工作流。外部 Agent 连接桌面 HTTP 端点时共享未保存会话；通过 stdio 启动时操作独立无窗口会话。

## 技术与约束

- Go 1.27、Gio v0.10.3、现有中立文档模型、格式适配器和 SQLite 工作区。
- MCP 使用官方 [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)，固定 v1.8.0；SDK 提供 stdio、Streamable HTTP 和进程内传输。
- 当前只实现 MCP，不实现 WebSocket、mDNS、局域网协作、通用插件或通用 REST API。
- Agent 能力严格大于 Ribbon；不要求 Agent 模拟 Gio 对话框或鼠标点击。
- `layout.Context` 不能离开 Gio 帧。视图上下文只能用明确、可序列化的数据表达。
- Office 第三方库类型留在格式适配器后；保存/覆盖能力仍需满足 round-trip 安全门槛。
- 按任务用例验收实现；协议边界使用 MCP SDK 内存和 HTTP 传输进行验证。

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

**接口与结果：** 每条可触发的 Ribbon 操作有稳定 Action ID、格式、Agent 能力 ID、目标类型、状态影响、风险类别、用途说明和精确参数 Schema。对话框操作改写成直接参数；视图动作注明所需窗口/文档上下文。能力目录是 MCP 工具定义的来源；清单还登记 Ribbon 之外的 Agent 查询和工作流。

- [x] 添加 `TestRibbonActionCoverageManifest`，对比清单与当前 Ribbon Action ID，检查缺项、重复和孤立项。
- [x] 按 Word、表格、演示文稿、JSON、Markdown、PDF 和工作区逐项盘点；标注现有能力限制。
- [x] 审阅清单中的目标、参数、格式能力和视图依赖，确保不把“打开 UI”当成 Agent 操作；Ribbon 能力均有精确参数 Schema，并由能力目录导出 MCP 工具。
- [x] 完成清单与覆盖检查。

## 任务 2：提取 UI 无关的 Runtime 和实时文档会话

**文件：** 新建 `internal/application/runtime.go`、`session.go`、对应测试；修改 `internal/desktopui/runtime.go`、`document_session.go`、`session_state.go`、`workspace_sessions.go` 和启动代码。

**接口：**

```go
type RuntimeOptions struct {
    WorkspaceDBPath string
    Factories []FormatFactory // omitted to use built-in format adapters
}
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

- [x] 先为 Runtime 生命周期、格式工厂和会话 ID 写应用层测试。
- [x] 在桌面共享会话中把规范文档 ID、路径、修订、格式文档对象及其同步锁提取为 `liveDocumentState`；Gio 控件仍留在窗口视图，应用 Runtime 通过格式中立适配器操作同一份活文档。
- [x] 已评估 `liveDocumentState` 的具体格式对象暂不迁入应用会话：桌面适配器还负责同步 Gio 未提交草稿，当前边界让桌面 Agent、外部桌面 MCP 与 Ribbon 共用同一实时文档，同时避免应用层反向依赖 UI 或绕过草稿。只有定义中立的草稿提交/冲突接口后，才重新评估所有权迁移。
- [x] 让桌面启动创建 Runtime；文档打开/保存调用 Runtime，关闭时同步注销应用会话。
- [x] 覆盖路径规范化、重复打开、脏文档关闭和恢复快照行为。

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

- [x] 为 JSON、Markdown、Word、表格、演示文稿和 PDF 建立有界内容读取夹具，覆盖未知文档 ID 和不适用的筛选参数。
- [x] 为超大范围、负数边界、超出偏移/条数/字节限制添加拒绝用例。
- [x] 让查询读取 Runtime 的实时适配器会话，并返回当前修订与脏状态，不读取工作区恢复快照。

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

- [x] 覆盖重复/缺失能力 ID、无处理器、无效 Schema、只读格式和不支持功能。
- [x] 覆盖版本冲突、幂等重放、不同载荷复用幂等键和事件元数据。
- [x] 先实现 `spreadsheet.cell.set`、`word.text_run.replace` 和 Agent 专用 `word.paragraph.append` 纵向切片。
- [x] 为表格注册公式设置、有界批量写入、区域读取与复制、区域格式、排序、查找、撤销/重做和结构编辑命令；Ribbon 对应操作复用应用命令处理器，批量单元格数、返回结果数和格式/排序范围均有限制，结构修改要求人类确认。
- [x] 让 Gio 对应按钮提交相同目标 ID、参数和命令 ID，确认 UI 与 MCP 最终经过同一处理器。
- [x] 保留现有撤销、脏状态、草稿和格式适配器保存语义；全量桌面回归通过。

## 任务 5：提供无窗口 MCP stdio 服务

**文件：** 新建 `internal/mcpserver/server.go`、`tools.go`、测试和 `cmd/glance/mcp.go`；修改 `cmd/glance/main.go`、`go.mod`、`go.sum`。

**接口与行为：** `mcpserver.New(runtime *application.Runtime)` 创建由能力目录注册工具的 MCP Server；`RunStdio(ctx)` 在 stdio 上服务。CLI 入口为 `glance mcp serve --stdio`；没有子命令时继续启动桌面。无窗口启动创建自己的 Runtime，处理显式指定的文件路径，不假设有“当前标签页”。

- [x] 用 SDK 内存传输覆盖连接、列举工具、结构化结果、版本冲突、取消和关闭。
- [x] 添加 `TestMCPToolsExactlyMatchCatalog`，保证所有注册能力都映射到 MCP 工具，且 MCP 没有旁路工具。
- [x] 验证无窗口模式可以新建 Word 文件、写入内容、保存到新路径并关闭。
- [x] CLI 参数错误和帮助写入 stderr；协议使用 SDK stdio transport，服务路径不写协议外文本到 stdout。

## 任务 6：让外部 MCP 通过桌面端点共享实时会话

**文件：** 新建 `internal/mcpserver/desktop_endpoint.go` 和测试；修改桌面 Runtime 生命周期与设置界面，增加 MCP 端点信息输出。

**行为：** 桌面 Runtime 启动 MCP Streamable HTTP 服务，只绑定 `127.0.0.1` 随机端口并要求 Bearer 凭据；关闭时撤销端点。首页提供当前端点、Token 和可复制的通用连接参数。Token 每次启动重新生成，仅保存在进程内，退出后失效。外部客户端直接连接桌面 MCP Server，不需要 stdio 到 HTTP 的协议代理。`glance mcp serve --stdio` 始终启动独立无窗口 Runtime，避免把两份会话混为一谈。

- [x] 覆盖回环绑定、缺失/无效凭据、非本机 Host/Origin 拒绝、同源预检、桌面退出和重复启动。
- [x] 使用桌面 HTTP MCP 客户端调用工具，验证修改桌面实时会话并同步 Gio 会话修订。
- [x] 使用 stdio MCP 启动独立 Runtime，验证它不会读取或覆盖桌面未保存会话；子进程独立创建、编辑并保存 Word 文档。
- [x] 确认桌面关闭会结束所有 MCP 服务端会话；客户端断连和 HTTP 端点关闭后会话及请求连接可释放。

## 任务 7：迁移 Word、表格和演示文稿 Ribbon 操作

**文件：** 按格式和职责在 `internal/application/` 注册能力并添加测试；Word 能力按文本、页面、元数据、图片等职责拆分。修改对应 Ribbon 映射和覆盖清单。

- [x] 为每项能力定义直接参数和结构化结果，包含明确文档、段落/运行、工作表/区域、幻灯片/形状 ID；MCP 参数和结果 Schema 从能力注册表生成。
- [x] Word 首批接入 Ribbon 能力，包含文本/段落编辑、文本范围复制/剪切/粘贴、符号插入、格式刷、水印、修订跟踪、接受或拒绝全部修订（MCP 高风险调用须人工确认，Ribbon 确认后复用同一能力）、排列对齐和页面设置、页眉页脚、文档属性、页面底色、分页/分栏、段落间距预设和图片插入/替换/删除/缩放/旋转，并通过 MCP 目录导出。
- [x] Word“布局/位置”复用 `word.image.move`：按稳定图片、段落和段内锚点 ID 调整可编辑行内图片的正文流位置；Ribbon 将选中图片放到当前光标位置，MCP 可直接指定目标。当前不创建浮动定位。
- [x] Word“布局/环绕文字”复用 `word.image.wrap.set`：对 `document.inspect` 标记为 `wrap_editable=true` 的 DrawingML 浮动图片，安全移除浮动定位并转换为嵌入型；保留图片和尺寸，正文可能重排。其他复杂结构明确拒绝修改。
- [x] `word.version.list` 查询已保存 DOCX 的最近 50 条版本元数据，返回时间、来源、哈希和大小，不加载版本快照正文；新文档需先保存。
- [x] Word “选择”Ribbon 复用 `word.view.set`：可全选正文、按稳定 run_id 与 Unicode 字符偏移选择明确范围，或清除选择；结果返回所选范围与字符数，只改变附着窗口状态，不修改文档修订。
- [x] `word.autosave.set` 设置 Word 实时会话的自动保存；MCP 调用需要桌面人类确认，Ribbon 通过相同能力切换；未保存的新文档在另存为前不会写文件。
- [x] `document.export_copy` 为 Word 和 XLSX 提供显式路径副本输出；不修改源会话路径、修订或脏状态，不覆盖已有目标。Word 文本或表格单元格有未提交草稿时拒绝导出；两个“共享”Ribbon 按钮复用此命令。
- [x] Word 的 11 项窗口视图操作映射到附着窗口能力；Agent 可读取状态，并组合设置版式、阅读模式、标尺、网格线、导航、多页布局缩放、新窗口和并排查看。
- [x] Word 编辑保护查询与设置注册为能力；设置操作要求桌面人类确认，Ribbon 对话框确认后复用 `word.document.protection.set`。能力说明明确该标记不是密码保护或文件加密。
- [x] `spreadsheet.workbook.recalculate` 提供 XLSX 公式缓存失效与按需重算；“重新计算”和“刷新全部”Ribbon 共用该能力。计算限于当前 Excelize 支持的函数和后续读取的单元格，不预计算整本工作簿、不刷新外部数据连接。
- [x] `spreadsheet.document.export_pdf` 接受明确工作表和单元格范围，限制为 5000 行、100 列和 500000 个单元格；校验后独占创建 PDF，保留源工作簿状态并支持页面方向和锚定图片。MCP 指定任意范围，Ribbon 以当前工作表已用范围复用同一能力。
- [x] 表格“翻译”入口复用 `spreadsheet.cells.set`，MCP 可提供最多 1000 个明确坐标的翻译结果；界面不声称 Glance 内置翻译服务。
- [x] Word“开始/编辑器”和“审阅/编辑器”共用有界 `word.review.editor.analyze` MCP 查询；明确返回启发式规则、稳定正文块/段落/单元格/文本运行 ID 和分页状态，并通过 Gio 后台任务扫描，避免阻塞帧循环。
- [x] Word“审阅/比较”复用只读 `document.compare`：从实时会话选择另一份已打开的 DOCX，按双方 document_id 和修订号比较最多 1000 个正文块，并在后台展示变化路径。
- [x] Word“宏”入口复用只读 `word.macros.inspect`，检查 DOCX 是否包含 VBA 宏部件；明确 Glance 不执行宏，也不判断恶意性。
- [x] Word“打印”入口复用高风险 `word.document.print` 工作流：Agent 请求经 Gio 人工确认后，渲染指定修订版本并调用桌面原生打印服务；Ribbon 在确认预览后复用相同能力，无打印服务时明确返回不支持。
- [x] 表格“打印”入口复用高风险 `spreadsheet.document.print` 工作流：Agent 按明确工作表和范围打印，MCP 请求经 Gio 人工确认；Ribbon 打印当前工作表已用范围，并复用同一打印服务。
- [x] PDF“打印”入口复用高风险 `pdf.document.print` 工作流：Agent 可指定页码范围和份数，MCP 请求经 Gio 人工确认；Ribbon 打印当前 PDF 全部页面，并复用同一打印服务。
- [x] Word“加载项”入口复用低风险 `word.addins.list` 查询：按当前运行时目录列出 Word 可用能力的稳定 ID、操作说明、风险和输入/输出 Schema，可按能力类别筛选。
- [x] Word 原生批注通过 `word.comments.list`、`word.comment.add` 和高风险 `word.comment.delete` 提供有界查询及明确 run_id/comment_id 的创建、删除；Ribbon 新建/删除入口复用同一能力，删除批注由 Gio 人类确认。
- [x] Word“主题”“颜色”“字体”和“翻译”入口分别复用 `word.paragraph.spacing_all.set`、`word.page.background.set`、`word.text_run.font_family.set` 和 `word.text_run.replace`；能力说明与 Schema 提供明确段落间距、颜色、run_id、字体族和替换文本，Agent 可使用任意合法值，不依赖 UI 选区。翻译能力接收调用方生成的译文，不声称 Glance 内置外部翻译服务。 Word“设计器”复用全段落间距能力，UI 提供固定版式预设，Agent 可设置任意明确间距；界面不再声称调用 AI 排版服务。 Word“听写”通过 `word.text.insert` 按明确 run_id 追加文本，或按 paragraph_id 插入文本框；文字由调用方提供，Glance 不内置语音识别。
- [x] 演示文稿“幻灯片浏览”通过 `presentation.view.set` 的 `mode=sorter` 实现附着窗口缩略图网格；Ribbon 共用该 ViewIntent，卡片以稳定 slide_id 选择并返回普通视图，未提交文本/备注草稿和裁剪预览会阻止进入。
- [x] 演示文稿“标尺”通过 `presentation.view.set` 的 `set_ruler_visible` 操作控制附着窗口中的水平和垂直英寸刻度；刻度按当前缩放与幻灯片 point 坐标绘制，只改变窗口状态。
- [x] Word“形状”和“艺术字”Ribbon 复用 `word.text_box.insert`，“图表”Ribbon 复用 `word.table.insert`，“公式”Ribbon 复用 `word.equation.insert`；MCP 使用同一命令并可明确指定目标段落、文本运行、内容和表格尺寸。文档说明当前形状/艺术字仍以文本框表格表达，图表模板仍插入数据表，公式仍是普通文本，不虚报 DrawingML 形状、WordArt 特效、嵌入式图表或 Office 数学对象。
- [x] Word“开始/编辑器”和“审阅/编辑器”共用有界 `word.review.editor.analyze` MCP 查询；明确返回启发式规则、稳定正文目标 ID 和分页状态，并通过 Gio 后台任务扫描，避免阻塞帧循环。
- [x] Word 覆盖文本、段落、页面、图片、表格和窗口视图等当前按钮能力；不支持结构返回诊断，不静默重写。视图命令通过附着窗口上下文读取/设置阅读、版式、分页、导航、标尺、网格线和缩放状态；主题效果也以高风险能力提供明确预设并由 Ribbon 复用。
- [x] 表格“拼写检查”通过 `spreadsheet.text.proofread` 提供有界只读文本检查；Ribbon 与 MCP 共用能力，Agent 可传入调用方词典核对拉丁词条。没有词典时明确不判定拼写；词条未命中只作为复核提示。
- [x] 表格覆盖当前单元格、格式、结构、行列和工作表按钮；已共享打开/另存、复制/剪切/粘贴底层能力、单格写入、公式设置、批量写入、部分区域格式、排序、查找、撤销/重做、页面布局查询与设置、网格线显示、定义名称、冻结窗格、工作表普通视图与分页预览、标尺、缩放、按值筛选、批注、超链接、文本分列、快速填充、去重、数据验证、条件格式、结构化表格、按明确源区域和字段创建数据透视表、按明确源区域/锚点/标题插入柱状图/折线图/饼图、按明确单元格/几何/像素尺寸插入形状和文本框、表格图片列表/插入/移动/替换/调整宽度/删除和需确认的结构修改命令。表格图片已增加 `spreadsheet.images.list`、`spreadsheet.image.insert/move/replace/resize/delete`；Ribbon 的替换、删除、宽度和拖动提交复用注册命令，MCP 使用明确工作表与图片 ID，替换和删除需要人类确认。工作表和工作簿保护通过 `spreadsheet.protection.get/set` 查询或设置；Ribbon 两个保护入口复用同一命令，密码在待确认请求中短暂保留于进程内存，确认后交给格式适配器；确认摘要和返回结果不回显，也不会以明文写入应用设置，MCP 写操作需要人类确认。Ribbon 页面设置、数据工具、图片和保护动作共用同一命令目录与执行处理器。范围保持有界，公式计算能力不虚报，`.xls` 只公布已支持项。
- [x] 演示文稿覆盖当前幻灯片和形状操作；中立几何参数通过格式适配器处理。已将默认版式“新建幻灯片”注册为 `presentation.slide.add`，Ribbon 与 MCP 共用命令处理器；已将幻灯片删除注册为高风险 `presentation.slide.delete`，通过稳定 `slide_id` 定位且不能删除最后一页，Agent 调用须经桌面人类确认，Ribbon 人工操作共用处理器；Agent-only `presentation.slide.duplicate` 按稳定 ID 将整张幻灯片复制到文稿末尾，使用 go-pptx 的受限同文档克隆，遇到未知内部关系时失败关闭；窗口视图和放映导航通过 `presentation.view.set` 共用 ViewIntent，Agent 可查询 `presentation.view.get` 并设置页面、缩放、备注和放映状态；元素文本 `presentation.element.text.set` 与组合样式 `presentation.element.style.set` 通过稳定幻灯片/元素 ID 提供 Agent 编辑，样式命令包含项目符号、文本和形状属性，Ribbon 格式动作共用样式处理器，并保护桌面所有窗口的未提交文本草稿；Agent-only `presentation.element.transform.set` 提供绝对位置、大小和旋转角度，桌面拖动、缩放、旋转也复用同一处理器；演讲者备注通过 `presentation.slide.notes.set` 按稳定幻灯片 ID 和 `expected_notes` 比较后编辑，Gio 备注编辑器复用该命令并提供并发冲突解决选择。 演示文稿转场通过 `presentation.transition.get/set` 提供 Agent 查询和编辑，Ribbon 的淡化、推进、方向、速度和全部应用共用 `set` 能力；表格和图表分别通过 `presentation.element.table.create`、`presentation.element.chart.create` 按稳定 slide_id、明确坐标和真实数据创建，Ribbon 插入入口复用这两个能力；演示文稿“拼写检查”通过有界只读 `presentation.text.proofread` 扫描重复中文标点及中文括号/弯引号数量，明确不宣称词典拼写、语法或语义检查。`presentation.element.textbox.create` 已支持按明确 slide_id 和 point 坐标创建单段落文本框，Ribbon 点击放置与 Agent 调用共用处理器；`presentation.element.shape.create` 已支持显式几何、坐标和尺寸创建矩形、圆角矩形或椭圆，两个 Ribbon 形状按钮共用命令并在点击页面时创建矩形；`presentation.element.picture.create` 已支持显式传入最多 32 MiB 的 PNG/JPEG Base64 数据、point 坐标、适配方式和无障碍替代文本，Ribbon 选图后点击画布调用同一处理器；图片替换通过 `presentation.element.picture.replace` 按明确 slide_id、element_id 和最多 32 MiB 的 PNG/JPEG 数据调用同一处理器，保留图片几何框并重置裁剪；`presentation.element.picture.crop.set` 使用四边万分比设置裁剪，Ribbon 完成裁剪时复用同一处理器；进入、取消和重置裁剪预览通过附着窗口的 `presentation.view.set` ViewIntent，并由 Ribbon 与 MCP 共用；音频和视频通过 `presentation.element.audio.create`、`presentation.element.video.create` 按稳定 slide_id 和本机 source_path 嵌入，Ribbon 文件选择器复用同一能力；幻灯片和形状其余操作继续按明确 ID 与参数接入。
- [x] PPTX 经典幻灯片批注已接入 `presentation.comments.list`、`presentation.comment.add` 和高风险 `presentation.comment.delete`；Ribbon“审阅/新建批注”和“审阅/删除批注”复用同一能力，Agent 可通过明确 slide_id、point 坐标和 comment_id 操作，不依赖当前选区。当前仅编辑经典批注，新版 PowerPoint 批注变更失败关闭；`TestPresentationCommentCapabilitySavesAndReopensWithoutLosingSlideContent` 验证 Glance 生成的 PPTX 保存后重开可读取批注，并保留原幻灯片 ID 和已有文本框内容。此证据不代表 PowerPoint/LibreOffice 互操作验收。
- [x] 演示文稿“背景格式”通过 `presentation.slide.background.set` 按稳定 `slide_id` 写入 `#RRGGBB` 纯色；Ribbon 背景选项与 MCP 共用能力，保存阶段会重开临时 PPTX 并校验颜色再原子替换目标文件。
- [x] 演示文稿“幻灯片大小”通过 `presentation.slide.size.set` 接收 point 宽高（36–4032 point）；Ribbon 提供 16:9、4:3 和 A4 横向预设，MCP 可设置任意合法尺寸。改画布保留现有对象的绝对坐标，暂存 PPTX 会重开并校验尺寸后再原子替换。
- [x] 演示文稿“导出 PDF”通过 `presentation.document.export_pdf` 接入 MCP；Ribbon 预览与 MCP 共用中立快照、A4 页面渲染、修订保护和独占新文件写入。导出限制和媒体占位符警告已记录。人工视觉检查以 Glance 新建 PPTX 和一段中文文本框导出 A4 横向 PDF；页面无裁切/重叠，文本保持单行。检查发现 PDF 曾给无填充/无边框文本元素画默认描边，且横向文本框按纵向页面排版导致误换行；现已修复并由 `TestPresentationPDFLeavesUnstyledTextElementInvisible`、`TestPresentationPDFKeepsWideTextboxTextOnOneLine` 回归覆盖。PPTX 导入样例中的超限媒体和超出文本框边界内容仍按既有策略拒绝导出，不据此宣称完整 Office 投影兼容。
- [x] 演示文稿“打印”通过高风险 `presentation.document.print` 接入 MCP；Ribbon 打印预览确认后复用相同 A4 渲染器和桌面原生打印服务，MCP 请求须经 Gio 人工确认。
- [x] 演示文稿“对齐”提供相对幻灯片画布的水平/垂直预设，通过 `presentation.element.transform.set` 提交显式几何；MCP 可直接指定任意坐标、尺寸和旋转。
- [x] 演示文稿“复制”通过 `presentation.element.text.copy` 按稳定 `slide_id`、`element_id` 读取整个可编辑文本元素的纯文本；“粘贴”通过 `presentation.element.text.set` 写入显式目标文本元素。桌面 Ribbon 使用系统文本剪贴板并复用同一能力，Agent 不依赖 UI 选区；当前只支持单段落文本，换行会明确失败。
- [x] 演示文稿“动画窗格”通过只读 `presentation.animations.list` 按稳定 `slide_id` 检查一页的 OOXML 动画时序；Ribbon 调用同一能力并显示有界动画列表。
- [x] 演示文稿“出现/淡入/飞入/添加动画”通过 `presentation.element.animation.add` 追加点击触发的进入动画；`presentation.element.animation.duration.set` 按明确目标和列表序号调整时长。Agent-only `presentation.element.animation.delete` 按同一修订下的明确 slide_id、element_id、animation_index 删除 Glance 生成的简单进入动画组；不安全或含糊结构失败关闭，保留其余时序。Runtime 与格式层测试覆盖添加、改时长、删除、失败原子性及 Glance 生成 PPTX 的保存重开。
- [x] 在 LibreOffice 26.8 中确认 appear、fade、fly_in 分属三次点击顺序播放；PPTX 经 LibreOffice 保存、关闭和重新打开后，动画列表与播放顺序仍保留。
- [x] 在 WPS Office 12.1.28496 中确认 appear、fade、fly_in 分属三次点击顺序播放；WPS 保存 PPTX、关闭并重新打开后，再次确认三段动画及顺序仍保留。保存后的 OOXML 保留 presetID 1/10/2、淡入/飞入效果和 800 ms 时长。另验证新建演示文稿和新增幻灯片使用空标题/副标题/正文占位符：编辑态显示 WPS 提示，放映态不显示提示；经 WPS 保存、关闭、重开后仍成立。此项记录 WPS 互操作结果，不代表已在 Microsoft PowerPoint 中验证。
- [x] Agent-only `presentation.layouts.list` 返回最多 500 个文档版式及其文档内索引、名称和修订号；`presentation.slide.layout.set` 使用稳定 `slide_id` 和同一修订下的 `layout_index` 修改已有幻灯片版式，Ribbon“版式”选择器复用该能力。格式适配器在暂存 PPTX 中修改“幻灯片到版式”关系，刷新目标页的继承内容/占位符缓存，并在替换前校验关系和重新打开暂存包。使用 WPS Office 12.1.28496 打开 `testdata/office-corpus/pptx/layouts.pptx`，确认 10 页内容与选定版式正常；WPS 保存、关闭并重新打开后再次核对，内容和版式仍保留。此互操作证据限定于 WPS，不代表 Microsoft PowerPoint 验收。
- [x] Word 主题效果、PPTX 主题与配色、演示文稿元素组合均注册为具有精确参数 Schema、用途说明和风险级别的 MCP 能力，并由对应 Ribbon 操作复用。覆盖清单的 306 项 Ribbon Action 已全部映射至能力目录。
- [x] 对能力目录中的 120 条命令用 Schema 合法参数验证过期 revision 会在处理器执行前被拒绝，且 revision/dirty 不变；再将每条命令分发到对应格式的隔离会话，核对成功结果与失败无部分修改：`TestEveryCommandRejectsStaleRevisionBeforeHandlerExecution`、`TestEveryCommandDispatchesSchemaValidRequestWithoutPartialFailure`。
- [x] 补齐逐命令的有效目标成功路径、无效目标、无操作和 dirty-state 专题覆盖，以及需要保存的格式保存/重开矩阵；现有命令专题回归仍分布在各能力测试文件。已补 Word 原生批注/主题效果/加载项目录/打印/图片移动与环绕、文本片段和 Unicode 文本范围编辑/字体段落格式/页面布局/打印、XLSX 批注/超链接/定义名称/公式/公式缓存重算/排序与历史/删重/拆列/Flash Fill/图表创建/条件格式与数据验证/结构化表格与数据透视表/形状与文本框/区域复制与搜索/范围格式与格式刷/工作表结构高风险操作/视图与页面设置/图片插入与编辑/工作表和工作簿保护/按值筛选/工作表打印、PPTX 批注删除/主题配色/幻灯片复制/元素组合/动画/切换效果/背景色/画布尺寸/版式、PDF 导出与打印，以及 PDF 打印的专题证据；`TestWordTextRunReplaceCommandUsesNeutralRunIdentity` 与 `TestWordTextRangeReplaceUsesUnicodeRuneOffsets` 覆盖有效替换、重复 no-op、失败原子性和 DOCX 保存重开；`TestGridResizeCancelAndFailureRestoreGeometry` 覆盖行高/列宽取消与失败后的几何恢复；`TestWordImageCapabilitiesRoundTripAndDelete` 覆盖行内图片移动、失败原子性、no-op 和 DOCX 保存重开后的目标段落归属；`TestWordImageWrapCapabilityRequiresHumanAndPreservesImage` 覆盖浮动图片环绕转换的人工确认、拒绝无副作用、重复 no-op 及精确图片数据/尺寸保存重开；`TestPresentationObjectCapabilitiesUseStableTargetsAndRoundTrip` 覆盖 PPTX 文本、样式、几何、形状、表格、图表和图片命令的稳定目标、无效参数失败原子性、no-op 与两轮保存重开；`TestPresentationSlideNotesAndDeleteCapabilitiesUseStableIDsAndConfirmation` 覆盖幻灯片创建、备注比较写入/冲突/no-op/保存重开、高风险删除确认和禁止删除最后一页；`TestPresentationMediaCapabilitiesValidateAndEmbedExplicitFiles` 覆盖 WAV/MP3/MP4 的显式路径插入、媒体类型与内容校验、无效请求原子性、几何/替代文本及媒体字节保存重开；`TestPDFEditCapabilitiesValidateAtomicityHistoryAndSaveReopen` 覆盖 PDF 页面旋转、文本替换、批注/高亮/墨迹、历史、失败原子性、no-op 和保存重开；`TestJSONTransformAndValidationCapabilitiesPreserveStateAndRoundTrip` 覆盖 Agent 格式化/压缩/键排序/校验、失败原子性、no-op 与保存重开；`TestJSONSearchCapabilityReturnsEscapedPointersWithoutMutation` 覆盖 JSON Pointer 键转义、查询大小写、结果限制/截断、空查询拒绝及只读状态保持；`TestMarkdownEditCapabilityUsesRuneRangesAndRoundTrips` 覆盖 Markdown 全部十种编辑动作、中文 rune 区间、失败原子性和保存重开；`TestSpreadsheetFormulaCapabilityUsesExplicitCellAndRoundTrips` 覆盖 Agent 公式写入、失败原子性、Excel 8192 UTF-16 单元公式边界（补充平面字符按 2 个单元计）、等价公式 no-op 和保存重开；`TestSpreadsheetRecalculateCapabilityRecalculatesWorkbook` 覆盖输入修改后的公式缓存失效、Glance 更新值读取、外部数据不刷新和 XLSX 保存重开；`TestSpreadsheetPrintCapabilityRequiresHumanAndPreservesWorkbook` 覆盖明确范围、Agent 调用的人类确认、参数传递、PDF 捕获/解析和文档状态不变；`TestWordPrintCapabilityRequiresHumanAndValidatesPages` 覆盖 Agent 人工确认、DOCX 实时 PDF 渲染/捕获、越界页码失败和文档状态不变；`TestPresentationLayoutCapabilitiesUseLiveCatalogAndRoundTrip` 覆盖修订绑定版式目录、稳定 slide_id、无效目标、重复设置 no-op 和 PPTX 保存重开；`TestSpreadsheetSortAndHistoryCapabilitiesUseLiveWorkbookAndRoundTrip` 覆盖显式区域排序、无效目标、公式区域失败、撤销/重做和 XLSX 保存重开；`TestSpreadsheetDuplicateRemovalAndTextSplitCapabilitiesRoundTrip` 覆盖高风险删重/拆列处理器、失败原子性、no-op 和 XLSX 保存重开；`TestSpreadsheetFlashFillCapabilityTreatsFilledRangeAsNoop` 覆盖填充、满目标 no-op、无模式失败原子性和 XLSX 保存重开；`TestSpreadsheetChartCapabilityUsesExplicitRangeAndRoundTrips` 覆盖 Agent 图表创建、缺失工作表失败原子性及 XLSX 图表范围保存重开。`TestSpreadsheetConditionalFormatAndValidationCapabilitiesRoundTrip` 覆盖四类条件格式、下拉列表/数字验证、缺失工作表失败原子性、重复设置/清除 no-op、revision/dirty 及 XLSX 保存重开。`TestSpreadsheetTableCapabilityCreatesExplicitTableAndRoundTrips` 覆盖结构化表格创建、无效/重名目标原子性和 XLSX 保存重开；`TestSpreadsheetPivotCapabilityUsesNamedFieldsAndRoundTrips` 覆盖明确源字段/输出范围、无效字段原子性和 pivot/cache 部件保存重开。`TestSpreadsheetShapeCapabilityUsesExplicitTargetsAndRoundTrips` 覆盖形状/文本框的明确目标、尺寸/文字、默认尺寸、无效目标原子性和 XLSX DrawingML 保存重开。`TestSpreadsheetRangeQueryAndFormattingCapabilitiesUseExplicitTargets` 覆盖只读范围复制/搜索、范围格式/格式刷、无效工作表原子性、重复格式 no-op 和样式保存重开。`TestSpreadsheetStructureCapabilityRequiresHumanAndRoundTrips` 覆盖高风险结构命令的待确认/拒绝零副作用/人类批准执行及工作表保存重开。`TestSpreadsheetImageCapabilitiesRequireHumanForDestructiveEditsAndRoundTrip` 覆盖图片插入、列表、移动、保比例缩放、失效图片 ID 失败原子性、重复 no-op、替换/删除待确认与人类批准、拒绝无副作用及图片数据/元数据保存重开。`TestSpreadsheetProtectionCapabilitiesRequireHumanAndRoundTripWithoutLeakingPassword` 覆盖表格保护查询、确认、人类批准、错误密码失败原子性、密码不回显与 XLSX 保存重开。`TestSpreadsheetFilterCapabilityReadsExplicitRangeAndRoutesWindowIntent` 覆盖按值/源行筛选、可见行集计算、附着窗口路由、无窗口/非法请求拒绝和文档 revision/dirty 不变；窗口内行隐藏与清除由桌面网格测试覆盖。`TestSpreadsheetZoomCapabilitiesRouteToAttachedWindowWithoutDocumentMutation` 另行验证窗口缩放的查询、设置、无效窗口、no-op 和文档状态不变，但缩放是视图工作流，不计入 120 项 `CapabilityCommand`。120 项命令的有效目标行为矩阵已完成：90 个 application Runtime 专题测试覆盖全部命令 ID 并通过 race；旧 revision/样例分发与 Office 客户端互操作仍作为独立证据记录，未实测兼容性继续列为限制。
- [x] 补强 XLSX 核心单元格写入的行为矩阵：单格写入覆盖成功、重复 no-op、缺失工作表/非法地址失败原子性及保存重开；`spreadsheet.cells.set` 覆盖批量成功、重复 no-op、重复坐标/缺失工作表失败时原值与 revision/dirty 不变，以及 XLSX 保存重开。证据见 `TestSpreadsheetCellSetCommandMutatesAndRequeriesLiveDocument` 和 `TestSpreadsheetBatchCellSetIsAtomicIdempotentAndRoundTrips`。
- [x] 补强 XLSX 区域复制和搜索查询：有效请求核对返回内容/截断标志与只读状态；缺失工作表、反向区域、超 10000 单元格区域和超 1000 个结果限制均拒绝且 revision/dirty 不变。证据见 `TestSpreadsheetRangeQueryAndFormattingCapabilitiesUseExplicitTargets`。
- [x] 补充 Agent 查询、审批与文档保护专题证据：`TestJSONSearchCapabilityReturnsEscapedPointersWithoutMutation` 覆盖 JSON Pointer 转义、大小写、截断和只读状态；`TestMarkdownEditCapabilityUsesRuneRangesAndRoundTrips` 覆盖十种 Markdown 编辑、中文 rune 区间、失败原子性和保存重开；`TestPDFSearchCapabilityPaginatesWithoutMutation` 覆盖跨页续查、文本边界、越界拒绝和只读状态，且发现并修复 PDF 搜索将 `width/height` 错误解码成 `w/h` 导致边界尺寸归零；`TestPresentationProofreadCapabilityPaginatesStableFindingsWithoutMutation` 覆盖 PPTX 检查分页、中文标点/括号发现、稳定 slide/element ID、无效起点及只读状态；`TestWordDocumentProtectionCapabilityRequiresHumanAndRoundTrips` 覆盖 Word 编辑保护查询、高风险 Agent 确认、Agent 无法自批、桌面人类批准、no-op 和 DOCX 保存重开；`TestDocumentPlanApplyRequiresHumanAndReportsPartialFailure` 覆盖高风险计划审批、逐条 revision、成功执行和中途失败后的部分提交/后续跳过语义；`TestDocumentSaveCapabilityRequiresHumanAndWritesLiveSession` 覆盖 MCP Agent 保存确认、`confirmation.get` 待处理/完成状态查询、批准前不写文件、实时会话落盘/重开以及 clean no-op。
- [x] 修复并验证能力目录参数 Schema 的运行时契约：新增 `TestValidateArgumentsEnforcesDeclaredSchemaConstraints`，覆盖 enum/const、精确数值上下界、`exclusiveMinimum`/`multipleOf`、Unicode 字符长度、正则、数组长度/唯一性、嵌套 item 约束及 `anyOf`/`allOf`/`oneOf`；这同时修复 `word.page_setup.*` 越界参数被 Schema 放行的问题。Word 页眉/页脚/页码、校对语言与六项页面设置现在根据 DOCX 包部件快照判定真实变化，重复设置不会增加 revision/dirty，桌面实时会话也有 no-op 回归；字体族、字号、列表、段间距、段落对齐/缩进与组合文本样式也都有重复 no-op 和修订号断言。Word 元数据与页面底色专题增加了重复写入 no-op、非法输入和状态不变断言。`TestEveryCommandRejectsStaleRevisionBeforeHandlerExecution` 与 `TestEveryCommandDispatchesSchemaValidRequestWithoutPartialFailure` 的 120 项命令目录矩阵重新通过；该分发矩阵仍不替代逐命令有效目标编辑矩阵。
- [x] 按现有交互回归保护 Gio 选择、IME、拖动、对话框草稿和无窗口执行行为：验证 `TestCellSelectionSyncsNameAndValue`、`TestAgentChatDoesNotSubmitChineseIMEComposition`、`TestGridResizeCancelAndFailureRestoreGeometry`、`TestMarkdownDialogCancellation` 和 `TestMCPViewCapabilityWithoutAttachedWindowReturnsNoAttachedView`。

## 任务 8：迁移 JSON、Markdown、PDF 与工作区操作

**文件：** 按格式和职责在 `internal/application/` 注册 JSON、Markdown、PDF 与工作区能力并添加测试；JSON 转换能力位于 `json_capabilities.go`，有界搜索和原文读取位于 `json_search.go`；Markdown 编辑能力位于 `markdown_capabilities.go`。修改清单中对应 UI 事件处理器。

- [x] 为每项清单能力提供 MCP 可调用的直接参数和明确文档 ID。
- [x] JSON/Markdown/PDF 打开、JSON/Markdown 另存及相关演示文稿创建/打开/另存 Ribbon 操作映射到文档生命周期能力；PDF 不支持任意排版改写，受支持的页面视图、文本替换与批注按各自能力逐项接入。
- [x] JSON 格式化、压缩、键排序、校验和有界搜索注册为 MCP 能力；Gio 按钮通过同一应用处理器执行，操作要求文档修订号并限制在 8 MiB。搜索最多遍历 200000 个值、深度 128 并返回 500 项。
- [x] JSON 树全部展开、折叠和层级设置通过 `json.view.set` 与附着窗口 ViewIntent 共用处理器；无窗口时返回 `no_attached_view`。
- [x] JSON 有界搜索返回可继续访问的 JSON Pointer；原文读取/复制最多 8 MiB，Ribbon 复制按钮复用同一读取处理器。
- [x] Markdown 标题、文本样式、引用、代码、链接、图片、列表、待办和公式通过 `markdown.text.edit` 注册；输入使用明确 rune 区间，Ribbon 按钮复用同一命令，内容限制为 8 MiB。
- [x] Markdown 编辑、预览和分屏通过 `markdown.view.set` 路由到附着窗口 ViewIntent；无窗口时返回 `no_attached_view`。
- [x] `markdown.document.export_pdf` 从实时 Markdown 快照生成 PDF，支持纵向/横向，报告不可用图片和布局诊断；桌面和 MCP 共享 PDF 校验及不覆盖目标的写入器，待同步的编辑草稿或 IME 组合文本会阻止快照导出。
- [x] PDF 搜索栏、页码导航、缩放、页面适配、缩略图和阅读/批注/签名模式注册为 `pdf.view.set`；`pdf.view.get` 查询附着窗口状态，Ribbon 通过同一 PDF ViewIntent 处理器更新视图。Agent 专属 `pdf.document.search` 按页范围执行有界文本搜索并返回命中定位和截断状态。
- [x] PDF 页面旋转、高亮、文本批注、墨迹、撤销/重做和高风险文本替换注册为显式页面/坐标/运行参数的命令；桌面操作复用 MCP 能力处理器，文本替换按高风险确认。PDF 另存和下载按钮在路径选择后调用 `document.save_as`，使用不覆盖已有目标的保存处理器。
- [x] Word、表格、演示文稿、JSON、Markdown 和 PDF 的保存按钮映射到 `document.save`；Agent 覆盖源文件前等待桌面人类确认，无源路径时引导使用不覆盖目标的 `document.save_as`。
- [x] 对 JSON/Markdown 内容和 PDF 页数/范围设置大小上限：通用查询限制条目数、序列化字节数和表格单元格数；JSON 原文与 Markdown 编辑内容最多 8 MiB；PDF 按页分页，并限制返回的文本和批注元素数量。
- [x] 工作区最近文件查询按格式与路径筛选，使用 limit/offset 分页；单次最多返回 100 项，筛选结果最多跳过 10000 项。
- [x] Agent 文件能力使用 `document.open`、`document.save_as` 等明确路径参数；无窗口 MCP 不创建 Gio 文件对话框。
- [x] 已注册的视图动作使用 `ViewIntent`，无附着窗口时返回 `no_attached_view`；窗口 ID、文档 ID、修订号和动作参数都是显式数据，不把 Gio `layout.Context` 或 widget 带出帧循环。
- [x] `TestRibbonActionCoverageManifest` 核对全部 306 个 Ribbon 操作与当前 UI 来源键、能力 ID 和已实现状态；`TestEveryManifestCapabilityIsRegisteredAndCallable` 验证 Ribbon 与 36 项 Agent-only 清单能力均已注册、导出为带说明和 Schema 的 MCP 工具，并逐项经 MCP 调用分发。

## 任务 9：加入 Ribbon 之外的 Agent 工作流与人工确认

**文件：** 新建 `internal/application/workflows.go`、`confirmation.go` 和测试；修改 MCP 工具与桌面确认事件。

**首批工作流：** `workbook.audit`、`document.compare`、`document.plan.validate`、`document.plan.preview`、`document.plan.apply`。修改计划包含文档 ID、基准修订、命令 ID、参数和计划 ID；执行前校验目标/能力，执行时拒绝过期修订并报告部分失败。

**确认接口：** 高影响能力调用返回 `confirmation_required` 和一次性 ID，不执行原操作。桌面通过 `confirmation.requested` / `confirmation.updated` 事件显示批准或拒绝卡片；只有 Gio 人类交互能调用运行时确认入口。MCP 暴露 `confirmation.get` 供 Agent 查询结果，不暴露批准工具。批准后按原参数重放，并重新校验文档修订版本；stdio 无窗口会话不能批准，未处理请求到期后失效。

**已接入的 Agent 工作流：** `workbook.audit` 提供有界只读审计；`document.compare` 对两个同格式文档执行有界、只读的中立视图比较，输出变化路径和修订号，不声称比较未读取的部分。`document.plan.validate` 校验同一文档的已注册命令、Schema、格式和修订并生成有时限的计划 ID；`document.plan.preview` 只读显示计划步骤、参数与风险，不伪造命令处理器未提供的模拟差异；`document.plan.apply` 在桌面人工确认后按序执行，命令失败时停止并返回部分结果，不自动回滚。

- [x] 实现有界只读 `workbook.audit`：检查重复表头、缺失必需表头、重复键和错误样式值，统计空白/公式单元格；不重新计算公式，限制区域和返回发现数量。
- [x] 实现单文档修改计划校验、只读步骤预览和人工确认后逐项应用；限制计划命令数、参数字节数和有效期，失败时停止并报告部分结果。
- [x] 证明只读审计和比较不修改文档，计划预览不修改文档。
- [x] 确认请求绑定一次性 ID、目标、影响摘要和修订版本；Agent 传 `confirmed: true` 无效。
- [x] 对覆盖现有文件或删除等高影响操作建立待确认结果；无确认能力的客户端不能执行。
- [x] 验证新建路径保存不覆盖源文件，并可通过 MCP 无窗口创建新的 Word 文档。

## 任务 10：最终覆盖与安全验收

**文件：** 更新 `docs/architecture.md`、`docs/office-support.md` 和 `docs/compatibility/agent-command-coverage.md`；保留实现中新增的测试。

- [x] 执行全量 Go 测试和格式保存/重开测试；记录与本功能无关的基线失败。
- [x] 端到端验证：无窗口 MCP 列出/打开、检查、修改、另存、关闭。
- [x] 端到端验证：桌面 MCP、Gio 和内置 MCP Client 看到同一实时文档会话。
- [x] 静态核对覆盖清单：306 项 Ribbon 操作全部映射到能力目录；目录另含 36 项 Agent 专属查询与工作流。
- [x] 校正证据矩阵：明确 MCP 清单覆盖测试只证明目录发布和路由分发，不代表每条命令均有成功编辑或 Office 兼容证据；更新 PPTX 图片裁剪/对象样式和 Word 段前段后间距状态，分别链接 Gio 与保存/重开测试。其余未验证互操作性仍保留为限制。

## 执行顺序

任务 1 后，任务 2 至 6 先建立共享基础；任务 7 至 9 可按格式分组推进，但依赖任务 4 的接口和任务 6 的 MCP 入口；任务 10 最后收口。内置聊天计划在任务 5 后可以开始 Agent Host，但 UI/确认接入要等任务 9 的能力策略稳定。

## 增量回归：Agent 单元格 JSON 类型

- [x] `spreadsheet.cell.set` 与 `spreadsheet.cells.set` 接收 JSON 字符串、数字和布尔值；字符串按文本保存，保留前导零；数字/布尔值保持 XLSX 类型。文本上限按 Unicode 字符计算。`TestSpreadsheetCellCommandsPreserveExplicitJSONValueTypes` 覆盖类型、no-op、失败原子性及保存重开；桌面实时会话由 `TestAgentSpreadsheetCellCommandsPreserveTypedValuesInLiveSession` 覆盖。Ribbon 现有字符串推断行为不变。
- [x] 120 项命令均关联到有效 Runtime 行为用例；90 个专题测试通过 race，旧 revision 检查与成功/预期失败分发矩阵也通过 race。
- [x] 对齐 Word 段落、文本运行、文本框、公式、页眉和页脚输入的长度契约：处理器按 Schema 声明的 Unicode 字符上限执行。`TestWordParagraphAppendUsesUnicodeCharacterLimitAtomically`、`TestWordTextInsertAcceptsUnicodeWithinSchemaLimit`、`TestWordTextBoxCapabilityRoundTrips`、`TestWordEquationCapabilityReplacesRunsAndInsertsTextBoxes` 与 `TestWordHeaderFooterUseUnicodeCharacterLimitsAndRoundTrip` 覆盖长中文成功、超限或换行失败、状态原子性和适用的 DOCX 保存重开。
- [x] 对齐表格批注、区域搜索和拆列分隔符的 Unicode 字符长度限制；处理器按 Schema 码点上限而非 UTF-8 字节数拒绝输入。`TestSpreadsheetCommentUsesUnicodeCharacterLimits` 覆盖长中文作者/批注、超限原子性及保存重开；`TestSpreadsheetRangeQueryAndFormattingCapabilitiesUseExplicitTargets` 覆盖多字节搜索；`TestSpreadsheetDuplicateRemovalAndTextSplitCapabilitiesRoundTrip` 覆盖 emoji 分隔符拆列和保存重开。
- [x] 对齐定义名称字段和 Word 范围替换的 Unicode 长度限制。`TestSpreadsheetDefinedNameCapabilitiesRoundTripAndNoop` 覆盖中文说明保存重开及超限失败原子性；`TestWordTextRangeReplaceUsesSchemaUnicodeLength` 覆盖 1 MiB 以上 UTF-8 替换、基于 rune 的后续范围替换和实时正文。
- [x] 补齐 `word.review.revisions.apply` 的真实 DOCX Runtime 矩阵：接受/拒绝均经人工确认，Agent 自批/人工拒绝/非法 action 无副作用，核对 revision 计数、dirty、保存重开后文本语义以及重复 no-op。证据见 `TestWordRevisionApplyUsesLiveDOCXAndRoundTrips`。
- [x] 补强 `word.blank_page.insert` 的真实目标矩阵：验证非空 Unicode rune 范围替换和前后文本保留、可编辑插入位置、两个分页符、缺失 run/越界失败原子性及保存状态。证据见 `TestWordBlankPageCapabilityInsertsTwoPageBreaks`。
- [x] 修正 DOCX 书签和超链接格式层对不存在的显式 `run_id` 回退到首个文本运行的问题；无效目标现失败且不改 XML/undo，MCP Runtime 维持 revision/dirty。证据见 `TestWordBookmark_UnknownExplicitRunDoesNotFallBackOrMutate`、`TestWordHyperlink_UnknownExplicitRunDoesNotFallBackOrMutate` 和 `TestWordBookmarkAndHyperlinkCapabilitiesRoundTrip`。
- [x] 修正 DOCX 表格与文本框格式层对不存在的显式 `after_paragraph_id` 回退到文档末尾的问题；格式层和 MCP Runtime 都验证失败时 XML、undo、revision/dirty 不变，空目标仍保留默认插入。证据见 `TestWordInsertTable_UnknownExplicitParagraphDoesNotFallBackOrMutate`、`TestWordTextBox_UnknownExplicitParagraphDoesNotFallBackOrMutate`、`TestWordTableInsertCapabilityCreatesEditableTable` 和 `TestWordTextBoxCapabilityRoundTrips`。
- [x] 修正 DOCX 正文段落格式 API 对未知显式锚点回退到文末的问题；`InsertParagraphAfter` 现在在写入前拒绝无效 ID，默认正文追加路径不变。证据见 `TestInsertParagraphAfterUnknownExplicitTargetDoesNotFallBackOrMutate`、`TestInsertParagraphAfterSpecificParagraphAndUndo`。
- [x] 增加 JSON、Markdown、PDF 和 Word 窗口工作流能力的逐项路由回归：验证指定窗口收到准确操作、参数、文档 ID 和 revision；Schema 错误及未附着窗口不触达处理器，文档 revision 不变。证据见 `TestWindowCapabilitiesRouteValidRequestsAndRejectInvalidContext`；实际 Gio 状态与 Ribbon 行为另由现有各格式窗口测试覆盖。
- [x] 对齐 Word 与 PPTX 批注的 Schema/运行时 Unicode 长度契约：Word 正文/作者分别允许 32768/256 个 Unicode 字符，PPTX 分别允许 8192/256 个；列表仍按 8 KiB 安全截断且保留有效 UTF-8，保存重开保留完整批注。证据见 `TestWordCommentUsesUnicodeCharacterLimitsAndRoundTrips`、`TestPresentationCommentUsesUnicodeCharacterLimitsAndRoundTrips`。
- [x] 补强 Word 修订跟踪与水印的状态回归：修订跟踪启用/关闭后均检查 revision/dirty、错误参数原子性、重复设置 no-op 和两种状态各自的 DOCX 保存重开；水印检查写入后 dirty、保存后 clean 及 DOCX 保存重开。证据见 `TestWordTrackRevisionsCapabilitiesUseAbsoluteState` 和 `TestWordWatermarkCapabilitiesRoundTripThroughRuntime`。
- [x] 补强 PPTX 组合能力的 Agent 审批路径：Agent 请求在审批前不改变文档，人类批准后组合成功，失效元素目标失败原子性，组合结果经 PPTX 保存重开。证据见 `TestPresentationGroupCapabilityUsesExplicitAdjacentElementIDs`。
- [x] 补齐 Word 稳定目标编辑的错误原子性与保存重开：字体、段落列表/间距/对齐/缩进、组合文本样式、格式刷、听写插入、公式文本和分页符均验证未知 run/paragraph ID 不改变 revision/dirty；格式刷、听写、公式文本、分页符和文本范围剪切/粘贴均验证 DOCX 保存重开。另为封面插入补充 dirty 与保存后 clean 断言。证据见 `TestWordFontListAndSpacingCapabilitiesRoundTrip`、`TestWordParagraphAlignmentAndIndentCapabilitiesRoundTrip`、`TestWordTextRunStyleSetIsIdempotentAndRoundTrips`、`TestWordFormatPainterUsesExplicitSourceAndTargetRuns`、`TestWordTextInsertCapabilitySupportsExplicitRunAndParagraphTargets`、`TestWordEquationCapabilityReplacesRunsAndInsertsTextBoxes`、`TestWordPageBreakCapabilitySplitsUnicodeRun`、`TestWordTextRangeCopyCutAndPasteUseExplicitUnicodeRanges` 和 `TestWordCoverPageCapabilityRoundTrips`。
- [x] 逐项验证目录内 22 项高风险命令的 Agent 调用先进入人类确认，审批前及拒绝后文档状态不变；Word 图片替换/删除与 PPTX 组合另通过各自命令的人类批准路径执行回归。证据见 `TestEveryHighRiskCommandRequiresHumanConfirmation`、`TestWordImageCapabilitiesRoundTripAndDelete` 和 `TestPresentationGroupCapabilityUsesExplicitAdjacentElementIDs`。
- [x] 补齐页眉/页脚/页码、页面底色、全段落间距、Word/PPTX 批注删除的 mutation dirty 与 save-clean 断言，保持其重复设置、无效请求及保存重开检查。证据见 `TestWordHeaderFooterCapabilitiesRoundTrip`、`TestWordPageBackgroundCapabilitySetsAndClears`、`TestWordAllParagraphSpacingCapabilityRoundTripsAndNoops`、`TestWordCommentCapabilitiesAddListDeleteRoundTrip` 和 `TestPresentationCommentCapabilitySavesAndReopensWithoutLosingSlideContent`。
- [x] 补齐图片与批注稳定目标的负向 Runtime 回归：Word 图片插入/旋转/缩放/替换/删除以及 Word/PPTX 批注增删遇到未知目标时均失败且 revision/dirty 不变；Word 文本范围剪切也检查无效 run 和范围不会改变状态。新增用例通过 application 与 PPTX 格式层 race 聚焦回归。
- [x] 最终回归收尾（2026-10-11）：`go test ./... -count=1` 通过（退出码 0）；Word 工具栏三项聚焦回归通过 `go test -race ./internal/desktopui -run '^(TestWordToolbar_WatermarkTrackRevisionsEditorAndLayout|TestWordToolbarLayoutPositionUsesEditableInlineImage|TestWordToolbar_RemainingActionsBatch)$' -count=1`。桌面夹具明确覆盖旧式 VML 浮动图的只读禁用状态，并在测试结束前处理完宏检查后台结果。`git diff --check` 通过。WPS 与 Microsoft PowerPoint 的互操作范围分别见前述记录；未实测 Microsoft PowerPoint 的结果仍是限制。
