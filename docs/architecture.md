# Glance-Go Desktop Architecture

## Goal

Build a local desktop document viewer/editor for basic Excel and Word workflows. The UI is rendered with Gio. Keep format libraries replaceable and keep their types out of UI state.

This architecture borrows the format-adapter, normalized-document, session, and compatibility boundaries used by the sibling `tools` project, and applies Gio interaction lessons from `mysql-ai`. Reuse the boundaries and proven interaction patterns; do not copy their Rust/Tauri/Svelte or database-specific implementation.

## Layers and ownership

```text
Gio window and widgets
        ↓ user intents / immutable view data
Desktop UI session collection (`internal/desktopui`, `internal/workspace`)
        ↓ format-neutral requests
Document adapter interface
        ↓ library-specific operations
XLSX / DOCX libraries and local filesystem
```

### Desktop UI (`internal/desktopui`)

- Own the Gio window, theme, persistent `widget.Editor`/`widget.List`/`widget.Clickable` state, selection, scroll position, and transient drafts.
- Draw from a stable session snapshot and turn user events into controller calls.
- Do not hold Excelize, DOCX, OOXML, ZIP, or XML library objects in Gio widgets.
- Keep rendering/layout helpers separate from event handling and file/session logic. Avoid growing one all-purpose `App` file; use focused view, interaction, and widget-state files as the UI grows.

### Application/session layer (`internal/workspace`, `internal/desktopui`)

- Own the active document identity, path, format, loading/error state, capabilities, selection, dirty state, and save/close lifecycle.
- Expose snapshots or narrowly scoped state operations to the UI. Keep the session as the source of truth for committed document data; widget editors are only transient drafts.
- Perform file/library work away from the Gio frame layout path. Report completion through synchronized state and request a redraw using the established Gio event-loop mechanism.
- Validate edits and capabilities here and in the adapter. A UI control being disabled is not validation.
- `internal/workspace` owns versioned SQLite metadata, recents, ordered tabs, active-tab ID, byte snapshots, dirty flags, and opaque per-tab view state. Format snapshots remain owned by the adapters; Gio widget instances stay in each in-memory document session.
- The workspace tab strip sits flush at the top edge of the full-height client area: recent/document tabs and the `+` tab share one horizontal list. File, folder, create, save, and save-as actions live in the `+` menu; the document canvas receives the space previously used by the path toolbar. On platforms where Gio reports client-side decorations, the tab strip occupies the transparent titlebar area; custom window controls and a dedicated drag strip preserve window actions. If the platform retains native decorations (currently X11), keep the system titlebar and place tabs directly below it rather than disabling move/resize behavior.
- The Recent tab is permanent. Closing a dirty document tab is blocked until it is saved; window-close confirmation is unavailable because Gio reports destruction after the native window has closed. Persist committed edits and drafts to the recovery store.
- File dialogs, document package opens/saves, and adapter serialization must not run in Gio layout callbacks. Do not serialize an entire workbook merely because the user selected a cell or scrolled; persist view state separately.

### Neutral document model (`internal/document` or `internal/docmodel`)

- Define product-facing DTOs for workbook sheets/cells/ranges and Word blocks such as paragraphs, text runs, tables, and image references.
- Represent unsupported content explicitly when it affects preview or save safety; do not silently imply complete Office support.
- Define format capabilities (read, edit constants/text, save) independently from library names.
- Keep memory and rendering bounded: use visible ranges/pages, avoid duplicating whole files as formatted strings, and provide cancellation for long operations where practical.

### Format adapters (`internal/format/xlsx`, `internal/format/docx`)

- Implement a small shared open/read/edit/save contract, with typed format-specific operations behind it where necessary.
- Translate third-party library values to/from neutral DTOs at this boundary.
- Preserve source path/session context only where required to safely save. Use temporary sibling output and replace the original only after a successful write.
- Keep formulas as formula text plus cached result when available. Do not claim recalculation unless a calculation engine is separately integrated and verified.
- DOCX text mutation must preserve unrelated package parts and formatting in the supported corpus. If a library cannot do that reliably, limit the first release to read-only DOCX or narrowly scoped text replacement.
- XLSX and DOCX snapshots are self-contained Office package bytes; JSON snapshots preserve current editor text, including invalid intermediate JSON. Snapshot payloads are not interchange DTOs and must only be interpreted by their matching adapter.

### Platform and file access

