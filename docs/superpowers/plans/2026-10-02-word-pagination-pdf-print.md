# Word Pagination, PDF, and Print Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a real in-process Word pagination result shared by the page view, status bar, PDF export, and native print flow, with the Pen `Clpgm`, `kiBim`, and `FsQrk` UI as the visual acceptance reference.

**Architecture:** Add neutral page/style metadata to the Word model and DOCX adapter, then paginate immutable inputs in a Gio/DOCX/PDF-independent `internal/wordlayout` package. The UI paints and edits from those positioned fragments; an in-process Go PDF backend and per-platform native print adapters consume the same result. Unsupported layout constructs return structured diagnostics and block output when they could cause missing content or incorrect pagination.

**Tech Stack:** Go, Gio, existing neutral `internal/document` types, project-local DOCX ZIP/XML adapter, a Go PDF library selected by a required CJK/positioning probe, and platform-native print APIs. Runtime must not require LibreOffice or another Office program.

**Spec:** `docs/superpowers/specs/2026-10-02-word-pagination-pdf-print-design.md`

## Global Constraints

- Glance is a standalone desktop app: pagination, page preview, and PDF output run in the Go process; do not start LibreOffice or another Office program.
- Keep the desktop entry point `go run ./cmd/desktop` and the built application display name `desktop`.
- Use one pagination, wrapping, font-substitution, and image-position result for screen, PDF, and printing.
- Compile the selected PDF writer into the Go application; do not require a separately installed program or runtime download.
- Printing calls only operating-system print APIs/services. Never use `exec` to start LibreOffice, a browser, a PDF reader, or a command-line print program.
- Keep third-party Office, PDF, and platform print types behind adapters; neutral document/layout packages must not depend on Gio widgets, DOCX XML/ZIP types, PDF types, or platform APIs.
- Layout reads committed edits and immutable snapshots only. Preserve unknown OOXML parts; report unsupported content explicitly and block output where it could be lost or paginate incorrectly.
- MVP support is one section with common page geometry, supported paragraph/run/list/page-break/inline PNG/JPEG content, and ordinary tables; do not claim universal Microsoft Word pixel fidelity.
- Keep pagination, font shaping, PDF writing, and file I/O off Gio frame/layout code; apply asynchronous results only to their matching document ID, revision, and page setup.
- A platform print entry is enabled only after that platform adapter passes its native-print verification. PDF export remains independent of print support.

## Review Focus

- CJK mixed with Latin text and a missing glyph must wrap without losing/duplicating Unicode characters and must record the selected fallback font (Task 2 shaper regression).
- Content ending exactly at a page boundary, followed by an explicit page break, must not create an empty/extra page or place text outside the page (Task 3 pagination boundary regression).
- A table row taller than the printable area must produce a located overflow diagnostic and terminate layout without negative geometry or an infinite retry (Task 3 tall-row regression).
- Multiple section settings or nested/merged unsupported table content must be identified by source location and block PDF/print when pagination would be wrong (Task 1 projection and Task 3 blocking-diagnostic regressions).
- A document edited while an old layout/export is pending, or an export canceled/failing during replacement, must not apply stale page state, change the source DOCX, damage an existing PDF, or clear dirty state (Tasks 4 and 5 stale-result and atomic-write regressions).

---

### Task 1: Project neutral page geometry and effective styles from DOCX

**Files:**
- Create: `internal/document/word_layout.go`
- Modify: `internal/document/word.go`
- Modify: `internal/format/docx/word_projection.go`
- Create: `internal/format/docx/word_layout_projection.go`
- Test: `internal/format/docx/word_layout_projection_test.go`
- Modify: `docs/word-ribbon-todo-zh.md`, `docs/office-support.md`, `docs/office-library-evaluation.md`, and `docs/compatibility/office-feature-matrix.md`

**Interfaces:**
- Produces `document.WordLayoutReader` with `WordLayoutData() document.WordLayoutData`.
- `WordLayoutData` carries `Sections []WordSection`, `Styles []WordStyle`, and `Unsupported []WordLayoutUnsupported`; a section carries `StartBlock int` and a `WordPageSetup` with width, height, and four margins in points. Paragraph additions carry before/after spacing, line-spacing mode/value, first-line indent, and keep-with-next/keep-together flags. Table additions carry source grid/cell widths in EMU.
- Style entries carry style ID, `BasedOn` ID, and neutral paragraph/text properties. Keep source OOXML names and nodes inside `internal/format/docx`.

- [x] **Step 1: Add projection fixture tests**

