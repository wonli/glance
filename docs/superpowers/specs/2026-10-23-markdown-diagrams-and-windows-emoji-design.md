# Markdown diagrams and Windows emoji

Date: 2026-10-23
Status: Written design for review; implementation has not started.

## Intent and approved scope

The user requested Windows emoji support, SVG images in Markdown, and Mermaid diagrams. The user approved the first-release Mermaid scope and requires offline operation without separately installing Node.js or a browser.

Success means that a distributed Glance build displays common emoji on Windows, renders standard fenced `mermaid` flowcharts and local SVGs offline, and includes those diagrams in Markdown PDF exports. HTTP SVG images use the existing explicit image-fetch path and naturally still require their source to be reachable. The supplied `Downloads/complex_markdown.md` references a remote SVG rather than a Mermaid fence; both representations must be verified separately.

Use the current checkout. Preserve all existing uncommitted work, including the previous macOS emoji fix and unrelated proofreading documentation.

## Existing behavior and root causes

- Gio v0.10.3 can draw outline and bitmap glyphs. Its current text renderer does not draw go-text `GlyphColor` COLR paint data. Adding `Segoe UI Emoji` to a family list alone therefore does not establish Windows color emoji support.
- `fontfallback.PreferredTypeface` now names platform emoji fonts explicitly to avoid Gio's missing script forwarding to fontscan. The macOS bitmap path was verified at 1x and 2x; those tests do not prove Windows support.
- `markdowndoc.Parse` currently emits every fenced block as code. The neutral block already carries its language and source text.
- `PreviewResourcesContext` already runs outside Gio layout, limits the job to 30 seconds, uses four workers, bounds resource count to 128 and retained pixels to 64 MiB, and supports cancellation.
- Local and HTTP image loading accept raster images. `image.Decode` does not decode SVG. `oksvg`, used for small application icons, is insufficient to establish document-SVG CSS, text and foreignObject compatibility.
- PDF projection already converts decoded images into neutral Word image inlines. Diagram output should enter that same resource path.
- Current macOS and Windows packaging does not include a diagram runtime. An installed browser or a developer machine's Node environment must not silently satisfy release acceptance.

## Chosen architecture

Keep Gio UI → application/controller → neutral document model → format adapters → local resources. Add a small renderer adapter used by Markdown resource workers, with no browser, CDP or font-library types exposed to UI widgets or neutral DTOs.

Use the official Mermaid JavaScript distribution and a bundled Chromium headless-shell runtime, controlled directly from Go over CDP. Node.js is needed neither at runtime nor for end-user installation. The same worker rasterizes SVG image bytes using the browser's SVG implementation. The renderer returns a decoded image and neutral logical dimensions, or a categorized error.

This reuses one packaged browser for Mermaid's layout and for document SVG fidelity. A custom Go flowchart engine was rejected because it would implement only an undocumented Mermaid subset. Relying on a machine-installed browser was rejected because it violates the accepted distribution requirement. Using the icon SVG parser for document images was rejected because the supplied diagram's text/CSS must actually render.

Initial packaged targets: Windows amd64 and macOS arm64/amd64, matching the existing desktop release targets. Other targets must not claim packaged diagram support until their runtime bundle and acceptance tests exist.

## Windows emoji

Bundle the static CBDT/CBLC `NotoColorEmoji.ttf` variant, not the COLRv1 variant, with its upstream OFL license and provenance. Register it under a private app family so resolution cannot accidentally select a same-name system font. On Windows, prefer that family for emoji fallback before Segoe UI Emoji while retaining existing Latin/CJK text preference. macOS keeps its tested Apple Color Emoji path.

Parse the bundled font once in managed startup/background work. Share immutable parsed font data; construct per-shaper faces because font face state is not thread-safe. Every production Gio shaper factory must retain the bundled fallback, including catalog-driven shaper replacement and transient measurement shapers. Font-picker updates must not remove it. Keep bundled font memory accounting explicit and avoid copying its bytes per window or frame. It is an application rendering resource, not a font installed in the OS or a document formatting mutation.

