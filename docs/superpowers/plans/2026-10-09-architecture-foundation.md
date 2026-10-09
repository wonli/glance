# Architecture Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the existing document architecture easier to evolve without changing product behavior or UI.

**Architecture:** Keep the current Gio UI, session/controller, neutral document model, format adapter, and local-file flow. Improve confidence first with render QA, then make small same-package file splits and package-boundary decisions backed by tests.

**Tech Stack:** Go, Gio, existing office format adapters, `go test`, `glance-office qa render`.

**Spec:** `docs/architecture.md`, `docs/office-support.md`, and `docs/compatibility/office-render-qa.md`.

## Global Constraints

- Do not change user-visible functionality or UI during the foundation refactor.
- Keep production source files focused on one responsibility/functionality and generally at or below 500 lines; split by cohesive domain, not one function per file.
- Keep third-party Office library types behind format adapters.
- Keep the Gio window loop responsive; parsing and saves run outside layout/frame code.
- Preserve dirty state on save failure and do not claim formula recalculation or Word pagination without evidence.
- Do not add test-file source/version provenance; the corpus consists of concrete Office files.
- Work directly on `main`; do not create a worktree.

## Review Focus

- Word search and navigation share match semantics; the first file extraction must retain Unicode case-insensitive and draft-aware behavior.
- Existing desktop UI suite has a known editor-font-size failure; compare before/after rather than attributing it to a structural move.
- Render QA diagnostics are expected compatibility findings, not rendering crashes; preserve each diagnostic and manifest expectation.
- Platform fonts can affect image output; compare only the reviewed deterministic baselines configured in the manifest.
- Folder/package name exceptions may express distinct domains (`xls` vs `xlsx`, `pdf` vs `pdfdoc`); rename only when the package boundary is demonstrably wrong.

---

### Task 1: Establish The Current Quality Baseline

**Files:**
- Inspect: `internal/officeqa/`, `testdata/office-corpus/manifest.json`, `docs/compatibility/office-render-qa.md`
- No behavior changes in this task; retain existing local QA edits.

**Interfaces:**
- Consumes: existing `glance-office qa render` CLI and checked-in Office corpus.
- Produces: a recorded baseline report and known-failure list for subsequent structure-only changes.

- [x] Run `go test ./internal/officeqa -count=1`.
- [x] Build and run `qa render` against all 50 corpus documents.
- [x] Run `go test ./...` and record any unrelated existing failures.

### Task 2: Split The Word Toolbar By Functionality

**Files:**
- Create: focused `internal/desktopui/word_*.go` files for toolbar events, commands, text formatting, page setup, insert, review, and design responsibilities.
- Modify: `internal/desktopui/word_toolbar.go`
- Test: `internal/desktopui/word_toolbar_test.go`
- Collaborates with: `internal/desktopui/word_navigation_pane.go`

**Interfaces:**
- Keep all package-private types and method signatures unchanged.
- Keep every moved declaration in package `desktopui`; do not change API, state flow, status text, widgets, or behavior.
- Target one cohesive functionality per file and generally no more than 500 lines, including imports and comments.

- [x] Run the focused existing Word find tests before moving code.
- [x] Move the search/match/replacement block into `word_search.go`; do not alter implementation or UI text.
- [x] Run focused Word find tests after the first move.
- [x] Run the full `internal/desktopui` suite and confirm it reports only the known baseline failure.
- [x] Run the 50-document render QA and confirm configured baselines and expected diagnostics are unchanged.
- [x] Split the remaining Word toolbar declarations into cohesive files, keeping every resulting production source file around or below 500 lines.
- [x] Run focused tests for Word find and the full `internal/desktopui` suite after both file splits; the only failure matches the pre-existing font-size assertion.
- [x] Run the 50-document render QA after both file splits; configured baselines and expected diagnostics are unchanged.
- [x] Run `git diff --check` and inspect the resulting move-only diff.

### Task 3: Record Package Naming And Boundary Decisions

**Files:**
- Inspect: all `internal/**` package declarations and import dependencies.
- Modify only if needed: `docs/architecture.md`.

**Interfaces:**
- Do not rename a package solely to make names uniform; document a convention and list only proven exceptions.
- Preserve separate legacy and OOXML adapters (`xls`, `xlsx`) and reader/renderer responsibilities (`pdfdoc`, `pdf`).

