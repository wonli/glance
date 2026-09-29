# Excel Toolbar Design

## Goal

Bring the main spreadsheet editing tools from the sibling `tools` project into `glance-go` as a Gio-native toolbar, backed by real workbook operations and the existing format-neutral document boundary.

## User-facing scope

The spreadsheet toolbar has **Home** and **Data** tabs and keeps the existing grid and sheet tabs as the main editing surface.

Home provides:

- Undo and redo for spreadsheet edits.
- Copy and cut for the current cell or selected range, plus paste from the system clipboard.
- Common cell formatting: bold, italic, underline, font color, fill color, horizontal alignment, number format, border presets, and clear formatting.
- Insert/delete selected rows and columns.
- Save when the workbook is dirty and save is supported.

Data provides ascending/descending sort and a reversible value filter for the selected column within the selected range.

The grid must support rectangular range selection so range formatting, clipboard operations, sorting, and filtering have explicit targets. Existing search, cell editing, workbook opening, and row/column resizing continue to work. Formula calculation, formula entry, charts, pivot tables, macro editing, conditional formatting, and full Excel visual fidelity are outside this toolbar scope.

## Interaction decisions

- Persistent Gio widgets and toolbar state belong to the active document session, so switching tabs does not reset selection or toolbar state unexpectedly.
- The toolbar reflects the active cell's common style. Formatting applies to the selected range; when mixed values exist, the toolbar uses a neutral/mixed state rather than claiming the entire range has one value.
- Structural operations target the selected row or column. Delete operations and destructive cut/clear operations require a valid selection; invalid operations are disabled and also rejected below the UI.
- Clipboard copy produces tab-separated plain text. Paste accepts tab/newline-delimited text into the grid; HTML formatting is not required in this iteration.
- Sort uses the active cell's column as its single key and sorts every row in the selected range, including its first row. Numeric keys compare numerically, text keys compare case-insensitively, and blank keys sort last in ascending order. Formula cells in the sorted region are rejected without changing data.
- Filter uses the active cell's displayed value, treats the first selected row as a header, and hides nonmatching rows in the remaining selected rows. Matching is exact; blank displayed values can be selected. Clearing the filter restores all rows without changing workbook data. Filter state is transient view state, not a persisted worksheet edit.
- A no-op operation does not mark the workbook dirty or add an undo entry.
- Editing and save work must preserve the existing dirty-state and temporary-sibling-file replacement guarantees. On failure, keep the prior workbook/session data and report the failure.

## Architecture

The data path remains:

```text
Gio toolbar/grid -> spreadsheet intents and selection -> neutral document operations -> XLSX adapter -> temporary sibling save/reopen gate
```

`internal/document` gains format-neutral spreadsheet operation types and optional capability interfaces rather than exposing Excelize values. The XLSX adapter implements only operations whose behavior and save safety are covered by fixtures. `internal/desktopui` owns persistent widgets, selection, transient filter state, clipboard integration, command dispatch, async completion, and user-visible errors. Layout and drawing stay separate from event/controller logic; workbook reads and writes stay outside frame layout code.

Where an operation can change many cells or worksheet structure, represent it as a reversible edit with enough before/after state to support undo/redo. History belongs to the open document session and is not serialized as workbook content. On successful save, rebase history against the saved state so a later undo is allowed and marks the workbook dirty again; replace history only when a different document session is installed. Keep bounded viewport reads and invalidate/reload only affected grid regions after edits.

## Error handling and safety

- Enforce edit capability, valid selection/range, worksheet limits, and operation-specific input constraints in the controller and adapter.
- Reject unsupported format operations explicitly; do not show enabled controls for read-only `.xls` or other non-editable sessions.
- Preserve dirty state when an operation or save fails. Do not replace the source until a temporary sibling workbook has been written successfully.
- Before enabling an operation for overwrite, add round-trip coverage that reopens a temporary workbook and checks changed content plus representative untouched values, formulas/cached results, styles, merges, dimensions, and relevant package content.
- Update `docs/office-support.md` and the compatibility matrix at `../tools/docs/compatibility/office-feature-matrix.md` with evidence and known gaps as capabilities land.

## Verification

- Unit tests cover neutral range/history behavior, adapter operation limits and no-op behavior, and controller selection/dirty-state transitions.
- Adapter tests use synthetic workbooks with sparse ranges, styled and merged cells, formulas/cached values, and row/column dimensions; each destructive edit is applied to a temporary copy, saved, reopened, and checked for both intended edits and representative untouched content.
- UI interaction tests cover single-cell and multi-cell selection, copy/paste of blank and styled cells, format changes, undo/redo, row/column operations, sheet actions, sorting, filter clearing, and errors while preserving a draft/dirty state.
- Run the repository test suite and, when the local corpus is available, the documented Office sample regression command. Manual Gio checks confirm keyboard focus, clipboard integration, selection feedback, and responsiveness during reads/saves.

## Delivery phases

1. Add rectangular selection and the Gio Home/Data toolbar shell, with capability-aware controls.
2. Add neutral formatting operations, common XLSX formats, save integration, and edit history.
3. Add copy/cut/paste and range-aware clipboard behavior.
4. Add row/column insertion/deletion and worksheet create/rename/delete operations.
5. Add range sorting and transient value filtering.
6. Complete round-trip and interaction verification; update support and compatibility documentation.

Each phase must leave the application buildable and update its Todo/implementation plan status. Do not enable write/save capability for a new operation before its preservation evidence passes.
