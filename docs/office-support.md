# Office Preview and Editing Status

This status describes the current desktop implementation. A dependency's advertised feature set is not counted as an application feature until the UI and round-trip path are wired and verified.

## Current desktop command

Run `go run ./cmd/desktop [path]` from the repository root. Gio renders the window directly. The toolbar accepts a path or opens the platform file picker.

## XLSX

| Feature | Status | Current boundary |
|---|---|---|
| Open workbook and switch worksheets | Implemented | `.xlsx` only. |
| Preview cell values | Implemented, basic | First 300 rows and 16 columns; no horizontal scrolling yet. Style IDs receive a subtle display treatment; complete Excel styling is not rendered. |
| Edit a cell constant | Implemented, basic | Text, finite numbers, booleans, and blank values. Formula-like input is rejected. Select a cell, edit in the footer editor, apply, then Save. |
| Save edited workbook | Implemented, sample verified | Sibling temporary file then replacement. Unit round-trip checks value/style; local sample tests reopen saved copies and compare bounded cell previews. This is not a universal Office-part preservation guarantee. |
| Formula calculation, formatting UI, merged-cell interaction | Not implemented | Cached values may display through Excelize; calculation and full layout are not promised. |
| Resize row heights / column widths | Not implemented | When added, reuse `../mysql-ai/docs/drag-patterns.md` for coordinate normalization, stable preview, edge extension, cursor coverage, and release-time commit. |
| Legacy `.xls` | Read-only, limited | Opens the first worksheet only. The supplied `.xls` sample contains three empty sheets, so it verifies package opening but not cell-value fidelity. Editing/saving is disabled. |

## DOCX

| Feature | Status | Current boundary |
|---|---|---|
| Preview paragraphs and tables | Implemented, basic | Plain text block layout; no Word pagination, typography fidelity, headers/footers, or image rendering. |
| Edit paragraph text | Implemented, limited | Only paragraphs represented by one `w:t` text run are editable so the run's existing formatting can be retained. Multi-run paragraphs are read-only. |
| Save edited document | Implemented, sample verified | The package is rewritten through a temporary sibling file. The sample regression verifies that every package part except `word/document.xml` is byte-for-byte unchanged. |

## Regression with the local sample folder

To include the user's local files without checking them into the repository:

```sh
GLANCE_OFFICE_TEST_DIR="$HOME/Downloads/office" go test ./internal/format/...
```

The tests open and save copies of `.xlsx`/`.docx` samples, reopen the copies, and leave originals untouched. The `.xls` sample is opened read-only. `.ppt` and PDF files are outside this first desktop slice.
