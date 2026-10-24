# 校对 OCR 任务恢复与空白页补入实施计划

> 实施前先由用户审阅本计划并选择执行方式。所有步骤均在当前授权源码目录完成；不创建 worktree。

**目标：** 为校对工作台加入可跨重启恢复的整本 PaddleOCR 任务、系统凭据库令牌复用，以及“PDF 有 OCR 内容、B 有唯一对应空页”时的预览确认填入。

**架构：** OCR 适配器继续处理整本上传、按 job ID 轮询和下载结果。项目 JSON 恢复负载新增向后兼容的任务元数据；全局 workspace preference 只保存系统凭据库的 opaque reference 和 endpoint。异步持久化确认后再进入可恢复轮询。缺页映射由纯 helper 根据 textalign 结果和当前 B 页模型保守计算；UI 只通过既有 proofreading controller 修改 B。

**技术栈：** Go、Gio、现有 PaddleOCR 适配器、SQLite workspace preference / proofreading payload、`agentsettings.SystemCredentialStore`、现有 proofread model/controller。

**设计：** [2026-10-24-proofreading-ocr-resume-and-missing-pages-design.md](../specs/2026-10-24-proofreading-ocr-resume-and-missing-pages-design.md)

## 约束和审查重点

- 仅在 `/Users/mima1234/project/glance-go` 编辑、构建、测试和启动；不新增 worktree、构建缓存、重复依赖缓存或应用副本。
- 令牌只进操作系统凭据库和短时内存；日志、SQLite payload、草稿文件、OCR 结果均不得含令牌。
- keyring 读写、PDF 读取、网络、OCR 对齐和项目保存都在后台完成，不在 Gio layout/frame 中阻塞。
- job ID 要先可靠持久化再开始恢复轮询；异步旧草稿保存不得覆盖或清除新 job 元数据。
- 恢复必须同时校验 A PDF 源 hash、B 源文件 hash 和本次对齐所用 B 草稿快照 hash；任何一项变化都不能把旧结果应用到当前文稿。
- 只有 OCR 有正文、对齐状态允许、并且匹配邻页唯一夹定的一个空 B 页时才出现填入候选。`ambiguous`、`limited`、多空页和不确定页界都不能自动选页。
- 确认前只读预览；确认时再次校验目标页为空和快照 hash。走正常 undo/dirty/autosave 路径，不标记已校对。
- 完成后按仓库 `AGENTS.md` 清理旧 Glance/`go run` 进程，并从当前源码目录执行 `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance` 做真实 UI 检查；明确区分测试通过与实际窗口观察。

## 实施任务

### Task 1：定义安全凭据与 OCR 恢复记录

**文件：**

- 新建：`internal/desktopui/proofread_ocr_auth.go`
- 新建：`internal/desktopui/proofread_ocr_auth_test.go`
- 修改：`internal/desktopui/proofread_project_persistence.go`
- 修改：`internal/desktopui/proofread_project_persistence_test.go`
- 修改：`internal/desktopui/proofread_window.go`

**接口与状态：**

- 为 OCR 认证存储定义可替换的 `Get/Put/Delete` 接口，生产实现复用 `agentsettings.SystemCredentialStore`，测试用内存 fake。
- 定义 OCR 全局认证偏好结构：endpoint 与随机 opaque credential reference；不得有 token 字段。
- 定义 `proofreadingOCRJobRecord`：job ID、endpoint、model、A 源 PDF hash、B 源文件 hash、B 草稿快照 hash、创建时间和服务任务阶段。`proofreadingProjectPayload` 增加 `omitempty` 字段，旧 payload 仍可解码。
- 为 B 草稿快照计算独立 SHA-256：`sourceBHash` 标识所选文件，草稿 hash 标识实际传给 `textalign.Align` 的文本，二者不可混用。
- OCR job 元数据纳入现有项目异步保存快照；为“job ID 写入成功”建立可观察的保存确认/失败结果。保存失败时允许当前窗口继续处理，但状态明确提示关窗无法恢复。

**步骤：**

1. 先为认证偏好编码/解码、空 ref、endpoint 不匹配、旧偏好、payload 旧版本兼容、job record 往返和“序列化数据无 token”添加失败测试。
2. 运行 `go test ./internal/desktopui -run 'TestProofreadingOCR(AuthPreference|JobRecord|ProjectPayload)' -count=1 -v`，确认失败来自缺少预期行为。
3. 实现纯数据结构、哈希 helper、凭据 ref 生成和恢复 payload 接口；在后台 helper 中执行 credential store 与 workspace preference 的先后顺序。首次保存顺序为 Put credential → SetPreference；偏好失败则删除新 credential 并终止上传。轮换顺序为 Put 新 credential → 更新 preference → 删除旧 credential。
4. 在项目草稿持久化中保留 OCR job 字段，避免过期草稿快照覆盖它；实现 job 持久化确认与错误回传。
5. 重跑上述针对性测试，并增加回归：旧草稿保存晚完成时仍保留最新 job ID，失败写入不会被报告为可恢复。

