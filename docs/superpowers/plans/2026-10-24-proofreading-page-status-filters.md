# Proofreading Page Status Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the proofreading toolbar filter reviewed or unreviewed PDF pages in both A and X panes while preserving page identity and synchronization.

**Architecture:** Store an ephemeral filter mode in `proofreadingWorkspace` and derive one ordered list of original PDF page indexes. Both A and X page lists render rows through that mapping; navigation remains expressed in original page indexes and translates to filtered rows only at the list boundary. Keep only the two status filters in the toolbar; remove its no-op previous/next items and redundant page/sync labels. The full-book thumbnail strip stays unfiltered and clears an active filter when the user selects an excluded page.

**Tech Stack:** Go, Gio `widget.List`, existing proofreading workspace state and `internal/desktopui` tests.

**Spec:** `docs/superpowers/specs/2026-10-24-proofreading-page-status-filters-design.md`

## Global Constraints

- A and X must display the same original PDF pages in the same order.
- Filter values come from each page's `Reviewed` state, not from Git snapshot state.
- The default mode is all pages; clicking the active status filter returns to all pages.
- Preserve original PDF page indexes for editing, status, history, and page cache operations.
- Keep the bottom thumbnail strip unfiltered; selecting an excluded page clears the active filter.
- When starting from a PDF page, keep the source PDF embedded as the proofreading task's A pane and do not create a second regular PDF document tab in the workbench.
- Do not create a commit per task; include all work in the existing unified commit when finished.
- Run tests, builds, and launches in `/Users/mima1234/project/glance-go`; reuse existing Go caches.
- Before actual UI verification, terminate existing Glance and `go run` processes, then launch with `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance`.

## Review Focus

- Filtering to a status with no matching pages must not index an empty page list; Task 1 tests empty results and row bounds.
- Switching filters while the current page is excluded must select the next matching page in original order and wrap; Task 1 tests target selection.
- A and X must map the same filtered row to the same original page while sharing scroll position; Task 2 tests row mapping and layout synchronization.
- A page confirmation can change membership in the active filter; Task 3 tests re-evaluation and empty-state behavior.
- Selecting an excluded page from the unfiltered thumbnail strip must clear the filter and show that exact page; Task 3 tests the transition.

---

### Task 1: Filter mode and original-page mapping

**Files:**
- Create: `internal/desktopui/proofread_page_filter.go`
- Create: `internal/desktopui/proofread_page_filter_test.go`
- Modify: `internal/desktopui/proofread_window.go`

**Interfaces:**
- Produce `proofreadingPageFilter` with all, reviewed, and unreviewed modes.
- Produce `proofreadingFilteredPages(ownership []proofreadingPageOwnership, pageCount int, filter proofreadingPageFilter) []int`.
- Produce `proofreadingFilterModeAfterClick(current, requested proofreadingPageFilter) proofreadingPageFilter`; clicking the active mode returns all.
- Produce `proofreadingFilterRowForPage(pages []int, page int) int` and `proofreadingFilterPageAtRow(pages []int, row int) (int, bool)`.
- Produce `proofreadingFilterTargetPage(pages []int, current int) int`; it keeps a matching current page, otherwise chooses the next original page and wraps, returning `-1` for an empty result.
- Store the selected filter mode and derived original-page index list in `proofreadingWorkspace`.

- [x] **Step 1: Write failing helper tests** for ordered all/reviewed/unreviewed page lists, toggling the active mode off, switching between modes, empty results, row-to-page mapping, invalid rows, and keep/advance/wrap current-page selection.
- [x] **Step 2: Run** `go test ./internal/desktopui -run '^TestProofreadingPageFilter' -count=1 -v` and confirm the failures name the missing filter behavior.
- [x] **Step 3: Implement** the filter enum, pure mapping helpers, and workspace state.
- [x] **Step 4: Run** the same test command and confirm it passes.

### Task 2: Apply one filtered page sequence to A and X

**Files:**
- Modify: `internal/desktopui/pdf_workspace_layout.go`
- Modify: `internal/desktopui/proofread_projection.go`
- Modify: `internal/desktopui/proofread_navigator.go`
- Create: `internal/desktopui/proofread_page_filter_layout_test.go`

**Interfaces:**
- Consume `proofreadingFilteredPages` from Task 1.
- Both canvas layout callbacks receive filtered row indexes and translate them to original PDF page indexes before reading page geometry, render cache, page ownership, or editor state.
- Keep `view.page` as an original PDF page index; keep the A and X `widget.List.Position.First` values as the shared filtered row index.

- [x] **Step 1: Write failing layout tests** proving that a filtered row resolves to the same original page in A and X and that both list positions remain synchronized.
- [x] **Step 2: Run** `go test ./internal/desktopui -run '^TestProofreadingPageFilterLayout' -count=1 -v` and confirm the expected mapping failure.
- [x] **Step 3: Implement** filtered row mapping in the source PDF and X projection lists, maintain the logical current page, and show a localized empty state when no page matches.
- [x] **Step 4: Run** the layout test and the existing proofreading scroll synchronization tests; confirm both pass.

