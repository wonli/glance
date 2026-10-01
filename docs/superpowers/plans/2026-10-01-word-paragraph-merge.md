# Word Paragraph Boundary Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement safe Backspace/Delete paragraph joins at text boundaries for DOCX body and same-cell paragraphs.

**Architecture:** Add a neutral optional join capability whose DOCX implementation merges adjacent paragraph XML and returns the caret target. The Gio editor intercepts only unmodified boundary keys, flushes drafts, invokes the adapter asynchronously, then refreshes the projection and focus.

**Tech Stack:** Go, Gio `widget.Editor`, neutral `internal/document` interfaces, DOCX OOXML adapter, ZIP round-trip tests.

**Spec:** `docs/superpowers/specs/2026-10-01-word-paragraph-merge-design.md`

## Global Constraints

- Keep third-party Office library types behind format adapters.
- Keep parsing and saves outside Gio layout/frame code; publish results through synchronized application state.
- Reject ambiguous or unsupported XML joins without mutation; preserve dirty state and drafts on failure.
- Do not claim overwrite safety without save/reopen tests that preserve edited and representative untouched content.

## Review Focus

- Unicode caret offsets: adapter join tests assert rune offsets, not byte offsets.
- Inline pictures at the seam: adapter tests verify order and caret placement when the left paragraph ends in a picture.
- Table boundaries: tests prove joins stay inside one `w:tc` and reject neighboring cells.
- Unsupported sibling blocks and section properties: tests prove no cross-boundary mutation.
- Pending drafts and key modifiers/selections: Gio tests prove drafts flush before joins and ordinary delete gestures remain unchanged.

---

### Task 1: Neutral API and transactional DOCX joins

**Files:**
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_paragraph_edit.go`
- Test: `internal/format/docx/word_text_test.go`
- Test: `internal/format/docx/word_history_test.go`

**Interfaces:**
- Produce `document.WordParagraphJoiner.JoinParagraphAtBoundary(runID string, direction WordParagraphJoinDirection) (WordParagraphJoinResult, error)`.
- `WordParagraphJoinDirection` has `WordJoinPrevious` and `WordJoinNext`; `WordParagraphJoinResult` carries `ParagraphID`, `RunID`, and rune `Caret` offset.
- The adapter retains left paragraph properties and ordered raw inline content; it inserts an empty editable run at the seam when no left-side text run can host the caret.

- [x] **Step 1: Write failing adapter tests** for previous/next body joins, same-cell joins, mixed formatting/image order, seam caret target, unsupported/cross-container rejection, save/reopen, and undo/redo.
- [x] **Step 2: Run** `go test ./internal/format/docx -run 'TestJoinParagraph' -count=1` and confirm failures are caused by the missing join capability/behavior.
- [x] **Step 3: Implement** the neutral types and transactional `JoinParagraphAtBoundary` in the DOCX adapter; reject non-adjacent or unsafe XML before mutation.
- [x] **Step 4: Run** the targeted join tests, then `go test ./internal/format/docx ./internal/document -count=1`.

### Task 2: Gio boundary-key handling and caret restoration

**Files:**
- Modify: `internal/desktopui/word_layout.go`
- Modify: `internal/desktopui/word_editing.go`
- Modify: `internal/desktopui/session_state.go`
- Modify: `internal/desktopui/layout.go`
- Test: `internal/desktopui/word_native_regression_test.go`

**Interfaces:**
- Consume `document.WordParagraphJoiner` and its `WordParagraphJoinResult` from Task 1.
- Add a session-owned merge intent/result path parallel to paragraph split handling; adapter calls remain asynchronous.

- [x] **Step 1: Write failing Gio router tests** for Backspace-at-start and Delete-at-end joins, caret at the seam, draft flush, interior deletion, non-collapsed selections, modifiers, and absent adjacent paragraphs.
- [x] **Step 2: Run** `go test ./internal/desktopui -run 'TestWord.*Join' -count=1` and confirm failures represent the missing key behavior.
- [x] **Step 3: Implement** boundary-only key interception before `widget.Editor.Update`, serialized draft flushing, asynchronous adapter invocation, projection refresh, and focus/caret restoration.
- [x] **Step 4: Run** the targeted Gio tests and `GLANCE_OFFICE_TEST_DOCX='/Users/mima1234/Downloads/office/大猫OA软件使用说明.docx' go test ./internal/desktopui -run '^TestWord' -count=1`.

### Task 3: Support evidence and final verification

**Files:**
- Modify: `docs/office-support.md`
- Modify: `../tools/docs/compatibility/office-feature-matrix.md`

- [x] **Step 1: Update** the DOCX editing row with the tested paragraph-join boundary and known rejection limits.
- [x] **Step 2: Update** the Glance Go compatibility matrix with the adapter and Gio regression evidence.
- [x] **Step 3: Run** `go test ./internal/desktopui ./internal/format/docx ./internal/document -count=1` and `git diff --check`; report any unrelated failures by name.
