# Word“保护文档”设计规格

## 状态

**待用户审阅。** 用户已批准保护范围为“打开密码 / 文件级加密”，并选择启用保护时将现有本机 SQLite 工作区快照及版本历史迁移为加密内容、保留全部历史记录。此规格尚未授权实现；通过审阅后再写实施计划。

目标入口是 Word“文件”功能区的“保护文档”。它提供真正的 DOCX 打开密码加密，不实现“审阅 > 限制编辑”一类防误改属性。

## 用户可见行为

1. 选择“保护文档”后，对未加密 DOCX 显示设置密码和确认密码的遮罩输入。二者不一致、密码为空或不符合 Office Agile 密码范围时，不更改文件、会话、工作区或版本历史。
2. 保护已保存的本地 DOCX。启动前先提交当前文档的所有正文草稿；任一草稿提交失败则取消。加密当前编辑器中的完整 DOCX 包，并把源文件替换为 Office 可识别的加密 DOCX。保护成功后文档保持打开且可继续编辑。
3. 打开加密 DOCX 时显示遮罩口令输入。错误口令可重试；验证失败、HMAC 缺失/不匹配、包损坏或算法不支持均不创建可编辑文档，也不覆盖源文件。
4. “新建窗口”继续共享同一文档会话和同一内存中的解锁状态；各窗口的光标、滚动和缩放仍独立。最后一个窗口关闭或进程退出后不持久化口令。退出后恢复该 DOCX 工作区标签时显示锁定状态，必须重新输入口令。
5. 受保护文档的保存、自动保存、另存为和本地“共享副本”继续写入 Agile 加密 DOCX。Save As 使用活动文档密码；目标路径的历史按现有路径隔离规则处理。自动保存不得退化为明文写入。
6. 允许在“保护文档”中更改密码或移除密码。更改密码会用新口令重加密源 DOCX、工作区快照和该路径全部历史快照。移除密码会将这些内容恢复为普通 ZIP DOCX/明文本机快照；确认界面明确说明整个历史时间线也将变为明文。
7. 受保护文档仍可按现有路径导出 PDF 和请求原生打印。PDF 和打印数据是另行生成的未加密输出，不继承 DOCX 打开密码；相关操作在开始前向用户说明输出文件未加密。
8. 所有口令框使用 `widget.Editor.Mask` 遮罩；不得将口令写入日志、SQLite、错误文本、URL、文件名、最近记录、剪贴板或持久化视图状态。口令只存放于共享文档会话内存，窗口/会话结束时尽力清理字节缓冲。Go 字符串及第三方密码 API 的复制意味着不能承诺整个进程内存中彻底擦除。

## 加密格式与兼容性边界

