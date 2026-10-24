# Proofreading confirmation and text history Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make reconfirmation discoverable and show readable character changes in history.
**Architecture:** Reuse page content tokens for menu state. Read parent/current page snapshots on the history worker, publish neutral text spans, render those with stable selectable state and a raw-patch disclosure.
**Tech Stack:** Go, Gio, existing Git repository adapter; no new dependencies.
**Spec:** docs/superpowers/specs/2026-10-24-proofreading-confirmation-text-history-design.md

## Global Constraints
- Work only in /Users/mima1234/project/glance-go; reuse existing caches and leave no validation artifacts.
- Keep file reads and comparison outside Gio frames; preserve IME and confirmation save behavior.

## Review Focus
- Multiple separated Chinese edits and repeated characters must reconstruct both source texts exactly.
- Long unrelated text must not allocate quadratic memory.
- First/deleted page and metadata-only commits must not invent text changes.
- Saving/error/late confirmation must not show changed content as confirmed.
- Wrapped deleted text must retain visible strike and disclosure interactions at 1x and 2x.

### Task 1: Confirmation menu
- [x] Add failing menu-state regression for confirmed, edited, saving and failed states.
- [x] Reuse proofreadingPageStatusKey to set label, selected color, hint and enabled state; localize new strings.
- [x] Run focused confirmation/menu regressions.

### Task 2: Text history adapter
- [x] Add failing Unicode and historical snapshot diff tests, including large text, no-op and reconstruction.
- [x] Add TextChange spans and bounded-memory rune comparison; load actual commit parent/current PageResult in PageChanges and preserve complete raw Diff.
- [x] Run proofhistory tests.

### Task 3: History presentation
- [x] Add persistent selectable/disclosure state, render contextual inline colors and deletion strikes, default raw data collapsed.
- [x] Add interaction and GPU checks at 1x/2x, rerun existing long-diff and history layout regressions.
- [x] Update architecture docs, check git diff --check; self-review the final change and report actual UI verification limits.

## Execution evidence
- Unicode reconstruction, separated Chinese deletions, initial/deleted/metadata-only snapshots, and long-text fallback passed in `go test ./internal/proofhistory -count=1`.
- `go test ./internal/desktopui -run '^TestProofreading' -count=1 -v` passed with Metal access, including 1x/2x confirmation menu click, wrapped red deletion strike, green additions, raw disclosure, existing history layout and IME regressions; no skips.
- Ruling: preserve full text for large unrelated replacements with a bounded Myers search (512 edits / 4,000,000 search steps); this avoids quadratic storage. For exceptionally dissimilar middles the full removed/added middle is displayed.
- User approval already authorizes implementation in the current workspace; no new worktree or additional design gate is needed. No implementation commit is made automatically.