Add `TestWordLayoutProjectionReadsSectionPageSetupAndStyleDefaults`, asserting point conversions, orientation, body/paragraph section boundaries, `docDefaults`, `basedOn` inheritance inputs, paragraph spacing/keep flags, and table grid widths. Add `TestWordLayoutProjectionReportsUnsupportedSectionsAndTableStructures`, asserting source IDs are returned for multiple/different sections and merged/nested tables without modifying the DOCX snapshot.

- [x] **Step 2: Run the new focused tests and confirm the missing capability**

Run: `go test ./internal/format/docx -run 'TestWordLayoutProjection' -count=1`
Expected: FAIL because the Word layout metadata capability and projection are not implemented.

- [x] **Step 3: Define the neutral layout metadata types**

Add `WordPageSetup`, `WordSection`, `WordStyle`, `WordLayoutUnsupported`, `WordLayoutData`, `WordParagraphLayout`, and `WordTableLayout` in `internal/document/word_layout.go`. Extend `WordParagraph`/`WordTable` with only the neutral layout fields used by the spec. Implement `WordLayoutReader` as an optional capability so other Word readers are not forced to provide it.

- [x] **Step 4: Implement DOCX page/style projection**

Implement `(*Document).WordLayoutData() document.WordLayoutData` and projection helpers in `word_layout_projection.go`. Read `sectPr` geometry and `styles.xml`/`docDefaults`; convert twips to points and preserve section starts. Mark unsupported settings/structures with stable source IDs instead of guessing. Return cloned immutable slices and never write to package parts.

- [x] **Step 5: Run focused projection and preservation tests**

Run: `go test ./internal/format/docx -run 'TestWord(LayoutProjection|Projection)' -count=1`
Expected: PASS; snapshot bytes and all package parts remain unchanged by projection.

- [x] **Step 6: Update format support evidence and commit Task 1**

Document precisely which page/style metadata is projected and which section/table cases produce diagnostics. Update the Word TODO and compatibility matrix to show this as read-only projection only and keep pagination/output marked unsupported until downstream tasks pass.

### Task 2: Select and implement an in-process text shaping/font service

**Files:**
- Create: `internal/wordlayout/shaping.go`
- Create: `internal/desktopui/word_text_shaper.go`
- Create: `internal/wordlayout/shaping_test.go`
- Test: `internal/desktopui/word_text_shaper_test.go`
- Modify: `docs/office-library-evaluation.md`
- Modify: `go.mod` and `go.sum` only if the probe selects a new compiled dependency

**Interfaces:**
- Produces `TextShaper` with `Shape(text string, style document.WordTextStyle) (ShapedText, error)` and `FontData(fontKey string) ([]byte, error)`.
- `FontSource` is the narrow PDF-facing interface `{ FontData(fontKey string) ([]byte, error) }`; `TextShaper` implements it.
- `ShapedText` carries a stable font key, ascent/descent, glyph IDs, glyph advances/offsets, and Unicode rune-cluster start/end offsets. Layout consumes point units; no pixel dimensions are stored here.
- `ShapedGlyph` carries glyph ID, rune range, advance X, and X/Y offset, all in points. Implement a deterministic fake shaper in tests; production shaping must remain in-process and usable by the Go PDF backend.
- Probe ruling: the selected `SalvioniDigitalSolutions/gopdf` API does not accept shaped glyph IDs/positions as visible text. The shared page scene must replay vector glyph outlines derived from these glyph IDs and font keys at the supplied positions; use an alpha-zero Unicode text layer for extraction only, after checking that embedding is permitted. The Gio screen painter and PDF writer must consume identical outline geometry and positions.

- [x] **Step 1: Add shaping contract tests**

Add `TestTextShaperReturnsGlyphClustersAndMetrics` in `internal/wordlayout` and the Review Focus regression `TestWordTextShaperMixedCJKLatinMissingGlyphUsesReportedFallback` in `internal/desktopui`. Assert monotonically ordered rune clusters cover the original input exactly once, metrics are finite/nonnegative, and fallback font keys are explicit.

- [x] **Step 2: Run shaping tests before implementation**

Run: `go test ./internal/wordlayout ./internal/desktopui -run 'Test(TextShaperReturnsGlyphClustersAndMetrics|WordTextShaperMixedCJKLatinMissingGlyphUsesReportedFallback)' -count=1`
Expected: FAIL until the shaping package and contract exist.

- [x] **Step 3: Probe available Go/Gio shaping and PDF font requirements**

