# Excel AutoSum Formula Semantics

## Goal

Make the Formula ribbon's Sum, Average, Count, Max, and Min commands behave like Excel AutoSum: write formulas into the inferred destination cells so later edits to the source range recalculate the results.

## User-visible behavior

- A multi-row selection produces one formula per selected column in the inferred result row.
- A one-row, multi-column selection produces one formula in the inferred result column.
- If the selection has an empty trailing row or column, the formula is placed in that trailing dimension and excludes the destination from its source range. Otherwise the formula is placed immediately after the selection.
- With a single cell selected, infer the contiguous numeric run above it first, then to its left, retaining the current bounds of 100 rows and 50 columns. Write the formula into the selected cell.
- Sum, Average, Count, Max, and Min generate `SUM`, `AVERAGE`, `COUNT`, `MAX`, and `MIN` formulas, respectively. Formula inputs remain numeric ranges; Excel owns ongoing recalculation semantics.
- Nonnumeric cells are excluded when discovering the adjacent run. In a multi-cell selected range, nonnumeric values remain in the formula range and are ignored by Excel's aggregate functions.
- Each command writes all output formulas as one undoable workbook mutation. Repeating the same operation over the same destination/formula set is a no-op.
- The command reports the destination cells and does not claim a calculated result when the local formula engine cannot evaluate it.

## Architecture

- Add neutral `CellFormulaEdit` and `SpreadsheetFormulaBatchEditor` contracts to `internal/document`; keep Excelize types inside `internal/format/xlsx`.
- Implement a validated, atomic formula batch in the XLSX adapter. Validate the worksheet, coordinates, duplicate destinations, and the 8192 UTF-16-unit formula limit before mutation. Snapshot once, write all formulas, update linked values, record one history entry, and restore the snapshot on any write/history error.
- Add a medium-risk, revision-checked `spreadsheet.formulas.set` application capability for explicit formula batches. Do not permit formula strings through the value-only `spreadsheet.cells.set` capability.
- Route desktop AutoSum through that application command when a session is registered, and through the neutral adapter contract otherwise. Publish successful formula destinations so the grid refreshes and navigates to the first result.
- Preserve the current bounded range inference and selection behavior; change aggregate planning to return formulas instead of computed numeric values.

## Failure and fidelity boundaries

- Reject malformed batches atomically: invalid worksheet/coordinates, invalid UTF-8 or empty formulas, formulas beyond 8192 UTF-16 units, duplicate destinations, or a batch over 10,000 cells.
- A formula write failure or history-snapshot failure leaves the workbook, dirty state, revision, and undo/redo stacks unchanged.
- No-op formula batches do not create history entries or advance the document revision.
- Glance's preview continues to use the existing bounded formula calculation support and cached values; the change does not claim full Excel formula-engine parity. Saved formulas are native XLSX formulas and are recalculated by Excel.
- Formula batches preserve cell styles and unrelated package parts through the existing adapter snapshot/history/save boundary.

## Verification

- Adapter tests cover batch formulas, no-op behavior, full validation before mutation, atomic rollback, dirty/history state, single-step undo/redo, and XLSX save/reopen formula persistence.
- Application tests verify the revision-checked capability routes a batch exactly once, rejects malformed or duplicate targets atomically, and reports `changed` accurately.
- UI tests cover all five formula kinds for horizontal, vertical, trailing-blank, and single-cell selections; verify destination inference, no-op behavior, one-step undo, result navigation, and selection/editor focus safety.
- Run focused tests for `internal/document`, `internal/format/xlsx`, `internal/application`, and the relevant `internal/desktopui` tests; inspect `git diff --check`.
