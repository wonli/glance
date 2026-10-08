# PPTX Layout and Master Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render supported backgrounds, layout/master visuals, and inherited placeholder geometry while keeping all four supplied PPTX files usable through the normal desktop flow.

**Architecture:** The PPTX adapter will resolve each part's OPC relationships and project supported inherited content into the neutral presentation snapshot. Canvas, thumbnail, and slideshow code will paint the immutable background and read-only inherited layer before slide-local elements. Parsing and image decoding stay in the adapter's open path.

**Tech Stack:** Go, `archive/zip`, `encoding/xml`, `image`, Gio, existing `github.com/F31/go-pptx/v2 v2.0.3` adapter.

**Spec:** `docs/superpowers/specs/2026-10-08-pptx-layout-master-rendering-design.md`

## Global Constraints

- Keep third-party Office library types behind format adapters.
- Continue using the existing PPTX open path and ZIP reader; do not introduce a runtime dependency on LibreOffice or another Office application.
- Keep all ZIP and XML parsing outside Gio layout/frame callbacks.
- Normalize relationship targets relative to the owning part and reject external targets for local media reads.
- Decode only supported raster formats through the existing image decoder imports. Unsupported or malformed inherited media stays absent or receives the existing unsupported-content treatment; it must not be substituted with another media part.
- Reuse the existing package/resource limits where available. Do not add unbounded copies of every media part to each slide snapshot; share immutable decoded image references within the opened document.
- Preserve existing source-file bytes during open and display. No save path or overwrite policy changes are part of this design.
- Do not claim pixel parity with Microsoft PowerPoint or LibreOffice.

## Review Focus

- External relationship targets must never be opened as local images; pin with `TestOPCPackageRejectsExternalImageRelationships` in Task 1.
- Relative and root-relative targets must resolve from the owner part without escaping the package; pin with `TestResolveOPCTargetNormalizesOwnerRelativeAndRootRelativePaths` in Task 1.
- A disabled master/layout visual layer must not leak into a slide; pin with `TestInheritedVisualsRespectShowMasterSpFlags` in Task 3.
- An unsupported or corrupt raster target must be omitted without borrowing another cached image; pin with `TestPPTXPictureWithUnsupportedMediaHasNoImage` in Task 1 and `TestPPTXBackgroundWithUnsupportedMediaHasNoImage` in Task 2.
- Missing/duplicate placeholder matches must retain local geometry when present and otherwise stay safe at zero bounds; pin with `TestPlaceholderGeometryUsesLocalThenUniqueTypeAndIndexFallback` in Task 3.

---

## File Structure

- `internal/format/pptx/package_relationships.go`: OPC relationship parsing, safe target normalization, ordered slides, and package-part lookup.
- `internal/format/pptx/inherited.go`: effective background/theme resolution.
- `internal/format/pptx/inherited_projection.go`: inherited visual projection and placeholder geometry matching.
- `internal/format/pptx/document.go`: connect package inspection to `Snapshot` and retain the existing slide-local projection/edit behavior.
- `internal/document/presentation.go`: neutral background image and inherited read-only visual fields.
- `internal/desktopui/presentation_static_layers.go`: shared non-interactive paint helpers for background images and static elements.
- `internal/desktopui/presentation_canvas.go`, `presentation_thumbnail.go`: paint the static layers in the same order before slide-local elements.
- `internal/format/pptx/inherited_test.go`: compact generated package fixtures and adapter behavior regressions.
- `internal/desktopui/presentation_layers_test.go`: Gio pixel and read-only interaction regressions.
- `internal/desktopui/presentation_workspace_test.go`: four-sample open/layout/render acceptance check.
- `docs/office-support.md`, `docs/office-library-evaluation.md`, `../tools/docs/compatibility/office-feature-matrix.md`: tested boundary, evidence, and remaining gaps.

The package tests use `func writePPTXFixture(t *testing.T, replacements map[string][]byte) string`: save a disposable `NewBlank` deck, replace only the named ZIP parts, and return its temporary path. Do not use or rewrite the supplied Downloads files as fixtures.

## Task 1: Resolve OPC relationships and exact media references

**Files:**
- Create: `internal/format/pptx/package_relationships.go`
- Create: `internal/format/pptx/inherited_test.go`
- Modify: `internal/format/pptx/document.go`

