# Markdown 双屏可调列宽 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 允许用户在 Markdown 双屏模式拖动中线调整编辑区和预览区宽度。

**Architecture:** 在 `markdownViewState` 保存当前视图的列宽比例和拖动手势状态。布局保留 50/50 初始比例，通过独立的窄分割线控件接收水平拖动，并按两侧可用最小宽度限制比例；不持久化到设置或文档。

**Tech Stack:** Go、Gio `gesture.Drag`、Gio `layout.Flex`。

**Spec:** `docs/superpowers/specs/2026-10-23-markdown-split-resizable-design.md`

## Global Constraints

- 所有改动、检查都在 `/Users/mima1234/project/glance-go`。
- 不新增或运行测试；仅运行 `gofmt` 与 `git diff --check`。
- 非双屏模式行为不变，视图重建时比例回到 50/50。

## Review Focus

- 窄窗口和侧栏占宽后，两列仍不会被拖到不可用宽度；比例按当前双屏内容区域计算。
- 指针离开分割线后继续拖动时，列宽仍连续更新。
- Gio 发出 `pointer.Cancel` 时清除拖动状态并恢复拖动前比例。
- 切换离开双屏模式时，分割线不绘制也不拦截编辑器/预览输入。
- 窗口宽度变化后沿用当前比例，同时重新执行列宽限制。

---

### Task 1: Markdown 双屏分割线拖动

**Files:**
- Modify: `internal/desktopui/markdown_state.go` — 保存当前视图比例与稳定拖动状态。
- Modify: `internal/desktopui/markdown_layout.go` — 用可调比例布局编辑器、分割线和预览区。
- Create: `internal/desktopui/markdown_split.go` — 处理 Gio 拖动事件、取消恢复、比例限制和分割线绘制。

**Interfaces:**
- `markdownViewState` 提供比例、`gesture.Drag`、拖动起始位置/比例、拖动状态及最近一次双屏内容宽度。
- `updateMarkdownSplitDrag(gtx layout.Context, v *markdownViewState)` 在 `handleMarkdownEvents` 中、仅双屏模式调用。
- `layoutMarkdownSplitHandle(gtx layout.Context, v *markdownViewState, width int)` 为布局中线注册拖动命中区并绘制细分隔线。
- 初始比例为 `0.5`；可拖范围由当前可用像素宽度和两侧各 `280dp` 最小宽度约束得出。可用宽度不足以容纳两侧最小值时，两侧平均分配且不再允许进一步缩窄。

- [x] **Step 1: 在 `markdownViewState` 增加稳定的比例和手势状态**

初始化比例为 `0.5`，保存拖动起点、起始比例、拖动状态和最后布局的双屏内容宽度；不添加持久化。

- [x] **Step 2: 在独立文件实现拖动事件与分割线**

用 `gesture.Drag` 水平更新比例；按双屏内容宽度把像素增量换成比例增量，限制编辑区和预览区各不小于 `280dp`。按下开始拖动，释放保留当前比例，取消时恢复起始比例并清除拖动态。命中区域加 `pointer.CursorColResize`，绘制与现有边框一致的细线。

- [x] **Step 3: 接入双屏布局**

在双屏 Flex 中为编辑器和预览区使用当前比例权重，中间插入稳定宽度的分割线命中区域；记录扣除分割线后的内容宽度。比例重算只作用于双屏模式。

- [x] **Step 4: 格式和补丁检查**

运行 `gofmt -w internal/desktopui/markdown_state.go internal/desktopui/markdown_layout.go internal/desktopui/markdown_split.go` 与 `git diff --check`。不运行测试或 GUI。
