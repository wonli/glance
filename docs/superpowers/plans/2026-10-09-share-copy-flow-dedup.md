# Share-Copy Flow Deduplication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce duplicated XLSX and DOCX share-copy implementation while preserving user-visible behavior and document-safety guarantees.

**Architecture:** Add same-package helpers for extension normalization, guarded snapshot-copy writing, and result status reporting. Keep format-specific picker lifecycle, stale-state validation, mutexes, adapter creation, goroutine/result handling, and status text inputs in the existing per-format flows.

**Tech Stack:** Go 1.27, Gio, existing XLSX and DOCX format adapters.

**Spec:** `docs/superpowers/specs/2026-10-09-share-copy-flow-dedup-design.md`

## Global Constraints

- Reduce duplicated implementation in the XLSX and DOCX share-copy export flows while preserving all user-visible behavior and document-safety guarantees.
- Keep request and picker lifecycle, including Word draft flushing, in the format-specific files.
- Keep format-specific freshness checks, commit mutexes, and final commit guards unchanged.
- Keep adapter temporary-file replacement behavior unchanged.
- Keep the existing share-copy tests and assertions unchanged.
- Do not unify ordinary Save/Save As operations or move adapter construction out of `desktopui`.

## Review Focus

- Missing or differently cased extension: append the requested extension only when it is absent; retain an existing matching extension regardless of case. Pin this in `TestEnsureShareCopyExtension`.
- Snapshot and freshness ordering: return snapshot errors unchanged and skip later callbacks; after a successful snapshot, return stale-state errors before opening a copy. Pin both paths in `TestSaveShareCopySnapshotMaintainsCallbackOrderAndErrorPrecedence`.
- Snapshot opener failure: retain the localized `校验 {format} 副本：` prefix and do not attempt save or close. Pin this in the same callback-order test.
- Commit-guard and close behavior: forward the supplied guard unchanged; preserve the save error if save and close both fail, or return the close error when save succeeds. Pin this in the callback-order test.
- Result status and session visibility: preserve format-specific completion text and suppress updates for closed or noncurrent sessions. Pin this in `TestApplyShareCopyResultPreservesCompletionStatus`.

---

### Task 1: Add Shared Share-Copy Helpers

**Files:**
- Create: `internal/desktopui/share_copy.go`
- Create: `internal/desktopui/share_copy_test.go`

**Interfaces:**
- Produces `shareCopyDocument`, implemented by reopened XLSX and DOCX adapters, with `SaveAsWithCommitGuard(path string, guard func() (func(), error)) error` and `Close() error` methods.
- Produces `ensureShareCopyExtension(path, extension string) string`; `extension` is passed without a leading dot, and an existing extension is compared case-insensitively.
- Produces generic `saveShareCopySnapshot[T shareCopyDocument](path, format string, snapshot func() ([]byte, error), revalidate func() error, openSnapshot func([]byte) (T, error), commitGuard func() (func(), error)) error`; the type parameter keeps each adapter's concrete snapshot-open result statically checked.
- Produces `(a *App) applyShareCopyResult(state *documentSessionState, path string, err error, format string)` for the existing common completion status behavior.

- [x] **Step 1: Add extension-normalization cases**

Add `TestEnsureShareCopyExtension` with cases for a missing extension, a matching uppercase extension, and a different existing extension. Expected behavior must match the current per-format suffix rule: a different extension is retained and the requested extension is appended.

- [x] **Step 2: Run the extension test and confirm it fails because the helper is not defined**

Run: `go test ./internal/desktopui -run '^TestEnsureShareCopyExtension$' -count=1`
Expected: FAIL because `ensureShareCopyExtension` is undefined.

- [x] **Step 3: Add snapshot-copy callback and error-precedence cases**

Add `TestSaveShareCopySnapshotMaintainsCallbackOrderAndErrorPrecedence` using a small fake `shareCopyDocument`. Cover snapshot failure, stale revalidation before open, localized open failure, forwarding the commit guard through adapter save, save failure with close failure, and close-only failure. Record callback calls so the test verifies the order and skipped operations.

- [x] **Step 4: Run the snapshot-copy test and confirm it fails because the helper is not defined**

Run: `go test ./internal/desktopui -run '^TestSaveShareCopySnapshotMaintainsCallbackOrderAndErrorPrecedence$' -count=1`
Expected: FAIL because `saveShareCopySnapshot` is undefined.

- [x] **Step 5: Add the common completion-status cases**

Add `TestApplyShareCopyResultPreservesCompletionStatus` for successful XLSX and DOCX status text, error status text on the active session, and no status update for a closed or noncurrent session.

- [x] **Step 6: Run the result-status test and confirm it fails because the helper is not defined**

Run: `go test ./internal/desktopui -run '^TestApplyShareCopyResultPreservesCompletionStatus$' -count=1`
Expected: FAIL because `applyShareCopyResult` is undefined.

