# DOCX VML Tight-Wrap Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Project and preview the pinned VML corpus fixture at its Microsoft-defined text-relative location while flowing body text around its tight wrap polygon.

**Architecture:** The DOCX adapter converts VML dimensions, offsets, anchor defaults, and wrap polygon into a format-neutral `WordImage` float value. `internal/wordlayout` places supported floats and rewraps body paragraph lines against active polygon exclusions; unsupported modes keep the current visible fallback. Float-bearing documents bypass incremental suffix reuse until checkpoints include float state.

**Tech Stack:** Go, DOCX OOXML/VML projection, `internal/document` neutral Word model, `internal/wordlayout` point-based page scenes.

**Spec:** [docs/superpowers/specs/2026-10-09-docx-vml-tight-wrap-design.md](../specs/2026-10-09-docx-vml-tight-wrap-design.md)

## Global Constraints

- Treat Microsoft's Open Specifications and Microsoft Learn VML documentation as the semantic reference; do not use WPS output as normative evidence.
- Keep third-party and XML details inside the DOCX adapter; page layout consumes only neutral document values.
- Preserve ordinary inline images and existing unsupported-image visible fallbacks.
- Keep VML pictures read-only and leave DOCX save/package behavior unchanged.
- Float-aware reflow is limited to paragraph-leading floats in body paragraphs and same-page/same-column polygon exclusion; later inline anchors retain source order through a visible warning fallback.
- Each baseline consumes all usable horizontal intervals in reading order so text can continue on the other side of a tight polygon before moving down.
- Unsupported wrap modes and malformed geometry remain visible and diagnostic; do not guess a polygon or silently discard content.
- The projected tight-wrap subset accepts the default or explicit `both` side; explicit left/right/largest-side modes keep the visible fallback until their placement policy is modeled.
- Documents containing floats use full layout until incremental checkpoints represent active floats.

## Review Focus

- Missing, malformed, too-short, or oversized `wrapcoords` must not panic or generate unbounded geometry; adapter tests assert a visible warning fallback.
- An absent wrap type means Word `none`, not `tight`; an omitted anchor resolves to `text`, while explicit non-text anchors remain on the fallback path.
- Concave polygons and a line band crossing a polygon vertex must use a conservative valid exclusion interval; layout tests assert text does not overlap the excluded band.
- A shape that leaves no horizontal room must move text below its bottom without looping; layout tests assert progress and retained text.
- Editing an earlier paragraph can shift a later float; incremental output must match a full layout, covered by a float-bearing incremental regression.

---

### Task 1: Project bounded VML float geometry into the neutral model

