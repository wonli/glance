# 校对列表完整稿导出实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从校对列表导出完整的 `.md`、`.markdown` 或 `.txt` 稿件，已确认页使用确认内容，其他页使用导入时 B 原稿，并按 PDF 页序排列。

**Architecture:** 在 `internal/application` 增加纯文本组装器，按页 ID 校验并组合已确认文本和原始文本。桌面 UI 在首次分页归属确定后保存原始 B 文本的逐页 rune 边界，任务列表通过后台任务重建原文页、读取恢复快照、打开保存对话框并原子写入所选文件。

**Tech Stack:** Go、Gio、现有 `workspace.Store` / `proofhistory.Repository`、`fileutil.AtomicWriteString`、现有原生 `chooseSavePath` 文件选择器。

**Spec:** `docs/superpowers/specs/2026-10-26-proofreading-list-export-design.md`

## Global Constraints

- 所有测试、构建、启动和验证必须直接在当前授权代码目录 `/Users/mima1234/project/glance-go` 中执行。
- 不创建、添加、移动或重定位 Git worktree；保留工作区已有修改。
- 页面顺序使用 PDF 页 ID 升序；提交时间和校对时间不参与排序。
- `.md`、`.markdown`、`.txt` 输出扩展名与 B 原稿一致；Word 暂不支持。
- Git 快照不是导出前提；已确认页必须由确认哈希证明与当前内容一致。
- 未确认页使用任务保存的原始 B 页文本；不得从 A PDF 抽取文本或 OCR。
- Gio layout/frame 内不得执行文件读取、解析、组装或写入。
- 另存导出不得修改源文件、校对副本、SQLite 进度或 Git 历史；目标写入使用同目录临时文件并在写入成功后替换。

## Review Focus

- 6、500、7 等非确认顺序最终仍按页 ID 排列：由 Task 1 的 `TestAssembleProofreadingExportOrdersByPageID` 固定。
- 无 Git 快照但当前模型与确认哈希匹配时必须导出确认内容：由 Task 3 的 `TestBuildProofreadingExportCombinesOriginalAndConfirmedPagesWithoutGit` 固定。
- 确认后有修改的页面不得混入输出：由 Task 1 的 `TestAssembleProofreadingExportRejectsStaleReviewedPage` 与 Task 3 的 `TestBuildProofreadingExportRejectsModifiedConfirmedPage` 固定。
- 未确认页使用导入时 B 文本，且原稿保持不变：由 Task 2 的 `TestProofreadingOriginalPagesPersistAndRestore` 与 Task 3 的 `TestBuildProofreadingExportCombinesOriginalAndConfirmedPagesWithoutGit` 固定。
- 保存路径取消、写入失败和扩展名错误均不写入：由 Task 4 的 `TestProofreadingExportFlowCancelDoesNotWrite`、`TestProofreadingExportFlowReportsWriteError` 与 `TestProofreadingExportFlowRejectsExtensionMismatchWithoutOverwritingExpectedExtension` 固定。
- 目标路径的扩展名必须一致，且不能指向源文件、项目目录、恢复副本或工作区数据库；符号链接别名也必须拦截。
- 打开中的校对标签页必须以点击导出时的已保存快照为准；待保存或跨窗口状态不可读取时先阻止导出。
- 旧任务用重建分页补足已删除原文字符留下的边界空档，并以仍保留的原文字符页归属约束分页；字符归属顺序冲突或完全没有原文字符身份时失败关闭。

---

## File Structure

- Create `internal/application/proofreading_export.go`: format-neutral page input and ordered full-document assembler.
- Create `internal/application/proofreading_export_test.go`: pure ordering, source selection, page completeness, and confirmation validation tests.
- Modify `internal/desktopui/proofread_window.go`: stable per-task state for frozen original page rune ends.
- Modify `internal/desktopui/proofread_controller.go`: capture original B rune ends exactly when initial PDF-to-B assignment succeeds.
- Modify `internal/desktopui/proofread_edit.go`: persist original page rune ends in the proofreading-copy metadata and project payload; restore them with the draft.
- Modify `internal/desktopui/proofread_project_persistence.go`: include original page rune ends in saved/recovered project state without changing the retained source B file.
- Create `internal/desktopui/proofread_export.go`: load a task for export, resolve confirmed-current pages, handle legacy projects without frozen page text, and write using `fileutil.AtomicWriteString`.
- Create `internal/desktopui/proofread_export_test.go`: project-level export, no-Git behavior, legacy reconstruction, and atomic output tests.
- Modify `internal/desktopui/proofread_workbench_layout.go`: add a stable per-row Export action.
- Modify `internal/desktopui/proofread_workbench_state.go`: launch save selection and export as a managed background job; deliver success/failure feedback to the UI thread.
- Modify `internal/desktopui/proofread_workbench_layout_test.go`: verify the row registers and dispatches the Export action.
- Modify `internal/i18n/locales/zh-CN.json` and `internal/i18n/locales/en-US.json`: export button, unsupported format, progress, success, and actionable error messages.

