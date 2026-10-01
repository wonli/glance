# Word In-Place Editing and Page Flow Design

## Goal

Correct the first five Word editing regressions: scrolling remains reversible at both ends, clicking text edits it in place with a caret, clicking the blank body area at the document end creates an editable paragraph, inline content wraps in source order, and the paper column is centered in the window.

## User-visible behavior

- DOCX body paragraphs and table-cell paragraphs lay out text runs and inline images in their source order. Runs share a line where space permits and wrap at run boundaries; long runs wrap within the page text column. Paragraph alignment remains respected.
- A4-like white paper is centered on a neutral workspace background, with readable page margins and a bounded text column. The view remains a continuous flow; this change does not claim Word-compatible pagination or print fidelity.
- Text runs use persistent Gio editors at their rendered positions. Clicking a run positions the caret there. Text remains a draft in the editor while focused, then commits through the existing async adapter path after focus moves. Composition text is not committed while the editor retains focus. Save flushes pending text drafts before writing.
- The blank body area after the final block is an insertion target. Clicking it inserts an editable empty body paragraph through an optional neutral adapter capability, scrolls to it, and focuses its editor.
- The vertical flow list is tested with routed large downward and upward wheel events at the bottom boundary, plus reverse scrolling at the top. List state and measured block heights must not trap the user at either boundary.

## Architecture and safety

- Keep page geometry, editor widgets, draft text, focus identity, and insertion state in `documentSessionState`.
- Use a small paragraph inline-flow layout helper that measures each neutral inline item, wraps at the page text width, then places recorded Gio operations on shared lines. Keep adapter IDs and DTOs format-neutral.
- Add `document.WordBodyEditor.InsertBodyParagraph()`. The DOCX adapter inserts a minimal editable paragraph before `w:sectPr`, reparses transactionally, and returns the new paragraph ID. It changes only `word/document.xml` and records an undo entry.
- Text drafts are committed asynchronously and serially. The latest draft for each run wins. A save requested during a flush starts only after all pending drafts succeed; a failed edit keeps its draft and reports the error.
- Add the virtual blank-end target as a final list item. It must retain stable widget identity and must not alter the position of existing body blocks.
- Do not modify the user's source DOCX during acceptance. Use fixture documents and a temporary sibling copy of `大猫OA软件使用说明.docx`.

## Acceptance

- A Gio router regression scrolls a long document to its end with a large wheel event, then moves upward with one reverse event. The visible `Position.First` or `Position.Offset` moves immediately. The symmetric top-to-down check also passes.
- A mixed text-image-text paragraph lays out in the original order with adjacent items on shared lines when there is room and wraps without overlap when the line fills.
- A click on an in-place text editor gives it focus and preserves a non-end caret position. Typing changes the editor draft; focus loss commits it to the DOCX projection; save/reopen retains it.
- Clicking the end-of-body blank target adds one editable paragraph, focuses its empty run editor, and save/reopen retains the paragraph and typed text.
- A visual/layout regression shows a centered paper column narrower than a wide window and full-width on a narrow window. Table-cell content keeps its boundaries.
- Existing image rendering, edit operations, Excel toolbar behavior, and unsupported XML preservation continue to pass their relevant tests.

## Known limit after this increment

This increment improves continuous document flow but does not implement Word's page breaking, headers/footers, floating-image wrapping, or full Office typography. Support documentation must list these as remaining gaps rather than imply native pagination.
