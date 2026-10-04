# Word Shared Page Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make continuous and print Word views draw and edit the same canonical paginated page scenes, with reliable zoom, pan, and bounded interaction state.

**Architecture:** Keep one accepted `wordlayout.Result` per shared document session and render both page presentations from it. Add page-space transforms and visible-page hit regions, route edits into existing persistent session editors/drafts, and include draft epochs in background layout identity. Release cancelled layout work and large cached scenes only when the shared session is finalized.

**Tech Stack:** Go, Gio (`widget.Editor`, `widget.List`, `gesture`), `internal/wordlayout`, desktop UI shared document sessions.

**Spec:** `docs/superpowers/specs/2026-10-04-word-shared-page-rendering-design.md`

## Global Constraints

- Keep third-party Office library types behind format adapters.
- Keep Gio widgets in stable session state and run shaping/pagination outside frame layout.
- Keep the page geometry in points; apply one reversible page-to-screen transform for drawing and pointer input.
- Keep editor/hit-test work bounded to visible pages plus the focused run and outstanding drafts.
- Reject stale background layouts by document ID, committed revision, draft epoch, and page-setup key.
- Preserve text drafts and dirty state on layout or edit failure.
- Do not change web-flow geometry or unsupported-content/save boundaries as part of this work.
- Preserve all pre-existing staged and unstaged user changes; do not create a worktree.
- Do not add or run tests unless the user asks. Use formatting and diff-integrity checks only.

## Review Focus

- Editing at a page boundary while a layout is in flight must preserve the latest draft and rune caret.
- A stale or cancelled layout result must not replace current pages or retain a finalized session.
- Long documents must not accumulate per-run editors or hit regions as pages are scrolled.
- Manual zoom and horizontal/vertical pan must use inverse transforms for selection and image gestures.
- Continuous view must retain page-local styling, cell fills, borders, image bounds, and text wrapping from the accepted scene.

---

### Task 1: Version and bound the canonical page-layout snapshot

**Files:**
- Modify: `internal/desktopui/word_pagination.go`
- Modify: `internal/desktopui/session_state.go`
- Modify: `internal/desktopui/word_layout.go`

**Interfaces:**
- Keep the `wordLayoutKey` identity and extend it with `DraftEpoch uint64`.
- Add a projection helper that overlays the immutable `wordRunDrafts` snapshot into cloned `document.WordBlock` text runs without mutating the adapter or committed blocks.
- `startWordPagination(state)` schedules only when `{DocumentID, Revision, DraftEpoch, PageSetupKey}` differs from the accepted/pending key.

- [ ] Extend layout keys and equality/staleness checks to include the draft epoch; capture the epoch with the same projection snapshot as blocks, committed text, and drafts.
- [ ] Project drafts into cloned layout blocks and allow pagination during ordinary text editing; continue deferring only while a flush, format operation, or reset makes the snapshot unsafe.
- [ ] Keep one cancellable layout request per session, discard stale results before acceptance, and ensure a cancelled worker cannot publish a result that pins the session after finalization.
- [ ] Keep the last accepted scene available while a replacement computes; replace it atomically only after its full identity matches current state.

**Review:** Verify by code inspection that every result acceptance path checks all four key components and that draft projection touches only cloned neutral blocks.

### Task 2: Present the same page scenes in continuous and print modes

**Files:**
- Modify: `internal/desktopui/word_navigation_pane.go`
- Modify: `internal/desktopui/word_page_view.go`
- Create: `internal/desktopui/word_page_presentation.go`
- Modify: `internal/desktopui/session_state.go`

**Interfaces:**
- Add `layoutWordPages(gtx, state, presentation)` as the shared entry for both paginated presentations.
- `presentation` selects only surface/shadow, page gap, and single-/multiple-page arrangement; page-local scene geometry always comes from `state.wordLayoutResult`.
- Preserve `layoutWordCanvas` for web layout and any nonpaged content fallback.

- [ ] Extract page row construction and page-surface drawing from print view into the shared page presenter.
- [ ] Route continuous view through the accepted `wordlayout.Result` and stack pages with compact separators; keep print surfaces, shadows, and print gaps as presentation-only choices.
- [ ] Use a single page list/scene reference for both modes and preserve the active page when switching modes.
- [ ] Ensure the presenter reads fills, borders, glyphs, paragraph rules, and image placements from the same `PageScene` path in both modes.

**Review:** Confirm no continuous-mode function remeasures, wraps, or repositions page content; only page surface and inter-page spacing may differ.

### Task 3: Add page-space interaction and visible-page editing

