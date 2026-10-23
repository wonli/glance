# Offline Markdown SVG and Mermaid Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship offline Mermaid flowcharts and SVG images in Markdown preview and PDF, with no separately installed browser or Node.js.

**Architecture:** A lazy process-wide renderer adapter controls an app-bundled headless-shell over Go CDP and returns neutral raster images with logical geometry. Existing resource workers own cancellation, fetching and publication; preview and PDF reuse those images. This plan is independent of `2026-10-23-windows-emoji.md` except for final combined acceptance.

**Tech Stack:** Go, Gio, goldmark, chromedp/CDP, official embedded Mermaid JavaScript, bundled Chromium headless-shell, existing Word/PDF image projection.

**Spec:** `docs/superpowers/specs/2026-10-23-markdown-diagrams-and-windows-emoji-design.md` (approved in chat).

## Global Constraints

- Current checkout only; preserve unrelated work. No new worktrees.
- Targets: Windows amd64 and macOS arm64/amd64. No runtime dependency downloads, PATH browser lookup or Node.js requirement.
- Retain browser sandbox; deny renderer-originated network and local-file access.
- Mermaid source at most 64 KiB; at most 500 edges; 10-second render deadline inside the existing 30-second resource deadline.
- Images at most 32 MiB input and 16 megapixels output; existing 128 resources and 64 MiB retained resource budget.
- Render diagrams at 2x density; carry logical dimensions explicitly. No browser or format-library types in UI/neutral DTOs.
- One active render, bounded queue/LRU, lazy startup, cancellation, child cleanup and recovery after failure.
- Native platform verification is required before claiming that target passed; report unavailable runners.

## Review Focus

- Runtime/sidecar removed or moved: clear failure, never a system-browser fallback (Tasks 1, 7).
- SVG with BOM/comments, forged suffix or a zero/huge viewBox: sniff correctly and bound allocation (Task 3).
- Document configuration, external fonts/images, scripts and nested URLs: no access escapes (Tasks 2, 3).
- A canceled job must not stall the next document or survive shutdown (Tasks 2, 5).
- A 2x raster must not double preview/PDF geometry, and export must not refetch after confirmation (Tasks 4, 6).

---

### Task 1: Verifiable runtime distribution

**Files:** Create `internal/diagramrender/bundle.go`, `bundle_test.go`, `assets/manifest.json`, `assets/mermaid.min.js`, `assets/mermaid-LICENSE`; create `packaging/render-runtime/main.go`, `main_test.go`, `manifest.json`. Modify `go.mod`, `go.sum` only for the chosen CDP adapter.

**Interfaces:** Produce `diagramrender.Bundle { Executable string; Version string; MermaidVersion string }`, `ResolveBundle(executable string) (Bundle, error)`. The packaging command accepts `-target windows-amd64|darwin-arm64|darwin-amd64 -output DIR -manifest FILE`. Build artifact validation must check each selected archive actually supplies its target layout before recording the executable path. Application-relative paths: Windows `render-runtime/chrome-headless-shell-win64/chrome-headless-shell.exe`; macOS `Contents/Resources/render-runtime/chrome-headless-shell-mac-{arm64,x64}/chrome-headless-shell`.

- [ ] **Step 1: Write failing tests** `TestResolveBundleNeverUsesPATH` puts a fake Chrome on PATH and asserts a missing-sidecar error. `TestExtractRuntimeRejectsTraversalAndSymlinkEscape` asserts `../`, absolute names and escaping links cannot write outside staging. `TestRuntimeDigestMismatchDoesNotPublish` asserts the old complete staging directory survives a bad archive. Cover spaces/non-ASCII in the app path and missing runtime data files.
- [ ] **Step 2: Run RED** `go test ./internal/diagramrender ./packaging/render-runtime -count=1 -v`.
- [ ] **Step 3: Implement bundling** Select released upstream versions at execution time, materialize exact immutable URLs, versions, SHA-256 values and notices in manifests before use. Embed the self-contained Mermaid browser bundle (no dynamic CDN imports). Fetch via HTTPS only at build time, verify hashes, validate complete target archive layout and publish a complete staging directory atomically. `ResolveBundle` never downloads and validates required files without hashing a large browser on every render.
- [ ] **Step 4: Run GREEN** Package tests pass. Stage the native runtime under `dist/render-runtime`; verify a clean app-relative lookup and dependency notices. Add a developer test flag `-diagram-runtime EXPLICIT_PATH` in renderer integration tests only; production never reads a developer override environment variable.
- [ ] **Step 5: Commit** `build: stage pinned offline diagram runtime` with exact paths from this task.

