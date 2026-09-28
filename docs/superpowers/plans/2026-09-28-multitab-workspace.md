# Multi-tab Document Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a multi-document desktop workspace with SQLite-backed working document state, recoverable drafts, recent files, and horizontally scrolling tabs.

**Architecture:** Keep SQLite and filesystem operations behind Rust Tauri commands. Give each document adapter a serializable working-state boundary so SQLite can restore current edits independently of original paths; let the Svelte workspace own tab order and lazily mounted viewer instances. Preserve existing adapter/session boundaries and route all saves through validated native destination selection.

**Tech Stack:** Svelte 5, TypeScript, Tauri 2, Rust, rusqlite, serde/serde_json, existing document adapters.

**Spec:** `docs/superpowers/specs/2026-09-28-multitab-workspace-design.md`

## Global Constraints

- A permanent Recent tab is always present and cannot be closed.
- New documents are drafts. Their data is stored in SQLite; Save asks for a destination path and writes the selected document format there.
- SQLite is the durable source of truth for the app's current working document state, not merely a recent-file index.
- Existing local files retain their current explicit save semantics: SQLite persists the current working document state for recovery, while writing that state back to the source file still requires the existing Save action.
- File paths remain local-only and are not sent to a network service.
- The `+` menu offers opening a local folder, opening a local file, and creating a blank Excel, Word, or JSON document.
- JSON formatting, collapse, and expand UI is not part of this change.
- Ribbon content must never expose a vertical scrollbar. Horizontal overflow remains available for narrow windows.
- Do not persist transient viewer data such as a live DOM tree, component instances, or browser selection objects.
- Closing one tab closes only that backend session. Closing the window checks all dirty sessions before exiting.

## Review Focus

- Existing source path is missing or changed when restoring: preserve the SQLite working copy and mark the source unavailable without discarding edits.
- SQLite write fails during an edit: keep the live session usable and surface a recoverable persistence error; never report the edit as durably saved.
- Save dialog is cancelled: retain the draft and all document state with no filesystem write.
- Destination extension does not match document format: reject before writing and keep the draft/session dirty.
- Several dirty tabs exist at window close: prompt once for the complete set and close only after every required state is safely persisted or explicitly discarded.

---

## File Map

- Create `apps/desktop/src-tauri/src/workspace_store.rs`: SQLite schema, migrations, and typed CRUD operations for workspace records and serialized document state.
- Modify `apps/desktop/src-tauri/src/commands.rs`: attach the store to `DesktopCommands`; restore, persist, create, open, save, close, and enumerate document sessions.
- Modify `apps/desktop/src-tauri/src/lib.rs`: initialize the database in Tauri app data and register workspace commands.
- Modify `crates/app-core/src/adapter.rs`: add a serialization/restoration boundary for current adapter state and blank-document creation where needed.
- Modify `crates/format-json/src/lib.rs` and `crates/format-json/src/tree.rs`: initialize JSON documents from pasted text and serialize current JSON state.
- Modify `crates/format-xlsx/src/lib.rs` and `crates/format-xlsx/src/workbook.rs`: create blank workbooks and serialize edited workbook state for SQLite and Save As.
- Modify `crates/format-docx/src/lib.rs`: preserve source package bytes, create a blank DOCX draft, and serialize its current state.
- Create `apps/desktop/src/lib/components/WordDraftEditor.svelte`: provide paragraph editing for newly created Word drafts while existing DOCX files remain in the current read-only viewer.
- Modify `apps/desktop/src/lib/bridge/documents.ts` and `apps/desktop/src/lib/types/documents.ts`: define typed Tauri workspace commands and tab/document records.
- Modify `apps/desktop/src/App.svelte`: own tabs, restore workspace, route activation/open/close/save, and protect all dirty sessions on window close.
- Create `apps/desktop/src/lib/components/WorkspaceTabs.svelte`: Recent tab, document tabs, horizontal scrolling, close/activate, and plus menu.
- Modify `apps/desktop/src/lib/components/OpenFileView.svelte`: render recent files, restored drafts, folder contents, and empty-state actions.
- Modify `apps/desktop/src/lib/components/SpreadsheetViewer.svelte` and shared ribbon styles in `apps/desktop/src/style.css`: ensure ribbon only scrolls horizontally.
- Modify `apps/desktop/src/lib/i18n/locales/zh-CN.ts` and `apps/desktop/src/lib/i18n/locales/en-US.ts`: all new menu, tab, recent, draft, persistence, and error labels.
- Create/modify focused Rust and Vitest tests alongside the above modules; extend `apps/desktop/src/App.test.ts` for workspace interaction.

## Task 1: SQLite Workspace Store

