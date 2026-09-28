# Office Preview and Editing Status

This status describes the current desktop implementation. A dependency's advertised feature set is not counted as an application feature until the UI and round-trip path are wired and verified. Multi-document workspace behavior is documented below; a feature's workspace persistence does not imply stronger Office format fidelity.

## Current desktop command

Run `go run ./cmd/desktop [path]` from the repository root. Gio renders the window directly. Use the `+` tab menu to open a file or folder, create a draft, or save/Save As.

## Workspace

| Feature | Status | Current boundary |
|---|---|---|
| Permanent Recent tab and document tabs | Implemented | Gio-native horizontal `widget.List` at the top edge of the full-height content area; tab title and close control share one tab surface, and `+` follows the final tab. Tabs retain their `documentSessionState` including editor and scroll widgets while inactive. Dirty tabs cannot be closed until saved. |
| Recent files and shallow folder listing | Implemented, basic | Local paths and supported direct children are saved to SQLite; missing recent files can be removed. Folder scan is non-recursive. |
| Drafts and Save As | Implemented, basic | Blank XLSX/DOCX/JSON drafts are recoverable. Save As validates `.xlsx`, `.docx`, or `.json`; cancellation leaves the draft intact. |
| Restart recovery | Implemented, basic | SQLite stores self-contained adapter snapshots, dirty state, tab order/active tab, and view state. Missing original paths keep the snapshot and show an unavailable-source status. Gio does not expose a cancellable pre-close event, so closing the window cannot prompt before OS shutdown. |

Workspace snapshot serialization and SQLite writes run through a single background writer so Gio frame layout stays responsive. The writer drains queued snapshots on shutdown. Snapshot serialization is performed by the format adapter and still needs continued stress testing with large workbooks and concurrent editing.

## JSON

| Feature | Status | Current boundary |
|---|---|---|
| Open and edit JSON text | Implemented, basic | Text editor preserves invalid intermediate text in recovery snapshots; Save/Save As rejects invalid JSON and preserves dirty state. No tree view or formatting controls. |

## XLSX

| Feature | Status | Current boundary |
|---|---|---|
| Open workbook and switch worksheets | Implemented | `.xlsx` only. |
| Preview cell values | Implemented, virtualized | Full legal XLSX address extent (1,048,576 rows × 16,384 columns), rendered from the visible window with two rows/columns of overscan. Horizontal and vertical scrolling are supported; blank cells outside the used range can be selected. Opens and range reads run off the Gio frame loop. Scrolling replays bounded cached cell paint operations using full cell bounds, so clipping at the viewport edge does not change text layout; cached operations are rebuilt when value, style, size, or display scale changes. Gio still composites each frame. |
| Cell styles | Partial | Common fill/font colors, workbook font family, bold, italic, horizontal/vertical alignment, wrapping, and simple cell borders render from neutral style data. Font family names are passed to Gio so system font fallback can select a CJK-capable face for bold text. Theme/indexed colors fall back to defaults; number formats, rich text, conditional formatting, and Excel-perfect typography are not rendered. |
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
