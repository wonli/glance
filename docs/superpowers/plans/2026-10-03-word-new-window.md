# Word New Window Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Word View > New Window so multiple desktop windows edit one shared document session while keeping independent Gio views.

**Architecture:** Split shared format/document state from per-window widgets and view state. A process-level `desktopRuntime` owns workspace persistence, the logical document-session registry, and attached windows. Each window keeps its own `App`, Gio widgets, active tab, selection, scroll, and zoom; adapter work is serialized by the shared session and published to every attached view.

**Tech Stack:** Go, Gio `app.Window`, existing `internal/desktopui`, neutral `internal/document` snapshots, existing DOCX/XLSX/JSON adapters, and `internal/workspace`. No new runtime or document-format dependency.

**Spec:** `docs/superpowers/specs/2026-10-03-word-new-window-design.md`

## Global Constraints

- A new window attaches to the active Word document session, including unsaved documents.
- The feature must not create a second DOCX adapter for the same document or a second workspace tab.
- `app.Main()` remains called exactly once.
- Each Gio window has its own event loop and `App`/theme/op state; Gio widgets and `op.Ops` are never shared across windows.
- Route adapter reads that can overlap mutations, all document mutations, saves, and snapshot generation through the shared session's serialized operation boundary.
- Async jobs carry logical document ID, revision, and operation generation; stale results cannot publish or target detached windows.
- The window manager owns one workspace collection and writer; existing single-window view state restores into the primary view.
- Secondary view states are independent for their in-memory lifetime and are not restored after process restart.
- Mobile, WebAssembly, and other single-window targets keep the action disabled with a clear capability reason.
- The UI still launches through `go run ./cmd/desktop`, and the standalone binary remains named `desktop`.

## Review Focus

- A second window opened while a Word draft/save is pending must attach to the live session, display current state, and rebase its editors without losing local caret/selection or overwriting a newer shared draft (Tasks 1 and 4: `TestWordNewWindowAttachesToLiveSessionDuringPendingWork` and `TestWindowEditorsRebaseFromSharedDraftWithoutMovingCaret`).
- An operation started in one view and completed after that view closes must publish to the remaining views without invalidating a destroyed window (Tasks 3 and 7: `TestSessionResultFansOutAfterOriginWindowCloses`).
- Two windows switching tabs independently must retain separate active tabs while sharing the workspace tab catalog; closing a logical tab must not leave another window pointing at a closed adapter (Task 4: `TestWorkspaceTabsAreSharedButActiveTabIsPerWindow` and `TestClosingDocumentTabDetachesEveryView`).
- Closing the primary window while a secondary remains must promote the oldest view without closing the session; final shutdown must persist/close the shared store and adapters exactly once (Task 7: `TestPrimaryWindowPromotionAndFinalShutdown`).
- A new-window creation error or unsupported platform must leave the source window usable and explain why the action is unavailable (Task 5: `TestNewWindowFailureKeepsSourceWindow` and `TestNewWindowCapabilityMatchesPlatform`).

---

### Task 1: Separate shared document data from per-window view state

**Files:**
- Create: `internal/desktopui/document_session.go`
- Modify: `internal/desktopui/session_state.go`
- Test: `internal/desktopui/document_session_test.go`
- Modify: affected tests that construct `documentSessionState` directly

**Interfaces:**
- Produces `type sharedDocumentSession struct { ... }`, which owns `documentID`, `path`, `book`, `word`, `jsonDoc`, `wordBlocks`, `sheets`, closed/dirty/revision state, `wordRunCommitted`, `wordRunDrafts`, Word layout request/result/error state, and shared operation/save state.
- Produces `type documentSessionState struct { *sharedDocumentSession; windowID uint64; ... }`, which owns all persistent Gio widgets and per-window interaction/view state.
- Produces `func newSharedDocumentSession(path string) *sharedDocumentSession`, `func newDocumentSessionView(shared *sharedDocumentSession, windowID uint64) *documentSessionState`, and keeps `func newDocumentSessionState(path string) *documentSessionState` as the single-window constructor used by existing callers/tests.
- Widgets, editor caret/selection, active tab, list positions, zoom, view modes, and hit/drag state stay per view. Adapters, neutral committed blocks, sheet names, drafts, document revision, dirty state, and in-flight mutation/save state are shared. Grid viewport data, render caches, image decode tasks, and interaction caches that contain window geometry stay per view.