- Isolate open/save dialogs, clipboard, and OS-specific behavior behind small platform helpers, following the build-tag-specific file-picker pattern in `mysql-ai` where needed.
- Keep local-file processing local. Do not introduce a server path for document contents.

## Gio interaction guidance

- Gio is immediate mode for layout, but interactive widget state must persist across frames. Keep each editor/list/clickable in stable application or view state; do not construct a fresh editor during every layout pass.
- Layout functions should describe the current view and register input areas. Process clicks, keys, editor changes, and async completion as events, update state, then request another frame.
- Initialize every `widget.List` with its intended axis, including lists stored in newly created dialog state.
- Keep selection and editing distinct. For spreadsheet editing retain the original cell value, draft text, and edit mode; commit once, make no-op focus/blur a no-op, and cancel without mutating the workbook.
- Treat IME composition as an editing concern: do not commit intermediate composition text. Test Chinese input, Enter/Escape/Tab behavior, and focus transfer on actual supported platforms.
- Virtualize spreadsheet rows/cells and Word pages/blocks. Register hit targets only for visible content; use stable row/column or block identities rather than one widget per cell in the whole workbook.
- Keep bounded spreadsheet window reads, document opens, and saves off the Gio frame loop. Return results to the UI loop through synchronized state and invalidate the window after applying each result. Reject stale read results when the active document or sheet changes.
- Resolve pointer coordinates into one documented local/content coordinate system for selection and drag operations. If Gio reports nested-target coordinates, normalize them at the input boundary instead of mixing spaces in layout math.
- For Excel row-height and column-width handles, follow the stable resize pattern in `../mysql-ai/docs/drag-patterns.md`: resolve the pointer in grid-content coordinates, keep the preview line under the pointer, maintain the resize cursor throughout the active drag, and persist the new dimension only on release. Expand the temporary preview viewport when dragging past the current grid edge. Convert Gio screen pixels to logical grid units and then to the file format's row/column dimension units in one named adapter/helper; do not store raw pixels as workbook dimensions.
- Keep row and column resize preview state separate from committed workbook dimensions. Rebase the drag anchor if the input coordinate space changes during the gesture so the preview does not jump. Cover dragging both narrower and wider, the first and last visible dimension, scrolling/edge cases, and release/cancel behavior with geometry tests plus platform interaction checks.
- Text editors in vertical list rows should occupy available width. Clear inherited minimum constraints for compact chips, icons, and badges. Flush editor contents to a dialog draft before closing or saving.
- Keep files focused (mysql-ai uses a 500-line practical source-file guardrail); split view, event, and state code by responsibility as features grow.

## Save safety and compatibility

- Saving is an overwrite of a user's document; advertise edit/save capability only when that adapter and file class pass round-trip preservation checks.
- First editor scope should be small: XLSX cell constants (text, number, boolean, blank), and a clearly defined limited DOCX text workflow. Defer formula calculation, range paste, style editing, page-layout editing, and legacy `.xls`/`.doc` unless explicitly required.
- The adapter must report unsupported or failed operations. Failed saves retain the dirty session and must leave the original file intact.
- Track compatibility by concrete feature and fixture evidence. A successful parse is not evidence of lossless save. Reopen saved files and compare changed values plus representative untouched styles, merges, formulas/cached values, images, tables, and package relationships.
- Never claim Word pagination or Excel visual parity based only on extracted text. Keep preview layout fidelity separate from document read/write fidelity.

## Reference projects

- [Virtualized spreadsheet grid design](superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md) — two-axis virtual extent, inline editing, common styles, merged ranges, and dimension resizing.
- `../tools/docs/compatibility/office-feature-matrix.md` — evidence-based feature matrix and distinction between supported, partial, and unsupported behavior.
- `../tools/docs/superpowers/specs/2026-09-27-xlsx-cell-editing-design.md` — session, dirty state, atomic save, and XLSX round-trip safety.
- `../mysql-ai/AGENTS.md` — focused-file and Gio form guardrails.
- `../mysql-ai/docs/gio-form-layout-gotchas.md` — persistent widget/list, row width, constraints, draft flush, and end-to-end state tracing.
- `../mysql-ai/docs/drag-patterns.md` — stable preview/persist separation and pointer-coordinate normalization.

These sibling paths are local references, not build dependencies. When documenting a rule in this repository, keep its rationale and actionable form here so the project remains understandable if siblings move.
