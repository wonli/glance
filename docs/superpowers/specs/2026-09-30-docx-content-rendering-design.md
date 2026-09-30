# DOCX Content Rendering and Editing Design

## Goal

Render the common body content of DOCX files in document order, including paragraphs, tables, text runs, and inline raster images, and let users edit supported text and image objects while preserving unrelated package content.

## User-facing scope

- Reuse the visual language and shared ribbon chrome from the existing spreadsheet toolbar. Word gets its own command groups and actions; spreadsheet commands such as worksheet and cell operations do not appear in Word.
- The first Word ribbon has Home and Insert tabs. Home provides undo/redo, cut/copy/paste, bold/italic/underline, font color, left/center/right/justified paragraph alignment, and save. Insert provides image insertion. Selecting an image exposes replace, delete, and aspect-ratio-locked size controls. Dragging an image changes its position in the body flow.
- Render body paragraphs, tables, text runs, and inline raster images in document order. Within a paragraph, text and images occupy ordered inline slots; text wrapping around images is not promised. Preserve image aspect ratio. This is a flow preview, not Word-compatible page layout or pagination.
- Text edits target an individual text run so its existing run formatting remains attached to the edited text. Paragraph and table-cell text that maps safely to text runs is editable.
- Inline images in body paragraphs and tables can be inserted, replaced, moved in the flow, resized, and deleted. PNG and JPEG are decoded for preview; GIF preview uses its first frame. Image insertion/replacement accepts PNG and JPEG. Unsupported media types remain intact and appear as placeholders.
- Floating drawings, headers and footers, equations, fields, tracked revisions, and constructs that cannot be mapped safely are displayed as read-only content or a clear placeholder and remain in the DOCX package unchanged.

## Current evidence and sample corpus

The user-provided external sample `大猫OA软件使用说明.docx` contains 106 body paragraphs, 28 inline drawings, 28 image relationships, and 28 PNG media parts. It contains no tables or floating drawings. Use it as a local regression sample without checking the document into the repository. The regression accepts its path through `GLANCE_OFFICE_TEST_DOCX`. Keep synthetic DOCX fixtures for table content, text/image interleaving, and relationship edge cases.

## Architecture

```text
Word ribbon and body view (Gio)
        ↓ text/image intents and neutral view data
Document session and neutral Word/image DTOs (internal/document)
        ↓ supported Word operations
DOCX adapter (internal/format/docx)
        ↓ OOXML parts and local file
```

### Neutral content model

- Add format-neutral Word block, table, text-run, and image-reference view data under `internal/document`. Ordered inline content represents text/image interleaving without exposing ZIP, XML, or Word-specific library values to Gio state.
- A text run includes editable text plus bold, italic, underline, font family, font size, and font color. Paragraph data includes left, center, right, and justified alignment. An image reference carries a stable object ID, media type, original dimensions, and the information needed to request its bytes from the adapter.
- Define narrow optional reader/editor capabilities for Word content and image operations. The DOCX adapter remains responsible for mapping a neutral object ID to OOXML elements, relationship IDs, media parts, and EMU dimensions.
- Represent unsupported content explicitly. Parsing a document must not silently omit a package part or make an unsupported object appear editable.

### DOCX adapter

- Parse `word/document.xml` and its relationship part into ordered body paragraphs, tables, text runs, and `wp:inline` pictures. Resolve `r:embed` IDs to media parts and retain original image bytes and package metadata.
- Leave unsupported/floating XML structures in the source package. Update only the target `w:t` text, selected run's `w:rPr`, paragraph's `w:pPr`, inline picture markup, relationship entries, content types, and media part required by an accepted operation.
- Replacing an image creates or reuses an appropriate media relationship for that image object; it must not alter other drawings that reference the same media part. Deleting a picture removes only its drawing and removes relationships/media only when no package content still references them.
- Insert/replace operations use image media bytes and a supported raster content type. Resize updates the OOXML extent while preserving the source aspect ratio. Move updates the image's inline position in the body content order.
- Save through a temporary sibling file and replace the source only after writing and syncing the complete DOCX package succeeds. A failed operation/save preserves the old package and dirty state.
- Keep project-local ZIP/XML parsing unless corpus evidence shows a concrete gap that justifies a dependency change. Any new library must be evaluated and recorded in `docs/office-library-evaluation.md` before it becomes part of the adapter.