- 输出使用 MS-OFFCRYPTO Agile 加密：AES-256-CBC、SHA-512、随机 salt/key、Office `spinCount=100000` 和 `dataIntegrity` HMAC。加密后的 `.docx` 为含 `EncryptionInfo` 与 `EncryptedPackage` 流的 CFB 容器，内层仍是原有完整 OOXML ZIP 包。
- 代码边界位于 `internal/format/docx`。候选实现为 Spine v0.3.1 的直接 `common/crypto` 严格解密/加密 API 与 `opc.SaveEncrypted` 写入路径，以及 `github.com/richardlehane/mscfb v1.0.9` 读取 CFB 流；Spine 的 `opc.OpenReader` 不可用于安全打开，因为它丢弃 Agile scheme/HMAC 验证结果并接受 Standard/RC4。依赖尚未选定，CFB 读取边界、资源上限和互操作验证仍待审。两者类型不得进入 `internal/document`、workspace API 或 Gio UI。
- 解密路径必须检查 `DecryptResult.Scheme == SchemeAgile` 且 `IntegrityVerified == true`。调用必须使用 strict 默认选项；禁止启用 `AllowMissingDataIntegrity`。Standard AES、RC4 CryptoAPI、Extensible、缺失/半缺失 dataIntegrity、HMAC 不匹配和其他未支持容器一律拒绝，不把“密码 verifier 匹配”误当成内容经过认证。
- DOCX 完整 ZIP 的部件字节由现有适配器解析、保留与序列化。CFB 读取必须使用可检查 Agile scheme 和认证结果的 API，不得通过仅暴露已解密 OPC 对象的接口绕过 scheme 检查；不得自行实现 AES、SHA、HMAC 或 CFB 扇区算法。
- 此功能不启动、不调用、不依赖 LibreOffice、Microsoft Office、其他外部 Office 程序、打印 CLI 或网络服务。`go run ./cmd/desktop` 仍是开发启动入口，构建应用名保持 `desktop`。
- Spine v0.3.1 是 pre-1.0、MIT 许可，声明 Go 1.25.0；mscfb v1.0.9 为 Apache-2.0、声明 Go 1.18。2026-10-04 源码检查与隔离的 Spine `common/crypto`、`opc` 测试通过已记录在 `docs/office-library-evaluation.md`。mscfb 有 sector/cycle 检查并限制部分表分配，但 `New` 不接收总输入上限，目录遍历会递归跟随不可信链接；必须由应用限定输入文件、流和目录条目，并验证畸形/深链输入。CFB 必需流须由单一解析结果无歧义选出，再传入 Spine strict crypto API。可选 `msoffcrypto-tool` 交叉验证当时不可用，Office 生成文档 corpus、CFB 资源上限和畸形容器互操作测试尚未验证。`msoffcrypto-tool` 只可作为开发期验证器，不是运行时依赖。实现计划获批前，需完成这些审查并更新评估；若依赖源审查、项目安全回归或 Office Agile 互操作门槛不通过，不启用按钮。

## 明文、密文与摘要

- 解锁时 `docx.Document` 在进程内持有普通 OOXML 部件，以支持现有编辑和预览；保存到外部 DOCX、SQLite 工作区文档快照和版本历史的受保护内容必须是完整 Office Agile CFB 密文。
- SQLite 中仍可能保留文档路径、标签标题、dirty 状态、视图状态、版本来源、时间、大小等工作区元数据。这些不是内容加密。本地 SQLite 全库加密不属于本功能范围。
- 对受保护版本，SQLite 的 `sha256` 只保存密文摘要；不得写入明文包的 SHA-256、正文片段或明文 ZIP。恢复时先校验数据库保存的密文摘要，再做严格 Agile/HMAC 解密。
- 加密会为相同内容生成不同密文。为保持现有“内容相同只留一个版本”规则，保护文档写入新版本时，在后台读取并严格解密该路径最多 512 MiB 的现存密文版本，比较内存中的明文 SHA-256 后去重；明文摘要只在内存短暂存在。工作区帧/布局线程不得做此操作。
- 启用、改密或移除保护时保留该路径版本的 ID、时间、来源、路径、顺序和可恢复内容。启用/改密时逐条改为新口令保护的 Agile CFB；移除时逐条还原普通 DOCX ZIP。重算数据库 `sha256`/`size_bytes` 以对应变换后的 payload。
- 保留 50 条 / 512 MiB 规则继续约束新产生的历史；仅因 CFB/SQLite 记录开销造成边界超额时，不得在用户批准“保留历史”后静默删掉迁移前已存在的版本。记录有效 package 字节数和 SQLite 记录开销的统计口径，并保证后续新增版本按既有最旧记录策略收敛到上限。
- 启用保护后，当前 `documents.payload` 与历史 `word_versions.payload` 中原有 DOCX 明文应由相应密文替换。不能承诺擦除 SQLite WAL、既有数据库备份、文件系统快照、临时备份或 SSD 的历史物理块；保护成功提示和数据安全说明必须准确表达这一边界。

## 工作区与启动恢复

当前桌面启动时会立即对所有 DOCX 工作区 payload 调用 `docx.OpenSnapshot`，所以加密 payload 必须先变成显式锁定状态，而不能当作损坏 DOCX 丢弃。