Native Gio editor selection, scrolling and IME remain in charge of text interaction. The change does not replace emoji text with manually positioned images. Verify single emoji, variation selectors, skin tones, a family ZWJ sequence, a flag sequence and keycaps, in normal/bold/italic and monospace contexts, alongside Latin/CJK text. Unsupported sequences must be documented from observed results rather than silently claimed supported.

This scope covers Gio text rendering. Word/PDF text uses a different shaping/output pipeline; adding the bitmap font does not establish color emoji PDF text export. Diagram images embedded in PDF may contain emoji because they are raster output.

## Mermaid parsing and presentation

Recognize fenced blocks whose language token is `mermaid` (case-insensitive). Preserve exact source and offsets and represent the block with a neutral diagram kind. Ordinary, indented and escaped example code remains literal.

The preview displays the rendered diagram at its logical size, constrained proportionally to the available width. Render at 2x density for normal/Retina previews and PDF embedding, with explicit pixel-to-logical conversion. Do not infer document size directly from raster pixel dimensions. An in-progress diagram displays a stable loading placeholder. A syntax/render failure displays an error and the original diagram source, retaining a useful document preview.

First-release acceptance covers `flowchart` and `graph`, TD/TB/LR directions, nodes, arrow/edge labels, decision diamonds, branches, Chinese labels, subgraphs and basic style/class definitions. Other official-engine diagram types may work but are not advertised until tested. Mermaid node callbacks, embedded applications and HTML execution are not product features.

## SVG image loading

Extend both relative/local-file and HTTP/HTTPS image paths. Detect SVG from a bounded, parsed XML root and namespace after optional BOM/XML declaration/comments; do not trust extension or MIME type alone. Preserve PNG/JPEG/GIF behavior. Advertise SVG in the image Accept header.

Read bytes through existing size and source-path boundaries. Rasterize the SVG in an image context rather than injecting its XML into the trusted Mermaid renderer page. Allow static text/CSS/gradients/clipping/markers and text-bearing foreignObject content to the extent supported by that image context; prove the supplied fixture visually. Do not execute SVG scripts, document events or remote/nested resource fetches. Embedded data images may be allowed within the same byte/pixel budget. External fonts, local-file references and external images are unavailable inside a document SVG.

Resolve intrinsic geometry using width/height and viewBox, reject invalid or excessive dimensions, and preserve aspect ratio. Share an internal byte-decoding helper for local and network images so format checks, limits and diagnostics cannot drift.

## Renderer lifetime, isolation and bounds

Own the renderer as a lazily started managed service, reusable across document previews and PDF exports. Start on first SVG/Mermaid job, not during ordinary Markdown startup. Serialize initial rendering (one active tab/job) to bound memory; resource workers may queue jobs with their cancellation contexts. No parsing, browser startup or screenshots run inside Gio layout/frame callbacks.

The service receives only bounded diagram text or already-read image bytes, never arbitrary navigation URLs or file paths. Load trusted embedded HTML/JS locally. Keep browser network access denied, including redirects, WebSocket and nested resources. Mermaid runs with strict security settings and fixed application-owned configuration; document directives cannot relax limits or security. Disable external image/icon packs and custom CSS resource loads. Treat library configuration as trusted only after validation.

Retain the browser sandbox. Use a fresh temporary profile, disable unnecessary background features, expose CDP only through a local/private transport, and never publish a network-accessible debug endpoint. Shut down/reap the child and remove the profile at runtime shutdown. A canceled active render closes its job context; crash or render timeout replaces the damaged context before the next job.

Bounds: Mermaid source at most 64 KiB, at most 500 edges, 10-second per-job deadline within the existing 30-second resource deadline, existing 32 MiB input limit for images, at most 16 megapixels per raster output, 64 MiB retained resource budget. Reject before allocating oversized screenshots. Bound the queue and use a bounded LRU for successful diagram results; cache identity includes content digest, renderer/Mermaid version, configuration, density and font identity. Do not retain canceled jobs, document-specific failures or unlimited source strings globally.

## Resource and PDF integration

