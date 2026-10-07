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

#### Password-to-open encryption: no dependency selected

Inspected the exact pinned `github.com/xuri/excelize/v2 v2.11.0` implementation on 2026-10-03. `Encrypt` calls `standardKeyEncryption` and encrypts package blocks with AES-ECB; Standard encryption has no package-integrity HMAC. `Decrypt` recognizes Agile encryption, but its Agile path decrypts the package key and package without validating the encrypted password verifier or `dataIntegrity` HMAC. Do not route DOCX reads or writes through these APIs as a security feature.

Audited the exact `github.com/mgilbir/spine v0.3.1` tag (commit `4a39ee030412a9b04fa2b8439610b6cdce0eaa1a`) from source on 2026-10-04. Spine is pre-1.0, declares Go 1.25.0, and is MIT licensed; the project currently declares Go 1.27.0. Its `common/crypto.Encrypt` writes Agile AES-256-CBC/SHA-512 with `dataIntegrity`; strict `DecryptWithOptions` rejects missing/partial integrity data and HMAC mismatch, caps attacker-controlled `spinCount`, and returns both the detected scheme and an `IntegrityVerified` result. The isolated upstream package checks passed with `go test ./common/crypto ./opc -count=1` using a temporary module/cache, without adding Spine to this project. The optional upstream `msoffcrypto-tool` cross-validation was unavailable, and no Office-generated DOCX corpus was tested.

Do not use Spine's `opc.OpenReader` as the security-sensitive decrypt path: its encrypted-container helper accepts Standard/RC4 as well as Agile and discards the crypto result's scheme/integrity fields before returning a generic reader. A future adapter must inspect CFB streams and call `common/crypto.DecryptWithOptions` directly, then require Agile plus `IntegrityVerified == true`; the candidate CFB reader is `github.com/richardlehane/mscfb v1.0.9` (Apache-2.0, Go 1.18, with `msoleps v1.0.3`). Its source checks sector ranges and chain cycles and caps some table allocations at 64,000 entries. However, `mscfb.New` accepts an `io.ReaderAt` without a total-input limit, and directory traversal recursively follows untrusted links; an application-enforced file/stream/entry budget and adversarial depth tests are still needed. Required CFB streams must be selected unambiguously from that same parsed container before passing their bytes to Spine crypto. Spine's `opc.SaveEncrypted` writes Agile output and may be evaluated for the write side independently. Neither package requires LibreOffice or an Office process at runtime. No encryption dependency is selected until the CFB boundary, limits, strict read path, and Office interoperability are validated.

The integration boundary is broader than the file codec: `snapshotState` stores the full DOCX ZIP snapshot in the local SQLite workspace. A complete implementation must keep that recovery payload encrypted, re-prompt for the password on restore, and preserve encryption across Save, Save As, AutoSave, and Share. Do not enable File > Protect Document until the CFB/dependency boundary is reviewed and strict tamper, wrong-password, bounded-resource, and Office-compatible save/open checks pass across those persistence paths.