**Files:**
- Modify: `Cargo.toml`
- Modify: `apps/desktop/src-tauri/Cargo.toml`
- Create: `apps/desktop/src-tauri/src/workspace_store.rs`
- Modify: `apps/desktop/src-tauri/src/lib.rs`
- Test: `apps/desktop/src-tauri/tests/workspace_store.rs`

**Interfaces:**
- Produces `WorkspaceStore::open(path: &Path) -> Result<WorkspaceStore, WorkspaceStoreError>`.
- Produces transactional methods for upserting workspace metadata, tab order/active tab, recent paths, and `(document_id, format, payload, dirty, updated_at)` serialized state.
- Produces `WorkspaceStore::load_workspace() -> Result<WorkspaceSnapshot, WorkspaceStoreError>` and `delete_document_state(document_id)`.

- [ ] **Step 1: Write failing SQLite tests** named `migrates_empty_database`, `round_trips_workspace_and_document_payload`, `updates_tab_order_transactionally`, and `keeps_database_consistent_after_failed_transaction`. Assert app state, path, payload bytes, dirty state, and active tab values exactly.
- [ ] **Step 2: Run** `cargo test -p desktop-app --test workspace_store`; confirm the new module/test target fails because the store is not implemented.
- [ ] **Step 3: Add rusqlite with bundled SQLite and implement schema version 1.** Use tables for recent files, tabs/workspace settings, and document state; use foreign keys and transactions for coupled updates.
- [ ] **Step 4: Initialize the database under Tauri's app data directory** in `lib.rs`; fail initialization visibly instead of silently starting with an empty workspace.
- [ ] **Step 5: Run** `cargo test -p desktop-app --test workspace_store`; confirm all store tests pass.
- [ ] **Step 6: Commit** as `feat: add sqlite workspace state store`.

## Task 2: Adapter Snapshot and Draft Contract

**Files:**
- Modify: `crates/app-core/src/adapter.rs`
- Modify: `crates/app-core/src/lib.rs`
- Modify: `crates/format-json/src/lib.rs`
- Modify: `crates/format-json/src/tree.rs`
- Modify: `crates/format-xlsx/src/lib.rs`
- Modify: `crates/format-xlsx/src/workbook.rs`
- Modify: `crates/format-docx/src/lib.rs`
- Test: `crates/format-json/tests/json_adapter.rs`
- Test: `crates/format-xlsx/tests/xlsx_adapter.rs`
- Test: `crates/format-docx/tests/docx_adapter.rs`

**Interfaces:**
- Add `FormatAdapter::snapshot(&DocumentHandle) -> Result<Vec<u8>, AppError>` to serialize current working document state.
- Add `FormatAdapter::edit_word_draft(&DocumentHandle, paragraphs: &[String]) -> Result<(), AppError>`; the default implementation returns unsupported operation.
- Add registry-level `create_blank(kind: DocumentKind) -> Result<(Arc<dyn FormatAdapter>, OpenedDocument), AppError>` and `restore(adapter_id: &str, file_name: &str, snapshot: Vec<u8>, draft: bool) -> Result<(Arc<dyn FormatAdapter>, OpenedDocument), AppError>`.
- Define snapshots as self-contained format payloads accepted by the corresponding adapter's existing byte-source open path.

- [ ] **Step 1: Add failing round-trip tests** named `json_snapshot_restores_pasted_content`, `xlsx_snapshot_restores_committed_edits_and_styles`, `docx_snapshot_restores_package_content`, and `word_draft_edits_restore_paragraphs`; add `creates_blank_json_xlsx_and_docx_documents` asserting kind and capabilities.
- [ ] **Step 2: Run** `cargo test -p format-json -p format-xlsx -p format-docx`; confirm snapshot/draft contract tests fail.
- [ ] **Step 3: Implement adapter snapshots and blank constructors.** JSON serializes its current source/value; XLSX serializes the in-memory workbook to an in-memory package; DOCX preserves source package bytes for read-only opened files and creates a minimal package for blank drafts. A draft-only paragraph model is editable and serializes to a valid DOCX package. Keep serialization and file writing in adapters, not UI code.
- [ ] **Step 4: Implement registry dispatch for create and restore** using the adapter ID and self-contained bytes.
- [ ] **Step 5: Run** `cargo test -p format-json -p format-xlsx -p format-docx`; confirm new and existing tests pass.
- [ ] **Step 6: Commit** as `feat: add serializable document draft sessions`.

## Task 3: Persist Every Session Mutation and Restore Workspace

**Files:**
- Modify: `apps/desktop/src-tauri/src/commands.rs`
- Modify: `apps/desktop/src-tauri/src/workspace_store.rs`
- Modify: `apps/desktop/src-tauri/src/lib.rs`
- Test: `apps/desktop/src-tauri/tests/commands.rs`
- Test: `apps/desktop/src-tauri/tests/workspace_restore.rs`

