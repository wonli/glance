# Virtualized Spreadsheet Grid Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a familiar Excel-like XLSX editor with a virtualized full-address grid, common styles, merged cells, in-place editing, two-axis scrolling, and resizable rows and columns.

**Architecture:** Keep Gio drawing and interaction in `internal/desktopui`, format-neutral windows and styles in `internal/document`, and Excelize calls in `internal/format/xlsx`. A custom viewport uses bounded visible windows and sparse dimension geometry; it never creates one widget per worksheet cell.

**Tech Stack:** Go, Gio `gioui.org v0.10.2`, Excelize `github.com/xuri/excelize/v2 v2.11.0`, Go tests.

**Spec:** `docs/superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md`

## Global Constraints

- Preserve the existing Gio and Excelize dependency versions.
- XLSX address bounds are 1,048,576 rows and 16,384 columns.
- Render and fetch only the visible range plus a small overscan margin.
- Keep Excelize and OOXML types inside `internal/format/xlsx`.
- Keep XLS read-only and DOCX behavior unchanged.
- Resize previews remain transient; persist row/column dimensions only on pointer release.
- Preserve atomic save and dirty state on failure.
- Work on branch `codex/virtualized-spreadsheet-grid` in the approved isolated worktree.
- Commit each completed task locally after its final passing test run. Do not push or merge.

## Review Focus

- Sparse and hidden dimension overrides at large row indexes must not make scroll lookup linear; test the maximum XLSX row boundary and zero-height rows in Task 1.
- Merged regions crossing a viewport boundary must retain correct anchor selection and clipping; test merge intersection and anchor geometry in Task 3.
- Empty cells beyond the used range must read, select, edit, and save without loading a huge range; test bounded windows and blank-cell round trips in Task 2 and Task 6.
- Pointer coordinate changes while scrolling during a resize must not jump the preview or commit early; test rebase, cancel, and release-only persistence in Task 5.
- Unsupported or theme/indexed style colors and save errors must not produce false styling or clear dirty state; test color fallback and adapter save failure in Task 2.

---

### Task 1: Sparse spreadsheet dimension geometry

**Files:**
- Create: `internal/desktopui/grid_geometry.go`
- Create: `internal/desktopui/grid_geometry_test.go`

**Interfaces:**
- Produces `NewAxisGeometry(count int, defaultSize float64) (*AxisGeometry, error)`.
- Produces `(*AxisGeometry).Boundary(index int) (float64, error)`, where index is a zero-based boundary in `[0,count]`.
- Produces `(*AxisGeometry).IndexAt(position float64) int`, returning a zero-based non-hidden dimension index, or `-1` if every dimension is hidden.
- Produces `(*AxisGeometry).VisibleRange(offset, extent float64, overscan int) (start, end int)`, with an exclusive end.
- Produces `(*AxisGeometry).SetSize(index int, size float64) error` and `TotalSize() float64`.
- Use a Fenwick tree of dimension deltas plus a sparse override map. Initialize the zero-delta tree in O(count); boundary lookup, inverse prefix lookup, and update are O(log count); allocate no Gio object per dimension. Implement `IndexAt` with Fenwick binary lifting over `index*defaultSize + prefixDelta` so it does not binary-search O(log count) prefix queries.
- `NewAxisGeometry` rejects non-positive counts and non-finite or non-positive default sizes. `Boundary` returns an error for indexes outside `[0,count]`. `IndexAt` clamps finite out-of-range coordinates to the first/last non-hidden dimension, skips zero-sized dimensions, and returns `-1` for non-finite positions or if all dimensions have been hidden. `VisibleRange` clamps negative offsets to zero and negative overscan to zero; non-finite offsets/extents produce an empty range.

- [ ] **Step 1: Write failing geometry tests**

Add `TestAxisGeometryBoundaryAndIndexRoundTrip`, `TestAxisGeometrySparseSizeOverridesAndHiddenDimensions`, `TestAxisGeometryBoundsAndInvalidSizes`, and `TestAxisGeometryRejectsInvalidConstruction`. Cover default sizes, a widened column, a resized row, a zero-sized hidden row, negative/NaN sizes, positions before and after the extent, zero visible extent, and the final Excel row index.

- [ ] **Step 2: Run tests and verify the expected failures**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^TestAxisGeometry' -count=1`

Expected: FAIL because `AxisGeometry` is not implemented.

- [ ] **Step 3: Implement `AxisGeometry`**

