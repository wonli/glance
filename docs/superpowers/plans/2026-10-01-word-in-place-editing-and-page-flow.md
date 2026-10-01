# Word In-Place Editing and Page Flow Implementation Plan

Design: `docs/superpowers/specs/2026-10-01-word-in-place-editing-and-page-flow-design.md`

## Task 1: Reproduce and fix the scroll boundary regression

- Add a `gioui.org/io/input.Router` regression around the real Word flow. Use a long mixed-height document, a large downward wheel event to the end, then one upward event; assert visible list position changes immediately. Add the symmetric top boundary case.
- Diagnose the failing state before changing code. Keep `widget.List` state stable and preserve virtualization.
- Fix the boundary behavior at the Word list integration/layout boundary. Do not reset scroll position as a workaround.

## Task 2: Implement centered page and inline paragraph flow

- Add regression coverage for a centered bounded page, narrow-window sizing, inline text/image order, shared-line placement, wrapping, and preserved paragraph alignment.
- Replace per-run vertical stacking with measured inline flow. Record each child once, then replay its Gio operations at its wrapped position.
- Center the paper column, add page margins and a workspace background, and keep list items at measured heights.
- Make the centered paper width adjustable by dragging either side edge. Single-run paragraph editors fill the page text width and wrap naturally; clicking text should not paint a selected-run background.

## Task 3: Add in-place run editors and safe draft commits

- Add persistent editor-per-run state and UI tests for focus, caret placement, draft retention, async commit after blur, and saving pending edits.
- Render each editable run as its styled Gio editor in the body. Preserve formatting as run properties and keep text input responsive.
- Serialize adapter commits, coalesce later draft text by run, preserve IME composition while focused, and flush before save.

## Task 4: Add blank-end paragraph editing

- Add failing DOCX adapter tests for body paragraph insertion before `w:sectPr`, stable projection, undo/redo, and save/reopen preservation.
- Add a neutral `WordBodyEditor` capability and transactional OOXML insertion.
- Add a stable blank-end hit target; on click, insert, scroll, and focus the new run editor. Cover click/duplicate suppression and the saved typed value.

## Task 5: Verify sample and update support evidence

- Run focused adapter and desktop UI suites, relevant repository regressions, and the opt-in user sample on a temporary copy.
- Cover image rotation in the picture toolbar, OOXML round trip, and undo/redo; verify the scroll bar remains at the workspace right edge and the page stays centered after resizing.
- Review the rendered sample for centered content, inline order, scrolling, and caret editing. Confirm the original sample file hash is unchanged.
- Update `docs/office-support.md` and the compatibility matrix with only verified behavior; preserve the no-pagination caveat.

## Completion

- Review the complete diff and run the required regressions. Then create the one requested squash commit and integrate it into `main` as previously authorized.