Sources: [Microsoft Agile data encryption](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-offcrypto/9e61da63-8ddb-4c0a-b25d-f85d990f44c8), [Microsoft Agile integrity generation](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-offcrypto/63d9c262-82b9-4fa3-a06d-d087b93e3b00), [Excelize v2.11.0 source](https://github.com/qax-os/excelize/blob/v2.11.0/crypt.go), [Spine v0.3.1 crypto API](https://pkg.go.dev/github.com/mgilbir/spine@v0.3.1/common/crypto), [Spine v0.3.1 OPC API](https://pkg.go.dev/github.com/mgilbir/spine@v0.3.1/opc), [mscfb v1.0.9](https://pkg.go.dev/github.com/richardlehane/mscfb@v1.0.9).

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

### Markdown: selected source parser and local math renderer

`github.com/yuin/goldmark v1.7.13` (MIT) is selected behind the neutral Markdown adapter. GFM and the built-in Footnote extension cover table alignment, task checkboxes, emphasis/deletion, links/linked images, nested lists/quotes and standard named footnotes. Literal code, punctuation escapes, entities, escaped dollars and currency have dedicated regressions. Source is retained independently of the AST, including BOM and CRLF; heading offsets use a linear source-line index. Parser objects never enter Gio state. See [Goldmark's primary documentation](https://github.com/yuin/goldmark).

`github.com/alecthomas/chroma/v2 v2.27.0` (MIT; Go 1.25 minimum; pure Go) supplies fenced-code syntax tokens behind the Markdown adapter. A fixed light palette becomes neutral literal code spans; lexer/token types never enter UI state. C# (`c#` alias), CSS, JavaScript and PHP snippets without an opening `<?php` have keyword-colour and exact-source regressions, including BOM, CRLF and Chinese. Unknown or absent language labels remain plain code. Highlighting accepts at most 1 MiB and 100,000 tokens, checks a 250 ms work budget between iterator calls, and relies on Chroma's 250 ms timeout for each regex match; this is not a hard total wall-clock deadline. Any limit, lexer failure or source mismatch returns the entire original text without partial highlighting. Tokenisation does not interpret Markdown links, images or formulas. Sources: [Chroma primary documentation](https://github.com/alecthomas/chroma), [v2.27.0 lexers](https://pkg.go.dev/github.com/alecthomas/chroma/v2@v2.27.0/lexers).

`codeberg.org/go-latex/latex v0.3.0` (BSD-3-Clause) supplies local math glyph rendering. The earlier GitHub project is archived; the maintained Codeberg module is selected. Evaluation found that the upstream renderer does not implement all script AST forms needed by the approved fixture corpus. A bounded adapter composition layer handles superscripts, subscripts, fractions and roots, with serialized upstream access and explicit diagnostics. Tests cover Greek/operator glyphs, scripts, fractions, roots, malformed expressions and unsupported commands. Accepted expression length is 4096 bytes, nesting depth 24, and output dimensions at most 8192 × 4096 pixels. These limits and tests support only the documented subset, not full TeX. See [module documentation](https://pkg.go.dev/codeberg.org/go-latex/latex).

PDF uses the already selected Word PDF/font/layout adapters. Markdown PDF regressions cover Chinese searchable text and images/formulas both in body and table cells. Unsupported resources fail before output replacement; a revision/draft/session guard prevents stale results from replacing an existing destination. Gio captures and the representative PDF were visually inspected on macOS arm64.

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

Markdown resource acceptance now includes bounded HTTP/HTTPS raster loading with redirect/status/byte/pixel validation, cancellation, deduplication and four-worker concurrency regressions. Formula UI dimensions scale with screen metrics; PDF uses 0.5 points per 144-DPI raster pixel.


### PDF workspace adapter — 2026-10-07

Reuses the already pinned `github.com/SalvioniDigitalSolutions/gopdf` at `v0.0.0-20260922202729-55a8f5fed5f5`; no new PDF dependency. Third-party reader/update/render types stay inside `internal/format/pdfdoc`; UI consumes neutral `document.PDFPage`, `PDFInfo` and `PDFCommand`.

The canonical state is a validated PDF package revision. Commands apply an incremental update, serialize and reopen before publication. Rendering uses an immutable package snapshot; raster pixels and hit data carry the same revision. Tests cover text edit/no-op/stale command, notes/highlights/ink, rotation, save/reopen, unchanged second page, existing notes, undo/redo and failed-save dirty preservation. UI tests cover shared model identity, recovery, stale scenes and visible canvas pixels at two window sizes.

API availability is not a compatibility guarantee. Current text-fragment API does not expose every graphics-state transform/opacity needed for safe arbitrary editing. Consequently pages with ExtGState/XObject resources, invisible text and rotated text are restricted; signed PDFs reject mutations, and encrypted/repaired files reject opening. Text replacement depends on the original font encoding and does not reconstruct paragraphs. Native printing is integrated but not hardware-verified. See `office-support.md` for the maintained boundary.

Native render fallback: macOS/cgo uses CoreGraphics/CoreFoundation when the pure-Go renderer reports missing glyphs or an error. The renderer receives in-memory bytes of the accepted model revision, never reopens the source path. Annotation appearances are composited from a temporary, unsaved rendering projection of that same revision. Geometry regression checks 1× and 2× placement. This is a platform rendering backend, not a separate editing model; non-macOS builds retain the Go renderer and diagnostics. The local SimSun-embedded Chinese PDF exposed the need for this fallback.
