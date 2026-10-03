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

`internal/format/docx` uses Go ZIP/XML libraries for neutral ordered body projection and targeted text, formatting, alignment, paragraph-indentation, and inline-image operations. The adapter now also projects common section page geometry, style/default inheritance inputs, paragraph spacing/line rules/indents/keep flags, and table grid/cell widths as read-only neutral data. Focused fixtures verify twips/EMU/point conversion, explicit-off formatting masks, unsupported section/table diagnostics, and unchanged DOCX snapshot bytes. This is projection evidence only: it does not establish page layout, pagination, floating-image layout, or broad Word fidelity. The Gio renderer still handles body paragraphs, tables, and inline raster previews in continuous flow.

### Deferred third-party DOCX candidates

- **UniOffice** advertises existing DOCX read/edit operations, formatting, images, tables, and conversion features. Its project states that it is commercial and requires a license code. Consider it only if its license/price model is acceptable and corpus tests justify it.
- **godocx** and **fumiama/go-docx** describe Go APIs for reading and writing DOCX. Treat them as candidates for a limited workflow, not as evidence of broad Word fidelity. Review project activity, license, API coverage, and round-trip output for the exact fixture corpus before selection.
- **office_oxide Go binding** offers extraction/conversion APIs and limited editing methods such as text replacement and XLSX cell setting, according to its Go README. It calls a Rust core through cgo and requires the native library/header to be built or installed. Factor native build and distribution into the decision.

Sources: [UniOffice](https://github.com/unidoc/unioffice), [godocx](https://github.com/gomutex/godocx), [fumiama/go-docx](https://github.com/fumiama/go-docx), [office_oxide Go README](https://github.com/yfedoseev/office_oxide/blob/main/go/README.md).

### PDF generation: selected vector replay backend (integrated, visual acceptance remains open)

The controlled probe selected [`SalvioniDigitalSolutions/gopdf`](https://github.com/SalvioniDigitalSolutions/gopdf) for page boxes, vector paths, images, alpha masks, and PDF serialization. Pin: `v0.0.0-20260922202729-55a8f5fed5f5` (module checksum `h1:8D8Tk4WTPnSJTFdB172lkyBLlExzC/EysR1ECIx+QR8=`). Its module declares Go 1.22, has no third-party module requirements, and carries an MIT license. It is not a text-layout engine: `Page.Text` maps Unicode characters back through the font cmap and applies its own kerning; it has no public positioned-glyph-ID interface. Therefore visible text must not use `Page.Text` as the layout source.

Probe date: 2026-10-02. An isolated program used Go-text `v0.3.4` to resolve an outline from an actual glyph ID, replayed its quadratic/cubic segments as PDF vector paths at the shaped origin, and added an alpha-zero Unicode text layer for extraction. Results:

- Mixed `A中B` rendered visibly. The probe output was independently parsed by Poppler `pdfinfo` and rendered by Poppler `pdftoppm`; the generated page was 320 × 180 points and one page. Production package regressions now also render vector output and read text through `gopdf.Reader`; independent Poppler/PDFKit text extraction remains unverified.
- The generated probe PDF contained `/ToUnicode`; `gopdf.Reader.PageText(0)` returned exactly `A中B` for both regular text and the alpha-zero text-layer experiment. The in-project `TestWriteWordPDFExtractsMixedCJKLatinTextWhenSystemFontIsAvailable` verifies production export returns the same CJK/Latin string when `/Library/Fonts/Arial Unicode.ttf` exists on the host. The upstream module also tests embedded font ToUnicode output.
- A translucent NRGBA pixel survived PDF round trip: decoded alpha was `24672/65535` (96/255).
- `RenderPageDetail` reported 3 glyphs and 0 missing for the test; the outline replay and extraction overlay stayed aligned to the same page origin. The source font was the host-installed `/Library/Fonts/Arial Unicode.ttf`; its `OS/2.fsType` was `0x0008` (editable embedding). No font file is bundled or redistributed by this dependency decision. Production must check embedding permissions before adding the invisible searchable text layer; if a chosen font forbids embedding, export must report that limitation rather than silently embed it.
- Pure-Go probe code (`gopdf` + Go-text outline APIs) cross-built with `CGO_ENABLED=0` for macOS/arm64, Windows/amd64, and Linux/amd64. The app's Gio desktop package compiled on macOS/arm64 and Windows/amd64. Linux cross-compilation from this macOS host remains unverified because Gio excludes its Vulkan files when `CGO_ENABLED=0`; that is a host/toolchain limitation, not a gopdf limitation.

Production integration: `internal/format/pdf/word.go` replays each point-based scene's page box, vector glyph paths, highlights/underlines, table borders, and PNG/JPEG image rectangles/rotation. `Page.Text` is used only for a fully transparent Unicode search/copy layer; it does not determine visible geometry. `ExportWordPDFFile` writes a same-directory temporary file, reopens it to validate page count and each page size, then atomically replaces the target. The UI confirms layout warnings, flushes drafts, compares the exported DOCX snapshot with the accepted neutral blocks/layout projection, and checks a revision/draft epoch immediately before commit. Regression tests verify source DOCX snapshot and dirty state remain unchanged, and stale/canceled/failed work preserves an existing PDF.

Embedding is fail-closed: the writer reads `OS/2.fsType`, checks the requested TTC face, cmap coverage for each invisible text chunk, and the selected PDF backend's TrueType subsetting behavior. It rejects restricted embedding, bitmap-only embedding, missing permission tables, TrueType no-subsetting, and collection faces other than face 0. The host CJK fixture reports `fsType=0x0008`. Fonts that cannot be embedded currently block export; a nonembedded `ActualText` extraction fallback is not implemented. The in-project reader covers CJK/Latin extraction and visible vector rendering; independent Poppler/PDFKit text extraction and Pen/Glance Computer Use visual comparison remain pending. PDF integration is available for the declared layout subset but is not 1:1 acceptance evidence.

Other candidates remain unselected: `go-pdf/fpdf` is archived; `signintech/gopdf` carries a `gofpdi` dependency and its positioned-CJK path has not passed a project probe. Source review is not acceptance evidence.

### macOS print service: PDFKit/AppKit adapter implemented

`internal/printservice/print_darwin.go` and `print_darwin.m` accept in-memory PDF bytes, validate page count/options, copy the input for the Objective-C call, create a PDFKit document, and use AppKit's native print operation with page scaling disabled and auto-rotation off. Panel creation is dispatched to Cocoa's main queue so the Gio window-loop goroutine can continue polling job completion. `darwin/arm64` package tests/build pass. Computer Use opened the native panel and showed Page 1 of 1 and Copies 1, then canceled. In the current visual pass the bindable QA window displayed a blank DOCX (1 page, 0 words), not the Pen sample; the current `go run ./cmd/desktop` process itself could not be bound by CUA. Therefore this is evidence for the native panel path only, not matched-document page fidelity. The machine had no selected printer, so successful submission, virtual-printer copies/ranges, and paper dimensions remain unverified. Windows, Linux, and no-cgo builds return unavailable and keep Print disabled. The app does not start LibreOffice, a PDF viewer, or a command-line print program.

Sources: [PDFKit `PDFDocument(data:)`](https://developer.apple.com/documentation/pdfkit/pdfdocument/init%28data%3A%29), [PDFKit print scaling / print operation](https://developer.apple.com/documentation/pdfkit/pdfprintscalingmode), [AppKit `NSPrintOperation`](https://developer.apple.com/documentation/appkit/nsprintoperation/).

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
