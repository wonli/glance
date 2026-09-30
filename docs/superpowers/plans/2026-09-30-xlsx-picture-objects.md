# XLSX Picture Objects Implementation Plan

## Goal

Implement the approved first-slice XLSX picture overlay and editing flow described in [the design spec](../specs/2026-09-30-xlsx-picture-objects-design.md). Pictures remain independent objects over the virtual grid. Preserve workbook safety and reuse the Word Ribbon chrome completed in the previous phase.

## Architecture

```text
Gio grid, Ribbon, image overlay
        ↓ picture intents and neutral picture data
documentSessionState / workbook controller
        ↓ optional spreadsheet-picture interfaces
internal/format/xlsx adapter (Excelize only here)
        ↓ XLSX package and local file
```

- Use only neutral DTOs and intents in Gio/session state.
- Keep persistent interaction/cache/selection state in `documentSessionState` and `gridViewport`; split image painting, hit testing, operations, and picker handling into focused files.
- Keep chooser, reads, decode, mutations, and saves off the layout/frame loop. Apply results only to the matching workbook generation, tab, and sheet.
- Add picture operations to the existing workbook history transaction; preserve the current safe save/reopen path.

## Current constraints

- Excelize v2.11.0 has `GetPictureCells`, `GetPictures`, `AddPictureFromBytes`, and `DeletePicture`. Picture scale returned by `GetPictures` is rounded to two decimal places; `DeletePicture` deletes every ordinary picture anchored at a given cell.
- First-slice editing is therefore limited to one ordinary over-cell picture per anchor. Existing pictures that share an anchor stay visible and read-only; insertion searches for a nearby free anchor.
- The current virtual grid stores geometry in scaled content pixels and tracks scroll in `gridViewport.X/Y`. Picture layout converts anchor + offsets into that same coordinate space and clips to the grid viewport.

## Implementation tasks

### Task 1: Neutral picture model and read projection

**Files**

- Extend `internal/document/model.go` with spreadsheet picture metadata and optional reader capability.
- Add `internal/format/xlsx/pictures.go` and focused adapter tests.
- Extend `internal/format/xlsx/workbook_test.go` fixture helpers where useful.

**Behavior**

- Project per-sheet over-cell pictures in drawing order with unique session object IDs, anchor cell, offsets, rendered size, media type, name/alt text, editability, and read-only reason.
- Read bytes by object ID through a narrow adapter method; do not expose Excelize `Picture` or `GraphicOptions`.
- Validate all sheet/address/scale metadata. Mark same-anchor groups and unsupported in-cell picture types read-only.
- Preserve stable object identity until successful mutation; reproject after undo/redo/open.

**TDD and verification**

1. Add synthetic XLSX cases for one picture, two pictures at one anchor, pictures on separate sheets, unsupported/in-cell images, invalid or missing media, and configured offsets/scales.
2. Run the focused adapter tests and observe the projection tests fail before implementation.
3. Implement the neutral data boundary and reader. Verify aspect ratio metadata, source bytes, order, and read-only guards.
4. Run `go test ./internal/document ./internal/format/xlsx`.

**Acceptance**

- The neutral reader returns the supported picture set without exposing Excelize types.
- A picture group with ambiguous per-cell deletion is explicitly read-only.

### Task 2: Transactional picture mutations and round trips

**Files**

- Extend `internal/document/model.go` with optional spreadsheet picture edit operations.
- Implement insert, replace, move, resize, and delete in `internal/format/xlsx/pictures.go`.
- Add focused save/reopen/history and preservation tests.

**Behavior**

- Accept PNG/JPEG insertion/replacement after file-byte, decode, and pixel-limit validation. Preserve selected metadata when replacing an existing safe picture.
- Resolve an object to its one-based sheet/cell anchor and recheck the single-picture guard under the workbook lock.
- Move changes its anchor/offset; resize validates finite positive dimensions and retains display ratio. Geometry is converted through one tested pixels-to-Excelize-scale helper.
- Stage operations from a workbook snapshot. On any error reopen the snapshot, keep dirty/history state unchanged, and return a contextual error. On success record one bounded undo step and publish the new projection.
- Save/reopen tests verify each operation, undo/redo, untouched workbook values, styles, merges, formulas, dimensions, and an unrelated picture.

**TDD and verification**

1. Add failing operation tests for PNG/JPEG, duplicate-anchor rejection, invalid data/dimensions, undo/redo, and package preservation.
2. Implement safe mutation/rollback and record history only after a verified state change.
3. Save and reopen the fixture after insert/replace/move/resize/delete and assert bytes, anchor, dimensions, dirty state, untouched cells/formulas/styles/drawings, and undo/redo.
4. Run `go test ./internal/document ./internal/format/xlsx`.

