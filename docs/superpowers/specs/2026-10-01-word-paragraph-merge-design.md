# Word Paragraph Boundary Merge Design

## Goal

Add the common Word paragraph join behavior for Backspace at a paragraph start and Delete at a paragraph end, including body paragraphs and paragraphs inside one table cell.

## User-visible behavior

- With a collapsed caret at the start of the first text run, Backspace joins the paragraph with the immediately preceding paragraph in the same body or table cell.
- With a collapsed caret at the end of the last text run, Delete joins the paragraph with the immediately following paragraph in the same body or table cell.
- Selection deletion and Backspace/Delete away from those paragraph boundaries continue to use Gio's normal editor behavior. Modified shortcuts do not trigger paragraph joins.
- The left paragraph's paragraph properties win. Text run formatting, inline pictures, unsupported inline XML, and source order remain intact.
- The caret returns to the join seam. If the left paragraph has no text run at the seam (for example, it ends in an image), insert an empty editable run there so the user can continue typing at that position.
- A join is rejected without mutation when paragraphs are not direct adjacent `w:p` siblings in the same `w:body` or `w:tc`, or when paragraph structure cannot be preserved safely. It never joins across a table, cell, unsupported body block, or section-properties paragraph.
- A successful join is one undo/redo operation and survives save/reopen.

## Architecture and safety

- Expose an optional neutral paragraph-join capability from `internal/document`; keep OOXML nodes and package details inside `internal/format/docx`.
- The adapter locates the adjacent paragraph, retains the left `w:pPr`, concatenates safe ordered paragraph content, reparses the package projection, and returns a neutral caret target (paragraph ID, text-run ID, and rune offset).
- The Gio layer intercepts only unmodified boundary Backspace/Delete on the focused first/last run. It flushes pending drafts before starting the adapter mutation and restores focus/caret from the adapter result.
- All parsing and package mutation run outside the Gio layout/frame path. Failed operations preserve the current editor draft and document bytes.

## Acceptance

- Adapter tests cover previous/next joins in body paragraphs and one table cell; reject table/cell/body/unsupported-block boundaries without changing package bytes; retain paragraph/run formatting and inline image order; and verify undo, redo, and save/reopen.
- Gio router tests cover Backspace/Delete joins, caret placement at the seam, pending text flush, ordinary interior deletion, selected-text deletion, and boundary events that have no eligible adjacent paragraph.
- The external DOCX sample remains unchanged; all sample mutation checks use a temporary copy.
