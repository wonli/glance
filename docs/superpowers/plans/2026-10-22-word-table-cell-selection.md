# Word Table Cell Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add reliable cell-range selection, TSV copy, table-aware Delete behavior, and stable ruler geometry to Word tables in all three editing views.

**Architecture:** Keep cell selection as transient `desktopui` session state and keep table mutation behind an optional neutral capability in `internal/document`. Map both flow and page-scene pointer geometry to source cell IDs; run DOCX edits through the existing managed mutation path so each table action is atomic and undoable.

**Tech Stack:** Go, Gio pointer/key/clipboard APIs, neutral Word document interfaces, DOCX XML adapter and history.

**Spec:** `docs/superpowers/specs/2026-10-22-word-table-cell-selection-design.md`

## Global Constraints

- The behavior applies to Web Layout, Continuous Layout, and Print Layout.
- Columns are separated by tabs and rows by newlines; paragraphs within a cell are joined by newlines.
- Hyperlinks contribute their displayed text; inline images use `[图片]`.
- A table is deleted only when every cell was empty before the Delete action; otherwise only selected cells are cleared.
- A supported merged region is selected as a whole and contributes its owning cell once.
- Keep DOCX library/XML types inside `internal/format/docx`; perform parsing and mutation outside Gio layout/frame code.
- Persistent Gio widgets and pointer tags belong in session state; normalize pointer coordinates before selection math.
- Do not create or move worktrees. Keep unrelated workspace edits intact.
- Do not add or run tests unless the user explicitly asks to test or verify implementation. Validation in this plan is limited to formatting, build, and diff checks.

## Review Focus

- Dragging within a single cell must preserve native text selection; crossing a cell boundary must promote the gesture to a cell range.
- Shift-click and drag must resolve the same rectangular source-cell set across all three views, including valid spans and vertical merges.
- Clipboard serialization must use pending cell text drafts and must not steal a hyperlink Cmd/Ctrl-click.
- Delete on a populated table must clear only selected cells; Delete on a fully empty table must remove it; stale or unsupported targets must not partially mutate the DOCX.
- Ruler track width and tick scale must remain tied to the page text area when the caret or cell range is inside a table.

---

## File map

- `internal/document/word.go`: declare the format-neutral table selection mutation result and optional editor capability.
- `internal/document/word.go`, `internal/format/docx/word_projection_table.go`: expose legacy horizontal-merge presence as neutral table metadata so every view can fail closed consistently.
- `internal/format/docx/word_table_selection.go` (new): resolve projected table/cell IDs, validate requests, clear selected XML cell content or remove an empty table, and apply one history edit.
- `internal/desktopui/session_state.go`: store the active cell-range selection and in-progress table drag using persistent state.
- `internal/desktopui/word_table_selection.go` (new): logical-grid range resolution, selection validation, cell-range serialization, and shared flow/page selection helpers.
- `internal/desktopui/word_layout_blocks.go`: expose stable per-cell pointer regions and selection visuals for Web Layout.
- `internal/desktopui/word_page_interaction.go`, `internal/desktopui/word_page_view.go`, `internal/desktopui/word_page_paint.go`: hit-test, route, and paint cell ranges in the shared Continuous/Print page-scene path.
- `internal/desktopui/word_page_caret.go`, `internal/desktopui/word_paragraph_layout.go`, `internal/desktopui/word_selection_edit.go`: give table-range keyboard actions precedence while preserving text editing when no cell range is active.
- `internal/desktopui/word_clipboard.go`, `internal/desktopui/word_toolbar_events.go`, `internal/desktopui/word_toolbar_dispatch.go`: serialize/write selected cells and route both shortcut and Ribbon copy actions.
- `internal/desktopui/word_commands.go`, `internal/desktopui/word_command_async_results.go`: execute table edits on the managed mutation path and apply results to the active view state.
- `internal/desktopui/word_ruler_geometry.go`, `internal/desktopui/word_ruler.go`: separate page ruler track geometry from current paragraph/cell indent marker geometry.

### Task 1: Add the neutral table-selection mutation and DOCX implementation

**Files:**
- Modify: `internal/document/word.go`
- Create: `internal/format/docx/word_table_selection.go`
- Reference: `internal/format/docx/word_text_edit.go`, `internal/format/docx/word_table_insert.go`, `internal/format/docx/word_layout_projection_table.go`

