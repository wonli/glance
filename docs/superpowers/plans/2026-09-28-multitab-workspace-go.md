# Go/Gio Multi-tab Workspace Plan

This is the Go/Gio adaptation of `../tools/docs/superpowers/plans/2026-09-28-multitab-workspace.md` and its design spec. Preserve its user-visible lifecycle while using Glance Go's existing format adapters, Gio window, and local SQLite state.

## Rulings and architecture constraints

- The reference plan's Tauri, Rust, Svelte, and file paths do not exist here. Implement equivalent boundaries in Go packages and Gio; do not introduce a second UI stack.
- Keep a permanent Recent workspace tab, open document sessions as independent tabs, and retain per-tab view state while inactive.
- Persist current adapter snapshots and tab/recent metadata in SQLite. A failed persistence write must remain visible and must not clear dirty state.
- Keep existing source-file writes explicit. Drafts use Save As; cancellation leaves draft and snapshot intact.
- Native Gio currently reports `app.DestroyEvent` after the OS has closed the window; no cancellable pre-close event is exposed by Gio. Rely on durable session snapshots for crash/close recovery and document this platform limitation instead of pretending to show a close confirmation.
- Folder open is shallow: show supported direct children in Recent; no recursive scan.
- Preserve existing bounded spreadsheet rendering and asynchronous file opens. Do not duplicate viewer widgets or persist transient Gio widget internals.

## Tasks

### Task 1: Workspace store

- Add a local `internal/workspace` SQLite store with versioned schema, recent records, ordered tab records, active tab, and opaque document snapshots.
- TDD: migration, workspace ordering, payload/dirty round-trip, and rollback on transaction failure.
- Store lives under the user's config directory; database startup errors are visible.
- [ ] Step 1: Add failing tests in `internal/workspace/store_test.go` for opening/migrating an empty DB, metadata and payload round-trip, transactional ordering/active tab, and rollback on a failed transaction.
- [ ] Step 2: Run `go test ./internal/workspace`; verify it fails because the store API is missing.
- [ ] Step 3: Implement a `database/sql` store using the already-present `modernc.org/sqlite`, with schema versioning and transactional `ReplaceWorkspace`/`SaveDocument` methods.
- [ ] Step 4: Run `go test ./internal/workspace`; verify the new tests pass.
- [ ] Step 5: Commit `feat: add sqlite workspace store`.
- Expected: workspace records and byte snapshots survive close/reopen; transaction errors leave prior state intact.

### Task 2: Adapter snapshots and drafts

- Add byte snapshots/restoration and blank XLSX/DOCX documents behind their existing format packages.
- Add a minimal JSON text document with edit, snapshot, restore, and Save As support.
- TDD: round-trip current edits/styles, blank document creation, malformed payload rejection, and save failure preservation.
- [ ] Step 1: Write adapter-level failing snapshot/restore tests for XLSX and DOCX; add JSON adapter tests.
- [ ] Step 2: Run focused tests and record the missing contracts.
- [ ] Step 3: Implement byte snapshot/restore and blank constructors behind format adapters; implement minimal JSON editor model.
- [ ] Step 4: Verify malformed snapshots and failed saves preserve live dirty state; run `go test ./internal/format/...`.
- [ ] Step 5: Commit `feat: add document snapshots and drafts`.
- Expected: snapshots are self-contained source-format packages; format library types remain inside adapters.

### Task 3: Session collection and recovery

- Move active document/view state into a persistent per-document session object and let `App` switch the active pointer without closing inactive sessions.
- Restore tabs and their snapshots before they become interactive; keep Recent permanent and represent failed/missing source files without discarding snapshots.
- Persist successful edits, sheet changes, tab order, and active tab; closing a tab closes only its document adapter.
- TDD: independent active state, close isolation, missing-source restore, and restart recovery.
- [ ] Step 1: Write session collection tests for switching, close isolation, missing paths, and snapshot recovery.
- [ ] Step 2: Implement neutral workspace/session types and durable restore/persist boundaries.
- [ ] Step 3: Verify session tests and `go test ./internal/...`.
- [ ] Step 4: Commit `feat: persist and restore document sessions`.
- Expected: inactive sessions retain independent document and view state; restoring an unavailable source does not discard its snapshot.

### Task 4: Gio workspace UI and file commands

- Add horizontally scrolling document tabs, close controls, dirty markers, and a plus menu for file/folder open and blank Excel/Word/JSON drafts.
- Add shallow folder listing, native Save As selection, extension validation, and recoverable cancellation/failure behavior.
- Render Recent entries, restored drafts, JSON text drafts, and draft Word content; keep existing DOCX files on the existing preview path.
- TDD: permanent Recent tab, switching/closing, horizontal overflow, menu actions, Save As cancellation, and ribbon/grid viewport sizing.
- Update architecture/support documentation and run full tests, vet, and platform builds supported by the repository.
- [ ] Step 1: Add focused tests for permanent Recent, independent tab activation/close, and file-command cancellation/extension validation.
- [ ] Step 2: Implement the session-backed tab strip and Gio file/folder/draft commands with persistent widget state.
- [ ] Step 3: Verify all workspace interaction tests, `go test ./...`, and `go vet ./...`.
- [ ] Step 4: Update architecture, support, and compatibility documentation; document Gio's close-event limitation.
- [ ] Step 5: Commit `feat: add multi-tab document workspace`.
- Expected: existing spreadsheet grid remains bounded and responsive, Recent is permanent, and every live session is independently recoverable.
