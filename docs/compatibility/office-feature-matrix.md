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
| Preview and limited single-run paragraph edits | Supported, limited | `internal/format/docx/document_test.go`; pagination and complex layout are not represented. |
| Workspace snapshot and blank draft | Supported, basic | `TestSnapshotRestoresEditedDocument`, `TestNewBlankDraftContainsEditableParagraph`; package parts are preserved by the adapter. |

## JSON

| Feature | Status | Evidence / limitation |
|---|---|---|
| Plain text editing, snapshot restore and Save As | Supported, basic | `internal/format/jsondoc/document_test.go`; invalid interim text is recoverable, save requires valid JSON. No tree/collapse/format UI. |
