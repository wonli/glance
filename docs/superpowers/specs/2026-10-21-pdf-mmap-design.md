# PDF 大文件的文件映射设计

## 目标

降低大 PDF 从工作区恢复时的 Go heap 峰值，同时保留现有 SQLite 快照作为唯一可恢复的数据来源。对于只读浏览和滚动，PDF 包字节不应被完整读入 Go heap；页面仍由现有 `gopdf.Reader` 解析和渲染。关闭文档时释放映射并清理临时 backing。

## 现状与问题

工作区的 `documents.payload` 保存完整 PDF BLOB。`Collection.Get` 通过 `Store.LoadDocumentPayload` 把它扫描成一个完整 `[]byte`，集合又会缓存该 slice。恢复 PDF 时再把 payload 转交给 `pdfdoc.Document`。此前 pprof 的 in-use heap 中，约 207 MiB 来自 `LoadDocumentPayload` 的 `bytes.Clone` 调用链；这是恢复快照的整包副本，与当前页渲染缓存不同。

PDF adapter 使用的 `gopdf.Reader` 保留输入字节 slice。适配器本身以不可变 PDF revision 工作；编辑操作会序列化成新的 revision。因而可以让 reader 直接引用只读文件映射，并将映射生命周期绑定到 `pdfdoc.Document`，无需改第三方解析库。

## 设计

### 1. 有界内存构造稳定快照

- 新增一个 PDF 快照打开入口，接受向 `io.Writer` 写入完整快照的回调。该入口创建权限为 `0600` 的临时文件，回调成功后关闭文件，再对其做只读映射。
- macOS 和 Linux 使用标准库 syscall mmap 实现；映射为只读，输入文件不会在映射后被写入。映射完成后立即 unlink 临时路径，文件内容仅由映射持有；失败路径关闭并删除临时文件。
- 不支持 mmap 的目标使用兼容实现，将流读取到 Go 内存后打开。若 mmap 在支持平台上因系统错误失败，返回错误并清理资源；不静默退回整包 heap 分配。
- `pdfdoc.Open(path)` 也通过此入口把源文件复制到受控临时快照后再映射。这样保留当前 `os.ReadFile` 的稳定快照语义：源文件随后被外部修改、替换或被 Glance 保存时，当前 reader 不会读到源文件的新内容。
- 零长度、超出当前进程可寻址 slice 长度、写入/关闭/映射失败均在构造 reader 前报错并回收资源。SQLite 读取失败或磁盘空间不足时不更改工作区数据库。

### 2. SQLite BLOB 分块流出