- [ ] **Step 1: Add `TestDocumentViewsShareSessionButOwnGioState`**

Create two views over one `sharedDocumentSession`. Assert they share the adapter, draft, and revision, while each view has distinct Word editors, list widgets, toolbar clickables, selection, zoom, and view mode. Mutating one view's caret/zoom must not affect the other; updating the shared draft/revision must be visible through both.

- [ ] **Step 2: Run the regression and verify the missing split**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run '^TestDocumentViewsShareSessionButOwnGioState$' -count=1`
Expected: FAIL because `documentSessionState` currently combines adapter data and Gio state and has no way to create two independent views over one session.

- [ ] **Step 3: Extract `sharedDocumentSession` and implement the view constructor**

Move only the shared fields listed above into `sharedDocumentSession`; keep field promotion through embedding so existing `state.word`, `state.wordBlocks`, and equivalent access remains direct. Initialize every `widget.List` axis and every map in `newDocumentSessionView`. Make `newDocumentSessionState` create a fresh shared session and one view.

- [ ] **Step 4: Run Word, workspace, and state construction regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'Test(DocumentViews|Word|Workspace)' -count=1`
Expected: PASS for all matching tests, including the new shared-core/independent-widget test.

- [ ] **Step 5: Commit the session/view split**

Commit the implementation and its tests as `refactor: split document session from window view state`.

### Task 2: Add the process-level runtime and shared session registry

**Files:**
- Create: `internal/desktopui/runtime.go`
- Create: `internal/desktopui/runtime_test.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/workspace.go`
- Modify: `internal/desktopui/session_state.go`

**Interfaces:**
- Produces `type desktopRuntime struct { mu sync.Mutex; collection *workspace.Collection; workspaceStore *workspace.Store; sessions map[string]*sharedDocumentSession; sessionsByCanonicalPath map[string]string; windows map[uint64]*App; primaryWindowID uint64; ... }`.
- Produces `func (r *desktopRuntime) session(documentID string) (*sharedDocumentSession, bool)`, `func (r *desktopRuntime) openDocumentSession(path string) (*sharedDocumentSession, error)`, `func (r *desktopRuntime) attachWindow(windowID uint64, app *App)`, `func (r *desktopRuntime) detachWindow(windowID uint64)`, and `func (r *desktopRuntime) viewsForSession(documentID string) []*App`.
- Keeps `App.sessions map[string]*documentSessionState` as window-local views. Multiple Apps resolve a document ID through the same runtime session and create independent views around that shared session.
- The runtime is the only owner of `workspace.Store`, `workspace.Collection`, workspace writer, and logical-session registry. Preserve `New(nil, "")` as an isolated single-window test constructor.

- [ ] **Step 1: Add runtime registry and single-owner tests**

Add `TestRuntimeReusesLogicalSessionAcrossWindows`, asserting same ID returns the same shared core, each App receives a different view, and one logical tab/session is registered. Add `TestRuntimeReusesLogicalSessionForOpenPath`, asserting opening the same canonical path twice reuses the existing adapter/session. Add `TestRuntimeOwnsOneWorkspaceWriter`, asserting two windows route persistence through one writer/store owner.

- [ ] **Step 2: Run the tests to verify current ownership is per App**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run '^TestRuntime(ReusesLogicalSessionAcrossWindows|ReusesLogicalSessionForOpenPath|OwnsOneWorkspaceWriter)$' -count=1`
Expected: FAIL because every `App` currently opens/owns its own workspace store, collection, and session map.

- [ ] **Step 3: Implement `desktopRuntime` and route workspace ownership through it**

Move store/collection initialization, writer lifecycle, shared session creation, and final adapter cleanup to the runtime. Route `App.open(path)` through `openDocumentSession`; key the path index with the existing `canonicalDocumentPath` helper. Keep one-window construction behavior for tests by constructing an isolated runtime only when a windowed app is created; direct `New(nil, "")` tests must not open a database.

- [ ] **Step 4: Run runtime and existing workspace regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'Test(Runtime|Workspace)' -count=1`
Expected: PASS; the runtime has one store/writer while existing single-window workspace behavior remains unchanged.

- [ ] **Step 5: Commit runtime/session registry**