### Task 3: Toolbar interaction and filtered navigation

**Files:**
- Modify: `internal/desktopui/proofread_chrome.go`
- Modify: `internal/desktopui/proofread_window.go`
- Modify: `internal/desktopui/proofread_pagination.go`
- Modify: `internal/desktopui/pdf_commands.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Modify: `internal/desktopui/proofread_page_filter_test.go`

**Interfaces:**
- Toolbar buttons use stable `widget.Clickable` state and call the Task 1 filter-mode transition.
- The bottom filmstrip's previous/next page actions choose the adjacent matching original PDF page while a filter is active.
- Thumbnail selection remains full-book navigation; selecting a page outside the active filter clears the filter before showing that page.

- [x] **Step 1: Add failing interaction tests** for selecting reviewed/unreviewed, toggling the active filter to all, previous/next skipping nonmatching pages, and excluded-thumbnail selection clearing the filter.
- [x] **Step 2: Run** the focused filter tests and confirm they fail on the unhandled clicks/navigation.
- [x] **Step 3: Implement** filter button event handling, filmstrip previous/next navigation, thumbnail filter clearing, localized no-results labels, and removal of the no-op/redundant toolbar groups.
- [x] **Step 4: Run** the focused filter tests plus existing proofreading navigation and thumbnail regressions.

### Task 4: Rebuild all history as a full page

**Files:**
- Modify: `internal/proofhistory/repository.go`
- Modify: `internal/proofhistory/repository_test.go`
- Modify: `internal/desktopui/proofread_workbench_state.go`
- Modify: `internal/desktopui/proofread_history_layout.go`
- Modify: `internal/desktopui/proofread_layout.go`
- Modify: `internal/desktopui/proofread_editor_events.go`
- Modify: `internal/desktopui/proofread_page_menu.go`
- Modify: `internal/desktopui/proofread_history_layout_test.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`

**Interfaces:**
- `Repository.ListPages(ctx)` returns one summary per page with committed history, ordered by latest commit.
- `Repository.PageChanges(ctx, page)` returns newest-first commits that touch the page. Status is page-scoped; changed paths, insertion/deletion totals, file count, and bounded diff excerpt cover the complete Git commit.
- The history route is a full-page child of the existing A/X tab chrome. It has a back row, left page list, and right per-page commit timeline. No scrim or drawer remains.
- Each page/commit control uses stable widget state. Selecting a page loads that page's timeline asynchronously; restoring remains page-scoped and returns to the proofreading workspace.
- Remove the floating “本页历史” menu row and its click handler/localization key.
- Match the sJZ5u positions for the centered back row, title/subtitle spacing, timeline top inset, right-aligned three-item legend, commit type badge, and right-aligned Git-derived change statistics. Verify rendered pixels at 1x and 2x.

- [x] **Step 1: Write failing repository tests** for unique page summaries ordered by latest commit and per-page change metadata/diff, including a commit that changes two files.
- [x] **Step 2: Run** `go test ./internal/proofhistory -run 'TestProofreadingHistory(PageSummaries|PageChanges)' -count=1 -v` and confirm the missing APIs/field fail.
- [x] **Step 3: Implement** page aggregation with page-scoped status and bounded commit-wide paths, insertion/deletion totals, file count, and diff excerpt.
- [x] **Step 4: Run** the focused repository tests and confirm they pass.
- [x] **Step 5: Write a failing history-page layout and pixel test** for full-window panels, centered back row, heading/timeline spacing, right-aligned legend/statistics, and no drawer overlay.
- [x] **Step 6: Run** `go test ./internal/desktopui -run '^TestProofreadingHistoryPage' -count=1 -v` and confirm it fails on the missing route/alignment behavior.
- [x] **Step 7: Implement** the full-page route, asynchronous page timeline loading, design-aligned history panels, restore action, Escape/back handling, and remove the page-history menu item.
- [x] **Step 8: Run** history-page tests and proofreading history regressions; confirm they pass at 1x and 2x.

### Task 5: Regression verification and unified commit

**Files:**
- Verify: `internal/proofhistory` history summaries and change metadata.
- Verify: `internal/desktopui` proofreading filter, scroll, navigation, workbench-tab isolation, and history-page tests.
- Verify: both locales are valid JSON and have matching proofreading history keys.
- Verify: the approved spec and this plan.

- [x] **Step 1: Run** `git diff --check`, focused repository/desktop UI/i18n tests, and inspect any skipped visual checks separately.
- [x] **Step 2: Stop existing Glance processes and launch** `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance` from the repository root; report whether the actual window is available for visual inspection.
- [x] **Step 3: Complete the fresh read-only review, then amend** the existing unified commit once implementation and verification are complete; do not create an additional commit.
