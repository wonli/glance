# DOCX Content Rendering and Editing Implementation Plan

## Goal

Implement the approved DOCX design in Glance: render body paragraphs, tables, text runs, and inline images in document order; allow edits to supported text and image objects; reuse the Excel ribbon's visual style for Word commands; and preserve unsupported or untouched DOCX package content. Use `/Users/mima1234/Downloads/office/大猫OA软件使用说明.docx` as an external regression sample without adding it to the repository.

This plan covers the DOCX phase only. XLSX image objects remain the next phase and will reuse the ribbon chrome after DOCX has been completed and verified.

## Architecture

```text
Word ribbon and ordered Gio flow
        ↓ user intents and neutral Word view data
document session and internal/document DTOs
        ↓ optional reader/editor/history capabilities
internal/format/docx OOXML package adapter
        ↓ local DOCX file, atomically saved
```

- Keep ZIP/XML types and relationships inside `internal/format/docx`.
- Keep persistent Gio widgets and selection state in `documentSessionState`; keep drawing, event handling, and adapter mutations in separate focused files.
- Run file selection, image decoding, DOCX mutations, and saves outside the Gio frame callback. Apply completed results through the existing session/result-channel pattern.
- Preserve existing Excel toolbar appearance and behavior while extracting only shared ribbon presentation helpers.

## Tech Stack

- Go, `archive/zip`, `encoding/xml`, and the existing project-local DOCX adapter.
- Gio v0.10.2 for ribbon controls, document flow, selection, editing, pointer gestures, and image drawing.
- Standard image decoders for PNG/JPEG and GIF first-frame preview. Do not add a DOCX dependency unless fixture evidence demonstrates a concrete gap and `docs/office-library-evaluation.md` records the evaluation first.

## Spec

- Add format-neutral Word blocks, paragraphs, table rows/cells, ordered text/image inline items, text style/alignment values, and narrow optional reader/editor/history interfaces under `internal/document`.
- Project `word/document.xml` and its relationship part into those DTOs, retaining paragraph and table-cell structure, inline order, run formatting, image media type, and original EMU dimensions. Use adapter-owned object IDs; Gio must not inspect OOXML.
- Word Home and Insert tabs use the Excel ribbon's spacing, icon, grouping, and colors. Home includes undo/redo, cut/copy/paste, bold/italic/underline, font color, paragraph alignment, and save. Insert includes image insertion. Selecting an image exposes replace, delete, and aspect-ratio-locked resize controls.
- Text editing targets one selected run, including runs in table cells, so existing run formatting remains attached. Paragraph alignment applies to the containing paragraph.
- Support inline PNG/JPEG insertion and replacement; preview PNG/JPEG and the first GIF frame. Inline pictures in body paragraphs and table cells can be selected, moved in document order, resized, replaced, and deleted. Image bytes are resolved by the adapter.
- Preserve unrelated OOXML parts, unknown XML children/attributes, unsupported drawings, and unsupported media. A missing/bad relationship or decode failure renders a visible read-only placeholder with status.
- Keep this a scrollable flow preview. Do not claim Word pagination, floating-image placement, text wrapping around images, or editing headers/footers, equations, fields, or tracked revisions.
- Undo/redo covers supported text, formatting, alignment, and image operations, with at most 100 actions per open document session.
- Save via a temporary sibling file and replace only after a complete successful write and sync. Preserve dirty state after operation or save failure.

## Global Constraints

- Respect repository `AGENTS.md` and `docs/architecture.md`: Gio → session/controller → neutral model → format adapter → local file.
- Keep `internal/format/docx` as the only layer that knows package part paths, `rId` values, OOXML tags, and EMU units.
- Do not discard unknown package parts or make ambiguous/unsupported constructs editable.
- Keep widgets stable across frames; give every `widget.List` an explicit axis and editors in rows the full available width.
- Normalize pointer coordinates at input boundaries. Persist image dimensions in EMUs, never screen pixels.
- Cap decoded preview images at 32 megapixels and the per-document least-recently-used image cache at 64 MiB.
- Keep the sample document outside the repository. Tests that use it must be opt-in through `GLANCE_OFFICE_TEST_DOCX`, edit a temporary copy, save and reopen it, and verify the original bytes are unchanged.
- Do not claim overwrite safety for an operation until a save/reopen regression verifies that edit and representative untouched text, styles, tables, relationships, media, and package parts survive.

## Implementation Tasks

### Task 1: Neutral Word DTOs and ordered DOCX projection

**Files**