### Gio Word surface and ribbon

- Extract only reusable ribbon chrome, spacing, icon, group, and color behavior from the existing spreadsheet toolbar. Keep separate Word action definitions and dispatch so the current Excel toolbar behavior does not change.
- Start with Word-appropriate Home and Insert commands: undo/redo, clipboard actions, bold/italic/underline, font color, paragraph alignment, insert image, and save. Formatting applies to the selected run; alignment applies to its paragraph. Selecting an image exposes replace, delete, and resize controls in a Picture group.
- Render the body in a virtualized, scrollable ordered flow. Render text runs and inline pictures as distinct hit targets. Selecting a text run populates the persistent Word editor; selecting an image shows its selection state and resize/move affordances.
- Keep editors, list state, clickables, selection, and decoded-image cache in per-document session state. Do not create persistent widgets in each frame. Resolve pointer coordinates at the input boundary and store document dimensions in OOXML-neutral units, not screen pixels.
- File selection, image decoding, package reads, edits that may be expensive, and saves run outside Gio layout/frame callbacks. Apply results through synchronized session state and request a redraw.

## Error handling and preservation

- If a relationship is missing, an image cannot be decoded, or an OOXML construct is unsupported, show a visible placeholder and a user-facing status without dropping its package data.
- Reject raster previews above 32 megapixels and cap the decoded-image cache at 64 MiB with least-recently-used eviction. Avoid retaining duplicate decoded copies for every frame. Failed image operations do not mutate the document or its dirty state.
- Undo and redo cover supported text, formatting, paragraph alignment, and image operations in the active document session, with at most 100 actions. They do not rewrite the source until save.
- Advertise a write operation only after save/reopen fixtures prove that the requested text or image edit survives and representative untouched text styles, tables, image bytes, relationships, and package parts remain intact.
- Keep atomic temporary-sibling replacement and preserve dirty state on failure.

## Verification

- Adapter fixtures cover mixed text runs, boundary whitespace, text and formatting edits, paragraph alignment, inline images between text runs, images in table cells, relationship resolution, supported PNG/JPEG replacement/insertion/deletion, move/resize dimensions, malformed relationships, and unsupported content preservation.
- Save/reopen tests reopen the produced DOCX and verify edited text/media/relationships and representative untouched package parts. No operation is enabled for overwrite before its corresponding round-trip evidence passes.
- A desktop regression opens the user's sample through `GLANCE_OFFICE_TEST_DOCX`, verifies all 28 inline pictures are projected in order with the expected aspect ratios, edits text and resizes or replaces an image on a copy, saves, reopens, and checks the original file is unchanged.
- UI tests cover selection, text editor handoff, ribbon formatting/paragraph commands, image selection and resize/move gestures, ribbon capability state, undo/redo, and async result application. Visual checks use the sample and synthetic table fixture; they do not claim Word page pagination.
- Update `docs/office-support.md`, `docs/office-library-evaluation.md`, and the evidence-based compatibility matrix at `../tools/docs/compatibility/office-feature-matrix.md` as verified features land.

## Out of scope

- Word page layout, pagination, print fidelity, headers/footers, floating drawing layout, and full Office typography parity.
- General shape/chart editing, equation/field editing, tracked-revision editing, and editing unsupported or ambiguous OOXML structures.
- The XLSX picture overlay/editor, which is a separate follow-on subproject and will reuse the shared ribbon chrome from this work.
