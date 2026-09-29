# Excel Toolbar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the approved Home/Data spreadsheet toolbar in Gio with real, reversible XLSX operations and evidence-based save safety.

**Architecture:** Gio keeps selection, persistent widgets, transient filter state, and commands in each document session. Format-neutral optional interfaces in `internal/document` express workbook edits; `internal/format/xlsx` implements and tests them, and UI commands run outside frame layout before invalidating bounded grid windows.

**Tech Stack:** Go, Gio v0.10.2, Excelize v2.11.0, Go `testing`.

**Spec:** `docs/superpowers/specs/2026-09-29-excel-toolbar-design.md`

## Global Constraints

- Keep third-party Office library types behind format adapters.
- Keep the Gio window loop responsive; parsing and saves run outside frame/layout code.
- Persistent Gio widgets stay in stable per-session state.
- Formula calculation and formula entry remain unsupported; sort rejects formula cells in its target range.
- Overwrite remains enabled only for operations with save/reopen evidence; failed edits/saves preserve source data and dirty state.
- Do not include existing `main` worktree edits in this feature branch.

## Review Focus

- Undo after a successful save must restore the last saved value and mark the workbook dirty; cover in the history task.
- Mixed-format range operations must update only the requested property and retain other style properties; cover in the format task.
- TSV paste with ragged rows, blank fields, and trailing newline must map cells predictably without erasing cells outside its rectangle; cover in the clipboard task.
- Deleting the last worksheet or editing at XLSX row/column limits must fail atomically and retain selection/data; cover in structural operations.
- Sorting a region with a formula or filtering an empty displayed value must follow the spec without partial edits; cover in the data operations task.

---

## File Map

- `internal/document/model.go`: neutral selection ranges, formatting patches, structure/sort request DTOs, optional spreadsheet capability interfaces.
- `internal/format/xlsx/workbook.go`: adapter implementations and atomic validation for formatting, history, clipboard batch edits, structure edits, and sorting.
- `internal/format/xlsx/workbook_test.go`: synthetic workbook preservation and save/reopen tests.
- `internal/desktopui/session_state.go`: persistent selection, toolbar widgets, history state, filter view state, and undo stack/session baselines.
- `internal/desktopui/grid_selection.go` (new): normalized pointer/key selection math for single cells and rectangular ranges.
- `internal/desktopui/spreadsheet_commands.go` (new): UI command dispatch, range-to-neutral request conversion, async completion, error/dirty updates, and targeted grid invalidation.
- `internal/desktopui/layout_spreadsheet_toolbar.go` (new): Gio Home/Data tabs and controls; drawing only.
- `internal/desktopui/layout.go`: place the toolbar in the spreadsheet view and route persistent widget events to commands.
- `internal/desktopui/grid_hit.go`, `grid_layout.go`, `grid_view.go`: visible-cell range hit mapping and selection rendering/invalidation.
- `internal/desktopui/grid_*_test.go` plus `spreadsheet_commands_test.go` (new): selection, toolbar state, and command behavior.
- `docs/office-support.md`: feature status, limits, and evidence.
- `../tools/docs/compatibility/office-feature-matrix.md`: sibling compatibility matrix entries and fixture evidence.

### Task 1: Rectangular Grid Selection and Toolbar Shell

**Files:**
- Create: `internal/desktopui/grid_selection.go`
- Create: `internal/desktopui/layout_spreadsheet_toolbar.go`
- Modify: `internal/desktopui/session_state.go`, `internal/desktopui/layout.go`, `internal/desktopui/grid_hit.go`, `internal/desktopui/grid_layout.go`, `internal/desktopui/grid_paint.go`
- Test: `internal/desktopui/grid_selection_test.go`, `internal/desktopui/grid_view_test.go`