Build a focused probe for the current Gio/font stack and candidate Go PDF writers (including the candidates recorded in `docs/office-library-evaluation.md`). Verify positioned glyph output, simplified Chinese and mixed Latin visibility/extraction, embeddable licensed font data/ToUnicode, transparent PNG handling, and macOS/Windows/Linux compile behavior. Keep the Gio-dependent production adapter in `internal/desktopui`; `internal/wordlayout` must stay independent of Gio. Do not choose a library based on README claims alone.

- [x] **Step 4: Record and implement the selected production shaper**

Record the measured API, version, license, font source/fallback policy, dependencies, and test evidence in `docs/office-library-evaluation.md`. Implement `Shape`/`FontData` behind the neutral interface in `internal/desktopui/word_text_shaper.go`. If existing Gio APIs cannot provide reusable glyph positions and font data without frame state, add the smallest reviewed pure-Go dependency and keep it inside this adapter.

- [x] **Step 5: Run shaper tests and commit Task 2**

Run: `go test ./internal/wordlayout ./internal/desktopui -run 'Test(TextShaperReturnsGlyphClustersAndMetrics|WordTextShaperMixedCJKLatinMissingGlyphUsesReportedFallback)' -count=1`
Expected: PASS, including the CJK/missing-glyph fallback regression; no external process is started.

### Task 3: Implement the shared page layout engine

**Files:**
- Create: `internal/wordlayout/layout.go`
- Create: `internal/wordlayout/paragraph.go`
- Create: `internal/wordlayout/table.go`
- Create: `internal/wordlayout/layout_test.go`

**Interfaces:**
- Consumes Task 1 neutral `WordLayoutData`, existing `[]document.WordBlock`, and Task 2 `TextShaper`.
- Produces `Engine.Layout(ctx context.Context, input Input) (Result, error)`. `Input` contains document ID, revision, blocks, layout metadata, and product default page setup.
- `Result` contains ordered `Pages []PageScene`, `Diagnostics`, and `Locations map[SourcePosition]Position`. `SourcePosition` is `{RunID string; RuneOffset int}`; `Position` is `{Page int; XPT, YPT float64}`. Each `PageScene` is a neutral Canvas-like vector display list in points: page geometry plus positioned glyph runs, glyph font keys/IDs, images, table cells, clipping, and source ranges. Visible glyphs resolve to outlines once from the selected font face and are replayed at identical origins by the Gio painter and PDF writer; neither sink measures or wraps text again. The scene contains no Gio/PDF/platform drawing objects.
- Diagnostics use a typed severity (`warning`/`error`), stable source ID, code, and user-readable detail. `Result.CanPreview` is false for fatal layout errors; warnings require explicit user confirmation before PDF/print.

- [x] **Step 1: Add deterministic pagination tests using the fake shaper**

Add table-driven tests named `TestLayoutPageSetupAndMargins`, `TestLayoutParagraphRunFormattingAndMixedScripts`, `TestLayoutListsInlineImagesAndPageBreaks`, `TestLayoutOrdinaryTableSplitsBetweenRows`, `TestLayoutMapsRunRuneOffsetsToPage`, and `TestLayoutUnsupportedSectionsBlockOutput`. Assert page count, fragment coordinates/source ranges, order, no duplicate/lost runes, and typed diagnostics.

- [x] **Step 2: Add Review Focus boundary and tall-row tests**

Add `TestLayoutExactPageBoundaryThenExplicitBreakHasNoExtraPage` and `TestLayoutOversizedTableRowReportsOverflowAndTerminates`. Assert no unintended blank page, no unlocated overflow, finite/nonnegative geometry, and bounded layout completion.

- [x] **Step 3: Run the focused layout tests to confirm the missing behavior**

Run: `go test ./internal/wordlayout -run 'TestLayout' -count=1`
Expected: FAIL until the engine exists.

- [x] **Step 4: Implement `Engine.Layout` and paragraph placement**

Implement point-based page boxes/margins, style resolution (`BasedOn` plus direct run/paragraph values), shaping-aware line breaking, supported alignments/indents/spacing/list markers, inline raster dimensions, explicit page breaks, keep controls, source maps at Unicode rune boundaries, cancellation checks, and diagnostics. Empty paragraphs/documents produce valid deterministic pages.

- [x] **Step 5: Implement ordinary table layout and conservative unsupported handling**

Lay out grid/cell widths and cell paragraphs; split only between rows. If a row cannot fit on a fresh page, emit one located overflow diagnostic and advance/terminate without retry loops. Merged/nested structures that cannot be represented accurately emit blocking diagnostics; do not silently flatten them.