Keep UI state and resource-map values neutral. Extend resource identity so Mermaid source cannot collide with math or image destinations. Include density and logical geometry in diagram cache results; update preview/PDF consumers explicitly rather than doubling displayed size when using 2x rasters.

Reuse the existing source/path/generation checks when publishing asynchronous results. Closing a document or superseding its draft cancels its jobs and prevents late publication. The PDF export must use its accepted/frozen resource result instead of rendering again after confirmation. A missing/invalid diagram becomes a labeled placeholder and a resource diagnostic, using the same confirmation/error behavior as existing unavailable resources. Cancellation and save failure preserve the document draft and existing files.

## Distribution and provenance

Embed the version-pinned Mermaid bundle and Windows bitmap font with licenses and a manifest. Stage platform-specific headless-shell executable, resource data, helper binaries and notices as a complete directory, not just one executable. Resolve only the app-relative bundled runtime in production; do not fall back to PATH, a system Chrome or a runtime download.

For macOS, place the runtime under app Resources and keep the executable/helper permissions and nested signing requirements. For Windows, produce a portable ZIP containing the EXE plus its runtime directory and notices; an EXE alone is no longer a complete Mermaid-enabled distribution. Preserve the existing EXE build as a developer artifact with an explicit missing-runtime diagnostic if launched without the sidecar.

Build-time fetch tooling selects exact versions per supported target, verifies checked-in SHA-256 pins, rejects unsafe archive paths and fails packaging when any required file is absent. Pin versions and record upstream license/provenance before checking in or using assets. A release build may fetch verified dependencies; the installed application must never fetch runtime dependencies on first launch. Do not distribute an unverified or partly downloaded runtime.

## Validation and evidence

1. Add meaningful failing tests before production changes: Windows-compatible bundled glyph data and fallback selection; SVG load rejection; Mermaid fences currently treated as code; missing diagram PDF output.
2. Test actual renderer output using the shipped runtime, not a mocked successful image: Chinese flowchart, branches/subgraph, SVG text/CSS/markers/foreignObject, source errors, empty/huge geometry, cancellation and restart.
3. Verify no diagram-initiated network requests, local-file reads or script execution for adversarial fixtures. Keep the renderer sandbox enabled during tests.
4. Run the supplied `complex_markdown.md` acceptance preview and a local-only equivalent so a remote SVG outage does not mask offline correctness. Add a separate real Mermaid-fence fixture.
5. Check preview at 1x/2x, resizing, rapid edits and tab closure; export/reopen PDF and verify diagrams and readable labels.
6. Verify Windows emoji with the exact embedded font on macOS via an isolated collection, then Windows GPU and editor interaction tests. Cross-compilation is useful but is not Windows runtime verification.
7. Smoke-test packaged Windows amd64 and both macOS architectures in environments without Node/Chrome, with network disabled for local fixtures. Record any unavailable native target rather than claim completion for it.
8. Run appropriate package tests and `git diff --check`. The earlier full-suite failures are recorded in `docs/emoji-rendering-verification.md`; report remaining failures and baseline-check those plausibly affected by this work.

Update `docs/architecture.md`, `docs/office-support.md` and a focused verification report with observed support and runtime/package implications. Do not upgrade PDF emoji-text or arbitrary SVG/Mermaid compatibility claims without evidence.

## Review boundary

This written design follows the user's approved first-release direction. The brainstorming workflow requires written-design approval before creating the implementation plan, followed by plan review and selection of execution method. No runtime dependencies, assets or product code are added by this document.

## Primary references

- https://github.com/googlefonts/noto-emoji — bitmap font variant and license provenance.
- https://learn.microsoft.com/en-us/windows/win32/directwrite/color-fonts — Windows color font formats.
- https://mermaid.js.org/config/usage — official Mermaid API and strict configuration.
- https://developer.chrome.com/docs/chromium/headless — headless browser operation.
- https://chromium.googlesource.com/chromium/src/+/main/headless/README.md — headless-shell distribution model.
- https://github.com/chromedp/chromedp — Go CDP adapter; use explicit bundled executable path.