- Add `internal/document/word.go`.
- Add `internal/format/docx/word_projection.go` and update `internal/format/docx/document.go` to publish the projection while retaining compatibility wrappers for the current `Blocks`/`ReplaceParagraph` callers until the UI migration is complete.
- Extend `internal/format/docx/document_test.go` fixture helpers and add projection tests in `internal/format/docx/word_projection_test.go`.

**Interfaces and data**

- Define neutral `WordBlock`, `WordParagraph`, `WordTable`, `WordTableRow`, `WordTableCell`, and ordered `WordInline` DTOs. `WordInline` is a tagged value containing either a `WordTextRun`, a `WordImage`, or a read-only unsupported-item description.
- `WordTextRun` carries an adapter object ID, text, editability, and a `WordTextStyle` with bold, italic, underline, font family, font size, and font color.
- `WordParagraph` carries an object ID, style name, alignment, and ordered inlines. A table cell contains its ordered paragraphs rather than flattened strings.
- `WordImage` carries an object ID, media type, and original `WidthEMU`/`HeightEMU`. `WordContentReader` exposes `WordBlocks() []WordBlock` and `WordImageBytes(imageID string) ([]byte, error)`.
- Keep Word format values independent of Gio and OOXML enums. IDs must be unique in a projection and remain resolvable until the next successful edit/reparse.

**TDD steps**

1. Add ZIP fixtures with two paragraphs, a table with multiple paragraphs in a cell, mixed styled text runs, text-image-text order, relationship targets, and unsupported/floating content. Assert the current parser cannot satisfy ordered projection and stable object lookup yet.
2. Add DTO and reader capability definitions, then parse body children and nested table cells in source order. Parse run style, alignment, inline `wp:inline` extents, and relationship targets without treating `wp:anchor` as editable inline content.
3. Add tests for unique IDs, exact order, text/style extraction, cell paragraph boundaries, image-byte lookup, malformed/missing relationships, and read-only unsupported items.
4. Run `go test ./internal/document ./internal/format/docx` and confirm old paragraph snapshot tests still pass through the compatibility surface.

**Acceptance**

- Projection includes all supported paragraphs/tables and inline image slots in document order.
- The table fixture can identify and resolve text and image objects inside cells.
- Missing or unsupported media is visible in the DTO and remains byte-for-byte in the package.

### Task 2: Safe text, formatting, alignment, and DOCX undo/redo

**Files**

- Add `internal/format/docx/word_text.go` and `internal/format/docx/word_history.go`.
- Extend `internal/document/word.go` with text editor, formatting patch, alignment, and history capabilities.
- Add `internal/format/docx/word_text_test.go` and `internal/format/docx/word_history_test.go`.

**Interfaces and behavior**

- `WordTextEditor` exposes `ReplaceTextRun(runID, value string) error`, `SetTextRunStyle(runID string, patch WordTextFormatPatch) error`, and `SetParagraphAlignment(paragraphID string, value WordAlignment) error`.
- Format patches cover bold, italic, underline, and font color. Alignment maps left/center/right/justified to `w:jc` values.
- Locate target XML ranges in the adapter and patch only the target `w:t`, `w:rPr`, or `w:pPr`; preserve boundary spaces with `xml:space="preserve"` and leave unknown style properties/children intact.
- Record before/after deltas for only the touched package parts. Retain at most 100 actions, clear redo history after a new edit, and track saved versus current history state so undo across a save correctly updates dirty state.
- Failed patch, parse, undo, or redo must restore the previous in-memory parts and projection.

**TDD steps**

1. Add failing fixture tests for editing an individual run in a multi-run paragraph and table cell; toggling styles without removing unrelated run properties; all four alignments; Unicode, XML metacharacters, and boundary spaces; undo/redo including edits on both sides of a save; and the 100-action limit.
2. Implement surgical text/style/alignment patches and transactional reparse.
3. Implement bounded history deltas and dirty-state tracking. Keep the old `ReplaceParagraph` wrapper only where existing callers/tests need it; remove its single-run-only restriction when the UI has moved to run IDs.
4. Add save/reopen assertions proving edited values/styles survive and unrelated XML/package parts remain unchanged.
5. Run `go test ./internal/document ./internal/format/docx`.

**Acceptance**

- A multi-run paragraph can edit one selected run without flattening adjacent text or styles.
- Unsupported runs and unknown properties are retained.
- Undo/redo reverses supported edits and reports the correct dirty state after save, undo, and redo.

### Task 3: Inline image package operations

**Files**

- Add `internal/format/docx/word_images.go`.
- Extend `internal/document/word.go` with `WordImageEditor` operations.
- Add `internal/format/docx/word_images_test.go`; add focused relationship/content-type fixture helpers where needed.

**Interfaces and behavior**

