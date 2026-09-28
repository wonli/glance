# Office Preview and Editing Status

This status describes the current desktop implementation. A dependency's advertised feature set is not counted as an application feature until the UI and round-trip path are wired and verified.

## Current desktop command

Run `go run ./cmd/desktop [path]` from the repository root. Gio renders the window directly. The toolbar accepts a path or opens the platform file picker.

## XLSX

| Feature | Status | Current boundary |
|---|---|---|
| Open workbook and switch worksheets | Implemented | `.xlsx` only. |
| Preview cell values | Implemented, virtualized | Full legal XLSX address extent (1,048,576 rows × 16,384 columns), rendered from the visible window with two rows/columns of overscan. Horizontal and vertical scrolling are supported; blank cells outside the used range can be selected. Opens and range reads run off the Gio frame loop. Scrolling replays bounded cached cell paint operations using full cell bounds, so clipping at the viewport edge does not change text layout; cached operations are rebuilt when value, style, size, or display scale changes. Gio still composites each frame. |
| Cell styles | Partial | Common fill/font colors, bold, italic, horizontal alignment, wrapping, and simple cell borders render from neutral style data. Theme/indexed colors fall back to defaults; number formats, rich text, conditional formatting, and Excel-perfect typography are not rendered. |
| Merged cells | Partial | Visible merge ranges render as the anchor cell across rows/columns, clipped to the viewport. Clicking an interior selects the anchor. |
| Edit a cell constant | Implemented, basic | Click selects; double-click or F2 opens an editor over the cell. The editor retains the rendered font color, weight, style, fill, alignment, and wrapping while keeping the same cell bounds. Enter commits, Escape cancels. The name box and formula/value bar track the active cell. Text, finite numbers, booleans, and blanks are supported; formula-like input is rejected. |
| Save edited workbook | Implemented, sample verified | Background save through a sibling temporary file then replacement. Adapter tests reopen synthetic and `~/Downloads/office/人事表格-员工绩效表.xlsx` copies after cell edits and dimension changes; representative untouched values, styles, merges, and formula data survive, and the source sample remains unchanged. This is not a universal Office-part preservation guarantee. |
| Formula calculation and broad formatting edits | Not implemented | Cached values may display through Excelize; formula calculation and full Excel layout are not promised. |
| Resize row heights / column widths | Implemented, fixture scope | Drag a row/column heading boundary. Preview geometry and cursor remain active during drag; cancellation or write failure restores prior dimensions. Only release writes dimensions. Pixels convert to points/Excel character widths; XLSX limits are enforced. |
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
GOCACHE=/tmp/glance-go-gocache GLANCE_OFFICE_TEST_DIR="$HOME/Downloads/office" go test ./...
```

The tests open and save copies of `.xlsx`/`.docx` samples, reopen the copies, and leave originals untouched. The `.xls` sample is opened read-only. `.ppt` and PDF files are outside this first desktop slice.