**Interfaces:**
- Extend `DesktopCommands::new(picker, registry, store)` to own shared store access.
- Add `restore_workspace() -> Result<WorkspaceSnapshotDto, AppErrorDto>` and `create_draft(kind) -> Result<OpenDocumentDto, AppErrorDto>`.
- After a successful mutation or undo/redo, snapshot the adapter and transactionally update the SQLite state before returning success to the caller.

- [ ] **Step 1: Write failing lifecycle tests** named `edit_is_restored_after_commands_recreated`, `draft_survives_close_and_restart`, `failed_sqlite_snapshot_does_not_claim_durable_success`, and `restore_marks_missing_source_without_dropping_snapshot`.
- [ ] **Step 2: Run** `cargo test -p desktop-app --test workspace_restore`; confirm persistence and restore tests fail.
- [ ] **Step 3: Persist current session state** from every spreadsheet mutation, structure edit, sort, undo, redo, pasted JSON update, and draft operation. Return a persistence error if the edit cannot be durably committed, while retaining the in-memory session.
- [ ] **Step 4: Restore sessions at startup** by loading snapshot bytes through the registry; when a source path exists, retain it as Save destination metadata and mark missing paths unavailable without losing the snapshot.
- [ ] **Step 5: Ensure saved documents update database snapshot and dirty state only after successful atomic file save.** Failed saves leave both session and SQLite state dirty.
- [ ] **Step 6: Run** `cargo test -p desktop-app --test workspace_restore`; confirm restart, missing-source, and failure tests pass.
- [ ] **Step 7: Commit** as `feat: persist and restore document sessions`.

## Task 4: Native Open, Folder, and Save As Commands

**Files:**
- Modify: `apps/desktop/src-tauri/src/commands.rs`
- Modify: `apps/desktop/src-tauri/src/lib.rs`
- Modify: `crates/app-core/src/adapter.rs`
- Modify: `crates/platform-desktop/src/lib.rs`
- Modify: `apps/desktop/src/lib/bridge/documents.ts`
- Test: `apps/desktop/src-tauri/tests/commands.rs`

**Interfaces:**
- Add backend commands `open_path(path)`, `open_folder(path)`, and `save_document_as(document_id, path)`.
- File/folder selection stays in native Tauri dialogs; Rust validates canonical paths, source kind, and supported extension before opening or writing.
- Folder result is a flat list of supported files directly within the selected folder; no recursive indexing.

- [ ] **Step 1: Write failing tests** named `open_path_registers_recent_document`, `open_folder_lists_only_direct_supported_children`, `save_as_cancel_or_invalid_extension_preserves_draft`, and `save_as_writes_validated_snapshot_atomically`.
- [ ] **Step 2: Run** `cargo test -p desktop-app --test commands`; confirm the added command contract tests fail.
- [ ] **Step 3: Implement path validation and recent metadata updates** with canonical paths and explicit unavailable-file errors.
- [ ] **Step 4: Implement Save As** by serializing the current session to a temporary sibling file, atomically replacing only the selected destination, and updating SQLite metadata after the write succeeds.
- [ ] **Step 5: Add native dialog wrappers** for folder selection and save destination; cancellation returns a distinct non-error outcome and leaves state unchanged.
- [ ] **Step 6: Run** `cargo test -p desktop-app --test commands`; confirm command tests pass.
- [ ] **Step 7: Commit** as `feat: add workspace file and save commands`.

## Task 5: Multi-Tab Frontend Workspace

**Files:**
- Modify: `apps/desktop/src/App.svelte`
- Create: `apps/desktop/src/lib/components/WorkspaceTabs.svelte`
- Modify: `apps/desktop/src/lib/components/OpenFileView.svelte`
- Modify: `apps/desktop/src/lib/bridge/documents.ts`
- Modify: `apps/desktop/src/lib/types/documents.ts`
- Create: `apps/desktop/src/lib/components/WordDraftEditor.svelte`
- Modify: `apps/desktop/src/App.test.ts`
- Create: `apps/desktop/src/lib/components/WorkspaceTabs.test.ts`

**Interfaces:**
- `WorkspaceTab` is discriminated as `{ type: 'recent'; id: 'recent' } | { type: 'document'; id: string; document: OpenDocument; dirty: boolean; state: 'loading' | 'ready' | 'error' }`.
- `WorkspaceTabs` receives `tabs`, `activeTabId`, `onActivate(id)`, `onClose(id)`, and `onCreate()`.
- `WordDraftEditor` receives `documentId`, `paragraphs: string[]`, and `onChange(paragraphs: string[])`; it is rendered only for a new Word draft.
- App boot calls `restoreWorkspace()`; opened documents map to lazy-mounted, retained viewer instances; the `recent` tab cannot be closed.

