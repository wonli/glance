# Word Shared Page Rendering and Editing Design

## Goal

Make Word's page layout the single source of document geometry for the editable continuous view and print layout. Switching view modes may change page surfaces, shadows, and inter-page separators, but it must not change the text, tables, styles, or object positions within a page.

The paginated renderer currently has better style fidelity than the continuous renderer. Preserve that fidelity while restoring the editing interactions that are currently available only in continuous view.

## User-visible behavior

- Continuous and print layout use identical page dimensions, text wrapping, paragraph spacing, table column widths, cell fills, borders, inline images, and page breaks.
- Continuous view stacks those same pages with a thin separator between pages. Print layout shows separate white paper surfaces with the existing page gaps and shadows.
- Clicking a run in either view places the caret at the matching character offset. Text entry, selection, keyboard navigation, IME composition, supported toolbar commands, and table-cell text editing behave the same in both views.
- Supported images remain selectable and keep the current image toolbar and resize/rotation interactions in both views.
- Manual zoom scales the page and all page content together. Fit-to-page and fit-to-multiple-pages remain separate presets; manual zoom is not reset by a frame redraw. Pages larger than the viewport can be panned horizontally and vertically.
- Layout or edit failures keep the current document and draft data intact, show a useful status, and reject stale background layout results.

## Current implementation

- `internal/wordlayout` resolves supported Word styles, wraps text, paginates blocks, and returns point-based `PageScene` values with glyph outlines, cell bounds, paragraph borders, and run-to-page positions.
- `layoutWordPageView` replays that result as a static scene. It does not route pointer events to `widget.Editor` instances.
- `layoutWordCanvas` independently lays out paragraphs and tables as Gio widgets and owns the active `widget.Editor` interactions. It performs separate text measurement and style selection rather than consuming `PageScene` geometry.
- Page zoom scales the static scene, while page-fit mode recalculates the zoom and enlarged pages have no horizontal scrolling. This makes manual zoom hard to use and does not provide a reversible coordinate transform for editing gestures.

These are implementation choices, not a requirement of pagination. Two independent layout/render paths caused the observed style, line-position, fill, and column-width differences.

## Selected approach

Use `internal/wordlayout` as the canonical layout engine for both editable page presentations. Keep one point-based layout result per document revision and draft epoch. Continuous and print layout consume the same accepted pages and differ only in their page presentation and viewport behavior.

The renderer will be split into focused responsibilities:

1. **Layout service**: consumes an immutable neutral document snapshot plus visible editor drafts and supported layout metadata. It produces page geometry, styled text fragments, table cell fills/bounds, image placements, paragraph decorations, and source-position maps. It runs off the Gio frame loop.
2. **Page presentation**: paints each page from that result. Print layout uses paper, shadow, and larger gaps. Continuous view uses the same page rectangles with compact separators. Neither presentation path re-wraps or re-measures content.
3. **Editing interaction layer**: keeps persistent Gio editors in session state. Visible editable runs register page-space hit regions derived from the same fragments. Pointer coordinates are translated through the page origin and zoom transform before mapping to run IDs and character offsets. Editors remain the source of drafts; selection/caret visuals follow the shared page geometry.
4. **Viewport and zoom**: separates fit presets from manual zoom. Zoom changes presentation scale only, not pagination geometry. Hit testing applies the inverse transform. At zoom levels larger than the viewport, horizontal and vertical scrolling expose the full page.

The existing `PageScene` source IDs and rune offsets should be extended only where needed for hit testing and selection geometry. Third-party DOCX types remain behind the format adapter; Gio widget state remains in `documentSessionState`.

## Draft and layout lifecycle

- A layout request captures document ID, committed revision, draft epoch, and page-setup key. Draft text is projected into an immutable layout snapshot without mutating the DOCX adapter.
- The background layout engine can be cancelled or superseded. The UI accepts a result only when all captured identity/version values still match.
- When editing changes text, schedule a new layout so wrapping, page breaks, table row heights, and source-position maps stay current. Do not perform shaping or pagination during layout callbacks.
- The focused `widget.Editor` remains stable while a new layout result is accepted. Reconcile its selection by run ID and rune offset; if the run no longer exists, retain the draft and report that selection could not be restored.
- Saving continues to flush drafts through the existing adapter/controller path before serialization. View-mode changes, zoom, scrolling, and page presentation never mark the document dirty.

## Zoom and pointer coordinates

- Page geometry remains in points. One named transform maps page points to screen pixels using device density and manual/fit zoom; its inverse maps pointer events back to page coordinates.
- The same transform is used for drawing glyphs, fills, borders, image handles, selection rectangles, caret positions, and click/drag hit tests.
- Fit zoom is recomputed when viewport dimensions, page setup, or page-column count changes. Manual zoom disables fit mode and remains stable across frames and page navigation until the user chooses a fit preset.
- A scrollable two-dimensional viewport is required when a page exceeds either viewport dimension. Centering is used only while the scaled page fits that axis.

## Scope and compatibility boundary

- This change guarantees parity between Glance's continuous and print-layout views for content supported by `internal/wordlayout`; it does not claim pixel-perfect parity with every Microsoft Word version or unsupported OOXML construct.
- Existing unsupported-content diagnostics and safe DOCX mutation boundaries remain in force. The renderer must not silently discard content to make the two in-app modes appear equal.
- Pagination, PDF export, and native printing continue to consume the canonical page geometry. This work must not introduce a separate PDF/print measurement path.
- Web layout may retain its distinct web-style page width, but it must use the same run styling and interactive editing layer where it displays editable document content. Its explicit web-flow geometry is not required to match print pagination.

## Acceptance criteria

- Switching between continuous and print layout preserves page count, page-local text/table/image positions, styles, cell backgrounds, paragraph rules, and text wrapping. Only page surface treatment and inter-page spacing change.
- In both modes, clicking text positions the caret correctly; typing, selecting, keyboard movement, Chinese IME composition, and table-cell text edits produce the same document draft and adapter mutation behavior.
- In both modes, supported image selection and existing image interactions target the same image IDs and page-space bounds.
- Zoom controls visibly change page and content size. Fit presets remain responsive to viewport changes; manual zoom remains unchanged on redraw and can pan to clipped page edges.
- Editing at a page boundary triggers re-layout without losing the draft or moving the caret to a different character. Stale layout results are ignored.
- Page layout and visible widget work remain responsive: shaping/pagination occurs off the Gio frame loop, and only visible pages register interactive run/object hit regions.

## Implementation notes

- Keep the implementation split across focused layout, page presentation, editing-event, and viewport files instead of growing `word_layout.go` or `word_page_view.go` into an all-purpose renderer.