- [x] **Step 6: Run focused engine tests and commit Task 3**

Run: `go test ./internal/wordlayout -count=1`
Expected: PASS with deterministic page geometry, source mapping, cancellation, and unsupported-content diagnostics.

### Task 4: Add print-layout view, real page status, and revision-safe background layout

**Files:**
- Create: `internal/desktopui/word_pagination.go`
- Create: `internal/desktopui/word_page_view.go`
- Modify: `internal/desktopui/session_state.go`
- Modify: `internal/desktopui/word_status_bar.go`
- Modify: `internal/desktopui/word_toolbar.go`
- Modify: `internal/desktopui/layout.go`
- Test: `internal/desktopui/word_pagination_test.go`

**Interfaces:**
- Consumes Task 1 `document.WordLayoutReader` and Task 3 `wordlayout.Engine`/`Result`.
- Produces an asynchronous `wordLayoutJob`/`wordLayoutResult` keyed by `{documentID, revision, pageSetupKey}`. Keep view mode, pending job, cancellation, accepted result, active page, and warning confirmation in stable session state.
- `wordPageForCaret(result wordlayout.Result, runID string, runeOffset int) (page int, ok bool)` uses the layout source map; status page labels display one-based indexes only at the UI boundary.

- [x] **Step 1: Add status/page mapping, open-time layout, and stale-job tests**

Add `TestWordPageForCaretUsesRuneSourceMap`, `TestWordStatusShowsLayoutErrorInsteadOfEstimatedPageCount`, `TestWordOpenStartsBackgroundPagination`, and the Review Focus regression `TestWordPaginationIgnoresResultForSupersededRevision`. Assert opening a DOCX schedules layout without blocking the Gio loop, status says calculating until a matching result arrives, no page count is guessed, and a stale result cannot replace a newer revision.

- [x] **Step 2: Implement asynchronous layout lifecycle**

When a DOCX opens, schedule initial pagination in the background. Also flush Word editor drafts through the existing controller path before user-triggered layout, capture immutable blocks/layout metadata/snapshot revision, run `Engine.Layout` off the frame loop, and publish through the app's synchronized result channel. Cancel obsolete jobs and accept results only when all request keys still match.

- [ ] **Step 3: Implement print-layout view from positioned fragments**

Add a print-layout toggle while preserving continuous flow. Replay the accepted `PageScene` at the current zoom into Gio page painting; reuse persistent run editors at mapped fragment positions for supported edits. Virtualize painting to visible pages and do not paginate from `Layout`/frame code.

- [x] **Step 4: Wire `FsQrk` page and zoom status**

Map active run/caret to the one-based current page and result page count; when there is no text focus use the first visible page. Keep word count and existing status controls. Show explicit calculating/unsupported/error states rather than fallback/static values. Keep the `Clpgm`/`kiBim` layout, Excel icon sprite/tints, and `desktop` title unchanged.

- [x] **Step 5: Run focused UI tests and commit the Task 4 slice**

Run: `go test ./internal/desktopui -run 'TestWord(PageForCaret|StatusShowsLayoutError|PaginationIgnoresResult|PrintLayout)' -count=1`
Expected: PASS; UI tests cover page mapping, stale results, and view toggle without depending on wall-clock layout.

Task 4 Step 3 is partial: Gio replays the accepted `PageScene` as a read-only print-layout view and virtualizes pages, but it does not overlay the persistent editors. Keeping the visible page as the engine's vector scene preserves the screen/PDF/print geometry; text editing currently returns to continuous flow. A future editor overlay can be added with a separate draft-layout result and caret scene so in-progress edits also reflow through the same engine rather than replacing positioned glyphs with Gio's independent text shaper.

### Task 5: Add PDF backend and safe asynchronous PDF export

**Files:**
- Create: `internal/format/pdf/word_writer.go`
- Create: `internal/format/pdf/word_writer_test.go`
- Create: `internal/desktopui/word_pdf.go`
- Modify: `internal/desktopui/session_state.go`
- Modify: `internal/desktopui/word_toolbar.go`
- Modify: `internal/desktopui/layout.go`
- Modify: `go.mod` and `go.sum` for the selected writer only
- Modify: `docs/office-library-evaluation.md`, `docs/office-support.md`, and `docs/compatibility/office-feature-matrix.md`

