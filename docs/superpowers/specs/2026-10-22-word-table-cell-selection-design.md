# Word Table Cell Selection and Editing Design

**Status:** The four design sections and this consolidated specification were approved by the user on 2026-10-22. The implementation plan is saved separately and awaits review.

## Goal

Fix Word table interaction so clicking a cell does not change the horizontal ruler's overall width, and users can select rectangular ranges of cells with a mouse drag or Shift-click. Copying a selected range should produce useful tab-separated text. Delete should clear selected cell contents, then remove the table only when the entire table was already empty before the Delete action.

The behavior applies to Web Layout, Continuous Layout, and Print Layout. Normal text editing, hyperlink navigation, image interaction, and the existing page/document architecture must continue to work.

## Current constraints

- Web Layout renders tables as flow rows and cells; Continuous and Print Layout render page-scene tables. Their pointer geometry and event paths differ.
- The neutral Word document model exposes table rows, cells, `GridSpan`, vertical merge state, paragraphs, and inline text/images/unsupported content. It has no table-range selection state or table-cell editing capability.
- DOCX cell IDs are projected from table and source row/cell positions. The DOCX adapter owns the XML and history; UI code must pass neutral IDs and must not depend on XML nodes or third-party Office types.
- Logical grid spans and supported vertical merges are represented. Legacy horizontal `hMerge` is not reliably represented by the current neutral model and is reported as unsupported for layout.
- The horizontal ruler currently derives a cell-specific text area in some caret contexts, which can make the ruler appear to shrink when the caret enters a table.

## Design

### 1. Independent cell-range selection

Store an optional `WordTableSelection` in the active document session. It identifies the document, table, anchor cell, and focus cell. The selected cell IDs are resolved from the neutral table snapshot; the range state is separate from the active text run, editor selection, and caret.

All three Word views expose cell hit geometry and map pointer positions to the same source cell IDs. A normal click inside text continues to place the text caret. A text drag remains a text selection while it stays within one cell; when it crosses a cell boundary it becomes a rectangular cell selection. Shift-click extends the current range from its anchor. Clicking a different table starts a new range there. Escape and clicking outside a table clear the cell range.

The range is drawn as an independent cell highlight in each view. Creating or extending it does not move the text caret. Selection is cleared when its document/table/cell IDs become stale, including after structural edits or switching documents.

### 2. Copy behavior

Cmd+C on macOS and Ctrl+C on other platforms checks for an active cell range before using the existing text-selection copy path. The selected logical range is serialized as plain TSV:

- Columns are separated by tabs and rows by newlines.
- Paragraphs within one cell are joined by newlines.
- Hyperlinks contribute their displayed text.
- Inline images use the literal placeholder `[图片]`.
- For a merged region, the owning cell's content is emitted once and covered grid positions are empty.
- The current unsaved text draft for a cell is used when present.

If the selected visual range cannot be mapped to source cells without ambiguity, copy fails without changing the document or clipboard. Existing text selection copying remains unchanged when no cell range is active.

### 3. Delete behavior and document capability

Add an optional, format-neutral Word table selection editor capability to `internal/document`. The UI/controller passes a table ID and selected cell IDs; the DOCX adapter validates all IDs and performs one atomic mutation. It inspects the whole table at the time of the Delete command:

- If any table cell contains content, clear the contents of the selected cells while preserving table geometry, cell properties, and paragraph formatting. Empty paragraph structure remains so the cells stay editable.
- If every cell in the table was empty before the command, remove the entire table, even if the active selection covers only part of it.
- If the table has content but the selected cells are already empty, the command is a no-op.

For the whole-table emptiness check, nonempty text, images/drawings, visible tabs or breaks, field content, and other content-bearing payloads count as content. Empty paragraphs and formatting/property nodes do not. Pending text drafts are flushed through the existing edit flow before the operation so the decision uses the latest document contents.

Clearing selected cells or deleting an empty table is one undoable document operation. The adapter validates the entire request before writing; stale IDs or unsupported structure that prevents an exact safe edit fail atomically. Adapter work and document parsing remain outside Gio layout/frame code. After a successful mutation the view refreshes from the document snapshot and clears any now-stale selection/caret references.

### 4. Ruler and keyboard/pointer precedence

The ruler's main track width remains based on the page text area in table and non-table contexts. Paragraph indent markers may still use the active cell's paragraph geometry, but the cell must not redefine the ruler's overall width. Cell-range selection does not change the text caret or paragraph context used by the ruler.

The active cell range takes precedence for Cmd/Ctrl+C and Delete. Without a cell range, the current text selection and editor behavior remain in force. Plain clicks on text and hyperlinks retain their existing caret/hover behavior; Cmd/Ctrl-click on a hyperlink retains navigation behavior. A drag is promoted to cell selection only after it crosses into another cell. Pointer coordinates are normalized at each input boundary before hit testing.

### 5. Merged cells and fail-closed cases

Selection and serialization use the table's logical grid. The neutral table model carries a flag for legacy `hMerge` so all views can reject ambiguous ranges without inspecting DOCX XML. A range that intersects a supported `GridSpan` or vertical merge expands to include the full merged region. The owner cell is represented once; continuation/covered coordinates do not duplicate the cell ID or its copied text.

When legacy `hMerge`, malformed merge geometry, stale IDs, or another unsupported structure prevents a trustworthy mapping from the visible range to source cells, copy or Delete does not guess. It returns an actionable UI error and leaves document contents unchanged. Ordinary supported tables and valid grid-span/vertical-merge tables remain editable.

## Alternatives considered

1. **Reuse text selection and text-run batch replacement.** This does not model rectangular cell selection, handles mixed inline objects poorly, and makes table deletion/undo semantics awkward.
2. **Add a dedicated neutral table-selection capability and session state (chosen).** This separates UI selection from text caret state, keeps Office XML in the adapter, and provides an atomic history boundary.
3. **Mutate cells individually from the UI.** This risks partial edits, stale IDs, blocking frame work, and multiple undo steps.

## Scope boundaries

- This change does not add an insertion dialog for hyperlinks or alter existing hyperlink styling/navigation.
- It does not add table row/column insertion, table resizing, formatting commands, or cross-table range selection.
- Legacy `hMerge` and malformed merge structures remain unsupported for cell-range copy/delete.
- This change does not claim full Microsoft Word layout or DOCX compatibility.

## Acceptance behavior

- In each of Web, Continuous, and Print Layout, clicking inside a table leaves the ruler's overall width stable.
- A click in cell text positions the text caret. Dragging across cells and Shift-click create a visible rectangular cell range without moving that caret.
- Cmd/Ctrl+C produces the specified TSV output for text, multi-paragraph cells, hyperlinks, images, and supported merged ranges; ordinary text copy still works with no cell range.
- Delete in a nonempty table clears only selected cell contents and preserves the table/cell geometry. Delete again when the entire table is empty removes the table. Undo restores each table operation in one step.
- Empty selected cells in a table containing other content produce no document mutation. Unsupported or stale cell mappings fail without partial changes.
- Hyperlink Cmd/Ctrl-click, regular text selection, and existing image interactions continue to work.

## Compatibility notes

This adds a neutral document capability and transient UI selection state, plus a DOCX adapter implementation. It does not change third-party Office dependencies or file save/overwrite policy. The hyperlink underline edits already present in `internal/desktopui/word_inline_layout.go` and `internal/desktopui/word_paragraph_layout.go` belong to the earlier hyperlink rendering task and are outside this design.