- `WordImageEditor` exposes `InsertInlineImage(paragraphID, afterInlineID, mediaType string, data []byte) (imageID string, err error)`, `ReplaceInlineImage(imageID, mediaType string, data []byte) error`, `MoveInlineImage(imageID, targetParagraphID, afterInlineID string) error`, `ResizeInlineImage(imageID string, widthEMU int64) error`, and `DeleteInlineImage(imageID string) error`.
- Accept insertion/replacement only for validated PNG/JPEG payloads. Insert into body or table-cell paragraphs at the requested inline slot. Create a unique relationship and valid content-type declaration as required.
- Replacement creates/uses a media relationship for the selected object without changing another drawing that points to the same media part. Deletion removes only that drawing and prunes relationship/media only when no remaining package content refers to them.
- Resize updates `wp:extent` width and height from the original ratio. Move changes the inline object order within the supported body/cell paragraph positions while retaining its object identity.
- Each mutation is transactional and participates in Task 2 history. Reject zero/negative/extreme dimensions and empty/invalid image payloads before dirty state changes.

**TDD steps**

1. Add failing synthetic package tests for PNG/JPEG insert, replace, move, resize, delete, images in table cells, shared media targets, existing `rId` collisions, content types, missing relationships, and preservation of untouched package parts.
2. Implement package relationship/content-type and drawing edits, using unique IDs and package-wide reference checks before removing parts.
3. Add save/reopen checks for each operation, verify media bytes and EMU extents, and assert undo/redo restores both the image and its relationships.
4. Run `go test ./internal/format/docx`.

**Acceptance**

- Inserted/replaced image survives save/reopen and is addressable as one object.
- Editing or deleting one drawing never changes a second drawing that shared the original media target.
- Failed image operations leave the original package, dirty state, and history unchanged.

### Task 4: Shared ribbon chrome and Word commands

**Files**

- Extract reusable tab/action/group/icon layout helpers from `internal/desktopui/layout_spreadsheet_toolbar.go` into `internal/desktopui/ribbon_chrome.go` without changing spreadsheet action definitions or behavior.
- Add `internal/desktopui/word_toolbar.go` and `internal/desktopui/word_toolbar_test.go`.
- Extend `internal/desktopui/session_state.go` with Word tab selection, stable tab/action clickables, selected run/paragraph/image IDs, and Word command state.
- Update `internal/desktopui/layout.go` routing and existing toolbar tests only where the extracted helper signatures require it.

**Behavior**

- Word shows Home/Insert tabs and a contextual Picture command group only when an image is selected. It must not show worksheet/cell actions.
- Home commands: undo/redo, cut/copy/paste, bold/italic/underline, font color, left/center/right/justified alignment, and save. Insert provides image insertion. Picture provides replace, delete, and aspect-ratio-locked resize.
- Formatting and alignment commands are disabled when the current selection does not support the operation. Clipboard commands target the selected run and preserve surrounding runs.
- Reuse the current ribbon's color palette, group dividers, accessible labels, and icon sprite; do not copy spreadsheet command dispatch into Word.

**TDD steps**

1. Add tests describing Word tab/action groups, accessible labels and icons, selection-dependent enabled states, and contextual Picture visibility. Ensure those expectations fail before Word ribbon wiring.
2. Extract presentation helpers and keep `TestSpreadsheetToolbarActionsCoverHomeAndDataGroups`, icon tests, and current spreadsheet toolbar rendering behavior intact.
3. Implement Word action definitions and dispatch through narrow Word adapter capabilities.
4. Run `go test ./internal/desktopui` and verify existing spreadsheet ribbon tests plus Word action tests.

**Acceptance**

- Excel appearance/action tests continue to pass.
- Word toolbar contains only Word commands and its selection state matches the active text run, paragraph, or image.

### Task 5: Ordered Word flow, image rendering, and editing interactions

**Files**

- Add `internal/desktopui/word_layout.go`, `internal/desktopui/word_render.go`, and focused `internal/desktopui/word_layout_test.go` / `word_render_test.go`.
- Update `internal/desktopui/layout.go`, `internal/desktopui/session_state.go`, and `internal/desktopui/app.go` to route the new model and command results.
- Replace Word's index-keyed block clickables with per-object persistent widget state keyed by the current adapter IDs.

**Behavior**