**Interface:**
- Produce `document.WordTableSelectionEditResult` with outcomes `WordTableSelectionNoChange`, `WordTableSelectionCellsCleared`, and `WordTableSelectionTableDeleted`.
- Produce optional capability `document.WordTableSelectionEditor` with `DeleteWordTableSelection(tableID string, cellIDs []string) (WordTableSelectionEditResult, error)`.
- Implement the method on `*docx.Document` and add a compile-time interface assertion.

- [x] Validate nonempty, unique table/cell IDs under the DOCX document lock. Resolve the table block and source cell order using the same table/row/cell projection IDs used by `word_projection_table.go`; reject all invalid targets before preparing XML changes.
- [x] Determine table emptiness from source cell payloads, ignoring cell/paragraph formatting and empty paragraph scaffolding. If any table cell has content, clear requested cell contents while preserving `tcPr`, the first `pPr`, and table geometry; if every table cell is empty, remove the complete table node regardless of range size.
- [x] Return `WordTableSelectionNoChange` when the table contains content but all requested cells are already empty. Apply a successful XML patch once through `applyWordDocumentXMLLocked` so reparse/history remain one undoable operation.
- [x] Keep mutation atomic on malformed XML, stale IDs, or unsupported structure: return an error before applying the patch, or rely on the existing `applyWordDocumentXMLLocked` rollback path if reparse fails.
- [x] Run `gofmt` on changed Go files and `go build ./internal/document ./internal/format/docx`.

### Task 2: Add logical-grid selection state, range mapping, and TSV serialization

