# 校对逐页比对历史徽标实施计划

> **2026-10-25 实际窗口修正：** 下文首次实施中的“窗口固定”定义已被后续页框边界需求替代。以当前设计稿为准：徽标保存页框内相对位置；正文内部滚动不影响徽标，整页滚动随页移动，离屏后不滞留到下一页。人工/AI 均绘制有效历史的连线和 A/X 差异文字框。后续修复已加入针对性逻辑及 Metal 像素回归，并实际操作源码窗口上下滚动。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在人工与 AI 模式下显示有效的逐页差异编号徽标，并让每个页对的徽标首次定位后固定在窗口坐标。

**Architecture:** 复用 `proofreadingOCRStepState.comparisons` 中已有的逐页结果和页级已处理状态。根级覆盖层遍历有效页对，在页面首次可见时捕获 A/X 页面位置生成固定徽标锚点；滚动只影响页面内容与连接线，不再重算徽标坐标。徽标交互携带自己的页对，点击时打开对应检查项。

**Tech Stack:** Go、Gio、现有 `proofreadingOCRStepState` 与 `proofreadingDiffInspector`。

**Spec:** `docs/superpowers/specs/2026-10-25-proofreading-ocr-history-badges-design.md`

## Global Constraints

- 所有修改留在 `/Users/mima1234/project/glance-go`；不创建、切换、移动或删除 worktree。
- 保留工作区已有的 OCR 溢出未提交改动，不覆盖或清理相关文件。
- 徽标两种模式都显示；页面滚动后保留首次窗口坐标；横坐标使用 A/X 中线。
- 只显示当前 PDF 来源及当前 X 页草稿有效的逐页比较结果。
- 不在 Gio layout/frame 中读取比较缓存；缺少内存历史时，在受管理后台任务中恢复匹配当前草稿的比较和 OCR 页数据。
- 不新增或运行测试；只进行必要的格式化与补丁格式检查。

## Review Focus

- **来源 PDF 改变：**旧页对不得继续绘制；由来源有效性过滤保证。
- **X 草稿改变：**对应页对和徽标应立即失效；由 `proofreadingOCRStepComparisonValid` 约束。
- **滚动任一 A/X 画布或 X 页内部滚动条：**已捕获徽标窗口坐标必须保持不变。
- **人工/AI 模式切换：**不改变历史徽标的集合、编号、颜色或点击目标。
- **多页编号碰撞及历史点击：**徽标维持稳定顺序和最小间距；点击后检查对应的历史页对，而不是当前页对。
- **历史缓存与新草稿：**仅恢复当前 PDF 哈希及对应 X 页草稿哈希都匹配的结果；迟到结果再次校验后才进入 UI 状态。

---

### Task 1: 后台恢复逐页比较历史

**Files:**
- Modify: `internal/desktopui/proofread_ocr_step.go`
- Modify: `internal/desktopui/proofread_ocr_cache.go`

**Interfaces:**
- Consumes: 当前 PDF 来源哈希、`xPages` 草稿快照、现有逐页比较缓存与 `loadProofreadingOCRCachePage`。
- Produces: `proofreadingOCRStepHistoryResult`，包含恢复的 `{APage, BPage, OCR layout, comparison}` 条目；由 `processProofreadingOCRStep` 校验 generation、来源和当前页草稿后写回 `step.pages` / `step.comparisons`。

- [x] 为逐页状态新增独立的历史结果通道和 source generation；仅在有效 source / 页草稿快照准备好后启动一次后台加载。
- [x] 后台按 A/X 同页号构造每页对，使用该 X 页当前草稿哈希读取比较缓存；只有比较命中时再读对应 OCR 页缓存，并在取消时停止遍历。
- [x] UI 消费结果时重新验证 source hash 与 `proofreadingOCRStepComparisonValid`，丢弃过期页；成功项恢复 OCR layout、比较结果和源哈希。缓存读取错误不阻断后续页面。

### Task 2: 绘制窗口固定徽标并路由历史检查

**Files:**
- Modify: `internal/desktopui/proofread_ocr_step.go`
- Modify: `internal/desktopui/proofread_ocr_connector.go`
- Modify: `internal/desktopui/proofread_ocr_inspector.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`

**Interfaces:**
- Consumes: Task 1 恢复的 `step.comparisons` / `step.pages`、`proofreadingOCRStepComparisonValid`、`proofreadingOCRStepConnections`、`step.inspector.pageResolved`。
- Produces: 按 `proofreadingOCRStepPair` 索引的连接、徽标锚点与稳定点击状态；历史徽标点击进入所选页对的 Inspector。

- [x] 为每个有效页对缓存连接分组、差异编号和窗口纵坐标；页面首次进入可视范围时按 A/X 页面几何捕获纵坐标（Dp），之后任何页面或正文滚动均不重算。
- [x] 根级覆盖层按页对稳定顺序遍历所有内存和恢复的有效比较；新结果首次可见时建锚点，source / 草稿失效时清除对应数据。不以 `aiProofreading` 限制显示。
- [x] 对窗口锚点做稳定最小间距排布，编号顺序保持不变；不将徽标强制放在屏幕中心。
- [x] 待处理、当前差异处理中、已处理分别用红、蓝、绿显示，并为徽标提供本地化可访问状态说明。
- [x] 为 Inspector 保存目标页对；点选任一历史编号时从该页对取连接和比较，不依赖当前 A/X 页对。保留确认、忽略和关闭操作。
- [x] 仅解除差异编号徽标覆盖层的 AI 模式门控；其他 AI 专属操作和 OCR 页面叠层不变。
- [x] 对修改过的 Go 文件运行 `gofmt`，再运行 `git diff --check`。

**完成检查：**逐项对照 Review Focus 和设计验收范围做代码审查；按仓库指令不新增或运行测试。