Commit as `feat: centralize workspace and document sessions`.

### Task 3: Serialize shared adapter work and publish revisions to attached views

**Files:**
- Create: `internal/desktopui/session_operations.go`
- Create: `internal/desktopui/session_operations_test.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/layout.go`
- Modify: `internal/desktopui/workspace.go`
- Modify: `internal/desktopui/word_editing.go`
- Modify: `internal/desktopui/word_pagination.go`
- Modify: other `internal/desktopui` adapter-operation files identified by the direct-adapter-access audit

**Interfaces:**
- Produces `type sessionOperationKind uint8`, `type sessionOperationToken struct { documentID string; generation uint64; revision uint64; kind sessionOperationKind }`, `func (s *sharedDocumentSession) beginOperation(kind sessionOperationKind) (sessionOperationToken, bool)`, and `func (s *sharedDocumentSession) finishOperation(token sessionOperationToken) bool`.
- Produces `func (r *desktopRuntime) publishSessionRevision(documentID string, revision uint64)` to invalidate all live views after publishing a new immutable document snapshot.
- Adapter access in layout reads immutable neutral snapshots only. Background reads/mutations/saves/snapshot generation acquire the session operation boundary; only one save or format mutation per document runs at a time. Per-window `documentGeneration` cannot substitute for the shared operation generation.

- [ ] **Step 1: Add operation gate and cross-window publication regressions**

Add `TestSessionOperationGateSerializesSaveAndMutation` and `TestSessionPublicationInvalidatesEveryAttachedView`. Assert a second operation is rejected while one is active, successful publication advances the revision exactly once, and all attached windows receive invalidation. The lifecycle test for a result arriving after its source window closes is covered in Task 7.

- [ ] **Step 2: Run the tests to verify no shared-session operation boundary exists**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run '^Test(SessionOperationGate|SessionPublication)' -count=1`
Expected: FAIL because `formatRunning`, save state, job generations, and invalidation are currently scoped to one `App`.

- [ ] **Step 3: Implement shared operation tokens and revision fan-out**

Move operation/save flags and generation to `sharedDocumentSession`; make every worker validate its session token before publishing. Publish neutral block/grid snapshots and revision under the session boundary, then invalidate live views through `desktopRuntime.viewsForSession`.

- [ ] **Step 4: Audit and route adapter reads/writes**

Run `rg -n '\.(word|book|jsonDoc)\.' internal/desktopui --glob '*.go'` and classify each production access. Move frame/layout reads to the shared immutable snapshot; route adapter reads that can race, mutations, saves, and snapshots through the session boundary. Keep parsing, serialization, and format calls outside Gio layout callbacks.

- [ ] **Step 5: Run operation, Word editing, save, print, and adapter regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'Test(Session|Word.*(Save|Draft|Pagination|PDF|Print)|Workspace.*(Save|Snapshot))' -count=1`
Expected: PASS, with no stale revision accepted and no adapter operation running concurrently for one session.

- [ ] **Step 6: Commit serialized session operations**

Commit as `refactor: serialize document session operations`.

### Task 4: Share workspace tabs and attach an independent view to a document

**Files:**
- Modify: `internal/desktopui/workspace.go`
- Modify: `internal/desktopui/workspace_layout.go`
- Modify: `internal/desktopui/session_state.go`
- Create: `internal/desktopui/window_view.go`
- Create: `internal/desktopui/window_view_test.go`
- Modify: `internal/desktopui/workspace_test.go`

**Interfaces:**
- Produces `type windowViewSnapshot struct { activeTabID string; wordViewMode wordViewMode; wordPageFitMode, wordMultiplePagesMode bool; wordLayoutZoom float64; wordRulerHidden, wordGridlinesVisible, wordNavigationVisible bool; wordNavigationQuery string; wordToolbarTab wordToolbarTab; selectedWordRunID, selectedWordParagraphID, selectedWordImageID string; selectedWordSelectionStart, selectedWordSelectionEnd, wordListFirst, wordPageFirst int }`, `func captureWindowView(app *App) windowViewSnapshot`, and `func newDocumentViewFromSnapshot(shared *sharedDocumentSession, windowID uint64, snapshot windowViewSnapshot) *documentSessionState`.
- `newDocumentViewFromSnapshot` constructs all Gio widgets afresh, initializes editor text from the latest shared draft/committed snapshot, then restores selection and clamps selection/list indexes to valid content. `func syncWindowViewFromSharedSession(gtx layout.Context, state *documentSessionState)` updates non-focused, non-composing editors from a new shared draft while preserving/clamping each view's local caret/selection; focused editor and IME composition remain under that window's control.
- Workspace tab metadata is shared; `App.activeTabID` is per-window. Only the runtime's primary window calls `Collection.Activate` and persists the existing active-tab/view-state slot. If primary detaches, promote the oldest remaining window and persist its state.
- Plan decision for close semantics: closing a workspace document tab closes that logical session for every window after the existing save/discard confirmation; switching tabs remains per-window.

