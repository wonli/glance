# Word New Window Design

## Status

The user approved the product direction: a new window shows the same live Word document, with document edits and dirty state shared and view state independent per window. The implementation design is pending user review.

## Goal

Implement Word View > New Window as a second desktop window attached to the active document. Both windows operate on one logical document session. Editing or saving from either window updates the shared document; each window keeps its own scroll position, selection/caret, zoom, and view-mode controls.

The feature must not create a second DOCX adapter for the same document or a second workspace tab. It must not allow two windows to mutate or save the same adapter concurrently.

## Product requirements

1. **Same live document:** New Window attaches to the active Word document session, including unsaved documents. The new window starts on that document and reflects its current committed text, pending text drafts, and dirty state.
2. **Independent view:** Each window has its own Gio widgets, caret/selection, scroll position, page position, zoom, active Ribbon tab, ruler/gridline/navigation-pane state, and Reading/Web/Print view mode. Changing one window's view does not change another's view.
3. **Shared edits:** A text or document operation made in one window updates the shared session and is reflected in every attached window after the operation publishes its result. Local caret/selection remains with its originating window; if a document edit removes its anchor, that window reconciles selection to a valid nearby position.
4. **Shared dirty/save state:** All windows show the same dirty and operation state. Save from any window saves the current shared session once. Only one save or format mutation may run per document at a time; other windows show the in-progress state and cannot start a conflicting mutation.
5. **One workspace record:** Workspace tabs, snapshots, recents, view persistence, and recovery are owned once per logical document. Creating/closing an additional window does not add/remove a workspace tab or close the shared adapter.
6. **Window lifecycle:** Closing a non-final window detaches and disposes only that window's UI state. The document session remains available to other windows. When the final window closes, the existing application shutdown/recovery path persists the document once, stops shared workers, and then closes the adapter and workspace store once.
7. **Failure behavior:** If the platform cannot create a new window, retain the current window/session and report the error there. A destroyed window is removed from invalidation fan-out before it is released; late worker results must not target its widgets.
8. **Platform scope:** The feature is enabled only on desktop targets that support multiple Gio windows. Mobile, WebAssembly, and other single-window targets keep the action disabled with a clear capability reason.

## Architecture

### Window manager and workspace owner

Create one process-level owner for the workspace store/collection, open document-session registry, window registry, and final shutdown. It creates the initial window and any later windows, and it is the only owner allowed to add/remove a logical document tab or close the workspace database. `app.Main()` remains called exactly once. Each Gio window has its own event loop and `App`/theme/op state; the manager registers it before showing the window and unregisters it on `DestroyEvent`. If a view that initiated a document operation closes while other views remain, the operation still belongs to the shared document session and may publish normally; only that detached view is excluded from redraw fan-out.

The owner tracks attached windows per document session. Redraw fan-out uses a synchronized registry and skips windows already destroyed. Gio widgets and `op.Ops` are never shared across windows.

### Shared document session and per-window view state

Split the current `documentSessionState`, which presently combines adapters/document data with persistent Gio widgets and view data, into two responsibilities:

- A shared document-session/controller owns the format adapter, committed neutral snapshot, transient Word drafts, revision/dirty state, document identity/path, mutation/save queue, and workspace persistence identity.
- A per-window view/session owns persistent widgets, selection/caret, scroll/page position, zoom and view preferences, window-local action state, and a reference to the shared document session.

Keep the format-specific DOCX/XLSX/JSON objects behind their current adapter boundaries. Do not move third-party format-library types into Gio widgets or neutral view DTOs.

### Mutation serialization and publication

Route adapter reads that can overlap mutations, all document mutations, saves, and snapshot generation through the shared session's serialized operation boundary. A completion publishes an immutable neutral snapshot and a monotonically increasing document revision. All attached views receive an invalidation and reconcile against the new snapshot. No window may access a mutable adapter concurrently with another window or with a session worker.

Async jobs carry logical document ID, revision, and operation generation. Results from stale revisions, removed sessions, or detached windows cannot publish into the active session or invalidate a dead window. View-only operations stay local to their window and do not increment the document revision or dirty state.

### Workspace persistence

The window manager owns one workspace collection and writer. A logical document has one existing workspace tab, identity, adapter snapshot stream, recents entry, dirty flag, and autosave path. Keep the workspace's existing single view-state slot for one designated primary window. The first view is primary; if it closes while other views remain, promote the oldest remaining view and persist its view state from then on. Secondary view states are independent for their in-memory lifetime and are not restored after process restart. Existing single-window view state must continue to restore into the primary view.

### New Window interaction

The View command is enabled for an active Word document on a supported desktop platform. Activating it requests a new window from the process-level owner and attaches a fresh window-local view to the current logical document ID. At creation, the new view copies the source window's view mode, fit/multiple-page preset, zoom, gridline/ruler settings, navigation-pane visibility/query, Ribbon tab, current selection/caret when its run still exists, and visible block/page position. It receives its own Gio widget instances; later navigation and view changes are independent. The command does not switch tabs in or create a duplicate workspace tab.

## Error and shutdown behavior

- If a mutation/save is active, the new window may still open and displays the shared in-progress state; it must not clone a stale snapshot.
- If new-window creation fails, keep the source window usable and show a recoverable status message.
- Destroying one window detaches its view and drops window-scoped tasks/widgets without closing the shared document adapter. Its current view state is persisted first if it is primary; otherwise it is discarded when the view is disposed.
- Final process shutdown waits for the one workspace writer and document operation queue before closing shared adapters and the store. Preserve the repository's existing recovery and unsaved-change behavior; this feature does not claim to add a native close-confirmation event that Gio does not expose after destruction.
- Reopening a document already present in the current process must attach to its existing logical session rather than opening another adapter for the same path.

## Verification

### Automated regressions

- A second view attaches to the same session/controller and does not add a duplicate workspace tab or adapter.
- Edits, drafts, dirty state, operation state, and saves are visible/shared across two views.
- Scroll, caret/selection, zoom, page position, Ribbon tab, and view mode remain independent across views.
- Simultaneous mutation/save requests serialize; stale results do not overwrite a newer revision.
- Closing one view does not close the shared adapter or interrupt work for the remaining view; final shutdown closes the adapter/store exactly once.
- New Window failure leaves the original view operational.
- Existing single-window startup, open, tab switching, recovery, save, close, and print/export regressions continue to pass.

### Desktop verification

Launch using `go run ./cmd/desktop`. Open one DOCX, create a second window, verify both identify the same document, type in one window and observe the update/dirty state in the other, then verify independent scrolling, selection, zoom, and page layout. Save from either window and reopen the source to confirm one shared save. Close one window and continue editing in the other; close the final window and verify one recovery/persistence write. Perform the live check on each supported desktop OS before advertising cross-platform support.

## Non-goals

- Side-by-side comparison of two different documents; that is the next View TODO and requires its own design.
- Macro reading/execution or VBA support.
- Concurrent editing across separate Glance processes.
- Adding new DOCX mutation features, changing Word page-rendering fidelity, or changing icon artwork.
- Sharing any Gio widget, pointer router, theme, `op.Ops`, selection, or scroll position across windows.