**Interfaces:**
- Consumes Task 3 `wordlayout.Result` and the selected Task 2 font data/shaped glyphs.
- Produces `WriteWordPDF(dst io.Writer, result wordlayout.Result, fonts wordlayout.FontSource) error` in `internal/format/pdf`; PDF types do not escape this adapter.
- UI export captures the committed document revision, requests layout, requires confirmation for warnings, then chooses a `.pdf` destination and atomically writes a temporary sibling file. Existing files remain intact on any error/cancel; DOCX dirty state is unchanged.

- [ ] **Step 1: Add PDF writer geometry and content tests**

Add `TestWriteWordPDFPreservesPageCountAndPlacedGlyphs`, `TestWriteWordPDFEmbedsCJKFontAndToUnicode`, `TestWriteWordPDFPreservesImageRectangles`, and `TestWriteWordPDFRejectsBlockingDiagnostics`. Validate page count, extracted Chinese/mixed Latin boundary text, font embedding/ToUnicode, and image rectangles using a test-only parser/inspector; compare extracted/page geometry with the input layout result.

- [ ] **Step 2: Run PDF tests against the selected library probe**

Run: `go test ./internal/format/pdf -run 'TestWriteWordPDF' -count=1`
Expected: FAIL until positioned page replay and PDF adapter are implemented.

- [ ] **Step 3: Implement in-process page replay**

Implement `WriteWordPDF` by replaying layout page dimensions, glyph IDs/positions, image rectangles, and supported table/paragraph geometry. Embed reviewed fonts/subsets where licensing permits; use reported fallback fonts and `ToUnicode` mappings. Do not reflow text in the PDF writer.

- [ ] **Step 4: Implement atomic export and cancellation/error handling**

Add an export worker that consumes only the committed snapshot revision. Reuse a cached layout only when document ID, revision, and page setup all match; otherwise paginate that snapshot before PDF writing. Write/close/validate in a same-directory temporary file, then atomically replace the target. On cancel/failure remove the temporary file and preserve any existing target and source dirty state. Ignore stale worker results.

- [ ] **Step 5: Run writer/export tests and commit Task 5**

Run: `go test ./internal/format/pdf ./internal/desktopui -run 'Test(WriteWordPDF|WordPDFExport)' -count=1`
Expected: PASS for PDF geometry/content, blocking diagnostics, cancel/failure atomicity, and unchanged DOCX dirty state.

### Task 6: Add native print adapter and user-confirmed print flow

**Files:**
- Create: `internal/printservice/print.go`
- Create: `internal/printservice/print_unsupported.go`
- Create: `internal/printservice/print_darwin.go`
- Create: `internal/printservice/print_darwin_test.go`
- Create: `internal/desktopui/word_print.go`
- Modify: `internal/desktopui/session_state.go`, `internal/desktopui/word_toolbar.go`, and `internal/desktopui/layout.go`
- Modify: `docs/office-library-evaluation.md`, `docs/office-support.md`, and `docs/compatibility/office-feature-matrix.md`

**Interfaces:**
- Produces platform-neutral `Printer` with `Available() bool` and `Print(ctx context.Context, pdfData []byte, options Options) error`; `Options` contains copies and page range.
- macOS adapter receives in-memory PDF bytes, creates PDFKit document data, and invokes AppKit's native print operation on the Cocoa main thread/Gio-compatible event loop. Unsupported platforms return unavailable and do not expose an active ribbon action.
- UI first lays out an immutable committed snapshot and previews it; only a second explicit confirmation invokes the native system print panel. Cancel leaves the document and dirty state untouched.

- [ ] **Step 1: Add print adapter contract and flow tests**

Add `TestWordPrintRequiresSecondConfirmationAfterPreview`, `TestWordPrintReusesMatchingOpenLayout`, `TestWordPrintCancelPreservesDirtyState`, and adapter tests for cancellation, page count, copies/page range, temporary data cleanup, and native API errors. Add a build check asserting unsupported platforms report unavailable.

- [ ] **Step 2: Implement the unsupported-platform adapter first**

Provide build-tagged unavailable implementations so the app compiles everywhere and print remains disabled until an adapter is validated.

- [ ] **Step 3: Implement and verify macOS PDFKit/AppKit bridge**

Use only in-memory PDF bytes and native print APIs. Verify Objective-C bridge requirements, dispatch to Cocoa main thread without blocking Gio's event loop, print panel cancellation/confirmation, virtual printer page count/copies, and cleanup. Do not create a subprocess fallback.

- [ ] **Step 4: Connect the toolbar print flow and validate platform gating**