- [ ] **Step 1: Add view-copy and independent-tab tests**

Add `TestWindowViewSnapshotCopiesPresentationIntoFreshWidgets`, `TestWorkspaceTabsAreSharedButActiveTabIsPerWindow`, `TestClosingDocumentTabDetachesEveryView`, `TestWindowEditorsRebaseFromSharedDraftWithoutMovingCaret`, and `TestWordNewWindowAttachesToLiveSessionDuringPendingWork`. Assert two views start from the selected page/caret and presentation settings, own separate editors/lists/clickables, retain independent active tabs after switching, closing the logical tab removes its session from all views, a sibling editor refreshes without moving its caret, and a new view attaches to the current shared revision/drafts without adding a workspace tab or adapter.

- [ ] **Step 2: Run tests to verify views and workspace activation are currently single-window**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run '^Test(WindowViewSnapshot|WorkspaceTabsAreSharedButActiveTabIsPerWindow|ClosingDocumentTab|WindowEditorsRebaseFromSharedDraftWithoutMovingCaret|WordNewWindowAttachesToLiveSessionDuringPendingWork)' -count=1`
Expected: FAIL because `activateSession` and widgets are currently bound to one `App`/`documentSessionState`.

- [ ] **Step 3: Implement per-window view construction and tab activation**

Create view widgets from `windowViewSnapshot`, copy the source view values once, and keep later selections/scroll/zoom per view. Share the tab catalog/session cores while storing active tab on each App. Persist active tab/view data only for the primary window; on promotion, use the promoted window's current view for subsequent persistence.

- [ ] **Step 4: Implement shared document-tab close behavior**

When a tab is closed, use one existing save/discard decision for the logical session, detach every view from that session, move affected Apps to Recent, remove the workspace tab once, and close the adapter only after no live operation references it.

- [ ] **Step 5: Run new view, workspace, and tab-close regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'Test(WindowView|Workspace.*(Tab|Session)|ClosingDocumentTab|WordNewWindowAttaches)' -count=1`
Expected: PASS; switching tabs in one window leaves sibling active tabs unchanged, and a shared tab close leaves no App pointing at a closed session.

- [ ] **Step 6: Commit shared tabs and per-window views**

Commit as `feat: attach window-local views to shared sessions`.

### Task 5: Add the desktop window manager and multi-window Gio lifecycle

**Files:**
- Create: `internal/desktopui/window_manager.go`
- Create: `internal/desktopui/window_manager_test.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/runtime.go`
- Modify: `cmd/desktop/main.go`
- Create or modify: platform capability files under `internal/desktopui` if build tags are needed

**Interfaces:**
- Produces `func Run(initialPath string) <-chan struct{}`, `func (r *desktopRuntime) openDocumentWindow(documentID string, snapshot windowViewSnapshot) error`, `func (r *desktopRuntime) runWindow(windowID uint64, window *app.Window, initialDocumentID string, snapshot windowViewSnapshot)`, and `func supportsMultipleDesktopWindows() bool`.
- Production creates one `desktopRuntime`, launches the initial window and any later windows on their own `App`/event loop, and calls `app.Main()` once from the process entry point. `Run` returns a channel closed only after final persistence and cleanup; the process entry point waits for it in a watcher and exits after cleanup because Gio's desktop `app.Main()` blocks even after the last window is destroyed.
- Tests inject `type windowLauncher func(documentID string, snapshot windowViewSnapshot) error` so creation failure can be verified without constructing a native window.

- [ ] **Step 1: Add window lifecycle/failure and platform capability tests**

