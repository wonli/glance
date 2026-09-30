# XLSX Picture Objects Design

## Goal

Render ordinary embedded pictures as objects over the virtual worksheet grid and let the user select, insert, replace, move, resize, and delete them. Reuse the current spreadsheet Ribbon and the shared Ribbon chrome added for Word. Preserve unrelated workbook content and verify every picture mutation through save/reopen.

## User-approved scope

- The virtual cell grid remains the document canvas. Pictures are a separate object layer positioned by worksheet cell anchors and offsets.
- Selecting a picture shows its bounds and resize affordance, and exposes picture-specific Ribbon commands. Insertion starts at the selected cell.
- Picture width and height retain their display aspect ratio on resize. Dragging shows preview geometry; only release commits a move or size change.
- Images and cells remain separate document objects. Moving or resizing a picture does not change cell values or dimensions.
- XLSX picture support is the second phase after the DOCX flow and Ribbon work.

## Supported first slice

- Preview common DrawingML pictures anchored over cells on the active worksheet. Decode PNG, JPEG, and GIF (first frame) asynchronously; reject corrupt or oversized images and show a visible placeholder for unsupported image media or unsupported picture anchors.
- Keep spreadsheet-library and OOXML values behind `internal/format/xlsx`. Add neutral picture-object DTOs and optional reader/editor capabilities under `internal/document`.
- Position pictures from their top-left cell anchor and pixel offset, with rendered size derived from image dimensions and drawing scale. Preserve the original image bytes until replacement.
- Support a safe picture edit when the object is the only picture with that anchor. Excelize's public `DeletePicture(sheet, cell)` removes every picture at a cell, so pictures sharing an anchor are visible but read-only until the adapter can target one drawing safely.
- Insert at the current selected cell, or the first nearby unoccupied anchor if that cell already contains a picture. This prevents the new object from making an existing picture ambiguous to the current per-cell deletion API.
- Use picture-specific UI state per workbook tab, including selected object ID, stable interaction state, pending image reads, and a bounded decoded-image LRU cache. Decode and package operations stay off Gio layout/frame code.
- Preserve other picture objects and representative worksheet content through adapter transaction rollback, undo/redo, save, and reopen. Never enable a mutation that fails those checks.

## Format adapter contract

- Project pictures through neutral values containing object ID, sheet, one-based anchor row/column, cell-local pixel offsets, source media type/bytes access, current rendered width/height, editability, and a read-only reason when needed.
- Keep object IDs valid until a successful picture mutation/reprojection. A stable UI selection must be cleared or rebound when the selected object is replaced, moved, or deleted.
- Use the pinned Excelize v2.11.0 picture API behind the adapter for first-slice add/read/delete operations. `GetPictures` exposes drawing scale with two decimal places and does not expose full anchor geometry; this first slice promises the common top-left-cell overlay and preserves the aspect ratio used for user resize, not exact Excel picture-layout fidelity.
- Edit only ordinary over-cell DrawingML pictures with a single picture at their start cell. Read-only boundaries include multiple pictures at one anchor, in-cell/`DISPIMG` objects, unsupported raster formats, charts/shapes, and picture metadata the public API cannot preserve safely.
- Validate media type, decode configuration, file size, pixel count, dimensions, and target sheet before mutation. Each operation is atomic and records a workbook snapshot in the existing bounded undo history.
- A failed mutation restores the previous in-memory workbook, history, dirty state, selection, and picture projection. Save continues using the existing sibling-temporary-file replacement gate.

## Gio overlay and interactions

- Draw visible images above the cell grid and below selection/editor overlays, clipped to the rounded grid viewport. Map anchors through `AxisGeometry` plus the current virtual-grid scroll offsets and device scale.
- Hit-test picture resize handles before picture bodies, then pictures before cells. Clicking an image selects it without changing the selected cell; dragging a selected image previews its new anchor/offset in grid-content coordinates.
- Keep move/resize preview separate from committed adapter state. Normalize pointer coordinates at the grid event boundary, rebase on coordinate-space or scroll changes, keep resize cursors active through the preview, and commit only on pointer release. Cancel restores the original geometry.
- Resize keeps the selected picture's current display ratio. A single adapter helper converts display pixels to Excelize scale values; persist no screen coordinates as file-format geometry.
- The Insert Ribbon gets “插入图片”; selecting an editable picture exposes a contextual Picture group with replace, delete, and ratio-locked resize. Reuse the existing shared Ribbon chrome and command enablement style.
- File selection, image loading/decoding, adapter mutation, and save run asynchronously through the established per-session result channels. Reject stale results after tab, workbook, or worksheet changes.

## Safety and known gaps

- Do not claim exact Excel object layout, crop/rotation/effects parity, in-cell IMAGE/DISPIMG support, SVG/EMF/TIFF rendering, picture printing, image hyperlinks, charts, or non-picture drawing editing.
- Unsupported pictures remain untouched in the XLSX package and are not silently converted to editable cell values.
- Save/reopen fixtures must verify changed image bytes/anchor/size, unchanged cells/styles/merges/formulas/dimensions, and representative untouched picture parts. The source file must remain unchanged in local sample regressions.
- Legacy `.xls` remains read-only.

## Verification

- Neutral/adapter tests cover image projection, PNG/JPEG/GIF decode metadata, anchor and offset mapping, multiple pictures at one anchor, unsupported picture kinds, safe insert/replace/move/resize/delete, invalid inputs, rollback, history, and save/reopen.
- Gio tests cover virtual-grid transform math, viewport clipping, selection/hit-test priority, move/resize previews, aspect ratio, release-only commits, cancel/no-op, tab/sheet stale results, toolbar enablement, and a headless GPU pixel check for the overlay.
- Verify with synthetic workbooks containing multiple sheets, varied row/column dimensions, overlapping picture anchors, cell content underneath pictures, formulas/merges/styles, and unrelated drawing objects.
- Run `go test ./internal/document ./internal/format/xlsx`, targeted desktop UI image tests, `go test ./...`, and the local `.xlsx` sample regression where it opens. Existing baseline failures must be recorded separately.

## Out of scope

- Changes to the virtual-grid geometry contract, cell editing, formula calculation, or worksheet structure.
- Full DrawingML shape/chart editing, exact Excel pagination/print view, floating text wrapping, picture crop/rotation/effects controls, and legacy `.xls` picture mutation.