Use the exact interfaces above. `Boundary` is the prefix size before the boundary. `IndexAt` uses Fenwick binary lifting and returns the dimension containing the position, skipping zero-sized dimensions; clamp an at-or-after-total position to the last visible dimension. Reject NaN/infinite sizes, negative sizes, invalid indexes, and invalid visible-range extents. Maintain exact XLSX maximum counts in later construction, not as a hard-coded geometry restriction.

- [ ] **Step 4: Run geometry tests**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^TestAxisGeometry' -count=1`

Expected: PASS for all four tests, including the maximum row index without a linear scan.

- [ ] **Step 5: Commit Task 1**

Run: `git add internal/desktopui/grid_geometry.go internal/desktopui/grid_geometry_test.go docs/superpowers/plans/2026-09-28-virtualized-spreadsheet-grid.md && git commit -m "feat: add sparse grid dimension geometry"`

Expected: commit created on `codex/virtualized-spreadsheet-grid`; no files remain staged.

### Task 2: Neutral visible-window model and XLSX adapter

**Files:**
- Modify: `internal/document/model.go`
- Modify: `internal/format/xlsx/workbook.go`
- Modify: `internal/format/xlsx/workbook_test.go`

**Interfaces:**
- Keep `document.Spreadsheet` and its bounded legacy API intact for `.xls`.
- Add `document.GridReader` with `Window(sheet string, startRow, startColumn, rowCount, columnCount int) (SheetWindow, error)`, `SetRowHeight(sheet string, row int, heightPoints float64) error`, and `SetColumnWidth(sheet string, column int, widthChars float64) error`.
- Add `document.SheetWindow` with one-based `StartRow`/`StartColumn`, `Cells [][]Cell`, inclusive one-based intersecting `Merges []MergeRegion`, `RowHeightsPoints []float64`, and `ColumnWidthsChars []float64`.
- Add `document.CellRange` with one-based inclusive `StartRow`, `StartColumn`, `EndRow`, `EndColumn`.
- Add `document.MergeRegion` with `Range CellRange` and `Anchor Cell`, so a merge whose top-left cell is outside the visible window still has the value and style needed to paint its clipped visible portion.
- Add neutral `document.CellStyle` fields for RGB `FillColor`/`FontColor` strings, bold, italic, font size, horizontal/vertical alignment, wrapping, and four neutral `CellBorder` values ordered top, right, bottom, left. Keep `StyleID` for existing code during migration.
- `Window` rejects out-of-bounds and non-positive requests, returns exactly the requested rectangular window (including blank cells beyond the used range), includes only merge ranges intersecting that window, and returns the corresponding row/column file dimensions.
- Map Excelize RGB/ARGB style colors to normalized `#RRGGBB`; unresolved theme/indexed/automatic colors return the neutral default instead of a guessed color.

- [ ] **Step 1: Write failing adapter tests**

Add `TestWorkbookWindowIncludesStylesMergesAndDimensions`, `TestWorkbookWindowReadsBlankCellsOutsideUsedRange`, `TestWorkbookResizeSaveReopen`, `TestWorkbookWindowRejectsInvalidBounds`, `TestWorkbookWindowFallsBackForUnsupportedStyleColors`, `TestWorkbookFailedSaveKeepsDirtyAndOriginal`, and `TestEmployeeWorkbookResizePreservesStylesAndMerges`. Use synthetic workbooks with a fill/font/alignment/border style, a merged range, custom row and column sizes, a blank address well beyond the used range, and unsupported theme/indexed color cases. Assert bounded shape, one-based coordinates, style values and fallback, merge intersection, dimension values, save/reopen persistence, and dirty/original preservation after a failed save. The employee-workbook test uses a temporary copy to edit one blank cell and resize one row and column, then checks untouched representative values/styles/merges and original source bytes. Extend the `GLANCE_OFFICE_TEST_DIR` regression to assert non-empty styles and merge ranges when the sample is present.

- [ ] **Step 2: Run adapter tests and verify the expected failures**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/format/xlsx -run '^Test(Workbook|EmployeeWorkbook)' -count=1`

Expected: FAIL because the neutral grid API and adapter methods are not implemented.

- [ ] **Step 3: Add neutral DTOs and `GridReader`**

Add the types and interface exactly as listed. Keep neutral style data independent of Gio and Excelize packages. Do not add `GridReader` to `document.Spreadsheet`; XLSX opts in by type assertion and legacy XLS keeps its existing implementation.

- [ ] **Step 4: Implement bounded `Workbook.Window`**

Read only requested cell coordinates through Excelize, translate each cell's common style with `GetCellStyle`/`GetStyle`, collect intersecting ranges using `GetMergeCells`, and fetch requested row/column dimensions. Do not derive navigable extent from used range.

- [ ] **Step 5: Implement row and column dimension mutation**

Implement `SetRowHeight` and `SetColumnWidth`, validating one-based XLSX bounds and finite positive values (zero remains reserved for hiding existing dimensions and is not produced by an interactive resize). Mark dirty only when the stored value changes.

- [ ] **Step 6: Run adapter tests**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/format/xlsx -run '^Test(Workbook|EmployeeWorkbook)' -count=1`

