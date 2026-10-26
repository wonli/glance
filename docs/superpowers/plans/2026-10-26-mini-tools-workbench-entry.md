# Mini Tools Workbench Entry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the home **小工具** page with one **文档工作台** card that opens the proofreading task list without requiring an open PDF.

**Architecture:** Add a home navigation section and a focused Gio page for the single card. Add a workbench-only request to the existing proofreading module window queue; it opens or raises that module on its task list while source-backed PDF requests continue through their current tab path.

**Tech Stack:** Go, Gio, the existing desktop UI layout helpers, and embedded `zh-CN` / `en-US` localization catalogs.

**Spec:** `docs/superpowers/specs/2026-10-26-mini-tools-workbench-entry-design.md`

## Global Constraints

- Work only in `/Users/mima1234/project/glance-go`; do not create or switch Git worktrees.
- Add only the **文档工作台** tool card in this pass.
- Do not require or create a source PDF to open the workbench task list.
- Preserve the existing PDF-to-proofreading request and source snapshot lifecycle.
- Keep filesystem, database, and parsing work outside Gio layout/frame code.
- Do not edit `internal/desktopui/proofread_workbench_layout.go`; it contains an existing uncommitted user change.
- Implement the screen in Glance; do not read or modify the Pen file outside Pen MCP.
- Do not add or run tests, builds, or GUI verification unless the user asks for verification.

## Review Focus

These conditions need code review attention; this plan adds no tests under the current instruction:

1. A workbench-only request is the only accepted request shape without a source snapshot; cleanup paths must handle its nil snapshot safely.
2. Opening the card while the proofreading module is already showing a manuscript returns to the task list and preserves the manuscript tab.
3. Existing PDF-triggered launches still carry and transfer their snapshot as before.
4. A full request queue or application shutdown reports failure without dereferencing a nil snapshot.
5. The single card stays legible at narrow window widths and all new text has Chinese and English catalog entries.
6. Search and category changes update both the count and card list, including empty categories and zero-result queries.
7. The heading search and category tabs remain within the content bounds at narrow widths.

---

### Task 1: Add a workbench-only module request

**Files:**
- Modify: `internal/desktopui/proofread_window.go`
- Modify: `internal/desktopui/proofread_module_window.go`
- Modify: `internal/desktopui/proofread_tabs.go`

**Interfaces:**
- Add `workbenchOnly bool` to `proofreadingWindowRequest`.
- The module queue accepts a request without `sourceSnapshot` only when `workbenchOnly` is true.
- A `workbenchOnly` request must not carry a source snapshot; reject and close an invalid attached snapshot.
- A workbench-only request raises the module window, selects the workbench route (`activeTabID = "recent"`), and clears the selected proofreading tab without creating a document tab.

- [x] Add the request flag and narrowly allow the no-snapshot workbench request in `enqueueProofreadingModuleRequest`; reject and close any snapshot attached to that request shape.
- [x] Guard snapshot cleanup in shutdown and queue-full paths so a nil snapshot is safe.
- [x] Branch in `processProofreadingTabs` to select the workbench for `workbenchOnly`; keep source-backed requests on `openProofreadingTab`.
- [x] Run `gofmt` on the three modified Go files.

### Task 2: Add the single-entry home tools page

**Files:**
- Modify: `internal/desktopui/home_layout.go`
- Modify: `internal/desktopui/settings_navigation.go`
- Create: `internal/desktopui/home_tools.go`
- Create: `internal/desktopui/assets/icons/home/blocks.svg`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`

**Interfaces:**
- Add `homeSectionTools` to the home route enum and a stable navigation button id `tools`.
- Add `layoutHomeToolsPage(layout.Context) layout.Dimensions` as the section body.
- The card action calls `ensureDesktopRuntime().openProofreadingModuleWindow(proofreadingWindowRequest{workbenchOnly: true})` and reports launch errors through the existing toast path.

- [x] Add the **小工具** sidebar row after **AI 助手** and before **设置**, and process its click before selecting the page layout.
- [x] Add the design's Lucide `blocks` SVG to the home icon family; render it through the same 20dp `homeIcon` and `#3F4752` navigation tint used by adjacent items.
- [x] Include `homeSectionTools` in the settings back-navigation cases so returning from settings restores the tools page.
- [x] Add `layoutHomeToolsPage` with the screenshot's title/subtitle treatment and one broad card using the established home card, icon, and color patterns. Cap the card width at 720dp and let it use the available width on narrow windows.
- [x] Defer the multi-tool upgrade banner while the page has one entry. (The original search/filter/count omission was superseded by Task 4.)
- [x] Make the complete card a stable clickable control that opens the workbench-only request; report failures with localized user-facing text.
- [x] Add Chinese and English strings for the navigation label, title, subtitle, card name, card description, card action, and launch failure.
- [x] Run `gofmt` on the modified Go files.

### Task 3: Update the documented navigation boundary

**Files:**
- Modify: `docs/architecture.md`
- Modify: `docs/office-support.md`

**Interfaces:**
- Document that **小工具** is a home page whose **文档工作台** card opens the existing proofreading module task list without a source document.
- Clarify that the entry is on the general tools page; no separate proofreading item is added to the home navigation or PDF toolbar.

- [x] Revise the existing proofreading module descriptions to include the workbench-only launch path and the preserved source-backed PDF path.
- [x] Review the plan against the design spec and preserve unrelated working-tree changes.

### Task 4: Restore the tools search and category toolbar

**Files:**
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/home_tools.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Modify: this design spec to record the toolbar refinement

**Interfaces:**
- Keep `homeToolsSearch` as a persistent single-line `widget.Editor`, `homeToolsCategory` as persistent selection state, and tool category `widget.Clickable`s in `App`.
- Show four reference categories; the only current entry, **文档工作台**, is in **文档与 PDF**.
- Search matches the localized tool title and description; the count shows the number of filtered entries. Empty filters and unmatched searches show localized empty state.
- Preserve the existing single card and its workbench launch action; add no placeholder tools.

- [x] Add persistent search/category state and initialize the editor, filter, and clickables in `App`.
- [x] Restore the heading search field, four category tabs, and visible result count using the reference colors and spacing; keep the controls usable at narrow widths.
- [x] Filter the existing workbench card by category and localized title/description search; show a localized empty state when no entries match.
- [x] Add Chinese and English labels, placeholder, and empty-state copy.
- [x] Run `gofmt` on modified Go files and `git diff --check`; do not run tests/builds/UI verification unless asked.

Task 4 review: a fresh read-only review found and prompted fixes for the missing Lucide separator line, inherited minimum height on the visible count, and narrow-layout category click processing. All three were corrected. No tests, builds, or UI verification were run.