**Acceptance**

- Every exposed operation survives save/reopen and can be undone/redone.
- Failed or ambiguous edits never delete another picture or mark the workbook dirty.

### Task 3: Virtual-grid image overlay and selection/drag state

**Files**

- Add focused `internal/desktopui/grid_picture.go`, `grid_picture_interaction.go`, and tests.
- Extend `internal/desktopui/session_state.go` and `grid_view.go` with stable picture metadata, image preview cache, selection, and gesture state.
- Update `internal/desktopui/grid_layout.go` / `grid_paint.go` only at the grid event and overlay composition boundaries.

**Behavior**

- Load active-sheet image metadata asynchronously on open/sheet switch. Request bytes and decode only for visible image objects, with PNG/JPEG/GIF support, a 32-megapixel ceiling, and a 64 MiB per-workbook LRU cache.
- Position and clip images through `AxisGeometry`, viewport scroll, header offsets, and scale. Draw overlays above cells and below selection/editor UI.
- Hit-test resize handles, then picture bodies, before cell selection. Clicking a picture selects it without changing the selected cell. Move/resize preview uses grid-content coordinates and commits only on release; cancel and zero-delta no-op leave workbook and geometry unchanged.
- Preserve aspect ratio during resize. Keep picture interaction IDs stable across frames; clear or restore selection on mutation/undo/redo/sheet switch.
- Show placeholders for corrupt, oversized, or unsupported media.

**TDD and verification**

1. Add geometry and interaction tests for anchor conversion, scroll/scale, clipping, overlap priority, selected image bounds, resize handle position, move/resize preview, release, cancel, and no-op.
2. Add a headless Gio GPU fixture with two colored image regions and assert both appear inside the viewport and outside clipped areas do not.
3. Run tests against the baseline virtual grid and verify existing cell selection, resizing, editing, and device-scale behavior are unchanged.

**Acceptance**

- Pictures appear at the correct visible grid location, scale with viewport geometry, and remain selectable while scrolled.
- Preview never mutates workbook state before release.

### Task 4: Ribbon, async picker, workbook sample evidence, and compatibility docs

**Files**

- Extend `internal/desktopui/layout_spreadsheet_toolbar.go` and session toolbar state with Insert Image and a selection-dependent Picture group, reusing `ribbon_chrome.go`.
- Add platform image picker wiring and async result handling in `filepicker_*.go`, `workspace.go`, and app result processing.
- Add focused UI/picker tests; update `docs/office-support.md`, `docs/office-library-evaluation.md`, and the Glance Go section in `../tools/docs/compatibility/office-feature-matrix.md`.

**Behavior**

- Insert opens a native PNG/JPEG picker in a background goroutine and adds the new image at the selected cell or nearby unoccupied anchor.
- Selected pictures expose replace, delete, and size commands; mutation results refresh image projection/cache and use existing save/undo status.
- Ignore cancellation without dirty state. Reject corrupt, mismatched-extension, unsupported, oversized, stale workbook/tab/sheet results.
- Update support docs only after round-trip and UI evidence pass. Keep all external workbook samples unchanged.

**TDD and verification**

1. Add chooser tests for cancel, invalid input, stale generation/sheet, and async non-blocking behavior.
2. Add toolbar capability tests and a fixture round-trip covering insert, move, resize, replace, delete, undo, redo, and reopen.
3. Run `go test ./internal/document ./internal/format/xlsx`, targeted `internal/desktopui` picture/grid/ribbon tests, `go test ./...`, and available local XLSX corpus checks.
4. Perform an interactive Gio check when the local desktop backend is available; verify ribbon, overlay clipping, selection handles, drag release, Save, and reopen.

**Acceptance**

- XLSX images can be inserted and selected/moved/resized/replaced/deleted on the UI and survive save/reopen.
- Image mutations preserve aspect ratio and unrelated workbook content. The source sample is unchanged.
- Compatibility docs distinguish supported over-cell pictures from read-only/unsupported drawing kinds.

## Review focus

1. Duplicate picture anchors: neither selection nor mutation may remove the wrong drawing.
2. Drawing position and size: scroll, scale, row/column dimensions, offsets, and ratio-preserving resize must share one documented coordinate model.
3. Transaction and history: failures cannot alter dirty state; undo after save must restore the exact picture set and remain consistent with Excelize snapshots.
4. Input and resource bounds: media validation happens before package mutation; decode remains bounded and asynchronous.
5. Package preservation: changed pictures must survive reopen while formulas, merges, styles, dimensions, and untouched pictures remain unchanged.