Expected: PASS, including reopening the resized workbook and preserving the synthetic merge/style.

- [ ] **Step 7: Commit Task 2**

Run: `git add internal/document/model.go internal/format/xlsx/workbook.go internal/format/xlsx/workbook_test.go docs/superpowers/plans/2026-09-28-virtualized-spreadsheet-grid.md && git commit -m "feat: expose styled spreadsheet windows"`

Expected: commit created on `codex/virtualized-spreadsheet-grid`; no files remain staged.

### Task 3: Virtual grid viewport, scrolling, styles, and merges

**Files:**
- Create: `internal/desktopui/grid_view.go`
- Create: `internal/desktopui/grid_paint.go`
- Create: `internal/desktopui/grid_hit_test.go`
- Create: `internal/desktopui/grid_hit_test_test.go`
- Create: `internal/desktopui/grid_view_test.go`
- Create: `internal/desktopui/grid_paint_test.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/layout.go`

**Interfaces:**
- Consumes Task 1 `AxisGeometry` and Task 2 `document.GridReader`/`SheetWindow`.
- Produces `gridViewport` state with horizontal/vertical offsets, current bounded `SheetWindow`, selected one-based cell coordinate, and stable event tags.
- Produces pure geometry helpers `cellAtPoint(...)`, `mergeAnchorAt(...)`, and `cellRect(...)` for testable selection/render bounds.
- Produces `visibleCellWindow(rowAxis, columnAxis *AxisGeometry, scrollX, scrollY, viewportWidth, viewportHeight float64) document.CellRange`, returning a one-based inclusive range.
- Route `.xlsx` to the virtual grid. Keep `.xls` on the existing read-only grid and keep Word rendering untouched.
- Register a stable pointer target for the grid surface. Wheel/trackpad scroll updates both axes; shift-wheel scrolls horizontally. Clamp offsets to the legal worksheet extent.
- Fetch visible rows/columns plus two rows/columns of overscan. Reuse a bounded cache and request a new window only when the visible range changes.
- Draw fixed row/column headings and visible cells directly with Gio operations. Draw common styles from neutral `CellStyle`. Draw each merged range as its anchor cell spanning its visible rectangle; do not draw interior cells; clip at viewport bounds.

- [ ] **Step 1: Write failing hit-test and merge geometry tests**

Add `TestCellAtPointUsesVariableDimensions`, `TestMergeAnchorAtInteriorAndEdge`, `TestMergedCellRectSpansRowsAndColumns`, `TestVisibleWindowAddsOverscanAndClamps`, `TestGridViewCachesAndInvalidatesWindow`, `TestGridViewRetainsPreviousWindowOnReadError`, and `TestGridPaintStyleUsesCellStyle`. Cover scrolled offsets, resized dimensions, hidden dimensions, merges crossing viewport edges, the final worksheet row/column, window refresh after edit/resize/sheet switch, read failures, and neutral style-to-Gio color/alignment mapping.

- [ ] **Step 2: Run tests and verify the expected failures**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^(TestCellAtPoint|TestMergeAnchorAt|TestMergedCellRect|TestVisibleWindow|TestGridView|TestGridPaint)' -count=1`

Expected: FAIL because the virtual-grid helpers do not exist.

- [ ] **Step 3: Implement pure cell/merge geometry helpers**

Translate viewport points through scroll offset and fixed header bands into one-based cell coordinates using `AxisGeometry`. Resolve any point within an inclusive merged range to its top-left cell.

- [ ] **Step 4: Implement viewport state and bounded window loading**

Initialize axes to 1,048,576 rows and 16,384 columns, using file-provided sizes for cells in loaded windows and defaults elsewhere. Retain only the visible window plus overscan. Preserve the last valid data window and show status if a new adapter request fails.

- [ ] **Step 5: Implement two-axis scrolling and virtualized drawing**

Draw row/column headers, default grid lines, visible values and style subset, merge spans, and clipped merged edges. Use scroll pointer events on stable viewport tags rather than per-cell Clickables. Keep worksheet tabs below the viewport. Re-fetch only when the visible window crosses its cache boundary.

- [ ] **Step 6: Run viewport tests**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^(TestCellAtPoint|TestMergeAnchorAt|TestMergedCellRect|TestVisibleWindow|TestGridView|TestGridPaint)' -count=1`

