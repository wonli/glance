# Whole Book Character Comparison Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Run the existing single-page character comparison sequentially with a blocking progress dialog and resumable pause.
**Architecture:** A UI-owned scheduler wraps compareProofreadingOCRStep without duplicating OCR, boundary, diff or cache logic. Persistent widget state lives on the step; checkpoint metadata uses the existing project payload and asynchronous persistence.
**Tech Stack:** Go / Gio / existing OCR and project adapters.
**Spec:** docs/superpowers/specs/2026-10-26-whole-book-comparison-design.md

## Global Constraints
- Current authorized checkout only; no additional caches or app copies.
- UI must not automatically confirm pages or correct ordinary differences.
- Name: 整本逐字比对; single page: 本页逐字比对.
- Reuse compareProofreadingOCRStep for every page.

## Review Focus
- Pause during OCR/boundary work cannot advance the model or next page.
- Late results after end/retry must not restart a stopped task.
- Editing while paused invalidates pending comparisons.
- Modal must block pointer, wheel, keyboard and Escape dismissal.
- Recovery resumes paused and validates source/page range.

### Task 1: Sequential scheduler and checkpoint
Files: proofread_ocr_batch.go, proofread_ocr_batch_test.go, proofread_ocr_step.go, proofread_layout.go, proofread_edit.go, proofread_project_persistence.go.
Interfaces: requestProofreadingOCRBatch(), processProofreadingOCRBatch(gtx), pause/resume/end helpers; consumes compareProofreadingOCRStep and existing step results.
- [x] Write failing progression, pause, edit, error, final page and checkpoint tests; run them.
- [x] Implement scheduling, suspend result consumption while paused, and persist a source-bound checkpoint. Use existing single-page comparison exclusively.
- [x] Run targeted state tests.

### Task 2: Blocking dialog and entry icons
Files: proofread_ocr_batch_ui.go, proofread_ocr_batch_ui_test.go, proofread_chrome.go, locales, assets/icons/home/scan-*.svg.
Interfaces: layoutProofreadingOCRBatchDialog and layoutProofreadingOCRBatchAction; consumes Task 1 state.
- [x] Write failing modal input and controls tests.
- [x] Add localized progress dialog, stable controls, blocking input, and page/stack scan icons; replace old toolbar entry.
- [x] Run interaction and real-pixel 1x/2x visual checks.

### Task 3: Integration validation and review
- [x] Run proofreading and locale regression plus git diff --check.
- [x] Restart from current source using mandated go run command, verify dialog controls and underlying input isolation with native UI.
- [x] Fresh review of scheduling and UI boundaries; fix findings and record evidence here.

## Execution ledger
- User authorized implementation after choosing the modal approach; execute inline without another approval round.
- Repository authorization takes precedence over skill defaults about worktrees and commits. No worktree or automatic commit.

- Task 1: complete — scheduler and checkpoint tests pass, including three cached pages through the unchanged single-page entry, edit invalidation, late results, ordinary differences and final-page stop.
- Task 2: complete — pointer/Escape shield, focused editor/scroll blocking, dialog controls, progress interpolation and bottom-right action geometry pass. Actual 1x/2x Metal icon/card checks passed (not skipped under the permitted GPU environment).
- Task 3: complete — final `go test ./internal/desktopui -run '^TestProofreading' -count=1` passed (14.368s); `go test ./internal/i18n -count=1` and `git diff --check` passed.
- Independent review found adjacent-page OCR errors were silently converted into boundary skips in the shared path. A failing regression reproduced it; the shared result handler now exposes failure and pauses for retry. Cancelling required boundary credentials likewise no longer skips the boundary.
- Live verification: restarted from current source with the mandated command; paused P892 at 131/183 characters, clicked the shielded P891 without navigation, viewed pages, reopened the paused task and continued to completion. Then started at P891, observed processing P891 and automatic transition to P892; final summary completed 2 pages. Pages remain unreviewed. Full 892-page processing was not rerun.
- Live QA found untranslated argument placeholders and actions floating above the dialog footer; failing regressions were added and both fixed before final source launch.
- Ruling: existing per-page boundary mutation invalidation now retains comparisons before the first changed page in the shared pipeline. Otherwise later processing would erase prior completed evidence; no separate batch comparison path is introduced.
