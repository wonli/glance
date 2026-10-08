# PPTX Text and Shape Style Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the bounded text, shape, and picture outline controls defined in [the style-editing design](../specs/2026-10-08-pptx-text-shape-style-editing-design.md), preserving every unrequested PPTX property through normal saves and recovery snapshots.

**Architecture:** Gio keeps selection, popovers, and transient style drafts. Neutral presentation DTOs carry validated optional style values. `internal/format/pptx` validates intents, projects pending overrides, and applies only requested DrawingML properties to the serialized target shape; its save and recovery pipelines verify the patched package before publication.

**Tech stack:** Go 1.27, Gio 0.10.2, pinned go-pptx v2.0.3, focused synthetic PPTX fixtures, existing presentation canvas and ribbon state.

**Design source:** `docs/superpowers/specs/2026-10-08-pptx-text-shape-style-editing-design.md`

## Current checkpoint (2026-10-08)

Object-wide style model, targeted persistence, rendering and ribbon code are present in the checkpoint. The original full acceptance criteria have not been met: multi-paragraph/soft-break editing, mixed-run editor layout, exact preview/edit parity, justify rendering and full asynchronous style-intent routing remain follow-up work. Existing unchecked steps must not be treated as a completion claim. See [the shared-components follow-up](2026-10-08-pptx-shared-editing-components.md).

## Task 1: Neutral style values and adapter mutation contract

**Files:**
- Modify: `internal/document/presentation.go`
- Modify: `internal/format/pptx/document.go`
- Test: `internal/format/pptx/inherited_test.go`

- [ ] **Step 1: Write failing adapter tests** for style patch validation, no-op/revision semantics, projected overlay values, unsupported targets, and slide add/delete lifecycle.
- [ ] **Step 2: Run the focused tests and inspect failures.**
- [ ] **Step 3: Add neutral optional text/paragraph/fill/line style fields** and a narrow `UpdateSlideElementStyle` adapter method that validates and stores an overlay atomically.
- [ ] **Step 4: Layer pending style values into `projectShape`** without exposing third-party types or mutating unsupported objects.
- [ ] **Step 5: Run focused adapter tests.**
- [ ] **Step 6: Commit** as `feat: add PPTX presentation style model`.

## Task 2: Targeted DrawingML style patches and save gates

**Files:**
- Modify: `internal/format/pptx/geometry.go`
- Modify: `internal/format/pptx/document.go`
- Modify: `internal/format/pptx/inherited_test.go`

- [ ] **Step 1: Write failing XML/save tests** for run font attributes, underline, paragraph alignment and spacing, solid/no fill, line color/width/no line, unknown-property preservation, and composed style+transform+crop changes.
- [ ] **Step 2: Run focused tests and inspect generated XML/package differences.**
- [ ] **Step 3: Extend the shape-ID patch pipeline** to patch only explicitly requested style attributes/subtrees while preserving unknown XML and non-target ZIP entries.
- [ ] **Step 4: Include styles in save expectations, reopening validation, atomic save, and `SnapshotBytes()`.** Verify validation failure preserves the original file and dirty state.
- [ ] **Step 5: Run focused adapter save/reopen and snapshot tests.**
- [ ] **Step 6: Commit** as `feat: persist PPTX text and shape styles`.

## Task 3: Canvas projection and style-preserving edit metrics

**Files:**
- Modify: `internal/desktopui/presentation_canvas.go`
- Modify: `internal/desktopui/presentation_rich_text.go`
- Modify: `internal/desktopui/presentation_state.go`
- Test: `internal/desktopui/presentation_workspace_test.go`

- [ ] **Step 1: Write failing UI tests** for preview style changes, line/fill rendering, text frame retention, edit draft retention, and no dirty state on merely entering edit mode.
- [ ] **Step 2: Run the focused UI tests and inspect failures.**
- [ ] **Step 3: Render projected style overlays** in the shared canvas/thumbnail/slideshow rendering path; keep paragraph metrics identical in preview and edit mode.
- [ ] **Step 4: Add persistent style-popover draft state** and stale-result guards where an asynchronous picker is required.
- [ ] **Step 5: Run focused rendering and editor interaction tests.**
- [ ] **Step 6: Commit** as `feat: render PPTX style edits in the canvas`.

## Task 4: Ribbon text, shape, and picture outline controls

**Files:**
- Modify: `internal/desktopui/presentation_view.go`
- Modify: `internal/desktopui/presentation_state.go`
- Create or modify: `internal/desktopui/presentation_style.go`
- Test: `internal/desktopui/presentation_workspace_test.go`
- Test: `internal/desktopui/presentation_interaction_test.go`

- [ ] **Step 1: Write failing action tests** for selected text/shape/picture availability, explicit mixed state, font/color/size/bold/italic/underline/alignment/spacing commands, shape fill/outline, and picture outline.
- [ ] **Step 2: Run focused UI tests and inspect failures.**
- [ ] **Step 3: Make the existing ribbon actions functional** using bounded reusable color/font/spacing controls and neutral intents; disable controls for unsupported content.
- [ ] **Step 4: Preserve text draft, active selection, and the single shared transform frame** while commands update the style snapshot; run package changes outside layout callbacks.
- [ ] **Step 5: Run focused ribbon, stale-result, and edit-transform tests.**
- [ ] **Step 6: Commit** as `feat: add PPTX style ribbon controls`.

## Task 5: Compatibility evidence and final verification

**Files:**
- Modify: `docs/office-support.md`
- Modify: `../tools/docs/compatibility/office-feature-matrix.md`
- Modify: `docs/office-library-evaluation.md` only if new dependency evidence changes the assessment
- Test: `internal/format/pptx` and `internal/desktopui` package suites

- [ ] **Step 1: Run focused adapter and UI suites** and inspect all output.
- [ ] **Step 2: Run `go test ./...`** and use disposable PPTX copies for save/reopen checks; verify source files remain byte-identical.
- [ ] **Step 3: Update support and compatibility docs** with only verified controls and explicit unsupported cases.
- [ ] **Step 4: Review the complete diff and run `git diff --check`.**
- [ ] **Step 5: Commit** as `docs: record PPTX style editing support`.

## Completion Contract

- Text style, paragraph spacing, fill, and line changes pass adapter projection tests and PPTX save/reopen plus recovery snapshot verification.
- Untouched object XML, geometry, image bytes, relationships, and representative package parts survive targeted style edits.
- The same edit/selection frame and transforms remain active; entering edit mode causes no style or dirty-state changes.
- `go test ./...` passes and support documentation matches the tested boundary.
