# Virtualized Spreadsheet Grid Design

## Goal

Replace the current bounded Flex/List XLSX preview with an Excel-like, two-dimensional virtualized grid. The grid must extend through the full XLSX address space even when the worksheet's used range is small, show common cell formatting and merged regions, support horizontal and vertical scrolling, allow row-height and column-width resizing, and support editing in the selected cell.

The interaction layout keeps the familiar spreadsheet structure:

- Existing file open/save controls remain in the top toolbar.
- A name box and formula/value bar sit above the grid.
- Column headings and row headings frame the grid.
- Worksheet tabs sit below the grid.
- A cell is selected with one click. Double-click or F2 enters in-place edit mode. Enter commits and Escape cancels. The formula/value bar edits the selected cell as well.

The Word preview and editing flow is out of scope and must remain unchanged.

## Product requirements

### Virtual worksheet extent

- XLSX worksheets expose the full legal address range: 1,048,576 rows by 16,384 columns.
- Blank cells outside the used range can be scrolled to, selected, and edited.
- The implementation must not allocate one Gio widget or one document cell for every possible address.
- Only cells intersecting the viewport plus a small overscan margin are fetched and rendered.
- The workbook's used range remains separate from the navigable worksheet extent.

### Selection and editing

- Keep the name box synchronized with the active cell address.
- Keep the formula/value bar synchronized with the selected cell's displayed value.
- Double-click or F2 opens a persistent editor over the selected cell without changing grid geometry.
- Enter commits once; Escape discards the draft. Clicking another cell must not silently commit an unvalidated intermediate edit.
- Editing a merged range targets its top-left anchor cell.
- Formula entry remains unsupported and must continue to be rejected. This work does not add formula calculation.
- Track committed document value separately from transient editor text. Do not commit intermediate IME composition text.

### Dimensions and resizing

- Read and show worksheet column widths and row heights through their configured file dimensions, converted to Gio density-aware logical dimensions for drawing.
- Provide drag handles at row and column heading boundaries.
- Enforce a usable minimum dimension and preserve hidden rows/columns as hidden when their source dimension is zero.
- During a drag, update only transient preview geometry. Commit a new file-format width/height only on pointer release.
- Keep the preview attached to the pointer through scrolling and pointer-coordinate-space changes. Expand the preview drawing bounds when resizing the final visible dimension wider/taller.
- Persist dimensions in Excel units, never pixels.

### Cell appearance and merged cells

- Render a clearly defined common style subset: fill color, font color, bold, italic, font size, horizontal and vertical alignment, wrapping, and borders.
- Continue displaying values formatted by Excelize where supported. Number-format fidelity beyond those formatted strings is out of scope.
- Read merged-cell ranges. Draw the anchor cell across the complete merged rectangle, omit interior grid cells, and map hits anywhere inside the merge to the anchor.
- Merges crossing the viewport edge must clip correctly while preserving the anchor text and visible boundaries.
- Unsupported Excel rendering features (conditional formatting, rich text runs, drawings, charts, pivots, print layout, and formula calculation) must not be implied as supported.

## Architecture

### Viewport and geometry

Use one custom Gio grid viewport with an explicit content coordinate system, rather than composing one widget per cell with nested `layout.Flex` or `layout.List` instances. The viewport owns scroll offsets, viewport size, selection, and hit testing. Row and column headings are fixed viewport bands; the cell canvas scrolls in both axes; sheet tabs remain outside the scrolling canvas.

The geometry model must support default dimensions and sparse per-row/per-column overrides without a per-cell or per-dimension widget allocation. It must provide:

- logical coordinate for a row/column boundary;
- dimension index for a content coordinate;
- visible row/column interval plus overscan;
- dimension changes and scroll clamping at XLSX bounds.

Use a tested prefix-sum/index structure for dimension overrides so coordinate lookup and resizing remain efficient at maximum row counts. The exact data structure may be chosen during implementation; it must not require iterating all preceding rows for each frame.

### Format-neutral model and XLSX adapter

Keep Excelize types inside `internal/format/xlsx`. Expand the neutral document DTOs/API to supply a bounded visible window containing values, common style attributes, and merged ranges, and to get/set row and column dimensions. The adapter must support reading blank visible cells beyond the used range and editing legal blank addresses.

The UI must request a visible window rather than a whole preview rectangle. Cache only a bounded window and invalidate affected entries after cell edits, dimension changes, or sheet switches. Preserve existing atomic-save behavior.

Before enabling any new mutation path, tests must verify saved output by reopening it. Resizing writes only the requested row/column dimension; a cell edit writes only its cell; representative untouched values, styles, merges, and formulas/cached values in the supplied fixture must survive.

### Pointer and keyboard interaction

Use one documented grid-local coordinate space for scrolling, selection, resize handles, preview lines, and the in-cell editor. Keep pointer event registration on stable viewport layers rather than recycled cell widgets. Normalize pointer positions at the event boundary. If Gio changes coordinate space during a gesture, choose a continuous resolved position and rebase the drag anchor so the preview does not jump.

Selection, edit draft, resize preview, and committed workbook state are distinct. Pointer release is the only resize commit point. Enter/Escape behavior and IME composition are handled by the editor state machine, not by mutating workbook values from layout code.

## File format boundary

- XLSX: virtual extent, visible-window reads, style/merge preview, cell edit, dimension resize, and save/reopen validation.
- Legacy XLS: remains read-only and limited to the existing adapter boundary. It does not gain virtual full-sheet editing or dimension resize in this work.
- DOCX: unchanged.

The conceptual grid is unbounded within the legal XLSX address space; it is not mathematically infinite and does not allow addresses beyond XLSX limits.

## Error handling and safeguards

- Reject addresses outside the XLSX row/column limits in both UI and adapter.
- Reject invalid or non-finite dimension edits; clamp interactive drag previews to documented minimums and file-format limits.
- On adapter read errors, keep the previous stable viewport data and show an error status rather than rendering a misleading blank grid.
- On save failure, preserve dirty state and the original file.
- Do not discard an in-cell draft when focus changes. Require Enter/Escape or an explicit handled transition.

## Verification

### Unit and adapter tests

- Geometry coordinate/index round trips with default dimensions, sparse overrides, minimums, hidden dimensions, and maximum XLSX indexes.
- Visible-window bounds, overscan, cache invalidation, and blank cells beyond the used range.
- Merge visibility/intersection, anchor lookup, hit testing, and clipped rendering geometry.
- Style translation for each supported style property.
- Resize preview math while dragging narrower/wider and shorter/taller, including scroll offset changes, coordinate rebasing, edge extension, cancel, and release commit.
- Editor state transitions for click selection, F2/double-click, Enter, Escape, no-op commit, formula rejection, and IME composition.
- XLSX save/reopen tests for edited cells and dimensions, while comparing representative untouched content.

### Local sample verification

Use `~/Downloads/office` without changing source files. In particular, verify the employee-performance workbook's merged title/header regions, formatting, visible values, row/column dimensions, scrolling into blank areas, and at least one dimension resize round trip. Add synthetic fixtures for features not represented by that sample.

### Desktop verification

Build the Gio application and perform interactive checks on a supported desktop platform. Verify two-axis scrolling, sticky headings, inline edit commit/cancel, drag preview/cursor continuity, merge selection, and save/reopen after resize/edit. Compilation alone is not visual verification.

## Non-goals

- Full Microsoft Excel rendering compatibility.
- Formula authoring or calculation.
- Conditional formatting, chart/image rendering, drawing objects, pivot UI, freeze panes, printing, or workbook structure editing.
- Word UI changes.
- XLS editing or XLS dimension resizing.