- [x] **Step 7: Implement the helpers in `share_copy.go`**

Preserve these exact semantics in `saveShareCopySnapshot`: snapshot errors pass through unchanged; revalidation runs only after snapshot succeeds and before opening; opener errors are wrapped as `校验 {format} 副本：%w`; the opened copy is always closed; a save error takes precedence over a close error.

- [x] **Step 8: Run the helper tests**

Run: `go test ./internal/desktopui -run '^(TestEnsureShareCopyExtension|TestSaveShareCopySnapshotMaintainsCallbackOrderAndErrorPrecedence|TestApplyShareCopyResultPreservesCompletionStatus)$' -count=1`
Expected: PASS.

### Task 2: Route XLSX and DOCX Exports Through the Shared Helpers

**Files:**
- Modify: `internal/desktopui/spreadsheet_share_copy.go`
- Modify: `internal/desktopui/word_share_copy.go`
- Test: existing `internal/desktopui/spreadsheet_share_copy_test.go`
- Test: existing `internal/desktopui/word_share_copy_test.go`

**Interfaces:**
- Consumes the helper interfaces from Task 1.
- Keeps `spreadsheetShareCopyResult` and `wordShareCopyResult`, each format-specific picker request, async result channel, shutdown selection, and window invalidation unchanged.

- [x] **Step 1: Replace duplicated path-suffix handling**

Call `ensureShareCopyExtension(path, "xlsx")` and `ensureShareCopyExtension(path, "docx")` after each existing same-source-path rejection. Preserve each current overwrite status message.

- [x] **Step 2: Replace duplicated result-status handling**

Route each existing apply-result method through `applyShareCopyResult`, passing the same session, path, error, and uppercase format label. Preserve the methods used by async-result dispatch.

- [x] **Step 3: Replace duplicated snapshot/open/save/close bodies**

In each existing goroutine, call `saveShareCopySnapshot`. The snapshot callback must keep `lockSessionAdapterRead`; the revalidation callback must keep the current closed/history or closed/revision/draft-epoch check and exact error; the opener must call the current format's `OpenSnapshot`; and the commit guard must retain the current format-specific mutex and final freshness check. Keep result construction, channel send, shutdown case, and window invalidation in their current location.

- [x] **Step 4: Run the existing share-copy regression tests**

Run: `go test ./internal/desktopui -run '^(TestSpreadsheetShare|TestWordShare)' -count=1`
Expected: PASS, including export/source preservation, overwrite and stale-picker rejection, Word write-failure state preservation, and Word pending-draft flushing.

- [x] **Step 5: Review the behavior-preserving diff**

Run: `git diff --check`
Expected: no whitespace errors. Inspect both per-format flows against the spec to confirm the freshness checks, lock scopes, status strings, and asynchronous result delivery remain unchanged.

### Final Review Fix: Guard the Extension-Normalized Destination

**Files:**
- Create: `internal/desktopui/share_copy_overwrite_test.go`
- Modify: `internal/desktopui/spreadsheet_share_copy.go`
- Modify: `internal/desktopui/word_share_copy.go`

**Interfaces:**
- Preserve the current format-specific rejection messages and use `ensureShareCopyExtension` to derive the final target path before comparing it with the source path.
- Keep the existing share-copy tests and assertions unchanged; put the new regression cases in the new test file.

- [x] **Step 1: Add failing extensionless-source cases for XLSX and DOCX**

Add `TestShareCopyRejectsExtensionlessSourcePath` with separate XLSX and DOCX cases. Each case passes a source path with its format suffix removed, expects the existing same-source rejection status, and verifies no export operation starts.

- [x] **Step 2: Run the regression test and confirm the current source check is bypassed**

Run: `go test ./internal/desktopui -run '^TestShareCopyRejectsExtensionlessSourcePath$' -count=1`
Expected: FAIL because the export path receives the operation-in-progress status instead of the same-source rejection status.

- [x] **Step 3: Normalize the destination before same-source comparison**

In both format-specific entry points, call `ensureShareCopyExtension` before `sameDocumentPath`, then retain each existing rejection message and return path.

- [x] **Step 4: Run the focused regression and existing share-copy tests**

Run: `go test ./internal/desktopui -run '^(TestShareCopyRejectsExtensionlessSourcePath|TestSpreadsheetShare|TestWordShare)' -count=1`
Expected: PASS.

- [x] **Step 5: Run the full Go test suite after the review fix**

Run: `go test ./... -count=1`
Expected: all packages pass except any explicitly documented pre-existing failures; report those failures.

Execution result (2026-10-09): the new XLSX and DOCX regression cases failed before the fix with successful-export status, then passed after normalizing the destination before `sameDocumentPath`. The focused share-copy suite passed. The full suite completed with the previously recorded `TestCellEditorUsesRenderedStyleAndKeepsSingleCellGeometry` failure at `internal/desktopui/grid_paint_test.go:93` (editor text size 12); all other reported packages passed.