Flush drafts, reuse a matching current layout or layout the committed snapshot asynchronously, present preview/diagnostics, require warning confirmation, then call the native adapter after user confirmation. Keep Print disabled on any platform lacking passing adapter evidence.

- [ ] **Step 5: Run print flow and platform build checks; commit Task 6**

Run: `go test ./internal/printservice ./internal/desktopui -run 'Test(WordPrint|Print)' -count=1`
Run: `GOOS=darwin GOARCH=arm64 go test -c ./internal/printservice -o /tmp/printservice-darwin.test` and the equivalent current host build.
Expected: PASS/compile for the implemented native adapter; unsupported platform behavior is unavailable. Remove temporary test binaries after validation.

### Task 7: Update Word TODO and complete Computer Use acceptance

**Files:**
- Modify: `docs/word-ribbon-todo-zh.md`
- Modify: `docs/office-support.md`
- Modify: `docs/office-library-evaluation.md`
- Modify: `docs/compatibility/office-feature-matrix.md`
- Evidence: Computer Use screenshots/notes for Pen `Clpgm`, `kiBim`, `FsQrk` and Glance

**Interfaces:**
- Consumes the completed page view, status mapping, PDF writer, and each verified platform adapter.
- Produces feature-status/evidence entries that mark Print, PDF, print-layout, and real page count complete only when each UI path, error path, regression, and visual check passed.

- [ ] **Step 1: Run all targeted Word pagination/PDF/print regressions**

Run the focused commands listed in Tasks 1–6, followed by `go test ./... -count=1` and `go build ./cmd/desktop`. Record any pre-existing unrelated failures separately; do not attribute them to this feature.

- [ ] **Step 2: Launch the product through the required command and capture the app**

Run `go run ./cmd/desktop` from the repository. Use Computer Use to bind the `desktop` window and Pen. Keep the source DOCX unchanged and use the same document, window dimensions, display scaling, and available fonts on both sides.

- [ ] **Step 3: Compare Pen references at matched dimensions**

Compare `Clpgm` primary page, `kiBim` ribbon, and `FsQrk` bottom bar. Verify group/icon positions and Excel sprite/tints; continuous/print layout switching; page margins, multi-page scroll; bottom-bar focus page/total, word count, zoom controls. Use the reference document's expected values only as comparison evidence; derive displayed values from the loaded document and layout result.

- [ ] **Step 4: Verify export/print paths and diagnostics**

Export and reopen a PDF; check page count, extracted CJK/Latin boundary text, font mapping, and picture rectangles. Exercise warning confirmation, unsupported-layout blocking, print preview, system print cancellation/confirmation, and platform gating. Verify source DOCX bytes and dirty state are unchanged by preview/export/print.

- [ ] **Step 5: Record evidence and update support matrix**

Add actual launch command, OS/build target, document fixture, computed page/word counts, fonts/fallbacks, diagnostics, screenshot locations, PDF checks, print target, and remaining differences to the docs. Leave any unverified item unchecked; do not treat startup as 1:1 visual acceptance.

## Self-Review

- **Spec coverage:** neutral page/style/table projection (Task 1); in-process font shaping and library selection (Task 2); pagination, mapping, cancellation and diagnostics (Task 3); page view/status and revision-safe UI flow (Task 4); PDF replay, export atomicity and CJK validation (Task 5); native print and platform gating (Task 6); docs, TODO, required launch command, Pen comparison and acceptance evidence (Task 7).
- **Step scan:** each task establishes a focused failure case before implementation, defines a concrete interface, then implements and verifies one independently reviewable capability. Test commands are planned only; no product tests have been run as part of writing this plan.
- **Type consistency:** Task 1's neutral `WordLayoutReader`, `WordLayoutData`, and point-based `WordPageSetup` feed Task 3 `Input`; Task 2's `TextShaper` feeds Task 3 and PDF; Task 3's `Result`/`SourcePosition` feed Tasks 4–6; Task 4 keys acceptance by document/revision/page setup; Tasks 5–6 share PDF data and diagnostics without leaking backend types.
- **Review Focus:** all five high-risk cases name regressions in the owning task(s); boundary, CJK fallback, unsupported layout, oversized rows, stale results, and atomic export failure each have explicit assertions.
- **Scope/proportion:** pagination is the shared prerequisite for page count, screen, PDF, and print, so these dependent output paths remain in one plan. Library choice and platform bridge are evidence gates, not assumptions. No PDF library or operating-system behavior is presented as verified before its probe passes.