- 锁定记录保留原 tab、document ID、路径、视图状态、dirty 状态和排序；UI 允许用户选择该标签并输入口令。
- 口令验证成功后在 worker 中解密、验证 HMAC、调用现有 `docx.OpenSnapshot`，再注册共享文档会话并刷新当前窗口视图。失败不替换锁定记录，其他窗口仍能看到相同锁定状态。
- 没有口令时禁止自动排版、自动保存、版本恢复和 PDF/打印任务读取锁定包；不会清空或覆盖密文 payload。
- 启动若发现保护迁移日志，必须在恢复相关文档和响应保存任务前完成安全的迁移收尾或回滚，并按源文件 hash、事务阶段和日志中的密文数据判断。不能把日志存在与否当成迁移成功的唯一证据。

## 文件与 SQLite 的一致性迁移

外部文件系统替换与 SQLite 事务无法组成同一原子事务。Protect、改密和移除密码都使用可恢复 journal，避免崩溃后留下用户无法判断的半完成状态。

1. 对当前共享文档会话取得串行操作权；flush 所有窗口草稿；捕获一致 revision 的完整 DOCX 包、源文件原字节、工作区文档快照和当前路径全部版本记录。
2. 在后台校验源包、加密/解密所有目标包，预先完成保存-重开-完整部件比对和密文摘要。为源文件写同目录临时文件并 `fsync`。
3. 在独立 SQLite 事务中持久化迁移 journal：操作 ID、文档/规范化路径键、操作类型、源文件旧/新摘要、阶段，以及足以完成目标 DB 更新的变换后密文数据。journal payload 自身不得包含明文 DOCX 或密码。现有明文行在此时仍原样可恢复。
4. 通过现有安全文件替换助手原子替换源 DOCX；替换前复核源文件仍匹配捕获的旧摘要，避免覆盖用户外部修改。
5. 在一次 SQLite 事务中把工作区文档快照和该路径所有版本 payload、摘要、大小一起切换到目标状态，写入新保护状态元数据（若 schema 需要），最后删除 journal。
6. 仅在源文件与 SQLite 更新均确认成功后把共享 session 的口令/保护状态切换为新状态，释放文档操作锁并重新发布页面结果。

失败与崩溃规则：

- 草稿提交、DOCX 解析、加/解密、密码验证、源文件版本检查或临时写入失败时，不写 journal，不替换源文件，不改动 SQLite/历史，也不清除 dirty 状态。
- 源替换前崩溃时，源文件和主数据库仍是旧状态；启动可清理无 journal 指向的临时文件。
- journal 已提交但源文件仍匹配旧摘要时，可安全回滚并删除 journal，也可完整重试迁移；不能把半转换的 DB payload 应用到会话。
- 源文件已匹配 journal 的新摘要但数据库仍旧时，启动依据 journal 完成 DB 事务；在此期间锁定该文档及冲突操作。源文件既不匹配旧摘要也不匹配新摘要时，保留 journal 和原 DB 行、阻止覆盖，展示可恢复错误。
- DB 切换失败时不能谎报 Protect 成功或清除 dirty 状态。当前进程保留可重试状态；重启按 journal 收尾。源文件已经加密但 DB 还旧时不可允许退出流程删除 journal。
- 操作最终失败且源文件尚未替换时，整条历史和工作区明文留在原位；操作日志/错误信息不包含密码、正文或密文内容。

## 关键集成路径