- [x] Inventory package declarations and direct third-party imports.
- [x] Confirm that `internal/desktopui` does not import third-party Office libraries directly; it does import format adapter packages for creation, view-data, and export responsibilities.
- [x] Document naming rules in `docs/architecture.md`; current adapter package names are purposeful, so no cosmetic renames were made.

### Task 4: Split Large Production Files By Responsibility

**Files:**
- Select files based on responsibility boundaries and dependency direction.
- Keep all moved declarations in their existing package.

**Interfaces:**
- Keep exported APIs and runtime behavior unchanged.
- Prefer cohesive ownership-based splits over line-count-only cuts.

- [x] Record the chosen file boundary and why it is cohesive; the spreadsheet command surface has independently testable operation families and remained in the same `desktopui` package.
- [x] Split `internal/desktopui/spreadsheet_commands.go` into cohesive files, each under 500 lines, without changing names or behavior.
- [x] Run focused tests for spreadsheet operations.
- [x] Run the full `internal/desktopui` suite and Office render QA after both large-file splits.
- [x] Prefer production files over large test files for the initial 500-line guardrail; retain test files as a later feature-owned cleanup.
- [x] Inventory and split cohesive responsibilities in `desktopui` app/document lifecycle, Word/PPTX presentation/workspace/layout views, spreadsheet toolbar and grid/status views, DOCX projection/text/XML editing, PPTX document/geometry, PDF spreadsheet export, and `wordlayout` setup/table/paragraph layout. Follow-up splits cover Word navigation/insertion, font-cache setup, DOCX package-property helpers, PPTX style XML helpers, Word PDF rendering, and dialog styles.
- [x] Run owning package tests before and after each selected move; preserve API and behavior.
- [x] Run `go test ./... -count=1`; all packages pass except the known `TestCellEditorUsesRenderedStyleAndKeepsSingleCellGeometry` editor-font-size assertion.
- [x] Build the desktop binary and run all 50 Office corpus documents; 0 open/render/baseline failures and 0 unexpected outcomes. `drawing-shapes.docx` continues to report `error:nested-table`.
- [x] Finish the production-source inventory: no production Go source file remains above 500 lines. The former async-results, text-shaper, and JSON state files were further separated at their Word-command, glyph-outline, and JSON-tree boundaries.

### Task 5: Keep QA And Architecture Evidence Current

**Files:**
- Modify as evidence changes: `docs/architecture.md`, `docs/office-support.md`, `docs/compatibility/office-render-qa.md`.

**Interfaces:**
- QA expectations describe observed compatibility behavior, not desired behavior.
- Performance metrics remain observational and comparable only across matching corpus, viewport, platform, and runtime conditions.

- [x] No Office support boundary changed in this structure-only batch.
- [x] Keep expected unsupported diagnostics explicit in corpus expectations; verify the nested-table diagnostic remains present.
- [x] Re-run verification for this foundation milestone before reporting its status.

### Task 6: Split Oversized Test Files By Feature

**Files:**
- Split large `*_test.go` files in `internal/desktopui`, `internal/format/{docx,pptx,xlsx}`, `internal/wordlayout`, and `internal/officeqa` by existing test responsibilities.
- Keep test functions and assertions unchanged; move only declarations and file-scoped imports.

**Interfaces:**
- Tests remain in their existing Go package and keep their names, fixtures, and assertions.
- Shared helpers remain package-level and are placed in a focused `*_test.go` file when that reduces duplication or keeps feature files cohesive.

- [x] Split `word_toolbar_test.go` and `spreadsheet_toolbar_test.go` by tested feature family; run `go test ./internal/desktopui -count=1` and compare with the known baseline failure.
- [x] Split oversized presentation, workspace, Word interaction/history, and home tests by feature; run the owning package tests.
- [x] Split oversized DOCX, PPTX, XLSX, and `wordlayout` tests by adapter/engine responsibility; run each owning package.
- [x] Split the oversized Office QA runner tests by command, report, and diagnostic behavior; run `go test ./internal/officeqa -count=1`.
- [x] Recount all production and test Go files; no Go file remains above 500 lines. The sorted `Test...` declaration set matches HEAD exactly.
- [x] Run `go test ./... -count=1`, `make desktop`, and the 50-document render QA; preserve the known editor-font-size failure and expected Office diagnostics. QA: 40 rendered, 10 completed with expected diagnostics, 0 open/render/baseline failures, 0 unexpected outcomes; `drawing-shapes.docx` still reports `error:nested-table`.
