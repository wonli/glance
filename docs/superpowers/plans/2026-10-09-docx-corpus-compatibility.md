# DOCX Corpus Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render the special content in the curated DOCX corpus without allowing unsupported elements to hide the rest of the document.

**Architecture:** Keep OOXML parsing in `internal/format/docx`, project only format-neutral values into `internal/document`, and place page/section content in the shared `internal/wordlayout` scenes. Preserve source bytes and use visible, located warnings only for bounded fallbacks.

**Tech Stack:** Go, existing DOCX package parser, neutral document model, shared Word layout engine, Gio headless Office QA.

**Spec:** `docs/superpowers/specs/2026-10-09-docx-corpus-compatibility-design.md`

## Global Constraints

- Keep third-party and OOXML package types behind the DOCX adapter.
- Preserve source DOCX parts on read-only preview and on unrelated edits.
- Do not clear an error diagnostic unless the sample's content has a visible projection and a regression assertion.
- Keep cached fields read-only and warn that they are not recalculated.
- Keep unsupported malformed geometry as a located error when no safe visible fallback exists.

## Review Focus

- Note references with absent, duplicate, separator, or negative IDs must not bind to the wrong note; test them in Task 4.
- Numbering overrides must be scoped to one `numId`; custom restarts remain read-only unless the fixture test proves them; cover them in Task 3.
- Header inheritance for default/first/even pages must not leak a header across sections; cover the corpus's three sections in Task 5.
- VML and DrawingML relationship IDs may be missing/external; retain a located fallback without hiding neighboring text in Task 6.
- Nested tables and percentage widths must remain inside their parent cell/page bounds; cover nested and malformed fixtures in Task 7.

---

### Task 1: Restore the Spreadsheet Cell Font Contract

**Files:**
- Modify: `internal/desktopui/grid_paint.go`
- Test: `internal/desktopui/grid_paint_test.go`

- [x] Confirm the failing editor-style test and the current shared viewer/editor font-size implementation.
- [x] Reconcile the stale editor-style assertion against the 12sp renderer baseline; keep production font sizing unchanged and assert that editor sizing follows `gridCellTextSize`.

### Task 2: Support Soft Breaks and Stored Pagination Hints

**Files:**
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_projection_inline.go`
- Modify: `internal/format/docx/word_projection_paragraph.go`
- Modify: `internal/wordlayout/paragraph.go`
- Modify: `internal/desktopui/word_paragraph_layout.go`
- Test: `internal/format/docx/word_page_break_test.go`, `internal/wordlayout/layout_inline_content_test.go`

- [x] Add failing corpus-backed tests for soft line breaks and stored pagination hints; assert real page breaks remain projected.
- [x] Map soft breaks to the existing line-break layout atom and ignore stored pagination hints.
- [x] Focused projection/layout tests pass; `sample.docx` and `page-breaks.docx` render without blocking inline errors.

### Task 3: Render the Corpus Numbering Formats

**Files:**
- Modify: `internal/format/docx/word_relationships.go`
- Modify: `internal/format/docx/word_projection_paragraph.go`
- Test: `internal/format/docx/word_numbering_test.go`

- [x] Add fixture-backed assertions for every numbered-list marker and nested alphabetic level in `numbered-lists.docx`.
- [x] Parse and format the sample's alphabetic numbering without changing custom restart safety.
- [x] Numbering regressions pass; the sample renders without `error:unsupported-list`.

### Task 4: Project Footnotes and Endnotes

**Files:**
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_projection_core.go`
- Modify: `internal/format/docx/word_projection_inline.go`
- Modify: `internal/format/docx/word_relationships.go`
- Modify: `internal/wordlayout/paragraph.go`
- Test: create or extend `internal/format/docx/word_notes_test.go` and `internal/wordlayout/layout_inline_content_test.go`

- [x] Add corpus tests proving both note references resolve to the expected text and remain read-only.
- [x] Project related note parts, skip separator records, preserve order, and append stable reference markers/content to the neutral model.
- [x] Production-shaper corpus preview tests cover both samples; snapshot and dirty state are unchanged after read-only projection.

### Task 5: Render Section Headers

**Files:**
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_layout_projection.go`
- Modify: `internal/wordlayout/layout.go`
- Modify: `internal/wordlayout/model.go`
- Modify: Gio/PDF page-scene renderers as needed
- Test: create or extend `internal/format/docx/word_layout_sections_test.go` and `internal/wordlayout/layout_pagination_test.go`

- [x] Add corpus assertions mapping each of the three header strings to its own section.
- [x] Project default header relationships/text, inherit omitted headers, and tolerate one twip of geometry rounding.
- [x] Scene tests verify headers repeat only on the matching section pages; genuine geometry differences remain blocking.

### Task 6: Project Floating VML and DrawingML Content

**Files:**
- Modify: `internal/format/docx/word_projection_inline.go`
- Modify: `internal/format/docx/word_projection_paragraph.go`
- Modify: `internal/document/word.go`
- Modify: `internal/wordlayout/paragraph.go`, `internal/wordlayout/model.go`
- Test: create or extend `internal/format/docx/word_projection_test.go` and `internal/wordlayout/layout_inline_content_test.go`

- [x] Add corpus tests for the VML picture, resolved image bytes, dimensions, and adjacent text.
- [x] Project VML/anchored pictures to read-only inline fallbacks and emit located warnings when exact floating wrap is unavailable.
- [x] The sample renders without a blocking inline error; the visible fallback stays within the accepted page flow.

### Task 7: Preserve Nested Tables and AlternateContent Fallbacks

**Files:**
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_projection_table.go`
- Modify: `internal/format/docx/word_layout_projection_table.go`
- Modify: `internal/wordlayout/table.go` and table pagination files
- Modify: `testdata/office-corpus/manifest.json`
- Test: `internal/format/docx/word_layout_tables_test.go`, `internal/wordlayout/layout_table_pagination_test.go`

- [x] Add sample-backed tests for nested-table text/order and percentage-width fallbacks in `drawing-shapes.docx`.
- [x] Flatten nested-cell paragraphs into read-only parent-cell flow and resolve percentage widths against declared grid geometry.
- [x] Project readable Choice/Fallback textbox text with a visible warning for unsupported chart/OLE graphics.
- [x] Correct manifest feature tags to describe the actual grouped drawing, textbox, chart, and AlternateContent fixture.

### Task 8: Update Compatibility Evidence and Verify the Corpus

**Files:**
- Modify: `testdata/office-corpus/manifest.json`
- Modify: `docs/office-support.md`
- Modify: `docs/compatibility/office-feature-matrix.md`
- Modify: `docs/compatibility/office-render-qa.md`

- [x] Add a production-shaper regression gate requiring a previewable page scene for every curated DOCX.
- [x] Run DOCX, wordlayout, and desktop UI tests, then `go test ./... -count=1`.
- [x] Run full `qa render`; all 17 DOCX produce previewable scenes, all 50 corpus outcomes match the manifest, and remaining warnings and limitations are recorded.
