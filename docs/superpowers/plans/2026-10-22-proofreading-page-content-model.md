# Proofreading Page Content Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace offset-based proofreading with unique character ownership, reliable page editing, local adjustment and reversible pagination.

**Architecture:** A neutral proofreading model owns stable characters, pages and transactions. An application controller applies edits and versioned layout results. Gio consumes snapshots and maintains editor/viewport state; format adapters supply B content and A geometry.

**Tech Stack:** Go, Gio, existing wordlayout/font shaping, local versioned proofreading copy.

**Spec:** `docs/superpowers/specs/2026-10-22-proofreading-page-content-model.md` (user approved).

## Global Constraints

- Work in `/Users/mima1234/project/glance-go`; do not create or change worktrees.
- Preserve A and source B. No OCR, Git history, new comparison formats or overall Pen redesign.
- Unique character identity and Page ownership include whitespace and newlines; no persistent page-break positions.
- Ordinary pages reflow; locked/reviewed pages isolate their content. Reviewed pages permit local editing but forbid transfer and pagination.
- A-area scrolling moves A/X together. X-page scrolling only views that page's owned content. X-canvas margin scrolling adjusts the active locked page; with no active lock it moves A/X together.
- Preserve existing unrelated modifications; remove superseded proofreading paths only when their replacements are connected.
- Parsing, file I/O and shaping run outside Gio frame layout; reject stale asynchronous results.

## Review Focus

- Several Enter edits followed immediately by wheel input must not overwrite unconsumed editor changes.
- Split while locked, then reverse wheel, must not restore the old pre-split baseline.
- IME, combining characters and emoji must retain valid Unicode and native selection semantics.
- Empty pages, trailing source content and barriers must not cause dropped or duplicated characters.
- Save failure, old corrupt metadata and late layout responses must preserve recoverable content.

## Task 1: Neutral character ownership and transactions

**Files:** Create `internal/proofreading/model.go`, `operations.go`, `history.go`, and corresponding tests.

**Interfaces:**
- `CharacterID`, `PageID`: stable integer identities, serialized explicitly.
- `Character { ID CharacterID; Value rune; Page PageID }` defines logical semantics; internal runs may compress contiguous identities.
- `PageState` identifies ordinary, locked and reviewed state. Locked baselines reference identities rather than numerical text offsets.
- `Snapshot` includes ordered characters, ordered pages, page states, revision and next identity.
- `New(text string, pages []PageID) (*Model, error)` retains all source characters; initial ownership comes from validated layout in Task 2.
- `(*Model).Snapshot() Snapshot` returns an immutable view.
- `(*Model).Apply(Operation) error`, `Undo() bool`, `Redo() bool` implement atomic state changes.
- Operations: page-local replace using stable boundary anchors, lock/unlock, review/unreview, transfer to adjacent page and split up/down. Expected revision rejects stale input.

- [ ] Write failing operation tests: unique identity through edits, newlines belong to insertion page, transfer changes Page without creating characters, ordinary/locked/reviewed permission matrix, empty-page insertion, source tail retained.
- [ ] Run `go test ./internal/proofreading -count=1`; observe failures before implementation.
- [ ] Implement the interfaces, validation and transactional history. Preserve IDs of unchanged characters during replacement; create IDs only for inserted text. Keep deletion recoverable through history. Reject operations before mutating state when a barrier or revision check fails.
- [ ] Add sequence tests: lock → Enter edits → transfer forward/back → split down → edit → undo all → redo all. Check text, IDs, order, ownership and page state at every step. Splitting deliberately resets the remaining locked baseline.
- [ ] Run the model tests and inspect outcomes before proceeding.

## Task 2: Real layout mapping and controller

**Files:** Create `internal/application/proofreading_controller.go`, `proofreading_layout.go` and tests; add focused mapping helpers under `internal/wordlayout` only if existing source mapping is insufficient.

**Interfaces:**
- `ProofreadingController` owns a `*proofreading.Model`, layout generation and immutable published snapshots.
- `Dispatch(op proofreading.Operation) error` applies the command and requests appropriate layout.
- `ProofreadingLayoutSnapshot` records revision, style revision, per-page lines, character boundary anchors and content height in document units.
- `Reflow(ctx context.Context, snapshot proofreading.Snapshot, geometry []ProofreadingPageGeometry, style ProofreadingTextStyle) (ProofreadingLayoutSnapshot, error)` uses existing shaping/source mapping.
- `AdjustLockedPage(page proofreading.PageID, viewportTop, viewportHeight, delta float64) error` uses actual line geometry and stable anchors; no fixed pixels-to-characters ratio.

- [ ] Test ordinary reflow after edits/font changes, locked/reviewed overflow stays local, barriers stop flow, empty lines map to caret boundaries, and all source tail characters survive.
- [ ] Implement initial assignment and ordinary contiguous-flow reflow using the internal engine. Retain existing assignments for independent pages.
- [ ] Implement locked viewport scrolling and forward/reverse boundary adjustment. Scroll existing content before acquiring more; protect baseline and newly inserted IDs; transfer only contiguous eligible following content. Respect reviewed barriers. Rebase baseline after explicit split.
- [ ] Test delayed results: edit/lock/split/undo while a prior layout is pending; reject old revisions. Test physical scaling does not change which logical boundary is selected.
- [ ] Run `go test ./internal/application ./internal/proofreading -run 'Proofreading' -count=1`.