**Interfaces:**
- Produce `type opcRelationship struct { ID, Type, Target string; External bool }` and `type pptxPackage struct { parts map[string]*zip.File; rels map[string]map[string]opcRelationship; slideParts []string }`.
- Produce `func newPPTXPackage(zr *zip.Reader) *pptxPackage`, `func parseOPCRelationships(data []byte) map[string]opcRelationship`, `func resolveOPCTarget(ownerPart, target string) (string, bool)`, `func relationshipPartPath(ownerPart string) string`, `func (p *pptxPackage) relationship(ownerPart, relID string) (opcRelationship, bool)`, and `func (p *pptxPackage) relatedPart(ownerPart, relationshipType string) (string, bool)`.
- `pptxPackage.slideParts` follows `presentation.xml` slide order; no filename or relationship-ID ordering.

- [x] **Step 1: Write failing relationship and image-reference tests** in `inherited_test.go`.
  - `TestResolveOPCTargetNormalizesOwnerRelativeAndRootRelativePaths`: from `ppt/slides/slide1.xml`, resolve `../media/image2.png` to `ppt/media/image2.png` and `/ppt/media/image3.png` to `ppt/media/image3.png`; reject a target that escapes the package root.
  - `TestOPCPackageRejectsExternalImageRelationships`: a relationship with `TargetMode="External"` is not returned as a local part.
  - `TestPPTXPackagePreservesPresentationSlideRelationshipOrder`: a fixture whose relationship IDs and part filenames sort differently still reports parts in `presentation.xml` order.
  - `TestPPTXMissingOrMalformedRelationshipsAreSafe`: missing and malformed `.rels` entries produce a safe absent relationship without a panic.
  - `TestPPTXPictureUsesItsOwningPartRelationship`: two image relationships point to distinct solid-color PNGs and a picture referencing `rId2` selects only image 2.
  - `TestPPTXPictureWithUnsupportedMediaHasNoImage`: an unsupported/corrupt referenced target produces a picture with `Image == nil`, never an arbitrary cached image.
  - `TestPPTXReferencedRasterDecoderSupportsPNGJPEGGIF`: exact local relationships to each supported raster encoding decode to non-nil images.
- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/format/pptx -run 'TestResolveOPCTarget|TestOPCPackageRejectsExternalImageRelationships|TestPPTX(Package|Missing|Picture|ReferencedRaster)' -count=1`
Expected: FAIL because relationship records, safe owner-relative target resolution, and exact picture mapping are not implemented.

- [x] **Step 3: Implement the package graph and exact relationship lookup** in `package_relationships.go`; replace `projectShape`'s first-map-entry picture selection in `document.go` with the owning slide part's `r:embed` lookup.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/format/pptx -run 'TestResolveOPCTarget|TestOPCPackageRejectsExternalImageRelationships|TestPPTX(Package|Missing|Picture|ReferencedRaster)' -count=1`
Expected: PASS; malformed, external, escaping, and unsupported references yield no local image and do not panic.

- [x] **Step 5: Commit**

```bash
git add internal/format/pptx/package_relationships.go internal/format/pptx/inherited_test.go internal/format/pptx/document.go
git commit -m "fix: resolve PPTX media through package relationships"
```

## Task 2: Resolve effective backgrounds and theme colors

**Files:**
- Modify: `internal/document/presentation.go`
- Create: `internal/format/pptx/inherited.go`
- Modify: `internal/format/pptx/document.go`
- Test: `internal/format/pptx/inherited_test.go`

**Interfaces:**
- Add `BackgroundImage image.Image` and `InheritedElements []PresentationElement` to `document.PresentationSlide`; these are immutable snapshot data and stay separate from editable `Elements`.
- Produce `type pptxBackground struct { ColorHex string; Image image.Image }` and `func (p *pptxPackage) resolveBackground(slidePart, layoutPart, masterPart string) pptxBackground`.
- `resolveBackground` applies explicit slide background, then layout, then master; it parses `p:bgPr` and `p:bgRef`, follows the owning part's image relationship, and resolves supported scheme colors using the master color map and related theme.

- [x] **Step 1: Write failing background tests** in `inherited_test.go`.
  - `TestPPTXBackgroundUsesSlideThenLayoutThenMasterFallback`: a slide image wins over layout/master; with slide background removed, the layout color wins; with both removed, the master color wins.
  - `TestPPTXBackgroundWithUnsupportedMediaHasNoImage`: an unsupported referenced background image remains absent and does not select another package image.
  - `TestPPTXBackgroundResolvesThemeTx1AndBg1ThroughMasterClrMap`: `tx1`/`bg1` map through `p:clrMap` to the selected theme scheme entries, and both `p:bgPr` and `p:bgRef` are covered.
  - `TestPPTXBackgroundUsesExactOwningPartRelationship`: a background fill in a slide chooses its referenced media part even when an unrelated image appears earlier in the ZIP.
- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/format/pptx -run 'TestPPTXBackground' -count=1`
Expected: FAIL because the neutral slide model and adapter do not resolve inherited backgrounds or theme colors.

- [x] **Step 3: Implement `resolveBackground`** in `inherited.go`, add the neutral fields, and populate them for each ordered slide during adapter snapshot preparation. Decode only PNG/JPEG/GIF through the existing image decoder registrations.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/format/pptx -run 'TestPPTXBackground' -count=1`
Expected: PASS; explicit and inherited backgrounds, supported theme mappings, and exact relationship selection match the fixture assertions.

- [x] **Step 5: Commit**

```bash
git add internal/document/presentation.go internal/format/pptx/inherited.go internal/format/pptx/inherited_test.go internal/format/pptx/document.go
git commit -m "feat: project PPTX slide backgrounds"
```

## Task 3: Project layout/master visuals and placeholder geometry

**Files:**
- Create: `internal/format/pptx/inherited_projection.go`
- Modify: `internal/format/pptx/inherited.go`
- Modify: `internal/format/pptx/document.go`
- Test: `internal/format/pptx/inherited_test.go`

**Interfaces:**
- Produce `func (p *pptxPackage) inheritedElements(slidePart, layoutPart, masterPart string) []document.PresentationElement` for supported decorative layout/master shapes and raster pictures, in master-then-layout order.
- Produce `func (p *pptxPackage) placeholderBounds(slidePart, layoutPart, masterPart string) map[uint32]document.PresentationRect`, keyed by slide `cNvPr` ID, and apply results only when the slide placeholder has no local transform.
- Match placeholders on normalized `(type, idx)`, with missing type/index normalized to `obj`/`0`; local slide transform wins, then layout, then master. Duplicate candidates are ambiguous and are not matched.

- [x] **Step 1: Write failing inheritance tests** in `inherited_test.go`.
  - `TestPPTXInheritedVisualsRenderMasterThenLayout`: supported master decoration precedes layout decoration in the snapshot; direct slide elements remain in their existing collection and order.
  - `TestInheritedVisualsRespectShowMasterSpFlags`: `showMasterSp="0"` on the slide or layout suppresses master visuals while layout visuals remain visible.
  - `TestPlaceholderGeometryUsesLocalThenUniqueTypeAndIndexFallback`: explicit slide geometry wins; absent transform matches only equal normalized type/index on layout, then master; absent/duplicate candidates do not borrow a different placeholder's geometry.
  - `TestPPTXLayoutPictureUsesLayoutRelationship`: an inherited picture follows the layout part's own relationship and selects its exact image.
- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/format/pptx -run 'TestPPTXInheritedVisuals|TestInheritedVisuals|TestPlaceholderGeometry|TestPPTXLayoutPicture' -count=1`
Expected: FAIL because layout/master visuals and placeholder geometry are not projected.

- [x] **Step 3: Implement inherited XML projection and placeholder fallback** in `inherited_projection.go`; populate `InheritedElements` and the fallback-bound lookup before `Snapshot` returns. Project only the supported shape/picture subset, honor slide/layout `showMasterSp` by suppressing master shapes while preserving layout shapes, and omit malformed or unsupported nodes safely.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/format/pptx -run 'TestPPTXInheritedVisuals|TestInheritedVisuals|TestPlaceholderGeometry|TestPPTXLayoutPicture' -count=1`
Expected: PASS; ordering, visibility, normalized placeholder matching, and owning-part image lookup match fixture assertions.

- [x] **Step 5: Commit**

```bash
git add internal/format/pptx/inherited_projection.go internal/format/pptx/inherited.go internal/format/pptx/inherited_test.go internal/format/pptx/document.go
git commit -m "feat: project PPTX layout and master visuals"
```

## Task 4: Paint static layers in canvas, thumbnails, and slideshow

**Files:**
- Create: `internal/desktopui/presentation_static_layers.go`
- Modify: `internal/desktopui/presentation_canvas.go`
- Modify: `internal/desktopui/presentation_thumbnail.go`
- Create: `internal/desktopui/presentation_layers_test.go`

