# PPTX Layout and Master Rendering

**Status:** Design direction approved on 2026-10-08. Implementation is not yet approved.

## Problem

The four supplied PPTX packages pass ZIP integrity checks and open through both `pptx.Open` and the desktop app's `open → install → layout` path. The visible result is incomplete because the adapter projects slide-local shapes but does not resolve all visual content inherited from slide backgrounds, layouts, masters, and theme colors.

The samples show distinct cases:

- `1.pptx` has 30 slides with slide-local text and shapes; it currently opens and projects its first slide.
- `PPT_RealTest.pptx` has two empty slide placeholders with no local transforms. Its layout contains a large JPEG and other inherited objects, so the current slide snapshot has no visible image and zero-size placeholder geometry.
- `slidetemplate.pptx` stores its photo in the slide's `p:bg` image fill. Its title and subtitle geometry comes from the selected layout, but the current snapshot leaves both at the origin with zero size.
- `test-template.pptx` resolves its dark background from a theme color on the master; the current UI falls back to white.

The current adapter's ZIP inspection associates images by package path only, and picture projection selects an arbitrary cached image instead of following the owning part's relationship. The neutral slide model has no background image or read-only inherited visual layer, and the canvas paints a white background before slide elements.

## Goal

Open all four supplied files through the normal desktop flow and render their supported slide-local and inherited content in the correct layer order. Resolve placeholder geometry from the selected layout, then the master. Preserve the existing editable slide-shape behavior and keep Office package types inside the PPTX adapter.

This is a bounded rendering subset. It does not promise pixel parity with Microsoft PowerPoint or LibreOffice.

## Chosen approach

Extend the existing `internal/format/pptx` adapter to resolve package relationships and build a complete neutral visual snapshot before the UI lays out a frame.

For each slide, resolve its `slideLayout` relationship and that layout's `slideMaster` relationship. Use normalized OPC part paths and each part's own `.rels` file; accept only internal relationships for local visuals. Do not infer slide order from filenames or relationship IDs.

Build the rendered slide in this order:

1. The explicit slide background, if present.
2. Otherwise the layout background, then the master background.
3. Master decorative shapes and pictures, then layout decorative shapes and pictures, when the relevant OOXML visibility flags allow them.
4. Slide-local elements in their existing z-order.

Resolve both `p:bgPr` and `p:bgRef` backgrounds. Map theme colors through the master color map and its related theme, including the sample-required `tx1` and `bg1` entries and only the color transforms supported by the adapter. Resolve picture data through the exact `r:embed` relationship belonging to the part that contains the picture or background fill. Do not guess by media iteration order.

For each slide placeholder, use a locally declared transform when available. If absent, match by normalized placeholder `(type, idx)` on the selected layout and then the master; absent `type` and `idx` use the OOXML defaults `obj` and `0`. Do not match by shape name, coordinate, or document order. Keep slide-local text and formatting as the content source; inherited placeholder geometry and supported default presentation properties supply its visual placement. Missing or malformed relationships should produce a safe omission or the existing open error rather than a panic.

Add neutral fields for a slide background image and inherited read-only visuals. The existing canvas, thumbnail, and slideshow paths will paint those values before editable slide elements. Inherited visuals must not enter the text-edit interaction path. The Gio frame loop will consume only the completed neutral snapshot; ZIP/XML parsing and image decoding stay in the adapter's document-open work.

## Compatibility boundary

The initial implementation covers:

- Slide, layout, and master relationship traversal and visual layer ordering.
- Slide/layout/master `p:bgPr` and `p:bgRef` backgrounds, including the sample-required theme colors `tx1` and `bg1`.
- Raster pictures (PNG, JPEG, and GIF) referenced from slide backgrounds or inherited visual shapes, with exact relationship resolution.
- Placeholder transform fallback by `(type, idx)` from slide to layout to master.
- Existing supported neutral text, shape, fill, custom path, gradient, and raster image rendering.

Unsupported media such as EMF, unsupported theme transforms, effects, SmartArt, charts, groups outside the current adapter subset, animations, transitions, and exact Office text shaping remain outside this change. They must remain explicit gaps in the support matrix; the implementation must not claim full-fidelity rendering. Opening these files does not enable new overwrite guarantees or broaden the existing save preservation boundary.

## Components

- `internal/format/pptx/document.go`: read the relationship graph, resolve the effective background and placeholder geometry, and project inherited visuals using their owning-part relationships.
- `internal/document/presentation.go`: add neutral background-image and read-only inherited-visual fields without exposing third-party types.
- `internal/desktopui/presentation_canvas.go` and the shared slide renderer: paint background and inherited visuals before slide-local editable shapes for canvas, thumbnails, and slideshow.
- PPTX adapter and desktop UI regression tests: use compact generated OOXML packages for relationship, inheritance, ordering, and read-only interaction cases.
- `docs/office-support.md`, `docs/office-library-evaluation.md`, and `../tools/docs/compatibility/office-feature-matrix.md`: document the evidence and remaining rendering gaps.

## Acceptance criteria

- The app opens each of the four supplied files without a parse, install, or layout error and reports slide counts of 30, 1, 3, and 3 respectively.
- The `PPT_RealTest.pptx` first slide renders the JPEG referenced by its selected layout; it no longer appears as an empty white slide.
- The `slidetemplate.pptx` first slide renders its slide-level background image and places its title and subtitle using the selected layout's placeholder transforms.
- The `test-template.pptx` first slide resolves its master background through the theme color scheme instead of defaulting to white.
- `1.pptx` retains its current slide-local content projection, slide order, dimensions, and text editing behavior.
- Inherited visuals are read-only. Clicking them does not start slide-shape text editing.
- Synthetic regressions cover explicit slide background, layout/master fallback, theme color lookup, exact media relationship selection, placeholder matching/fallback, missing relationships, and rendering layer order.
- Verification opens and lays out each original sample, checks representative rendered pixels or image presence, and confirms the source PPTX bytes remain unchanged. No sample file is committed to the repository.
- The documentation and compatibility matrix state the tested subset and do not claim PowerPoint fidelity.

## Error handling and resource bounds

- Continue using the existing PPTX open path and ZIP reader; do not introduce a runtime dependency on LibreOffice or another Office application.
- Keep all ZIP and XML parsing outside Gio layout/frame callbacks.
- Normalize relationship targets relative to the owning part and reject external targets for local media reads.
- Decode only supported raster formats through the existing image decoder imports. Unsupported or malformed inherited media stays absent or receives the existing unsupported-content treatment; it must not be substituted with another media part.
- Reuse the existing package/resource limits where available. Do not add unbounded copies of every media part to each slide snapshot; share immutable decoded image references within the opened document.
- Preserve existing source-file bytes during open and display. No save path or overwrite policy changes are part of this design.

## Verification approach

Add adapter tests that construct minimal PPTX packages with explicit and inherited backgrounds, relationships to multiple media parts, and placeholders whose transforms live on the layout or master. Add desktop rendering tests for the background/layer order and the read-only inherited layer. Run the relevant package tests and the full Go test suite, then use the four original Downloads samples for end-to-end open/layout/render checks without modifying them. Update the compatibility documentation from observed test evidence only.
