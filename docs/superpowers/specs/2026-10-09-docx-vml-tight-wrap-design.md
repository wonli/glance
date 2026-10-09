# DOCX VML Tight-Wrap Preview Design

## Goal

Render the read-only VML floating picture in the pinned `floating-image-wrap-vml.docx` regression fixture at its text-relative offset and wrap paragraph text around its declared tight polygon, including later paragraphs on the same page and column.

## Microsoft reference basis

- [MS-OI29500: `wrap` (Text Wrapping)](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/8b6f1c0c-0283-4e0a-bf36-746e029bd1de) records Microsoft's Word-specific defaults: omitted `anchorx`/`anchory` resolve to `text`, and an omitted `type` resolves to `none`.
- [Microsoft VML `wrapcoords`](https://learn.microsoft.com/en-us/windows/win32/vml/msdn-online-vml-wrapcoords-attribute) defines a coordinate-pair polygon used for tight/through wrapping.
- [Microsoft VML `shape`](https://learn.microsoft.com/en-us/windows/win32/vml/shape-element--vml) describes the shape's positioning, margins, dimensions, wrap polygon, and z-order attributes.
- [Microsoft VML `position`](https://learn.microsoft.com/en-us/windows/win32/vml/position-attribute--shape--vml) says Word floating objects use `absolute` positioning with top/left values relative to the parent.
- [MS-OI29500: VML shape properties](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/d019f23f-5977-43d8-b463-99ba8f6784eb) documents Office's 9 pt default for left/right wrap distance when not specified.
- [Word `WdWrapType`](https://learn.microsoft.com/en-us/office/vba/api/word.wdwraptype) describes tight wrapping and says square, tight, and through behave essentially alike. [Word `WdWrapSideType`](https://learn.microsoft.com/en-us/office/vba/api/word.wdwrapsidetype) defines wrapping on both sides, and the [tight-wrap example](https://learn.microsoft.com/en-us/office/vba/api/word.wrapformat.distanceleft) sets `Side = wdWrapBoth`.
- [MS-DOC `Spa`](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-doc/26fcafc0-554b-46bc-aebf-c755276c77a4) lists tight wrapping as contour flow on left and right and records side value `0` as both sides. This legacy binary reference is evidence for Word behavior; the VML subset accepts the omitted/default or explicit `both` side and leaves explicit left/right/largest-side selection on fallback.

These documents define the input semantics and supported side behavior; they do not establish complete Word renderer parity.

## Sample evidence

`testdata/office-corpus/docx/floating-image-wrap-vml.docx` preserves the original VML corpus sample used for this regression: a picture with positive width/height, `position:absolute`, `margin-left:120.75pt`, `margin-top:26.9pt`, a shape coordinate size of `21600,21600`, a `wrapcoords` polygon, and `<w10:wrap type="tight"/>`. The wrap element omits `anchorx` and `anchory`, so the Office-specific defaults apply. The object is paragraph-leading and followed by prose. The working copy of `floating-image-wrap.docx` changed during this task, so the VML regression uses this separately named immutable fixture and leaves the working copy untouched.

Today `internal/format/docx/word_projection_vml.go` discards the positioning and wrapping data, sets `InlineFallback`, and `internal/wordlayout/paragraph.go` inserts the object as an ordinary image atom. The fixed-width paragraph wrapper consequently cannot create a text exclusion zone.

## Scope and behavior

1. Add format-neutral floating-image metadata to `document.WordImage`: signed X/Y offsets in EMU, the resolved wrap type, left/right wrap distances in EMU, and a polygon represented by normalized shape coordinates. The model contains no VML or Open XML types.
2. Project a VML image as a float only when its dimensions are valid, its shape uses `position:absolute`, its effective anchors are `text`, its wrap side is omitted or `both`, and it has a valid `tight` wrap polygon and coordinate size. Normalize XML units at the DOCX adapter boundary.
3. Interpret omitted Word anchor attributes as `text`; interpret an omitted wrap type as `none`. `none`, `square`, `through`, top/bottom modes, explicit left/right/largest side selection, page/margin anchors, relative/static positioning, malformed coordinates, or unsupported geometry remain visible through the existing inline placeholder/image fallback and a located warning. Do not silently approximate them as tight wrap.
4. A supported body float must precede other flow content in its anchor paragraph; place it at that paragraph's text-area origin plus the VML offsets. If it follows text or a break, preserve source order with a visible inline fallback and warning because this layout slice does not model an intra-paragraph anchor position. Its exclusion polygon is the image-local normalized polygon mapped to its rendered bounds, expanded horizontally by the effective wrap distances. If Office left/right distances are absent, use Microsoft's documented 9 pt defaults.
5. Reflow each body paragraph baseline across every usable horizontal interval in left-to-right reading order. When a tight polygon splits the line, continue the same baseline in the interval on its other side, preserving atom/source order. Keep the float active for subsequent body paragraphs in the same page and column while their line band intersects the polygon. If no interval can hold the next atom, continue below the blocking shape. Expire float exclusions on a page or column transition; keep the already placed image on its anchor page.
6. Preserve normal inline-image layout. VML images in table cells and unsupported float inputs use a visible inline fallback because table measurement/placement has no shared body-flow exclusion context.
7. Keep pictures read-only. Existing image bytes, package parts, and Save behavior are unchanged. Screen and PDF preview continue to consume the same `wordlayout.PageScene`.
8. Any document containing a projected float uses a full layout pass for an edit/revision; suffix reuse is unsafe until float state is part of layout checkpoints.

## Safety and compatibility

- Bound polygon input to 3–256 finite coordinate pairs and reject zero coordinate extents or malformed pairs.
- Reject invalid/non-finite dimensions or offsets as unsupported geometry and show the existing fallback with a warning.
- Do not mutate the DOCX package or claim editable floating-image support.
- Do not claim universal Word parity. This slice covers the VML absolute-position + tight-polygon behavior found in the named fixture; all other floating layouts remain partial.

## Acceptance criteria

- The corpus adapter exposes a resolved, read-only floating image with the sample's dimensions, offsets, tight mode, and polygon.
- The corpus layout scene places one paragraph-leading image at the projected float bounds, wraps text around the polygon in the anchor paragraph/following body paragraphs, uses both sides on the same baseline where available, and returns to full width after the polygon ends.
- Unsupported or malformed VML wrap data remains previewable and produces a located warning.
- Ordinary inline images and existing paragraph/page-flow behavior remain unchanged.
- Updating a paragraph in a document with a float produces pages and locations identical to a full layout from the same inputs.
- DOCX package bytes are not modified by preview projection/layout.