Expected: PASS for geometry, selection mapping, merge spans, overscan, and end-of-sheet clamping.

- [ ] **Step 7: Commit Task 3**

Run: `git add internal/desktopui/app.go internal/desktopui/layout.go internal/desktopui/grid_*.go docs/superpowers/plans/2026-09-28-virtualized-spreadsheet-grid.md && git commit -m "feat: render virtual spreadsheet grid"`

Expected: commit created on `codex/virtualized-spreadsheet-grid`; no files remain staged.

### Task 4: Excel-style controls and in-place cell editing

**Files:**
- Create: `internal/desktopui/grid_edit.go`
- Create: `internal/desktopui/grid_edit_test.go`
- Modify: `internal/desktopui/grid_view.go`
- Modify: `internal/desktopui/layout.go`
- Modify: `internal/desktopui/app.go`

**Interfaces:**
- Consumes Task 3 selection and visible cell model.
- Produces persistent editing state with selected address, original value, draft value, and edit mode.
- Expose pure state transitions `select(address, value string)`, `begin()`, `setDraft(value string)`, `commit(apply func(address, value string) error) error`, and `cancel()` so commit/cancel/error behavior is testable without Gio frame state.
- Add name box and formula/value bar above the grid; name box shows selected address and the formula/value bar follows selection.
- One click selects; double-click or F2 starts in-cell editing at the selected cell rectangle. The editor overlays the selected cell and does not affect row/column geometry.
- Enter commits once through `SetCellText`; Escape restores the original value. Formula-bar Enter applies the same validation path. Formula-like values remain rejected.
- Keep the Word paragraph editor panel unchanged; remove the bottom XLSX editor panel.

- [ ] **Step 1: Write failing editor state tests**

Add `TestCellEditBeginCommitCancel`, `TestCellEditNoOpDoesNotDirty`, `TestCellEditRejectsFormulaAndRetainsDraftOnError`, and `TestCellSelectionSyncsNameAndValue`. Assert committed value is unchanged before commit and after cancel; Enter-equivalent commit applies once; invalid formula preserves the document and draft; selection updates both the displayed address and value.

- [ ] **Step 2: Run tests and verify the expected failures**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^TestCellEdit|^TestCellSelection' -count=1`

Expected: FAIL because the grid editing state is not implemented.

- [ ] **Step 3: Implement persistent edit state transitions**

Implement begin, commit, and cancel functions independently of Gio layout. On successful commit, update the visible window/cache and clear edit mode; on validation failure, preserve draft and edit mode.

- [ ] **Step 4: Add name box, formula/value bar, and cell overlay editor**

Place the controls above the grid, route F2/double-click to edit mode, and anchor the editor overlay to the selected/merged cell rectangle. Keep grid geometry fixed while text changes. Integrate Gio key and editor events without committing intermediate IME composition.

- [ ] **Step 5: Run editor tests**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^TestCellEdit|^TestCellSelection' -count=1`

Expected: PASS for synchronization, commit, cancel, no-op, and formula rejection.

- [ ] **Step 6: Commit Task 4**

Run: `git add internal/desktopui/app.go internal/desktopui/layout.go internal/desktopui/grid_*.go && git commit -m "feat: edit selected spreadsheet cells in place"`

Expected: commit created on `codex/virtualized-spreadsheet-grid`; no files remain staged.

### Task 5: Row-height and column-width drag resizing

**Files:**
- Create: `internal/desktopui/grid_resize.go`
- Create: `internal/desktopui/grid_resize_test.go`
- Modify: `internal/desktopui/grid_view.go`
- Modify: `internal/desktopui/grid_paint.go`
- Modify: `internal/desktopui/grid_geometry.go`

**Interfaces:**
- Consumes Task 1 geometry and Task 2 dimension setters.
- Produces resize state with axis, one-based row/column index, original file size, start pointer anchor, preview position, and coordinate-space mode.
- Provide tested conversions between grid logical dimensions and Excel row points/column character widths. Persist Excel units, never raw pixels.
- A drag handle exists on row/column heading boundaries. During drag, paint a preview line and keep the corresponding resize cursor active over the preview region.
- Rebase the pointer anchor when coordinate-space resolution changes. Expand temporary preview bounds past the old grid edge. Commit the dimension only on release; cancel/lost gesture restores the original size.

- [ ] **Step 1: Write failing resize geometry/state tests**

