# 逐页 OCR 校对实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 默认人工校对，并提供逐页、可独立操作的 OCR 识别与字符比对。

**Architecture:** 为当前 PDF 页渲染临时 PNG，复用 OCR 客户端、系统凭据库和 `fed4c5e` 已提交的内容寻址缓存设施。整本 OCR 完成后的缓存继续按原 manifest 契约使用；手动逐页识别仅在同一缓存根目录下增加可校验的页记录，让每个已完成页可恢复，不另建缓存系统。逐字比对直接对当前所选 A/B 页面全文运行 Unicode 字符编辑距离，不以自动定位器找到候选为前提；定位可信度单独展示。对齐序列生成逐字播放帧，B 侧精确高亮、A 侧按本机实测字符框框选；替换在一个播放步中并列显示 A/B 字符。

**Tech Stack:** Go, Gio, `internal/ocr`, `internal/textalign`, 现有 proofreading workspace 状态与凭据缓存。

**Spec:** [2026-10-25-proofreading-page-by-page-design.md](../specs/2026-10-25-proofreading-page-by-page-design.md)

## Global Constraints

- 所有修改、启动和实际界面检查仅在 `/Users/mima1234/project/glance-go`。
- 默认人工模式保留已校对/未校对筛选，AI 识别/比对动作只在用户显式切换后出现。
- Ribbon 从左到右固定为两个全局分页筛选、竖分割线、相邻等宽模式按钮；AI 操作紧邻 AI 按钮，状态提示靠 Ribbon 右侧垂直居中。
- AI 模式保留原整本 OCR 扫描、恢复、取消、重扫、进度、动画和块级联动；逐页识别与逐字比对是新增操作，不能替换旧链路。
- 单页识别只上传当前页；页面切换不自动触发识别或比对。
- 缓存 OCR 页时不缓存 B 对齐结果；B 变化后可重用 OCR，旧比对证据失效。
- 复用 `fed4c5e` 的缓存根目录、PDF 内容哈希、模型/选项哈希、原子 JSON 写入、权限和 OCR layout 序列化；不得改变完整整本缓存的 manifest 语义或再造并行缓存目录。
- 现有整本缓存只有在 manifest 标记完整且页面身份全部匹配时才算命中；它不代表整本任务运行中已有逐页断点。新增逐页缓存用于本次手动识别已完成的页面。
- 复用已提交的 OCR 块到稿件联动与动画；字符差异直接比较选定 A/B 页全文，不能因 `unmatched`、`ambiguous` 或 `limited` 定位状态而不给结果；不确定时明确标成暂定。
- 逐字播放逐帧更新相同、A 多 B 少、B 多 A 少数量和当前字符；替换字符同帧显示 A/B 两字。支持暂停、继续、重播，完成后显示完整分类结果。
- 播放期间 B 页精确标记当前字符，A 页高亮当前 OCR 块；只在差异处从 A 字符框边缘连到 X 字符，连续差异的端点固定在段首字；已扫描的差异连线在完成和滚动后持续保留，颜色区分 A 多、B 多和替换。页面缩放与文本绘制共用页面内坐标变换。macOS 本机补充字符框并保留原块 rune 索引；未定位字符不伪造端点。连线采用页间直角轨道和最少交叉代价，必要交叉直接自然相交。
- 将相同字、B 独有字着色到 X 正文相应字符，A 独有字通过连线标记插入位置；移除 X 正文左侧小字提示；播放控制放在 X 标题栏，不增加浮动层或底部结果区。
- 不修改文稿文字、分页归属或页面校对状态。
- 不新增或运行自动化测试；仅在仓库允许的范围做代码、格式和实际 UI 检查。

## Review Focus

- 空白或无法渲染的 PDF 页：错误要指向当前页，不能上传整本作为回退。
- 重复点击或翻页时存在运行中的请求：结果必须绑定源哈希和请求页，迟到结果不能覆盖另一页。
- 当前 B 页为空、被重新分页或草稿更新：清空比对证据，保留 OCR 页缓存。
- 旧版完整整本缓存：单页读取应验证 manifest、模型、选项与页数，不误读不完整缓存。
- OCR 输出有竖排阅读顺序/空 layout block：保留服务阅读顺序，字符统计对空页给出明确结果。

---

### Task 1: 复用既有 OCR 缓存并增加逐页记录适配

**Files:**
- Modify: `internal/desktopui/proofread_ocr_cache.go`
- Create: `internal/desktopui/proofread_ocr_step.go`
- Modify: `internal/desktopui/proofread_window.go`
- Reuse: `internal/desktopui/proofread_ocr.go` OCR client flow
- Reuse: `internal/desktopui/proofread_ocr_auth.go` credential helpers
- Reuse: `internal/desktopui/proofread_edit.go` proofreading store access