**Interfaces:**
- Produces `type CellSelection struct { Anchor, Active CellPosition }`, `func (s CellSelection) Bounds() document.CellRange`, and `func selectionAtPoint(v *gridViewport, point image.Point) (CellPosition, bool)`.
- `CellPosition` uses zero-based UI row/column coordinates; conversion to `document.CellRange` is one-based inclusive.

- [x] Add failing tests for click selection, Shift extension, drag extension, normalized bounds, and clamping to XLSX limits.
- [x] Run `go test ./internal/desktopui -run 'TestCellSelection' -count=1`; expected new tests fail because selection model is absent.
- [x] Implement persistent per-session selection state and wire visible grid hits through one normalized coordinate boundary.
- [x] Add rendering assertions for anchor/active/range outline and ensure merged-cell clicks select the merge anchor.
- [x] Add Home/Data toolbar shell with persistent tab/button state, capability-aware disabled controls, and no document mutations yet.
- [x] Run `go test ./internal/desktopui`; expected selection and layout tests pass.
- [x] Commit as `feat: add spreadsheet selection and toolbar shell`.

### Task 2: Neutral Format Operations and XLSX Formatting

**Files:**
- Modify: `internal/document/model.go`, `internal/format/xlsx/workbook.go`, `internal/format/xlsx/workbook_test.go`
- Create: `internal/format/xlsx/format_edit.go`
- Modify: `internal/desktopui/session_state.go`, `internal/desktopui/spreadsheet_commands.go`, `internal/desktopui/layout_spreadsheet_toolbar.go`
- Test: `internal/desktopui/spreadsheet_commands_test.go`

**Interfaces:**
- Add `type CellFormatPatch struct` with optional `Bold`, `Italic`, `Underline *bool`; `FontColor`, `FillColor`, `HorizontalAlignment`, `NumberFormat *string`; `BorderPreset *string`; `ClearFormat bool`.
- Add `type SpreadsheetFormatter interface { SetCellFormats(sheet string, bounds CellRange, patch CellFormatPatch) error }`.
- `(*Workbook).SetCellFormats(sheet string, bounds document.CellRange, patch document.CellFormatPatch) error` validates bounds, preserves untouched style properties, and leaves no-op workbooks clean.
- `(*App).applyCellFormat(patch document.CellFormatPatch) error` dispatches the selected bounds and refreshes the bounded window after success.

- [x] Add failing adapter tests for each supported style patch, mixed prior styles, clear format, invalid ranges, and no-op dirty behavior.
- [x] Run `go test ./internal/format/xlsx -run 'TestSetCellFormats' -count=1`; expected tests fail because `SpreadsheetFormatter` is absent.
- [x] Implement the neutral patch/interface and XLSX conversion using adapter-only Excelize style types.
- [x] Add save/reopen tests that compare edited styles and untouched styled, merged, formula, and dimension fixtures.
- [x] Connect Home formatting controls and active-cell/mixed-selection state without constructing widgets during layout.
- [x] Run `go test ./internal/format/xlsx ./internal/desktopui`; expected formatter, preservation, and toolbar state tests pass.
- [x] Commit as `feat: apply spreadsheet cell formatting`.

### Task 3: Reversible Edit History and Save Rebase

**Files:**
- Modify: `internal/document/model.go`, `internal/format/xlsx/workbook.go`, `internal/format/xlsx/format_edit.go`, `internal/format/xlsx/workbook_test.go`
- Create: `internal/format/xlsx/history.go`
- Modify: `internal/desktopui/session_state.go`, `internal/desktopui/spreadsheet_commands.go`, `internal/desktopui/app.go`, `internal/desktopui/layout_spreadsheet_toolbar.go`
- Test: `internal/desktopui/spreadsheet_commands_test.go`

**Interfaces:**
- Add `type SpreadsheetHistory interface { Undo() (bool, error); Redo() (bool, error); RebaseHistory() }`.
- Adapter history records reversible cell/style/structure snapshots, has bounded capacity, clears redo on a new edit, and reports no-op as unapplied.
- Save completion calls `RebaseHistory`; a later undo remains available and marks dirty.

