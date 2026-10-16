# Spreadsheet Selection AutoFit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Excel AutoFit apply to every selected row or column and undo/redo the whole operation as one edit.

**Architecture:** Add an optional range visitor and a batched dimension editor at the neutral spreadsheet boundary. XLSX scans the worksheet once for the selected dimension range and writes changed dimensions under one snapshot. The UI measures each populated row/column in the background, commits the batch, then updates sparse grid geometry from the returned result.

**Tech Stack:** Go, Gio, Excelize v2.11.0, existing document capability interfaces and workbook snapshot history.

**Spec:** `docs/office-support.md` (Spreadsheet active row height and column width); `docs/compatibility/office-feature-matrix.md` (Excel active row/column AutoFit).

## Global Constraints

- Keep third-party Office library types behind `internal/format/xlsx`.
- Keep worksheet scans, text measurement, and workbook writes outside the Gio layout/frame loop.
- Treat selection bounds as one-based document coordinates; preserve reverse drag selections through `CellSelection.Bounds()`.
- A multi-dimension AutoFit action creates at most one undo/redo entry and rolls back all dimensions on failure.
- Continue to exclude merged-cell text and preserve untouched worksheet package parts.
- Do not expand a full worksheet into a dense grid; scan the worksheet XML once and retain only sparse changed dimensions.

## Review Focus

- Cell-range and whole-row/column selections target the full selected dimension interval, including a reverse selection.
- Wrapped row text measures against each cell's actual worksheet column width, including offscreen cells.
- Empty selected dimensions remain unchanged; an entirely empty selection reports a no-op.
- Dimension validation or write failure leaves all sizes and history unchanged.
- Undo and redo restore every size in a multi-row/column AutoFit as one operation.

---

### Task 1: Stream cells for a selected dimension range

**Files:**
- Modify: `internal/document/model.go`
- Modify: `internal/format/xlsx/workbook_dimension.go`
- Test: `internal/format/xlsx/workbook_dimension_test.go`

**Interfaces:**
- Produce `SpreadsheetDimensionRangeCellReader.VisitDimensionRangeCells(sheet string, axis SpreadsheetDimensionAxis, start, end int, visit func(SpreadsheetDimensionCell) error) error`.
- Extend each visited cell with its one-based dimension index and current row height/column width while retaining the existing perpendicular `Index` and row `ColumnWidthChars`.
- Keep `VisitDimensionCells` as a compatibility wrapper for a single index.

- [x] **Step 1: Write failing tests** for row and column range visitation, dimension coordinates/current sizes, actual worksheet column widths, and merged-cell exclusion.
- [x] **Step 2: Run the new XLSX tests** and confirm they fail because the range reader is not implemented.
- [x] **Step 3: Implement one-pass sparse range visitation** in the XLSX adapter, caching styles and dimension sizes during the scan.
- [x] **Step 4: Run the focused dimension visitor tests** and confirm they pass.

### Task 2: Commit a dimension batch atomically

**Files:**
- Modify: `internal/document/model.go`
- Modify: `internal/format/xlsx/workbook_edit.go`
- Modify: `internal/format/xlsx/workbook.go`
- Test: `internal/format/xlsx/workbook_resize_test.go`

**Interfaces:**
- Produce `SpreadsheetDimensionBatchEditor.SetDimensionSizes(sheet string, axis SpreadsheetDimensionAxis, sizes []SpreadsheetDimensionSize) error`, where each item contains one-based `Index` and format-unit `Value`.
- Validate the full input before mutation, skip no-op entries, write one snapshot/history entry, and roll back the full snapshot if any write fails.

- [x] **Step 1: Write failing tests** for multi-row and multi-column edits, one-step undo/redo, duplicate/invalid input atomicity, and an all-no-op batch that creates no history.
- [x] **Step 2: Run the focused resize tests** and confirm they fail because the batch editor is not implemented.
- [x] **Step 3: Implement the batch editor** and route existing single-dimension setters through it without changing their public behavior.
- [x] **Step 4: Run the focused resize tests** and confirm they pass.

### Task 3: Apply AutoFit to the selected dimensions

**Files:**
- Modify: `internal/desktopui/grid_auto_fit.go`
- Modify: `internal/desktopui/spreadsheet_commands.go`
- Modify: `internal/desktopui/spreadsheet_async_format_result.go`
- Test: `internal/desktopui/grid_auto_fit_test.go`

**Interfaces:**
- Consume the range reader and batch editor from Tasks 1 and 2.
- Replace the singular dimension result with a sparse slice of `{index, value}` changes so the UI can update all measured geometry after the background operation succeeds.

- [x] **Step 1: Write failing UI/controller tests** showing that selected row/column ranges are measured and committed together, while a single-cell selection still targets one active row/column.
- [x] **Step 2: Run the focused UI tests** and confirm they fail because only the active dimension is currently used.
- [x] **Step 3: Measure each populated selected dimension off the UI loop**, apply the batch once, and update row/column geometry only after a current successful result.
- [x] **Step 4: Run the focused UI tests** and confirm they pass.

### Task 4: Record support and verify integration

**Files:**
- Modify: `docs/office-support.md`
- Modify: `docs/compatibility/office-feature-matrix.md`

- [x] **Step 1: Update the documented boundary** to state multi-row/column AutoFit behavior and its existing measurement limits.
- [x] **Step 2: Run `go test ./internal/format/xlsx -count=1` and the focused desktop UI AutoFit/resize tests.**
- [x] **Step 3: Run `git diff --check`** and review the final diff for API-boundary and undo/history correctness.
