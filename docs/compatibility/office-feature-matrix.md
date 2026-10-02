# Glance Go Office Feature Matrix

“Supported” means this repository has focused tests for the behavior. It does not imply full Office compatibility.

## Workspace

| Feature | Status | Evidence / limitation |
|---|---|---|
| Permanent Recent tab, ordered document tabs, switching and independent close | Supported, basic | `internal/desktopui/workspace_test.go`; horizontal Gio list and per-document widget state. Dirty tabs are blocked from close until saved. |
| Recent paths and shallow folders | Supported, basic | `internal/workspace/collection_test.go`; local paths only, direct children only, unavailable recent entries are removable. |
| SQLite restart recovery | Supported, basic | `internal/workspace/store_test.go` and `collection_test.go`; versioned schema stores adapter bytes, dirty state, tab order/active tab, recents and view state. Gio's current native API does not permit cancelling window close before `DestroyEvent`. |
| Draft Save As | Supported, basic | JSON save tests and desktop extension guard tests; cancel does not call a writer. `.xlsx`, `.docx`, and `.json` extensions are validated. |

## XLSX

| Feature | Status | Evidence / limitation |
|---|---|---|
| Open and render virtualized worksheet | Supported, sample verified | `internal/format/xlsx/workbook_test.go`, `internal/desktopui/grid_*_test.go`; see `docs/office-support.md` for style/merge limits. |
| Edit constants and resize dimensions | Supported, fixture scope | `internal/desktopui/grid_edit_test.go`, `grid_resize_test.go`; saves use sibling temporary files. |
| Workspace snapshot and blank draft | Supported, basic | `TestSnapshotRestoresEditedWorkbook`; package bytes preserve the current in-memory workbook for adapter restore. |
| Formula calculation and full Excel fidelity | Unsupported | No calculation engine; common styles/merges are partial as listed in `office-support.md`. |

## DOCX

| Feature | Status | Evidence / limitation |
|---|---|---|
| Continuous-flow preview of body paragraphs, tables, styled runs, and inline pictures | Supported, limited | `internal/format/docx/word_projection_test.go`, `internal/desktopui/word_layout_test.go`; no Word pagination, headers/footers, floating-object layout, or wrap-around-image layout. |
| Word text edits, paragraph split/join, and direct run formatting | Supported, scoped | `internal/format/docx/word_text_test.go`, `internal/format/docx/word_paragraph_edit.go`, `internal/desktopui/word_toolbar_test.go`; only editable projected runs and supported direct-format properties are writable. Style/theme inheritance is incomplete; complex fields and unsupported structures remain read-only. |
| Format painter | Supported, same-document run scope | `TestWordFormatPainterCopiesSupportedTextAndParagraphFormatting`, `TestWordFormatPainterAppliesSourceFormattingToNextFocusedRun`; copies supported run properties and paragraph style/list/alignment/indent to the next editable run as one undoable change, preserves unsupported target properties, and verifies save/reopen. Unsupported numbering is rejected; sub-run selection formatting is not supported. |
| Bullet and decimal lists, including defined nested levels | Supported, common numbering subset | `TestWordParagraphListsRoundTripContinuityAndPreserveOtherPackageParts`, `TestNestedWordNumberingCanChangeLevelAndRoundTrip`, `TestNestedWordNumberingRestartsAfterParentLevel`, `TestCustomWordNumberingRestartRemainsReadOnly`; generated lists support levels 0–8 and decimal/bullet formats. Custom `lvlOverride` restarts, other numbering formats, and style-inherited numbering are read-only. Unrelated package parts are checked byte-for-byte. |
| Increase/decrease indent | Supported, context-aware | `TestWordIndentToolbarActionsStepLeftIndentAndKeepRightIndent`, `TestWordIndentToolbarMovesListThroughNestedLevels`, `TestWordParagraphIndentsRoundTripAndPreserveOtherParagraphProperties`; ordinary paragraphs change left indent by 0.5 inch, while lists move within defined `ilvl` levels. Right indent and `firstLine`/`hanging` attributes are retained. |
| Find editable Word text | Supported, run-scoped | `TestWordFindCyclesCaseInsensitivelyAcrossEditableRunsAndDrafts`, `TestWordFindToolbarOpensAndNavigatesToSelectedMatch`; searches are case-insensitive, include pending drafts, select and navigate to each match, and wrap at document end. Matches do not span runs and read-only field text is skipped. |
| Replace editable Word text | Supported, selected range in one run | `TestReplaceWordTextRangeUsesRuneOffsetsAndPreservesRunFormatting`, `TestWordReplaceReplacesSelectionAndSelectsNextMatch`; replaces the selected case-insensitive match, preserves run formatting, and navigates to the next match. Unicode rune offsets are range-checked and save/reopen is covered. Cross-run, field, image, and paragraph-boundary replacement is not supported. |
| Document information | Supported, five DOCX core properties | `internal/format/docx/word_properties_test.go`, `internal/desktopui/word_document_info_test.go`; title, subject, creator, keywords, and description are edited by a narrow package patch. Save/reopen, escaping, undo/redo, unknown-property and unrelated-part preservation, and missing relationship/content-type creation are covered. Duplicate properties and nested/non-text target markup are rejected. Custom properties and other metadata are not editable. |
| Insert page break | Supported, direct body text runs | `internal/format/docx/word_page_break_test.go`, `internal/desktopui/word_page_break_test.go`; writes a native page-type `w:br` at a Unicode caret/range, replaces selected text, preserves run formatting, and returns an editable caret run. Projection, snapshot reopen, undo/redo, and asynchronous toolbar insertion are covered. Table-cell paragraphs and runs with additional non-text children are read-only; continuous-flow preview does not paginate. |
| Save edited DOCX and reopen | Supported, fixture scope | `internal/format/docx/word_text_test.go`, `word_images_test.go`; package edits participate in undo/redo and reparse after mutation. List tests verify unrelated package part preservation. This does not establish universal preservation of all unsupported Office extensions. |
| Workspace snapshot and blank draft | Supported, basic | `TestSnapshotRestoresEditedDocument`, `TestNewBlankDraftContainsEditableParagraph`; package parts are preserved by the adapter. |

## JSON

| Feature | Status | Evidence / limitation |
|---|---|---|
| Plain text editing, snapshot restore and Save As | Supported, basic | `internal/format/jsondoc/document_test.go`; invalid interim text is recoverable, save requires valid JSON. No tree/collapse/format UI. |