Add `TestRuntimeRegistersAndRunsIndependentWindowApps`, `TestNewWindowFailureKeepsSourceWindow`, and `TestNewWindowCapabilityMatchesPlatform`. Assert unique window IDs, separate Apps/widgets, shared session attachment, recoverable failure status, and disabled reason on unsupported targets.

- [ ] **Step 2: Run tests to verify only one Gio window can be registered by the app**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run '^Test(RuntimeRegistersAndRunsIndependentWindowApps|NewWindowFailureKeepsSourceWindow|NewWindowCapabilityMatchesPlatform)$' -count=1`
Expected: FAIL because `desktopui.Run` currently creates and owns exactly one `app.Window` and no window registry exists.

- [ ] **Step 3: Implement the runtime window registry and `openDocumentWindow`**

Register the new App before its `Window.Event()` loop first shows the native window. Give it a copied `windowViewSnapshot`, a fresh per-window session-view map, and the process runtime. Report early `DestroyEvent.Err` back to the source App. Keep `cmd/desktop/main.go` calling `app.Main()` once and add a completion watcher that terminates the process only after `Run` signals fully completed cleanup.

- [ ] **Step 4: Run window manager and current single-window startup regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'Test(RuntimeRegisters|NewWindow|WindowChrome|Workspace)' -count=1`
Expected: PASS; existing `New(nil, "")` tests remain headless and one-window startup remains available.

- [ ] **Step 5: Commit Gio window lifecycle**

Commit as `feat: manage multiple desktop windows`.

### Task 6: Wire Word View > New Window

**Files:**
- Modify: `internal/desktopui/word_toolbar.go`
- Modify: `internal/desktopui/word_ribbon_layout.go` only if command availability needs a disabled reason
- Create: `internal/desktopui/word_new_window.go`
- Create: `internal/desktopui/word_new_window_test.go`

**Interfaces:**
- Produces `func (a *App) requestWordNewWindow()` and `func wordNewWindowAvailability(state *documentSessionState, runtime *desktopRuntime) (enabled bool, reason string)`.
- `view-new-window` is enabled only for an active Word document with an attached runtime on a supported multiple-window desktop target. Unsupported platforms and test Apps without a runtime expose a clear disabled reason.
- The handler captures the current `windowViewSnapshot` and asks the runtime to attach a new App to the same document ID; it never reloads a source path or changes active tabs in the source App.

- [ ] **Step 1: Add action behavior tests**

Add `TestWordNewWindowActionOpensSharedDocumentView` and `TestWordNewWindowActionDisabledWithoutWordRuntime`. Assert a new window request carries the same document ID and current view snapshot, creates no duplicate workspace tab/session, and does not dirty or change the source view.

- [ ] **Step 2: Run tests to verify the Ribbon action is disabled**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run '^TestWordNewWindowAction' -count=1`
Expected: FAIL because `view-new-window` currently has no enabled handler.

- [ ] **Step 3: Connect availability and action dispatch**

Enable the action through `wordNewWindowAvailability`; route it to `requestWordNewWindow`, report asynchronous creation failure in the source window, and leave unrelated View commands unchanged.

- [ ] **Step 4: Run Word Ribbon and action regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'TestWord(NewWindow|View|Toolbar)' -count=1`
Expected: PASS; action state, shared session ID, copied initial view, and per-view divergence are verified.

- [ ] **Step 5: Commit the New Window command**

Commit as `feat: add Word new-window command`.

### Task 7: Implement window detach, primary promotion, and final shutdown

**Files:**
- Modify: `internal/desktopui/window_manager.go`
- Modify: `internal/desktopui/runtime.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/workspace.go`
- Create: `internal/desktopui/window_lifecycle_test.go`

**Interfaces:**
- `func (r *desktopRuntime) detachWindow(windowID uint64)`, `func (r *desktopRuntime) promotePrimaryWindow()`, and `func (r *desktopRuntime) shutdown()` are idempotent.
- Detaching a non-final window releases only its views/widgets and removes its invalidation targets. The shared session/core, adapters, workspace store, and writer remain open while another window or accepted worker references them.
- Final shutdown persists once, cancels or drains shared jobs, joins the single workspace writer, closes each adapter once, then closes the one workspace store. The runtime closes the `Run` completion channel only after cleanup; the process entry point reacts to that signal so Gio's blocking `app.Main()` cannot keep a fully shut down process alive.