- Render a virtualized, scrollable ordered flow of paragraphs and tables. Render text runs and inline images as distinct selection targets; preserve cell/paragraph boundaries and make table-cell text editable.
- Use style data for supported font weight, italic, underline, color, and paragraph alignment. Preserve list/heading presentation where safe; unsupported structures get a visible read-only placeholder.
- Decode PNG/JPEG preview and GIF first frame off the Gio layout loop. Enforce a 32-megapixel decode limit and a 64 MiB per-document LRU cache; invalidate cache entries on image replacement/deletion.
- Fit images to available width while preserving ratio, and show a placeholder plus status on unsupported/corrupt media. Keep image measurements in EMUs and convert only at the drawing/input boundary.
- Selecting a run hands text to a persistent, multiline-capable Word editor without dropping the selection. Selecting an image exposes replace/delete/resize actions. Image dragging previews a new inline slot; resize dragging previews ratio-preserving dimensions and commits only on release.
- Undo/redo updates the view, selection, image cache, and session snapshot. Failed edits retain the original view and dirty state.

**TDD steps**

1. Add projection/layout tests for text-image-text order, table-cell flows, selection by run/image ID, image fit/aspect ratio, placeholders, and stable widgets across frames.
2. Add interaction regressions for edit selection handoff, formatting/alignment commands, image move/resize preview and release commit, cancel/no-op behavior, undo/redo, and asynchronous result application.
3. Implement flow layout and drawing separately from pointer/event handling. Keep adapter calls and decoding out of layout callbacks; use result channels and invalidate the window after applying results.
4. Run `go test ./internal/desktopui` and correct any existing Excel/Word surface regressions.

**Acceptance**

- Paragraphs, tables, text runs, and inline images display in source order and can be selected independently.
- Common supported text and inline image edits update the neutral projection after success and remain visible after reopening.
- Image width/height retain aspect ratio; drag preview does not persist until release.

### Task 6: Async image picker, sample acceptance, and compatibility evidence

**Files**

- Add platform-specific `chooseWordImageFile()` implementations in `internal/desktopui/filepicker_darwin.go`, `filepicker_windows.go`, and `filepicker_other.go`.
- Add a Word image selection/result path in `internal/desktopui/workspace.go` and the existing app result handling in `internal/desktopui/app.go` / `internal/desktopui/layout.go`.
- Update `internal/format/docx/document_test.go` and desktop regression tests to use `GLANCE_OFFICE_TEST_DOCX`.
- Update `docs/office-support.md`, `docs/office-library-evaluation.md`, and `../tools/docs/compatibility/office-feature-matrix.md` with only the capabilities demonstrated by passing evidence.

**TDD and verification steps**

1. Add picker/result-flow tests for cancellation, unsupported extensions, corrupt/oversized images, stale document generations, and ensuring file reads/decode never run in Gio layout.
2. Add the opt-in sample regression: open the external file; assert 106 body paragraphs, 28 inline drawings/image relationships, 27 resolved PNG media parts, one dangling `rId4` (`../NULL`) placeholder, document order, and the resolved images' extent ratios; edit text and resize or replace an image on a temporary copy; save and reopen; verify edited content/media, placeholder, and untouched package parts; confirm source bytes remain unchanged.
3. Add a synthetic table fixture with inline images for the sample gaps (sample has no tables). Exercise text and image edit/save/reopen there.
4. Run `go test ./internal/format/docx ./internal/desktopui`; with the local file available, run the targeted sample test using `GLANCE_OFFICE_TEST_DOCX="/Users/mima1234/Downloads/office/大猫OA软件使用说明.docx"`. Run the relevant full repository regression once the targeted suites pass.
5. Perform a Gio visual pass with the sample and synthetic table fixture. Check visible text flow, scrolling to resolved images, table image pixels, clipping, image selection handles, toolbar enabled state, and save status; explicitly do not score pagination or floating-image layout.
6. Update documentation and the compatibility matrix to distinguish implemented UI behavior, adapter round-trip evidence, sample evidence, and known gaps.

**Acceptance**

- The sample's 27 resolved inline images render in the expected order and preserve their OOXML aspect ratios; its broken relationship remains a placeholder; its original file is unchanged after copy edits.
- Save/reopen evidence covers text, image, relationships, package preservation, and table-cell content before advertising those operations as writable.
- Docs make no claim of Word pagination or unsupported feature fidelity.

## Review Focus

1. Shared media references: replacing or deleting one drawing must not alter any other drawing that uses the same media part.
2. Ordered nested content: table cells with multiple paragraphs and inline images must retain per-cell and per-paragraph order and object identity.
3. XML preservation: boundary whitespace, Unicode, unknown run properties, unknown children, relationships, and unsupported structures must survive targeted edits.
4. Image geometry and resource handling: EMU dimensions, aspect ratio, normalized drag coordinates, decode limits, and LRU eviction must be correct.
5. Gio responsiveness and state: persistent widgets, async image/file/edit work, save failure dirty state, selection refresh, and undo/redo after save must not race or drop edits.