### Task 2: Isolated renderer and bounded service

**Files:** Create `internal/diagramrender/{renderer.go,service.go,cache.go,page.go,renderer_test.go,service_test.go,security_test.go}`, `internal/document/resource_image.go`, `resource_image_test.go`.

**Interfaces:** Produce `Request { Kind string; Source []byte; Density int }`, `Result { Image image.Image; LogicalSize image.Point }`, `New(bundle Bundle) *Renderer`, `(*Renderer).Render(context.Context, Request) (Result,error)`, `(*Renderer).Close() error`, `Default() (*Renderer,error)`, `CloseDefault() error`. `Kind` is `svg` or `mermaid`; `Density` is 2. Produce neutral `document.ResourceImage { image.Image; Size image.Point }` with `LogicalSize() image.Point`; `document.ResourceLogicalSize(image.Image) image.Point` returns metadata when available, otherwise pixel bounds. Existing `map[string]image.Image` contracts remain valid.

- [ ] **Step 1: Write failing integration tests** `TestRenderChineseFlowchart`, `TestRenderSVGTextAndForeignObject` require actual pixels and correct logical/raster size. `TestRenderEnforcesLimits` asserts 64 KiB source, 500-edge limit and 16-Mpixel rejection before capture. `TestCanceledRenderAllowsNextJob`, `TestCrashRestartsRenderer`, `TestCloseReapsChildAndProfile`, `TestQueueAndCacheAreBounded` assert real service behavior; no mock successful rendering.
- [ ] **Step 2: Run RED** `go test ./internal/diagramrender -run 'Test(Render|Canceled|Crash|Close|Queue)' -count=1 -v -args -diagram-runtime=EXPLICIT_STAGED_PATH`. Integration mode must fail clearly if a requested runtime is absent, not silently skip.
- [ ] **Step 3: Implement rendering/service** Use chromedp with explicit executable path and fresh profile; keep sandbox flags. Launch lazily, cap outstanding requests at 16 with context-aware queue waits, serialize render work and expire idle browser after 30 seconds. Keep an LRU capped at 32 entries and 32 MiB, accounting for pixel bytes. Cache digest includes input, kind, versions, fixed config, density and platform font identity; never cache errors. Close active contexts on cancel and replace damaged pages. Apply 10-second job deadlines, context checks before/after capture, and bound dimensions before screenshot allocation. `CloseDefault` detaches/cleans the current singleton so a future independently owned runtime may start anew.
- [ ] **Step 4: Implement and verify isolation** Load only trusted embedded HTML/JS with CSP and CDP request restrictions. SVG uses a data-image context; Mermaid uses fixed strict config with secure configuration keys. Validate frontmatter/directives cannot override the fixed policy; reject unsupported resource/config constructs. `TestRendererRejectsExternalRequests` uses an HTTP trap and local sentinel file, attempting script, CSS imports, image/font loads, redirects, WebSocket, file URLs and diagram config overrides; assert zero requests and no sentinel content. Add graph fixtures covering TD/TB/LR, edge labels, decisions, subgraphs and class/style declarations. Run package tests with sandbox enabled; inspect actual output.
- [ ] **Step 5: Commit** `feat: add bounded offline diagram renderer`.

### Task 3: Local and HTTP SVG resources