- [x] Add failing adapter tests for undo/redo cell edits, format edits, no-op, redo invalidation, failed edit atomicity, and post-save rebase semantics.
- [x] Run `go test ./internal/format/xlsx -run 'TestWorkbookHistory' -count=1`; expected tests fail because history capability is absent.
- [x] Implement bounded reversible history behind the neutral capability interface.
- [x] Wire persistent undo/redo controls and history state updates through successful edit/save completion only.
- [x] Run `go test ./internal/format/xlsx ./internal/desktopui`; expected history and dirty-state tests pass.
- [x] Commit as `feat: add spreadsheet undo and redo`.

### Task 4: Clipboard Copy, Cut, and Paste

**Files:**
- Create: `internal/desktopui/spreadsheet_clipboard.go`
- Modify: `internal/desktopui/session_state.go`, `internal/desktopui/spreadsheet_commands.go`, `internal/desktopui/layout_spreadsheet_toolbar.go`, `internal/desktopui/layout.go`
- Modify: `internal/format/xlsx/workbook.go`, `internal/format/xlsx/workbook_test.go`
- Test: `internal/desktopui/spreadsheet_clipboard_test.go`, `internal/desktopui/spreadsheet_commands_test.go`

**Interfaces:**
- Add `type CellEdit struct { Row, Column int; Value string }` and `type SpreadsheetBatchEditor interface { SetCells(sheet string, edits []CellEdit) error }`.
- `func encodeTSV(cells [][]document.Cell) string` exports selected displayed values with tabs/newlines.
- `func decodeTSV(text string) ([][]string, error)` preserves empty fields and trims only the final record created by a trailing newline.
- `(*App).copySelection() (string, error)` and `(*App).pasteClipboard(text string) error` operate on the selected rectangle; toolbar cut writes TSV then asynchronously clears the source range.

- [x] Add failing tests for TSV round trip, ragged rows, empty fields, trailing newline, and atomic overflow rejection at worksheet limits.
- [x] Run `go test ./internal/desktopui -run 'Test.*Clipboard' -count=1`; expected tests fail because clipboard encoding/commands are absent.
- [x] Implement TSV conversion, Gio/system clipboard event handling, atomic batch edit, and cut as copy followed by one reversible clear edit.
- [x] Add adapter save/reopen tests for pasted constants and untouched styled/merged/formula content.
- [x] Run `go test ./internal/format/xlsx ./internal/desktopui`; expected clipboard and preservation tests pass.
- [x] Commit as `feat: support spreadsheet clipboard operations`.

### Task 5: Row, Column, and Worksheet Structure Operations

**Files:**
- Modify: `internal/document/model.go`, `internal/format/xlsx/workbook.go`, `internal/format/xlsx/workbook_test.go`
- Create: `internal/format/xlsx/structure_edit.go`
- Modify: `internal/desktopui/session_state.go`, `internal/desktopui/spreadsheet_commands.go`, `internal/desktopui/layout_spreadsheet_toolbar.go`, `internal/desktopui/layout.go`
- Test: `internal/desktopui/spreadsheet_commands_test.go`

**Interfaces:**
- Add `type SpreadsheetStructureEditor interface { EditStructure(edit StructureEdit) error }` and a neutral tagged `StructureEdit` for create/rename/delete sheet and insert/delete rows/columns.
- `(*Workbook).EditStructure(edit document.StructureEdit) error` validates sheet identity/name, last-sheet deletion, counts, and XLSX limits before mutation.
- `(*App).applyStructure(edit document.StructureEdit)` runs in the background and refreshes sheet metadata, selection, and the grid window only after success.