## Task 1: Build the neutral ordered assembler

**Files:**
- Create: `internal/application/proofreading_export.go`
- Test: `internal/application/proofreading_export_test.go`

**Interfaces:**
- Produces:
  - `type ProofreadingExportPage struct { ID proofreading.PageID; Reviewed bool; CurrentConfirmed bool; CurrentText string; OriginalText string }`
  - `func AssembleProofreadingExport(pages []ProofreadingExportPage) (string, error)`
- `AssembleProofreadingExport` sorts by `ID`, requires one entry for each page ID from 1 through `len(pages)`, uses `CurrentText` only for a reviewed/currently-confirmed page, uses `OriginalText` for an unreviewed page, and returns a page-specific error for a reviewed page whose current content is not confirmed. Page strings are concatenated without synthetic separators.

- [x] **Step 1: Write failing assembler tests** for mixed reviewed/original pages, unordered input, 500 page entries with reviewed pages 6, 7, and 500, duplicate/missing page IDs, and a modified reviewed page.
- [x] **Step 2: Run the focused test** with `go test ./internal/application -run '^TestAssembleProofreadingExport' -count=1`; confirm the new tests fail because the assembler is absent.
- [x] **Step 3: Implement `AssembleProofreadingExport`** in `internal/application/proofreading_export.go` using page-ID ordering and the exact selection rules above.
- [x] **Step 4: Run the focused test** with `go test ./internal/application -run '^TestAssembleProofreadingExport' -count=1`; confirm all assembler tests pass.

## Task 2: Freeze and restore original B text by page

**Files:**
- Modify: `internal/desktopui/proofread_window.go`
- Modify: `internal/desktopui/proofread_controller.go`
- Modify: `internal/desktopui/proofread_edit.go`
- Modify: `internal/desktopui/proofread_project_persistence.go`
- Test: `internal/desktopui/proofread_project_persistence_test.go`
- Test: `internal/desktopui/proofread_export_test.go`

**Interfaces:**
- Consumes: `application.ProofreadingLayoutSnapshot.Ends` at the successful initial assignment in `acceptProofreadingModelLayout`.
- Produces: `proofreadingWorkspace.originalPageEnds []int` and optional `proofreadingPersistedDraft.OriginalPageEnds []int`; JSON absence remains valid for old drafts.
- Capture the original B text's page rune ends at initial assignment, before later text edits or page ownership changes. Persist only the boundaries in both recovery forms and restore them independently of current page ownership.
- Old project payloads without page ends remain readable with the optional field nil; Task 3 reconstructs them only when an export is requested.

- [x] **Step 1: Add failing tests** for CJK/combining-rune page slicing, OriginalPageEnds round-trip through draft and project persistence, and legacy payload decoding when the optional field is absent.
- [x] **Step 2: Run `go test ./internal/desktopui -run 'TestProofreading(OriginalPages|ProjectPayload)' -count=1`** and confirm the new assertions fail before persistence changes.
- [x] **Step 3: Capture and persist `originalPageEnds`** after the first successful `AssignInitial`, and restore them independently of current page ownership.
- [x] **Step 4: Validate persisted boundaries** in `encodeProofreadingDraftFile` and `decodeProofreadingDraftFile`: when present, their count must match `Model.Pages`; an absent field remains valid for legacy copies.
- [x] **Step 5: Run the focused persistence tests** with `go test ./internal/desktopui -run 'TestProofreading(OriginalPages|ProjectPayload)' -count=1`; confirm new and legacy payload tests pass.

## Task 3: Load and write a complete project export

**Files:**
- Create: `internal/desktopui/proofread_export.go`
- Test: `internal/desktopui/proofread_export_test.go`