### Task 2：复用凭据、提交后可靠保存并恢复原 job

**文件：**

- 修改：`internal/desktopui/proofread_ocr.go`
- 修改：`internal/desktopui/proofread_ocr_chrome.go`
- 修改：`internal/desktopui/proofread_window.go`
- 修改：`internal/desktopui/proofread_edit.go`
- 修改：`internal/ocr/paddle.go`（仅当需要稳定区分 401/403 认证失败时）
- 修改：`internal/desktopui/proofread_ocr_test.go`
- 修改：`internal/desktopui/proofread_project_persistence_test.go`
- 修改：`internal/i18n/locales/zh-CN.json`
- 修改：`internal/i18n/locales/en-US.json`

**行为：**

- 点击扫描后在后台查找与当前 endpoint 对应的已存令牌；找到就直接继续，不弹令牌输入框。无令牌或 endpoint 不一致时才弹掩码输入框。
- 首次输入成功保存后复用该 token；服务明确拒绝认证时保留 job ID，并提供替换 token 流程。网络故障不要求重输 token，也不丢失恢复任务。
- 令牌 UI 文案说明只在首次设置或服务拒绝认证时输入，之后由系统凭据库保存并自动使用；仅在确认上传时开始发送 PDF。
- Submit 返回 job ID 后先将恢复记录交给 UI state，并等项目保存结果；成功后才开始按同一 job ID 轮询。若保存失败则本次继续处理，同时显示恢复风险状态。
- 关闭/取消只停止本地轮询；保留服务端 job record。重开项目并完成草稿恢复后，点击“继续 OCR”从同 endpoint keyring 取 token，验证 A/B 源和草稿哈希后调用 Status/Wait/Results，不重新 Submit。
- 当前 OCR 的长任务取消与重开生成号隔离；过期 goroutine 和迟到的保存结果不能覆盖新任务。

**步骤：**

1. 添加失败回归：同 endpoint 自动取 key、不触发输入弹窗；keyring 不可用/缺失时转输入；偏好写失败不上传；401/403 保留 job ID 并可替换 token；网络错误保留任务且不要求替换；服务端完成后恢复路径不调用 Submit。
2. 运行 `go test ./internal/desktopui -run 'TestProofreadingOCR(Credential|Resume|JobPersistence|AuthFailure)' -count=1 -v`，确认新行为失败。
3. 将 keyring 访问移到 managed background worker；在结果 channel 中传递不含 token 的认证/恢复状态，令牌使用后清空引用。实现提交后持久化确认再 Wait 的握手。
4. 把恢复入口接入草稿恢复结果处理；哈希任一不符时保留可检查的旧 job 记录但拒绝继续应用，提示重新扫描。
5. 更新中英文令牌与“继续 OCR/恢复失败/凭据替换”文案，确保同源 key 对齐。
6. 重跑 OCR 凭据、恢复和既有 proofread OCR focused tests；检查测试日志和 JSON 序列化内容不出现合成 token。

### Task 3：保守计算唯一空 B 页候选

**文件：**

- 新建：`internal/desktopui/proofread_ocr_missing_page.go`
- 新建：`internal/desktopui/proofread_ocr_missing_page_test.go`
- 修改：`internal/desktopui/proofread_ocr.go`
- 修改：`internal/desktopui/proofread_window.go`

**接口与规则：**

- 产生 OCR A 页到空 B 页的候选映射，输入只包括对齐结果、OCR layout 和 B 的 page ownership/model 快照。
- 候选条件：A 页至少一个可用正文块；该页没有可用匹配结果且状态不是 `ambiguous`/`limited`；前后 matched A 页可用其 B 页面边界夹出唯一、连续且为空的 B 页。
- 空页由当前 proof model 页面内容判断；不能因文本匹配不完整、多个空页、页界包络、序号空缺或仅靠物理页码相同而猜测。
- 文本预览按 OCR 块的阅读顺序拼接，保留原文，剔除空块；不把 `textalign` 的近似匹配范围伪装成精确页界。

**步骤：**