Add `TestResizePreviewTracksPointerAcrossCoordinateRebase`, `TestResizePreviewExtendsPastGridEdge`, `TestResizeCancelKeepsOriginalSize`, `TestResizeCommitOccursOnlyOnRelease`, and `TestExcelDimensionConversions`. Cover dragging both larger and smaller, scroll offset changes, row and column resize, minimum size, invalid floats, and release exactly once.

- [ ] **Step 2: Run tests and verify the expected failures**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^TestResize|^TestExcelDimension' -count=1`

Expected: FAIL because resize preview and conversion helpers do not exist.

- [ ] **Step 3: Implement dimension conversion and resize preview state**

Add the named conversion helpers and preview state machine. Store preview geometry separately from committed `AxisGeometry` and workbook dimensions. Enforce documented minimum interactive dimensions.

- [ ] **Step 4: Implement stable pointer drag handling**

Register handles on fixed headings, normalize pointer coordinates into grid-local space, and rebase anchors if Gio changes coordinate space. Keep cursor and line visible across the preview region and beyond the previous final edge.

- [ ] **Step 5: Commit through adapter on release**

On release, convert the preview to Excel units, call the appropriate `GridReader` setter once, update geometry and window cache, and mark dirty. On cancellation or adapter error, restore the prior geometry; retain dirty state correctly.

- [ ] **Step 6: Run resize tests**

Run: `GOCACHE=/tmp/glance-go-gocache go test ./internal/desktopui -run '^TestResize|^TestExcelDimension' -count=1`

Expected: PASS, with setter invocation only on successful release.

- [ ] **Step 7: Commit Task 5**

Run: `git add internal/desktopui/grid_*.go internal/format/xlsx/workbook.go internal/format/xlsx/workbook_test.go && git commit -m "feat: resize spreadsheet rows and columns"`

Expected: commit created on `codex/virtualized-spreadsheet-grid`; no files remain staged.

### Task 6: Sample regression, compatibility docs, and end-to-end verification

**Files:**
- Modify: `docs/office-support.md`
- Modify: `docs/architecture.md`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md` only if implementation discovers a spec defect; record the reason.

**Interfaces:**
- Consumes all earlier task interfaces.
- Does not expand `.xls` or DOCX scope.
- Uses local samples only through `GLANCE_OFFICE_TEST_DIR`; never modifies sample originals.

- [ ] **Step 1: Verify the supplied workbook copy round trip**

Run the Task 2 temporary-copy test against `人事表格-员工绩效表.xlsx`, comparing edited dimensions and cell with untouched representative values, styles, and merges. Use the Task 2 synthetic fixture for formula/cached-value preservation if the supplied sample has no formulas. Confirm source bytes remain unchanged.

- [ ] **Step 2: Run all sample workbooks and verify round trips**

Run: `GOCACHE=/tmp/glance-go-gocache GLANCE_OFFICE_TEST_DIR="$HOME/Downloads/office" go test ./internal/format/xlsx -run 'TestOfficeXlsxSamples|TestEmployeeWorkbook' -count=1`

Expected: PASS with sample originals unchanged.

- [ ] **Step 3: Update support and architecture docs**

Document the implemented style subset, merge behavior, XLSX legal grid bounds, virtualized scrolling, edit gestures, and row/column resizing. State explicitly that XLS and DOCX retain their earlier boundaries and that full Excel rendering/formula calculation remain unsupported.

- [ ] **Step 4: Run complete repository verification**

Run:

```sh
GOCACHE=/tmp/glance-go-gocache GLANCE_OFFICE_TEST_DIR="$HOME/Downloads/office" go test ./...
GOCACHE=/tmp/glance-go-gocache go vet ./...
GOCACHE=/tmp/glance-go-gocache go build -o /tmp/glance-desktop ./cmd/desktop
GOCACHE=/tmp/glance-go-gocache GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/glance-desktop.exe ./cmd/desktop
```

Expected: all commands exit successfully.

- [ ] **Step 5: Perform interactive desktop checks**

Open the employee-performance workbook and verify the original Excel-like arrangement, visible styles and merges, two-axis scrolling into blank cells, name/formula bar synchronization, inline edit commit/cancel, resize cursor/preview continuity, resizing at the final visible row/column, and save/reopen. Record any manual checks unavailable in the current desktop automation environment instead of claiming them as verified.

- [ ] **Step 6: Review compatibility status**

Confirm that docs match actual test evidence and keep unsupported features explicit. No feature is marked verified solely from a successful build.

- [ ] **Step 7: Commit Task 6**

Run: `git add AGENTS.md docs/architecture.md docs/office-support.md docs/superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md && git commit -m "docs: document virtual spreadsheet grid support"`

Expected: commit created on `codex/virtualized-spreadsheet-grid`; no files remain staged.