- [ ] **Step 1: Write failing component tests** named `recent_tab_is_permanent`, `document_tab_switches_and_closes_independently`, `tabs_scroll_horizontally`, and `plus_menu_exposes_open_and_create_actions`.
- [ ] **Step 2: Run** `pnpm --dir apps/desktop test -- App.test.ts WorkspaceTabs.test.ts`; confirm behavior tests fail before implementation.
- [ ] **Step 3: Implement the workspace tab strip** with accessible tab/close controls, file-format icons, dirty markers, horizontal scrolling, and a plus menu for open folder, open file, and new Excel/Word/JSON.
- [ ] **Step 4: Replace single `opened` state in `App.svelte`** with tab/session collection and active ID. Preserve inactive viewer instances after first activation; closing a tab closes only its own document session.
- [ ] **Step 5: Render Recent content** from persisted records, support folder contents, restore drafts, remove unavailable recent entries, and create JSON drafts from pasted input.
- [ ] **Step 6: Render the draft-only Word editor** for new Word documents and persist paragraph edits through the backend; existing DOCX documents continue using the read-only `DocumentViewer`.
- [ ] **Step 7: Protect close operations** by querying dirty state across tabs and confirming before discard; persist workspace order and active tab on changes.
- [ ] **Step 8: Run** `pnpm --dir apps/desktop test -- App.test.ts WorkspaceTabs.test.ts`; confirm all workspace tests pass.
- [ ] **Step 9: Commit** as `feat: add multi-tab document workspace`.

## Task 6: Save UX, Locales, and Ribbon Overflow

**Files:**
- Modify: `apps/desktop/src/lib/components/OfficeTitleBar.svelte`
- Modify: `apps/desktop/src/lib/components/SpreadsheetViewer.svelte`
- Modify: `apps/desktop/src/style.css`
- Modify: `apps/desktop/src/lib/i18n/locales/zh-CN.ts`
- Modify: `apps/desktop/src/lib/i18n/locales/en-US.ts`
- Test: `apps/desktop/src/lib/components/SpreadsheetViewer.test.ts` or existing viewer UI test module

**Interfaces:**
- Title bar Save dispatches `save_document` for existing documents and native Save As for drafts.
- Ribbon overflow uses an explicit horizontally scrolling group container with `overflow-y: hidden` and no fixed-height content clipping.
- All labels, errors, tooltips, and accessible names exist in both supported locales.

- [ ] **Step 1: Add failing UI tests** named `draft_save_uses_save_as`, `cancelled_save_keeps_dirty_state`, and `ribbon_has_no_vertical_scrollbar_with_overflowing_groups`.
- [ ] **Step 2: Run** `pnpm --dir apps/desktop test -- SpreadsheetViewer.test.ts`; confirm save and overflow assertions fail.
- [ ] **Step 3: Implement draft Save As routing and localized actions/errors**; preserve dirty state on cancellation or failure.
- [ ] **Step 4: Fix ribbon overflow CSS** so long groups can scroll horizontally and the ribbon itself never develops vertical scrolling.
- [ ] **Step 5: Run** `pnpm --dir apps/desktop test -- SpreadsheetViewer.test.ts` and `pnpm --dir apps/desktop check`; confirm tests pass and Svelte check reports 0 errors.
- [ ] **Step 6: Commit** as `fix: refine draft save and ribbon overflow behavior`.

## Task 7: End-to-End Recovery and Verification

**Files:**
- Modify: `apps/desktop/src/App.test.ts`
- Modify: `apps/desktop/src-tauri/tests/workspace_restore.rs`
- Modify: `apps/desktop/src/lib/components/WorkspaceTabs.test.ts`

- [ ] **Step 1: Add regression coverage** for two dirty document tabs during window close, stale/missing source recovery, corrupt database startup error, Save As cancellation, and draft recovery after simulated restart.
- [ ] **Step 2: Run Rust tests** with `cargo test --workspace` and fix failures.
- [ ] **Step 3: Run frontend tests and checks** with `pnpm --dir apps/desktop test` and `pnpm --dir apps/desktop check`.
- [ ] **Step 4: Run production build** with `pnpm --dir apps/desktop build`.
- [ ] **Step 5: Inspect the workspace UI** at narrow width to confirm tabs scroll, active labels remain visible, and Ribbon has no vertical scrollbar; report separately whether the running Tauri WebView was manually exercised.
- [ ] **Step 6: Commit** any final fixes as `fix: harden workspace restore and close lifecycle`.
