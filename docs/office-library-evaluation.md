# Office Library Evaluation

Status: candidate guidance, not a final dependency lock. Select libraries only after running the project's representative-file and round-trip corpus.

## GUI dependency

Use **Gio v0.10.2**, the latest released `gioui.org` version checked on 2026-09-28. The desktop command is isolated at `cmd/desktop` so the existing server CLI and its cross-compilation settings do not need to import the native UI package. Lock updates in `go.mod` and `go.sum` and rebuild the desktop target when upgrading Gio.

Source checked: [gioui.org/app on pkg.go.dev](https://pkg.go.dev/gioui.org/app).

## Selection principles

1. Separate parsing/editing from rendering. A Go library that reads OOXML does not provide an Excel/Word editor UI; Gio owns interaction and drawing.
2. Prefer separate XLSX and DOCX adapters if that gives better capability and preservation. A single multi-format library is not automatically better.
3. Pin versions and record license, Go/cgo requirements, supported features, known omissions, and measured behavior in this file when a dependency is selected.
4. Prove save/reopen behavior before enabling overwrite. Read tests alone do not establish safe editing.

## Candidates to validate

### XLSX: Excelize

Excelize is the leading initial candidate for Go-native XLSX read/write and cell/style operations. Its official documentation describes OOXML spreadsheet read/write support and streaming APIs. Evaluate the exact operations needed for bounded ranges, displayed values, formulas/cached values, styles, merged regions, and preservation on save.

Sources: [Excelize documentation](https://xuri.me/excelize/en/index.html), [source repository](https://github.com/xuri/excelize).

### DOCX: compare based on preservation and license

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

- First XLSX editing slice: constants in ordinary cells; formula calculation, broad formatting edits, range paste, macros, charts, and pivots remain out of scope.
- First DOCX editing slice: define and test a narrow text-editing operation. If preserving unrelated structure cannot be demonstrated, ship DOCX as preview-only until it can.
- `.xls` and `.doc` are separate format work; do not infer legacy-format support from `.xlsx`/`.docx` support.

Update this document after evaluation; do not leave a candidate presented as selected merely because an API example compiles.