**Files:**
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_projection_vml.go`
- Modify: `internal/format/docx/word_projection_clone.go`
- Modify: `internal/format/docx/word_projection_vml_test.go`
- Modify: `internal/format/docx/word_layout_projection_test.go` if a focused ZIP fixture is needed

**Interfaces:**
- Produces: `document.WordImage.Floating *document.WordImageFloating`; `WordImageFloating` stores signed `XOffsetEMU`/`YOffsetEMU`, `WrapType`, left/right `WrapDistanceEMU`, and normalized `[]WordImageWrapPoint` polygon coordinates.
- Adapter projection treats omitted `anchorx`/`anchory` as `text`, omitted wrap type as Word `none`, and only constructs floating metadata for absolute/text/tight inputs with a valid polygon.

- [x] **Step 1: Add adapter regressions**
  - Update `TestFloatingImageWrapCorpusProjectsVMLImageAndFollowingText` to assert the pinned VML corpus sample is resolved/read-only, has `InlineFallback == false`, has the expected EMU dimensions and offsets, uses `tight`, and contains the normalized polygon points.
  - Add table-driven VML fixture cases proving omitted type, non-text anchors, malformed coordinates, and unsupported positioning retain a visible image fallback with a paragraph warning.
- [x] **Step 2: Run the adapter regressions and confirm they fail for the missing neutral float metadata**

Run: `go test ./internal/format/docx -run 'TestFloatingImageWrapCorpusProjectsVMLImageAndFollowingText|TestVMLFloatingImageUnsupportedGeometryFallsBack' -count=1`

Expected: FAIL because `WordImage.Floating` and the bounded VML projection are not implemented.
- [x] **Step 3: Add neutral float structs and bounded VML parsers**
  - Parse signed VML offsets and positive dimensions with checked finite conversions.
  - Parse at most 256 coordinate pairs; normalize by the shape coordinate extent and reject fewer than three pairs, odd counts, invalid numbers, or zero extents.
  - Resolve Microsoft Word anchor/wrap defaults and Microsoft wrap-distance defaults; unsupported semantics must set the existing `InlineFallback` path and a located warning.
- [x] **Step 4: Run the adapter regressions and DOCX adapter package**

Run: `go test ./internal/format/docx -run 'TestFloatingImageWrapCorpusProjectsVMLImageAndFollowingText|TestVMLFloatingImageUnsupportedGeometryFallsBack' -count=1`

Expected: PASS; then `go test ./internal/format/docx -count=1` passes.

### Task 2: Place and flow body text around active tight polygons

**Files:**
- Modify: `internal/wordlayout/paragraph.go`
- Modify: `internal/wordlayout/paragraph_wrap.go`
- Modify: `internal/wordlayout/paragraph_placement.go`
- Modify: `internal/wordlayout/cursor.go`
- Modify: `internal/wordlayout/incremental.go`
- Create: `internal/wordlayout/floating.go`
- Modify: `internal/wordlayout/layout_floating_test.go`

**Interfaces:**
- Consumes: `document.WordImageFloating` from Task 1.
- Produces: a private float-aware paragraph placement path; `PageScene.Images` retains ordinary image bounds, and line placement receives an optional explicit horizontal range.

- [x] **Step 1: Add layout regressions**
  - `TestLayoutTightFloatWrapsAnchorAndFollowingParagraphs` places a rectangle/tight polygon in one body paragraph and proves later paragraph glyphs stay outside it, the image uses its text-relative offsets, and text returns to the full width below it.
  - `TestLayoutTightFloatFlowsOneBaselineAcrossBothIntervals` proves text continues at the same baseline in both available intervals.
  - `TestLayoutTightFloatMovesTextBelowWhenNoLineIntervalFits` proves full-width exclusions advance vertically without dropping text or looping.
  - `TestFloatBearingIncrementalLayoutMatchesFullLayout` changes text before/at a float anchor and compares pages and source locations to a fresh full layout.
  - `TestFloatingImageFallbackInTableKeepsVisibleImageAndWarns` covers the bounded table fallback.
- [x] **Step 2: Run the layout regressions and confirm they fail on inline placement/overlap**

Run: `go test ./internal/wordlayout -run 'TestLayoutTightFloatWrapsAnchorAndFollowingParagraphs|TestLayoutTightFloatMovesTextBelowWhenNoLineIntervalFits|TestFloatBearingIncrementalLayoutMatchesFullLayout|TestFloatingImageFallbackInTableKeepsVisibleImageAndWarns' -count=1`

Expected: FAIL because floating metadata is ignored and the float image is currently inserted in ordinary text flow.
- [x] **Step 3: Implement polygon obstacles and float-aware line placement**
  - Keep raw paragraph atoms so lines can be rewrapped using the interval available at each line Y.
  - Register float image bounds at anchor paragraph text-area origin plus its offsets; draw a single `ImagePlacement` and maintain a page/column-scoped exclusion polygon.
  - Compute conservative polygon X bounds across a line's vertical band and subtract all active exclusion zones from the paragraph's indented width.
  - Reuse the existing paragraph line wrapper for word breaks, tabs, hard breaks, and page-break markers; consume each usable interval from left to right on the same baseline while preserving alignment/justification and source run offsets.
  - If no interval can fit a line, advance to the nearest intersecting float bottom; expire obstacles when page/column flow advances.
  - In table cells, keep the image visible as an inline fallback with a located warning.
  - Disable incremental suffix reuse for documents containing any floating image until checkpoint state tracks exclusions.
- [x] **Step 4: Run the layout regressions and Word layout package**

Run: `go test ./internal/wordlayout -run 'TestLayoutTightFloatWrapsAnchorAndFollowingParagraphs|TestLayoutTightFloatMovesTextBelowWhenNoLineIntervalFits|TestFloatBearingIncrementalLayoutMatchesFullLayout|TestFloatingImageFallbackInTableKeepsVisibleImageAndWarns' -count=1`

Expected: PASS; then `go test ./internal/wordlayout -count=1` passes.

### Task 3: Verify the corpus path and publish the compatibility boundary

**Files:**
- Modify: `internal/format/docx/word_projection_vml_test.go`
- Modify: `internal/desktopui/word_corpus_layout_test.go`
- Modify: `docs/office-support.md`
- Modify: `../tools/docs/compatibility/office-feature-matrix.md` only if that sibling reference is writable and tracks this repository's Glance row

**Interfaces:**
- Consumes: adapter and float-aware layout behavior from Tasks 1–2.
- Produces: a corpus regression for `floating-image-wrap-vml.docx` and an evidence-based compatibility note that separates this bounded wrap subset from universal Word parity.

- [x] **Step 1: Add the corpus layout regression**
  - `TestFloatingImageWrapCorpusFlowsTextAlongTightPolygon` opens `floating-image-wrap-vml.docx` through the real DOCX adapter and production shaper, checks the projected float placement, verifies text starts in available intervals around it and resumes at full width after the shape bottom, and confirms projection leaves package bytes unchanged.
- [x] **Step 2: Run the corpus regression as end-to-end acceptance after the adapter/layout implementation**

Run: `go test ./internal/desktopui -run 'TestFloatingImageWrapCorpusFlowsTextAlongTightPolygon' -count=1`

Expected: PASS; the corpus scene places the floating image and reflows text around its polygon.
- [x] **Step 3: Update DOCX support and compatibility evidence**
  - State that paragraph-leading absolute/text-anchor/tight-polygon VML is supported for preview in body paragraphs, including same-page/same-column following text; later inline anchors use a visible warning fallback.
  - Keep other float modes, table-cell floats, edit/save, and exact Word rendering parity marked partial/unsupported with their fallbacks.
- [x] **Step 4: Run the requested final verification set**

Run: `go test ./internal/format/docx ./internal/wordlayout ./internal/desktopui -count=1`

Expected: all three package suites pass and the corpus regression is included.
- [x] **Step 5: Review the final diff against the spec**
  - Confirm normal inline image behavior is unchanged, float pictures remain read-only, and no DOCX source bytes are written.
- Check all spec acceptance criteria and list any documented unsupported case in the final report.

### Task 4: Continue a baseline on both sides of a tight float

The screenshot review found that selecting only the widest available interval leaves the shape's opposite side unused, unlike the target Word flow. Microsoft describes tight wrap as close to the shape and defines wrapping on both sides; the approved fixture omits a side restriction.

**Files:**
- Modify: `internal/wordlayout/floating.go`
- Modify: `internal/wordlayout/paragraph_placement.go`
- Modify: `internal/wordlayout/layout_floating_test.go`
- Modify: `internal/desktopui/word_corpus_layout_test.go`
- Modify: `internal/format/docx/word_projection_vml.go`
- Modify: `internal/format/docx/word_projection_vml_test.go`
- Modify: `docs/office-support.md`
- Modify: `../tools/docs/compatibility/office-feature-matrix.md`

- [x] Add a synthetic regression requiring text to occupy both intervals at one baseline and confirm it fails against widest-interval-only placement.
- [x] Flow paragraph atoms through usable intervals left-to-right on the same baseline; preserve source order and advance vertically only after those intervals are exhausted.
- [x] Extend the production-shaper corpus regression to require both sides on one baseline and full-width flow below the polygon.
- [x] Keep explicit one-side VML wrapping on the existing visible fallback; omitted side is treated as both for the fixture subset.
- [x] Update the compatibility boundary and run the layout plus corpus regressions.