- [ ] **Step 1: Add close/promotion/final-shutdown tests**

Add the Review Focus test `TestPrimaryWindowPromotionAndFinalShutdown` and `TestSessionResultFansOutAfterOriginWindowCloses`. Assert closing the primary promotes the oldest remaining view without closing a shared adapter, secondary view state becomes the primary persisted view, final shutdown persists and closes each shared resource exactly once, and late results do not touch detached widgets.

- [ ] **Step 2: Run the lifecycle tests to verify current per-App teardown is unsafe**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run '^Test(PrimaryWindowPromotionAndFinalShutdown|SessionResultFansOutAfterOriginWindowCloses)$' -count=1`
Expected: FAIL because each `DestroyEvent` currently calls `App.closeWorkspace` and closes all adapters/store owned by that App.

- [ ] **Step 3: Implement idempotent detach, promotion, and runtime shutdown**

Route `DestroyEvent` through `desktopRuntime.detachWindow`; remove a window before release, persist the old primary view, promote the oldest remaining view if needed, and invoke `shutdown` only when the registry has no remaining windows. Ensure shutdown waits for operations and the sole writer before closing adapters/store.

- [ ] **Step 4: Run workspace, async command, save, and close regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'Test(PrimaryWindow|SessionResultFansOut|Workspace|Word.*(Save|Close|Draft|Pagination|Print|PDF))' -count=1`
Expected: PASS; no adapter/store is closed early or more than once.

- [ ] **Step 5: Commit window lifecycle and shutdown**

Commit as `feat: preserve shared sessions across window close`.

### Task 8: Verify full single-/multi-window behavior and update support records

**Files:**
- Modify: `docs/word-ribbon-todo-zh.md`
- Modify: `docs/office-support.md`
- Modify: `docs/compatibility/office-feature-matrix.md`
- Test: `internal/desktopui` full suite and supported desktop runtime

**Interfaces:**
- The View TODO marks `view-new-window` complete only on desktop targets whose multi-window path passed runtime verification. Keep `view-side-by-side` and `view-macros` pending.
- Compatibility/support records say one logical document/tab and shared edits are supported, secondary view state is runtime-only, and platform gaps remain explicit.

- [ ] **Step 1: Run the focused New Window and multi-window regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/desktopui -run 'Test(Runtime|DocumentViews|WindowView|WorkspaceTabsAreShared|WordNewWindow|PrimaryWindow|SessionOperationGate|SessionPublication|SessionResultFansOut|ClosingDocumentTab)' -count=1`
Expected: PASS.

- [ ] **Step 2: Run DOCX/workspace and full repository regressions**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go test ./internal/format/docx ./internal/workspace ./internal/desktopui -count=1` and `GOCACHE=/private/tmp/glance-go-build-cache go test ./... -count=1`.
Expected: no new failures. At plan creation, the full suite already had five `internal/desktopui` failures: `TestCellTypefaceKeepsWorkbookFamilyAndAddsCJKFallbacks`, `TestGridResizeHandleUsesFixedHeaderCoordinates`, `TestSelectionRectUsesVisibleGridGeometry`, `TestGridViewportScalesGeometryToDevicePixels`, and `TestWordUpDownCrossesParagraphsAtPreferredVisualColumn`; record these if they remain and fix any newly introduced failures.

- [ ] **Step 3: Build the standalone desktop binary**

Run: `GOCACHE=/private/tmp/glance-go-build-cache go build -o /private/tmp/desktop ./cmd/desktop`
Expected: PASS; app entry remains `go run ./cmd/desktop` and does not launch an external Office process.

- [ ] **Step 4: Perform supported-desktop interaction verification**

Launch `go run ./cmd/desktop`, open one DOCX, use View > New Window, verify shared edits/dirty state/save and independent caret/scroll/zoom/view mode, close one window and keep editing in the other, then close the final window and verify one workspace persistence/teardown. Capture the tested OS and whether the live Computer Use approval gate allowed the interaction; do not mark unverified platforms supported.

- [ ] **Step 5: Update TODO, support matrix, plan ledger, and commit the verified feature**

Update the View count from 9/12 to 10/12 and the overall Ribbon count from 44/99 to 45/99 only after implementation and its required platform evidence pass. Keep side-by-side and macros pending. Commit as `docs: record Word new-window support and evidence`.