## Task 3: Versioned copy and safe migration

**Files:** Create `internal/proofreading/codec.go` and tests; modify `internal/desktopui/proofread_edit.go`, `proofread_window.go`.

**Interfaces:** `Encode(snapshot Snapshot) ([]byte,error)` and `Decode(data []byte) (Snapshot,error)` own version 2 payloads. A separate legacy decoder validates existing text/ranges before producing a new model. Keep path selection and asynchronous temporary-file replacement in existing persistence code.

- [ ] Add save/reopen tests for stable IDs, page state, locked baseline and full Unicode text. Test invalid IDs, duplicate IDs, missing Page references and malformed data fail without mutation.
- [ ] Add legacy migration fixtures: valid ownership, untouched drafts, overlapping ranges, corrupt metadata and trailing content. Never infer deletion from visually repeated text.
- [ ] Implement v2 encoding and explicit legacy migration; preserve original copy before first migrated write and never overwrite it on failure. Remove manual break metadata from new snapshots.
- [ ] Test write/replace failures retain dirty state and last valid copy; test stale async save results do not mark newer edits saved.
- [ ] Run focused codec and persistence tests.

## Task 4: Replace Gio editing and menu commands

**Files:** Modify `internal/desktopui/proofread_window.go`, `proofread_edit.go`, `proofread_projection.go`, `proofread_page_menu.go`, `proofread_interaction.go`, `proofread_pagination_interaction.go`; add `proofread_controller.go`, `proofread_editor_events.go`, `proofread_history.go`.

**Interfaces:** Persistent page UI state is keyed by PageID. It owns Editor, internal viewport and snapshot revision, but not content ownership. A preprocessing event pass consumes editor changes before page/menu/wheel commands; render callbacks draw published state and enqueue intents only. Page editors synchronize only on relevant external revisions and preserve stable caret anchors and viewport.

- [ ] Write Gio regressions for click focus/input, several Enter inputs immediately followed by wheel, native selection and Chinese input, and stale editor values after split/undo.
- [ ] Replace draft text splicing and range rebuilding with controller dispatch. Flush editor text before commands, avoid committing intermediate IME composition, and prevent editor-local undo from conflicting with controller history.
- [ ] Connect Pen iAozW actions and shared undo/redo. Hide/disable pagination and transfer for reviewed pages. Persist actions through the v2 path.
- [ ] Keep stable widgets and explicit vertical list axes. Do not call SetText for unrelated frame changes or perform model mutation while drawing.
- [ ] Remove legacy `manualBreaks`, offset-based page ownership, global B alignment as an ownership authority, pagination-only undo snapshots and persistent manual-break marker drawing after replacement is connected. Keep source/geometry adapters and original-file safety.
- [ ] Run `go test ./internal/desktopui -run 'Proofreading' -count=1`.

## Task 5: Page-local scroll and geometry

**Files:** Modify `proofread_interaction.go`, `proofread_projection.go`, `proofread_edit.go`; add `proofread_scroll.go` and interaction tests.

**Interfaces:** Pointer input is normalized to page/document coordinates once. A-area scroll mirrors A/X. X-page viewports have independent scrollbars and offsets that never change character ownership. X-canvas margin scroll uses shaped line boundaries to adjust the active locked page; with no active lock, it mirrors A/X.

- [x] Test page-local wheel versus X-exterior adjustment, A-to-X scroll mirroring, locked ownership adjustment, page scrollbar drag/range, and no-lock exterior synchronization. Regress upward scrolling at adjustment top so it cannot acquire following text.
- [x] Route wheel and draggable page scrollbars inside X pages to local viewing only. Route X-exterior scrolling to the active lock's line-based ownership adjustment; without an active lock, preserve A/X sync. Preserve editor clicks with non-blocking observer regions. Remove Ctrl/Command-specific behavior.
- [x] Keep persistent whole-page lock outline and inward fade, drawing under text. Unlock/review ends adjustment. Use actual viewport offset and line anchors for click/caret pagination.
- [x] Test view position remains stable during content transfer and exact split location after internal scrolling. Reviewed-page viewing does not change ownership.
- [ ] Verify scroll routing with the menu open and in a live native window at 1x/2x; headless Metal and temporary app launch are unavailable in the current environment.
- [ ] Run focused input and geometry regressions.

## Task 6: Integration and actual-window acceptance

**Files:** Add integrated proofreading regressions, update `docs/architecture.md` and a focused verification record under `docs/`.

- [ ] Exercise the spec's nine acceptance groups end-to-end, including reload, barriers, forward/reverse adjustments, split during lock and full undo/redo in the native window.
- [ ] Run model/controller/codec tests and targeted Gio tests; then `go build ./...` and `git diff --check`. Broaden tests only for touched shared paths or unresolved failures.
- [ ] Exit the old process before launching the newly built app. Use synthetic A/B documents to verify actual click/edit/Enter/wheel/split/undo/redo and independent reviewed pages. Verify whole-page highlight and menu typography at normal/Retina scale.
- [ ] Record exact evidence and remaining limits; a skipped GPU test or successful build is not UI validation. Preserve user copies and do not automatically remove corrupted content from earlier implementations.

No commits or worktree operations are part of this plan without separate user direction. Execute in this chat, task by task; do not delegate unless authorized.
