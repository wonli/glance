# Markdown Typography and Emoji Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add shared semantic line-height tokens, increase Markdown source and preview line spacing, and fix Markdown preview emoji presentation and spacing.

**Architecture:** Keep font family selection and bundled font resources in the shared font manager, and put semantic line-height values in the existing `Typography` design tokens. Apply the body line-height token to Markdown editor and preview rows first. Keep emoji shaping in the normal text run: the manager provides one normalized emoji face whose proportional space metric is shared by Gio rendering and static PDF bitmap extraction.

**Tech Stack:** Go, Gio v0.10.3, headless Gio rendering tests, embedded Noto Color Emoji.

**Spec:** User request in this Codex thread, 2026-10-23.

## Global Constraints

- Make all edits, tests, builds, launches, and verification in `/Users/mima1234/project/glance-go`.
- Do not create or switch Git worktrees.
- Reuse existing Go caches and do not leave build artifacts, screenshots, or temporary fixtures.
- Preserve existing visual design except for the requested Markdown line-height increase and emoji corrections.
- Do not alter Word, Excel, or other document-native line spacing in this change.

## Review Focus

- Multiline Markdown selections with CJK and emoji have non-overlapping highlight rows after the shared body line-height is applied.
- Emoji without whitespace adjacent to CJK stays at the expected inline advance and uses a color glyph.
- Markdown code spans, links, and ordinary text retain their existing typeface chain and spacing.
- The warning triangle U+26A0 FE0F renders as a color emoji in the Markdown preview.
- The line-height token has a stable default and participates in typography token fallback.

---

### Task 1: Shared body line-height token and Markdown layout

**Files:**
- Modify: `internal/desktopui/design_tokens.go`
- Modify: `internal/desktopui/markdown_layout.go`
- Modify: `internal/desktopui/markdown_preview.go`
- Test: `internal/desktopui/design_tokens_test.go`
- Test: `internal/desktopui/markdown_layout_test.go`

**Interfaces:**
- Produces `Typography.BodyLineHeightScale float32`, defaulting to `1.5`.
- Markdown editor and preview use the shared scale for body line placement.

- [ ] **Step 1: Write tests** for the default token and for disjoint selected row bounds after Markdown editor layout.
- [ ] **Step 2: Run the targeted tests and confirm the expected failures.**
- [ ] **Step 3: Add the token and apply it to Markdown editor and preview rows.**
- [ ] **Step 4: Run the targeted typography and Markdown layout tests.**

### Task 2: Markdown emoji fallback and inline metrics

**Files:**
- Modify: `internal/desktopui/markdown_preview.go`
- Test: `internal/desktopui/markdown_visual_test.go`

**Interfaces:**
- Moves the Gio emoji adapter under `fontmanager` and normalizes the bundled face's U+0020 metric in `fontmanager/builtin` so renderer adapters share the same resource bytes.

- [ ] **Step 1: Write a failing regression** that renders `⚠️安全` through the Markdown preview and checks for a yellow glyph and bounded visual gap before the text.
- [ ] **Step 2: Run the visual regression and confirm it fails on the current implementation.**
- [ ] **Step 3: Put normalized emoji resource ownership in fontmanager and retain normal Markdown text runs.**
- [ ] **Step 4: Run the visual regression at 1x and 2x and inspect its rendered pixels.**

### Task 3: Verification and report

**Files:**
- Modify: `docs/architecture.md` if the shared token ownership changes the documented typography boundary.

- [ ] Run targeted Markdown and typography tests in the authorized directory.
- [ ] Run `gofmt` and `git diff --check`.
- [ ] If live UI validation is available, stop the existing Glance process and launch the current checkout with the repository-approved command; report the actual UI result separately from headless test results.