**Files:**
- Modify: `internal/desktopui/session_state.go`
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_projection_table.go`
- Create: `internal/desktopui/word_table_selection.go`
- Reference: `internal/document/word.go`, `internal/wordlayout/model.go`

**Interfaces:**
- Add private `wordTableSelection` state with `documentID`, `tableID`, `anchorCellID`, and `focusCellID`.
- Add private `wordTableSelectionDrag` state with pointer identity, source view/page, anchor cell, and normalized drag origin.
- Add `wordTableCellRange(table document.WordTable, anchorCellID, focusCellID string) ([]string, error)` to resolve a rectangular range in logical grid order.
- Add `wordTableSelectionTSV(table document.WordTable, cellIDs []string, drafts map[string]string) (string, error)` to serialize selected cells in logical row/column order.
- Add neutral `WordTable.HasLegacyHorizontalMerge bool`, populated from DOCX's legacy `w:hMerge` markers.

- [x] Implement grid-coordinate resolution using `GridSpan` and supported vertical-merge owner/continuation state. Expand a range to include every merged region it intersects and emit each owning source cell ID once; return an error for ambiguous `hMerge` or malformed geometry.
- [x] Serialize selected range values as TSV: tab between columns, newline between rows, newline between paragraphs, hyperlink display strings as text, and `[图片]` for images. Substitute draft text for committed run text when present. Leave covered merge positions empty.
- [x] Add validation that clears a stored range when the document/table/cell IDs no longer exist or the document session changes. Do not change text caret or editor selection when a range is created.
- [x] Run `gofmt` and `go build ./internal/desktopui`.

### Task 3: Wire pointer selection and range highlighting into all views

**Files:**
- Modify: `internal/desktopui/word_layout_blocks.go`
- Modify: `internal/desktopui/word_page_interaction.go`
- Modify: `internal/desktopui/word_page_view.go`
- Modify: `internal/desktopui/word_page_paint.go`
- Modify: `internal/desktopui/word_table_selection.go`

**Interfaces:**
- Consume `wordTableSelection`, `wordTableSelectionDrag`, and `wordTableCellRange` from Task 2.
- Web Layout cell bounds and page-scene `TableCellPlacement` both resolve to the same source cell IDs.

- [x] Add persistent per-cell pointer tags in `documentSessionState`; avoid constructing persistent Gio widgets or event tags as throwaway frame-local values.
- [x] In Web Layout, collect cell bounds in flow-canvas coordinates and route press, drag, release, Shift-click, Escape, and outside-click through the shared range state. Keep within-cell text dragging on the existing editor path and promote only after entering a different cell.
- [x] In the shared Continuous/Print page interaction, hit-test `TableCellPlacement` using the existing page transform and normalized pointer coordinates. Preserve image hit behavior and Command/Ctrl-click hyperlink navigation precedence.
- [x] Paint an independent range highlight over every selected cell in Web Layout and in page scenes. Keep current text selection/caret painting behavior when no cell range is active.
- [x] Run `gofmt` and `go build ./internal/desktopui`.

### Task 4: Route copy and Delete through cell selection and managed mutation flow

**Files:**
- Modify: `internal/desktopui/word_clipboard.go`
- Modify: `internal/desktopui/word_page_caret.go`
- Modify: `internal/desktopui/word_paragraph_layout.go`
- Modify: `internal/desktopui/word_selection_edit.go`
- Modify: `internal/desktopui/word_commands.go`
- Modify: `internal/desktopui/word_command_async_results.go`
- Modify: `internal/desktopui/word_toolbar_events.go`
- Modify: `internal/desktopui/word_toolbar_dispatch.go`
- Modify: `internal/desktopui/session_state.go`

**Interfaces:**
- Consume `document.WordTableSelectionEditor` from Task 1 and range serialization/state from Task 2.
- Add `queueWordTableSelectionDelete(state *documentSessionState) bool` and `startWordTableSelectionDelete(state *documentSessionState)`; publish completion through `wordCommandResult`.

- [x] Make Cmd/Ctrl+C with a cell range active write `wordTableSelectionTSV` through Gio `clipboard.WriteCmd` before the existing text-range copy logic. Route the Ribbon Copy action through the same helper. With no cell range, preserve existing run and document text copying.
- [x] Make Delete with a cell range active take precedence over character/text-selection deletion in page and flow editors. Preserve Backspace and all no-cell-range editing behavior.
- [x] Before mutation, reject active IME composition and flush pending text drafts using the existing Word draft flow. Capture table/cell IDs in an intent so async completion cannot apply to a different selection.
- [x] Run `DeleteWordTableSelection` through `runWordMutation` on the managed background path. Add an explicit async result branch that reports adapter errors, updates the neutral block snapshot on success, clears stale run/caret/range references, and schedules the normal layout refresh.
- [x] Use one adapter call per Delete action; do not issue per-cell XML edits from the UI. Keep no-op results from adding history or marking the document dirty.
- [x] Run `gofmt` and `go build ./internal/desktopui ./internal/format/docx`.

### Task 5: Keep ruler scale stable while retaining cell paragraph context

**Files:**
- Modify: `internal/desktopui/word_ruler_geometry.go`
- Modify: `internal/desktopui/word_ruler.go`

**Interfaces:**
- Keep `wordRulerGeometryForState` as the source of page and paragraph ruler geometry.
- Extend `wordRulerGeometry` so page text-track bounds are distinct from the current paragraph's marker origin and indent limits.

- [x] Compute ruler ticks, text-area fill, and overall track width from the accepted page `Content` rectangle, regardless of whether the caret is inside a table.
- [x] Compute paragraph indent marker positions from the active paragraph area; for table paragraphs, retain the cell text area and its safe drag limits without redefining the page track or its tick scale.
- [x] Ensure a cell-range selection does not replace the text caret's paragraph context for the ruler.
- [x] Run `gofmt` and `go build ./internal/desktopui`.

### Task 6: Final source and build validation

**Files:**
- Review all files changed by Tasks 1–5.

- [x] Run `gofmt` on every changed Go file.
- [x] Run `go build ./...`; expected result is a successful build. Existing platform linker warnings may still be emitted, but no Go compile errors should remain.
- [x] Run `git diff --check`; expected result is no whitespace errors.
- [x] Review the final diff against the approved design and confirm unrelated hyperlink underline edits remain intact.

## Execution notes

Use the current authorized checkout at `/Users/mima1234/project/glance-go`; the repository's AGENTS.md prohibits creating or relocating worktrees without explicit authorization. No commit is included in this plan.

## Self-review

- Spec coverage: selection state, drag/Shift-click, all three view modes, highlight, TSV copy, image placeholder, draft-aware serialization, Delete semantics, table-wide emptiness, undo, stale/unsupported targets, async mutation, ruler geometry, hyperlink precedence, and merged-cell behavior are assigned above.
- The neutral API and command result type are defined before dependent UI tasks, and later task interfaces reuse the same names.
- Automated test changes and test commands are omitted because the current session instructions prohibit adding or running tests unless the user explicitly asks for testing or verification.