- **普通打开：** 先识别 CFB/OOXML 加密包装；加密 DOCX 先进入口令提示，再调用 docx 适配器做 Agile scheme、HMAC 和 ZIP 校验。普通未加密 DOCX 保留现有流程。
- **保存与自动保存：** 保持当前后台保存、临时 sibling 文件、revision guard 和 dirty 语义；受保护会话传入口令完成加密，落盘内容与返回给工作区/版本历史的实际写入快照完全一致。
- **另存为与共享副本：** 以同一解锁会话产生新密文；同源共享目标仍拒绝覆盖。未加密 DOCX 的既有行为不变。
- **版本历史：** 列表仍只查元数据；选择版本后在 worker 校验密文摘要、严格解密和解析。恢复前保护版本及恢复选中版本均保持当前保护模式/口令；整包恢复仍是一项可撤销的 Word 编辑。
- **PDF/打印：** 只消费当前已解锁内存文档产生的布局场景；任何锁定状态不得开始任务。操作确认清楚指出输出不受 DOCX 口令保护。
- **共享多窗口会话：** 密码与保护状态属于 shared session；不属于窗口 view state，不跨进程写入。等待中的文件、PDF、保存与版本恢复操作遵守 session operation lock。

## 错误信息

- 将“口令错误”和“文件已损坏/完整性验证失败/算法不支持”区分显示；前者允许重试，后三者提示文件可能已损坏或当前版本不支持且拒绝打开。
- 密码创建错误说明 Office 互操作长度限制；不打印用户输入。
- 数据迁移失败区分“源文件尚未改变”“源文件已经加密、工作区迁移待收尾”和“检测到源文件被外部修改”，明确阻止继续写入会冲突的版本。
- SQLite 历史迁移失败不可吞掉版本条目、改报普通保存失败或错误地声称无历史。

## 不在本规格范围

- `w:documentProtection`、限制编辑、文档 IRM、证书加密和权限管理。
- Standard/RC4/Extensible 加密包的自动降级读取或写出。
- 把密码保存在钥匙串、系统凭据库、配置文件或工作区数据库。
- 为 PDF、打印机输出、系统剪贴板或屏幕截图增加单独的内容密码层。
- 对旧 SQLite WAL、备份、SSD、APFS 快照作安全擦除承诺。
- 对 DOCX 其他尚未支持的 OOXML 部件作超出当前适配器保存/重开边界的内容编辑；加密前后的 package parts 必须可按现有适配器原样保留。

## 验收条件

- 普通 DOCX 可通过保护对话框输出 Agile CFB 文件；使用同一口令重开后完整 DOCX package bytes/parts、已编辑文字、未知部件和文档 dirty/save 行为正确。
- 错误口令、篡改 `EncryptedPackage`、篡改/删除/半删除 `dataIntegrity`、截断 CFB、恶意 `spinCount`、unsupported scheme 均 fail closed；源文件和会话快照逐字节不变。
- 任何 Agile 解密结果必须明确报告 `SchemeAgile` 与 `IntegrityVerified=true`；Standard/RC4 即使密码正确也拒绝打开。
- 保存、自动保存、另存为、共享副本和应用退出恢复都保持外部 DOCX 与 SQLite payload 加密；可用工具检查工作区/历史 BLOB 中不存在 DOCX ZIP 明文签名或正文标记。
- 保护前创建的版本 ID、时间、来源、顺序和每份解密内容在迁移后保留；旧历史可以列出和恢复。改密、移除、取消、磁盘失败、DB 失败及进程重启 journal 收尾均验证数据无丢失。
- 相同明文内容在受保护历史中仍去重，且 SQLite 不含这些明文摘要；最多 512 MiB 历史时去重扫描在 worker 中进行。
- 多窗口共享编辑、密码状态、锁定/解锁、关闭窗口和最后窗口退出符合共享会话生命周期；日志和 SQLite 中均不出现密码。
- 过期 revision、外部文件被替换、临时/SQLite 写入失败、损坏历史、自动保存/版本并发、窗口关闭期间 worker 完成时都不覆盖错误版本或丢 dirty state。
- 运行入口仍为 `go run ./cmd/desktop`；运行时无 LibreOffice/其他 Office 进程与服务依赖。
- 更新 `docs/office-library-evaluation.md`、`docs/office-support.md`、中文 Word TODO 与 compatibility matrix，注明 Agile-only 支持边界和不可物理擦除的本地存储限制。