**Files:** Create `internal/format/markdowndoc/image_decode.go`, `svg_resources_test.go`; modify `resources.go`, `resources_test.go`.

**Interfaces:** Consume Task 2 renderer. Produce `DecodeImage(ctx context.Context, data []byte) (image.Image,error)` internal shared implementation for both local and HTTP paths; existing `LoadImage`, `LoadLocalImage`, `PreviewResourcesContext` signatures remain. SVG results wrap `document.ResourceImage` to preserve logical geometry. Produce a scoped adapter `type ResourceLoader struct` with `NewResourceLoader(provider func() (*diagramrender.Renderer,error)) *ResourceLoader`, `(*ResourceLoader).LoadImage(ctx context.Context, destination, sourcePath string) (image.Image,error)` and `(*ResourceLoader).PreviewResources(ctx context.Context, snapshot document.MarkdownSnapshot, sourcePath string) (map[string]image.Image,map[string]string)`. Public compatibility functions delegate to a default loader using `diagramrender.Default`; integration tests construct a loader with an explicitly staged renderer, avoiding global test hooks or product environment overrides. Provide the loader methods in this task, with existing image/math collection behavior; Task 4 adds diagram dispatch.

- [ ] **Step 1: Write failing tests** `TestLoadLocalSVGWithBOMAndComments`, `TestHTTPImageSniffsSVGRegardlessOfSuffix`, `TestSVGRejectsInvalidAndHugeGeometry`, `TestRasterImagesDoNotStartDiagramRuntime` assert identical PNG/JPEG/GIF behavior and real SVG pixels. Local fixture tests include relative paths, file URLs, spaces, Chinese filenames and Windows drive notation. HTTP tests use the existing test server infrastructure and forged MIME/extension combinations.
- [ ] **Step 2: Run RED** `go test ./internal/format/markdowndoc -run 'Test(LoadLocalSVG|HTTPImage|SVG|RasterImages)' -count=1 -v` using test-local renderer construction from `GLANCE_TEST_DIAGRAM_RUNTIME`, read only in `_test.go` helpers; the test then calls the scoped loader. An integration run requested without a runtime must fail clearly.
- [ ] **Step 3: Implement image decoding** Read local/HTTP bytes with the existing 32 MiB bounds, detect a valid XML SVG root after optional BOM/declaration/comments and reject DTD/entity expansion. Use the renderer for SVG, existing image decoders for rasters, and existing 16-Mpixel and resource budgets. Add `image/svg+xml` to Accept. Keep local-only resource loading from ever fetching HTTP and preserve cancellation; retain the exported `LoadLocalImage` as a background-only wrapper around the context-aware helper.
- [ ] **Step 4: Run GREEN and isolation fixtures** Assert SVG scripts/foreignObject script cannot mutate the trusted page; external nested content cannot fetch or read files. Static foreignObject Chinese labels must be visible. Run `go test ./internal/format/markdowndoc -count=1` and renderer integration tests.
- [ ] **Step 5: Commit** `feat: render SVG Markdown image resources`.

### Task 4: Mermaid parsing and shared resource results

**Files:** Modify `internal/format/markdowndoc/{parse.go,resources.go}`, `internal/document/markdown.go` comments; create `internal/format/markdowndoc/mermaid_test.go` and `testdata/flowchart.md`.

**Interfaces:** A real Mermaid fence emits `MarkdownBlock{Kind:"diagram", Language:"mermaid", Text: exactBody}` with existing source offsets retained. Produce `markdowndoc.DiagramSpan(block document.MarkdownBlock) document.MarkdownSpan` (Kind `diagram`, exact Text). Extend `ResourceKey` with `diagram:mermaid:` plus SHA-256 of source. The Task 3 loader (and its `PreviewResourcesContext` compatibility wrapper) renders that span at density 2 and stores a `document.ResourceImage`.

