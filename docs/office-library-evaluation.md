# Office Library Evaluation

Status: initial implementation dependencies selected and evaluated on 2026-09-28. `go.mod`/`go.sum` are the version authority; the feature boundary and evidence are in [office-support.md](office-support.md).

## GUI dependency

Use **Gio v0.10.2**, the latest released `gioui.org` version checked on 2026-09-28, as requested. The desktop command is isolated at `cmd/desktop`. The XLSX surface is drawn directly by Gio; it does not depend on a web view or third-party Office widget.

Source checked: [gioui.org/app on pkg.go.dev](https://pkg.go.dev/gioui.org/app).

## Selection principles

1. Separate parsing/editing from rendering. A Go library that reads OOXML does not provide an Excel/Word editor UI; Gio owns interaction and drawing.
2. Prefer separate XLSX and DOCX adapters if that gives better capability and preservation. A single multi-format library is not automatically better.
3. Pin versions and record license, Go/cgo requirements, supported features, known omissions, and measured behavior in this file when a dependency is selected.
4. Prove save/reopen behavior before enabling overwrite. Read tests alone do not establish safe editing.

## Candidates to validate

### XLSX: Excelize (selected)

The implementation uses `github.com/xuri/excelize/v2 v2.11.0`. The adapter exposes bounded cell windows, common styles, merge ranges, row heights, column widths, and ordinary over-cell pictures through neutral DTOs. `internal/format/xlsx/pictures.go` uses the public picture read/add/delete surface behind the adapter; tests show that `GetPictures` returns drawing scale rounded to two decimal places and `DeletePicture(sheet, cell)` targets a cell rather than one drawing, so same-anchor pictures are read-only. The first slice does not preserve full DrawingML anchor geometry or promise exact Excel layout. Synthetic picture fixtures cover insert/replace/move/resize/delete, undo/redo, save/reopen, and unrelated content preservation. The local employee-performance sample has malformed `xl/styles.xml` (`<font>` closed by `</name>`) and is not an Excelize-readable evidence source. Formula calculation and universal unsupported-part preservation are not established.

Sources: [Excelize documentation](https://xuri.me/excelize/en/index.html), [source repository](https://github.com/xuri/excelize).

### DOCX: project-local OOXML adapter (selected for the first slice)

`internal/format/docx` uses Go ZIP/XML libraries for neutral ordered body projection and targeted text, formatting, alignment, paragraph-indentation, and inline-image operations. The Gio renderer handles paragraphs, tables, and inline raster previews; package types remain behind the adapter. Its toolbar uses a fixed-height, single-row control layout with Word-specific groups. The supplied sample round-trip verifies text and image resize edits, untouched package parts, 27 resolved images, and one dangling media relationship that stays a placeholder. Synthetic fixtures cover table-cell image rendering and editing, plus horizontal-ruler paragraph indents that preserve unrelated paragraph properties through save/reopen. This does not promise pagination, floating-image layout, or broad Word fidelity.

### Deferred third-party DOCX candidates

- **UniOffice** advertises existing DOCX read/edit operations, formatting, images, tables, and conversion features. Its project states that it is commercial and requires a license code. Consider it only if its license/price model is acceptable and corpus tests justify it.
- **godocx** and **fumiama/go-docx** describe Go APIs for reading and writing DOCX. Treat them as candidates for a limited workflow, not as evidence of broad Word fidelity. Review project activity, license, API coverage, and round-trip output for the exact fixture corpus before selection.
- **office_oxide Go binding** offers extraction/conversion APIs and limited editing methods such as text replacement and XLSX cell setting, according to its Go README. It calls a Rust core through cgo and requires the native library/header to be built or installed. Factor native build and distribution into the decision.

Sources: [UniOffice](https://github.com/unidoc/unioffice), [godocx](https://github.com/gomutex/godocx), [fumiama/go-docx](https://github.com/fumiama/go-docx), [office_oxide Go README](https://github.com/yfedoseev/office_oxide/blob/main/go/README.md).

### Legacy XLS: `extrame/xls` (read-only candidate)

The app currently uses `github.com/extrame/xls` to attempt first-sheet read-only access to BIFF `.xls`. The provided sample opens, but LibreOffice inspection shows all three sheets are empty; this is not evidence of cell-value fidelity. Do not advertise useful legacy XLS preview until a populated `.xls` corpus fixture passes. No XLS write path is enabled.

## Required evaluation corpus

Keep small, redistributable fixtures for at least:

- XLSX: strings/numbers/dates, formulas with cached results, common styles, merged cells, sparse/large used ranges, images/drawings, and unsupported package parts.
- DOCX: headings and mixed runs, tables, lists, inline images, headers/footers, hyperlinks, and unsupported/complex constructs such as fields or floating objects.
- Corrupt, truncated, encrypted/password-protected, and resource-heavy inputs where applicable.

For every candidate, record: open success/error, extracted model, visible rendering needs, edit behavior, save/reopen behavior, preservation differences, memory/time for representative files, license, cgo/native build needs, and platform build results.

## Provisional product boundary

- First XLSX editing slice: constants in ordinary cells plus ordinary over-cell picture objects with PNG/JPEG/GIF preview and safe unique-anchor edits. Formula calculation, broad formatting edits, macros, charts, pivots, in-cell images, and non-picture drawings remain out of scope.
- First DOCX editing slice: define and test a narrow text-editing operation. If preserving unrelated structure cannot be demonstrated, ship DOCX as preview-only until it can.
- `.xls` and `.doc` are separate format work; do not infer legacy-format support from `.xlsx`/`.docx` support.

Update this document after evaluation; do not leave a candidate presented as selected merely because an API example compiles.