- [ ] 复用既有 `proofreadingOCRCachePaths`、选项哈希、权限受限原子写入及 layout 编解码；优先从完整整本缓存按页读取。
- [ ] 在同一缓存根目录增加逐页结果文件，校验 PDF 哈希、模型、OCR 参数、页码、总页数和渲染 DPI；不改完整整本 manifest，也不把未完成的远程作业描述为可恢复。
- [ ] 在后台渲染当前 PDF 页为 PNG；调用现有 OCR Submit/Wait/Results，复用凭据库存取和令牌弹窗。
- [ ] 仅在单页识别成功后保存 OCR layout；B 草稿与对齐结果不进入 OCR 缓存。已有逐页结果重开后命中缓存，当前未完成页可重试。
- [ ] 将请求页与 generation/source hash 一并绑定，取消、切页或新请求后丢弃迟到结果。

### Task 2: 将工具栏改为人工默认与显式 AI 模式

**Files:**
- Modify: `internal/desktopui/proofread_chrome.go`
- Modify: `internal/desktopui/proofread_layout.go`
- Modify: `internal/desktopui/proofread_ocr_chrome.go`
- Modify: `internal/desktopui/proofread_window.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`

- [ ] 默认显示人工校对状态、原有已校对/未校对筛选和 AI 校对入口；不自动运行识别或比对。
- [ ] 从左到右放置已校对/未校对全局筛选、竖分割线、相邻等宽模式按钮；该导航区在两种模式中保持原位。
- [ ] AI 操作依次紧邻 AI 模式按钮，状态提示在 Ribbon 右端垂直居中；人工模式下不显示 AI 操作按钮。
- [ ] AI 模式继续显示原有整本 OCR 扫描/恢复/取消/重扫按钮，并增加独立的“识别本页”和“逐字比对”动作及返回人工入口。
- [ ] 保留原有整本 OCR 的进度状态、扫描动画、OCR 块联动、首尾锚点和路径选择操作。
- [ ] 显示当前 A 页、当前 B 页、识别/比对状态；切页只更新当前页，不触发后台任务。

### Task 3: 单页双侧字符差异与只读结果

**Files:**
- Create: `internal/desktopui/proofread_ocr_diff.go`
- Create: `internal/desktopui/proofread_ocr_diff_ui.go`
- Modify: `internal/desktopui/proofread_ocr_step.go`
- Modify: `internal/desktopui/proofread_layout.go`
- Modify: `internal/desktopui/proofread_projection.go`
- Modify: `internal/desktopui/proofread_ocr_overlay.go`
- Modify: `internal/desktopui/proofread_chrome.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Modify: `internal/desktopui/proofread_ocr_chrome.go`

- [ ] 把当前 A 页所有可读 OCR 块按阅读顺序拼成全文，与当前 B 页全文独立做字符动态规划；自动定位结果只作为可信度提示，即使 `unmatched` 也允许比较用户选定的页对。
- [ ] 为对齐序列生成逐字符播放帧；相同为绿色，A 独有为红色，B 独有为蓝色；替换在一个帧中并列显示 A/B 字符，并分别计入 A 独有与 B 独有。
- [ ] 在 X 正文字符位置直接显示逐字状态和静态差异类别；在 X 标题栏提供逐字进度、暂停、继续和重播，不增加浮动面板或底部结果区。
- [ ] 字符比对保留标点并忽略空白；所选整页矩阵超出 4,000,000 个动态规划单元时，显式标记不完整，不展示伪完整的部分统计。
- [ ] 播放期间在稿件上精确标出当前字符；页面导航或草稿变化时隐藏失效结果。播放结束后保留最终相同与 B 独有字符标色。
- [ ] A 侧播放时框出本机实测字符，结束后保留差异字框；只绘制实测字框到 X 字符的类别色连线，使用直角轨道，必要交叉直接自然相交。
- [ ] 源文、B 内容、页面归属、generation 或页码变化时清理旧比对结果。

### Task 4: 格式与真实 UI 检查

- [ ] 运行 `git diff --check` 并阅读本次源码 diff。
- [ ] 查找并终止旧 Glance 与 `go run ./cmd/glance` 进程，在授权目录按仓库要求启动全新进程。
- [ ] 在实际窗口确认人工默认、AI 模式按钮分离、翻页不自动扫描、当前页状态更新；不为界面检查提交云端 OCR。
- [ ] 删除本次生成的临时截图/日志，只保留代码与设计文档。