- [ ] **Step 1: Write failing tests** `TestMermaidFenceBecomesDiagram` checks case-insensitive language, exact source/offsets and unchanged save/reopen source. `TestMermaidExamplesRemainLiteral` covers nested escaped examples, indented code and ordinary fences. `TestMermaidResourcesDeduplicateAndCancel` asserts repeated diagrams share work within a request, source changes get different keys and canceled jobs leave no published images. Verify image/math/diagram keys cannot collide.
- [ ] **Step 2: Run RED** `go test ./internal/format/markdowndoc -run '^TestMermaid' -count=1 -v`.
- [ ] **Step 3: Implement parser/resource routing** Preserve code highlighting for all non-Mermaid code. Reuse four bounded resource workers, 128-resource and 64 MiB budgets and the 30-second job deadline. Failed diagrams retain structured diagnostic strings rather than modifying source. Default renderer is acquired only for a diagram or SVG; ordinary Markdown and raster-only documents must not launch it.
- [ ] **Step 4: Run GREEN** Repeat Mermaid tests with real renderer, then full markdowndoc tests. Verify a standard local flowchart renders with network disabled.
- [ ] **Step 5: Commit** `feat: parse and resolve Mermaid Markdown diagrams`.

### Task 5: Responsive preview and process cleanup

**Files:** Modify `internal/desktopui/{markdown_preview.go,markdown_async.go,markdown_resource_warnings.go,markdown_pdf_export.go}`, `internal/desktopui/runtime_workspace_persistence.go`, `internal/desktoplaunch/cli.go`; create `internal/desktopui/markdown_diagram_visual_test.go`, `markdown_diagram_async_test.go`. Add lifecycle cleanup in the actual desktop and headless CLI ownership exits, before `os.Exit` paths bypass defers.

**Interfaces:** Consume `DiagramSpan`, `ResourceLogicalSize` and the existing generation/cancel/resource-map publication API. `diagramrender.CloseDefault()` is called only at process-level runtime shutdown, never on individual tab/window close while another window owns work.

- [ ] **Step 1: Write failing tests** `TestMarkdownDiagramPreviewLogicalSize` asserts a 2x raster preserves logical layout at 1x/2x and narrows proportionally. `TestMarkdownDiagramErrorsKeepSource` checks syntax/missing-runtime error and original source display. `TestMarkdownDiagramRapidEditsDiscardStaleResults`, `TestMarkdownDiagramTabCloseCancelsWork` check active job cancellation and no stale publish. `TestMarkdownShutdownClosesRenderer` checks the child/profile lifecycle without interfering with another window.
- [ ] **Step 2: Run RED** `go test ./internal/desktopui -run '^TestMarkdown(Diagram|Shutdown)' -count=1 -v` with Metal access where required. Test the ordinary default resolution using a staged sidecar next to a separately built test executable (`go test -c`); resource-worker tests may also pass a scoped Task 3 loader into the internal worker function, without a product-only test hook.
- [ ] **Step 3: Implement preview/lifecycle** Draw diagram resources through shared asset sizing, using explicit logical size; preserve math's existing density behavior and ordinary raster sizes. Show loading placeholder, error plus literal source, and include diagram failures in existing resource warnings. Keep request/source/path/generation guards and native editors intact. Close the shared service at final desktop runtime shutdown and after a headless CLI invocation finishes, including error exits; an individual document cancellation only cancels its job.
- [ ] **Step 4: Run GREEN** Targeted tests and existing Markdown layout/async/IME tests. Inspect actual pixels for Chinese node labels, arrowheads and SVG text at both scales. Run supplied-file acceptance capture and a checked-in local fixture, logging remote outages separately.
- [ ] **Step 5: Commit** `feat: display cancellable SVG and Mermaid previews`.

### Task 6: Frozen diagrams in PDF export

**Files:** Modify `internal/format/pdf/markdown.go`, `internal/format/markdownpdf/export.go`, `internal/desktopui/markdown_pdf_export.go`; extend corresponding `_test.go` files and `internal/application/markdown_pdf_export_test.go`.