**Files:**
- Create: `internal/desktopui/word_page_interaction.go`
- Modify: `internal/desktopui/word_page_view.go`
- Modify: `internal/desktopui/word_navigation.go`
- Modify: `internal/desktopui/word_layout.go`
- Modify: `internal/desktopui/session_state.go`

**Interfaces:**
- Add `wordPageTransform` with `PageToScreen` and `ScreenToPage` operations for points, based on visible page origin, density, and zoom.
- Add a per-frame `wordVisiblePageInteraction` snapshot containing only visible page rectangles and references/ranges for text fragments and image placements; replace it every frame rather than append to it.
- Add page-scene hit testing that returns `{RunID, RuneOffset}` from glyph advances/source offsets and image ID/bounds from image placements.

- [ ] Build visible interaction regions while the virtual page list lays out visible pages; clear them on early-return, document change, and mode transition.
- [ ] Route pointer down/drag through the inverse transform and scene geometry, then set/focus the corresponding persistent `widget.Editor` and rune caret.
- [ ] Render selection and caret overlays from the canonical fragment glyph positions so the visual position uses the same wrap and line geometry as page text.
- [ ] Preserve existing keyboard, clipboard, toolbar, and IME draft paths; schedule a new draft-epoch layout after editor changes and reconcile the caret by run ID/rune offset when it is accepted.
- [ ] Add image placement hit regions and route selection/gesture intents through page-space bounds so image interactions target the same IDs in both modes.
- [ ] Bound `wordRunEditors` to visible runs plus the focused/selected run and runs with outstanding drafts; before pruning, flush the latest editor text into its draft, and never prune the focused composition editor.

**Review:** Trace click, drag selection, keyboard edit, IME composition, and image hit paths to ensure all coordinates pass through the same transform and no page-scene geometry is duplicated per run.

### Task 4: Make zoom, fit, and two-axis panning consistent

**Files:**
- Create: `internal/desktopui/word_page_viewport.go`
- Modify: `internal/desktopui/word_page_view.go`
- Modify: `internal/desktopui/word_status_bar.go`
- Modify: `internal/desktopui/word_zoom.go`
- Modify: `internal/desktopui/session_state.go`

**Interfaces:**
- Add persistent viewport state for manual page offsets and fit-mode measurements.
- Use the Task 3 `wordPageTransform` in both rendering and hit testing; zoom changes presentation scale only.

- [ ] Separate fit-to-page/multiple-pages calculations from manual zoom and recompute fit only when viewport, page setup, or page arrangement changes.
- [ ] Add horizontal and vertical pan when scaled page bounds exceed the viewport; center each axis only while its page dimension fits.
- [ ] Preserve manual zoom and scroll offsets across frames, redraws, and continuous/print mode switches where the same page remains active.
- [ ] Apply pointer inverse transforms to text and image interactions after zoom/pan and clamp offsets to reachable page bounds.

**Review:** Check page corners and an interior caret at fit, below 100%, above 100%, and after panning both axes; displayed zoom must persist through a frame redraw.

### Task 5: Release document-sized layout and editor state safely

**Files:**
- Modify: `internal/desktopui/runtime.go`
- Modify: `internal/desktopui/document_session.go`
- Modify: `internal/desktopui/word_pagination.go`
- Modify: `internal/desktopui/session_state.go`

**Interfaces:**
- Add a final shared-session cleanup helper that cancels pending layout and releases the shared page result, pending/accepted keys, and page-count metadata after the final session view/adapter lifecycle has ended.
- Keep per-window widget state independent; closing one window must not clear a result still used by another view of the same shared document.

- [ ] Cancel pagination when the final shared session is finalized and release its large `wordlayout.Result` and associated keys/maps.
- [ ] Ensure cancelled workers stop promptly and cannot enqueue a closure retaining `documentSessionState` or the scene after shutdown.
- [ ] Clear each window's visible-page hit regions and prune safe editor entries when that view closes or changes document; preserve focused composition and dirty drafts until the existing flush/recovery path completes.
- [ ] Audit all error, stale-result, close-window, last-window, and shared-session paths for idempotent cleanup.

**Review:** Inspect reference ownership for one-window and multi-window documents and verify no cleanup can drop a scene still needed by a live sibling view.

### Task 6: Update compatibility and implementation notes

**Files:**
- Modify: `docs/office-support.md`
- Modify: `docs/office-library-evaluation.md` only if the implementation changes a format-library boundary.

- [ ] Record that supported continuous and print page presentations share `internal/wordlayout` geometry and list any remaining scene/layout limitations without claiming full Microsoft Word parity.
- [ ] Keep the compatibility boundary explicit for unsupported content, pagination, and editing interactions.

**Verification:** Run `gofmt` on changed Go files and `git diff --check`. Do not run or add tests unless the user asks.