- `Store` 提供按 document ID 将 payload 写入 `io.Writer` 的接口，不返回完整 `[]byte`。
- 使用 SQLite 增量 BLOB API，而不是 `SELECT payload` 或 `substr(payload, ...)`：创建只读 SQLite 连接，在同一 read transaction 内按 ID 查询 rowid，再通过 [`sqlite3_blob_open`](https://www3.sqlite.org/c3ref/blob_open.html) / [`sqlite3_blob_read`](https://www.sqlite.org/c3ref/blob_read.html) 每次读取固定 1 MiB。SQLite 文档说明增量接口把指定字节段写入调用方缓冲；[`substr`](https://www.sqlite.org/lang_corefunc.html) 则把 BLOB 值作为函数输入，不能作为有界内存保证。
- 连接使用 `Store` 打开时保存的数据库文件路径；参数化查询 rowid 后，在同一个 SQLite 连接和 transaction 中打开 BLOB handle，确保并发更新时仍读取一个一致快照。对非文件数据库或不支持增量读取的 DSN 明确报错，不退回整包分配。
- 每个 chunk 写入临时文件后立即释放；写入必须完整，数据库异常、短写、transaction commit 失败都使整个打开失败，临时文件被移除。
- 不改 SQLite schema，不压缩、重编码或规范化 payload。复制出的字节必须逐字节等于 transaction 开始时读到的 `documents.payload`。

### 3. 映射所有权与 PDF 生命周期

- `pdfdoc.Document` 持有 backing owner；`gopdf.Reader` 收到 owner 暴露的只读 byte slice。
- `Document.Close` 在现有互斥锁内先清除 reader、当前/保存 revision、undo/redo 引用，然后解除映射。重复关闭安全；解除映射或清理错误通过 `Close` 返回。
- `OpenSnapshot` 和 `OpenSnapshotOwned` 保持现有调用语义。普通内存快照不转为 mmap；新增映射入口只用于源路径与工作区大快照。
- 修改后的 PDF revision 仍由当前 serializer 生成并存于 heap；原始映射保持到文档关闭，以支持保存基线及 undo 回到原 revision。页面解析、图片解码和渲染缓存本身不在本次替换范围内。

### 4. 工作区恢复不再水合完整 payload

- 增加只返回 tab/document 元数据和 view state 的 Collection 读取方法。保留现有 `Get` / `Tabs` 的 payload 语义，避免破坏其他格式的调用方。
- 元数据包含 payload 是否存在/长度信息（由 SQLite `length(payload)` 查询，不扫描 BLOB），用来区分“有工作区快照”与“需要从 SourcePath 恢复”，避免为此重新载入完整 PDF。
- PDF 恢复流程先读取元数据，再通过 Store 的分块流接口和新的 PDF 快照打开入口构造映射。PDF 路径不调用会载入 payload 的 `Collection.Get`。
- 恢复过程在现有后台文档打开/恢复 worker 中完成，完成后通过已有 UI 结果通道发布。切换或关闭标签后到达的迟到结果立即关闭，确保 mmap 不泄漏；Gio frame/layout 不执行 BLOB 分块、文件写入或 PDF 解析。
- 激活、持久化 view state 和其他只需要 tab 元数据的 PDF 路径统一使用 metadata-only API。其他格式继续按现有逻辑载入其 payload。

### 5. 工作区持久化和 payload 缓存

- SQLite 仍是恢复快照的权威副本。干净且已持久化的 PDF revision 只更新 dirty/view state 等元数据，不为滚动、缩放或普通标签切换重新克隆和写入整份 PDF。
- 初次从源路径打开且尚无工作区 payload 时，仍必须先写入一次完整快照；若写入失败，保持原有错误状态，不将 revision 标成已持久化。
- PDF 内容编辑、旋转、批注、撤销/重做产生新 revision 后，持久化该 revision 的完整字节；只有 SQLite 写入成功后才推进“已持久化 revision”。失败时保留文档 dirty 状态并允许重试。
- PDF 的完整 payload 不长期保留在 `Collection.Session.Payload`。成功写入后只更新集合元数据并清空 payload 缓存；metadata-only 查询不得重新加载它。非 PDF 格式当前的缓存行为不变。
- PDF 专用持久化路径先读取轻量、原子发布的 `Document.Info().Revision`，与共享会话中成功落库的 revision 比较。revision 相同则只更新 view state/metadata，不调用快照 API；revision 不同才在 adapter 读锁内调用 `SnapshotWithRevision()`，取得同一 revision 的完整副本并写库。因而只读恢复和 clean view-state 更新不产生完整 Go heap 副本；一次确有内容更新的 checkpoint 最多额外产生一个完整快照副本，且不会为 Collection 缓存再克隆一份。
- 保存到源文件或另存为的原有原子写入路径不变；SQLite 工作区 snapshot 与用户源文件是两个不同的持久化目标。

## 接口边界

- `internal/workspace.Store` 负责在单个 SQLite read transaction 中分块读取现有 BLOB，并将字节写入调用方提供的 writer。
- `internal/workspace.Collection` 负责提供 metadata-only 会话访问，以及 PDF payload 写成功后的非缓存更新路径。
- `internal/format/pdfdoc` 负责临时快照文件、平台 mmap/fallback、PDF Reader 构造和 backing 释放。
- `internal/desktopui` 负责把 PDF 工作区恢复放入后台任务，并在 UI 线程接受成功/失败结果和关闭失效结果。
- 不新增第三方依赖、不改数据库版本、不把 `gopdf` 类型泄露到 UI/workspace。

## 失败与资源规则

- temp 创建、BLOB 读取、chunk 写入、transaction、映射、PDF 校验任一步失败都不会替换现有活动文档；失败对象释放已创建的文件和映射。
- mmap 成功后的 reader 只读访问映射内容。`Document.Close` 与 render/page/mutation 共享同一把锁，解除映射不会与 reader 使用并发。
- 已映射的临时文件在 Unix 上立即 unlink；打开失败路径也删除文件。未完成 unlink 的崩溃残留只会位于系统 temp 目录，由系统临时目录清理策略处理；本次不添加跨进程的主动清扫器。
- unlink 只移除目录项；映射存活期间 backing 文件数据仍需占用约一份 PDF 的临时磁盘空间，解除映射后空间才释放。磁盘空间不足时打开失败，不影响 SQLite 快照或源文件。
- 映射将大文件字节从 Go heap 移到文件映射区，不保证 macOS 进程 footprint 按文件大小等量下降。访问过的映射页仍可能计入 resident/footprint，渲染产生的图像、CoreGraphics/Metal 资源和 gopdf 内部缓存也仍占内存。

## 验收标准

1. 用 SQLite 中的合成 PDF payload 打开后，`sqlite3_blob_read` 按固定块生成的映射字节与原 payload 完全一致；测试覆盖多块、末尾不足一块、短写、读取失败和并发替换 payload。
2. 从工作区恢复大 PDF 时，调用链不再执行整包 `LoadDocumentPayload` / `bytes.Clone`；对应 BLOB 读取的单块 Go 分配不超过 1 MiB 加 SQL/文件缓冲开销。
3. 已持久化且未编辑的 PDF 只更新 view state/metadata，不重新序列化或写完整 payload；新 revision 成功写库后可关闭并从 SQLite 恢复到相同字节。数据库写入失败时 dirty 与重试状态不丢失。
4. 页面渲染、搜索、编辑、undo/redo、保存、关闭、重新打开均保留现有结果；close 后映射确实解除，临时路径不存在。
5. PDF restore、源文件打开与标签切换时，Gio frame/layout 不承担分块复制或 parser 工作；迟到的后台结果关闭后不泄漏映射。
6. macOS mmap 路径可构建运行；不支持 mmap 的目标可构建并走兼容实现；不增加 workspace schema migration。

## 不在本次范围内

- 改造 gopdf 的解析算法或其内部对象缓存。
- 页面像素缓存、缩略图质量、GPU 资源、并发渲染队列或 PDF worker 数量调整。
- 更改 PDF 保存格式、压缩 PDF、改写 SQLite schema、删除 workspace 快照或降低恢复能力。
- 为所有 Office 格式引入 mmap/流式解析。

## 风险与验证重点

- mmap 生命周期若与 `gopdf.Reader` 或 undo slice 的引用边界不一致会造成崩溃；close 必须同步并在解除映射前清除所有引用。
- SQLite BLOB offset、transaction 边界或 BLOB 更新期间若处理错误，会产生混合快照；需验证并发更新时逐字节稳定。
- 初始源文件打开、脏 PDF 持久化、保存后 clean 状态和多窗口共享会话必须覆盖，否则仅恢复路径成功不足以证明可恢复性。
- mmap 优化主要减少 Go heap 中完整 payload 副本；整体 footprint 改善需要在同一 PDF、同一工作负载下同时观察 heap profile 与 macOS footprint，不能由 mmap 本身推断。