1. 添加纯函数失败测试：左右 matched 锚点唯一夹定空页、文稿首尾的唯一边界、连续多个空页、多候选、无文本、matched/ambiguous/limited 状态、不同页数、中文竖排块顺序。
2. 运行 `go test ./internal/desktopui -run '^TestProofreadingOCRMissingPage' -count=1 -v`，确认候选规则未实现时失败。
3. 实现纯候选 helper；候选只存 A 页、B 页、预览文本、扫描哈希和 B 页范围指纹，不依赖布局控件；确认时用指纹拒绝页界变化后的旧映射。
4. 将候选结果挂到 OCR 页状态；B 文稿一变即清除/重算候选，并确认候选不会让页面自动变为已校对。
5. 重跑缺页映射测试与既有 textalign/OCR 对齐回归。

### Task 4：缺文颜色、预览确认与常规编辑路径

**文件：**

- 修改：`internal/desktopui/proofread_ocr_chrome.go`
- 修改：`internal/desktopui/proofread_ocr_overlay.go`
- 修改：`internal/desktopui/proofread_projection.go`
- 修改：`internal/desktopui/proofread_pagination.go`
- 修改：`internal/desktopui/proofread_edit.go`
- 修改：`internal/desktopui/proofread_window.go`
- 修改：`internal/desktopui/proofread_ocr_test.go`
- 修改：`internal/i18n/locales/zh-CN.json`
- 修改：`internal/i18n/locales/en-US.json`

**行为：**

- 唯一补入候选在对应 X 页面顶部状态和页码导航显示独立的缺文颜色；无候选的 OCR matched/ambiguous/limited 状态沿用当前颜色。
- 点击 X 状态或清晰的“查看 OCR”入口打开持久 UI state 的预览对话框，展示 A 页、目标 B 页、OCR 原文和“填入/取消”。
- “填入”前重新检查源 A hash、B 源 hash、当前 B 草稿快照 hash 和目标 B 页空状态；任一条件改变就拒绝写入并刷新候选。
- 确认后调用现有 `Model.ReplacePage` 与 `proofreadingModelChanged(false)`，复用撤销、脏状态和自动保存；不绕过 controller、不改其它 B 页、不标已校对。

**步骤：**

1. 添加交互/状态失败测试：只有有候选时可开预览，取消不改 model；确认只替换目标页；已非空目标、过期 B hash 或 A/B 源变更都拒绝；撤销恢复空页。
2. 运行 `go test ./internal/desktopui -run 'TestProofreadingOCR(MissingPagePreview|ApplyMissingPage)' -count=1 -v`，确认缺少交互时失败。
3. 增加稳定的预览 dialog/button state、状态标色和确认 handler；所有写操作经过 proof model/controller 的既有页面编辑路径。
4. 更新中英文候选、预览、填入、过期和取消文案。
5. 重跑缺页交互、proofreading model undo/autosave 与 X 页面状态 focused tests。

### Task 5：定向回归与当前源码 UI 验证

**文件：**

- 验证：OCR 认证、项目恢复、空页候选和确认流程的新增/既有 focused tests。
- 验证：两份 locale JSON 格式、键集合与插值参数一致。
- 验证：设计文档、本计划与最终 diff。

**步骤：**

1. 在当前目录执行最小必要测试：`go test ./internal/ocr ./internal/workspace ./internal/desktopui -run 'Test(ProofreadingOCR|ProofreadingProject)' -count=1`；再按失败点补跑具体测试，不反复全量构建。
2. 执行 locale JSON 解析与 `git diff --check`；检查 SQLite payload、草稿文件、测试错误和日志都没有 token，确认旧版 payload 仍可恢复。
3. 按仓库规则先查找并结束已运行的 Glance 应用与 `go run ./cmd/glance` 启动器，再从本目录运行 `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance`。
4. 实际窗口操作：首次输入后确认 PDF 才上传；关窗再开后显示继续任务并保持服务端进度；完成后检查唯一空页颜色、OCR 预览、取消无写入、确认填入和撤销。没有实际看到的 UI 状态必须明确记录为未验证。
5. 停止本次验证进程；不删除用户数据/缓存，不遗留临时截图、日志、测试文件或二进制；最后复查 diff。

## 交付检查

- 首次输入后同 endpoint 自动复用 keychain token；不同 endpoint 不会误发旧 token。
- job ID 在开始轮询前已确认写入恢复 payload；关闭重开不会重复上传。
- 源或 B 草稿快照改变时不复用旧结果。
- 不确定页不出现可执行填入动作；唯一空页必须预览并由用户确认。
- 填入走标准编辑/撤销/自动保存，未标为已校对。
- 报告 focused tests 和真实 UI 检查的各自结果，不用编译或单测代替实际窗口验证。