**Interfaces:** Existing `MarkdownWordBlocks(snapshot, resources, diagnostics)` signature stays. Recognize diagram blocks/spans; size `ResourceImage` from logical dimensions, then map logical pixels to existing PDF points. Frozen PNG bytes retain the existing `MarkdownImages.ImageBytes` API. Report unavailable diagrams through existing neutral resource warning/error structures, not browser types.

- [ ] **Step 1: Write failing tests** `TestMarkdownDiagramPDFUsesLogicalGeometry`, `TestMarkdownDiagramPDFKeepsFrozenResources`, `TestMarkdownDiagramPDFFailureKeepsPlaceholder`, `TestMarkdownSVGInTablePDF` assert image presence, correct dimensions and no second render/fetch after user confirmation. Verify failed/canceled export retains draft/dirty state and does not overwrite a source or existing PDF.
- [ ] **Step 2: Run RED** `go test ./internal/format/pdf ./internal/format/markdownpdf ./internal/application -run 'TestMarkdown.*(Diagram|SVG)' -count=1 -v`.
- [ ] **Step 3: Implement projection/warnings** Route diagram blocks to image inlines, encode frozen accepted pixels once, scale by logical geometry and existing page bounds. Exempt diagram failures from the formula-fatal branch and emit explicit placeholders plus confirmable resource warnings. Generalize the missing-resource warning collector to visit diagrams as well as image/table spans without changing published field meanings silently. Preserve all revision/draft guards.
- [ ] **Step 4: Run GREEN and reopen PDF** Repeat tests, run `go test ./internal/format/pdf ./internal/format/markdownpdf -count=1`, render/reopen output and visually inspect readable Chinese nodes/arrows and absence of 2x oversizing. Verify both desktop and headless application export paths release renderer ownership.
- [ ] **Step 5: Commit** `feat: include frozen Markdown diagrams in PDF exports`.

### Task 7: Complete release packages and compatibility evidence

**Files:** Modify `Makefile`, `packaging/macos/Info.plist` only if needed; create `packaging/render-runtime/package_test.go`, `docs/markdown-diagrams-verification.md`; update existing architecture/support docs with localized additions preserving prior work.

**Interfaces:** Build tooling from Task 1 stages a complete runtime. Add `windows-portable` (EXE plus sidecar/notices ZIP); make macOS app/portable targets include full runtime under Resources. Existing standalone EXE is a developer artifact with an explicit missing-runtime diagnostic. No OS font install or Node installation step.

- [ ] **Step 1: Write failing package checks** `TestWindowsPortableIncludesRuntimeAndNotices`, `TestMacAppIncludesRuntimeAndExecutablePermissions` inspect complete archive layout including runtime data/helper binaries. `TestPackageFailsWhenRuntimeIsIncomplete` asserts no distributable artifact is published. Cover non-ASCII install paths and read-only application directories with writable temporary profiles.
- [ ] **Step 2: Run RED** `go test ./packaging/render-runtime -count=1 -v` on generated fixture archives and smoke-build native package.
- [ ] **Step 3: Implement staging/package targets** Include all pinned runtime resources and notices, preserve executable permissions, stage atomically and document nested macOS signing order. Use app-relative resolution only. Do not introduce runtime downloads or developer-machine Chrome fallback.
- [ ] **Step 4: Verify combined release** Windows amd64 and macOS arm64/amd64 clean-profile runs: no Node/system browser, network disabled, local SVG and Mermaid fixture preview/PDF. Run Windows emoji plan acceptance in the same Windows bundle. If a native runner is unavailable, label that target unverified and retain concrete build outputs/checklists. Run `go test ./...` once after focused checks; record failures by name and baseline-check plausible regressions. `git diff --check` must pass.
- [ ] **Step 5: Commit docs/package changes** `build: ship offline Markdown rendering runtime`; record asset versions/digests, package sizes, actual native coverage, known diagram limits and source-file acceptance screenshots. Request whole-change review before integration, preserving unrelated staged/unstaged work.