**Interfaces:**
- Produce non-interactive helpers `drawPresentationBackground(gtx layout.Context, slide document.PresentationSlide, size image.Point)` and `(a *App) drawPresentationStaticElements(gtx layout.Context, elements []document.PresentationElement, scale float64, bounds image.Rectangle)`; the app method reuses the existing read-only thumbnail text renderer.
- Static elements never register clickables, selection, caret, or editing state. Keep slide-local `layoutPresentationElement` interaction behavior unchanged.
- Canvas and thumbnails paint the resolved background, then `InheritedElements`, then existing slide-local `Elements`; slideshow continues to use the canvas path.

- [x] **Step 1: Write failing Gio regressions** in `presentation_layers_test.go`.
  - `TestPresentationCanvasPaintsBackgroundInheritedAndSlideOrder`: a headless pixel capture verifies the background image/color, inherited decoration over it, and slide-local shape over both at three sample points.
  - `TestPresentationThumbnailPaintsBackgroundAndInheritedLayer`: thumbnail pixels include the background image and inherited decoration before slide elements.
  - `TestPresentationInheritedVisualsAreNotInteractive`: clicking an inherited shape/picture does not set `selectedShape`, `editingShape`, or create an editor semantic; clicking a slide-local text shape still enters edit mode.
- [x] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/desktopui -run 'TestPresentation(Canvas|Thumbnail|InheritedVisuals)' -count=1`
Expected: FAIL because the canvas and thumbnails currently omit background images and inherited elements.

- [x] **Step 3: Implement shared static drawing helpers** in `presentation_static_layers.go`; use them from canvas and thumbnail before local elements without adding input handlers.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/desktopui -run 'TestPresentation(Canvas|Thumbnail|InheritedVisuals)' -count=1`
Expected: PASS; sampled pixels show the prescribed layer order and inherited shapes remain read-only.

- [x] **Step 5: Commit**

```bash
git add internal/desktopui/presentation_static_layers.go internal/desktopui/presentation_canvas.go internal/desktopui/presentation_thumbnail.go internal/desktopui/presentation_layers_test.go
git commit -m "feat: render PPTX inherited slide layers"
```

## Task 5: Verify all four samples and update the compatibility record

**Files:**
- Modify: `internal/desktopui/presentation_workspace_test.go`
- Modify: `docs/office-support.md`
- Modify: `docs/office-library-evaluation.md`
- Modify: `../tools/docs/compatibility/office-feature-matrix.md`

**Interfaces:**
- Add `TestPPTXLayoutMasterSamplesOpenAndRender`; read sample files from `GLANCE_PPTX_TEST_DIR` when set, otherwise `/Users/mima1234/Downloads`, and skip with the missing filename if a sample is absent.
- Expected slide counts: `1.pptx` 30, `PPT_RealTest.pptx` 1, `slidetemplate.pptx` 3, `test-template.pptx` 3.

- [x] **Step 1: Add the four-sample acceptance test** in `presentation_workspace_test.go`.
  - Open and install each sample through `loadDocument` and `installDocument`, lay out its first slide, assert the slide count and nonzero content geometry/image presence specific to that sample, and compare file bytes before and after.
  - Verify `PPT_RealTest.pptx` resolves its selected layout image, `slidetemplate.pptx` has its slide background and nonzero title/subtitle bounds, and `test-template.pptx` resolves its dark theme background.
  - Keep `1.pptx` assertions on dimensions, slide order/count, and existing local slide content; do not edit or save the originals.
- [x] **Step 2: Run the targeted sample acceptance test**

Run: `go test ./internal/desktopui -run TestPPTXLayoutMasterSamplesOpenAndRender -count=1 -v`
Expected: PASS for all four files; source byte comparisons remain equal.

- [x] **Step 3: Update the support, library-evaluation, and feature-matrix entries** with the exact tested subset, regression locations, known unsupported media/theme/effects, and explicit non-parity boundary.

- [x] **Step 4: Run focused package tests and the full suite**

Run: `go test ./internal/format/pptx ./internal/desktopui -count=1`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/desktopui/presentation_workspace_test.go docs/office-support.md docs/office-library-evaluation.md ../tools/docs/compatibility/office-feature-matrix.md
git commit -m "test: verify inherited PPTX rendering samples"
```

## Final Review

- Confirm all spec requirements map to Tasks 1–5: OPC part traversal (1), exact media (1–3), backgrounds/theme mapping (2), inherited visuals and placeholder geometry (3), static UI layer order/read-only behavior (4), four original samples plus unchanged source bytes (5), and documentation (5).
- Run `git diff --check` and `git status --short`; verify no supplied PPTX or diagnostic image is staged.
- Follow `superpowers:verification-before-completion` before reporting the bug fixed.