**Interfaces:**
- Consumes: `application.AssembleProofreadingExport`, `workspace.Store.LoadProofreadingProject`, `proofreadingPersistedDraft.OriginalPageEnds`, `proofreadingConfirmedCurrent`, and retained `SourceB`.
- Produces: `func buildProofreadingProjectExport(ctx context.Context, store *workspace.Store, project workspace.ProofreadingProject, themeFace string, fonts fontcatalog.Snapshot) (string, string, error)` returning UTF-8 document text and the original supported extension; `func writeProofreadingProjectExport(path, text string) error` using `fileutil.AtomicWriteString` with `fileutil.WithTempDir(filepath.Dir(path))`.
- Also produces: `func reconstructProofreadingOriginalPageEnds(ctx context.Context, project workspace.ProofreadingProject, themeFace string, fonts fontcatalog.Snapshot) ([]int, error)` for legacy projects whose payload has no stored boundaries.
- Re-project retained Source B through its existing adapter, verify the projected-text hash, and slice original page text using `OriginalPageEnds`. Derive `CurrentText` from persisted model runs grouped by PDF page. Derive `CurrentConfirmed` from the same confirmation hash rule used by project persistence. Do not require Git or create a Git operation.
- Reject `.doc` / `.docx`; support `.md`, `.markdown`, and `.txt`. Preserve original B input and task state.

- [x] **Step 1: Write failing project tests** for full ordered hybrid output without a Git repository, a modified confirmed page, legacy page-map reconstruction and unverified mappings, supported extension matching, unsupported Word input, and source B immutability.
- [x] **Step 2: Run the focused export tests** and confirm failures identify missing export behavior and unsafe legacy assumptions.
- [x] **Step 3: Implement `reconstructProofreadingOriginalPageEnds`** to re-project retained Source B through the matching adapter, open Source A to read page geometry, run the original initial layout at 12sp, and return boundaries after validating hashes and page count. For legacy tasks, constrain the reconstructed ends with surviving original character IDs and their saved page ownership, using layout only to place deleted-ID gaps; reject contradictory ownership or a model with no original IDs. All-confirmed-current legacy tasks do not need original slices.
- [x] **Step 4: Implement `buildProofreadingProjectExport`** to load task state, validate source hashes and page count, use stored or verified reconstructed original ends, slice original text, resolve each page, then call `application.AssembleProofreadingExport`. Active same-window editors use an immutable click-time snapshot; pending, failed, closing, or cross-window editors are blocked.
- [x] **Step 5: Implement `writeProofreadingProjectExport`** with `fileutil.AtomicWriteString` and `fileutil.WithTempDir(filepath.Dir(path))`; preserve the selected path and reject mismatched extensions or protected source/project/recovery/database paths, including symlink aliases.
- [x] **Step 6: Run focused export tests** and confirm data, format, path protection, active editor snapshot, and atomic-write cases pass.

## Task 4: Add the task-list export flow

**Files:**
- Modify: `internal/desktopui/proofread_workbench_layout.go`
- Modify: `internal/desktopui/proofread_workbench_state.go`
- Modify: `internal/desktopui/proofread_workbench_layout_test.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Test: `internal/desktopui/proofread_export_test.go`

**Interfaces:**
- Consumes: `buildProofreadingProjectExport`, `writeProofreadingProjectExport`, existing `chooseSavePath`, and `proofreadingWorkbenchJob`.
- Produces: a stable `export-<projectID>` row button and `startProofreadingProjectExport(project workspace.ProofreadingProject)`.
- Set the save dialog's default name from the retained B original's basename and the original extension. Keep file selection and document work off Gio layout/frame code. On cancel, do not write. On success or failure, post feedback through existing workbench state.

- [x] **Step 1: Add failing row/flow tests** for export button registration, `.md` / `.markdown` / `.txt` path defaults, Word rejection before showing a picker, save cancellation, successful export feedback, and write-error feedback.
- [x] **Step 2: Run focused row/flow tests** and confirm the expected failures before adding the export flow.
- [x] **Step 3: Add the Export control** alongside Continue/Delete and wire it to `startProofreadingProjectExport`; use the existing native save path chooser and background lifecycle.
- [x] **Step 4: Add Chinese and English localized strings** for the action, unsupported format, progress, success, and actionable errors.
- [x] **Step 5: Run focused UI/state tests** and confirm cancellation, result delivery, extension matching, destination protection, and live snapshot capture work.

## Final Verification

- [x] Run `go test ./internal/application -run '^TestAssembleProofreadingExport' -count=1`.
- [x] Run focused desktop UI export and persistence tests using the repository's existing Go cache and a temporary directory under the current workspace.
- [x] Run `git diff --check` and inspect the complete diff, preserving unrelated existing workspace changes.
- [x] Report test output accurately. No actual window was launched, so this implementation report does not claim rendered-window verification.