- [x] Add failing adapter tests for create/rename/delete sheet, insert/delete rows/columns, invalid names, deleting the last sheet, bounds, no-op, and atomic failure.
- [x] Run `go test ./internal/format/xlsx -run 'TestEditStructure' -count=1`; expected tests fail because structure capability is absent.
- [x] Implement supported adapter operations and reversible snapshots; reject unsupported cases explicitly.
- [x] Add save/reopen tests checking changed dimensions/sheet names plus representative untouched cells, formulas, styles, and merged regions.
- [x] Wire Home controls and sheet-tab create/rename/delete interactions; preserve current selection or rebase it to a valid cell after success.
- [x] Run `go test ./internal/format/xlsx ./internal/desktopui`; expected structure and preservation tests pass.
- [x] Commit as `feat: edit spreadsheet structure`.

### Task 6: Sorting and Transient Value Filtering

**Files:**
- Modify: `internal/document/model.go`, `internal/format/xlsx/workbook.go`, `internal/format/xlsx/workbook_test.go`
- Create: `internal/format/xlsx/sort.go`
- Modify: `internal/desktopui/session_state.go`, `internal/desktopui/spreadsheet_commands.go`, `internal/desktopui/layout_spreadsheet_toolbar.go`, `internal/desktopui/grid_layout.go`, `internal/desktopui/grid_view.go`
- Test: `internal/desktopui/spreadsheet_commands_test.go`, `internal/desktopui/grid_view_test.go`

**Interfaces:**
- Add `type SpreadsheetSorter interface { SortRange(request SortRequest) error }` with sheet, one-based inclusive `CellRange`, key column, and descending flag.
- `(*Workbook).SortRange(request document.SortRequest) error` compares finite numeric strings numerically, text case-insensitively, blanks last ascending, preserves row cell styles, rejects formula-containing ranges, and is reversible.
- Session filter state is `*ValueFilter{Sheet string; Bounds CellRange; Column int; Value string}`; filtering hides rows after the first selected header row and is cleared without workbook mutation.

- [x] Add failing adapter tests for numeric/text/blank ordering, style movement with rows, formula rejection without partial mutation, bounds, and undo.
- [x] Run `go test ./internal/format/xlsx -run 'TestSortRange' -count=1`; expected tests fail because sorter is absent.
- [x] Implement adapter sort request and reversible row snapshots.
- [x] Add failing UI tests for active-column sort, header-preserving transient filtering, exact/blank matches, clear-filter restoration, and filter reset on sheet/session change.
- [x] Implement Data controls and view-row filtering without modifying workbook row indices or virtual geometry.
- [x] Run `go test ./internal/format/xlsx ./internal/desktopui`; expected sort/filter tests pass.
- [x] Commit as `feat: add spreadsheet sort and filter`.

### Task 7: Round-Trip Audit, Documentation, and Full Verification

**Files:**
- Modify: `docs/office-support.md`, `../tools/docs/compatibility/office-feature-matrix.md`
- Test: `internal/format/xlsx/workbook_test.go`, existing `internal/desktopui/*_test.go`

- [x] Audit each toolbar operation against the spec and adapter preservation fixtures; identify any unproven operation and keep its write control disabled until evidenced.
- [x] Run `GOCACHE=/tmp/glance-go-gocache go test ./... -count=1`; all repository packages pass.
- [x] Run `GOCACHE=/tmp/glance-go-gocache GLANCE_OFFICE_TEST_DIR="$HOME/Downloads/office" go test ./...`; recorded blocker: `人事表格-员工绩效表.xlsx` has malformed `xl/styles.xml` (`<font>` closed by `</name>`), so two corpus open tests fail before round-trip checks.
- [x] Attempted `GOCACHE=/tmp/glance-go-gocache go run ./cmd/desktop`; the process emitted macOS LaunchServices/XPC connection errors and no accessible app window appeared in the desktop inventory, so manual Gio smoke coverage remains unverified.
- [x] Update support status and compatibility evidence for every implemented/partial/unsupported feature.
- [x] Commit final audit fixes and support evidence as `b68e26a fix: preserve spreadsheet history and values`.
